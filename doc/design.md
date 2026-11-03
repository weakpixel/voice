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
| `voice/ebiten` (nested module) | `Player` on an Ebiten audio context, example program |
| `voice/cmd/voiceplay` | renders to WAV for listening |

## The `Sink` seam

Composing and sound meet only in `voice.Sink`. The core never sees a sample rate or an audio device, and a test can record the notes instead of hearing them. Notes are float semitones so random pitch variation is not rounded to the piano. Owners keep speakers apart: stopping one line releases only that owner's notes.

## Languages and voices

Both file formats are plain JSON decoded with unknown fields rejected, then validated in one pass that collects every error with its JSON path. Defaults are applied while building, so the in-memory `Language` and `Voice` never carry "unset" values. A `Language` is opaque and immutable (a trie is built from it), so `DefaultLanguage()` can be shared; a `Voice` is a plain struct because games tweak voices per character.

Instruments in a voice file are `voice.InstrumentSpec`, a flat description in milliseconds. The root package cannot import `synth` (it imports the root), so `synth.PatchFromSpec` converts specs to patches, and sample names are resolved by a caller-supplied loader: the core never opens files on its own, which keeps it usable with `embed.FS`, archives and web builds alike.

## Parsing

A prefix tree keyed by folded runes holds both phonemes and punctuation, so one walk finds the longest symbol of either kind. Folding maps a rune to the smallest member of its `unicode.SimpleFold` orbit, so `K`, `k` and the Kelvin sign meet. Whitespace runs are collapsed into one pause before the tree is consulted. The tokens' byte spans tile the text, which is what the reveal needs and what a fuzz test checks.

## Composing

Composition is a pure function of voice, text and tokens. The only randomness is a PCG generator seeded with the voice seed xor an FNV-1a hash of the text, so a line always sounds the same and two lines of one character differ. Times are computed in float seconds and converted per event, so rounding does not accumulate. Golden timelines of the starter voices in `testdata/compose/` catch unintended changes (`go test -run ComposeGolden -update` rewrites them).

Reveal follows the tokens, never a separate character timer: a token's text appears when it starts, so the typewriter is always in step with the sound, and punctuation shows before its pause.

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

**Filter.** A trapezoidal (TPT) state-variable low-pass after Andrew Simper. Unlike the classic Chamberlin form it stays stable at high cutoffs and high resonance; resonance is capped internally at 0.98. Coefficients are computed once per note.

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

All cases but `Say` allocate nothing. A dialogue line uses two to four voices at a time (up to about twelve for `divine` with its chords and overlapping releases), so the synthesizer costs well under 2 % of a core for the light voices and about 3 % native, 11 % in a browser, for the heaviest.

## Known limits

- `NoteOff` matches owner and pitch, not the instrument. A phoneme note at exactly the pitch of a sounding motif note would release both; with any pitch variation this does not happen. The planned `TimedSink` or the `Speaker` can carry a note id if it matters.

## Not built yet

- A version tag of the root module: until then the `ebiten` module reaches it through `replace => ..` and is not `go get`-able on its own.
- Sample-accurate scheduling: notes start at the next 64-frame block. An optional `TimedSink` with time offsets can add it without changing `Sink`.
- A SoundFont backend in a nested `sf2` module (optional).
- Chorus, delay, FM, a reverb per speaker.
