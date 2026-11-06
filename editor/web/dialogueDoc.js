// What the Dialogue mode reads from the dialogue document without the Go
// loader: how many lines it has and who says each, for the status line. The
// game (Go) is the authority on whether the document is valid.

/**
 * @typedef {{ who: string, text: string }} LineInfo
 */

/**
 * Summarises a dialogue document. Returns null while the text is not a
 * JSON object with a lines array.
 * @param {string} text
 * @returns {LineInfo[] | null}
 */
export function summarize(text) {
  let doc;
  try {
    doc = JSON.parse(text);
  } catch {
    return null;
  }
  if (!doc || typeof doc !== "object" || !Array.isArray(doc.lines)) return null;
  /** @type {Map<string, string>} */
  const names = new Map();
  for (const sp of Array.isArray(doc.speakers) ? doc.speakers : []) {
    if (sp && typeof sp.id === "string") names.set(sp.id, typeof sp.name === "string" && sp.name ? sp.name : sp.id);
  }
  return doc.lines.map((l) => {
    const id = l && typeof l.speaker === "string" ? l.speaker : "";
    return { who: names.get(id) || id, text: l && typeof l.text === "string" ? l.text : "" };
  });
}

/**
 * The status line for the game being on line index of lines.
 * @param {LineInfo[] | null} lines
 * @param {number} index
 */
export function statusText(lines, index) {
  if (!lines || index < 0 || index >= lines.length) return `Line ${index + 1}`;
  return `Line ${index + 1} of ${lines.length}: ${lines[index].who}`;
}
