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
//
// and what has to come of them:
//
//   ["box", text]      the caption box reads this, word by word
//   ["open", word]     this word is open for typing
//   ["same", label]    the engine's captions and pieces are what they were
//                      at the mark
//   ["cuts", n]        the clip timeline shows n cuts
//   ["spans", label]   every caption appears and goes when it did at the mark
//
// The episode is the bridge's, so the words are the sentences its speech
// stand-in says: "Ich war vielleicht sechs Jahre alt, als mich auf dem
// Schulhof irgendein Typ geschubst hat."
import { open, chosen, shown, engineState, handles, timeline, middle, high, drag } from "./bridge.mjs";
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
