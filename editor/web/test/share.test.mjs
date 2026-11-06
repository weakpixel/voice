import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { encodeShare, decodeShare, encodeBundle, decodeBundle } from "../share.js";

test("a share fragment carries the document unchanged and short", async () => {
  const text = readFileSync(new URL("../../../presets/divine.voice.json", import.meta.url), "utf8");
  const hash = await encodeShare(text);
  assert.match(hash, /^#v=[A-Za-z0-9_-]+$/);
  assert.ok(hash.length < text.length, `${hash.length} chars for ${text.length}`);
  assert.equal(await decodeShare(hash), text);
  assert.equal(await decodeShare(await encodeShare("ünïcödé ✓")), "ünïcödé ✓");
});

test("other fragments are not shares", async () => {
  assert.equal(await decodeShare(""), null);
  assert.equal(await decodeShare("#top"), null);
  assert.equal(await decodeShare("#v=@@@"), null);
});

test("a dialogue link carries the dialogue and its voice files", async () => {
  const dialogue = readFileSync(new URL("../../../ebiten/example/village.dialogue.json", import.meta.url), "utf8");
  const voice = readFileSync(new URL("../../../presets/gruff.voice.json", import.meta.url), "utf8");
  const hash = await encodeBundle({ dialogue, voices: { "bram.voice.json": voice } });
  assert.match(hash, /^#d=[A-Za-z0-9_-]+$/);
  assert.deepEqual(await decodeBundle(hash), { dialogue, voices: { "bram.voice.json": voice } });
  assert.equal(await decodeShare(hash), null, "a voice link reader ignores it");
  assert.equal(await decodeBundle(await encodeShare("{}")), null, "and the other way round");
  assert.equal(await decodeBundle("#d=@@@"), null);
  assert.equal(await decodeBundle(await encodeShare("x").then((h) => h.replace("#v=", "#d="))), null, "not a bundle");
});
