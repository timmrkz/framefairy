// A walk over playback: the space bar, playing for a moment or to the
// end, with cuts made and put back and the playhead put anywhere, in an
// order a seed decides. Besides the rules every walk keeps, see rules.mjs,
// it watches every frame the video preview puts on screen while a clip
// plays. See docs/TESTING.md.
//
//   SEED=12 STEPS=25 BRIDGE_URL=http://127.0.0.1:8123/ node playback.mjs
import { begin, walk } from "./walk.mjs";
import { handles, inside, middle, high, ask, recordFrames, preview, engineState, drag, settle } from "./bridge.mjs";
import { overlaid } from "./rules.mjs";

const w = await begin({ steps: 25 });
const { page, rng, watch } = w;
const frame = 1 / (await ask(page, "Source", watch.at.path)).fps;
const at = () => preview(page, frame);

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
// one of its pieces, a frame of give either side, since the frame that
// holds a piece's first moment starts before it.
function framesInClip(frames, p) {
  const out = frames.filter((t) => !p.some((x) => t >= x.start - frame - 1e-3 && t < x.end + frame));
  if (out.length) {
    watch.broke(
      "a playing clip shows only its own frames",
      `pieces ${p.map((x) => `${x.start.toFixed(2)}-${x.end.toFixed(2)}`).join(" ")}\nframes not in them ${out.map((t) => t.toFixed(3)).join(" ")}`,
    );
  }
}

// On every animation frame of a play, the video preview shows the video
// alone while the playhead is on the video, and the clip's crop frame and
// captions while it is on the clip, see overlaid in rules.mjs.
function overlaysFollow(overlays, state) {
  const broke = overlaid(overlays, state, frame);
  if (broke) watch.broke("the video preview shows the clip only on the clip", broke);
}

// Paused, the picture stays where it was paused: the playhead stays, and
// the frame on screen is the one that holds it, or where a play stopped on
// the clip's end, the frame that holds the moment a frame before it, the
// last the short shows, see frameAt in lib/flow.ts.
async function staysPaused(what) {
  const a = await at();
  await page.waitForTimeout(400);
  const b = await at();
  const holds = Math.floor((b.at + 0.001) / frame) * frame;
  const endsOn = Math.floor((b.at - frame + 0.001) / frame) * frame;
  const pictured = Math.abs(b.frame - holds) < 1e-6 || Math.abs(b.frame - endsOn) < 1e-6;
  if (!a.paused || !b.paused || Math.abs(a.at - b.at) > 1e-3 || !pictured) {
    watch.broke(
      "paused, the picture stays",
      `${what}: at ${a.at.toFixed(3)}, then ${b.at.toFixed(3)}, frame ${b.frame.toFixed(2)} on screen, paused ${a.paused} then ${b.paused}`,
    );
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
  const was = (await at()).at;
  await page.mouse.click(x, high(g.track));
  await page
    .waitForFunction((was) => Number(document.querySelector(".screen").dataset.playhead) !== was, was, { timeout: 2000 })
    .catch(() => {});
}

const gestures = [
  {
    name: "play",
    weight: 4,
    when: () => true,
    async run() {
      const state = await engineState(page, watch.at);
      const p = state.segments;
      const from = (await at()).at;
      const clip = await playsClip();
      const ms = 300 + rng.int(2700);
      const stop = await recordFrames(page, frame);
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      await page.waitForTimeout(ms);
      // A clip that reached its end has stopped by itself, and the space
      // bar would play it again from its start. So whether it still plays
      // is asked in the same moment as the space bar is pressed, inside
      // the page: asked first and pressed after, the clip reached its end
      // in between and played again.
      await page.evaluate(() => {
        if (document.querySelector('button[aria-label="Play"]')) return;
        const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
        document.body.dispatchEvent(new KeyboardEvent("keydown", key));
        document.body.dispatchEvent(new KeyboardEvent("keyup", key));
      });
      const { frames, moved, overlays } = await stop();
      if (!frames.length && !moved) watch.broke("the space bar plays", `played for ${ms} ms from ${from.toFixed(3)} and no frame came`);
      if (clip) framesInClip(frames, p);
      overlaysFollow(overlays, state);
      await staysPaused(`after playing for ${ms} ms`);
      return `play ${clip ? "the clip" : "the episode"} from ${from.toFixed(2)} for ${ms} ms`;
    },
  },
  {
    name: "end",
    weight: 1,
    when: () => true,
    async run(g) {
      const state = await engineState(page, watch.at);
      const p = state.segments;
      if (!(await playsClip())) return "nothing, the playhead is on the video";
      // Played from anywhere, a clip takes up to half a minute of real
      // time to reach its end, and that was most of what the walks took.
      // So the playhead goes a moment before the last cut first, which is
      // still a cut to jump on the way, or before the end when there is
      // none.
      await nearEnd(p, g);
      if (!(await playsClip())) return "a click on the clip timeline, which put the playhead on the video";
      const from = (await at()).at;
      // A clip at its end starts over, so as long as the whole clip.
      const length = p.reduce((sum, x) => sum + (x.end - x.start), 0);
      const stop = await recordFrames(page, frame);
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      // Playing, the play button says Pause, and once the clip has stopped
      // at its end, Play again.
      await page.locator('button[aria-label="Pause"]').waitFor({ timeout: 2000 }).catch(() => {});
      const end = p[p.length - 1].end;
      // Half the time L loops the clip on the way and L again stops the
      // loop a moment before the end, once the next time through is worked
      // out, which is how Tim heard the clip's start play on after the
      // play had stopped at its end.
      const looped = rng.next() < 0.5 ? 0.3 + rng.next() * 2 : 0;
      if (looped) {
        await page.keyboard.press("l");
        await page
          .waitForFunction((to) => Number(document.querySelector(".screen").dataset.playhead) > to, end - looped, {
            timeout: (length + 5) * 1000,
            polling: "raf",
          })
          .catch(() => {});
        await page.keyboard.press("l");
      }
      await page.waitForFunction(() => document.querySelector('button[aria-label="Play"]') !== null, null, {
        timeout: (length + 5) * 1000,
        polling: 100,
      }).catch(() => {});
      const { frames, overlays } = await stop();
      framesInClip(frames, p);
      overlaysFollow(overlays, state);
      // At the clip's end, the play stops: no more than half a second of
      // animation frames with the playhead there and the button on Pause.
      const stood = overlays.filter((o) => o.playing && o.at >= end - 1.5 * frame).length;
      if (stood > 30) {
        watch.broke("a clip played to its end stops there", `${stood} animation frames played on with the playhead at the end ${end.toFixed(3)}${looped ? ", loop switched off on the way" : ""}`);
      }
      const v = await at();
      if (!v.paused || Math.abs(v.at - end) > frame + 0.1) {
        watch.broke("a clip played to its end stops there", `end ${end.toFixed(3)}, stopped ${v.paused} at ${v.at.toFixed(3)}`);
      }
      await staysPaused("at the end");
      return `play the clip from ${from.toFixed(2)} to its end${looped ? `, loop on and off ${looped.toFixed(1)} s before it` : ""}`;
    },
  },
  {
    // The clip starts with the bridge's episode, so there is nothing of
    // the episode before it until its start is trimmed.
    name: "trim",
    weight: 1,
    when: (g) => g.end.x - (g.start.x + g.start.w) > 400 && g.start.x - g.track.x < 40,
    async run(g) {
      const px = 80 + rng.int(220);
      await drag(page, middle(g.start), high(g.track), px, false);
      return `drag the clip's start by ${px} px`;
    },
  },
  {
    // Plays the episode from before the clip on into it, which is on the
    // video the whole way, so the video preview shows the video alone.
    name: "into",
    weight: 2,
    when: (g) => g.start.x - g.track.x > 40,
    async run(g) {
      const state = await engineState(page, watch.at);
      const start = state.segments[0].start;
      const x = g.track.x + 4 + rng.next() * (g.start.x - g.track.x - 12);
      await page.mouse.click(x, high(g.track));
      await settle(page);
      const from = (await at()).at;
      const stop = await recordFrames(page, frame);
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      // Half the time the hand skips ahead on the way, shift and the right
      // arrow, or the arrow alone, every 300 ms: on the video either is a
      // skip through the episode, and the clip stays dimmed.
      const skip = rng.next() < 0.5 ? rng.pick(["Shift+ArrowRight", "ArrowRight"]) : "";
      const past = (to) => page.evaluate((to) => Number(document.querySelector(".screen").dataset.playhead) > to, to);
      if (skip) {
        const until = Date.now() + (start - from + 6) * 1000;
        while (Date.now() < until && !(await past(start + 1.5))) {
          await page.waitForTimeout(300);
          await page.keyboard.press(skip);
        }
      } else {
        await page
          .waitForFunction((to) => Number(document.querySelector(".screen").dataset.playhead) > to, start + 1.5, {
            timeout: (start - from + 6) * 1000,
            polling: 100,
          })
          .catch(() => {});
      }
      await page.evaluate(() => {
        if (document.querySelector('button[aria-label="Play"]')) return;
        const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
        document.body.dispatchEvent(new KeyboardEvent("keydown", key));
        document.body.dispatchEvent(new KeyboardEvent("keyup", key));
      });
      const { overlays } = await stop();
      const to = (await at()).at;
      if (to < start + 1) watch.broke("the episode plays on into the clip", `played from ${from.toFixed(3)} to ${to.toFixed(3)}, the clip starts at ${start.toFixed(3)}`);
      const lit = overlays.find((o) => o.playing && !o.video);
      if (lit) watch.broke("the episode plays on into the clip", `the clip lit up at ${lit.at.toFixed(3)}${skip ? ` with ${skip} pressed on the way` : ""}`);
      overlaysFollow(overlays, state);
      await staysPaused("after playing into the clip");
      return `play the episode from ${from.toFixed(2)} into the clip to ${to.toFixed(2)}${skip ? `, ${skip} on the way` : ""}`;
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
    const v = await at();
    return `at ${v.at.toFixed(2)}${v.paused ? "" : " playing"}`;
  },
});
