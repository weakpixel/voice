# voice

A Go library that turns dialogue text into musical gibberish for games. Text is split into phonemes of a configurable fictional language, each phoneme plays a short blip on a small built-in synthesizer, a quiet motif follows the punctuation, and the caller learns how much text to reveal at every moment. A voice can also speak on a beat, like a rapper, over its own percussion or in time with the game's music.

It works in any Go program, with any audio output, natively and in js/wasm, and renders offline to WAV. The core module has no dependencies. MIT licensed.

**Status:** milestones 1 to 3 are built: the synthesizer, languages, voices, the parser, the composer, the `Speaker` runtime and the Ebiten player. The library is at version 0; formats and API may still change.

Hear the starter voices:

```bash
go run ./cmd/voiceplay -voice chirpy -o build/line.wav "Hello there, traveler!"
go run ./cmd/voiceplay -voice flow -o build/flow.wav "Yo, listen up. I came from the hills, with nothing but skills."
```

## Quick start: a game (Ebiten)

```go
stream, _ := synth.NewStream(synth.StreamConfig{SampleRate: 48000})
player, _ := voiceebiten.NewPlayer(audio.NewContext(48000), stream, 0)
defer player.Close()

v, _ := voice.Preset("soft")
stream.SetVoice(1, v, nil)
sp := voice.NewSpeaker(stream, 1)
sp.Say(v, "Hello there, traveler!", voice.SayOptions{})

// in Update:  sp.Update(time.Second / 60)
// in Draw:    draw text[:sp.Revealed()]
// on Space:   if !sp.Done() { sp.Skip() }
```

The full program is `ebiten/example` (`cd ebiten && go run ./example`). Any other audio output works the same way: the stream is an `io.Reader` of float32 (or int16) stereo PCM.

## Quick start: standalone, as a timeline

```go
v, _ := voice.Preset("soft")
tl := voice.ComposeText(v, voice.DefaultLanguage(), "Hello there, traveler!")
for _, n := range tl.Notes { ... }     // n.On, n.Off, n.Instrument, n.Note, n.Velocity
for _, r := range tl.Reveals { ... }   // from r.At on, show text[:r.Offset]
```

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
