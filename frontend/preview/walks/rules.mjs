// The rules that hold whatever was done, checked after every step of a walk
// and of a sequence. They compare the screen with the engine's own answer
// and the engine's answer with itself before and after, and know nothing
// of what a correction should do, so they cannot drift into a third
// engine. See docs/TESTING.md.
import { settle, engineState, shown, timeline, chosen } from "./bridge.mjs";

// The engine's captions as a list of words, and as one comparable value.
export const words = (caps) =>
  caps.captions.flatMap((c) => c.lines.flatMap((l) => l.words.map((w) => ({ ...w }))));
export const same = (a, b) =>
  JSON.stringify(a.captions) === JSON.stringify(b.captions) &&
  JSON.stringify(a.segments) === JSON.stringify(b.segments);
export const editing = (s) => s.some((w) => w.focused);
export const keyed = (s) => s.find((w) => w.keyed);

export function describe(a, b) {
  const line = (caps) =>
    caps.captions.map((c) => `[${c.lines.flatMap((l) => l.words.map((w) => w.text)).join(" ")}]`).join(" ");
  const pieces = (s) => (s.segments ?? []).map((p) => `${p.start.toFixed(2)}-${p.end.toFixed(2)}`).join(" ");
  return `before ${line(a)}\n       pieces ${pieces(a)}\nafter  ${line(b)}\n       pieces ${pieces(b)}`;
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
// a tenth of a second or more inside it. Says what broke, or null.
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
  for (const o of samples) {
    const at = Number.isFinite(o.at) ? o.at.toFixed(3) : "nowhere";
    const how = o.playing ? "playing" : "paused";
    if (o.video) {
      const over = [o.crop && "the crop frame", o.shade && "the shade", o.captions && "the captions"].filter(Boolean);
      if (over.length) return `on the video at ${at}, ${how}, the video preview shows ${over.join(" and ")}`;
    } else if (inPiece(o.at)) {
      if (!o.crop) return `on the clip at ${at}, ${how}, the video preview shows no crop frame`;
      if (captioned(o.at) && !o.captions) return `on the clip at ${at}, ${how}, the engine has a caption and the video preview shows none`;
    }
  }
  return null;
}

// Watches one page through the steps made on it.
export class Watch {
  constructor(page, at, errors) {
    this.page = page;
    this.at = at;
    this.errors = errors;
    // The engine's captions before each step that changed them, for Undo,
    // and after each step taken back, for Redo.
    this.undone = [];
    this.redone = [];
    this.failed = null;
  }

  async start() {
    this.before = this.at ? await engineState(this.page, this.at) : null;
    this.callsSeen = await this.page.evaluate(() => {
      window.__watched = true;
      return window.__calls.length;
    });
  }

  broke(rule, detail) {
    this.failed ??= { rule, detail };
  }

  // After a step: kind is "undo", "redo" or "open" for those three, and s
  // is the caption box as it was before the step. Waits for the app to
  // settle, checks every rule, and hands back what is now on screen and
  // what the engine now says.
  async step(kind, s) {
    const { page } = this;
    await settle(page);
    // The clip on screen, which a step that opens another episode or
    // another clip changes. Then what was before is another clip's, and
    // nothing is compared across the change. It is read before and after
    // what the screen shows of it, and read again if it changed in
    // between: a search lands its clip without a call on its way, so the
    // screen can change after the app has settled, and the clip timeline
    // was then held to the clip of the episode open before. With no clip
    // of the engine's on screen, a clip still on its way among them, there
    // is nothing of the engine's to hold the screen to, and the clip on
    // screen before is not it.
    let at, now, drawn, after;
    for (let tries = 0; ; tries++) {
      at = await chosen(page);
      now = await shown(page);
      drawn = await timeline(page);
      after = at ? await engineState(page, at) : null;
      if (JSON.stringify(await chosen(page)) === JSON.stringify(at) || tries === 20) break;
      await settle(page);
    }
    const moved = JSON.stringify(at) !== JSON.stringify(this.at);
    if (moved) {
      this.at = at;
      this.undone = [];
      this.redone = [];
    }
    const before = moved ? after : this.before;

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

    // Undo puts back what was there before the step it takes back, and
    // Redo what was there after it.
    if (!after) {
      // No clip of the engine's on screen, so nothing to take back.
    } else if (kind === "undo" || kind === "redo") {
      const [from, to] = kind === "undo" ? [this.undone, this.redone] : [this.redone, this.undone];
      if (from.length) {
        const want = from.pop();
        to.push(before);
        if (!same(after, want)) this.broke(`${kind} puts it back`, describe(want, after));
      } else if (!same(after, before)) {
        // A step from before the watch began, which it does not know.
        this.undone.length = this.redone.length = 0;
      }
    } else if (!same(after, before)) {
      this.undone.push(before);
      this.redone.length = 0;
      // An edit stays where it was made: every word more than three
      // seconds of the episode away from the word corrected keeps its text
      // and its time.
      // A caption removed whole is its words removed, from its first to
      // its last, so the same holds around all of them.
      const last = [...calls].reverse().find((c) => c.name === "SetWord" || c.name === "RemoveCaption");
      if (last) {
        const whole = last.name === "RemoveCaption";
        const where = last.args[3];
        const cue = whole ? before.captions.find((c) => Math.abs((c.first ?? NaN) - where) < 1e-6) : null;
        if (whole && !cue) this.broke("a caption removed was there", `no caption began on ${where}`);
        const till = cue?.last ?? where;
        const far = (caps) =>
          words(caps)
            .filter((w) => w.said === undefined || w.said < where - 3 || w.said > till + 3)
            .map((w) => `${w.text}@${w.start.toFixed(3)}-${w.end.toFixed(3)}`)
            .join(" ");
        if (far(before) !== far(after)) this.broke("an edit stays where it was made", describe(before, after));
        // Taking a word out takes out the word and changes no caption:
        // every caption still there appears when it did and goes when it
        // did. A caption whose words were all removed goes and leaves its
        // time empty. The one before it used to stay up through that time,
        // and its block on the clip timeline grew across the place, which
        // Tim read as two captions merging.
        if (whole && cue && after.captions.some((c) => Math.abs((c.first ?? NaN) - where) < 1e-6)) {
          this.broke("a caption removed is gone", describe(before, after));
        }
        if (whole || last.args[4] === "") {
          const near = (a, b) => Math.abs(a - b) < 1e-3;
          const kept = after.captions.every((c) => {
            const i = before.captions.findIndex((b) => near(b.start, c.start));
            return i >= 0 && near(c.end, before.captions[i].end);
          });
          if (!kept) {
            const spans = (caps) => caps.captions.map((c) => `${c.start.toFixed(3)}-${c.end.toFixed(3)}`).join(" ");
            this.broke("a removal changes no caption", `${describe(before, after)}\nthen ${spans(before)}\nnow  ${spans(after)}`);
          }
        }
      }
    }
    this.before = after;
    return { now, after };
  }
}
