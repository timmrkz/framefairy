import { describe, expect, test } from "vitest";
import { suggestedCount } from "./count";

describe("the count a dragged window suggests", () => {
  // The lengths at which the count rises, as engine.SuggestedSteps sends
  // them for a half hour video: one clip, then two from 180 s on, three
  // from 360 s.
  const steps = [180, 360];

  test("is one clip below the first length the count rises at", () => {
    expect(suggestedCount(steps, 0)).toBe(1);
    expect(suggestedCount(steps, 179.9)).toBe(1);
  });

  test("rises on the length itself, and once for every length passed", () => {
    expect(suggestedCount(steps, 180)).toBe(2);
    expect(suggestedCount(steps, 359)).toBe(2);
    expect(suggestedCount(steps, 360)).toBe(3);
    expect(suggestedCount(steps, 3600)).toBe(3);
  });

  test("is one clip when Go sends no lengths", () => {
    expect(suggestedCount([], 900)).toBe(1);
  });
});
