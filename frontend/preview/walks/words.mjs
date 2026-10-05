// A walk over the words of a clip: the gestures of the caption box in a
// random order, with rules checked after every step. A seed decides the
// order, so a walk that breaks a rule is walked again step for step with
// the same seed. See docs/TESTING.md.
//
//   SEED=12 STEPS=80 BRIDGE_URL=http://127.0.0.1:8123/ node words.mjs
import { random, open, settle, chosen, engineCaptions, shown } from "./bridge.mjs";

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
const seed = Number(process.env.SEED ?? Math.floor(Math.random() * 1e6));
const steps = Number(process.env.STEPS ?? 60);
const rng = random(seed);

// Words to type: some of the episode's own, and some it never says.
const pool = ["auf", "dem", "Schulhof", "Typ", "eine", "Arm", "ja", "Kaffee", "zwei Worte"];

const { browser, page, errors } = await open(url);
const at = await chosen(page);
const walked = [];
// The words removed in this walk, to be typed back in.
const removed = [];

// The engine's captions as one comparable value, and as a list of words.
const words = (caps) =>
  caps.captions.flatMap((c) => c.lines.flatMap((l) => l.words.map((w) => ({ ...w }))));
const same = (a, b) => JSON.stringify(a.captions) === JSON.stringify(b.captions);

// The undo and redo the walk expects: the engine's captions before each
// step that changed them.
const undone = [];
const redone = [];

let failed = null;
function broke(rule, detail) {
  failed ??= { rule, detail };
}

// The gestures, each with when it can be made and how often it is picked
// among those that can. Edits come up most, since that is where the
// caption box and the engine can part ways.
const editing = (s) => s.some((w) => w.focused);
const keyed = (s) => s.find((w) => w.keyed);
const gestures = [
  {
    name: "walk",
    weight: 2,
    when: (s) => !editing(s),
    async run() {
      const key = rng.next() < 0.6 ? "Shift+ArrowRight" : "Shift+ArrowLeft";
      const n = 1 + rng.int(3);
      for (let i = 0; i < n; i++) {
        await page.keyboard.press(key);
        await page.waitForTimeout(120);
      }
      return `${key} x${n}`;
    },
  },
  {
    name: "click",
    weight: 3,
    when: (s) => s.length > 0,
    async run(s) {
      const i = rng.int(s.length);
      await page.locator(".captions .word").nth(i).click();
      return `click "${s[i].text}"`;
    },
  },
  {
    name: "open",
    weight: 2,
    when: (s) => !editing(s) && keyed(s),
    async run(s) {
      await page.keyboard.press("Enter");
      return `Enter on "${keyed(s).text}"`;
    },
  },
  {
    name: "append",
    weight: 3,
    when: (s) => editing(s),
    async run() {
      const word = removed.length && rng.next() < 0.6 ? rng.pick(removed) : rng.pick(pool);
      await page.keyboard.press("End");
      await page.keyboard.type(" " + word);
      return `type " ${word}" at the end`;
    },
  },
  {
    name: "prepend",
    weight: 2,
    when: (s) => editing(s),
    async run() {
      const word = removed.length && rng.next() < 0.6 ? rng.pick(removed) : rng.pick(pool);
      await page.keyboard.press("Home");
      await page.keyboard.type(word + " ");
      return `type "${word} " at the start`;
    },
  },
  {
    name: "replace",
    weight: 2,
    when: (s) => editing(s),
    async run() {
      const word = rng.pick(pool);
      await page.keyboard.press("ControlOrMeta+A");
      await page.keyboard.type(word);
      return `replace with "${word}"`;
    },
  },
  {
    name: "clear",
    weight: 2,
    when: (s) => editing(s),
    async run(s) {
      removed.push(s.find((w) => w.focused).text);
      await page.keyboard.press("ControlOrMeta+A");
      await page.keyboard.press("Backspace");
      return "clear the word";
    },
  },
  {
    name: "save",
    weight: 4,
    when: (s) => editing(s),
    async run() {
      await page.keyboard.press("Enter");
      return "Enter";
    },
  },
  {
    name: "leave",
    weight: 1,
    when: (s) => editing(s),
    async run() {
      await page.keyboard.press("Escape");
      return "Escape";
    },
  },
  {
    name: "delete",
    weight: 3,
    when: (s) => !editing(s) && keyed(s),
    async run(s) {
      const word = keyed(s).text;
      removed.push(word);
      await page.keyboard.press(rng.next() < 0.5 ? "Backspace" : "Delete");
      return `delete "${word}"`;
    },
  },
  {
    name: "undo",
    weight: 2,
    when: (s) => !editing(s),
    async run() {
      await page.evaluate(() => window.__menu("undo"));
      return "Undo";
    },
  },
  {
    name: "redo",
    weight: 1,
    when: (s) => !editing(s),
    async run() {
      await page.evaluate(() => window.__menu("redo"));
      return "Redo";
    },
  },
  {
    name: "play",
    weight: 1,
    when: (s) => !editing(s),
    async run() {
      const ms = 300 + rng.int(1500);
      await page.keyboard.press("Space");
      await page.waitForTimeout(ms);
      await page.keyboard.press("Space");
      return `play for ${ms} ms`;
    },
  },
];

let before = await engineCaptions(page, at);
let callsSeen = await page.evaluate(() => window.__calls.length);
for (let step = 1; step <= steps && !failed; step++) {
  const s = await shown(page);
  const can = gestures.filter((g) => g.when(s));
  const g = rng.weighted(can);
  const said = await g.run(s);
  walked.push(`${String(step).padStart(3)}  ${said}`);
  await settle(page);

  const now = await shown(page);
  const after = await engineCaptions(page, at);
  if (process.env.VERBOSE) {
    const box = now.map((w) => (w.focused ? `[${w.text}]` : w.keyed ? `<${w.text}>` : w.text)).join(" ");
    console.log(`${walked[walked.length - 1].padEnd(36)} ${box}`);
  }

  // No call failed and the page threw nothing.
  const calls = await page.evaluate((from) => window.__calls.slice(from), callsSeen);
  callsSeen += calls.length;
  for (const c of calls) if (c.failed) broke("a call failed", `${c.name}: ${c.failed}`);
  for (const e of errors.splice(0)) broke("the page threw", e);

  // Enter opens the word in the keyboard's frame, and no other: the frame
  // is there to say which word Enter opens.
  if (g.name === "open") {
    const was = s.find((w) => w.keyed);
    const open = now.find((w) => w.focused);
    if (!open || Math.abs(open.at - was.at) > 1e-6) {
      broke("Enter opens the word in the frame", `framed "${was.text}" at ${was.at}, opened ${open ? `"${open.text}" at ${open.at}` : "nothing"}`);
    }
  }

  // One word at most wears the keyboard's frame, and one at most is open.
  if (now.filter((w) => w.keyed).length > 1) broke("one frame", "two words wear the frame");
  if (now.filter((w) => w.focused).length > 1) broke("one open word", "two words are open");

  // The caption box shows a caption of the engine's, exactly: the same
  // words, in the same order, at the same moments, nothing more.
  if (!editing(now) && now.length) {
    const caption = after.captions.find((c) =>
      c.lines.some((l) => l.words.some((w) => Math.abs(w.start - now[0].at) < 1e-6)),
    );
    const want = caption ? caption.lines.flatMap((l) => l.words.map((w) => `${w.text}@${w.start.toFixed(3)}`)) : [];
    const have = now.map((w) => `${w.text}@${w.at.toFixed(3)}`);
    if (want.join(" ") !== have.join(" ")) {
      broke("the caption box is the engine's caption", `shown  ${have.join(" ")}\nengine ${want.join(" ")}`);
    }
  }

  // Undo puts back what was there before the step it takes back, and Redo
  // what was there after it.
  if (g.name === "undo" || g.name === "redo") {
    const [from, to] = g.name === "undo" ? [undone, redone] : [redone, undone];
    if (from.length) {
      const want = from.pop();
      to.push(before);
      if (!same(after, want)) broke(`${g.name} puts it back`, describe(want, after));
    } else if (!same(after, before)) {
      // A step from before this walk, which the walk does not know.
      undone.length = redone.length = 0;
    }
  } else if (!same(after, before)) {
    undone.push(before);
    redone.length = 0;
    // An edit stays where it was made: every word more than three seconds
    // of the episode away from the word corrected keeps its text and its
    // time.
    const last = [...calls].reverse().find((c) => c.name === "SetWord");
    if (last) {
      const where = last.args[3];
      const far = (caps) =>
        words(caps)
          .filter((w) => w.said === undefined || Math.abs(w.said - where) > 3)
          .map((w) => `${w.text}@${w.start.toFixed(3)}-${w.end.toFixed(3)}`)
          .join(" ");
      if (far(before) !== far(after)) broke("an edit stays where it was made", describe(before, after));
      // Taking a word out never needs a caption more.
      if (last.args[4] === "" && after.captions.length > before.captions.length) {
        broke("a removal adds no caption", describe(before, after));
      }
    }
  }
  before = after;
}

function describe(a, b) {
  const line = (caps) => caps.captions.map((c) => `[${c.lines.flatMap((l) => l.words.map((w) => w.text)).join(" ")}]`).join(" ");
  return `before ${line(a)}\nafter  ${line(b)}`;
}

console.log(`seed ${seed}, ${walked.length} steps`);
if (failed) {
  console.log(walked.join("\n"));
  console.log(`\nbroke: ${failed.rule}\n${failed.detail}`);
  await page.screenshot({ path: `/tmp/walk-${seed}.png` });
  console.log(`picture: /tmp/walk-${seed}.png`);
}
await browser.close();
process.exit(failed ? 1 : 0);
