// Where the playhead is, as far as playing goes: on the chosen clip or on
// the video. It is a state, set by the gesture that put the playhead
// there, and never worked out from a clock.
//
// It used to be a distance, when the video preview was a video element.
// The space bar played the clip when the playhead stood within half a
// frame of its start and before its end, and the episode anywhere else.
// But the playhead was never where it was put: the paused video on the
// Mac answered with where the frame it showed began, up to a frame before
// the moment it was sent to, a video that had read nothing answered zero,
// and every answer replaced the playhead. A
// clip just picked then measured as before its own start, and the space
// bar played the episode straight through its cuts and past its end. Each
// fix moved the place the measurement could round to the wrong side. A
// gesture knows what it is about, so it says, and nothing after it can
// change that. See Playback in .claude/skills/interface/SKILL.md.

import { frameAt, type Piece } from "./flow";

// On the clip, on the clip at its end, or on the video. The end is a place
// of its own because the space bar starts the clip over from there, and
// a playhead in the clip's last frame, where a trim on frames leaves it,
// must not make that a play of that one frame.
export type Place = "video" | "clip" | "end";

// The place, and the clip it is about. A place on one clip says nothing
// about another, so a clip chosen without a gesture that put the playhead
// on it is on the video.
export type Playhead = { place: Place; clip: string };

export const onVideo: Playhead = { place: "video", clip: "" };

// Where a gesture that puts the playhead at a moment leaves it, for the
// clip chosen, by its key and its pieces, in an episode of fps frames a
// second whose picture starts videoStart seconds into the file.
//
// A gesture about the clip says so, about: picking it, a click on one of
// its edges, a trim, a click on a caption. That decides an edge, which is
// the clip's own, and a trim, whose playhead stands on the edge being
// dragged and so can be a hair outside the pieces the video preview has
// at that moment. Every other gesture, a click, a drag, a step or a seek,
// is decided by the frame it lands in: from the frame that holds the
// clip's start to the frame that holds the moment a frame before its end,
// cuts included, is on the clip, and any other frame is on the video.
// Either way, that last frame is the clip's end, and so is the end itself.
// The edges are on frames, see frameAt in lib/flow.ts, so these are the
// frames the short starts and ends with.
//
// By frame and not by second, because what is on screen is a frame. The
// video element on the Mac answered a seek with where its frame began,
// and steps went from there, so a step left a clip that started inside a
// frame while that frame, the clip's own first, was on screen, and the
// clip dimmed. A clip's start kept to the millisecond can still lie a hair
// before its frame. And a trim of the end on frames leaves the playhead a
// frame before the end, in the clip's last frame, which is where a play
// of the clip stops.
export function placeOf(
  pieces: Piece[],
  key: string,
  at: number,
  fps: number,
  about?: "clip",
  videoStart = 0,
): Playhead {
  if (!key || !pieces.length) return onVideo;
  const start = pieces[0].start;
  const end = pieces[pieces.length - 1].end;
  const rate = fps > 0 ? fps : 30;
  const frame = frameAt(at, 1 / rate, videoStart);
  const first = frameAt(start, 1 / rate, videoStart);
  // The frame that holds the moment a frame before the end, see frameAt.
  const last = Math.max(frameAt(end - 1 / rate, 1 / rate, videoStart), first);
  if (about !== "clip" && at !== end && (frame < first || frame > last)) return onVideo;
  return { place: at >= end || frame >= last ? "end" : "clip", clip: key };
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
// cut. From before the clip's start, inside the frame the clip begins in,
// and from its end, the start: the clip starts over, the way QuickTime
// starts a video over from its end.
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

// A moment the queue reports, kept inside the episode of duration seconds.
// The queue reads the length of the episode from the file's own sample
// tables, and on a real recording the last sample ends a little after the
// end ffmpeg gives, 0.06 seconds on one episode. A playhead that played to
// the end stopped there, past the end of the clip timeline and the range
// picker, and was drawn nowhere, plan row 2.172. A duration not yet known
// keeps the moment as it is.
export function inEpisode(at: number, duration: number): number {
  return duration > 0 ? Math.max(0, Math.min(at, duration)) : at;
}
