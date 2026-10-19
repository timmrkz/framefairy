import { describe, expect, it } from "vitest";
import { found, matches } from "./match";

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

// The search in the clip column, over what was said (#366).
describe("found", () => {
  const said = "meine Mutter hat mich einfach in den Arm genommen.";
  const bold = (text: string, typed: string) => found(text, typed)?.map(([a, b]) => text.slice(a, b)) ?? null;

  it("sets the words that hold what was typed in bold", () => {
    expect(bold(said, "Arm")).toEqual(["Arm"]);
    expect(bold(said, "den arm")).toEqual(["den", "Arm"]);
    expect(bold(said, "mutt")).toEqual(["Mutter"]);
  });

  it("finds every word typed somewhere in the sentence, the way a font finds its name", () => {
    expect(bold(said, "arm mutter")).toEqual(["Mutter", "Arm"]);
    expect(bold(said, "genomen")).toEqual(["genommen"]);
  });

  it("finds only what holds what was typed when not fuzzy", () => {
    // Three letters find a lot of words the way a font finds its name, so
    // the transcript asks for those only where nothing holds them.
    expect(bold("Die Anrufmaschine im Arbeitszimmer.", "arm")).toEqual(["Anrufmaschine", "Arbeitszimmer"]);
    expect(found("Die Anrufmaschine im Arbeitszimmer.", "arm", false)).toBeNull();
    expect(found(said, "arm", false)?.map(([a, b]) => said.slice(a, b))).toEqual(["Arm"]);
    expect(found(said, "genomen", false)).toBeNull();
  });

  it("finds nothing where a word typed is not there", () => {
    expect(found(said, "Vater")).toBeNull();
    expect(found(said, "arm vater")).toBeNull();
  });

  it("answers every sentence with nothing bold while nothing is typed", () => {
    expect(found(said, "")).toEqual([]);
    expect(found(said, "   ")).toEqual([]);
  });
});
