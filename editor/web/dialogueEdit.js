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

/** Whether any line is said by the speaker with this id, or gives a voice span to it. */
export function speakerUsed(doc, id) {
  return lines(doc).some((l) => l?.speaker === id) || speakerInSpans(doc, id);
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
    out = withList(out, "lines", lines(out).map((l) => {
      if (!l || typeof l !== "object") return l;
      const text = typeof l.text === "string" ? renameInText(l.text, old.id, sp.id) : l.text;
      return l.speaker === old.id || text !== l.text ? { ...l, speaker: l.speaker === old.id ? sp.id : l.speaker, text } : l;
    }));
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

// ---- voice markup in a line's text: {id}…{/} (or {/id}), {id~} for an impression ----

/**
 * The voice spans of a line's text as written (offsets are string indexes
 * into the text with its markup): open is the opening tag, close the
 * closing one, inner the text between. Malformed markup is skipped (the
 * library's loader reports it); spans do not nest.
 * @param {string} text
 * @returns {{name: string, impression: boolean, open: [number, number], close: [number, number]}[]}
 */
export function spansOf(text) {
  const out = [];
  let open = null;
  const s = String(text ?? "");
  for (let i = 0; i < s.length; ) {
    if (s[i] !== "{") {
      i++;
      continue;
    }
    if (s[i + 1] === "{") {
      i += 2;
      continue;
    }
    const j = s.indexOf("}", i + 1);
    if (j < 0) break;
    const tag = s.slice(i + 1, j);
    if (tag.startsWith("/")) {
      const name = tag.slice(1);
      if (open && (name === "" || name === open.name) && open.open[1] < i) out.push({ ...open, close: [i, j + 1] });
      open = null;
    } else if (!open && /^[a-z0-9_.-]+~?$/.test(tag)) {
      const impression = tag.endsWith("~");
      open = { name: impression ? tag.slice(0, -1) : tag, impression, open: [i, j + 1] };
    }
    i = j + 1;
  }
  return out;
}

/** The span whose tags or text contain the index pos (or the selection start..end), or undefined. */
export function spanAt(text, pos, end = pos) {
  return spansOf(text).find((sp) => sp.open[0] <= pos && end <= sp.close[1]);
}

/** The text with its voice markup removed, as the library shows it ({{ becomes {). */
export function shownText(text) {
  let out = "";
  const s = String(text ?? "");
  const spans = spansOf(s);
  let k = 0;
  for (let i = 0; i < s.length; ) {
    const sp = spans[k];
    if (sp && i === sp.open[0]) {
      i = sp.open[1];
      continue;
    }
    if (sp && i === sp.close[0]) {
      i = sp.close[1];
      k++;
      continue;
    }
    if (s[i] === "{" && s[i + 1] === "{") {
      out += "{";
      i += 2;
      continue;
    }
    out += s[i++];
  }
  return out;
}

const tagOf = (id, impression) => `{${id}${impression ? "~" : ""}}`;

/**
 * Gives the text start..end of line i to speaker id's voice: wraps it in
 * {id}…{/} ({id~} with impression), without the spaces at its ends. A
 * selection inside a span changes that span's voice instead. A selection
 * that is empty or crosses a span's tags changes nothing.
 */
export function wrapSpan(doc, i, start, end, id, impression = false) {
  const text = String(lines(doc)[i]?.text ?? "");
  const inside = spanAt(text, start, end);
  if (inside) return setLineField(doc, i, "text", text.slice(0, inside.open[0]) + tagOf(id, impression) + text.slice(inside.open[1]));
  while (start < end && /\s/.test(text[start])) start++;
  while (end > start && /\s/.test(text[end - 1])) end--;
  if (start >= end) return doc;
  if (spansOf(text).some((sp) => start < sp.close[1] && sp.open[0] < end)) return doc;
  return setLineField(doc, i, "text", text.slice(0, start) + tagOf(id, impression) + text.slice(start, end) + "{/}" + text.slice(end));
}

/** Removes the voice span at index pos of line i (its tags; the words stay). */
export function unwrapSpan(doc, i, pos) {
  const text = String(lines(doc)[i]?.text ?? "");
  const sp = spanAt(text, pos);
  if (!sp) return doc;
  return setLineField(doc, i, "text", text.slice(0, sp.open[0]) + text.slice(sp.open[1], sp.close[0]) + text.slice(sp.close[1]));
}

/** The text with every tag naming from renamed to to. */
export function renameInText(text, from, to) {
  const s = String(text ?? "");
  let out = "";
  let at = 0;
  for (const sp of spansOf(s)) {
    if (sp.name !== from) continue;
    out += s.slice(at, sp.open[0]) + tagOf(to, sp.impression);
    const close = s.slice(sp.close[0], sp.close[1]);
    out += s.slice(sp.open[1], sp.close[0]) + (close === "{/}" ? close : `{/${to}}`);
    at = sp.close[1];
  }
  return out + s.slice(at);
}

/** Whether a line's voice span uses the speaker with this id. */
export function speakerInSpans(doc, id) {
  return lines(doc).some((l) => spansOf(l?.text).some((sp) => sp.name === id));
}
