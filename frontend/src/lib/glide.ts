// A crop that follows, plan row 3.26. The crop of a piece with crop marks
// glides from each mark to the next, see CropMark in engine/cropmarks.go,
// and the video preview places its crop frame by the same sum the render
// gives ffmpeg, so what plays is what renders. glide.cases.json holds this
// file and the engine to the same left edge on every frame.

import type { CropMark } from "./api";

// A crop mark on its frame of the video, with its left edge.
export type FrameMark = [frame: number, left: number];

// The frame of the video a moment means: the one whose start is nearest,
// half way going to the later, the way the engine's nearestFrame counts.
export function frameOf(at: number, fps: number, videoStart = 0): number {
  return Math.floor((at - videoStart) * fps + 0.5);
}

// The left edge of the crop on frame k, gliding over marks on frames in
// order: still before the first and after the last, and between two the
// whole way by smoothstep, u²(3-2u), on an even pixel. Each step is the
// engine's glideX, in its order, so the two agree to the pixel.
export function glideX(marks: FrameMark[], k: number): number {
  if (k <= marks[0][0]) return marks[0][1];
  for (let i = 1; i < marks.length; i++) {
    const [kb, xb] = marks[i];
    if (k >= kb) continue;
    const [ka, xa] = marks[i - 1];
    const u = (k - ka) / (kb - ka);
    const ease = u * u * (3 - 2 * u);
    const x = xa + (xb - xa) * ease;
    return 2 * Math.floor(x / 2 + 0.5);
  }
  return marks[marks.length - 1][1];
}

// A crop's left edge kept inside the picture and on an even pixel, the
// engine's ClampCropX.
export function clampLeft(left: number, cropWidth: number, width: number): number {
  const x = Math.max(0, Math.min(left, width - cropWidth));
  return x - (x % 2);
}

// The left edge of a piece's crop on the frame that starts at a moment,
// gliding over the piece's marks, or null for a piece with none, whose
// crop stands still.
export function glidingLeft(
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
  return glideX(onFrames, frameOf(at, fps, videoStart));
}

// The crop mark of a clip on the frame a moment means, or null.
export function markOn(marks: CropMark[] | undefined, at: number, fps: number, videoStart = 0): CropMark | null {
  if (!marks?.length || !(fps > 0)) return null;
  const k = frameOf(at, fps, videoStart);
  return marks.find((m) => frameOf(m.at, fps, videoStart) === k) ?? null;
}
