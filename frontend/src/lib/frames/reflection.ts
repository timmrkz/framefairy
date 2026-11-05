// The faint reflection of the video preview at the foot of the workspace,
// as if the video shone towards the person in front of it and a glossy
// desk gave a little of it back. It is the frame on screen, upside down,
// made small here and then stretched, blurred and faded by the stylesheet,
// see Reflection.svelte. Tim picked it from the mockups of #717.
//
// It costs playback nothing it would notice: the frame is read on the
// processor where it already is, at COLS by ROWS cells of four pixels
// each, a few thousand pixels however large the video, never read back
// from the GPU and never decoded again.

import { toSDR, type Light } from "./light";

// The reflection's cells across and down. The blur over it is wider than
// a cell, so more of them would show nothing more.
export const COLS = 64;
export const ROWS = 36;
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

// The frame as COLS by ROWS colours of 0 to 255, red, green and blue, row
// after row, upside down: the frame's top row is the reflection's bottom,
// the row nearest to the person. The colours are the ones the video
// preview shows, standard video with its look and HDR cut at white, see
// light.ts. data is the frame's bytes, 4 a pixel, see picture.ts.
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
    const at = (ROWS - 1 - r) * COLS * 3;
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
      into[at + c * 3] = red / n;
      into[at + c * 3 + 1] = green / n;
      into[at + c * 3 + 2] = blue / n;
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

// The colours as a 2D canvas takes them, opaque.
export function pixels(from: Float32Array, into: Uint8ClampedArray) {
  for (let p = 0, i = 0; p < from.length; p += 3, i += 4) {
    into[i] = from[p];
    into[i + 1] = from[p + 1];
    into[i + 2] = from[p + 2];
    into[i + 3] = 255;
  }
}
