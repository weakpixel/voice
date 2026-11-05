// The Voice Editor page: the voice file as a form (Visual) or as text
// (Source), live validation through the Go session (voice.wasm), and
// playback of a test line by the real Speaker on a synth stream that oto
// plays. The text is the document; the form edits it through format.js.

import { utf8, revealedText, locatePath, downloadName, docName, looksLikeVoice } from "./text.js";
import { format } from "./format.js";
import { VisualEditor } from "./visual.js";
import { installTooltips } from "./tooltip.js";

const $ = (id) => /** @type {HTMLElement} */ (document.getElementById(id));
const docArea = /** @type {HTMLTextAreaElement} */ ($("doc"));
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

const DRAFT_KEY = "voice-editor/draft";
const VALIDATE_DELAY = 250; // ms after the last keystroke

/** @type {any} the Go API, set once voice.wasm has started */
let api = null;
let openedName = ""; // file name of the opened document, "" for a starter or a new one
let voiceOK = false;
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
      JSON.stringify({ doc: docArea.value, line: lineInput.value, opened: openedName, volume: volume.value, view, replay: replay.checked }),
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
  view = v === "source" ? "source" : "visual";
  for (const b of document.querySelectorAll("[data-view]")) {
    const on = /** @type {HTMLElement} */ (b).dataset.view === view;
    b.classList.toggle("active", on);
    b.setAttribute("aria-selected", String(on));
  }
  $("visual").hidden = view !== "visual";
  docArea.hidden = view !== "source";
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
  showProblems(r.problems || []);
  const v = $("validity");
  v.textContent = r.ok ? "Valid" : `${(r.problems || []).length} problem${(r.problems || []).length === 1 ? "" : "s"}`;
  v.className = `validity ${r.ok ? "ok" : "bad"}`;
  buttons.play.disabled = !r.ok;
  visual?.showProblems(r.problems || []);
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
  const tl = api.say(lineText, { speed: 1 });
  if (!tl) return;
  $("events").textContent = "";
  maxPeak = 0; // the meter's data-max-peak is per line (the smoke test reads it)
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
  const shown = revealedText(lineBytes, st.revealed);
  $("shown").textContent = shown;
  $("hidden").textContent = lineText.slice(shown.length);
  $("time").textContent = `${st.time.toFixed(2)} s`;
  if (st.events.length) {
    $("events").textContent = [$("events").textContent, ...st.events].filter(Boolean).join(" · ");
  }
  meter();
  if (st.done) {
    playing = false;
    // Keep the meter running briefly so the tail shows.
    setTimeout(meter, 100);
  }
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
  for (const b of [buttons.pause, buttons.skip, buttons.stop]) b.disabled = !playing;
}

function control(action) {
  if (!api) return;
  api[action]();
  // Show the result now: the next frame would come only while playing.
  showState(api.update(0));
  if (playing) return;
  setTransport();
}

// ---- files ----

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
  lineInput.addEventListener("input", saveDraft);
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
  $("open").addEventListener("click", () => fileInput.click());
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
    if (!(e.ctrlKey || e.metaKey) || e.altKey || e.target === lineInput) return;
    const k = e.key.toLowerCase();
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
    const f = e.dataTransfer?.files?.[0];
    if (f) openFile(f);
  });
  document.addEventListener("paste", (e) => {
    if (e.target === docArea || e.target === lineInput) return;
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
  try {
    const fields = await (await fetch("fields.json")).json();
    visual = new VisualEditor($("visual"), fields, api.percussionDefaults(), onVisualChange);
  } catch (e) {
    toast(`The visual editor could not load: ${e}`);
  }
  validate();
  record();
  committedDoc = parsed();
  setView(draft?.view || "visual");
  setTransport();
  document.body.dataset.ready = "true";
}

main();
