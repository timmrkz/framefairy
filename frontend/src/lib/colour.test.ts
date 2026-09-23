import { describe, expect, test } from "vitest";

import { joinColour, splitColour } from "./colour";

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
