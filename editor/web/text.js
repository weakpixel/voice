// Pure helpers of the page: no DOM, no wasm, so node --test covers them.

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/** @param {string} text @returns {Uint8Array} the UTF-8 bytes of text */
export function utf8(text) {
  return encoder.encode(text);
}

/**
 * The text shown after `n` bytes are revealed. The library counts the reveal
 * in UTF-8 bytes and always ends between characters.
 * @param {Uint8Array} bytes @param {number} n @returns {string}
 */
export function revealedText(bytes, n) {
  return decoder.decode(bytes.subarray(0, Math.max(0, Math.min(n, bytes.length))));
}

/**
 * Finds where the value at a JSON path (as the library reports it, for
 * example "instruments.vowel.sustain" or "phonemes[2].symbol") is written in
 * a document: the key of the last named segment, searched after the keys of
 * the segments before it. A heuristic for jumping to a problem; null when a
 * key is not found.
 * @param {string} doc @param {string} path @returns {{start: number, end: number} | null}
 */
export function locatePath(doc, path) {
  const keys = path.split(".").map((seg) => seg.replace(/\[\d+\]/g, "")).filter((k) => k !== "");
  if (keys.length === 0) return null;
  let from = 0;
  let found = null;
  for (const key of keys) {
    const re = new RegExp(`"${escapeRegExp(key)}"\\s*:`, "g");
    re.lastIndex = from;
    const m = re.exec(doc);
    if (!m) return null;
    found = { start: m.index, end: m.index + key.length + 2 };
    from = re.lastIndex;
  }
  return found;
}

function escapeRegExp(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * The file name to download a document as: the opened file's name, else the
 * voice's name, else "voice", with the .voice.json extension.
 * @param {string} doc @param {string} [opened] @returns {string}
 */
export function downloadName(doc, opened) {
  if (opened) return opened;
  const name = docName(doc).replace(/[^\w.-]+/g, "-").replace(/^-+|-+$/g, "");
  return `${name || "voice"}.voice.json`;
}

/** @param {string} doc @returns {string} the "name" of the document, "" if none or unparseable */
export function docName(doc) {
  try {
    const v = JSON.parse(doc);
    return v && typeof v.name === "string" ? v.name : "";
  } catch {
    return "";
  }
}

/** @param {string} text @returns {boolean} whether pasted text looks like a voice document */
export function looksLikeVoice(text) {
  try {
    const v = JSON.parse(text);
    return v !== null && typeof v === "object" && !Array.isArray(v) && "instruments" in v;
  } catch {
    return false;
  }
}

/** @param {string} text @returns {boolean} whether text looks like a dialogue document */
export function looksLikeDialogue(text) {
  try {
    const v = JSON.parse(text);
    return v !== null && typeof v === "object" && !Array.isArray(v) && Array.isArray(v.lines);
  } catch {
    return false;
  }
}
