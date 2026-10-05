// The video preview of an episode just added, while its first search runs
// and after: the canvas shows the frame that holds the playhead the whole
// time. Adding a video does not run against the bridge, which adds its
// episode and searches it before it serves, so this is a probe on the
// preview, with ?growing: the first search started by the Go side, heard
// from 10 minutes and then found, job events four times a second, twelve
// clips landing and the earliest chosen when it is over. Then three clips
// picked from the list.
//
//   npx vite build --config frontend/preview/vite.config.ts
//   node frontend/preview/frames/added.mjs
//
// On every animation frame from the click that opens the episode it reads
// the playhead, data-playhead on .screen, and the frame on the canvas from
// its bars, and whether a picture is there at all. The rules: the first
// picture is the episode's first frame, with the playhead at 0, and the
// playhead stays there until a clip is chosen. No animation frame shows a
// frame that holds neither the playhead nor where it was a moment before,
// and a seek's frame is up within a quarter of a second. It exits 1 when a
// rule is broken.
import { screen } from "../open.mjs";
const query = process.argv[2] ?? "?growing&lagging&hear=100";
// From the first moment of the page, on every animation frame.
const recorder = () => {
  window.__rows = [];
  window.__mark = (what) => window.__rows.push({ t: performance.now(), mark: what });
  const step = () => {
    const s = document.querySelector(".screen");
    const c = s?.querySelector("canvas");
    if (c && c.width) {
      const g = c.getContext("2d");
      const scale = Math.min(c.width / 96, c.height / 54);
      const x0 = (c.width - 96 * scale) / 2;
      const y0 = (c.height - 54 * scale) / 2;
      const row = g.getImageData(0, Math.round(y0 + 13 * scale), c.width, 1).data;
      let n = 0;
      for (let b = 0; b < 24; b++) {
        const x = Math.round(x0 + (b * 4 + 2) * scale);
        n = n * 2 + (row[x * 4] > 125 ? 1 : 0);
      }
      // The grey of the bottom half: black is no picture.
      const grey = g.getImageData(Math.round(c.width / 2), Math.round(y0 + 40 * scale), 1, 1).data[0];
      const sel = document.querySelector("aside .list li.selected, aside .list li[aria-selected='true'], aside .list li.chosen");
      window.__rows.push({
        t: performance.now(),
        at: Number(s.dataset.playhead),
        pic: grey > 20 ? n : -1,
        clip: !!document.querySelector(".chosen"),
        job: document.querySelector(".ghost.next")?.textContent.trim().slice(0, 30) ?? "",
      });
    }
    requestAnimationFrame(step);
  };
  requestAnimationFrame(step);
};

const { page, stop } = await screen({ query, init: recorder });
const errors = [];
page.on("pageerror", (e) => errors.push(String(e)));
await page.evaluate(() => window.__mark("open the episode"));
await page.getByText("Mein Arm ist zersprungen").first().click();
// The search: heard from 10 minutes to the end of its half hour at 100 s a
// second, then twelve clips found, a few seconds each way.
await page.waitForTimeout(26000);
const picked = page.locator("aside .list li button.pick");
const n = await picked.count();
const clips = [];
for (const i of [Math.min(3, n - 1), Math.min(7, n - 1), 0]) {
  if (n === 0) break;
  await page.evaluate((i) => window.__mark(`pick clip ${i + 1}`), i);
  await picked.nth(i).click();
  clips.push(i);
  await page.waitForTimeout(1500);
}
const rows = await page.evaluate(() => window.__rows);
await stop();

// What each animation frame showed against what it should: the frame
// that holds the playhead. A row that shows the frame of the playhead a
// moment before is a seek on its way, counted on its own and timed.
const holds = (at) => Math.floor(at * 25 + 1e-6);
let wrong = 0;
let black = 0;
const wrongs = [];
const ways = [];
let way = null;
let lastAt = null;
const marks = [];
for (const r of rows) {
  if (r.mark) {
    marks.push(r);
    continue;
  }
  const want = holds(r.at);
  if (r.pic === want) {
    if (way) ways.push({ ...way, ms: r.t - way.t });
    way = null;
  } else if (r.pic === -1) {
    black++;
  } else if (lastAt !== null && r.pic === holds(lastAt.held) && r.at !== lastAt.held) {
    way ??= { t: r.t, from: lastAt.held, to: r.at, frames: 0 };
    way.frames++;
  } else {
    wrong++;
    if (wrongs.length < 12) wrongs.push(`${(r.t - marks[0].t).toFixed(0)} ms: playhead ${r.at.toFixed(3)} holds ${want}, canvas ${r.pic}${r.job ? `, "${r.job}"` : ""}`);
  }
  if (r.pic === want) lastAt = { held: r.at };
}
const t0 = marks[0].t;
const shown = rows.filter((r) => !r.mark);
const firstPicture = shown.find((r) => r.pic >= 0);
console.log(`${query}, from the click that opens the episode, ${shown.length} animation frames`);
console.log(`first picture ${firstPicture ? `${(firstPicture.t - t0).toFixed(0)} ms, frame ${firstPicture.pic}, playhead ${firstPicture.at}` : "never"}`);
console.log(`animation frames with no picture after the first ${shown.filter((r) => firstPicture && r.t > firstPicture.t && r.pic === -1).length}`);
// Where the playhead went, and when.
const moves = [];
for (let i = 1; i < shown.length; i++) if (shown[i].at !== shown[i - 1].at) moves.push(`${(shown[i].t - t0).toFixed(0)} ms to ${shown[i].at.toFixed(3)}`);
console.log(`the playhead moved ${moves.length} times: ${moves.join(", ")}`);
for (const m of marks.slice(1)) console.log(`${m.mark} at ${(m.t - t0).toFixed(0)} ms`);
console.log(`seeks on their way, the frame before still up: ${ways.map((w) => `${w.from.toFixed(2)} to ${w.to.toFixed(2)} for ${w.frames} frames, ${w.ms.toFixed(0)} ms`).join("; ") || "none"}`);
console.log(`animation frames with a frame that holds neither the playhead nor the one before: ${wrong}`);
for (const w of wrongs) console.log(`  ${w}`);
if (errors.length) console.log("page errors", errors);
const broken = [];
if (!firstPicture || firstPicture.pic !== 0 || firstPicture.at !== 0) broken.push("the first picture is not the episode's first frame at 0");
if (shown.some((r) => firstPicture && r.t > firstPicture.t && r.pic === -1)) broken.push("the picture went black");
const firstPick = marks.find((m) => m.mark.startsWith("pick"));
const firstMove = shown.findIndex((r, i) => i > 0 && r.at !== shown[i - 1].at);
if (firstMove < 0) broken.push("no clip was ever chosen");
if (wrong) broken.push("a frame that holds neither the playhead nor where it was");
if (ways.some((w) => w.ms > 250)) broken.push("a seek's frame took longer than a quarter of a second");
if (!firstPick || ways.length < 4) broken.push("the search chose no clip, or a pick did not move the playhead");
if (errors.length) broken.push("the page threw");
console.log(broken.length ? `broken: ${broken.join(", ")}` : "every rule held");
process.exit(broken.length ? 1 : 0);
