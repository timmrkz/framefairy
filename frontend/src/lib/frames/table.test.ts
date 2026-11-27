import { describe, expect, test } from "vitest";
import { FrameTable } from "./table";

// Three frames at 25 a second from 0.5 s into the file, each holding from
// a little before it begins, the way the engine says it.
const table = new FrameTable({ origin: 0, start: 0.5, num: 25, den: 1, count: 3 }, Float64Array.of(-Infinity, 0.5385, 0.5785));

describe("FrameTable", () => {
  test("finds the frame that holds a moment where the engine says it holds from", () => {
    expect(table.holding(-3)).toBe(0);
    expect(table.holding(0.5384)).toBe(0);
    expect(table.holding(0.5385)).toBe(1);
    expect(table.holding(0.578)).toBe(1);
    expect(table.holding(0.579)).toBe(2);
    expect(table.holding(99)).toBe(2);
  });

  test("names where each frame begins, the last for any after it", () => {
    expect(table.begins(0)).toBe(0.5);
    expect(table.begins(2)).toBeCloseTo(0.58, 9);
    expect(table.begins(7)).toBeCloseTo(0.58, 9);
    expect(table.startOf(0.5786)).toBeCloseTo(0.58, 9);
    expect(table.length).toBe(0.04);
  });
});
