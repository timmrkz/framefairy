import { describe, expect, it } from "vitest";
import { matches } from "./match";

// The search field over the Font list. Tim found it too strict, often
// finding nothing, after letters from inside a word stopped counting (#349).
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

  it("finds a name with a letter left out of it", () => {
    expect(matches("Helvetica", "helvtica")).toBe(true);
    expect(matches("Baskerville", "baskervile")).toBe(true);
    expect(matches("Garamond", "garmond")).toBe(true);
  });

  it("finds a name by a few letters from inside it", () => {
    expect(matches("Times New Roman", "tms")).toBe(true);
    expect(matches("Georgia", "ga")).toBe(true);
  });

  it("finds no name whose every letter was picked from somewhere else", () => {
    expect(matches("Chalkboard", "hlbr")).toBe(false);
    expect(matches("Arial Rounded MT Bold", "iudl")).toBe(false);
  });

  it("keeps the letters in the order they were typed", () => {
    expect(matches("American Typewriter", "tyam")).toBe(false);
  });

  it("costs nothing much on a name of one letter over and over", () => {
    expect(matches("a".repeat(64), "a".repeat(30) + "b")).toBe(false);
  });
});
