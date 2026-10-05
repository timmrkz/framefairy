// Named sequences of steps with what has to come of them: the cases found
// by hand, kept so they stay fixed. Every step is also checked against the
// rules every walk is, see rules.mjs. See docs/TESTING.md.
//
//   BRIDGE_URL=http://127.0.0.1:8123/ node sequences.mjs [part of a name]
//
// A step is a list, its name first:
//
//   ["frame", word]    walks the playhead with Shift and → until the word is
//                      in the caption box, clicks it and leaves it with
//                      Escape, so it wears the keyboard's frame
//   ["click", word]    clicks the word, which opens it
//   ["press", key]     presses a key, as Playwright names it
//   ["type", text]     types
//   ["undo"], ["redo"] the menu bar's Undo and Redo
//   ["mark", label]    remembers the engine's captions and pieces
//   ["cut at", share]  double-clicks in the clip, this share of the way
//                      along it, which cuts a part out there
//   ["join", n]        double-clicks on the clip timeline's nth cut, from 1,
//                      which puts the part back
//   ["trim", edge, px] drags the clip's "start" or "end" edge by px
//   ["reset", edge]    double-clicks the clip's edge, which puts it back
//                      where the clip was found
//   ["model", how]     the language model "holds" its answers, "fails" or
//                      "answers"
//   ["add", seconds]   adds a new video of so many seconds with Add
//   ["add", seconds, rate]
//                      adds one filmed at a frame rate, "30000/1001" for
//                      29.97 fps, whose frames keep their numbers in a
//                      short
//   ["render"]         presses Render and waits until the short is written
//   ["head"]           presses the clip list's head button, New, Cancel or
//                      Continue
//   ["restart"]        closes the app and opens it again on the episode
//
// and what has to come of them:
//
//   ["box", text]      the caption box reads this, word by word
//   ["open", word]     this word is open for typing
//   ["same", label]    the engine's captions and pieces are what they were
//                      at the mark
//   ["cuts", n]        the clip timeline shows n cuts
//   ["wait for", word] the clip list's head button comes to say this, in a
//                      minute at most
//   ["row", words]     a row of the clip list says this
//   ["cards", n]       the clip list holds n clips
//   ["spans", label]   every caption appears and goes when it did at the mark
//   ["short", cuts]    the clip has at least so many cuts, none of them on
//                      a frame that starts on a whole millisecond, and the
//                      short Render wrote, read back from disk, holds
//                      exactly the frames of the clip's pieces, each the
//                      right frame of the episode, with its sound as long
//                      as its picture and quiet at a cut for no longer
//                      than the render's fade
//
// The episode is the bridge's, so the words are the sentences its speech
// stand-in says: "Ich war vielleicht sechs Jahre alt, als mich auf dem
// Schulhof irgendein Typ geschubst hat."
import { existsSync } from "node:fs";
import { open, chosen, shown, engineState, handles, timeline, middle, high, drag } from "./bridge.mjs";
import { clipList, control, pressHead, fromSidebar, settle, ask, shortFrames, shortSound } from "./bridge.mjs";
import { Watch, same, describe } from "./rules.mjs";

export const sequences = [
  {
    name: "delete removes the framed word and nothing else",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["box", "Ich war sechs Jahre alt,"],
      ["spans", "start"],
    ],
  },
  {
    name: "a word saved empty is removed",
    steps: [
      ["click", "Jahre"],
      ["press", "ControlOrMeta+A"],
      ["press", "Backspace"],
      ["press", "Enter"],
      ["box", "Ich war vielleicht sechs alt,"],
    ],
  },
  {
    name: "undo puts a removed word back and redo takes it again",
    steps: [
      ["mark", "start"],
      ["frame", "sechs"],
      ["press", "Delete"],
      ["mark", "removed"],
      ["undo"],
      ["same", "start"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["redo"],
      ["same", "removed"],
    ],
  },
  {
    name: "a removed word typed back after the word before it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "war"],
      ["press", "End"],
      ["type", " vielleicht"],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a removed word typed back before the word after it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "sechs"],
      ["press", "Home"],
      ["type", "vielleicht "],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a word typed in beside another is a word of its own, removed alone",
    steps: [
      ["mark", "start"],
      ["click", "war"],
      ["press", "End"],
      ["type", " noch"],
      ["press", "Enter"],
      // A word more takes room, so the last word goes on to the next
      // caption. Adding may move words, removing may not.
      ["box", "Ich war noch vielleicht sechs Jahre"],
      ["click", "noch"],
      ["open", "noch"],
      ["press", "Escape"],
      ["press", "Backspace"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["same", "start"],
    ],
  },
  {
    name: "Enter opens the framed word, wherever the playhead is",
    steps: [
      ["click", "Jahre"],
      ["press", "Escape"],
      ["press", "Enter"],
      ["open", "Jahre"],
    ],
  },
  {
    name: "a double-click in the clip cuts a part out, and one on the cut puts it back",
    steps: [
      ["mark", "start"],
      ["cut at", 0.5],
      ["cuts", 1],
      ["join", 1],
      ["cuts", 0],
      ["same", "start"],
    ],
  },
  {
    name: "a double-click on a trimmed edge puts it back where the clip was found",
    steps: [
      ["mark", "start"],
      ["trim", "end", -120],
      ["reset", "end"],
      ["same", "start"],
      ["trim", "start", 90],
      ["reset", "start"],
      ["same", "start"],
    ],
  },
  {
    name: "undo takes back a trim and then a cut, and redo does them again",
    steps: [
      ["mark", "start"],
      ["cut at", 0.4],
      ["mark", "cut"],
      ["trim", "end", -100],
      ["mark", "trimmed"],
      ["undo"],
      ["same", "cut"],
      ["undo"],
      ["same", "start"],
      ["redo"],
      ["same", "cut"],
      ["redo"],
      ["same", "trimmed"],
    ],
  },
  {
    name: "a new video gets its first clips with no click",
    steps: [
      ["add", 60],
      ["wait for", "New"],
      ["cards", 1],
    ],
  },
  {
    name: "Cancel stops a search, and Continue carries it on to its clips",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "New"],
      ["cards", 1],
    ],
  },
  {
    name: "a search the app was closed on says Interrupted, after it opens again",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Interrupted. Click Continue"],
    ],
  },
  {
    name: "a search that failed says so, and Continue tries again",
    steps: [
      ["model", "fails"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Failed. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "New"],
    ],
  },
  {
    // Found while 2.124 was proven: from a 29.97 fps episode every piece
    // of a short had a frame too many and about 30 ms of padded silence at
    // each cut, because a piece went to ffmpeg rounded to the millisecond.
    // TestARenderCutsOnWholeFrames holds the engine to it, this the app.
    name: "a render of a 29.97 fps episode cuts on whole frames",
    steps: [
      ["add", 40, "30000/1001"],
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["short", 2],
    ],
  },
  {
    name: "removing a word in the middle of a caption leaves the caption whole",
    steps: [
      ["mark", "start"],
      ["frame", "auf"],
      ["press", "Backspace"],
      ["box", "als mich dem Schulhof irgendein"],
      ["spans", "start"],
    ],
  },
];

// What is wrong with the short Render wrote for a clip, read back from
// disk, or null. Its pieces are cut the way the render cuts them, see
// cutOf in engine/render.go: the frames from the one a piece's start is on
// up to the one before its end, at least one. Which frame of the episode a
// frame of the short is comes from its bands, see shortFrames, and is
// compared with the frame it should be, so a frame too many, one lost or
// one from the wrong place shows. The episode's sound is a steady tone, so
// padded silence at a cut, or sound that runs on after its picture, shows.
// The engine's own test of this is TestARenderCutsOnWholeFrames.
function shortProblem(entry, short, fps, cutsWanted) {
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const pieces = entry.segments;
  const problems = [];
  // The case is the one it says: a clip with cuts whose edges, on the
  // frames of the episode where the render puts them, fall between
  // milliseconds, where rounding to one moved a piece by a frame. The
  // engine keeps an edge where a word starts or ends, which here is a
  // whole millisecond, so it is the frame it lands on that counts.
  const edges = pieces.slice(1).flatMap((p, i) => [pieces[i].end, p.start]).map((t) => Math.round(t * fps) / fps);
  if (pieces.length - 1 < cutsWanted) problems.push(`the clip has ${pieces.length - 1} cuts, not ${cutsWanted}`);
  const whole = edges.filter((t) => Math.abs(t * 1000 - Math.round(t * 1000)) < 0.01);
  if (whole.length) problems.push(`cuts on frames that start on a whole millisecond, ${whole.join(" ")}, not the case this is about`);

  // Seen: the frames of each piece in order, each once, and nothing else.
  const want = [];
  const cuts = [];
  for (const p of pieces) {
    const first = Math.round(p.start * fps);
    const end = Math.max(Math.round(p.end * fps), first + 1);
    for (let k = first; k < end; k++) want.push(k);
    cuts.push(want.length / fps);
  }
  cuts.pop();
  const { frames, modulo } = shortFrames(short);
  if (frames.length !== want.length) problems.push(`the short has ${frames.length} frames, its pieces hold ${want.length}`);
  let wrongFrames = 0;
  for (let i = 0; i < Math.min(frames.length, want.length); i++) {
    if (frames[i] !== want[i] % modulo && ++wrongFrames <= 5) {
      problems.push(`frame ${i} of the short is frame ${frames[i]} of the episode, not ${want[i]}`);
    }
  }
  if (wrongFrames > 5) problems.push(`and ${wrongFrames - 5} frames more are the wrong frame`);

  // Heard: as long as the picture, the tone right up to each cut, and
  // quiet there only for the fade out and in, 15 ms, Fade in
  // engine/render.go, and that quiet where the picture cuts.
  const rate = 48000;
  const samples = shortSound(short);
  const heard = samples.length / rate;
  const pictured = want.length / fps;
  if (Math.abs(heard - pictured) > 0.025) {
    problems.push(`the sound is ${heard.toFixed(3)} s long, the picture ${pictured.toFixed(3)} s`);
  }
  // The loudest sample in the 2.5 ms around each one, more than one swing
  // of the tone, so how loud the tone is there.
  const loud = (k) => {
    let most = 0;
    for (let j = Math.max(0, k - 60); j < Math.min(samples.length, k + 60); j++) most = Math.max(most, Math.abs(samples[j]));
    return most;
  };
  const steady = loud(Math.round(0.5 * rate));
  cuts.forEach((cut, n) => {
    const from = Math.round((cut - 0.06) * rate);
    const to = Math.round((cut + 0.06) * rate);
    if (to > samples.length) {
      problems.push(`the sound ends before cut ${n + 1} at ${cut.toFixed(3)} s`);
      return;
    }
    let first = -1;
    let last = -1;
    for (let k = from; k < to; k++) {
      if (loud(k) < 0.25 * steady) {
        if (first < 0) first = k;
        last = k;
      }
    }
    if (first < 0) {
      problems.push(`cut ${n + 1} at ${cut.toFixed(3)} s has no fade in its sound`);
      return;
    }
    const quiet = (last - first) / rate;
    const mid = (first + last) / 2 / rate;
    if (quiet > 0.015) problems.push(`the sound is quiet for ${(1000 * quiet).toFixed(1)} ms at cut ${n + 1}, more than its fade`);
    if (Math.abs(mid - cut) > 0.003) {
      problems.push(`the sound cuts at ${mid.toFixed(4)} s, the picture at ${cut.toFixed(4)} s, cut ${n + 1}`);
    }
  });
  if (!problems.length) return null;
  const said = pieces.map((p) => `${p.start}-${p.end}`).join(" ");
  return `${problems.join("\n")}\npieces ${said} at ${fps} fps, the short ${short}`;
}

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
const only = process.argv[2] ?? "";
let failures = 0;

for (const seq of sequences.filter((q) => q.name.includes(only))) {
  const { browser, page, errors } = await open(url);
  const at = await chosen(page);
  const watch = new Watch(page, at, errors);
  await watch.start();
  const marks = {};
  let wrong = null;
  const done = [];
  const word = (text) => page.locator(".captions .word").filter({ hasText: new RegExp(`^${text}$`) }).first();

  for (const [what, ...args] of seq.steps) {
    const arg = args.length > 1 ? args : args[0];
    if (wrong || watch.failed) break;
    done.push(arg === undefined ? what : `${what} ${JSON.stringify(arg)}`);
    const s = await shown(page);
    switch (what) {
      case "frame": {
        for (let i = 0; i < 80 && !(await word(arg).count()); i++) {
          await page.keyboard.press("Shift+ArrowRight");
          await page.waitForTimeout(100);
        }
        await word(arg).click();
        await page.keyboard.press("Escape");
        await watch.step("frame", s);
        break;
      }
      case "click":
        await word(arg).click();
        await watch.step("click", s);
        break;
      case "press":
        await page.keyboard.press(arg);
        await watch.step(arg === "Enter" && !s.some((w) => w.focused) ? "open" : "press", s);
        break;
      case "type":
        await page.keyboard.type(arg);
        await watch.step("type", s);
        break;
      case "undo":
      case "redo":
        await page.evaluate((m) => window.__menu(m), what);
        await watch.step(what, s);
        break;
      case "cut at": {
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        await page.mouse.dblclick(from + arg * (g.end.x - from), high(g.track));
        await watch.step("cut", s);
        break;
      }
      case "join": {
        const g = await handles(page);
        await page.mouse.dblclick(middle(g.cuts[arg - 1]), high(g.track));
        await watch.step("join", s);
        break;
      }
      case "trim": {
        const g = await handles(page);
        const [edge, px] = arg;
        await drag(page, middle(g[edge]), high(g.track), px, false);
        await watch.step("trim", s);
        break;
      }
      case "reset": {
        const g = await handles(page);
        await page.mouse.dblclick(middle(g[arg]), high(g.track));
        await watch.step("reset", s);
        break;
      }
      case "cuts": {
        const t = await timeline(page);
        if (t?.cuts.length !== arg) wrong = `the clip timeline shows ${t?.cuts.length ?? "no"} cuts, not ${arg}`;
        break;
      }
      case "model": {
        const how = { holds: "hang=1&fail=0", fails: "hang=0&fail=1", answers: "hang=0&fail=0" }[arg];
        await control(url, `/model?${how}`);
        break;
      }
      case "add": {
        const [seconds, rate] = [arg].flat();
        await control(url, `/pick?seconds=${seconds}${rate ? `&rate=${rate}` : ""}`);
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await watch.step("add", s);
        break;
      }
      case "render": {
        const on = await chosen(page);
        await page.waitForFunction(
          (r) => {
            const b = document.querySelector(r);
            return b && !b.disabled && b.textContent.trim().startsWith("Render");
          },
          "button.primary.render",
          { timeout: 60000, polling: 200 },
        );
        const renders = async () => (await ask(page, "Jobs")).filter((j) => j.kind === "render" && j.episode === on.path);
        const before = (await renders()).length;
        await page.locator("button.primary.render").click();
        let job = null;
        for (let i = 0; i < 600 && !job; i++) {
          await page.waitForTimeout(200);
          job = (await renders()).slice(before).find((j) => j.state === "done" || j.state === "failed") ?? null;
        }
        if (job?.state !== "done") wrong = `Render ${job ? `failed: ${job.error}` : "wrote no short in two minutes"}`;
        await watch.step("render", s);
        break;
      }
      case "head":
        await pressHead(page);
        await watch.step("head", s);
        break;
      case "restart": {
        const path = await page.evaluate(() => [...window.__calls].reverse().find((c) => c.name === "Clips")?.args[0]);
        await control(url, "/reopen");
        await page.reload();
        await page.waitForTimeout(1200);
        await fromSidebar(page, () => page.locator("aside li", { hasText: path.split("/").pop() }).first().click());
        await settle(page);
        break;
      }
      case "wait for": {
        const came = await page
          .waitForFunction(
            (word) => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              return pane?.querySelector(".listhead button.new")?.textContent.replace(/\s+/g, " ").trim() === word;
            },
            arg,
            { timeout: 60000, polling: 200 },
          )
          .then(() => true, () => false);
        if (!came) wrong = `the clip list's head says ${(await clipList(page))?.head}, not ${arg}, after a minute`;
        break;
      }
      case "row": {
        const list = await clipList(page);
        if (!list?.rows.some((r) => r.what === arg)) wrong = `no row says "${arg}": ${JSON.stringify(list?.rows)}`;
        break;
      }
      case "cards": {
        const list = await clipList(page);
        if (list?.cards.length !== arg) wrong = `the clip list holds ${list?.cards.length} clips, not ${arg}`;
        break;
      }
      case "mark":
        marks[arg] = await engineState(page, at);
        break;
      case "box": {
        const box = s.map((w) => w.text).join(" ");
        if (box !== arg) wrong = `the caption box reads "${box}", not "${arg}"`;
        break;
      }
      case "open": {
        const o = s.find((w) => w.focused);
        if (o?.text !== arg) wrong = `"${o?.text ?? "nothing"}" is open, not "${arg}"`;
        break;
      }
      case "same": {
        const now = await engineState(page, at);
        if (!same(now, marks[arg])) wrong = `the captions are not what they were at "${arg}"\n${describe(marks[arg], now)}`;
        break;
      }
      case "spans": {
        const spans = (caps) => caps.captions.map((c) => `${c.start.toFixed(3)}-${c.end.toFixed(3)}`).join(" ");
        const now = await engineState(page, at);
        if (spans(now) !== spans(marks[arg])) {
          wrong = `the captions do not appear and go when they did at "${arg}"\nthen ${spans(marks[arg])}\nnow  ${spans(now)}\n${describe(marks[arg], now)}`;
        }
        break;
      }
      case "short": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const { fps } = await ask(page, "Source", on.path);
        // The short is where the clip says, or in the output folder when
        // the settings name one, which the clip does not know of.
        const folder = (await ask(page, "GetSettings")).outputDir;
        const short = entry?.rendered ?? (folder && entry ? `${folder}/${entry.basename}.mp4` : "");
        wrong = shortProblem(entry, short, fps, arg);
        break;
      }
      default:
        throw new Error(`no step called ${what}`);
    }
  }
  const failed = wrong ?? (watch.failed && `broke: ${watch.failed.rule}\n${watch.failed.detail}`);
  console.log(`${failed ? "FAIL" : "ok  "}  ${seq.name}`);
  if (failed) {
    failures++;
    console.log(`      after ${done.join(", ")}\n      ${failed.replaceAll("\n", "\n      ")}`);
  }
  await browser.close();
}
process.exit(failures ? 1 : 0);
