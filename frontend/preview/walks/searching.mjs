// A walk over finding clips: adding a video, its first search starting by
// itself, New, Cancel, Continue, the app closed and opened again, with the
// language model holding its answers or failing and the speech model slow
// or quick, in an order a seed decides. Besides the rules every walk
// keeps, see rules.mjs, the clip list says after every step what the
// engine's jobs are doing. See docs/TESTING.md.
//
//   SEED=12 STEPS=30 BRIDGE_URL=http://127.0.0.1:8123/ node searching.mjs
import { begin, walk } from "./walk.mjs";
import { ask, clipList, control, settle, pressHead as pressHeadOn, fromSidebar } from "./bridge.mjs";

const w = await begin({ steps: 30 });
const { page, rng, watch, url } = w;

// How the model and the speech are set, which decides what may be
// expected of a search.
const model = { hang: false, fail: false };
let pace = 0;
const setModel = () => control(url, `/model?hang=${model.hang ? 1 : 0}&fail=${model.fail ? 1 : 0}`);

// The episode on screen: the one the interface last asked the clips of.
const episodeOn = () =>
  page.evaluate(() => [...window.__calls].reverse().find((c) => c.name === "Clips")?.args[0] ?? null);

// The last search of an episode, as the engine has it.
async function searchOf(path) {
  const jobs = (await ask(page, "Jobs")).filter((j) => j.episode === path && j.kind === "search");
  return jobs[jobs.length - 1] ?? null;
}

// What the clip list should say for the episode's last search: its head
// button, and the row a search that stopped leaves.
function expected(job) {
  if (job && (job.state === "running" || job.state === "queued")) return { head: ["Cancel", "Cancelling"] };
  if (job && job.state === "failed") return { head: ["Continue"], row: "Failed. Click Continue" };
  if (job && job.state === "interrupted") {
    return { head: ["Continue"], row: job.step === "stopped" ? "Stopped. Click Continue" : "Interrupted. Click Continue" };
  }
  return { head: ["New"] };
}

// The clip list and the engine agree: the head button, the row of a
// search that stopped, no word of a failure that did not happen, and at
// rest as many cards as the engine has clips. The rows are held a moment
// so they can be read, so this is asked again for a few seconds before it
// is a broken rule.
async function agree() {
  const path = await episodeOn();
  if (!path) return null;
  let problem = null;
  for (let tries = 0; tries < 24; tries++) {
    const [list, job] = [await clipList(page), await searchOf(path)];
    const want = expected(job);
    problem = null;
    if (!list) problem = "there is no clip list";
    else if (!want.head.includes(list.head)) problem = `the head says ${list.head}, the search is ${job?.state ?? "none"}${job?.step ? ` (${job.step})` : ""}`;
    else if (want.row && !list.rows.some((r) => r.what === want.row)) problem = `no row says "${want.row}": ${JSON.stringify(list.rows)}`;
    else if (job?.state !== "failed" && list.rows.some((r) => /fail/i.test(r.what))) problem = `a row says a failure the engine did not have: ${JSON.stringify(list.rows)}`;
    else if (list.head === "New") {
      const clips = (await ask(page, "Clips", path)).filter((c) => !c.rejected);
      if (list.cards.length !== clips.length || list.count !== clips.length) {
        problem = `${list.cards.length} cards and the count ${list.count}, the engine has ${clips.length} clips`;
      }
    }
    if (!problem) return null;
    await page.waitForTimeout(250);
  }
  return problem;
}

const head = async () => (await clipList(page))?.head;
const pressHead = () => pressHeadOn(page);
const sidebar = (click) => fromSidebar(page, click);

const gestures = [
  {
    name: "add",
    weight: 2,
    when: (s) => s.episodes < 3,
    async run() {
      const seconds = 40 + rng.int(80);
      await control(url, `/pick?seconds=${seconds}`);
      await sidebar(() => page.locator("aside").getByText("Add", { exact: true }).first().click());
      // A new video is transcribed and its first search starts by itself.
      // With the model answering and the speech quick, the clips come
      // without a click. With the speech slow the walk goes on while it is
      // transcribed, so the next steps land in the middle of it.
      if (!model.hang && !model.fail && pace === 0) {
        const came = await page
          .waitForFunction(
            () => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              return pane?.querySelector(".listhead button.new")?.textContent.includes("New") &&
                pane.querySelectorAll("ol li[data-key]").length > 0;
            },
            null,
            { timeout: 60000, polling: 250 },
          )
          .then(() => true, () => false);
        if (!came) watch.broke("a new video gets its first clips by itself", `added ${seconds} s and no clip came in a minute`);
      }
      return `add a video of ${seconds} s`;
    },
  },
  {
    name: "new",
    weight: 2,
    when: (s) => s.head === "New",
    async run() {
      await pressHead();
      return "New";
    },
  },
  {
    name: "cancel",
    weight: 3,
    when: (s) => s.head === "Cancel",
    async run() {
      const seen = await pressHead();
      // A click shows at once: in the frame after the click the button no
      // longer says Cancel, or a row says Stopping. A search that stops
      // within that frame already says Continue, which shows it too.
      if (seen.head === "Cancel" && !seen.rows.includes("Stopping")) {
        watch.broke("Cancel shows at once", `the frame after the click: ${JSON.stringify(seen)}`);
      }
      return "Cancel";
    },
  },
  {
    name: "continue",
    weight: 2,
    when: (s) => s.head === "Continue",
    async run() {
      await pressHead();
      return "Continue";
    },
  },
  {
    name: "hold",
    weight: 1,
    when: () => !model.hang,
    async run() {
      model.hang = true;
      await setModel();
      return "the model holds its answers";
    },
  },
  {
    name: "release",
    weight: 2,
    when: () => model.hang,
    async run() {
      model.hang = false;
      await setModel();
      return "the model answers";
    },
  },
  {
    name: "fail",
    weight: 1,
    when: () => !model.fail,
    async run() {
      model.fail = true;
      await setModel();
      return "the model fails";
    },
  },
  {
    name: "mend",
    weight: 2,
    when: () => model.fail,
    async run() {
      model.fail = false;
      await setModel();
      return "the model works again";
    },
  },
  {
    name: "pace",
    weight: 2,
    when: () => true,
    async run() {
      const ms = rng.pick([0, 0, 900, 2500]);
      pace = ms;
      await control(url, `/speech?ms=${ms}`);
      return `speech takes ${ms} ms a piece`;
    },
  },
  {
    name: "restart",
    weight: 1,
    when: () => true,
    async run(s) {
      const path = await episodeOn();
      const before = s.head;
      await control(url, "/reopen");
      await page.reload();
      await page.waitForTimeout(1200);
      const name = path.split("/").pop();
      await sidebar(() => page.locator("aside li", { hasText: name }).first().click());
      await settle(page);
      // How the work ended is still there after the app was closed: a
      // search that was running was cut off, one that stopped stopped.
      const after = await head();
      const fine =
        (before === "New" && after === "New") ||
        (before !== "New" && after === "Continue") ||
        (before === "Continue" && after === "Continue");
      if (!fine) watch.broke("how work ended stays after a restart", `before ${before}, after ${after}`);
      return `close the app and open it again on ${name}`;
    },
  },
  {
    name: "switch",
    weight: 1,
    when: (s) => s.episodes > 1,
    async run(s) {
      const name = rng.pick(s.names.filter((n) => n !== s.on));
      await sidebar(() => page.locator("aside li", { hasText: name }).first().click());
      return `open ${name}`;
    },
  },
  {
    name: "wait",
    weight: 2,
    when: () => true,
    async run() {
      const ms = 500 + rng.int(3000);
      await page.waitForTimeout(ms);
      return `wait ${ms} ms`;
    },
  },
];

// What a gesture needs to know: the head button, the episodes in the
// library and the one on screen.
async function look() {
  const list = await clipList(page);
  const library = await ask(page, "Library");
  const names = (library ?? []).map((e) => e.source.split("/").pop());
  const on = ((await episodeOn()) ?? "").split("/").pop();
  return { head: list?.head, episodes: names.length, names, on };
}

// Every step ends with the clip list and the engine agreeing.
for (const g of gestures) {
  const run = g.run;
  g.run = async (s) => {
    const said = await run(s);
    const problem = await agree();
    if (problem) watch.broke("the clip list says what the engine's work is doing", problem);
    return said;
  };
}

await walk(w, gestures, {
  look,
  async show() {
    const list = await clipList(page);
    return `${list?.head} ${list?.count} ${list?.rows.map((r) => `[${r.what}]`).join(" ")} model ${model.hang ? "holds" : model.fail ? "fails" : "answers"}`;
  },
});
