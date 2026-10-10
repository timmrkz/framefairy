// The frame queue: the episode played onto one canvas with the sound
// card's clock, so a clip plays through its cuts the way the render does.
// Every frame and every sample comes from ffmpeg on the Go side, the
// episode's decoder, see docs/VIDEO-PREVIEW.md. The browser decodes
// nothing. Everything this file decides is decided in plan.ts, with tests.
// This is only the glue: the file's index, asking the Go side for frames
// and sound, drawing, and scheduling the sound.
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
// - Nothing polls. Work is started by the decoders' own events, by frames
//   and sound arriving, and by the animation frame that draws while
//   playing.
import { AppFrames, AppPictures } from "./app";
import { findMoov, MP4Error, parseMoov, rankAt, type AudioTrack, type Movie, type VideoTrack } from "./mp4";
import {
  AudioPlan,
  fade,
  FADE,
  heardAt,
  Keeper,
  Program,
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
// How far ahead the plan is worked out, seconds.
const READ_AHEAD = 4;
// How much sound is decoded ahead of what is heard, seconds.
const SOUND_AHEAD = 1;
// How far ahead the sound is fed while the page's frames stop, behind
// another window, where a timer may run only once a second, see keepOn.
const SOUND_BEHIND = 3;
// Sound decoded before a play starts, seconds.
const SOUND_FIRST = 0.15;
// How far ahead of the sound card's clock a play's first sound is put,
// seconds. The sound card renders a buffer at a time, and a stretch put
// where it has rendered already starts late and out of step with the
// picture. A buffer is 512 to 1024 samples on the Mac, 11 to 21 ms at
// 48 kHz, and 25 ms is a whole one of the larger with room left. It was
// 50, and the harness played to the sample from 2, so this is the Mac's
// margin and not the harness's. Every millisecond here is a millisecond
// from the press to the picture moving.
const START_LEAD = 0.025;
// Samples in one stretch of sound handed to the sound card.
const CHUNK = 4096;

// A unique timestamp for every sample fed, in the order the samples of
// its run are shown, so a decoder that orders its output by timestamp
// orders it right too. Runs are 100 000 seconds apart.
function stamp(run: number, seconds: number): number {
  return run * 1e11 + 1e10 + Math.round(seconds * 1e6);
}

// The episode file, of which only the index is read here: which frames
// and which packets of sound there are, and when. The Go side decodes
// both, see docs/VIDEO-PREVIEW.md. Step 5 there takes this away too.
class Reader {
  size = 0;
  reads = 0;
  bytes = 0;
  // The file as the server last described it, its length and when it was
  // written, see fileStamps.
  stamp = "";

  constructor(
    readonly url: string,
    private known: Movie | null = null,
  ) {}

  async read(from: number, to: number): Promise<Uint8Array> {
    this.reads++;
    // Never from the webview's cache: a file written long ago is taken as
    // unchanged for a while, and one written over since would be read as
    // it was.
    const res = await fetch(this.url, { headers: { Range: `bytes=${from}-${to - 1}` }, cache: "no-store" });
    if (res.status !== 206 && res.status !== 200) throw new MP4Error(`the video file could not be read, it answered ${res.status}`);
    const total = /\/(\d+)$/.exec(res.headers.get("content-range") ?? "");
    if (total) this.size = Number(total[1]);
    this.stamp = `${total?.[1] ?? res.headers.get("content-length") ?? ""}|${res.headers.get("last-modified") ?? ""}`;
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
    if (this.known && (await this.same(this.known))) return this.known;
    const early = ahead.get(this.url);
    if (early) {
      ahead.delete(this.url);
      const movie = await early.catch(() => null);
      if (movie) return movie;
    }
    return this.read0();
  }

  private async read0(): Promise<Movie> {
    const first = await this.read(0, 1 << 16);
    if (!this.size) this.size = first.length;
    const moov = await findMoov(
      async (from, to) => (to <= first.length ? first.subarray(from, to) : this.read(from, to)),
      this.size,
    );
    const movie = parseMoov(moov);
    fileStamps.set(movie, this.stamp);
    return movie;
  }

  // Whether the file is still the one the index was read from. A workspace
  // keeps the index while it sleeps, and an episode written over in the
  // meantime, exported again under its name, would be read by the old one.
  private async same(movie: Movie): Promise<boolean> {
    try {
      const res = await fetch(this.url, { method: "HEAD", cache: "no-store" });
      return res.ok && `${res.headers.get("content-length") ?? ""}|${res.headers.get("last-modified") ?? ""}` === fileStamps.get(movie);
    } catch {
      return false;
    }
  }

}

// The file each index was read from, its length and when it was written.
const fileStamps = new WeakMap<Movie, string>();

// The indexes read ahead, by the episode's address, until a queue takes
// its own. See readAhead.
const ahead = new Map<string, Promise<Movie>>();

// Starts what a video preview needs before anything else, the moment a
// workspace starts to open, rather than once the workspace has asked the
// Go side what the episode is: the file's index, read in up to three reads
// one after another, and the episode's decoder on the Go side, ffmpeg
// started with the file open. Both were the last links of a chain, and the
// first frame waited for each in turn. The decoder is held only until it
// is up: the queue holds it for itself, and one nobody holds stands for
// twenty seconds before it is closed.
export function readAhead(url: string, path: string) {
  if (!ahead.has(url)) {
    const movie = new Reader(url).open();
    movie.catch(() => ahead.delete(url));
    ahead.set(url, movie);
    // Never more than a few waiting: an index is megabytes for a long
    // episode.
    for (const old of ahead.keys()) {
      if (ahead.size <= 4) break;
      ahead.delete(old);
    }
  }
  const q = new URLSearchParams({ path });
  // Released only once held, so a hold that failed takes nobody else's.
  void fetch(`/frames/hold?${q}`)
    .then((res) => (res.ok ? fetch(`/frames/release?${q}`) : null))
    .catch(() => {});
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

// A sound decoder, ffmpeg's on the Go side or the one for plain sound,
// which needs none. restart drops everything and waits for the first
// packet of a run. Each packet comes with where it lies in the track, at,
// and how long it is, n, in samples.
interface SoundDecoder {
  readonly state: string;
  readonly decodeQueueSize: number;
  decode(timestamp: number, data: Uint8Array, at: number, n: number): void;
  flush(): Promise<void>;
  restart(): void;
  close(): void;
}

// The sound of the episode as ffmpeg decodes it, on the Go side, see
// /frames/sound in cmd/framefairy-app/frames.go: the same sound the render
// and the transcript are made of, whatever decoder the webview has. The
// Mac's own decoder of AAC had its own idea of where a file's sound
// starts, and Tim heard picture and sound out of step in a file from
// DaVinci Resolve, which keeps the encoder's 44 ms of silence at the
// front of its sound and holds the picture back by as much.
//
// A run of packets is one stream, opened at the first packet's place,
// and every packet after it is the next n samples of it, the way ffmpeg
// reads a piece of a render. It answers the way a decoder does, after the
// call that fed it, so the queue cannot tell it from one. The packets
// themselves are not needed, only where they lie.
export class GoSound implements SoundDecoder {
  state = "configured";
  // Packets and flushes in the order they came. A flush ends the run of
  // the packets before it, and the packets after it are another run, read
  // from a stream of their own where the first of them lies.
  private waiting: ({ timestamp: number; at: number; n: number } | { done: () => void; fail: (e: Error) => void })[] = [];
  private stream: GoSoundStream | null = null;
  private running = false;
  // Bumped by restart, so a pump that was waiting on the Go side drops
  // what it brings back.
  private era = 0;
  constructor(
    private path: string,
    private rate: number,
    private channels: number,
    private output: (s: Sound) => void,
    private error: (what: string) => void,
    private dequeue: () => void,
  ) {}
  get decodeQueueSize() {
    return this.waiting.filter((w) => "at" in w).length;
  }
  decode(timestamp: number, _data: Uint8Array, at: number, n: number) {
    this.waiting.push({ timestamp, at, n });
    void this.run();
  }
  flush() {
    return new Promise<void>((done, fail) => {
      this.waiting.push({ done, fail });
      void this.run();
    });
  }
  restart() {
    this.era++;
    this.running = false;
    const was = this.waiting;
    this.waiting = [];
    this.stream?.close();
    this.stream = null;
    for (const w of was) if ("fail" in w) w.fail(new Error("restarted"));
  }
  close() {
    this.restart();
    this.state = "closed";
  }
  private async run() {
    if (this.running || this.state !== "configured") return;
    this.running = true;
    const era = this.era;
    try {
      while (this.waiting.length) {
        const p = this.waiting[0];
        if (!("at" in p)) {
          this.waiting.shift();
          this.stream?.close();
          this.stream = null;
          p.done();
          continue;
        }
        this.stream ??= new GoSoundStream(this.path, p.at, this.rate, this.channels);
        const got = await this.stream.take(p.n);
        if (era !== this.era) return;
        this.waiting.shift();
        if (got.length) {
          const frames = got.length / this.channels;
          const channels = this.channels;
          this.output({
            timestamp: p.timestamp,
            numberOfFrames: frames,
            numberOfChannels: channels,
            sampleRate: this.rate,
            copyTo: (dst, o) => {
              const n = Math.min(frames, dst.length);
              for (let i = 0; i < n; i++) dst[i] = got[i * channels + o.planeIndex];
            },
            close() {},
          });
        }
        this.dequeue();
        if (era !== this.era) return;
      }
    } catch (e) {
      if (era !== this.era) return;
      this.error(e instanceof Error ? e.message : String(e));
      // The stream is dropped with the packet it failed on, and the next
      // packet opens a stream of its own where it lies. Kept, every packet
      // after it failed on the same stream, no sound came, and a play
      // waiting for its first sound never started: the space bar seemed
      // to do nothing, Tim found.
      this.stream?.close();
      this.stream = null;
      if (this.waiting.length && "at" in this.waiting[0]) this.waiting.shift();
      this.dequeue();
    } finally {
      if (era === this.era) this.running = false;
    }
    if (era === this.era && this.waiting.length) void this.run();
  }
}

// One stream of sound from the Go side, from a place in the track on, in
// samples. Sound before the track's start, the priming of AAC that the
// edit list puts before zero, is silence, as it is to ffmpeg.
//
// The Go side closes a stream nobody has read from for 20 seconds, and
// the least read when too many are open, see previewIdle in
// cmd/framefairy-app/frames.go, and then answers 404. A pause holds the
// play and its stream, so a play paused for longer found it gone, and the
// video preview said the sound had stopped decoding, Tim found. A stream
// that is gone is opened again where it got to.
export class GoSoundStream {
  private id: Promise<string>;
  private have = new Float32Array(0);
  private silence: number;
  private ended = false;
  // Where the Go side's stream started, and how many moments of it came.
  private from: number;
  private came = 0;
  // Closed by the page: a read still on its way that finds it gone on
  // the Go side ends it, it never opens it again. Opened again, every
  // seek of a play left a stream that nobody closed.
  private closed = false;
  constructor(
    private path: string,
    at: number,
    private rate: number,
    private channels: number,
  ) {
    this.silence = Math.max(0, -at) * channels;
    this.from = Math.max(0, at);
    this.id = this.open(this.from);
  }
  private open(at: number): Promise<string> {
    const q = new URLSearchParams({ path: this.path, from: (at / this.rate).toFixed(9), rate: String(this.rate), ch: String(this.channels) });
    const id = fetch(`/frames/sound?${q}`).then(async (r) => {
      if (!r.ok) throw new Error(`the sound could not be read, it answered ${r.status}`);
      const j = (await r.json()) as { id?: string };
      if (!j.id) throw new Error("the sound could not be read");
      return j.id;
    });
    id.catch(() => {});
    return id;
  }
  // The next n moments, all channels side by side, or fewer at the end.
  async take(n: number): Promise<Float32Array> {
    const want = n * this.channels;
    const out = new Float32Array(want);
    let filled = 0;
    if (this.silence) {
      const z = Math.min(this.silence, want);
      this.silence -= z;
      filled = z;
    }
    while (filled < want) {
      if (!this.have.length) {
        if (this.ended || this.closed) break;
        await this.pull();
        continue;
      }
      const k = Math.min(this.have.length, want - filled);
      out.set(this.have.subarray(0, k), filled);
      this.have = this.have.subarray(k);
      filled += k;
    }
    return out.subarray(0, filled);
  }
  private async pull() {
    let r = await fetch(`/frames/read?id=${await this.id}&n=16`);
    if (this.closed) return;
    if (r.status === 404) {
      // Closed on the Go side: on again from the moment after the last
      // that came, once, and a stream that is gone straight away is
      // a fault.
      this.id = this.open(this.from + this.came);
      r = await fetch(`/frames/read?id=${await this.id}&n=16`);
    }
    if (!r.ok) throw new Error(`the sound stopped, it answered ${r.status}`);
    const got = soundChunks(new Uint8Array(await r.arrayBuffer()), this.channels);
    this.came += got.length / this.channels;
    if (got.length) {
      const joined = new Float32Array(this.have.length + got.length);
      joined.set(this.have);
      joined.set(got, this.have.length);
      this.have = joined;
    }
    if (r.headers.get("X-Frames-End")) {
      const why = r.headers.get("X-Frames-Error");
      if (why) throw new Error(why);
      this.ended = true;
    }
  }
  close() {
    this.closed = true;
    void this.id.then((id) => fetch(`/frames/close?id=${id}`)).catch(() => {});
  }
}

const NOTHING = new Uint8Array(0);

// engine.SoundChunk, how many moments of sound the Go side sends at a time.
export const SOUND_CHUNK = 1024;

// The samples in what the Go side sent for a pull of a sound stream: each
// chunk the moment it starts at, 8 bytes, then SOUND_CHUNK moments of
// float samples, the channels side by side, little endian, and the last
// chunk of a stream maybe shorter. The moments are not needed, a stream
// being one run read straight on.
export function soundChunks(body: Uint8Array, channels: number): Float32Array {
  const whole = 8 + SOUND_CHUNK * channels * 4;
  let count = 0;
  for (let off = 0; off + 8 < body.length; off += whole) count += Math.floor((Math.min(off + whole, body.length) - off - 8) / 4);
  const out = new Float32Array(count);
  const v = new DataView(body.buffer, body.byteOffset, body.byteLength);
  let k = 0;
  for (let off = 0; off + 8 < body.length; off += whole) {
    const end = Math.min(off + whole, body.length);
    for (let b = off + 8; b + 4 <= end; b += 4) out[k++] = v.getFloat32(b, true);
  }
  return out;
}

// A picture decoder, AppPictures, whose frames the Go side decodes. It
// answers the calls of a VideoDecoder, so the queue feeds it the way it fed
// the browser's. A chunk is fed with its rank and whether it will be drawn.
interface Pictures {
  readonly state: string;
  readonly decodeQueueSize: number;
  // first is the rank of the first frame the run draws.
  decode(timestamp: number, key: boolean, data: Uint8Array, rank: number, wanted: boolean, first: number): void;
  flush(): Promise<void>;
  // Drops everything, ready for a key frame.
  reset(): void;
  close(): void;
}

// Nothing, for a chunk whose bytes the Go side reads for itself.
const NO_DATA = new Uint8Array(0);

// A stretch of sound on its way to the sound card: where on the program it
// goes, in samples, and its samples, one array a channel.
type Chunk = { out: number; length: number; data: Float32Array<ArrayBuffer>[] };

type Held = { frame: VideoFrame; rank: number };

// One picture decoder and the run it is on.
type Slot = {
  decoder: Pictures;
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
  // The Go side's frames, from the episode's decoder: every frame drawn,
  // whatever the file. Null only until the file's index is read.
  private app: AppFrames | null = null;
  private closed = false;
  private trouble = "";

  // What play plays, as it was asked for: pieces, or null for the whole
  // episode straight on, and whether it loops. The program is made from
  // it once the length of the episode is known.
  private wanted: { pieces: Piece[] | null; loop: boolean } = { pieces: null, loop: false };
  private wantedKey = "";
  private built: Program | null = null;
  private asked: [Piece[] | null, boolean] | null = null;

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
  // The ticket of the seek or play whose own frame is on the canvas, and
  // the newest ask of the Go side's frames made when it was put up.
  private putFor = -1;
  private shownAsked = 0;
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

  // known is the file's index as a queue before this one read it, for a
  // workspace woken again, see sleeping in Player.svelte. The canvas then
  // holds this episode's own frame, which stays until the first frame of
  // this queue replaces it.
  constructor(canvas: HTMLCanvasElement, url: string, known: Movie | null = null) {
    this.canvas = canvas;
    const ctx = canvas.getContext("2d", { alpha: false });
    if (!ctx) throw new Error("the canvas has no 2d context");
    this.draw2d = ctx;
    // Black until the first frame, never the last frame of an episode
    // drawn on this canvas before.
    if (!known) this.paint();
    this.reader = new Reader(url, known);
    this.ready = this.open().catch((e) => {
      throw new Error(sentence(e));
    });
    // Whoever needs the reason asks ready for it. Nobody asking is not an
    // error of its own.
    this.ready.catch(() => {});
  }

  private async open() {
    if (typeof VideoFrame === "undefined") throw new MP4Error("the app cannot draw video on this system");
    const movie = await this.reader.open();
    this.stats.reads = this.reader.reads;
    this.stats.bytesRead = this.reader.bytes;
    if (this.closed) throw new Error("closed");
    this.movie = movie;
    this.video = movie.video ?? null;
    this.sound = movie.audio ?? null;
    if (!this.video) throw new MP4Error("the video has no picture the video preview can decode");
    // Every file's picture comes from ffmpeg on the Go side, the episode's
    // decoder. The webview decodes nothing, see docs/VIDEO-PREVIEW.md.
    this.app = this.fromApp();
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
    for (let i = 0; i < 2; i++) this.slots.push(this.makeSlot());
    if (this.sound) await this.openSound(this.sound);
    if (this.closed) throw new Error("closed");
  }

  // The sound, decoded by ffmpeg on the Go side whatever it is, plain
  // sound in a MOV too.
  private async openSound(a: AudioTrack) {
    const output = (d: Sound) => this.soundOut(d);
    const dequeue = () => this.pumpSound();
    const path = new URL(this.reader.url, location.href).searchParams.get("path") ?? "";
    this.soundDecoder = new GoSound(path, a.sampleRate, Math.min(a.channels, 8), output, (e) => this.fault(`The sound stopped decoding: ${e}.`), dequeue);
  }

  private makeSlot(): Slot {
    const slot: Slot = {
      decoder: null as unknown as Pictures,
      run: null,
      keeper: null,
      next: 0,
      fed: new Map(),
      coming: 0,
      ready: [],
      flushing: false,
      firstRank: 0,
    };
    slot.decoder = new AppPictures(
      this.app!,
      (f) => this.frameOut(slot, f),
      () => this.pump(),
    );
    return slot;
  }

  // The Go side's frames for this episode, decoded at the size of the
  // canvas and no larger than the file's own.
  private fromApp(): AppFrames {
    const path = new URL(this.reader.url, location.href).searchParams.get("path") ?? "";
    const v = this.video!;
    const app = new AppFrames(path, v, () => this.previewSize(), (why) =>
      this.fault(`The app could not decode the picture: ${why.replace(/\.$/, "")}.`),
    );
    app.onPassing = (rank, frame, seq) => this.passing(rank, frame, seq);
    // For the walks, which read how the frames came.
    (window as unknown as { __appFrames?: AppFrames }).__appFrames = app;
    return app;
  }

  // The size a frame is decoded at: the canvas's, no larger than the
  // file's own.
  private previewSize(): { width: number; height: number } {
    const v = this.video!;
    let w = this.canvas.width;
    let h = this.canvas.height;
    // Not laid out yet: a size that is sharp in most windows.
    if (w < 64 || h < 64) [w, h] = [1280, 720];
    const scale = Math.min(1, w / v.width, h / v.height);
    const even = (n: number) => Math.max(2, Math.round(n / 2) * 2);
    return { width: even(v.width * scale), height: even(v.height * scale) };
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
    if (this.relooped(pieces, loop)) return;
    // A running play is never changed in place, rule 6 of
    // docs/VIDEO-PREVIEW.md: it plays the program it started with, and the
    // program asked for now is what the next play plays, the space bar's
    // or a click's on the clip timeline. A clip edge or a cut dragged
    // while the clip played started the play over at every step, and the
    // picture, the sound and the playhead stuttered with the hand.
    if (this.state === "playing" || this.state === "starting") {
      const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
      this.asked = JSON.stringify([copy, loop]) === this.wantedKey ? null : [pieces, loop];
      return;
    }
    this.asked = null;
    if (!this.want(pieces, loop)) return;
    if (this.state === "cued" || this.vplan) this.seek(this.at);
  }

  // The program asked for while a play ran, for the next play, see
  // setProgram. Taken as the program, it says whether it is another.
  private takePending(): boolean {
    const p = this.asked;
    this.asked = null;
    return p ? this.want(...p) : false;
  }

  // Only the loop switched, with the same pieces, while a play starts or
  // plays: the play goes on as it is, because the loop says nothing until
  // the end of the clip. Started again for it, the play threw away what
  // was decoded and the picture and the sound stopped for a moment. Only
  // in the last moments of a time through, once what comes after its end
  // is worked out already, is it started again.
  private relooped(pieces: Piece[] | null, loop: boolean): boolean {
    if (this.state !== "playing" && this.state !== "starting") return false;
    const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
    if (JSON.stringify([copy, !loop]) !== this.wantedKey) return false;
    if (this.vplan && this.built) {
      if (loop) this.built.loopOn();
      else this.built.loopOff(this.reached);
      if (!this.vplan.relooped() || (this.aplan && !this.aplan.relooped())) return false;
    } else {
      // Not worked out yet: the play starting builds it from what is wanted.
      this.built = null;
    }
    this.wanted = { pieces: copy, loop };
    this.wantedKey = JSON.stringify([copy, loop]);
    return true;
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
    // A seek is a gesture of its own, so it plays what is asked for now.
    if (program) this.asked = null;
    const changed = program ? this.want(...program) : this.takePending();
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
  // where it is not, in a cut of the clip or past its end. On the clip's
  // end itself it is the clip's last frame, the one that holds the moment
  // a frame before the end, where a play of the clip stops and the short
  // ends, see end.
  private settle(at: number) {
    const ticket = this.ticket;
    this.ready.then(
      () => {
        if (ticket !== this.ticket || this.closed) return;
        const s = this.video!.samples;
        const program = this.program;
        const p0 = program.place(at);
        const there = p0 < program.length ? program.locate(p0) : null;
        const last = program.pieces[program.pieces.length - 1];
        if (there && rankAt(s, there.at) === rankAt(s, at)) this.start(at, true);
        else if (last && at === last.end) void this.still(at, Math.max(last.start, at - this.video!.frame));
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
    // What was asked for while the last play ran is what this one plays,
    // so a play paused or cued on the program before starts again.
    const changed = this.takePending();
    // Paused in the middle of a play: the play is all still there, the
    // sound card was only held.
    if (!changed && this.vplan && this.c0 !== null && this.state === "paused") {
      clearTimeout(this.suspendTimer);
      this.state = "playing";
      this.ramp(1);
      this.report();
      this.loop();
      return;
    }
    // Cued: everything is there but the clock.
    if (!changed && this.state === "cued") {
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
      s.decoder.close();
    }
    this.app?.close();
    if (this.shown) this.closeFrame(this.shown.frame);
    this.shown = null;
    this.soundDecoder?.close();
    if (this.audio && this.audio.state !== "closed") void this.audio.close();
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

  // Draws a frame and closes the one it replaces. One passing is put up
  // on the way to the frame asked for, see passing.
  private put(frame: VideoFrame, run: number, rank: number, passing = false) {
    if (this.shown && this.shown.frame !== frame) this.closeFrame(this.shown.frame);
    this.shown = { frame, run, rank };
    if (!passing) {
      this.putFor = this.ticket;
      this.shownAsked = this.app?.asked ?? 0;
    }
    this.paint();
    this.stats.drawn++;
  }

  // A frame that came for a place a paused drag has passed, while the frame
  // for where the hand is now is still on its way: it is put up meanwhile,
  // unless a frame asked for later is on screen already. Every move of the
  // hand is a seek, which withdraws the frame asked for by the seek before,
  // and a frame takes longer to come than the hand takes to move, 30 to
  // 50 ms against 16. So every frame came for a place already left, none
  // was drawn, and the picture stood still for the whole drag, found by
  // Tim on start.mp4. Nothing is reported: the playhead is the hand's.
  private passing(rank: number, frame: VideoFrame, seq: number) {
    if (this.closed || (this.state !== "paused" && this.state !== "cued") || this.putFor === this.ticket) return;
    if (seq <= this.shownAsked) return;
    this.shownAsked = seq;
    if (this.shown?.rank === rank) return;
    this.stats.open++;
    this.put(new VideoFrame(frame, { timestamp: frame.timestamp }), -1, rank, true);
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

  // The frame drawn is the one that holds the moment it is given, the
  // playhead unless said otherwise.
  private async still(at: number, holds = at) {
    try {
      await this.ready;
    } catch {
      return;
    }
    const ticket = ++this.ticket;
    const slot = this.slots[0];
    this.release(slot);
    const s = this.video!.samples;
    const feed = stillFeed(s, holds);
    // Already on screen: nothing to decode.
    if (this.shown && this.shown.rank === feed.rank) {
      this.putFor = ticket;
      this.at = at;
      this.report();
      return;
    }
    slot.run = { id: -1 - ticket, key: feed.key, last: feed.last, kFirst: 0, kLast: 0, lastRank: feed.rank };
    slot.firstRank = feed.rank;
    for (let i = feed.key; i <= feed.last; i++) {
      if (ticket !== this.ticket || slot.decoder.state !== "configured") return;
      const ts = stamp(0, s.pts[i] / s.timescale);
      slot.fed.set(ts, s.rank[i]);
      slot.decoder.decode(ts, s.key[i] === 1, NO_DATA, s.rank[i], s.rank[i] === feed.rank, feed.rank);
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
    if (slot.decoder.state === "configured" && !idle) slot.decoder.reset();
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
      this.putFor = this.ticket;
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

  // The page's frames stop while the app is behind another window, and with
  // them the tick that feeds the sound: the sound ran out a second later
  // and came back only with the app, whenever any of it showed. So while no
  // frame comes, a timer ticks instead, and the sound is fed further ahead,
  // since a timer behind another window may be held to once a second.
  private lastFrame = 0;
  private keeper = 0;
  private behind = false;

  private loop() {
    // One frame asked for at a time: a tick from the timer asks again, and
    // a frame left waiting from before would tick twice once it comes.
    cancelAnimationFrame(this.frameRequest);
    this.frameRequest = requestAnimationFrame((now) => {
      this.lastFrame = performance.now();
      this.behind = false;
      this.tick(now);
    });
    if (!this.keeper) {
      this.keeper = window.setInterval(() => this.keepOn(false), 250);
      document.addEventListener("visibilitychange", this.hidden);
    }
  }

  // The app going behind another window is said as the page going hidden,
  // and the timer's first tick after that can come a second late, as late
  // as the sound fed for the frames runs out. So the sound is fed further
  // ahead the moment the page is hidden.
  private hidden = () => {
    if (document.hidden) this.keepOn(true);
  };

  private keepOn(now: boolean) {
    if (this.state !== "playing" || this.closed) {
      clearInterval(this.keeper);
      document.removeEventListener("visibilitychange", this.hidden);
      this.keeper = 0;
      this.behind = false;
      return;
    }
    const at = performance.now();
    if (!now && at - this.lastFrame < 400) return;
    this.behind = true;
    this.tick(at);
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
    // The frame due, but never past a piece's first or last frame without
    // drawing it: a frame at the edge of a piece can be due for less than a
    // display frame, and the frame before a cut is the one that says where
    // the cut is.
    const k = Math.max(this.k, Math.min(plan.gridAt(pos), plan.nextEdge(this.k)));
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
    // Anything that plays or seeks after the end clears this first.
    this.suspendTimer = window.setTimeout(() => {
      if (this.state === "ended") void this.audio.suspend();
    }, 300);
    // Stopped at the end, the picture is the last frame of what played.
    // The frames are drawn on a grid of frames from where the play began,
    // so a play that began inside a frame reaches the end with the frame
    // before the last on screen, and the last never drawn: the playback
    // walk found it, the frame 24.40 standing on a clip that ended at
    // 24.72. Nearly always it is decoded already, and where it is not, it
    // is decoded on its own, as a paused frame is.
    if (!last || !this.video) return;
    // The frame that holds the moment a frame before the end, see frameAt
    // in lib/flow.ts.
    const holds = Math.max(last.start, last.end - this.video.frame);
    const want = rankAt(this.video.samples, holds);
    if (this.shown?.rank === want) return;
    for (const slot of this.slots) {
      const i = slot.ready.findIndex((h) => h.rank === want);
      if (i < 0 || !slot.run) continue;
      const [h] = slot.ready.splice(i, 1);
      this.put(h.frame, slot.run.id, h.rank);
      this.report(true);
      return;
    }
    void this.still(this.at, holds);
  }

  private slotOf(run: number): Slot | null {
    return this.slots.find((s) => s.run?.id === run) ?? null;
  }

  // ---- Feeding the picture

  private pump() {
    const plan = this.vplan;
    if (!plan || (this.state !== "starting" && this.state !== "playing" && this.state !== "cued")) return;
    // Put runs on free decoders: the run being drawn, and the next one
    // once it is close.
    for (const run of plan.runs) {
      if (run.kLast < this.k || this.slotOf(run.id)) continue;
      if (plan.at(run.kFirst) - plan.at(this.k) > NEXT_AHEAD) break;
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
      const ts = stamp(run.id, s.pts[i] / s.timescale);
      const rank = s.rank[i];
      slot.fed.set(ts, rank);
      const wanted = rank >= slot.firstRank && rank <= run.lastRank;
      if (wanted) slot.coming++;
      slot.decoder.decode(ts, s.key[i] === 1, NO_DATA, rank, wanted, slot.firstRank);
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
    const ahead = (this.c0 === null ? SOUND_FIRST * 2 : this.behind ? SOUND_BEHIND : SOUND_AHEAD) * rate;
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
      const ts = stamp(run.id, s.pts[j] / s.timescale);
      this.soundFed.push({ ts, run, j, length: (s.duration[j] / s.timescale) * 1e6 });
      this.soundNext++;
      const p = plan.packet(j);
      for (const sl of plan.slices(run, j, p.n)) this.soundReach = Math.max(this.soundReach, sl.out + (sl.to - sl.from));
      dec.decode(ts, NOTHING, p.at, p.n);
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
    const parts = plan.decoded(fed.run, fed.j, d.numberOfFrames);
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
