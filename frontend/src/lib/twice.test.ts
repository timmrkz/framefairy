import { describe, expect, test } from "vitest";
import { DOUBLE_CLICK_MS, DOUBLE_CLICK_PX, Presses } from "./twice";

describe("a double-click", () => {
  test("is a second press soon after the first, near it", () => {
    const p = new Presses();
    expect(p.twice({ at: 1000, x: 100, y: 50 })).toBe(false);
    expect(p.twice({ at: 1000 + DOUBLE_CLICK_MS - 1, x: 105, y: 45 })).toBe(true);
  });

  test("is not a second press once the Mac's time has passed", () => {
    const p = new Presses();
    p.twice({ at: 1000, x: 100, y: 50 });
    expect(p.twice({ at: 1000 + DOUBLE_CLICK_MS, x: 100, y: 50 })).toBe(false);
  });

  test("is not a second press as far as the Mac's distance either way", () => {
    const along = new Presses();
    along.twice({ at: 0, x: 100, y: 50 });
    expect(along.twice({ at: 10, x: 100 + DOUBLE_CLICK_PX, y: 50 })).toBe(false);
    // Up the clip timeline at the same x, a caption block and then the
    // keyframe over it, #535.
    const up = new Presses();
    up.twice({ at: 0, x: 100, y: 50 });
    expect(up.twice({ at: 10, x: 100, y: 50 - DOUBLE_CLICK_PX })).toBe(false);
  });

  test("starts no new one with its own second press, so three quick presses are one and a press", () => {
    const p = new Presses();
    expect([0, 100, 200, 300].map((at) => p.twice({ at, x: 10, y: 10 }))).toEqual([false, true, false, true]);
  });

  test("is not the first press ever, however early it comes", () => {
    expect(new Presses().twice({ at: 0, x: 0, y: 0 })).toBe(false);
  });
});
