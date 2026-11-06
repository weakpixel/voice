// Headless Chromium test of the built Voice Editor site: every starter plays
// (oto pulls samples, the typewriter reaches the whole line), the visual
// editor edits the document (sliders, dropdowns, checkboxes, undo, replay),
// problems show with their paths, transport buttons work, a file opens and
// downloads, and the draft survives a reload.
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
    assert.deepEqual(names, ["soft", "chirpy", "gruff", "divine", "flow", "verse", "warrior", "robot", "ghost", "elder"]);

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

    await visualEditor(page);

    // Problems with their paths.
    await page.selectOption("#starter", "soft");
    await page.waitForSelector(".validity.ok");
    await page.click("#tab-source");
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

    await soundSurvives(page);
    console.log("ok sound survives edits");

    await rollAuditionWAV(page);
    console.log("ok piano roll, waveform, audition, WAV");

    await beatGrid(page);
    console.log("ok beat grid, accents, mute beat");
    await samples(page);
    console.log("ok sample files");
    await language(page);
    console.log("ok language");
    await shareLink(page, context);
    console.log("ok share link");
    await dialogueMode(page);
    console.log("ok dialogue mode");
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

/**
 * The Dialogue mode: the game loads in its iframe, plays the example, plays a
 * short dialogue hands-free to its end (line messages in order), takes edits,
 * shows problems at their paths, and starts its sound on a click inside it.
 */
async function dialogueMode(page) {
  await page.evaluate(() => {
    window.__game = [];
    addEventListener("message", (e) => e.data && e.data.source === "voice-game" && window.__game.push(e.data));
  });
  await page.click("#mode-dialogue");
  assert.equal(await page.getAttribute("body", "data-mode"), "dialogue");
  assert.ok(!(await page.isVisible("#doc")), "the voice editor is hidden");
  await page.waitForFunction(() => document.getElementById("dlg-validity").textContent === "Valid", null, { timeout: 60000 });
  await page.waitForFunction(() => document.getElementById("dlg-status").textContent.startsWith("Line 1 of 9: Wren"), null, { timeout: 10000 });
  assert.match(await page.inputValue("#dlg-doc"), /"name": "village"/);

  // A short dialogue plays hands-free to its end, the lines in order.
  const short = JSON.stringify({
    version: 0, hold: 0.2,
    speakers: [{ id: "a", name: "Ann", voice: "soft" }, { id: "b", name: "Bo", voice: "chirpy" }],
    lines: [{ speaker: "a", text: "Hi." }, { speaker: "b", text: "Hello!" }, { speaker: "a", text: "Bye." }],
  }, null, 2);
  await page.check("#dlg-auto");
  await page.evaluate(() => (window.__game.length = 0));
  await page.fill("#dlg-doc", short);
  await page.waitForFunction(() => window.__game.some((m) => m.type === "finished"), null, { timeout: 60000 });
  const lines = await page.evaluate(() => window.__game.filter((m) => m.type === "line").map((m) => m.index));
  assert.deepEqual(lines.slice(-3), [0, 1, 2], "line messages in order");
  assert.match(await page.textContent("#dlg-status"), /finished/);

  // An edit reaches the game: the new text's problems show with their paths.
  await page.fill("#dlg-doc", short.replace('"speaker": "b"', '"speaker": "zed"'));
  await page.waitForFunction(() => document.getElementById("dlg-problems").textContent.includes("lines[1].speaker"), null, { timeout: 10000 });
  assert.equal(await page.textContent("#dlg-validity"), "1 problem");
  // A voice file the page does not have is a problem at its speaker.
  await page.fill("#dlg-doc", short.replace('"voice": "soft"', '"voice": "bram.voice.json"'));
  await page.waitForFunction(() => document.getElementById("dlg-problems").textContent.includes("speakers[0].voice"), null, { timeout: 10000 });
  // A fixed document is valid again and the game plays it.
  await page.fill("#dlg-doc", short);
  await page.waitForFunction(() => document.getElementById("dlg-validity").textContent === "Valid", null, { timeout: 10000 });

  // Pause and resume, play from a line.
  await page.uncheck("#dlg-auto");
  await page.click("#dlg-pause");
  assert.equal((await page.textContent("#dlg-pause")).trim(), "Resume");
  await page.click("#dlg-pause");
  await page.fill("#dlg-from", "2");
  await page.click("#dlg-from-play");
  await page.waitForFunction(() => document.getElementById("dlg-status").textContent.startsWith("Line 2 of 3: Bo"), null, { timeout: 10000 });

  // Sound starts with the first click inside the game (not before).
  await page.frameLocator("#dlg-game").locator("body").click({ position: { x: 100, y: 100 } });
  await page.waitForFunction(() => document.getElementById("dlg-audio").textContent === "Sound is on", null, { timeout: 10000 });
  assert.ok(await page.evaluate(() => window.__game.some((m) => m.type === "audio" && m.running)));

  // The mode and the document survive a reload; back to the voice editor.
  await page.reload();
  await page.waitForSelector("body[data-ready=true]");
  assert.equal(await page.getAttribute("body", "data-mode"), "dialogue");
  assert.equal(await page.inputValue("#dlg-doc"), short);
  await page.click("#mode-voice");
  assert.equal(await page.getAttribute("body", "data-mode"), "voice");
  assert.ok(await page.isVisible("#voice-mode"), "the voice editor is back");
}

/** How many notes the piano roll shows. */
async function rollNotes(page) {
  return Number(await page.getAttribute("#roll", "data-notes"));
}

/** The beat grid edits the patterns and accents; mute beat leaves the drums out of the line. */
async function beatGrid(page) {
  const json = async () => JSON.parse(await page.inputValue("#doc"));
  await page.selectOption("#starter", "flow");
  await page.waitForSelector(".validity.ok");
  await page.click("#tab-visual");
  const before = (await json()).rhythm.beat.kick;
  await page.click('[data-path="rhythm.beat.kick"] [data-step="1"]');
  const after = (await json()).rhythm.beat.kick;
  assert.equal(after[1], before[1] === "." ? "x" : before[1] === "x" ? "-" : ".", "a click cycles the step");
  await page.click('[data-path="rhythm.accents"] [data-step="1"]');
  assert.equal((await json()).rhythm.accents.length, 16, "a click writes all accents");
  await page.selectOption('.row[data-path="rhythm.division"] select', "3");
  const r = (await json()).rhythm;
  assert.ok(Object.values(r.beat).every((p) => p.length === 12) && r.accents.length === 12, "steps per beat refits the bar");
  await page.waitForSelector(".validity.ok");
  if (await page.isEnabled("#stop")) await page.click("#stop");
  await page.waitForFunction(() => !document.getElementById("rhythm-options").hidden);
  await page.waitForTimeout(1500);
  const withBeat = await rollNotes(page);
  await page.check("#mute-beat");
  await page.waitForFunction((n) => Number(document.getElementById("roll").dataset.notes) < n, withBeat, { timeout: 5000 });
  await page.uncheck("#mute-beat");
}

/** A 16-bit PCM mono WAV: a 440 Hz sine of half a second. */
function sineWAV() {
  const rate = 22050;
  const n = rate / 2;
  const b = Buffer.alloc(44 + 2 * n);
  b.write("RIFF", 0);
  b.writeUInt32LE(36 + 2 * n, 4);
  b.write("WAVEfmt ", 8);
  b.writeUInt32LE(16, 16);
  b.writeUInt16LE(1, 20);
  b.writeUInt16LE(1, 22);
  b.writeUInt32LE(rate, 24);
  b.writeUInt32LE(rate * 2, 28);
  b.writeUInt16LE(2, 32);
  b.writeUInt16LE(16, 34);
  b.write("data", 36);
  b.writeUInt32LE(2 * n, 40);
  for (let i = 0; i < n; i++) b.writeInt16LE(Math.round(16000 * Math.sin((2 * Math.PI * 440 * i) / rate)), 44 + 2 * i);
  return b;
}

/** A voice with a sample instrument is a problem until its WAV file is added; the file stays after a reload. */
async function samples(page) {
  const doc = JSON.stringify({ version: 0, name: "hum", instruments: { vowel: { wave: "triangle" }, motif: { sample: "hum.wav", root: 69, loop: true, sustain: 1 } } }, null, 2);
  await page.click("#tab-source");
  await page.fill("#doc", doc);
  await page.waitForSelector(".validity.bad");
  assert.match(await page.textContent("#problems"), /instruments\.motif\.sample/);
  await page.setInputFiles("#sample-file", { name: "hum.wav", mimeType: "audio/wav", buffer: sineWAV() });
  await page.waitForSelector(".validity.ok");
  await page.click("#tab-visual");
  await page.click('[data-instrument="motif"]');
  await page.click('[data-audition="motif"]');
  await page.waitForTimeout(600);
  assert.ok(Number(await page.getAttribute("#meter-bar", "data-max-peak")) > 0.05, "the sample sounds");
  await page.reload();
  await page.waitForSelector("body[data-ready=true]", { timeout: 30000 });
  assert.match(await page.textContent("#samples"), /hum\.wav/);
  await page.waitForSelector(".validity.ok");
  await page.click('#samples button');
  await page.waitForSelector(".validity.bad");
}

/** The Language tab checks a language file and the preview speaks it. */
async function language(page) {
  await page.selectOption("#starter", "soft");
  await page.waitForSelector(".validity.ok");
  await page.click("#tab-language");
  await page.fill("#lang", "{ broken");
  await page.waitForFunction(() => document.getElementById("lang-state").textContent.includes("problem"));
  await page.click("#lang-builtin");
  await page.waitForFunction(() => document.getElementById("lang-state").textContent === "Valid");
  await page.click("#lang-clear");
  await page.waitForFunction(() => document.getElementById("lang-state").textContent === "Built-in");
}

/** Share link copies an address that opens the same document. */
async function shareLink(page, context) {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.selectOption("#starter", "ghost");
  await page.waitForSelector(".validity.ok");
  const doc = await page.inputValue("#doc");
  await page.click("#share");
  await page.waitForSelector("#toast:not([hidden])");
  const url = await page.evaluate(() => navigator.clipboard.readText());
  assert.match(url, /#v=/);
  const other = await context.newPage();
  await other.goto(url);
  await other.waitForSelector("body[data-ready=true]", { timeout: 30000 });
  assert.equal(await other.inputValue("#doc"), doc);
  assert.ok(!(await other.evaluate(() => location.hash)), "the link is cleared from the address");
  await other.close();
}

/** The roll and waveform follow the document, Hear plays an instrument, WAV downloads the line. */
async function rollAuditionWAV(page) {
  await page.selectOption("#starter", "flow");
  await page.waitForSelector(".validity.ok");
  await page.waitForFunction(() => Number(document.getElementById("roll").dataset.notes) > 0, null, { timeout: 5000 });
  if (await page.isEnabled("#stop")) await page.click("#stop"); // a replaying line holds the waveform back
  await page.waitForFunction(() => Number(document.getElementById("wave").dataset.columns) > 10, null, { timeout: 10000 });
  const before = await page.getAttribute("#roll", "data-notes");
  await page.fill("#line", "Yo.");
  await page.waitForFunction((n) => document.getElementById("roll").dataset.notes !== n, before, { timeout: 5000 });

  await page.click("#tab-visual");
  for (const name of ["vowel", "kick"]) {
    await page.click(`[data-audition="${name}"]`);
    await page.waitForTimeout(600);
    const peak = Number(await page.getAttribute("#meter-bar", "data-max-peak"));
    assert.ok(peak > 0.01, `audition ${name}: peak ${peak}`);
  }

  const [dl] = await Promise.all([page.waitForEvent("download"), page.click("#wav")]);
  assert.equal(dl.suggestedFilename(), "flow-line.wav");
  const data = require("node:fs").readFileSync(await dl.path());
  assert.equal(data.subarray(0, 4).toString(), "RIFF");
  assert.equal(data.subarray(8, 12).toString(), "WAVE");
  assert.ok(data.length > 48000 * 8 * 0.3, `WAV of ${data.length} bytes`);
}

/**
 * Many plays and slider drags allocate enough for garbage collections; the
 * player must keep pulling samples through them (oto closes a player that
 * nothing references).
 */
async function soundSurvives(page) {
  await page.selectOption("#starter", "soft");
  await page.waitForSelector(".validity.ok");
  await page.click("#tab-visual");
  const before = await page.evaluate(() => voiceEditor.framesRead());
  for (let round = 0; round < 12; round++) {
    await page.click("#play");
    await page.evaluate(() => {
      const e = document.querySelector('.row[data-path="instruments.vowel.decayMs"] input[type=range]');
      for (let i = 0; i < 30; i++) {
        e.value = String(50 + i * 10);
        e.dispatchEvent(new Event("input", { bubbles: true }));
      }
      e.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await page.waitForTimeout(300);
  }
  const mid = await page.evaluate(() => voiceEditor.framesRead());
  await page.waitForTimeout(500);
  const after = await page.evaluate(() => voiceEditor.framesRead());
  assert.ok(mid > before && after > mid, `the player stopped reading: frames ${before}, ${mid}, ${after}`);
}

/** The Visual tab edits the document text and plays the line after an edit. */
async function visualEditor(page) {
  const doc = () => page.inputValue("#doc");
  const json = async () => JSON.parse(await doc());
  await page.selectOption("#starter", "soft");
  await page.waitForSelector(".validity.ok");
  await page.click("#tab-visual");
  assert.ok(await page.isVisible("#visual"));
  const tipOf = (sel) => page.$eval(sel, (e) => e.title || e.dataset.tip);
  assert.match(await tipOf('.row[data-path="speed"]'), /fast/, "a field explains itself");
  assert.match(await tipOf('.row[data-path="instruments.vowel.wave"]'), /square: /, "a menu explains its choices");
  await page.hover('.row[data-path="speed"] .label');
  await page.waitForSelector(".tooltip:not([hidden])", { timeout: 1000 });
  assert.match(await page.textContent(".tooltip"), /fast/, "the tooltip shows on hover");
  assert.equal(await page.getAttribute('.row[data-path="speed"]', "title"), null, "no second, native tooltip");
  await page.mouse.move(1, 1);
  await page.waitForSelector(".tooltip", { state: "hidden" });
  assert.ok(!(await page.isVisible("#doc")));

  // A number field: type a value, then undo and redo it.
  await page.fill('.row[data-path="speed"] .num', "20");
  await page.press('.row[data-path="speed"] .num', "Enter");
  assert.equal((await json()).speed, 20);
  assert.match(await doc(), /\n  "speed": 20,\n/, "the form writes the starter style");
  // Replay on change: audio runs (the starters played), so the edit plays the line.
  await page.waitForFunction(() => !document.getElementById("stop").disabled, null, { timeout: 5000 });
  await page.click("#stop");
  await page.click("#undo");
  assert.equal((await json()).speed, 13);
  assert.equal(await page.inputValue('.row[data-path="speed"] .num'), "13");
  await page.click("#redo");
  assert.equal((await json()).speed, 20);

  // A slider moves the value; reset removes the key (the default applies).
  const slider = page.locator('.row[data-path="instruments.vowel.decayMs"] input[type=range]');
  await slider.fill("400");
  assert.equal((await json()).instruments.vowel.decayMs, 400);
  await page.click('.row[data-path="speed"] .reset');
  assert.equal((await json()).speed, undefined);
  assert.ok(await page.$('.row.default[data-path="speed"]'), "a field left out shows its default");

  // A dropdown, and a problem shown at its field.
  await page.selectOption('.row[data-path="instruments.vowel.wave"] select', "saw");
  assert.equal((await json()).instruments.vowel.wave, "saw");
  await page.fill('.row[data-path="pitch"] .num', "30");
  await page.press('.row[data-path="pitch"] .num', "Enter");
  await page.waitForSelector('.row.bad[data-path="pitch"] .row-problem');
  await page.click('.row[data-path="pitch"] .reset');
  await page.waitForSelector(".validity.ok");

  // Instrument tabs: the sonorant can fall back to the vowel's sound and get its own again.
  await page.click('[data-instrument="sonorant"]');
  await page.uncheck('input[data-own="sonorant"]');
  assert.equal((await json()).instruments.sonorant, undefined);
  await page.check('input[data-own="sonorant"]');
  assert.deepEqual((await json()).instruments.sonorant, (await json()).instruments.vowel);

  // Rhythm, a beat track and a percussion sound.
  await page.check("input[data-rhythm]");
  await page.selectOption('.beat select.add', "kick");
  assert.equal((await json()).rhythm.beat.kick, "x...............");
  await page.selectOption('[data-section="percussion"] select.add', "snare");
  assert.deepEqual((await json()).percussion, { snare: {} });
  await page.fill('.row[data-path="percussion.snare.note"] .num', "40");
  await page.press('.row[data-path="percussion.snare.note"] .num', "Enter");
  assert.equal((await json()).percussion.snare.note, 40);
  await page.waitForSelector(".validity.ok");
  assert.ok(await page.evaluate((t) => voiceEditor.loadVoice(t).ok, await doc()));
  console.log("ok visual editor");
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
