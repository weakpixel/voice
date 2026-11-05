# Voice Editor

A web page to edit `*.voice.json` files and hear them while you type. It runs entirely in the browser: the library compiled to WebAssembly plays the test line through the same `Speaker` and synthesizer a game uses. It lives in the nested module `editor/` and is deployed to GitHub Pages.

## Using it

- **New from starter** opens one of the six starter voices as a new document. **Open** (or dropping a file on the page, or pasting voice JSON anywhere outside the text fields) opens a `*.voice.json` file.
- The left panel shows the voice in two tabs. **Visual** is a form: sliders with a number field for numbers, dropdowns for the wave, filter, rests and steps per beat, checkboxes for on and off. Instruments have one tab each; an instrument other than the vowel can use the vowel's sound (its checkbox cleared, the key left out of the file) or its own. **Rhythm** turns the beat grid on and edits the beat's patterns (`x` hit, `-` ghost, `.` rest); **Percussion** adds built-in or new sounds. A field the file leaves out shows the library's default, dimmed; changing it writes the field, ↺ removes it again. Hover a label for what the field does.
- **Source** is the same voice as JSON text, for anything the form does not cover (accents, sample loops). Both tabs edit one document: the form writes the text in the starter voices' style, keeping the order of the keys you wrote.
- Every change is checked by the library itself: **Problems** lists what is wrong, each with the JSON path of its value, and the form marks the field; click a problem to select it in the text. The file format is in the [guide](guide.md#voices-voicejson).
- **Undo** and **Redo** (Ctrl+Z, Ctrl+Shift+Z) step through the edits of both tabs.
- **Play** speaks the test line with the voice; the text types out as it is spoken. **Pause**, **Resume**, **Skip** and **Stop** do what they do in a game (`Speaker.Pause`, `Resume`, `Skip`, `Stop`). Enter in the test line plays it too. Browsers start audio only after a click or key press; the first Play is that click.
- A valid edit applies at once: change an instrument while a line plays and the next notes use the new sound. Changes to timing and pitch are heard the next time the line plays: with **Replay on change** on, the line plays again after such an edit (and after any edit while nothing plays), once audio has started.
- **Download** saves the file (under the name it was opened with, else the voice's name), **Copy** puts the JSON on the clipboard. Only valid voices play; any text can be downloaded.
- The page keeps the current document, the test line and the volume in the browser's local storage and restores them on the next visit.

Voices with sample instruments (`"sample": ...`) and custom languages are not supported yet: a sample instrument is reported as a problem.

## Building and running locally

```bash
make editor        # the site in build/editor (voice.wasm, wasm_exec.js from your Go, the page, the starter voices)
make editor-serve  # builds and serves it on http://localhost:8000
make editor-test   # node --test of the page's scripts
make editor-smoke  # headless Chromium: every starter plays, problems, transport, open, download, draft
```

`make editor-smoke` needs Playwright with Chromium (`require('playwright')` must work, for example through `NODE_PATH`). The site is static and uses relative URLs, so any static file server works; it must serve `.wasm` as `application/wasm`.

## Deployment

`.github/workflows/pages.yml` runs `make editor` on every push to `main` (and by hand) and deploys `build/editor` to GitHub Pages. It needs Pages enabled in the repository settings with the source "GitHub Actions". While the repository is private the workflow's jobs are skipped, because Pages for a private repository needs a paid plan; it deploys by itself once the repository is public. The CI workflow builds the site and runs `make editor-test` on every pull request.

How the editor is built and why: [design.md](design.md#editor).
