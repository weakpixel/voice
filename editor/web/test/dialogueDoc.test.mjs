import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { summarize, statusText } from "../dialogueDoc.js";

const village = readFileSync(new URL("../../../ebiten/example/village.dialogue.json", import.meta.url), "utf8");

test("the village dialogue has nine lines said by named speakers", () => {
  const lines = summarize(village);
  assert.equal(lines?.length, 9);
  assert.equal(lines?.[0].who, "Wren");
  assert.equal(lines?.[6].who, "Kip");
  assert.equal(statusText(lines, 1), "Line 2 of 9: Bram");
});

test("text that is not a dialogue has no summary", () => {
  assert.equal(summarize("{"), null);
  assert.equal(summarize("[]"), null);
  assert.equal(summarize('{"lines": 3}'), null);
  assert.equal(statusText(null, 2), "Line 3");
});

test("an unknown speaker shows its id", () => {
  const lines = summarize('{"speakers":[],"lines":[{"speaker":"z","text":"x"}]}');
  assert.equal(lines?.[0].who, "z");
});
