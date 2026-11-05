import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { format, get, set, remove } from "../format.js";

const presets = new URL("../../../presets/", import.meta.url);
const starters = readdirSync(presets).filter((f) => f.endsWith(".voice.json"));

test("formatting a starter voice reproduces its file", () => {
  assert.equal(starters.length, 10);
  for (const f of starters) {
    const text = readFileSync(new URL(f, presets), "utf8");
    assert.equal(format(JSON.parse(text)), text, f);
  }
});

test("set adds keys in the guide's order and keeps existing places", () => {
  let d = { version: 0, name: "x", speed: 3, instruments: { vowel: { reverb: 0.1, wave: "sine" } } };
  d = set(d, "pitch", 2);
  assert.deepEqual(Object.keys(d), ["version", "name", "pitch", "speed", "instruments"]);
  d = set(d, "breath", 1);
  assert.deepEqual(Object.keys(d), ["version", "name", "pitch", "speed", "breath", "instruments"]);
  d = set(d, "instruments.vowel.attackMs", 5);
  assert.deepEqual(Object.keys(d.instruments.vowel), ["reverb", "wave", "attackMs"]);
  d = set(d, "instruments.vowel.pulseWidth", 0.3);
  assert.deepEqual(Object.keys(d.instruments.vowel), ["reverb", "wave", "pulseWidth", "attackMs"]);
  d = set(d, "instruments.motif.gain", 0.5);
  assert.deepEqual(Object.keys(d.instruments), ["vowel", "motif"]);
  d = set(d, "instruments.sonorant", {});
  assert.deepEqual(Object.keys(d.instruments), ["vowel", "sonorant", "motif"]);
  d = set(d, "rhythm.rest.end", "bar");
  d = set(d, "rhythm.bpm", 90);
  assert.deepEqual(Object.keys(d.rhythm), ["bpm", "rest"]);
  d = set(d, "speed", 4);
  assert.equal(get(d, "speed"), 4);
  assert.deepEqual(Object.keys(d).slice(0, 4), ["version", "name", "pitch", "speed"]);
});

test("set does not change its input", () => {
  const d = { instruments: { vowel: {} } };
  set(d, "instruments.vowel.gain", 2);
  assert.deepEqual(d, { instruments: { vowel: {} } });
});

test("remove drops a key and empty parents, but keeps empty instruments", () => {
  let d = { version: 0, rhythm: { rest: { end: "bar" } }, instruments: { vowel: { gain: 1 }, motif: { gain: 1 } } };
  d = remove(d, "rhythm.rest.end");
  assert.equal(get(d, "rhythm"), undefined);
  d = remove(d, "instruments.motif.gain");
  assert.deepEqual(get(d, "instruments.motif"), {});
  d = remove(d, "instruments.motif");
  assert.deepEqual(Object.keys(d.instruments), ["vowel"]);
  assert.equal(remove(d, "nothing.here"), d);
});

test("get", () => {
  const d = { a: { b: 0 } };
  assert.equal(get(d, "a.b"), 0);
  assert.equal(get(d, "a.c"), undefined);
  assert.equal(get(d, "a.b.c"), undefined);
});
