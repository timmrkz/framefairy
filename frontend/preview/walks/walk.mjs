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
import { random, open, chosen, shown, addHEVC, recordCover } from "./bridge.mjs";
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

// Walks: look is what a gesture's when and run are handed, the caption box
// unless a walk says otherwise, and show is what VERBOSE prints. first is
// the gestures made before the seed picks any, each as its name and what
// its run is handed after look, for what a walk checks on every push
// whatever its seed draws. They take no random number, so the seed still
// decides every step after them, the same way on every run. They leave
// the app in another state than the walk began in, so a seed walks
// another path after them than it did before a walk had them. One that
// cannot be made in the state it finds breaks a rule, rather than run
// where it was never meant to.
//
// Each step is printed as it begins, with the seed, so a walk that waits
// for good on something that never comes says which step it hung in when
// the -timeout of make walks ends it. A step that throws ends the walk as
// a broken rule, with the seed and the steps.
export async function walk(w, gestures, { look = shown, show, first = [] } = {}) {
  const { page, rng, watch, seed, steps } = w;
  const walked = [];
  const firsts = first.map(([name, ...args]) => {
    const g = gestures.find((x) => x.name === name);
    if (!g) throw new Error(`no gesture ${name} to make first`);
    return { g, args };
  });
  await watch.start();
  for (let step = 1; step <= firsts.length + steps && !watch.failed; step++) {
    try {
      const seen = await look(page);
      const box = await shown(page);
      const { g, args } = firsts[step - 1] ?? { g: rng.weighted(gestures.filter((g) => g.when(seen))), args: [] };
      console.log(`seed ${seed}, step ${step}: ${g.name}`);
      if (!g.when(seen)) {
        watch.broke("a gesture made first can be made", `step ${step}, ${g.name}`);
        break;
      }
      const said = await g.run(seen, ...args);
      walked.push(`${String(step).padStart(3)}  ${said}`);
      const { now } = await watch.step(g.name, box);
      if (process.env.VERBOSE) {
        const line = show
          ? await show(page)
          : now.map((x) => (x.focused ? `[${x.text}]` : x.keyed ? `<${x.text}>` : x.text)).join(" ");
        console.log(`${walked[walked.length - 1].padEnd(36)} ${line}`);
      }
    } catch (e) {
      watch.broke("a step ends", `step ${step} threw: ${e?.stack ?? e}`);
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
  await recordCover(w.browser);
  await w.browser.close();
  process.exit(failed ? 1 : 0);
}
