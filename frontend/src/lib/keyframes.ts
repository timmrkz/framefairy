// Keyframes, plan row 3.26 and #750. A keyframe is a turning point: the
// crop of a piece switches on its frame and holds until the next one, see
// CropMark and heldX in engine/cropmarks.go. The video preview places its
// crop frame by the same choice the render gives ffmpeg, so what plays is
// what renders. keyframes.cases.json holds this file and the engine to the
// same left edge on every frame.

import type { CropMark } from "./api";
import { frameAt } from "./flow";

// A keyframe on its frame of the video, with its left edge.
export type FrameMark = [frame: number, left: number];

// The frame of the video a moment means: the one whose start is nearest,
// half way going to the later, the way the engine's nearestFrame counts.
export function frameOf(at: number, fps: number, videoStart = 0): number {
  return Math.floor((at - videoStart) * fps + 0.5);
}

// The left edge of the crop on frame k: that of the last keyframe on or
// before it, or null before the first, where the crop is the piece's own.
// The engine's heldX, which takes the piece's crop for null.
export function heldX(marks: FrameMark[], k: number): number | null {
  let x: number | null = null;
  for (const [km, xm] of marks) {
    if (km > k) break;
    x = xm;
  }
  return x;
}

// A crop's left edge kept inside the picture and on an even pixel, the
// engine's ClampCropX.
export function clampLeft(left: number, cropWidth: number, width: number): number {
  const x = Math.max(0, Math.min(left, width - cropWidth));
  return x - (x % 2);
}

// The left edge of a piece's crop on the frame that holds a moment, the
// frame on the canvas: that of the keyframe it holds from, or null before
// the piece's first keyframe and for a piece with none, whose crop is its
// own.
export function heldLeft(
  marks: CropMark[] | undefined,
  at: number,
  fps: number,
  videoStart: number,
  cropWidth: number,
  width: number,
): number | null {
  if (!marks?.length || !(fps > 0)) return null;
  const onFrames: FrameMark[] = [];
  for (const m of marks) {
    const mark: FrameMark = [frameOf(m.at, fps, videoStart), clampLeft(m.x, cropWidth, width)];
    const last = onFrames[onFrames.length - 1];
    if (last && last[0] === mark[0]) onFrames[onFrames.length - 1] = mark;
    else onFrames.push(mark);
  }
  return heldX(onFrames, frameAt(at, 1 / fps, videoStart));
}

// The keyframe of a clip on the frame that holds a moment, the playhead's,
// or null.
export function markOn(marks: CropMark[] | undefined, at: number, fps: number, videoStart = 0): CropMark | null {
  if (!marks?.length || !(fps > 0)) return null;
  const k = frameAt(at, 1 / fps, videoStart);
  return marks.find((m) => frameOf(m.at, fps, videoStart) === k) ?? null;
}
