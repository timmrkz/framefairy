import { describe, expect, it } from "vitest";
import type { CaptionFont } from "./api";
import { families, nearestFace, styleNames } from "./fonts";

// The fonts are offered the way Pages offers them, a list of families and
// the styles of the one picked, after Tim saw a row for every face (#349).
const face = (name: string, family: string, style: string, weight = 400, width = 5, italic = false, machine = true) =>
  ({ name, family, style, weight, width, italic, machine, about: "", file: "" }) as CaptionFont;

const typewriter = [
  face("American Typewriter Condensed Bold", "American Typewriter", "Condensed Bold", 700, 3),
  face("American Typewriter", "American Typewriter", "Regular"),
  face("American Typewriter Light", "American Typewriter", "Light", 300),
  face("American Typewriter Bold", "American Typewriter", "Bold", 700),
  face("American Typewriter Condensed", "American Typewriter", "Condensed", 400, 3),
];
const georgia = [
  face("Georgia Bold Italic", "Georgia", "Bold Italic", 700, 5, true),
  face("Georgia", "Georgia", "Regular"),
  face("Georgia Italic", "Georgia", "Italic", 400, 5, true),
  face("Georgia Bold", "Georgia", "Bold", 700),
];
const ours = [
  face("Inter Black", "Inter", "Black", 900, 5, false, false),
  face("Anton", "Anton", "Regular", 400, 5, false, false),
];

describe("families", () => {
  it("offers ours first in their order, then the machine's by name", () => {
    const list = families([...ours, ...georgia, ...typewriter]);
    expect(list.map((f) => f.name)).toEqual(["Inter", "Anton", "American Typewriter", "Georgia"]);
    expect(list.map((f) => f.machine)).toEqual([false, false, true, true]);
  });

  it("lists a family's styles narrowest, then lightest, then upright first", () => {
    const [family] = families(typewriter);
    expect(family.faces.map((f) => f.style)).toEqual(["Condensed", "Condensed Bold", "Light", "Regular", "Bold"]);
    const [serif] = families(georgia);
    expect(serif.faces.map((f) => f.style)).toEqual(["Regular", "Italic", "Bold", "Bold Italic"]);
  });

  it("counts a family with a face of ours as ours", () => {
    const list = families([...ours, face("Inter Light", "Inter", "Light", 300)]);
    expect(list.map((f) => [f.name, f.machine, f.faces.length])).toEqual([
      ["Inter", false, 2],
      ["Anton", false, 1],
    ]);
  });

  it("takes a face with no family as a family of its own", () => {
    const list = families([face("Old Face", "", "")]);
    expect(list.map((f) => f.name)).toEqual(["Old Face"]);
  });
});

describe("nearestFace", () => {
  const [family] = families(georgia);

  it("comes in the regular face when picked from scratch", () => {
    expect(nearestFace(family).name).toBe("Georgia");
  });

  it("keeps a style of the same name", () => {
    expect(nearestFace(family, georgia[0]).name).toBe("Georgia Bold Italic");
  });

  it("keeps leaning first, then the width, then the weight", () => {
    // Inter Black is upright and the heaviest, so Georgia comes in bold.
    expect(nearestFace(family, ours[0]).name).toBe("Georgia Bold");
    // A light italic from elsewhere stays italic before it stays light.
    expect(nearestFace(family, face("X Light Italic", "X", "Light Italic", 300, 5, true)).name).toBe("Georgia Italic");
    const [type] = families(typewriter);
    expect(nearestFace(type, face("Y Narrow", "Y", "Narrow", 400, 2)).name).toBe("American Typewriter Condensed");
  });
});

describe("styleNames", () => {
  it("names a face by its style, and by its full name where two share one", () => {
    const [family] = families([
      face("Test Sans", "Test Sans", "Regular"),
      face("Test Sans Text", "Test Sans", "Regular"),
      face("Test Sans Bold", "Test Sans", "Bold", 700),
    ]);
    expect([...styleNames(family).values()]).toEqual(["Test Sans", "Test Sans Text", "Bold"]);
  });
});
