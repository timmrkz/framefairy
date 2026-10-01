import { describe, expect, test } from "vitest";

import { captionColours, hexToHSV, hsvToHex, joinColour, readHex, rgbToHex, splitColour } from "./colour";

describe("a caption colour", () => {
  test("is taken apart into a colour and how opaque it is", () => {
    expect(splitColour("rgba(0, 0, 0, 0.498)")).toEqual({ hex: "#000000", alpha: 0.498 });
    expect(splitColour("rgba(255, 204, 0, 1)")).toEqual({ hex: "#ffcc00", alpha: 1 });
    expect(splitColour("rgb(16, 32, 48)")).toEqual({ hex: "#102030", alpha: 1 });
  });

  test("reads as opaque white when it is not a colour", () => {
    expect(splitColour("")).toEqual({ hex: "#ffffff", alpha: 1 });
    expect(splitColour("red")).toEqual({ hex: "#ffffff", alpha: 1 });
  });

  test("goes back together the way it came apart", () => {
    expect(joinColour("#102030", 0.25)).toBe("rgba(16, 32, 48, 0.25)");
    const { hex, alpha } = splitColour("rgba(1, 2, 3, 0.5)");
    expect(joinColour(hex, alpha)).toBe("rgba(1, 2, 3, 0.5)");
    expect(joinColour("nonsense", 1)).toBe("rgba(255, 255, 255, 1)");
  });
});

describe("a colour picked by hand", () => {
  test("is read from what is typed, with or without the #", () => {
    expect(readHex("#FFCC00")).toBe("#ffcc00");
    expect(readHex(" ffcc00 ")).toBe("#ffcc00");
    expect(readHex("#fc0")).toBe("#ffcc00");
    expect(readHex("ffcc0")).toBeNull();
    expect(readHex("red")).toBeNull();
    expect(readHex("#ffcc00;x")).toBeNull();
  });

  test("goes through the square and the strip and comes back the same", () => {
    for (const { hex } of captionColours) expect(hsvToHex(hexToHSV(hex))).toBe(hex);
    for (const hex of ["#102030", "#7f7f7f", "#00ff80", "#123456", "#fedcba"]) {
      expect(hsvToHex(hexToHSV(hex))).toBe(hex);
    }
  });

  test("is where the square and the strip say", () => {
    expect(hexToHSV("#ff0000")).toEqual({ h: 0, s: 1, v: 1 });
    expect(hsvToHex({ h: 120, s: 1, v: 1 })).toBe("#00ff00");
    expect(hsvToHex({ h: 240, s: 1, v: 0.5 })).toBe("#000080");
    expect(hsvToHex({ h: 360, s: 1, v: 1 })).toBe("#ff0000");
    expect(hsvToHex({ h: 200, s: 0, v: 1 })).toBe("#ffffff");
    expect(hsvToHex({ h: 200, s: 2, v: -1 })).toBe("#000000");
  });

  test("is read from a pixel", () => {
    expect(rgbToHex(255, 204, 0)).toBe("#ffcc00");
    expect(rgbToHex(300, -4, 16.4)).toBe("#ff0010");
  });

  test("offers each preset once, as #rrggbb", () => {
    const hexes = captionColours.map((c) => c.hex);
    expect(new Set(hexes).size).toBe(hexes.length);
    for (const hex of hexes) expect(readHex(hex)).toBe(hex);
  });
});
