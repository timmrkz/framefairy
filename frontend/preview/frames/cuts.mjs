// The proof of the frame queue: a clip of three pieces with two cuts whose
// edges are not on frame edges, played through, with every frame drawn
// and every sample heard written down and checked.
//
//   npx vite build --config frontend/preview/vite.config.ts
//   node frontend/preview/frames/cuts.mjs [loop] [slowread=200] [seconds=8]
//   node frontend/preview/frames/cuts.mjs memory
//
// What it checks, for the picture: across each cut the time between the
// last frame drawn before it and the first after it, that no frame from
// inside a cut is drawn, the longest any frame stays, that every frame
// drawn is the one after the last apart from at a cut, and that the
// number read back from the canvas is the frame the queue said it drew.
// For the sound: every sample the sound card was handed, laid over the
// episode's own sound decoded by ffmpeg, cut and faded the way the render
// does it. A sample missing, doubled or taken from inside a cut shows as a
// stretch that no longer matches.
//
// memory plays a minute of the episode straight on and reads how many
// frames are open each second.
import { execFile } from "node:child_process";
import { readFile, stat } from "node:fs/promises";
import { promisify } from "node:util";
import { frames, framesAt } from "../open.mjs";

const run = promisify(execFile);
const args = process.argv.slice(2);
const loop = args.includes("loop");
const memory = args.includes("memory");
const slow = Number(args.find((a) => a.startsWith("slowread="))?.split("=")[1] ?? 0);
const seconds = Number(args.find((a) => /^\d+$/.test(a)) ?? (loop ? 14 : 8));
const R = 48000;
const d = 0.04;
const FADE = 0.015;
const pieces = [
  { start: 57, end: 59.53 },
  { start: 60.31, end: 62.17 },
  { start: 63.05, end: 65.0 },
];

const query = `?read&hear${slow ? `&slowread=${slow}` : ""}`;
const { page, stop } = await frames({ query });
const errors = [];
page.on("pageerror", (e) => errors.push(String(e)));

if (memory) {
  await page.evaluate(() => {
    window.__queue.setProgram([{ start: 0, end: 600 }], false);
    window.__queue.seek(100);
  });
  await page.waitForTimeout(500);
  await page.evaluate(() => window.__queue.play());
  const rows = [];
  for (let s = 1; s <= 60; s++) {
    await page.waitForTimeout(1000);
    rows.push(
      await page.evaluate(() => ({
        open: window.__queue.stats.open,
        most: window.__queue.stats.mostOpen,
        drawn: window.__queue.stats.drawn,
        late: window.__queue.stats.late,
        heap: Math.round(performance.memory.usedJSHeapSize / 1e6),
        at: window.__drawn[window.__drawn.length - 1]?.at,
      })),
    );
  }
  const opens = rows.map((r) => r.open);
  console.log(`a minute from 100 s: frames open ${Math.min(...opens)} to ${Math.max(...opens)}, most ever ${rows[rows.length - 1].most}`);
  console.log(`JS heap ${rows[0].heap} MB after a second, ${rows[rows.length - 1].heap} MB after a minute`);
  console.log(`drawn ${rows[rows.length - 1].drawn}, late ${rows[rows.length - 1].late}, playhead at ${rows[rows.length - 1].at.toFixed(2)}`);
  await stop();
  process.exit(0);
}

await page.evaluate(
  ([pieces, loop]) => {
    window.__queue.setProgram(pieces, loop);
    window.__queue.seek(pieces[0].start);
  },
  [pieces, loop],
);
await page.waitForTimeout(400);
const started = await page.evaluate(() => {
  window.__drawn = [];
  window.__heard = [];
  window.__queue.play();
  return performance.now();
});
await page.waitForTimeout(seconds * 1000);
const rec = await page.evaluate(() => ({
  drawn: window.__drawn,
  heard: window.__heard.map((h) => ({ frame: h.frame, data: Array.from(h.data) })),
  c0: window.__queue.c0,
  m0: window.__queue.m0,
  rate: window.__queue.audio.sampleRate,
  stats: window.__queue.stats,
}));
await page.evaluate(() => window.__queue.pause());
await stop();

// ---- The picture

const playing = rec.drawn.filter((x) => x.playing);
const length = pieces.reduce((s, p) => s + p.end - p.start, 0);
const cuts = pieces.slice(0, -1).map((p, i) => [p.end, pieces[i + 1].start]);
if (loop) cuts.push([pieces[pieces.length - 1].end, pieces[0].start]);
const floorFrame = (t) => Math.floor(t / d + 1e-6) * d;
const inCut = (f) => cuts.some(([e, s]) => f >= e - 1e-6 && f < floorFrame(s) - 1e-6) || f >= pieces[pieces.length - 1].end - 1e-6 || f < pieces[0].start - d;
const rows = [];
let wrongPicture = 0;
let notNext = 0;
let longest = 0;
const holds = [];
// The first frame drawn is the one under the playhead, there before the
// press, and it stays until the sound is heard: the time to the second is
// how long a press takes to show movement, reported on its own.
const moving = playing.length > 1 ? playing[1].drawnAt - started : NaN;
for (let i = 2; i < playing.length; i++) {
  const a = playing[i - 1];
  const b = playing[i];
  longest = Math.max(longest, b.drawnAt - a.drawnAt);
  if (b.drawnAt - a.drawnAt > 55) holds.push(`${a.frame.toFixed(2)} held ${(b.drawnAt - a.drawnAt).toFixed(0)} ms at ${(a.drawnAt - started).toFixed(0)} ms`);
  const jump = cuts.find(([e, s]) => a.frame < e && a.frame > e - 0.2 && b.frame >= floorFrame(s) - 1e-6 && b.frame < s + 0.2);
  if (jump) {
    rows.push({
      cut: `${jump[0].toFixed(2)} to ${jump[1].toFixed(2)}`,
      last: a.frame.toFixed(2),
      first: b.frame.toFixed(2),
      gap: (b.drawnAt - a.drawnAt).toFixed(1),
      at: (a.drawnAt - started).toFixed(0),
    });
  } else if (Math.abs(b.frame - a.frame - d) > 1e-6) {
    notNext++;
  }
}
for (const x of rec.drawn) if (x.pictured !== undefined && Math.abs(x.pictured * d - x.frame) > 1e-6) wrongPicture++;
const intervals = playing.slice(1).map((b, i) => b.drawnAt - playing[i].drawnAt);
const mean = intervals.reduce((s, x) => s + x, 0) / Math.max(intervals.length, 1);
const shownInCut = playing.filter((x) => inCut(x.frame)).map((x) => x.frame.toFixed(2));
const last = rec.drawn[rec.drawn.length - 1];

console.log(`frame queue, ${loop ? "looping" : "once"}${slow ? `, file answered ${slow} ms late` : ""}`);
console.log("");
console.log("| cut           | last before | first after | gap ms | at ms |");
console.log("| ------------- | ----------- | ----------- | -----: | ----: |");
for (const r of rows) console.log(`| ${r.cut.padEnd(13)} | ${r.last.padStart(11)} | ${r.first.padStart(11)} | ${r.gap.padStart(6)} | ${r.at.padStart(5)} |`);
console.log("");
console.log(`from the press to moving    ${moving.toFixed(0)} ms`);
console.log(`frames drawn while playing   ${playing.length}, ${mean.toFixed(1)} ms apart on average, the longest ${longest.toFixed(1)} ms`);
console.log(`frames held past 55 ms      ${holds.join(", ") || "none"}`);
console.log(`frames from inside a cut     ${shownInCut.join(" ") || "none"}`);
console.log(`not the next frame, off a cut ${notNext}`);
console.log(`canvas not the frame said    ${wrongPicture}`);
console.log(`at the end                   ${last.ended ? "ended" : last.playing ? "playing" : "paused"}, playhead ${last.at.toFixed(3)}, frame ${last.frame.toFixed(2)}`);
console.log(`late frames ${rec.stats.late}, late sound ${rec.stats.lateSound}, decoded ${rec.stats.decoded}, closed unseen ${rec.stats.unseen}, most frames open ${rec.stats.mostOpen}, reads ${rec.stats.reads}`);

// ---- The sound

const refAt = framesAt + ".f32";
try {
  await stat(refAt);
} catch {
  await run("ffmpeg", ["-v", "error", "-c:a", "libopus", "-i", framesAt, "-f", "f32le", "-ac", "1", "-ar", "48000", refAt, "-y"]);
}
const refBytes = await readFile(refAt);
const ref = new Float32Array(refBytes.buffer, refBytes.byteOffset, refBytes.length / 4);

// The program, sample by sample, the way plan.ts lays it out.
const visits = [];
for (let v = 0, from = 0; from < (loop ? 3 : 1) * length; v++) {
  const p = pieces[v % pieces.length];
  visits.push({ start: p.start, out: Math.round(from * R), end: Math.round((from + p.end - p.start) * R) });
  from += p.end - p.start;
}
function expected(m) {
  const v = visits.find((x) => m >= x.out && m < x.end);
  if (!v) return 0;
  const inside = Math.min(m - v.out + 0.5, v.end - m - 0.5);
  const gain = inside >= FADE * R ? 1 : Math.max(0, inside / (FADE * R));
  return ref[Math.round(v.start * R) + (m - v.out)] * gain;
}

const c0 = Math.round(rec.c0 * rec.rate);
// The sound card's own skips: render quanta it never rendered, which
// happen in a headless browser on a busy machine whatever was scheduled.
// A block they fall in is counted apart, not as a fault of the queue.
const skips = [];
for (let i = 1; i < rec.heard.length; i++) {
  const step = rec.heard[i].frame - rec.heard[i - 1].frame;
  if (step !== rec.heard[i - 1].data.length) skips.push({ at: (rec.heard[i - 1].frame + 128 - c0) / R, step });
}
const heard = new Map();
for (const h of rec.heard) for (let i = 0; i < h.data.length; i++) heard.set(h.frame + i - c0 + rec.m0, h.data[i]);
const ms = [...heard.keys()].filter((m) => m >= 0).sort((a, b) => a - b);
const end = loop ? ms[ms.length - 1] - 2000 : Math.round(length * R);
const block = 120;
let worst = { err: 0, at: 0 };
let silent = 0;
let blocks = 0;
let skipped = 0;
for (let b = 0; b + block <= end; b += block) {
  let err = 0;
  let energy = 0;
  let got = 0;
  let missing = 0;
  for (let m = b; m < b + block; m++) if (!heard.has(m)) missing++;
  if (missing) {
    skipped++;
    continue;
  }
  for (let m = b; m < b + block; m++) {
    const e = expected(m);
    const h = heard.get(m) ?? 0;
    err += (h - e) ** 2;
    energy += e * e;
    got += h * h;
  }
  blocks++;
  if (energy > 1e-3 * block && got < 1e-6 * block) silent++;
  const rel = Math.sqrt(err / Math.max(energy, 1e-9));

  if (energy > 1e-3 * block && rel > worst.err) worst = { err: rel, at: b / R };
}
// And past the end of a clip played once: nothing at all.
let after = 0;
if (!loop) for (const m of ms) if (m >= Math.round(length * R) && Math.abs(heard.get(m)) > 1e-4) after++;
// Each cut, sample by sample: the error of the 10 ms on either side, and
// the lag that would fit them best, which is 0 when the cut is at the
// sample.
const lagRows = [];
for (const v of visits.slice(1)) {
  if (v.out >= end) break;
  const fit = (lag) => {
    let err = 0;
    let energy = 0;
    for (let m = v.out + Math.round(FADE * R); m < v.out + Math.round(FADE * R) + 480; m++) {
      const e = expected(m);
      err += ((heard.get(m + lag) ?? 0) - e) ** 2;
      energy += e * e;
    }
    return Math.sqrt(err / energy);
  };
  let best = 0;
  for (let lag = -48; lag <= 48; lag++) if (fit(lag) < fit(best)) best = lag;
  lagRows.push({ at: (v.out / R).toFixed(4), err: fit(0).toFixed(4), best });
}
console.log("");
console.log("| cut on the program, s | error after the fade | best lag, samples |");
console.log("| --------------------: | -------------------: | ----------------: |");
for (const r of lagRows) console.log(`| ${r.at.padStart(21)} | ${r.err.padStart(20)} | ${String(r.best).padStart(17)} |`);
console.log("");
console.log(`render quanta the sound card skipped ${skips.length}${skips.length ? `, at ${skips.map((x) => `${x.at.toFixed(3)} s by ${x.step}`).join(" ")}` : ""}, blocks left out for them ${skipped}`);
console.log(`sound blocks of 2.5 ms checked ${blocks}, silent where sound belongs ${silent}, the worst off by ${(worst.err * 100).toFixed(2)} % at ${worst.at.toFixed(3)} s`);
if (!loop) console.log(`samples heard past the end     ${after}`);
if (errors.length) console.log("page errors", errors);
