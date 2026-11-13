// The rules that hold whatever was done, checked after every step of a walk
// and of a sequence. They compare the screen with the engine's own answer,
// and know nothing of what a correction should do, so they cannot drift
// into a third engine. What an edit does to the engine's captions, and
// what Undo puts back, are the Go side's rules, tested in Go, see
// cmd/framefairy-app/captionedits_test.go. See docs/TESTING.md.
import { letHeld, engineState, settle, settledScreen } from "./bridge.mjs";

// The engine's clip as one comparable value: its captions, how they look,
// and its pieces with their crops, everything of it the history takes
// back, see docs/APP.md, Undo and redo.
export const same = (a, b) =>
  JSON.stringify(a.captions) === JSON.stringify(b.captions) &&
  JSON.stringify(a.style) === JSON.stringify(b.style) &&
  JSON.stringify(a.segments) === JSON.stringify(b.segments);
export const editing = (s) => s.some((w) => w.focused);
export const keyed = (s) => s.find((w) => w.keyed);

export function describe(a, b) {
  const line = (caps) =>
    caps.captions.map((c) => `[${c.lines.flatMap((l) => l.words.map((w) => w.text)).join(" ")}]`).join(" ");
  const pieces = (s) =>
    (s.segments ?? [])
      .map((p) => `${p.start.toFixed(2)}-${p.end.toFixed(2)}${p.cropX === null ? "" : ` crop ${p.cropX}`}${p.marks?.length ? ` ${p.marks.length} keyframes` : ""}${p.rotate ? ` turned ${p.rotate}` : ""}`)
      .join(" ");
  const look = (s) => (s.style ? `${s.style.font} ${s.style.chosenSize} ${s.style.primary} ${s.style.box}` : "none");
  return `before ${line(a)}\n       pieces ${pieces(a)}\n       look ${look(a)}\nafter  ${line(b)}\n       pieces ${pieces(b)}\n       look ${look(b)}`;
}

// What the clip timeline should show for the engine's pieces: the first
// start, the last end, and a cut wherever two pieces have time between
// them. Pieces that touch, which a crop moved part way makes, are no cut.
export function expectedTimeline(segments) {
  if (!segments.length) return null;
  const cuts = [];
  for (let i = 1; i < segments.length; i++) {
    if (segments[i].start > segments[i - 1].end) cuts.push({ from: segments[i - 1].end, to: segments[i].start });
  }
  return { start: segments[0].start, end: segments[segments.length - 1].end, cuts };
}

// How long by the clock a caption may wait for its late layer, in ms.
const LATE_FOR = 1000;

// What the video preview lays over the picture, held to where the
// playhead is, for samples as recordFrames and overlay read them, against
// the engine's clip, state, in an episode of frames a frame long.
//
// On the video the video preview shows the video and nothing else: no crop
// frame, no shade beside it, no caption box, wherever the playhead is,
// the chosen clip's own moments included. Tim saw the captions stay up
// while the episode played through a clip. On the clip, with the playhead
// a frame or more inside one of its pieces, the crop frame stands over the
// picture, and the caption box with it wherever the engine has a caption,
// a tenth of a second or more inside it, on the frame on screen. The
// caption box shows what the picture shows, and the picture trails the
// playhead while it plays, by up to a frame, see Showing in
// lib/frames/captions.ts. A sample with no frame number, pictured, is held
// to the playhead. A frame whose own captions are still on their way is
// drawn with the last ones, and the caption box shows what those show, so
// a caption may be missing while the layer is late, for two frames of the
// playhead at most, and for a second of the clock at most, since a paused
// playhead does not move and would excuse it for good. Says what broke,
// or null.
export function overlaid(samples, state, frame) {
  const pieces = state?.segments ?? [];
  const clipTime = (t) => {
    let sum = 0;
    for (const p of pieces) {
      if (t < p.start) return sum;
      if (t < p.end) return sum + (t - p.start);
      sum += p.end - p.start;
    }
    return sum;
  };
  const inPiece = (t) => pieces.some((p) => t >= p.start + frame && t < p.end - frame);
  const captioned = (t) => {
    const c = clipTime(t);
    return (state?.captions ?? []).some((x) => c >= x.start + 0.1 && c < x.end - 0.1);
  };
  let missing = null;
  for (const o of samples) {
    const at = Number.isFinite(o.at) ? o.at.toFixed(3) : "nowhere";
    const how = o.playing ? "playing" : "paused";
    if (o.video) {
      const over = [o.crop && "the crop frame", o.shade && "the shade", o.captions && "the captions"].filter(Boolean);
      if (over.length) return `on the video at ${at}, ${how}, the video preview shows ${over.join(" and ")}`;
    } else if (inPiece(o.at)) {
      if (!o.crop) return `on the clip at ${at}, ${how}, the video preview shows no crop frame`;
      const shownAt = Number.isFinite(o.pictured) && o.pictured >= 0 ? o.pictured * frame : o.at;
      const none = captioned(shownAt) && !o.captions;
      missing = none ? (missing ?? o) : null;
      const long = none && (o.at - missing.at > 2 * frame || o.t - missing.t > LATE_FOR);
      if (none && (!o.late || long)) return `on the clip at ${at}, ${how}, on frame ${o.pictured}${o.late ? " with its captions late" : ""}, the engine has a caption and the video preview shows none`;
    }
  }
  return null;
}

// What is wrong with the size of the frame standing on the video preview,
// or null: nothing stands still, or it was decoded at the size the canvas
// asks for. A frame asked for again after a resize may still be on its
// way, so the size is read once the frame queue says the picture stands,
// see standing in lib/frames/queue.ts, in the same moment. The page hears
// that from the queue and from the page changing, the canvas's size among
// it, never on the clock, and follows a new queue when a step opens
// another video, see __standing in preview/standing.ts. A frame that
// never comes is a walk that hangs, ended by the -timeout of make walks. The queue needs the app's own timers to stand, so under a
// held clock a picture that does not stand yet is a walk that reads it at
// the wrong time, and fails at once.
export async function stillBlurred(page) {
  const got = await stillFrame(page);
  if (!got || (got.state !== "paused" && got.state !== "cued")) return null;
  if (got.frame[0] === got.want[0] && got.frame[1] === got.want[1]) return null;
  return `the frame on the video preview is ${got.frame.join(" by ")}, the canvas is ${got.canvas.join(" by ")} and the file ${got.file.join(" by ")}, so it should be ${got.want.join(" by ")}`;
}

// The frame on the video preview once the picture stands, read in the
// then of __standing so nothing comes between: its size, the canvas's,
// the file's, the size it should have been decoded at, the size the
// workspace asked its first frame at and the frame queue's state. Null
// with no frame on the canvas. Under a held clock the picture cannot
// come to stand by itself, so a read then is a mistake of the walk's.
export async function stillFrame(page) {
  const got = await page.evaluate(
    (held) =>
      window.__standing(held).then((stands) => {
        if (!stands) return { held: true };
        const q = window.__frameQueue;
        const c = document.querySelector(".screen canvas");
        const f = q?.picture();
        const v = q?.video;
        if (!f || !v || !c) return null;
        const first = window.__appFrames?.stats.first ?? "";
        return { frame: [f.width, f.height], canvas: [c.width, c.height], file: [v.width, v.height], first, state: q.state };
      }),
    Boolean(page.__held),
  );
  if (got?.held) throw new Error("the picture was read for its size under a held clock before it stood, see stillFrame");
  if (!got) return null;
  const [cw, ch] = got.canvas;
  const [vw, vh] = got.file;
  const scale = Math.min(1, cw / vw, ch / vh);
  const even = (n) => Math.max(2, Math.round(n / 2) * 2);
  return { ...got, want: [even(vw * scale), even(vh * scale)] };
}

// Watches one page through the steps made on it.
export class Watch {
  constructor(page, at, errors) {
    this.page = page;
    this.at = at;
    this.errors = errors;
    this.failed = null;
  }

  async start() {
    this.callsSeen = await this.page.evaluate(() => {
      window.__watched = true;
      return window.__calls.length;
    });
  }

  broke(rule, detail) {
    this.failed ??= { rule, detail };
  }

  // After a step: kind is "open" for Enter on the framed word, and s is
  // the caption box as it was before the step. Waits for the app to
  // settle, checks every rule, and hands back what is now on screen and
  // what the engine now says. acted is the page the step acted on, the
  // report window when it is not the app's window.
  async step(kind, s, acted = this.page) {
    const { page } = this;
    // A call a step held comes back before the screen is compared with
    // the engine, which has not heard it while it is held.
    await letHeld(page);
    // The report window is a page of its own, with calls of its own the
    // app's window does not count. Settled only there, an Undo of Clear
    // in it was still putting its pictures back when the next step
    // pressed Redo, and one came back after Redo had cleared them,
    // issue #554. The app's window settles in settledScreen below.
    if (acted !== page) {
      await letHeld(acted);
      await settle(acted);
    }
    // The clip on screen, which a step that opens another episode or
    // another clip changes. Then what was before is another clip's, and
    // nothing is compared across the change. It is read with what the
    // screen shows of it in the moment the app settled, see
    // settledScreen: a search lands its clip without a call on its way,
    // so the screen can change after the app has settled, and a clip
    // read a call after the clip timeline held the clip timeline to the
    // clip before. With no clip of the engine's on screen, a clip still
    // on its way among them, there is nothing of the engine's to hold the
    // screen to, and the clip on screen before is not it.
    const { at, clips, now, drawn } = await settledScreen(page);
    const after = at ? await engineState(page, at, clips) : null;
    this.at = at;

    // No call failed and the page threw nothing. A page loaded again, as
    // after a restart, starts its list of calls from nought and has lost
    // the mark the watch left on it, so its calls are read from the first.
    // Read on from the old count, the calls it made up to that count went
    // unchecked.
    const { from, calls } = await page.evaluate((seen) => {
      const from = window.__watched ? seen : 0;
      window.__watched = true;
      return { from, calls: window.__calls.slice(from) };
    }, this.callsSeen);
    this.callsSeen = from + calls.length;
    for (const c of calls) if (c.failed) this.broke("a call failed", `${c.name}: ${c.failed}`);
    for (const e of this.errors.splice(0)) this.broke("the page threw", e);

    // A picture that stands still is as sharp as the video preview lets it
    // be: the frame on the canvas was decoded at the canvas's size, no
    // larger than the file's own. A frame asked for again after a resize
    // may still be on its way, and is waited for. Tim saw the
    // first frame of every workspace blurred, decoded at the 300 by 150 of
    // a canvas not laid out yet, plan row 2.187.
    const blurred = await stillBlurred(page);
    if (blurred) this.broke("a still frame is at the canvas's size", blurred);

    // Enter opens the word in the keyboard's frame, and no other: the
    // frame is there to say which word Enter opens.
    if (kind === "open") {
      const was = keyed(s);
      const open = now.find((w) => w.focused);
      if (was && (!open || Math.abs(open.at - was.at) > 1e-6)) {
        this.broke(
          "Enter opens the word in the frame",
          `framed "${was.text}" at ${was.at}, opened ${open ? `"${open.text}" at ${open.at}` : "nothing"}`,
        );
      }
    }

    // The clip timeline shows the clip the engine has: where it starts and
    // ends and every cut, to a millisecond.
    const want = after && expectedTimeline(after.segments);
    if (drawn && want) {
      const near = (a, b) => Math.abs(a - b) < 1e-3;
      const fine =
        near(drawn.start, want.start) &&
        near(drawn.end, want.end) &&
        drawn.cuts.length === want.cuts.length &&
        drawn.cuts.every((c, i) => near(c.from, want.cuts[i].from) && near(c.to, want.cuts[i].to));
      if (!fine) {
        const say = (t) => `${t.start.toFixed(3)}-${t.end.toFixed(3)} cuts ${t.cuts.map((c) => `${c.from.toFixed(3)}-${c.to.toFixed(3)}`).join(" ")}`;
        this.broke("the clip timeline is the engine's clip", `drawn  ${say(drawn)}\nengine ${say(want)}`);
      }
    }

    // One word at most wears the keyboard's frame, and one at most is open.
    if (now.filter((w) => w.keyed).length > 1) this.broke("one frame", "two words wear the frame");
    if (now.filter((w) => w.focused).length > 1) this.broke("one open word", "two words are open");

    // The caption box shows a caption of the engine's, exactly: the same
    // words, in the same order, at the same moments, nothing more.
    if (after && !editing(now) && now.length) {
      const caption = after.captions.find((c) =>
        c.lines.some((l) => l.words.some((w) => Math.abs(w.start - now[0].at) < 1e-6)),
      );
      const want = caption
        ? caption.lines.flatMap((l) => l.words.map((w) => `${w.text}@${w.start.toFixed(3)}`))
        : [];
      const have = now.map((w) => `${w.text}@${w.at.toFixed(3)}`);
      if (want.join(" ") !== have.join(" ")) {
        this.broke("the caption box is the engine's caption", `shown  ${have.join(" ")}\nengine ${want.join(" ")}`);
      }
    }

    return { now, after };
  }
}
