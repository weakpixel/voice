// The Visual tab: a form generated from fields.json that edits the voice
// document. A field the document leaves out shows the library's default
// (dimmed) and has no reset button; changing it writes the key, reset
// removes it. Values are checked by the library, not here: problems come
// back from the page and are shown at their field.

import { get, set, remove } from "./format.js";

/**
 * @typedef {{key: string, label: string, kind: string, min?: number, max?: number,
 *   uiMin?: number, uiMax?: number, step?: number, default?: any, unit?: string,
 *   options?: string, zeroIsDefault?: boolean, group?: string, help?: string,
 *   sample?: boolean, maxItems?: number}} Field
 * @typedef {{options: Record<string, any[]>, voice: Field[], instrument: Field[],
 *   percussion: Field[], rhythm: Field[]}} Fields
 * @typedef {{path: string, message: string}} Problem
 * @typedef {(doc: any, how: {commit: boolean, structural?: boolean}) => void} OnChange
 */

const MAX_PERCUSSION = 8;

/** Creates an element with attributes and children. */
function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k === "dataset") Object.assign(e.dataset, v);
    else if (k.startsWith("on")) e.addEventListener(k.slice(2), v);
    else if (v === true) e.setAttribute(k, "");
    else e.setAttribute(k, String(v));
  }
  e.append(...children.filter((c) => c !== null && c !== undefined && c !== false));
  return e;
}

export class VisualEditor {
  /**
   * @param {HTMLElement} root
   * @param {Fields} fields
   * @param {Record<string, Record<string, any>>} percussionDefaults built-in sounds as entry fields
   * @param {OnChange} onChange
   */
  constructor(root, fields, percussionDefaults, onChange) {
    this.root = root;
    this.fields = fields;
    this.percDefaults = percussionDefaults;
    this.onChange = onChange;
    /** @type {any} */
    this.doc = {};
    this.instrument = "vowel"; // the instrument tab shown
    /** @type {Problem[]} */
    this.problems = [];
  }

  /** Shows doc (after a change outside the form, or a structural one). */
  render(doc) {
    this.doc = doc;
    const scroll = this.root.scrollTop;
    const focused = document.activeElement?.id;
    this.root.replaceChildren(
      this.section("Voice", "voice", this.groups(this.fields.voice, "")),
      this.instrumentsSection(),
      this.rhythmSection(),
      this.percussionSection(),
    );
    this.root.scrollTop = scroll;
    if (focused) document.getElementById(focused)?.focus();
    this.showProblems(this.problems);
  }

  /** Marks the fields that have problems; the rest are listed by the page. */
  showProblems(problems) {
    this.problems = problems;
    for (const row of this.root.querySelectorAll(".row")) {
      row.classList.remove("bad");
      row.querySelector(".row-problem")?.remove();
    }
    for (const p of problems) {
      if (!p.path) continue;
      const row = this.root.querySelector(`.row[data-path="${CSS.escape(p.path)}"]`);
      if (!row) continue;
      row.classList.add("bad");
      row.append(el("div", { class: "row-problem" }, p.message));
    }
  }

  // ---- edits ----

  /** A value changed; commit is false while a slider moves or a text is typed. */
  change(path, value, commit) {
    this.doc = value === undefined ? remove(this.doc, path) : set(this.doc, path, value);
    if (commit) this.render(this.doc); // defaults, reset buttons and dependent rows follow
    this.onChange(this.doc, { commit });
  }

  structural(doc) {
    this.render(doc);
    this.onChange(doc, { commit: true, structural: true });
  }

  // ---- sections ----

  section(title, id, ...content) {
    return el("section", { class: "v-section", dataset: { section: id } }, el("h3", {}, title), ...content);
  }

  /** Rows of fields grouped by their group, under prefix (a path ending in "." or ""). */
  groups(fields, prefix, defaults = {}) {
    const out = [];
    let group = null;
    let box = null;
    for (const f of fields) {
      if (f.group !== group) {
        group = f.group;
        box = el("div", { class: "group" }, el("h4", {}, group || ""));
        out.push(box);
      }
      box.append(this.row(f, prefix + f.key, defaults[f.key]));
    }
    return el("div", { class: "groups" }, ...out);
  }

  instrumentsSection() {
    const names = this.fields.options.instruments;
    const has = (n) => get(this.doc, `instruments.${n}`) !== undefined;
    const tabs = el(
      "div",
      { class: "tabs", role: "tablist" },
      ...names.map((n) =>
        el(
          "button",
          {
            type: "button",
            role: "tab",
            class: `tab${n === this.instrument ? " active" : ""}${has(n) || n === "vowel" ? "" : " inherits"}`,
            "aria-selected": n === this.instrument ? "true" : "false",
            dataset: { instrument: n },
            title: has(n) || n === "vowel" ? "" : "Uses the vowel's sound",
            onclick: () => {
              this.instrument = n;
              this.render(this.doc);
            },
          },
          n,
        ),
      ),
    );
    const n = this.instrument;
    const path = `instruments.${n}`;
    let body;
    if (n !== "vowel" && !has(n)) {
      body = el(
        "div",
        { class: "inherits-note" },
        el(
          "label",
          { class: "check" },
          el("input", {
            type: "checkbox",
            dataset: { own: n },
            onchange: () => this.structural(set(this.doc, path, structuredClone(get(this.doc, "instruments.vowel") || {}))),
          }),
          ` Give the ${n} its own sound (it uses the vowel's now)`,
        ),
      );
    } else {
      const own =
        n === "vowel"
          ? null
          : el(
              "label",
              { class: "check" },
              el("input", { type: "checkbox", checked: true, dataset: { own: n }, onchange: () => this.structural(remove(this.doc, path)) }),
              " Own sound (clear to use the vowel's)",
            );
      body = el("div", {}, own, this.instrumentFields(path + ".", {}));
    }
    return this.section("Instruments", "instruments", tabs, body);
  }

  /** The instrument fields; the sample ones fold away unless used. */
  instrumentFields(prefix, defaults) {
    const osc = this.fields.instrument.filter((f) => !f.sample);
    const sample = this.fields.instrument.filter((f) => f.sample);
    const usesSample = sample.some((f) => get(this.doc, prefix + f.key) !== undefined);
    return el(
      "div",
      {},
      this.groups(osc, prefix, defaults),
      el("details", { class: "sample", open: usesSample }, el("summary", {}, "Sample (not playable in the editor yet)"), this.groups(sample, prefix, defaults)),
    );
  }

  rhythmSection() {
    const on = get(this.doc, "rhythm") !== undefined;
    const toggle = el(
      "label",
      { class: "check" },
      el("input", {
        type: "checkbox",
        checked: on,
        dataset: { rhythm: "" },
        onchange: (e) => this.structural(e.target.checked ? set(this.doc, "rhythm", {}) : remove(this.doc, "rhythm")),
      }),
      " Speak in rhythm, on a beat grid",
    );
    if (!on) return this.section("Rhythm", "rhythm", toggle);
    return this.section("Rhythm", "rhythm", toggle, this.groups(this.fields.rhythm, "rhythm."), this.beat());
  }

  /** One pattern per percussion sound of the beat, and a menu to add one. */
  beat() {
    const beat = get(this.doc, "rhythm.beat") || {};
    const division = Number(get(this.doc, "rhythm.division") ?? 4);
    const steps = 4 * division;
    const defined = Object.keys(get(this.doc, "percussion") || {});
    const sounds = [...new Set([...defined, ...this.fields.options.percussion])].filter((s) => !(s in beat));
    const rows = Object.entries(beat).map(([sound, pattern]) => {
      const path = `rhythm.beat.${sound}`;
      return el(
        "div",
        { class: "row", dataset: { path } },
        el("span", { class: "label" }, sound),
        el("input", {
          type: "text",
          class: "pattern",
          value: pattern,
          maxlength: 32,
          spellcheck: "false",
          title: `${steps} steps: x hit, - ghost hit, . rest`,
          oninput: (e) => this.change(path, e.target.value, false),
          onchange: (e) => this.change(path, e.target.value, true),
        }),
        el("button", { type: "button", class: "small", title: "Remove from the beat", onclick: () => this.structural(remove(this.doc, path)) }, "Remove"),
      );
    });
    const add = el(
      "select",
      {
        class: "add",
        "aria-label": "Add a sound to the beat",
        onchange: (e) => {
          const s = e.target.value;
          if (s) this.structural(set(this.doc, `rhythm.beat.${s}`, "x" + ".".repeat(steps - 1)));
        },
      },
      el("option", { value: "" }, "Add a sound to the beat…"),
      ...sounds.map((s) => el("option", { value: s }, s)),
    );
    return el("div", { class: "group beat" }, el("h4", {}, "Beat patterns"), ...rows, add);
  }

  percussionSection() {
    const entries = get(this.doc, "percussion") || {};
    const names = Object.keys(entries);
    const cards = names.map((name) => {
      const prefix = `percussion.${name}.`;
      const base = get(this.doc, prefix + "base") || (name in this.percDefaults ? name : "");
      const defaults = base ? { ...this.percDefaults[base], base: name in this.percDefaults ? name : "" } : {};
      return el(
        "details",
        { class: "card", open: names.length === 1 },
        el(
          "summary",
          {},
          el("strong", {}, name),
          base ? ` (from ${base})` : "",
          el("button", { type: "button", class: "small", title: "Remove the sound", onclick: () => this.structural(remove(this.doc, `percussion.${name}`)) }, "Remove"),
        ),
        this.groups(this.fields.percussion, prefix, defaults),
        this.instrumentFields(prefix, defaults),
      );
    });
    const full = names.length >= MAX_PERCUSSION;
    const add = el(
      "select",
      {
        class: "add",
        disabled: full,
        "aria-label": "Add a percussion sound",
        onchange: (e) => {
          const s = e.target.value;
          if (s) this.structural(set(this.doc, `percussion.${s}`, {}));
        },
      },
      el("option", { value: "" }, full ? `At most ${MAX_PERCUSSION} sounds` : "Add a built-in sound…"),
      ...this.fields.options.percussion.filter((s) => !(s in entries)).map((s) => el("option", { value: s }, s)),
    );
    const custom = el("input", { type: "text", placeholder: "or a new name", class: "custom", disabled: full, "aria-label": "Name of a new sound" });
    const addCustom = el(
      "button",
      {
        type: "button",
        class: "small",
        disabled: full,
        onclick: () => {
          const s = custom.value.trim();
          if (s && !(s in entries)) this.structural(set(this.doc, `percussion.${s}`, { base: "kick" }));
        },
      },
      "Add",
    );
    return this.section(
      "Percussion",
      "percussion",
      el("p", { class: "hint" }, "The sounds a rhythm's beat plays. Built-in sounds work in a beat without an entry; add one here to change it."),
      ...cards,
      el("div", { class: "add-row" }, add, custom, addCustom),
    );
  }

  // ---- one field ----

  /** A row with the field's label, its control and a reset button. */
  row(f, path, inherited) {
    const raw = get(this.doc, path);
    const present = raw !== undefined;
    const fallback = inherited !== undefined ? inherited : f.default;
    const value = !present || (f.zeroIsDefault && raw === 0) ? fallback : raw;
    const reset = el(
      "button",
      { type: "button", class: "reset", title: "Reset to the default (remove the field)", hidden: !present, onclick: () => this.structural(remove(this.doc, path)) },
      "↺",
    );
    const label = el("label", { class: "label", title: f.help || "", for: `f-${path}` }, f.label);
    return el(
      "div",
      { class: `row${present ? "" : " default"}`, dataset: { path } },
      label,
      this.control(f, path, value),
      f.unit ? el("span", { class: "unit" }, f.unit) : el("span"),
      reset,
    );
  }

  control(f, path, value) {
    const id = `f-${path}`;
    const commitChange = (v) => this.change(path, v, true);
    switch (f.kind) {
      case "number":
      case "int": {
        const step = f.step ?? (f.kind === "int" ? 1 : 0.01);
        const lo = f.uiMin ?? f.min ?? 0;
        const hi = f.uiMax ?? f.max;
        const num = el("input", {
          id,
          type: "number",
          class: "num",
          min: f.min,
          max: f.max,
          step,
          value: value ?? "",
          onchange: (e) => {
            const v = e.target.value === "" ? undefined : Number(e.target.value);
            if (slider) slider.value = String(v ?? f.default ?? 0);
            commitChange(v);
          },
        });
        if (hi === undefined) return el("div", { class: "ctl" }, num);
        const slider = el("input", {
          type: "range",
          min: lo,
          max: hi,
          step,
          value: value ?? lo,
          "aria-label": f.label,
          oninput: (e) => {
            num.value = e.target.value;
            this.change(path, Number(e.target.value), false);
          },
          onchange: (e) => commitChange(Number(e.target.value)),
        });
        return el("div", { class: "ctl" }, slider, num);
      }
      case "select": {
        const options = this.fields.options[f.options] || [];
        const blank = f.key === "base" ? [el("option", { value: "" }, "(none)")] : [];
        return el(
          "div",
          { class: "ctl" },
          el(
            "select",
            {
              id,
              onchange: (e) => {
                const raw = e.target.value;
                const v = raw === "" ? undefined : typeof options[0] === "number" ? Number(raw) : raw;
                commitChange(v);
              },
            },
            ...blank,
            ...options.map((o) => el("option", { value: o, selected: String(o) === String(value ?? "") }, String(o))),
          ),
        );
      }
      case "bool":
        return el("div", { class: "ctl" }, el("input", { id, type: "checkbox", checked: Boolean(value), onchange: (e) => commitChange(e.target.checked || undefined) }));
      case "list":
        return el(
          "div",
          { class: "ctl" },
          el("input", {
            id,
            type: "text",
            value: Array.isArray(value) ? value.join(", ") : "",
            placeholder: "0",
            onchange: (e) => {
              const text = e.target.value.trim();
              if (text === "") return commitChange(undefined);
              const list = text.split(/[\s,]+/).map(Number);
              e.target.classList.toggle("invalid", list.some(Number.isNaN));
              if (!list.some(Number.isNaN)) commitChange(list);
            },
          }),
        );
      default:
        return el(
          "div",
          { class: "ctl" },
          el("input", {
            id,
            type: "text",
            value: value ?? "",
            oninput: (e) => this.change(path, e.target.value === "" ? undefined : e.target.value, false),
            onchange: (e) => commitChange(e.target.value === "" ? undefined : e.target.value),
          }),
        );
    }
  }
}
