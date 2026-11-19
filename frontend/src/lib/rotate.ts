import type { Rotation, SourceView } from "./api";
import { FRAME_HAIR } from "./flow";

// Rotate: the part of a clip between two cuts, rotated a quarter turn to
// the left at a time, for a phone that was turned while it recorded. What
// turns is the stretch of the video the part shows, kept in the clip's
// rotations, see engine.Rotation, and a frame is turned by the stretch its
// start lies in, in the video preview as in the render. Plan row 3.25.

// The picture as a piece rotated by so many degrees shows it: its size and
// how wide the crop is on it. A quarter turn either way swaps the width and
// the height.
export function pictureOf(source: SourceView, rotate: number): { width: number; height: number; cropWidth: number } {
  if (rotate % 180 !== 0) {
    return { width: source.height, height: source.width, cropWidth: source.turnedCropWidth ?? source.width };
  }
  return { width: source.width, height: source.height, cropWidth: source.cropWidth };
}

// How far the frame that starts at a moment is turned: by the stretch of
// the clip's rotations it lies in, engine.RotationAt. A moment a hair
// before a stretch starts is in it, since a file's frames stand a little
// off the millisecond the plan keeps.
export function rotationAt(rotations: Rotation[] | undefined, at: number): number {
  for (const r of rotations ?? []) {
    if (at >= (r.from ?? -Infinity) - FRAME_HAIR && at < (r.to ?? Infinity) - FRAME_HAIR) return r.deg;
  }
  return 0;
}

// The moment the picture on the canvas is turned for: the start of the
// frame that is on it, as the frame queue says, and the playhead only
// before there is a frame. The playhead runs ahead of its frames, longer
// going back, so turned for the playhead, a frame from one side of a cut
// stood turned the way the other side is, and back, which Tim saw on
// #188. The render turns a frame by its own start, so this does too, and
// never guesses from the playhead or the length of a frame, which a phone
// that records at a varying rate does not keep.
export function shownAt(frameStart: number, playhead: number): number {
  return Number.isFinite(frameStart) ? frameStart : playhead;
}

// The piece that holds a moment, or -1 for a moment in a cut or outside the
// clip.
export function pieceHolding(pieces: { start: number; end: number }[], at: number): number {
  return pieces.findIndex((p) => at >= p.start - FRAME_HAIR && at < p.end - FRAME_HAIR);
}
