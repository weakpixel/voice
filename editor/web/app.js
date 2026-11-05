// The Voice Editor page: the voice file as text, live validation through
// the Go session (voice.wasm), and playback of a test line by the real
// Speaker on a synth stream that oto plays.

import { utf8, revealedText, locatePath, downloadName, docName, looksLikeVoice } from "./text.js";

const $ = (id) => /** @type {HTMLElement} */ (document.getElementById(id));
const docArea = /** @type {HTMLTextAreaElement} */ ($("doc"));
const lineInput = /** @type {HTMLInputElement} */ ($("line"));
const starter = /** @type {HTMLSelectElement} */ ($("starter"));
const fileInput = /** @type {HTMLInputElement} */ ($("file"));
const volume = /** @type {HTMLInputElement} */ ($("volume"));
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
    localStorage.setItem(DRAFT_KEY, JSON.stringify({ doc: docArea.value, line: lineInput.value, opened: openedName, volume: volume.value }));
  } catch {
    // storage blocked or full: the draft is a convenience only
  }
}

// ---- document ----

/** Replaces the document, e.g. after opening a file or picking a starter. */
function setDocument(text, name) {
  docArea.value = text;
  openedName = name;
  docArea.scrollTop = 0;
  validate();
  saveDraft();
}

let validateTimer = 0;
function scheduleValidate() {
  clearTimeout(validateTimer);
  validateTimer = setTimeout(() => {
    validate();
    saveDraft();
  }, VALIDATE_DELAY);
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
  buttons.play.disabled = true;
  const draft = loadDraft();
  const [names] = await Promise.all([fillStarters()]);
  if (draft) {
    docArea.value = draft.doc;
    lineInput.value = draft.line || lineInput.value;
    openedName = draft.opened || "";
    volume.value = draft.volume || volume.value;
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
  validate();
  setTransport();
  document.body.dataset.ready = "true";
}

main();
