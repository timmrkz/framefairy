// What every walk needs: the interface opened on the bridge's episode with
// its clip chosen, a way to wait until the app has settled, and the
// engine's own answer to compare the screen with. See docs/TESTING.md.
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { chromium } from "playwright";
import { filmed as filmedList } from "./filmed.mjs";

// The Chromium the pinned Playwright was made for. CI's image holds only
// its headless shell, which a headless launch finds by itself, so there
// the full browser this names is missing and the launch is left to
// Playwright. A machine that has only another Chromium walks on that
// rather than not at all:
// a cloud session keeps the Chromium of the Playwright it came with in
// PLAYWRIGHT_BROWSERS_PATH, and after Playwright went from 1.56 to 1.63
// every walk there stopped on a browser that was not installed. Whether
// to fall back is Playwright's own answer, the launch saying the browser
// it would run is not there, since the file it would run is its headless
// shell, not the browser executablePath names. The fallback is the newest
// headless shell there, the kind CI runs, and only then the newest full
// browser, which runs another headless mode. Newer builds keep them in
// folders ending in linux64, older ones in chrome-linux, and a walk that
// looked only in the second stopped again, #490. The fallback is said
// once, so a walk that fails on it says which browser it ran in.
const builds = [
  ["chromium_headless_shell", ["chrome-headless-shell-linux64", "chrome-headless-shell"], ["chrome-linux", "headless_shell"]],
  ["chromium", ["chrome-linux64", "chrome"], ["chrome-linux", "chrome"]],
];
function fallbackBrowser() {
  const root = process.env.PLAYWRIGHT_BROWSERS_PATH;
  if (!root || !existsSync(root)) return undefined;
  const dirs = readdirSync(root);
  for (const [kind, ...places] of builds) {
    const found = dirs
      .filter((d) => new RegExp(`^${kind}-\\d+$`).test(d))
      .sort((a, b) => Number(b.split("-")[1]) - Number(a.split("-")[1]));
    for (const dir of found) {
      for (const place of places) {
        const exe = join(root, dir, ...place);
        if (existsSync(exe)) return exe;
      }
    }
  }
  return undefined;
}
let fellBack;
async function launch(args) {
  // An interrupt is the walk's own to answer, see keepTraces, not
  // Playwright's, which would close the browser under the traces.
  const options = { args, handleSIGINT: !traces };
  try {
    return await chromium.launch(options);
  } catch (e) {
    const exe = String(e?.message).includes("Executable doesn't exist") && fallbackBrowser();
    if (!exe) throw e;
    if (fellBack !== exe) {
      fellBack = exe;
      console.error(`walking on ${exe}, since the Chromium of this Playwright is not installed`);
    }
    return chromium.launch({ ...options, executablePath: exe });
  }
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

// SLOW=4 runs a page four times slower, the way a busy CI runner does,
// to find a walk that counts on a machine being quick. Every page a walk
// opens goes through it, the report window too. See docs/TESTING.md.
export async function slowDown(page) {
  const slow = Number(process.env.SLOW);
  if (slow > 1) {
    const cdp = await page.context().newCDPSession(page);
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: slow });
  }
}

// No action and no wait of Playwright's on page gives up after a time of
// its own, 30 s unless set: a wait that never ends is ended by the
// -timeout of make walks, which says which walk it was and what it said
// last. Every page a walk opens goes through it, the report window too.
export function noDeadlines(page) {
  page.setDefaultTimeout(0);
}

// A browser for the walks to open the interface in. WebGPU on, which
// headless Chromium on Linux only gives when asked, for the step that
// checks what the GPU draws, ["screen light"]. The video preview itself
// draws on a 2D canvas here, __flatScreen, because headless Chromium gives
// nothing back from a WebGPU canvas, and the walks read the frame on
// screen from the canvas. See screen.ts.
export async function newBrowser() {
  const browser = await launch(["--enable-unsafe-webgpu"]);
  if (traces) {
    const plain = browser.newPage.bind(browser);
    browser.newPage = async (options) => {
      const page = await plain(options);
      await traced(page);
      return page;
    };
  }
  if (coverage) counted(browser);
  return browser;
}

// WALKCOVERAGE names a folder, and every page a walk opens counts the
// lines of the interface it runs, which make walks COVERAGE= turns into
// a report on the components, see Coverage in docs/TESTING.md. The
// counts of a page are kept when it is closed, by keep in counts.mjs, in
// the folder monocart-coverage-reports gathers them in, one file for each
// page, so the walks running side by side never write the same file.
// Each script is named by its path alone, so the same script served by
// another bridge on another port is the same script, and its source map
// is the one the build left beside it, see preview/bridge.config.ts.
const coverage = process.env.WALKCOVERAGE;
const built = join(import.meta.dirname, "..", "dist-bridge");
const top = resolve(import.meta.dirname, "..", "..", "..");
function counted(browser) {
  const open = new Set();
  const keep = async (page) => {
    if (!open.delete(page)) return;
    let entries;
    try {
      entries = await page.coverage.stopJSCoverage();
    } catch {
      // A page that crashed has no counts to give, and its walk fails
      // on what crashed it.
      return;
    }
    const scripts = [];
    for (const entry of entries) {
      const path = new URL(entry.url).pathname;
      if (!/^\/assets\/.+\.js$/.test(path)) continue;
      const map = join(built, `${path}.map`);
      if (!existsSync(map)) continue;
      // Each source of the map named from the top of the repository,
      // frontend/src/lib/api.ts, for the report and for Codecov.
      const sourceMap = JSON.parse(readFileSync(map, "utf8"));
      sourceMap.sources = sourceMap.sources.map((s) => relative(top, resolve(dirname(map), s)));
      scripts.push({ ...entry, url: `http://bridge${path}`, sourceMap });
    }
    const { keep: keepCounts } = await import("../counts.mjs");
    await keepCounts(coverage, scripts);
  };
  const plain = browser.newPage.bind(browser);
  browser.newPage = async (options) => {
    const page = await plain(options);
    await page.coverage.startJSCoverage({ resetOnNavigation: false });
    open.add(page);
    // A page is closed with itself, with the context browser.newPage
    // made for it, which is how a sequence ends, or with the browser.
    const close = page.close.bind(page);
    page.close = async (options) => {
      await keep(page);
      return close(options);
    };
    const context = page.context();
    const closeContext = context.close.bind(context);
    context.close = async (options) => {
      await keep(page);
      return closeContext(options);
    };
    return page;
  };
  const close = browser.close.bind(browser);
  browser.close = async (options) => {
    for (const page of [...open]) await keep(page);
    return close(options);
  };
}

// WALKTRACES names a folder, and every page a walk opens is recorded in
// a Playwright trace: each call of the walk's with when it began and
// ended, what the page logged and threw, and every request to the Go
// side, kept beside it, see hearRequests. A walk that filmed.mjs
// names records a film of the screen too. A walk that passed throws its
// traces away. One that failed, threw or was stopped keeps them in the
// folder, one file for each page it had open, and says where, so a walk
// that failed in CI comes with what happened in it, and nobody has to
// make it fail again to see. TRACE=film films every walk, and
// TRACE=none records nothing. See Finding why a walk failed in
// docs/TESTING.md. make walks sets both.
const traces = process.env.WALKTRACES;
const tracingWhich = process.env.TRACE || "light";
if (!["light", "film", "none"].includes(tracingWhich)) {
  throw new Error(`TRACE is light, film or none, not ${JSON.stringify(tracingWhich)}`);
}
const tracing = new Set();
let tracedAs = "walk";
let filmingNow = false;

// The name the traces a walk keeps are filed under, its sequence or its
// script and seed, and whether it is filmed: names are what filmed.mjs
// knows it by, its sequence's name, or its script and its script with
// the seed.
export function traceAs(title, names = [title]) {
  tracedAs = title;
  filmingNow = tracingWhich === "film" || (tracingWhich === "light" && names.some((n) => n in filmedList));
}

async function traced(page) {
  if (tracingWhich === "none") return;
  const context = page.context();
  // The film of the screen made eight sequences take 15 % longer in a
  // cloud session, and every walks job in CI a fifth longer, so only the
  // walks filmed.mjs names have it. The page as it was before and after
  // every call, snapshots, 41 % longer and a walks job in CI over its
  // five minutes, no walk has.
  await context.tracing.start({ screenshots: filmingNow, snapshots: false, title: tracedAs });
  hearRequests(page);
  tracing.add(context);
  context.on("close", () => {
    tracing.delete(context);
    requestLogs.delete(context);
  });
}

// Every request the page makes, heard in Node and kept beside the
// trace. Playwright records the network into a trace only along with
// the snapshots, which no walk has, see traced. Nothing of this runs in
// the page, so the record adds nothing to what it records. A line is the milliseconds since the trace
// began, the request's number and what became of it: "ask" with the
// method and the path, the call's name and arguments for /call, "answer"
// with the status, or "failed" with why. The events the Go side sends
// come on one request to /events that stays open, so they are not in it.
const requestLogs = new Map();
function hearRequests(page) {
  // The clock only stamps the record, nothing waits on it.
  const clock = () => Date.now();
  const t0 = clock();
  const lines = [];
  const ids = new Map();
  const at = () => String(clock() - t0).padStart(7);
  page.on("request", (r) => {
    const id = ids.size + 1;
    ids.set(r, id);
    const url = new URL(r.url());
    let what = url.pathname + url.search;
    if (url.pathname === "/call") {
      try {
        const { name, args } = JSON.parse(r.postData() ?? "");
        what = `/call ${name} ${JSON.stringify(args).slice(0, 200)}`;
        // What the app logged, every line whole: cut to 200 characters, a
        // walk that hung after a play showed the play asked for and
        // nothing of what came of it.
        if (name === "main.FrameFairy.Said") what = [`/call ${name}`, ...(args[0] ?? []).map((l) => `    ${l.at} ${l.act ?? ""} ${l.text}`)].join("\n");
      } catch {
        // Said as its path alone.
      }
    }
    lines.push(`${at()}  ${id}  ask ${r.method()} ${what}`);
  });
  page.on("response", (r) => lines.push(`${at()}  ${ids.get(r.request())}  answer ${r.status()}`));
  page.on("requestfailed", (r) => lines.push(`${at()}  ${ids.get(r)}  failed ${r.failure()?.errorText ?? ""}`));
  requestLogs.set(page.context(), lines);
}

// Each step of a walk as a group of its own in the trace, named the way
// the walk prints it, so the trace reads step by step. Ends the group the
// step before left open.
export async function traceStep(page, name) {
  if (!tracing.has(page.context())) return;
  const { tracing: t } = page.context();
  if (page.__traceStep) await t.groupEnd();
  page.__traceStep = true;
  await t.group(name);
}

// Keeps the traces of every page still open and says where they are.
// Pages closed before were let go of, as a walk lets go of a report
// window once it is done with it.
export async function keepTraces() {
  if (!tracing.size) return;
  mkdirSync(traces, { recursive: true });
  const file = tracedAs.replace(/[^\p{L}\p{N}]+/gu, "-").replace(/^-|-$/g, "").slice(0, 100);
  let n = 0;
  for (const context of [...tracing]) {
    tracing.delete(context);
    const path = join(traces, `${file}${n++ ? `-${n}` : ""}.zip`);
    await context.tracing.stop({ path });
    console.log(`trace: ${path}`);
    const lines = requestLogs.get(context);
    if (lines) writeFileSync(path.replace(/\.zip$/, ".requests.txt"), lines.join("\n") + "\n");
  }
}

// A walk stopped from outside, because it hung until the time of make
// walks ran out, keeps its traces on the way out: the step it hung in
// is the one a trace is wanted for most. TestWalks stops it with an
// interrupt and waits for it to end.
if (traces) {
  process.once("SIGINT", async () => {
    await keepTraces();
    process.exit(1);
  });
}

// The interface on the bridge, in the episode's workspace with its clip
// chosen and the keyboard on the page. viewport is the size of the page,
// scale its device pixels to a CSS pixel, 2 for a Mac's screen, and again
// opens another page on the episode as it is now, beside the first, in
// its browser. browser is a browser to open it in rather than one of its
// own, see newBrowser. clock is the clock a play runs on, see
// preview/clock.ts: "frames", the walk's clock, stands on each frame
// until it is up and holds the sound card meanwhile, so every frame of a
// play is shown and read on any machine, issues #534 and #588. "sound"
// is the sound card's, as the app runs without a walk, for what a walk
// checks on the sound's own time, see soundCard in common/steps.mjs.
export async function open(url, { viewport = { width: 1500, height: 1000 }, scale = 1, again = null, browser: given = null, still = false, clock = "frames" } = {}) {
  // The episode the way the bridge found it, whatever the walk before did.
  if (!again) {
    const reset = await fetch(new URL("/reset", url), { method: "POST" });
    if (!reset.ok) throw new Error(`the bridge did not reset: ${await reset.text()}`);
  }
  const browser = again ?? given ?? (await newBrowser());
  // A page of its own in a context of its own, which browser.newPage
  // makes, so nothing a page before it kept is there.
  // A page still asks the system for less motion, the way a person can
  // in the Mac's settings, so the app leaves out what it animates and
  // the walk does not wait for it, see still in sequences.mjs.
  const page = await browser.newPage({ viewport, deviceScaleFactor: scale, reducedMotion: still ? "reduce" : "no-preference" });
  noDeadlines(page);
  await page.addInitScript((clock) => {
    window.__flatScreen = true;
    window.__playClock = clock;
  }, clock);
  await slowDown(page);
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
    await sidebarStill(page);
  }
  await page.locator("aside .list li button.pick").first().waitFor();
  await page.locator("aside .list li button.pick").first().click();
  await page.locator(".captions .word").first().waitFor();
  await settle(page);
  await page.evaluate(() => document.body.focus());
  return { browser, page, errors };
}

// Waits until the app has nothing on its way: no call out, no timer of
// the app's set to fire in a moment and nothing on the page moving for a
// moment. The page answers when the last of them ends, __settled in
// preview/settled.ts, and the walk only waits for that answer.
export async function settle(page) {
  await page.evaluate((held) => window.__settled(held), Boolean(page.__held));
}

// Holds the next call of a name the page makes on the Go side until
// letHeld, the way a busy machine delivers it late. settle does not wait
// for it while it is held, so a walk can look at what the app shows
// meanwhile. A step that holds a call lets it go before it is done, and
// the watch lets go of whatever is still held before it compares the
// screen with the engine, see Watch.step. With answer, the call reaches
// the Go side and only its answer is held, so the news of what it set
// going comes first.
export async function hold(page, name, answer = false) {
  await page.evaluate(([name, answer]) => window.__hold(name, answer), [name, answer]);
}

// Lets every call held on the page go on to the Go side. The next settle
// waits for them to come back. A hold no call came for held nothing, so
// what the walk did while it held tested nothing, and the walk fails.
export async function letHeld(page) {
  const empty = (await page.evaluate(() => window.__letHeld?.())) ?? [];
  if (empty.length) throw new Error(`a hold of ${empty.join(", ")} held nothing: no such call was made while it held`);
}

// Gives a page Playwright's own fake clock, once, in place of the app's
// timers, frames and performance.now. The walk's own waits keep the real
// frames, kept on window.__real before the fake clock comes, and again on
// every load, so settle still looks while the app's time is held. The
// clock runs as the real one does until holdClock stops it.
export async function fakeClock(page) {
  if (page.__clock) return;
  const keep = () => {
    window.__real = { frame: requestAnimationFrame.bind(window) };
  };
  await page.addInitScript(keep);
  await page.evaluate(keep);
  await page.clock.install();
  await page.evaluate(() => window.__forgetSoon?.());
  page.__clock = true;
}

// Holds the app's time where it is, so the ten seconds a toast or a
// removed row has run only as far as runClock turns them on, and a busy
// machine cannot spend them on the steps before, #341. letClockGo lets it
// run as it does without a walk, from where it was held.
//
// Playwright holds the clock only at a moment it is told, and the app's
// time runs on between reading the moment and holding it there, so now
// and then the moment read is already past and Playwright says so. It
// has held the clock by then, where the app's time had got to, since
// pauseAt holds it before it moves it on, see pauseAt in the clock
// Playwright puts in the page, and that moment is as good as the one
// read. Seen with Playwright 1.64.1: after "Cannot fast-forward to the
// past" the page's Date.now stood still. It was read again before, and
// the next reading was not behind.
export async function holdClock(page) {
  await fakeClock(page);
  try {
    await page.clock.pauseAt(await pageNow(page));
  } catch (e) {
    if (!String(e?.message).includes("fast-forward to the past")) throw e;
    // That the clock is held after the error is Playwright's own order of
    // doing things, so it is checked rather than trusted: over two of the
    // real frames the app's time must not move. An upgrade that held the
    // clock after it moved it would let the app's time run on, and every
    // walk that holds the clock would go quietly back to racing it.
    const at = await pageNow(page);
    await page.evaluate(() => new Promise((done) => window.__real.frame(() => window.__real.frame(done))));
    const after = await pageNow(page);
    if (after !== at) throw new Error(`pauseAt did not hold the clock: it went from ${at} to ${after}`);
  }
  page.__held = true;
}

// What the app's time is now, the one place a walk reads it.
const pageNow = (page) => page.evaluate(() => Date.now());

// Runs the app's held time on by ms, every timer and frame on the way
// firing in turn, and holds it again there.
export async function runClock(page, ms) {
  await page.clock.runFor(ms);
}

export async function letClockGo(page) {
  await page.clock.resume();
  page.__held = false;
}

// Waits for n of the browser's own frames, which go on while the app's
// time is held.
export async function realFrames(page, n = 2) {
  await page.evaluate((n) => {
    const frame = window.__real?.frame ?? ((f) => requestAnimationFrame(f));
    return new Promise((done) => {
      const next = (left) => (left ? frame(() => next(left - 1)) : done());
      next(n);
    });
  }, n);
}

// Waits until every row of the clip list on its way out has folded and
// left the page. Svelte marks a leaving row inert the moment it starts to
// go and takes it out when its fold ends, so the page changing is the
// signal. Its animations are no signal: Svelte starts the fold from the
// end of a first animation of no length. Two frames more let the rows
// under it settle.
export async function foldsDone(page) {
  await page.evaluate(
    () =>
      new Promise((done) => {
        const gone = () => !document.querySelector("aside ol li[inert]");
        if (gone()) return done();
        const watch = new MutationObserver(() => {
          if (gone()) {
            watch.disconnect();
            done();
          }
        });
        watch.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["inert"] });
      }),
  );
  await realFrames(page);
}

// Waits until the sidebar has stopped sliding, so what a walk reads or
// clicks next stays where it is. It opens and shuts in the frame the
// pointer reaches or leaves it, and slides for 140 ms, see App.svelte,
// which a walk once waited 400 ms for each way. A sidebar that is pinned,
// or held open while a video is dragged in the list, does not slide at
// all, and then this is two frames.
export async function sidebarStill(page) {
  await page.evaluate(async () => {
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
    const aside = document.querySelector("aside");
    while (aside?.getAnimations().length) await Promise.all(aside.getAnimations().map((a) => a.finished.catch(() => {})));
  });
}

// What the app marks as current, read in the page: the name of the
// episode the sidebar marks, the clips the interface has asked for,
// newest first, and the key of the card the clip list marks. The one
// reader of it, so episodeOn, chosen and settledScreen all read the same
// screen the same way.
function readChosen() {
  return {
    name: document.querySelector("aside button.episode.current .name")?.textContent ?? null,
    asked: [...window.__calls].reverse().filter((c) => c.name === "Clips").map((c) => c.args[0]),
    key: document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? null,
  };
}

// The episode a reading of readChosen is of, by its path in the library.
async function chosenPath(page, seen) {
  if (!seen.name) return null;
  const named = (await ask(page, "Library")).filter((e) => e.name === seen.name).map((e) => e.source);
  if (named.length < 2) return named[0] ?? null;
  // Two episodes of one file name, from two folders, look the same in the
  // sidebar, so they are told apart by the one whose clips the app asked
  // for last: the one on screen, once its clip list has come, which a
  // walk waits for before it asks.
  return seen.asked.find((p) => named.includes(p)) ?? null;
}

// The clip a reading of readChosen is of, by its plan and id as the
// engine names them.
async function chosenClip(page, seen) {
  const path = await chosenPath(page, seen);
  if (!path || !seen.key) return null;
  const entry = (await ask(page, "Clips", path)).find((c) => c.key === seen.key);
  return entry ? { path, plan: entry.plan, clip: entry.id } : null;
}

// Which episode is on screen: the one the sidebar marks as the current
// one, by its path in the library. Not the last one the interface asked
// the clips of: a video just added asks for them only later, and a walk
// then held its clip list to the episode before.
export async function episodeOn(page) {
  return chosenPath(page, await page.evaluate(readChosen));
}

// Which clip is on screen: the card the clip list marks as the current
// one, in the episode on screen. Not the last captions asked for: an
// episode just opened shows its clip before that, and a walk then held
// the clip on screen to another episode's.
export async function chosen(page) {
  return chosenClip(page, await page.evaluate(readChosen));
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
    // Each piece with its crop, the keyframes that glide it and its
    // rotation, which are steps of the history as the edges are, issue
    // #248.
    const pieces = (entry?.segments ?? []).map((s) => ({ start: s.start, end: s.end, cropX: s.cropX ?? null, marks: s.marks ?? [], rotate: s.rotate ?? 0 }));
    return { ...captions, segments: pieces };
  }, at);
}

// What the clip timeline shows: where the clip starts and ends and where
// each cut is, on the episode's clock, read off its handles.
export async function timeline(page) {
  return page.evaluate(readTimeline);
}
function readTimeline() {
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
}

// What the caption box shows: each word, the moment it stands for, and
// the word of the episode it is.
export async function shown(page) {
  return page.evaluate(readShown);
}
function readShown() {
  return [...document.querySelectorAll(".captions .word")].map((w) => ({
    text: w.textContent,
    at: Number(w.dataset.at),
    to: Number(w.dataset.to),
    keyed: w.classList.contains("keyed"),
    focused: document.activeElement === w,
  }));
}

// What the screen shows once the app has settled, read in the moment it
// settled: the clip on screen, see chosen, the caption box, see shown,
// and the clip timeline, see timeline. All of it is read in the one turn
// of the page in which the settle answers, so nothing of the app runs
// between the reads. Read one call at a time, a search that landed a
// clip between two of them changed the clip on screen under the walk,
// and the clip timeline was held to the clip before.
export async function settledScreen(page) {
  const seen = await page.evaluate(
    async ([held, marked, words, track]) => {
      const run = (src) => new Function(`return (${src})`)()();
      await window.__settled(held);
      return { ...run(marked), now: run(words), drawn: run(track) };
    },
    [Boolean(page.__held), readChosen.toString(), readShown.toString(), readTimeline.toString()],
  );
  return { at: await chosenClip(page, seen), now: seen.now, drawn: seen.drawn };
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

// One click on the clip timeline, never the second of a double-click. The
// clip timeline tells a double-click from two presses within half a
// second and six pixels of each other, the Mac's own, see twice in
// ClipTimeline.svelte, and a double-click inside a clip cuts a part out.
// A walk that clicks where it clicked a step before used to be half a
// second apart only because it waited on the clock between steps. Now
// the hand lands eight pixels beside the last spot first, on the side
// away from the new one, as a hand that misses by a little does, so the
// click is one on its own however soon it comes.
export async function clickOnTrack(page, x, y) {
  await notTwice(page, x, y);
  await page.mouse.click(x, y);
}

// The hand taking hold of something on the clip timeline at x, y, the
// press of a drag, never the second of a double-click either: a keyframe
// put on the spot just clicked is taken hold of where that click was, and
// a double-click on a keyframe removes it. The walk used to be half a
// second apart there only because it waited on the app's animations.
export async function pressOnTrack(page, x, y) {
  await notTwice(page, x, y);
  await page.mouse.move(x, y);
  await page.mouse.down();
}

// Lands the hand eight pixels beside the last spot pressed on the clip
// timeline first, when x is within six pixels of it, see clickOnTrack.
// The clip timeline measures only along itself, so a press under or over
// the last one at the same x is a double-click too.
async function notTwice(page, x, y) {
  const last = page.__pressed;
  if (last && Math.abs(last.x - x) < 6) {
    await page.mouse.click(last.x + (x < last.x ? 8 : -8), y);
  }
  page.__pressed = { x, y };
}

// Waits for the page to draw n frames, so whatever the walk did last has
// reached the screen however busy the machine is.
export async function nextPaint(page, n = 1) {
  await page.evaluate(
    (n) =>
      new Promise((r) => {
        const next = (left) => (left ? requestAnimationFrame(() => next(left - 1)) : r());
        next(n);
      }),
    n,
  );
}

// Waits for the video preview to show the frame that holds the playhead,
// by where the app says the frame shown begins, and with captions for
// that frame to be drawn with its own captions over it, see data-captioned
// in Player.svelte. It says true then, and false when a layer of captions
// is given up on first, data-captions-lost, counted from lost, the count
// read before the gesture, since a layer can be given up on while the
// walk settles after it. T and C put a mark on the frame shown, the frame
// a person sees, and the caption box shows what was drawn on it, and on a
// busy machine both come a moment after the playhead. It waits on the
// page changing, never on the clock, so a frame that never comes is ended
// by the -timeout of make walks.
export async function shownOnPlayhead(page, { captions = false, lost } = {}) {
  const { fps, videoStart = 0 } = await ask(page, "Source", await episodeOn(page));
  return page.locator(".screen").evaluate(
    (screen, [fps, start, captions, lost]) =>
      new Promise((done) => {
        const since = lost ?? screen.dataset.captionsLost ?? "";
        const gone = () => captions && (screen.dataset.captionsLost ?? "") !== since;
        const up = () => {
          const at = Number(screen.dataset.playhead);
          const shown = Number(screen.dataset.shown);
          // The frame that holds the playhead, one that begins a
          // millisecond or less after it included, see frameAt in
          // lib/flow.ts.
          const begins = start + Math.floor((at - start + 0.001) * fps) / fps;
          if (!Number.isFinite(shown) || Math.abs(shown - begins) >= 0.5 / fps) return false;
          return !captions || (screen.dataset.captioned !== undefined && Number(screen.dataset.captioned) === shown);
        };
        if (up()) return done(true);
        if (gone()) return done(false);
        const seen = new MutationObserver(() => {
          const came = up();
          if (!came && !gone()) return;
          seen.disconnect();
          done(came);
        });
        seen.observe(screen, {
          attributes: true,
          attributeFilter: ["data-playhead", "data-shown", "data-captioned", "data-captions-lost"],
        });
      }),
    [fps, videoStart, captions, lost],
  );
}

// Waits, while a play goes on, until the playhead the video preview says
// is past the moment past, or the play has stopped, or the playhead has
// gone on by most seconds of the episode without stopping, whichever
// comes first, and says where it stood, whether it still plays and how
// far it went. A jump over a cut counts as far as it jumps, and a play
// that loops back to the clip's start counts on from there. Each is read
// when the page changes, the playhead or the Play button coming back,
// never on the clock, and most is counted in the episode's own time, so
// a busy machine plays as far as a quiet one, only slower. The way goes
// from the moment from, the playhead now unless it is given. With pause,
// a play still going is paused in the frame the wait ends, inside the
// page: pressed a trip later, the play went on further the busier the
// machine, and a clip that reached its end in between would play again
// from its start.
export async function playUntil(page, { past = Infinity, most = Infinity, from = NaN, pause = false } = {}) {
  return page.evaluate(
    ([past, most, from, pause]) =>
      new Promise((done) => {
        let last = Number.isFinite(from) ? from : Number(document.querySelector(".screen")?.dataset.playhead);
        let gone = 0;
        const look = () => {
          const at = Number(document.querySelector(".screen")?.dataset.playhead);
          const playing = !document.querySelector('button[aria-label="Play"]');
          if (Number.isFinite(at) && at > last) gone += at - last;
          last = at;
          if (playing && !(at > past) && gone < most) return null;
          if (playing && pause) {
            const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
            document.body.dispatchEvent(new KeyboardEvent("keydown", key));
            document.body.dispatchEvent(new KeyboardEvent("keyup", key));
          }
          return { at, playing, gone };
        };
        const now = look();
        if (now) return done(now);
        const watch = new MutationObserver(() => {
          const now = look();
          if (!now) return;
          watch.disconnect();
          done(now);
        });
        watch.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ["data-playhead", "aria-label"] });
      }),
    [past === Infinity ? Number.MAX_VALUE : past, most === Infinity ? Number.MAX_VALUE : most, from, pause],
  );
}

// Holds the sound card, or lets it go. Held, a play stands where it is,
// picture and sound together, while a walk's hand takes as long as the
// machine needs, and let go it plays on from there, issue #583, once the
// walk's clock holds it no longer either, see holdSound in
// preview/clock.ts.
export async function holdSound(page, held) {
  await page.evaluate((held) => window.__holdSound(held), held);
}

// Plays from the playhead until it has gone on by seconds of the episode,
// or until the play has stopped by itself at the clip's end, and pauses it
// there, see playUntil. Says false, and plays nothing, when the space bar
// did not start a play, once the app has settled on the press.
export async function playFor(page, seconds) {
  const from = await page.evaluate(() => document.querySelector(".screen")?.dataset.playhead ?? null);
  await page.keyboard.press("Space");
  await settle(page);
  const now = await page.evaluate(() => ({
    playing: !document.querySelector('button[aria-label="Play"]'),
    at: document.querySelector(".screen")?.dataset.playhead ?? null,
  }));
  if (!now.playing) return now.at !== from;
  await playUntil(page, { most: seconds, from: Number(from), pause: true });
  return true;
}

// How many times the Go side has told the interface something of a name,
// "job" above all, counted from the first time a walk asks, so it can wait
// for the next. The bridge sends every event to every page listening, so
// the walk listens beside the app and takes nothing from it.
// The first ask waits until the walk is listening, so nothing the Go side
// says after it is missed.
export async function news(page, name = "job") {
  return page.evaluate(async (name) => {
    if (!window.__news) {
      window.__news = {};
      window.__newsWaiting = [];
      const source = new EventSource("/events");
      source.onmessage = (m) => {
        const ev = JSON.parse(m.data);
        window.__news[ev.name] = (window.__news[ev.name] ?? 0) + 1;
        const now = window.__newsWaiting.filter((w) => w.name === ev.name && window.__news[ev.name] > w.seen);
        window.__newsWaiting = window.__newsWaiting.filter((w) => !now.includes(w));
        for (const w of now) w.done();
      };
      window.__listening = new Promise((r) => (source.onopen = r));
    }
    await window.__listening;
    return window.__news[name] ?? 0;
  }, name);
}

// Waits until the interface has heard everything the Go side told it up
// to now, by a mark the bridge sends behind it, see /mark in
// bridge_test.go, and the newest change of every job in jobs, a list the
// engine gave, by the job's seq. Then until the app has settled on it.
// What the engine said and sent before this is then what the screen
// shows, however late a busy machine delivered it. The seq is waited for
// too because the Go side sends a job's news after it lets go of the
// queue, so a list of jobs can show a change whose news the mark
// overtook. The mark is asked for once the interface is listening, so
// after a reload too it is not sent to nobody.
export async function caughtUp(page, url, jobs = []) {
  await page.evaluate(() => window.__listening);
  const mark = await control(url, "/mark");
  const due = jobs.map((j) => [j.id, j.seq]);
  await page.evaluate(([mark, due]) => window.__heardMark(mark).then(() => window.__heardSeqs(due)), [mark, due]);
  await settle(page);
}

// Waits until the Go side has told the interface something of a name more
// often than seen, a count news gave, and says how often now. It waits
// for good: news that never comes is a walk that hangs, ended by the
// -timeout of make walks with the step it was in printed, see walk.mjs.
export async function nextNews(page, seen, name = "job") {
  await page.evaluate(
    ([name, seen]) =>
      new Promise((done) => ((window.__news[name] ?? 0) > seen ? done() : window.__newsWaiting.push({ name, seen, done }))),
    [name, seen],
  );
  return news(page, name);
}

// A drag of the pointer from a point, by dx, in steps the way a hand
// moves, with shift held when asked. Each step is a frame of its own, the
// way a pointer moves on screen. A step a fixed 30 ms after the last can
// come two to a frame on a busy machine, and Chromium then hands the app
// only the last of them.
export async function drag(page, x, y, dx, shift) {
  if (shift) await page.keyboard.down("Shift");
  await page.mouse.move(x, y);
  await page.mouse.down();
  const n = 6;
  for (let i = 1; i <= n; i++) {
    await page.mouse.move(x + (dx * i) / n, y);
    await nextPaint(page);
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
export const BITS = 10;
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
// one already on screen is seen to have started. And on every animation
// frame, what lies over the picture, see overlay, with the number of the
// frame on screen under it, pictured, or -1 while there is none.
export async function recordFrames(page, frame) {
  await reader(page);
  await overlayReader(page);
  await page.evaluate(() => {
    const screen = document.querySelector(".screen");
    const from = screen.dataset.playhead;
    let last = window.__pictured();
    window.__frames = [];
    window.__overlays = [];
    window.__moved = false;
    window.__recording = true;
    const watch = () => {
      if (!window.__recording) return;
      const n = window.__pictured();
      if (n >= 0 && n !== last) window.__frames.push(n);
      last = n;
      if (screen.dataset.playhead !== from) window.__moved = true;
      window.__overlays.push({ ...window.__overlay(), pictured: n });
      requestAnimationFrame(watch);
    };
    requestAnimationFrame(watch);
  });
  return async () => {
    const r = await page.evaluate(() => {
      window.__recording = false;
      return { frames: window.__frames, moved: window.__moved, overlays: window.__overlays };
    });
    return { frames: r.frames.map((n) => n * frame), moved: r.moved, overlays: r.overlays };
  };
}

// Puts the overlay reader on the page once: where the playhead is, whether
// it plays, whether the clip timeline says it is on the video, by the
// chosen clip dimmed there or no clip chosen at all, which card of the
// clip list is chosen, a clip on its way included, and what the video
// preview lays over the picture: the crop frame, the shade beside it and
// the caption box, and whether the captions over the picture wait for the
// layer of its frame, late.
async function overlayReader(page) {
  await page.evaluate(() => {
    if (window.__overlay) return;
    window.__overlay = () => {
      const screen = document.querySelector(".screen");
      const chosen = document.querySelector(".clip-timeline .chosen");
      return {
        at: Number(screen?.dataset.playhead),
        playing: !!document.querySelector('button[aria-label="Pause"]'),
        video: !chosen || chosen.classList.contains("dim"),
        card:
          document.querySelector("aside ol li[data-key].current, aside ol li[data-key] button.pick.current")?.closest("li")
            ?.dataset.key ?? "",
        crop: !!document.querySelector(".atop .frame"),
        shade: !!screen?.querySelector(".crop"),
        captions: !!screen?.querySelector(".captions"),
        late: !!window.__captionLayers?.late,
        t: performance.now(),
      };
    };
  });
}

// What lies over the picture of the video preview now, as recordFrames
// records it on every animation frame.
export async function overlay(page) {
  await overlayReader(page);
  return page.evaluate(() => window.__overlay());
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
      head: text(pane.querySelector(".listhead button.search")),
      count: Number(text(pane.querySelector(".listhead .num")) || 0),
      cards: [...pane.querySelectorAll("ol li[data-key]")].map((li) => li.dataset.key),
      rows: [...pane.querySelectorAll("ol li.ghost")]
        .map((li) => ({ what: text(li.querySelector(".title")), meta: text(li.querySelector(".meta")), leaving: li.inert }))
        .filter((r) => r.what),
    };
  });
}

// Asks the bridge itself for something a walk needs and the interface
// cannot do: a file for the Add button's box, the model holding or
// failing its answers, the speech model held or let on, the app opened
// again.
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
        const button = pane.querySelector(".listhead button.search");
        const pressed = text(button);
        button.click();
        requestAnimationFrame(() => {
          done({
            pressed,
            head: text(pane.querySelector(".listhead button.search")),
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
  await sidebarStill(page);
  await click();
  await page.mouse.move(750, 400);
  await sidebarStill(page);
  // An episode picked is built behind the one on screen and takes its
  // place once it is ready, see App.svelte. Until then there are two, and
  // whatever a walk reads next is about the one it picked.
  await page.evaluate(
    () =>
      new Promise((done) => {
        const one = () => document.querySelectorAll(".workspace").length <= 1 && !document.querySelector(".workspace.behind");
        if (one()) return done();
        const watch = new MutationObserver(() => {
          if (!one()) return;
          watch.disconnect();
          done();
        });
        watch.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["class"] });
      }),
  );
}

// Until fn, run in the page with arg, says true. It is asked again each
// time the page changes, an attribute, a node or a text, never on the
// clock, so what it waits for has to show on the page. One that never
// comes is a walk that hangs, ended by the -timeout of make walks with
// the step the walk printed last. seenUntil is the same wait for a read
// the walk makes itself.
export function pageUntil(page, fn, arg) {
  return page.evaluate(
    ([source, arg]) =>
      new Promise((done) => {
        const holds = new Function(`return (${source})`)();
        if (holds(arg)) return done();
        const o = new MutationObserver(() => {
          if (!holds(arg)) return;
          o.disconnect();
          done();
        });
        o.observe(document, { subtree: true, childList: true, characterData: true, attributes: true });
      }),
    [fn.toString(), arg],
  );
}

// Reads read until ok says it is what was wanted, and says what it read
// last: once the interface has heard everything the Go side told it and
// has settled on it, see caughtUp, and then each time the page's body
// changes, never on the clock. What the app shows a moment later by
// itself, like the Updates page holding its looking for 1.4 s, comes that
// way. read must change nothing in the body, or it wakes itself. A page
// that never comes to it hangs the walk, as pageUntil does.
export async function seenUntil(page, url, read, ok) {
  await caughtUp(page, url);
  for (;;) {
    const changed = page.evaluate(
      () =>
        new Promise((done) => {
          const o = new MutationObserver(() => {
            o.disconnect();
            done();
          });
          o.observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true });
        }),
    );
    // Left waiting once ok is met, so it must not throw when the page
    // closes.
    changed.catch(() => {});
    const now = await read();
    if (ok(now)) return now;
    await changed;
  }
}

// Adds an HEVC 10-bit episode with Add and chooses the clip its first
// search finds.
export async function addHEVC(url, page) {
  await control(url, "/pick?seconds=40&codec=hevc");
  await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
  await pageUntil(page, () => {
    const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
    const on = document.querySelector("aside button.episode.current .name")?.textContent ?? "";
    return (
      on.startsWith("folge-") &&
      pane?.querySelector(".listhead button.search")?.textContent.includes("Search") &&
      pane.querySelectorAll("ol li[data-key]").length > 0
    );
  });
  await page.locator("aside ol li[data-key] button.pick").first().click();
  await settle(page);
  // The frames on screen came through the Go side's streams, or this walk
  // proves nothing about them.
  const streams = await page.evaluate(() => window.__appFrames?.stats.streams ?? 0);
  if (!streams) throw new Error("the picture of the HEVC episode did not come from the Go side");
  if (process.env.VERBOSE) console.log("frames read in the", await page.evaluate(() => window.__appFrames.puller.where));
  await page.evaluate(() => document.body.focus());
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
// the frame that holds it, see shownOnPlayhead. Says whether the picture
// is that frame by the number drawn on it, at fps frames a second.
//
// The click is never the second of a double-click, see clickOnTrack. Two
// moments of one short word lie a few pixels apart, and the two clicks
// on them, under half a second apart on a quick run, cut a part out of
// the clip: the captions walk then compared the clip with the cut to the
// short made before it, and failed now and then, issue #560.
export async function seekTo(page, at, fps) {
  const g = await handles(page);
  await clickOnTrack(page, await xOf(page, at), high(g.track));
  await settle(page);
  await reader(page);
  await shownOnPlayhead(page);
  return page.evaluate((fps) => {
    const at = Number(document.querySelector(".screen").dataset.playhead);
    // The frame that holds the playhead, a frame that begins a
    // millisecond or less after it included, see frameAt in lib/flow.ts.
    return window.__pictured() === Math.floor((at + 0.001) * fps);
  }, fps);
}

// The crop frame in the video preview, where the playhead is: where it
// stands, as shares of the picture, and what the picture shows inside it,
// read off the canvas under it. Null while the playhead is outside the
// clip and there is no frame.
export async function cropFrame(page) {
  const got = await page.evaluate(() => {
    const frame = document.querySelector(".atop .frame");
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
    const { data } = t.getImageData(0, 0, x1 - x0, canvas.height);
    return { left, width, w: x1 - x0, h: canvas.height, pixels: new Uint8Array(data.buffer).toBase64() };
  });
  if (!got) return null;
  return { left: got.left, width: got.width, look: lookOf(bytes(got.pixels), got.w, got.h) };
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

// The crop in the video preview as the canvas has it, at the canvas's own
// pixels, where the captions are drawn: from the crop's left edge in
// pixels of the picture, data-crop, and the episode's width and the
// crop's, source, as the video preview works it out, and its pixels, red,
// green, blue and alpha, row after row. Null while there is no crop.
export async function cropPixels(page, source) {
  const got = await page.evaluate(({ width, cropWidth }) => {
    const left = Number(document.querySelector(".screen").dataset.crop);
    const canvas = document.querySelector(".screen canvas");
    if (!Number.isFinite(left) || !canvas?.width) return null;
    const x = Math.round((left / width) * canvas.width);
    const w = Math.round((cropWidth / width) * canvas.width);
    const h = canvas.height;
    // The canvas may be a WebGPU canvas, which has no 2D context, so the
    // piece is copied to a 2D canvas to be read.
    const s = document.createElement("canvas");
    s.width = w;
    s.height = h;
    const t = s.getContext("2d", { willReadFrequently: true });
    t.drawImage(canvas, x, 0, w, h, 0, 0, w, h);
    const { data } = t.getImageData(0, 0, w, h);
    return { x, w, h, pixels: new Uint8Array(data.buffer).toBase64() };
  }, source);
  return got && { ...got, pixels: bytes(got.pixels) };
}

// The pixels a page read off a canvas, which it hands over as base64
// text. As an array of numbers, [...data], each is a number in the JSON
// both sides write and read, and a picture the size of a short took most
// of the 64 s of the longest sequence, issue #525.
function bytes(base64) {
  return Buffer.from(base64, "base64");
}

// One frame of a short read back from disk with ffmpeg, the nth from
// nought, at width by height, red, green and blue, row after row.
export function shortFrame(path, n, w, h) {
  return execFileSync("ffmpeg", ["-v", "error", "-i", path, "-vf", `select=eq(n\\,${n}),scale=${w}:${h}:flags=area`,
    "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-"], { maxBuffer: 1 << 28 });
}

// The frames of a short from the nth to the mth, both counted, read back
// with ffmpeg in one go, each the way shortFrame gives one. ffmpeg decodes
// a short from its start to reach a frame, so the frames either side of
// one cost nothing more read with it, and three times as much read alone.
export function shortFramesFrom(path, n, m, w, h) {
  const raw = execFileSync("ffmpeg", ["-v", "error", "-i", path, "-vf", `select=between(n\\,${n}\\,${m}),scale=${w}:${h}:flags=area`,
    "-fps_mode", "passthrough", "-frames:v", String(m - n + 1), "-f", "rawvideo", "-pix_fmt", "rgb24", "-"], { maxBuffer: 1 << 28 });
  const size = w * h * 3;
  const out = [];
  for (let at = 0; at + size <= raw.length; at += size) out.push(raw.subarray(at, at + size));
  return out;
}
