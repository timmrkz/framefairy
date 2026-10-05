// The frame queue: the episode played from its own file onto one canvas,
// with the browser's decoders and the sound card's clock, so a clip plays
// through its cuts the way the render does. Everything this file decides
// is decided in plan.ts, with tests. This is only the glue to the browser:
// reading the file, feeding VideoDecoder and AudioDecoder, drawing, and
// scheduling the sound.
//
// How it runs, in short:
//
// - The sound card's clock drives everything. The sound is decoded ahead,
//   cut at the sample, and scheduled on the AudioContext at the exact place
//   on the program it belongs, so the stretches meet without a gap.
// - A frame is drawn when the sound heard has reached its grid point, on
//   the animation frame before it is due on screen. The playhead is the
//   frame drawn, reported to whoever listens as it is drawn.
// - Two picture decoders take turns: while one plays a piece, the other is
//   already decoding the next piece from its key frame, closing the frames
//   before the piece's start unseen, and holds the first few of it. So a
//   cut never waits for a decoder.
// - Frames are closed the moment they are replaced or not needed, and each
//   decoder holds at most a few, so memory stays flat over any length.
// - Nothing polls. Work is started by the decoders' own events, by a read
//   arriving, and by the animation frame that draws while playing.
import { findMoov, MP4Error, parseMoov, type AudioTrack, type Movie, type VideoTrack } from "./mp4";
import {
  AudioPlan,
  fade,
  FADE,
  heardAt,
  Keeper,
  Program,
  spans,
  stillFeed,
  VideoPlan,
  type AudioRun,
  type Piece,
  type Run,
} from "./plan";

export type { Piece } from "./plan";

// What is on the canvas, told to whoever listens each time it changes.
export type Shown = {
  // The moment of the episode the frame drawn stands for: where the
  // playhead is. Paused, it is exactly where the playhead was put.
  at: number;
  // Where the frame drawn begins, in the episode.
  frame: number;
  playing: boolean;
  // The program played to its end and stopped there.
  ended: boolean;
  // performance.now() when it was drawn.
  drawnAt: number;
};

export type Stats = {
  // VideoFrames open now, and the most there ever were.
  open: number;
  mostOpen: number;
  decoded: number;
  // Decoded and closed without being drawn: the frames between a key frame
  // and a piece's start.
  unseen: number;
  drawn: number;
  // Grid points whose frame was not there in time, so the frame before
  // stayed.
  late: number;
  // Stretches of sound scheduled after their time had come.
  lateSound: number;
  reads: number;
  bytesRead: number;
  errors: string[];
};

// How many frames a decoder may hold or have on its way for drawing.
const DEPTH = 4;
// How far ahead of the playhead the next piece's decoder starts, seconds.
const NEXT_AHEAD = 2;
// How far ahead the plan is worked out and the file is read, seconds.
const READ_AHEAD = 4;
// How much sound is decoded ahead of what is heard, seconds.
const SOUND_AHEAD = 1;
// Sound decoded before a play starts, seconds.
const SOUND_FIRST = 0.15;
// How long the sound card is given to start, seconds.
const START_LEAD = 0.05;
// Samples in one stretch of sound handed to the sound card.
const CHUNK = 4096;
// Bytes of the file kept read, at most.
const CACHE = 64 << 20;

// A unique timestamp for every sample fed, in the order the samples of
// its run are shown, so a decoder that orders its output by timestamp
// orders it right too. Runs are 100 000 seconds apart.
function stamp(run: number, seconds: number): number {
  return run * 1e11 + 1e10 + Math.round(seconds * 1e6);
}

type Span = { from: number; to: number; data: Uint8Array | null; ready: Promise<void>; used: number };

// The episode file, read in ranges and kept for a while.
class Reader {
  size = 0;
  reads = 0;
  bytes = 0;
  private spans: Span[] = [];
  private kept = 0;

  constructor(readonly url: string) {}

  async read(from: number, to: number): Promise<Uint8Array> {
    this.reads++;
    const res = await fetch(this.url, { headers: { Range: `bytes=${from}-${to - 1}` } });
    if (res.status !== 206 && res.status !== 200) throw new Error(`the episode file answered ${res.status}`);
    const total = /\/(\d+)$/.exec(res.headers.get("content-range") ?? "");
    if (total) this.size = Number(total[1]);
    let body = new Uint8Array(await res.arrayBuffer());
    // A server that sends the whole file has sent what was asked too.
    if (res.status === 200) {
      this.size = body.length;
      body = body.subarray(from, to);
    }
    this.bytes += body.length;
    return body;
  }

  async open(): Promise<Movie> {
    const first = await this.read(0, 1 << 16);
    if (!this.size) this.size = first.length;
    const moov = await findMoov(
      async (from, to) => (to <= first.length ? first.subarray(from, to) : this.read(from, to)),
      this.size,
    );
    return parseMoov(moov);
  }

  // Starts reading whatever of these ranges is not read or on its way.
  want(ranges: { from: number; to: number }[]) {
    for (const r of spans(ranges, this.spans)) {
      const span: Span = { from: r.from, to: r.to, data: null, ready: Promise.resolve(), used: performance.now() };
      span.ready = this.read(r.from, r.to).then(
        (data) => {
          span.data = data;
          this.kept += data.length;
          this.trim();
        },
        () => {
          this.spans = this.spans.filter((s) => s !== span);
        },
      );
      this.spans.push(span);
    }
  }

  // The bytes of a sample if they are in hand, or the promise of them.
  get(offset: number, size: number): Uint8Array | Promise<void> {
    const span = this.spans.find((s) => s.from <= offset && offset + size <= s.to);
    if (!span) {
      this.want([{ from: offset, to: offset + size }]);
      return this.spans[this.spans.length - 1].ready;
    }
    span.used = performance.now();
    if (!span.data) return span.ready;
    return span.data.subarray(offset - span.from, offset - span.from + size);
  }

  // Lets go of what was used longest ago once too much is kept.
  private trim() {
    if (this.kept <= CACHE) return;
    const done = this.spans.filter((s) => s.data).sort((a, b) => a.used - b.used);
    for (const s of done) {
      if (this.kept <= CACHE) break;
      this.kept -= s.data!.length;
      this.spans = this.spans.filter((x) => x !== s);
    }
  }

  forget() {
    this.spans = [];
    this.kept = 0;
  }
}

// A stretch of sound on its way to the sound card: where on the program it
// goes, in samples, and its samples, one array a channel.
type Chunk = { out: number; length: number; data: Float32Array<ArrayBuffer>[] };

type Held = { frame: VideoFrame; rank: number };

// One picture decoder and the run it is on.
type Slot = {
  decoder: VideoDecoder;
  run: Run | null;
  keeper: Keeper | null;
  next: number;
  // Fed and not yet out, by timestamp: the rank of each.
  fed: Map<number, number>;
  // Of those, how many will be drawn.
  coming: number;
  ready: Held[];
  flushing: boolean;
  // The rank of the first frame its run draws, so the frames before it are
  // not counted as coming.
  firstRank: number;
};

type State = "paused" | "starting" | "playing" | "ended";

export class FrameQueue {
  readonly ready: Promise<void>;
  movie: Movie | null = null;
  // The sound card. Everything played goes through out, which a probe can
  // listen to.
  audio!: AudioContext;
  out!: GainNode;
  readonly stats: Stats = {
    open: 0,
    mostOpen: 0,
    decoded: 0,
    unseen: 0,
    drawn: 0,
    late: 0,
    lateSound: 0,
    reads: 0,
    bytesRead: 0,
    errors: [],
  };

  private reader: Reader;
  private canvas: HTMLCanvasElement;
  private draw2d: CanvasRenderingContext2D;
  private listeners = new Set<(s: Shown) => void>();
  private video: VideoTrack | null = null;
  private sound: AudioTrack | null = null;
  private slots: Slot[] = [];
  private soundDecoder: AudioDecoder | null = null;

  private program = new Program([], false);
  private state: State = "paused";
  // Where the playhead is, in the episode, while paused.
  private at = 0;
  // A play under way: its plans and where it stands.
  private vplan: VideoPlan | null = null;
  private aplan: AudioPlan | null = null;
  private k = 0;
  private shown: { frame: VideoFrame; run: number; rank: number } | null = null;
  // The sound card's time at which the program's sample m0 is played.
  private c0: number | null = null;
  private m0 = 0;
  private sources = new Set<AudioBufferSourceNode>();
  private pending: Chunk | null = null;
  private unscheduled: Chunk[] = [];
  private soundRun: AudioRun | null = null;
  private soundNext = 0;
  // The packets fed and not yet out, in the order they were fed. A sound
  // decoder puts out one stretch per packet, in order, but it may stamp it
  // with a time of its own: Chromium counts on from the first packet after
  // it starts, so the stamp is not a key to look the packet up by.
  private soundFed: { ts: number; run: AudioRun; j: number; length: number }[] = [];
  // How far on the program the sound fed so far reaches, in samples.
  private soundReach = 0;
  private soundDone = false;
  private frameRequest = 0;
  private suspendTimer = 0;
  // Every play and seek gets a ticket, and work from an older one is
  // dropped when it comes back.
  private ticket = 0;
  private scratch: Float32Array[] = [];

  constructor(canvas: HTMLCanvasElement, url: string) {
    this.canvas = canvas;
    const ctx = canvas.getContext("2d", { alpha: false });
    if (!ctx) throw new Error("the canvas has no 2d context");
    this.draw2d = ctx;
    this.reader = new Reader(url);
    this.ready = this.open();
  }

  private async open() {
    const movie = await this.reader.open();
    this.movie = movie;
    this.video = movie.video ?? null;
    this.sound = movie.audio ?? null;
    if (!this.video) throw new MP4Error("the episode has no picture the video preview can decode");
    // A sound card at the episode's own rate, so the sound is resampled
    // once, on its way out, rather than stretch by stretch. It starts held,
    // and the first play, a gesture, lets it go.
    this.audio = new AudioContext(this.sound ? { sampleRate: this.sound.sampleRate } : {});
    this.out = this.audio.createGain();
    this.out.connect(this.audio.destination);
    const v = this.video;
    const config: VideoDecoderConfig = {
      codec: v.codec,
      description: v.description,
      codedWidth: v.width,
      codedHeight: v.height,
      optimizeForLatency: true,
    };
    const support = await VideoDecoder.isConfigSupported(config);
    if (!support.supported) throw new MP4Error(`this browser cannot decode ${v.codec}`);
    for (let i = 0; i < 2; i++) this.slots.push(this.makeSlot(config));
    if (this.sound) {
      const a = this.sound;
      // The pre-skip of Opus is cut by the edit list already, see editShift
      // in mp4.ts, and a run starts ahead of what it plays anyway. Told it
      // as well, the decoder would cut it a second time from the first
      // packet after every start, so it is told there is none.
      let description = a.description;
      if (a.codec === "opus" && description && description.length >= 12) {
        description = description.slice();
        description[10] = 0;
        description[11] = 0;
      }
      const aconfig: AudioDecoderConfig = {
        codec: a.codec,
        description,
        sampleRate: a.sampleRate,
        numberOfChannels: a.channels,
      };
      const asupport = await AudioDecoder.isConfigSupported(aconfig);
      if (asupport.supported) {
        this.soundDecoder = new AudioDecoder({
          output: (d) => this.soundOut(d),
          error: (e) => this.fault(`sound decoder: ${e.message}`),
        });
        this.soundDecoder.configure(aconfig);
        this.soundDecoder.addEventListener("dequeue", () => this.pumpSound());
        this.soundConfig = aconfig;
      } else {
        this.fault(`this browser cannot decode the sound, ${a.codec}`);
      }
    }
    if (this.canvas.width !== v.width || this.canvas.height !== v.height) {
      this.canvas.width = v.width;
      this.canvas.height = v.height;
    }
    this.program = new Program([{ start: 0, end: movie.duration }], false);
  }

  private soundConfig: AudioDecoderConfig | null = null;
  private videoConfig: VideoDecoderConfig | null = null;

  private makeSlot(config: VideoDecoderConfig): Slot {
    this.videoConfig = config;
    const slot: Slot = {
      decoder: null as unknown as VideoDecoder,
      run: null,
      keeper: null,
      next: 0,
      fed: new Map(),
      coming: 0,
      ready: [],
      flushing: false,
      firstRank: 0,
    };
    slot.decoder = new VideoDecoder({
      output: (f) => this.frameOut(slot, f),
      error: (e) => this.fault(`picture decoder: ${e.message}`),
    });
    slot.decoder.configure(config);
    slot.decoder.addEventListener("dequeue", () => this.pump());
    return slot;
  }

  // The sound card's rate is the episode's, where the browser allows it.
  get sampleRate(): number {
    return this.sound?.sampleRate ?? this.audio.sampleRate;
  }

  listen(fn: (s: Shown) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  // What play plays. A change while playing goes on from where the
  // playhead is, on the new pieces.
  setProgram(pieces: Piece[], loop: boolean) {
    this.program = new Program(pieces, loop);
    if (this.state === "playing" || this.state === "starting") this.start(this.at);
    // A play held in the middle was a play of the old pieces.
    else if (this.vplan) this.stopPlay();
  }

  // The playhead to a moment of the episode. Paused, the frame that holds
  // it is decoded and drawn. Playing, play goes on from there.
  seek(at: number) {
    this.at = at;
    if (this.state === "playing" || this.state === "starting") {
      this.start(at);
      return;
    }
    this.stopPlay();
    this.state = "paused";
    void this.still(at);
  }

  play() {
    if (this.state === "playing" || this.state === "starting") return;
    // The sound card starts on the gesture that asked for it, or a browser
    // keeps it silent.
    void this.audio?.resume();
    // Paused in the middle of a play: the play is all still there, the
    // sound card was only held.
    if (this.vplan && this.c0 !== null && this.state === "paused") {
      clearTimeout(this.suspendTimer);
      this.state = "playing";
      this.ramp(1);
      this.loop();
      return;
    }
    let from = this.at;
    if (this.state === "ended" || this.program.place(from) >= this.program.length) {
      from = this.program.pieces[0]?.start ?? 0;
    }
    this.start(from);
  }

  pause() {
    if (this.state === "starting") {
      // Nothing was heard yet: stop it and stay on the frame shown.
      this.stopPlay();
      this.state = "paused";
      this.report();
      return;
    }
    if (this.state !== "playing") return;
    this.state = "paused";
    cancelAnimationFrame(this.frameRequest);
    // The sound fades out over the render's fade, then the sound card is
    // held, with everything scheduled kept, so play goes on from here.
    this.ramp(0);
    this.suspendTimer = window.setTimeout(() => void this.audio.suspend(), FADE * 1000 + 10);
    this.report();
  }

  close() {
    this.stopPlay();
    for (const s of this.slots) {
      this.release(s);
      if (s.decoder.state !== "closed") s.decoder.close();
    }
    if (this.shown) this.closeFrame(this.shown.frame);
    this.shown = null;
    if (this.soundDecoder && this.soundDecoder.state !== "closed") this.soundDecoder.close();
    void this.audio.close();
  }

  private ramp(to: number) {
    const now = this.audio.currentTime;
    this.out.gain.cancelScheduledValues(now);
    this.out.gain.setValueAtTime(this.out.gain.value, now);
    this.out.gain.linearRampToValueAtTime(to, now + FADE);
  }

  private fault(what: string) {
    this.stats.errors.push(what);
    console.error("frame queue:", what);
  }

  private emit(s: Shown) {
    for (const fn of this.listeners) fn(s);
  }

  private report() {
    this.emit({
      at: this.at,
      frame: this.shown ? this.shownAt(this.shown.rank) : NaN,
      playing: this.state === "playing" || this.state === "starting",
      ended: this.state === "ended",
      drawnAt: performance.now(),
    });
  }

  private closeFrame(f: VideoFrame) {
    f.close();
    this.stats.open--;
  }

  // Draws a frame and closes the one it replaces.
  private put(frame: VideoFrame, run: number, rank: number) {
    this.draw2d.drawImage(frame, 0, 0, this.canvas.width, this.canvas.height);
    if (this.shown && this.shown.frame !== frame) this.closeFrame(this.shown.frame);
    this.shown = { frame, run, rank };
    this.stats.drawn++;
  }

  private shownAt(rank: number): number {
    const s = this.video!.samples;
    return s.pts[s.order[rank]] / s.timescale;
  }

  // ---- A paused frame

  private async still(at: number) {
    await this.ready;
    const ticket = ++this.ticket;
    const slot = this.slots[0];
    this.release(slot);
    const s = this.video!.samples;
    const feed = stillFeed(s, at);
    // Already on screen: nothing to decode.
    if (this.shown && this.shownAt(this.shown.rank) === this.shownAt(feed.rank) && this.shown.run === -1) {
      this.report();
      return;
    }
    const want: { from: number; to: number }[] = [];
    for (let i = feed.key; i <= feed.last; i++) want.push({ from: s.offset[i], to: s.offset[i] + s.size[i] });
    this.reader.want(want);
    slot.run = { id: -1 - ticket, key: feed.key, last: feed.last, kFirst: 0, kLast: 0, lastRank: feed.rank };
    slot.firstRank = feed.rank;
    for (let i = feed.key; i <= feed.last; i++) {
      let data = this.reader.get(s.offset[i], s.size[i]);
      while (!(data instanceof Uint8Array)) {
        await data;
        if (ticket !== this.ticket) return;
        data = this.reader.get(s.offset[i], s.size[i]);
      }
      if (ticket !== this.ticket || slot.decoder.state !== "configured") return;
      const ts = stamp(0, s.pts[i] / s.timescale);
      slot.fed.set(ts, s.rank[i]);
      slot.decoder.decode(
        new EncodedVideoChunk({ type: s.key[i] ? "key" : "delta", timestamp: ts, data }),
      );
    }
    try {
      await slot.decoder.flush();
    } catch {
      return;
    }
    if (ticket !== this.ticket) return;
    const got = slot.ready.findIndex((h) => h.rank === feed.rank);
    if (got >= 0) {
      const [h] = slot.ready.splice(got, 1);
      this.put(h.frame, -1, h.rank);
    }
    this.release(slot);
    this.at = at;
    this.report();
  }

  // ---- Playing

  private stopPlay() {
    this.ticket++;
    cancelAnimationFrame(this.frameRequest);
    clearTimeout(this.suspendTimer);
    for (const src of this.sources) {
      try {
        src.stop();
      } catch {}
      src.disconnect();
    }
    this.sources.clear();
    for (const s of this.slots) this.release(s);
    if (this.soundDecoder && this.soundDecoder.state === "configured") {
      this.soundDecoder.reset();
      this.soundDecoder.configure(this.soundConfig!);
    }
    this.soundFed = [];
    this.soundRunDone.clear();
    this.soundRun = null;
    this.pending = null;
    this.unscheduled = [];
    this.vplan = null;
    this.aplan = null;
    this.c0 = null;
    this.reader.forget();
  }

  // Empties a decoder and lets go of its run.
  private release(slot: Slot) {
    for (const h of slot.ready) this.closeFrame(h.frame);
    slot.ready = [];
    slot.coming = 0;
    slot.run = null;
    slot.keeper = null;
    // A decoder that was flushed and has nothing on its way is ready for a
    // key frame as it is. Any other is reset, which drops what it holds.
    const idle = slot.flushing && slot.fed.size === 0;
    slot.fed.clear();
    slot.flushing = false;
    if (slot.decoder.state === "configured" && !idle) {
      slot.decoder.reset();
      slot.decoder.configure(this.videoConfig!);
    }
  }

  private start(from: number) {
    this.stopPlay();
    const ticket = this.ticket;
    this.state = "starting";
    void this.audio?.resume();
    void this.ready.then(() => {
      if (ticket !== this.ticket) return;
      this.out.gain.cancelScheduledValues(this.audio.currentTime);
      this.out.gain.setValueAtTime(1, this.audio.currentTime);
      const p0 = this.program.place(from);
      if (!this.program.loop && p0 >= this.program.length) {
        this.state = "ended";
        this.report();
        return;
      }
      this.vplan = new VideoPlan(this.video!.samples, this.video!.frame, this.program, p0);
      this.vplan.extend(p0 + READ_AHEAD);
      if (this.sound && this.soundDecoder) {
        this.aplan = new AudioPlan(this.sound.samples, this.sound.sampleRate, this.program, p0);
        this.aplan.extend(p0 + READ_AHEAD);
        this.m0 = this.aplan.m0;
        this.soundReach = this.m0;
        this.soundDone = false;
      } else {
        this.m0 = Math.round(p0 * this.sampleRate);
        this.soundDone = true;
      }
      this.k = 0;
      this.readAhead();
      this.pump();
      this.pumpSound();
    });
  }

  // Starts the sound card's clock once the first frame and the first of
  // the sound are in hand.
  private maybeBegin() {
    if (this.state !== "starting" || !this.vplan) return;
    const need = this.vplan.need(0);
    const slot = need ? this.slotOf(need.run) : null;
    const frameIn =
      !need || (this.shown && this.shown.run === need.run && this.shown.rank === need.rank) || slot?.ready[0]?.rank === need.rank;
    const soundIn =
      this.soundDone || this.soundReach - this.m0 >= SOUND_FIRST * this.sampleRate || (this.aplan?.finished && this.soundRun === null);
    if (!frameIn || !soundIn) return;
    if (this.audio.state !== "running") {
      const ticket = this.ticket;
      void this.audio.resume().then(() => {
        if (ticket === this.ticket) this.maybeBegin();
      });
      return;
    }
    const rate = this.audio.sampleRate;
    this.c0 = Math.ceil((this.audio.currentTime + START_LEAD) * rate) / rate;
    this.state = "playing";
    if (this.pending) {
      this.unscheduled.push(this.pending);
      this.pending = null;
    }
    for (const c of this.unscheduled) this.schedule(c);
    this.unscheduled = [];
    this.tick(performance.now());
  }

  // Where the program is now, from the sound being heard, at the moment a
  // frame drawn now is on screen, a display frame from now.
  private position(now: number): number {
    if (this.c0 === null || !this.vplan) return this.vplan?.p0 ?? 0;
    const stampNow = this.audio.getOutputTimestamp ? this.audio.getOutputTimestamp() : null;
    const latency = (this.audio.outputLatency || 0) + (this.audio.baseLatency || 0);
    const heard = heardAt(
      stampNow && stampNow.contextTime !== undefined && stampNow.performanceTime !== undefined
        ? { contextTime: stampNow.contextTime, performanceTime: stampNow.performanceTime }
        : null,
      this.audio.currentTime,
      latency,
      now + 1000 / 60,
    );
    return this.m0 / this.sampleRate + (heard - this.c0);
  }

  private loop() {
    this.frameRequest = requestAnimationFrame((now) => this.tick(now));
  }

  private tick(now: number) {
    if (this.state !== "playing" || !this.vplan) return;
    const plan = this.vplan;
    const pos = this.position(now);
    plan.extend(pos + READ_AHEAD);
    this.aplan?.extend(pos + READ_AHEAD);
    const k = Math.max(this.k, plan.gridAt(pos));
    if (plan.finished && k > plan.lastK) {
      this.end();
      return;
    }
    this.k = k;
    const need = plan.need(k);
    if (need) {
      const slot = this.slotOf(need.run);
      const onScreen = this.shown && this.shown.run === need.run && this.shown.rank === need.rank;
      if (!onScreen) {
        if (slot) {
          while (slot.ready.length && slot.ready[0].rank < need.rank) {
            this.closeFrame(slot.ready.shift()!.frame);
          }
          if (slot.ready[0]?.rank === need.rank) {
            const h = slot.ready.shift()!;
            this.put(h.frame, need.run, h.rank);
            this.at = plan.moment(k) ?? this.at;
            this.emit({ at: this.at, frame: this.shownAt(h.rank), playing: true, ended: false, drawnAt: now });
          } else {
            this.stats.late++;
          }
        } else {
          this.stats.late++;
        }
      }
    }
    // A run whose frames are all behind the playhead is done with.
    for (const s of this.slots) if (s.run && s.run.id >= 0 && s.run.kLast < k) this.release(s);
    plan.forget(k);
    this.aplan?.forget(Math.round(pos * this.sampleRate));
    this.readAhead();
    this.pump();
    this.pumpSound();
    this.loop();
  }

  private end() {
    const plan = this.vplan!;
    const last = this.program.pieces[this.program.pieces.length - 1];
    this.state = "ended";
    this.at = last ? last.end : this.at;
    this.k = plan.lastK;
    this.report();
    // The sound card is held once the last of the sound has been heard.
    const ticket = this.ticket;
    this.suspendTimer = window.setTimeout(() => {
      if (ticket === this.ticket) void this.audio.suspend();
    }, 300);
  }

  private slotOf(run: number): Slot | null {
    return this.slots.find((s) => s.run?.id === run) ?? null;
  }

  // Reads what the next seconds of feeding will need, picture and sound
  // together, so a request brings both from the part of the file they
  // share.
  private readAhead() {
    const want: { from: number; to: number }[] = [];
    const v = this.video!.samples;
    if (this.vplan) {
      const frames = Math.ceil(READ_AHEAD / this.video!.frame);
      let budget = frames * 2;
      for (const run of this.vplan.runs) {
        if (run.kLast < this.k) continue;
        const slot = this.slotOf(run.id);
        const from = slot ? slot.next : run.key;
        for (let i = from; i <= run.last && budget > 0; i++, budget--) {
          want.push({ from: v.offset[i], to: v.offset[i] + v.size[i] });
        }
      }
    }
    if (this.aplan && this.sound) {
      const a = this.sound.samples;
      let budget = Math.ceil((READ_AHEAD * this.sound.sampleRate) / 960);
      for (const run of this.aplan.runs) {
        if (budget <= 0) break;
        if (this.soundRun && run.id < this.soundRun.id) continue;
        const from = this.soundRun === run ? this.soundNext : run.first;
        for (let j = from; j <= run.last && budget > 0; j++, budget--) {
          want.push({ from: a.offset[j], to: a.offset[j] + a.size[j] });
        }
      }
    }
    if (want.length) this.reader.want(want);
    this.stats.reads = this.reader.reads;
    this.stats.bytesRead = this.reader.bytes;
  }

  // ---- Feeding the picture

  private pump() {
    const plan = this.vplan;
    if (!plan || (this.state !== "starting" && this.state !== "playing")) return;
    // Put runs on free decoders: the run being drawn, and the next one
    // once it is close.
    for (const run of plan.runs) {
      if (run.kLast < this.k || this.slotOf(run.id)) continue;
      if ((run.kFirst - this.k) * this.video!.frame > NEXT_AHEAD) break;
      const slot = this.slots[run.id % 2];
      if (slot.run) break;
      slot.run = run;
      slot.keeper = new Keeper(plan, run);
      slot.next = run.key;
      slot.firstRank = plan.need(run.kFirst)?.rank ?? 0;
    }
    for (const slot of this.slots) this.feed(slot);
  }

  private feed(slot: Slot) {
    const run = slot.run;
    const plan = this.vplan;
    if (!run || !plan || slot.decoder.state !== "configured") return;
    const s = this.video!.samples;
    while (slot.next <= run.last) {
      const busy = slot.decoder.decodeQueueSize;
      const hungry = busy === 0 && slot.ready.length === 0;
      if (busy >= 3 || (slot.ready.length + slot.coming >= DEPTH && !hungry)) return;
      const i = slot.next;
      const data = this.reader.get(s.offset[i], s.size[i]);
      if (!(data instanceof Uint8Array)) {
        const ticket = this.ticket;
        void data.then(() => {
          if (ticket === this.ticket) this.pump();
        });
        return;
      }
      const ts = stamp(run.id, s.pts[i] / s.timescale);
      const rank = s.rank[i];
      slot.fed.set(ts, rank);
      if (rank >= slot.firstRank && rank <= run.lastRank) slot.coming++;
      slot.decoder.decode(new EncodedVideoChunk({ type: s.key[i] ? "key" : "delta", timestamp: ts, data }));
      slot.next++;
    }
    // Everything fed. Once no later piece can carry the run on, the decoder
    // is flushed so the last frames come out without waiting for more.
    const closed = plan.finished || plan.runs[plan.runs.length - 1] !== run;
    if (closed && !slot.flushing) {
      slot.flushing = true;
      const ticket = this.ticket;
      slot.decoder.flush().then(
        () => {
          if (ticket === this.ticket) this.maybeBegin();
        },
        () => {},
      );
    }
  }

  private frameOut(slot: Slot, frame: VideoFrame) {
    this.stats.open++;
    this.stats.mostOpen = Math.max(this.stats.mostOpen, this.stats.open);
    this.stats.decoded++;
    const rank = slot.fed.get(frame.timestamp);
    slot.fed.delete(frame.timestamp);
    const run = slot.run;
    if (rank === undefined || !run) {
      this.stats.unseen++;
      this.closeFrame(frame);
      return;
    }
    if (rank >= slot.firstRank && rank <= run.lastRank) slot.coming = Math.max(0, slot.coming - 1);
    // A still: kept for still() to pick out.
    if (run.id < 0) {
      if (rank === run.lastRank) slot.ready.push({ frame, rank });
      else {
        this.stats.unseen++;
        this.closeFrame(frame);
      }
      return;
    }
    if (!slot.keeper || !slot.keeper.offer(rank)) {
      this.stats.unseen++;
      this.closeFrame(frame);
    } else {
      slot.ready.push({ frame, rank });
    }
    if (this.state === "starting") this.maybeBegin();
    this.pump();
  }

  // ---- Feeding the sound

  private pumpSound() {
    const plan = this.aplan;
    const dec = this.soundDecoder;
    if (!plan || !dec || dec.state !== "configured" || this.soundDone) return;
    if (this.state !== "starting" && this.state !== "playing") return;
    const s = this.sound!.samples;
    const rate = this.sampleRate;
    const heard = this.c0 === null ? this.m0 : Math.round(this.position(performance.now()) * rate);
    const ahead = (this.state === "starting" ? SOUND_FIRST * 2 : SOUND_AHEAD) * rate;
    for (;;) {
      if (!this.soundRun) {
        const next = plan.runs.find((r) => !this.soundRunDone.has(r.id));
        if (!next) {
          if (plan.finished) this.finishSound();
          return;
        }
        this.soundRun = next;
        this.soundNext = next.first;
      }
      const run = this.soundRun;
      if (this.soundNext > run.last) {
        const closed = plan.finished || plan.runs[plan.runs.length - 1] !== run;
        if (!closed) return;
        // A cut that is not carried on: what the decoder still holds is
        // put out before the next run's lead starts.
        this.soundRunDone.add(run.id);
        this.soundRun = null;
        void dec.flush().catch(() => {});
        continue;
      }
      if (this.soundReach - heard >= ahead || dec.decodeQueueSize >= 8) return;
      const j = this.soundNext;
      const data = this.reader.get(s.offset[j], s.size[j]);
      if (!(data instanceof Uint8Array)) {
        const ticket = this.ticket;
        void data.then(() => {
          if (ticket === this.ticket) this.pumpSound();
        });
        return;
      }
      const ts = stamp(run.id, s.pts[j] / s.timescale);
      this.soundFed.push({ ts, run, j, length: (s.duration[j] / s.timescale) * 1e6 });
      dec.decode(new EncodedAudioChunk({ type: "key", timestamp: ts, data }));
      this.soundNext++;
      const p = plan.packet(j);
      for (const sl of plan.slices(run, j, p.n)) this.soundReach = Math.max(this.soundReach, sl.out + (sl.to - sl.from));
    }
  }

  private soundRunDone = new Set<number>();

  private finishSound() {
    if (this.soundDone || !this.soundDecoder) return;
    this.soundDone = true;
    const ticket = this.ticket;
    this.soundDecoder.flush().then(
      () => {
        if (ticket !== this.ticket) return;
        if (this.pending) {
          this.hand(this.pending);
          this.pending = null;
        }
        this.maybeBegin();
      },
      () => {},
    );
  }

  private soundOut(d: AudioData) {
    // A packet that came out as nothing is passed over: the stretch is
    // further on than that packet could be by its stamp, and as near to
    // the next one as it should be.
    const q = this.soundFed;
    while (
      q.length > 1 &&
      Math.abs(q[0].ts - d.timestamp) > q[0].length &&
      Math.abs(q[1].ts - d.timestamp) < Math.abs(q[0].ts - d.timestamp)
    ) {
      q.shift();
    }
    const fed = q.shift();
    if (!fed || !this.aplan) {
      d.close();
      return;
    }
    const plan = this.aplan;
    const parts = plan.slices(fed.run, fed.j, d.numberOfFrames);
    if (parts.length) {
      const channels = d.numberOfChannels;
      const rate = this.sampleRate;
      for (let c = 0; c < channels; c++) {
        if (!this.scratch[c] || this.scratch[c].length < d.numberOfFrames) this.scratch[c] = new Float32Array(Math.max(d.numberOfFrames, 4096));
        d.copyTo(this.scratch[c], { planeIndex: c, format: "f32-planar" });
      }
      for (const sl of parts) {
        let at = sl.from;
        while (at < sl.to) {
          const out = sl.out + (at - sl.from);
          if (this.pending && this.pending.out + this.pending.length !== out) {
            this.hand(this.pending);
            this.pending = null;
          }
          if (!this.pending) {
            this.pending = { out, length: 0, data: Array.from({ length: channels }, () => new Float32Array(CHUNK)) };
          }
          const p = this.pending;
          const n = Math.min(sl.to - at, CHUNK - p.length);
          for (let c = 0; c < channels; c++) {
            const src = this.scratch[c];
            const dst = p.data[c];
            for (let i = 0; i < n; i++) {
              const m = out + i;
              dst[p.length + i] = src[at + i] * fade(m, sl.pieceOut, sl.pieceEnd, rate);
            }
          }
          p.length += n;
          at += n;
          if (p.length === CHUNK) {
            this.hand(p);
            this.pending = null;
          }
        }
      }
    }
    d.close();
    if (this.state === "starting") this.maybeBegin();
    this.pumpSound();
  }

  // A stretch of sound to the sound card, or kept until the clock starts.
  private hand(c: Chunk) {
    if (this.c0 === null) this.unscheduled.push(c);
    else this.schedule(c);
  }

  private schedule(c: Chunk) {
    if (!c.length || this.c0 === null) return;
    const rate = this.sampleRate;
    const buffer = this.audio.createBuffer(c.data.length, c.length, rate);
    for (let ch = 0; ch < c.data.length; ch++) buffer.copyToChannel(c.data[ch].subarray(0, c.length), ch);
    const src = this.audio.createBufferSource();
    src.buffer = buffer;
    src.connect(this.out);
    const when = this.c0 + (c.out - this.m0) / rate;
    const now = this.audio.currentTime;
    if (when < now) {
      this.stats.lateSound++;
      const skip = now - when;
      if (skip < c.length / rate) src.start(now, skip);
    } else {
      src.start(when);
    }
    src.onended = () => {
      this.sources.delete(src);
      src.disconnect();
    };
    this.sources.add(src);
  }
}
