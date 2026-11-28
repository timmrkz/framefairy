import { describe, expect, it } from "vitest";
import type { CaptionCue } from "./api";
import { spliced } from "./splice";

// A caption on the clip's clock, and where it is in the episode, which a
// change before it on the clip's clock does not move.
const cue = (start: number, text: string, at = start): CaptionCue => ({
  start,
  end: start + 1,
  lines: [{ words: [{ start, end: start + 1, text }] }],
  from: at,
  to: at + 1,
  up: [at, at + 1],
});

describe("spliced", () => {
  const held = [cue(0, "a"), cue(1, "b"), cue(2, "c"), cue(3, "d")];

  it("takes the captions sent whole without a splice", () => {
    expect(spliced(held, [cue(5, "x")])).toEqual([cue(5, "x")]);
  });

  it("puts the captions sent between the ones held, those after moved on", () => {
    expect(spliced(held, [cue(1.5, "x")], { head: 1, tail: 3, shift: 0.5 })).toEqual([
      cue(0, "a"),
      cue(1.5, "x"),
      cue(3.5, "d", 3),
    ]);
  });

  it("keeps the captions held up to the end when the end was dragged", () => {
    expect(spliced(held, [cue(3, "y")], { head: 3, tail: 4, shift: 0 })).toEqual([cue(0, "a"), cue(1, "b"), cue(2, "c"), cue(3, "y")]);
  });
});
