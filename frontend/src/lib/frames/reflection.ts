// The faint reflection of the video preview at the foot of the workspace,
// as if the video shone towards the person in front of it and a glossy
// desk gave a little of it back. It is the frame on screen, upside down,
// made small, blurred and faded here, and stretched along the bottom edge
// of the app by the stylesheet, see Reflection.svelte. Tim picked it from
// the mockups of #717.
//
// The frame is read on the processor where it already is, at COLS by
// ROWS cells of four pixels each, 9216 pixels however large the video,
// never read back from the GPU and never decoded again. The blur and the
// fade are worked out here, on those cells, and the cells are then drawn
// on every pixel of the band, with noise to keep a faint shade from
// falling into bands, by the graphics chip where there is one, see
// reflected.ts, and by painted here where there is none. Not by a filter
// and a mask in the stylesheet: the browser would then blur the band
// anew, its full size and as far again as the blur reaches around it,
// every time a cell changes, which is 30 times a second while a video
// plays. Without a graphics chip, as on a computer with a weak one, that
// took more of the processor than playing the video did.

import { toSDR, type Light } from "./light";

// The reflection's cells across and down. The blur over it is wider than
// a cell, so more of them would show nothing more.
export const COLS = 64;
export const ROWS = 36;
// How far the blur reaches, in pixels of the app: the standard deviation
// of its bell, as the stylesheet's blur() takes it, and as Tim set it in
// the mockups of #717.
export const BLUR = 100;
// How much of the reflection is seen where it is strongest, at the foot of
// the app under its middle.
export const STRENGTH = 0.5;
// How long a change of picture takes to settle, in seconds: after this
// the reflection is two thirds of the way to the new frame, so a cut
// fades over rather than flashing.
export const SETTLE = 0.25;

// Each cell is the mean of a square of SPREAD by SPREAD pixels spread
// evenly over it, so a fine pattern in the video does not flicker in the
// reflection as it moves.
const SPREAD = 2;
const words = new Uint32Array(COLS * ROWS * SPREAD * SPREAD);
const colours = new Uint8ClampedArray(words.length * 4);

// The frame as COLS by ROWS colours of 0 to 255, red, green, blue and an
// opaque alpha, row after row, upside down: the frame's top row is the
// reflection's bottom, the row nearest to the person. The colours are the
// ones the video preview shows, standard video with its look and HDR cut
// at white, see light.ts. data is the frame's bytes, 4 a pixel, see
// picture.ts.
export function mirrored(data: Uint8Array, width: number, height: number, light: Light, into: Float32Array) {
  const across = COLS * SPREAD;
  const down = ROWS * SPREAD;
  const src = new Uint32Array(data.buffer, data.byteOffset, width * height);
  for (let y = 0; y < down; y++) {
    const row = Math.min(height - 1, Math.floor(((y + 0.5) * height) / down)) * width;
    for (let x = 0; x < across; x++) {
      words[y * across + x] = src[row + Math.min(width - 1, Math.floor(((x + 0.5) * width) / across))];
    }
  }
  toSDR(light, new Uint8Array(words.buffer), colours);
  const n = SPREAD * SPREAD;
  for (let r = 0; r < ROWS; r++) {
    const at = (ROWS - 1 - r) * COLS * 4;
    for (let c = 0; c < COLS; c++) {
      let red = 0;
      let green = 0;
      let blue = 0;
      for (let dy = 0; dy < SPREAD; dy++) {
        for (let dx = 0; dx < SPREAD; dx++) {
          const i = ((r * SPREAD + dy) * across + c * SPREAD + dx) * 4;
          red += colours[i];
          green += colours[i + 1];
          blue += colours[i + 2];
        }
      }
      into[at + c * 4] = red / n;
      into[at + c * 4 + 1] = green / n;
      into[at + c * 4 + 2] = blue / n;
      into[at + c * 4 + 3] = 255;
    }
  }
}

// The band the reflection is drawn in, wide by high pixels of the app, as
// the stylesheet lays it out: for each cell, how much of every other
// cell's light the blur carries onto it, across and down, and how much of
// it is seen there, faded out at the sides and upwards. Beyond the band
// there is nothing, so the blur thins out towards its edges the way the
// stylesheet's does.
export type Band = { across: Float32Array; down: Float32Array; seen: Float32Array };

export function band(wide: number, high: number): Band {
  const seen = new Float32Array(COLS * ROWS);
  for (let r = 0; r < ROWS; r++) {
    // From the foot, the row nearest to the person, upwards.
    const up = fade((ROWS - r - 0.5) / ROWS, UPWARDS);
    for (let c = 0; c < COLS; c++) seen[r * COLS + c] = STRENGTH * up * fade((c + 0.5) / COLS, SIDEWAYS);
  }
  return { across: bell(COLS, wide / COLS), down: bell(ROWS, high / ROWS), seen };
}

// How the reflection fades, as the stops of a gradient: at what share of
// the way how much of it is seen, in between in a straight line. Upwards
// from the foot, and across from the left.
const UPWARDS: [number, number][] = [
  [0, 1],
  [0.35, 0.45],
  [0.7, 0.12],
  [1, 0],
];
const SIDEWAYS: [number, number][] = [
  [0, 0],
  [0.25, 1],
  [0.75, 1],
  [1, 0],
];

function fade(at: number, stops: [number, number][]): number {
  for (let i = 1; i < stops.length; i++) {
    const [a, x] = stops[i - 1];
    const [b, y] = stops[i];
    if (at <= b) return x + ((y - x) * (at - a)) / (b - a);
  }
  return stops[stops.length - 1][1];
}

// For n cells of size pixels each, row i says how much of cell j's light
// the blur lands on the middle of cell i: the share of the bell around
// that middle which cell j covers.
function bell(n: number, size: number): Float32Array {
  const w = new Float32Array(n * n);
  const sigma = BLUR / size;
  for (let i = 0; i < n; i++)
    for (let j = 0; j < n; j++) {
      const d = i - j;
      w[i * n + j] = sigma > 0 ? below((d + 0.5) / sigma) - below((d - 0.5) / sigma) : d === 0 ? 1 : 0;
    }
  return w;
}

// The share of a bell of width 1 that lies below x, from the error
// function as Abramowitz and Stegun give it, 7.1.26, to within 1.5e-7.
function below(x: number): number {
  const z = Math.abs(x) / Math.SQRT2;
  const t = 1 / (1 + 0.3275911 * z);
  const erf = 1 - ((((1.061405429 * t - 1.453152027) * t + 1.421413741) * t - 0.284496736) * t + 0.254829592) * t * Math.exp(-z * z);
  return x >= 0 ? 0.5 * (1 + erf) : 0.5 * (1 - erf);
}

const across = new Float32Array(COLS * ROWS * 4);

// The cells of a frame, as mirrored makes them, blurred and faded over the
// band, into the colours drawn: red, green and blue already multiplied by
// how much of them is seen, the alpha, so that two of them mix the way the
// screen mixes them. A frame faded in or out keeps its colours.
export function spread(from: Float32Array, b: Band, into: Float32Array) {
  for (let r = 0; r < ROWS; r++)
    for (let c = 0; c < COLS; c++) {
      const w = b.across.subarray(c * COLS, c * COLS + COLS);
      let red = 0;
      let green = 0;
      let blue = 0;
      let alpha = 0;
      for (let j = 0; j < COLS; j++) {
        const i = (r * COLS + j) * 4;
        const a = (w[j] * from[i + 3]) / 255;
        red += a * from[i];
        green += a * from[i + 1];
        blue += a * from[i + 2];
        alpha += a * 255;
      }
      const o = (r * COLS + c) * 4;
      across[o] = red;
      across[o + 1] = green;
      across[o + 2] = blue;
      across[o + 3] = alpha;
    }
  for (let r = 0; r < ROWS; r++) {
    const w = b.down.subarray(r * ROWS, r * ROWS + ROWS);
    for (let c = 0; c < COLS; c++) {
      const o = (r * COLS + c) * 4;
      const seen = b.seen[r * COLS + c];
      for (let k = 0; k < 4; k++) {
        let sum = 0;
        for (let j = 0; j < ROWS; j++) sum += w[j] * across[(j * COLS + c) * 4 + k];
        into[o + k] = sum * seen;
      }
    }
  }
}

// The pixels the reflection is drawn on, wide by high pixels of the
// canvas: the row of cells above each row of pixels and how far it is on
// the way to the next, and the pixel each cell's middle falls on across,
// so a pixel is the mix of the four cells round it, and along a row the
// colour moves by the same amount from one pixel to the next between two
// middles. Before the first middle and after the last it stays.
export type Raster = { wide: number; high: number; row: Int32Array; down: Float32Array; middle: Int32Array };

export function raster(wide: number, high: number): Raster {
  const row = new Int32Array(high);
  const down = new Float32Array(high);
  for (let y = 0; y < high; y++) {
    // The middle of pixel y, in cells, from the middle of the first.
    const v = Math.min(ROWS - 1, Math.max(0, ((y + 0.5) * ROWS) / high - 0.5));
    row[y] = Math.min(ROWS - 2, Math.floor(v));
    down[y] = v - row[y];
  }
  // The first pixel whose middle is at or past the middle of cell c.
  const middle = new Int32Array(COLS);
  for (let c = 0; c < COLS; c++) middle[c] = Math.min(wide, Math.max(0, Math.ceil(((c + 0.5) * wide) / COLS - 0.5)));
  return { wide, high, row, down, middle };
}

// The noise added to every pixel before it is cut to the 256 steps a
// screen has, from nothing to a whole step, the same for a pixel every
// time it is drawn so it never flickers. A reflection is faint, its
// brightest a few dozen steps over the ground, so rounded alone it fell
// into bands a step apart, their edges stepping from cell to cell, and
// Tim saw them as pixels. The noise turns the edge of every band into a
// mix of the steps on either side, which the eye takes as the shade in
// between. It is interleaved gradient noise, Jimenez 2014: next to no
// pattern, and no two neighbours alike. Never a whole step, so the
// ground, a whole number, is cut to itself and the band has no edge.
const TILE = 64;
const noise = new Float32Array(TILE * TILE);
for (let y = 0; y < TILE; y++)
  for (let x = 0; x < TILE; x++) {
    const n = (52.9829189 * ((0.06711056 * x + 0.00583715 * y) % 1)) % 1;
    noise[y * TILE + x] = (Math.floor(n * 255) + 0.5) / 256;
  }

// What spread made, laid over the ground, the colour of the app behind the
// band: each cell's red, green, blue and an alpha of 255, in steps of 0 to
// 255 and not yet cut to whole ones. Mixed as numbers and cut once, where
// each pixel is drawn: a canvas that kept the band's own transparency
// stored its colours already multiplied by it, in 256 steps, and a faint
// reflection had a handful of steps left before the screen rounded it
// again. The graphics chip draws from these, see reflected.ts.
export function laid(from: Float32Array, ground: readonly [number, number, number], into: Float32Array) {
  for (let i = 0; i < from.length; i += 4) {
    const rest = 1 - from[i + 3] / 255;
    into[i] = Math.min(255, from[i] + ground[0] * rest);
    into[i + 1] = Math.min(255, from[i + 1] + ground[1] * rest);
    into[i + 2] = Math.min(255, from[i + 2] + ground[2] * rest);
    into[i + 3] = 255;
  }
}

const opaque = new Float32Array(COLS * ROWS * 4);
const line = new Float32Array(COLS * 3);

// The cells laid over the ground, drawn on the processor, as the pixels of
// an opaque canvas of r's size, each one word of red, green, blue and
// alpha from the lowest byte up, the way ImageData's bytes are read on a
// little-endian machine, which every machine the app runs on is. Where
// the graphics chip cannot draw it, see reflected.ts.
export function painted(from: Float32Array, ground: readonly [number, number, number], r: Raster, into: Uint32Array) {
  laid(from, ground, opaque);
  const { wide, high, row, down, middle } = r;
  const step = COLS / wide;
  for (let y = 0; y < high; y++) {
    const above = row[y] * COLS * 4;
    const f = down[y];
    for (let c = 0; c < COLS; c++)
      for (let k = 0; k < 3; k++) {
        const a = above + c * 4 + k;
        line[c * 3 + k] = opaque[a] + (opaque[a + COLS * 4] - opaque[a]) * f;
      }
    const tile = (y % TILE) * TILE;
    const start = y * wide;
    // Before the first middle, the first cell's colour.
    let x = 0;
    for (let c = -1; c < COLS; c++) {
      const end = c + 1 < COLS ? middle[c + 1] : wide;
      const k = Math.max(0, c) * 3;
      const n = Math.min(COLS - 1, c + 1) * 3;
      // Where pixel x lies between the middles of cell c and the next.
      const at = c < 0 || c + 1 >= COLS ? 0 : (x + 0.5) * step - 0.5 - c;
      const by = c < 0 || c + 1 >= COLS ? 0 : step;
      let red = line[k] + (line[n] - line[k]) * at;
      let green = line[k + 1] + (line[n + 1] - line[k + 1]) * at;
      let blue = line[k + 2] + (line[n + 2] - line[k + 2]) * at;
      const dr = (line[n] - line[k]) * by;
      const dg = (line[n + 1] - line[k + 1]) * by;
      const db = (line[n + 2] - line[k + 2]) * by;
      for (; x < end; x++) {
        const d = noise[tile + (x & (TILE - 1))];
        into[start + x] = (0xff000000 | (((blue + d) | 0) << 16) | (((green + d) | 0) << 8) | ((red + d) | 0)) >>> 0;
        red += dr;
        green += dg;
        blue += db;
      }
    }
  }
}

// How much of the way to the new frame the reflection goes in dt seconds.
export function share(dt: number): number {
  return dt <= 0 ? 0 : 1 - Math.exp(-dt / SETTLE);
}

// Moves now that share of the way to then, and says how far the colour
// furthest from its goal still has to go, in steps of 0 to 255.
export function ease(now: Float32Array, then: Float32Array, by: number): number {
  let left = 0;
  for (let i = 0; i < now.length; i++) {
    now[i] += (then[i] - now[i]) * by;
    left = Math.max(left, Math.abs(then[i] - now[i]));
  }
  return left;
}

// Black in the video preview, which has no frame: the colours kept and
// the alpha gone, so the reflection fades out where it was rather than
// darkening the foot of the app.
export function clear(into: Float32Array) {
  for (let i = 3; i < into.length; i += 4) into[i] = 0;
}
