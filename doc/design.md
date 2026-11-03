# Design

How the library is built and why. The user-facing behavior is in [guide.md](guide.md).

## Packages

The core module (`github.com/weakpixel/voice`) uses the standard library only; `deps_test.go` checks `go.mod` and every import. Ebiten and SoundFont support will live in nested modules, so a program that uses neither never sees them in its `go.sum`.

| Package | Role |
| --- | --- |
| `voice` | `Sink`, `Owner`, `Instrument`, languages, voices, starter voices, `Parse`, `Compose`; later the `Speaker` |
| `voice/synth` | the synthesizer, implements `voice.Sink` |
| `voice/wav` | WAV in and out |
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

## Synthesizer

**Render blocks.** The stream renders in blocks of 64 frames. Commands from the `Sink` methods are applied at the start of a block, and glide and vibrato are evaluated once per block (750 Hz at 48 kHz), which is smooth for pitch movements of a few milliseconds and saves an `exp2` per frame.

**Double buffer.** The `Sink` methods append a command to a slice under a mutex. At each block the renderer swaps that slice for its own empty one, so the lock is held for the swap only and the caller never waits for rendering. Nothing is dropped. Once both slices have grown to the busiest block nothing allocates; the alloc test checks `Read`, `Render` and the `Sink` methods.

**Voice pool.** A fixed pool (24 by default) avoids allocation per note. Stealing takes the oldest releasing voice first (it is fading anyway), then the oldest sounding one. A stolen voice fades out over 2 ms and the new note waits for the fade: cutting to zero would click, and 2 ms of delay is inaudible next to the block and player latency.

**No clicks.** Every jump in level clicks, so: attack, release and a falling decay last at least 2 ms; a note off during the attack waits for the peak (also making on and off in one block audible); one-shot samples fade over their last 2 ms; steals fade.

**Oscillators.** Phase accumulators from 0 to 1. Saw and square use PolyBLEP, a two-sample polynomial correction at each discontinuity that removes most of the aliasing at the cost of a few multiplications. The triangle's harmonics fall off fast enough that a naive one is fine. Sine and triangle start at 0. Noise is a per-voice xorshift whose seeds come from a stream-level generator, so the same sequence of notes always renders the same samples.

**Filter.** A trapezoidal (TPT) state-variable low-pass after Andrew Simper. Unlike the classic Chamberlin form it stays stable at high cutoffs and high resonance; resonance is capped internally at 0.98. Coefficients are computed once per note.

**Output.** The voices are summed, scaled by the master gain and hard-clipped to -1 to 1. Dialogue blips rarely stack loud enough to clip; the clip is a guard, not a limiter.

## Measured costs

`make bench` and `make bench-wasm`, 2026-11-03, Intel Xeon @ 2.10 GHz (cloud container), 48 kHz, one `Read` of 1024 frames (21.3 ms of audio). Patches use a detuned second oscillator; rt% is the share of real time one core spends rendering. js/wasm runs under Node 22.

| Case | native | rt% | js/wasm | rt% |
| --- | --- | --- | --- | --- |
| silent | 6 µs | 0.03 | 60 µs | 0.28 |
| 4 voices | 94 µs | 0.44 | 344 µs | 1.6 |
| 8 voices, filter | 184 µs | 0.86 | 626 µs | 2.9 |
| 24 voices, filter | 508 µs | 2.4 | 1.7 ms | 8.0 |
| NoteOn + NoteOff, applied | 89 ns | | 330 ns | |

All cases allocate nothing. A dialogue line uses two to four voices at a time, so the synthesizer costs well under 2 % of a core even in a browser.

## Known limits

- `NoteOff` matches owner and pitch, not the instrument. A phoneme note at exactly the pitch of a sounding motif note would release both; with any pitch variation this does not happen. The planned `TimedSink` or the `Speaker` can carry a note id if it matters.

## Not built yet

- The `Speaker` runtime, `voicetest.RecordingSink`, the Ebiten player module (milestone 3).
- Sample-accurate scheduling: notes start at the next 64-frame block. An optional `TimedSink` with time offsets can add it without changing `Sink`.
- A SoundFont backend in a nested `sf2` module (optional).
- Reverb, chorus, FM.
