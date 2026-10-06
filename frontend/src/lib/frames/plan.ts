// What the frame queue decides, away from the browser so it can be
// tested: which samples are fed to the decoders for a list of pieces,
// which decoded frames are drawn and which are closed unseen, where the
// sound is cut, and what is read from the file. The browser side, the
// decoders, the canvas and the sound card, is queue.ts, and it does only
// what this file says.
//
// The words used here:
//
// - The program is what play plays: the pieces one after the other, from
//   the first again if it loops. A position on the program is in seconds
//   from the start of its first piece, and it goes on growing through a
//   loop, so it never goes back while playing.
// - A visit is one piece played once. With a loop the second time round is
//   another visit of the same piece.
// - The grid is the moments of the program a frame is drawn at: where each
//   frame of the episode begins, carried onto the program through the
//   pieces, so every frame is drawn when the sound heard reaches its own
//   start. A visit's first point is where the visit begins on the program,
//   or where the play began, with the frame that holds that moment, the
//   piece-start frame. Its last is the frame that holds the moment just
//   before the piece ends, the piece-end frame. So the last frame before a
//   cut is followed straight by the first frame after it, both drawn,
//   whatever moment the play began at. A grid that began where the play
//   began drew each frame up to a frame late, and a play begun inside a
//   frame never drew the last frame of a piece at all.
// - A run is what one decoder is fed in one go, in decode order, from a
//   key frame on. A piece that starts after the last one ends, inside what
//   has already been decoded, carries the run on. Any other piece starts a
//   new run, from the key frame before it.
import { rankAt, type Samples } from "./mp4";

export type Piece = { start: number; end: number };

// The fade at each edge of a piece, the same as the render's, Fade in
// engine/render.go, so a cut sounds in the video preview as it does in the
// short. It is also what keeps a start or a stop from clicking.
export const FADE = 0.015;

export type Visit = {
  v: number;
  piece: number;
  // Where it starts on the program.
  from: number;
  start: number;
  end: number;
};

export class Program {
  readonly pieces: Piece[];
  // The length of one time through.
  readonly length: number;
  private offsets: number[] = [];

  constructor(
    pieces: Piece[],
    readonly loop: boolean,
  ) {
    // Pieces in order, and none that is nothing.
    this.pieces = pieces.filter((p) => p.end > p.start).sort((a, b) => a.start - b.start);
    let at = 0;
    for (const p of this.pieces) {
      this.offsets.push(at);
      at += p.end - p.start;
    }
    this.length = at;
  }

  // The v-th piece played, or null past the end of a program that does not
  // loop.
  visit(v: number): Visit | null {
    const n = this.pieces.length;
    if (!n || v < 0 || (!this.loop && v >= n)) return null;
    const cycle = Math.floor(v / n);
    const piece = v % n;
    const p = this.pieces[piece];
    return { v, piece, from: cycle * this.length + this.offsets[piece], start: p.start, end: p.end };
  }

  // The visit a position is in and the moment of the episode it stands
  // for, or null at or past the end of a program that does not loop.
  locate(pos: number): { v: number; at: number } | null {
    const n = this.pieces.length;
    if (!n || this.length <= 0) return null;
    if (!this.loop && pos >= this.length) return null;
    const cycle = this.loop ? Math.floor(Math.max(pos, 0) / this.length) : 0;
    const within = Math.max(pos - cycle * this.length, 0);
    let piece = 0;
    while (piece + 1 < n && this.offsets[piece + 1] <= within) piece++;
    const p = this.pieces[piece];
    const at = Math.min(p.start + (within - this.offsets[piece]), p.end);
    return { v: cycle * n + piece, at };
  }

  // Where on the program a moment of the episode is. Inside a piece it is
  // its own place. In a cut, or before the first piece, it is the start of
  // the piece after it, and past the last piece it is the end.
  place(at: number): number {
    for (let i = 0; i < this.pieces.length; i++) {
      const p = this.pieces[i];
      if (at < p.start) return this.offsets[i];
      if (at < p.end) return this.offsets[i] + (at - p.start);
    }
    return this.length;
  }
}

// The key frame decoding has to start from for the frame shown rank-th:
// the last one, in decode order, before that frame, that is not shown
// after it.
export function keyBefore(s: Samples, rank: number): number {
  let key = s.order[rank];
  while (key > 0 && !(s.key[key] && s.rank[key] <= rank)) key--;
  return key;
}

// What a paused frame is decoded from: the key frame before it to the
// frame itself, in decode order. A frame shown before it but decoded after
// it is not needed, and the decoder is flushed after the last sample so the
// frame comes out without waiting for more.
export function stillFeed(s: Samples, at: number): { key: number; last: number; rank: number } {
  const rank = rankAt(s, at);
  return { key: keyBefore(s, rank), last: s.order[rank], rank };
}

// One frame on the program: which run decodes it, and which frame of the
// episode it is, by its place in the order frames are shown.
export type Need = { run: number; rank: number };

export type Run = {
  id: number;
  // Decode indices, both ends included: the key frame it starts from and
  // the last sample it is fed. A run carried on by the next piece has its
  // last sample moved on.
  key: number;
  last: number;
  // The grid points it draws, both ends included.
  kFirst: number;
  kLast: number;
  // The latest frame it draws, by rank, so a piece that would have to go
  // back starts a run of its own.
  lastRank: number;
};

// One visit on the grid: its points kFirst to kLast draw the frames of
// rank first to first + kLast - kFirst, one point a frame. Where on the
// program its points begin and where the visit ends.
type Span = { visit: Visit; run: number; kFirst: number; kLast: number; first: number; from: number; to: number };

// The hair before a piece's end that the piece-end frame holds. A frame
// that begins within it would be on screen for no time at all.
const HAIR = 1e-4;

// Which frames the picture needs, from a play that starts at P0 on the
// program, worked out a visit at a time as far ahead as it is asked.
export class VideoPlan {
  readonly runs: Run[] = [];
  private spans: Span[] = [];
  private nextVisit: number;
  private nextK = 0;
  private done = false;

  constructor(
    readonly samples: Samples,
    // How long a frame lasts, for how far ahead a decoder starts.
    readonly frame: number,
    readonly program: Program,
    readonly p0: number,
  ) {
    const first = program.locate(p0);
    this.nextVisit = first ? first.v : 0;
    if (!first) this.done = true;
  }

  // Where a frame of the episode begins, in seconds.
  private begins(rank: number): number {
    const s = this.samples;
    return s.pts[s.order[rank]] / s.timescale;
  }

  private posOf(sp: Span, k: number): number {
    if (k === sp.kFirst) return sp.from;
    return sp.visit.from + (this.begins(sp.first + (k - sp.kFirst)) - sp.visit.start);
  }

  // The position of grid point k on the program, or NaN where it is not
  // worked out.
  at(k: number): number {
    const sp = this.spanOf(k);
    return sp ? this.posOf(sp, k) : NaN;
  }

  // The grid point a position is in: the last one at or before it. Past
  // the end of a program worked out to its end, the point after the last.
  gridAt(pos: number): number {
    const all = this.spans;
    if (!all.length) return this.nextK;
    let lo = 0;
    let hi = all.length - 1;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (all[mid].from <= pos + 1e-9) lo = mid;
      else hi = mid - 1;
    }
    const sp = all[lo];
    if (pos < sp.from - 1e-9) return sp.kFirst;
    if (pos >= sp.to - 1e-9 && lo === all.length - 1) return this.done ? sp.kLast + 1 : sp.kLast;
    let a = sp.kFirst;
    let b = sp.kLast;
    while (a < b) {
      const mid = (a + b + 1) >> 1;
      if (this.posOf(sp, mid) <= pos + 1e-9) a = mid;
      else b = mid - 1;
    }
    return a;
  }

  // The next grid point after k that has to be drawn, whatever is due: the
  // piece-end frame of k's visit, or the piece-start frame of the next. A
  // frame that is on screen for less than a display frame, at the edge of a
  // piece that ends just after a frame begins, is drawn for one, and the
  // frame after it waits that long. Any other frame due at the same moment
  // as a later one is passed over.
  nextEdge(k: number): number {
    const sp = this.spanOf(k);
    return sp && k < sp.kLast ? sp.kLast : k + 1;
  }

  // Works out the visits that start before a position on the program.
  // Returns false once there are no more.
  extend(until: number): boolean {
    while (!this.done) {
      const visit = this.program.visit(this.nextVisit);
      if (!visit) {
        this.done = true;
        break;
      }
      if (visit.from > until) break;
      this.nextVisit++;
      this.add(visit);
    }
    return !this.done;
  }

  // Whether every visit has been worked out, so the last run is the last.
  get finished(): boolean {
    return this.done;
  }

  // The last grid point of the program, or Infinity while it loops or is
  // not yet worked out to the end.
  get lastK(): number {
    if (!this.done) return Infinity;
    return this.nextK - 1;
  }

  // The first grid point still worked out, after forget.
  get firstK(): number {
    return this.spans.length ? this.spans[0].kFirst : 0;
  }

  private add(visit: Visit) {
    const s = this.samples;
    const length = visit.end - visit.start;
    const from = Math.max(visit.from, this.p0);
    const to = visit.from + length;
    if (to - from <= 1e-9 || s.count === 0) return;
    const moment = visit.start + (from - visit.from);
    const a = rankAt(s, moment);
    const b = Math.max(a, rankAt(s, Math.max(moment, visit.end - HAIR)));
    const kFirst = this.nextK;
    const kLast = kFirst + (b - a);
    this.nextK = kLast + 1;
    const key = keyBefore(s, a);
    let last = s.order[a];
    for (let r = a; r <= b; r++) if (s.order[r] > last) last = s.order[r];
    const span: Span = { visit, run: 0, kFirst, kLast, first: a, from, to };
    const run = this.runs[this.runs.length - 1];
    if (run && run.key <= key && key <= run.last + 1 && a >= run.lastRank) {
      run.last = Math.max(run.last, last);
      run.kLast = kLast;
      run.lastRank = b;
      span.run = run.id;
      this.spans.push(span);
      return;
    }
    const id = this.runs.length;
    this.runs.push({ id, key, last, kFirst, kLast, lastRank: b });
    span.run = id;
    this.spans.push(span);
  }

  // The frame drawn at grid point k, or null where the program has none,
  // past its end or ahead of what has been worked out.
  need(k: number): Need | null {
    const span = this.spanOf(k);
    if (!span) return null;
    return { run: span.run, rank: span.first + (k - span.kFirst) };
  }

  // The moment of the episode grid point k stands for.
  moment(k: number): number | null {
    const span = this.spanOf(k);
    if (!span) return null;
    return span.visit.start + (this.posOf(span, k) - span.visit.from);
  }

  private spanOf(k: number): Span | null {
    let lo = 0;
    let hi = this.spans.length - 1;
    while (lo <= hi) {
      const mid = (lo + hi) >> 1;
      const sp = this.spans[mid];
      if (k < sp.kFirst) hi = mid - 1;
      else if (k > sp.kLast) lo = mid + 1;
      else return sp;
    }
    return null;
  }

  // Drops the visits already played, so a loop that runs for an hour keeps
  // a few, not thousands.
  forget(k: number) {
    let n = 0;
    while (n < this.spans.length - 1 && this.spans[n].kLast < k) n++;
    if (n) this.spans.splice(0, n);
  }
}

// Decides, frame by frame as a run's decoder puts them out, which to keep
// for drawing and which to close at once. A run's decoder puts its frames
// out in the order they are shown, and the frames a run draws come in that
// order too, so one walk along the grid is enough.
export class Keeper {
  private k: number;
  constructor(
    private plan: VideoPlan,
    private run: Run,
  ) {
    this.k = run.kFirst;
  }

  // Whether the frame of this rank is drawn by this run.
  offer(rank: number): boolean {
    // A decoder that is a whole piece late is behind what the plan still
    // remembers, and starts from what it does.
    this.k = Math.max(this.k, Math.min(this.plan.firstK, this.run.kLast + 1));
    for (;;) {
      if (this.k > this.run.kLast) return false;
      const need = this.plan.need(this.k);
      if (!need || need.run !== this.run.id) return false;
      if (need.rank > rank) return false;
      if (need.rank === rank) return true;
      this.k++;
    }
  }
}

// The sound, in the sound card's samples. A visit's sound is a stretch of
// the episode's samples, put on the program at a place of its own, and the
// stretches meet exactly: each visit begins on the program where the last
// one ended, to the sample.
export type Stretch = {
  visit: Visit;
  // Samples of the episode, [from, to), at the track's own rate.
  from: number;
  to: number;
  // Where on the program, in samples, the stretch begins.
  out: number;
  // Where the piece's own edges are on the program, for the fade.
  pieceOut: number;
  pieceEnd: number;
};

export type AudioRun = {
  id: number;
  first: number;
  last: number;
  stretches: Stretch[];
  // Where the sound the decoder last put out for this run ended, in the
  // track's samples, see decoded.
  end?: number;
};

// What the sound decoder is fed. Sound has no key frames, but a decoder
// needs a little before the first sample it is to put out right, so a run
// starts that far before, and what comes out of that is not played.
export class AudioPlan {
  readonly runs: AudioRun[] = [];
  private nextVisit: number;
  private done = false;
  // The sound card's samples a second, the track's own.
  readonly rate: number;
  readonly preroll: number;

  constructor(
    readonly samples: Samples,
    rate: number,
    readonly program: Program,
    readonly p0: number,
    // How much before a run's first sample is fed, in seconds. Opus asks
    // for 80 ms, and AAC for one frame, so both get 100.
    preroll = 0.1,
  ) {
    this.rate = rate;
    this.preroll = Math.round(preroll * rate);
    const first = program.locate(p0);
    this.nextVisit = first ? first.v : 0;
    if (!first) this.done = true;
  }

  get finished(): boolean {
    return this.done;
  }

  // The first sample of the program play starts at.
  get m0(): number {
    return Math.round(this.p0 * this.rate);
  }

  // Where a packet's sound starts and how long it is, in the track's
  // samples a second, whatever its ticks are.
  packet(j: number): { at: number; n: number } {
    const s = this.samples;
    const scale = this.rate / s.timescale;
    return { at: Math.round(s.pts[j] * scale), n: Math.round(s.duration[j] * scale) };
  }

  // The first packet that holds sample x or anything after it.
  private packetFrom(x: number): number {
    let lo = 0;
    let hi = this.samples.count - 1;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      const p = this.packet(mid);
      if (p.at + p.n > x) hi = mid;
      else lo = mid + 1;
    }
    return lo;
  }

  // The last packet that starts before sample x.
  private packetBefore(x: number): number {
    let lo = 0;
    let hi = this.samples.count - 1;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (this.packet(mid).at < x) lo = mid;
      else hi = mid - 1;
    }
    return lo;
  }

  extend(until: number): boolean {
    while (!this.done) {
      const visit = this.program.visit(this.nextVisit);
      if (!visit) {
        this.done = true;
        break;
      }
      if (visit.from > until) break;
      this.nextVisit++;
      this.add(visit);
    }
    return !this.done;
  }

  private add(visit: Visit) {
    if (!this.samples.count) return;
    const r = this.rate;
    // On the program, by the visit's edges, so one visit ends where the
    // next begins.
    const pieceOut = Math.round(visit.from * r);
    const pieceEnd = Math.round((visit.from + visit.end - visit.start) * r);
    const out = Math.max(pieceOut, this.m0);
    if (pieceEnd <= out) return;
    const from = Math.round(visit.start * r) + (out - pieceOut);
    const to = from + (pieceEnd - out);
    const stretch: Stretch = { visit, from, to, out, pieceOut, pieceEnd };
    const first = this.packetFrom(from);
    const last = this.packetBefore(to);
    const run = this.runs[this.runs.length - 1];
    // Carried on when the stretch starts no further on than a run would
    // start before it anyway, and not back in what was fed.
    const lead = this.packetFrom(from - this.preroll);
    if (run && first >= run.first && lead <= run.last + 1) {
      run.last = Math.max(run.last, last);
      run.stretches.push(stretch);
      return;
    }
    this.runs.push({ id: this.runs.length, first: lead, last, stretches: [stretch] });
  }

  // Where the sound of packet j goes on the program, when it came out of
  // the decoder as `got` samples. A decoder that leaves out the start of a
  // packet, the way one may with the first packet after it starts, leaves
  // out its front, so what came out is the end of it.
  slices(run: AudioRun, j: number, got: number, at?: number): Slice[] {
    const p = this.packet(j);
    at ??= p.at + Math.max(0, p.n - got);
    const end = at + got;
    const out: Slice[] = [];
    for (const st of run.stretches) {
      const a = Math.max(at, st.from);
      const b = Math.min(end, st.to);
      if (a >= b) continue;
      out.push({ from: a - at, to: b - at, out: st.out + (a - st.from), pieceOut: st.pieceOut, pieceEnd: st.pieceEnd });
    }
    return out;
  }

  // The slices of packet j as it came out of the decoder, straight after
  // what came out of it before in the same run. A decoder puts its sound
  // out as one stream, every AAC packet 1024 samples, but a file need not
  // say so to the sample. Tim's start.mp4 says its packets last 1008,
  // 1056 and 1008 samples in turn, 1024 on average, and a clock of 600
  // ticks a second, QuickTime's, says where a packet starts only to the
  // nearest 80 samples at 48 kHz. A packet placed by its stamp missed the
  // one before by 16 to 40 samples at most packets, a gap or an overlap
  // heard as a click, a crackle over the whole play. A packet more than
  // half a packet off is a real jump, and goes where its stamp says.
  decoded(run: AudioRun, j: number, got: number): Slice[] {
    const p = this.packet(j);
    let at = p.at + Math.max(0, p.n - got);
    if (run.end !== undefined && Math.abs(at - run.end) < got / 2) at = run.end;
    run.end = at + got;
    return this.slices(run, j, got, at);
  }

  forget(m: number) {
    for (const run of this.runs) {
      if (run.stretches.length > 1) {
        let n = 0;
        while (n < run.stretches.length - 1 && run.stretches[n].out + (run.stretches[n].to - run.stretches[n].from) < m) n++;
        if (n) run.stretches.splice(0, n);
      }
    }
  }
}

// Samples [from, to) of a decoded packet go to the program at `out`.
export type Slice = { from: number; to: number; out: number; pieceOut: number; pieceEnd: number };

// How loud sample m of the program is at the edges of its piece: a linear
// fade over FADE at each end, the way the render's afade does it.
export function fade(m: number, pieceOut: number, pieceEnd: number, rate: number): number {
  const f = FADE * rate;
  const inside = Math.min(m - pieceOut + 0.5, pieceEnd - m - 0.5);
  return inside >= f ? 1 : Math.max(0, inside / f);
}

// Byte ranges to read so the samples wanted are in hand: what is not read
// already, put together where they lie close, so one request brings what
// the picture and the sound both need from the same part of the file.
export function spans(
  wants: { from: number; to: number }[],
  have: { from: number; to: number }[],
  gap = 1 << 18,
  most = 1 << 23,
): { from: number; to: number }[] {
  const missing = wants
    .filter((w) => !have.some((h) => h.from <= w.from && w.to <= h.to))
    .sort((a, b) => a.from - b.from);
  const out: { from: number; to: number }[] = [];
  for (const w of missing) {
    const last = out[out.length - 1];
    if (last && w.from - last.to <= gap && Math.max(last.to, w.to) - last.from <= most) {
      last.to = Math.max(last.to, w.to);
    } else {
      out.push({ from: w.from, to: w.to });
    }
  }
  return out;
}

// Where the sound being heard is now, on the sound card's clock. The
// output timestamp says which moment of the clock the speaker plays at
// which moment of the page's clock, so the latency of the output is in it.
// Where a browser does not give one, the clock less the latency it says.
//
// Never later than the clock itself: nothing can be heard that was not
// played yet. A stamp from before the sound card was held goes on counting
// from where it was taken, and after a pause of a second it said the sound
// was a second further on than it was, and play went on from there.
//
// And never far from the clock less its latency. WebKitGTK without a sound
// device gives stamps on another page clock, a quarter of a minute off,
// and the picture stood still on its first frame waiting for them, then
// jumped. A stamp taken more than a second ago, or in the future, or more
// than a quarter of a second from what the clock says, is not taken.
export function heardAt(
  stamp: { contextTime: number; performanceTime: number } | null,
  currentTime: number,
  latency: number,
  now: number,
): number {
  const plain = Math.max(0, currentTime - latency);
  const age = stamp ? now - stamp.performanceTime : -1;
  if (stamp && stamp.performanceTime > 0 && stamp.contextTime > 0 && age >= -20 && age <= 1000) {
    const heard = Math.min(stamp.contextTime + (now - stamp.performanceTime) / 1000, currentTime);
    if (Math.abs(heard - plain) <= 0.25) return heard;
  }
  return plain;
}
