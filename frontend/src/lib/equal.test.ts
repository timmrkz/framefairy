import { describe, expect, test } from "vitest";
import { equal } from "./equal";

// The clips and the status of a video are compared with what is on
// screen on every state Go sends, issue #805. The answer has to be the
// one comparing their JSON gave, which is what it replaced.
describe("two values read from JSON", () => {
  const clip = () => ({ key: "a", start: 1.5, segments: [{ start: 1.5, end: 3, cropX: null, marks: [] }], found: [1.5, 3], title: "Mein Arm" });

  test("are equal when they hold the same, whoever made them", () => {
    expect(equal(clip(), clip())).toBe(true);
    expect(equal(clip(), JSON.parse(JSON.stringify(clip())))).toBe(true);
    expect(equal(null, null)).toBe(true);
    expect(equal([], [])).toBe(true);
  });

  test("are not equal when anything deep in them differs", () => {
    const moved = clip();
    moved.segments[0].end = 3.04;
    expect(equal(clip(), moved)).toBe(false);
    expect(equal(clip(), { ...clip(), title: "Mein Bein" })).toBe(false);
    expect(equal(clip(), { ...clip(), extra: 1 })).toBe(false);
    expect(equal({ found: [1, 2] }, { found: [1, 2, 3] })).toBe(false);
    expect(equal({ a: [] }, { a: {} })).toBe(false);
    expect(equal({ a: null }, { a: {} })).toBe(false);
    expect(equal({ a: 0 }, { a: "0" })).toBe(false);
    expect(equal(null, {})).toBe(false);
  });

  test("take a key that holds nothing for no key, the way JSON writes it", () => {
    expect(equal({ a: 1, b: undefined }, { a: 1 })).toBe(true);
    expect(equal({ a: 1 }, { a: 1, b: undefined })).toBe(true);
    expect(equal({ a: 1, b: undefined }, { a: 1, b: null })).toBe(false);
  });

  test("take keys in another order for the same", () => {
    expect(equal({ a: 1, b: [2] }, { b: [2], a: 1 })).toBe(true);
  });
});
