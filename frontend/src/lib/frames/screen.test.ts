import { describe, expect, test } from "vitest";
import { Picture } from "./picture";
import { fit } from "./screen";

describe("where a frame goes on the canvas", () => {
  test("as large as the side that runs out first allows, in the middle, on whole pixels", () => {
    expect(fit(1000, 500, 1920, 1080)).toEqual({ x: 56, y: 0, w: 889, h: 500 });
    expect(fit(500, 1000, 1920, 1080)).toEqual({ x: 0, y: 360, w: 500, h: 281 });
    expect(fit(1280, 720, 1280, 720)).toEqual({ x: 0, y: 0, w: 1280, h: 720 });
  });

  test("nowhere while the canvas or the frame has no size", () => {
    expect(fit(0, 500, 1920, 1080)).toBeNull();
    expect(fit(1000, 500, 0, 0)).toBeNull();
  });
});

describe("a frame", () => {
  test("has 4 bytes a pixel, or it is no frame", () => {
    expect(() => new Picture(new ArrayBuffer(2 * 2 * 3), 2, 2, 0)).toThrow();
  });

  test("shared by two holders is the same bytes, and one closing leaves the other its pixels", () => {
    const a = new Picture(new Uint8Array([1, 2, 3, 255, 4, 5, 6, 255]), 2, 1, 40000);
    const b = a.clone(80000);
    expect(b.data).toBe(a.data);
    expect(b.timestamp).toBe(80000);
    a.close();
    expect(a.closed).toBe(true);
    expect([...b.data!]).toEqual([1, 2, 3, 255, 4, 5, 6, 255]);
    expect(() => a.clone()).toThrow();
  });
});
