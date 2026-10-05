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
// from the playhead: when the playhead is on the clip, which the app
// shows by not dimming the chosen clip on the clip timeline. Read off the
// app, never worked out here: a walk that decided it by itself was a
// second idea of where play starts, and drifted from the app's when 2.121
// made it a state.
const playsClip = () =>
  page.evaluate(() => {
    const chosen = document.querySelector(".clip-timeline .chosen");
    return !!chosen && !chosen.classList.contains("dim");
  });

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

// Puts the playhead a moment before the last cut, or before the clip's
// end when it has no cut, with a click on the clip timeline. Where on the
// clip timeline a moment is comes from the two edge handles, which stand
// centred on the clip's start and end. Left where it was when that place
// is close to a handle, which a click there would take hold of.
async function nearEnd(p, g) {
  const last = p[p.length - 1];
  const before = p[p.length - 2];
  const t = before ? Math.max(before.start + 0.1, before.end - 1.5) : Math.max(last.start + 0.1, last.end - 3);
  const s = middle(g.start);
  const e = middle(g.end);
  const x = s + ((t - p[0].start) / (last.end - p[0].start)) * (e - s);
  const handle = [g.start, g.end, ...g.cutEdges].some((h) => Math.abs(middle(h) - x) < 10);
  if (handle || !inside(g.track, x)) return;
  await page.mouse.click(x, high(g.track));
  await page.waitForFunction(() => !document.querySelector("video").seeking, null, { timeout: 5000 });
}

const gestures = [
  {
    name: "play",
    weight: 4,
    when: () => true,
    async run() {
      const p = await pieces();
      const from = (await video(page)).at;
      const clip = await playsClip();
      const ms = 300 + rng.int(2700);
      const stop = await recordFrames(page);
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      await page.waitForTimeout(ms);
      // A clip that reached its end has stopped by itself, and the space
      // bar would play it again from its start. So whether it still plays
      // is asked in the same moment as the space bar is pressed, inside
      // the page: asked first and pressed after, the clip reached its end
      // in between and played again.
      await page.evaluate(() => {
        if (document.querySelector("video").paused) return;
        const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
        document.body.dispatchEvent(new KeyboardEvent("keydown", key));
        document.body.dispatchEvent(new KeyboardEvent("keyup", key));
      });
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
    async run(g) {
      const p = await pieces();
      if (!(await playsClip())) return "nothing, the playhead is on the video";
      // Played from anywhere, a clip takes up to half a minute of real
      // time to reach its end, and that was most of what the walks took.
      // So the playhead goes a moment before the last cut first, which is
      // still a cut to jump on the way, or before the end when there is
      // none.
      await nearEnd(p, g);
      if (!(await playsClip())) return "a click on the clip timeline, which put the playhead on the video";
      const from = (await video(page)).at;
      // A clip at its end starts over, so as long as the whole clip.
      const length = p.reduce((sum, x) => sum + (x.end - x.start), 0);
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
