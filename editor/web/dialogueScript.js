// The Script tab of the Dialogue mode: the speakers table and the list of
// lines, drawn from the dialogue document and editing it through the pure
// functions of dialogueEdit.js. It owns no document: every edit calls
// onChange with the new document object, and the page writes it as text.

import * as E from "./dialogueEdit.js";

/** The colour of speaker number i, to tell speakers apart in the list. */
export function speakerColor(i) {
  return `hsl(${(i * 67 + 210) % 360} 55% 52%)`;
}

/**
 * @typedef {{ path: string, message: string }} Problem
 * @typedef {{
 *   starters: string[],
 *   onChange: (doc: any) => void,
 *   onPreview: (i: number) => void,
 *   onPlayFrom: (i: number) => void,
 *   onEditVoice: (ref: string) => void,
 *   onAddVoiceFile: () => void,
 *   toast?: (message: string) => void,
 * }} Handlers
 */

/** A small element builder. */
function el(tag, props = {}, ...children) {
  const { dataset, ...rest } = /** @type {any} */ (props);
  const e = Object.assign(document.createElement(tag), rest);
  if (dataset) Object.assign(e.dataset, dataset);
  e.append(...children);
  return e;
}

export class ScriptEditor {
  /** @param {HTMLElement} root @param {Handlers} h */
  constructor(root, h) {
    this.root = root;
    this.h = h;
    this.doc = null;
    this.voiceFiles = /** @type {string[]} */ ([]);
    this.problems = /** @type {Problem[]} */ ([]);
    this.current = -1;
  }

  /** Draws doc (null while the text is not a JSON object). Focus moves to focusSelector, if given. */
  setDoc(doc, voiceFiles = this.voiceFiles, focusSelector = "") {
    this.doc = doc;
    this.voiceFiles = voiceFiles;
    this.render(focusSelector);
  }

  /** @param {Problem[]} problems shown at their rows */
  setProblems(problems) {
    this.problems = problems;
    this.showProblems();
  }

  /** @param {number} index highlights the line the game is on */
  setCurrent(index) {
    this.current = index;
    for (const li of this.root.querySelectorAll(".line")) li.classList.toggle("current", Number(/** @type {HTMLElement} */ (li).dataset.i) === index);
  }

  /** An edit by the user: keeps the document, tells the page; redraw also rebuilds the rows. */
  edit(doc, redraw, focusSelector = "") {
    this.doc = doc;
    this.h.onChange(doc);
    if (redraw) this.render(focusSelector);
    else this.showProblems(); // problems of the old text are stale until the game answers
  }

  render(focusSelector) {
    const root = this.root;
    root.replaceChildren();
    if (!this.doc) {
      root.append(el("p", { className: "hint", textContent: "The text is not a JSON object. Fix it in the Source tab." }));
      return;
    }
    root.append(this.speakersSection(), this.linesSection());
    this.setCurrent(this.current);
    this.showProblems();
    if (focusSelector) /** @type {HTMLElement | null} */ (root.querySelector(focusSelector))?.focus();
  }

  speakersSection() {
    const doc = this.doc;
    const sp = E.speakers(doc);
    const table = el("div", { className: "speakers" });
    sp.forEach((s, i) => {
      const row = el("div", { className: "speaker", dataset: { i: String(i), path: `speakers[${i}]` } });
      row.style.setProperty("--c", speakerColor(i));
      const id = el("input", { type: "text", value: s?.id ?? "", className: "sp-id", ariaLabel: "Speaker id", title: "Id: letters, digits, - and _; the lines refer to it" });
      id.addEventListener("change", () => this.edit(E.setSpeakerField(this.doc, i, "id", id.value.trim()), true));
      const name = el("input", { type: "text", value: s?.name ?? "", placeholder: s?.id ?? "name", className: "sp-name", ariaLabel: "Name shown in the box", title: "The name the dialogue box shows (the id when empty)" });
      name.addEventListener("input", () => this.edit(E.setSpeakerField(this.doc, i, "name", name.value), false));
      const voice = this.voiceSelect(s?.voice ?? "", (v) => this.edit(E.setSpeakerField(this.doc, i, "voice", v), true));
      const pan = el("input", { type: "number", min: "-1", max: "1", step: "0.1", value: s?.pan === undefined ? "" : String(s.pan), placeholder: "pan", className: "sp-pan", ariaLabel: "Pan", title: "Stereo position, -1 left to 1 right; empty keeps the voice's own" });
      pan.addEventListener("input", () => this.edit(E.setSpeakerField(this.doc, i, "pan", pan.value === "" ? undefined : Number(pan.value)), false));
      const used = E.speakerUsed(doc, s?.id);
      const edit = el("button", { type: "button", className: "small", textContent: "Edit voice", title: "Open this voice in the Voice mode" });
      edit.addEventListener("click", () => this.h.onEditVoice(s?.voice ?? ""));
      const del = el("button", { type: "button", className: "small", textContent: "✕", ariaLabel: "Delete speaker", disabled: used, title: used ? "Lines or voice spans still use this speaker" : "Delete the speaker" });
      del.addEventListener("click", () => this.edit(E.removeSpeaker(this.doc, i), true));
      row.append(id, name, voice, pan, edit, del, el("div", { className: "row-problem" }));
      table.append(row);
    });
    const add = el("button", { type: "button", className: "small", textContent: "Add speaker" });
    add.addEventListener("click", () => this.edit(E.addSpeaker(this.doc, this.h.starters), true, ".speaker:last-of-type .sp-name"));
    const file = el("button", { type: "button", className: "small", textContent: "Add voice file…", title: "Add *.voice.json files to the workspace, for the voice dropdowns" });
    file.addEventListener("click", () => this.h.onAddVoiceFile());
    return el("section", {}, el("h3", { textContent: "Speakers" }), table, el("div", { className: "add-row" }, add, file));
  }

  /** The voice dropdown: starters, then workspace files; an unknown value stays selectable. */
  voiceSelect(value, onChange) {
    const select = el("select", { className: "sp-voice", ariaLabel: "Voice" });
    const group = (label, names) => {
      const g = el("optgroup", { label });
      for (const n of names) g.append(new Option(n, n));
      return g;
    };
    select.append(group("Starter voices", this.h.starters));
    if (this.voiceFiles.length) select.append(group("Voice files", this.voiceFiles));
    if (value && ![...this.h.starters, ...this.voiceFiles].includes(value)) select.append(new Option(`${value} (missing)`, value));
    select.value = value;
    select.addEventListener("change", () => onChange(select.value));
    return select;
  }

  linesSection() {
    const doc = this.doc;
    const sp = E.speakers(doc);
    const list = el("ol", { className: "lines" });
    E.lines(doc).forEach((l, i) => list.append(this.lineRow(l, i, sp)));
    const add = el("button", { type: "button", className: "small", textContent: "Add line" });
    add.addEventListener("click", () => {
      const last = E.lines(this.doc).length - 1;
      this.edit(E.addLine(this.doc, last, E.nextSpeaker(this.doc, last) || E.speakers(this.doc)[0]?.id || ""), true, ".line:last-child .text");
    });
    return el("section", {}, el("h3", { textContent: "Lines" }), list, el("div", { className: "add-row" }, add));
  }

  lineRow(l, i, sp) {
    const doc = this.doc;
    const idx = sp.findIndex((s) => s?.id === l?.speaker);
    const li = el("li", { className: "line", dataset: { i: String(i), path: `lines[${i}]` } });
    li.style.setProperty("--c", speakerColor(Math.max(0, idx)));

    const who = el("select", { className: "who", ariaLabel: "Speaker" });
    for (const s of sp) who.append(new Option(s?.name || s?.id || "?", s?.id ?? ""));
    if (idx < 0) who.append(new Option(`${l?.speaker ?? ""} (unknown)`, l?.speaker ?? ""));
    who.value = l?.speaker ?? "";
    who.addEventListener("change", () => this.edit(E.setLineField(this.doc, i, "speaker", who.value), true));

    const text = el("input", { type: "text", className: "text", value: l?.text ?? "", ariaLabel: `Line ${i + 1}`, placeholder: "What they say" });
    const marks = el("div", { className: "text-marks", ariaHidden: "true" });
    const paint = () => {
      this.paintMarks(marks, text.value, sp);
      marks.scrollLeft = text.scrollLeft;
    };
    paint();
    text.addEventListener("input", () => {
      paint();
      this.edit(E.setLineField(this.doc, i, "text", text.value), false);
    });
    text.addEventListener("scroll", () => (marks.scrollLeft = text.scrollLeft));
    const voiceBtn = el("button", { type: "button", className: "small voice-btn", textContent: "Voice ▾", title: "Give the selected words another speaker's voice (Alt+V)", ariaLabel: "Voice for the selected words", ariaHasPopup: "menu" });
    voiceBtn.addEventListener("mousedown", (e) => e.preventDefault()); // keep the text's selection
    voiceBtn.addEventListener("click", () => this.voiceMenu(i, text, voiceBtn));
    text.addEventListener("keydown", (e) => {
      if (e.altKey && (e.key === "v" || e.key === "V")) {
        e.preventDefault();
        this.voiceMenu(i, text, voiceBtn);
      } else if (e.key === "Enter" && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        this.edit(E.addLine(this.doc, i, E.nextSpeaker(this.doc, i)), true, `.line[data-i="${i + 1}"] .text`);
      } else if (e.altKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
        e.preventDefault();
        const d = e.key === "ArrowUp" ? -1 : 1;
        if (E.moveLine(this.doc, i, d) !== this.doc) this.edit(E.moveLine(this.doc, i, d), true, `.line[data-i="${i + d}"] .text`);
      }
    });

    const button = (label, title, fn, extra = {}) => {
      const b = el("button", { type: "button", className: "small icon-btn", textContent: label, title, ariaLabel: title, ...extra });
      b.addEventListener("click", fn);
      return b;
    };
    const hear = button("▸", "Hear this line in the speaker's voice", () => this.h.onPreview(i));
    const from = button("⏵", "Play the dialogue from this line in the game", () => this.h.onPlayFrom(i));
    const up = button("↑", "Move up (Alt+↑)", () => this.edit(E.moveLine(this.doc, i, -1), true), { disabled: i === 0 });
    const down = button("↓", "Move down (Alt+↓)", () => this.edit(E.moveLine(this.doc, i, 1), true), { disabled: i === E.lines(doc).length - 1 });
    const del = button("✕", "Delete the line", () => this.edit(E.removeLine(this.doc, i), true));

    const main = el("div", { className: "line-main" }, who, el("div", { className: "text-wrap" }, marks, text), voiceBtn, hear, from, up, down, del);
    li.append(main, this.timing(l, i), el("div", { className: "row-problem" }));
    return li;
  }

  /** Draws the highlight under a line's text: each voice span in its speaker's colour, its tags dimmed. */
  paintMarks(marks, value, sp) {
    marks.replaceChildren();
    let at = 0;
    for (const s of E.spansOf(value)) {
      const k = sp.findIndex((x) => x?.id === s.name);
      const color = k < 0 ? "var(--error)" : speakerColor(k);
      marks.append(value.slice(at, s.open[0]));
      const tag = (a, b) => el("span", { className: "tag", textContent: value.slice(a, b) });
      const inner = el("mark", { textContent: value.slice(s.open[1], s.close[0]), title: s.impression ? `${s.name} (impression)` : s.name });
      inner.style.setProperty("--c", color);
      if (s.impression) inner.classList.add("impression");
      marks.append(tag(s.open[0], s.open[1]), inner, tag(s.close[0], s.close[1]));
      at = s.close[1];
    }
    marks.append(value.slice(at) + " "); // a trailing space keeps the last character's width
  }

  /**
   * The Voice menu of line i: the speakers (giving the selected words their
   * voice, or changing the voice of the span at the caret), Impression,
   * Remove voice and New speaker for a voice.
   */
  voiceMenu(i, input, anchor) {
    this.root.querySelector(".voice-menu")?.remove();
    const start = input.selectionStart ?? 0;
    const end = input.selectionEnd ?? start;
    const inside = E.spanAt(input.value, start, end);
    if (!inside && start === end) {
      this.h.toast?.("Select the words first, then pick the voice that says them");
      input.focus();
      return;
    }
    const menu = el("div", { className: "voice-menu", role: "menu" });
    const close = () => {
      menu.remove();
      removeEventListener("pointerdown", outside, true);
    };
    const outside = (e) => {
      if (!menu.contains(/** @type {Node} */ (e.target))) close();
    };
    addEventListener("pointerdown", outside, true);
    const impression = el("input", { type: "checkbox", checked: !!inside?.impression });
    const give = (id) => {
      close();
      this.edit(E.wrapSpan(this.doc, i, start, end, id, impression.checked), true, `.line[data-i="${i}"] .text`);
    };
    E.speakers(this.doc).forEach((s, k) => {
      if (!s?.id) return;
      const b = el("button", { type: "button", role: "menuitem", className: "small", textContent: s.name || s.id });
      b.style.setProperty("--c", speakerColor(k));
      if (inside?.name === s.id) b.classList.add("chosen");
      b.addEventListener("click", () => give(s.id));
      menu.append(b);
    });
    const add = el("button", { type: "button", role: "menuitem", className: "small", textContent: "New speaker for a voice…", title: "Adds a speaker without lines (pick its voice in the Speakers table) and gives it these words" });
    add.addEventListener("click", () => {
      close();
      const doc = E.addSpeaker(this.doc, this.h.starters);
      const id = E.speakers(doc).at(-1).id;
      this.edit(E.wrapSpan(doc, i, start, end, id, impression.checked), true, `.speaker:last-child .sp-voice`);
    });
    menu.append(el("label", { className: "field-inline", title: "The line's speaker imitates the voice: its sound, halfway to its pitch" }, impression, " Impression"), add);
    if (inside) {
      const rm = el("button", { type: "button", role: "menuitem", className: "small", textContent: "Remove voice" });
      rm.addEventListener("click", () => {
        close();
        this.edit(E.unwrapSpan(this.doc, i, start), true, `.line[data-i="${i}"] .text`);
      });
      menu.append(rm);
    }
    (anchor.closest(".line-main") ?? anchor).after(menu);
    /** @type {HTMLElement | null} */ (menu.querySelector("button"))?.focus();
    menu.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        close();
        input.focus();
      }
    });
  }

  /** The Timing disclosure: fields left empty are at their default and left out of the file. */
  timing(l, i) {
    const details = el("details", { className: "timing" });
    const set = Object.keys(l || {}).filter((k) => ["delay", "speed", "instant", "advance", "hold"].includes(k));
    details.open = false;
    details.append(el("summary", { textContent: set.length ? `Timing (${set.join(", ")})` : "Timing" }));
    const num = (key, label, ph, min, max, step) => {
      const input = el("input", { type: "number", min, max, step, placeholder: ph, value: l?.[key] === undefined ? "" : String(l[key]) });
      input.addEventListener("input", () => this.edit(E.setLineField(this.doc, i, key, input.value === "" ? undefined : Number(input.value)), false));
      return el("label", { className: "field-inline", title: `${label}; empty uses ${ph}` }, `${label} `, input);
    };
    const instant = el("input", { type: "checkbox", checked: !!l?.instant });
    instant.addEventListener("change", () => this.edit(E.setLineField(this.doc, i, "instant", instant.checked), false));
    const advance = el("select", {}, new Option("advance: default", ""), new Option("input (wait for a click)", "input"), new Option("auto (go on by itself)", "auto"));
    advance.value = l?.advance ?? "";
    advance.addEventListener("change", () => this.edit(E.setLineField(this.doc, i, "advance", advance.value === "" ? undefined : advance.value), false));
    details.append(
      el("div", { className: "timing-fields" },
        num("delay", "delay (s)", "0", "0", "10", "0.1"),
        num("speed", "speed", "1", "0.25", "4", "0.05"),
        el("label", { className: "field-inline", title: "Show all text at once and play only the motif" }, instant, " instant"),
        advance,
        num("hold", "hold (s)", "dialogue's", "0", "10", "0.1"),
      ),
    );
    return details;
  }

  /** Puts each problem at its row; one not at a row is listed on top. */
  showProblems() {
    for (const e of this.root.querySelectorAll(".bad")) e.classList.remove("bad");
    for (const e of this.root.querySelectorAll(".row-problem")) e.textContent = "";
    for (const p of this.problems) {
      const m = /^(speakers|lines)\[(\d+)\]/.exec(p.path);
      const row = m && this.root.querySelector(`[data-path="${m[1]}[${m[2]}]"]`);
      if (!row) continue;
      row.classList.add("bad");
      const slot = row.querySelector(".row-problem");
      if (slot) slot.textContent += `${slot.textContent ? " " : ""}${p.path.replace(/^[a-z]+\[\d+\]\.?/, "") || "row"}: ${p.message}`;
    }
  }
}
