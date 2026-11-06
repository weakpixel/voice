import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { encodeShare, decodeShare } from "../share.js";

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
