// The picture of the episode, decoded on the Go side by the episode's
// decoder, see cmd/framefairy-app/frames.go: the frames of the short, on
// the grid the engine names, see movie.ts. AppPictures answers the calls
// the frame queue makes of a decoder.
//
// While the video preview stands still, the window says where the hand is
// and what it does there, a look, and draws the frame it is sent: which
// frame the decoder makes, what it gives up on and what it keeps is the Go
// side's, see engine.Schedule and
// docs/decisions/2026-11-19-go-schedules-the-video-previews-frames.md.
// A play reads streams of frames, a few at a time.
//
// - A frame is decoded at the size of the canvas and no larger.
// - The frames sent last are kept, as memory to draw from again, and the
//   ones the Go side read around the place the hand rests on, so a step
//   with the arrow keys shows its frame with no request.
// - What to read while the hand rests, the words beside and the cards, is
//   handed to the Go side, which reads it when its decoder has nothing to
//   do for the hand and sends what it read, kept here too, see ahead. The
//   cards' frames are kept for as long as their cards are there.
// - The queue's second decoder asks for the piece after a cut while the
//   first plays, so a stream for it is open before it is needed.
import { rankOf, ticks, type VideoTrack } from "./movie";
import type { Picture } from "./picture";
import { pictures, pull, type Pulled } from "./pull";
import { frames as framePool } from "./pool";

// A pull with its frames as Pictures.
type Got = Omit<Pulled, "frames"> & { frames: { at: number; frame: Picture }[] };

// How much of the frames sent last is kept, and how far ahead a stream may
// be from a frame wanted of it and still be waited for, in seconds. Further
// than that, a new stream from the key frame before is quicker. The kept
// frames are counted, so the room follows the size of the frames: 40 are
// about 90 MB at 1012 by 570 and 190 MB at 1512 by 850, a big window's.
// With 96 MB at that size a word back sought again for frames it had let
// go of, plan row 2.269. Never more than 192 MB, so a big screen on a Mac
// with little memory keeps no more than that.
const KEPT_FRAMES = 40;
const KEPT_BYTES = 192 << 20;
const REACH = 1.5;
// How many streams are kept open: one for each of the queue's two
// decoders, and one more.
const STREAMS = 3;

// A frame asked for. gone is set when the one who asked no longer wants it,
// a decoder reset for a newer target, and a stream then neither pulls for
// it nor opens another stream for it.
// seq numbers the asks in the order they were made. place is set on the
// frame that holds the playhead, a place the hand put it, see frame, and
// moving where the hand moves over it. follows is set on the frames after
// a place the hand rests on, which a play from there shows first.
type Waiter = {
  rank: number;
  seq: number;
  done: (f: Picture | null) => void;
  gone?: boolean;
  place?: boolean;
  moving?: boolean;
  follows?: boolean;
};

// Where the time of a stream's first frame went, in milliseconds from when
// it was asked for, for a probe to read, see docs/VIDEO-PREVIEW.md, Speed:
// ms until the interface had it, and on the Go side until the decoder had
// the request, until it was at the place, until the first frame was out,
// and whether that meant opening the file or only moving a cursor. on says
// the cursor read on to the place from where it stood, without a seek.
export type Opened = { ms: number; started: number; opened: number; first: number; file: boolean; on: boolean };

class Stream {
  id: Promise<string | null>;
  // The rank of the last frame read, and the first this stream can give.
  position: number;
  readonly first: number;
  ended = false;
  // Whether it has given a frame yet.
  private gave = false;
  // When it was asked for, to count how long its first frame took.
  private asked = performance.now();
  used = performance.now();
  private waiting: Waiter[] = [];
  private pulling = false;
  // While a pull that skips is on its way, the rank it skips to: the Go
  // side lets every frame before it go unmade, see reaches.
  private skipping = -Infinity;

  constructor(
    private owner: AppFrames,
    first: number,
    readonly width: number,
    readonly height: number,
  ) {
    this.first = first;
    this.position = first - 1;
    // The frame's own moment: the Go side starts at the frame of the grid
    // that holds it, which is this one, see movie.ts.
    const from = owner.ptsOf(first);
    const q = new URLSearchParams({ path: owner.path, from: from.toFixed(6), w: String(width), h: String(height) });
    // The grid the page counts on goes with every open, and the Go side
    // makes the frames of that grid and no other, so the two never count
    // differently, even for a file written over while the page slept.
    const g = owner.track.grid;
    if (g) q.set("grid", owner.gridText);
    this.id = fetch(`/frames/open?${q}`)
      .then(async (r) => {
        if (r.ok) return r.json();
        owner.trace(`a stream from ${from.toFixed(3)} s could not be opened, it answered ${r.status}: ${(await r.text()).trim()}`);
        return null;
      })
      .then((j: { id?: string } | null) => j?.id ?? null)
      .catch((e: unknown) => {
        owner.trace(`a stream from ${from.toFixed(3)} s could not be opened: ${e instanceof Error ? e.message : String(e)}`);
        return null;
      });
  }

  // Whether this stream is the quickest way to the frame: it has not passed
  // it, is not too far behind it, and no pull on its way skips over it. A
  // frame skipped over never came, and once the pull was in it was handed
  // the frame before it, so the video preview showed that frame and said
  // it showed this one, issue #358.
  reaches(rank: number): boolean {
    if (this.ended || rank <= this.position || rank < this.first || rank < this.skipping) return false;
    return this.owner.ptsOf(rank) - this.owner.ptsOf(Math.max(this.position, this.first)) <= REACH;
  }

  want(w: Waiter) {
    this.used = performance.now();
    this.waiting.push(w);
    void this.pull();
  }

  private async pull() {
    if (this.pulling) return;
    this.pulling = true;
    try {
      const id = await this.id;
      if (!id) {
        this.end("the app could not decode the picture of this video");
        return;
      }
      for (;;) {
        for (const w of this.waiting) if (w.gone) w.done(null);
        this.waiting = this.waiting.filter((w) => !w.gone);
        if (!this.waiting.length || this.ended) return;
        // The frames wanted next, one run of them: a frame read ahead
        // further on is read with a skip of its own, after these, so the
        // frames between are never made into pictures.
        const ranks = this.waiting.map((w) => w.rank).sort((a, b) => a - b);
        const lowest = ranks[0];
        let highest = lowest;
        for (const r of ranks) if (r <= highest + 1) highest = r;
        const from = lowest;
        const skip = from > this.position + 1 ? this.owner.ptsOf(from) - this.owner.track.frame / 2 : 0;
        const n = Math.min(16, Math.max(1, highest - Math.max(this.position, from - 1)));
        const q = new URLSearchParams({ id, n: String(n) });
        if (skip > 0) q.set("skip", skip.toFixed(6));
        this.skipping = skip > 0 ? from : -Infinity;
        const got = await this.owner.puller.pull(`/frames/read?${q}`, this.width, this.height);
        this.skipping = -Infinity;
        // Closed here while the pull was on its way: what it waited for has
        // gone to another stream already.
        if (this.ended) {
          for (const { frame } of got.frames) frame.close();
          return;
        }
        if (this.over(got, id)) return;
        this.take(got);
      }
    } finally {
      this.pulling = false;
    }
  }

  // Whether a pull's answer ends this stream: closed or gone on the Go
  // side, not answered, or at the end of the file with nothing more. What
  // still waits then goes to another stream or comes as the nearest frame.
  private over(got: Got, id: string): boolean {
    if (got.status === 404 || got.closed) {
      this.owner.trace(`stream ${id.slice(0, 6)} was ${got.closed ? "closed" : "gone"} on the Go side, ${this.waiting.length} frames go to another`);
      // Closed on the Go side, to make room for newer streams or after
      // standing unused. That is no failure of the decoder: what was
      // waited for goes to a new stream.
      this.ended = true;
      for (const { frame } of got.frames) frame.close();
      const again = this.waiting;
      this.waiting = [];
      for (const w of again) this.owner.route(w);
      return true;
    }
    if (got.status === 0) {
      this.owner.trace(`stream ${id.slice(0, 6)}: the Go side did not answer a pull, ${this.gave ? "after frames" : "before any frame"}`);
      // The Go side could not be reached at all. A stream that never
      // gave a frame is no stream, and asking again would only ask again.
      if (!this.gave) {
        this.end("the app's decoder did not answer");
        return true;
      }
      this.ended = true;
      const again = this.waiting;
      this.waiting = [];
      for (const w of again) this.owner.route(w);
      return true;
    }
    if (got.end && got.frames.length === 0) {
      this.owner.trace(`stream ${id.slice(0, 6)} ended at frame ${this.position}${got.error ? `: ${got.error}` : ""}`);
      this.end(got.error);
      return true;
    }
    return false;
  }

  private take(got: Got) {
    if (!this.gave && got.frames.length) this.owner.opened(performance.now() - this.asked, got.times);
    for (const { at, frame } of got.frames) {
      const rank = rankOf(this.owner.track.samples, at);
      this.position = Math.max(this.position, rank);
      this.gave = true;
      this.owner.keep(rank, frame);
    }
    // Everything waited for that this stream has reached, as it came or as
    // the frame before it when the file has none there.
    const still: Waiter[] = [];
    for (const w of this.waiting) {
      if (w.rank <= this.position) w.done(this.owner.nearest(w.rank));
      else still.push(w);
    }
    this.waiting = still;
  }

  // How it stands, for the app's log: the frames it can give, its size,
  // and whether it ended or skips.
  get state(): string {
    return `${this.first} to ${this.position} at ${this.width}x${this.height}${this.ended ? " ended" : ""}${this.skipping > -Infinity ? ` skipping to ${this.skipping}` : ""}`;
  }

  private end(why: string) {
    this.ended = true;
    if (why) this.owner.failed(why);
    for (const w of this.waiting) w.done(this.owner.nearest(w.rank));
    this.waiting = [];
  }

  // Closed with frames still waited for: they go to another stream.
  close() {
    this.ended = true;
    const again = this.waiting;
    this.waiting = [];
    for (const w of again) this.owner.route(w);
    void this.id.then((id) => {
      if (id) void fetch(`/frames/close?id=${id}`).catch(() => {});
    });
  }
}

// The streams of one episode and the frames kept from them, shared by the
// queue's decoders.
export class AppFrames {
  private streams: Stream[] = [];
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
  // video preview stands still again, see hold, and whether it has asked
  // for a frame since.
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
  // The looks on their way, by the rank they ask for, see look.
  private asking = new Map<number, Promise<unknown>>();
  private nearNext = -1;
  // first is the size the first frame was asked at, for the walks.
  // keptRanks are the ranks of the frames last handed over from the kept
  // ones, here or on the Go side, for the walks. ahead counts the frames
  // handed to the Go side to read ahead. looks counts the looks sent, near the frames the Go side sent on the
  // way to a place, overtaken the looks it answered with nothing, a newer
  // look having taken their place, and fromGo every frame the Go side
  // sent, by a look or a stream.
  readonly stats = {
    streams: 0,
    continued: 0,
    kept: 0,
    keptRanks: [] as number[],
    looks: 0,
    near: 0,
    overtaken: 0,
    fromGo: 0,
    ahead: 0,
    opens: [] as Opened[],
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
  ) {
    const g = track.grid;
    this.gridText = g ? [g.origin, g.start, g.num, g.den, g.count].join(",") : "";
    void fetch(`/frames/hold?${new URLSearchParams({ path })}`).catch(() => {});
  }

  private closed = false;

  // A stream gave its first frame: how long that took here, from asking to
  // having it, and on the Go side, see Opened.
  opened(ms: number, times: string): Opened {
    const [started, open, first, file, on] = times.split(",").map(Number);
    const o = { ms: Math.round(ms), started: started || 0, opened: open || 0, first: first || 0, file: file === 1, on: on === 1 };
    this.stats.opens.push(o);
    if (this.stats.opens.length > 64) this.stats.opens.shift();
    return o;
  }

  ptsOf(rank: number): number {
    const s = this.track.samples;
    return ticks(s, Math.max(0, Math.min(rank, s.count - 1))) / s.timescale;
  }

  // The hand takes hold of the playhead, or lets go of it, see
  // FrameQueue.grip. Nothing is read ahead while it holds it, see ahead.
  grip(held: boolean) {
    this.dragging = held;
    this.dragPlaces = 0;
    this.aheadNow();
  }

  // The frame of this rank, as a new frame with this timestamp, once it is
  // there. Null only when nothing near it could be decoded, or when a
  // newer look took its place.
  // withdrawn, when it is set to true before the frame came, makes it
  // come as null and stops it holding a stream.
  //
  // place says the frame holds the playhead, the frame of a still or the
  // first of a cue, and follows that it is one of the frames after such a
  // place, which a play from there shows first. While the video preview
  // stands still, each is a look, see look: the Go side decides how the
  // decoder gets there.
  frame(rank: number, timestamp: number, asked?: Set<Waiter>, place = false, follows = false): Promise<Picture | null> {
    const still = this.paused();
    if (!still) this.played = true;
    const dragged = place && this.dragging;
    return new Promise((done) => {
      const w: Waiter = {
        rank,
        seq: ++this.asked,
        place: place && still,
        follows: follows && still && !place,
        // Every place but the first of a hold, which is where the press
        // landed: a click moves over nothing.
        moving: still && dragged && ++this.dragPlaces > 1,
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
    if (w.follows) q.set("follow", "1");
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
      if (got.near) this.near(rank);
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
    const has = this.keptHere(width, height).filter((r) => r >= rank - 16 && r <= rank + 8);
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
      // The kept frame stays the most recently used.
      this.kept.delete(w.rank);
      this.kept.set(w.rank, have);
      w.done(have);
      return;
    }
    const card = this.cardKept.get(w.rank);
    if (card && card.width === width && card.height === height) {
      this.tookKept(w.rank);
      w.done(card);
      return;
    }
    if (w.place || w.follows) {
      // Where the hand is, and what a play from there shows first: the Go
      // side's to decide, see look.
      void this.look(w);
      return;
    }
    let s = this.streams.find((x) => x.width === width && x.height === height && x.reaches(w.rank));
    if (s) this.stats.continued++;
    else {
      s = new Stream(this, w.rank, width, height);
      this.trace(`a new stream for frame ${w.rank} at ${width}x${height}, beside ${this.streams.filter((x) => x !== s).map((x) => x.state).join(", ") || "none"}`);
      this.stats.first ||= `${width}x${height}`;
      this.stats.streams++;
      this.streams.push(s);
      this.streams = this.streams.filter((x) => !x.ended);
      while (this.streams.length > STREAMS) {
        const oldest = this.streams.reduce((a, b) => (a.used <= b.used ? a : b));
        this.trace(`a stream at frame ${oldest.position} is closed to make room`);
        oldest.close();
        this.streams = this.streams.filter((x) => x !== oldest);
      }
    }
    s.want(w);
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
    for (const s of this.streams) s.close();
    this.streams = [];
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
      for (const done of this.waiting.values()) done({ status: 0, end: false, error: "", closed: false, times: "", light: "", exact: false, kept: false, near: false, frames: [], spare: [] });
      this.waiting.clear();
    };
    this.worker = w;
    this.where = "worker";
  }

  async pull(url: string, width: number, height: number): Promise<Got> {
    const got = await this.pulled(url, width, height);
    for (const b of got.spare ?? []) framePool.give(b);
    return { ...got, frames: pictures(got, width, height) };
  }

  // A pull is handed as many kept buffers as frames it asks for, see
  // pool.ts.
  private async pulled(url: string, width: number, height: number): Promise<Pulled> {
    await this.ready;
    const asked = Number(new URL(url, location.href).searchParams.get("n") ?? 1) || 1;
    const spare = framePool.takeUpTo(width * height * 4, asked);
    const w = this.worker;
    if (!w) return pull(url, width, height, spare);
    const n = this.count++;
    return new Promise((done) => {
      this.waiting.set(n, done);
      w.postMessage({ n, url: new URL(url, location.href).href, width, height, spare }, spare);
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

// A picture decoder for the frame queue, with the calls it makes of a
// VideoDecoder, whose frames come from AppFrames. Only the frames the
// queue will draw are asked for: the ones fed before them, from the key
// frame on, are what a decoder needs and the Go side decodes for itself.
// A run is fed in the order it is decoded, where a frame shown later can
// come before one shown earlier, and like a decoder it puts the frames out
// in the order they are shown, from the first the run draws.
export class AppPictures {
  state: "configured" | "closed" = "configured";
  // Asked for and not come yet.
  private asked = 0;
  // Come, and waiting for the ones shown before them.
  private held = new Map<number, Picture | null>();
  // The rank to put out next, or -1 before the first frame of a run.
  private next = -1;
  private generation = 0;
  private flushes: { done: () => void; fail: (e: Error) => void }[] = [];
  // The frames asked of AppFrames and not come yet, withdrawn on a reset.
  private waiting = new Set<Waiter>();
  // The run began with a place, the frame under the playhead: the frames
  // fed after it are the ones a play from there shows first.
  private placed = false;

  constructor(
    private frames: AppFrames,
    private output: (f: Picture) => void,
    private dequeue: () => void,
  ) {}

  get decodeQueueSize() {
    return this.asked;
  }

  decode(timestamp: number, _key: boolean, _data: Uint8Array, rank: number, wanted: boolean, first: number, place = false) {
    if (this.state !== "configured" || !wanted) return;
    if (this.next < 0) this.next = first;
    this.asked++;
    const generation = this.generation;
    const follows = this.placed && !place;
    if (place) this.placed = true;
    void this.frames.frame(rank, timestamp, this.waiting, place, follows).then((f) => {
      if (generation !== this.generation || this.state !== "configured") {
        f?.close();
        return;
      }
      this.asked--;
      this.held.set(rank, f);
      this.putOut(false);
      this.dequeue();
      this.settle();
    });
  }

  // Puts out the frames that are next in the order they are shown, or with
  // all, everything held, the way a flush makes a decoder do.
  private putOut(all: boolean) {
    if (all) {
      for (const r of [...this.held.keys()].sort((a, b) => a - b)) {
        const f = this.held.get(r);
        this.held.delete(r);
        if (f) this.output(f);
      }
      return;
    }
    while (this.held.has(this.next)) {
      const f = this.held.get(this.next);
      this.held.delete(this.next);
      this.next++;
      if (f) this.output(f);
    }
  }

  flush(): Promise<void> {
    return new Promise((done, fail) => {
      this.flushes.push({ done, fail });
      this.settle();
    });
  }

  // A flushed decoder starts again with the next run it is fed, from that
  // run's first frame, the way a VideoDecoder after a flush does. Kept on
  // from the run before, a run that came after it on the same decoder
  // with lower ranks, the clip's start the next time through a loop, was
  // held back for good waiting for a rank after the last run's end. The
  // sound went round and the video preview stood on the clip's last
  // frame, found by Tim (2.264).
  private settle() {
    if (this.asked > 0 || !this.flushes.length) return;
    this.putOut(true);
    this.next = -1;
    this.placed = false;
    const was = this.flushes;
    this.flushes = [];
    for (const f of was) f.done();
  }

  reset() {
    this.generation++;
    // What was asked for an older target no longer holds a stream, nor
    // opens one: a drag asks for many frames and wants only the last.
    for (const w of this.waiting) w.gone = true;
    this.waiting.clear();
    this.asked = 0;
    this.next = -1;
    this.placed = false;
    for (const f of this.held.values()) f?.close();
    this.held.clear();
    const was = this.flushes;
    this.flushes = [];
    for (const f of was) f.fail(new DOMException("reset", "AbortError"));
  }

  close() {
    this.reset();
    this.state = "closed";
  }
}
