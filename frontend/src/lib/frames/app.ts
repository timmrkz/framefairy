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
import { rankAt, type VideoTrack } from "./mp4";
import { pull, type Pulled } from "./pull";

// How much of the frames shown last is kept, and how far ahead a stream may
// be from a frame wanted of it and still be waited for, in seconds. Further
// than that, a new stream from the key frame before is quicker.
const KEPT_BYTES = 96 << 20;
const REACH = 1.5;
// How many streams are kept open: one for each of the queue's two
// decoders, and one more for a still or a seek while they play.
const STREAMS = 3;

type Waiter = { rank: number; done: (f: VideoFrame | null) => void };

class Stream {
  id: Promise<string | null>;
  // The rank of the last frame read, and the first this stream can give.
  position: number;
  readonly first: number;
  ended = false;
  // Whether it has given a frame yet.
  private gave = false;
  used = performance.now();
  private waiting: Waiter[] = [];
  private pulling = false;

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
      .then((r) => (r.ok ? r.json() : null))
      .then((j: { id?: string } | null) => j?.id ?? null)
      .catch(() => null);
  }

  // Whether this stream is the quickest way to the frame: it has not passed
  // it and is not too far behind it.
  reaches(rank: number): boolean {
    if (this.ended || rank <= this.position || rank < this.first) return false;
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
        this.end("the app could not decode the picture of this episode");
        return;
      }
      while (this.waiting.length && !this.ended) {
        const lowest = Math.min(...this.waiting.map((w) => w.rank));
        const highest = Math.max(...this.waiting.map((w) => w.rank));
        // Frames before the first one wanted are decoded on the Go side
        // and dropped there.
        const skip = lowest > this.position + 1 ? this.owner.ptsOf(lowest) - this.owner.track.frame / 2 : 0;
        const n = Math.min(16, Math.max(1, highest - Math.max(this.position, lowest - 1)));
        const q = new URLSearchParams({ id, n: String(n) });
        if (skip > 0) q.set("skip", skip.toFixed(6));
        const got = await this.owner.puller.pull(`/frames/read?${q}`, this.width, this.height, { ...this.owner.track.colour, fullRange: false });
        if (got.status === 0 || got.status === 404) {
          // Closed on the Go side after standing unused: what was waited
          // for goes to a new stream. A stream that never gave a frame is
          // no stream at all, and asking again would only ask again.
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
          this.end(got.error);
          return;
        }
        this.take(got);
      }
    } finally {
      this.pulling = false;
    }
  }

  private take(got: Pulled) {
    for (const { at, frame } of got.frames) {
      const rank = rankAt(this.owner.track.samples, at + this.owner.track.frame / 2);
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
  private kept = new Map<number, VideoFrame>();
  private keptBytes = 0;
  // Why the Go side could not decode a frame, for the queue to say.
  trouble = "";
  readonly stats = { streams: 0, continued: 0, kept: 0 };
  readonly puller = new Puller();

  constructor(
    readonly path: string,
    readonly track: VideoTrack,
    private size: () => { width: number; height: number },
    private onTrouble: (why: string) => void,
  ) {}

  ptsOf(rank: number): number {
    const s = this.track.samples;
    return s.pts[s.order[Math.max(0, Math.min(rank, s.count - 1))]] / s.timescale;
  }

  // The frame of this rank, as a new frame with this timestamp, once it is
  // there. Null only when nothing near it could be decoded.
  frame(rank: number, timestamp: number): Promise<VideoFrame | null> {
    return new Promise((done) => {
      this.route({
        rank,
        done: (f) => done(f ? new VideoFrame(f, { timestamp }) : null),
      });
    });
  }

  route(w: Waiter) {
    const have = this.kept.get(w.rank);
    if (have) {
      this.stats.kept++;
      // The kept frame stays the most recently used.
      this.kept.delete(w.rank);
      this.kept.set(w.rank, have);
      w.done(have);
      return;
    }
    const { width, height } = this.size();
    let s = this.streams.find((x) => x.width === width && x.height === height && x.reaches(w.rank));
    if (s) this.stats.continued++;
    else {
      s = new Stream(this, w.rank, width, height);
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

  keep(rank: number, frame: VideoFrame) {
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
  nearest(rank: number): VideoFrame | null {
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
    this.puller.close();
    for (const s of this.streams) s.close();
    this.streams = [];
    for (const f of this.kept.values()) f.close();
    this.kept.clear();
    this.keptBytes = 0;
  }
}

// Pulls go through a Worker, pull.worker.ts, once it has shown that it can
// reach the Go side and hand a frame back, and are read on the page where
// it cannot: a WebKit that will not run a Worker for the app's own scheme
// still plays.
class Puller {
  private worker: Worker | null = null;
  private ready: Promise<void>;
  private count = 0;
  private waiting = new Map<number, (p: Pulled) => void>();
  // Where the pulls are read, for the walks.
  where: "worker" | "page" | "starting" = "starting";

  constructor() {
    this.ready = this.start();
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
      w.onmessage = (e: MessageEvent<{ n: number; frame: VideoFrame | null }>) => {
        if (e.data.n !== -1) return;
        clearTimeout(late);
        const f = e.data.frame;
        const good = typeof VideoFrame !== "undefined" && f instanceof VideoFrame;
        f?.close();
        done(good);
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
    w.onerror = () => {
      this.worker = null;
      this.where = "page";
      for (const done of this.waiting.values()) done({ status: 0, end: false, error: "", frames: [] });
      this.waiting.clear();
    };
    this.worker = w;
    this.where = "worker";
  }

  async pull(url: string, width: number, height: number, colour: VideoColorSpaceInit): Promise<Pulled> {
    await this.ready;
    const w = this.worker;
    if (!w) return pull(url, width, height, colour);
    const n = this.count++;
    return new Promise((done) => {
      this.waiting.set(n, done);
      w.postMessage({ n, url: new URL(url, location.href).href, width, height, colour });
    });
  }

  close() {
    this.worker?.terminate();
    this.worker = null;
  }
}

function bytes(f: VideoFrame): number {
  return (f.codedWidth * f.codedHeight * 3) / 2;
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
  private held = new Map<number, VideoFrame | null>();
  // The rank to put out next, or -1 before the first frame of a run.
  private next = -1;
  private generation = 0;
  private flushes: { done: () => void; fail: (e: Error) => void }[] = [];

  constructor(
    private frames: AppFrames,
    private output: (f: VideoFrame) => void,
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
    void this.frames.frame(rank, timestamp).then((f) => {
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
