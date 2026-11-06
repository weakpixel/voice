// Edits of the dialogue document as a plain object, for the Script tab.
// Pure: every function returns a changed copy and never mutates its input,
// so node --test covers them. A field at its default is left out of the
// file, as in the example.

/** The defaults of the optional line fields; a value equal to its default is removed. */
export const LINE_DEFAULTS = { delay: 0, speed: 1, instant: false };

/** @param {any} doc @returns {any[]} */
export const speakers = (doc) => (Array.isArray(doc?.speakers) ? doc.speakers : []);
/** @param {any} doc @returns {any[]} */
export const lines = (doc) => (Array.isArray(doc?.lines) ? doc.lines : []);

function withList(doc, key, list) {
  return { ...doc, [key]: list };
}

/** A speaker id not used yet, from base (letters, digits, - and _ only). */
export function uniqueId(doc, base) {
  const clean = String(base || "speaker").toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "") || "speaker";
  const used = new Set(speakers(doc).map((s) => s?.id));
  if (!used.has(clean)) return clean;
  for (let n = 2; ; n++) if (!used.has(`${clean}-${n}`)) return `${clean}-${n}`;
}

/** Adds a speaker with a starter voice (the first not used by another speaker when possible). */
export function addSpeaker(doc, starters) {
  const used = new Set(speakers(doc).map((s) => s?.voice));
  const voice = starters.find((s) => !used.has(s)) || starters[0] || "soft";
  const id = uniqueId(doc, voice);
  const name = id[0].toUpperCase() + id.slice(1);
  return withList(doc, "speakers", [...speakers(doc), { id, name, voice }]);
}

/** Whether any line is said by the speaker with this id. */
export function speakerUsed(doc, id) {
  return lines(doc).some((l) => l?.speaker === id);
}

/** Removes speaker i; the caller checks [speakerUsed] first (lines left behind are a loader problem). */
export function removeSpeaker(doc, i) {
  return withList(doc, "speakers", speakers(doc).filter((_, k) => k !== i));
}

/**
 * Sets a speaker field (id, name, voice or pan); an empty name or pan
 * (undefined) removes the key. Renaming the id renames it in the lines.
 */
export function setSpeakerField(doc, i, key, value) {
  const old = speakers(doc)[i];
  if (!old) return doc;
  const sp = { ...old };
  if (value === undefined || value === "" || (key === "pan" && Number.isNaN(value))) delete sp[key];
  else sp[key] = value;
  let out = withList(doc, "speakers", speakers(doc).map((s, k) => (k === i ? sp : s)));
  if (key === "id" && old.id !== sp.id) {
    out = withList(out, "lines", lines(out).map((l) => (l?.speaker === old.id ? { ...l, speaker: sp.id } : l)));
  }
  return out;
}

/** The id of the speaker after the line's, in speaker order (a dialogue takes turns). */
export function nextSpeaker(doc, index) {
  const sp = speakers(doc);
  if (sp.length === 0) return "";
  const at = sp.findIndex((s) => s?.id === lines(doc)[index]?.speaker);
  return sp[(at + 1) % sp.length].id;
}

/** Inserts an empty line after index (-1: at the start), said by speaker. */
export function addLine(doc, index, speaker) {
  const l = lines(doc);
  return withList(doc, "lines", [...l.slice(0, index + 1), { speaker, text: "" }, ...l.slice(index + 1)]);
}

export function removeLine(doc, i) {
  return withList(doc, "lines", lines(doc).filter((_, k) => k !== i));
}

/** Moves line i by delta places (-1 up, 1 down); out of range does nothing. */
export function moveLine(doc, i, delta) {
  const l = [...lines(doc)];
  const j = i + delta;
  if (i < 0 || i >= l.length || j < 0 || j >= l.length) return doc;
  [l[i], l[j]] = [l[j], l[i]];
  return withList(doc, "lines", l);
}

/**
 * Sets a line field. undefined, an empty value and a value equal to the
 * field's default (delay 0, speed 1, instant false) remove the key.
 * advance and hold have no fixed default: they are removed with undefined.
 */
export function setLineField(doc, i, key, value) {
  const old = lines(doc)[i];
  if (!old) return doc;
  const l = { ...old };
  const isDefault = key in LINE_DEFAULTS && value === LINE_DEFAULTS[key];
  if (value === undefined || value === "" || Number.isNaN(value) || (key !== "text" && isDefault)) delete l[key];
  else l[key] = value;
  if (key === "text") l.text = value ?? "";
  return withList(doc, "lines", lines(doc).map((x, k) => (k === i ? l : x)));
}
