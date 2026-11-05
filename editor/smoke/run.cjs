// Headless Chromium test of the built Voice Editor site: every starter plays
// (oto pulls samples, the typewriter reaches the whole line), problems show
// with their paths, transport buttons work, a file opens and downloads, and
// the draft survives a reload.
//
//   make editor-smoke        (builds build/editor first)
"use strict";
const path = require("node:path");
const assert = require("node:assert/strict");
const { chromium } = require("playwright");
const { serve } = require("./serve.cjs");

const root = process.argv[2] || "build/editor";
const LONG = "Hold up, traveler, hear me out. The mill is old, but the road is long, and the river sings a different song.";

async function main() {
  const server = await serve(root, 0);
  const url = `http://127.0.0.1:${server.address().port}/`;
  const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const failures = [];
  try {
    const context = await browser.newContext({ acceptDownloads: true });
    const page = await context.newPage();
    page.on("pageerror", (e) => failures.push(`page error: ${e.message}`));
    page.on("console", (m) => m.type() === "error" && failures.push(`console: ${m.text()}`));
    await page.goto(url);
    await page.waitForSelector("body[data-ready=true]", { timeout: 30000 });

    const names = await page.$$eval("#starter option", (os) => os.map((o) => o.value).filter(Boolean));
    assert.deepEqual(names, ["soft", "chirpy", "gruff", "divine", "flow", "verse"]);

    const line = "Hello there, traveler! Are you lost?";
    for (const name of names) {
      await page.selectOption("#starter", name);
      await page.waitForFunction((n) => document.getElementById("doc").value.includes(`"name": "${n}"`), name);
      await page.waitForSelector(".validity.ok");
      await page.fill("#line", line);
      await page.click("#play");
      await page.waitForFunction(() => !document.getElementById("stop").disabled, null, { timeout: 5000 });
      await page.waitForFunction(() => document.getElementById("stop").disabled, null, { timeout: 30000 });
      const shown = await page.textContent("#shown");
      assert.equal(shown, line, `${name}: typewriter`);
      const events = await page.textContent("#events");
      assert.match(events, /started.*completed/, `${name}: events`);
      await page.waitForTimeout(150);
      const peak = Number(await page.getAttribute("#meter-bar", "data-max-peak"));
      assert.ok(peak > 0.01, `${name}: peak ${peak}, oto did not pull samples`);
      console.log(`ok ${name}: plays, peak ${peak.toFixed(3)}`);
    }

    // Pause, resume and skip a long line.
    await page.selectOption("#starter", "soft");
    await page.waitForSelector(".validity.ok");
    await page.fill("#line", LONG);
    await page.click("#play");
    await page.waitForTimeout(400);
    await page.click("#pause");
    assert.equal((await page.textContent("#pause")).trim(), "Resume");
    const t1 = await page.textContent("#time");
    await page.waitForTimeout(300);
    assert.equal(await page.textContent("#time"), t1, "time moved while paused");
    await page.click("#pause");
    await page.waitForTimeout(200);
    await page.click("#skip");
    assert.equal(await page.textContent("#shown"), LONG, "skip shows the whole line");
    assert.match(await page.textContent("#events"), /skipped/);
    console.log("ok pause, resume, skip");

    // Problems with their paths.
    const soft = await page.inputValue("#doc");
    await page.fill("#doc", soft.replace('"speed": 13', '"speed": 100'));
    await page.waitForSelector(".validity.bad");
    assert.match(await page.textContent("#problems"), /speed: 100 is outside 1 to 60/);
    assert.ok(await page.isDisabled("#play"));
    await page.click("#problems button");
    const sel = await page.$eval("#doc", (t) => t.value.slice(t.selectionStart, t.selectionEnd));
    assert.equal(sel, '"speed"', "jump to the problem selects its key");
    await page.fill("#doc", "{\n  \"version\": 0,\n  oops\n}");
    await page.waitForFunction(() => document.getElementById("problems").textContent.includes("line 3"));
    console.log("ok problems");

    // Open a file, then download it.
    const guard = soft.replace('"name": "soft"', '"name": "guard"').replace('"speed": 13', '"speed": 9');
    await page.setInputFiles("#file", { name: "guard.voice.json", mimeType: "application/json", buffer: Buffer.from(guard) });
    await page.waitForSelector(".validity.ok");
    assert.equal(await page.textContent("#doc-name"), "guard.voice.json");
    const [dl] = await Promise.all([page.waitForEvent("download"), page.click("#download")]);
    assert.equal(dl.suggestedFilename(), "guard.voice.json");
    const text = await streamText(await dl.createReadStream());
    assert.equal(text, guard);
    assert.ok(await page.evaluate((t) => voiceEditor.loadVoice(t).ok, text), "downloaded file loads");
    console.log("ok open and download");

    // The draft survives a reload.
    await page.reload();
    await page.waitForSelector("body[data-ready=true]");
    assert.equal(await page.inputValue("#doc"), guard);
    assert.equal(await page.inputValue("#line"), LONG);
    console.log("ok draft");
  } catch (e) {
    failures.push(e.stack || String(e));
  } finally {
    await browser.close();
    server.close();
  }
  if (failures.length) {
    console.error(failures.join("\n"));
    process.exit(1);
  }
  console.log("editor smoke test passed");
}

function streamText(stream) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    stream.on("data", (c) => chunks.push(c));
    stream.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    stream.on("error", reject);
  });
}

main();
