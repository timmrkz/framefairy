import { describe, expect, test } from "vitest";
import { cutAt, snapCut, wordStep, type Word } from "./api";

// The timeline draws the block a cut will leave out while the hand is still
// moving, so this snapping has to agree with the engine's. They are the
// same rules written twice, once in Go and once here, and the day they
// disagree the app draws one cut and renders another. These are the cases
// engine/cuts_test.go holds, so a change to one that is not made to the
// other shows up here.

const said = (list: [number, number, string][]): Word[] =>
  list.map(([start, end, text]) => ({ start, end, text }));

// The same words engine/cuts_test.go uses.
const words = said([
  [10, 10.5, "eins"],
  [11, 11.5, "zwei"],
  [14, 14.5, "drei"],
  [15.2, 15.8, "vier"],
]);

const round = ([a, b]: [number, number]): [number, number] => [
  Math.round(a * 1000) / 1000,
  Math.round(b * 1000) / 1000,
];

describe("a cut lands where the render will cut", () => {
  test("a cut in a pause keeps a tenth of a second either side", () => {
    // The pause runs from the end of zwei at 11.5 to the start of drei at 14.
    expect(round(snapCut(words, 12, 13.5, 0.1))).toEqual([11.6, 13.9]);
  });

  test("a nudge inside a pause takes the whole pause", () => {
    // Taking a pause out is the job, so a rough drag does the right thing.
    expect(round(snapCut(words, 12.6, 12.7, 0.1))).toEqual([11.6, 13.9]);
  });

  test("a cut over speech takes whole words", () => {
    // The drag starts inside drei and ends inside vier, so both go rather
    // than being left half spoken.
    const [from, to] = round(snapCut(words, 14.2, 15.5, 0.1));
    expect(from).toBeLessThanOrEqual(14);
    expect(to).toBeGreaterThanOrEqual(15.8);
  });

  test("a cut never stops inside a word", () => {
    // Every edge, for a lot of drags, has to fall in a gap between words.
    for (let from = 9.5; from < 16; from += 0.1) {
      for (const width of [0.05, 0.4, 1.3, 2.6]) {
        const [a, b] = snapCut(words, from, from + width, 0.1);
        for (const w of words) {
          const insideStart = a > w.start && a < w.end;
          const insideEnd = b > w.start && b < w.end;
          expect(
            insideStart || insideEnd,
            `a cut of ${width}s from ${from.toFixed(1)} landed ${a} to ${b}, inside ${w.text}`,
          ).toBe(false);
        }
      }
    }
  });

  test("with no words there is nothing to snap to", () => {
    // Before the transcript reaches a part there is nothing to line up
    // with, so the drag stands as it was drawn.
    expect(round(snapCut([], 12, 13.5, 0.1))).toEqual([12, 13.5]);
  });

  test("a cut never starts before the episode does", () => {
    expect(snapCut(words, -5, 10.2, 0.1)[0]).toBeGreaterThanOrEqual(0);
  });

  test("the edges keep their order", () => {
    for (let from = 9; from < 17; from += 0.25) {
      const [a, b] = snapCut(words, from, from + 0.3, 0.1);
      expect(b, `a cut from ${from} came back as ${a} to ${b}`).toBeGreaterThanOrEqual(a);
    }
  });
});

describe("cutAt", () => {
  // A clip in two pieces with a cut already between them.
  const pieces = [
    { start: 10, end: 20 },
    { start: 22, end: 30 },
  ];
  const frame = 1 / 25;

  test("lands where it was asked for, on frames, around what it was asked", () => {
    const [a, b] = cutAt(pieces, 15, 0.25, 0.05, frame)!;
    // Centred on the click.
    expect((a + b) / 2).toBeCloseTo(15, 1);
    // A whole number of frames wide, which is the asked-for width rounded
    // up: seven frames of 40 milliseconds is 280, for a quarter second
    // asked.
    expect(b - a).toBeCloseTo(0.28, 6);
    expect(b - a).toBeGreaterThanOrEqual(0.25);
    expect(Math.abs(a / frame - Math.round(a / frame))).toBeLessThan(1e-9);
    expect(Math.abs(b / frame - Math.round(b / frame))).toBeLessThan(1e-9);
  });

  test("makes no cut where there is no piece", () => {
    expect(cutAt(pieces, 21, 0.25, 0.05, frame)).toBeNull();
    expect(cutAt(pieces, 5, 0.25, 0.05, frame)).toBeNull();
    expect(cutAt(pieces, 40, 0.25, 0.05, frame)).toBeNull();
    expect(cutAt([], 15, 0.25, 0.05, frame)).toBeNull();
  });

  test("leaves room on both sides of the piece, wherever it is clicked", () => {
    for (const at of [10.001, 10.1, 15, 19.9, 19.999]) {
      const [a, b] = cutAt(pieces, at, 0.25, 0.05, frame)!;
      expect(a).toBeGreaterThanOrEqual(10.05 - 1e-9);
      expect(b).toBeLessThanOrEqual(19.95 + 1e-9);
      expect(b - a).toBeGreaterThanOrEqual(0.05 - 1e-9);
    }
  });

  test("makes no cut in a piece with no room to spare", () => {
    expect(cutAt([{ start: 10, end: 10.1 }], 10.05, 0.25, 0.05, frame)).toBeNull();
  });

  test("gives back what room there is when the piece is short", () => {
    const [a, b] = cutAt([{ start: 10, end: 10.4 }], 10.2, 0.25, 0.05, frame)!;
    expect(a).toBeGreaterThanOrEqual(10.05 - 1e-9);
    expect(b).toBeLessThanOrEqual(10.35 + 1e-9);
    expect(b - a).toBeGreaterThanOrEqual(0.05 - 1e-9);
  });
});

describe("cutAt, asked for a width the zoom worked out", () => {
  const piece = [{ start: 10, end: 30 }];
  const frame = 1 / 25;

  // Zoomed right out, forty pixels of a four hour episode across a
  // thousand-pixel track is a long time. The cut is that long.
  test("takes a wide part when the zoom says forty pixels are wide", () => {
    const [a, b] = cutAt(piece, 20, 8, 0.05, frame)!;
    expect(b - a).toBeCloseTo(8, 6);
  });

  // Zoomed right in, forty pixels can be worth less than the least a cut
  // may be. It is held open at the least rather than refused.
  test("holds a cut open at the least when the zoom says less", () => {
    const [a, b] = cutAt(piece, 20, 0.001, 0.05, frame)!;
    expect(b - a).toBeGreaterThanOrEqual(0.05 - 1e-9);
  });

  test("never takes more than the piece has room for", () => {
    const [a, b] = cutAt(piece, 20, 999, 0.05, frame)!;
    expect(a).toBeGreaterThanOrEqual(10.05 - 1e-9);
    expect(b).toBeLessThanOrEqual(29.95 + 1e-9);
  });
});

describe("wordStep", () => {
  // Three words with a pause after the second, which is where a second at
  // a time used to land on nothing at all.
  const said = [
    { start: 10.0, end: 10.4, text: "Und" },
    { start: 10.5, end: 10.9, text: "da" },
    { start: 13.2, end: 13.8, text: "war" },
  ];
  // A frame at twenty-five a second, which is what a real episode gives.
  const frame = 0.04;

  // What the picture would light up with the playhead here, which is the
  // whole question these keys are about. The caption lights the word being
  // spoken and nothing in the gaps between words.
  const lit = (at: number | null) =>
    at === null ? "(nowhere)" : (said.find((w) => at >= w.start && at < w.end)?.text ?? "(none)");

  // What the picture answers with, which is not what it was asked for. A
  // video element gives back the frame it is showing, and a decoder that
  // shows the next frame it has answers with one at or after where the
  // playhead was sent. That is the direction that matters: a clock that
  // comes back later is a clock that walks away from where it was aimed.
  const shown = (at: number | null) =>
    at === null ? null : Math.ceil(at / frame - 1e-9) * frame;

  test("lights the next word up, every press", () => {
    expect(lit(wordStep(said, 9, false, frame))).toBe("Und");
    expect(lit(wordStep(said, 10.2, false, frame))).toBe("da");
    expect(lit(wordStep(said, 10.7, false, frame))).toBe("war");
  });

  test("goes back a word, not to the start of the one it is in", () => {
    expect(lit(wordStep(said, 10.7, true, frame))).toBe("Und");
    expect(lit(wordStep(said, 13.5, true, frame))).toBe("da");
    expect(wordStep(said, 10.2, true, frame)).toBe(null);
  });

  test("goes back to the word just spoken when it is in the silence after it", () => {
    expect(lit(wordStep(said, 12, true, frame))).toBe("da");
    expect(lit(wordStep(said, 10.45, true, frame))).toBe("Und");
  });

  test("never lands on a word's edge", () => {
    for (const at of [9, 10.2, 10.7, 12]) {
      const to = wordStep(said, at, false, frame);
      expect(to).not.toBe(null);
      expect(said.some((w) => to === w.start || to === w.end)).toBe(false);
    }
  });

  // The one that had no way out of it. Stepping used to ask how far the
  // playhead was from a word, and the picture answers with the frame it is
  // showing, which can be later than the playhead was sent. Back then
  // found the word the playhead was already on and sent it to the same
  // place again, so the key did nothing at all and nothing but the mouse
  // got out of it.
  // The one that had no way out. These are the words of Tim's own episode
  // around the moment he was stuck on, and a picture that answers a frame
  // late. Stepping back asked how far the playhead was from a word, the
  // clock had drifted past the word it was on, so back found that same
  // word and sent the playhead where it already stood.
  test("gets out of a word the clock has drifted inside of", () => {
    const his = [
      { start: 876.88, end: 877.12, text: "So," },
      { start: 877.28, end: 877.36, text: "ich" },
      { start: 877.44, end: 877.68, text: "hätte" },
      { start: 877.68, end: 877.84, text: "also." },
    ];
    const where = (at: number) => his.find((w) => at >= w.start && at < w.end)?.text ?? "(none)";
    let at = 877.72;
    const back: string[] = [];
    for (let i = 0; i < 4; i++) {
      const to = wordStep(his, at, true, frame);
      if (to === null) break;
      const next = Math.ceil(to / frame - 1e-9) * frame;
      expect(next).not.toBe(at);
      back.push(where(next));
      at = next;
    }
    expect(back).toEqual(["hätte", "ich", "So,"]);
  });

  test("gets out of a word however the picture rounds the clock", () => {
    let at: number | null = shown(wordStep(said, 9, false, frame));
    expect(lit(at)).toBe("Und");
    const walk: string[] = [];
    for (let i = 0; i < 4; i++) {
      const to = shown(wordStep(said, at as number, false, frame));
      if (to === null) break;
      walk.push(lit(to));
      at = to;
    }
    expect(walk).toEqual(["da", "war"]);
    const back: string[] = [];
    for (let i = 0; i < 4; i++) {
      const to = shown(wordStep(said, at as number, true, frame));
      if (to === null) break;
      back.push(lit(to));
      at = to;
    }
    expect(back).toEqual(["da", "Und"]);
  });

  test("moves every press, never finding the word it just landed on", () => {
    let at: number | null = 9;
    const forward: string[] = [];
    for (let i = 0; i < 5 && at !== null; i++) {
      const to = wordStep(said, at, false, frame);
      if (to === null) break;
      forward.push(lit(to));
      at = to;
    }
    expect(forward).toEqual(["Und", "da", "war"]);
  });

  test("crosses a pause in one press rather than landing in it", () => {
    // A second at a time took three presses to get from da to war and
    // spent two of them in silence, with nothing lit in the picture.
    expect(lit(wordStep(said, 10.7, false, frame))).toBe("war");
  });

  test("steps into a word shorter than two frames by half of itself", () => {
    const brief = [{ start: 5, end: 5.004, text: "a" }];
    expect(wordStep(brief, 4, false, frame)).toBe(5.002);
  });

  test("says there is nowhere to go", () => {
    expect(wordStep([], 10, false, frame)).toBe(null);
    expect(wordStep([], 10, true, frame)).toBe(null);
    expect(wordStep(said, 20, false, frame)).toBe(null);
    expect(wordStep(said, 0, true, frame)).toBe(null);
  });
});
