import type { ClipEntry, Segment, SourceView } from "./api";

// Rotate: the part of a clip between two cuts, rotated a quarter turn to
// the left at a time, for a phone that was turned while it recorded. The
// engine answers for it, engine.RotatePart, and this is the same rule, so
// the video preview turns in the frame the key is pressed in rather than
// when the answer comes back. Plan row 3.25.

// The picture as a piece rotated by so many degrees shows it: its size and
// how wide the crop is on it. A quarter turn either way swaps the width and
// the height.
export function pictureOf(source: SourceView, rotate: number): { width: number; height: number; cropWidth: number } {
  if (rotate % 180 !== 0) {
    return { width: source.height, height: source.width, cropWidth: source.turnedCropWidth ?? source.width };
  }
  return { width: source.width, height: source.height, cropWidth: source.cropWidth };
}

// The first and last of the pieces around the one at i that play straight
// on from each other, with no cut between them: engine.partOf.
export function partOf(pieces: { start: number; end: number }[], i: number): [number, number] {
  let first = i;
  let last = i;
  while (first > 0 && pieces[first].start - pieces[first - 1].end <= 0.0005) first--;
  while (last + 1 < pieces.length && pieces[last + 1].start - pieces[last].end <= 0.0005) last++;
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

// The clip as it will be once the part at a moment is rotated a quarter
// further, the way engine.RotatePart leaves it: the part's pieces turned
// together, and its crop in the middle of the picture rotated. Turned all
// the way round, the part waits for the engine to give back the crop its
// shot was found with, and stands in the middle until then.
export function rotated(clip: ClipEntry, source: SourceView, at: number): ClipEntry {
  const i = pieceAt(clip.segments, at);
  if (i < 0) return clip;
  const [first, last] = partOf(clip.segments, i);
  const turned = ((clip.segments[i].rotate ?? 0) + 90) % 360;
  const picture = pictureOf(source, turned);
  const middle = Math.floor((picture.width - picture.cropWidth) / 2);
  const segments: Segment[] = clip.segments.map((s, k) =>
    k < first || k > last ? s : { ...s, rotate: turned, cropX: null, moved: false },
  );
  const cropLefts = clip.cropLefts.map((x, k) => (k < first || k > last ? x : middle - (middle % 2)));
  return { ...clip, segments, cropLefts };
}
