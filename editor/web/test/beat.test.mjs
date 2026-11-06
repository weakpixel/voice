import { test } from "node:test";
import assert from "node:assert/strict";
import { nextStep, toggleStep, nextAccent, defaultAccents, resample, resamplePattern } from "../beat.js";

test("a click cycles rest, hit, ghost", () => {
  assert.equal(nextStep("."), "x");
  assert.equal(nextStep("x"), "-");
  assert.equal(nextStep("-"), ".");
  assert.equal(toggleStep("x...", 2), "x.x.");
  assert.equal(toggleStep("x...", 0), "-...");
});

test("accents cycle none, medium, strong", () => {
  assert.equal(nextAccent(0), 0.6);
  assert.equal(nextAccent(0.6), 1);
  assert.equal(nextAccent(1), 0);
  assert.equal(nextAccent(0.3), 0.6);
});

test("default accents match the guide", () => {
  assert.deepEqual(defaultAccents(2), [1, 0, 0.6, 0, 1, 0, 0.6, 0]);
  assert.equal(defaultAccents(4).length, 16);
});

test("resampling keeps the steps on the new grid", () => {
  assert.equal(resamplePattern("x.x.x.x.x.x.x.x.", 8), "xxxxxxxx");
  assert.equal(resamplePattern("x-..x...x...x...", 12), "x..x..x..x..");
  assert.equal(resamplePattern("x.x.x.x.", 16), "x...x...x...x...");
  assert.deepEqual(resample([1, 0, 0.6, 0, 1, 0, 0.6, 0], 12, 0), [1, 0, 0, 0.6, 0, 0, 1, 0, 0, 0.6, 0, 0]);
  assert.equal(resamplePattern("", 4), "....");
});
