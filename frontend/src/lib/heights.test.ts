import { describe, expect, test } from "vitest";

import { nearestHeight } from "./api";

// The captions dragged up or down land on a height the Go side offers,
// see engine.CaptionStart, the same while they are dragged and when they
// are let go.
describe("the captions dragged", () => {
  const heights = [120, 180, 240, 300];

  test("land on the height offered nearest the hand", () => {
    expect(nearestHeight(heights, 120)).toBe(120);
    expect(nearestHeight(heights, 149)).toBe(120);
    expect(nearestHeight(heights, 151)).toBe(180);
    expect(nearestHeight(heights, 299)).toBe(300);
  });

  test("land on the higher of two heights as near", () => {
    expect(nearestHeight(heights, 150)).toBe(180);
    expect(nearestHeight(heights, 270)).toBe(300);
  });

  test("stop at the lowest and the highest height offered", () => {
    expect(nearestHeight(heights, -500)).toBe(120);
    expect(nearestHeight(heights, 5000)).toBe(300);
  });

  test("stay where the hand is with no heights offered yet", () => {
    expect(nearestHeight([], 211)).toBe(211);
  });
});
