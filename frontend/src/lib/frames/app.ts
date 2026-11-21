// The picture of the episode, decoded on the Go side by the episode's
// decoder, see cmd/framefairy-app/frames.go: the frames of the short, on
// the grid the engine names, see movie.ts.
//
// While the video preview stands still, the window says where the hand is
// and what it does there, a look, and draws the frame it is sent: which
// frame the decoder makes, what it gives up on and what it keeps is the Go
// side's, see engine.Schedule and
// docs/decisions/2026-11-19-go-schedules-the-video-previews-frames.md.
// A play is the Go side's, see play.ts.
//
// - A frame is decoded at the size of the canvas and no larger.
// - The frames sent last are kept, as memory to draw from again, and the
//   ones the Go side read around the place the hand rests on, so a step
//   with the arrow keys shows its frame with no request.
// - What to read while the hand rests, the words beside and the cards, is
//   handed to the Go side, which reads it when its decoder has nothing to
//   do for the hand and sends what it read, kept here too, see ahead. The
//   cards' frames are kept for as long as their cards are there.
import { rankOf, ticks, type VideoTrack } from "./movie";
import type { Picture } from "./picture";
import { pictures, pull, type Pulled } from "./pull";
import { frames as framePool } from "./pool";

// A pull with its frames as Pictures.
type Got = Omit<Pulled, "frames"> & { frames: { at: number; frame: Picture; stamp: number[] }[] };

// How much of the frames sent last is kept. The kept frames are counted, so
// the room follows the size of the frames: 64 are about 150 MB at 1012 by
// 570. A place the hand rests on is sent up to 20 frames around it and 12
// for the words beside, see engine.Schedule, and the window keeps the
// place before it too, so a word key back finds its frame. With 40 it let
// go of the frames of the place before as the words beside came, and a
// word back found its frame missing. Never more than 192 MB, so a big
// screen on a Mac with little memory keeps no more than that.
const KEPT_FRAMES = 64;
const KEPT_BYTES = 192 << 20;
// How many frames after a word's first the Go side reads ahead with it,
// lookAhead in engine/schedule.go, and the one before it.

// A frame asked for. gone is set when the one who asked no longer wants
// it, a newer still. seq numbers the asks in the order they were made.
// moving is set where the hand moves over the frame, see frame.
export type Waiter = {
  rank: number;
  seq: number;
  done: (f: Picture | null) => void;
  gone?: boolean;
  moving?: boolean;
};

// The streams of one episode and the frames kept from them, shared by the
// queue's decoders.
export class AppFrames {
  private kept = new Map<number, Picture>();
  private keptBytes = 0;
  // Why the Go side could not decode a frame, for the queue to say.
  trouble = "";
  // Told of a frame the Go side sent on the way to the place the hand
  // moves to, so the queue can put it up while the frame under the hand is
  // on its way. seq is the ask's. The frame stays the kept one's.
  onPassing?: (rank: number, frame: Picture, seq: number) => void;
  // The frame on screen, or -1, which a look says, so the Go side sends a
  // frame on the way only where it is nearer the hand than that one.
  shownRank: () => number = () => -1;
  // Whether the hand holds the playhead, see grip, and the places it
  // asked for since it took hold.
  private dragging = false;
  private dragPlaces = 0;
  // The number of the newest ask.
  asked = 0;
  // This window's name and how many looks and asks to read ahead it has
  // sent the Go side, which serves each on a goroutine of its own, so they
  // may come in another order: by these it knows which one was sent last,
  // see engine.Sent.
  private readonly name = Math.random().toString(36).slice(2, 12);
  private sent = 0;
  // What to read while the hand rests, see ahead and cards: the frames of
  // the words beside, the cards', what was last handed to the Go side,
  // and the ask still on its way, which settles once the Go side has read
  // it all, or a newer ask took its place.
  private aheadWanted: number[] = [];
  private cardOrder: number[] = [];
  // The cards' frames the Go side sent, kept apart from the frames sent
  // last, for as long as their cards are there.
  private cardKept = new Map<number, Picture>();
  private aheadSent = "";
  private aheadAsk: Promise<void> | null = null;
  // Whether a play is on its way, which holds the reading ahead until the
  // video preview stands still again, see hold, and whether it has begun
  // since, see began.
  private playingSoon = false;
  private played = false;
  // Whether the app is being sized: the cards' frames are read at the
  // size it comes to rest at, not at every size a dragged edge passes.
  private sizing = false;
  // Whether the video preview stands still. A card's frame read during a
  // play is a second decoder seeking in the file the play decodes, so the
  // reading ahead waits for the pause, the way the captions do, see
  // CaptionLayers.more.
  paused: () => boolean = () => true;
  // The near frames asked of the Go side, and the place asked for next.
  private nearAsk = false;
  // The place of the hand the Go side was asked for or told last, and
  // whether the hand moved there, see tell.
  private place = -1;
  private placeMoving = false;
  // The looks on their way, by the rank they ask for, see look.
  private asking = new Map<number, Promise<unknown>>();
  private nearNext = -1;
  // first is the size the first frame was asked at, for the walks.
  // keptRanks are the ranks of the frames last handed over from the kept
  // ones, here or on the Go side, for the walks. ahead counts the frames
  // handed to the Go side to read ahead. looks counts the looks sent, near the frames the Go side sent on the
  // way to a place, overtaken the looks it answered with nothing, a newer
  // look having taken their place, and fromGo every frame the Go side
  // sent.
  readonly stats = {
    kept: 0,
    keptRanks: [] as number[],
    looks: 0,
    near: 0,
    overtaken: 0,
    fromGo: 0,
    ahead: 0,
    first: "",
  };
  // Every step on the way to a frame, for the app's log, see lib/said.ts.
  trace: (line: string) => void = () => {};
  readonly puller = new Puller((line) => this.trace(line));
  // The grid as the Go side reads it, see framewire.GridText.
  readonly gridText: string;

  // The episode's decoder on the Go side is held for as long as these
  // frames are open: started now, with the file open and decoders ready,
  // and never closed for standing unused while the episode is open, so a
  // click after a long pause is as quick as the first.
  constructor(
    readonly path: string,
    readonly track: VideoTrack,
    private size: () => { width: number; height: number },
    private onTrouble: (why: string) => void,
    // Calls back once the frame on screen is drawn, see afterPaint.
    private painted: (then: () => void) => void = afterPaint,
  ) {
    const g = track.grid;
    this.gridText = g ? [g.origin, g.start, g.num, g.den, g.count].join(",") : "";
    void fetch(`/frames/hold?${new URLSearchParams({ path })}`).catch(() => {});
  }

  private closed = false;

  ptsOf(rank: number): number {
    const s = this.track.samples;
    return ticks(s, Math.max(0, Math.min(rank, s.count - 1))) / s.timescale;
  }

  // The hand takes hold of the playhead, or lets go of it, see
  // FrameQueue.grip. Nothing is read ahead while it holds it, see ahead.
  grip(held: boolean) {
    const letGo = this.dragging && !held;
    this.dragging = held;
    this.dragPlaces = 0;
    // A hand that lets go where the frame is on screen asks for no frame,
    // so the Go side is told: a click after it was taken for a drag, and
    // every frame on the way to it was made and sent.
    if (letGo && this.placeMoving && this.place >= 0) this.tell({ rank: this.place, seq: ++this.asked, done: () => {} });
    this.aheadNow();
  }

  // The frame of this rank, as a new frame with this timestamp, once it is
  // there. Null only when nothing near it could be decoded, or when a
  // newer look took its place.
  // The frame is where the hand put the playhead, the frame of a still:
  // it is a look, see look, and the Go side decides how the decoder gets
  // there. A play's frames are the play's, see play.ts.
  frame(rank: number, timestamp: number, asked?: Set<Waiter>): Promise<Picture | null> {
    return new Promise((done) => {
      const w: Waiter = {
        rank,
        seq: ++this.asked,
        // Every place but the first of a hold, which is where the press
        // landed: a click moves over nothing.
        moving: this.dragging && ++this.dragPlaces > 1,
        done: (f) => {
          asked?.delete(w);
          done(f && !w.gone ? f.clone(timestamp) : null);
        },
      };
      asked?.add(w);
      this.route(w);
    });
  }

  // Asks the Go side for where the hand is, see engine.Schedule, and
  // draws what it sends: the frame of the place, or a frame on the way to
  // it, put up as a frame passing, after which the place is asked again. A
  // look a newer one took the place of comes back with nothing.
  private async look(w: Waiter) {
    const was = this.asking.get(w.rank);
    if (was) {
      // The same frame is asked for already, the frame under the hand and
      // the first of the play cued there as the hand lets go: one answer
      // serves both, and the frame is sent once. Sent twice, the second
      // came 40 ms after the first, and the cue waited for it.
      await was;
      if (w.gone || this.closed) w.done(null);
      else this.route(w);
      return;
    }
    const { width, height } = this.size();
    this.stats.first ||= `${width}x${height}`;
    this.stats.looks++;
    const q = new URLSearchParams({
      path: this.path,
      frame: String(w.rank),
      w: String(width),
      h: String(height),
      grid: this.gridText,
      shown: String(this.shownRank()),
    });
    q.set("win", this.name);
    q.set("n", String(++this.sent));
    if (w.moving) q.set("move", "1");
    this.place = w.rank;
    this.placeMoving = !!w.moving;
    const asked = this.puller.pull(`/frames/look?${q}`, width, height);
    this.asking.set(w.rank, asked);
    const got = await asked;
    if (this.asking.get(w.rank) === asked) this.asking.delete(w.rank);
    if (this.closed) {
      for (const { frame } of got.frames) frame.close();
      w.done(null);
      return;
    }
    const sent = got.frames[got.frames.length - 1];
    if (!sent) {
      if (got.error) {
        this.trace(`a look at frame ${w.rank} failed: ${got.error}`);
        this.failed(got.error);
      } else if (got.status === 204) this.stats.overtaken++;
      else if (got.status !== 200) this.trace(`a look at frame ${w.rank} answered ${got.status}`);
      w.done(got.error ? this.nearest(w.rank) : null);
      return;
    }
    const rank = rankOf(this.track.samples, sent.at);
    this.keep(rank, sent.frame);
    if (got.exact) {
      if (got.kept) this.tookKept(rank);
      w.done(sent.frame);
      // Where the hand rests, the Go side reads nothing around the place
      // until the frame is drawn and it is asked, see
      // engine.Schedule.Near.
      if (got.near) {
        if (w.moving) this.near(rank);
        else this.drawn(rank);
      }
      return;
    }
    this.stats.near++;
    this.onPassing?.(rank, sent.frame, w.seq);
    if (w.gone) w.done(null);
    else void this.look(w);
  }

  // The frames of these ranks are read while the hand rests and kept
  // here: the first frame of the word after the playhead and of the word
  // before, the words Shift and an arrow walk to, so a step to one of them
  // finds its frame kept. The Go side reads them once its decoder has
  // nothing to do for the hand, with the frames beside them, see
  // engine.Ahead. Each call replaces the ones before.
  ahead(ranks: number[]) {
    this.aheadWanted = ranks.filter((r) => r >= 0);
    this.aheadNow();
  }

  // The first frames of the clips on the cards, by rank, the likeliest to
  // be clicked first: read after the words beside, as many as the Go side
  // says, and kept here for as long as their cards are there. Each call
  // replaces the ones before. Tim asked for every clip on the cards to be
  // ready to show, issue #383.
  cards(ranks: number[]) {
    this.cardOrder = ranks.filter((r) => r >= 0);
    const on = new Set(this.cardOrder);
    for (const [r, f] of this.cardKept)
      if (!on.has(r)) {
        this.cardKept.delete(r);
        f.close();
      }
    this.aheadNow();
  }

  resizing(now: boolean) {
    this.sizing = now;
    this.aheadNow();
  }

  // A play is on its way: nothing is read ahead until the video preview
  // stands still again, see aheadNow.
  hold() {
    this.playingSoon = true;
    this.played = false;
    this.aheadNow();
  }

  // The play on its way has begun, so the pause that ends it lets the
  // reading ahead go on, see aheadNow.
  began() {
    this.played = true;
  }

  // The hand asks again while the video preview stands still: a play that
  // was asked for and never started holds nothing any more.
  still() {
    if (this.paused()) this.playingSoon = false;
    this.aheadNow();
  }

  // Hands the Go side what to read while the hand rests, unless it is
  // what it has already. While the hand holds the playhead, the app is
  // sized or a play runs, it is handed over held, which stops the reading.
  aheadNow() {
    if (this.closed) return;
    // A play that ran and stopped lets the reading go on.
    if (this.playingSoon && this.played && this.paused()) this.playingSoon = false;
    const held = this.dragging || this.sizing || this.playingSoon || !this.paused();
    const { width, height } = this.size();
    const q = new URLSearchParams({
      path: this.path,
      w: String(width),
      h: String(height),
      grid: this.gridText,
      frames: this.aheadWanted.join(","),
      cards: this.cardOrder.join(","),
    });
    if (held) q.set("held", "1");
    const ask = q.toString();
    if (ask === this.aheadSent) return;
    this.aheadSent = ask;
    if (!held) this.stats.ahead += this.aheadWanted.length + this.cardOrder.length;
    // What is kept here already is not sent again.
    q.set("has", this.keptHere(width, height).join(","));
    q.set("win", this.name);
    q.set("n", String(++this.sent));
    const sent = this.puller.pull(`/frames/ahead?${q}`, width, height).then((got) => {
      // The words' frames held here already, which the Go side did not
      // send again, are made the ones used last before the frames that
      // came are kept: kept as they were, the frames that came made room
      // by letting them go, and a word key pressed next found its frame
      // missing. The Go side names them, see engine.Ahead.Keep.
      for (const r of got.keep) this.used(r);
      const cards = new Set(this.cardOrder);
      for (const { at, frame } of got.frames) {
        const rank = rankOf(this.track.samples, at);
        if (this.closed || frame.width !== this.size().width || frame.height !== this.size().height) frame.close();
        else if (cards.has(rank)) this.keepCard(rank, frame);
        else this.keep(rank, frame);
      }
      if (got.error) this.trace(`reading ahead failed: ${got.error}`);
      else if (got.status !== 200 && got.status !== 204 && got.status !== 0) this.trace(`reading ahead answered ${got.status}`);
    });
    this.aheadAsk = sent;
    void sent.then(() => {
      if (this.aheadAsk === sent) this.aheadAsk = null;
      this.settleRest();
    });
  }

  // Settles once nothing is left to read ahead, for the walks, which wait
  // on it rather than on the clock: the newest ask has been read, or was
  // held, which the Go side answers at once.
  rested(): Promise<void> {
    return new Promise((done) => {
      this.restWaiters.push(done);
      this.settleRest();
    });
  }
  private restWaiters: (() => void)[] = [];

  private settleRest() {
    if (this.aheadAsk && !this.closed) return;
    const was = this.restWaiters;
    this.restWaiters = [];
    for (const done of was) done();
  }

  // The ranks of the frames kept here at this size, which the Go side
  // need not send.
  private keptHere(width: number, height: number): number[] {
    const out: number[] = [];
    for (const m of [this.kept, this.cardKept]) for (const [r, f] of m) if (f.width === width && f.height === height) out.push(r);
    return out;
  }

  // A card's frame the Go side sent, kept while its card is there.
  private keepCard(rank: number, frame: Picture) {
    if (!this.closed) this.stats.fromGo++;
    const old = this.cardKept.get(rank);
    if (old === frame) return;
    old?.close();
    this.cardKept.set(rank, frame);
  }

  // A frame the Go side had kept, nothing decoded for it, for the walks.
  private tookKept(rank: number) {
    this.stats.kept++;
    this.stats.keptRanks.push(rank);
    if (this.stats.keptRanks.length > 16) this.stats.keptRanks.shift();
  }

  // Asks the Go side for the frames it read around the place the hand
  // rests on that are not kept here, see engine.Schedule.Near, and keeps
  // them, so a step with the arrow keys or a slow drag finds its frame
  // here. One ask at a time, for the newest place.
  private near(rank: number) {
    this.nearNext = rank;
    if (this.nearAsk || this.closed) return;
    this.nearAsk = true;
    this.nearNext = -1;
    const { width, height } = this.size();
    const has = this.around(rank, width, height);
    const q = new URLSearchParams({ path: this.path, frame: String(rank), w: String(width), h: String(height), grid: this.gridText, has: has.join(",") });
    void this.puller
      .pull(`/frames/near?${q}`, width, height)
      .then((got) => {
        if (this.closed) {
          for (const { frame } of got.frames) frame.close();
          return;
        }
        for (const { at, frame } of got.frames) this.keep(rankOf(this.track.samples, at), frame);
      })
      .finally(() => {
        this.nearAsk = false;
        if (this.nearNext >= 0) this.near(this.nearNext);
      });
  }

  // Tells the Go side where the hand is when the frame under it is kept
  // here: nothing is sent back, and the Go side reads the frames around
  // the place before the hand gets to them, see engine.Schedule. Without
  // it, a hand that moved over kept frames was where the Go side did not
  // know, and the frames past them were read only once it got there.
  private tell(w: Waiter) {
    // The place the Go side was told or asked last is where it knows the
    // hand to be, unless the hand let go there or took hold: a let-go the
    // Go side never heard leaves it thinking the hand still drags.
    if (w.rank === this.place && !!w.moving === this.placeMoving) return;
    this.place = w.rank;
    this.placeMoving = !!w.moving;
    const { width, height } = this.size();
    const q = new URLSearchParams({
      path: this.path,
      frame: String(w.rank),
      w: String(width),
      h: String(height),
      grid: this.gridText,
      shown: String(w.rank),
    });
    q.set("win", this.name);
    q.set("n", String(++this.sent));
    if (w.moving) q.set("move", "1");
    else q.set("has", this.around(w.rank, width, height).join(","));
    q.set("kept", "1");
    void this.puller.pull(`/frames/look?${q}`, width, height).then((got) => {
      for (const { frame } of got.frames) frame.close();
    });
  }

  // The ranks of the frames kept here around a place at this size, which
  // the Go side need not read for it, see engine.Hand.Holds.
  private around(rank: number, width: number, height: number): number[] {
    return this.keptHere(width, height).filter((r) => r >= rank - 16 && r <= rank + 8);
  }

  // Asks Near for the place the hand rests on once its frame is on
  // screen, see afterPaint, unless the video preview closed by then.
  private drawn(rank: number) {
    this.painted(() => {
      if (!this.closed) this.near(rank);
    });
  }

  route(w: Waiter) {
    if (w.gone || this.closed) {
      w.done(null);
      return;
    }
    const { width, height } = this.size();
    // A kept frame of another size is not this frame: one decoded before
    // the canvas had its size, or before the app grew, stays as blurred as
    // it was.
    const have = this.kept.get(w.rank);
    if (have && have.width === width && have.height === height) {
      this.tookKept(w.rank);
      this.used(w.rank);
      w.done(have);
      this.tell(w);
      return;
    }
    const card = this.cardKept.get(w.rank);
    if (card && card.width === width && card.height === height) {
      this.tookKept(w.rank);
      w.done(card);
      this.tell(w);
      return;
    }
    // Where the hand is: the Go side's to decide, see look.
    void this.look(w);
  }

  keep(rank: number, frame: Picture) {
    if (!this.closed) this.stats.fromGo++;
    const old = this.kept.get(rank);
    if (old === frame) return;
    if (old) {
      this.keptBytes -= bytes(old);
      old.close();
      this.kept.delete(rank);
    }
    this.kept.set(rank, frame);
    this.keptBytes += bytes(frame);
    for (const [r, f] of this.kept) {
      if (this.keptBytes <= KEPT_BYTES && this.kept.size <= KEPT_FRAMES) break;
      if (f === frame) continue;
      this.kept.delete(r);
      this.keptBytes -= bytes(f);
      f.close();
    }
  }

  // Makes a kept frame the one used last, see keep.
  private used(rank: number) {
    const have = this.kept.get(rank);
    if (!have) return;
    this.kept.delete(rank);
    this.kept.set(rank, have);
  }

  // The kept frame of this rank, or the closest one before it.
  nearest(rank: number): Picture | null {
    for (let r = rank; r >= Math.max(0, rank - 4); r--) {
      const f = this.kept.get(r);
      if (f) return f;
    }
    return null;
  }

  failed(why: string) {
    this.trouble = why;
    this.onTrouble(why);
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    this.settleRest();
    void fetch(`/frames/release?${new URLSearchParams({ path: this.path })}`).catch(() => {});
    this.puller.close();
    for (const f of this.kept.values()) f.close();
    this.kept.clear();
    this.keptBytes = 0;
    for (const f of this.cardKept.values()) f.close();
    this.cardKept.clear();
  }
}

// Pulls go through a Worker, pull.worker.ts, once it has shown that it can
// reach the Go side, and are read on the page where it cannot: a WebKit
// that will not run a Worker for the app's own scheme still plays.
class Puller {
  private worker: Worker | null = null;
  private ready: Promise<void>;
  private count = 0;
  private waiting = new Map<number, (p: Pulled) => void>();
  // Where the pulls are read, for the walks.
  where: "worker" | "page" | "starting" = "starting";

  constructor(private trace: (line: string) => void) {
    this.ready = this.start().then(() => this.trace(`frames are read on the ${this.where}`));
  }

  private async start() {
    let w: Worker;
    try {
      w = new Worker(new URL("./pull.worker.ts", import.meta.url), { type: "module" });
    } catch {
      this.where = "page";
      return;
    }
    const ok = await new Promise<boolean>((done) => {
      const late = setTimeout(() => done(false), 2000);
      w.onmessage = (e: MessageEvent<{ n: number; reached: boolean }>) => {
        if (e.data.n !== -1) return;
        clearTimeout(late);
        done(e.data.reached);
      };
      w.onerror = () => {
        clearTimeout(late);
        done(false);
      };
      w.postMessage({ n: -1 });
    });
    if (!ok) {
      w.terminate();
      this.where = "page";
      return;
    }
    w.onmessage = (e: MessageEvent<{ n: number; got: Pulled }>) => {
      const done = this.waiting.get(e.data.n);
      this.waiting.delete(e.data.n);
      done?.(e.data.got);
    };
    // A Worker that stops answers what it owed as unreachable, and the
    // pulls after it are read on the page.
    w.onerror = (e) => {
      this.trace(`the frames' worker stopped: ${e.message}, the page reads them now`);
      this.worker = null;
      this.where = "page";
      for (const done of this.waiting.values()) done({ status: 0, end: false, error: "", closed: false, light: "", exact: false, kept: false, near: false, keep: [], frames: [], spare: [] });
      this.waiting.clear();
    };
    this.worker = w;
    this.where = "worker";
  }

  async pull(url: string, width: number, height: number, head = 8, frames = 1): Promise<Got> {
    const got = await this.pulled(url, width, height, head, frames);
    for (const b of got.spare ?? []) framePool.give(b);
    return { ...got, frames: pictures(got, width, height) };
  }

  // A pull is handed as many kept buffers as frames it asks for, see
  // pool.ts. The n of a look's URL is where it stands among what the app
  // sent, and was once taken for how many frames it asks: every look
  // took the whole pool, and a play found it empty.
  private async pulled(url: string, width: number, height: number, head: number, frames: number): Promise<Pulled> {
    await this.ready;
    const spare = framePool.takeUpTo(width * height * 4, frames);
    const w = this.worker;
    if (!w) return pull(url, width, height, spare, head);
    const n = this.count++;
    return new Promise((done) => {
      this.waiting.set(n, done);
      w.postMessage({ n, url: new URL(url, location.href).href, width, height, spare, head }, spare);
    });
  }

  close() {
    this.worker?.terminate();
    this.worker = null;
  }
}

function bytes(f: Picture): number {
  return f.width * f.height * 4;
}

// Calls back once the frame drawn now is on screen: the animation frame
// comes before the paint, the timeout after it. The Go side reads around a
// resting hand only from then on, see engine.Schedule.Near: read at once,
// the decoder took the processor from the app as it drew the frame the
// hand went to.
function afterPaint(then: () => void) {
  requestAnimationFrame(() => setTimeout(then, 0));
}
