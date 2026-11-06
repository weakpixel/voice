import { test } from "node:test";
import assert from "node:assert/strict";
import { nextTheme } from "../theme.js";

test("the Theme button cycles system, light, dark", () => {
  assert.equal(nextTheme("system"), "light");
  assert.equal(nextTheme("light"), "dark");
  assert.equal(nextTheme("dark"), "system");
  assert.equal(nextTheme("nonsense"), "system");
});
