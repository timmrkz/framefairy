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
// change that. See Playback in docs/INTERFACE.md.

import { wordStep, type LitWord } from "./api";
import type { Piece } from "./flow";

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

// What a gesture says it is about, when it says, see placeOf.
export type About = "clip" | "video";

// Where a gesture that puts the playhead at a moment leaves it, for the
// clip chosen, by its key, its pieces and its places, see ClipEntry.places.
//
// A gesture about the clip says so, about: picking it, a click on one of
// its edges, a trim, a click on a caption. That decides an edge, which is
// the clip's own, and a trim, whose playhead stands on the edge being
// dragged and so can be a hair outside the pieces the video preview has
// at that moment. A step taken while the episode plays on the video says
// it is about the video, about "video": it skips ahead or back through the
// episode and stays there, through the clip and its cuts, the way a play
// on the video does. Found by Tim, who sped through a dimmed clip with
// shift and the right arrow and had the clip light up under him. So does
// a step taken on the video while paused, Tim on #938: moving along the
// clip timeline, a clip passed over is not active. Every
// other gesture, a click, a drag, a step or a seek, is decided by the
// frame it lands in: from the frame that holds the
// clip's start to the frame that holds the moment a frame before its end,
// cuts included, is on the clip, and any other frame is on the video.
// Either way, that last frame is the clip's end, and so is the end itself.
// Which frame holds a moment is the engine's to say, and it says where
// those frames begin to hold, so these are the frames the short starts
// and ends with.
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
  clip: { segments: Piece[]; places?: [number, number, number] } | null | undefined,
  key: string,
  at: number,
  about?: About,
): Playhead {
  if (!key || !clip?.places || !clip.segments.length || about === "video") return onVideo;
  const [on, last, off] = clip.places;
  const end = clip.segments[clip.segments.length - 1].end;
  if (about !== "clip" && at !== end && (at < on || at >= off)) return onVideo;
  return { place: at >= end || at >= last ? "end" : "clip", clip: key };
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

// The playhead's time, for the field beside the buttons under the clip
// timeline: where the frame it stands in starts, to the millisecond. A
// frame and not the moment itself, because the playhead is never quite
// where it was put, and the frame is what the video preview shows and
// what a thumbnail, a keyframe or an edge is put on. So the field steps a
// frame at a time, finer on a video with more frames a second. begins is
// where that frame begins, see FrameTable.startOf. long says the hours,
// for a video of an hour or more, so the field is as wide at the start of
// the video as at its end.
export function playheadClock(begins: number, long: boolean): string {
  const ms = Math.max(0, Math.round(begins * 1000));
  const h = Math.floor(ms / 3_600_000);
  const m = Math.floor(ms / 60_000) % 60;
  const s = Math.floor(ms / 1000) % 60;
  const two = (n: number) => String(n).padStart(2, "0");
  const rest = `${two(s)}.${String(ms % 1000).padStart(3, "0")}`;
  return long ? `${h}:${two(m)}:${rest}` : `${two(m + h * 60)}:${rest}`;
}

// How far Shift and an arrow skip outside a clip, in seconds, the jump
// Tim chose for #377. YouTube's arrows jump 5 seconds too.
export const SKIP = 5;

// Where an arrow key puts the playhead on the clip timeline, the way every
// video tool steps: a frame at a time, and with Shift by words on a clip,
// which is the thing the picture shows, and SKIP seconds anywhere else,
// transcribed or not. "clip" is out of the clip's words without loop,
// where the clip beside it takes over. The component puts the place in
// the episode.
export type ArrowKey = {
  time: number;
  frame: number;
  back: boolean;
  shift: boolean;
  // On the video, where the clip is not active: Shift skips, over the
  // clip and its cuts.
  through: boolean;
  // The whole clip, from its first piece's start to its last piece's end.
  clip: Piece | null;
  // The words the caption lights, the only words a step lands on: a word
  // a cut takes out is not in the short, so there is nothing to step to.
  lit: LitWord[];
  looping: boolean;
};

export function arrowStep(k: ArrowKey): { to: number; word: boolean } | "clip" {
  const sign = k.back ? -1 : 1;
  // A step is never shorter than a frame at 240 a second, so a video that
  // says no rate still moves.
  const step = Math.max(k.frame, 1 / 240);
  if (!k.shift) return { to: k.time + sign * step, word: false };
  // On the video, or with no clip, Shift skips. On the clip it goes by
  // its words, which the place says, not a moment: the place is where
  // the gesture put the playhead, as the engine counts its frames.
  if (k.through || !k.clip) return { to: k.time + sign * SKIP, word: false };
  // Until the captions come there is no word to go to, so a step is a
  // second.
  if (!k.lit.length) return { to: k.time + sign, word: false };
  // From a cut too, the next word is the next one the short says.
  const to = wordStep(k.lit, k.time, k.back);
  if (to !== null) return { to, word: true };
  // Out of words. With loop on, the clip comes round the way a play of it
  // does: past its last word is its first word, and before its first its
  // last. Without, at its ends is the clip beside it, because the words
  // go on even where this clip does not.
  if (k.looping) {
    const places = k.lit.filter((w) => w.land >= 0);
    const word = k.back ? places[places.length - 1] : places[0];
    if (word) return { to: word.land, word: true };
  }
  return "clip";
}
