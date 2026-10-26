// Named sequences of steps with what has to come of them: the cases found
// by hand, kept so they stay fixed. Every step is also checked against the
// rules every walk is, see rules.mjs. See docs/TESTING.md.
//
//   BRIDGE_URL=http://127.0.0.1:8123/ node sequences.mjs [part of a name]
//
// A step is a list, its name first, and is something to do or something
// that has to come of what was done. The sequences and their steps are kept
// in a folder for each part of the app: captions, timeline, playback,
// clips, search, videos, report and app, each with its sequences.mjs and
// its steps.mjs, where every step is said. The steps every folder takes are
// in common/steps.mjs. This file walks them.
//
// The episode is the bridge's, so the words are the sentences its speech
// stand-in says: "Ich war vielleicht sechs Jahre alt, als mich auf dem
// Schulhof irgendein Typ geschubst hat."
import { chosen, newBrowser, open, settle, shown, shownOnPlayhead, waitsOn } from "./bridge.mjs";
import { moving, still, url } from "./common/steps.mjs";
import { Watch } from "./rules.mjs";
import captions from "./captions/sequences.mjs";
import timeline from "./timeline/sequences.mjs";
import playback from "./playback/sequences.mjs";
import clips from "./clips/sequences.mjs";
import search from "./search/sequences.mjs";
import videos from "./videos/sequences.mjs";
import report from "./report/sequences.mjs";
import app from "./app/sequences.mjs";
import { steps as captionsSteps } from "./captions/steps.mjs";
import { steps as timelineSteps } from "./timeline/steps.mjs";
import { steps as playbackSteps } from "./playback/steps.mjs";
import { steps as clipsSteps } from "./clips/steps.mjs";
import { steps as searchSteps } from "./search/steps.mjs";
import { steps as videosSteps } from "./videos/steps.mjs";
import { steps as reportSteps } from "./report/steps.mjs";
import { steps as appSteps } from "./app/steps.mjs";
import { steps as commonSteps } from "./common/steps.mjs";

// Every sequence, in the order of the folders they are kept in.
export const sequences = [captions, timeline, playback, clips, search, videos, report, app].flat();

// Every step by its name, from every folder. A name in two folders would
// walk whichever came last, so it is named by --names and fails.
const everyStep = {};
const stepTwice = [];
for (const own of [captionsSteps, timelineSteps, playbackSteps, clipsSteps, searchSteps, videosSteps, reportSteps, appSteps, commonSteps]) {
  for (const [name, step] of Object.entries(own)) {
    if (everyStep[name]) stepTwice.push(name);
    everyStep[name] = step;
  }
}


const only = process.argv[2] ?? "";
let failures = 0;

// --names lists the sequences, one name a line, for make walks to hand
// out one at a time to as many bridges as it runs, see TestWalks. SEQUENCE
// walks the one of that name and no other, where a part of a name could
// be a part of two.
if (only === "--names") {
  const names = sequences.map((q) => q.name);
  const twice = names.filter((n, i) => names.indexOf(n) !== i);
  if (twice.length) {
    console.log(`two sequences are called ${JSON.stringify(twice[0])}`);
    process.exit(1);
  }
  // Two steps of one name are the same mistake, and a quieter one: the
  // walks that meant the one walked over check nothing. Two pull requests
  // that each add a step can pick the same name, in one folder or in two.
  if (stepTwice.length) {
    console.log(`two steps are called ${JSON.stringify(stepTwice[0])}`);
    process.exit(1);
  }
  // A step renamed and left in moving by its old name would be walked
  // still, and check motion it never sees.
  const noStep = [...moving].filter((n) => !everyStep[n]);
  if (noStep.length) {
    console.log(`moving names ${JSON.stringify(noStep[0])}, which is no step`);
    process.exit(1);
  }
  console.log(names.join("\n"));
  process.exit(0);
}

// --serve walks one sequence after another in one browser, each named on
// a line of its own on standard input as JSON, for TestWalks to hand one
// walker its sequences without starting node, Playwright and Chromium
// again for each, 1.45 s and 1.2 s of CPU a sequence, issue #526. Each
// still starts with /reset and pages of its own, so it walks the same as
// alone. What a sequence prints begins with a line of its own, walking,
// see walkingLine, so what the node says between two sequences is told
// from either. It ends in a line that starts with walked, see walkedLine,
// then "ok" or "FAIL", or "threw" when it threw and this node ends, so a
// sequence after it starts in a new one.
if (only === "--serve") {
  const { createInterface } = await import("node:readline");
  const browser = await newBrowser();
  for await (const line of createInterface({ input: process.stdin })) {
    await mark("walking");
    const name = JSON.parse(line);
    const seq = sequences.find((q) => q.name === name);
    if (!seq) {
      console.log(`no sequence is called ${line}`);
      await said("FAIL");
      continue;
    }
    try {
      await said((await walkSequence(seq, browser)) ? "FAIL" : "ok");
    } catch (e) {
      console.log(e?.stack ?? String(e));
      await said("threw");
      process.exit(1);
    }
  }
  await browser.close();
  process.exit(0);
}

const exact = process.env.SEQUENCE;
const toWalk = sequences.filter((q) => (exact ? q.name === exact : q.name.includes(only)));
if (exact && !toWalk.length) {
  console.log(`no sequence is called ${JSON.stringify(exact)}`);
  process.exit(1);
}

for (const seq of toWalk) if (await walkSequence(seq)) failures++;
process.exit(failures ? 1 : 0);

// The line --serve ends what a sequence printed with, walkedLine in
// TestWalks, once everything it printed, on both outputs, is out.
async function said(how) {
  await mark(`walked ${how}`);
}

// A line of --serve's own, after everything printed before it, on both
// outputs.
async function mark(what) {
  await new Promise((done) => process.stderr.write("", done));
  await new Promise((done) => process.stdout.write(`\u001e${what}\n`, done));
}


// Walks one sequence and says whether it failed, in a browser of its own
// or in browser, whose pages it closes again.
async function walkSequence(seq, shared = null) {
  const began = performance.now();
  const opened = await open(url, { browser: shared, still: still(seq) });
  const { browser, errors } = opened;
  // The page the steps act on: the app's window, or the report window
  // once Bugs opened it. The watch keeps to the app's window.
  let page = opened.page;
  const main = page;
  let reportPage = null;
  const at = await chosen(page);
  const watch = new Watch(page, at, errors);
  await watch.start();
  const marks = {};
  const looks = {};
  // The second the video added with "add cameras" switches camera at.
  let cameraSwitch = null;
  const kept = {};
  // Whether the walk holds the speech model, see ["speech", "hold"], so
  // what is heard is only what the walk lets through.
  let speechHeld = false;
  // The models the settings name, at "settings", and the episode that was
  // on screen before them.
  let named = null;
  let episodeBefore = "";
  // The Updates page in the frame after a channel was picked.
  let picked = null;
  // What the video preview laid over the picture on every animation frame
  // of the last "play from", and the clip the engine had then.
  let played = null;
  let made = null;
  // The episode's last search and the window "move window" left, for
  // "searched moved".
  let movedOff = null;
  // The clip "remove clip" removed last, for "put back".
  let removedLast = null;
  let removedChosen = "";
  let loopEnd = null;
  // Whether the last "drag edge" held shift.
  let dragShift = false;
  const edgeLabel = { start: "Clip start", end: "Clip end", "cut from": "Where cut 1 starts", "cut to": "Where cut 1 ends" };
  let dragPx = 0;
  // Whether the edge "drag edge twice" took again moved while the hand
  // still held it.
  let movedHeld = false;
  // What the press that took the edge again landed on, the grips under
  // it, said if the playhead did not go with the edge (2.271).
  let pressedOn = "";
  let held = null;
  // What "scrub far" saw, for "keyed on the way".
  let scrubbed = null;
  let wrong = null;
  // The video and the other file dropped last.
  let droppedVideo = "";
  // How long a removed thing can be put back, in seconds, secondThoughts
  // in lib/removed.ts.
  const removeTime = 10;
  // The job of the clip made by hand that "hand stopped" stopped.
  let handJob = "";
  // The video whose folder a walk deleted from outside the app.
  let folderGone = "";
  // Whether the walk deleted that video too.
  let videoGone = false;
  let droppedOther = "";
  let droppedBefore = "";
  // Where the workspace and the report's field stood before a toast came.
  let standing = null;
  const done = [];
  const word = (text) => page.locator(".captions .word").filter({ hasText: new RegExp(`^${text}$`) }).first();
  // Shift and the right arrow, a frame on, and once the app has settled
  // on the press the video preview shows that frame, so the caption box
  // shows its words.
  const stepOn = async () => {
    await page.keyboard.press("Shift+ArrowRight");
    await settle(page);
    await shownOnPlayhead(page);
  };
  // The letters on screen in a box of the page, libass's on the canvas or
  // the caption box's own, as the box of their near white pixels, in
  // pixels of the page from the box's corner. Read off a screenshot, since
  // the page's own text is on no canvas.
  const letters = async (box) => {
    const clip = { x: Math.floor(box.left), y: Math.floor(box.top), width: Math.ceil(box.width), height: Math.ceil(box.height) };
    const shot = (await page.screenshot({ clip })).toString("base64");
    return page.evaluate(async (data) => {
      const img = new Image();
      img.src = `data:image/png;base64,${data}`;
      await img.decode();
      const c = document.createElement("canvas");
      c.width = img.width;
      c.height = img.height;
      const g = c.getContext("2d", { willReadFrequently: true });
      g.drawImage(img, 0, 0);
      const d = g.getImageData(0, 0, c.width, c.height).data;
      let left = Infinity, right = -Infinity, top = Infinity, bottom = -Infinity, n = 0;
      for (let y = 0; y < c.height; y++)
        for (let x = 0; x < c.width; x++) {
          const i = (y * c.width + x) * 4;
          if (d[i] > 200 && d[i + 1] > 200 && d[i + 2] > 200) {
            n++;
            left = Math.min(left, x);
            right = Math.max(right, x + 1);
            top = Math.min(top, y);
            bottom = Math.max(bottom, y + 1);
          }
        }
      const most = (keep) => {
        const seen = new Map();
        for (let i = 0; i < d.length; i += 4)
          if (keep(d[i], d[i + 1], d[i + 2])) {
            const k = `${d[i]},${d[i + 1]},${d[i + 2]}`;
            seen.set(k, (seen.get(k) ?? 0) + 1);
          }
        return [...seen].sort((a, b) => b[1] - a[1])[0]?.[0] ?? "";
      };
      const ink = most((r, g, b) => r > 200 && g > 200 && b > 200);
      const pill = most((r, g, b) => !(r > 200 && g > 200 && b > 200) && Math.max(r, g, b) - Math.min(r, g, b) > 40);
      return n ? { left, right, top, bottom, width: right - left, height: bottom - top, n, ink, pill } : null;
    }, shot);
  };
  // Until the captions over the frame are drawn, twice over, so a layer
  // asked for by the last is drawn too.
  const settled = async () => {
    for (let i = 0; i < 2; i++) {
      await page.evaluate(() => window.__captionLayers?.rested());
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
    }
  };
  // Until the frame on screen is drawn with its own captions over it, so
  // what libass drew is there to be read, see data-captioned in
  // Player.svelte, true then, and false if a layer is given up on first,
  // data-captions-lost. Nothing waiting for a layer was no sign of it: the
  // walk of a word opened in the caption box read a picture with none of
  // its letters on CI, issue #389.
  const captioned = () =>
    page.locator(".screen").evaluate(
      (screen) =>
        new Promise((done) => {
          const lost = screen.dataset.captionsLost;
          const own = () => screen.dataset.captioned !== undefined && Number(screen.dataset.captioned) === Number(screen.dataset.shown);
          if (own()) return done(true);
          const seen = new MutationObserver(() => {
            const gone = screen.dataset.captionsLost !== lost;
            if (!gone && !own()) return;
            seen.disconnect();
            done(!gone);
          });
          seen.observe(screen, { attributes: true, attributeFilter: ["data-captioned", "data-shown", "data-captions-lost"] });
        }),
    );

  // A step may put more steps after itself, see "row folds", so the
  // sequence's own list is left as it is.
  const steps = [...seq.steps];

  // What every step is handed, the walk as far as it has come: what
  // walkSequence holds, by name, read and written on it, so a step in any
  // folder reads the page, the marks and what went wrong as this does.
  const walk = {
    browser,
    get cameraSwitch() {
      return cameraSwitch;
    },
    set cameraSwitch(v) {
      cameraSwitch = v;
    },
    captioned,
    get dragPx() {
      return dragPx;
    },
    set dragPx(v) {
      dragPx = v;
    },
    get dragShift() {
      return dragShift;
    },
    set dragShift(v) {
      dragShift = v;
    },
    get droppedBefore() {
      return droppedBefore;
    },
    set droppedBefore(v) {
      droppedBefore = v;
    },
    get droppedOther() {
      return droppedOther;
    },
    set droppedOther(v) {
      droppedOther = v;
    },
    get droppedVideo() {
      return droppedVideo;
    },
    set droppedVideo(v) {
      droppedVideo = v;
    },
    edgeLabel,
    get episodeBefore() {
      return episodeBefore;
    },
    set episodeBefore(v) {
      episodeBefore = v;
    },
    errors,
    get folderGone() {
      return folderGone;
    },
    set folderGone(v) {
      folderGone = v;
    },
    get handJob() {
      return handJob;
    },
    set handJob(v) {
      handJob = v;
    },
    get held() {
      return held;
    },
    set held(v) {
      held = v;
    },
    kept,
    letters,
    looks,
    get loopEnd() {
      return loopEnd;
    },
    set loopEnd(v) {
      loopEnd = v;
    },
    get made() {
      return made;
    },
    set made(v) {
      made = v;
    },
    main,
    marks,
    get movedHeld() {
      return movedHeld;
    },
    set movedHeld(v) {
      movedHeld = v;
    },
    get movedOff() {
      return movedOff;
    },
    set movedOff(v) {
      movedOff = v;
    },
    get named() {
      return named;
    },
    set named(v) {
      named = v;
    },
    get page() {
      return page;
    },
    set page(v) {
      page = v;
    },
    get picked() {
      return picked;
    },
    set picked(v) {
      picked = v;
    },
    get played() {
      return played;
    },
    set played(v) {
      played = v;
    },
    get pressedOn() {
      return pressedOn;
    },
    set pressedOn(v) {
      pressedOn = v;
    },
    removeTime,
    get removedChosen() {
      return removedChosen;
    },
    set removedChosen(v) {
      removedChosen = v;
    },
    get removedLast() {
      return removedLast;
    },
    set removedLast(v) {
      removedLast = v;
    },
    get reportPage() {
      return reportPage;
    },
    set reportPage(v) {
      reportPage = v;
    },
    get scrubbed() {
      return scrubbed;
    },
    set scrubbed(v) {
      scrubbed = v;
    },
    seq,
    settled,
    get speechHeld() {
      return speechHeld;
    },
    set speechHeld(v) {
      speechHeld = v;
    },
    get standing() {
      return standing;
    },
    set standing(v) {
      standing = v;
    },
    stepOn,
    get videoGone() {
      return videoGone;
    },
    set videoGone(v) {
      videoGone = v;
    },
    watch,
    word,
    get wrong() {
      return wrong;
    },
    set wrong(v) {
      wrong = v;
    },
  };

  for (let at = 0; at < steps.length; at++) {
    const [what, ...args] = steps[at];
    const arg = args.length > 1 ? args : args[0];
    if (wrong || watch.failed) break;
    done.push(arg === undefined ? what : `${what} ${JSON.stringify(arg)}`);
    // Each step is said as it begins, so a step that waits for good is
    // named when the -timeout of make walks stops the sequence.
    console.log(`      ${done.at(-1)}`);
    waitsOn("nothing yet in this step");
    const s = await shown(page);
    const step = everyStep[what];
    if (!step) throw new Error(`no step called ${what}`);
    await step(walk, arg, s, what);
  }
  const failed = wrong ?? (watch.failed && `broke: ${watch.failed.rule}\n${watch.failed.detail}`);
  const took = ((performance.now() - began) / 1000).toFixed(1).padStart(5);
  console.log(`${failed ? "FAIL" : "ok  "}  ${took} s  ${seq.name}`);
  if (failed) console.log(`      after ${done.join(", ")}\n      ${failed.replaceAll("\n", "\n      ")}`);
  if (shared) await Promise.all(shared.contexts().map((c) => c.close()));
  else await browser.close();
  return Boolean(failed);
}
