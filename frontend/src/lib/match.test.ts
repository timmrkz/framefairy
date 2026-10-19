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

  it("finds a word typed with a letter too many, too few, wrong or swapped", () => {
    // Tim typed arrm, armm and aarm for arm and found nothing.
    expect(bold(said, "arrm")).toEqual(["Arm"]);
    expect(bold(said, "armm")).toEqual(["Arm"]);
    expect(bold(said, "aarm")).toEqual(["Arm"]);
    expect(bold(said, "genomen")).toEqual(["genommen"]);
    expect(bold(said, "mtuter")).toEqual(["Mutter"]);
    expect(bold(said, "arrm mutter")).toEqual(["Mutter", "Arm"]);
    expect(bold("Das ist ernst gemeint.", "ernste")).toEqual(["ernst"]);
    // The start of a long word with a slip in it, the way it reads while
    // the rest is still being typed.
    expect(bold("Die Verantwortung liegt bei uns.", "verantwotr")).toEqual(["Verantwortung"]);
  });

  it("never finds a word that only holds the letters typed somewhere in it", () => {
    // Tim typed ernste and was shown Verteidigungsministers, Veranstalter
    // and Quereinsteigermentalität.
    expect(found("unter politischer Führung des Verteidigungsministers.", "ernste")).toBeNull();
    expect(found("Und die sind auch in Amerika die Veranstalter?", "ernste")).toBeNull();
    expect(found("Glaubst du, dass es so eine Art Quereinsteigermentalität braucht, um.", "ernste")).toBeNull();
    expect(found("Die Anrufmaschine im Arbeitszimmer.", "arm")).toBeNull();
    // Two letters are too few to have a slip in them.
    expect(found(said, "ma")).toBeNull();
  });

  it("finds only what holds what was typed when not fuzzy", () => {
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
