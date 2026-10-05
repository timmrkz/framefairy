// A walk over the words of a clip: the gestures of the caption box in a
// random order, with rules checked after every step. A seed decides the
// order, so a walk that breaks a rule is walked again step for step with
// the same seed. See docs/TESTING.md.
//
//   SEED=12 STEPS=80 BRIDGE_URL=http://127.0.0.1:8123/ node words.mjs
import { begin, walk } from "./walk.mjs";
import { editing, keyed } from "./rules.mjs";

const w = await begin();
const { page, rng } = w;

// Words to type: some of the episode's own, and some it never says.
const pool = ["auf", "dem", "Schulhof", "Typ", "eine", "Arm", "ja", "Kaffee", "zwei Worte"];

// The words removed in this walk, to be typed back in.
const removed = [];

// The gestures, each with when it can be made and how often it is picked
// among those that can. Edits come up most, since that is where the
// caption box and the engine can part ways.
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

await walk(w, gestures);
