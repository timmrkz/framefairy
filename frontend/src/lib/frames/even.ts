// How even a play was, for the log: whether every frame stood on the
// screen for as long as it lasts in the video, as near as the screen's
// refreshes allow. Away from the browser so it can be tested, fed by the
// frame queue on every animation frame of a play, see tick in queue.ts.
//
// A frame that was not decoded in time is counted by the queue as late.
// That count said 0 on nearly every play on Tim's Mac while he still saw
// the play not quite even, issue #251. A play can be uneven with every
// frame decoded in time, in three ways, and this counts each:
//
// - The page missed a refresh: an animation frame came two refreshes or
//   more after the one before, so the frame on screen stood longer.
// - A frame stood a whole refresh longer or shorter than it lasts. On a
//   screen of 60 Hz a frame of 25 a second lasts 2.4 refreshes, so it
//   stands 2 or 3, and that is as even as the screen allows. 1 or 4 is
//   uneven.
// - The clock moved unevenly: the sound heard, worked out on each
//   animation frame, strayed from a straight line through the play. A
//   frame goes up when the clock reaches it, so a clock that steps puts
//   frames up a refresh early or late.

// Where the moment heard on an animation frame came from, see heard in
// plan.ts: the output timestamp, the output timestamp held back to the
// sound card's own time, or that time less its latency.
export type Source = "stamp" | "held" | "plain";

// Where an animation frame's time went, milliseconds on the page's clock:
// how late its callback began after the frame's own time, then putting
// the frame up, which is left out on an animation frame that puts none
// up, telling the app, feeding the decoders and the sound, and all of the
// callback. after is from the callback's end to the first
// moment the page had nothing else to do, which is the page drawing what
// the callback changed and whatever else waited, see drawn.
export type Work = { late: number; put?: number; tell: number; feed: number; all: number; after?: number };

export const PARTS = ["late", "put", "tell", "feed", "all", "after"] as const;
export type Part = (typeof PARTS)[number];

// A part of the work, typical and most over every animation frame, and on
// average over the animation frames just before a missed refresh, where
// late is the late one's own.
export type Spent = Record<Part, { typical: number; most: number; beforeMissed: number }>;

export type Evenness = {
  // The screen's refresh, milliseconds, from the time between two
  // animation frames, see refreshOf.
  refresh: number;
  ticks: number;
  // Animation frames that came a refresh or more late, the refreshes they
  // missed together, and the longest time between two, milliseconds.
  missed: number;
  missedRefreshes: number;
  longest: number;
  // How many frames stood for how many refreshes, and how many of them
  // stood a whole refresh longer or shorter than they last.
  held: Map<number, number>;
  frames: number;
  uneven: number;
  // The clock's stray from a straight line through the play, the typical
  // and the most, milliseconds, and how fast it ran against the page's
  // clock, 1 being the same.
  stray: number;
  mostStray: number;
  rate: number;
  sources: Record<Source, number>;
  spent: Spent;
};

export class Even {
  private ticks: { at: number; pos: number }[] = [];
  // The times between animation frames that came straight after the one
  // before, without a timer's tick between, see behind, each with the
  // place of the later one among the ticks.
  private gaps: { gap: number; i: number }[] = [];
  private puts: { at: number; due: number }[] = [];
  private sources: Record<Source, number> = { stamp: 0, held: 0, plain: 0 };
  private broken = true;
  // The latest times between animation frames, for the refresh now.
  private recent: number[] = [];
  // The work of each animation frame, in step with ticks.
  private works: (Work | null)[] = [];

  // An animation frame of the play: when it came on the page's clock,
  // milliseconds, the place on the program it showed, seconds, and where
  // that place came from.
  tick(at: number, pos: number, source: Source) {
    const last = this.ticks[this.ticks.length - 1];
    if (last && !this.broken) {
      this.gaps.push({ gap: at - last.at, i: this.ticks.length });
      this.recent.push(at - last.at);
      if (this.recent.length > RECENT) this.recent.shift();
    }
    this.ticks.push({ at, pos });
    this.works.push(null);
    this.sources[source]++;
    this.broken = false;
  }

  // Where the time of the animation frame just ticked went. Gives its
  // place, for drawn.
  work(w: Work): number {
    const i = this.works.length - 1;
    if (i >= 0) this.works[i] = { ...w };
    return i;
  }

  // The page had nothing else to do at, after the animation frame at place
  // i, whose callback ended at end.
  drawn(i: number, at: number, end: number) {
    const w = this.works[i];
    if (w) w.after = at - end;
  }

  // A frame put up on the animation frame at, which lasts from due on the
  // program, seconds.
  put(at: number, due: number) {
    this.puts.push({ at, due });
  }

  // A tick that was not an animation frame: the app was behind another
  // window and a timer ticked instead. Nothing measured across it counts,
  // since nothing could be seen.
  behind() {
    this.broken = true;
    this.puts.push({ at: NaN, due: NaN });
  }

  // The screen's refresh as the play goes, milliseconds, from the latest
  // animation frames, and a refresh at 60 Hz until there are a few. A
  // frame drawn on an animation frame is on screen a refresh later, so
  // the frame queue picks the frame for that moment, see heardOn in
  // queue.ts. It took a refresh at 60 Hz for granted, and at 120 Hz put
  // each frame up 8 ms ahead of its sound.
  get refreshNow(): number {
    return this.recent.length >= 4 ? refreshOf(this.recent) : 1000 / 60;
  }

  get evenness(): Evenness {
    const refresh = refreshOf(this.gaps.map((g) => g.gap));
    let missed = 0;
    let missedRefreshes = 0;
    let longest = 0;
    // The animation frames on either side of each missed refresh, by
    // their place among the ticks.
    const before: number[] = [];
    const lateOnes: number[] = [];
    for (const { gap, i } of this.gaps) {
      longest = Math.max(longest, gap);
      const over = Math.round(gap / refresh) - 1;
      if (refresh > 0 && over >= 1) {
        missed++;
        missedRefreshes += over;
        before.push(i - 1);
        lateOnes.push(i);
      }
    }
    const held = new Map<number, number>();
    let frames = 0;
    let uneven = 0;
    for (let i = 0; i + 1 < this.puts.length; i++) {
      const a = this.puts[i];
      const b = this.puts[i + 1];
      if (!(refresh > 0) || Number.isNaN(a.at) || Number.isNaN(b.at)) continue;
      const stood = Math.round((b.at - a.at) / refresh);
      const lasts = ((b.due - a.due) * 1000) / refresh;
      held.set(stood, (held.get(stood) ?? 0) + 1);
      frames++;
      if (Math.abs(stood - lasts) >= 1 - 1e-6) uneven++;
    }
    const line = straight(this.ticks);
    return {
      refresh,
      ticks: this.ticks.length,
      missed,
      missedRefreshes,
      longest,
      held,
      frames,
      uneven,
      stray: line.stray,
      mostStray: line.most,
      rate: line.rate,
      sources: { ...this.sources },
      spent: spentOf(this.works, before, lateOnes),
    };
  }
}

function spentOf(works: (Work | null)[], before: number[], lateOnes: number[]): Spent {
  const out = {} as Spent;
  for (const part of PARTS) {
    const all = works.flatMap((w) => (w && w[part] !== undefined ? [w[part]!] : []));
    const near = (part === "late" ? lateOnes : before).flatMap((i) => {
      const w = works[i];
      return w && w[part] !== undefined ? [w[part]!] : [];
    });
    out[part] = {
      typical: median(all),
      most: all.length ? Math.max(...all) : 0,
      beforeMissed: near.length ? near.reduce((a, b) => a + b, 0) / near.length : 0,
    };
  }
  return out;
}

// How many of the latest times between animation frames the refresh now
// is taken from, about half a second at 60 Hz.
const RECENT = 32;

// How an evenness reads in the log, in one line.
export function describeEvenness(e: Evenness): string {
  if (!e.ticks) return "no animation frame came";
  const hz = e.refresh > 0 ? Math.round(1000 / e.refresh) : 0;
  const held = [...e.held.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([n, count]) => `${count} for ${n}`)
    .join(", ");
  const s = e.sources;
  return [
    `the screen at ${hz} Hz, ${e.refresh.toFixed(1)} ms a refresh`,
    `${e.missed} animation frames late by ${e.missedRefreshes} refreshes in all, the longest gap ${e.longest.toFixed(1)} ms`,
    `${e.frames} frames stood for so many refreshes: ${held || "none"}`,
    `${e.uneven} frames a refresh longer or shorter than they last`,
    `the clock off a straight line by ${e.stray.toFixed(1)} ms typically and ${e.mostStray.toFixed(1)} ms at most, running at ${e.rate.toFixed(4)} of the page's clock`,
    `the moment heard from the output timestamp ${s.stamp} times, held to the sound card's time ${s.held}, the sound card's time less its latency ${s.plain}`,
    `an animation frame's work in ms, typical, most and before a missed refresh: ${describeSpent(e.spent)}`,
  ].join(", ");
}

// The screen's refresh, from the times between animation frames: the mean
// of those near the most common one, so a refresh missed does not count.
// The mean and not the most common time itself, because WebKit gives the
// page its times to the whole millisecond: at 120 Hz they come 8 and 9 ms
// apart, and the most common, 8, made a screen of 125 Hz, on which every
// frame of 25 a second that stood its 4 refreshes of the true 8.33 ms
// counted as uneven. Tim's second report said 92 of 446.
function refreshOf(gaps: number[]): number {
  const typical = median(gaps);
  const near = gaps.filter((g) => g > typical * 0.5 && g < typical * 1.5);
  return near.length ? near.reduce((a, b) => a + b, 0) / near.length : typical;
}

const PART_NAMES: Record<Part, string> = {
  late: "begun late",
  put: "putting the frame up",
  tell: "telling the app",
  feed: "feeding the decoders",
  all: "all of it",
  after: "the page after it",
};

function describeSpent(spent: Spent): string {
  return PARTS.map((p) => {
    const x = spent[p];
    return `${PART_NAMES[p]} ${x.typical.toFixed(1)}/${x.most.toFixed(1)}/${x.beforeMissed.toFixed(1)}`;
  }).join(", ");
}

function median(xs: number[]): number {
  if (!xs.length) return 0;
  const s = [...xs].sort((a, b) => a - b);
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}

// The straight line through the places the clock gave, by least squares,
// and how far the places strayed from it: the root of the mean square and
// the most, milliseconds. Its slope is how fast the clock ran against the
// page's.
function straight(ticks: { at: number; pos: number }[]): { stray: number; most: number; rate: number } {
  const n = ticks.length;
  if (n < 3) return { stray: 0, most: 0, rate: 1 };
  const t0 = ticks[0].at;
  const p0 = ticks[0].pos;
  let st = 0;
  let sp = 0;
  for (const k of ticks) {
    st += (k.at - t0) / 1000;
    sp += k.pos - p0;
  }
  const mt = st / n;
  const mp = sp / n;
  let stt = 0;
  let stp = 0;
  for (const k of ticks) {
    const t = (k.at - t0) / 1000 - mt;
    stt += t * t;
    stp += t * (k.pos - p0 - mp);
  }
  const rate = stt > 0 ? stp / stt : 1;
  let sum = 0;
  let most = 0;
  for (const k of ticks) {
    const off = (k.pos - p0 - mp - rate * ((k.at - t0) / 1000 - mt)) * 1000;
    sum += off * off;
    most = Math.max(most, Math.abs(off));
  }
  return { stray: Math.sqrt(sum / n), most, rate };
}
