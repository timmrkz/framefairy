import { describe, expect, test } from "vitest";
import { clock } from "./clock";

// The walks write what they expect to see with clock, so its own words
// are held here, with the numbers written out.
describe("a moment the way the app writes it", () => {
  test.each([
    [0, "0:00"],
    [9.99, "0:09"],
    [61, "1:01"],
    [599, "9:59"],
    [600, "10:00"],
    [3599, "59:59"],
    [3600, "1:00:00"],
    [3661.5, "1:01:01"],
    [36000, "10:00:00"],
    [-1, "0:00"],
    [Number.NaN, "0:00"],
    [Number.POSITIVE_INFINITY, "0:00"],
  ])("%s s reads %s", (seconds, want) => {
    expect(clock(seconds)).toBe(want);
  });
});
