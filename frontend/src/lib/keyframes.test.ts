import { describe, expect, test } from "vitest";
import { frameOf, heldLeft, markOn } from "./keyframes";

describe("the crop a keyframe holds", () => {
  test("is the step the engine sent for the frame shown", () => {
    // Frames 10 to 19 at 25 a second, a switch on frame 15 and on 17.
    const steps = [
      { frame: 10, x: 100 },
      { frame: 15, x: 300 },
      { frame: 17, x: 40 },
    ];
    expect(heldLeft(steps, 0.4, 25, 0)).toBe(100);
    expect(heldLeft(steps, 0.59, 25, 0)).toBe(100);
    expect(heldLeft(steps, 0.6, 25, 0)).toBe(300);
    expect(heldLeft(steps, 0.67, 25, 0)).toBe(300);
    expect(heldLeft(steps, 0.68, 25, 0)).toBe(40);
    expect(heldLeft(steps, 2, 25, 0)).toBe(40);
    // Before the piece's first frame, the crop it starts with.
    expect(heldLeft(steps, 0.1, 25, 0)).toBe(100);
  });

  test("is the step of each frame of a glide, off the whole pixel", () => {
    // A glide is a step on every frame, 3000 of them here, and the step
    // asked for is found by halving.
    const steps = Array.from({ length: 3000 }, (_, i) => ({ frame: 10 + i, x: 100 + i * 0.25 }));
    expect(heldLeft(steps, 0.4, 25, 0)).toBe(100);
    expect(heldLeft(steps, 0.52, 25, 0)).toBe(100.75);
    expect(heldLeft(steps, 100, 25, 0)).toBe(100 + 2490 * 0.25);
    expect(heldLeft(steps, 1000, 25, 0)).toBe(100 + 2999 * 0.25);
    expect(heldLeft(steps, 0, 25, 0)).toBe(100);
  });

  test("is the piece's own where the engine sent no steps", () => {
    expect(heldLeft(undefined, 3, 25, 0)).toBeNull();
    expect(heldLeft(null, 3, 25, 0)).toBeNull();
    expect(heldLeft([], 3, 25, 0)).toBeNull();
    expect(heldLeft([{ frame: 0, x: 8 }], 3, 0, 0)).toBeNull();
  });

  test("counts frames the way the engine does", () => {
    expect(frameOf(10.52, 25)).toBe(263);
    expect(frameOf(10.5, 25)).toBe(263);
    expect(frameOf(10.519, 25)).toBe(263);
    // 29.97 frames a second, a frame's start kept to the millisecond.
    expect(frameOf(1.368, 30000 / 1001)).toBe(41);
    expect(frameOf(0.5, 25, 0.1)).toBe(10);
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
