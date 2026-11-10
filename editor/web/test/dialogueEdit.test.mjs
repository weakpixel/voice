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

test("voice spans: parse, shown text, wrap, retag, unwrap", () => {
  const t = "Grandpa said {bram}keep dry{/}, and {{x} {ghost~}hi{/ghost}.";
  const sp = E.spansOf(t);
  assert.deepEqual(sp.map((s) => [s.name, s.impression, t.slice(s.open[1], s.close[0])]), [["bram", false, "keep dry"], ["ghost", true, "hi"]]);
  assert.equal(E.shownText(t), "Grandpa said keep dry, and {x} hi.");
  assert.deepEqual(E.spansOf("{a}{/} {b}x {/c} {Bad}y{/}"), []);

  const doc = { version: 0, speakers: [{ id: "wren" }, { id: "bram" }], lines: [{ speaker: "wren", text: "I said keep dry now" }] };
  const at = (s) => doc.lines[0].text.indexOf(s);
  let d = E.wrapSpan(doc, 0, at("keep") - 1, at("dry") + 4, "bram"); // spaces at the ends are left out
  assert.equal(d.lines[0].text, "I said {bram}keep dry{/} now");
  assert.equal(E.wrapSpan(d, 0, 9, 12, "wren", true).lines[0].text, "I said {wren~}keep dry{/} now"); // inside: retag
  assert.equal(E.wrapSpan(d, 0, 2, 10, "wren"), d); // crosses a tag: nothing
  assert.equal(E.wrapSpan(d, 0, 3, 3, "wren"), d); // empty
  assert.equal(E.unwrapSpan(d, 0, 15).lines[0].text, "I said keep dry now");
  assert.equal(E.unwrapSpan(d, 0, 2), d);
  assert.equal(doc.lines[0].text, "I said keep dry now"); // inputs unchanged
});

test("voice spans: a speaker in a span is in use; renaming renames the tags", () => {
  const doc = { version: 0, speakers: [{ id: "wren" }, { id: "bram" }], lines: [{ speaker: "wren", text: "a {bram}b{/} {bram~}c{/bram}" }] };
  assert.ok(E.speakerUsed(doc, "bram"));
  assert.ok(!E.speakerUsed(doc, "kip"));
  const r = E.setSpeakerField(doc, 1, "id", "old");
  assert.equal(r.lines[0].text, "a {old}b{/} {old~}c{/old}");
  assert.equal(r.lines[0].speaker, "wren");
});
