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
