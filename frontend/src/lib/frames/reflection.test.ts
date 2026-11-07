import { describe, expect, test } from "vitest";
import { BLUR, COLS, ROWS, band, clear, ease, mirrored, painted, share, spread } from "./reflection";

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
    const b = band(1200, 100);
    const cells = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(64, 36, () => [1023, 0, 0]), 64, 36, "", cells);
    const now = new Float32Array(COLS * ROWS * 4);
    spread(cells, b, now);
    clear(cells);
    const goal = new Float32Array(COLS * ROWS * 4);
    spread(cells, b, goal);
    expect(Math.max(...goal)).toBe(0);
    ease(now, goal, share(0.25));
    const bytes = new Uint8ClampedArray(now.length);
    painted(now, bytes);
    const i = ((ROWS - 1) * COLS + COLS / 2) * 4;
    expect(bytes[i + 3]).toBeGreaterThan(0);
    expect([bytes[i], bytes[i + 1], bytes[i + 2]]).toEqual([255, 0, 0]);
  });

  test("fades in on its first frame with that frame's colours, not from black", () => {
    const cells = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(64, 36, () => [0, 0, 1023]), 64, 36, "", cells);
    const goal = new Float32Array(COLS * ROWS * 4);
    spread(cells, band(1200, 100), goal);
    const now = new Float32Array(COLS * ROWS * 4);
    ease(now, goal, share(0.05));
    const bytes = new Uint8ClampedArray(now.length);
    painted(now, bytes);
    const i = ((ROWS - 1) * COLS + COLS / 2) * 4;
    expect(bytes[i + 3]).toBeGreaterThan(0);
    expect([bytes[i], bytes[i + 1], bytes[i + 2]]).toEqual([0, 0, 255]);
  });
});

// The share of a bell of width 1 below x, added up finely, to check the
// blur against without the error function it is worked out with.
function below(x: number): number {
  let sum = 0;
  const step = 0.001;
  for (let t = -8; t < x; t += step) sum += Math.exp((-(t + step / 2) * (t + step / 2)) / 2) * step;
  return sum / Math.sqrt(2 * Math.PI);
}

describe("the reflection blurred and faded over its band", () => {
  // A band as the walks lay it out, four fifths of 1500 pixels by 100.
  const wide = 1200;
  const high = 100;
  const grey = (cells: Float32Array, r: number, c: number) => cells[(r * COLS + c) * 4 + 3];

  function spreadOf(at: (x: number, y: number) => [number, number, number]) {
    const cells = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(COLS * 2, ROWS * 2, at), COLS * 2, ROWS * 2, "", cells);
    const into = new Float32Array(COLS * ROWS * 4);
    spread(cells, band(wide, high), into);
    return into;
  }

  test("thins out towards the edges of the band, as the stylesheet's blur did over nothing beyond it", () => {
    // A band narrow enough for the bell to reach past its edge from the
    // part the sides do not fade. Of two cells of the foot row there, the
    // one nearer the edge has less of the band under its bell, and so
    // less light, by as much as the share of the bell the band covers.
    const narrow = 300;
    const cells = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(64, 36, () => [1023, 1023, 1023]), 64, 36, "", cells);
    const into = new Float32Array(COLS * ROWS * 4);
    spread(cells, band(narrow, high), into);
    const r = ROWS - 1;
    const cw = narrow / COLS;
    const covered = (c: number) => below(((COLS - c - 0.5) * cw) / BLUR) - below((-(c + 0.5) * cw) / BLUR);
    const near = COLS / 4 + 1;
    const middle = COLS / 2;
    expect(grey(into, r, near) / grey(into, r, middle)).toBeCloseTo(covered(near) / covered(middle), 3);
    expect(covered(near)).toBeLessThan(covered(middle) - 0.05);
  });

  test("keeps the colour of a frame of one colour everywhere it is seen", () => {
    const cells = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(64, 36, () => [1023, 512, 0]), 64, 36, "", cells);
    const colour = [...cells.subarray(0, 3)].map((v) => Math.round(v));
    const into = spreadOf(() => [1023, 512, 0]);
    const bytes = new Uint8ClampedArray(into.length);
    painted(into, bytes);
    for (const [r, c] of [
      [ROWS - 1, COLS / 2],
      [ROWS / 2, COLS / 4],
      [5, (3 * COLS) / 4],
    ]) {
      const i = (r * COLS + c) * 4;
      expect([bytes[i], bytes[i + 1], bytes[i + 2]]).toEqual(colour);
    }
  });

  test("fades out at the sides and upwards, the way the stops of the gradients said", () => {
    const into = spreadOf(() => [1023, 1023, 1023]);
    const foot = ROWS - 1;
    // Nothing at the very sides, strongest from a quarter in to three
    // quarters, and from the foot to a third of the way up already less
    // than half.
    expect(grey(into, foot, 0)).toBeLessThan(grey(into, foot, COLS / 4) / 10);
    expect(grey(into, foot, COLS - 1)).toBeLessThan(grey(into, foot, (3 * COLS) / 4) / 10);
    const third = ROWS - 1 - Math.round(0.35 * ROWS);
    expect(grey(into, third, COLS / 2)).toBeLessThan(grey(into, foot, COLS / 2) * 0.5);
    expect(grey(into, 0, COLS / 2)).toBeLessThan(grey(into, foot, COLS / 2) / 50);
  });

  test("blurs across by as many pixels of the app as BLUR says, however wide the band", () => {
    // A white left half and a black right half: the edge blurred, the
    // colour at the middle of the band is half way, and a bell's width
    // further on it is the share of the bell beyond that, in pixels of
    // the app, so the same in a wide band and in a narrow one.
    for (const w of [1200, 600]) {
      const cells = new Float32Array(COLS * ROWS * 4);
      mirrored(frame(COLS * 2, ROWS * 2, (x) => (x < COLS ? [1023, 1023, 1023] : [0, 0, 0])), COLS * 2, ROWS * 2, "", cells);
      const into = new Float32Array(COLS * ROWS * 4);
      spread(cells, band(w, high), into);
      const r = ROWS - 1;
      const cell = w / COLS;
      const c = COLS / 2 + Math.round(BLUR / cell) - 1;
      // The middle of that cell, from the edge, in bells.
      const x = ((c + 0.5 - COLS / 2) * cell) / BLUR;
      const lit = into[(r * COLS + c) * 4] / into[(r * COLS + c) * 4 + 3];
      const bandFrom = ((c + 0.5) * cell) / BLUR;
      const bandTo = ((COLS - c - 0.5) * cell) / BLUR;
      // White from the band's left edge to its middle, of all the band
      // the bell covers.
      const expected = (below(-x) - below(-bandFrom)) / (below(bandTo) - below(-bandFrom));
      expect(lit).toBeCloseTo(expected, 2);
    }
  });
});
