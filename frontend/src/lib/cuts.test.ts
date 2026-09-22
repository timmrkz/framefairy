import { describe, expect, test } from "vitest";
import { cutAt, snapCut, type Word } from "./api";

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
    // Before the transcript reaches a stretch there is nothing to line up
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

  test("lands where it was asked for, on frames, as wide as it was asked", () => {
    const [a, b] = cutAt(pieces, 15, 0.25, 0.05, frame)!;
    expect(a).toBeCloseTo(14.88, 2);
    expect(b).toBeCloseTo(15.12, 2);
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
