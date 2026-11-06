// The Dialogue mode of the page: the dialogue document as a Script (speakers
// and lines) or as text (Source), the workspace of voice files its speakers
// use, and the game (game.html in an iframe, its own Go program with Ebiten)
// that plays it. The game is loaded the first time the mode is shown; the
// page talks to it with postMessage (see editor/game/main.go).

import { summarize, statusText } from "./dialogueDoc.js";
import { ScriptEditor } from "./dialogueScript.js";
import { formatDialogue } from "./format.js";
import { allVoices, saveVoice, usedVoiceFiles } from "./workspace.js";
import { storedSamples } from "./samples.js";
import { encodeBundle, decodeBundle } from "./share.js";

const $ = (id) => /** @type {HTMLElement} */ (document.getElementById(id));
const DRAFT_KEY = "voice-editor/dialogue-draft";
const MODE_KEY = "voice-editor/mode";
const EDIT_DELAY = 300; // ms after the last keystroke, until the game gets the edit
const HISTORY_SIZE = 200;

/**
 * @param {{
 *   stopPreview: () => void,
 *   toast: (msg: string) => void,
 *   starters: string[],
 *   speak: (voiceText: string, text: string, opts: {speed: number, instant: boolean}) => Promise<void>,
 *   openVoice: (ref: string) => Promise<boolean>,
 *   restoreVoice: () => void,
 * }} hooks
 */
export function initDialogue({ stopPreview, toast, starters, speak, openVoice, restoreVoice }) {
  const doc = /** @type {HTMLTextAreaElement} */ ($("dlg-doc"));
  const frame = /** @type {HTMLIFrameElement} */ ($("dlg-game"));
  const auto = /** @type {HTMLInputElement} */ ($("dlg-auto"));
  const liveBox = /** @type {HTMLInputElement} */ ($("dlg-live"));
  const from = /** @type {HTMLInputElement} */ ($("dlg-from"));
  const volume = /** @type {HTMLInputElement} */ ($("dlg-volume"));
  const pauseButton = $("dlg-pause");
  const version = document.querySelector('meta[name="version"]')?.getAttribute("content") || "";

  let loadedFrame = false; // game.html requested
  let gameReady = false;
  let paused = false;
  let index = 0; // the line the game is on
  let view = "script"; // "script" or "source"
  let fileName = ""; // the opened dialogue file's name, for Download
  /** @type {ReturnType<typeof setTimeout> | undefined} */
  let timer;
  /** @type {ReturnType<typeof setTimeout> | undefined} */
  let recordTimer;
  const history = { stack: /** @type {string[]} */ ([]), index: -1 };

  /** @returns {any} the document as an object, null while the text is not JSON */
  function parsed() {
    try {
      const d = JSON.parse(doc.value);
      return d !== null && typeof d === "object" && !Array.isArray(d) ? d : null;
    } catch {
      return null;
    }
  }

  // ---- the Script tab ----

  const script = new ScriptEditor($("dlg-script"), {
    starters,
    onChange: (d) => setText(formatDialogue(d), { redraw: false }),
    onPreview: previewLine,
    onPlayFrom: (i) => {
      if (gameReady) sendLoad(i);
    },
    onEditVoice: async (ref) => {
      if (await openVoice(ref)) setMode("voice");
    },
    onAddVoiceFile: () => voiceFiles.click(),
  });

  async function redrawScript(focus = "") {
    script.setDoc(parsed(), [...(await allVoices()).keys()], focus);
  }

  // ---- mode ----

  function setMode(mode) {
    document.body.dataset.mode = mode;
    $("mode-voice").classList.toggle("active", mode === "voice");
    $("mode-dialogue").classList.toggle("active", mode === "dialogue");
    try {
      localStorage.setItem(MODE_KEY, mode);
    } catch {}
    if (mode === "dialogue") {
      stopPreview(); // two audio contexts, one sounding at a time
      if (!loadedFrame) {
        loadedFrame = true;
        frame.src = `game.html?v=${version}`;
      } else if (gameReady) {
        sendLoad(index, false); // voices edited in the Voice mode reach the game
      }
      redrawScript();
      fitFrame();
    } else {
      restoreVoice(); // line previews borrowed the Voice mode's voice
      if (gameReady) {
        post({ type: "pause" });
        setPaused(true);
      }
    }
  }
  $("mode-voice").addEventListener("click", () => setMode("voice"));
  $("mode-dialogue").addEventListener("click", () => setMode("dialogue"));

  function setView(v) {
    view = v;
    $("dlg-script").hidden = v !== "script";
    doc.hidden = v !== "source";
    $("dlg-tab-script").classList.toggle("active", v === "script");
    $("dlg-tab-source").classList.toggle("active", v === "source");
    saveDraft();
  }
  $("dlg-tab-script").addEventListener("click", () => setView("script"));
  $("dlg-tab-source").addEventListener("click", () => setView("source"));

  // ---- talking to the game ----

  function post(msg) {
    frame.contentWindow?.postMessage({ source: "voice-editor", ...msg }, location.origin);
  }

  /** The workspace voice files the dialogue uses, and the sample files, as the game takes them. */
  async function request(start) {
    const d = parsed();
    const used = usedVoiceFiles(d, starters);
    const files = await allVoices();
    const voices = Object.fromEntries(used.filter((n) => files.has(n)).map((n) => [n, files.get(n)]));
    const samples = used.length ? Object.fromEntries(await storedSamples()) : {};
    return { type: "load", dialogue: doc.value, voices, samples, start, autoAdvance: auto.checked };
  }

  async function sendLoad(start, unpause = true) {
    post(await request(start));
    if (unpause) setPaused(false);
  }

  addEventListener("message", (e) => {
    if (e.origin !== location.origin || !e.data || e.data.source !== "voice-game") return;
    const msg = e.data;
    switch (msg.type) {
      case "ready":
        gameReady = true;
        $("dlg-note").textContent = "";
        sendLoad(0);
        break;
      case "line":
        index = msg.index;
        from.value = String(index + 1);
        script.setCurrent(index);
        $("dlg-status").textContent = statusText(summarize(doc.value), index);
        break;
      case "finished":
        $("dlg-status").textContent = "The dialogue is finished. Restart plays it again.";
        break;
      case "problems":
        showProblems(msg.list);
        break;
      case "audio":
        $("dlg-audio").textContent = msg.running ? "Sound is on" : "Click the game once to start sound";
        break;
      case "error":
        $("dlg-note").textContent = `The game could not start: ${msg.message}`;
        break;
    }
  });

  /** @param {{file: string, path: string, message: string}[]} list */
  function showProblems(list) {
    const ul = $("dlg-problems");
    ul.replaceChildren();
    if (!list.length) {
      const li = document.createElement("li");
      li.className = "none";
      li.textContent = "None";
      ul.append(li);
    }
    for (const p of list) {
      const li = document.createElement("li");
      const code = document.createElement("code");
      code.textContent = p.path ? `${p.file}: ${p.path}` : p.file;
      li.append(code, ` ${p.message}`);
      ul.append(li);
    }
    script.setProblems(list.filter((p) => p.file === "dialogue"));
    const v = $("dlg-validity");
    v.textContent = list.length ? `${list.length} problem${list.length === 1 ? "" : "s"}` : "Valid";
    v.className = `validity ${list.length ? "bad" : "ok"}`;
  }

  // ---- the document and its history ----

  function saveDraft() {
    try {
      localStorage.setItem(DRAFT_KEY, JSON.stringify({ doc: doc.value, name: fileName, view, auto: auto.checked, live: liveBox.checked, volume: volume.value }));
    } catch {}
  }

  function record() {
    clearTimeout(recordTimer);
    if (history.stack[history.index] === doc.value) return;
    history.stack = history.stack.slice(0, history.index + 1);
    history.stack.push(doc.value);
    if (history.stack.length > HISTORY_SIZE) history.stack.shift();
    history.index = history.stack.length - 1;
    updateUndo();
  }

  function updateUndo() {
    /** @type {HTMLButtonElement} */ ($("dlg-undo")).disabled = history.index <= 0;
    /** @type {HTMLButtonElement} */ ($("dlg-redo")).disabled = history.index >= history.stack.length - 1;
  }

  /** Makes text the document: draft, history, the Script tab, and the game after a short pause. */
  function setText(text, { redraw = true, immediate = false, fromHistory = false } = {}) {
    doc.value = text;
    saveDraft();
    if (!fromHistory) {
      if (immediate || redraw) record();
      else {
        clearTimeout(recordTimer);
        recordTimer = setTimeout(record, 500);
      }
    }
    if (redraw) redrawScript();
    scheduleSend();
  }

  /** Sends the edit to the game once typing stops. */
  function scheduleSend() {
    clearTimeout(timer);
    timer = setTimeout(() => {
      if (!gameReady || !liveBox.checked) return; // off: edits wait for Restart
      const n = summarize(doc.value)?.length ?? 1;
      sendLoad(Math.min(index, Math.max(0, n - 1)), false);
    }, EDIT_DELAY);
  }

  doc.addEventListener("input", () => {
    setText(doc.value, { redraw: false });
    redrawScript();
  });

  function step(by) {
    record();
    const i = history.index + by;
    if (i < 0 || i >= history.stack.length) return;
    history.index = i;
    setText(history.stack[i], { fromHistory: true });
    updateUndo();
  }
  $("dlg-undo").addEventListener("click", () => step(-1));
  $("dlg-redo").addEventListener("click", () => step(1));
  document.addEventListener("keydown", (e) => {
    if (document.body.dataset.mode !== "dialogue" || !(e.ctrlKey || e.metaKey) || e.altKey) return;
    const k = e.key.toLowerCase();
    if (k === "z" || k === "y") {
      e.preventDefault();
      step(k === "y" || e.shiftKey ? 1 : -1);
    }
  });

  async function loadExample() {
    openDocument(await (await fetch(`village.dialogue.json?v=${version}`)).text(), "");
  }
  $("dlg-example").addEventListener("click", () => loadExample().catch((e) => toast(`Could not load the example: ${e}`)));

  /** Opens text as the dialogue (a file, a drop, a paste or a link). */
  function openDocument(text, name) {
    fileName = name;
    $("dlg-name").textContent = name;
    index = 0;
    setText(text, { immediate: true });
    if (gameReady) sendLoad(0);
  }

  const file = /** @type {HTMLInputElement} */ ($("dlg-file"));
  $("dlg-open").addEventListener("click", () => file.click());
  file.addEventListener("change", async () => {
    const f = file.files?.[0];
    file.value = "";
    if (f) openDocument(await f.text(), f.name);
  });

  // Voice files for the workspace.
  const voiceFiles = /** @type {HTMLInputElement} */ ($("dlg-voice-files"));
  async function addVoiceFiles(files) {
    for (const f of files) await saveVoice(f.name, await f.text());
    toast(`Added ${files.map((f) => f.name).join(", ")}`);
    await redrawScript();
    scheduleSend();
  }
  voiceFiles.addEventListener("change", () => {
    if (voiceFiles.files?.length) addVoiceFiles([...voiceFiles.files]);
    voiceFiles.value = "";
  });

  // ---- download, share, copy ----

  function saveAs(name, text) {
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "application/json" }));
    a.download = name;
    document.body.append(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  }

  function dialogueFileName() {
    if (fileName) return fileName;
    const name = String(parsed()?.name || "dialogue");
    return `${name.replace(/[^\w.-]+/g, "-").replace(/^-+|-+$/g, "") || "dialogue"}.dialogue.json`;
  }

  $("dlg-download").addEventListener("click", () => saveAs(dialogueFileName(), doc.value));
  $("dlg-download-all").addEventListener("click", async () => {
    const files = await allVoices();
    const used = usedVoiceFiles(parsed(), starters);
    saveAs(dialogueFileName(), doc.value);
    for (const n of used) {
      if (!files.has(n)) continue;
      await new Promise((r) => setTimeout(r, 250)); // browsers drop downloads fired in one go
      saveAs(n, files.get(n));
    }
    toast(`Downloaded the dialogue and ${used.filter((n) => files.has(n)).length} voice file(s)`);
  });
  $("dlg-copy").addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(doc.value);
      toast("Copied the dialogue JSON");
    } catch {
      toast("The browser did not allow copying");
    }
  });
  $("dlg-share").addEventListener("click", async () => {
    const files = await allVoices();
    const voices = Object.fromEntries(usedVoiceFiles(parsed(), starters).filter((n) => files.has(n)).map((n) => [n, files.get(n)]));
    const url = location.href.split("#")[0] + (await encodeBundle({ dialogue: doc.value, voices }));
    try {
      await navigator.clipboard.writeText(url);
      toast("Link copied");
    } catch {
      prompt("Copy this link:", url);
    }
  });

  /** Opens the dialogue a #d= link carries, with its voice files; clears the link. */
  async function openShared() {
    const bundle = await decodeBundle(location.hash);
    if (!bundle) return false;
    window.history.replaceState(null, "", location.pathname + location.search);
    for (const [name, text] of Object.entries(bundle.voices)) await saveVoice(name, text);
    openDocument(bundle.dialogue, "");
    setMode("dialogue");
    return true;
  }
  addEventListener("hashchange", openShared);

  // ---- line preview: the editor's own preview says the line in the speaker's voice ----

  async function previewLine(i) {
    const d = parsed();
    const l = d?.lines?.[i];
    const sp = (d?.speakers || []).find((s) => s?.id === l?.speaker);
    if (!l || !sp || !l.text) return toast("The line needs a speaker and a text");
    try {
      let text;
      const files = await allVoices();
      if (files.has(sp.voice)) text = files.get(sp.voice);
      else if (starters.includes(sp.voice)) text = await (await fetch(`presets/${sp.voice}.voice.json`)).text();
      else return toast(`No voice file ${sp.voice} in the workspace`);
      if (typeof sp.pan === "number") text = JSON.stringify({ ...JSON.parse(text), pan: sp.pan });
      post({ type: "pause" });
      setPaused(true);
      await speak(text, l.text, { speed: l.speed ?? 1, instant: !!l.instant });
    } catch (e) {
      toast(`Could not play the line: ${e}`);
    }
  }

  // ---- transport ----

  function setPaused(p) {
    paused = p;
    pauseButton.querySelector(".label").textContent = p ? "Resume" : "Pause";
  }
  $("dlg-restart").addEventListener("click", () => {
    if (!gameReady) return;
    clearTimeout(timer);
    sendLoad(0); // also picks up edits that are still waiting
  });
  $("dlg-from-play").addEventListener("click", () => {
    if (!gameReady) return;
    const lines = summarize(doc.value);
    const n = Math.min(Math.max(1, Math.trunc(Number(from.value) || 1)), lines?.length || 1);
    sendLoad(n - 1);
  });
  pauseButton.addEventListener("click", () => {
    if (!gameReady) return;
    post({ type: paused ? "resume" : "pause" });
    setPaused(!paused);
  });
  auto.addEventListener("change", () => {
    saveDraft();
    if (gameReady) sendLoad(index);
  });
  liveBox.addEventListener("change", saveDraft);
  volume.addEventListener("input", () => {
    saveDraft();
    post({ type: "volume", value: Number(volume.value) });
  });

  // ---- the game's size: whole-pixel multiples of 480 x 270 ----

  function fitFrame() {
    const room = $("dlg-stage").clientWidth;
    const scale = Math.max(1, Math.min(4, Math.floor((room || 480) / 480)));
    frame.style.width = `${480 * scale}px`;
    frame.style.height = `${270 * scale}px`;
  }
  new ResizeObserver(fitFrame).observe($("dlg-stage"));
  frame.addEventListener("load", () => post({ type: "volume", value: Number(volume.value) }));

  // ---- start ----

  let draft = null;
  try {
    draft = JSON.parse(localStorage.getItem(DRAFT_KEY) || "null");
  } catch {}
  if (draft && typeof draft.doc === "string") {
    doc.value = draft.doc;
    fileName = draft.name || "";
    $("dlg-name").textContent = fileName;
    auto.checked = !!draft.auto;
    liveBox.checked = draft.live ?? true;
    volume.value = draft.volume || volume.value;
    record();
    redrawScript();
  } else {
    liveBox.checked = true;
    loadExample().catch(() => {});
  }
  setView(draft?.view === "source" ? "source" : "script");

  let mode = "voice";
  try {
    mode = localStorage.getItem(MODE_KEY) === "dialogue" ? "dialogue" : "voice";
  } catch {}
  openShared().then((opened) => !opened && setMode(mode));

  return {
    openDocument,
    addVoiceFiles,
    /** Shows the Dialogue mode. */
    show: () => setMode("dialogue"),
    /** Whether the Dialogue mode is showing. */
    active: () => document.body.dataset.mode === "dialogue",
  };
}
