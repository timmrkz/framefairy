import { describe, expect, test } from "vitest";
import { BLUR, COLS, ROWS, band, clear, ease, mirrored, painted, raster, share, spread } from "./reflection";

// The colour of the app behind the band, --ink-0.
const ground = [22, 23, 26] as const;

// What the cell at i of spread's colours looks like laid over the ground.
function over(from: Float32Array, i: number): number[] {
  const rest = 1 - from[i + 3] / 255;
  return [0, 1, 2].map((k) => from[i + k] + ground[k] * rest);
}

// What painted draws of spread's colours on a canvas wide by high, as its
// bytes, red, green, blue and alpha.
function paint(from: Float32Array, wide: number, high: number): Uint8ClampedArray {
  const pixels = new Uint32Array(wide * high);
  painted(from, ground, raster(wide, high), pixels);
  return new Uint8ClampedArray(pixels.buffer);
}

// Every channel of the pixel at i within one step of what it should be:
// the noise that keeps the reflection from falling into bands moves a
// pixel by less than half a step before it is rounded.
function near(bytes: Uint8ClampedArray, i: number, want: number[]) {
  for (let k = 0; k < 3; k++) expect(Math.abs(bytes[i + k] - want[k])).toBeLessThanOrEqual(1);
}

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
    const bytes = paint(now, COLS, ROWS);
    const i = ((ROWS - 1) * COLS + COLS / 2) * 4;
    expect(now[i + 3]).toBeGreaterThan(0);
    // Red still, over the ground, and as much of it as is left.
    expect(bytes[i]).toBeGreaterThan(ground[0] + 10);
    near(bytes, i, over(now, i));
    expect(over(now, i)[0]).toBeCloseTo(ground[0] + ((255 - ground[0]) * now[i + 3]) / 255, 3);
  });

  test("fades in on its first frame with that frame's colours, not from black", () => {
    const cells = new Float32Array(COLS * ROWS * 4);
    mirrored(frame(64, 36, () => [0, 0, 1023]), 64, 36, "", cells);
    const goal = new Float32Array(COLS * ROWS * 4);
    spread(cells, band(1200, 100), goal);
    const now = new Float32Array(COLS * ROWS * 4);
    ease(now, goal, share(0.05));
    const bytes = paint(now, COLS, ROWS);
    const i = ((ROWS - 1) * COLS + COLS / 2) * 4;
    expect(now[i + 3]).toBeGreaterThan(0);
    expect(bytes[i + 2]).toBeGreaterThan(ground[2]);
    expect(over(now, i)[2]).toBeCloseTo(ground[2] + ((255 - ground[2]) * now[i + 3]) / 255, 3);
    near(bytes, i, over(now, i));
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
    const bytes = paint(into, COLS, ROWS);
    for (const [r, c] of [
      [ROWS - 1, COLS / 2],
      [ROWS / 2, COLS / 4],
      [5, (3 * COLS) / 4],
    ]) {
      const i = (r * COLS + c) * 4;
      const a = into[i + 3] / 255;
      near(bytes, i, [0, 1, 2].map((k) => colour[k] * a + ground[k] * (1 - a)));
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

describe("the reflection drawn on the screen's pixels", () => {
  // The band as the walks lay it out, four fifths of 1500 by 100, on a
  // screen of two pixels a point, as a Mac's is.
  const wide = 2400;
  const high = 200;

  test("is the ground and nothing else where nothing is seen, so the band has no edge", () => {
    const cells = new Float32Array(COLS * ROWS * 4);
    const bytes = paint(cells, wide, high);
    for (let i = 0; i < bytes.length; i += 4) {
      if (bytes[i] !== ground[0] || bytes[i + 1] !== ground[1] || bytes[i + 2] !== ground[2] || bytes[i + 3] !== 255) {
        throw new Error(`pixel ${i / 4} is ${[...bytes.subarray(i, i + 4)]}, not the ground`);
      }
    }
  });

  // Tim saw the reflection as pixels: a faint shade, a few steps of 256
  // over the ground, rounded into flat bands a step apart, the edge of
  // each stepping from cell to cell. A ramp of white from nothing to a
  // sixth across the band climbs one step every 55 pixels or so.
  function ramp() {
    const cells = new Float32Array(COLS * ROWS * 4);
    for (let r = 0; r < ROWS; r++)
      for (let c = 0; c < COLS; c++) {
        const a = (42 * c) / (COLS - 1);
        cells.set([a, a, a, a], (r * COLS + c) * 4);
      }
    const bytes = paint(cells, wide, high);
    // What the middle row should be, not rounded, from the cells' middles.
    const want = (x: number) => {
      const u = Math.min(COLS - 1, Math.max(0, ((x + 0.5) * COLS) / wide - 0.5));
      const a = (42 * u) / (COLS - 1);
      return a + ground[0] * (1 - a / 255);
    };
    return { bytes, want, row: (high / 2) * wide * 4 };
  }

  test("shows the shade between two steps as their mix, so the eye sees the ramp and not the steps", () => {
    const { bytes, want, row } = ramp();
    let worst = 0;
    for (let x = 0; x + 32 <= wide; x += 32) {
      let seen = 0;
      let meant = 0;
      for (let k = x; k < x + 32; k++) {
        seen += bytes[row + k * 4];
        meant += want(k);
      }
      worst = Math.max(worst, Math.abs(seen - meant) / 32);
    }
    // Rounded alone, a window was a third of a step off: 0.33.
    expect(worst).toBeLessThan(0.15);
  });
});
