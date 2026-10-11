// What the frame queue plays, counted on the frames the engine names:
// the frames of the short, one every den/num seconds from start, and the
// sound in packets of the same length one after another. The interface
// reads nothing of the episode file itself, see docs/VIDEO-PREVIEW.md,
// step 5. Every frame and every sample comes from the episode's decoder
// on the Go side, which makes each frame of the grid from the file's own
// by the rule the render cuts by, so a file ffmpeg reads plays here
// whatever its container, and a file whose frames come at uneven times
// shows the frames its short will have.

import { FRAME_HAIR } from "../flow";

// The frames of the short, as the engine names them, see
// engine.SourceInfo.Grid: count of them, one every den/num seconds from
// start, which is in seconds after the start of the file.
export type Grid = { start: number; num: number; den: number; count: number };

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
};

export type AudioTrack = {
  kind: "audio";
  // The samples a second the sound is played at, and its channels.
  sampleRate: number;
  channels: number;
  samples: Samples;
};

// The frames of the picture, or the packets of the sound, in the order
// they are shown. Times are in the track's own ticks, counted from the
// start of the file, as the engine counts every moment. seconds() gives
// seconds. Every frame of the grid is one a decoder can start at, and is
// shown in the order it comes, so key, order and rank are the same for
// every frame: they are kept so that the plan reads one kind of track.
export type Samples = {
  count: number;
  timescale: number;
  // When the sample is shown, in ticks, and how long it lasts.
  pts: Float64Array;
  duration: Float64Array;
  // Whether decoding can start at this sample.
  key: Uint8Array;
  // The samples in the order they are shown: order[r] is the sample shown
  // r-th, and rank[i] is where sample i comes in that order.
  order: Uint32Array;
  rank: Uint32Array;
};

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
    const at0 = g.start * g.num;
    const pts = new Float64Array(g.count);
    for (let r = 0; r < g.count; r++) pts[r] = at0 + r * g.den;
    movie.video = {
      kind: "video",
      width: s.width,
      height: s.height,
      frame: g.den / g.num,
      samples: even(pts, g.num, g.den),
    };
  }
  const rate = s.soundRate ?? 0;
  if (rate > 0 && (s.soundChannels ?? 0) > 0) {
    const count = Math.max(1, Math.ceil((s.duration * rate) / PACKET));
    const pts = new Float64Array(count);
    for (let j = 0; j < count; j++) pts[j] = j * PACKET;
    movie.audio = { kind: "audio", sampleRate: rate, channels: s.soundChannels!, samples: even(pts, rate, PACKET) };
  }
  return movie;
}

// Samples one after another, each as long as every other, every one a
// key, in the order they are shown.
function even(pts: Float64Array, timescale: number, length: number): Samples {
  const count = pts.length;
  const order = new Uint32Array(count);
  for (let i = 0; i < count; i++) order[i] = i;
  return {
    count,
    timescale,
    pts,
    duration: new Float64Array(count).fill(length),
    key: new Uint8Array(count).fill(1),
    order,
    rank: order,
  };
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
  const here = s.pts[s.order[r]] / s.timescale;
  const next = s.pts[s.order[r + 1]] / s.timescale;
  return Math.abs(next - at) < Math.abs(here - at) ? r + 1 : r;
}

// The rank of the frame that holds a moment: the last one shown at or
// before it, or one that begins up to FRAME_HAIR after it, the way frameAt
// in lib/flow.ts counts. Before the first frame it is the first.
export function rankAt(s: Samples, at: number): number {
  const ticks = (at + FRAME_HAIR) * s.timescale;
  let lo = 0;
  let hi = s.count - 1;
  if (hi < 0) return 0;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (s.pts[s.order[mid]] <= ticks) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}
