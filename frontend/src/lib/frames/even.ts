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
//   frames up a refresh early or late. The line starts once the sound
//   does: before it, the play stands on its first frame on purpose, see
//   startWait.

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
  // How long the play stood on its first frame while the sound card
  // reached the moment the sound starts, milliseconds, see START_LEAD in
  // queue.ts. It read as the clock off its line by 23 to 47 ms once every
  // play, on Tim's Mac and in the cloud, until it was said apart.
  startWait: number;
  sources: Record<Source, number>;
  spent: Spent;
};

export class Even {
  // Everything is counted as the play goes, in sums and in samples of a
  // bounded size, so the line costs the same and the memory stays the
  // same however long the play. Kept per animation frame, the line of a
  // play of 10 minutes took 90 ms to work out at the pause, and one of 9
  // minutes at 120 Hz threw, see the review of #430.
  private ticks = 0;
  private last: { at: number; pos: number } | null = null;
  private broken = true;
  private sources: Record<Source, number> = { stamp: 0, held: 0, plain: 0 };
  // The latest times between animation frames, for the refresh now, and
  // a longer sample of them for the refresh of the play.
  private recent: number[] = [];
  private refresh = 1000 / 60;
  private gaps = new Sample(SAMPLE);
  private missed = 0;
  private missedRefreshes = 0;
  private longest = 0;
  // The frame put up last, gone after a tick that was not an animation
  // frame.
  private lastPut: { at: number; due: number } | null = null;
  private held = new Map<number, number>();
  private frames = 0;
  private uneven = 0;
  // The clock: where it stood first, and from the moment it moved a
  // straight line through it, see Line.
  private first: { at: number; pos: number } | null = null;
  private line: Line | null = null;
  // How many animation frames the clock stood at its first place.
  private stood = 0;
  // The work of the animation frame ticked last, whether it came just
  // before a missed refresh, and whether the one ticked now came late
  // after one.
  private work_: { w: Work; i: number; beforeMissed: boolean } | null = null;
  private lateNow = false;
  private parts = Object.fromEntries(PARTS.map((p) => [p, { sample: new Sample(SAMPLE), most: 0, near: 0, nearCount: 0 }])) as Record<
    Part,
    { sample: Sample; most: number; near: number; nearCount: number }
  >;

  // An animation frame of the play: when it came on the page's clock,
  // milliseconds, the place on the program it showed, seconds, and where
  // that place came from.
  tick(at: number, pos: number, source: Source) {
    this.lateNow = false;
    if (this.last && !this.broken) {
      const gap = at - this.last.at;
      // Judged by the refresh just before it, so a gap is a missed
      // refresh whatever the screen's rate was earlier in the play.
      const over = Math.round(gap / this.refreshNow) - 1;
      if (over >= 1) {
        this.missed++;
        this.missedRefreshes += over;
        this.lateNow = true;
        const before = this.work_;
        if (before && before.i === this.ticks - 1) {
          before.beforeMissed = true;
          for (const p of PARTS) if (p !== "late" && before.w[p] !== undefined) this.near(p, before.w[p]!);
        }
      }
      this.longest = Math.max(this.longest, gap);
      this.gaps.push(gap);
      this.recent.push(gap);
      if (this.recent.length > RECENT) this.recent.shift();
      this.refresh = this.recent.length >= 4 ? refreshOf(this.recent) : 1000 / 60;
    }
    this.first ??= { at, pos };
    if (!this.line && pos > this.first.pos) {
      this.line = new Line(at, pos);
      this.stood = this.ticks;
    }
    this.line?.add(at, pos);
    this.last = { at, pos };
    this.ticks++;
    this.sources[source]++;
    this.broken = false;
  }

  // Where the time of the animation frame just ticked went. Gives its
  // place, for drawn.
  work(w: Work): number {
    const i = this.ticks - 1;
    this.work_ = { w: { ...w }, i, beforeMissed: false };
    for (const p of PARTS) if (w[p] !== undefined) this.count(p, w[p]!);
    if (this.lateNow) this.near("late", w.late);
    return i;
  }

  // The page had nothing else to do at, after the animation frame at place
  // i, whose callback ended at end.
  drawn(i: number, at: number, end: number) {
    const k = this.work_;
    if (!k || k.i !== i || k.w.after !== undefined) return;
    k.w.after = at - end;
    this.count("after", k.w.after);
    if (k.beforeMissed) this.near("after", k.w.after);
  }

  // A frame put up on the animation frame at, which lasts from due on the
  // program, seconds.
  put(at: number, due: number) {
    const a = this.lastPut;
    this.lastPut = { at, due };
    if (!a) return;
    const refresh = this.refreshNow;
    const stood = Math.round((at - a.at) / refresh);
    const lasts = ((due - a.due) * 1000) / refresh;
    this.held.set(stood, (this.held.get(stood) ?? 0) + 1);
    this.frames++;
    if (Math.abs(stood - lasts) >= 1 - 1e-6) this.uneven++;
  }

  // A tick that was not an animation frame: the app was behind another
  // window and a timer ticked instead. Nothing measured across it counts,
  // since nothing could be seen.
  behind() {
    this.broken = true;
    this.lastPut = null;
  }

  private count(p: Part, x: number) {
    const part = this.parts[p];
    part.sample.push(x);
    part.most = Math.max(part.most, x);
  }

  private near(p: Part, x: number) {
    this.parts[p].near += x;
    this.parts[p].nearCount++;
  }

  // The screen's refresh as the play goes, milliseconds, from the latest
  // animation frames, and a refresh at 60 Hz until there are a few. A
  // frame drawn on an animation frame is on screen a refresh later, so
  // the frame queue picks the frame for that moment, see heardOn in
  // queue.ts. It took a refresh at 60 Hz for granted, and at 120 Hz put
  // each frame up 8 ms ahead of its sound.
  get refreshNow(): number {
    return this.refresh;
  }

  get evenness(): Evenness {
    const line = this.line;
    const first = this.first;
    const spent = {} as Spent;
    for (const p of PARTS) {
      const x = this.parts[p];
      spent[p] = { typical: median(x.sample.values()), most: x.most, beforeMissed: x.nearCount ? x.near / x.nearCount : 0 };
    }
    return {
      refresh: refreshOf(this.gaps.values()),
      ticks: this.ticks,
      missed: this.missed,
      missedRefreshes: this.missedRefreshes,
      longest: this.longest,
      held: new Map(this.held),
      frames: this.frames,
      uneven: this.uneven,
      stray: line?.stray ?? 0,
      mostStray: line?.most ?? 0,
      rate: line?.rate ?? 1,
      startWait: line && first && this.stood > 1 ? Math.max(0, line.reaches(first.pos) - first.at) : 0,
      sources: { ...this.sources },
      spent,
    };
  }
}

// How many of the latest values a sample keeps, about half a minute of
// animation frames at 120 Hz: enough for what is typical, and the same
// for every play.
const SAMPLE = 4096;

// The latest values of something counted, at most size of them.
class Sample {
  private xs: Float64Array;
  private n = 0;
  constructor(size: number) {
    this.xs = new Float64Array(size);
  }
  push(x: number) {
    this.xs[this.n % this.xs.length] = x;
    this.n++;
  }
  values(): number[] {
    return Array.from(this.xs.subarray(0, Math.min(this.n, this.xs.length)));
  }
}

// A straight line through the places the clock gave, by least squares,
// kept as running means and sums of squares, Welford's way, so it stays
// exact over an hour of animation frames. Its slope is how fast the clock
// ran against the page's, and how far the places strayed from it is the
// root of the mean square for the whole play, and the most any place
// strayed from the line as it stood when that place came, once the line
// had a second to settle.
class Line {
  n = 0;
  most = 0;
  private mt = 0;
  private mp = 0;
  private ctt = 0;
  private ctp = 0;
  private cpp = 0;
  // Milliseconds and seconds from the first place on the line.
  constructor(
    private at0: number,
    private pos0: number,
  ) {}

  add(at: number, pos: number) {
    const t = (at - this.at0) / 1000;
    const p = pos - this.pos0;
    if (this.n > 2 && t > 1 && this.ctt > 0) {
      const off = Math.abs(p - this.mp - this.rate * (t - this.mt)) * 1000;
      this.most = Math.max(this.most, off);
    }
    this.n++;
    const dt = t - this.mt;
    const dp = p - this.mp;
    this.mt += dt / this.n;
    this.mp += dp / this.n;
    this.ctt += dt * (t - this.mt);
    this.ctp += dt * (p - this.mp);
    this.cpp += dp * (p - this.mp);
  }

  get rate(): number {
    return this.ctt > 0 ? this.ctp / this.ctt : 1;
  }

  get stray(): number {
    if (this.n < 3 || !(this.ctt > 0)) return 0;
    return Math.sqrt(Math.max(0, this.cpp - (this.ctp * this.ctp) / this.ctt) / this.n) * 1000;
  }

  // When on the page's clock the line stood at a place.
  reaches(pos: number): number {
    const r = this.rate;
    return r > 0 ? this.at0 + (this.mt + (pos - this.pos0 - this.mp) / r) * 1000 : this.at0;
  }
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
    `the picture waited ${e.startWait.toFixed(1)} ms for the sound to start`,
    `then the clock off a straight line by ${e.stray.toFixed(1)} ms typically and ${e.mostStray.toFixed(1)} ms at most, running at ${e.rate.toFixed(4)} of the page's clock`,
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
