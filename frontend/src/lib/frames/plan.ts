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
import type { Source } from "./even";
import { rankAt, type Samples } from "./movie";

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
  // Where each piece starts on one time through, and the length after the
  // last.
  private offsets: number[] = [];
  // How many times through it plays, Infinity while it loops. A loop
  // switched while it plays changes this and nothing else, see loopOn and
  // loopOff.
  private times: number;
  // Where a running play took in a change of the clip, see spliced.
  private splice: Splice | null = null;

  constructor(pieces: Piece[], loop: boolean) {
    this.times = loop ? Infinity : 1;
    // Pieces in order, and none that is nothing.
    this.pieces = pieces.filter((p) => p.end > p.start).sort((a, b) => a.start - b.start);
    let at = 0;
    for (const p of this.pieces) {
      this.offsets.push(at);
      at += p.end - p.start;
    }
    this.offsets.push(at);
    this.length = at;
  }

  // The program a running play goes on with once the clip changed under
  // it, by a drag, an undo or a redo, see FrameQueue.setProgram. Visit v,
  // the one the playhead is in at the moment `at`, ends where the piece
  // that holds that moment ends now, and after it come the pieces after
  // that one, round again from the first while it loops. Everything before
  // stays where it was on the program, so the play goes on with no stop.
  // Null where the moment is in none of the pieces: in a cut, or outside
  // the clip.
  static spliced(before: Program, v: number, at: number, pieces: Piece[], loop: boolean): Program | null {
    const was = before.visit(v);
    const p = new Program(pieces, loop);
    const j = p.pieces.findIndex((q) => at >= q.start && at < q.end);
    if (!was || j < 0) return null;
    const visit = { ...was, piece: j, end: p.pieces[j].end };
    // Spliced again at the same visit, the one before it is what stays.
    const root = before.splice && before.splice.v === v ? before.splice.before : before;
    p.splice = { before: root, v, visit, next: j + 1, base: visit.from + visit.end - visit.start };
    return p;
  }

  // Whether a running play took in a change, see spliced. Such a program
  // is only for the play under way: a new play builds its own.
  get spliced(): boolean {
    return this.splice !== null;
  }

  get loop(): boolean {
    return this.times === Infinity;
  }

  // Where on the program one time through starts: after a splice, the
  // place the pieces after the visit in hand would have.
  private get shift(): number {
    const s = this.splice;
    return s ? s.base - this.offsets[s.next] : 0;
  }

  // Where the program ends, as a position on it, Infinity while it loops.
  // This is all the loop decides: a play goes on until here, whatever it
  // has worked out beyond.
  get end(): number {
    if (this.times === Infinity) return Infinity;
    return this.shift + this.times * this.length;
  }

  // The last piece it plays, the one it ends on.
  get lastPiece(): Piece | undefined {
    const s = this.splice;
    if (s && s.next >= this.pieces.length && this.times === 1) return { start: s.visit.start, end: s.visit.end };
    return this.pieces[this.pieces.length - 1];
  }

  // The loop switched on while it plays: it goes on and on from here.
  loopOn() {
    this.times = Infinity;
  }

  // The loop switched off while it plays, at a position on the program: it
  // ends at the end of the time through that position is in.
  loopOff(pos: number) {
    this.times = this.length > 0 ? Math.floor(Math.max(pos - this.shift, 0) / this.length) + 1 : 1;
  }

  // The v-th piece played, or null past the end of a program that does not
  // loop.
  visit(v: number): Visit | null {
    const s = this.splice;
    if (s && v < s.v) return s.before.visit(v);
    if (s && v === s.v) return s.visit;
    const n = this.pieces.length;
    // Counted on through the times through from the first piece.
    const w = s ? v - s.v - 1 + s.next : v;
    if (!n || w < 0 || w >= this.times * n) return null;
    const cycle = Math.floor(w / n);
    const piece = w % n;
    const p = this.pieces[piece];
    return { v, piece, from: this.shift + cycle * this.length + this.offsets[piece], start: p.start, end: p.end };
  }

  // The visit a position is in and the moment of the episode it stands
  // for, or null at or past the end of a program that does not loop.
  locate(pos: number): { v: number; at: number } | null {
    const s = this.splice;
    if (s && pos < s.visit.from) return s.before.locate(pos);
    if (s && pos < s.base) return { v: s.v, at: s.visit.start + (pos - s.visit.from) };
    const n = this.pieces.length;
    if (!n || this.length <= 0) return null;
    if (pos >= this.end) return null;
    const rel = pos - this.shift;
    const cycle = Math.floor(Math.max(rel, 0) / this.length);
    const within = Math.max(rel - cycle * this.length, 0);
    let piece = 0;
    while (piece + 1 < n && this.offsets[piece + 1] <= within) piece++;
    const p = this.pieces[piece];
    const at = Math.min(p.start + (within - this.offsets[piece]), p.end);
    const w = cycle * n + piece;
    return { v: s ? s.v + 1 + w - s.next : w, at };
  }

  // Where on the program a moment of the episode is, for a play that
  // starts there. Inside a piece it is its own place. In a cut, or before
  // the first piece, it is the start of the piece after it, and past the
  // last piece it is the end.
  place(at: number): number {
    for (let i = 0; i < this.pieces.length; i++) {
      const p = this.pieces[i];
      if (at < p.start) return this.offsets[i];
      if (at < p.end) return this.offsets[i] + (at - p.start);
    }
    return this.length;
  }
}

// What a running play keeps of the program it had when the clip changed
// under it, see Program.spliced: the visits before v as they were, visit v
// with its new end, and where on the program the pieces after it begin,
// with next the first of them.
type Splice = { before: Program; v: number; visit: Visit; next: number; base: number };

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
  // Its decoder was fed past what it draws now, after the clip changed
  // under the play, so no later piece carries it on, see rewind.
  sealed?: boolean;
};

// One visit on the grid: its points kFirst to kLast draw the frames of
// rank first to first + kLast - kFirst, one point a frame. Where on the
// program its points begin and where the visit ends.
type Span = { visit: Visit; run: number; kFirst: number; kLast: number; first: number; from: number; to: number };

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
    public program: Program,
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
  // The program's loop was switched while this plays. Says whether what
  // was worked out so far still holds: switched on, a plan that had come
  // to the end goes on, and switched off, it holds as long as no visit
  // past the new end was worked out yet.
  relooped(): boolean {
    if (this.nextVisit > 0 && !this.program.visit(this.nextVisit - 1)) return false;
    this.done = false;
    return true;
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

  // The clip changed under the play, see Program.spliced: the visit the
  // playhead is in ends where its piece ends now, and what was worked out
  // after it goes, to be worked out again from the new program. Says how,
  // or null where the decoders were fed what no longer holds: a run
  // after the visit's has a decoder, or the visit runs on into frames its
  // decoder was fed past. fedTo says how far a run's decoder was fed, by
  // decode index, -1 for none and Infinity once it was flushed.
  rewind(program: Program, visit: Visit, fedTo: (run: Run) => number): (() => void) | null {
    let i = this.spans.length - 1;
    while (i >= 0 && this.spans[i].visit.v !== visit.v) i--;
    if (i < 0) return null;
    const sp = this.spans[i];
    // Cut short, with nothing after the new end: what was worked out stays
    // as it was fed, and the play ends at the program's end. A later
    // change is weighed against it, see AudioPlan.rewind.
    if (visit.end < sp.visit.end && !program.visit(visit.v + 1)) {
      return () => {
        this.program = program;
        this.nextVisit = visit.v + 1;
        this.done = false;
      };
    }
    const run = this.runs[sp.run];
    if (this.runs.slice(sp.run + 1).some((r) => fedTo(r) >= 0)) return null;
    const s = this.samples;
    const was = sp.first + (sp.kLast - sp.kFirst);
    const b = Math.max(sp.first, rankAt(s, Math.max(visit.start + (sp.from - sp.visit.from), visit.end - this.frame)));
    // The frames it now draws that it did not, each first in decode order.
    let needs = Infinity;
    for (let r = was + 1; r <= b; r++) needs = Math.min(needs, s.order[r]);
    const fed = fedTo(run);
    if (fed > needs) return null;
    return () => {
      this.program = program;
      this.spans.length = i + 1;
      this.runs.length = sp.run + 1;
      sp.visit = visit;
      sp.kLast = sp.kFirst + (b - sp.first);
      sp.to = visit.from + (visit.end - visit.start);
      run.kLast = sp.kLast;
      run.lastRank = b;
      run.last = 0;
      for (const o of this.spans) {
        if (o.run !== run.id) continue;
        for (let r = o.first; r <= o.first + (o.kLast - o.kFirst); r++) run.last = Math.max(run.last, s.order[r]);
      }
      if (fed > run.last) run.sealed = true;
      this.nextK = sp.kLast + 1;
      this.nextVisit = visit.v + 1;
      this.done = false;
    };
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
    // The last frame of a piece is the one that holds the moment a frame
    // before its end, see frameAt in lib/flow.ts.
    const a = rankAt(s, moment);
    const b = Math.max(a, rankAt(s, Math.max(moment, visit.end - this.frame)));
    const kFirst = this.nextK;
    const kLast = kFirst + (b - a);
    this.nextK = kLast + 1;
    const key = keyBefore(s, a);
    let last = s.order[a];
    for (let r = a; r <= b; r++) if (s.order[r] > last) last = s.order[r];
    const span: Span = { visit, run: 0, kFirst, kLast, first: a, from, to };
    const run = this.runs[this.runs.length - 1];
    if (run && !run.sealed && run.key <= key && key <= run.last + 1 && a >= run.lastRank) {
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
  // track's samples, and the packet it came from, see decoded.
  end?: number;
  endPacket?: number;
};

// What the sound decoder is fed. Sound has no key frames, but a decoder
// needs a little before the first sample it is to put out right, so a run
// starts that far before, and what comes out of that is not played.
export class AudioPlan {
  readonly runs: AudioRun[] = [];
  private nextVisit: number;
  private done = false;
  // The program ends inside a stretch fed as it was before, with no fade
  // at that end, see rewind.
  cutShort = false;
  // The sound card's samples a second, the track's own.
  readonly rate: number;
  readonly preroll: number;

  constructor(
    readonly samples: Samples,
    rate: number,
    public program: Program,
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

  // The program's loop was switched while this plays. Says whether what
  // was worked out so far still holds: switched on, a plan that had come
  // to the end goes on, and switched off, it holds as long as no visit
  // past the new end was worked out yet.
  relooped(): boolean {
    if (this.nextVisit > 0 && !this.program.visit(this.nextVisit - 1)) return false;
    this.done = false;
    return true;
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

  // The clip changed under the play, see Program.spliced and
  // VideoPlan.rewind: the stretch of the visit the playhead is in ends
  // where its piece ends now, and what was worked out after it goes. Says
  // how, or null where the sound card was fed what no longer holds: fed to
  // `fed` on the program, in samples, past where the two programs part,
  // or into the fade where the stretch ended, or a stretch that runs on
  // in a run whose decoder was flushed, see closed.
  //
  // A play cut short, with nothing after its new end, keeps its stretches
  // as they are and is fed on past that end, where nothing is heard: the
  // sound card stops it there and fades it out, see cutShort. So an end
  // dragged in towards the playhead and put back by an undo, or dragged
  // on past it, is weighed against what was really fed, which has no fade
  // where the play now ends.
  rewind(program: Program, visit: Visit, fed: number, closed: (run: AudioRun) => boolean): (() => void) | null {
    let ri = this.runs.length - 1;
    let si = -1;
    for (; ri >= 0 && si < 0; ri--) {
      const sts = this.runs[ri].stretches;
      for (si = sts.length - 1; si >= 0 && sts[si].visit.v !== visit.v; si--);
      if (si >= 0) break;
    }
    if (ri < 0) return null;
    const run = this.runs[ri];
    const st = run.stretches[si];
    const pieceEnd = Math.round((visit.from + visit.end - visit.start) * this.rate);
    if (pieceEnd < st.pieceEnd && !program.visit(visit.v + 1)) {
      return () => {
        this.program = program;
        this.nextVisit = visit.v + 1;
        this.done = false;
        this.cutShort = true;
      };
    }
    const parts = pieceEnd === st.pieceEnd ? st.pieceEnd : Math.min(pieceEnd, st.pieceEnd) - FADE * this.rate;
    if (fed > parts) return null;
    // A run whose decoder was flushed is fed no more.
    if (pieceEnd > st.pieceEnd && closed(run)) return null;
    return () => {
      this.program = program;
      run.stretches.length = si + 1;
      this.runs.length = ri + 1;
      st.visit = visit;
      st.to = st.from + (pieceEnd - st.out);
      st.pieceEnd = pieceEnd;
      run.last = this.packetBefore(st.to);
      this.nextVisit = visit.v + 1;
      this.done = false;
      this.cutShort = false;
    };
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
    // start before it anyway, and not back in what was fed: never before
    // where the run's last stretch ends, which a loop back to the start of
    // a clip with no cut would be.
    const lead = this.packetFrom(from - this.preroll);
    if (run && from >= run.stretches[run.stretches.length - 1].to && lead <= run.last + 1) {
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
  // what came out of the packet before it. A decoder puts its sound out as
  // one stream, and a player counts its samples: the stamps only say where
  // the stream starts. They need not say more to the sample. Every AAC
  // packet decodes to 1024 samples, but Tim's start.mp4 says its packets
  // last 1008, 1056 and 1008 in turn, and a clock of 600 ticks a second,
  // QuickTime's, says where a packet starts only to the nearest 80 samples
  // at 48 kHz. Placed by its stamp, a packet missed the one before at most
  // packets, a gap or an overlap heard as a click, a crackle over the whole
  // play. So only the first packet that comes out of a run goes where its
  // stamp says, and one after a packet that came out as nothing, since
  // then the stream itself has a gap.
  decoded(run: AudioRun, j: number, got: number): Slice[] {
    const p = this.packet(j);
    const at = run.end !== undefined && run.endPacket === j - 1 ? run.end : p.at + Math.max(0, p.n - got);
    run.end = at + got;
    run.endPacket = j;
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
//
// It says where the moment came from too, for the log: the output
// timestamp, the timestamp held back to the clock, or the clock less its
// latency, see even.ts.
export function heard(
  stamp: { contextTime: number; performanceTime: number } | null,
  currentTime: number,
  latency: number,
  now: number,
): { at: number; source: Source } {
  const plain = Math.max(0, currentTime - latency);
  const age = stamp ? now - stamp.performanceTime : -1;
  if (stamp && stamp.performanceTime > 0 && stamp.contextTime > 0 && age >= -20 && age <= 1000) {
    const said = stamp.contextTime + (now - stamp.performanceTime) / 1000;
    const at = Math.min(said, currentTime);
    if (Math.abs(at - plain) <= 0.25) return { at, source: said > currentTime ? "held" : "stamp" };
  }
  return { at: plain, source: "plain" };
}
