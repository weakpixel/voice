import { test } from "node:test";
import assert from "node:assert/strict";
import { utf8, revealedText, locatePath, downloadName, docName, looksLikeVoice } from "../text.js";

test("revealedText decodes a byte prefix", () => {
  const b = utf8("Héllo… there");
  assert.equal(revealedText(b, 0), "");
  assert.equal(revealedText(b, 3), "Hé");
  assert.equal(revealedText(b, 9), "Héllo…");
  assert.equal(revealedText(b, 999), "Héllo… there");
});

test("locatePath finds nested keys", () => {
  const doc = `{
  "version": 0,
  "speed": 3,
  "instruments": {
    "vowel": {"wave": "sine", "sustain": 2},
    "motif": {"sustain": 1}
  },
  "rhythm": {"beat": {"kick": "x..."}}
}`;
  const at = (p) => {
    const r = locatePath(doc, p);
    return r && doc.slice(r.start, r.end);
  };
  assert.equal(at("speed"), '"speed"');
  const r = locatePath(doc, "instruments.motif.sustain");
  assert.ok(r.start > doc.indexOf('"motif"'));
  assert.equal(at("instruments.vowel.sustain"), '"sustain"');
  assert.ok(locatePath(doc, "instruments.vowel.sustain").start < doc.indexOf('"motif"'));
  assert.equal(at("rhythm.beat.kick"), '"kick"');
  assert.equal(at("phonemes[0].symbol"), null);
  assert.equal(at(""), null);
});

test("downloadName", () => {
  assert.equal(downloadName('{"name": "Old Guard"}'), "Old-Guard.voice.json");
  assert.equal(downloadName("{"), "voice.voice.json");
  assert.equal(downloadName('{"name": "x"}', "mine.voice.json"), "mine.voice.json");
  assert.equal(docName('{"name": 3}'), "");
});

test("looksLikeVoice", () => {
  assert.ok(looksLikeVoice('{"version": 0, "instruments": {}}'));
  assert.ok(!looksLikeVoice("hello"));
  assert.ok(!looksLikeVoice("[1]"));
});
