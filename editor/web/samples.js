// Sample files the user added, kept in the browser's IndexedDB so they are
// there again after a reload. Every call tolerates a browser without
// storage (private windows, blocked site data): the samples then last
// until the page closes.

const DB = "voice-editor";
const STORE = "samples";

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

/** Runs one request on the store; resolves to its result, or undefined on any failure. */
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

/** Stores a sample file's bytes under its name. */
export function saveSample(name, bytes) {
  return run("readwrite", (s) => s.put(bytes, name));
}

/** Forgets a stored sample. */
export function deleteSample(name) {
  return run("readwrite", (s) => s.delete(name));
}

/** @returns {Promise<[string, Uint8Array][]>} every stored sample */
export async function storedSamples() {
  const names = (await run("readonly", (s) => s.getAllKeys())) || [];
  const out = [];
  for (const name of names) {
    const bytes = await run("readonly", (s) => s.get(name));
    if (bytes) out.push([String(name), new Uint8Array(bytes)]);
  }
  return out;
}
