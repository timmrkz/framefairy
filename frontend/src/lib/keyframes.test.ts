import { describe, expect, test } from "vitest";
import cases from "./keyframes.cases.json";
import { frameOf, heldLeft, heldX, markOn, type FrameMark } from "./keyframes";

// The same table the engine is held to, in engine/cropmarks_test.go, so
// the crop frame in the video preview stands on every frame where the
// render crops it.
describe("the crop a keyframe holds", () => {
  test("is the engine's on every frame", () => {
    const marks = cases.marks as FrameMark[];
    for (const [k, want] of cases.frames) expect([k, heldX(marks, k) ?? cases.base]).toEqual([k, want]);
  });

  test("counts frames the way the engine does", () => {
    expect(frameOf(10.52, 25)).toBe(263);
    expect(frameOf(10.5, 25)).toBe(263);
    expect(frameOf(10.519, 25)).toBe(263);
    // 29.97 frames a second, a frame's start kept to the millisecond.
    expect(frameOf(1.368, 30000 / 1001)).toBe(41);
    expect(frameOf(0.5, 25, 0.1)).toBe(10);
  });

  test("is the piece's own before the first keyframe, and kept inside the picture after it", () => {
    expect(heldLeft(undefined, 3, 25, 0, 608, 1920)).toBeNull();
    expect(heldLeft([], 3, 25, 0, 608, 1920)).toBeNull();
    expect(heldLeft([{ at: 1, x: 500 }], 0.5, 25, 0, 608, 1920)).toBeNull();
    expect(heldLeft([{ at: 1, x: 500 }], 3, 25, 0, 608, 1920)).toBe(500);
    expect(heldLeft([{ at: 1, x: 1800 }], 3, 25, 0, 608, 1920)).toBe(1312);
  });

  test("switches on the keyframe's own frame, with no glide", () => {
    const marks = [
      { at: 10.2, x: 100 },
      { at: 10.8, x: 300 },
    ];
    // The frame before the second keyframe still has the first one's
    // crop, and the second one's frame has its own.
    expect(heldLeft(marks, 10.2, 25, 0, 608, 1920)).toBe(100);
    expect(heldLeft(marks, 10.52, 25, 0, 608, 1920)).toBe(100);
    expect(heldLeft(marks, 10.79, 25, 0, 608, 1920)).toBe(100);
    expect(heldLeft(marks, 10.8, 25, 0, 608, 1920)).toBe(300);
    expect(heldLeft(marks, 12, 25, 0, 608, 1920)).toBe(300);
  });

  test("finds the mark on the playhead's frame", () => {
    const marks = [{ at: 10.52, x: 100 }];
    // The playhead anywhere in the frame, the last moment of it too. A
    // hair before the next frame's start is in the next frame, the way
    // the engine counts it, see heldMicros in flow.ts.
    expect(markOn(marks, 10.53, 25)).toEqual(marks[0]);
    expect(markOn(marks, 10.558, 25)).toEqual(marks[0]);
    expect(markOn(marks, 10.559, 25)).toBeNull();
    expect(markOn(marks, 10.56, 25)).toBeNull();
  });
});
