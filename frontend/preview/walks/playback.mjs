// A walk over playback: the space bar, playing for a moment or to the
// end, with cuts made and put back and the playhead put anywhere, in an
// order a seed decides. Besides the rules every walk keeps, see rules.mjs,
// it watches every frame the video preview puts on screen while a clip
// plays. See docs/TESTING.md.
//
//   SEED=12 STEPS=25 BRIDGE_URL=http://127.0.0.1:8123/ node playback.mjs
import { begin, walk } from "./walk.mjs";
import { handles, inside, middle, high, ask, recordFrames, video, engineState } from "./bridge.mjs";

const w = await begin({ steps: 25 });
const { page, rng, watch } = w;
const frame = 1 / (await ask(page, "Source", watch.at.path)).fps;

// The clip as the engine has it now.
const pieces = async () => (await engineState(page, watch.at)).segments;

// Whether a press of the space bar plays the clip rather than the episode
// from the playhead: when the playhead stands in the clip, the way the
// video preview decides it.
const playsClip = (p, at) => at >= p[0].start - frame / 2 && at < p[p.length - 1].end - 0.05;

// Every frame put on screen while a clip played is one of the clip's: in
// one of its pieces, a frame of give either side, since the jump over a
// cut is made once the frame at its edge is up.
function framesInClip(frames, p) {
  const out = frames.filter((t) => !p.some((x) => t >= x.start - frame - 1e-3 && t < x.end + frame));
  if (out.length) {
    watch.broke(
      "a playing clip shows only its own frames",
      `pieces ${p.map((x) => `${x.start.toFixed(2)}-${x.end.toFixed(2)}`).join(" ")}\nframes not in them ${out.map((t) => t.toFixed(3)).join(" ")}`,
    );
  }
}

// Paused, the picture stays where it was paused.
async function staysPaused(what) {
  const a = await video(page);
  await page.waitForTimeout(400);
  const b = await video(page);
  if (!a.paused || Math.abs(a.at - b.at) > 1e-3) {
    watch.broke("paused, the picture stays", `${what}: at ${a.at.toFixed(3)}, then ${b.at.toFixed(3)}, paused ${b.paused}`);
  }
}

const gestures = [
  {
    name: "play",
    weight: 4,
    when: () => true,
    async run() {
      const p = await pieces();
      const from = (await video(page)).at;
      const clip = playsClip(p, from);
      const ms = 300 + rng.int(2700);
      const stop = await recordFrames(page);
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      await page.waitForTimeout(ms);
      // A clip that reached its end has stopped by itself, and the space
      // bar would play it again from its start.
      if (!(await video(page)).paused) await page.keyboard.press("Space");
      const frames = await stop();
      if (!frames.length) watch.broke("the space bar plays", `played for ${ms} ms from ${from.toFixed(3)} and no frame came`);
      if (clip) framesInClip(frames, p);
      await staysPaused(`after playing for ${ms} ms`);
      return `play ${clip ? "the clip" : "the episode"} from ${from.toFixed(2)} for ${ms} ms`;
    },
  },
  {
    name: "end",
    weight: 1,
    when: () => true,
    async run() {
      const p = await pieces();
      const from = (await video(page)).at;
      if (!playsClip(p, from)) return "nothing, the playhead is not in the clip";
      const length = p.reduce((sum, x) => sum + Math.max(0, x.end - Math.max(x.start, from)), 0);
      const stop = await recordFrames(page);
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      await page.waitForFunction(() => document.querySelector("video").paused, null, {
        timeout: (length + 5) * 1000,
        polling: 100,
      }).catch(() => {});
      const frames = await stop();
      framesInClip(frames, p);
      const v = await video(page);
      const end = p[p.length - 1].end;
      if (!v.paused || Math.abs(v.at - end) > frame + 0.1) {
        watch.broke("a clip played to its end stops there", `end ${end.toFixed(3)}, stopped ${v.paused} at ${v.at.toFixed(3)}`);
      }
      await staysPaused("at the end");
      return `play the clip from ${from.toFixed(2)} to its end`;
    },
  },
  {
    name: "cut",
    weight: 2,
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
    weight: 1,
    when: (g) => g.cuts.some((c) => inside(g.track, middle(c))),
    async run(g) {
      const c = rng.pick(g.cuts.filter((c) => inside(g.track, middle(c))));
      await page.mouse.dblclick(middle(c), high(g.track));
      return `double-click on the cut at ${Math.round(middle(c))} px`;
    },
  },
  {
    name: "seek",
    weight: 2,
    when: () => true,
    async run(g) {
      const x = g.track.x + 4 + rng.next() * (g.track.w - 8);
      await page.mouse.click(x, high(g.track));
      return `click the clip timeline at ${Math.round(x)} px`;
    },
  },
  {
    name: "walk",
    weight: 1,
    when: () => true,
    async run() {
      const key = rng.next() < 0.7 ? "Shift+ArrowRight" : "Shift+ArrowLeft";
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press(key);
      return key;
    },
  },
];

await walk(w, gestures, {
  look: handles,
  async show(page) {
    const v = await video(page);
    return `at ${v.at.toFixed(2)}${v.paused ? "" : " playing"}`;
  },
});
