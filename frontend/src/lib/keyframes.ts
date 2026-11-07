// Keyframes, plan row 3.26 and #750. A keyframe is a turning point: the
// crop of a piece switches on its frame and holds until the next one. The
// engine works out how each piece's crop runs over its frames, the same
// the render crops by, and sends it with the clip as cropSteps, see
// engine.CropSteps. The window keeps no rule of its own about it: it only
// takes the step for the frame it shows.

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
  let x = steps[0].x;
  for (const step of steps) {
    if (step.frame > k) break;
    x = step.x;
  }
  return x;
}

// The keyframe of a clip on the frame that holds a moment, the playhead's,
// or null.
export function markOn(marks: CropMark[] | undefined, at: number, fps: number, videoStart = 0): CropMark | null {
  if (!marks?.length || !(fps > 0)) return null;
  const k = frameAt(at, 1 / fps, videoStart);
  return marks.find((m) => frameOf(m.at, fps, videoStart) === k) ?? null;
}
