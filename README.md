# voice

A Go library that turns dialogue text into musical gibberish for games. Text is split into phonemes of a configurable fictional language, each phoneme plays a short blip on a small built-in synthesizer, a quiet motif follows the punctuation, and the caller learns how much text to reveal at every moment.

It works in any Go program, with any audio output, natively and in js/wasm, and renders offline to WAV. The core module has no dependencies. MIT licensed.

**Status:** the synthesizer is built (milestone 1). Languages, voices, the parser, the composer and the `Speaker` runtime come next.

## Quick start: the synthesizer

```go
s, err := synth.NewStream(synth.StreamConfig{SampleRate: 48000})
if err != nil { ... }
s.SetPatch(1, voice.InstVowel, synth.Patch{
	Wave: synth.Square, PulseWidth: 0.3, Gain: 0.5,
	Env:  synth.Envelope{Decay: 80 * time.Millisecond},
})
s.NoteOn(1, voice.InstVowel, 64, 0.8, 0) // owner 1, E4, velocity 0.8, centre

// Hand s (an io.Reader of float32 stereo PCM) to an audio player, or render offline:
left, right := make([]float32, 48000), make([]float32, 48000)
s.Render(left, right)
```

Hear every feature of the synthesizer:

```bash
go run ./cmd/voiceplay -patches -o build/patches.wav
```

## Documentation

- [doc/guide.md](doc/guide.md): user guide.
- [doc/design.md](doc/design.md): how it is built and why, measured costs, what is not built yet.
