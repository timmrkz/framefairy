// The steps of the sequences about the clip timeline: edges, cuts, trims,
// marks, keyframes and the playhead under the hand. Each is said above it,
// a list with its name first. walkSequence in ../sequences.mjs walks it,
// and hands it the walk so far, the step's argument, what the caption box
// showed before it and the step's name. A sequence of any folder can take
// it. Below them the helpers only these steps use. See docs/TESTING.md.
import { execFileSync } from "node:child_process";
import { BITS, ask, caughtUp, chosen, clickOnTrack, drag, engineState, episodeOn, handles, high, hold, holdSound, inside, letHeld, middle, news, nextNews, nextPaint, pageUntil, playUntil, pressOnTrack, pressPlay, preview, reader, seenUntil, settle, shownOnPlayhead, timeline, xOf } from "../bridge.mjs";
import { pieceCount, rate, url } from "../common/steps.mjs";

export const steps = {
  // ["trim into", s]   drags the clip's start to s seconds before the end of
  //                    the first word of its first caption
  async "trim into"(walk, arg, s) {
    const { page, watch } = walk;
    // The clip's start dragged to a moment arg seconds before the end
    // of its first word, so what is left of that word is a sliver.
    const now = await engineState(page, watch.at);
    const w = now.captions[0].lines[0].words[0];
    const t = await timeline(page);
    const g = await handles(page);
    const to = now.segments[0].start + w.end - arg;
    await drag(page, middle(g.start), high(g.track), (await xOf(page, to)) - (await xOf(page, t.start)), false);
    await watch.step("trim", s);
    // Until the engine has answered on which frames the picture lights
    // the words of the captions the trim saved, as it has long before a
    // hand gets to a block. Until then a click goes by the words' own
    // times, as it did before. The walk used to tell by the asks for
    // them and a moment's quiet after the last, and a busy runner now
    // and then went on before the saved captions' answer was in, so a
    // click landed on a frame lighting no word, issue #290.
    const { track } = await engineState(page, watch.at);
    const framed = await page.locator(".detail").evaluate(
      (detail, track) =>
        new Promise((done) => {
          const answered = () => detail.dataset.answered === track;
          if (answered()) return done(detail.dataset.framed === track);
          const seen = new MutationObserver(() => {
            if (!answered()) return;
            seen.disconnect();
            done(detail.dataset.framed === track);
          });
          seen.observe(detail, { attributes: true, attributeFilter: ["data-answered"] });
        }),
      track,
    );
    if (!framed) walk.wrong = "the engine did not say on which frames the words of the trimmed captions are lit";
  },
  // ["cut at", share]  double-clicks in the clip, this share of the way
  //                    along it, which cuts a part out there
  async "cut at"(walk, arg, s) {
    const { page, watch } = walk;
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    await page.mouse.dblclick(from + arg * (g.end.x - from), high(g.track));
    await watch.step("cut", s);
  },
  // ["join", n]        double-clicks on the clip timeline's nth cut, from 1,
  //                    which puts the part back
  async join(walk, arg, s) {
    const { page, watch } = walk;
    const g = await handles(page);
    await page.mouse.dblclick(middle(g.cuts[arg - 1]), high(g.track));
    await watch.step("join", s);
  },
  // ["trim", edge, px] drags the clip's "start" or "end" edge by px
  async trim(walk, arg, s) {
    const { page, watch } = walk;
    const g = await handles(page);
    const [edge, px] = arg;
    await drag(page, middle(g[edge]), high(g.track), px, false);
    await watch.step("trim", s);
  },
  // ["drag edge", which, px]
  //                    drags the clip's "start" or "end" edge, or the
  //                    "cut from" or "cut to" edge of its first cut, by px
  //                    in twenty steps while it is paused, reading the
  //                    playhead on every animation frame from the press to
  //                    after it lets go
  // ["drag edge", which, px, "shift"]
  //                    the same with shift held, so the edge lands on words
  // ["drag edge", which, px, "at the playhead"]
  //                    the same, pressed where the playhead's grip and the
  //                    edge's handle overlap rather than in its middle
  async "drag edge"(walk, arg, s) {
    const { edgeLabel, page, watch } = walk;
    const [which, px, how] = arg;
    walk.dragShift = how === "shift";
    walk.dragPx = px;
    const g = await handles(page);
    const box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[which];
    if (!box) {
      walk.wrong = `there is no ${which} edge to drag`;
      return;
    }
    let x = middle(box);
    if (how === "at the playhead") {
      // The playhead's grip is its line and four pixels either side.
      const line = await page.evaluate(() => {
        const b = document.querySelector(".clip-timeline .playhead").getBoundingClientRect();
        return b.left + b.width / 2;
      });
      const from = Math.max(line - 4, box.x + 1);
      const to = Math.min(line + 4, box.x + box.w - 1);
      if (!(to > from)) {
        walk.wrong = `the playhead's line at ${line.toFixed(1)} is not on the ${which} edge's handle, ${box.x.toFixed(1)} to ${(box.x + box.w).toFixed(1)}`;
        return;
      }
      x = (from + to) / 2;
    }
    await watchEdge(page, edgeLabel[which]);
    const y = box.y + box.h / 2;
    await page.mouse.move(x, y);
    if (walk.dragShift) await page.keyboard.down("Shift");
    // The sound the paused video preview opens while the edge is dragged.
    // Held under the hand, each place shows its frame alone, so only the
    // press may cue a play and open its sound, not every move after it.
    let sounds = 0;
    const opened = (r) => new URL(r.url()).pathname === "/frames/sound" && sounds++;
    page.on("request", opened);
    try {
      await page.mouse.down();
      for (let i = 1; i <= 20; i++) {
        await page.mouse.move(x + (px * i) / 20, y);
        await nextPaint(page);
      }
    } finally {
      page.off("request", opened);
    }
    await page.mouse.up();
    if (walk.dragShift) await page.keyboard.up("Shift");
    if (sounds > 1) {
      walk.wrong = `the paused video preview opened its sound ${sounds} times while the ${which} edge was dragged, not at most once`;
      return;
    }
    await watch.step("drag edge", s);
    await page.evaluate(() => (window.__followWatch = false));
  },
  // ["scrub far"]      presses the clip timeline's track near the clip's
  //                    start, waits for its frame, and with the button
  //                    still down moves the hand in one step at least 4 s
  //                    on, to 7 frames after a key frame of the video, as
  //                    ffprobe finds them, and waits for the frame under
  //                    the hand, noting every frame the video preview puts
  //                    up on the way and the key frames the frame queue
  //                    put up, then lets go
  // ["click far"]      clicks the clip timeline's track near the clip's
  //                    start and waits for its frame, then the same 7
  //                    frames after a key frame at least 4 s on, the button
  //                    down for 8 animation frames the way a person's click
  //                    is, and asks that the frame queue put up no key
  //                    frame and the video preview shows the frame clicked
  async "click far"(walk, arg, s, what) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const t = await timeline(page);
    const g = await handles(page);
    const y = high(g.track);
    // The key frames of the video, in the episode's time, as the
    // packets say, and the first at least 4 s on with 7 frames after
    // it in the clip.
    const keyTimes = execFileSync("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", await episodeOn(page)])
      .toString()
      .trim()
      .split("\n")
      .filter((l) => l.split(",")[1]?.startsWith("K"))
      .map((l) => Number(l.split(",")[0]))
      .sort((a, b) => a - b);
    const k = keyTimes.find((at) => at >= t.start + 4 && at + 8 / fps < t.end);
    if (k === undefined) {
      walk.wrong = `no key frame 4 s into the clip from ${t.start.toFixed(3)} to ${t.end.toFixed(3)} s, the key frames are at ${keyTimes.join(", ")}`;
      return;
    }
    const to = k + 7.5 / fps;
    const click = what === "click far";
    if (click) {
      await clickOnTrack(page, await xOf(page, t.start + 0.4), y);
    } else {
      await page.mouse.move(await xOf(page, t.start + 0.4), y);
      await page.mouse.down();
    }
    await settle(page);
    await shownOnPlayhead(page);
    // Every frame the video preview puts up, by where the app says it
    // begins, each a change of data-shown, and the key frames the frame
    // queue puts up.
    await page.evaluate(() => {
      const screen = document.querySelector(".screen");
      window.__scrubShown = [];
      window.__scrubKeys = window.__frameQueue?.stats.keys ?? 0;
      window.__scrubWentOn = window.__appFrames?.stats.keyWentOn ?? 0;
      // Each record's old value, and the value now once it is read, so
      // two changes before the observer runs are both kept.
      window.__scrubWatch = new MutationObserver((records) => {
        for (const r of records) window.__scrubShown.push(Number(r.oldValue));
      });
      window.__scrubWatch.observe(screen, { attributes: true, attributeOldValue: true, attributeFilter: ["data-shown"] });
    });
    await page.mouse.move(await xOf(page, to), y);
    if (click) {
      await page.mouse.down();
      await page.evaluate(
        () =>
          new Promise((done) => {
            let n = 0;
            const frame = () => (++n >= 8 ? done() : requestAnimationFrame(frame));
            requestAnimationFrame(frame);
          }),
      );
      await page.mouse.up();
    }
    await settle(page);
    await shownOnPlayhead(page);
    const rested = await preview(page, 1 / fps);
    const seen = await page.evaluate(() => {
      for (const r of window.__scrubWatch.takeRecords()) window.__scrubShown.push(Number(r.oldValue));
      window.__scrubWatch.disconnect();
      window.__scrubShown.push(Number(document.querySelector(".screen").dataset.shown));
      const q = window.__frameQueue?.stats;
      const n = (q?.keys ?? 0) - window.__scrubKeys;
      // The frames the decoder read on to, without a key frame first.
      const f = window.__appFrames?.stats;
      const m = Math.min((f?.keyWentOn ?? 0) - window.__scrubWentOn, 16);
      const wentOn = m > 0 ? f.wentOnRanks.slice(-m) : [];
      return { shown: window.__scrubShown, keys: n > 0 ? (q?.keysAt.slice(-n) ?? []) : [], count: n, wentOn };
    });
    if (click) {
      const under = Math.floor((rested.at + 0.001) * fps);
      if (seen.count) walk.wrong = `a click put up key frames, ${seen.count} of them, at ${seen.keys.map((a) => a.toFixed(3)).join(", ")}, before the frame clicked at ${rested.at.toFixed(3)} s`;
      else if (Math.round(rested.frame * fps) !== under) walk.wrong = `after the click the picture stood on frame ${Math.round(rested.frame * fps)}, not frame ${under} clicked`;
      await watch.step(what, s);
      return;
    }
    await page.mouse.up();
    await settle(page);
    await shownOnPlayhead(page);
    // The key frame the frame under the hand is decoded from: the last
    // at or before it.
    const key = keyTimes.filter((at) => at <= rested.at + 0.001).at(-1);
    walk.scrubbed = { fps, ...seen, key, rested, after: await preview(page, 1 / fps) };
    await watch.step("scrub far", s);
  },
  // ["once a frame", seconds]
  //                    clicks the clip timeline's track half a second into
  //                    the clip, plays it for so many seconds of the
  //                    episode and counts every change of the playhead on
  //                    the way: one for each frame of the video the play
  //                    crosses, two more or fewer at most, never one for
  //                    every animation frame (#446)
  async "once a frame"(walk, arg, s, what) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const t = await timeline(page);
    const g = await handles(page);
    await clickOnTrack(page, await xOf(page, t.start + 0.5), high(g.track));
    await settle(page);
    await shownOnPlayhead(page);
    // Every moment the playhead stands on while it plays, from the
    // first change after the play starts.
    await page.evaluate(() => {
      const screen = document.querySelector(".screen");
      window.__onceAt = [];
      window.__onceWatch = new MutationObserver(() => window.__onceAt.push(Number(screen.dataset.playhead)));
      window.__onceWatch.observe(screen, { attributes: true, attributeFilter: ["data-playhead"] });
    });
    await page.keyboard.press("Space");
    await playUntil(page, { most: arg });
    const at = await page.evaluate(() => {
      window.__onceWatch.disconnect();
      return window.__onceAt;
    });
    await page.keyboard.press("Space");
    await settle(page);
    const frameOf = (x) => Math.floor((x + 0.001) * fps);
    const crossed = at.length ? frameOf(at.at(-1)) - frameOf(at[0]) : 0;
    if (crossed < arg * fps - 2) walk.wrong = `the play crossed only ${crossed} frames in ${arg} s`;
    else if (at.length > crossed + 2) walk.wrong = `the playhead changed ${at.length} times while the play crossed ${crossed} frames`;
    else if (at.length < crossed - 2) walk.wrong = `the playhead changed only ${at.length} times while the play crossed ${crossed} frames`;
    await watch.step(what, s);
  },
  // ["keyed on the way"]
  //                    since "scrub far", the frame queue put up the key
  //                    frame the frame under the hand is decoded from,
  //                    that one exactly, unless the decoder said it read
  //                    on to that frame without a seek, and the frame it
  //                    ended on, with the button down and after it was let
  //                    go, is the frame under the hand, by the number
  //                    drawn on it
  async "keyed on the way"(walk) {
    if (!walk.scrubbed) {
      walk.wrong = "nothing was scrubbed";
      return;
    }
    const { fps, shown, keys, key, rested, after, wentOn } = walk.scrubbed;
    const frameOf = (at) => Math.floor((at + 0.001) * fps);
    const under = frameOf(rested.at);
    // What the frame queue put up, rather than what was painted: on a
    // busy machine the frame under the hand can come before the next
    // paint, and then the key frame is rightly never painted.
    //
    // A cursor of the decoder can stand between the key frame and the
    // frame under the hand already, left there by whatever the app read
    // before, a clip card's first frame most likely. Reading on from it is
    // cheaper than the key frame, so the decoder does, and says so, and
    // no key frame comes. On a busy machine that was 1 run in 8, issue
    // #511. Only a read on to the frame under the hand stands in for the
    // key frame, and that frame is still checked below.
    const wentOnInstead = !keys.length && wentOn.includes(under);
    if (!wentOnInstead && !keys.some((at) => Math.abs(at - key) < 0.5 / fps)) {
      walk.wrong = `the key frame at ${key.toFixed(3)} s was not put up on the way to the frame under the hand at ${rested.at.toFixed(3)} s, the frame queue put up key frames at ${keys.map((a) => a.toFixed(3)).join(", ") || "none"} and the video preview showed ${shown.map((a) => a.toFixed(3)).join(", ")}`;
    } else if (Math.round(rested.frame * fps) !== under) {
      walk.wrong = `with the button still down, the picture stood on frame ${Math.round(rested.frame * fps)}, not frame ${under} under the hand`;
    } else if (Math.round(after.frame * fps) !== frameOf(after.at)) {
      walk.wrong = `once the button was let go, the picture stood on frame ${Math.round(after.frame * fps)}, not frame ${frameOf(after.at)} under the hand`;
    }
  },
  // ["hold on", which] presses the track near the end away from it, then the
  //                    clip's "start" or "end" edge, the "cut from" or "cut
  //                    to" edge of its first cut, its first "thumbnail" or
  //                    "crop" mark, or the "caption start" or "caption end"
  //                    edge of its second caption, reading the playhead on
  //                    every animation frame while the button is down, and
  //                    lets go
  async "hold on"(walk, arg, s) {
    const { page, watch } = walk;
    // The playhead read on every animation frame from the press, for a
    // press on the track first and then for the thing held, so "at
    // once" is a number: no more frames than the track takes.
    //
    // The frames are counted from the press as the page has it, not
    // from when the watch began. Moving the pointer and pressing it are
    // two calls to the browser, and on a busy runner either can take a
    // frame or two to arrive, so counted from the watch the same app
    // took 2 frames for the track and 4 for the mark, at random.
    const at = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    const watchFrames = () =>
      page.evaluate(() => {
        window.__pressed = [];
        window.__pressedAt = -1;
        window.__pressWatch = true;
        addEventListener("pointerdown", () => (window.__pressedAt = window.__pressed.length), {
          capture: true,
          once: true,
        });
        const tick = () => {
          if (!window.__pressWatch) return;
          window.__pressed.push(Number(document.querySelector(".screen")?.dataset.playhead));
          requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      });
    // The frames from the press on, the first of them the first painted
    // after it, up to the next frame: the watch reads a frame before the
    // app paints in it, so what the frame settle ended on showed only in
    // the frame after, and a settle of one frame ended before it.
    const seen = () =>
      page.evaluate(
        () =>
          new Promise((done) =>
            requestAnimationFrame(() => {
              window.__pressWatch = false;
              done(window.__pressedAt < 0 ? [] : window.__pressed.slice(window.__pressedAt));
            }),
          ),
      );
    const g = await handles(page);
    const caption = (edge) =>
      page.locator(`.clip-timeline [aria-label="When caption 2 ${edge === "end" ? "goes" : "appears"}"]`).first().boundingBox();
    let box;
    if (arg === "thumbnail" || arg === "crop") {
      box = await page.locator(`.clip-timeline .mark${arg === "crop" ? ".cropdot" : ".thumb"}`).first().boundingBox();
    } else if (arg === "caption start" || arg === "caption end") {
      box = await caption(arg === "caption end" ? "end" : "start");
    } else {
      box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[arg];
    }
    if (!box) {
      walk.wrong = `there is no ${arg} to press`;
      return;
    }
    // The track, near the end away from what is held, so the press on
    // it moves the playhead off the frame it is then to come back to.
    const left = box.x + (box.width ?? box.w) / 2 > g.track.x + g.track.w / 2;
    const trackBefore = await at();
    await watchFrames();
    await page.mouse.move(left ? g.track.x + 8 : g.track.x + g.track.w - 8, high(g.track));
    await page.mouse.down();
    // Held until the video preview shows the frame under the hand.
    await settle(page);
    await shownOnPlayhead(page);
    const track = await seen();
    await page.mouse.up();
    await watch.step("hold on", s);
    const trackFrames = track.findIndex((t) => Math.abs(t - trackBefore) > 1e-6);
    const before = await at();
    await watchFrames();
    await page.mouse.move(box.x + box.width / 2 || middle(box), box.y + (box.height ?? box.h) / 2);
    await page.mouse.down();
    await settle(page);
    await shownOnPlayhead(page);
    walk.held = { which: arg, before, frames: await seen(), trackFrames, at: await at() };
    await page.mouse.up();
    await watch.step("hold on", s);
  },
  // ["on press", which]
  //                    since "hold on", the playhead had moved while the
  //                    button was still down, onto the frame "followed"
  //                    names for an edge, the mark's frame, or the frame
  //                    the caption appears on or the last it is up on, in
  //                    no more animation frames than the press on the
  //                    track took
  async "on press"(walk, arg) {
    const { page, watch } = walk;
    const frame = 1 / (await rate(page));
    const now = await engineState(page, watch.at);
    const segs = now.segments;
    // A moment on the clip's clock in the episode, the way the clip
    // timeline reads it, a start where two pieces meet.
    const into = (t, closes) => {
      let sum = 0;
      for (const p of segs) {
        const span = p.end - p.start;
        if (closes ? t <= sum + span + 1e-6 : t < sum + span - 1e-6) return p.start + t - sum;
        sum += span;
      }
      return segs.at(-1).end;
    };
    let want;
    if (arg === "thumbnail" || arg === "crop") {
      want = await page.evaluate(
        (kind) => Number(document.querySelector(`.clip-timeline .mark${kind}`)?.getAttribute("aria-valuenow")),
        arg === "crop" ? ".cropdot" : ".thumb",
      );
    } else if (arg === "caption start") {
      want = into(now.captions[1].start, false);
    } else if (arg === "caption end") {
      want = into(now.captions[1].end, true) - frame;
    } else {
      want = { start: segs[0].start, end: segs.at(-1).end - frame, "cut from": segs[0].end - frame, "cut to": segs[1]?.start }[arg];
    }
    const same = (a, b) => Math.floor(a / frame + 0.001 / frame) === Math.floor(b / frame + 0.001 / frame);
    const on = walk.held?.frames?.findIndex((t) => same(t, want)) ?? -1;
    if (walk.held?.which !== arg) walk.wrong = `${arg} was not held`;
    else if (same(walk.held.before, want)) walk.wrong = `the playhead stood on the ${arg}'s frame before the press, which proves nothing`;
    else if (!same(walk.held.at, want)) {
      walk.wrong = `the playhead stood at ${walk.held.at.toFixed(3)} with the button down on the ${arg}, not on its frame at ${want.toFixed(3)}`;
    } else if (walk.held.trackFrames < 0) walk.wrong = "a press on the track did not move the playhead";
    else if (on < 0) walk.wrong = `the playhead came onto the ${arg} only after 150 ms`;
    else if (on > walk.held.trackFrames) {
      walk.wrong = `the playhead came onto the ${arg} ${on} frames after the press, a press on the track takes ${walk.held.trackFrames}`;
    }
  },
  // ["watch captions"] from now on reads, on every animation frame, the
  //                    frame on the canvas of the video preview and how
  //                    many of its pixels are the white of the captions
  //                    libass drew over it, and whether the crop frame
  //                    stands over it
  async "watch captions"(walk) {
    await watchCaptions(walk.page);
  },
  // ["captions stayed"] since "watch captions", the captions were seen,
  //                    and a frame that stayed on the video preview kept
  //                    them and the crop frame: neither went off over a
  //                    picture that did not change. Tim saw them blink off for a frame each
  //                    time a clip's start dragged later passed a frame,
  //                    issue #755.
  async "captions stayed"(walk) {
    const seen = await walk.page.evaluate(() => {
      window.__captionWatch.on = false;
      return window.__captionWatch.seen;
    });
    // The captions, then the crop frame, gone over a frame that stayed.
    const gone = (k, what) => {
      const off = seen.findIndex((s, i) => i > 0 && s[0] >= 0 && s[0] === seen[i - 1][0] && seen[i - 1][k] && !s[k]);
      if (off < 0) return "";
      const back = seen.slice(off).findIndex((s) => s[k]);
      return `frame ${seen[off][0]} stayed on the video preview and its ${what} went off for ${back < 0 ? "good" : `${back} animation frames`}, after ${off} of ${seen.length}`;
    };
    if (!seen.some(([, ink]) => ink > 0)) walk.wrong = `no captions were seen over the ${seen.length} frames read`;
    else {
      const why = gone(1, "captions") || gone(2, "crop frame");
      if (why) walk.wrong = why;
    }
  },
  // ["went the hand's way", which]
  //                    since "drag edge", the edge never stood on the far
  //                    side of where it was pressed from the way the hand
  //                    went
  async "went the hand's way"(walk, arg) {
    const { page } = walk;
    const seen = await page.evaluate(() => window.__edgeSeen ?? []);
    const back = seen.find((v) => (v - seen[0]) * walk.dragPx < -0.0005);
    if (!seen.length) walk.wrong = `the ${arg} edge was not seen`;
    else if (back !== undefined) {
      walk.wrong = `the ${arg} edge went from ${seen[0].toFixed(3)} to ${back.toFixed(3)}, against the hand, which went ${walk.dragPx > 0 ? "right" : "left"}`;
    }
  },
  // ["moved", which]   since "drag edge", the edge moved by a frame or more
  async moved(walk, arg) {
    const { page } = walk;
    const frame = 1 / (await rate(page));
    const seen = await page.evaluate(() => window.__edgeSeen ?? []);
    if (!(seen.length && Math.abs(seen.at(-1) - seen[0]) >= frame - 0.001)) {
      walk.wrong = `the ${arg} edge stayed at ${seen[0]?.toFixed(3)} while the hand dragged it`;
    }
  },
  // ["kept up", which] since "drag edge twice", the edge moved while the
  //                    hand took it again and still held it, so it went
  //                    with the hand while the drag before it saved, and
  //                    the edge was saved where the hand left it last
  async "kept up"(walk, arg) {
    const { page, watch } = walk;
    const seen = await page.evaluate(() => window.__edgeSeen ?? []);
    // The two drags are saved in the order they were let go, so the
    // engine has the edge where the hand left it the second time, the
    // farthest it went the hand's way, once both saves are done.
    await settle(page);
    const segs = (await engineState(page, watch.at)).segments;
    const saved = { start: segs[0].start, end: segs.at(-1).end, "cut from": segs[0].end, "cut to": segs[1]?.start }[arg];
    const left = walk.dragPx < 0 ? Math.min(...seen) : Math.max(...seen);
    if (!walk.movedHeld) walk.wrong = `the ${arg} edge stood where the drag before let it go while the hand took it again, it did not go with the hand`;
    else if (!(Math.abs(saved - left) < 0.002)) {
      walk.wrong = `the hand left the ${arg} edge at ${left.toFixed(3)} and it was saved at ${saved?.toFixed(3)}`;
    }
  },
  // ["drag edge twice", which, px, again, late]
  //                    drags an edge by px, lets go, and at once, with
  //                    nothing waited for, drags it again by again. With
  //                    late, the last place of the first drag is a frame
  //                    further on and its answer is held until the edge
  //                    has been taken again and dragged. A call held by
  //                    ["hold"] before it comes back then too
  async "drag edge twice"(walk, arg, s) {
    const { edgeLabel, page, watch } = walk;
    // Dragged, let go, and taken again at once where it was let go, with
    // nothing waited for in between, while the first is being saved.
    const [which, px, again, late] = arg;
    walk.dragShift = false;
    walk.dragPx = again;
    walk.pressedOn = "";
    const g = await handles(page);
    const box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[which];
    if (!box) {
      walk.wrong = `there is no ${which} edge to drag`;
      return;
    }
    const y = box.y + box.h / 2;
    const x = middle(box);
    await page.mouse.move(x, y);
    await page.mouse.down();
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(x + (px * i) / 10, y);
      await nextPaint(page);
    }
    // Every place so far answered and drawn, which the clip timeline
    // says by dropping data-shaping, and then the edge stands under the
    // hand before it lets go, the way a person sees it. On a busy
    // machine the answers came late, and the hand let go and pressed
    // again beside an edge still on its way, on the caption under it,
    // issue #427. No call on its way was no sign of it: the app asks
    // for the next place the moment an answer comes, so for a moment
    // none is. With late, the last place, ten pixels on, which is more
    // than a frame at this zoom, is then held on its way.
    const took = late ? x + px + Math.sign(px) * 10 : x + px;
    const standing = await page.evaluate(
      ([label, x, y]) =>
        new Promise((done) => {
          const timeline = document.querySelector(".clip-timeline");
          const under = () => {
            const edge = timeline.querySelector(`[aria-label="${label}"]`);
            return !!edge && document.elementsFromPoint(x, y).includes(edge);
          };
          if (!timeline.hasAttribute("data-shaping")) return done(under());
          const answered = new MutationObserver(() => {
            if (timeline.hasAttribute("data-shaping")) return;
            answered.disconnect();
            done(under());
          });
          answered.observe(timeline, { attributes: true, attributeFilter: ["data-shaping"] });
        }),
      [edgeLabel[which], x + px, y],
    );
    if (!standing) {
      await page.mouse.up();
      walk.wrong = `the ${which} edge was not under the hand once every place it was dragged to was answered`;
      return;
    }
    if (late) {
      await hold(page, "Shape");
      await page.mouse.move(took, y);
    }
    await page.mouse.up();
    await watchEdge(page, edgeLabel[which]);
    walk.pressedOn = await page.evaluate(
      ([x, y]) =>
        [...new Set(document.elementsFromPoint(x, y).map((el) => el.closest("[data-grip]")?.dataset.grip).filter(Boolean))].join(", ") || "no grip",
      [took, y],
    );
    await page.mouse.down();
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(took + (again * i) / 10, y);
      await nextPaint(page);
    }
    // The hand stays down until the app has settled on the drag, with
    // the call held still on its way, so an edge that waits for it
    // before it moves is seen standing. Then the call held comes back,
    // late, while the hand is still down.
    await settle(page);
    walk.movedHeld = await page.evaluate(() => new Set(window.__edgeSeen.map((t) => t.toFixed(3))).size > 1);
    await letHeld(page);
    await settle(page);
    await page.mouse.up();
    await watch.step("drag edge twice", s);
    await page.evaluate(() => (window.__followWatch = false));
  },
  // ["edge then track", which]
  //                    clicks an edge and at once presses the track near
  //                    its right end: the playhead stands where the track
  //                    was pressed
  async "edge then track"(walk, arg, s) {
    const { page, watch } = walk;
    // A click on an edge, and at once a press on the track: the
    // playhead stays where the track was pressed.
    const g = await handles(page);
    const box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[arg];
    if (!box) {
      walk.wrong = `there is no ${arg} edge to click`;
      return;
    }
    const y = box.y + box.h / 2;
    await clickOnTrack(page, middle(box), y);
    const x = g.track.x + g.track.w * 0.9;
    await page.mouse.move(x, high(g.track));
    await page.mouse.down();
    await settle(page);
    const at = await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    const want = await page.evaluate((x) => {
      const b = document.querySelector(".clip-timeline .track").getBoundingClientRect();
      return x - b.left;
    }, x);
    await page.mouse.up();
    await watch.step("edge then track", s);
    const placed = await xOf(page, at);
    if (Math.abs(placed - x) > 3) {
      walk.wrong = `the playhead stands ${(placed - x).toFixed(1)} px from where the track was pressed after a click on the ${arg}, ${want.toFixed(0)} px in`;
    }
  },
  // ["followed", which]
  //                    since "drag edge", the playhead went with the edge:
  //                    on frames, the edge stood in two places or more,
  //                    and on each the playhead came onto the edge's frame
  //                    within three animation frames of the edge leaving
  //                    it, and once the hand let go it stands on the
  //                    frame the edge leaves on screen, the clip's or the
  //                    piece's first frame for "start" and "cut to", its
  //                    last for "end" and "cut from", with shift too
  async followed(walk, arg) {
    const { page, watch } = walk;
    const segs = (await engineState(page, watch.at)).segments;
    const frame = 1 / (await rate(page));
    const want = {
      start: segs[0].start,
      end: segs.at(-1).end - frame,
      "cut from": segs[0].end - frame,
      "cut to": segs[1]?.start,
    }[arg];
    const at = await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    // Every place the edge stood while it was watched, the playhead
    // stood on that place's frame too, the edge's own frame for "start"
    // and "cut to", the one before for "end" and "cut from", within
    // three animation frames of the edge leaving it. The two are read in
    // the same frames, so this holds on a machine of any speed. A count
    // of places did not: an edge taken again at once stands still until
    // the drag before it is saved (2.259), so on a busy runner it stood
    // in 2 or 3 places, and a playhead left behind stands in 2 as well.
    // On words the engine puts the playhead by the words, so this is
    // asked of a drag on frames.
    const frames = (await page.evaluate(() => window.__edgeFrames ?? [])).filter(([e]) => Number.isFinite(e));
    const onFrame = (a, b) => Math.floor(a / frame + 0.001 / frame) === Math.floor(b / frame + 0.001 / frame);
    const late = 3;
    const runs = [];
    frames.forEach(([e], i) => {
      if (runs.at(-1)?.edge.toFixed(3) === e.toFixed(3)) runs.at(-1).to = i;
      else runs.push({ edge: e, from: i, to: i });
    });
    const opens = arg === "start" || arg === "cut to";
    const missed = runs.find(
      (r) =>
        r.to + late < frames.length &&
        !frames.slice(r.from, r.to + late + 1).some(([, p]) => onFrame(p, opens ? r.edge : r.edge - frame)),
    );
    if (walk.dragShift) {
      // Nothing more than where it lands, below.
    } else if (runs.length < 2) {
      walk.wrong = `the ${arg} edge stood in ${runs.length ? "one place" : "no place"} while the hand dragged it, which proves nothing`;
    } else if (missed) {
      const went = frames.slice(missed.from, missed.to + late + 1).map(([, p]) => p.toFixed(3));
      // Every edge and playhead the watch saw, and what the press
      // landed on, since a runner saw this and this machine does not.
      const seen = frames.map(([e, p]) => `${e.toFixed(3)}/${p.toFixed(3)}`).filter((v, k, all) => v !== all[k - 1]);
      walk.wrong = `the ${arg} edge stood at ${missed.edge.toFixed(3)} and the playhead did not come onto its frame, it stood at ${[...new Set(went)].join(", ")}.${walk.pressedOn ? ` The press that took it again landed on ${walk.pressedOn}.` : ""} Edge/playhead went ${seen.join(" ")}`;
    } else if (!(Math.abs(at - want) < frame / 2 + 0.001)) {
      walk.wrong = `the playhead stands at ${at.toFixed(3)} after the ${arg} edge was dragged, not on its frame at ${want?.toFixed(3)}`;
    }
  },
  async "nothing past the cut"(walk) {
    const { page, watch } = walk;
    // While the cut's left edge was dragged, the video preview showed
    // the frames before the cut, the edge's, and never one after it:
    // the playhead stood before the cut all along.
    const segs = (await engineState(page, watch.at)).segments;
    const after = segs[1].start;
    const frames = await page.evaluate(() => window.__edgeFrames ?? []);
    const past = frames.filter(([, p, f]) => p < after && f >= after - 0.001);
    if (frames.length < 10) walk.wrong = `the drag was watched for ${frames.length} frames, which proves nothing`;
    else if (past.length) {
      const seen = [...new Set(past.map(([, p, f]) => `${f.toFixed(3)} with the playhead at ${p.toFixed(3)}`))];
      walk.wrong = `while the cut's left edge was dragged, the video preview showed frames from after the cut, which starts again at ${after.toFixed(3)}: ${seen.slice(0, 6).join(", ")}`;
    }
  },
  // ["drag while playing", edge, px]
  //                    plays the clip from a moment into it, and drags its
  //                    "end" edge, or the first edge of its first "cut", by
  //                    px in twenty steps while it plays, reading the
  //                    playhead and the frame on screen on every animation
  //                    frame from before the hand takes hold to after it
  //                    lets go, and pauses the play
  async "drag while playing"(walk, arg) {
    const { page } = walk;
    const [edge, px] = arg;
    // The play starts a moment into the clip, so however slowly a busy
    // machine moves the hand, the edge it drags stays far ahead of the
    // playhead. Played on from wherever the step before left it, the
    // play came within a frame of the clip's end while the hand was
    // still on its way, the end was dragged back over the playhead, and
    // the playhead went back with it, which is right and not what this
    // step is about.
    const g0 = await handles(page);
    const into = g0.start.x + g0.start.w;
    await clickOnTrack(page, into + 0.05 * (g0.end.x - into), high(g0.track));
    await settle(page);
    await page.evaluate(() => document.body.focus());
    await preview(page, 0.2);
    // Played with the sound card held until the app has settled on the
    // press, see pressPlay. A settle while the clip played in WebKit with
    // motion waited 8 to 12 s for the play, which ran to the clip's end
    // meanwhile, and the drag came after it, issue #715.
    if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]'))) && !(await pressPlay(page))) {
      walk.wrong = "the space bar started no play";
      return;
    }
    // Each wait is in the episode's time, read off the playhead, see
    // playUntil.
    await playUntil(page, { most: 0.7 });
    // The plays started so far, so "played through" can tell whether the
    // drag started the play again.
    const starts = await page.evaluate(() => window.__frameQueue.stats.starts);
    await page.evaluate(() => {
      window.__played = [];
      window.__playWatch = true;
      const tick = () => {
        if (!window.__playWatch) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        // The sound card's clock too, which the playhead follows while
        // it plays, whether the sound card runs, and whether the play is
        // on, see "played through".
        const card = window.__frameQueue?.audio;
        const playing = !!document.querySelector('button[aria-label="Pause"]');
        window.__played.push([performance.now(), at, window.__pictured ? window.__pictured() : -1, card?.currentTime, card?.state === "running", playing]);
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
    await playUntil(page, { most: 0.3 });
    const g = await handles(page);
    const box = edge === "cut" ? g.cutEdges[0] : g[edge];
    if (!box) {
      walk.wrong = `there is no ${edge} edge to drag`;
      return;
    }
    await page.mouse.move(middle(box), box.y + box.h / 2);
    await page.mouse.down();
    for (let i = 1; i <= 20; i++) {
      await page.mouse.move(middle(box) + (px * i) / 20, box.y + box.h / 2);
      await nextPaint(page);
    }
    await page.mouse.up();
    await playUntil(page, { most: 0.5 });
    // Half a second of the episode is thirty animation frames and more
    // on a machine at rest, and on a busy runner in WebKit it was five,
    // too few to prove anything by, #380. So the watch goes on while the
    // play does, until it has read thirty, counted on the page.
    await pageUntil(page, () => window.__played.length >= 30 || !document.querySelector('button[aria-label="Pause"]'));
    await page.evaluate(() => (window.__playWatch = false));
    walk.startedAgain = (await page.evaluate(() => window.__frameQueue.stats.starts)) - starts;
    // Paused in the frame the watch ended, so what comes next settles on
    // a clip that stands. Left to play, the settle waited for its end.
    await playUntil(page, { most: 0, pause: true });
    await settle(page);
  },
  // ["end back past the playhead", share]
  //                    plays the clip from near its end, drags the end
  //                    back to this share of the way along the clip while
  //                    it plays, so the playhead is outside the clip, and
  //                    lets the play run a second past where the clip
  //                    ended before. Tim's rule on #277: the playhead is on
  //                    the video then, and the play goes on as a play of
  //                    the video, never stopping, never going back, no
  //                    older frame drawn, and never started again. The
  //                    playhead waits while the hand is on the end
  async "end back past the playhead"(walk, arg, s) {
    const { page, watch } = walk;
    const oldEnd = (await engineState(page, watch.at)).segments.at(-1).end;
    const g0 = await handles(page);
    const into = g0.start.x + g0.start.w;
    await clickOnTrack(page, into + 0.8 * (g0.end.x - into), high(g0.track));
    await settle(page);
    await page.evaluate(() => document.body.focus());
    await preview(page, 0.2);
    await page.keyboard.press("Space");
    // The play is a fifth of the clip from its end, and on the sound
    // card's clock a busy runner ran it to the end before the hand let
    // go, so the walk had nothing to prove, issue #583. The sound card is
    // held from a moment into the play while the walk settles and the
    // hand takes the end back, as long as the machine needs, and the
    // play goes on from there once it lets go. Held only after the
    // settle, the play had run on through it, #380.
    await playUntil(page, { most: 0.3 });
    await holdSound(page, true);
    await settle(page);
    const starts = await page.evaluate(() => window.__frameQueue.stats.starts);
    await page.evaluate(() => {
      window.__played = [];
      window.__playWatch = true;
      const tick = () => {
        if (!window.__playWatch) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        const playing = !!document.querySelector('button[aria-label="Pause"]');
        window.__played.push([at, window.__pictured ? window.__pictured() : -1, playing]);
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
    const g = await handles(page);
    const to = into + arg * (g.end.x - into);
    // Three moves take the end past the playhead. The steps of a drag in
    // between are what "drag while playing" is about.
    await page.mouse.move(middle(g.end), g.end.y + g.end.h / 2);
    await page.mouse.down();
    for (let i = 1; i <= 3; i++) {
      await page.mouse.move(middle(g.end) + ((to - middle(g.end)) * i) / 3, g.end.y + g.end.h / 2);
      await nextPaint(page);
    }
    await page.mouse.up();
    const [atLetGo, playingAtLetGo] = await page.evaluate(() => [Number(document.querySelector(".screen").dataset.playhead), !!document.querySelector('button[aria-label="Pause"]')]);
    await holdSound(page, false);
    // Until a second past the old end, or until the play stops, and paused
    // there.
    const ran = await playUntil(page, { past: oldEnd + 1, pause: true });
    await page.evaluate(() => (window.__playWatch = false));
    await settle(page);
    await watch.step("trim", s);
    const newEnd = (await engineState(page, watch.at)).segments.at(-1).end;
    // Up to the pause at the end of the step.
    const all = await page.evaluate(() => window.__played ?? []);
    const paused = all.findIndex(([, , on]) => !on);
    const played = paused < 0 ? all : all.slice(0, paused);
    const startedAgain = (await page.evaluate(() => window.__frameQueue.stats.starts)) - starts;
    if (!playingAtLetGo) walk.wrong = `the play had ended at ${atLetGo.toFixed(3)} before the hand let go, so the walk proves nothing`;
    else if (!(newEnd < atLetGo)) walk.wrong = `the clip ends at ${newEnd.toFixed(3)}, not before the playhead at ${atLetGo.toFixed(3)} where the hand let go`;
    else if (!ran.playing) walk.wrong = `the play stopped at ${ran.at.toFixed(3)}, with the clip's end moved from ${oldEnd.toFixed(3)} to ${newEnd.toFixed(3)} behind the playhead`;
    else if (played.length < READS) walk.wrong = `only ${played.length} animation frames were read while it played`;
    else if (startedAgain) walk.wrong = `the play started again ${startedAgain} times once the end was behind the playhead`;
    for (let i = 1; i < played.length && !walk.wrong; i++) {
      const [at, n] = played[i];
      const [at0, n0] = played[i - 1];
      if (at < at0 - 1e-6) walk.wrong = `the playhead went back from ${at0.toFixed(3)} to ${at.toFixed(3)} while it played, with the end moved to ${newEnd.toFixed(3)}`;
      else if (n >= 0 && n0 >= 0 && n < n0) walk.wrong = `frame ${n} was drawn after frame ${n0} while it played`;
    }
  },
  // ["end in and back", share, when, ...menu]
  //                    drags the clip's end in to this share of the way
  //                    along the clip, while the clip plays or, paused,
  //                    before it plays, and takes Undo, or Undo and Redo,
  //                    from the menu bar while it plays. The play starts a
  //                    fifth of the clip before where the end is dragged
  //                    to, or with when "before a cut" a second and a half
  //                    before the first cut, where the decoders already
  //                    work on the piece after it. Tim on #277: the play
  //                    plays the clip as it is now, to its old end after
  //                    Undo and to the new one after Redo, jumping every
  //                    cut, never going back, no older frame drawn, and
  //                    never started again
  async "end in and back"(walk, arg, s) {
    const { page, watch } = walk;
    const [share, when, ...menu] = arg;
    const was = (await engineState(page, watch.at)).segments;
    const oldEnd = was.at(-1).end;
    const from = when === "before a cut" ? was[0].end - 1.5 : was[0].start + (share - 0.2) * (oldEnd - was[0].start);
    const record = () =>
      page.evaluate(() => {
        window.__played = [];
        window.__playWatch = true;
        const tick = () => {
          if (!window.__playWatch) return;
          const at = Number(document.querySelector(".screen")?.dataset.playhead);
          const playing = !!document.querySelector('button[aria-label="Pause"]');
          window.__played.push([at, window.__pictured ? window.__pictured() : -1, playing]);
          requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      });
    const play = async () => {
      await clickOnTrack(page, await xOf(page, from), high((await handles(page)).track));
      await settle(page);
      await page.evaluate(() => document.body.focus());
      await preview(page, 0.2);
      await page.keyboard.press("Space");
      // Played a moment, then held while the walk settles and acts, see
      // below: a busy runner in WebKit took seconds to settle, and the
      // play had run past the end before the hand came, #380.
      await playUntil(page, { most: 0.2 });
      await holdSound(page, true);
      await settle(page);
    };
    const pullIn = async () => {
      const g = await handles(page);
      const into = g.start.x + g.start.w;
      const to = into + share * (g.end.x - into);
      await page.mouse.move(middle(g.end), g.end.y + g.end.h / 2);
      await page.mouse.down();
      for (let i = 1; i <= 3; i++) {
        await page.mouse.move(middle(g.end) + ((to - middle(g.end)) * i) / 3, g.end.y + g.end.h / 2);
        await nextPaint(page);
      }
      await page.mouse.up();
      await settle(page);
    };
    if (when === "paused") await pullIn();
    await play();
    // The play is a fifth of the clip before the new end, and a busy
    // runner in WebKit played it to the end before Undo and Redo were
    // taken, so the walk had nothing to prove, #380. The sound card is
    // held from a moment into the play, see play, while the hand drags
    // and takes the menu, as long as the machine needs, and the play
    // goes on from there once it lets go, the way "end back past the
    // playhead" does, issue #583.
    const starts = await page.evaluate(() => window.__frameQueue.stats.starts);
    await record();
    if (when !== "paused") await pullIn();
    const newEnd = (await engineState(page, watch.at)).segments.at(-1).end;
    for (const m of menu) {
      await page.evaluate((m) => window.__menu(m), m);
      await settle(page);
    }
    const [atBack, playingAtBack] = await page.evaluate(() => [Number(document.querySelector(".screen").dataset.playhead), !!document.querySelector('button[aria-label="Pause"]')]);
    await holdSound(page, false);
    const pieces = (await engineState(page, watch.at)).segments;
    const end = pieces.at(-1).end;
    const want = menu.length % 2 ? oldEnd : newEnd;
    // The clip's play stops by itself at the clip's end, wherever that is.
    await playUntil(page, { past: oldEnd + 1, pause: true });
    await page.evaluate(() => (window.__playWatch = false));
    await settle(page);
    await watch.step("trim", s);
    const all = await page.evaluate(() => window.__played ?? []);
    const stopped = all.findIndex(([, , on]) => !on);
    const played = stopped < 0 ? all : all.slice(0, stopped);
    const reached = Math.max(...played.map(([at]) => at));
    const frame = 1 / (await rate(page));
    const startedAgain = (await page.evaluate(() => window.__frameQueue.stats.starts)) - starts;
    const done = `the end was dragged from ${oldEnd.toFixed(3)} to ${newEnd.toFixed(3)} ${when === "paused" ? "before it played" : "while it played"}, then ${menu.join(" and ")}`;
    if (!(newEnd < oldEnd - 1)) walk.wrong = `the end was dragged in only from ${oldEnd.toFixed(3)} to ${newEnd.toFixed(3)}`;
    else if (Math.abs(end - want) > 1e-3) walk.wrong = `${done}, and the clip ends at ${end.toFixed(3)}, not ${want.toFixed(3)}`;
    else if (!playingAtBack || !(atBack < newEnd - frame)) walk.wrong = `the play was at ${atBack.toFixed(3)}, ${playingAtBack ? "playing" : "stopped"}, after ${menu.join(" and ")}, not before the end at ${newEnd.toFixed(3)}, so the walk proves nothing`;
    else if (played.length < READS) walk.wrong = `only ${played.length} animation frames were read while it played`;
    else if (!(reached > want - 3 * frame && reached < want + frame)) walk.wrong = `${done}, and the play stopped at ${reached.toFixed(3)}, not at the clip's end at ${want.toFixed(3)}`;
    else if (startedAgain) walk.wrong = `${done}, and the play started again ${startedAgain} times`;
    else walk.wrong = inACut(played, pieces, frame);
    for (let i = 1; i < played.length && !walk.wrong; i++) {
      const [at, n] = played[i];
      const [at0, n0] = played[i - 1];
      if (at < at0 - 1e-6) walk.wrong = `the playhead went back from ${at0.toFixed(3)} to ${at.toFixed(3)} while it played`;
      else if (n >= 0 && n0 >= 0 && n < n0) walk.wrong = `frame ${n} was drawn after frame ${n0} while it played`;
    }
  },
  // ["menu while playing", what, share]
  //                    plays the clip from this share of the way along it
  //                    and takes Undo or Redo from the menu bar while it
  //                    plays, which puts a cut back or takes it away ahead
  //                    of the playhead. Tim on #277: the play plays the
  //                    clip as it is now, through where the cut was after
  //                    it is taken away and jumping it once it is put
  //                    back, on to a second past it, never going back, no
  //                    older frame drawn, and never started again
  async "menu while playing"(walk, arg, s) {
    const { page, watch } = walk;
    const [what, share] = arg;
    const before = (await engineState(page, watch.at)).segments;
    const g = await handles(page);
    const into = g.start.x + g.start.w;
    await clickOnTrack(page, into + share * (g.end.x - into), high(g.track));
    await settle(page);
    await page.evaluate(() => document.body.focus());
    await preview(page, 0.2);
    await page.keyboard.press("Space");
    // Played a moment, then held while the walk settles and takes the
    // menu, so the play cannot reach the cut first on a busy machine,
    // see "end in and back".
    await playUntil(page, { most: 0.2 });
    await holdSound(page, true);
    await settle(page);
    const starts = await page.evaluate(() => window.__frameQueue.stats.starts);
    await page.evaluate(() => {
      window.__played = [];
      window.__playWatch = true;
      const tick = () => {
        if (!window.__playWatch) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        const playing = !!document.querySelector('button[aria-label="Pause"]');
        window.__played.push([at, window.__pictured ? window.__pictured() : -1, playing]);
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
    await page.evaluate((m) => window.__menu(m), what);
    await settle(page);
    const [atBack, playingAtBack] = await page.evaluate(() => [Number(document.querySelector(".screen").dataset.playhead), !!document.querySelector('button[aria-label="Pause"]')]);
    await holdSound(page, false);
    const pieces = (await engineState(page, watch.at)).segments;
    // A second past the cut, whichever side of the menu it is on.
    const past = Math.max(...[...cutsOf(before), ...cutsOf(pieces)].map(([, b]) => b)) + 1;
    await playUntil(page, { past, pause: true });
    await page.evaluate(() => (window.__playWatch = false));
    await settle(page);
    await watch.step(what, s);
    const all = await page.evaluate(() => window.__played ?? []);
    const stopped = all.findIndex(([, , on]) => !on);
    const played = stopped < 0 ? all : all.slice(0, stopped);
    const reached = Math.max(...played.map(([at]) => at));
    const frame = 1 / (await rate(page));
    const startedAgain = (await page.evaluate(() => window.__frameQueue.stats.starts)) - starts;
    // The cuts the menu took away, which the play now goes through.
    const gone = cutsOf(before).filter(([a, b]) => !cutsOf(pieces).some(([c, d]) => c === a && d === b));
    const ahead = Math.min(...[...cutsOf(before), ...cutsOf(pieces)].map(([a]) => a));
    if (JSON.stringify(before) === JSON.stringify(pieces)) walk.wrong = `${what} changed nothing in the clip`;
    else if (!playingAtBack || !(atBack < ahead - frame)) walk.wrong = `the play was at ${atBack.toFixed(3)}, ${playingAtBack ? "playing" : "stopped"}, after ${what}, not before the cut at ${ahead.toFixed(3)}, so the walk proves nothing`;
    else if (played.length < READS) walk.wrong = `only ${played.length} animation frames were read while it played`;
    else if (!(reached > past - 0.5)) walk.wrong = `after ${what} the play stopped at ${reached.toFixed(3)}, not past the cut`;
    else if (startedAgain) walk.wrong = `after ${what} the play started again ${startedAgain} times`;
    else if (gone.some(([a, b]) => !played.some(([at]) => at > a + frame && at < b - frame))) walk.wrong = `after ${what} took the cut away the play still jumped it`;
    else walk.wrong = inACut(played, pieces, frame);
    for (let i = 1; i < played.length && !walk.wrong; i++) {
      const [at, n] = played[i];
      const [at0, n0] = played[i - 1];
      if (at < at0 - 1e-6) walk.wrong = `the playhead went back from ${at0.toFixed(3)} to ${at.toFixed(3)} while it played`;
      else if (n >= 0 && n0 >= 0 && n < n0) walk.wrong = `frame ${n} was drawn after frame ${n0} while it played`;
    }
  },
  // ["swipe after click", share, px]
  //                    clicks the clip timeline this share of the way
  //                    along the clip, which puts the playhead under the
  //                    hand, and then swipes with two fingers from there
  //                    three times by px without moving the hand, later in
  //                    the episode for more than nought. The clip has to
  //                    move along the clip timeline the other way on every
  //                    swipe
  async "swipe after click"(walk, arg, s) {
    const { page, watch } = walk;
    const [share, px] = arg;
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    const x = from + share * (g.end.x - from);
    if (!inside(g.track, x)) {
      walk.wrong = `${share} of the way along the clip is not on the clip timeline`;
      return;
    }
    await clickOnTrack(page, x, high(g.track));
    await settle(page);
    // The hand stays where it clicked, on the playhead now, and swipes
    // from there, the way a hand on a trackpad does after it clicks.
    for (let i = 1; i <= 3 && !walk.wrong; i++) {
      const was = (await handles(page)).start.x;
      await page.mouse.wheel(px, 0);
      await nextPaint(page);
      const moved = (await handles(page)).start.x - was;
      if (!(-moved * Math.sign(px) > Math.abs(px) / 3)) {
        walk.wrong = `swipe ${i} by ${px} px after the click moved the clip ${moved.toFixed(1)} px`;
      }
    }
    await settle(page);
    await watch.step("swipe", s);
  },
  // ["cut switch", n]  double-clicks on the clip timeline on the camera
  //                    switch of the video added with "add cameras", which
  //                    cuts a part out across it. n is 1, the one switch
  async "cut switch"(walk, arg, s) {
    const { page, watch } = walk;
    const at = walk.cameraSwitch;
    const g = await handles(page);
    await page.mouse.dblclick(await xOf(page, at), high(g.track));
    await watch.step("cut", s);
  },
  // ["trim past", edge, n]
  //                    drags the clip's "start" or "end" edge two seconds
  //                    past that camera switch, so the camera beyond it is
  //                    gone
  async "trim past"(walk, arg, s) {
    const { page, watch } = walk;
    const [edge] = arg;
    const at = walk.cameraSwitch;
    const g = await handles(page);
    const to = await xOf(page, edge === "end" ? at - 2 : at + 2);
    await drag(page, middle(g[edge]), high(g.track), to - middle(g[edge]), false);
    await watch.step("trim", s);
  },
  // ["pieces", n]      the clip timeline draws the clip in n pieces
  async pieces(walk, arg) {
    const { page } = walk;
    const n = await page.locator(".clip-timeline .piece").count();
    if (n !== arg) walk.wrong = `the clip timeline draws ${pieceCount(n)}, not ${arg}`;
  },
  // ["cuts", n]        the clip timeline shows n cuts
  async cuts(walk, arg) {
    const { page } = walk;
    const t = await timeline(page);
    if (t?.cuts.length !== arg) walk.wrong = `the clip timeline shows ${t?.cuts.length ?? "no"} cuts, not ${arg}`;
  },
  // ["time field"]     the row under the clip timeline does not say the
  //                    chosen clip's title, and its time field, as tall as
  //                    every control, says where the frame that holds the
  //                    playhead starts, to the millisecond: on every step
  //                    of a drag along the clip timeline with the button
  //                    held, standing as wide as it stood, and after each
  //                    of three presses of →, one frame on each time
  async "time field"(walk) {
    const { page } = walk;
    const { fps, videoStart = 0, duration = 0 } = await ask(page, "Source", await episodeOn(page));
    // Said here the way the field must say it, from the frame, so the
    // walk takes nobody's word for what the frame is: hours for a
    // video of an hour or more, and the minutes always two figures.
    const said = (at) => {
      const k = Math.floor((Math.max(at - videoStart, 0) + 0.001) * fps);
      const ms = Math.round((videoStart + k / fps) * 1000);
      const two = (n) => String(n).padStart(2, "0");
      const h = Math.floor(ms / 3600000);
      const m = Math.floor(ms / 60000) % 60;
      const rest = `${two(Math.floor(ms / 1000) % 60)}.${String(ms % 1000).padStart(3, "0")}`;
      return duration >= 3600 ? `${h}:${two(m)}:${rest}` : `${two(m + h * 60)}:${rest}`;
    };
    const read = () =>
      page.evaluate(() => {
        const f = document.querySelector(".detail .row .timecode");
        const b = f?.getBoundingClientRect();
        return {
          text: f?.textContent.trim() ?? null,
          w: b?.width ?? 0,
          h: b?.height ?? 0,
          control: parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--control-h")),
          at: Number(document.querySelector(".screen")?.dataset.playhead),
          row: document.querySelector(".detail .row")?.textContent ?? "",
          title: document.querySelector(".pick.current .title")?.textContent.trim() ?? "",
        };
      });
    const first = await read();
    if (first.text === null) {
      walk.wrong = "the row under the clip timeline has no time field";
      return;
    }
    if (first.title && first.row.includes(first.title)) {
      walk.wrong = `the row under the clip timeline still says the clip's title, "${first.title}"`;
      return;
    }
    if (Math.abs(first.h - first.control) > 0.01) {
      walk.wrong = `the time field is ${first.h.toFixed(2)} px tall, not ${first.control} as every control`;
      return;
    }
    // Down on the clip, a third of the way along, and along it with
    // the button held, each step a frame of its own.
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    const x = from + (g.end.x - from) / 3;
    const y = high(g.track);
    await page.mouse.move(x, y);
    await page.mouse.down();
    const seen = new Set();
    for (let i = 0; i <= 12 && !walk.wrong; i++) {
      if (i) await page.mouse.move(x + i * 8, y);
      await nextPaint(page, 2);
      const r = await read();
      seen.add(r.at);
      if (r.text !== said(r.at)) {
        walk.wrong = `${i ? `${i * 8} px into the drag` : "pressed on the clip"}, with the button held, the playhead is at ${r.at.toFixed(4)} s and the time field says ${r.text}, not ${said(r.at)}`;
      } else if (Math.abs(r.w - first.w) > 0.01) {
        walk.wrong = `the time field was ${first.w.toFixed(2)} px wide and is ${r.w.toFixed(2)} px at ${r.text}`;
      }
    }
    await page.mouse.up();
    await settle(page);
    if (walk.wrong) return;
    if (seen.size < 4) {
      walk.wrong = `the drag along the clip timeline moved the playhead to ${seen.size} places, so it proves nothing`;
      return;
    }
    // One frame on with each press of the arrow.
    await page.evaluate(() => document.body.focus());
    let was = await read();
    for (let i = 1; i <= 3 && !walk.wrong; i++) {
      await page.keyboard.press("ArrowRight");
      await nextPaint(page, 2);
      const r = await read();
      const step = Math.round((r.at - was.at) * fps);
      if (step !== 1) walk.wrong = `→ ${i} moved the playhead ${step} frames, from ${was.at.toFixed(4)} to ${r.at.toFixed(4)} s`;
      else if (r.text !== said(r.at)) walk.wrong = `after → ${i} the playhead is at ${r.at.toFixed(4)} s and the time field says ${r.text}, not ${said(r.at)}`;
      was = r;
    }
  },
  // ["waveform"]       the clip timeline draws the waveform across the whole
  //                    of its view within ten seconds, read from its canvas
  async waveform(walk) {
    const { page } = walk;
    // The canvas itself, not what the interface was told: a column of
    // it with more than a few pixels painted is the waveform there.
    // The bridge's episode has sound from its first second to its last.
    // Read once the Go side has measured all of it, which it says in
    // the state of the video it sends, and the interface has heard that
    // and drawn it: caughtUp waits behind every state due to be sent.
    const path = await episodeOn(page);
    for (let seen = await news(page, "video"); !(await ask(page, "Episode", path))?.measuredAll; ) {
      seen = await nextNews(page, seen, "video");
    }
    await caughtUp(page, url);
    const read = await page.evaluate(() => {
      const c = document.querySelector(".clip-timeline .track canvas");
      if (!c?.width || !c.height) return null;
      const d = c.getContext("2d").getImageData(0, 0, c.width, c.height).data;
      let drawn = 0;
      for (let x = 0; x < c.width; x++) {
        let painted = 0;
        for (let y = 0; y < c.height; y++) if (d[(y * c.width + x) * 4 + 3] > 0) painted++;
        if (painted > 6) drawn++;
      }
      return { drawn, width: c.width };
    });
    if (!read) walk.wrong = "the clip timeline has no canvas";
    else if (read.drawn < read.width * 0.9) {
      walk.wrong = `the clip timeline draws the waveform in ${read.drawn} of its ${read.width} columns`;
    }
  },
  // ["rulers fit", s]  the range picker has lines and times on it, every
  //                    one inside the episode of s seconds, and the clip
  //                    timeline shows that episode and nothing past it
  async "rulers fit"(walk, arg) {
    const { page } = walk;
    // Waited for while the workspace of an episode just added opens,
    // and then looked at once: looked at until it held, it held as
    // soon as the clip came, and passed on the old clip timeline.
    const seconds = (time) => time.split(":").reduce((sum, part) => sum * 60 + Number(part), 0);
    // Read until it holds or both are drawn, each time the page
    // changes, see seenUntil.
    const look = async () => {
      let wrong = "";
      const read = await page.evaluate(() => {
        const picker = document.querySelector('[aria-label="The range picker"]');
        const text = (e) => (e.textContent ?? "").trim();
        return {
          lines: picker?.querySelectorAll(".tick").length ?? 0,
          times: [...(picker?.querySelectorAll(".time:not(.start)") ?? [])].map(text),
          timeline: [...document.querySelectorAll(".clip-timeline .time")].map(text),
        };
      });
      const picked = read.times.map(seconds);
      const shown = read.timeline.map(seconds);
      if (read.lines < 2 || picked.length < 2) {
        wrong = `the range picker has ${read.lines} lines and the times ${JSON.stringify(read.times)}`;
      } else if (picked.some((t) => t <= 0 || t >= arg)) {
        wrong = `the range picker has times outside the episode: ${JSON.stringify(read.times)}`;
      } else if (!shown.length || Math.max(...shown) > arg || Math.max(...shown) < arg - 2) {
        wrong = `the clip timeline shows the times ${JSON.stringify(read.timeline)} for an episode of ${arg} s`;
      }
      return { wrong, drawn: read.lines > 0 && shown.length > 0 };
    };
    ({ wrong: walk.wrong } = await seenUntil(page, url, look, (r) => !r.wrong || r.drawn));
  },
  // ["made on the clip"]
  //                    since "make while playing", the clip made was the
  //                    chosen card from the frame it came on, on its way
  //                    and then written, and on every animation frame from
  //                    then on, playing and paused, the playhead was on it:
  //                    the clip timeline did not dim it
  async "made on the clip"(walk) {
    if (!walk.made) {
      walk.wrong = "made on the clip with no clip made";
      return;
    }
    const { playing, was, written, overlays } = walk.made;
    if (!written) {
      walk.wrong = "the clip made was never written and chosen while it played";
      return;
    }
    // From the frame the clip made is the chosen card on.
    const from = overlays.findIndex((o) => o.card && o.card !== was);
    const since = overlays.slice(from);
    const dim = since.find((o) => o.video);
    if (playing && !since.some((o) => o.playing && o.card === written)) {
      walk.wrong = "the play had stopped before the clip made was written";
    } else if (!since.some((o) => !o.playing)) {
      walk.wrong = "the play was not paused at the end";
    } else if (dim) {
      const when = dim.card === written ? "written" : dim.card ? "on its way" : "with no card chosen";
      walk.wrong = `the clip made was dimmed at ${dim.at.toFixed(3)}, ${dim.playing ? "playing" : "paused"}, ${when}`;
    }
  },
  // ["nailed"]         since "press while playing", the playhead stood
  //                    still under the hand while it held still, on the
  //                    press and again where the hand moved to, the frame
  //                    that holds it came and nothing was drawn after it,
  //                    the button said Pause the whole time, and the play
  //                    went on from where the hand let go
  async nailed(walk) {
    const { page } = walk;
    const { played, hand } = await page.evaluate(() => ({ played: window.__played ?? [], hand: window.__hand ?? {} }));
    const fps = await rate(page);
    const during = (from, to) => played.filter(([t]) => t >= from && t < to);
    // The frame that holds the playhead, a frame that begins a
    // millisecond or less after it included, see frameAt in
    // lib/flow.ts, as the bars carry it, its number modulo 2 ** BITS.
    const holds = (at) => Math.floor((at + 0.001) * fps) % 2 ** BITS;
    // How many frames had been read by a moment of the walk's.
    const readBy = (t) => played.filter(([at]) => at < t).length;
    // Each still stretch is read from the second animation frame after
    // the page heard the hand, from, given as the frames read by then:
    // the press puts the playhead under the hand at once, in the first
    // frame after it, see lib/scrub.ts, and from there it stands. The walk reads each frame as it begins,
    // before the app's own work in it, so the first frame it reads
    // after the hand still shows the playhead from before. Read from
    // 200 ms after the hand instead, a busy runner whose frames came a
    // quarter of a second apart read that first frame, and the playhead
    // seemed to move under a hand that held still. The picture takes as
    // long as the machine needs to decode the frame under the hand, and
    // the frames passing on the way are drawn. So the picture is read
    // from where its frame landed, which has to be within the stretch,
    // and from there nothing else is drawn.
    const stood = (from, to, what) => {
      const got = played.slice(from + 1).filter(([t]) => t < to);
      if (got.length < 10) return `only ${got.length} animation frames were read ${what}`;
      const ats = new Set(got.map(([, at]) => at));
      if (ats.size > 1) {
        const all = got.map(([, at]) => at);
        return `the playhead moved from ${Math.min(...all).toFixed(3)} to ${Math.max(...all).toFixed(3)} ${what}`;
      }
      const at = got[0][1];
      const landed = got.findIndex(([, , n]) => n === holds(at));
      if (landed < 0 || got.length - landed < 3) {
        return `frame ${holds(at)}, which holds the playhead at ${at.toFixed(3)}, was not drawn ${what}, frames ${[...new Set(got.map(([, , n]) => n))].join(", ")} were`;
      }
      const other = got.slice(landed).find(([, , n]) => n !== holds(at));
      if (other) return `frame ${other[2]} was drawn after frame ${holds(at)} had landed ${what}`;
      return at;
    };
    // From when the page heard the press, and the last move, which a
    // busy runner delivered long after the walk sent them.
    const first = stood(Math.max(readBy(hand.down), hand.heardDown ?? 0), hand.moves, "while the hand held still on the press");
    if (typeof first === "string") {
      walk.wrong = first;
      return;
    }
    const second = stood(Math.max(readBy(hand.still), hand.heardMove ?? 0), hand.up, "while the hand held still where it moved to");
    if (typeof second === "string") {
      walk.wrong = second;
      return;
    }
    if (!(second < first)) {
      walk.wrong = `the playhead did not follow the hand, it stood at ${first.toFixed(3)} and then at ${second.toFixed(3)}`;
      return;
    }
    if (during(hand.down, hand.up).some(([, , , playing]) => !playing)) {
      walk.wrong = "the button said Play while the hand held the playhead";
      return;
    }
    // And after the hand let go, from the second frame after the page
    // heard it, the same way.
    const after = played.slice(Math.max(readBy(hand.up), hand.heardUp ?? 0) + 1);
    if (after.length < 10) {
      walk.wrong = `only ${after.length} animation frames were read after the hand let go`;
      return;
    }
    const ats = after.map(([, at]) => at);
    if (!after.every(([, , , playing]) => playing)) walk.wrong = "the play did not go on after the hand let go";
    else if (ats[ats.length - 1] <= ats[0]) walk.wrong = "the playhead stood still after the hand let go";
    else if (ats[0] < second - 1e-6 || ats[0] > second + 1) {
      walk.wrong = `the play went on from ${ats[0].toFixed(3)}, not from ${second.toFixed(3)} where the hand let go`;
    }
  },
  // ["swipe keeps the canvas", n]
  //                    swipes along the clip timeline n times with two
  //                    fingers, and the waveform's canvas is never made new
  //                    on the way, only drawn again
  async "swipe keeps the canvas"(walk, arg) {
    const { page } = walk;
    await page.evaluate(() => {
      const canvas = document.querySelector(".clip-timeline canvas");
      window.__madeNew = 0;
      for (const side of ["width", "height"]) {
        const own = Object.getOwnPropertyDescriptor(HTMLCanvasElement.prototype, side);
        Object.defineProperty(canvas, side, {
          configurable: true,
          get() {
            return own.get.call(this);
          },
          set(v) {
            window.__madeNew++;
            own.set.call(this, v);
          },
        });
      }
    });
    const line = await page.locator(".clip-timeline").boundingBox();
    await page.mouse.move(line.x + line.width / 2, line.y + line.height * 0.3);
    for (let i = 0; i < arg; i++) await page.mouse.wheel(i % 2 ? -40 : 40, 0);
    await settle(page);
    const made = await page.evaluate(() => window.__madeNew);
    if (made) walk.wrong = `the waveform's canvas was made new ${made} times in ${arg} swipes`;
  },
  // ["rotate at", share]
  //                    puts the playhead in the clip, this share of the
  //                    way along it, with a click on the clip timeline, and
  //                    presses R, which rotates the part of the clip there,
  //                    and the video preview has to show it rotated in the
  //                    first frame drawn after the press, or the one after
  async "rotate at"(walk, arg, s) {
    const { page, watch } = walk;
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    await clickOnTrack(page, from + arg * (g.end.x - from), high(g.track));
    await settle(page);
    // R works on the frame the video preview shows, inShort in
    // Episode.svelte, and the turn read is that frame's, so R is
    // pressed once it is the playhead's frame, as S is, see keyframe
    // at. On a busy machine R was pressed while the video preview
    // still showed the frame from before the click, issue #320.
    await shownOnPlayhead(page);
    const turn = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.turn));
    const before = await turn();
    const frames = await keyShows(page, "r", (b) => Number(document.querySelector(".screen").dataset.turn) !== b, before);
    if (frames < 0) walk.wrong = `R left the video preview rotated ${before}`;
    else if (frames > 1) walk.wrong = `the video preview rotated ${frames} frames after R, not at once`;
    await watch.step("press", s);
  },
  // ["keyframe at", share]
  //                    puts the playhead in the clip, this share of the
  //                    way along it, with a click on the clip timeline,
  //                    waits for the video preview to show that frame, and
  //                    presses S, and the clip timeline has to show a
  //                    diamond lit on the playhead's frame in the first
  //                    frame drawn after the press, or the one after it
  // ["glide at", share]
  //                    the same with G, and a circle
  async "keyframe at"(walk, arg, s) {
    await keyframeAt(walk, arg, s, "s");
  },
  async "glide at"(walk, arg, s) {
    await keyframeAt(walk, arg, s, "g");
  },
  // ["keyframe key", [key, kind]]
  //                    with the playhead on a keyframe, presses S or G,
  //                    and the mark on the playhead's frame has to become a
  //                    "diamond" or a "circle", or go, "none", in the
  //                    first frame drawn after the press or the one after
  //                    it, and the engine has to have it so
  async "keyframe key"(walk, arg, s) {
    const { page, watch } = walk;
    const [key, kind] = arg;
    const here = () =>
      page.evaluate(() => {
        const e = document.querySelector(".clip-timeline .cropdot.here");
        return e ? (e.dataset.glide ? "circle" : "diamond") : "none";
      });
    const on = await chosen(page);
    const circles = async () =>
      ((await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.cropMarks ?? []).filter((m) => m.glide).length;
    const had = await circles();
    const before = await here();
    if (before === "none") {
      walk.wrong = `the playhead is on no keyframe for ${key.toUpperCase()} to change`;
      return;
    }
    const frames = await keyShows(
      page,
      key,
      (want) => {
        const e = document.querySelector(".clip-timeline .cropdot.here");
        return (e ? (e.dataset.glide ? "circle" : "diamond") : "none") === want;
      },
      kind,
    );
    if (frames < 0) walk.wrong = `${key.toUpperCase()} on the ${before} left it a ${await here()}, not a ${kind}`;
    else if (frames > 1) walk.wrong = `the ${kind} showed ${frames} frames after ${key.toUpperCase()}, not at once`;
    await watch.step("press", s);
    await settle(page);
    // The engine has one circle more or fewer, as the mark became.
    const want = had + (kind === "circle") - (before === "circle");
    const has = await circles();
    if (!walk.wrong && has !== want) walk.wrong = `the engine has ${has} circles after ${key.toUpperCase()} on the ${before}, not ${want}`;
  },
  // ["glides", n]      the clip timeline draws n glide lines along the top
  //                    rule, each from a circle to the next circle, as
  //                    thick as the rule, on it, brightening into the
  //                    second circle, and under the marks
  async glides(walk, arg) {
    const { page } = walk;
    await settle(page);
    const got = await page.evaluate(() => {
      const box = (e) => e.getBoundingClientRect();
      const dots = [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => ({ x: box(e).left + box(e).width / 2, glide: !!e.dataset.glide }));
      const rule = document.querySelector(".clip-timeline .frames .frame");
      return {
        dots,
        rule: rule ? { top: box(rule).top, line: parseFloat(getComputedStyle(rule).borderTopWidth) } : null,
        lines: [...document.querySelectorAll(".clip-timeline .glide")].map((e) => {
          const css = getComputedStyle(e);
          return { left: box(e).left, right: box(e).right, top: box(e).top, height: box(e).height, image: css.backgroundImage, z: Number(css.zIndex) };
        }),
        markZ: Number(getComputedStyle(document.querySelector(".clip-timeline .cropdot") ?? document.body).zIndex),
      };
    });
    if (got.lines.length !== arg) {
      walk.wrong = `the clip timeline draws ${got.lines.length} glide lines, not ${arg}`;
      return;
    }
    const near = (a, b) => Math.abs(a - b) <= 1.5;
    for (const l of got.lines) {
      const into = got.dots.find((d) => d.glide && near(d.x, l.right));
      const from = got.dots.find((d) => d.glide && near(d.x, l.left));
      if (!into) walk.wrong = `a glide line ends at ${l.right.toFixed(1)} px, on no circle`;
      else if (!from) walk.wrong = `a glide line starts at ${l.left.toFixed(1)} px, on no circle`;
      else if (got.rule && (!near(l.top, got.rule.top) || !near(l.height, got.rule.line)))
        walk.wrong = `a glide line stands ${l.top.toFixed(1)} px down and ${l.height.toFixed(1)} px thick, not on the top rule at ${got.rule.top.toFixed(1)} px, ${got.rule.line} px thick`;
      else if (!l.image.startsWith("linear-gradient")) walk.wrong = `a glide line is drawn with ${l.image}, not brightening into its circle`;
      else if (!(l.z < got.markZ)) walk.wrong = `a glide line is drawn over the keyframes`;
    }
  },
  // ["glide under the hand", share]
  //                    holds the clip's start and drags it in to this
  //                    share of the way along the clip, past the first
  //                    circle, and with the hand still down and every
  //                    place answered, the first circle rides on the start
  //                    edge and the glide line starts there, where both
  //                    stay once let go. Then drags the start back out to
  //                    where it was, and the circle and the line are on
  //                    the circle's own place again, held and let go.
  //                    The circle riding on the edge is lit as soon as the
  //                    answer to the hand is drawn. Found by Tim on #750,
  //                    the line stood where it was until the hand let go
  //                    of a cut's edge, and on #778, the keyframes did, and
  //                    the circle on the edge flickered
  async "glide under the hand"(walk, arg, s) {
    const { page, watch } = walk;
    const read = () =>
      page.evaluate(() => {
        const left = (e) => e.getBoundingClientRect().left;
        const frame = document.querySelector(".clip-timeline .frames .frame");
        const middle = (e) => {
          const r = e.getBoundingClientRect();
          return r.left + r.width / 2;
        };
        return {
          start: frame ? left(frame) : NaN,
          lines: [...document.querySelectorAll(".clip-timeline .glide")].map(left),
          circles: [...document.querySelectorAll(".clip-timeline .cropdot[data-glide]")].map(middle),
        };
      });
    const near = (a, b) => Math.abs(a - b) <= 1.5;
    const before = await read();
    if (before.lines.length !== 1 || before.circles.length !== 2) {
      walk.wrong = `the clip timeline draws ${before.lines.length} glide lines and ${before.circles.length} circles before the start is dragged, not 1 and 2`;
      return;
    }
    const drag = async (to, done) => {
      const g = await handles(page);
      const x = middle(g.start);
      const y = g.start.y + g.start.h / 2;
      await page.mouse.move(x, y);
      await page.mouse.down();
      for (let i = 1; i <= 3; i++) {
        await page.mouse.move(x + ((to - x) * i) / 3, y);
        await nextPaint(page);
      }
      // Every place the hand went answered and drawn, which the clip
      // timeline says by dropping data-shaping, see "drag edge twice".
      // Whether the first circle is lit is read in the same moment, before
      // the video preview has shown the frame the answer put the playhead
      // on: the circle riding on the edge is lit by the answer that put it
      // there, or it flickers, Tim on #778.
      const litWhenAnswered = await page.evaluate(
        () =>
          new Promise((answered) => {
            const timeline = document.querySelector(".clip-timeline");
            const lit = () => !!timeline.querySelector(".cropdot[data-glide]")?.classList.contains("here");
            if (!timeline.hasAttribute("data-shaping")) return answered(lit());
            const seen = new MutationObserver(() => {
              if (timeline.hasAttribute("data-shaping")) return;
              seen.disconnect();
              answered(lit());
            });
            seen.observe(timeline, { attributes: true, attributeFilter: ["data-shaping"] });
          }),
      );
      await nextPaint(page);
      const held = await read();
      await page.mouse.up();
      await settle(page);
      const left = await read();
      const wrong = (got) =>
        got.lines.length !== 1 || got.circles.length !== 2
          ? `${got.lines.length} glide lines and ${got.circles.length} circles`
          : !done(got)
            ? `the glide line starting at ${got.lines[0].toFixed(1)} px and the first circle at ${got.circles[0].toFixed(1)} px, the clip at ${got.start.toFixed(1)} px`
            : "";
      const whileHeld = wrong(held);
      const once = wrong(left);
      if (whileHeld) return `with the start ${to < x ? "dragged back out" : "dragged in"} and still held, the clip timeline draws ${whileHeld}`;
      if (to > x && !litWhenAnswered) return "with the start dragged in and still held, the first circle riding on it is not lit when the answer to the hand is drawn";
      if (once) return `with the start ${to < x ? "dragged back out" : "dragged in"} and let go, the clip timeline draws ${once}`;
      if (!near(held.lines[0], left.lines[0])) return `the glide line stood at ${held.lines[0].toFixed(1)} px while the start was held and at ${left.lines[0].toFixed(1)} px once let go`;
      return "";
    };
    const g = await handles(page);
    const into = g.start.x + g.start.w;
    walk.wrong = await drag(
      into + arg * (g.end.x - into),
      (got) => near(got.lines[0], got.start) && near(got.circles[0], got.start) && !near(got.lines[0], before.lines[0]),
    );
    if (!walk.wrong)
      walk.wrong = await drag(
        middle(g.start),
        (got) => near(got.lines[0], before.lines[0]) && near(got.circles[0], before.circles[0]),
      );
    await watch.step("drag", s);
  },
  // ["keyframe on", "last"]
  //                    a click on the clip's end edge, which leaves the
  //                    playhead on the end with the last frame on the
  //                    video preview, then S: a keyframe comes on the
  //                    clip's last frame and its diamond lights, and S again
  //                    removes it
  // ["keyframe on", "first"]
  //                    a click on the clip's start edge and three steps
  //                    on and three back with the arrow keys, which leave
  //                    the playhead a hair off the first frame's start
  //                    with that frame on the video preview, then the same
  async "keyframe on"(walk, arg) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const src = await ask(page, "Source", on.path);
    const pieces = (await entry()).segments;
    const want = arg === "last" ? pieces[pieces.length - 1].end - 1 / src.fps : pieces[0].start;
    const g = await handles(page);
    const edge = arg === "last" ? g.end : g.start;
    await clickOnTrack(page, edge.x + edge.w / 2, edge.y + edge.h / 2);
    await settle(page);
    if (arg === "first") {
      for (const key of ["ArrowRight", "ArrowRight", "ArrowRight", "ArrowLeft", "ArrowLeft", "ArrowLeft"]) {
        await page.keyboard.press(key);
        await settle(page);
      }
    }
    // S puts the keyframe on the frame the video preview shows, so it is
    // pressed once the playhead stands on the clip's frame and the video
    // preview shows that frame, both the app's own word. settle does not
    // count the frame on its way from the decoder, and under load the key once
    // came while the video preview still showed the clip's start, #559.
    await pageUntil(
      page,
      ([want, fps, start]) => {
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        const begins = start + Math.floor((at - start + 0.001) * fps) / fps;
        return Math.abs(begins - want) < 0.5 / fps;
      },
      [want, src.fps, src.videoStart ?? 0],
    );
    await shownOnPlayhead(page);
    const marks = async () => (await entry()).cropMarks ?? [];
    const before = (await marks()).length;
    const lit = () => page.evaluate(() => !!document.querySelector(".clip-timeline .cropdot.here"));
    await page.keyboard.press("s");
    await settle(page);
    const got = await marks();
    const near = got.find((m) => Math.abs(m.at - want) < 0.5 / src.fps);
    if (got.length <= before) {
      const at = await page.evaluate(() => document.querySelector(".screen").dataset.playhead);
      walk.wrong = `S put no keyframe with the playhead at ${Number(at).toFixed(4)} s, on the clip's ${arg} frame at ${want.toFixed(4)} s`;
      return;
    }
    if (!near) {
      walk.wrong = `S put a keyframe at ${got.map((m) => m.at).join(", ")}, not on the clip's ${arg} frame at ${want.toFixed(3)} s`;
      return;
    }
    await settle(page);
    if (!(await lit())) {
      walk.wrong = `the keyframe on the clip's ${arg} frame does not light while that frame is on the video preview`;
      return;
    }
    await page.keyboard.press("s");
    await settle(page);
    if ((await marks()).length > before) walk.wrong = `S again did not remove the keyframe on the clip's ${arg} frame`;
    await settle(page);
  },
  // ["thumbnail at", share]
  //                    puts the playhead in the clip, this share of the
  //                    way along it, with a click on the clip timeline,
  //                    waits for the video preview to show that frame, and
  //                    presses T, and the engine has a thumbnail more
  async "thumbnail at"(walk, arg, s) {
    const { page, watch } = walk;
    const on = await chosen(page);
    const count = async () => ((await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.thumbnails ?? []).length;
    const before = await count();
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    await clickOnTrack(page, from + arg * (g.end.x - from), high(g.track));
    await settle(page);
    // T makes a thumbnail of the frame the video preview shows, the
    // frame a person sees, so it is pressed once that is the playhead's
    // frame. Pressed before, on a loaded machine, T took the thumbnail
    // off the frame still shown from the step before (2.261).
    await shownOnPlayhead(page);
    await page.keyboard.press("t");
    await watch.step("press", s);
    await settle(page);
    if ((await count()) !== before + 1) walk.wrong = "T made no thumbnail";
  },
  // ["under the hand", kind]
  //                    moves the pointer onto the first "thumbnail" or
  //                    "crop" mark on the clip timeline: the mark is what
  //                    is under the pointer, it takes the hover, and it
  //                    lights the way it does under the hand
  async "under the hand"(walk, arg) {
    const { page } = walk;
    // A mark the playhead is not on, so only the hand can light it,
    // tagged so it is the same element all the way through.
    const kind = arg === "crop" ? ".cropdot" : ".thumb";
    const tagged = await page.evaluate((kind) => {
      const el = [...document.querySelectorAll(`.clip-timeline ${kind}`)].find((m) => !m.classList.contains("here"));
      el?.setAttribute("data-probe", "");
      return !!el;
    }, kind);
    const box = tagged ? await page.locator("[data-probe]").boundingBox() : null;
    if (!box) {
      walk.wrong = `the clip timeline shows no ${arg} mark off the playhead`;
      return;
    }
    const paint = () =>
      page.evaluate(() => {
        const el = document.querySelector("[data-probe]");
        const look = getComputedStyle(el);
        return `${look.backgroundColor} ${look.color}`;
      });
    const away = await paint();
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    await page.mouse.move(x, y);
    await settle(page);
    const got = await page.evaluate(
      ({ x, y }) => {
        const el = document.querySelector("[data-probe]");
        return { top: document.elementFromPoint(x, y)?.closest("[data-probe]") === el, hover: el.matches(":hover") };
      },
      { x, y },
    );
    const lit = await paint();
    await page.evaluate(() => document.querySelector("[data-probe]")?.removeAttribute("data-probe"));
    if (!got.top) walk.wrong = `something else lies over the ${arg} mark, so the pointer never reaches it`;
    else if (!got.hover) walk.wrong = `the ${arg} mark does not take the hover`;
    else if (lit === away) walk.wrong = `the ${arg} mark looks the same under the hand as away from it, ${away}`;
  },
  // ["answers late", ms]
  //                    every answer of the engine to a gesture on the clip
  //                    timeline, while the hand moves, comes ms late, the
  //                    way it does on a busy machine
  async "answers late"(walk, arg) {
    const { page } = walk;
    await page.evaluate((ms) => {
      const real = window.fetch;
      window.fetch = async (input, init) => {
        if (String(input).endsWith("/call") && init?.body && String(JSON.parse(init.body).name).split(".").pop() === "Shape") {
          await new Promise((r) => setTimeout(r, ms));
        }
        return real(input, init);
      };
    }, arg);
  },
  // ["playhead at", share]
  //                    a click on the clip timeline puts the playhead that
  //                    share of the way from the clip's start to its end,
  //                    and the video preview shows the frame there
  async "playhead at"(walk, arg) {
    await playheadAt(walk.page, arg);
  },
  // ["crop before", share]
  //                    puts the playhead that share of the way into the
  //                    clip and keeps where the crop frame stands there in
  //                    the video preview
  // ["crop kept", share]
  //                    with the playhead put there again, the crop frame
  //                    stands where it stood at ["crop before", share], so
  //                    what came between left that part alone, or put it
  //                    back
  async "crop before"(walk, arg) {
    const { page } = walk;
    await playheadAt(page, arg);
    walk.cropBefore = { ...walk.cropBefore, [arg]: await cropShown(page) };
  },
  async "crop kept"(walk, arg) {
    const { page } = walk;
    const was = walk.cropBefore?.[arg];
    if (was === undefined) {
      walk.wrong = `no crop was kept at ${arg} of the clip to compare with`;
      return;
    }
    await playheadAt(page, arg);
    const now = await cropShown(page);
    if (now !== was) walk.wrong = `${arg} of the way into the clip the crop frame stood at ${was} and stands at ${now} now`;
  },
  // ["playhead on", edge]
  //                    the playhead is on the frame the clip's "start" or
  //                    "end" edge is on
  async "playhead on"(walk, arg) {
    const { page } = walk;
    const on = await chosen(page);
    const clip = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const fps = await rate(page);
    const want = arg === "start" ? Math.floor((clip.start + 0.001) * fps) : Math.floor((clip.end + 0.001) * fps) - 1;
    const at = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
    const got = Math.floor((at + 0.001) * fps);
    if (got !== want) walk.wrong = `the playhead is at ${at} s, frame ${got}, not on the clip's ${arg} at frame ${want}, ${clip[arg]} s`;
  },
  // ["remove mark", kind]
  //                    double-clicks the first "thumbnail" or "crop" mark
  //                    on the clip timeline, and the engine has one fewer,
  //                    and no cut more
  async "remove mark"(walk, arg, s) {
    const { page, watch } = walk;
    const on = await chosen(page);
    const entry = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const count = (c) => (arg === "crop" ? c?.cropMarks ?? [] : c?.thumbnails ?? []).length;
    const was = await entry();
    const box = await page.locator(arg === "crop" ? ".clip-timeline .cropdot" : ".clip-timeline .thumb").first().boundingBox();
    if (!box) {
      walk.wrong = `the clip timeline shows no ${arg} mark`;
      return;
    }
    await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
    await watch.step("double-click", s);
    await settle(page);
    const now = await entry();
    if (count(now) !== count(was) - 1) walk.wrong = `a double-click left the clip ${count(now)} ${arg} marks, not ${count(was) - 1}`;
    else if (now.segments.length !== was.segments.length) walk.wrong = `a double-click on the ${arg} mark cut the clip`;
  },
  // ["press under, then on the keyframe"]
  //                    presses the track high over the waveform at the
  //                    first keyframe's x and then, at once, the keyframe's
  //                    diamond on the top rule over it: two presses at one x
  //                    but far apart in height are no double-click, so the
  //                    engine still has the keyframe and no cut more, #535
  async "press under, then on the keyframe"(walk, _arg, s) {
    const { page, watch } = walk;
    const on = await chosen(page);
    const entry = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const was = await entry();
    const dot = await page.locator(".clip-timeline .cropdot").first().boundingBox();
    if (!dot) {
      walk.wrong = "the clip timeline shows no keyframe";
      return;
    }
    const x = dot.x + dot.width / 2;
    const y = high((await handles(page)).track);
    if (y - (dot.y + dot.height / 2) < 12) walk.wrong = `the track is pressed ${y - (dot.y + dot.height / 2)} px under the keyframe, too near to tell`;
    // Pressed one after the other with nothing between, well inside the
    // half second of a double-click, so on main the second press removed
    // the keyframe.
    await clickOnTrack(page, x, y);
    await clickOnTrack(page, x, dot.y + dot.height / 2);
    await watch.step("press", s);
    await settle(page);
    const now = await entry();
    const marks = (c) => (c?.cropMarks ?? []).length;
    if (marks(now) !== marks(was)) walk.wrong = `a press under the keyframe and then on it left the clip ${marks(now)} keyframes, not ${marks(was)}`;
    else if (now.segments.length !== was.segments.length) walk.wrong = "a press under the keyframe and then on it cut the clip";
  },
  // ["on the playhead", n]
  //                    n marks are lit by the playhead, a thumbnail or a
  //                    keyframe on its frame, and each stands centred on
  //                    the playhead's line
  async "on the playhead"(walk, arg) {
    const { page } = walk;
    // Found by Tim: a keyframe lit by the playhead stood a pixel to the
    // right of it and a thumbnail half a pixel, as he saw it on his Mac.
    // The playhead was placed on the track's whole box and the marks on
    // its inside, a pixel in from either side, and the playhead on its
    // moment where a thumbnail stood on the middle of its frame and a
    // keyframe on its start. So up close, where a frame is many pixels
    // wide, they stood apart by as much. Plan row 2.234.
    const m = await page.evaluate(() => {
      const mid = (e) => {
        const b = e.getBoundingClientRect();
        return b.left + b.width / 2;
      };
      const line = document.querySelector(".clip-timeline .playhead");
      const head = document.querySelector(".clip-timeline .head") ?? line;
      return {
        line: line ? mid(line) : NaN,
        head: mid(head),
        lit: [...document.querySelectorAll(".clip-timeline .thumb.here, .clip-timeline .cropdot.here")].map((e) => ({
          kind: e.classList.contains("thumb") ? "thumbnail" : "keyframe",
          at: mid(e),
        })),
      };
    });
    const off = m.lit.find((l) => Math.abs(l.at - m.line) > 0.01);
    if (!Number.isFinite(m.line)) walk.wrong = "the clip timeline shows no playhead";
    else if (m.lit.length !== arg) walk.wrong = `${m.lit.length} marks light on the playhead's frame, not ${arg}`;
    else if (off) walk.wrong = `the ${off.kind} lit by the playhead stands ${(off.at - m.line).toFixed(2)} px from its line`;
    else if (Math.abs(m.head - m.line) > 0.01) walk.wrong = `the playhead's head stands ${(m.head - m.line).toFixed(2)} px from its line`;
  },
  // ["pinch in"]       pinches the clip timeline in to a tenth on the
  //                    playhead, where a frame is many pixels wide, and
  //                    takes the hand off it
  async "pinch in"(walk) {
    const { page } = walk;
    // Up close, pinched in on the playhead to a tenth, where a frame is
    // many pixels wide, and the hand taken off the clip timeline.
    const g = await handles(page);
    const line = await page.evaluate(() => {
      const b = document.querySelector(".clip-timeline .playhead")?.getBoundingClientRect();
      return b ? b.left + b.width / 2 : NaN;
    });
    await page.mouse.move(Number.isFinite(line) ? line : g.track.x + g.track.w / 2, high(g.track));
    await page.keyboard.down("Control");
    await page.mouse.wheel(0, -230);
    await page.keyboard.up("Control");
    await settle(page);
    await page.mouse.move(g.track.x + g.track.w / 2, g.track.y + g.track.h + 80);
    await settle(page);
  },
  // ["keyframe past", edge]
  //                    drags the first keyframe's diamond well past the
  //                    clip's "end" or "start" edge and lets go: the engine
  //                    has it on the clip's last or first frame, and the
  //                    diamond never stood further out while it was held
  //                    than where it stays once it is let go
  async "keyframe past"(walk, arg, s) {
    const { page, watch } = walk;
    const on = await chosen(page);
    const entry = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const src = await ask(page, "Source", on.path);
    const pieces = (await entry()).segments;
    const want = arg === "end" ? pieces[pieces.length - 1].end - 1 / src.fps : pieces[0].start;
    const centre = () =>
      page.evaluate(() => {
        const b = document.querySelector(".clip-timeline .cropdot").getBoundingClientRect();
        return b.left + b.width / 2;
      });
    const dot = await page.locator(".clip-timeline .cropdot").first().boundingBox();
    const g = await handles(page);
    const y = dot.y + dot.height / 2;
    const x = dot.x + dot.width / 2;
    const to = arg === "end" ? g.end.x + g.end.w + 60 : g.start.x - 60;
    await pressOnTrack(page, x, y);
    const held = [];
    for (let i = 1; i <= 12; i++) {
      await page.mouse.move(x + ((to - x) * i) / 12, y);
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
      held.push(await centre());
    }
    await page.mouse.up();
    await settle(page);
    const got = (await entry()).cropMarks ?? [];
    const left = await centre();
    const out = arg === "end" ? Math.max(...held) - left : left - Math.min(...held);
    if (!got.some((m) => Math.abs(m.at - want) < 0.5 / src.fps))
      walk.wrong = `the keyframe dragged past the clip's ${arg} is at ${got.map((m) => m.at).join(", ")}, not on its ${arg === "end" ? "last" : "first"} frame at ${want.toFixed(3)} s`;
    else if (out > 0.25)
      walk.wrong = `the keyframe stood ${out.toFixed(2)} px past where it stays while it was held past the clip's ${arg}, and jumped back when let go`;
    else if (Math.abs(held[held.length - 1] - left) > 0.25)
      walk.wrong = `the keyframe was held at ${held[held.length - 1].toFixed(2)} px and stays at ${left.toFixed(2)} px once let go`;
    await watch.step("drag", s);
  },
  // ["keyframes", n]  the engine has n keyframes on the clip, at as many
  //                    left edges, and the clip timeline draws n
  //                    diamonds, each 18 px wide, and every mark and the
  //                    playhead with its middle on a whole pixel
  async keyframes(walk, arg) {
    const { page } = walk;
    const on = await chosen(page);
    const marks = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.cropMarks ?? [];
    await settle(page);
    const got = await marks();
    const dots = await page.evaluate(() => document.querySelectorAll(".clip-timeline .cropdot").length);
    // Drawn as a diamond, the shape editors draw a keyframe in, and
    // the keyframe button's own: Lucide's diamond, filled, as a
    // thumbnail is Lucide's camera filled. Plan row 2.234. One the crop
    // glides into is a circle, #750.
    const shapes = await page.evaluate(() =>
      [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => e.querySelector("svg path")?.getAttribute("d") ?? "nothing"),
    );
    const diamond = (d) => d.startsWith("M2.7 10.3a2.41 2.41 0 0 0 0 3.41l7.59 7.59");
    const sizes = await page.evaluate(() => [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => e.getBoundingClientRect().width));
    // Each mark and the playhead with its middle on a whole pixel, so the
    // Mac draws a mark on the playhead's own pixel: between two it drew
    // the diamond half a pixel left of the playhead on its frame, found by
    // Tim on #750.
    const between = await page.evaluate(() =>
      [...document.querySelectorAll(".clip-timeline .mark, .clip-timeline .playhead")]
        .map((e) => {
          const b = e.getBoundingClientRect();
          return b.left + b.width / 2;
        })
        .filter((x) => Math.abs(x - Math.round(x)) > 0.01),
    );
    if (between.length) walk.wrong = `a mark or the playhead stands between two pixels, its middle at ${between.map((x) => x.toFixed(3)).join(", ")} px`;
    else if (got.length !== arg) walk.wrong = `the engine has ${got.length} keyframes, not ${arg}`;
    else if (new Set(got.map((m) => m.x)).size !== arg) walk.wrong = `the keyframes stand at ${got.map((m) => m.x).join(", ")}, not each at its own place`;
    else if (dots !== arg) walk.wrong = `the clip timeline draws ${dots} keyframes, not ${arg}`;
    else if (shapes.some((d, i) => diamond(d) === !!got[i]?.glide))
      walk.wrong = `a keyframe is not drawn as its kind, a diamond to switch and a circle to glide: ${shapes.join(", ")}`;
    else if (sizes.some((w) => w !== 18)) walk.wrong = `a keyframe is ${sizes.join(", ")} px wide, not 18 px like a thumbnail, found too small by Tim on #192`;
  },
  // ["scrub", from, to] reads the frame on the video preview's canvas, by
  //                    its bands, and how far the video preview turns it on
  //                    every animation frame, while the hand presses the
  //                    clip timeline this share of the way along the clip
  //                    and drags the playhead quickly back and forth three
  //                    times over the middle of the clip, where its cut is,
  //                    and then on to that share
  async scrub(walk, arg, s) {
    const { page, watch } = walk;
    const [from, to] = arg;
    const g = await handles(page);
    const left = g.start.x + g.start.w;
    const span = g.end.x - left;
    const y = high(g.track);
    await page.evaluate(
      ([bits, band]) => {
        // The frame number of a filmed video standing up by its display
        // matrix: its bands run down the right side, the highest bit
        // furthest right, see filmed-30s-25-turned.mp4 in
        // internal/fixtures/fixtures_test.go.
        const c = document.querySelector(".screen canvas");
        const s = document.createElement("canvas");
        const t = s.getContext("2d", { willReadFrequently: true });
        const read = () => {
          if (!c || !c.width || !c.height) return -1;
          const h = Math.max(1, Math.floor(c.height / 2));
          s.width = 1;
          s.height = h;
          let n = 0;
          for (let b = 0; b < bits; b++) {
            const x = Math.floor(c.width * (1 - ((b + 0.5) * band) / 180));
            t.drawImage(c, x, Math.floor(c.height / 4), 1, h, 0, 0, 1, h);
            const col = t.getImageData(0, 0, 1, h).data;
            let sum = 0;
            for (let i = 0; i < col.length; i += 4) sum += col[i];
            n = n * 2 + (sum / (col.length / 4) > 110 ? 1 : 0);
          }
          return n;
        };
        window.__scrubbed = [];
        window.__scrubbing = true;
        const tick = () => {
          if (!window.__scrubbing) return;
          const el = document.querySelector(".screen");
          window.__scrubbed.push([Number(el.dataset.turn), read(), Number(el.dataset.playhead)]);
          requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      },
      [11, 4],
    );
    // Over the cut and back, quickly, the way a hand looks for a frame,
    // so the playhead crosses it more often than its frames keep up.
    const path = [from, 0.42, 0.62, 0.42, 0.62, 0.42, 0.62, to];
    await page.mouse.move(left + from * span, y);
    await page.mouse.down();
    for (let leg = 1; leg < path.length; leg++) {
      const [a, b] = [path[leg - 1], path[leg]];
      for (let i = 1; i <= 12; i++) {
        await page.mouse.move(left + (a + ((b - a) * i) / 12) * span, y);
        await nextPaint(page);
      }
    }
    await page.mouse.up();
    // And the frames after the hand let go, until the picture stands
    // on the frame it let go on.
    await settle(page);
    await page.evaluate(() => window.__standing());
    await nextPaint(page, 2);
    await page.evaluate(() => (window.__scrubbing = false));
    await watch.step("scrub", s);
  },
  // ["slow drag left", n]
  //                    presses the clip timeline near the clip's end and
  //                    drags the playhead to the left a pixel every frame
  //                    of the page, the way a slow hand does, until its
  //                    frame is n frames back, then lets go. The frames
  //                    under the hand come from no more new streams than
  //                    one for every 15 frames back and one more: each
  //                    step back was a stream of its own, a seek, and the
  //                    picture stood for each, issue #596
  async "slow drag left"(walk, arg, s) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const g = await handles(page);
    const y = high(g.track);
    const at = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    // The cards' frames read first, which a drag leaves for after it, so
    // the streams counted are the drag's own: on a busy runner the cards
    // were still to come, and their streams were counted too.
    await page.evaluate(() => window.__readAheadRested());
    let x = g.end.x - 0.1 * (g.end.x - g.start.x - g.start.w);
    await page.mouse.move(x, y);
    await page.mouse.down();
    await settle(page);
    const from = Math.floor((await at()) * fps + 1e-6);
    const was = await page.evaluate(() => window.__appFrames.stats.streams);
    const calls = await page.evaluate(() => window.__calls.length);
    let back = 0;
    while (back < arg && x > g.start.x + g.start.w) {
      x -= 1;
      await page.mouse.move(x, y);
      await nextPaint(page);
      back = from - Math.floor((await at()) * fps + 1e-6);
    }
    await page.mouse.up();
    await settle(page);
    const streams = (await page.evaluate(() => window.__appFrames.stats.streams)) - was;
    await watch.step("scrub", s);
    if (back < arg) walk.wrong = `the drag reached the clip's start ${back} frames back, not ${arg}`;
    else if (streams > Math.ceil(back / 15) + 1) {
      const why = await page.evaluate((n) => window.__appFrames.stats.newFor.slice(-n), Math.min(16, streams));
      // What the video preview said of its streams meanwhile, in the
      // app's log: one closed on the Go side or to make room, or ended.
      const told = await page.evaluate(
        (from) =>
          window.__calls
            .slice(from)
            .filter((c) => c.name === "Said")
            .flatMap((c) => (c.args[0] ?? []).map((l) => l.text))
            .filter((t) => /stream/.test(t))
            .slice(-20),
        calls,
      );
      walk.wrong = `a slow drag ${back} frames to the left, from frame ${from}, opened ${streams} streams, more than one for every 15 frames back and one more: for ${why.join(", ")}. The video preview said: ${told.join(" | ")}`;
    }
  },
  // ["skip outside a clip"]
  //                    with no clip on the playhead, presses Shift and the
  //                    right arrow until the playhead is 5 s past where the
  //                    saved transcript ends, then Shift and the left arrow
  //                    once. Every press moves the playhead 5 s, over what
  //                    was heard and what was not alike
  async "skip outside a clip"(walk, arg, s) {
    const { page, watch } = walk;
    const path = await episodeOn(page);
    // Where the saved transcript ends: the end of its first part.
    const heard = (await ask(page, "Episode", path))?.heard ?? [];
    const edge = Math.max(0, ...heard.filter(([a]) => a < 1).map(([, b]) => b));
    const at = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    const press = async (key) => {
      const before = await at();
      await page.keyboard.press(key);
      await settle(page);
      await watch.step("press", s, page);
      return [before, await at()];
    };
    await page.evaluate(() => document.body.focus());
    if (await chosen(page)) {
      walk.wrong = "a clip is chosen, so the playhead may be on one";
      return;
    }
    // Enough presses to cross the end of what was heard and one more.
    const presses = Math.ceil(edge / 5) + 2;
    for (let i = 0; i < presses; i++) {
      const [from, to] = await press("Shift+ArrowRight");
      if (Math.abs(to - from - 5) > 0.002) {
        walk.wrong = `Shift and the right arrow at ${from.toFixed(3)} s went to ${to.toFixed(3)} s, not 5 s on, with what was heard ending at ${edge.toFixed(3)} s`;
        return;
      }
    }
    const [from, to] = await press("Shift+ArrowLeft");
    if (Math.abs(from - to - 5) > 0.002) walk.wrong = `Shift and the left arrow at ${from.toFixed(3)} s went to ${to.toFixed(3)} s, not 5 s back`;
  },
  // ["skips in sight", n]
  //                    clicks the clip timeline a second after the chosen
  //                    clip's end, presses Shift and the right arrow n
  //                    times and then the left arrow n times, and after
  //                    every press the playhead is in the clip timeline's
  //                    view: a skip out of it turns the page, the way a
  //                    play does
  async "skips in sight"(walk, arg, s) {
    const { page, watch } = walk;
    const t = await timeline(page);
    await clickOnTrack(page, await xOf(page, t.end + 1), high((await handles(page)).track));
    await settle(page);
    await page.evaluate(() => document.body.focus());
    const at = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    for (const key of [...Array(arg).fill("Shift+ArrowRight"), ...Array(arg).fill("Shift+ArrowLeft")]) {
      const from = await at();
      await page.keyboard.press(key);
      await settle(page);
      await watch.step("press", s, page);
      const to = await at();
      const seen = await page.evaluate(() => !!document.querySelector(".clip-timeline .playhead"));
      if (Math.abs(to - from) < 1) {
        walk.wrong = `${key} at ${from.toFixed(3)} s went to ${to.toFixed(3)} s, not a skip`;
        return;
      }
      if (!seen) {
        walk.wrong = `${key} from ${from.toFixed(3)} s to ${to.toFixed(3)} s left the playhead out of the clip timeline's view`;
        return;
      }
    }
  },
  // ["shift in a cut"] clicks the clip timeline in the middle of the
  //                    chosen clip's first cut and presses Shift and the
  //                    right arrow: a cut is inside the clip, so the press
  //                    walks to the next word, never the 5 s a skip
  //                    outside a clip goes
  async "shift in a cut"(walk, arg, s) {
    const { page, watch } = walk;
    const pieces = (await engineState(page, watch.at)).segments;
    if (pieces.length < 2) {
      walk.wrong = `the clip has ${pieces.length} piece, so no cut to press in`;
      return;
    }
    const middle = (pieces[0].end + pieces[1].start) / 2;
    await clickOnTrack(page, await xOf(page, middle), high((await handles(page)).track));
    await settle(page);
    const at = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    const from = await at();
    await page.evaluate(() => document.body.focus());
    await page.keyboard.press("Shift+ArrowRight");
    await settle(page);
    await watch.step("press", s, page);
    const to = await at();
    if (from < pieces[0].end || from > pieces[1].start) walk.wrong = `the click in the cut from ${pieces[0].end.toFixed(3)} to ${pieces[1].start.toFixed(3)} s put the playhead at ${from.toFixed(3)} s`;
    else if (!(to > from && to - from < 4.5)) walk.wrong = `Shift and the right arrow in the cut at ${from.toFixed(3)} s went to ${to.toFixed(3)} s, not to the next word`;
  },
};

// Steps that take the same body as the step they are walked by.
steps["scrub far"] = steps["click far"];

// keyframeAt puts the playhead this share of the way along the clip with
// a click on the clip timeline, waits for the video preview to show that
// frame, and presses key, S or G, and the clip timeline has to show a new
// mark lit on the playhead's frame in the first frame drawn after the press
// or the one after it, a diamond for S and a circle for G.
async function keyframeAt(walk, share, s, key) {
  const { page, watch } = walk;
  const g = await handles(page);
  const from = g.start.x + g.start.w;
  await clickOnTrack(page, from + share * (g.end.x - from), high(g.track));
  await settle(page);
  // The key puts the keyframe on the frame the video preview shows, so it
  // is pressed once that is the playhead's frame, see thumbnail at.
  await shownOnPlayhead(page);
  const count = () => page.evaluate(() => document.querySelectorAll(".clip-timeline .cropdot").length);
  const before = await count();
  const frames = await keyShows(
    page,
    key,
    ([b, glide]) => {
      const here = document.querySelector(".clip-timeline .cropdot.here");
      return document.querySelectorAll(".clip-timeline .cropdot").length > b && !!here && !!here.dataset.glide === glide;
    },
    [before, key === "g"],
  );
  const kind = key === "g" ? "circle" : "diamond";
  if (frames < 0) walk.wrong = `${key.toUpperCase()} put no ${kind} on the playhead's frame`;
  else if (frames > 1) walk.wrong = `the ${kind} showed ${frames} frames after ${key.toUpperCase()}, not at once`;
  await watch.step("press", s);
  await settle(page);
}

// How many animation frames the page drew after a key went down before
// the first one on which shows holds: 0 when the frame after the key
// shows it. Counted in frames, not in milliseconds: on a busy runner one
// frame can take a third of a second, and C was 538 ms and then 332 ms
// "after C" in WebKit in CI, where a frame or two take that long. The
// count starts when the page handles the keydown, so it is how the app
// answered the key, not how long a busy machine took to hand it over. -1
// when it does not hold once the app has settled on the key.
async function keyShows(page, key, shows, arg) {
  await page.evaluate(
    ([source, arg]) => {
      const shows = new Function(`return (${source})`)();
      window.__keyShows = { down: false, frames: 0, shown: false };
      addEventListener("keydown", () => (window.__keyShows.down = true), { capture: true, once: true });
      const tick = () => {
        const k = window.__keyShows;
        if (!k.down) return requestAnimationFrame(tick);
        if (shows(arg)) k.shown = true;
        else {
          k.frames++;
          requestAnimationFrame(tick);
        }
      };
      requestAnimationFrame(tick);
    },
    [shows.toString(), arg],
  );
  await page.keyboard.press(key);
  await settle(page);
  return page.evaluate(() => (window.__keyShows.shown ? window.__keyShows.frames : -1));
}

// Reads the edge of that label, the playhead and the frame the video
// preview shows on every animation frame, all in the same frame, until
// __followWatch is set false: the edge into __edgeSeen, and the three into
// __edgeFrames, the edge as NaN on a frame it is not drawn.
function watchEdge(page, label) {
  return page.evaluate((label) => {
    window.__edgeSeen = [];
    window.__edgeFrames = [];
    window.__followWatch = true;
    // Its own, so a watch that was stopped stops even when the next has
    // begun by its next frame, as watchPills does. On a machine too busy
    // to draw a frame between the two, it read the new watch's
    // __followWatch and went on reading its own edge into the new watch.
    const id = (window.__followId ?? 0) + 1;
    window.__followId = id;
    const tick = () => {
      if (!window.__followWatch || window.__followId !== id) return;
      const screen = document.querySelector(".screen");
      const playhead = Number(screen?.dataset.playhead);
      const shown = Number(screen?.dataset.shown);
      const edge = document.querySelector(`.clip-timeline [aria-label="${label}"]`);
      const at = edge ? Number(edge.getAttribute("aria-valuenow")) : NaN;
      if (edge) window.__edgeSeen.push(at);
      window.__edgeFrames.push([at, playhead, shown]);
      requestAnimationFrame(tick);
    };
    tick();
  }, label);
}

// Reads on every animation frame, until "captions stayed" stops it, the
// number of the frame on the canvas of the video preview, see reader, and
// how many pixels of the canvas's lower half, where the captions stand,
// are white, as libass draws their letters. The picture of the bridge's
// episode is grey and never that light. Its own, so a watch that was
// stopped stops even when the next has begun, as watchEdge does.
async function watchCaptions(page) {
  await reader(page);
  await page.evaluate(() => {
    const watch = { seen: [], on: true };
    window.__captionWatch = watch;
    const s = document.createElement("canvas");
    const g = s.getContext("2d", { willReadFrequently: true });
    const look = () => {
      if (!watch.on) return;
      requestAnimationFrame(look);
      const c = document.querySelector(".screen canvas");
      if (!c?.width || !c.height) return;
      const top = Math.floor(c.height / 2);
      s.width = c.width;
      s.height = c.height - top;
      g.drawImage(c, 0, top, s.width, s.height, 0, 0, s.width, s.height);
      const d = g.getImageData(0, 0, s.width, s.height).data;
      let ink = 0;
      for (let i = 0; i < d.length; i += 4) if (d[i] > 245 && d[i + 1] > 245 && d[i + 2] > 245) ink++;
      watch.seen.push([window.__pictured(), ink, !!document.querySelector(".atop .frame")]);
    };
    look();
  });
}

// The cuts between a clip's pieces, each from where a piece ends to where
// the next starts.
function cutsOf(pieces) {
  return pieces.slice(1).map((p, i) => [pieces[i].end, p.start]);
}

// The animation frames a play has to be read on before a walk judges it.
// On the walk's clock a play goes on a frame a refresh at most, so every
// frame it draws is read, and two reads are a play seen moving. Thirty
// were asked once, half a second of a play in real time, and a play on
// the walk's clock through a few frames of the clip makes fewer.
const READS = 2;

// What is wrong when a play recorded as [playhead, frame, playing] stood
// inside a cut of the pieces, more than a frame from its edges: it played
// a part the clip cuts out. Nothing when it jumped every cut.
function inACut(played, pieces, frame) {
  for (const [a, b] of cutsOf(pieces)) {
    const inside = played.find(([at]) => at > a + frame && at < b - frame);
    if (inside) return `the playhead stood at ${inside[0].toFixed(3)} while it played, inside the cut from ${a.toFixed(3)} to ${b.toFixed(3)}`;
  }
  return null;
}

// Puts the playhead that share of the way from the clip's start to its end
// by a click on the clip timeline, once the video preview shows the frame
// there.
async function playheadAt(page, share) {
  const g = await handles(page);
  const from = g.start.x + g.start.w;
  await clickOnTrack(page, from + share * (g.end.x - from), high(g.track));
  await settle(page);
  await shownOnPlayhead(page);
}

// Where the crop frame stands in the video preview, in pixels of the
// picture.
function cropShown(page) {
  return page.evaluate(() => Number(document.querySelector(".screen").dataset.crop));
}
