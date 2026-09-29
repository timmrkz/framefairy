import { describe, expect, test } from "vitest";
import { wordStep, type Word } from "./api";

const said = (list: [number, number, string][]): Word[] =>
  list.map(([start, end, text]) => ({ start, end, text }));

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
