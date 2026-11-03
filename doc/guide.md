# User guide

Package by package. Languages, voices, `Parse`, `Compose` and the `Speaker` are not built yet ([design.md](design.md#not-built-yet)).

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
go run ./cmd/voiceplay -patches -o build/patches.wav [-rate 48000]
```

Renders a short rising phrase on each built-in test patch in turn (sine blip, triangle, square chirp with glide, detuned and filtered saw, square with vibrato, filtered noise, a plucked sample). Exit codes: 0 success, 1 failure, 2 usage error. Rendering a line with a voice comes with the language and voice files.
