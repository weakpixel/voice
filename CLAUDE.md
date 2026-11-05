# voice

A Go library that turns dialogue text into musical gibberish for games: text is split into phonemes of a configurable fictional language, each phoneme plays a short blip on a small built-in synthesizer, a quiet motif follows the punctuation, and the caller learns how much text to reveal at every moment. Module `github.com/weakpixel/voice`, Go 1.27, MIT. It works standalone (any Go program, any audio output, offline rendering to WAV) and is wrapped later by the Weakpixel Engine (repository `weakpixel/weakpixel`) in a thin ECS package.

## The docs are the spec

- `README.md`: what the library is, quick start (standalone and Ebiten).
- `doc/guide.md`: user guide: API, the `*.lang.json` and `*.voice.json` formats, starter voices, `voiceplay`.
- `doc/design.md`: how it is built and why, measured costs (native and js/wasm), what is not built yet.

Every topic has one home; link to it instead of repeating it. If code and a doc disagree, don't silently pick one: fix the code, or update the doc in the same change and say so. Behavior the docs leave open: decide, implement, test and add it to the doc.

## Layout

| Package | Module | May import | What |
| --- | --- | --- | --- |
| `voice` (root) | `github.com/weakpixel/voice` | stdlib | `Language`, `Voice`, JSON loading, `Parse`, `Compose`, `Timeline`, `Speaker`, the `Sink` interface |
| `voice/synth` | same | stdlib, root | Built-in synthesizer: oscillator and sample patches, voice pool, `Stream` (PCM `io.Reader`), implements `voice.Sink` |
| `voice/wav` | same | stdlib | 16-bit PCM WAV in, float32 stereo WAV out |
| `voice/voicetest` | same | stdlib, root | Test helpers such as a recording `Sink` |
| `voice/cmd/voiceplay` | same | stdlib, the packages above | Renders a line or a voice's patches to WAV (`flag`, no Cobra) |
| `voice/ebiten` | `github.com/weakpixel/voice/ebiten` (nested) | Ebiten `audio` (and its oto), the core | `Player` for a `synth.Stream`; example program; `cmd/say` (speaks a line on the audio device) |
| `voice/editor` | `github.com/weakpixel/voice/editor` (nested) | oto, the core | The Voice Editor: a static web site (Go to js/wasm, plain HTML, CSS and JS, no npm) deployed to GitHub Pages; `session` (tested natively), `wasm` (the `syscall/js` bridge and oto playback), `cmd/mksite`, `web/`, `smoke/`. See `doc/editor.md` |
| `voice/sf2` | `github.com/weakpixel/voice/sf2` (nested, optional) | go-meltysynth fork, the core | SoundFont backend implementing `voice.Sink` |

The plan that describes the milestones is `ecs/plans/voice-library.plan.md` in the project's shared folder (not in this repository).

## Commands

```bash
make all         # every module: go fmt, go vet, go test -race
make wasm-test   # core tests under GOOS=js GOARCH=wasm (Node)
make bench       # benchmarks of synth and the Speaker
make bench-wasm  # the same under GOOS=js GOARCH=wasm (Node); numbers go into doc/design.md
make editor      # the Voice Editor's site in build/editor
make editor-test # node --test of the page's scripts
make editor-smoke # headless Chromium test of the built site (Playwright); run once before an editor PR
make editor-serve # serves build/editor on http://localhost:8000
go run ./cmd/voiceplay -voice soft -o build/line.wav "Hello there, traveler!"
go run ./cmd/voiceplay -patches -o build/patches.wav
```

While iterating, run only the tests of the package you changed. Run `make all` and `make wasm-test` once before opening a PR; CI (GitHub Actions) runs both on every pull request. Nothing here can play sound in the cloud: listening checks (WAV files, the Ebiten example natively and in a browser) go to the user as manual checks.

## Hard rules

- **The core module has no dependencies.** The root, `synth`, `wav`, `voicetest` and `cmd/voiceplay` use the standard library only. Ebiten lives only in the nested `ebiten` module, go-meltysynth only in the nested `sf2` module. Never add a `require` to the root `go.mod`. A test checks this.
- **No cgo**, anywhere. Everything must build for js/wasm.
- **No ECS and no engine code in this repository.** The library knows nothing about the Weakpixel Engine; the engine wraps it.
- **Build outputs go into `./build`** (`go build -o build/<name>`, WAV renders into `build/`), and `build/` stays in `.gitignore`. Never commit binaries, `*.test` files or rendered audio. Before every commit run `git status --short` and `git diff --cached --stat`, and stage files by name, not with `git add -A` or `git add .`.
- **Every note on gets a note off or a release.** `Skip`, `Stop`, a `Say` over a running line and `Close` release the speaker's notes. Tests check that no note survives.
- **Hot paths allocate nothing in steady state:** `Speaker.Update`, `Stream.Read`, `Stream.Render` and the `Sink` methods. Enforce it with `testing.AllocsPerRun` tests, not by eye.
- **Threading:** the `Stream` is owned by the audio player that reads it; callers only use the `Sink` methods, which go through a mutex-guarded double buffer. Never touch voice state from the caller's goroutine. Tests run with `-race`.
- **Determinism:** the same voice and text always produce the same timeline. Randomness only comes from the seeded generator (voice seed xor a hash of the text), never from `math/rand` globals or time.
- **Versions stay 0:** the `version` field of the file formats is 0 and no module tag is created without the user's word. No backward compatibility shims before a release.
- **No attribution and no names:** no Co-Authored-By, Claude-Session or "Generated with" lines in commits, PRs, docs or code; never mention a person's name in any document, commit or PR; no hidden HTML markers or comments anywhere.

## Code quality

- **Readability first.** Clear names, small functions, straightforward control flow. An optimization that makes code harder to follow needs a benchmark that shows it is worth it.
- **Model the domain.** Each concept gets its own type and file (language, parser, composer, speaker, oscillator, envelope, filter, voice pool, stream). No god object.
- **Document the public API.** Every exported identifier has a doc comment that states what it does, its contract (preconditions, ownership, goroutine, cost) and which errors or panics it can raise.
- **Explain DSP and performance tricks inline** (PolyBLEP, envelope minimums against clicks, buffer swaps, preallocation), so a reader does not "simplify" them away.
- **Errors, not panics, for bad input data.** Loading a language or voice collects every validation error with its JSON path and returns them together (`errors.Join`). Panics are only for programmer errors, and their messages start with the package name (`voice: `, `synth: `).

## Conventions

- File names are kebab-case: `voice-file.go`, `voice-pool.go`, `voice-pool_test.go`.
- Tests use the standard `testing` package only (no testify), table-driven with `t.Run`. Audio is checked numerically (pitch from zero crossings, envelope frames, peak and RMS limits), never with committed WAV files; golden timelines live in `testdata/` as JSON.
- Use modern Go: range-over-func, `for i := range n`, `slices`, `maps`, `iter`.
- Notes are float semitones in MIDI numbering (60 is middle C); times are `time.Duration` at the API and frames inside the synthesizer; text positions are byte offsets into the UTF-8 line.
