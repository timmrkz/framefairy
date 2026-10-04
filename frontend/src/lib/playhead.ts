// Where the playhead is, as far as playing goes: on the chosen clip or on
// the video. It is a state, set by the gesture that put the playhead
// there, and never worked out from the video's clock.
//
// It used to be a distance. The space bar played the clip when the
// playhead stood within half a frame of its start and before its end, and
// the episode anywhere else. But the playhead is never where it was put:
// the paused video on the Mac answers with where the frame it shows
// begins, up to a frame before the moment it was sent to, a video that has
// read nothing answers zero, and every answer replaced the playhead. A
// clip just picked then measured as before its own start, and the space
// bar played the episode straight through its cuts and past its end. Each
// fix moved the place the measurement could round to the wrong side. A
// gesture knows what it is about, so it says, and nothing after it can
// change that. See Playback in .claude/skills/interface/SKILL.md.

import type { Piece } from "./flow";

// On the clip, on the clip at its end, or on the video. The end is a place
// of its own because the space bar starts the clip over from there, and
// the video's answer, a frame before the end, must not make that a play
// of the clip's last frame.
export type Place = "video" | "clip" | "end";

// The place, and the clip it is about. A place on one clip says nothing
// about another, so a clip chosen without a gesture that put the playhead
// on it is on the video.
export type Playhead = { place: Place; clip: string };

export const onVideo: Playhead = { place: "video", clip: "" };

// Where a gesture that puts the playhead at a moment leaves it, for the
// clip chosen, by its key and its pieces.
//
// A gesture about the clip says so, about: picking it, a click on one of
// its edges, a trim, a click on a caption. That decides an edge, which is
// the clip's own, and a trim, whose playhead stands on the edge being
// dragged and so can be a hair outside the pieces the video preview has
// at that moment. Every other gesture, a click, a drag, a step or a seek,
// is decided by where it lands: from the clip's first start to its last
// end, cuts included, is on the clip, and anywhere else on the video.
export function placeOf(pieces: Piece[], key: string, at: number, about?: "clip"): Playhead {
  if (!key || !pieces.length) return onVideo;
  const start = pieces[0].start;
  const end = pieces[pieces.length - 1].end;
  if (about !== "clip" && (at < start || at > end)) return onVideo;
  return { place: at >= end ? "end" : "clip", clip: key };
}

// The place for the clip chosen now.
export function placeFor(p: Playhead, key: string | null | undefined): Place {
  return key && p.clip === key ? p.place : "video";
}

// Where a press of the space bar plays from, and whether it plays the clip,
// its cuts jumped and a stop at its end, or the episode straight on.
//
// On the video it is the episode from the playhead, through the clip and
// its cuts. Loop is not asked: it is about the clip, and on the video the
// clip's rules are not in play.
//
// On the clip it is the clip from the playhead. From a cut, the end of that
// cut. From before the clip's start, which only the video's answer can put
// the playhead at, and from its end, the start: the clip starts over, the
// way QuickTime starts a video over from its end.
export function playFrom(place: Place, pieces: Piece[], at: number): { clip: boolean; at: number } {
  if (place === "video" || !pieces.length) return { clip: false, at };
  const start = pieces[0].start;
  if (place === "end") return { clip: true, at: start };
  for (const p of pieces) {
    if (at < p.start) return { clip: true, at: p.start };
    if (at < p.end) return { clip: true, at };
  }
  return { clip: true, at: start };
}

// Where a play of the clip leaves the playhead once it reaches the clip's
// end: at its end, or with loop on back at its start, on the clip either
// way.
export function playedToEnd(p: Playhead, looping: boolean): Playhead {
  return { place: looping ? "clip" : "end", clip: p.clip };
}
