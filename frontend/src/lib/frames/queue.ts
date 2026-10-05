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
//   sound heard, through the program, cuts included, reported to whoever
//   listens on every animation frame, so it moves smoothly and stands in
//   the frame drawn.
// - Paused, the frame that holds the playhead is on the canvas, and the
//   play from there is already prepared, its first frames decoded and its
//   first sound with them, so the space bar starts it at once rather than
//   decoding the same frames again. That is the cue.
// - Two picture decoders take turns: while one plays a piece, the other is
//   already decoding the next piece from its key frame, closing the frames
//   before the piece's start unseen, and holds the first few of it. So a
//   cut never waits for a decoder.
// - Frames are closed the moment they are replaced or not needed, and each
//   decoder holds at most a few, so memory stays flat over any length.
// - Nothing polls. Work is started by the decoders' own events, by a read
//   arriving, and by the animation frame that draws while playing.
import { findMoov, MP4Error, parseMoov, pcmPlanes, rankAt, type AudioTrack, type Movie, type VideoTrack } from "./mp4";
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

// Where the playhead is and what is on the canvas, told to whoever
// listens each time either changes, which while playing is every
// animation frame.
export type Shown = {
  // Where the playhead is, in the episode. Paused, exactly where it was
  // put. Playing, the sound being heard, through the program, so it moves
  // on every animation frame, jumps a cut with the sound, and stands in
  // the frame drawn.
  at: number;
  // Where the frame on the canvas begins, in the episode.
  frame: number;
  // A new frame was drawn for this report.
  drew: boolean;
  playing: boolean;
  // The program played to its end and stopped there.
  ended: boolean;
  // performance.now() when it was reported.
  drawnAt: number;
  // Why the sound cannot be played, or the picture stopped decoding, in a
  // sentence, or nothing.
  trouble: string;
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
  // Packets of sound decoded, and stretches handed to the sound card.
  soundDecoded: number;
  soundScheduled: number;
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
// How far a cue reads ahead, seconds. A drag on the clip timeline makes a
// cue at every step, and only the last one is played.
const CUE_AHEAD = 1;
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
    if (res.status !== 206 && res.status !== 200) throw new MP4Error(`the episode file could not be read, it answered ${res.status}`);
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

// Sound out of a decoder, the part of AudioData the queue reads.
type Sound = {
  timestamp: number;
  numberOfFrames: number;
  numberOfChannels: number;
  sampleRate: number;
  copyTo(dst: Float32Array, options: { planeIndex: number; format: "f32-planar" }): void;
  close(): void;
};

// A sound decoder, the browser's or the one for plain sound, which needs
// none. restart drops everything and waits for the first packet of a run.
interface SoundDecoder {
  readonly state: string;
  readonly decodeQueueSize: number;
  decode(timestamp: number, data: Uint8Array): void;
  flush(): Promise<void>;
  restart(): void;
  close(): void;
}

class WebSound implements SoundDecoder {
  private d: AudioDecoder;
  constructor(
    private config: AudioDecoderConfig,
    output: (s: Sound) => void,
    error: (what: string) => void,
    dequeue: () => void,
  ) {
    this.d = new AudioDecoder({ output, error: (e) => error(e.message) });
    this.d.configure(config);
    this.d.addEventListener("dequeue", dequeue);
  }
  get state() {
    return this.d.state;
  }
  get decodeQueueSize() {
    return this.d.decodeQueueSize;
  }
  decode(timestamp: number, data: Uint8Array) {
    this.d.decode(new EncodedAudioChunk({ type: "key", timestamp, data }));
  }
  flush() {
    return this.d.flush();
  }
  restart() {
    if (this.d.state !== "configured") return;
    this.d.reset();
    this.d.configure(this.config);
  }
  close() {
    if (this.d.state !== "closed") this.d.close();
  }
}

// Plain sound, as it lies in the file, turned into the sound card's
// numbers. It answers the way a decoder does, after the call that fed it,
// so the queue cannot tell the two apart.
class PlainSound implements SoundDecoder {
  state = "configured";
  private waiting: { timestamp: number; data: Uint8Array }[] = [];
  private flushes: { done: () => void; fail: (e: Error) => void }[] = [];
  private due = false;
  constructor(
    private track: AudioTrack,
    private output: (s: Sound) => void,
    private dequeue: () => void,
  ) {}
  get decodeQueueSize() {
    return this.waiting.length;
  }
  decode(timestamp: number, data: Uint8Array) {
    this.waiting.push({ timestamp, data });
    this.kick();
  }
  flush() {
    return new Promise<void>((done, fail) => {
      this.flushes.push({ done, fail });
      this.kick();
    });
  }
  restart() {
    this.waiting = [];
    const was = this.flushes;
    this.flushes = [];
    for (const f of was) f.fail(new Error("restarted"));
  }
  close() {
    this.restart();
    this.state = "closed";
  }
  private kick() {
    if (this.due) return;
    this.due = true;
    setTimeout(() => this.run(), 0);
  }
  private run() {
    this.due = false;
    const pcm = this.track.pcm!;
    const channels = this.track.channels;
    while (this.waiting.length && this.state === "configured") {
      const p = this.waiting.shift()!;
      const n = Math.floor(p.data.length / (pcm.bytes * channels));
      const planes = Array.from({ length: channels }, () => new Float32Array(n));
      pcmPlanes(p.data, pcm, channels, planes);
      this.output({
        timestamp: p.timestamp,
        numberOfFrames: n,
        numberOfChannels: channels,
        sampleRate: this.track.sampleRate,
        copyTo: (dst, o) => dst.set(planes[o.planeIndex].subarray(0, Math.min(n, dst.length))),
        close() {},
      });
    }
    if (this.state !== "configured") return;
    this.dequeue();
    if (!this.waiting.length) {
      const was = this.flushes;
      this.flushes = [];
      for (const f of was) f.done();
    }
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

// Paused with nothing prepared, paused with a cue, starting a play until
// its first frame and sound are in hand, playing, and stopped at the end
// of the program.
type State = "paused" | "cued" | "starting" | "playing" | "ended";

// A reason, in a sentence, from whatever went wrong.
function sentence(e: unknown): string {
  const said = e instanceof Error ? e.message : String(e);
  const text = said.trim().replace(/\.$/, "") || "it could not be read";
  return text[0].toUpperCase() + text.slice(1) + ".";
}

export class FrameQueue {
  // Settles once the file is read and the decoders are set up, and fails
  // with the reason in a sentence when the file cannot be played here.
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
    soundDecoded: 0,
    soundScheduled: 0,
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
  private soundDecoder: SoundDecoder | null = null;
  private videoConfig: VideoDecoderConfig | null = null;
  private closed = false;
  private trouble = "";

  // What play plays, as it was asked for: pieces, or null for the whole
  // episode straight on, and whether it loops. The program is made from
  // it once the length of the episode is known.
  private wanted: { pieces: Piece[] | null; loop: boolean } = { pieces: null, loop: false };
  private wantedKey = "";
  private built: Program | null = null;

  private state: State = "paused";
  // Where the playhead is, in the episode.
  private at = 0;
  // A play under way or cued: its plans and where it stands.
  private vplan: VideoPlan | null = null;
  private aplan: AudioPlan | null = null;
  private k = 0;
  // Where the play began, in the episode, and the furthest the playhead
  // has got on the program, so it never goes back while playing.
  private startFrom = 0;
  private reached = 0;
  // The moment of the episode the frame drawn last was drawn for, and
  // whether the first frame of the play or cue is up.
  private drawnFor = 0;
  private firstShown = false;
  private shown: { frame: VideoFrame; run: number; rank: number } | null = null;
  // The sound card's time at which the program's sample m0 is played.
  private c0: number | null = null;
  private m0 = 0;
  private sources = new Set<AudioBufferSourceNode>();
  private pending: Chunk | null = null;
  private unscheduled: Chunk[] = [];
  private soundRun: AudioRun | null = null;
  private soundNext = 0;
  private soundRunDone = new Set<number>();
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
  // Every play, cue and seek gets a ticket, and work from an older one is
  // dropped when it comes back.
  private ticket = 0;
  private scratch: Float32Array[] = [];

  constructor(canvas: HTMLCanvasElement, url: string) {
    this.canvas = canvas;
    const ctx = canvas.getContext("2d", { alpha: false });
    if (!ctx) throw new Error("the canvas has no 2d context");
    this.draw2d = ctx;
    // Black until the first frame, never the last frame of an episode
    // drawn on this canvas before.
    this.paint();
    this.reader = new Reader(url);
    this.ready = this.open().catch((e) => {
      throw new Error(sentence(e));
    });
    // Whoever needs the reason asks ready for it. Nobody asking is not an
    // error of its own.
    this.ready.catch(() => {});
  }

  private async open() {
    if (typeof VideoDecoder === "undefined") throw new MP4Error("the app cannot decode video on this system");
    const movie = await this.reader.open();
    if (this.closed) throw new Error("closed");
    this.movie = movie;
    this.video = movie.video ?? null;
    this.sound = movie.audio ?? null;
    if (!this.video) throw new MP4Error("the episode has no picture the video preview can decode");
    const v = this.video;
    const config: VideoDecoderConfig = {
      codec: v.codec,
      description: v.description,
      codedWidth: v.width,
      codedHeight: v.height,
      optimizeForLatency: true,
    };
    const support = await VideoDecoder.isConfigSupported(config).catch(() => ({ supported: false }));
    if (this.closed) throw new Error("closed");
    if (!support.supported) throw new MP4Error(`the app cannot decode the picture of this episode, ${v.codec}`);
    // A sound card at the episode's own rate, so the sound is resampled
    // once, on its way out, rather than stretch by stretch. It starts held,
    // and the first play, a gesture, lets it go.
    try {
      this.audio = new AudioContext(this.sound ? { sampleRate: this.sound.sampleRate } : {});
    } catch {
      this.audio = new AudioContext();
    }
    this.out = this.audio.createGain();
    this.out.connect(this.audio.destination);
    for (let i = 0; i < 2; i++) this.slots.push(this.makeSlot(config));
    if (this.sound) await this.openSound(this.sound);
    if (this.closed) throw new Error("closed");
  }

  private async openSound(a: AudioTrack) {
    const output = (d: Sound) => this.soundOut(d);
    const dequeue = () => this.pumpSound();
    if (a.pcm) {
      this.soundDecoder = new PlainSound(a, output, dequeue);
      return;
    }
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
    const config: AudioDecoderConfig = {
      codec: a.codec,
      description,
      sampleRate: a.decoderRate,
      numberOfChannels: a.channels,
    };
    const ok =
      typeof AudioDecoder !== "undefined" &&
      (await AudioDecoder.isConfigSupported(config).catch(() => ({ supported: false }))).supported;
    if (!ok) {
      this.fault(`The app cannot decode the sound of this episode, ${a.codec}, so it plays without sound.`);
      return;
    }
    this.soundDecoder = new WebSound(config, output, (e) => this.fault(`The sound stopped decoding: ${e}.`), dequeue);
  }

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
      error: (e) => this.fault(`The picture stopped decoding: ${e.message}.`),
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

  private get program(): Program {
    if (!this.movie) return new Program([], false);
    this.built ??= new Program(this.wanted.pieces ?? [{ start: 0, end: this.movie.duration }], this.wanted.loop);
    return this.built;
  }

  // What play plays: the pieces of a clip, cuts jumped, or null for the
  // whole episode straight on, and whether it loops. The same program
  // again changes nothing. A new one while playing goes on from where the
  // playhead is, on the new pieces, and a cue made for the old one is made
  // again.
  setProgram(pieces: Piece[] | null, loop: boolean) {
    if (!this.want(pieces, loop)) return;
    if (this.state === "playing" || this.state === "starting") this.start(this.at);
    else if (this.state === "cued" || this.vplan) this.seek(this.at);
  }

  // Takes a program as the one play plays, and says whether it is another
  // than the one before. Nothing is started.
  private want(pieces: Piece[] | null, loop: boolean): boolean {
    const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
    const key = JSON.stringify([copy, loop]);
    if (key === this.wantedKey) return false;
    this.wantedKey = key;
    this.wanted = { pieces: copy, loop };
    this.built = null;
    return true;
  }

  // The playhead to a moment of the episode, and with a program, what play
  // plays from there. Paused, the frame that holds it is decoded and drawn,
  // and the play from it is cued. Playing, play goes on from there.
  //
  // The program comes with the moment because a click that takes the
  // playhead across the edge of a clip while it plays changes both, and
  // the play has to start once, from the click. Told the program first,
  // the queue started again from where it was and said so, and that old
  // moment came back as the playhead the seek after it was sent to: the
  // click was lost.
  seek(at: number, program?: [Piece[] | null, boolean]) {
    if (this.closed) return;
    const changed = program ? this.want(...program) : false;
    // A play starting from this very moment on this program is the play
    // asked for. A click seeks as the hand goes down and again as it comes
    // up, and starting over would throw away what was decoded between.
    if (!changed && this.state === "starting" && at === this.startFrom) return;
    this.at = at;
    if (this.state === "playing" || this.state === "starting") {
      this.start(at);
      return;
    }
    this.stopPlay();
    this.state = "paused";
    this.settle(at);
  }

  // A paused playhead: the play from it is cued when its first frame is the
  // frame that holds the playhead, and the frame is decoded on its own
  // where it is not, in a cut of the clip or past its end.
  private settle(at: number) {
    const ticket = this.ticket;
    this.ready.then(
      () => {
        if (ticket !== this.ticket || this.closed) return;
        const s = this.video!.samples;
        const program = this.program;
        const p0 = program.place(at);
        const there = p0 < program.length ? program.locate(p0) : null;
        if (there && rankAt(s, there.at) === rankAt(s, at)) this.start(at, true);
        else void this.still(at);
      },
      () => {},
    );
  }

  play() {
    if (this.closed || this.state === "playing" || this.state === "starting") return;
    // The sound card starts on the gesture that asked for it, or a browser
    // keeps it silent.
    void this.audio?.resume();
    // Paused in the middle of a play: the play is all still there, the
    // sound card was only held.
    if (this.vplan && this.c0 !== null && this.state === "paused") {
      clearTimeout(this.suspendTimer);
      this.state = "playing";
      this.ramp(1);
      this.report();
      this.loop();
      return;
    }
    // Cued: everything is there but the clock.
    if (this.state === "cued") {
      this.state = "starting";
      this.report();
      this.maybeBegin();
      return;
    }
    let from = this.at;
    if (this.state === "ended" || (this.movie && this.program.place(from) >= this.program.length)) {
      from = this.program.pieces[0]?.start ?? 0;
    }
    this.start(from);
  }

  pause() {
    if (this.state === "starting") {
      // Nothing was heard yet: stop it, stay on the frame shown, and cue
      // the play again from there.
      this.stopPlay();
      this.state = "paused";
      this.report();
      this.settle(this.at);
      return;
    }
    if (this.state !== "playing") return;
    this.state = "paused";
    cancelAnimationFrame(this.frameRequest);
    // The sound fades out over the render's fade, then the sound card is
    // held, with everything scheduled kept, so play goes on from here.
    this.ramp(0);
    this.suspendTimer = window.setTimeout(() => void this.audio.suspend(), FADE * 1000 + 10);
    this.hold();
    this.report();
  }

  // Paused, the picture is the frame that holds the playhead. The playhead
  // runs up to a frame ahead of the frame drawn for it, and that frame is
  // nearly always decoded already. Where it is not, the playhead goes back
  // to the moment the frame on screen was drawn for.
  private hold() {
    if (!this.shown || !this.video) return;
    const want = rankAt(this.video.samples, this.at);
    if (this.shown.rank === want) return;
    for (const slot of this.slots) {
      const i = slot.ready.findIndex((h) => h.rank === want);
      if (i < 0 || !slot.run) continue;
      const [h] = slot.ready.splice(i, 1);
      this.put(h.frame, slot.run.id, h.rank);
      return;
    }
    this.at = this.drawnFor;
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    this.stopPlay();
    for (const s of this.slots) {
      this.release(s);
      if (s.decoder.state !== "closed") s.decoder.close();
    }
    if (this.shown) this.closeFrame(this.shown.frame);
    this.shown = null;
    this.soundDecoder?.close();
    if (this.audio && this.audio.state !== "closed") void this.audio.close();
    this.reader.forget();
    this.listeners.clear();
  }

  // The canvas has this many device pixels, as the stylesheet laid it out.
  // The frame on it is drawn again to fit.
  resize(width: number, height: number) {
    const w = Math.max(1, Math.round(width));
    const h = Math.max(1, Math.round(height));
    if (this.canvas.width === w && this.canvas.height === h) return;
    this.canvas.width = w;
    this.canvas.height = h;
    this.paint();
  }

  // The frame on the canvas, in its own pixels, for a colour taken from the
  // picture, or null.
  picture(): VideoFrame | null {
    return this.shown?.frame ?? null;
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
    if (this.closed) return;
    this.trouble = what;
    this.report();
  }

  private emit(s: Shown) {
    for (const fn of this.listeners) fn(s);
  }

  private report(drew = false) {
    this.emit({
      at: this.at,
      frame: this.shown ? this.shownAt(this.shown.rank) : NaN,
      drew,
      playing: this.state === "playing" || this.state === "starting",
      ended: this.state === "ended",
      drawnAt: performance.now(),
      trouble: this.trouble,
    });
  }

  private closeFrame(f: VideoFrame) {
    f.close();
    this.stats.open--;
  }

  // Draws a frame and closes the one it replaces.
  private put(frame: VideoFrame, run: number, rank: number) {
    if (this.shown && this.shown.frame !== frame) this.closeFrame(this.shown.frame);
    this.shown = { frame, run, rank };
    this.paint();
    this.stats.drawn++;
  }

  // The frame on screen, as large as the canvas allows and in its own
  // shape, in the middle, on whole pixels, and black around it.
  private paint() {
    const c = this.canvas;
    const g = this.draw2d;
    g.fillStyle = "#000";
    g.fillRect(0, 0, c.width, c.height);
    const f = this.shown?.frame;
    if (!f || !f.displayWidth || !f.displayHeight) return;
    const scale = Math.min(c.width / f.displayWidth, c.height / f.displayHeight);
    const w = Math.round(f.displayWidth * scale);
    const h = Math.round(f.displayHeight * scale);
    g.drawImage(f, Math.round((c.width - w) / 2), Math.round((c.height - h) / 2), w, h);
  }

  private shownAt(rank: number): number {
    const s = this.video!.samples;
    return s.pts[s.order[rank]] / s.timescale;
  }

  // ---- A paused frame on its own

  private async still(at: number) {
    try {
      await this.ready;
    } catch {
      return;
    }
    const ticket = ++this.ticket;
    const slot = this.slots[0];
    this.release(slot);
    const s = this.video!.samples;
    const feed = stillFeed(s, at);
    // Already on screen: nothing to decode.
    if (this.shown && this.shown.rank === feed.rank) {
      this.at = at;
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
      slot.decoder.decode(new EncodedVideoChunk({ type: s.key[i] ? "key" : "delta", timestamp: ts, data }));
    }
    try {
      await slot.decoder.flush();
    } catch {
      return;
    }
    if (ticket !== this.ticket) return;
    const got = slot.ready.findIndex((h) => h.rank === feed.rank);
    let drew = false;
    if (got >= 0) {
      const [h] = slot.ready.splice(got, 1);
      this.put(h.frame, -1, h.rank);
      drew = true;
    }
    this.release(slot);
    this.at = at;
    this.report(drew);
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
    this.soundDecoder?.restart();
    this.soundFed = [];
    this.soundRunDone.clear();
    this.soundRun = null;
    this.pending = null;
    this.unscheduled = [];
    this.vplan = null;
    this.aplan = null;
    this.c0 = null;
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

  // A play from a moment of the episode, or with cue, the same play
  // prepared and waiting for play.
  private start(from: number, cue = false) {
    this.stopPlay();
    const ticket = this.ticket;
    this.state = cue ? "cued" : "starting";
    this.firstShown = false;
    this.startFrom = from;
    this.at = from;
    if (!cue) {
      void this.audio?.resume();
      this.report();
    }
    this.ready.then(
      () => {
        if (ticket !== this.ticket || this.closed) return;
        this.out.gain.cancelScheduledValues(this.audio.currentTime);
        this.out.gain.setValueAtTime(1, this.audio.currentTime);
        const p0 = this.program.place(from);
        if (!this.program.loop && p0 >= this.program.length) {
          this.state = "ended";
          this.report();
          return;
        }
        this.reached = p0;
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
      },
      () => {},
    );
  }

  // Puts the first frame of a play or a cue up the moment it is decoded,
  // and starts the sound card's clock once the first frame and the first
  // of the sound are in hand and play was asked for.
  private maybeBegin() {
    if ((this.state !== "starting" && this.state !== "cued") || !this.vplan) return;
    const need = this.vplan.need(0);
    const slot = need ? this.slotOf(need.run) : null;
    let frameIn = !need || (!!this.shown && this.shown.run === need.run && this.shown.rank === need.rank);
    if (need && !frameIn && slot?.ready[0]?.rank === need.rank) {
      const h = slot.ready.shift()!;
      this.put(h.frame, need.run, h.rank);
      this.drawnFor = this.vplan.moment(0) ?? this.at;
      frameIn = true;
      this.firstShown = true;
      this.report(true);
    }
    // A cue says once that its frame is on the canvas, drawn now or there
    // already, the way a still does.
    if (frameIn && !this.firstShown) {
      this.firstShown = true;
      this.drawnFor = this.vplan.moment(0) ?? this.at;
      if (this.state === "cued") this.report();
    }
    if (this.state === "cued") return;
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
    // Never back: the sound heard only ever moves on, and a stamp that says
    // a hair less than the last one is the clock's jitter.
    const pos = Math.max(this.position(now), this.reached);
    this.reached = pos;
    plan.extend(pos + READ_AHEAD);
    this.aplan?.extend(pos + READ_AHEAD);
    const k = Math.max(this.k, plan.gridAt(pos));
    if (plan.finished && k > plan.lastK) {
      this.end();
      return;
    }
    this.k = k;
    let drew = false;
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
            this.drawnFor = plan.moment(k) ?? this.drawnFor;
            drew = true;
          } else {
            this.stats.late++;
          }
        } else {
          this.stats.late++;
        }
      }
    }
    const there = this.program.locate(pos);
    if (there) this.at = there.at;
    this.emit({
      at: this.at,
      frame: this.shown ? this.shownAt(this.shown.rank) : NaN,
      drew,
      playing: true,
      ended: false,
      drawnAt: now,
      trouble: this.trouble,
    });
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
    const ahead = this.state === "cued" ? CUE_AHEAD : READ_AHEAD;
    if (this.vplan) {
      const frames = Math.ceil(ahead / this.video!.frame);
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
      // Packets of 20 ms, the shortest any codec here has, so a budget is
      // never short. Plain sound is read by the chunk.
      let budget = Math.ceil(ahead / 0.02);
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
    if (!plan || (this.state !== "starting" && this.state !== "playing" && this.state !== "cued")) return;
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
    if (rank === undefined || !run || this.closed) {
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
    if (this.state === "starting" || this.state === "cued") this.maybeBegin();
    this.pump();
  }

  // ---- Feeding the sound

  private pumpSound() {
    const plan = this.aplan;
    const dec = this.soundDecoder;
    if (!plan || !dec || dec.state !== "configured" || this.soundDone) return;
    if (this.state !== "starting" && this.state !== "playing" && this.state !== "cued") return;
    const s = this.sound!.samples;
    const rate = this.sampleRate;
    const heard = this.c0 === null ? this.m0 : Math.round(this.position(performance.now()) * rate);
    const ahead = (this.c0 === null ? SOUND_FIRST * 2 : SOUND_AHEAD) * rate;
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
      this.soundNext++;
      const p = plan.packet(j);
      for (const sl of plan.slices(run, j, p.n)) this.soundReach = Math.max(this.soundReach, sl.out + (sl.to - sl.from));
      dec.decode(ts, data);
    }
  }

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

  private soundOut(d: Sound) {
    // The decoder says the rate it puts the sound out at. Where that is not
    // the rate the file said, HE-AAC said only inside its sound above all,
    // the play starts over at the decoder's rate, before anything is heard.
    if (this.sound && this.c0 === null && Math.abs(d.sampleRate - this.sound.sampleRate) > 1) {
      d.close();
      this.sound.sampleRate = d.sampleRate;
      this.start(this.startFrom, this.state === "cued");
      return;
    }
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
    this.stats.soundDecoded++;
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
          if (this.pending && (this.pending.out + this.pending.length !== out || this.pending.data.length !== channels)) {
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
    if (this.state === "starting" || this.state === "cued") this.maybeBegin();
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
    this.stats.soundScheduled++;
    src.onended = () => {
      this.sources.delete(src);
      src.disconnect();
    };
    this.sources.add(src);
  }
}
