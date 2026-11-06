import { describe, expect, test } from "vitest";
import { COLS, ROWS, clear, ease, mirrored, share, unseen } from "./reflection";

// A frame of standard video, w by h, each pixel given as [r, g, b] of 0 to
// 1023 the way the Go side packs them, red lowest, see picture.ts.
function frame(w: number, h: number, at: (x: number, y: number) => [number, number, number]): Uint8Array {
  const words = new Uint32Array(w * h);
  for (let y = 0; y < h; y++)
    for (let x = 0; x < w; x++) {
      const [r, g, b] = at(x, y);
      words[y * w + x] = r | (g << 10) | (b << 20);
    }
  return new Uint8Array(words.buffer);
}

function cell(into: Float32Array, col: number, row: number): number[] {
  const i = (row * COLS + col) * 4;
  return [into[i], into[i + 1], into[i + 2], into[i + 3]].map((v) => Math.round(v));
}

describe("the reflection of a frame", () => {
  test("is upside down: the top of the video lies nearest to the person, at the foot", () => {
    const into = new Float32Array(COLS * ROWS * 4);
    // Red over blue, as a sky over a floor would be the other way round.
    mirrored(frame(640, 360, (_, y) => (y < 180 ? [1023, 0, 0] : [0, 0, 1023])), 640, 360, "", into);
    expect(cell(into, 0, ROWS - 1)).toEqual([255, 0, 0, 255]);
    expect(cell(into, COLS - 1, ROWS - 1)).toEqual([255, 0, 0, 255]);
    expect(cell(into, 0, 0)).toEqual([0, 0, 255, 255]);
    expect(cell(into, COLS - 1, 0)).toEqual([0, 0, 255, 255]);
  });

  test("keeps left and right where they are, as a reflection in front of the screen does", () => {
    const into = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(1280, 720, (x) => (x < 640 ? [0, 0, 0] : [1023, 1023, 1023])), 1280, 720, "", into);
    expect(cell(into, 0, ROWS / 2)).toEqual([0, 0, 0, 255]);
    expect(cell(into, COLS / 2 - 1, ROWS / 2)).toEqual([0, 0, 0, 255]);
    expect(cell(into, COLS / 2, ROWS / 2)).toEqual([255, 255, 255, 255]);
    expect(cell(into, COLS - 1, ROWS / 2)).toEqual([255, 255, 255, 255]);
  });

  test("averages the pixels it reads in a cell, so a thin line only tints it", () => {
    const into = new Float32Array(COLS * ROWS * 4);
    // A frame as small as the grid of points the cells are read from:
    // every other column white, so each cell reads one white and one black
    // column, twice.
    mirrored(frame(COLS * 2, ROWS * 2, (x) => (x % 2 ? [1023, 1023, 1023] : [0, 0, 0])), COLS * 2, ROWS * 2, "", into);
    expect(cell(into, 5, 5)).toEqual([128, 128, 128, 255]);
  });

  test("reads a frame smaller than its grid of cells without reading past it", () => {
    const into = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(3, 2, () => [0, 1023, 0]), 3, 2, "", into);
    expect(cell(into, 0, 0)).toEqual([0, 255, 0, 255]);
    expect(cell(into, COLS - 1, ROWS - 1)).toEqual([0, 255, 0, 255]);
  });

  test("wears the look the video preview gives standard video, its shadows lifted", () => {
    const into = new Float32Array(COLS * ROWS * 4);
    // A grey of 22 in 255 is shown as 25, see lift in light.ts.
    const grey = Math.round((22 / 255) * 1023);
    mirrored(frame(64, 36, () => [grey, grey, grey]), 64, 36, "", into);
    expect(cell(into, 0, 0)).toEqual([25, 25, 25, 255]);
  });
});

describe("the reflection settling on a new frame", () => {
  test("goes two thirds of the way in a quarter of a second, and all of it in time", () => {
    expect(share(0.25)).toBeCloseTo(0.632, 3);
    expect(share(1)).toBeCloseTo(0.982, 3);
    const now = new Float32Array([0, 255]);
    const left = ease(now, new Float32Array([255, 0]), share(0.25));
    expect(now[0]).toBeCloseTo(161.19, 1);
    expect(now[1]).toBeCloseTo(93.81, 1);
    expect(left).toBeCloseTo(93.81, 1);
  });

  test("does not move in no time, or in time that went backwards", () => {
    expect(share(0)).toBe(0);
    expect(share(-0.1)).toBe(0);
  });

  test("fades out on black in the video preview, keeping its colours, rather than going dark", () => {
    const into = new Float32Array([10, 20, 30, 255, 40, 50, 60, 255]);
    expect(unseen(into)).toBe(false);
    clear(into);
    expect([...into]).toEqual([10, 20, 30, 0, 40, 50, 60, 0]);
    expect(unseen(into)).toBe(true);
  });
});
