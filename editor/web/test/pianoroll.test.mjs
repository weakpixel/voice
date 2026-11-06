import { test } from "node:test";
import assert from "node:assert/strict";
import { layout } from "../pianoroll.js";

test("layout spans the melodic notes and the longer of line and render", () => {
  const tl = {
    duration: 2,
    reveals: [],
    textLen: 3,
    notes: [
      { on: 0, off: 0.1, instrument: "vowel", inst: 0, note: 60, velocity: 1 },
      { on: 0.2, off: 0.3, instrument: "motif", inst: 4, note: 40, velocity: 1 },
      { on: 0.2, off: 0.3, instrument: "kick", inst: 5, note: 10, velocity: 1 },
    ],
  };
  assert.deepEqual(layout(tl, 3), { lo: 39, hi: 61, end: 3, percussion: true });
  assert.deepEqual(layout(tl, 1).end, 2);
  const one = layout({ ...tl, notes: [tl.notes[0]] }, 0);
  assert.equal(one.hi - one.lo, 12, "a narrow line still shows an octave");
  assert.equal(one.percussion, false);
  assert.equal(layout(null, 0).end, 0.5);
});
