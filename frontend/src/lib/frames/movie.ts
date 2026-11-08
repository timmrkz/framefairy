// What the frame queue plays, counted on the frames the engine names:
// the frames of the short, one every den/num seconds from start, and the
// sound in packets of the same length one after another. The interface
// reads nothing of the episode file itself, see docs/VIDEO-PREVIEW.md,
// step 5. Every frame and every sample comes from the episode's decoder
// on the Go side, which makes each frame of the grid from the file's own
// by the rule the render cuts by, so a file ffmpeg reads plays here
// whatever its container, and a file whose frames come at uneven times
// shows the frames its short will have.

import { heldMicros } from "../flow";

// The frames of the short, as the engine names them, see
// engine.SourceInfo.Grid: count of them, one every den/num seconds from
// start, which is in seconds after the start of the file. origin is where
// that start is on the file's own clock. The page hands the grid back
// with every stream it opens, so the Go side makes the frames the page
// counts on, see Stream in app.ts.
export type Grid = { origin: number; start: number; num: number; den: number; count: number };

// What a video preview needs to know of an episode: how long it is, the
// size of its picture standing up, its grid, and its sound, nothing where
// it has none. The app's SourceView carries all of it.
export type Source = {
  duration: number;
  width: number;
  height: number;
  grid?: Grid;
  soundRate?: number;
  soundChannels?: number;
};

export type VideoTrack = {
  kind: "video";
  width: number;
  height: number;
  samples: Samples;
  // How long a frame lasts, in seconds.
  frame: number;
  // The grid the frames are, nothing for a track that is not.
  grid?: Grid;
};

export type AudioTrack = {
  kind: "audio";
  // The samples a second the sound is played at, and its channels.
  sampleRate: number;
  channels: number;
  samples: Samples;
};

// The frames of the picture, or the packets of the sound, in the order
// they are shown, one after another on a grid: sample i starts step ticks
// after sample i - 1, the first at start, and each lasts step. Times are in
// the track's own ticks, counted from the start of the file, as the engine
// counts every moment. ticks() gives where a sample starts, seconds() turns
// ticks into seconds. Every frame of the grid is one the Go side makes and
// shows in the order it comes, so a sample is its own key frame and its own
// place in the order, and nothing about them is kept: the arrays a
// WebCodecs decoder needed for that took about 25 MB for three hours at 60
// frames a second.
export type Samples = {
  count: number;
  timescale: number;
  start: number;
  step: number;
};

// Where sample i starts, in ticks.
export function ticks(s: Samples, i: number): number {
  return s.start + i * s.step;
}

export type Movie = {
  // Seconds, as the engine's probe says.
  duration: number;
  video?: VideoTrack;
  audio?: AudioTrack;
};

// How many moments of sound a packet holds, as the Go side hands sound
// over, see SOUND_CHUNK in queue.ts.
export const PACKET = 1024;

// The movie of an episode from what the engine says of it. A source with
// no grid has no picture the video preview can show.
export function movieOf(s: Source): Movie {
  const movie: Movie = { duration: s.duration };
  const g = s.grid;
  if (g && g.num > 0 && g.den > 0 && g.count > 0) {
    // Ticks of 1/num seconds, so every frame is a whole number of them
    // after the start.
    movie.video = {
      kind: "video",
      width: s.width,
      height: s.height,
      frame: g.den / g.num,
      grid: g,
      samples: { count: g.count, timescale: g.num, start: g.start * g.num, step: g.den },
    };
  }
  const rate = s.soundRate ?? 0;
  if (rate > 0 && (s.soundChannels ?? 0) > 0) {
    const count = Math.max(1, Math.ceil((s.duration * rate) / PACKET));
    movie.audio = {
      kind: "audio",
      sampleRate: rate,
      channels: s.soundChannels!,
      samples: { count, timescale: rate, start: 0, step: PACKET },
    };
  }
  return movie;
}

// Seconds from ticks.
export function seconds(s: Samples, ticks: number): number {
  return ticks / s.timescale;
}

// The frame whose own start is nearest a moment: which frame the Go side
// handed over, by the moment it says the frame starts at.
export function rankOf(s: Samples, at: number): number {
  const r = rankAt(s, at);
  if (r + 1 >= s.count) return r;
  const here = ticks(s, r) / s.timescale;
  const next = ticks(s, r + 1) / s.timescale;
  return Math.abs(next - at) < Math.abs(here - at) ? r + 1 : r;
}

// The rank of the frame that holds a moment: the last one shown at or
// before it, or one that begins up to FRAME_HAIR after it, counted to the
// microsecond the way frameAt in lib/flow.ts counts. The moment's ticks
// are whole, worked out from its whole seconds and the rest apart, so no
// product comes near 2^53 and a frame's ticks compare with them exactly.
// Before the first frame it is the first.
export function rankAt(s: Samples, at: number): number {
  const us = heldMicros(at);
  const seconds = Math.floor(us / 1_000_000);
  const held = seconds * s.timescale + Math.floor(((us - seconds * 1_000_000) * s.timescale) / 1_000_000);
  let lo = 0;
  let hi = s.count - 1;
  if (hi < 0) return 0;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (ticks(s, mid) <= held) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}
