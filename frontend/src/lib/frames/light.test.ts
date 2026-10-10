import { describe, expect, test } from "vitest";
import { hlgScene, lightOfPixel, pqNits, srgb, toSDR } from "./light";

// A pixel of 10 bits a colour as the Go side sends HDR, red lowest.
function word(r: number, g: number, b: number): number {
  return (r | (g << 10) | (b << 20)) >>> 0;
}

describe("HDR turned into light", () => {
  test("PQ is BT.2100's curve: 0 is black, 1 is 10000 nits, 0.5806 is the reference white", () => {
    expect(pqNits(0)).toBe(0);
    expect(pqNits(1)).toBeCloseTo(10000, 0);
    expect(pqNits(0.5806)).toBeCloseTo(203, 0);
  });

  test("HLG's reference white, 75 percent, is the white of the interface", () => {
    expect(hlgScene(0.5)).toBeCloseTo(1 / 12, 6);
    expect(hlgScene(1)).toBeCloseTo(1, 5);
    for (const v of lightOfPixel("hlg", 0.75, 0.75, 0.75)) expect(v).toBeCloseTo(1, 2);
  });

  test("a grey stays grey from BT.2020's colours into BT.709's, and a pure BT.2020 green leaves BT.709", () => {
    const [r, g, b] = lightOfPixel("pq", 0.5806, 0.5806, 0.5806);
    expect(r).toBeCloseTo(1, 2);
    expect(g).toBeCloseTo(1, 2);
    expect(b).toBeCloseTo(1, 2);
    const green = lightOfPixel("pq", 0, 0.5806, 0);
    expect(green[0]).toBeLessThan(0);
  });

  test("highlights are brighter than white", () => {
    expect(lightOfPixel("pq", 0.75, 0.75, 0.75)[0]).toBeGreaterThan(4);
    expect(lightOfPixel("hlg", 1, 1, 1)[0]).toBeCloseTo(1000 / 203, 1);
  });
});

describe("HDR cut at white, for a 2D canvas and a colour taken from the picture", () => {
  test("the reference white is white, black is black, and above white stays white", () => {
    const white = Math.round(0.5806 * 1023);
    const data = new Uint8Array(new Uint32Array([word(white, white, white), word(0, 0, 0), word(1023, 1023, 1023)]).buffer);
    const out = new Uint8ClampedArray(12);
    toSDR("pq", data, out);
    expect([...out]).toEqual([255, 255, 255, 255, 0, 0, 0, 255, 255, 255, 255, 255]);
  });

  test("a grey of 100 nits is shown as 100 over 203 of white in sRGB", () => {
    // PQ's signal for 100 nits.
    const code = Math.round(0.5081 * 1023);
    const out = new Uint8ClampedArray(4);
    toSDR("pq", new Uint8Array(new Uint32Array([word(code, code, code)]).buffer), out);
    const want = Math.round(255 * srgb(pqNits(code / 1023) / 203));
    expect(Math.abs(out[0] - want)).toBeLessThanOrEqual(1);
    expect(out[0]).toBeGreaterThan(175);
    expect(out[0]).toBeLessThan(195);
  });

  test("HLG's 75 percent grey is white", () => {
    const code = Math.round(0.75 * 1023);
    const out = new Uint8ClampedArray(4);
    toSDR("hlg", new Uint8Array(new Uint32Array([word(code, code, code)]).buffer), out);
    expect(out[0]).toBeGreaterThanOrEqual(254);
  });
});
