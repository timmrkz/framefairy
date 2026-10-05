// The loop every walk runs: look, pick a gesture that can be made now, make
// it, and have the watch check every rule. A walk file only says what its
// gestures are. See docs/TESTING.md.
//
//   SEED=12 STEPS=80 BRIDGE_URL=http://127.0.0.1:8123/ node <walk>.mjs
//
// VERBOSE=1 prints what the walk saw after every step.
import { random, open, chosen, shown } from "./bridge.mjs";
import { Watch } from "./rules.mjs";

// The page on the bridge, the seed's random numbers, and the watch.
export async function begin() {
  const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
  const seed = Number(process.env.SEED ?? Math.floor(Math.random() * 1e6));
  const steps = Number(process.env.STEPS ?? 60);
  const { browser, page, errors } = await open(url);
  const watch = new Watch(page, await chosen(page), errors);
  return { seed, steps, rng: random(seed), browser, page, watch };
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
