// Formatting and editing the voice document as a plain object. Pure, so
// node --test covers it. The style is the starter voices': the top level,
// "instruments", "percussion", "rhythm" and "rhythm.beat" one key per line,
// everything inside them on one line.

/** Paths of the objects written one key per line ("" is the top level). */
const MULTILINE = new Set(["", "instruments", "percussion", "rhythm", "rhythm.beat"]);

/** Key order of the guide, used where a form adds a key. */
export const KEY_ORDER = {
  "": ["version", "name", "language", "baseNote", "pitch", "variation", "velocity", "length", "speed", "pan", "motifVolume", "breath", "seed", "instruments", "percussion", "rhythm"],
  instruments: ["vowel", "sonorant", "fricative", "plosive", "motif"],
  instrument: ["base", "note", "wave", "pulseWidth", "detune", "unison", "spread", "layers", "glideFrom", "glideMs", "attackMs", "decayMs", "sustain", "releaseMs", "filter", "cutoff", "resonance", "vibratoRate", "vibratoDepth", "vibratoDelayMs", "reverb", "gain", "sample", "root", "loop", "loopStart", "loopEnd"],
  rhythm: ["bpm", "division", "swing", "accents", "rest", "countIn", "beatVolume", "beat"],
  "rhythm.rest": ["pause", "end"],
};

/**
 * Writes a document in the starter voices' style, with a final newline.
 * @param {any} doc @returns {string}
 */
export function format(doc) {
  return write(doc, "", 0) + "\n";
}

function write(v, path, depth) {
  if (Array.isArray(v)) return `[${v.map((x) => write(x, path + "[]", depth)).join(", ")}]`;
  if (v === null || typeof v !== "object") return JSON.stringify(v);
  const keys = Object.keys(v);
  if (keys.length === 0) return "{}";
  const child = (k) => write(v[k], path === "" ? k : `${path}.${k}`, depth + 1);
  if (!MULTILINE.has(path)) return `{${keys.map((k) => `${JSON.stringify(k)}: ${child(k)}`).join(", ")}}`;
  const pad = "  ".repeat(depth + 1);
  return `{\n${keys.map((k) => `${pad}${JSON.stringify(k)}: ${child(k)}`).join(",\n")}\n${"  ".repeat(depth)}}`;
}

/** @param {string} path @returns {string[]} the keys of a dotted path */
export function splitPath(path) {
  return path === "" ? [] : path.split(".");
}

/** @param {any} doc @param {string} path @returns {any} the value at path, undefined if absent */
export function get(doc, path) {
  let v = doc;
  for (const k of splitPath(path)) {
    if (v === null || typeof v !== "object" || !(k in v)) return undefined;
    v = v[k];
  }
  return v;
}

/** The canonical key order of the object at parentPath. */
function orderFor(parentPath) {
  if (parentPath.startsWith("instruments.") || parentPath.startsWith("percussion.")) return KEY_ORDER.instrument;
  return KEY_ORDER[parentPath] || [];
}

/**
 * Returns a copy of doc with value at path. Missing objects on the way are
 * created. A new key goes after the last existing key that comes before it
 * in the guide's order, so the document keeps its shape; existing keys keep
 * their place.
 * @param {any} doc @param {string} path @param {any} value @returns {any}
 */
export function set(doc, path, value) {
  return setIn(doc, splitPath(path), value, "");
}

function setIn(doc, keys, value, parentPath) {
  if (keys.length === 0) return value;
  const [k, ...rest] = keys;
  const obj = doc !== null && typeof doc === "object" && !Array.isArray(doc) ? doc : {};
  const here = parentPath === "" ? k : `${parentPath}.${k}`;
  const sub = setIn(obj[k], rest, value, here);
  if (k in obj) return { ...obj, [k]: sub };
  const order = orderFor(parentPath);
  const rank = order.indexOf(k);
  const existing = Object.keys(obj);
  // Insert after the last key ranked before k; with none, before the first
  // ranked key; unknown keys and keys of unknown objects go last.
  let at = existing.length;
  if (rank >= 0) {
    const before = existing.map((e) => order.indexOf(e)).map((r, i) => (r >= 0 && r < rank ? i : -1)).filter((i) => i >= 0);
    if (before.length) at = before[before.length - 1] + 1;
    else {
      const first = existing.findIndex((e) => order.indexOf(e) >= 0);
      if (first >= 0) at = first;
    }
  }
  const out = {};
  existing.forEach((e, i) => {
    if (i === at) out[k] = sub;
    out[e] = obj[e];
  });
  if (at === existing.length) out[k] = sub;
  return out;
}

/**
 * Returns a copy of doc without the key at path (so the library's default
 * applies). An object left empty by it is removed too, except an
 * instrument: an empty instrument is a sine, not "the vowel's sound".
 * @param {any} doc @param {string} path @returns {any}
 */
export function remove(doc, path) {
  return removeIn(doc, splitPath(path), "");
}

function removeIn(doc, keys, parentPath) {
  if (keys.length === 0 || doc === null || typeof doc !== "object") return doc;
  const [k, ...rest] = keys;
  if (!(k in doc)) return doc;
  const out = { ...doc };
  if (rest.length === 0) {
    delete out[k];
    return out;
  }
  const here = parentPath === "" ? k : `${parentPath}.${k}`;
  out[k] = removeIn(doc[k], rest, here);
  const v = out[k];
  const empty = v && typeof v === "object" && !Array.isArray(v) && Object.keys(v).length === 0;
  if (empty && parentPath !== "instruments" && here !== "instruments") delete out[k];
  return out;
}
