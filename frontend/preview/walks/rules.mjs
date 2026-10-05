// The rules that hold whatever was done, checked after every step of a walk
// and of a sequence. They compare the screen with the engine's own answer
// and the engine's answer with itself before and after, and know nothing
// of what a correction should do, so they cannot drift into a third
// engine. See docs/TESTING.md.
import { settle, engineState, shown, timeline } from "./bridge.mjs";

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
    this.before = await engineState(this.page, this.at);
    this.callsSeen = await this.page.evaluate(() => window.__calls.length);
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
    const now = await shown(page);
    const after = await engineState(page, this.at);
    const before = this.before;

    // No call failed and the page threw nothing.
    const calls = await page.evaluate((from) => window.__calls.slice(from), this.callsSeen);
    this.callsSeen += calls.length;
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
    const drawn = await timeline(page);
    const want = expectedTimeline(after.segments);
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
    if (!editing(now) && now.length) {
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
    if (kind === "undo" || kind === "redo") {
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
      const last = [...calls].reverse().find((c) => c.name === "SetWord");
      if (last) {
        const where = last.args[3];
        const far = (caps) =>
          words(caps)
            .filter((w) => w.said === undefined || Math.abs(w.said - where) > 3)
            .map((w) => `${w.text}@${w.start.toFixed(3)}-${w.end.toFixed(3)}`)
            .join(" ");
        if (far(before) !== far(after)) this.broke("an edit stays where it was made", describe(before, after));
        // Taking a word out takes out the word and changes no caption:
        // every caption still there appears when it did and goes when it
        // did. A caption whose words were all removed goes, and the one
        // before it then stays up through its time rather than leave the
        // box empty for a moment.
        if (last.args[4] === "") {
          const near = (a, b) => Math.abs(a - b) < 1e-3;
          const kept = after.captions.every((c) => {
            const i = before.captions.findIndex((b) => near(b.start, c.start));
            if (i < 0) return false;
            const next = before.captions[i + 1];
            const gone = next && !after.captions.some((a) => near(a.start, next.start));
            return near(c.end, before.captions[i].end) || (gone && near(c.end, next.end));
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
