import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { formatDialogue } from "../format.js";
import * as E from "../dialogueEdit.js";

const village = JSON.parse(readFileSync(new URL("../../../ebiten/example/village.dialogue.json", import.meta.url), "utf8"));
const STARTERS = ["soft", "chirpy", "gruff"];

test("edits never change their input", () => {
  const before = JSON.stringify(village);
  E.addLine(village, 0, "wren");
  E.moveLine(village, 1, 1);
  E.removeLine(village, 0);
  E.setLineField(village, 0, "delay", 2);
  E.setSpeakerField(village, 0, "id", "x");
  E.addSpeaker(village, STARTERS);
  assert.equal(JSON.stringify(village), before);
});

test("speakers: add takes an unused starter and a unique id; rename updates the lines", () => {
  let d = E.addSpeaker({ version: 0, speakers: [{ id: "soft", voice: "soft" }], lines: [] }, STARTERS);
  assert.deepEqual(d.speakers[1], { id: "chirpy", name: "Chirpy", voice: "chirpy" });
  d = E.addSpeaker(d, STARTERS);
  assert.equal(d.speakers[2].voice, "gruff");
  d = E.addSpeaker(d, STARTERS); // all starters used: the first again, with a new id
  assert.deepEqual([d.speakers[3].voice, d.speakers[3].id], ["soft", "soft-2"]);

  const r = E.setSpeakerField(village, 0, "id", "ann");
  assert.equal(r.speakers[0].id, "ann");
  assert.equal(r.lines[0].speaker, "ann");
  assert.equal(r.lines[1].speaker, "bram");
  assert.equal(E.setSpeakerField(village, 0, "pan", undefined).speakers[0].pan, undefined);
  assert.equal(E.setSpeakerField(village, 0, "name", "").speakers[0].name, undefined);
});

test("a speaker with lines is in use", () => {
  assert.ok(E.speakerUsed(village, "wren"));
  assert.ok(!E.speakerUsed(E.addSpeaker(village, STARTERS), "chirpy-2"));
  assert.equal(E.removeSpeaker(village, 2).speakers.length, 2);
});

test("lines: add after, take turns, move, remove", () => {
  assert.equal(E.nextSpeaker(village, 0), "bram");
  assert.equal(E.nextSpeaker(village, 6), "wren"); // after the last speaker comes the first
  let d = E.addLine(village, 0, "bram");
  assert.deepEqual(d.lines[1], { speaker: "bram", text: "" });
  assert.equal(d.lines.length, 10);
  assert.equal(E.addLine(village, -1, "wren").lines[0].text, "");
  d = E.moveLine(village, 0, 1);
  assert.deepEqual([d.lines[0].speaker, d.lines[1].speaker], ["bram", "wren"]);
  assert.strictEqual(E.moveLine(village, 0, -1), village);
  assert.strictEqual(E.moveLine(village, 8, 1), village);
  assert.equal(E.removeLine(village, 0).lines.length, 8);
});

test("a line field at its default is left out of the file", () => {
  let d = E.setLineField(village, 1, "delay", 0.4);
  assert.equal(d.lines[1].delay, 0.4);
  assert.deepEqual(Object.keys(d.lines[1]), ["speaker", "text", "delay"]);
  d = E.setLineField(d, 1, "delay", 0);
  assert.equal("delay" in d.lines[1], false);
  d = E.setLineField(d, 1, "speed", 1);
  assert.equal("speed" in d.lines[1], false);
  d = E.setLineField(d, 1, "speed", 0.9);
  d = E.setLineField(d, 1, "instant", true);
  d = E.setLineField(d, 1, "advance", "auto");
  d = E.setLineField(d, 1, "hold", 0.5);
  assert.deepEqual(d.lines[1], { speaker: "bram", text: village.lines[1].text, speed: 0.9, instant: true, advance: "auto", hold: 0.5 });
  d = E.setLineField(d, 1, "advance", undefined);
  assert.equal("advance" in d.lines[1], false);
  assert.equal(E.setLineField(village, 0, "text", "").lines[0].text, "", "an empty text stays (the loader reports it)");
});

test("edited documents format back in the example's style", () => {
  const d = E.setLineField(E.addLine(village, 0, "bram"), 1, "text", "Hi.");
  const text = formatDialogue(d);
  assert.ok(text.includes('{ "speaker": "bram", "text": "Hi." }'));
  assert.deepEqual(JSON.parse(text), d);
});
