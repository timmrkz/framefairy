// What every walk needs: the interface opened on the bridge's episode with
// its clip chosen, a way to wait until the app has settled, and the
// engine's own answer to compare the screen with. See docs/TESTING.md.
import { chromium } from "/opt/node22/lib/node_modules/playwright/index.mjs";

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

// What the engine says the clip's captions are, asked directly.
export async function engineCaptions(page, at) {
  return page.evaluate(async ({ path, plan, clip }) => {
    const answer = await fetch("/call", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: "Captions", args: [path, plan, clip] }),
    });
    return answer.json();
  }, at);
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
