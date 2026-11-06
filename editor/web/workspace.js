// The workspace: the voice files the Dialogue mode can name, kept by file
// name in the browser's IndexedDB (own database, the pattern of samples.js).
// Every call tolerates a browser without storage: the workspace then lasts
// until the page closes (the in-memory map).

const DB = "voice-editor-workspace";
const STORE = "voices";

/** Used when IndexedDB is not available; also the cache of what was stored. */
const memory = new Map();

/** @returns {Promise<IDBDatabase | null>} */
function open() {
  return new Promise((resolve) => {
    try {
      const req = indexedDB.open(DB, 1);
      req.onupgradeneeded = () => req.result.createObjectStore(STORE);
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => resolve(null);
    } catch {
      resolve(null);
    }
  });
}

async function run(mode, make) {
  const db = await open();
  if (!db) return undefined;
  return new Promise((resolve) => {
    try {
      const req = make(db.transaction(STORE, mode).objectStore(STORE));
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => resolve(undefined);
    } catch {
      resolve(undefined);
    }
  });
}

/** Stores a voice file's text under its file name. */
export async function saveVoice(name, text) {
  memory.set(name, text);
  await run("readwrite", (s) => s.put(text, name));
}

/** Forgets a voice file. */
export async function deleteVoice(name) {
  memory.delete(name);
  await run("readwrite", (s) => s.delete(name));
}

/** @returns {Promise<Map<string, string>>} every voice file, by name, sorted by name */
export async function allVoices() {
  const keys = await run("readonly", (s) => s.getAllKeys());
  const values = await run("readonly", (s) => s.getAll());
  const out = new Map(memory);
  if (keys && values) keys.forEach((k, i) => out.set(String(k), String(values[i])));
  return new Map([...out].sort(([a], [b]) => a.localeCompare(b)));
}

/**
 * The names of the voice files a dialogue document uses, in speaker order,
 * without starter voices.
 * @param {any} doc @param {string[]} starters
 * @returns {string[]}
 */
export function usedVoiceFiles(doc, starters) {
  const out = [];
  for (const sp of Array.isArray(doc?.speakers) ? doc.speakers : []) {
    const v = sp && sp.voice;
    if (typeof v === "string" && v && !starters.includes(v) && !out.includes(v)) out.push(v);
  }
  return out;
}
