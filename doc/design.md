# Design

How the library is built and why. The user-facing behavior is in [guide.md](guide.md).

## Packages

The core module (`github.com/weakpixel/voice`) uses the standard library only; `deps_test.go` checks `go.mod` and every import. Ebiten and SoundFont support will live in nested modules, so a program that uses neither never sees them in its `go.sum`.

| Package | Role |
| --- | --- |
| `voice` | `Sink`, `Owner`, `Instrument`, languages, voices, starter voices, `Parse`, `Compose`, `Speaker` |
| `voice/synth` | the synthesizer, implements `voice.Sink` |
| `voice/wav` | WAV in and out |
| `voice/voicetest` | `RecordingSink` |
| `voice/ebiten` (nested module) | `Player` on an Ebiten audio context, `dialoguebox` (the dialogue box game), example program, `cmd/say` (speaks a line on the audio device through oto) |
| `voice/cmd/voiceplay` | renders to WAV for listening |
| `voice/editor` (nested module) | the Voice Editor web site, see [Editor](#editor) |

## The `Sink` seam

Composing and sound meet only in `voice.Sink`. The core never sees a sample rate or an audio device, and a test can record the notes instead of hearing them. Notes are float semitones so random pitch variation is not rounded to the piano. Owners keep speakers apart: stopping one line releases only that owner's notes.

## Languages and voices

Both file formats are plain JSON decoded with unknown fields rejected, then validated in one pass that collects every error with its JSON path. Defaults are applied while building, so the in-memory `Language` and `Voice` never carry "unset" values. A `Language` is opaque and immutable (a trie is built from it), so `DefaultLanguage()` can be shared; a `Voice` is a plain struct because games tweak voices per character.

Instruments in a voice file are `voice.InstrumentSpec`, a flat description in milliseconds. The root package cannot import `synth` (it imports the root), so `synth.PatchFromSpec` converts specs to patches, and sample names are resolved by a caller-supplied loader: the core never opens files on its own, which keeps it usable with `embed.FS`, archives and web builds alike.

## Parsing

A prefix tree keyed by folded runes holds both phonemes and punctuation, so one walk finds the longest symbol of either kind. Folding maps a rune to the smallest member of its `unicode.SimpleFold` orbit, so `K`, `k` and the Kelvin sign meet. Whitespace runs are collapsed into one pause before the tree is consulted. The tokens' byte spans tile the text, which is what the reveal needs and what a fuzz test checks.

## Composing

Composition is a pure function of voice, text and tokens. The only randomness is a PCG generator seeded with the voice seed xor an FNV-1a hash of the text, so a line always sounds the same and two lines of one character differ. Times are computed in float seconds and converted per event, so rounding does not accumulate. Golden timelines of the starter voices in `testdata/compose/` catch unintended changes (`go test -run ComposeGolden -update` rewrites them).

**Endings.** A line used to stop dead: its last syllable was as short as any other, its note had the usual short release, and the motif ended on the same frame. Speech slows down and softens at the end of a phrase and a melody lets its last note ring, so Compose lengthens the last syllable (1.5×, 1.25× before a mid-line sentence end), fades the last three phonemes and holds the motif's last note for the voice's `breath`. This lives in Compose, not in the tools or the synthesizer, so every caller gets the same ending, and it is part of `Duration`, so a game that waits for `Completed` does not cut the ring. The reveal is not delayed: the text is complete when the voice is, and the breath is only sound. On a beat grid the lengthening rounds to whole steps and the breath ends on a beat.

Reveal follows the tokens, never a separate character timer: a token's text appears when it starts, so the typewriter is always in step with the sound, and punctuation shows before its pause.

**Rhythm.** A voice with a rhythm reuses everything above (tokens, pitch, contours, motif, the random draws in the same order) and changes only when notes start. The unit on the grid is the syllable, found from the phoneme categories alone, so every language can be rapped without new data: a vowel is a nucleus, and consonants between vowels are split so the last one leads into the next nucleus, which is how a listener hears the beat (on the vowel, with the consonant just before it). A syllable's length in steps comes from its natural length at the voice's `speed`, so dense and sparse flows are a matter of `speed` against `bpm`. Onsets borrow at most half of the shortest step (shorter with swing) before the nucleus, so a nucleus always keeps half a step. Accents are a lookup by step within the bar, not stress from the text. Rests snap to the grid; sentence ends wait for the next bar by default because rap lines are phrased in bars. The percussion is part of the timeline (a fixed note per sound on instruments after the motif), so `Speaker`, reveal, `Skip` and `Release` work unchanged. A `Clock` only moves the grid's origin to a bar line of the game's music and replaces the tempo; there is no tempo tracking within a line.

**Instruments.** The `Sink` sees 13 instruments per owner: the five voice instruments and eight percussion slots. `NoteOff` names the instrument as well as the pitch, because percussion plays fixed pitches that a motif note (whole semitones from the base note) can share.

## Speaker

The speaker turns a timeline into a schedule of Sink calls (offs before ons at equal times) when a line starts, then only walks two cursors per `Update`: the schedule and the reveals. That is why steady-state `Update` allocates nothing, and why the reveal depends only on accumulated time, never on the update rate. `Say` allocates (parsing reuses the speaker's token slice; composing builds a new timeline).

Every exit path (`Skip`, `Stop`, `Pause`, `Close`, a `Say` over a running line) calls `Release(owner)` rather than the remaining note offs: one call, and nothing can be missed. `Pause` releases too, because a sustained motif note would otherwise hang while time stands still.

Events of `Say`, `Skip` and `Stop` can happen between updates. `Update` drops only the events the previous `Update` returned, so a game that calls `Say` and then `Update` still sees `Started`, and a game that reads events right after `Say` sees it too.

Notes reach the synthesizer at the next 64-frame block after `Update`, and the player's buffer adds its length (50 ms by default in the Ebiten player). The reveal is not delayed to match: at 60 Hz the text runs at most about three frames ahead of the sound, which reads as in step.

## Synthesizer

**Render blocks.** The stream renders in blocks of 64 frames. Commands from the `Sink` methods are applied at the start of a block, and glide and vibrato are evaluated once per block (750 Hz at 48 kHz), which is smooth for pitch movements of a few milliseconds and saves an `exp2` per frame.

**Double buffer.** The `Sink` methods append a command to a slice under a mutex. At each block the renderer swaps that slice for its own empty one, so the lock is held for the swap only and the caller never waits for rendering. Nothing is dropped. Once both slices have grown to the busiest block nothing allocates; the alloc test checks `Read`, `Render` and the `Sink` methods.

**Voice pool.** A fixed pool (32 by default, since chord layers take a voice each) avoids allocation per note. Stealing takes the oldest releasing voice first (it is fading anyway), then the oldest sounding one. A stolen voice fades out over 2 ms and the new note waits for the fade: cutting to zero would click, and 2 ms of delay is inaudible next to the block and player latency.

**No clicks.** Every jump in level clicks, so: attack, release and a falling decay last at least 2 ms; a note off during the attack waits for the peak (also making on and off in one block audible); one-shot samples fade over their last 2 ms; steals fade.

**Oscillators.** Phase accumulators from 0 to 1. Saw and square use PolyBLEP, a two-sample polynomial correction at each discontinuity that removes most of the aliasing at the cost of a few multiplications. The triangle's harmonics fall off fast enough that a naive one is fine. Sine and triangle start at 0. Noise is a per-voice xorshift whose seeds come from a stream-level generator, so the same sequence of notes always renders the same samples.

**Filter.** A trapezoidal (TPT) state-variable filter after Andrew Simper. Unlike the classic Chamberlin form it stays stable at high cutoffs and high resonance; resonance is capped internally at 0.98. Coefficients are computed once per note. One step yields the band-pass and low-pass outputs, and the high-pass is the input minus both (the band-pass weighted by the damping), so the three types cost the same.

**Layers and unison.** A chord layer is a full voice: its own envelope, filter and steal handling, so a stolen layer fades like any note. Each layer remembers the played note, so a `NoteOff` of that note ends the whole chord and the `Sink` contract is unchanged. Unison copies live inside one voice (a fixed array of seven oscillators, so no allocation), share its envelope and filter, start at random phases from the seeded generator and are mixed at 1/√n, because detuned copies add up like noise, not like one wave.

**Reverb.** One stereo Freeverb (Jezar's public-domain design) per stream: eight low-pass feedback combs and four allpasses per channel, with Freeverb's delay lengths scaled to the stream's rate and the right channel 23 samples longer for width. Voices send a mono signal into a bus with their patch's `Reverb` level; the reverb adds its wet output to the mix. It is skipped while no patch sends and it has died away (no input for longer than its longest delay path and output below 10⁻⁶), which also keeps its filters out of slow denormal numbers. One reverb per stream rather than per speaker was chosen for cost: a game scene is one room. Its delay lines (about 70 KB at 48 kHz) are allocated once in `NewStream`.

**Output.** The voices and the reverb are summed, scaled by the master gain and hard-clipped to -1 to 1. Dialogue blips rarely stack loud enough to clip; the clip is a guard, not a limiter.

## Measured costs

`make bench` and `make bench-wasm`, 2026-11-03, Intel Xeon @ 2.10 GHz (cloud container), 48 kHz, one `Read` of 1024 frames (21.3 ms of audio). Patches use a detuned second oscillator; rt% is the share of real time one core spends rendering. js/wasm runs under Node 22.

| Case | native | rt% | js/wasm | rt% |
| --- | --- | --- | --- | --- |
| silent | 6 µs | 0.03 | 60 µs | 0.28 |
| 4 voices | 94 µs | 0.44 | 344 µs | 1.6 |
| 8 voices, filter | 184 µs | 0.86 | 626 µs | 2.9 |
| 24 voices, filter | 508 µs | 2.4 | 1.7 ms | 8.0 |
| 4 voices, reverb | 146 µs | 0.68 | 566 µs | 2.7 |
| 24 voices, filter, reverb | 586 µs | 2.7 | 2.2 ms | 10.4 |
| 12 voices, filter, reverb, unison 5 | 676 µs | 3.2 | 2.3 ms | 10.9 |
| NoteOn + NoteOff, applied | 89 ns | | 330 ns | |
| `Speaker.Update` at 60 Hz (a new line whenever one ends) | 64 ns | | 357 ns | |
| `Speaker.Say`, 36 characters (20 allocations) | 8.6 µs | | 43 µs | |
| `Speaker.Say`, 36 characters, `flow` (rhythm and percussion, 36 allocations) | 21.5 µs | | 137 µs | |

All cases but `Say` allocate nothing. A dialogue line uses two to four voices at a time (up to about twelve for `divine` with its chords and overlapping releases), so the synthesizer costs well under 2 % of a core for the light voices and about 3 % native, 11 % in a browser, for the heaviest.

## Editor

The Voice Editor ([editor.md](editor.md)) is a nested module so the core stays free of oto and of anything browser-specific. All its logic is in `editor/session`, which knows no browser and is tested natively; `editor/wasm` only maps it to `syscall/js` and starts the audio.

**The document is the model.** The editor edits the `*.voice.json` text itself and loads it with `voice.LoadVoice` after every change, so the library is the only validator and an exported file is exactly what the user wrote. There is no encoder from `Voice` back to JSON: loading fills missing instruments with the vowel's and adds the built-in percussion a beat names, so a re-encoded voice would not be the file the user wrote.

**The form is data.** The Visual tab is generated from `editor/web/fields.json`: each field's path, kind, range, default and help, and what each dropdown choice does. The help is the field's tooltip, written for someone who does not know synthesizers; a test checks that every field and every choice has one. Its ranges are the library's: a Go test (`editor/session`) loads a voice with every numeric field at its minimum and maximum and just outside both, expects the problem at that field's path, loads every dropdown option, and checks that every field of `voice.InstrumentSpec` has an entry, so a range changed in the core fails the editor's test instead of drifting. Edits go through `format.js`, which inserts a new key where the guide's order puts it and writes the starter voices' style; a node test formats every starter back to its file byte for byte.

**Playback is the game's.** A test line plays on a `synth.Stream` through a `voice.Speaker`, driven by the page's animation frames, so typewriter, Skip, Pause and the release of every note behave as in a game. The stream plays through oto, the library under Ebiten's audio: Ebiten's audio context only opens the device from a running game loop, which a page with a form does not have (the same reason `say` uses oto). oto's js backend feeds an `AudioWorkletNode` from the main thread, so it needs no `SharedArrayBuffer` and no COOP/COEP headers, which GitHub Pages cannot send. Its device buffer and the player's read-ahead are 50 ms: the stream applies notes at the next block it renders, so the read-ahead is how late a note can start. A valid edit sets the voice's patches on the stream at once, so an instrument change is heard on the next notes of a playing line.

**The picture is rendered offline.** The piano roll and the waveform come from a second render of the line on its own stream (`session.LineRender`, the same Speaker driven block by block, plus the tail until the reverb is quiet), which is also what the WAV download writes. That render costs about a second for a long `divine` line in a browser, so the page runs it in slices between animation frames (10 ms per frame, 3 ms while a line plays) instead of blocking; a newer edit replaces a render in progress. Measured: a 70-character `divine` line renders in about 1.6 s of wall time with no task over 50 ms. Auditions (Hear) play a fixed phrase, the one `voiceplay -patches` plays, on the live stream, advanced by the same frame loop as a line.

**What the page keeps.** Sample files live in the browser's IndexedDB by file name (the name a voice file uses), decoded into the session when the page starts; the voice document, the language text and the settings live in local storage. A share link carries the document itself, deflated (`CompressionStream("deflate-raw")`) and base64url-encoded after `#v=`, so no server is involved and the fragment never reaches GitHub Pages; a starter voice gives a fragment of 400 to 600 characters.

**The dialogue game is a second wasm in an iframe.** The Dialogue mode plays a dialogue in the same box as `ebiten/example` (`ebiten/dialoguebox`), built as `editor/game` into `game.wasm` and shown in an iframe of `game.html` on the same origin. It cannot share `voice.wasm`: Ebiten's js backend makes its own canvas and runs its own loop, and its audio creates an oto context, while oto allows one context per process and `voice.wasm` already has one. A second program has its own Go runtime and `AudioContext`, and talks to the page with `postMessage` (the messages are listed in `editor/game/main.go`). Only `game.wasm` links Ebiten (the editor module requires it with a `replace` to `../ebiten`); a test in `editor` checks that `session` and `wasm` import none, so `voice.wasm` stays 1.4 MB gzipped while `game.wasm` is 17 MB, 4.0 MB gzipped, and is fetched only when the mode is first shown. The game loads the dialogue with the library's own `dialogue.Load` and `LoadVoices` (`editor/game/load` turns the page's request into script, voices, language and samples, with problems by JSON path, and is tested natively), so it plays exactly what a Go game loads. Ebiten opens its audio only on input inside its canvas; a click on the page does not (probed in headless Chromium with and without `--autoplay-policy=no-user-gesture-required`), so the game shows "Click to start sound" until the first click.

Measured in headless Chromium (2026-11-05, same container as below), a 107-character line keeps the page's main thread busy 12 % of the time with `soft` and 15 % with `divine` (about 2 % with the page idle and the stream silent), with no task over 50 ms. The site is 5.1 MB, the wasm 1.4 MB gzipped.

## Not built yet

- A version tag of the root module: until then the `ebiten` module reaches it through `replace => ..` and is not `go get`-able on its own.
- Sample-accurate scheduling: notes start at the next 64-frame block. An optional `TimedSink` with time offsets can add it without changing `Sink`.
- A SoundFont backend in a nested `sf2` module (optional).
- Chorus, delay, FM, a reverb per speaker.
- Sung (melodic) rhythmic lines, word stress from the text, and tempo changes within a line or tempo tracking of the game's music.
