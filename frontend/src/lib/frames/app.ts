// The picture of a file the webview cannot decode, decoded on the Go side
// and pulled from it a few frames at a time, see
// cmd/framefairy-app/frames.go. WebKit says yes to HEVC with 10-bit colour
// and its decoder then fails on the first frame. The frame queue plays
// these frames the way it plays the ones WebKit decodes: AppPictures
// answers the calls it makes of a VideoDecoder.
//
// What makes it quick, since every new stream costs ffmpeg starting,
// reading the file's index and decoding from the key frame before:
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
import { rankOf, type VideoTrack } from "./mp4";
import type { Picture } from "./picture";
import { pictures, pull, type Pulled } from "./pull";

// A pull with its frames as Pictures.
type Got = Omit<Pulled, "frames"> & { frames: { at: number; frame: Picture }[] };

// How much of the frames shown last is kept, and how far ahead a stream may
// be from a frame wanted of it and still be waited for, in seconds. Further
// than that, a new stream from the key frame before is quicker.
const KEPT_BYTES = 96 << 20;
const REACH = 1.5;
// How many streams are kept open: one for each of the queue's two
// decoders, and one more for a still or a seek while they play.
const STREAMS = 3;

// A frame asked for. gone is set when the one who asked no longer wants it,
// a decoder reset for a newer target, and a stream then neither pulls for
// it nor opens another stream for it.
// seq numbers the asks in the order they were made.
type Waiter = { rank: number; seq: number; done: (f: Picture | null) => void; gone?: boolean };

// Where the time of a stream's first frame went, in milliseconds from when
// it was asked for, for a probe to read, see docs/VIDEO-PREVIEW.md, Speed:
// ms until the interface had it, and on the Go side until the decoder had
// the request, until it was at the place, until the first frame was out,
// and whether that meant opening the file or only moving a cursor.
export type Opened = { ms: number; started: number; opened: number; first: number; file: boolean };

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
  // The newest ask it was given.
  private newest = 0;

  constructor(
    private owner: AppFrames,
    first: number,
    readonly width: number,
    readonly height: number,
  ) {
    this.first = first;
    this.position = first - 1;
    const from = Math.max(0, owner.ptsOf(first) - owner.track.frame / 2);
    const q = new URLSearchParams({ path: owner.path, from: from.toFixed(6), w: String(width), h: String(height) });
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
    return !this.ended && !this.gave && this.waiting.every((w) => w.gone);
  }

  // Whether this stream is the quickest way to the frame: it has not passed
  // it and is not too far behind it.
  reaches(rank: number): boolean {
    if (this.ended || rank <= this.position || rank < this.first) return false;
    return this.owner.ptsOf(rank) - this.owner.ptsOf(Math.max(this.position, this.first)) <= REACH;
  }

  want(w: Waiter) {
    this.used = performance.now();
    this.newest = Math.max(this.newest, w.seq);
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
        const lowest = Math.min(...this.waiting.map((w) => w.rank));
        const highest = Math.max(...this.waiting.map((w) => w.rank));
        // Frames before the first one wanted are decoded on the Go side
        // and dropped there.
        const skip = lowest > this.position + 1 ? this.owner.ptsOf(lowest) - this.owner.track.frame / 2 : 0;
        const n = Math.min(16, Math.max(1, highest - Math.max(this.position, lowest - 1)));
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
      }
    } finally {
      this.pulling = false;
    }
  }

  // The first frame of a stream nobody waits on any more, read so that the
  // stream counts as come and the newest place asked for can go next.
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
      this.owner.onPassing?.(rankOf(this.owner.track.samples, at), frame, this.newest);
    } else this.end("");
  }

  private take(got: Got) {
    if (!this.gave && got.frames.length) {
      this.owner.opened(performance.now() - this.asked, got.times);
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
  // hand is now is on its way. seq is the ask's. The frame stays the kept
  // one's.
  onPassing?: (rank: number, frame: Picture, seq: number) => void;
  // The number of the newest ask.
  asked = 0;
  // first is the size the first stream was opened at, for the walks.
  readonly stats = { streams: 0, continued: 0, kept: 0, parked: 0, opens: [] as Opened[], first: "" };
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
  }

  // A stream gave its first frame: how long that took here, from asking to
  // having it, and on the Go side, see Opened.
  opened(ms: number, times: string) {
    const [started, open, first, file] = times.split(",").map(Number);
    this.stats.opens.push({ ms: Math.round(ms), started: started || 0, opened: open || 0, first: first || 0, file: file === 1 });
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
  frame(rank: number, timestamp: number, asked?: Set<Waiter>): Promise<Picture | null> {
    return new Promise((done) => {
      const w: Waiter = {
        rank,
        seq: ++this.asked,
        done: (f) => {
          asked?.delete(w);
          if (f && w.gone) this.onPassing?.(rank, f, w.seq);
          done(f && !w.gone ? f.clone(timestamp) : null);
        },
      };
      asked?.add(w);
      this.route(w);
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
      this.stats.kept++;
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
      s = new Stream(this, w.rank, width, height);
      this.stats.first ||= `${width}x${height}`;
      this.stats.streams++;
      this.streams.push(s);
      this.streams = this.streams.filter((x) => !x.ended);
      while (this.streams.length > STREAMS) {
        const oldest = this.streams.reduce((a, b) => (a.used <= b.used ? a : b));
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
    for (const [r, f] of this.kept) {
      if (this.keptBytes <= KEPT_BYTES) break;
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
      for (const done of this.waiting.values()) done({ status: 0, end: false, error: "", closed: false, times: "", light: "", frames: [] });
      this.waiting.clear();
    };
    this.worker = w;
    this.where = "worker";
  }

  async pull(url: string, width: number, height: number): Promise<Got> {
    const got = await this.pulled(url, width, height);
    return { ...got, frames: pictures(got, width, height) };
  }

  private async pulled(url: string, width: number, height: number): Promise<Pulled> {
    await this.ready;
    const w = this.worker;
    if (!w) return pull(url, width, height);
    const n = this.count++;
    return new Promise((done) => {
      this.waiting.set(n, done);
      w.postMessage({ n, url: new URL(url, location.href).href, width, height });
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

  decode(timestamp: number, _key: boolean, _data: Uint8Array, rank: number, wanted: boolean, first: number) {
    if (this.state !== "configured" || !wanted) return;
    if (this.next < 0) this.next = first;
    this.asked++;
    const generation = this.generation;
    void this.frames.frame(rank, timestamp, this.waiting).then((f) => {
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

  private settle() {
    if (this.asked > 0 || !this.flushes.length) return;
    this.putOut(true);
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
