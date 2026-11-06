// The Dialogue mode of the page: the dialogue document as text, and the game
// (game.html in an iframe, its own Go program with Ebiten) that plays it.
// The game is loaded the first time the mode is shown; the page talks to it
// with postMessage (see editor/game/main.go for the messages).

import { summarize, statusText } from "./dialogueDoc.js";

const $ = (id) => /** @type {HTMLElement} */ (document.getElementById(id));
const DRAFT_KEY = "voice-editor/dialogue-draft";
const MODE_KEY = "voice-editor/mode";
const EDIT_DELAY = 300; // ms after the last keystroke, until the game gets the edit

/**
 * @param {{ stopPreview: () => void, toast: (msg: string) => void }} hooks
 */
export function initDialogue({ stopPreview, toast }) {
  const doc = /** @type {HTMLTextAreaElement} */ ($("dlg-doc"));
  const frame = /** @type {HTMLIFrameElement} */ ($("dlg-game"));
  const auto = /** @type {HTMLInputElement} */ ($("dlg-auto"));
  const from = /** @type {HTMLInputElement} */ ($("dlg-from"));
  const volume = /** @type {HTMLInputElement} */ ($("dlg-volume"));
  const pauseButton = $("dlg-pause");
  const version = document.querySelector('meta[name="version"]')?.getAttribute("content") || "";

  let loadedFrame = false; // game.html requested
  let gameReady = false;
  let paused = false;
  let index = 0; // the line the game is on
  /** @type {ReturnType<typeof setTimeout> | undefined} */
  let timer;

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
      }
      fitFrame();
    } else if (gameReady) {
      post({ type: "pause" });
      setPaused(true);
    }
  }
  $("mode-voice").addEventListener("click", () => setMode("voice"));
  $("mode-dialogue").addEventListener("click", () => setMode("dialogue"));

  // ---- talking to the game ----

  function post(msg) {
    frame.contentWindow?.postMessage({ source: "voice-editor", ...msg }, location.origin);
  }

  function sendLoad(start) {
    post({ type: "load", dialogue: doc.value, start, autoAdvance: auto.checked });
    setPaused(false);
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
        $("dlg-audio").dataset.running = String(msg.running);
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
    const v = $("dlg-validity");
    v.textContent = list.length ? `${list.length} problem${list.length === 1 ? "" : "s"}` : "Valid";
    v.className = `validity ${list.length ? "bad" : "ok"}`;
  }

  // ---- the document ----

  function saveDraft() {
    try {
      localStorage.setItem(DRAFT_KEY, JSON.stringify({ doc: doc.value, auto: auto.checked, volume: volume.value }));
    } catch {}
  }

  doc.addEventListener("input", () => {
    saveDraft();
    clearTimeout(timer);
    timer = setTimeout(() => {
      if (gameReady) sendLoad(Math.min(index, Math.max(0, (summarize(doc.value)?.length ?? 1) - 1)));
    }, EDIT_DELAY);
  });

  async function loadExample() {
    doc.value = await (await fetch(`village.dialogue.json?v=${version}`)).text();
    saveDraft();
    index = 0;
    if (gameReady) sendLoad(0);
  }
  $("dlg-example").addEventListener("click", () => loadExample().catch((e) => toast(`Could not load the example: ${e}`)));

  const file = /** @type {HTMLInputElement} */ ($("dlg-file"));
  $("dlg-open").addEventListener("click", () => file.click());
  file.addEventListener("change", async () => {
    const f = file.files?.[0];
    file.value = "";
    if (!f) return;
    doc.value = await f.text();
    $("dlg-name").textContent = f.name;
    saveDraft();
    if (gameReady) sendLoad(0);
  });
  $("dlg-download").addEventListener("click", () => {
    let name = "dialogue";
    try {
      name = JSON.parse(doc.value).name || name;
    } catch {}
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([doc.value], { type: "application/json" }));
    a.download = `${String(name).replace(/[^\w.-]+/g, "-").replace(/^-+|-+$/g, "") || "dialogue"}.dialogue.json`;
    a.click();
    URL.revokeObjectURL(a.href);
  });
  $("dlg-copy").addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(doc.value);
      toast("Copied the dialogue JSON");
    } catch {
      toast("The browser did not allow copying");
    }
  });

  // ---- transport ----

  function setPaused(p) {
    paused = p;
    pauseButton.querySelector(".label").textContent = p ? "Resume" : "Pause";
  }
  $("dlg-restart").addEventListener("click", () => {
    if (!gameReady) return;
    sendLoad(0); // also picks up edits that are still waiting
    clearTimeout(timer);
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
  volume.addEventListener("input", () => {
    saveDraft();
    post({ type: "volume", value: Number(volume.value) });
  });

  // ---- the game's size: whole-pixel multiples of 480 x 270 ----

  function fitFrame() {
    const room = /** @type {HTMLElement} */ ($("dlg-stage")).clientWidth;
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
    auto.checked = !!draft.auto;
    volume.value = draft.volume || volume.value;
  } else {
    loadExample().catch(() => {});
  }
  let mode = "voice";
  try {
    mode = localStorage.getItem(MODE_KEY) === "dialogue" ? "dialogue" : "voice";
  } catch {}
  setMode(mode);
}
