// A walk over the clip timeline: the hand's gestures on a clip in a random
// order, with rules checked after every step, see rules.mjs. Above all,
// the clip timeline shows the engine's clip, and Undo and Redo put back
// the pieces and the captions as they were. See docs/TESTING.md.
//
//   SEED=12 STEPS=80 BRIDGE_URL=http://127.0.0.1:8123/ node timeline.mjs
import { begin, walk } from "./walk.mjs";
import { timeline, handles as look, inside, middle, high, drag } from "./bridge.mjs";

const w = await begin();
const { page, rng } = w;

const by = () => (rng.next() < 0.5 ? -1 : 1) * (8 + rng.int(180));

const gestures = [
  {
    name: "trim",
    weight: 3,
    when: (g) => inside(g.track, middle(g.start)) || inside(g.track, middle(g.end)),
    async run(g) {
      const edges = ["start", "end"].filter((e) => inside(g.track, middle(g[e])));
      const edge = rng.pick(edges);
      const dx = by();
      const shift = rng.next() < 0.3;
      await drag(page, middle(g[edge]), high(g.track), dx, shift);
      return `drag the clip ${edge} ${dx > 0 ? "+" : ""}${dx} px${shift ? " with shift" : ""}`;
    },
  },
  {
    name: "cut",
    weight: 3,
    when: (g) => g.end.x - (g.start.x + g.start.w) > 60,
    async run(g) {
      const from = Math.max(g.start.x + g.start.w + 10, g.track.x + 4);
      const to = Math.min(g.end.x - 10, g.track.x + g.track.w - 4);
      const x = from + rng.next() * (to - from);
      await page.mouse.dblclick(x, high(g.track));
      return `double-click in the clip at ${Math.round(x)} px`;
    },
  },
  {
    name: "join",
    weight: 2,
    when: (g) => g.cuts.some((c) => inside(g.track, middle(c))),
    async run(g) {
      const c = rng.pick(g.cuts.filter((c) => inside(g.track, middle(c))));
      await page.mouse.dblclick(middle(c), high(g.track));
      return `double-click on the cut at ${Math.round(middle(c))} px`;
    },
  },
  {
    name: "move",
    weight: 3,
    when: (g) => g.cutEdges.some((e) => inside(g.track, middle(e))),
    async run(g) {
      const e = rng.pick(g.cutEdges.filter((e) => inside(g.track, middle(e))));
      const dx = by();
      const shift = rng.next() < 0.3;
      await drag(page, middle(e), high(g.track), dx, shift);
      return `drag a cut's edge at ${Math.round(middle(e))} px ${dx > 0 ? "+" : ""}${dx} px${shift ? " with shift" : ""}`;
    },
  },
  {
    name: "reset",
    weight: 1,
    when: (g) => inside(g.track, middle(g.start)) || inside(g.track, middle(g.end)),
    async run(g) {
      const edge = rng.pick(["start", "end"].filter((e) => inside(g.track, middle(g[e]))));
      await page.mouse.dblclick(middle(g[edge]), high(g.track));
      return `double-click the clip ${edge}`;
    },
  },
  {
    name: "seek",
    weight: 1,
    when: () => true,
    async run(g) {
      const x = g.track.x + 4 + rng.next() * (g.track.w - 8);
      await page.mouse.click(x, high(g.track));
      return `click the clip timeline at ${Math.round(x)} px`;
    },
  },
  {
    name: "undo",
    weight: 2,
    when: () => true,
    async run() {
      await page.evaluate(() => window.__menu("undo"));
      return "Undo";
    },
  },
  {
    name: "redo",
    weight: 1,
    when: () => true,
    async run() {
      await page.evaluate(() => window.__menu("redo"));
      return "Redo";
    },
  },
  {
    name: "play",
    weight: 1,
    when: () => true,
    async run() {
      const ms = 300 + rng.int(1200);
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      await page.waitForTimeout(ms);
      await page.keyboard.press("Space");
      return `play for ${ms} ms`;
    },
  },
];

await walk(w, gestures, {
  look,
  async show(page) {
    const t = await timeline(page);
    return t
      ? `${t.start.toFixed(2)}-${t.end.toFixed(2)}${t.cuts.map((c) => ` cut ${c.from.toFixed(2)}-${c.to.toFixed(2)}`).join("")}`
      : "no clip";
  },
});
