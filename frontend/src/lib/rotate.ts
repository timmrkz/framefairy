import type { ClipEntry, Rotation, Segment, SourceView } from "./api";
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

// The most two pieces may stand apart and play straight on, with no cut
// between them: engine.PiecesMeet.
export const PIECES_MEET = 0.0005;

// The first and last of the pieces around the one at i that play straight
// on from each other, with no cut between them: engine.partOf.
export function partOf(pieces: { start: number; end: number }[], i: number): [number, number] {
  let first = i;
  let last = i;
  while (first > 0 && pieces[first].start - pieces[first - 1].end <= PIECES_MEET) first--;
  while (last + 1 < pieces.length && pieces[last + 1].start - pieces[last].end <= PIECES_MEET) last++;
  return [first, last];
}

// The piece that holds a moment, or the nearest: engine.segmentAt.
export function pieceAt(pieces: { start: number; end: number }[], at: number): number {
  let best = -1;
  let gap = Infinity;
  pieces.forEach((p, i) => {
    const d = at < p.start ? p.start - at : at >= p.end ? at - p.end : 0;
    if (d < gap) {
      best = i;
      gap = d;
    }
  });
  return best;
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

// The clip as it will be once the part at a moment is rotated a quarter
// further, for the video preview to turn in the frame the key is pressed
// in rather than when the engine's answer comes back, which replaces it:
// the stretch from the middle of the cut before the part to the middle of
// the cut after, or to that end of the video, turned, and its crop in the
// middle of the picture rotated. Its keyframes stay, and steer it only
// turned the way they were placed, see engine.CropMark. The engine puts
// each edge on a frame.
export function rotated(clip: ClipEntry, source: SourceView, at: number): ClipEntry {
  const i = pieceAt(clip.segments, at);
  if (i < 0) return clip;
  const [first, last] = partOf(clip.segments, i);
  const segs = clip.segments;
  const from = first > 0 ? (segs[first - 1].end + segs[first].start) / 2 : null;
  const to = last + 1 < segs.length ? (segs[last].end + segs[last + 1].start) / 2 : null;
  const turned = (rotationAt(clip.rotations, segs[i].start) + 90) % 360;
  const rotations = turnStretch(clip.rotations ?? [], from, to, turned);
  const segments: Segment[] = segs.map((s, k) => {
    const rotate = rotationAt(rotations, s.start);
    if (k < first || k > last) return { ...s, rotate };
    return { ...s, rotate, marks: s.marks?.filter((m) => (m.turn ?? 0) === turned) };
  });
  const cropLefts = clip.cropLefts.map((x, k) => {
    if (k < first || k > last || turned === 0) return x;
    const picture = pictureOf(source, turned);
    const middle = Math.floor((picture.width - picture.cropWidth) / 2);
    return middle - (middle % 2);
  });
  return { ...clip, rotations, segments, cropLefts };
}

// The rotations with the stretch from from to to turned deg degrees, and
// stretches that meet turned alike made one: engine.turnStretch.
function turnStretch(spans: Rotation[], from: number | null, to: number | null, deg: number): Rotation[] {
  const lo = from ?? -Infinity;
  const hi = to ?? Infinity;
  const out: { from: number; to: number; deg: number }[] = [];
  for (const r of spans) {
    const a = r.from ?? -Infinity;
    const b = r.to ?? Infinity;
    if (b <= lo || a >= hi) {
      out.push({ from: a, to: b, deg: r.deg });
      continue;
    }
    if (a < lo) out.push({ from: a, to: lo, deg: r.deg });
    if (b > hi) out.push({ from: hi, to: b, deg: r.deg });
  }
  if (deg !== 0) out.push({ from: lo, to: hi, deg });
  out.sort((x, y) => x.from - y.from);
  const merged: typeof out = [];
  for (const r of out) {
    const prev = merged[merged.length - 1];
    if (prev && prev.deg === r.deg && prev.to >= r.from) prev.to = Math.max(prev.to, r.to);
    else merged.push({ ...r });
  }
  return merged.map((r) => ({
    from: Number.isFinite(r.from) ? r.from : null,
    to: Number.isFinite(r.to) ? r.to : null,
    deg: r.deg,
  }));
}
