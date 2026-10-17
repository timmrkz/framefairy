import { describe, expect, it } from "vitest";
import { matches } from "./match";

// The search field over the Font list, after Tim typed orb and got
// Kohinoor Bangla and Chalkboard (#349).
describe("matches", () => {
  it("finds a name by the letters that start its words", () => {
    expect(matches("American Typewriter", "amtyp")).toBe(true);
    expect(matches("Bridge Narrow", "brnar")).toBe(true);
    expect(matches("Helvetica Neue", "hn")).toBe(true);
    expect(matches("FreeSerif", "fs")).toBe(true);
  });

  it("finds a name that holds what was typed, anywhere in it", () => {
    expect(matches("American Typewriter", "writer")).toBe(true);
    expect(matches("American Typewriter", "AMERICAN TYPE")).toBe(true);
    expect(matches("Georgia", "")).toBe(true);
  });

  it("takes no letter out of the middle of a word", () => {
    expect(matches("Kohinoor Bangla", "orb")).toBe(false);
    expect(matches("Chalkboard", "orb")).toBe(false);
    expect(matches("Arial Rounded MT Bold", "orb")).toBe(false);
    expect(matches("Georgia", "ga")).toBe(false);
  });

  it("keeps the letters in the order they were typed", () => {
    expect(matches("American Typewriter", "tyam")).toBe(false);
  });

  it("costs nothing much on a name of one letter over and over", () => {
    expect(matches("a".repeat(64), "a".repeat(30) + "b")).toBe(false);
  });
});
