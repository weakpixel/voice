// The Voice Editor page: the voice file as a form (Visual) or as text
// (Source), live validation through the Go session (voice.wasm), and
// playback of a test line by the real Speaker on a synth stream that oto
// plays. The text is the document; the form edits it through format.js.

import { utf8, revealedText, locatePath, downloadName, docName, looksLikeVoice } from "./text.js";
import { format } from "./format.js";
import { VisualEditor } from "./visual.js";
import { installTooltips } from "./tooltip.js";
import { PianoRoll } from "./pianoroll.js";
import { saveSample, deleteSample, storedSamples } from "./samples.js";
import { encodeShare, decodeShare } from "./share.js";
import { initDialogue } from "./dialogue.js";
import { initTheme } from "./theme.js";

const $ = (id) => /** @type {HTMLElement} */ (document.getElementById(id));
const docArea = /** @type {HTMLTextAreaElement} */ ($("doc"));
const langArea = /** @type {HTMLTextAreaElement} */ ($("lang"));
const lineInput = /** @type {HTMLInputElement} */ ($("line"));
const starter = /** @type {HTMLSelectElement} */ ($("starter"));
const fileInput = /** @type {HTMLInputElement} */ ($("file"));
const volume = /** @type {HTMLInputElement} */ ($("volume"));
const replay = /** @type {HTMLInputElement} */ ($("replay"));
const buttons = {
  play: /** @type {HTMLButtonElement} */ ($("play")),
  pause: /** @type {HTMLButtonElement} */ ($("pause")),
  skip: /** @type {HTMLButtonElement} */ ($("skip")),
  stop: /** @type {HTMLButtonElement} */ ($("stop")),
};

const roll = new PianoRoll(/** @type {HTMLCanvasElement} */ ($("roll")), /** @type {HTMLCanvasElement} */ ($("wave")));
const DRAFT_KEY = "voice-editor/draft";
const VALIDATE_DELAY = 250; // ms after the last keystroke

/** @type {any} the Go API, set once voice.wasm has started */
let api = null;
let openedName = ""; // file name of the opened document, "" for a starter or a new one
let voiceOK = false;
let voiceRhythm = false; // the voice speaks on a beat grid
/** @type {VisualEditor | null} */
let visual = null;
let view = "visual"; // "visual" or "source"
/** The document at the last commit (an edit that ended), to tell what changed. */
let committedDoc = null;
const HISTORY_SIZE = 200;
const history = { stack: /** @type {string[]} */ ([]), index: -1 };

// ---- draft (per-viewer convenience; the page works without storage) ----

function loadDraft() {
  try {
    const d = JSON.parse(localStorage.getItem(DRAFT_KEY) || "null");
    return d && typeof d.doc === "string" ? d : null;
  } catch {
    return null;
  }
}

function saveDraft() {
  try {
    localStorage.setItem(
      DRAFT_KEY,
      JSON.stringify({ doc: docArea.value, lang: langArea.value, line: lineInput.value, opened: openedName, volume: volume.value, view, replay: replay.checked }),
    );
  } catch {
    // storage blocked or full: the draft is a convenience only
  }
}

// ---- document ----

/** @returns {any} the document as an object, null while the text is not JSON */
function parsed() {
  try {
    const d = JSON.parse(docArea.value);
    return d !== null && typeof d === "object" && !Array.isArray(d) ? d : null;
  } catch {
    return null;
  }
}

/** Replaces the document, e.g. after opening a file or picking a starter. */
function setDocument(text, name) {
  openedName = name;
  fileHandle = null; // Save asks again where to save
  docArea.value = text;
  docArea.scrollTop = 0;
  validate();
  refreshVisual();
  record();
  saveDraft();
  committedDoc = parsed(); // a new document does not start playing by itself
}

/** Shows text as the document and commits it (history, draft, replay). */
function applyText(text, fromHistory = false) {
  docArea.value = text;
  validate();
  refreshVisual();
  commit(fromHistory);
}

/** An edit ended: remember it for undo, save the draft, replay if wanted. */
function commit(fromHistory = false) {
  if (!fromHistory) record();
  saveDraft();
  const doc = parsed();
  maybeReplay(committedDoc, doc);
  committedDoc = doc;
}

let validateTimer = 0;
function scheduleValidate() {
  clearTimeout(validateTimer);
  validateTimer = setTimeout(() => {
    validate();
    commit();
  }, VALIDATE_DELAY);
}

// ---- undo and redo: snapshots of the text ----

function record() {
  const text = docArea.value;
  if (history.stack[history.index] === text) return;
  history.stack = history.stack.slice(0, history.index + 1);
  history.stack.push(text);
  if (history.stack.length > HISTORY_SIZE) history.stack.shift();
  history.index = history.stack.length - 1;
  updateUndo();
}

function step(by) {
  clearTimeout(validateTimer);
  record(); // a source edit still waiting for its debounce
  const i = history.index + by;
  if (i < 0 || i >= history.stack.length) return;
  history.index = i;
  applyText(history.stack[i], true);
  updateUndo();
}

function updateUndo() {
  /** @type {HTMLButtonElement} */ ($("undo")).disabled = history.index <= 0;
  /** @type {HTMLButtonElement} */ ($("redo")).disabled = history.index >= history.stack.length - 1;
}

// ---- views ----

function setView(v) {
  view = ["source", "language"].includes(v) ? v : "visual";
  for (const b of document.querySelectorAll("[data-view]")) {
    const on = /** @type {HTMLElement} */ (b).dataset.view === view;
    b.classList.toggle("active", on);
    b.setAttribute("aria-selected", String(on));
  }
  $("visual").hidden = view !== "visual";
  docArea.hidden = view !== "source";
  $("language").hidden = view !== "language";
  refreshVisual();
  saveDraft();
}

/** Redraws the form from the text (when the Visual tab shows). */
function refreshVisual() {
  if (!visual || view !== "visual") return;
  const doc = parsed();
  if (doc) {
    visual.render(doc);
    return;
  }
  const note = document.createElement("p");
  note.className = "hint";
  note.textContent = "The JSON has a syntax error, so the form cannot show it. Fix it in Source (the problem is listed on the right).";
  $("visual").replaceChildren(note);
}

/** The form changed the document: write it as text and check it. */
function onVisualChange(doc, how) {
  docArea.value = format(doc);
  validate();
  if (how.commit) commit();
}

/** Without instruments, percussion sounds and names: what decides timing and pitch. */
function composition(doc) {
  if (!doc) return "";
  const { instruments, percussion, name, language, ...rest } = doc;
  return JSON.stringify(rest);
}

/** Plays the line after an edit, unless replay is off; an instrument edit
 * during a line is heard on its next notes, so the line keeps playing. */
function maybeReplay(prev, next) {
  // Before the first Play there is no audio yet (it starts on a click).
  if (!replay.checked || !voiceOK || !next || !api || !api.audioReady()) return;
  if (JSON.stringify(prev) === JSON.stringify(next)) return;
  if (playing && composition(prev) === composition(next)) return;
  play();
}

/** Loads the document in Go: a valid one becomes the voice at once (its
 * instruments change even during a line), problems are listed. */
function validate() {
  const name = docName(docArea.value);
  $("doc-name").textContent = openedName || name || "";
  if (!api) return;
  const r = api.loadVoice(docArea.value);
  voiceOK = r.ok;
  if (r.ok) {
    voiceRhythm = r.rhythm;
    $("rhythm-options").hidden = !r.rhythm;
  }
  showProblems(r.problems || []);
  const v = $("validity");
  v.textContent = r.ok ? "Valid" : `${(r.problems || []).length} problem${(r.problems || []).length === 1 ? "" : "s"}`;
  v.className = `validity ${r.ok ? "ok" : "bad"}`;
  buttons.play.disabled = !r.ok;
  visual?.showProblems(r.problems || []);
  if (r.ok) scheduleRender();
}

/** @param {{path: string, message: string}[]} problems */
function showProblems(problems) {
  const list = $("problems");
  list.replaceChildren();
  if (problems.length === 0) {
    const li = document.createElement("li");
    li.className = "none";
    li.textContent = "None";
    list.append(li);
    return;
  }
  for (const p of problems) {
    const li = document.createElement("li");
    const b = document.createElement("button");
    b.type = "button";
    if (p.path) {
      const code = document.createElement("code");
      code.textContent = p.path;
      b.append(code, `: ${p.message}`);
    } else {
      b.textContent = p.message;
    }
    b.addEventListener("click", () => jumpTo(p));
    li.append(b);
    list.append(li);
  }
}

/** Selects the place a problem is about in the text. */
function jumpTo(p) {
  let range = p.path ? locatePath(docArea.value, p.path) : null;
  const m = !range && /^line (\d+), column (\d+)/.exec(p.message);
  if (m) {
    const lines = docArea.value.split("\n");
    const start = lines.slice(0, Number(m[1]) - 1).reduce((n, l) => n + l.length + 1, 0) + Number(m[2]) - 1;
    range = { start, end: start + 1 };
  }
  docArea.focus();
  if (range) {
    docArea.setSelectionRange(range.start, range.end);
    const line = docArea.value.slice(0, range.start).split("\n").length;
    docArea.scrollTop = Math.max(0, (line - 4) * parseFloat(getComputedStyle(docArea).lineHeight));
  }
}

// ---- playback ----

let playing = false;
let auditioning = false; // the playing sound is an audition, not the line
let lastFrame = 0;
let lineBytes = utf8("");
let lineText = "";
let shownPeak = 0;
let maxPeak = 0;

async function play() {
  if (!api || !voiceOK) return;
  if (!(await audioStarted())) {
    toast(api.audioError() || "Audio did not start. Click the page and try again.");
    return;
  }
  lineText = lineInput.value;
  lineBytes = utf8(lineText);
  const tl = api.say(lineText, sayOptions());
  if (!tl) return;
  auditioning = false;
  roll.setTimeline(tl);
  $("events").textContent = "";
  maxPeak = 0; // the meter's data-max-peak is per line (the smoke test reads it)
  playing = true;
  lastFrame = performance.now();
  setTransport();
  requestAnimationFrame(frame);
}

/** Plays a short phrase on one instrument (or a few hits on a drum). */
async function audition(name) {
  if (!api || !voiceOK) return;
  if (!(await audioStarted())) {
    toast(api.audioError() || "Audio did not start. Click the page and try again.");
    return;
  }
  const err = api.audition(name);
  if (err) {
    toast(err);
    return;
  }
  auditioning = true;
  maxPeak = 0;
  playing = true;
  lastFrame = performance.now();
  setTransport();
  requestAnimationFrame(frame);
}

/** Waits up to two seconds for oto to resume the AudioContext (it does on
 * the click that started Play) and start its player. */
async function audioStarted() {
  for (let i = 0; i < 120 && !api.audioReady(); i++) {
    await new Promise((r) => requestAnimationFrame(r));
  }
  const ok = api.audioReady();
  $("audio-state").textContent = ok ? "Audio on" : "Audio off";
  return ok;
}

function frame(now) {
  if (!playing) return;
  // A hidden tab gets no frames; cap the step so the line does not jump.
  const dt = Math.min(now - lastFrame, 100);
  lastFrame = now;
  showState(api.update(dt));
  if (playing) requestAnimationFrame(frame);
}

/** @param {{revealed: number, done: boolean, paused: boolean, time: number, events: string[]}} st */
function showState(st) {
  if (!auditioning) {
    const shown = revealedText(lineBytes, st.revealed);
    $("shown").textContent = shown;
    $("hidden").textContent = lineText.slice(shown.length);
  }
  $("time").textContent = `${st.time.toFixed(2)} s`;
  if (st.events.length) {
    $("events").textContent = [$("events").textContent, ...st.events].filter(Boolean).join(" · ");
  }
  meter();
  if (st.done && !st.audition) {
    playing = false;
    auditioning = false;
    // Keep the meter running briefly so the tail shows.
    setTimeout(meter, 100);
  }
  roll.setTime(playing && !auditioning ? st.time : null);
  buttons.pause.querySelector(".label").textContent = st.paused ? "Resume" : "Pause";
  setTransport();
}

function meter() {
  const p = api.peak();
  maxPeak = Math.max(maxPeak, p);
  shownPeak = Math.max(p, shownPeak * 0.85);
  const bar = $("meter-bar");
  bar.style.width = `${Math.min(100, shownPeak * 100)}%`;
  bar.dataset.maxPeak = String(maxPeak);
}

function setTransport() {
  buttons.play.disabled = !voiceOK;
  buttons.stop.disabled = !playing;
  // An audition can only be stopped.
  for (const b of [buttons.pause, buttons.skip]) b.disabled = !playing || auditioning;
  $("wav").toggleAttribute("disabled", !voiceOK);
}

// ---- piano roll, waveform and WAV ----

const RENDER_DELAY = 300; // ms after the last edit
const RENDER_BLOCKS = 16; // blocks of 64 frames per renderStep call
let renderTimer = 0;
let renderJob = 0;
let renderingKey = "";
let renderPromise = Promise.resolve(false);
let renderedKey = "";

/** What the render depends on: the document and the line. */
function renderKey() {
  return `${docArea.value}\u0000${lineInput.value}\u0000${JSON.stringify(sayOptions())}`;
}

/** The preview options of a line: the music clock and the beat's mute, for rhythmic voices. */
function sayOptions() {
  const o = { speed: 1 };
  if (!voiceRhythm) return o;
  o.muteBeat = /** @type {HTMLInputElement} */ ($("mute-beat")).checked;
  if (/** @type {HTMLInputElement} */ ($("clock")).checked) {
    o.bpm = Number(/** @type {HTMLInputElement} */ ($("clock-bpm")).value) || 100;
    o.phase = Math.max(0, Number(/** @type {HTMLInputElement} */ ($("clock-phase")).value) || 0);
  }
  return o;
}

/** Renders the line again once edits settle. */
function scheduleRender() {
  clearTimeout(renderTimer);
  renderTimer = setTimeout(renderNow, RENDER_DELAY);
}

/**
 * Renders the line offline and shows its notes and waveform. The render
 * runs in slices between frames (a long divine line takes about a second
 * in all), smaller ones while a line plays so the audio keeps its share of
 * the main thread. A newer render replaces a running one.
 * @returns {Promise<boolean>} whether the shown render is current
 */
function renderNow() {
  if (!api || !voiceOK) return Promise.resolve(false);
  const key = renderKey();
  if (key === renderedKey) return Promise.resolve(true);
  if (key === renderingKey) return renderPromise;
  const job = ++renderJob;
  renderingKey = key;
  renderPromise = (async () => {
    if (!api.renderStart(lineInput.value, sayOptions())) return false;
    for (;;) {
      const budget = playing ? 3 : 10; // ms per frame
      const t0 = performance.now();
      let r = null;
      while (!r && performance.now() - t0 < budget) r = api.renderStep(RENDER_BLOCKS, roll.columns());
      if (r) {
        renderedKey = key;
        renderingKey = "";
        roll.setTimeline(r.timeline);
        roll.setWave(r.peaks, r.duration);
        return true;
      }
      await new Promise((res) => requestAnimationFrame(res));
      if (job !== renderJob) return false; // a newer render took over
    }
  })();
  return renderPromise;
}

/** Downloads the test line as a WAV file. */
async function downloadWAV() {
  if (!(await renderNow())) return;
  const bytes = api.wav();
  if (!bytes) return;
  const base = downloadName(docArea.value, openedName).replace(/(\.voice)?\.json$/, "");
  const url = URL.createObjectURL(new Blob([bytes], { type: "audio/wav" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = `${base}-line.wav`;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function control(action) {
  if (!api) return;
  api[action]();
  // Show the result now: the next frame would come only while playing.
  showState(api.update(0));
  if (playing) return;
  setTransport();
}

// ---- language ----

let langTimer = 0;

/** Loads the language text in Go (empty is the built-in language) and lists its problems. */
function validateLanguage() {
  if (!api) return;
  const r = api.loadLanguage(langArea.value);
  const problems = r.problems || [];
  const state = $("lang-state");
  state.textContent = langArea.value.trim() === "" ? "Built-in" : r.ok ? "Valid" : `${problems.length} problem${problems.length === 1 ? "" : "s"}`;
  state.className = `validity ${r.ok ? "ok" : "bad"}`;
  $("tab-language").classList.toggle("bad", !r.ok);
  $("lang-problems").replaceChildren(
    ...problems.map((p) => Object.assign(document.createElement("li"), { textContent: p.path ? `${p.path}: ${p.message}` : p.message })),
  );
  if (r.ok) {
    renderedKey = ""; // the same document and line sound different in another language
    scheduleRender();
  }
}

function setLanguage(text) {
  langArea.value = text;
  validateLanguage();
  saveDraft();
}

/** Adds WAV files as samples (by file name), keeps them and checks the voice again. */
async function addSampleFiles(files) {
  for (const f of files) {
    const bytes = new Uint8Array(await f.arrayBuffer());
    const err = api.addSample(f.name, bytes);
    if (err) {
      toast(err);
      continue;
    }
    await saveSample(f.name, bytes);
  }
  samplesChanged();
}

async function removeSample(name) {
  api.removeSample(name);
  await deleteSample(name);
  samplesChanged();
}

/** Shows the added samples and checks the voice again (a sample may now resolve). */
function samplesChanged() {
  const names = api.samples() || [];
  const list = $("samples");
  list.replaceChildren();
  if (!names.length) {
    const li = document.createElement("li");
    li.className = "none";
    li.textContent = "None";
    list.append(li);
  }
  for (const n of names) {
    const li = document.createElement("li");
    const label = document.createElement("span");
    label.textContent = n;
    const rm = document.createElement("button");
    rm.type = "button";
    rm.className = "small";
    rm.textContent = "Remove";
    rm.title = `Forget ${n}`;
    rm.addEventListener("click", () => removeSample(n));
    li.append(label, rm);
    list.append(li);
  }
  $("sample-names").replaceChildren(...names.map((n) => Object.assign(document.createElement("option"), { value: n })));
  renderedKey = ""; // the same document may sound different now
  validate();
}

async function openFile(file) {
  try {
    setDocument(await file.text(), file.name);
  } catch (e) {
    toast(`Could not read ${file.name}: ${e}`);
  }
}

function download() {
  const name = downloadName(docArea.value, openedName);
  const url = URL.createObjectURL(new Blob([docArea.value], { type: "application/json" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

// Save writes back to a file through the File System Access API where the
// browser has it (Chromium); elsewhere only Download is offered.
const canSave = "showSaveFilePicker" in window;
/** @type {any} the file the document was opened from or saved to, if any */
let fileHandle = null;

/** Opens a voice file through the file picker that can save back to it. */
async function openWithHandle() {
  try {
    const [h] = await /** @type {any} */ (window).showOpenFilePicker({ types: [{ description: "Voice file", accept: { "application/json": [".json"] } }] });
    const file = await h.getFile();
    setDocument(await file.text(), file.name);
    fileHandle = h;
  } catch (e) {
    if (e?.name !== "AbortError") toast(String(e));
  }
}

/** Saves to the opened file, or asks where to save the first time. */
async function save() {
  try {
    if (!fileHandle) {
      fileHandle = await /** @type {any} */ (window).showSaveFilePicker({
        suggestedName: downloadName(docArea.value, openedName),
        types: [{ description: "Voice file", accept: { "application/json": [".json"] } }],
      });
      openedName = fileHandle.name;
    }
    const w = await fileHandle.createWritable();
    await w.write(docArea.value);
    await w.close();
    $("doc-name").textContent = openedName;
    toast(`Saved ${fileHandle.name}`);
  } catch (e) {
    if (e?.name !== "AbortError") toast(`Could not save: ${e}`);
  }
}

/** Copies a link that opens the current document. */
async function share() {
  const url = location.href.split("#")[0] + (await encodeShare(docArea.value));
  try {
    await navigator.clipboard.writeText(url);
    toast("Link copied");
  } catch {
    prompt("Copy this link:", url);
  }
}

/** Opens the document a share link carries, then clears the link from the address. */
async function openShared() {
  const text = await decodeShare(location.hash);
  if (text === null) return false;
  window.history.replaceState(null, "", location.pathname + location.search);
  setDocument(text, "");
  return true;
}

async function copy() {
  try {
    await navigator.clipboard.writeText(docArea.value);
    toast("Copied");
  } catch {
    docArea.select();
    toast("Press Ctrl+C to copy");
  }
}

async function loadStarter(name) {
  const res = await fetch(`presets/${name}.voice.json`);
  if (!res.ok) throw new Error(`presets/${name}.voice.json: ${res.status}`);
  return res.text();
}

let toastTimer = 0;
function toast(text) {
  const t = $("toast");
  t.textContent = text;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), 2500);
}

// ---- wiring ----

function wire() {
  docArea.addEventListener("input", scheduleValidate);
  docArea.addEventListener("keydown", (e) => {
    if (e.key === "Tab" && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      docArea.setRangeText("  ", docArea.selectionStart, docArea.selectionEnd, "end");
      scheduleValidate();
    }
  });
  lineInput.addEventListener("input", () => {
    saveDraft();
    scheduleRender();
  });
  $("wav").addEventListener("click", downloadWAV);
  langArea.addEventListener("input", () => {
    clearTimeout(langTimer);
    langTimer = setTimeout(() => {
      validateLanguage();
      saveDraft();
    }, VALIDATE_DELAY);
  });
  const langFile = /** @type {HTMLInputElement} */ ($("lang-file"));
  $("lang-open").addEventListener("click", () => langFile.click());
  langFile.addEventListener("change", async () => {
    const f = langFile.files?.[0];
    langFile.value = "";
    if (f) setLanguage(await f.text());
  });
  $("lang-builtin").addEventListener("click", async () => {
    try {
      setLanguage(await (await fetch("presets/default.lang.json")).text());
    } catch (e) {
      toast(String(e));
    }
  });
  $("lang-clear").addEventListener("click", () => setLanguage(""));
  const sampleFile = /** @type {HTMLInputElement} */ ($("sample-file"));
  $("add-sample").addEventListener("click", () => sampleFile.click());
  sampleFile.addEventListener("change", () => {
    if (sampleFile.files?.length) addSampleFiles([...sampleFile.files]);
    sampleFile.value = "";
  });
  for (const id of ["mute-beat", "clock", "clock-bpm", "clock-phase"]) $(id).addEventListener("change", scheduleRender);
  lineInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") play();
  });
  buttons.play.addEventListener("click", play);
  buttons.pause.addEventListener("click", () => control(buttons.pause.textContent.includes("Resume") ? "resume" : "pause"));
  buttons.skip.addEventListener("click", () => control("skip"));
  buttons.stop.addEventListener("click", () => control("stop"));
  volume.addEventListener("input", () => {
    api?.setVolume(Number(volume.value));
    saveDraft();
  });
  $("open").addEventListener("click", () => (canSave ? openWithHandle() : fileInput.click()));
  $("save").hidden = !canSave;
  $("save").addEventListener("click", save);
  $("share").addEventListener("click", share);
  window.addEventListener("hashchange", openShared);
  fileInput.addEventListener("change", () => {
    if (fileInput.files?.[0]) openFile(fileInput.files[0]);
    fileInput.value = "";
  });
  $("download").addEventListener("click", download);
  $("undo").addEventListener("click", () => step(-1));
  $("redo").addEventListener("click", () => step(1));
  for (const b of document.querySelectorAll("[data-view]")) {
    b.addEventListener("click", () => setView(/** @type {HTMLElement} */ (b).dataset.view));
  }
  replay.addEventListener("change", saveDraft);
  document.addEventListener("keydown", (e) => {
    if (!(e.ctrlKey || e.metaKey) || e.altKey || e.target === lineInput || e.target === langArea) return;
    const k = e.key.toLowerCase();
    if (k === "s" && canSave) {
      e.preventDefault();
      save();
      return;
    }
    if (k === "z" || k === "y") {
      e.preventDefault();
      step(k === "y" || e.shiftKey ? 1 : -1);
    }
  });
  $("copy").addEventListener("click", copy);
  starter.addEventListener("change", async () => {
    const name = starter.value;
    starter.value = "";
    if (!name) return;
    try {
      setDocument(await loadStarter(name), "");
    } catch (e) {
      toast(String(e));
    }
  });

  // Drop a file anywhere; paste JSON outside the text area to open it.
  let depth = 0;
  const drop = $("drop");
  document.addEventListener("dragenter", (e) => {
    if (e.dataTransfer?.types.includes("Files")) drop.hidden = ++depth <= 0;
  });
  document.addEventListener("dragleave", () => (drop.hidden = --depth <= 0));
  document.addEventListener("dragover", (e) => e.preventDefault());
  document.addEventListener("drop", (e) => {
    e.preventDefault();
    depth = 0;
    drop.hidden = true;
    const files = [...(e.dataTransfer?.files || [])];
    const wavs = files.filter((f) => /\.wav$/i.test(f.name));
    if (wavs.length) addSampleFiles(wavs);
    const doc = files.find((f) => !/\.wav$/i.test(f.name));
    if (doc) openFile(doc);
  });
  document.addEventListener("paste", (e) => {
    if (e.target === docArea || e.target === lineInput || e.target === langArea) return;
    const text = e.clipboardData?.getData("text") || "";
    if (looksLikeVoice(text)) {
      e.preventDefault();
      setDocument(text, "");
    }
  });
}

async function fillStarters() {
  try {
    const names = await (await fetch("presets/index.json")).json();
    for (const n of names) starter.append(new Option(n, n));
    return names;
  } catch {
    return [];
  }
}

async function startGo() {
  const ready = new Promise((r) => addEventListener("voiceeditorready", r, { once: true }));
  const version = document.querySelector('meta[name="version"]')?.getAttribute("content") || "";
  const go = new Go();
  const res = await WebAssembly.instantiateStreaming(fetch(`voice.wasm?v=${version}`), go.importObject);
  go.run(res.instance);
  await ready;
  return /** @type {any} */ (globalThis).voiceEditor;
}

async function main() {
  wire();
  installTooltips();
  buttons.play.disabled = true;
  const draft = loadDraft();
  const [names] = await Promise.all([fillStarters()]);
  if (draft) {
    docArea.value = draft.doc;
    langArea.value = draft.lang || "";
    lineInput.value = draft.line || lineInput.value;
    openedName = draft.opened || "";
    volume.value = draft.volume || volume.value;
    replay.checked = draft.replay ?? true;
  } else if (names.length) {
    docArea.value = await loadStarter(names[0]);
  }
  $("doc-name").textContent = "Loading…";
  try {
    api = await startGo();
  } catch (e) {
    $("doc-name").textContent = "";
    toast(`The editor could not start: ${e}`);
    return;
  }
  api.setVolume(Number(volume.value));
  for (const [name, bytes] of await storedSamples()) api.addSample(name, bytes);
  try {
    const fields = await (await fetch("fields.json")).json();
    visual = new VisualEditor($("visual"), fields, api.percussionDefaults(), onVisualChange, audition);
  } catch (e) {
    toast(`The visual editor could not load: ${e}`);
  }
  validateLanguage();
  samplesChanged(); // lists the samples and validates
  await openShared();
  record();
  committedDoc = parsed();
  setView(draft?.view || "visual");
  setTransport();
  initTheme($("theme"), () => roll.draw());
  initDialogue({ stopPreview: () => api?.stop(), toast });
  document.body.dataset.ready = "true";
}

main();
