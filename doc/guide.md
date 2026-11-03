# User guide

Package by package. The `Speaker` runtime is not built yet ([design.md](design.md#not-built-yet)); until then, play a [`Timeline`](#compose) yourself.

## Speaking a line

```go
v, _ := voice.Preset("soft")                          // or voice.LoadVoiceFS(fsys, "guard.voice.json")
tl := voice.ComposeText(v, voice.DefaultLanguage(), "Hello there, traveler!")
s, _ := synth.NewStream(synth.StreamConfig{SampleRate: 48000})
s.SetVoice(1, v, nil)                                  // nil: the voice uses no samples
// play tl.Notes on s at their times, show the text of tl.Reveals
```

## Languages (`*.lang.json`)

A language says how text becomes phonemes. `voice.DefaultLanguage()` is built in: Latin letters with the digraphs `sh ch th ng ck ai ee oo ou`, the default punctuation, and unknown letters and digits played as `a`.

```json
{
  "version": 0,
  "name": "elvish",
  "phonemes": [
    {"symbol": "a", "category": "vowel"},
    {"symbol": "ae", "category": "vowel", "pitch": 2, "length": 1.4},
    {"symbol": "th", "category": "fricative", "length": 0.8, "velocity": 0.5}
  ],
  "punctuation": [
    {"symbol": ",", "pause": 2},
    {"symbol": ".", "pause": 4, "end": "fall"}
  ],
  "whitespace": {"pause": 0.5},
  "unknown": {"policy": "fallback", "phoneme": "a"}
}
```

| Field | Meaning |
| --- | --- |
| `version` | required, `0` |
| `phonemes[].symbol` | one or more characters, no whitespace; matched longest first and case-insensitively (Unicode simple case folding) |
| `phonemes[].category` | `vowel`, `sonorant`, `fricative` or `plosive`: which instrument of the voice plays it |
| `phonemes[].pitch` | semitones added to the voice's note, -24 to 24 (default 0) |
| `phonemes[].length` | slot length in beats, 0.05 to 10 (default 1) |
| `phonemes[].velocity` | relative loudness, 0 to 1 (default 1) |
| `punctuation` | list of `{symbol, pause, end}`: `pause` in beats (0 to 20), `end` is `""`, `fall`, `rise` or `accent`. Leaving the list out gives the defaults: `,` 2, `;` and `:` 2.5, `.` 4 fall, `...` and `…` 5, `?` 4 rise, `!` 4 accent. `[]` means no punctuation |
| `whitespace.pause` | a run of whitespace is one pause of this many beats (default 0.5) |
| `unknown.policy` | what a letter or digit without a phoneme does: `skip` (default; shown at once, silent), `fallback` (played as `unknown.phoneme`) or `pause` (a whitespace pause). Other unknown characters (quotes, dashes, emoji) are always skipped |

Symbols must be unique across phonemes and punctuation, compared case-insensitively. Text is not normalized: an accent written as a combining character is a separate character.

## Voices (`*.voice.json`)

A voice is how one character speaks. Three starter voices are built in, `voice.Preset("soft")`, `"chirpy"` and `"gruff"`:

| Voice | Sound |
| --- | --- |
| `soft` | mid pitch, triangle and sine blips, calm speed |
| `chirpy` | high and fast, square blips that glide into each note, wide pitch variation |
| `gruff` | low and slow, detuned filtered saw |

```json
{
  "version": 0,
  "name": "guard",
  "language": "",
  "baseNote": 55,
  "pitch": 0,
  "variation": 1,
  "velocity": 0.8,
  "length": 0.8,
  "speed": 12,
  "pan": 0,
  "motifVolume": 0.2,
  "seed": 7,
  "instruments": {
    "vowel": {"wave": "square", "pulseWidth": 0.3, "decayMs": 80, "cutoff": 2500, "gain": 0.5},
    "fricative": {"wave": "noise", "decayMs": 30, "cutoff": 4000, "gain": 0.3},
    "motif": {"sample": "hum.wav", "root": 60, "loop": true, "sustain": 1, "releaseMs": 200}
  }
}
```

| Field | Meaning |
| --- | --- |
| `version` | required, `0` |
| `language` | a name your game resolves to a language; `""` means the default language. The library does not resolve it |
| `baseNote` | note of a phoneme with pitch 0, 24 to 108 (default 64) |
| `pitch` | semitones added to every note, -24 to 24; use it to tell characters with one voice apart |
| `variation` | random pitch spread, plus or minus this many semitones, 0 to 12 (default 1) |
| `velocity` | loudness of the phonemes, 0 to 1 (default 0.8) |
| `length` | note length as a share of its slot, 0.05 to 1 (default 0.8) |
| `speed` | beats per second, 1 to 60 (default 14) |
| `pan` | -1 (left) to 1 (right) |
| `motifVolume` | loudness of the motif, 0 to 1 (default 0.2); 0 turns it off |
| `seed` | mixed with a hash of the text; change it for a different melody of the same lines |
| `instruments` | `vowel` (required), `sonorant`, `fricative`, `plosive`, `motif`; missing ones play the vowel's sound |

An instrument has the fields of a [synth patch](#patch), with times in milliseconds: `wave` (`sine` default, `triangle`, `square`, `saw`, `noise`), `pulseWidth`, `detune`, `attackMs`, `decayMs`, `sustain`, `releaseMs`, `glideFrom`, `glideMs`, `vibratoRate`, `vibratoDepth`, `vibratoDelayMs`, `cutoff`, `resonance`, `gain`. Or it is a sample: `sample` (a name your loader resolves), `root` (default 60), `loop`, `loopStart`, `loopEnd`, plus the envelope, glide, vibrato, filter and gain fields. A wave and a sample together are an error.

Loading (`LoadLanguage`, `LoadVoice` from an `io.Reader`, `LoadLanguageFS`, `LoadVoiceFS` from an `fs.FS`) reports malformed JSON and unknown fields on their own; otherwise all validation errors together, each with its JSON path, for example `voice: voice: instruments.vowel.sustain: 2 is outside 0 to 1`. A loaded `Voice` is a plain struct you may change; `Voice.Validate` checks it again.

## Parse

`voice.Parse(lang, text)` (or `AppendParse(dst, lang, text)` to reuse a slice) splits a line into tokens (`voice.Phoneme`): `TokenPhoneme` (symbol, category, pitch, length, velocity), `TokenPause` (whitespace or punctuation, with its length in beats and contour) and `TokenSkip` (unknown characters). Each token has its byte span `Start`, `End`; the spans tile the text exactly. Invalid UTF-8 bytes are skipped one by one.

## Compose

`voice.Compose(v, text, tokens)` (or `ComposeText(v, lang, text)`) turns tokens into a `Timeline`: `Notes` (on and off time, instrument, note, velocity, pan; sorted by on time), `Reveals` (from this time on, show the text up to this byte offset) and `Duration`. It is pure: the same voice and text always give the same timeline.

- A beat is `1/speed` seconds. A phoneme takes `length` beats; its note sounds for the voice's `length` share of that, at `baseNote + pitch + phoneme pitch ± variation`, on the instrument of its category.
- Pauses add silence. Skipped characters take no time.
- The text of each token is revealed when the token starts, so a punctuation mark shows before its pause.
- Each clause (text up to punctuation) follows its punctuation's end: `fall` lowers the last three notes by 1, 2 and 4 semitones, `rise` raises them by 1, 3 and 5, `accent` raises the whole clause by 1 semitone and makes it 20 % louder. Punctuation right after punctuation (`?!`) belongs to the same clause; the last end rule wins.
- The motif is two to four long soft notes on the `motif` instrument, an octave below the voice: one per clause (a single clause is split in two, more than four clauses are grouped into four). Each motif note moves from the previous one: down after a fall, up after a rise or accent (louder too), a small step otherwise.
- `Duration` is the end of the last phoneme; a final pause does not count. The last reveal shows the whole text.

To play a timeline on a `synth.Stream`, call `NoteOn` at each note's `On` and `NoteOff` at its `Off` (offs first when they share a time with ons), rendering in between; `cmd/voiceplay` does exactly that.

## The `Sink` (package `voice`)

`voice.Sink` is where the notes of a line go: `NoteOn(owner, inst, note, velocity, pan)`, `NoteOff(owner, note)` and `Release(owner)`.

- `Owner` is a small integer per speaker. Notes of different owners never touch each other: `Release(2)` ends the notes of owner 2 only.
- `Instrument` is one of `InstVowel`, `InstSonorant`, `InstFricative`, `InstPlosive`, `InstMotif` (`voice.InstrumentCount` of them).
- Notes are float semitones in MIDI numbering (60 is middle C, 69 is 440 Hz). Velocity is 0 to 1, pan is -1 (left) to 1 (right).
- Every `NoteOn` must be matched by a `NoteOff` with the same owner and note, or by `Release(owner)`.

`synth.Stream` implements it.

## The synthesizer (package `synth`)

### Stream

```go
s, err := synth.NewStream(synth.StreamConfig{
	SampleRate: 48000,              // 16000 to 192000, required
	Format:     synth.FormatFloat32, // or synth.FormatInt16
	Voices:     24,                  // pool size, 0 means 24
})
```

`NewStream` reports every problem of the config together. A stream is endless and silent until notes play.

- `Read(p)` fills `p` with whole stereo frames, little endian: 8 bytes a frame for `FormatFloat32` (what Ebiten's `NewPlayerF32` reads), 4 for `FormatInt16`. It never returns an error or `io.EOF`.
- `Render(left, right)` fills two float32 slices of the same length, for offline rendering. It continues where `Read` left off.
- `SetPatch(owner, inst, patch)` sets the sound of one instrument of one owner. It returns the patch's validation errors (all of them) and sets nothing if the patch is invalid. Notes already sounding keep their old patch. A note on an instrument without a patch is ignored.
- `SetVoice(owner, v, load)` sets all five instruments of a voice at once (`synth.PatchFromSpec` for each); `load` resolves sample names and may be nil when the voice uses none. It reports every instrument's errors together and sets nothing on error.
- `SetGain(g)` sets the master gain (1 by default). The output is clipped to -1 to 1.

**Threading.** `Read` and `Render` belong to the audio player's goroutine; never call them concurrently. `NoteOn`, `NoteOff`, `Release`, `SetPatch` and `SetGain` are safe from any goroutine. They take effect at the start of the next block of `synth.BlockFrames` (64) frames, 1.3 ms at 48 kHz. The latency you hear is the player's buffer plus one block; keep the player buffer small (50 ms works).

**Voices.** Each note takes a voice of the pool. When all are busy, a new note steals the oldest releasing voice, else the oldest sounding one; the stolen voice fades out over `synth.StealFade` (2 ms) and the new note starts after it.

### Patch

A patch is an oscillator, or a sample when `Sample` is set, followed by the filter, the envelope and the gain. The zero `Patch` is a sine with the shortest envelope.

| Field | Meaning |
| --- | --- |
| `Wave` | `Sine`, `Triangle`, `Square`, `Saw` (band-limited) or `Noise` (white, ignores the pitch; shape it with the filter) |
| `PulseWidth` | high part of a `Square`'s cycle, 0.01 to 0.99; 0 means 0.5 |
| `Detune` | a second oscillator of the same wave this many semitones away, mixed half and half; 0 means one oscillator |
| `Sample` | plays a `*synth.Sample` instead of the oscillator |
| `Env` | `Attack`, `Decay`, `Sustain` (0 to 1), `Release` |
| `Glide` | the note starts `From` semitones away and slides to the pitch over `Time` |
| `Vibrato` | pitch sine of `Rate` Hz and `Depth` semitones (peak), starting `Delay` after the note on |
| `Filter` | resonant low-pass: `Cutoff` in Hz (0 is off, at most 45 % of the sample rate), `Resonance` 0 to below 1 |
| `Gain` | output level; 0 means 1 |

The envelope is linear. Attack and release (and the decay, when the sustain is below 1) last at least `synth.MinEnvelopeTime` (2 ms), so notes never click. A `Sustain` of 0 ends the note after the decay without a note off, which suits blips. A note off during the attack takes effect when the attack ends: every note reaches its peak, so even a note on and off in the same block is heard.

### Sample

```go
data, rate, err := wav.Read(f) // 16-bit PCM WAV, mixed to mono
patch := synth.Patch{Sample: &synth.Sample{Data: data, Rate: rate, Root: 72}}
```

A sample sounds at its own pitch for the note `Root` and is resampled (linear interpolation) for other notes. A one-shot sample ends the note at its end, fading the last 2 ms. With `Loop`, the frames `LoopStart` to `LoopEnd` (0 means the end) repeat until the envelope ends. Do not change a sample after handing it to a stream.

## WAV files (package `wav`)

- `wav.Read(r)` reads a 16-bit PCM WAV (plain or `WAVE_FORMAT_EXTENSIBLE`, any channel count) into mono float32 samples and the sample rate.
- `wav.WriteStereo(w, rate, left, right)` writes a 32-bit float stereo WAV.

## voiceplay

```bash
go run ./cmd/voiceplay -voice soft -o build/line.wav "Hello there, traveler!"
go run ./cmd/voiceplay -voice my.voice.json -lang my.lang.json "Kel thae, varan."
go run ./cmd/voiceplay -patches -voice gruff -o build/gruff-patches.wav
go run ./cmd/voiceplay -patches -o build/patches.wav
```

| Flag | Meaning |
| --- | --- |
| `-voice` | a starter voice by name (default `soft`) or a `*.voice.json` file; its samples are 16-bit PCM WAV files resolved next to it |
| `-lang` | a `*.lang.json` file (default the built-in language) |
| `-patches` | instead of a line, play a short rising phrase on each instrument of the voice; without `-voice`, on each built-in test patch of the synthesizer |
| `-o` | output WAV (default `build/line.wav`, or `build/patches.wav` with `-patches`) |
| `-rate` | sample rate (default 48000) |

Exit codes: 0 success, 1 failure, 2 usage error.
