// The loop every walk runs: look, pick a gesture that can be made now, make
// it, and have the watch check every rule. A walk file only says what its
// gestures are. See docs/TESTING.md.
//
//   SEED=12 STEPS=80 BRIDGE_URL=http://127.0.0.1:8123/ node <walk>.mjs
//
// VERBOSE=1 prints what the walk saw after every step. HEVC=1 walks an
// episode whose picture is HEVC with 10-bit colour instead of the bridge's
// own, added and searched first: the Chromium a walk drives cannot decode
// it, so every frame of it comes from the Go side, see lib/frames/app.ts.
import { random, open, chosen, shown, control, fromSidebar, settle } from "./bridge.mjs";
import { Watch } from "./rules.mjs";

// The page on the bridge, the seed's random numbers, and the watch. A walk
// says how many steps it takes unless STEPS says otherwise: one whose steps
// play the episode in real time takes fewer.
export async function begin({ steps: usual = 60 } = {}) {
  const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
  const seed = Number(process.env.SEED ?? Math.floor(Math.random() * 1e6));
  const steps = Number(process.env.STEPS || usual);
  const { browser, page, errors } = await open(url);
  if (process.env.HEVC) await addHEVC(url, page);
  const watch = new Watch(page, await chosen(page), errors);
  return { url, seed, steps, rng: random(seed), browser, page, watch };
}

// Adds an HEVC 10-bit episode with Add and chooses the clip its first
// search finds.
async function addHEVC(url, page) {
  await control(url, "/pick?seconds=40&codec=hevc");
  await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
  await page.waitForFunction(
    () => {
      const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
      const on = document.querySelector("aside button.episode.current .name")?.textContent ?? "";
      return (
        on.startsWith("folge-") &&
        pane?.querySelector(".listhead button.new")?.textContent.includes("New") &&
        pane.querySelectorAll("ol li[data-key]").length > 0
      );
    },
    null,
    { timeout: 120000, polling: 250 },
  );
  await page.locator("aside ol li[data-key] button.pick").first().click();
  await settle(page);
  // The frames on screen came through the Go side's streams, or this walk
  // proves nothing about them.
  const streams = await page.evaluate(() => window.__appFrames?.stats.streams ?? 0);
  if (!streams) throw new Error("the picture of the HEVC episode did not come from the Go side");
  await page.evaluate(() => document.body.focus());
}

// Walks: look is what a gesture's when and run are handed, the caption box
// unless a walk says otherwise, and show is what VERBOSE prints.
export async function walk(w, gestures, { look = shown, show } = {}) {
  const { page, rng, watch, seed, steps } = w;
  const walked = [];
  await watch.start();
  for (let step = 1; step <= steps && !watch.failed; step++) {
    const seen = await look(page);
    const box = await shown(page);
    const can = gestures.filter((g) => g.when(seen));
    const g = rng.weighted(can);
    const said = await g.run(seen);
    walked.push(`${String(step).padStart(3)}  ${said}`);
    const { now } = await watch.step(g.name, box);
    if (process.env.VERBOSE) {
      const line = show
        ? await show(page)
        : now.map((x) => (x.focused ? `[${x.text}]` : x.keyed ? `<${x.text}>` : x.text)).join(" ");
      console.log(`${walked[walked.length - 1].padEnd(36)} ${line}`);
    }
  }
  const failed = watch.failed;
  console.log(`seed ${seed}, ${walked.length} steps`);
  if (failed) {
    console.log(walked.join("\n"));
    console.log(`\nbroke: ${failed.rule}\n${failed.detail}`);
    await page.screenshot({ path: `/tmp/walk-${seed}.png` });
    console.log(`picture: /tmp/walk-${seed}.png`);
  }
  await w.browser.close();
  process.exit(failed ? 1 : 0);
}
