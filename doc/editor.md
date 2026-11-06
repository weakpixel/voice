# Voice Editor

A web page to edit `*.voice.json` files and hear them while you type. It runs entirely in the browser: the library compiled to WebAssembly plays the test line through the same `Speaker` and synthesizer a game uses. It lives in the nested module `editor/` and is deployed to GitHub Pages.

## Using it

- **New from starter** opens one of the starter voices as a new document. **Open** (or dropping a file on the page, or pasting voice JSON anywhere outside the text fields) opens a `*.voice.json` file.
- The left panel shows the voice in tabs. **Visual** is a form: sliders with a number field for numbers, dropdowns for the wave, filter, rests and steps per beat, checkboxes for on and off. Instruments have one tab each; an instrument other than the vowel can use the vowel's sound (its checkbox cleared, the key left out of the file) or its own. **Rhythm** turns the beat grid on; its drum pattern is a grid with one cell per step of a bar (a click cycles hit, ghost hit, rest) and an accents row above it (none, medium, strong; dimmed while the file uses the default accents). Changing the steps per beat refits the patterns and accents, keeping the steps that still fall on the grid; **Percussion** adds built-in or new sounds. A field the file leaves out shows the library's default, dimmed; changing it writes the field, ↺ removes it again. Hover a field for what it changes in plain words; a dropdown's tooltip also explains each choice, and so do the instrument tabs and the drum names.
- **Source** is the same voice as JSON text, for anything the form does not cover (accents, sample loops). Both tabs edit one document: the form writes the text in the starter voices' style, keeping the order of the keys you wrote.
- Every change is checked by the library itself: **Problems** lists what is wrong, each with the JSON path of its value, and the form marks the field; click a problem to select it in the text. The file format is in the [guide](guide.md#voices-voicejson).
- **Undo** and **Redo** (Ctrl+Z, Ctrl+Shift+Z) step through the edits of both tabs.
- **Play** speaks the test line with the voice; the text types out as it is spoken. **Pause**, **Resume**, **Skip** and **Stop** do what they do in a game (`Speaker.Pause`, `Resume`, `Skip`, `Stop`). Enter in the test line plays it too. Browsers start audio only after a click or key press; the first Play is that click.
- **Hear** plays a short phrase on one instrument (next to the instrument tabs) or a few hits of one drum (on its card and its beat row), the same phrase `voiceplay -patches` plays. **Stop** ends it.
- Under the text, the **piano roll** shows the line's notes, one row per semitone and coloured by instrument, the drums in a band at the bottom and small ticks at the top where the text moves on; the **waveform** under it is the line as rendered, with the reverb ringing out. Both follow every edit, and a red playhead runs along them while the line plays. **WAV** downloads that render as `<name>-line.wav`.
- A valid edit applies at once: change an instrument while a line plays and the next notes use the new sound. Changes to timing and pitch are heard the next time the line plays: with **Replay on change** on, the line plays again after such an edit (and after any edit while nothing plays), once audio has started.
- **Download** saves the file (under the name it was opened with, else the voice's name), **Copy** puts the JSON on the clipboard. Only valid voices play; any text can be downloaded. In Chrome and Edge, **Save** (Ctrl+S) writes back to the file you opened, or asks where to save the first time.
- **Share link** copies an address that opens the current voice in the editor: the file is compressed into the part after `#`, so nothing is uploaded anywhere. Long voices make long links.
- For a voice with a rhythm, **Mute the beat** leaves out its drums (as when the game's music plays the beat), and **Music at** times the line to music at that tempo, coming in on the music's next bar, as a game does with `SayOptions.Clock`.
- **Sample files**: an instrument with `"sample": "<file name>"` plays a recorded sound. Add the 16-bit PCM WAV files by dropping them on the page or with **Add WAV files**; the browser keeps them for the next visit (IndexedDB). A sample the page does not have is a problem at that instrument's `sample` field.
- **Language** is the made-up language the preview speaks. Empty is the built-in one; paste or open a `*.lang.json` file, or start from the built-in language and change it. It has its own problem list and is kept with the draft, apart from the voice file.
- The page keeps the current document, the test line and the volume in the browser's local storage and restores them on the next visit.

### Dialogue mode

The **Voice** and **Dialogue** buttons in the top bar switch modes. The Dialogue mode edits a [`*.dialogue.json`](guide.md#dialogues-dialoguejson-package-dialogue) file as text (the Source tab; a form for speakers and lines comes later) and plays it in a small game: the same dialogue box as `ebiten/example`, running the library's `dialogue.Player`. The game is a second WebAssembly program (about 4 MB compressed) loaded in an iframe the first time the mode opens, so the Voice mode does not pay for it.

- The text starts as the example dialogue. **Start from the example**, **Open…**, **Download** and **Copy** work on the dialogue as for voices. A speaker's `voice` is a starter voice name; voice files are not available in this mode yet, and naming one is a problem at `speakers[i].voice`.
- The game loads every valid edit about 0.3 s after the last keystroke and continues on the line it was on. **Problems** lists what the library's loader finds, each with its JSON path, exactly as a Go program sees them.
- **Restart** plays from the first line, **from line … Play** from the line you choose, **Pause** and **Resume** stop time (the sounding notes are released). **Auto-advance** goes on after every line by itself (`hold` seconds), without a click. Inside the game, a click or Space skips the typing or goes on.
- The status line under the game names the line it is on and who says it.
- Sound starts with the first click **inside the game**: a click on the page's buttons does not start it, because the game has its own audio context (measured in Chromium, with and without its autoplay flag). The game says so until it is running. Switching to the Voice mode pauses the game, and the Voice preview stops when the Dialogue mode opens, so only one sounds at a time.
- The dialogue text, the mode, auto-advance and the volume are kept in local storage.


## Building and running locally

```bash
make editor        # the site in build/editor (voice.wasm, game.wasm, wasm_exec.js from your Go, the page, the starter voices)
make editor-serve  # builds and serves it on http://localhost:8000
make editor-test   # node --test of the page's scripts
make editor-smoke  # headless Chromium: every starter plays, problems, transport, open, download, draft, roll, Hear, WAV, beat grid, samples, language, share link, the Dialogue mode
```

`make editor-smoke` needs Playwright with Chromium (`require('playwright')` must work, for example through `NODE_PATH`). The site is static and uses relative URLs, so any static file server works; it must serve `.wasm` as `application/wasm`.

## Deployment

`.github/workflows/pages.yml` runs `make editor` on every push to `main` (and by hand) and deploys `build/editor` to GitHub Pages. It needs Pages enabled in the repository settings with the source "GitHub Actions". While the repository is private the workflow's jobs are skipped, because Pages for a private repository needs a paid plan; it deploys by itself once the repository is public. The CI workflow builds the site and runs `make editor-test` on every pull request.

How the editor is built and why: [design.md](design.md#editor).
