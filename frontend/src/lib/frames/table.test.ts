import { describe, expect, test } from "vitest";
import { FrameTable } from "./table";

// Three frames at 25 a second from 0.5 s into the file, each holding from
// a little before it begins, the way the engine says it.
const table = new FrameTable(Float64Array.of(-Infinity, 0.5385, 0.5785), Float64Array.of(0.5, 0.54, 0.58));

describe("FrameTable", () => {
  test("finds the frame that holds a moment where the engine says it holds from", () => {
    expect(table.holding(-3)).toBe(0);
    expect(table.holding(0.5384)).toBe(0);
    expect(table.holding(0.5385)).toBe(1);
    expect(table.holding(0.578)).toBe(1);
    expect(table.holding(0.579)).toBe(2);
    expect(table.holding(99)).toBe(2);
  });

  test("names where each frame begins as the engine says, the first and last for any outside", () => {
    expect(table.begins(-1)).toBe(0.5);
    expect(table.begins(2)).toBe(0.58);
    expect(table.begins(7)).toBe(0.58);
    expect(table.startOf(0.5786)).toBe(0.58);
    expect(table.beside(0.55, false)).toBe(0.58);
    expect(table.beside(0.55, true)).toBe(0.5);
    expect(table.count).toBe(3);
  });
});
