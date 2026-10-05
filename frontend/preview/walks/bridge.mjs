// What every walk needs: the interface opened on the bridge's episode with
// its clip chosen, a way to wait until the app has settled, and the
// engine's own answer to compare the screen with. See docs/TESTING.md.
import { chromium } from "playwright";

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
  const browser = await chromium.launch();
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

// Which clip is on screen: the episode, the plan and the clip the
// interface last asked the captions of.
export async function chosen(page) {
  return page.evaluate(() => {
    const last = [...window.__calls].reverse().find((c) => c.name === "Captions");
    return last ? { path: last.args[0], plan: last.args[1], clip: last.args[2] } : null;
  });
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
// canvas of the video preview, or -1 while it has none.
async function reader(page) {
  await page.evaluate(([bits, strip]) => {
    if (window.__pictured) return;
    window.__pictured = () => {
      const c = document.querySelector(".screen canvas");
      if (!c || !c.width || !c.height) return -1;
      const g = c.getContext("2d");
      const row = g.getImageData(0, Math.floor((c.height * strip) / 2), c.width, 1).data;
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
