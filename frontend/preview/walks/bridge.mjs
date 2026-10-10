// What every walk needs: the interface opened on the bridge's episode with
// its clip chosen, a way to wait until the app has settled, and the
// engine's own answer to compare the screen with. See docs/TESTING.md.
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { chromium } from "playwright";

// The Chromium the pinned Playwright was made for, which CI installs. A
// machine that has only another one walks on that rather than not at all:
// a cloud session keeps the Chromium of the Playwright it came with in
// PLAYWRIGHT_BROWSERS_PATH, and after Playwright went from 1.56 to 1.63
// every walk there stopped on a browser that was not installed.
function browserPath() {
  if (existsSync(chromium.executablePath())) return undefined;
  const root = process.env.PLAYWRIGHT_BROWSERS_PATH;
  if (!root || !existsSync(root)) return undefined;
  const builds = readdirSync(root)
    .filter((d) => /^chromium-\d+$/.test(d))
    .sort((a, b) => Number(b.split("-")[1]) - Number(a.split("-")[1]));
  for (const dir of builds) {
    const exe = join(root, dir, "chrome-linux", "chrome");
    if (existsSync(exe)) return exe;
  }
  return undefined;
}

// A random number generator that a seed decides, so a walk that breaks a
// rule can be walked again step for step. mulberry32.
export function random(seed) {
  let a = seed >>> 0;
  const next = () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
  return {
    next,
    int: (n) => Math.floor(next() * n),
    pick: (list) => list[Math.floor(next() * list.length)],
    // One of a list, each as likely as its weight.
    weighted(list) {
      let at = next() * list.reduce((sum, x) => sum + x.weight, 0);
      for (const x of list) if ((at -= x.weight) < 0) return x;
      return list[list.length - 1];
    },
  };
}

// The interface on the bridge, in the episode's workspace with its clip
// chosen and the keyboard on the page.
export async function open(url) {
  // The episode the way the bridge found it, whatever the walk before did.
  const reset = await fetch(new URL("/reset", url), { method: "POST" });
  if (!reset.ok) throw new Error(`the bridge did not reset: ${await reset.text()}`);
  // WebGPU on, which headless Chromium on Linux only gives when asked, so
  // the video preview draws the way it does in the app, see screen.ts.
  const browser = await chromium.launch({ executablePath: browserPath(), args: ["--enable-unsafe-webgpu"] });
  const page = await browser.newPage({ viewport: { width: 1500, height: 1000 } });
  const errors = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  await page.goto(url);
  await page.locator("aside li", { hasText: ".mp4" }).first().waitFor();
  await page.locator("aside li", { hasText: ".mp4" }).first().click();
  // The sidebar of a new library is pinned open, over the left of the
  // workspace and the start of the clip timeline. It is let go of, the way
  // a person who works in the workspace would, so a hand can reach what is
  // under it.
  if (await page.evaluate(() => document.querySelector("aside")?.classList.contains("open"))) {
    await page.locator("aside .head .glyph").click();
    await page.mouse.move(750, 500);
    await page.waitForTimeout(400);
  }
  await page.locator("aside .list li button.pick").first().waitFor();
  await page.locator("aside .list li button.pick").first().click();
  await page.locator(".captions .word").first().waitFor();
  await settle(page);
  await page.evaluate(() => document.body.focus());
  return { browser, page, errors };
}

// Waits until no call has been on its way for a while and the page has
// drawn what came back.
export async function settle(page) {
  await page.waitForFunction(
    () => {
      const w = window;
      if (w.__pending > 0) {
        w.__quietSince = 0;
        return false;
      }
      w.__quietSince ||= performance.now();
      return performance.now() - w.__quietSince > 250;
    },
    null,
    { polling: 50, timeout: 15000 },
  );
  await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
  await page.evaluate(() => (window.__quietSince = 0));
}

// Which episode is on screen: the one the sidebar marks as the current
// one, by its path in the library. Not the last one the interface asked
// the clips of: a video just added asks for them only later, and a walk
// then held its clip list to the episode before.
export async function episodeOn(page) {
  const name = await page.evaluate(
    () => document.querySelector("aside button.episode.current .name")?.textContent ?? null,
  );
  if (!name) return null;
  const named = (await ask(page, "Library")).filter((e) => e.name === name).map((e) => e.source);
  if (named.length < 2) return named[0] ?? null;
  // Two episodes of one file name, from two folders, look the same in the
  // sidebar, so they are told apart by the one whose clips the app asked
  // for last: the one on screen, once its clip list has come, which a
  // walk waits for before it asks.
  return page.evaluate(
    (paths) => [...window.__calls].reverse().find((c) => c.name === "Clips" && paths.includes(c.args[0]))?.args[0] ?? null,
    named,
  );
}

// Which clip is on screen: the card the clip list marks as the current
// one, in the episode on screen, by its plan and id as the engine names
// them. Not the last captions asked for: an episode just opened shows its
// clip before that, and a walk then held the clip on screen to another
// episode's.
export async function chosen(page) {
  const path = await episodeOn(page);
  const key = await page.evaluate(
    () => document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? null,
  );
  if (!path || !key) return null;
  const entry = (await ask(page, "Clips", path)).find((c) => c.key === key);
  return entry ? { path, plan: entry.plan, clip: entry.id } : null;
}

// What the engine says about the clip on screen, asked directly: its
// captions and its pieces, on the episode's clock. A walk compares what is
// on screen with this, and this before a step with this after it.
export async function engineState(page, at) {
  return page.evaluate(async ({ path, plan, clip }) => {
    const ask = async (name, args) => {
      const answer = await fetch("/call", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, args }),
      });
      return answer.json();
    };
    const [captions, clips] = await Promise.all([ask("Captions", [path, plan, clip]), ask("Clips", [path])]);
    const entry = clips.find((c) => c.plan === plan && (c.id === clip || c.basename === clip));
    return { ...captions, segments: (entry?.segments ?? []).map((s) => ({ start: s.start, end: s.end })) };
  }, at);
}

// What the clip timeline shows: where the clip starts and ends and where
// each cut is, on the episode's clock, read off its handles.
export async function timeline(page) {
  return page.evaluate(() => {
    const value = (el) => Number(el?.getAttribute("aria-valuenow"));
    const box = document.querySelector(".clip-timeline");
    const start = box?.querySelector('[aria-label="Clip start"]');
    if (!start) return null;
    const cuts = [];
    for (const edge of box.querySelectorAll(".cutedge")) {
      const hit = /cut (\d+) (starts|ends)/.exec(edge.getAttribute("aria-label") ?? "");
      if (!hit) continue;
      const i = Number(hit[1]) - 1;
      cuts[i] ??= {};
      cuts[i][hit[2] === "starts" ? "from" : "to"] = value(edge);
    }
    return {
      start: value(start),
      end: value(box.querySelector('[aria-label="Clip end"]')),
      cuts: cuts.filter(Boolean),
    };
  });
}

// What the caption box shows: each word, the moment it stands for, and
// the word of the episode it is.
export async function shown(page) {
  return page.evaluate(() =>
    [...document.querySelectorAll(".captions .word")].map((w) => ({
      text: w.textContent,
      at: Number(w.dataset.at),
      to: Number(w.dataset.to),
      keyed: w.classList.contains("keyed"),
      focused: document.activeElement === w,
    })),
  );
}

// Where everything a hand can take hold of on the clip timeline is on
// screen.
export async function handles(page) {
  return page.evaluate(() => {
    const rect = (el) => {
      if (!el) return null;
      const b = el.getBoundingClientRect();
      return { x: b.left, y: b.top, w: b.width, h: b.height };
    };
    const box = document.querySelector(".clip-timeline");
    return {
      track: rect(box.querySelector(".track")),
      start: rect(box.querySelector('[aria-label="Clip start"]')),
      end: rect(box.querySelector('[aria-label="Clip end"]')),
      cuts: [...box.querySelectorAll(".cut")].map(rect),
      cutEdges: [...box.querySelectorAll(".cutedge")].map(rect),
    };
  });
}

// A point is one a hand can reach when it is over the track.
export const inside = (t, x) => x > t.x + 2 && x < t.x + t.w - 2;
export const middle = (r) => r.x + r.w / 2;
// High on the track, over the waveform rather than the captions' band.
export const high = (t) => t.y + t.h * 0.2;

// A drag of the pointer from a point, by dx, in steps the way a hand
// moves, with shift held when asked.
export async function drag(page, x, y, dx, shift) {
  if (shift) await page.keyboard.down("Shift");
  await page.mouse.move(x, y);
  await page.mouse.down();
  const n = 6;
  for (let i = 1; i <= n; i++) {
    await page.mouse.move(x + (dx * i) / n, y);
    await page.waitForTimeout(30);
  }
  await page.mouse.up();
  if (shift) await page.keyboard.up("Shift");
}

// Asks the Go side something directly, the way the interface does.
export async function ask(page, name, ...args) {
  return page.evaluate(
    async ({ name, args }) => {
      const answer = await fetch("/call", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, args }),
      });
      return answer.json();
    },
    { name, args },
  );
}

// The bridge's episode carries its frame number in its picture, see
// bridge_test.go: a strip along its top, STRIP of its height, is BITS
// bars, the highest bit on the left, light for one and dark for nought,
// the recipe of the harness's own episodes in open.mjs. So a walk reads
// back from the video preview's canvas the frame that is on screen, as
// the probes of the frame queue do, rather than taking anybody's word for
// it.
const BITS = 10;
const STRIP = 12 / 180;

// Puts the reader on the page once: the number of the frame on the
// canvas of the video preview, or -1 while it has none. A canvas with
// nothing drawn on it is black, and so are the bars of frame 0, so whether
// there is a picture is read from the grey under the bars, which no frame
// has black.
async function reader(page) {
  await page.evaluate(([bits, strip]) => {
    if (window.__pictured) return;
    window.__pictured = () => {
      const c = document.querySelector(".screen canvas");
      if (!c || !c.width || !c.height) return -1;
      // The canvas may be a WebGPU canvas, which has no 2D context, so a piece
      // of it is copied to a 2D canvas to be read.
      const grab = (x, y, w, h) => {
        const s = document.createElement("canvas");
        s.width = w;
        s.height = h;
        const t = s.getContext("2d", { willReadFrequently: true });
        t.drawImage(c, x, y, w, h, 0, 0, w, h);
        return t.getImageData(0, 0, w, h).data;
      };
      const row = grab(0, Math.floor((c.height * strip) / 2), c.width, 1);
      const grey = grab(Math.floor(c.width / 2), Math.floor(c.height * 0.6), 1, 1)[0];
      if (grey < 6) return -1;
      let n = 0;
      for (let b = 0; b < bits; b++) {
        const x = Math.floor(((b + 0.5) * c.width) / bits);
        n = n * 2 + (row[x * 4] > 125 ? 1 : 0);
      }
      return n;
    };
  }, [BITS, STRIP]);
}

// Records every frame the video preview puts on screen, as the moment of
// the episode it starts at, until stopped: the picture read back from the
// canvas on every animation frame, not what the clock says. The frame on
// screen when recording starts is where it starts from and is not one of
// them, since it was put up before. Also whether the playhead moved,
// data-playhead on the video preview, so a play whose first frame is the
// one already on screen is seen to have started.
export async function recordFrames(page, frame) {
  await reader(page);
  await page.evaluate(() => {
    const screen = document.querySelector(".screen");
    const from = screen.dataset.playhead;
    let last = window.__pictured();
    window.__frames = [];
    window.__moved = false;
    window.__recording = true;
    const watch = () => {
      if (!window.__recording) return;
      const n = window.__pictured();
      if (n >= 0 && n !== last) window.__frames.push(n);
      last = n;
      if (screen.dataset.playhead !== from) window.__moved = true;
      requestAnimationFrame(watch);
    };
    requestAnimationFrame(watch);
  });
  return async () => {
    const r = await page.evaluate(() => {
      window.__recording = false;
      return { frames: window.__frames, moved: window.__moved };
    });
    return { frames: r.frames.map((n) => n * frame), moved: r.moved };
  };
}

// Where the video preview is: the playhead, data-playhead, the moment the
// frame on its canvas starts at, and whether it plays, which the play
// button says as it says it to a person.
export async function preview(page, frame) {
  await reader(page);
  const r = await page.evaluate(() => ({
    at: Number(document.querySelector(".screen").dataset.playhead),
    pictured: window.__pictured(),
    paused: document.querySelector('button[aria-label="Play"]') !== null,
  }));
  return { at: r.at, frame: r.pictured < 0 ? NaN : r.pictured * frame, paused: r.paused };
}

// What the clip list shows: the button in its head, the count beside the
// word Clips, the clip cards by their keys, and the rows that say what the
// work on the clips is doing, each with its words and its line under them.
export async function clipList(page) {
  return page.evaluate(() => {
    const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
    if (!pane) return null;
    const text = (el) => (el?.textContent ?? "").replace(/\s+/g, " ").trim();
    return {
      head: text(pane.querySelector(".listhead button.new")),
      count: Number(text(pane.querySelector(".listhead .num")) || 0),
      cards: [...pane.querySelectorAll("ol li[data-key]")].map((li) => li.dataset.key),
      rows: [...pane.querySelectorAll("ol li.ghost")]
        .map((li) => ({ what: text(li.querySelector(".title")), meta: text(li.querySelector(".meta")) }))
        .filter((r) => r.what),
    };
  });
}

// Asks the bridge itself for something a walk needs and the interface
// cannot do: a file for the Add button's box, the model holding or
// failing its answers, the speech model's pace, the app opened again.
export async function control(url, path) {
  const answer = await fetch(new URL(path, url), { method: "POST" });
  if (!answer.ok) throw new Error(`the bridge said no to ${path}: ${await answer.text()}`);
  return answer.json();
}

// Presses the clip list's head button, and reads what the list says in
// the very next frame. Also what the button said as it was pressed, which
// is not always what it said when the walk looked: a search the model
// fails ends in a moment, and Cancel is Continue by the time the hand
// lands.
export async function pressHead(page) {
  return page.evaluate(
    () =>
      new Promise((done) => {
        const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
        const text = (el) => (el?.textContent ?? "").replace(/\s+/g, " ").trim();
        const button = pane.querySelector(".listhead button.new");
        const pressed = text(button);
        button.click();
        requestAnimationFrame(() => {
          done({
            pressed,
            head: text(pane.querySelector(".listhead button.new")),
            rows: [...pane.querySelectorAll("ol li.ghost .title")].map(text),
          });
        });
      }),
  );
}

// Reaches into the sidebar, which is a rail until the pointer is on it,
// does something there and leaves it again.
export async function fromSidebar(page, click) {
  await page.mouse.move(20, 500);
  await page.waitForTimeout(400);
  await click();
  await page.mouse.move(750, 400);
  await page.waitForTimeout(400);
}

// What a picture shows, in a band across it from a tenth of its height
// to a half: below the strip that carries the frame number, and above
// the captions a short burns in. The mean of red, green and blue, and how
// far the brightness strays from its mean, which is high where there is
// detail, a subject in focus, and next to nothing on a plain backdrop.
export function lookOf(rgba, width, height, channels = 4) {
  let r = 0, g = 0, b = 0, n = 0;
  const lum = [];
  for (let y = Math.floor(height * 0.1); y < Math.floor(height * 0.5); y++) {
    for (let x = 0; x < width; x++) {
      const i = (y * width + x) * channels;
      r += rgba[i];
      g += rgba[i + 1];
      b += rgba[i + 2];
      lum.push(0.299 * rgba[i] + 0.587 * rgba[i + 1] + 0.114 * rgba[i + 2]);
      n++;
    }
  }
  const mean = lum.reduce((s, v) => s + v, 0) / n;
  const spread = Math.sqrt(lum.reduce((s, v) => s + (v - mean) ** 2, 0) / n);
  return { r: r / n, g: g / n, b: b / n, spread };
}

// Whether two looks are of the same thing: the same colour within a few
// levels, and as much detail within a share, so a subject in focus is
// never taken for a plain backdrop.
export function sameLook(a, b) {
  const near = (x, y) => Math.abs(x - y) <= 24;
  const ratio = (Math.max(a.spread, b.spread) + 4) / (Math.min(a.spread, b.spread) + 4);
  return near(a.r, b.r) && near(a.g, b.g) && near(a.b, b.b) && ratio < 1.6;
}

export const sayLook = (l) =>
  `rgb ${l.r.toFixed(0)} ${l.g.toFixed(0)} ${l.b.toFixed(0)}, detail ${l.spread.toFixed(0)}`;

// Where a moment of the clip is on the clip timeline, in pixels across
// the page: placed by the clip's own frame there, the rules that run from
// its first piece to its last.
export async function xOf(page, at) {
  const t = await timeline(page);
  const span = await page.evaluate(() => {
    const b = document.querySelector(".clip-timeline .span.frame").getBoundingClientRect();
    return { x: b.left, w: b.width };
  });
  return span.x + ((at - t.start) / (t.end - t.start)) * span.w;
}

// Puts the playhead on a moment of the clip with a click on the clip
// timeline, the way a hand does, and waits until the video preview shows
// the frame that holds it, at fps frames a second. Says whether that
// frame came.
export async function seekTo(page, at, fps) {
  const g = await handles(page);
  await page.mouse.click(await xOf(page, at), high(g.track));
  await settle(page);
  await reader(page);
  return page
    .waitForFunction(
      (fps) => {
        const at = Number(document.querySelector(".screen").dataset.playhead);
        // The frame that holds the playhead, a frame that begins a
        // millisecond or less after it included, see frameAt in
        // lib/flow.ts.
        return window.__pictured() === Math.floor((at + 0.001) * fps);
      },
      fps,
      { polling: 50, timeout: 5000 },
    )
    .then(() => true, () => false);
}

// The crop frame in the video preview, where the playhead is: where it
// stands, as shares of the picture, and what the picture shows inside it,
// read off the canvas under it. Null while the playhead is outside the
// clip and there is no frame.
export async function cropFrame(page) {
  const got = await page.evaluate(() => {
    const frame = document.querySelector(".screen .frame");
    const canvas = document.querySelector(".screen canvas");
    if (!frame || !canvas?.width) return null;
    const f = frame.getBoundingClientRect();
    const c = canvas.getBoundingClientRect();
    const left = (f.left - c.left) / c.width;
    const width = f.width / c.width;
    // A pixel in from either side, past the line the frame is drawn with.
    const x0 = Math.ceil((left + 0.01) * canvas.width);
    const x1 = Math.floor((left + width - 0.01) * canvas.width);
    // The canvas may be a WebGPU canvas, which has no 2D context, so the
    // piece is copied to a 2D canvas to be read.
    const s = document.createElement("canvas");
    s.width = x1 - x0;
    s.height = canvas.height;
    const t = s.getContext("2d", { willReadFrequently: true });
    t.drawImage(canvas, x0, 0, x1 - x0, canvas.height, 0, 0, x1 - x0, canvas.height);
    const pixels = t.getImageData(0, 0, x1 - x0, canvas.height).data;
    return { left, width, w: x1 - x0, h: canvas.height, pixels: [...pixels] };
  });
  if (!got) return null;
  return { left: got.left, width: got.width, look: lookOf(got.pixels, got.w, got.h) };
}

// A short read back from disk with ffmpeg: every frame, small, as what it
// looks like, and the sound, one channel at 48 kHz.
export function readShort(path) {
  const w = 90, h = 160;
  const raw = execFileSync("ffmpeg", ["-v", "error", "-i", path, "-vf", `scale=${w}:${h}`,
    "-f", "rawvideo", "-pix_fmt", "rgb24", "-"], { maxBuffer: 1 << 28 });
  const frames = [];
  for (let at = 0; at + w * h * 3 <= raw.length; at += w * h * 3) {
    frames.push(lookOf(raw.subarray(at, at + w * h * 3), w, h, 3));
  }
  const sound = shortSound(path);
  const rate = execFileSync("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries",
    "stream=r_frame_rate", "-of", "csv=p=0", path]).toString().trim().split("/");
  return { frames, fps: Number(rate[0]) / Number(rate[1] ?? 1), sound, rate: 48000 };
}

// How loud the sound is over 10 ms from a moment, as its root mean square.
export function loudness(short, at) {
  const from = Math.round(at * short.rate);
  const n = Math.round(0.01 * short.rate);
  let sum = 0;
  for (let i = from; i < from + n; i++) sum += (short.sound[i] ?? 0) ** 2;
  return Math.sqrt(sum / n);
}

// A filmed episode, made by /pick with a rate, carries its frame number
// in a way a render keeps, see makeFilmedEpisode in bridge_test.go: bands
// one above the other across the whole width of a picture 180 pixels
// high, FILMED_BITS of them, the highest bit at the top, each BAND pixels
// high, light for one and dark for nought.
const FILMED_BITS = 11;
const BAND = 4;

// The frame of the episode each frame of a rendered short is, by its
// number read back from the bands, in the order the short shows them. A
// short keeps the whole height of the episode, so ffmpeg makes each frame
// of it 180 pixels high again, and four wide, the average of each row,
// where the captions burned in lower down change nothing.
export function shortFrames(path) {
  const raw = execFileSync("ffmpeg", ["-v", "error", "-i", path, "-vf", "scale=4:180:flags=area",
    "-f", "rawvideo", "-pix_fmt", "gray", "-"], { maxBuffer: 1 << 30 });
  const size = 4 * 180;
  const frames = [];
  for (let at = 0; at + size <= raw.length; at += size) {
    let n = 0;
    for (let b = 0; b < FILMED_BITS; b++) {
      const row = at + Math.floor((b + 0.5) * BAND) * 4;
      const level = (raw[row] + raw[row + 1] + raw[row + 2] + raw[row + 3]) / 4;
      // Light is 160 and dark 60.
      n = n * 2 + (level > 110 ? 1 : 0);
    }
    frames.push(n);
  }
  return { frames, modulo: 2 ** FILMED_BITS };
}

// The number of the filmed episode's frame on the video preview's canvas,
// read from its bands the way shortFrames reads them from a short, or -1
// while there is no picture. It is put on the page as window.__filmed, so
// a walk can wait for a frame to come.
export async function filmedOnScreen(page) {
  await page.evaluate(([bits, band]) => {
    if (window.__filmed) return;
    window.__filmed = () => {
      const c = document.querySelector(".screen canvas");
      if (!c || !c.width || !c.height) return -1;
      // The canvas may be a WebGPU canvas, which has no 2D context, so a piece
      // of it is copied to a 2D canvas to be read.
      const grab = (x, y, w, h) => {
        const s = document.createElement("canvas");
        s.width = w;
        s.height = h;
        const t = s.getContext("2d", { willReadFrequently: true });
        t.drawImage(c, x, y, w, h, 0, 0, w, h);
        return t.getImageData(0, 0, w, h).data;
      };
      const x = Math.floor(c.width / 4);
      const w = Math.max(1, Math.floor(c.width / 2));
      let n = 0;
      for (let b = 0; b < bits; b++) {
        const y = Math.floor((((b + 0.5) * band) / 180) * c.height);
        const row = grab(x, y, w, 1);
        let sum = 0;
        for (let i = 0; i < row.length; i += 4) sum += row[i];
        n = n * 2 + (sum / (row.length / 4) > 110 ? 1 : 0);
      }
      return n;
    };
  }, [FILMED_BITS, BAND]);
  return page.evaluate(() => window.__filmed());
}

// The sound of a rendered short, one channel at 48 kHz, sample by sample.
export function shortSound(path) {
  const raw = execFileSync("ffmpeg", ["-v", "error", "-i", path, "-ac", "1", "-ar", "48000",
    "-f", "f32le", "-"], { maxBuffer: 1 << 30 });
  return new Float32Array(Uint8Array.from(raw).buffer);
}
