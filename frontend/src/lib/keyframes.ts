// Keyframes, plan row 3.26 and #750. A keyframe is a turning point: from
// its frame on, the crop of a piece is the keyframe's until the next one.
// A diamond switches the crop on its frame, and from a circle the crop
// glides into the next circle. The engine works out how each piece's crop
// runs over its frames, the same the render crops by, and sends it with
// the clip as cropSteps, see engine.CropSteps, a step on every frame of a
// glide. The window keeps no rule of its own about it: it only takes the
// step for the frame it shows.

import type { CropMark, CropStep } from "./api";
import { frameAt } from "./flow";

// The frame of the video a moment means: the one whose start is nearest,
// half way going to the later, the way the engine's nearestFrame counts.
export function frameOf(at: number, fps: number, videoStart = 0): number {
  return Math.floor((at - videoStart) * fps + 0.5);
}

// The left edge of a piece's crop on the frame that holds a moment, the
// frame on the canvas, from the steps the engine sent for the piece: the
// last to start on or before that frame, or the first for a frame before
// it. Null for a piece with no steps, whose crop is its own throughout.
export function heldLeft(steps: CropStep[] | null | undefined, at: number, fps: number, videoStart: number): number | null {
  if (!steps?.length || !(fps > 0)) return null;
  const k = frameAt(at, 1 / fps, videoStart);
  // The last step on or before k, found by halving: a glide is a step a
  // frame, and this is asked on every frame drawn.
  let lo = 0;
  let hi = steps.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (steps[mid].frame <= k) lo = mid;
    else hi = mid - 1;
  }
  return steps[lo].x;
}

// The keyframe of a clip on the frame that holds a moment, the playhead's,
// or null.
export function markOn(marks: CropMark[] | undefined, at: number, fps: number, videoStart = 0): CropMark | null {
  if (!marks?.length || !(fps > 0)) return null;
  const k = frameAt(at, 1 / fps, videoStart);
  return marks.find((m) => frameOf(m.at, fps, videoStart) === k) ?? null;
}
