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
import { readFile, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { extname, join, resolve } from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);
const dist = resolve(import.meta.dirname, "dist");
const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };

// An episode the harness can really play. The video element asks the Go
// side for /media/?path=..., which this server knew nothing about, so
// playing failed every time and everything that happens on the edge from
// paused to playing could not be reached at all. A probe about pressing
// the space bar passed against broken code because of it.
//
// Four hours of black at a frame a second, which ffmpeg makes in under two
// seconds and which comes to 320 KB, kept in the temp folder and made once.
// It is not in the repository: it is built output like everything else in
// dist/. Without ffmpeg there is no video, playing fails the way it did
// before, and a probe that needs it has to say so rather than pass.
//
// VP9 in a WebM, not H.264 in an MP4, because the Chromium that comes with
// Playwright is built without the proprietary codecs: canPlayType for
// avc1 answers with an empty string, so an MP4 loads, errors and never
// plays, which looks exactly like a video that is simply paused. The app
// itself runs in a WebKit view and plays the MP4 the Go side serves. This
// file only has to be a video the harness can play.
const episodeAt = join(tmpdir(), "framefairy-preview-episode.webm");

async function episode() {
  try {
    await stat(episodeAt);
    return episodeAt;
  } catch {}
  try {
    await run("ffmpeg", [
      "-v", "error",
      "-f", "lavfi", "-i", "color=c=black:s=64x64:r=1",
      "-t", "14423",
      "-c:v", "libvpx-vp9", "-b:v", "10k",
      "-deadline", "realtime", "-cpu-used", "8", "-g", "120",
      episodeAt, "-y",
    ]);
    return episodeAt;
  } catch {
    return null;
  }
}

// The video element asks for a stretch at a time and will not seek at all
// without a 206, so the range is answered rather than the whole file.
async function media(res, range) {
  const file = await episode();
  if (!file) {
    res.writeHead(404).end("no ffmpeg, so no episode to play");
    return;
  }
  const body = await readFile(file);
  const hit = /bytes=(\d*)-(\d*)/.exec(range ?? "");
  if (!hit) {
    res.writeHead(200, { "content-type": "video/webm", "content-length": body.length, "accept-ranges": "bytes" });
    res.end(body);
    return;
  }
  const from = hit[1] ? Number(hit[1]) : 0;
  const to = hit[2] ? Number(hit[2]) : body.length - 1;
  const part = body.subarray(from, to + 1);
  res.writeHead(206, {
    "content-type": "video/webm",
    "content-length": part.length,
    "accept-ranges": "bytes",
    "content-range": `bytes ${from}-${to}/${body.length}`,
  });
  res.end(part);
}

// A port of its own per run, so two probes never collide.
let port = 4300 + Math.floor(Math.random() * 400);

export async function workspace({
  // What the fake Go side should pretend. The modes are in wails-stub.ts:
  // ?busy, ?unknown, ?transcribing, ?growing, ?found.
  query = "",
  width = 1500,
  height = 1000,
  // 2 for a retina picture, which is what Tim sees. 1 makes a measurement
  // in whole pixels easier to read.
  scale = 1,
  // Picking a clip is what puts the caption settings and the clip panel on
  // screen. Without it half the workspace is not there to look at.
  clip = true,
} = {}) {
  const server = createServer(async (req, res) => {
    const url = new URL(req.url, "http://x");
    if (url.pathname.startsWith("/media/")) {
      await media(res, req.headers.range);
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

  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: scale });
  // A probe that reports nothing because the page threw is worse than no
  // probe at all, so anything thrown is printed.
  page.on("pageerror", (e) => console.log("pageerror", String(e)));
  await page.goto(`http://127.0.0.1:${at}/${query}`);
  await page.waitForTimeout(700);
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
    }
  }

  return {
    page,
    async stop() {
      await browser.close();
      server.close();
    },
  };
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
