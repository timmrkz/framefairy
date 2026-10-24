// The picture of the episode, decoded on the Go side by the episode's
// decoder and pulled from it a few frames at a time, see
// cmd/framefairy-app/frames.go: the frames of the short, on the grid the
// engine names, see movie.ts. AppPictures answers the calls the frame
// queue makes of a decoder.
//
// What makes it quick, since every new stream costs moving a cursor of
// the episode's decoder and decoding from the key frame before:
//
// - A stream that will reach a frame soon is waited for rather than a new
//   one started, so the arrow key stepping on, and a paused play going on,
//   cost one frame each. The frames on the way are dropped on the Go side
//   without being sent.
// - The frames shown last are kept, so stepping back or a still close by
//   costs nothing.
// - A frame is decoded at the size of the canvas and no larger.
// - The queue's second decoder asks for the piece after a cut while the
//   first plays, so a stream for it is open before it is needed.
import { rankOf, type VideoTrack } from "./movie";
import type { Picture } from "./picture";
import { pictures, pull, type Pulled } from "./pull";
import { frames as framePool } from "./pool";

// A pull with its frames as Pictures.
type Got = Omit<Pulled, "frames"> & { frames: { at: number; frame: Picture }[] };

// How much of the frames shown last is kept, and how far ahead a stream may
// be from a frame wanted of it and still be waited for, in seconds. Further
// than that, a new stream from the key frame before is quicker. The kept
// frames are counted, so the room follows the size of the frames: 40 are
// about 90 MB at 1012 by 570 and 190 MB at 1512 by 850, a big window's.
// With 96 MB at that size a word back sought again for frames it had let
// go of, plan row 2.269. Never more than 192 MB, so a big screen on a Mac
// with little memory keeps no more than that.
const KEPT_FRAMES = 40;
const KEPT_BYTES = 192 << 20;
// How much the frames of the clips on the cards may keep besides: a frame
// is its pixels at the size frames are read at, 4 bytes each, about 8 MB
// at the size of Tim's video preview, so about 48 clips. The likeliest
// cards are kept first, and the count is made again when that size
// changes, see cards.
const CARDS_BYTES = 400 << 20;
const REACH = 1.5;
// How many streams are kept open: one for each of the queue's two
// decoders, and one more for a still or a seek while they play.
const STREAMS = 3;

// A frame asked for. gone is set when the one who asked no longer wants it,
// a decoder reset for a newer target, and a stream then neither pulls for
// it nor opens another stream for it.
// seq numbers the asks in the order they were made. place is set on the
// frame that holds the playhead, a place the hand put it, see frame.
// ahead is set on a frame read ahead of the hand, see AppFrames.ahead.
type Waiter = { rank: number; seq: number; done: (f: Picture | null) => void; gone?: boolean; place?: boolean; ahead?: boolean; key?: boolean };

// Where the time of a stream's first frame went, in milliseconds from when
// it was asked for, for a probe to read, see docs/VIDEO-PREVIEW.md, Speed:
// ms until the interface had it, and on the Go side until the decoder had
// the request, until it was at the place, until the first frame was out,
// and whether that meant opening the file or only moving a cursor. key
// says the first frame was the key frame handed over first, issue #281,
// so the frame asked for came later than these times say.
export type Opened = { ms: number; started: number; opened: number; first: number; file: boolean; key: boolean };

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
  // The newest ask for a place it was opened for, its first frame, or 0.
  private placed = 0;
  // Opened for a frame read ahead of the hand, see AppFrames.ahead.
  aheadOnly = false;

  // With key, the stream hands over the key frame its decoding starts
  // from first, and then goes on to first, see AppFrames.route.
  constructor(
    private owner: AppFrames,
    first: number,
    readonly width: number,
    readonly height: number,
    readonly key = false,
  ) {
    this.first = first;
    // The key frame comes before first, at a rank only the Go side knows.
    this.position = key ? -1 : first - 1;
    // The frame's own moment: the Go side starts at the frame of the grid
    // that holds it, which is this one, see movie.ts.
    const from = owner.ptsOf(first);
    const q = new URLSearchParams({ path: owner.path, from: from.toFixed(6), w: String(width), h: String(height) });
    // The grid the page counts on goes with every open, and the Go side
    // makes the frames of that grid and no other, so the two never count
    // differently, even for a file written over while the page slept.
    const g = owner.track.grid;
    if (g) q.set("grid", [g.origin, g.start, g.num, g.den, g.count].join(","));
    if (key) q.set("key", "1");
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

  // Still on its way to its first frame for a place nobody wants any more:
  // a drag went on past it.
  get stale(): boolean {
    // A stream opened ahead of the hand holds no place back: the hand's
    // own frame never waits for one.
    return !this.aheadOnly && !this.ended && !this.gave && this.waiting.every((w) => w.gone);
  }

  // Whether this stream is the quickest way to the frame: it has not passed
  // it and is not too far behind it.
  reaches(rank: number): boolean {
    if (this.ended || rank <= this.position || rank < this.first) return false;
    return this.owner.ptsOf(rank) - this.owner.ptsOf(Math.max(this.position, this.first)) <= REACH;
  }

  want(w: Waiter) {
    this.used = performance.now();
    if (!w.ahead) this.aheadOnly = false;
    if (w.place && w.rank === this.first) this.placed = Math.max(this.placed, w.seq);
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
        if (!this.waiting.length || this.ended) {
          // Opened for a place the hand has left: once its first frame
          // comes it lets the newest place go, see unpark.
          if (!this.ended && !this.gave) await this.firstFrame(id);
          return;
        }
        // The frames wanted next, one run of them: a frame read ahead
        // further on is read with a skip of its own, after these, so the
        // frames between are never made into pictures.
        const ranks = this.waiting.map((w) => w.rank).sort((a, b) => a - b);
        const lowest = ranks[0];
        let highest = lowest;
        for (const r of ranks) if (r <= highest + 1) highest = r;
        // The key frame first, on its own, see key.
        const keyNow = this.key && !this.gave;
        // Frames before the first one wanted are decoded on the Go side
        // and never made into pictures, but for the one just before it,
        // which comes too and is kept: the arrow to the left after a jump
        // asks for it next, and it would cost a seek back to the key
        // frame. Plan row 2.269.
        const from = lowest > this.position + 2 ? lowest - 1 : lowest;
        const skip = from > this.position + 1 && !keyNow ? this.owner.ptsOf(from) - this.owner.track.frame / 2 : 0;
        const n = keyNow ? 1 : Math.min(16, Math.max(1, highest - Math.max(this.position, from - 1)));
        const q = new URLSearchParams({ id, n: String(n) });
        if (skip > 0) q.set("skip", skip.toFixed(6));
        const got = await this.owner.puller.pull(`/frames/read?${q}`, this.width, this.height);
        // Closed here while the pull was on its way: what it waited for has
        // gone to another stream already.
        if (this.ended) {
          for (const { frame } of got.frames) frame.close();
          return;
        }
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
          return;
        }
        if (got.status === 0) {
          this.owner.trace(`stream ${id.slice(0, 6)}: the Go side did not answer a pull, ${this.gave ? "after frames" : "before any frame"}`);
          // The Go side could not be reached at all. A stream that never
          // gave a frame is no stream, and asking again would only ask again.
          if (!this.gave) {
            this.end("the app's decoder did not answer");
            return;
          }
          this.ended = true;
          const again = this.waiting;
          this.waiting = [];
          for (const w of again) this.owner.route(w);
          return;
        }
        if (got.end && got.frames.length === 0) {
          this.owner.trace(`stream ${id.slice(0, 6)} ended at frame ${this.position}${got.error ? `: ${got.error}` : ""}`);
          this.end(got.error);
          return;
        }
        this.take(got);
        if (keyNow) this.keyCame(got);
      }
    } finally {
      this.pulling = false;
    }
  }

  // The key frame came, before the frame the hand asked for: it may be put
  // up while that one is decoded, whether the hand is still there or not,
  // see onPassing.
  private keyCame(got: Got) {
    const last = got.frames[got.frames.length - 1];
    if (!last || !this.placed) return;
    const rank = rankOf(this.owner.track.samples, last.at);
    if (rank < this.first) this.owner.onPassing?.(rank, last.frame, this.placed, true);
  }

  // The first frame of a stream nobody waits on any more, read so that the
  // stream counts as come and the newest place asked for can go next. It
  // is put up as a place the hand passed only when it is one, see frame.
  private async firstFrame(id: string) {
    const q = new URLSearchParams({ id, n: "1" });
    const got = await this.owner.puller.pull(`/frames/read?${q}`, this.width, this.height);
    if (this.ended) {
      for (const { frame } of got.frames) frame.close();
      return;
    }
    if (got.frames.length) {
      this.take(got);
      const { at, frame } = got.frames[got.frames.length - 1];
      const rank = rankOf(this.owner.track.samples, at);
      if (this.placed) this.owner.onPassing?.(rank, frame, this.placed, this.key && rank < this.first);
    } else this.end("");
  }

  private take(got: Got) {
    if (!this.gave && got.frames.length) {
      this.owner.opened(performance.now() - this.asked, got.times, this.key);
      queueMicrotask(() => this.owner.unpark());
    }
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
    if (!this.handWaits) this.owner.aheadNow();
  }

  // Whether a frame the hand asked for is still to come from this stream.
  get handWaits(): boolean {
    return !this.ended && this.waiting.some((w) => !w.ahead && !w.gone);
  }

  private end(why: string) {
    this.ended = true;
    queueMicrotask(() => this.owner.unpark());
    if (why) this.owner.failed(why);
    for (const w of this.waiting) w.done(this.owner.nearest(w.rank));
    this.waiting = [];
  }

  // Closed with frames still waited for: they go to another stream.
  close() {
    this.ended = true;
    queueMicrotask(() => this.owner.unpark());
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
  // Told of a frame that came for an ask withdrawn on the way, a place a
  // drag passed, so the queue can put it up while the frame for where the
  // hand is now is on its way. seq is the ask's. key says it is the key
  // frame decoding starts from, on the way to the frame asked for. The
  // frame stays the kept one's.
  onPassing?: (rank: number, frame: Picture, seq: number, key?: boolean) => void;
  // Whether the hand holds the playhead, see grip, and the places it
  // asked for since it took hold.
  private dragging = false;
  private dragPlaces = 0;
  // The number of the newest ask.
  asked = 0;
  // The frames read ahead of the hand, withdrawn by the hand's next ask.
  private aheadOf = new Set<Waiter>();
  // The ranks to read ahead once the hand waits for nothing, see ahead.
  private aheadWanted: number[] | null = null;
  // The ranks read ahead and not yet asked for by the hand, and those of
  // them the kept frames let go of first, for aheadLost.
  private aheadRead = new Set<number>();
  private aheadEvicted = new Set<number>();
  // The cards' frames, by rank, in the order they are read, those of them
  // CARDS_BYTES holds at the size frames are read at, which are kept
  // whatever else comes, and those asked for already, see cards.
  private cardOrder: number[] = [];
  private cardRanks = new Set<number>();
  private cardTried = new Set<number>();
  private cardSize = "";
  // Whether the video preview stands still. A card's frame read during a
  // play is a second decoder seeking in the file the play decodes, so the
  // cards wait for the pause, the way their captions do, see
  // CaptionLayers.more.
  paused: () => boolean = () => true;
  // What the cards' frames may keep, a field for the tests.
  cardsBytes = CARDS_BYTES;
  // first is the size the first stream was opened at, for the walks.
  // keptRanks are the ranks of the frames last handed over from the kept
  // ones, for the walks. aheadLost counts the frames the hand asked for
  // that were read ahead and let go of from the kept ones before it did.
  // keyFirst counts the streams opened with their key frame first.
  readonly stats = {
    streams: 0,
    continued: 0,
    kept: 0,
    keptRanks: [] as number[],
    parked: 0,
    ahead: 0,
    aheadLost: 0,
    keyFirst: 0,
    opens: [] as Opened[],
    first: "",
  };
  // Every step on the way to a frame, for the app's log, see lib/said.ts.
  trace: (line: string) => void = () => {};
  readonly puller = new Puller((line) => this.trace(line));

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
    void fetch(`/frames/hold?${new URLSearchParams({ path })}`).catch(() => {});
  }

  // The newest place asked for while a stream was on its way to a place
  // left behind, see route.
  // Those a reset has withdrawn since are let go as nothing.
  private parked: Waiter[] = [];
  private closed = false;

  unpark() {
    const was = this.parked;
    this.parked = [];
    for (const w of was) this.route(w);
    this.aheadNow();
  }

  // A stream gave its first frame: how long that took here, from asking to
  // having it, and on the Go side, see Opened.
  opened(ms: number, times: string, key = false) {
    const [started, open, first, file] = times.split(",").map(Number);
    this.stats.opens.push({ ms: Math.round(ms), started: started || 0, opened: open || 0, first: first || 0, file: file === 1, key });
    if (this.stats.opens.length > 64) this.stats.opens.shift();
  }

  ptsOf(rank: number): number {
    const s = this.track.samples;
    return s.pts[s.order[Math.max(0, Math.min(rank, s.count - 1))]] / s.timescale;
  }

  // The frame of this rank, as a new frame with this timestamp, once it is
  // there. Null only when nothing near it could be decoded.
  // withdrawn, when it is set to true before the frame came, makes it
  // come as null and stops it holding a stream.
  //
  // place says the frame holds the playhead, the frame of a still or the
  // first of a cue, and only such a frame withdrawn on the way is put up
  // as a place the hand passed, see onPassing. The frames a cue reads
  // ahead of it are not: the play from the last frame before a cut reads
  // the frames after the cut, every step of a drag of the cut's edge
  // withdrew them, and they were put up between the edge's frames, so the
  // video preview flickered between the two. Found by Tim on youtube.mp4,
  // plan row 2.270.
  // The hand takes hold of the playhead, or lets go of it, see
  // FrameQueue.grip.
  grip(held: boolean) {
    this.dragging = held;
    this.dragPlaces = 0;
  }

  frame(rank: number, timestamp: number, asked?: Set<Waiter>, place = false): Promise<Picture | null> {
    return new Promise((done) => {
      const w: Waiter = {
        rank,
        seq: ++this.asked,
        place,
        // Every place but the first of a hold, which is where the press
        // landed: a click outruns nothing, see route.
        key: place && this.dragging && ++this.dragPlaces > 1,
        done: (f) => {
          asked?.delete(w);
          if (f && w.gone && w.place) this.onPassing?.(rank, f, w.seq);
          done(f && !w.gone ? f.clone(timestamp) : null);
        },
      };
      asked?.add(w);
      this.route(w);
    });
  }

  // Reads the frames of these ranks while the hand rests, and keeps them,
  // so that a step to one of them finds its frame there: the first frame
  // of the word after the playhead and of the word before, the words
  // Shift and an arrow walk to. They come after what the hand asked for
  // in the stream that reads on to them, and a stream opened for one never
  // holds up a place the hand asks for. Any newer ask of the hand
  // withdraws them, see still. Plan row 2.269.
  //
  // They are asked only once nothing the hand asked for is still to come.
  // Asked at once, the frame under the hand came 80 to 120 ms later when a
  // drag of the playhead let go, measured with speed.mjs: the frames read
  // ahead competed with it for the decoder and for the streams.
  ahead(ranks: number[]) {
    this.still();
    this.aheadWanted = [...ranks];
    this.aheadNow();
  }

  // The first frames of the clips on the cards, by rank, the likeliest to
  // be clicked first: each is read once nothing else is wanted, after the
  // frames the hand asked for and those of the words either side of the
  // playhead, and kept for as long as its card is there. Plays and steps
  // never push one out, see keep. Each call replaces the ones before, and
  // the frames of cards no longer given are kept no longer. Tim asked for
  // every clip on the cards to be ready to show, issue #383.
  //
  // A frame that could not be decoded is not tried again for a new order
  // alone: the order changes with every card the pointer passes.
  cards(ranks: number[]) {
    const order = ranks.filter((r) => r >= 0);
    const was = new Set(this.cardOrder);
    if (order.length !== was.size || order.some((r) => !was.has(r))) this.cardTried.clear();
    this.cardOrder = order;
    this.cardSize = "";
    this.fitCards();
    this.aheadNow();
  }

  // The cards CARDS_BYTES holds at the size frames are read at now, the
  // likeliest first. A size of its own tries again the frames that could
  // not be decoded at the one before.
  private fitCards() {
    const { width, height } = this.size();
    const size = `${width}x${height}`;
    if (size === this.cardSize) return;
    if (this.cardSize) this.cardTried.clear();
    this.cardSize = size;
    this.cardRanks = new Set(this.cardOrder.slice(0, Math.floor(this.cardsBytes / Math.max(1, width * height * 4))));
  }

  // Asks the frames read ahead once the hand waits for nothing, one at a
  // time, in the order given, and then the cards' frames, one at a time.
  // They were asked all at once, and with the first frames of the clips
  // next to the one chosen among them, a click on a card waited for its
  // frame behind three seeks of the decoder and streams closed and opened
  // again for the rest, issue #383. The cards wait while it plays.
  aheadNow() {
    this.readNext();
    this.settleRest();
  }

  private readNext() {
    if (this.aheadOf.size || this.closed || this.parked.length || this.streams.some((s) => s.handWaits)) return;
    const ranks = this.aheadWanted ?? [];
    for (let rank = ranks.shift(); rank !== undefined; rank = ranks.shift()) {
      if (rank < 0 || this.has(rank)) continue;
      this.read(rank, false);
      return;
    }
    this.aheadWanted = null;
    if (!this.paused()) return;
    this.fitCards();
    const card = this.cardOrder.find((r) => this.cardRanks.has(r) && !this.has(r) && !this.cardTried.has(r));
    if (card === undefined) return;
    this.cardTried.add(card);
    this.read(card, true);
  }

  // Whether the frame of a rank is kept at the size frames are read at.
  private has(rank: number): boolean {
    const f = this.kept.get(rank);
    const { width, height } = this.size();
    return !!f && f.width === width && f.height === height;
  }

  // Reads one frame ahead of the hand: a card's, which is tried once
  // unless the hand withdrew it on the way, or another.
  private read(rank: number, card: boolean) {
    const w: Waiter = {
      rank,
      seq: ++this.asked,
      ahead: true,
      done: () => {
        this.aheadOf.delete(w);
        if (card && w.gone) this.cardTried.delete(rank);
        queueMicrotask(() => this.aheadNow());
      },
    };
    this.aheadOf.add(w);
    this.aheadRead.add(rank);
    if (this.aheadRead.size > 256) this.aheadRead.delete(this.aheadRead.values().next().value!);
    this.stats.ahead++;
    this.route(w);
  }

  // Settles once nothing is left to read ahead, the cards' frames among
  // them, for the walks, which wait on it rather than on the clock.
  rested(): Promise<void> {
    return new Promise((done) => {
      this.restWaiters.push(done);
      this.settleRest();
    });
  }
  private restWaiters: (() => void)[] = [];

  private settleRest() {
    if (this.readingAhead && !this.closed) return;
    const was = this.restWaiters;
    this.restWaiters = [];
    for (const done of was) done();
  }

  // Whether frames are still to be read ahead, the cards' among them.
  get readingAhead(): boolean {
    this.fitCards();
    return (
      this.aheadOf.size > 0 ||
      !!this.aheadWanted?.length ||
      this.cardOrder.some((r) => this.cardRanks.has(r) && !this.has(r) && !this.cardTried.has(r))
    );
  }

  // The hand asks again: what was read ahead for where it was is not
  // wanted any more.
  still() {
    this.aheadWanted = null;
    for (const w of this.aheadOf) w.gone = true;
    this.aheadOf.clear();
    this.settleRest();
  }

  route(w: Waiter) {
    if (w.gone || this.closed) {
      w.done(null);
      return;
    }
    if (!w.ahead) {
      if (this.aheadEvicted.delete(w.rank)) this.stats.aheadLost++;
      this.aheadRead.delete(w.rank);
    }
    const { width, height } = this.size();
    // A kept frame of another size is not this frame: one decoded before
    // the canvas had its size, or before the app grew, stays as blurred as
    // it was.
    const have = this.kept.get(w.rank);
    if (have && have.width === width && have.height === height) {
      this.stats.kept++;
      this.stats.keptRanks.push(w.rank);
      if (this.stats.keptRanks.length > 16) this.stats.keptRanks.shift();
      // The kept frame stays the most recently used.
      this.kept.delete(w.rank);
      this.kept.set(w.rank, have);
      w.done(have);
      return;
    }
    let s = this.streams.find((x) => x.width === width && x.height === height && x.reaches(w.rank));
    if (s) this.stats.continued++;
    else if (this.streams.some((x) => x.stale)) {
      // One new stream on its way at a time. While one is still coming for
      // a place the hand has already left, a drag over the clip timeline
      // would start an ffmpeg for every place it passes, more than anyone
      // can watch and more than the machine can decode. The newest place
      // waits instead, and goes as soon as that stream has come, so frames
      // follow the hand as fast as they can be made and no faster.
      this.parked.push(w);
      this.stats.parked++;
      return;
    } else {
      // A place the hand drags the playhead to is opened with its key frame
      // first, every place but the one the press landed on, see frame. The
      // key frame takes its own decoding and that of the few frames stored
      // next to it, where the frame under the hand takes every frame from
      // the key frame to it, up to
      // two seconds of them, which in a 4K video is a quarter of a second
      // of decoding or more, so while the hand moves faster than the
      // frames under it can be decoded the picture keeps up with key
      // frames, the way established players scrub. A stream left behind
      // by the hand costs its key frame only, and lets the newest place go
      // the moment it came, see unpark. Where the hand rests, the frame
      // under it follows from the same stream. Issue #281.
      const key = !!w.key && !w.ahead;
      s = new Stream(this, w.rank, width, height, key);
      if (key) this.stats.keyFirst++;
      s.aheadOnly = !!w.ahead;
      this.stats.first ||= `${width}x${height}`;
      this.stats.streams++;
      this.streams.push(s);
      this.streams = this.streams.filter((x) => !x.ended);
      while (this.streams.length > STREAMS) {
        // A stream opened only for frames read ahead goes first: the
        // others are a play's or the hand's.
        const ahead = this.streams.filter((x) => x.aheadOnly && x !== s);
        const oldest = (ahead.length ? ahead : this.streams).reduce((a, b) => (a.used <= b.used ? a : b));
        oldest.close();
        this.streams = this.streams.filter((x) => x !== oldest);
      }
    }
    s.want(w);
  }

  keep(rank: number, frame: Picture) {
    const old = this.kept.get(rank);
    if (old) {
      this.keptBytes -= bytes(old);
      old.close();
      this.kept.delete(rank);
    }
    this.kept.set(rank, frame);
    this.keptBytes += bytes(frame);
    // The cards' frames are kept apart from the count, and never let go of
    // for a newer frame: their number is held to CARDS_BYTES, see cards.
    this.fitCards();
    let free = this.keptBytes;
    let count = this.kept.size;
    for (const [r, f] of this.kept)
      if (this.cardRanks.has(r)) {
        free -= bytes(f);
        count--;
      }
    for (const [r, f] of this.kept) {
      if (free <= KEPT_BYTES && count <= KEPT_FRAMES) break;
      if (this.cardRanks.has(r)) continue;
      this.kept.delete(r);
      this.keptBytes -= bytes(f);
      free -= bytes(f);
      count--;
      f.close();
      if (this.aheadRead.delete(r)) {
        this.aheadEvicted.add(r);
        if (this.aheadEvicted.size > 256) this.aheadEvicted.delete(this.aheadEvicted.values().next().value!);
      }
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
    for (const w of this.parked) w.done(null);
    this.parked = [];
    this.puller.close();
    for (const s of this.streams) s.close();
    this.streams = [];
    for (const f of this.kept.values()) f.close();
    this.kept.clear();
    this.keptBytes = 0;
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
      for (const done of this.waiting.values()) done({ status: 0, end: false, error: "", closed: false, times: "", light: "", frames: [], spare: [] });
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
    void this.frames.frame(rank, timestamp, this.waiting, place).then((f) => {
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
