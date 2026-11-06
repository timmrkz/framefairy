// The faint reflection of the video preview at the foot of the workspace,
// as if the video shone towards the person in front of it and a glossy
// desk gave a little of it back. It is the frame on screen, upside down,
// made small, blurred and faded here, and stretched along the bottom edge
// of the app by the stylesheet, see Reflection.svelte. Tim picked it from
// the mockups of #717.
//
// It costs playback nothing it would notice: the frame is read on the
// processor where it already is, at COLS by ROWS cells of four pixels
// each, 9216 pixels however large the video, never read back from the GPU
// and never decoded again. The blur and the fade are worked out here, on
// those cells, and not by a filter and a mask in the stylesheet: the
// browser would then blur the band anew, its full size and as far again
// as the blur reaches around it, every time a cell changes, which is 30
// times a second while a video plays. Without a graphics chip, as on a
// computer with a weak one, that took more of the processor than playing
// the video did.

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

// What spread made, as the bytes a canvas takes, whose red, green and blue
// are not multiplied by the alpha.
export function painted(from: Float32Array, into: Uint8ClampedArray) {
  for (let i = 0; i < from.length; i += 4) {
    const a = from[i + 3];
    const k = a > 0 ? 255 / a : 0;
    into[i] = from[i] * k;
    into[i + 1] = from[i + 1] * k;
    into[i + 2] = from[i + 2] * k;
    into[i + 3] = a;
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
