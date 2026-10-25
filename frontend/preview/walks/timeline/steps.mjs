// The steps of the sequences about the clip timeline: edges, cuts, trims,
// marks, keyframes and the playhead under the hand. Each is said above it,
// a list with its name first. walkSequence in ../sequences.mjs walks it,
// and hands it the walk so far, the step's argument, what the caption box
// showed before it and the step's name. A sequence of any folder can take
// it. Below them the helpers only these steps use. See docs/TESTING.md.
import { execFileSync } from "node:child_process";
import { BITS, ask, caughtUp, chosen, clickOnTrack, drag, engineState, episodeOn, handles, high, hold, inside, letHeld, middle, news, nextNews, nextPaint, playUntil, pressOnTrack, preview, seenUntil, settle, shownOnPlayhead, timeline, xOf } from "../bridge.mjs";
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
    await page.mouse.down();
    for (let i = 1; i <= 20; i++) {
      await page.mouse.move(x + (px * i) / 20, y);
      await nextPaint(page);
    }
    await page.mouse.up();
    if (walk.dragShift) await page.keyboard.up("Shift");
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
      await page.mouse.click(await xOf(page, t.start + 0.4), y);
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
      return { shown: window.__scrubShown, keys: n > 0 ? (q?.keysAt.slice(-n) ?? []) : [], count: n };
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
    await page.mouse.click(await xOf(page, t.start + 0.5), high(g.track));
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
  //                    that one exactly, and the frame it ended on, with
  //                    the button down and after it was let go, is the
  //                    frame under the hand, by the number drawn on it
  async "keyed on the way"(walk) {
    if (!walk.scrubbed) {
      walk.wrong = "nothing was scrubbed";
      return;
    }
    const { fps, shown, keys, key, rested, after } = walk.scrubbed;
    const frameOf = (at) => Math.floor((at + 0.001) * fps);
    const under = frameOf(rested.at);
    // What the frame queue put up, rather than what was painted: on a
    // busy machine the frame under the hand can come before the next
    // paint, and then the key frame is rightly never painted.
    if (!keys.some((at) => Math.abs(at - key) < 0.5 / fps)) {
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
    // after it.
    const seen = () =>
      page.evaluate(() => {
        window.__pressWatch = false;
        return window.__pressedAt < 0 ? [] : window.__pressed.slice(window.__pressedAt);
      });
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
    await page.mouse.click(middle(box), y);
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
  //                    lets go
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
    await page.mouse.click(into + 0.05 * (g0.end.x - into), high(g0.track));
    await settle(page);
    await page.evaluate(() => document.body.focus());
    await preview(page, 0.2);
    if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
      await page.keyboard.press("Space");
    }
    // Each wait is in the episode's time, read off the playhead, see
    // playUntil, once the app has settled on the press.
    await settle(page);
    await playUntil(page, { most: 0.7 });
    await page.evaluate(() => {
      window.__played = [];
      window.__playWatch = true;
      const tick = () => {
        if (!window.__playWatch) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        window.__played.push([performance.now(), at, window.__pictured ? window.__pictured() : -1]);
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
    await page.evaluate(() => (window.__playWatch = false));
    await settle(page);
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
    await page.mouse.click(x, high(g.track));
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
    // a levels event, and the interface has heard that and drawn it.
    const path = await episodeOn(page);
    for (let seen = await news(page, "levels"); !(await ask(page, "Episode", path))?.measuredAll; ) {
      seen = await nextNews(page, seen, "levels");
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
    ({ wrong } = await seenUntil(page, url, look, (r) => !r.wrong || r.drawn));
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
    // Each still stretch is read from 200 ms after the hand stopped:
    // the press puts the playhead under the hand at once, and from
    // there it stands. The picture takes as long as the machine needs
    // to decode the frame under the hand, and the frames passing on
    // the way are drawn, so on a busy machine a frame was still on its
    // way 200 ms in. So the picture is read from where its frame
    // landed, which has to be within the stretch, and from there
    // nothing else is drawn.
    const stood = (from, to, what) => {
      const got = during(from + 200, to);
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
    const first = stood(hand.down, hand.moves, "while the hand held still on the press");
    if (typeof first === "string") {
      walk.wrong = first;
      return;
    }
    const second = stood(hand.still, hand.up, "while the hand held still where it moved to");
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
    const after = during(hand.up + 300, Infinity);
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
  //                    and the video preview has to show it rotated within
  //                    300 ms, before a held answer comes
  async "rotate at"(walk, arg, s) {
    const { page, watch } = walk;
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    await clickOnTrack(page, from + arg * (g.end.x - from), high(g.track));
    await settle(page);
    // R works on the frame the video preview shows, inShort in
    // Episode.svelte, and the turn read is that frame's, so R is
    // pressed once it is the playhead's frame, as C is, see keyframe
    // at. On a busy machine R was pressed while the video preview
    // still showed the frame from before the click, issue #320.
    await shownOnPlayhead(page);
    const turn = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.turn));
    const before = await turn();
    const took = await keyShows(page, "r", (b) => Number(document.querySelector(".screen").dataset.turn) !== b, before);
    if (took < 0) walk.wrong = `R left the video preview rotated ${before}`;
    else if (took > 300) walk.wrong = `the video preview rotated ${took} ms after R, not at once`;
    await watch.step("press", s);
  },
  // ["keyframe at", share]
  //                    puts the playhead in the clip, this share of the
  //                    way along it, with a click on the clip timeline,
  //                    waits for the video preview to show that frame, and
  //                    presses C, and the clip timeline has to show a crop
  //                    mark lit on the playhead's frame within 300 ms,
  //                    before a held answer comes
  async "keyframe at"(walk, arg, s) {
    const { page, watch } = walk;
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    await clickOnTrack(page, from + arg * (g.end.x - from), high(g.track));
    await settle(page);
    // C puts the keyframe on the frame the video preview shows, so it
    // is pressed once that is the playhead's frame, see thumbnail at.
    await shownOnPlayhead(page);
    const count = () => page.evaluate(() => document.querySelectorAll(".clip-timeline .cropdot").length);
    const before = await count();
    const took = await keyShows(
      page,
      "c",
      (b) => document.querySelectorAll(".clip-timeline .cropdot").length > b && !!document.querySelector(".clip-timeline .cropdot.here"),
      before,
    );
    if (took < 0) walk.wrong = "C put no keyframe on the playhead's frame";
    else if (took > 300) walk.wrong = `the keyframe showed ${took} ms after C, not at once`;
    await watch.step("press", s);
    await settle(page);
  },
  // ["keyframe on", "last"]
  //                    a click on the clip's end edge, which leaves the
  //                    playhead on the end with the last frame on the
  //                    video preview, then C: a keyframe comes on the
  //                    clip's last frame and its diamond lights, and C again
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
    await page.mouse.click(edge.x + edge.w / 2, edge.y + edge.h / 2);
    await settle(page);
    if (arg === "first") {
      for (const key of ["ArrowRight", "ArrowRight", "ArrowRight", "ArrowLeft", "ArrowLeft", "ArrowLeft"]) {
        await page.keyboard.press(key);
        await settle(page);
      }
    }
    const marks = async () => (await entry()).cropMarks ?? [];
    const before = (await marks()).length;
    const lit = () => page.evaluate(() => !!document.querySelector(".clip-timeline .cropdot.here"));
    await page.keyboard.press("c");
    await settle(page);
    const got = await marks();
    const near = got.find((m) => Math.abs(m.at - want) < 0.5 / src.fps);
    if (got.length <= before) {
      const at = await page.evaluate(() => document.querySelector(".screen").dataset.playhead);
      walk.wrong = `C put no keyframe with the playhead at ${Number(at).toFixed(4)} s, on the clip's ${arg} frame at ${want.toFixed(4)} s`;
      return;
    }
    if (!near) {
      walk.wrong = `C put a keyframe at ${got.map((m) => m.at).join(", ")}, not on the clip's ${arg} frame at ${want.toFixed(3)} s`;
      return;
    }
    await settle(page);
    if (!(await lit())) {
      walk.wrong = `the keyframe on the clip's ${arg} frame does not light while that frame is on the video preview`;
      return;
    }
    await page.keyboard.press("c");
    await settle(page);
    if ((await marks()).length > before) walk.wrong = `C again did not remove the keyframe on the clip's ${arg} frame`;
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
  //                    diamonds, each 16 px wide
  async keyframes(walk, arg) {
    const { page } = walk;
    const on = await chosen(page);
    const marks = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.cropMarks ?? [];
    await settle(page);
    const got = await marks();
    const dots = await page.evaluate(() => document.querySelectorAll(".clip-timeline .cropdot").length);
    // Drawn as a diamond, the shape editors draw a keyframe in, and
    // the keyframe button's own: Lucide's diamond, filled, as a
    // thumbnail is Lucide's camera filled. Plan row 2.234.
    const shapes = await page.evaluate(() =>
      [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => e.querySelector("svg path")?.getAttribute("d") ?? "nothing"),
    );
    const sizes = await page.evaluate(() => [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => e.getBoundingClientRect().width));
    if (got.length !== arg) walk.wrong = `the engine has ${got.length} keyframes, not ${arg}`;
    else if (new Set(got.map((m) => m.x)).size !== arg) walk.wrong = `the keyframes stand at ${got.map((m) => m.x).join(", ")}, not each at its own place`;
    else if (dots !== arg) walk.wrong = `the clip timeline draws ${dots} keyframes, not ${arg}`;
    else if (shapes.some((c) => !c.startsWith("M2.7 10.3a2.41 2.41 0 0 0 0 3.41l7.59 7.59")))
      walk.wrong = `a keyframe is not drawn as a diamond: ${shapes.join(", ")}`;
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
        // furthest right, see makeTurnedEpisode in bridge_test.go.
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
};

// Steps that take the same body as the step they are walked by.
steps["scrub far"] = steps["click far"];

// How long the page took from a key going down to the first animation
// frame on which shows holds, in the page's own time. Timed from Node, a
// press and the polling that waits for its answer are calls to the
// browser, and on a busy runner each can take a frame or more to arrive:
// C was once 538 ms "after C" in CI where the page showed the keyframe in
// a frame or two. The time starts when the page handles the keydown, so
// it is how long the app took to answer the key, not how long a busy
// machine took to hand the key over. -1 when it does not hold once the
// app has settled on the key.
async function keyShows(page, key, shows, arg) {
  await page.evaluate(
    ([source, arg]) => {
      const shows = new Function(`return (${source})`)();
      window.__keyShows = { down: -1, took: -1 };
      addEventListener("keydown", () => (window.__keyShows.down = performance.now()), { capture: true, once: true });
      const tick = () => {
        const k = window.__keyShows;
        if (k.down < 0) return requestAnimationFrame(tick);
        if (shows(arg)) k.took = performance.now() - k.down;
        else requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    },
    [shows.toString(), arg],
  );
  await page.keyboard.press(key);
  await settle(page);
  return page.evaluate(() => (window.__keyShows.took >= 0 ? Math.round(window.__keyShows.took) : -1));
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
