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
};

export class Even {
  private ticks: { at: number; pos: number }[] = [];
  // The animation frames that came straight after the one before, without
  // a timer's tick between, see behind.
  private gaps: number[] = [];
  private puts: { at: number; due: number }[] = [];
  private sources: Record<Source, number> = { stamp: 0, held: 0, plain: 0 };
  private broken = true;

  // An animation frame of the play: when it came on the page's clock,
  // milliseconds, the place on the program it showed, seconds, and where
  // that place came from.
  tick(at: number, pos: number, source: Source) {
    const last = this.ticks[this.ticks.length - 1];
    if (last && !this.broken) this.gaps.push(at - last.at);
    this.ticks.push({ at, pos });
    this.sources[source]++;
    this.broken = false;
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

  get evenness(): Evenness {
    const refresh = refreshOf(this.gaps);
    let missed = 0;
    let missedRefreshes = 0;
    let longest = 0;
    for (const g of this.gaps) {
      longest = Math.max(longest, g);
      const over = Math.round(g / refresh) - 1;
      if (refresh > 0 && over >= 1) {
        missed++;
        missedRefreshes += over;
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
    };
  }
}

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
