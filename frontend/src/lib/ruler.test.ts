import { describe, expect, test } from "vitest";
import { dragStep, fitsAt, GAP, pickerSteps, rulerStep, SHORT } from "./ruler";

const picker = [60, 300, 600, 900, 1800, 3600, 7200];

describe("rulerStep", () => {
  test("gives a small range picker a time every minute where they fit", () => {
    // Tim's six minute episode on a range picker 372 pixels wide: a minute
    // is 64 pixels, and 5:00 is about 25.
    expect(rulerStep(349, 372, 25, picker)).toBe(60);
  });

  test("takes the next step only when the times would touch", () => {
    // A minute 32 pixels wide leaves 25 + 2 * 4 = 33 no room.
    expect(rulerStep(600, 320, 25, picker)).toBe(300);
    expect(rulerStep(600, 330, 25, picker)).toBe(60);
  });

  test("keeps the times of a four hour episode apart", () => {
    const step = rulerStep(4 * 3600, 900, 32, picker);
    expect((step / (4 * 3600)) * 900).toBeGreaterThanOrEqual(32 + 2 * GAP);
    expect(step).toBe(900);
  });

  test("falls back to the largest step when none fits", () => {
    expect(rulerStep(100 * 3600, 50, 40, picker)).toBe(7200);
  });
});

describe("fitsAt", () => {
  test("draws a time that ends clear of the edge, and not one that runs into it", () => {
    // 5:00 at 320 of 372, the time Tim's small range picker left out.
    expect(fitsAt(320, 25, 372)).toBe(true);
    expect(fitsAt(340, 25, 372)).toBe(false);
  });
});

describe("pickerSteps", () => {
  test("gives a fifteen second episode lines every few seconds", () => {
    // Tim's fifteen second video had no line at all on the range picker.
    const step = rulerStep(15, 1150, 25, pickerSteps(15));
    expect(step).toBe(2);
    expect(Math.ceil(15 / step) - 1).toBeLessThanOrEqual(SHORT);
  });

  test("keeps a line a minute on an episode of a few minutes", () => {
    expect(rulerStep(349, 372, 25, pickerSteps(349))).toBe(60);
    expect(rulerStep(349, 1150, 25, pickerSteps(349))).toBe(60);
  });

  test("leaves the steps of a long episode as they were", () => {
    expect(pickerSteps(4 * 3600)).toEqual(picker);
  });
});

describe("dragStep", () => {
  test("lands on every line of the ruler on Tim's video of two hours and a half", () => {
    // youtube.mp4, 9609.58 s, a line every 15 minutes. Two minutes were
    // about eight pixels and the window could not start at 45:00.
    expect(dragStep(9609.58, 700, 900)).toBe(300);
    expect(dragStep(9609.58, 860, 900)).toBe(300);
  });

  test("is still the smallest round step about eight pixels wide", () => {
    expect(dragStep(4 * 3600, 915, 1800)).toBe(300);
    expect(dragStep(4 * 3600, 1370, 1800)).toBe(120);
    expect(dragStep(4 * 3600, 1370, 3600)).toBe(120);
    expect(dragStep(360, 915, 60)).toBe(5);
    expect(dragStep(1500, 870, 300)).toBe(15);
  });

  test("passes over a round step the ruler's step is no whole number of", () => {
    // 2 minutes into 5, 10 into 15, 10 minutes into 15.
    expect(dragStep(3600, 300, 300)).toBe(300);
    expect(dragStep(120, 150, 15)).toBe(15);
    expect(dragStep(20000, 300, 900)).toBe(900);
  });

  test("is the ruler's own step when no round step fits", () => {
    expect(dragStep(100 * 3600, 50, 7200)).toBe(7200);
  });
});
