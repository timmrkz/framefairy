// Opens the built preview and hands back a page with an episode workspace
// already on screen, so a probe is a few lines rather than fifty.
//
//   npx vite build --config frontend/preview/vite.config.ts
//   node frontend/preview/probe.mjs        (whatever you called yours)
//
// See .claude/skills/interface/SKILL.md for how to write one and, more to
// the point, for what to measure.
import { chromium } from "/opt/node22/lib/node_modules/playwright/index.mjs";
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import { readFile, rename, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { extname, join, resolve } from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);
const dist = resolve(import.meta.dirname, "dist");
const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };

// The episodes the harness plays, made with ffmpeg the first time and
// kept in the temp folder. They are not in the repository: they are built
// output like everything else in dist/. Without ffmpeg there is no
// episode, the video preview says it cannot read the file, and a probe
// that needs one has to say so rather than pass.
//
// VP9 and Opus in an MP4. An MP4, because the video preview reads the
// file itself, see frontend/src/lib/frames/, and it reads MP4 and MOV.
// VP9 and Opus, because the Chromium that comes with Playwright is built
// without the proprietary codecs and decodes no H.264 or AAC. The app
// itself runs in a WebKit view and decodes what the Mac decodes.
//
// The picture says which frame it is. Its top half is the frame number in
// bars, the highest bit on the left, light for one and dark for nought, so
// a probe reads back from the canvas the frame that was drawn rather than
// trusting what it was told. The bottom half is a grey that climbs from
// the first second to the last, so a probe can also say roughly where in
// the episode the picture is, and that something is drawn at all: black
// and no picture look the same, and around 28 is a picture.
//
// The sound says where it is. A tone whose pitch climbs, with a little
// noise under it that is never the same twice. The tone is what a person
// hears move. The noise is what makes every few milliseconds of the sound
// unlike every other, so a probe can lay what was played over the
// episode's own sound, decoded by ffmpeg, and find a single sample
// missing, doubled or taken from inside a cut.
//
// Sixteen by nine, at 25 frames a second with a key frame every four
// seconds, so a cut lands in the middle of a group of pictures the way it
// does in a camera's file.
async function made(file, { seconds, bits, bar, height, climb, picture, sound }) {
  try {
    await stat(file);
    return file;
  } catch {}
  const width = bits * bar;
  try {
    // The bars are worked out on a picture one pixel a bit and made large
    // without smoothing, and the grey on a picture of four pixels, because
    // an expression worked out for every pixel of four hours of frames
    // takes ten minutes.
    await run("ffmpeg", [
      "-v", "error",
      "-f", "lavfi", "-i", `color=c=black:s=${bits}x2:r=25`,
      "-f", "lavfi", "-i", "color=c=black:s=2x2:r=25",
      "-f", "lavfi", "-i", `aevalsrc=0.35*sin(2*PI*(200*t+${climb}*t*t))+0.15*(random(0)*2-1):s=48000:c=mono`,
      "-t", String(seconds),
      "-filter_complex",
      `[0]geq=lum='if(bitand(N\\,pow(2\\,${bits - 1}-X))\\,220\\,30)':cb=128:cr=128,scale=${width}:${height / 2}:flags=neighbor[b];` +
        `[1]geq=lum='40+160*T/${seconds}':cb=128:cr=128,scale=${width}:${height / 2}:flags=neighbor[g];` +
        "[b][g]vstack,format=yuv420p[v]",
      "-map", "[v]", "-map", "2",
      "-c:v", "libvpx-vp9", "-b:v", picture, "-deadline", "realtime", "-cpu-used", "8",
      "-g", "100", "-keyint_min", "100",
      "-c:a", "libopus", "-b:a", sound,
      "-movflags", "+faststart",
      file + ".part.mp4", "-y",
    ]);
    await rename(file + ".part.mp4", file);
    return file;
  } catch {
    return null;
  }
}

// The episode of the workspace: four hours, the length the fake Go side
// says, 24 bars of 4 pixels, 96 by 54. Made in about three minutes, 40 MB.
export const episodeAt = join(tmpdir(), "framefairy-preview-episode-v3.mp4");
export function episode() {
  return made(episodeAt, { seconds: 14423, bits: 24, bar: 4, height: 54, climb: 0.05, picture: "20k", sound: "16k" });
}

// The episode of the frame queue's own page, see frames() below: ten
// minutes, 16 bars of 12 pixels, 192 by 108, in about half a minute.
export const framesAt = join(tmpdir(), "framefairy-preview-frames-v2.mp4");
export function framesEpisode() {
  return made(framesAt, { seconds: 600, bits: 16, bar: 12, height: 108, climb: 1, picture: "200k", sound: "96k" });
}

// The files, read once each. A file was read from disk on every range
// request, and the video preview asks for a great many of them.
const bodies = new Map();
async function body(file) {
  if (!file) return null;
  if (!bodies.has(file)) bodies.set(file, await readFile(file));
  return bodies.get(file);
}

// The video preview reads the file a range at a time, so the range is
// answered rather than the whole file.
async function media(res, range, frames = false) {
  const data = await body(frames ? await framesEpisode() : await episode());
  if (!data) {
    res.writeHead(404).end("no ffmpeg, so no episode to play");
    return;
  }
  const hit = /bytes=(\d*)-(\d*)/.exec(range ?? "");
  if (!hit) {
    res.writeHead(200, { "content-type": "video/mp4", "content-length": data.length, "accept-ranges": "bytes" });
    res.end(data);
    return;
  }
  const from = hit[1] ? Number(hit[1]) : 0;
  const to = hit[2] ? Math.min(Number(hit[2]), data.length - 1) : data.length - 1;
  const part = data.subarray(from, to + 1);
  res.writeHead(206, {
    "content-type": "video/mp4",
    "content-length": part.length,
    "accept-ranges": "bytes",
    "content-range": `bytes ${from}-${to}/${data.length}`,
  });
  res.end(part);
}

// A port of its own per run, so two probes never collide.
let port = 4300 + Math.floor(Math.random() * 400);

// The app as it opens, with nothing clicked. For anything that is not the
// episode workspace: the setup on a new machine, the settings, the app
// with no episode open. workspace() is this with an episode picked.
export async function screen({
  // What the fake Go side should pretend. The modes are in wails-stub.ts.
  query = "",
  width = 1500,
  height = 1000,
  // 2 for a retina picture, which is what Tim sees. 1 makes a measurement
  // in whole pixels easier to read.
  scale = 1,
  // A script to run in the page before anything of the app does, with its
  // argument, the way page.addInitScript takes them: a probe's own
  // recorder, or window.__pieces set before the episode is opened.
  init = null,
  initArg = undefined,
  // The page to open, the app unless it says otherwise.
  page: path = "",
  // More for the browser's command line.
  args = [],
} = {}) {
  // ?slowread=200 answers every range of an episode 200 ms late, the way a
  // busy disk or a long episode can, the video preview's and the frame
  // queue page's alike. The frame queue reads ahead so a cut never waits on
  // the file, and this is how to show it.
  const slowread = Number(/[?&]slowread=(\d+)/.exec(query)?.[1] ?? 0);
  const server = createServer(async (req, res) => {
    const url = new URL(req.url, "http://x");
    if (url.pathname.startsWith("/media/")) {
      if (slowread) await new Promise((r) => setTimeout(r, slowread));
      await media(res, req.headers.range, url.searchParams.get("path") === "/frames.mp4");
      return;
    }
    const file = url.pathname === "/" ? "/index.html" : url.pathname;
    try {
      const body = await readFile(join(dist, file));
      res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream" });
      res.end(body);
    } catch {
      res.writeHead(404).end("no");
    }
  });
  const at = port++;
  await new Promise((r) => server.listen(at, r));

  const browser = await chromium.launch({ args });
  const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: scale });
  // A probe that reports nothing because the page threw is worse than no
  // probe at all, so anything thrown is printed.
  page.on("pageerror", (e) => console.log("pageerror", String(e)));
  if (init) await page.addInitScript(init, initArg);
  await page.goto(`http://127.0.0.1:${at}/${path}${query}`);
  await page.waitForTimeout(700);

  return {
    page,
    width,
    height,
    async stop() {
      await browser.close();
      server.close();
    },
  };
}

// The episode workspace, open on an episode and on a clip of it, which is
// what nearly every probe is about.
export async function workspace({
  query = "",
  width = 1500,
  height = 1000,
  scale = 1,
  // Picking a clip is what puts the caption settings and the clip panel on
  // screen. Without it half the workspace is not there to look at.
  clip = true,
  init = null,
  initArg = undefined,
} = {}) {
  const open = await screen({ query, width, height, scale, init, initArg });
  const { page } = open;
  await page.getByText("Mein Arm ist zersprungen").first().click();
  await page.waitForTimeout(1100);

  // The sidebar lies over the settings column when it is open, so it is
  // put away first or it covers half of what is being looked at.
  if (await page.evaluate(() => document.querySelector("aside")?.classList.contains("open"))) {
    await page.locator("aside .head .glyph").click();
    await page.mouse.move(width / 2, height / 2);
    await page.waitForTimeout(400);
  }
  if (clip) {
    const first = page.locator("aside .list li button.pick").first();
    if (await first.count()) {
      await first.click();
      await page.waitForTimeout(800);
      // And onto the clip's first word. A clip starts a little before the
      // first thing said in it, the way the engine cuts it, so a playhead
      // sitting on the clip's own start is sitting in silence and the
      // caption box is not on screen at all. Anything about the captions
      // would then be measuring an empty picture. Shift and an arrow step
      // a word, which is exactly the one step needed.
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Shift+ArrowRight");
      await page.waitForTimeout(400);
    }
  }

  return open;
}

// Where things are, to the hundredth of a pixel, and whether they sit
// between two of them. An element on a fraction is painted differently
// once a fade puts it on a surface of its own, which is how the info marks
// came to step as they arrived.
export function boxes(page, named) {
  return page.evaluate(
    (list) =>
      list
        .map(([what, sel]) => {
          const el = document.querySelector(sel);
          if (!el) return `${what.padEnd(16)} not on screen`;
          const b = el.getBoundingClientRect();
          const off = Math.abs(b.top - Math.round(b.top));
          return (
            `${what.padEnd(16)} top ${b.top.toFixed(2).padStart(9)}` +
            ` h ${b.height.toFixed(2).padStart(8)}` +
            (off > 0.01 ? `   off a whole pixel by ${off.toFixed(2)}` : "")
          );
        })
        .join("\n"),
    named,
  );
}

// What is actually painted in a stretch of the page, rather than where the
// layout says it is. getBoundingClientRect cannot see a painting bug: it
// gives the layout position, which never moves. This takes the picture and
// reads the pixels back, one row at a time.
export async function rows(page, clip) {
  const png = (await page.screenshot({ clip })).toString("base64");
  return page.evaluate(async (data) => {
    const img = new Image();
    img.src = "data:image/png;base64," + data;
    await img.decode();
    const c = document.createElement("canvas");
    c.width = img.width;
    c.height = img.height;
    const ctx = c.getContext("2d");
    ctx.drawImage(img, 0, 0);
    const d = ctx.getImageData(0, 0, c.width, c.height).data;
    const out = [];
    for (let y = 0; y < c.height; y++) {
      let sum = 0;
      for (let x = 0; x < c.width; x++) sum += d[(y * c.width + x) * 4];
      out.push(Math.round(sum / c.width));
    }
    return out;
  }, png);
}

// The frame queue on its own, see frontend/src/lib/frames/ and "The frame
// queue" in the interface skill: one canvas playing the episode
// framesEpisode() makes, and window.__queue to drive it. ?read and ?hear
// write down what was drawn and what was heard, see preview/frames/main.ts,
// and ?slowread slows the file. The sound card is let go without a gesture,
// which the app gets from the space bar.
export async function frames({ query = "", width = 900, height = 520 } = {}) {
  if (!(await framesEpisode())) throw new Error("no ffmpeg, so no episode for the frame queue");
  const open = await screen({
    query,
    width,
    height,
    page: "preview/frames/index.html",
    args: ["--autoplay-policy=no-user-gesture-required"],
  });
  await open.page.waitForFunction(() => window.__ready, null, { timeout: 20000 });
  const ready = await open.page.evaluate(() => window.__ready);
  if (ready !== true) throw new Error(`the frame queue did not open: ${ready}`);
  return open;
}
