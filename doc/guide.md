# User guide

Package by package, starting with what a game needs.

## Speaking a line

```go
stream, _ := synth.NewStream(synth.StreamConfig{SampleRate: 48000})
player, _ := voiceebiten.NewPlayer(audio.NewContext(48000), stream, 0) // or hand stream (an io.Reader) to any player

v, _ := voice.Preset("soft")              // or voice.LoadVoiceFS(fsys, "guard.voice.json")
stream.SetVoice(1, v, nil)                // owner 1 plays with this voice's instruments
sp := voice.NewSpeaker(stream, 1)
sp.Say(v, "Hello there, traveler!", voice.SayOptions{})

// every frame:
sp.Update(dt)
draw(text[:sp.Revealed()])
for _, e := range sp.Events() { ... }     // Started, Completed, Skipped, Stopped
```

Each speaker needs its own owner on the stream, and the stream needs that owner's instruments (`SetVoice`) before the speaker talks. One stream serves every speaker of a game.

## Speaker

`voice.NewSpeaker(sink, owner)` plays lines on any `voice.Sink` as owner. Use it from one goroutine (the game loop).

- `Say(v, text, opts)` composes the line and starts it; a running line is stopped first (its notes released, a `Stopped` event). Notes and reveals due at time 0 happen at once, so the first character shows right away. `SayOptions.Language` is the language (nil means the default), `Speed` scales playback (2 is twice as fast, 0 means 1), `Instant` shows all text at once and plays only the motif. `Clock` and `MuteBeat` time a voice with a [rhythm](#rhythm) to the game's music (see [Compose](#compose)); while a `Clock` is set, `Speed` is ignored.
- `Update(dt)` advances the line, sends the notes that are due and moves the reveal. The reveal depends only on the time that has passed, not on the frame rate. It allocates nothing.
- `Revealed()` is how many bytes of the text to show; `text[:Revealed()]` always ends between tokens, never inside a character.
- `Skip()` shows all text and releases the notes (`Skipped`). `Stop()` releases the notes and leaves the text as it is (`Stopped`). `Pause()` stops time and releases the sounding notes (a held note would hang); `Resume()` continues with the next notes. `Close()` stops like `Stop` and makes later `Say` calls do nothing.
- `Done()` is true when no line is playing: before the first `Say`, and after a line completed, was skipped or stopped. A line completes at its timeline's `Duration`: after its last sound and the voice's `breath`, so the motif can ring out; a final pause does not delay it. The text is complete earlier, when its last token starts.
- `Events()` returns what happened since the previous `Update` began: the events of `Say`, `Skip`, `Stop` and `Close` calls since then and those of the `Update` itself. Each `Event` has its `Kind` and the `Line` number (1 for the first `Say`). The slice is reused and valid until the next `Update`.
- `Timeline()` is the current line's composed [timeline](#compose).

Every note the speaker starts ends with a `NoteOff` or a `Release` of its owner.

## Dialogues (`*.dialogue.json`, package `dialogue`)

A dialogue is a conversation: lines said by speakers, each speaker with its own voice. `dialogue.Load(r)` (or `LoadFS(fsys, name)`) reads the file, `LoadVoices(fsys, script)` loads the voices it names, and a `dialogue.Player` speaks it on a `voice.Sink`, one `voice.Speaker` per speaker.

```json
{
  "version": 0,
  "name": "village",
  "advance": "input",
  "hold": 1.0,
  "speakers": [
    { "id": "wren", "name": "Wren", "voice": "chirpy", "pan": -0.3 },
    { "id": "bram", "name": "Bram", "voice": "bram.voice.json", "pan": 0.3 }
  ],
  "lines": [
    { "speaker": "wren", "text": "Hello there, traveler! Are you lost?" },
    { "speaker": "bram", "text": "Lost? No. Just... a little turned around.", "delay": 0.4, "speed": 0.9 },
    { "speaker": "wren", "text": "Then follow me!", "advance": "auto", "hold": 0.5 }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `version` | required, `0` |
| `name` | optional name of the dialogue |
| `advance` | `"input"` (default): a finished line waits for `Next`; `"auto"`: it goes on by itself after its hold. Lines can override it |
| `hold` | seconds a finished line stays before an auto advance, 0 to 10 (default 1) |
| `speakers[]` | at least one. Speaker `i` plays as owner `i + 1` |
| `speakers[].id` | required, unique, `[a-z0-9_-]+` |
| `speakers[].name` | what a dialogue box shows (default: the id) |
| `speakers[].voice` | required: a starter voice name (`chirpy`) or a `*.voice.json` file name relative to the dialogue file (no `..`) |
| `speakers[].pan` | -1 to 1, replaces the voice's pan for this speaker |
| `lines[]` | at least one |
| `lines[].speaker`, `lines[].text` | required: the id of a speaker, and the text (not empty) |
| `lines[].delay` | seconds before the line starts, 0 to 10 (default 0) |
| `lines[].speed` | `SayOptions.Speed`, 0.25 to 4 (default 1) |
| `lines[].instant` | `SayOptions.Instant` |
| `lines[].advance`, `lines[].hold` | override the dialogue's values for this line |

Every problem of a file is returned together (`errors.Join`) as a `voice.FieldError` with `File` `"dialogue"` and the JSON path (`lines[1].speaker`); `LoadFS` prefixes the file name. `Load` fills in every default, so a `Line`'s `Advance` and `Hold` are what the player uses. `LoadVoices` returns the voices keyed by the speaker's `voice` text; a voice that does not load is a problem at `speakers[i].voice`.

```go
script, _ := dialogue.LoadFS(fsys, "village.dialogue.json")
voices, _ := dialogue.LoadVoices(fsys, script)
player, _ := dialogue.NewPlayer(stream, script, voices, dialogue.Options{})
for _, sv := range player.Voices() {
	stream.SetVoice(sv.Owner, sv.Voice, nil) // the sink needs each speaker's instruments
}

// every frame:
player.Update(dt)                          // on a click or key: player.Next()
draw(player.Speaker().Name, player.Line().Text[:player.Revealed()])
for _, e := range player.Events() { ... }  // LineStarted, LineDone, DialogueFinished
```

- `NewPlayer(sink, script, voices, opts)` works on copies of the voices with each speaker's pan applied, and starts line `opts.Start` at once. `Options.Languages(name)` resolves a voice's `language` name (nil or a nil result means the default language); `Options.AutoAdvance` makes every line advance by itself.
- A line goes through the phases `Waiting` (its `delay`), `Speaking`, `Holding` (finished; waits for `Next`, or for its `hold` when it advances by itself) and, after the last line, `Finished`. `Phase()` tells which one. `Index()`, `Line()` and `Speaker()` describe the current line; `Revealed()` is how many bytes of its text to show (none while waiting, all while holding).
- `Next()` is the input: while a line types out it shows the rest of it (`LineDone`), while a finished line holds it goes on to the next one (or finishes). During a delay and after the end it does nothing. `Start(i)` jumps to line `i` (it panics for an index outside the script), also after `Finished`. `Pause()` and `Resume()` stop and continue time everywhere; `Stop()` ends the dialogue (phase `Finished`, no event); `Close()` stops and makes later calls do nothing.
- `Events()` are `LineStarted` (the delay passed and the line began), `LineDone` (it completed or was skipped) and `DialogueFinished`, each with the line index and `At`, the player's clock (the sum of the `Update` times). As with the speaker, the slice is valid until the next `Update`.
- Like the speaker, every note the player starts ends: `Next`, `Start`, `Pause`, `Stop` and `Close` release the notes of the speaker involved. `Update` allocates nothing in steady state; starting a line does. The same script and options give the same events.

## Ebiten (module `github.com/weakpixel/voice/ebiten`)

A nested module, so only programs that use it depend on Ebiten. `voiceebiten.NewPlayer(ctx, stream, bufferSize)` plays a `FormatFloat32` stream at the context's sample rate (an error otherwise) with the given buffer (0 means 50 ms, which keeps the sound within about a frame or three of the text). `SetVolume`, `Volume`, `Close`. Browsers start audio only after a click or key press; Ebiten resumes its context on the first input.

Until the root module has a version tag, the nested module finds it through a `replace` to `..`: use it from a checkout of this repository (or a `go.work`).

The example, a dialogue box with three characters (one speaks in rhythm with `verse`):

```bash
cd ebiten && go run ./example                     # Space or click: skip, then next line
```

## Tests (package `voicetest`)

`voicetest.NewRecordingSink(capacity)` is a `voice.Sink` that records every call. `Calls()` returns them, `Sounding(owner)` the notes of an owner that are still on (a test that ends a line checks it is empty), `Reset()` forgets them.

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

A voice is how one character speaks. Ten starter voices are built in, `voice.Preset("soft")`, `"chirpy"`, `"gruff"`, `"divine"`, `"flow"`, `"verse"`, `"warrior"`, `"robot"`, `"ghost"` and `"elder"`:

| Voice | Sound |
| --- | --- |
| `soft` | mid pitch, triangle and sine blips, calm speed |
| `chirpy` | high and fast, square blips that glide into each note, wide pitch variation |
| `gruff` | low and slow, detuned filtered saw |
| `divine` | very low and slow, orchestral: each syllable a chord (root, octave below, fifth above) of five-copy unison saws, long swells, deep in the reverb, a slow pad underneath |
| `flow` | rhythmic, like a rapper: low punchy square blips on a 92 bpm grid of sixteenths with a light swing, over kick, snare and hi-hat after one bar of count-in |
| `verse` | the rhythmic delivery of `flow` without a beat: 96 bpm sixteenths, accents and rests only, starting at once |
| `warrior` | very low, harsh and barking: detuned saw vowels an octave doubled that fall into each note, short clipped syllables, wide pitch swings, loud throaty rasps |
| `robot` | monotone (no pitch variation): thin buzzing square blips with a fifth on top and a fast metallic vibrato, sharp hiss and zaps |
| `ghost` | high, slow and floating: sine notes that slide up into place, a wavering vibrato, breathy whispers, everything deep in the reverb |
| `elder` | low and slow with a shaky voice: soft triangle blips with a wide, fast tremble, a slight sag at each note's start, a warm pad underneath |

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
| `breath` | seconds the motif's last note rings on after the voice, 0 to 2 (default 0.4); the line completes after it. 0 ends the motif with the voice |
| `seed` | mixed with a hash of the text; change it for a different melody of the same lines |
| `instruments` | `vowel` (required), `sonorant`, `fricative`, `plosive`, `motif`; missing ones play the vowel's sound |
| `percussion` | named [percussion sounds](#percussion) the rhythm's beat plays, at most 8 |
| `rhythm` | puts the voice on a [beat grid](#rhythm); left out, the voice speaks freely |

An instrument has the fields of a [synth patch](#patch), with times in milliseconds: `wave` (`sine` default, `triangle`, `square`, `saw`, `noise`), `pulseWidth`, `detune`, `attackMs`, `decayMs`, `sustain`, `releaseMs`, `glideFrom`, `glideMs`, `vibratoRate`, `vibratoDepth`, `vibratoDelayMs`, `filter` (`lowpass` default, `highpass`, `bandpass`), `cutoff`, `resonance`, `gain`, `reverb`, `layers`, `unison`, `spread`. Or it is a sample: `sample` (a name your loader resolves), `root` (default 60), `loop`, `loopStart`, `loopEnd`, plus the envelope, glide, vibrato, filter, gain, reverb and layers fields (not unison). A wave and a sample together are an error.

Loading (`LoadLanguage`, `LoadVoice` from an `io.Reader`, `LoadLanguageFS`, `LoadVoiceFS` from an `fs.FS`) reports malformed JSON and unknown fields on their own; otherwise all validation errors together, each with its JSON path, for example `voice: voice: instruments.vowel.sustain: 2 is outside 0 to 1`. Each of them is a `*voice.FieldError` (`File`, `Path`, `Message`); unwrap the joined error (its `Unwrap() []error`) to show each next to its field. A loaded `Voice` is a plain struct you may change; `Voice.Validate` checks it again.

### Percussion

```json
"percussion": {
  "kick": {"gain": 1.2},
  "boom": {"base": "tom", "note": 38},
  "zap": {"wave": "square", "note": 84, "decayMs": 40, "gain": 0.5}
}
```

Each entry is an instrument (the fields above) plus `note`, the note every hit plays (0 to 127, default 60), and `base`, a built-in sound to start from: the entry's own fields replace the built-in's. An entry named like a built-in starts from it, so `"kick": {"gain": 1.2}` is a louder built-in kick. The built-in sounds (`voice.PercussionNames()`, `voice.BuiltinPercussion(name)`) are `kick`, `snare`, `hat`, `clap`, `tom`, `rim` and `shaker`; a beat may name them without defining them. A voice has at most `voice.MaxPercussion` (8) sounds, sorted by name in `Voice.Percussion`; sound i plays on instrument `voice.InstPercussion + i`.

### Rhythm

```json
"rhythm": {
  "bpm": 92,
  "division": 4,
  "swing": 0.1,
  "accents": [1, 0, 0, 0, 0.6, 0, 0, 0, 1, 0, 0, 0, 0.6, 0, 0, 0],
  "rest": {"pause": "beat", "end": "bar"},
  "countIn": 1,
  "beatVolume": 0.8,
  "beat": {"kick": "x.....x...x.....", "snare": "....x.......x...", "hat": "x.x.x.x.x.x.x-x-"}
}
```

| Field | Meaning |
| --- | --- |
| `bpm` | tempo, 40 to 240 (default 90) |
| `division` | grid steps per beat: 2 (eighths), 3 (triplets) or 4 (sixteenths, the default). A bar has 4 beats |
| `swing` | delays every second step by this share of a step, 0 to 0.5 |
| `accents` | strength 0 to 1 of each step of a bar (4 × `division` values); left out, beats 1 and 3 are strong (1), 2 and 4 medium (0.6), the steps between them none |
| `rest` | where the voice starts again after punctuation: `none`, `step` (the punctuation's pause rounded to steps), `beat` (the next beat) or `bar` (the next bar). `pause` is for punctuation without an end rule (`,` `;` `:` `...`, default `beat`), `end` for punctuation with one (`.` `?` `!`, default `bar`). Every rest but `none` leaves at least one silent step |
| `countIn` | bars of beat before the voice starts, 0 to 2 |
| `beatVolume` | velocity of a percussion hit, 0 to 1 (default 0.8) |
| `beat` | one pattern per percussion sound, one character per step of a bar: `x` a hit, `-` a ghost hit (half velocity), `.` a rest |

How a rhythmic line is timed is described under [Compose](#compose).

## Parse

`voice.Parse(lang, text)` (or `AppendParse(dst, lang, text)` to reuse a slice) splits a line into tokens (`voice.Phoneme`): `TokenPhoneme` (symbol, category, pitch, length, velocity), `TokenPause` (whitespace or punctuation, with its length in beats and contour) and `TokenSkip` (unknown characters). Each token has its byte span `Start`, `End`; the spans tile the text exactly. Invalid UTF-8 bytes are skipped one by one.

## Compose

A `Speaker` does this for you; use `Compose` directly to render offline or to drive another sound engine.

`voice.Compose(v, text, tokens)` (or `ComposeText(v, lang, text)`, or `ComposeWith(v, text, tokens, opts)` with options) turns tokens into a `Timeline`: `Notes` (on and off time, instrument, note, velocity, pan; sorted by on time), `Reveals` (from this time on, show the text up to this byte offset) and `Duration`. It is pure: the same voice, text and options always give the same timeline.

- A beat is `1/speed` seconds. A phoneme takes `length` beats; its note sounds for the voice's `length` share of that, at `baseNote + pitch + phoneme pitch ± variation`, on the instrument of its category.
- Pauses add silence. Skipped characters take no time.
- The text of each token is revealed when the token starts, so a punctuation mark shows before its pause.
- Each clause (text up to punctuation) follows its punctuation's end: `fall` lowers the last three notes by 1, 2 and 4 semitones, `rise` raises them by 1, 3 and 5, `accent` raises the whole clause by 1 semitone and makes it 20 % louder. Punctuation right after punctuation (`?!`) belongs to the same clause; the last end rule wins.
- The motif is two to four long soft notes on the `motif` instrument, an octave below the voice: one per clause (a single clause is split in two, more than four clauses are grouped into four). Each motif note moves from the previous one: down after a fall, up after a rise or accent (louder too), a small step otherwise.
- The line ends the way speech does: its last syllable (the last vowel and the consonants after it) takes 1.5 times as long and its notes hold 0.15 more of their slot, the last syllable before each other `.` `?` `!` takes 1.25 times as long, and the last three phonemes play at 90 %, 80 % and 70 % of their velocity.
- The motif's last note rings on for the voice's `breath` after the last phoneme, then its release fades it.
- `Duration` is the end of the last phoneme plus the `breath`; a final pause does not count. The last reveal shows the whole text, at the latest when the last phoneme ends.

A voice with a `rhythm` is timed on its beat grid instead (contours and the motif stay as above):

- Phonemes are grouped into syllables. Within a word, each vowel is a syllable's nucleus; of the consonants between two vowels, the last one starts the next syllable and the others end the previous one. A word without a vowel is one syllable.
- Each syllable takes whole steps: its natural length (its phonemes' slots at `speed`) rounded to the nearest step, at least one. So `speed` still decides how dense the flow is. Its nucleus starts on a step; its leading consonants come just before it (at most half a step, less with swing); its nucleus and trailing consonants fill the rest of its steps.
- Whitespace takes no time, so words flow together. Punctuation rests as `rest` says.
- The accent of the nucleus's step makes the syllable up to 40 % louder and 2 semitones higher.
- The first nucleus sits on the bar line after the count-in (or, without a count-in, right after its leading consonants). The beat plays from the count-in to the end of the bar in which the voice's last sound ends, and `Duration` covers it.
- Text is revealed when its sound starts, so nothing shows during the count-in.
- The ending works as above in whole steps: the last syllable takes 1.5 times its steps (rounded), and the breath ends on the first beat at least `breath` after the voice.

`ComposeOptions` (also in `SayOptions`) tie a rhythmic line to the game's music. `Clock{BPM, Phase}` replaces the voice's tempo and puts the first nucleus on a bar line of the music: `Phase` is the time from the start of the line until the music's next bar begins (taken modulo a bar, so the negated time since the music started works too). The voice starts after its count-in, one bar later if its leading consonants do not fit before the bar line. `MuteBeat` leaves out the percussion, when the music has its own. Both do nothing for a voice without a rhythm.

To play a timeline yourself, call `NoteOn` at each note's `On` and `NoteOff` (with the note's instrument) at its `Off`, offs first when they share a time with ons.

## The `Sink` (package `voice`)

`voice.Sink` is where the notes of a line go: `NoteOn(owner, inst, note, velocity, pan)`, `NoteOff(owner, inst, note)` and `Release(owner)`.

- `Owner` is a small integer per speaker. Notes of different owners never touch each other: `Release(2)` ends the notes of owner 2 only.
- `Instrument` is one of `InstVowel`, `InstSonorant`, `InstFricative`, `InstPlosive`, `InstMotif`, or a percussion sound `InstPercussion + i` (`voice.InstrumentCount`, 13, in all).
- Notes are float semitones in MIDI numbering (60 is middle C, 69 is 440 Hz). Velocity is 0 to 1, pan is -1 (left) to 1 (right).
- Every `NoteOn` must be matched by a `NoteOff` with the same owner, instrument and note, or by `Release(owner)`. A `NoteOff` ends every sounding note of that owner, instrument and pitch, so a percussion hit never ends a motif note at the same pitch.

`synth.Stream` implements it.

## The synthesizer (package `synth`)

### Stream

```go
s, err := synth.NewStream(synth.StreamConfig{
	SampleRate: 48000,              // 16000 to 192000, required
	Format:     synth.FormatFloat32, // or synth.FormatInt16
	Voices:     32,                  // pool size, 0 means 32
})
```

`NewStream` reports every problem of the config together. A stream is endless and silent until notes play.

- `Read(p)` fills `p` with whole stereo frames, little endian: 8 bytes a frame for `FormatFloat32` (what Ebiten's `NewPlayerF32` reads), 4 for `FormatInt16`. It never returns an error or `io.EOF`.
- `Render(left, right)` fills two float32 slices of the same length, for offline rendering. It continues where `Read` left off.
- `SetPatch(owner, inst, patch)` sets the sound of one instrument of one owner. It returns the patch's validation errors (all of them) and sets nothing if the patch is invalid. Notes already sounding keep their old patch. A note on an instrument without a patch is ignored.
- `SetVoice(owner, v, load)` sets all instruments of a voice at once, its five instruments and its percussion sounds (`synth.PatchFromSpec` for each); `load` resolves sample names and may be nil when the voice uses none. It reports every instrument's errors together and sets nothing on error.
- `SetGain(g)` sets the master gain (1 by default). The output is clipped to -1 to 1.
- `SetReverb(synth.Reverb{Size, Damping, Width})` sets the stream's one reverb (all fields 0 to 1; `synth.DefaultReverb` is Size 0.75, Damping 0.4, Width 1). Every patch sends into it with its own `Reverb` level, so one stream is one room for all speakers. A reverb that receives nothing and has died away costs nothing.

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
| `Filter` | resonant filter: `Type` (`LowPass`, the default, `HighPass` or `BandPass`), `Cutoff` in Hz (0 is off, at most 45 % of the sample rate), `Resonance` 0 to below 1 |
| `Gain` | output level; 0 means 1 |
| `Reverb` | send into the stream's reverb, 0 (dry, the default) to 1 |
| `Layers` | semitone offsets played together for each note, like a chord: `[0, -12, 7]` is the note, an octave below and a fifth above. Each layer takes a voice; the velocity is scaled by 1/√n so a chord is about as loud as one note. `NoteOff` and `Release` end all layers. Empty means `[0]`; at most 4 |
| `Unison`, `Spread` | `Unison` copies of the oscillator (up to 7) spread evenly across `Spread` semitones in total, for the sound of a section or choir instead of one instrument. 0 and 1 mean one oscillator; oscillators only |

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
go run ./cmd/voiceplay -voice flow -o build/flow.wav "Yo, listen up. I came from the hills, with nothing but skills."
go run ./cmd/voiceplay -patches -voice gruff -o build/gruff-patches.wav
go run ./cmd/voiceplay -patches -o build/patches.wav
```

| Flag | Meaning |
| --- | --- |
| `-voice` | a starter voice by name (default `soft`) or a `*.voice.json` file; its samples are 16-bit PCM WAV files resolved next to it |
| `-lang` | a `*.lang.json` file (default the built-in language) |
| `-patches` | instead of a line, play a short rising phrase on each instrument of the voice and four hits on each percussion sound; without `-voice`, on each built-in test patch of the synthesizer |
| `-o` | output WAV (default `build/line.wav`, or `build/patches.wav` with `-patches`) |
| `-rate` | sample rate (default 48000) |

Exit codes: 0 success, 1 failure, 2 usage error.

## say

`say` speaks a line straight through the default audio device and types it out in the terminal as it is spoken, with no WAV file. It needs an audio backend, so it lives in the `ebiten` module (`ebiten/cmd/say`), not next to `voiceplay` in the dependency-free core. It plays through oto, the audio library under Ebiten's audio package, directly: Ebiten's audio context opens the device only from a running game loop, and a command line program has none.

```bash
cd ebiten
go run ./cmd/say "Hello there, traveler!"
go run ./cmd/say -voice flow "Yo, listen up. I came from the hills, with nothing but skills."
go run ./cmd/say -voice my.voice.json -lang my.lang.json "Kel thae, varan."
go build -o ../build/say ./cmd/say
```

| Flag | Meaning |
| --- | --- |
| `-voice` | as for `voiceplay` (default `soft`) |
| `-lang` | as for `voiceplay` |
| `-volume` | playback volume, 0 to 1 (default 1) |
| `-rate` | sample rate (default 48000) |

Ctrl+C stops the line and releases its notes. Exit codes: 0 success (also after Ctrl+C), 1 failure (bad voice or language, no audio device), 2 usage error. On Linux the audio goes through PulseAudio (or PipeWire's PulseAudio server), falling back to ALSA.
