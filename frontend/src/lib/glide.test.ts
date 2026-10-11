import { describe, expect, test } from "vitest";
import cases from "./glide.cases.json";
import { frameOf, glideX, glidingLeft, markOn, type FrameMark } from "./glide";

// The same table the engine is held to, in engine/cropmarks_test.go, so
// the crop frame in the video preview stands on every frame where the
// render crops it.
describe("the glide of a crop over its marks", () => {
  test("is the engine's on every frame", () => {
    const marks = cases.marks as FrameMark[];
    for (const [k, want] of cases.frames) expect([k, glideX(marks, k)]).toEqual([k, want]);
  });

  test("counts frames the way the engine does", () => {
    expect(frameOf(10.52, 25)).toBe(263);
    expect(frameOf(10.5, 25)).toBe(263);
    expect(frameOf(10.519, 25)).toBe(263);
    // 29.97 frames a second, a frame's start kept to the millisecond.
    expect(frameOf(1.368, 30000 / 1001)).toBe(41);
    expect(frameOf(0.5, 25, 0.1)).toBe(10);
  });

  test("stands still with no mark or one, and keeps the crop inside the picture", () => {
    expect(glidingLeft(undefined, 3, 25, 0, 608, 1920)).toBeNull();
    expect(glidingLeft([], 3, 25, 0, 608, 1920)).toBeNull();
    expect(glidingLeft([{ at: 1, x: 500 }], 3, 25, 0, 608, 1920)).toBe(500);
    expect(glidingLeft([{ at: 1, x: 1800 }], 3, 25, 0, 608, 1920)).toBe(1312);
  });

  test("glides between two marks of a piece, by the frame", () => {
    const marks = [
      { at: 10.2, x: 100 },
      { at: 10.8, x: 300 },
    ];
    // Frame 263 lies 8 of the 15 frames on, where smoothstep has gone
    // 55 % of the way: 209.985.
    expect(glidingLeft(marks, 10.52, 25, 0, 608, 1920)).toBeCloseTo(209.985, 3);
    expect(glidingLeft(marks, 9, 25, 0, 608, 1920)).toBe(100);
    expect(glidingLeft(marks, 12, 25, 0, 608, 1920)).toBe(300);
  });

  test("finds the mark on the playhead's frame", () => {
    const marks = [{ at: 10.52, x: 100 }];
    // The playhead anywhere in the frame, the last moment of it too.
    expect(markOn(marks, 10.53, 25)).toEqual(marks[0]);
    expect(markOn(marks, 10.559, 25)).toEqual(marks[0]);
    expect(markOn(marks, 10.56, 25)).toBeNull();
  });
});
