// How much of an episode one search can read, turned into seconds.
//
// A search sends its window to the model in one request, and one request
// holds only so much: a number of characters of transcript, worked out by
// the engine from the model that is going to read it (see engine/room.go).
// The engine also weighs every line the transcript has so far. Here those
// weights are added up over a window, so the range picker knows while it
// is dragged how far a window may reach, without asking the Go side at
// every step of a hand. The engine checks the same sum again before it
// sends anything.

export interface LineWeight {
  start: number;
  end: number;
  chars: number;
}

export interface RoomView {
  // The most characters of transcript one request can carry.
  chars: number;
  // What sets it: "context" for what the model holds at once, "memory" for
  // what this machine can hold it with, "budget" for what one search may
  // cost.
  by: string;
  lines: LineWeight[];
  // How far the transcript reaches, silence at its end included.
  heard: number;
  // Characters a second, for the part of the episode not heard yet.
  rate: number;
}

// Bisection runs this many times. Forty halvings of a four hour episode
// are far below a millisecond.
const halvings = 40;

export class Reach {
  private readonly lines: LineWeight[];
  // sums[i] is the weight of the lines before line i.
  private readonly sums: number[];
  // Where the transcript ends. Past it, a second weighs the rate.
  private readonly heard: number;

  constructor(
    readonly room: RoomView | null,
    readonly duration: number,
  ) {
    this.lines = room?.lines ?? [];
    this.sums = [0];
    for (const line of this.lines) this.sums.push(this.sums[this.sums.length - 1] + line.chars);
    const last = this.lines.length ? this.lines[this.lines.length - 1].end : 0;
    this.heard = Math.max(room?.heard ?? 0, last);
  }

  // Whether there is a room at all. Until the engine has answered there is
  // nothing to hold a window to.
  get known(): boolean {
    return !!this.room && this.room.chars > 0;
  }

  // What a window from..to sends. A line the window only touches is
  // counted whole, which only ever errs towards a window that fits.
  weight(from: number, to: number): number {
    if (to <= from) return 0;
    // The first line that ends after from, and the first that starts at
    // or after to.
    const first = this.search((i) => this.lines[i].end > from);
    const past = this.search((i) => this.lines[i].start >= to);
    let chars = past > first ? this.sums[past] - this.sums[first] : 0;
    const unheard = to - Math.max(from, this.heard);
    if (unheard > 0) chars += unheard * (this.room?.rate ?? 0);
    return chars;
  }

  fits(from: number, to: number): boolean {
    return !this.known || this.weight(from, to) <= this.room!.chars;
  }

  // The latest end a window starting at from may have.
  longestFrom(from: number): number {
    if (this.fits(from, this.duration)) return this.duration;
    let lo = from;
    let hi = this.duration;
    for (let i = 0; i < halvings; i++) {
      const mid = (lo + hi) / 2;
      if (this.fits(from, mid)) lo = mid;
      else hi = mid;
    }
    // On a whole second, so the window can be said out loud.
    const whole = Math.floor(lo + 1e-6);
    return whole > from && this.fits(from, whole) ? whole : lo;
  }

  // The earliest start a window ending at to may have.
  earliestTo(to: number): number {
    if (this.fits(0, to)) return 0;
    let lo = 0;
    let hi = to;
    for (let i = 0; i < halvings; i++) {
      const mid = (lo + hi) / 2;
      if (this.fits(mid, to)) hi = mid;
      else lo = mid;
    }
    const whole = Math.ceil(hi - 1e-6);
    return whole < to && this.fits(whole, to) ? whole : hi;
  }

  // The longest window that fits wherever it is drawn: the shortest of the
  // longest windows from every start. It is what the clip settings are held
  // to, so the clips they ask for fit in a window drawn anywhere.
  //
  // The densest stretch decides it, and a window is densest when it starts
  // just before a line ends: the line is counted whole for a moment of it.
  // Past the transcript every second weighs the same, so one start there
  // says it all. A window cut short by the end of the
  // episode does not count, because the end is not the model's limit.
  anywhere(): number {
    if (!this.known || this.fits(0, this.duration)) return this.duration;
    let shortest = this.duration;
    const starts = [0, this.heard, ...this.lines.map((l) => Math.max(l.start, l.end - 0.001))];
    for (const start of starts) {
      if (start >= this.duration) continue;
      const end = this.longestFrom(start);
      if (end >= this.duration) continue;
      shortest = Math.min(shortest, end - start);
    }
    return shortest;
  }

  // The first index where test turns true, over lines in order, or the
  // number of lines when it never does.
  private search(test: (i: number) => boolean): number {
    let lo = 0;
    let hi = this.lines.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (test(mid)) hi = mid;
      else lo = mid + 1;
    }
    return lo;
  }
}

// The shortest a window may be: room for the clips asked for, one after
// another at their shortest, and never longer than the episode. The engine
// refuses less, see Holds in engine/room.go.
export function leastWindow(count: number, shortest: number, duration: number, floor = 10): number {
  return Math.min(Math.max(count * shortest, floor), duration);
}

// The most clips a search may ask for: as many as fit at their shortest in
// the longest window that can be drawn anywhere, and no more than cap.
export function mostClips(reach: number, shortest: number, cap: number): number {
  return Math.max(1, Math.min(cap, Math.floor((reach + 0.01) / Math.max(shortest, 1))));
}

// The longest the shortest clip may be: the clips asked for, at that length,
// still fit in that window. Never more than cap, and never less than floor,
// the least any clip may be.
export function longestShortest(reach: number, count: number, floor: number, cap: number): number {
  return Math.max(floor, Math.min(cap, Math.floor((reach + 0.01) / Math.max(count, 1))));
}

// A window put back inside what it may be: no longer than the model reads
// from where it starts, no shorter than its clips need. It keeps its start
// when it can, and gives up the end first, because the start is where the
// window was put.
export function fitWindow(
  win: { from: number; to: number },
  reach: Reach,
  least: number,
): { from: number; to: number } {
  const duration = reach.duration;
  let from = Math.min(Math.max(win.from, 0), duration);
  let to = Math.min(Math.max(win.to, from), duration);
  to = Math.min(to, reach.longestFrom(from));
  if (to - from < least) {
    to = Math.min(duration, from + least);
    from = Math.max(0, to - least);
  }
  return { from, to };
}
