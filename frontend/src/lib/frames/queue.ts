// The frame queue: the episode played onto one canvas with the sound
// card's clock, so a clip plays through its cuts the way the render does.
// Every frame and every sample comes from ffmpeg on the Go side, the
// episode's decoder, see docs/VIDEO-PREVIEW.md. The browser decodes
// nothing. Everything this file decides is decided in plan.ts, with tests.
// This is only the glue: asking the Go side for frames and sound on the
// frames the engine names, see movie.ts, drawing, and scheduling the
// sound.
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
import type { Picture } from "./picture";
import type { Over } from "./captions";
import type { Light } from "./light";
import { screenOf, type Screen } from "./screen";
import { describeEvenness, Even, type Source } from "./even";
import { frames as framePool } from "./pool";
import { rankAt, type AudioTrack, type Movie, type VideoTrack } from "./movie";
import {
  AudioPlan,
  fade,
  FADE,
  heard,
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
  // The frame drawn is one passing on the way to the playhead under a hand
  // that drags it, see passing: only frame is news, the playhead is the
  // hand's.
  passing?: boolean;
};

export type Stats = {
  // Frames open now, and the most there ever were.
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
  // The longest a frame a gesture put the playhead on waited for what lies
  // over it, milliseconds, see dress.
  dressWait: number;
  // Frames a gesture put the playhead on that were put up without what
  // lies over it, because it did not come in DRESS_WAIT, see dress.
  bare: number;
  // Frames a gesture put the playhead on that waited for what lies over
  // it, counted as each starts to wait, so a walk knows the frame is
  // being held for its captions, see dress.
  waitedForDress: number;
  // Plays started, a seek while playing and a new program included. A
  // press of L starts none, see relooped.
  starts: number;
  // Key frames put up on the way to the frame under a hand that drags the
  // playhead, see passing, and where the last 16 begin, for the walks.
  keys: number;
  keysAt: number[];
  // Packets of sound decoded, and stretches handed to the sound card.
  soundDecoded: number;
  soundScheduled: number;
  // What the canvas draws with, webgpu or 2d, see screen.ts, and nothing
  // until it is set up.
  screen: string;
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
// The longest a frame a gesture put the playhead on waits for what lies
// over it, the captions, milliseconds, see dress. A layer comes in tens of
// milliseconds, and one that did not come is asked for once more after a
// quarter of a second, see AGAIN in captions.ts. Past this the frame is put
// up without it: a picture that stands still because its captions are slow
// is worse than captions that come a moment after it.
const DRESS_WAIT = 600;

// A unique timestamp for every sample fed, in the order the samples of
// its run are shown, so a decoder that orders its output by timestamp
// orders it right too. Runs are 100 000 seconds apart.
function stamp(run: number, seconds: number): number {
  return run * 1e11 + 1e10 + Math.round(seconds * 1e6);
}

// Starts the episode's decoder on the Go side, ffmpeg with the file open,
// the moment a workspace starts to open, rather than once the workspace
// has asked the Go side what the episode is: it was the last link of a
// chain, and the first frame waited for it. The decoder is held only
// until it is up: the queue holds it for itself, and one nobody holds
// stands for twenty seconds before it is closed.
export function holdAhead(path: string) {
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
  // first is the rank of the first frame the run draws. place says the
  // frame is the one that holds the playhead, the frame of a still or the
  // first of a cue, rather than one read ahead of it, see AppFrames.frame.
  decode(timestamp: number, key: boolean, data: Uint8Array, rank: number, wanted: boolean, first: number, place?: boolean): void;
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

type Held = { frame: Picture; rank: number };

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

// What the queue plays, in a few words, for the log.
function describe(m: Movie): string {
  const v = m.video;
  const a = m.audio;
  const picture = v
    ? `${v.width}x${v.height}, ${v.samples.count} frames of ${(v.frame * 1000).toFixed(2)} ms, the first at ${(v.samples.pts[0] / v.samples.timescale).toFixed(3)} s`
    : "no picture";
  const sound = a ? `${a.sampleRate} Hz, ${a.channels} channels` : "no sound";
  return `${m.duration.toFixed(3)} s, ${picture}, ${sound}`;
}

// A reason, in a sentence, from whatever went wrong.
function sentence(e: unknown): string {
  const said = e instanceof Error ? e.message : String(e);
  const text = said.trim().replace(/\.$/, "") || "it could not be read";
  return text[0].toUpperCase() + text.slice(1) + ".";
}

// The queues not closed, oldest first, for the walks: window.__frameQueue
// is the newest of them, the one on the video preview, and the newest made
// once all are closed. A workspace that went to sleep made a queue after
// the one on screen and closed it, and the walks then listened to a queue
// that was closed, see the walk speed.mjs, which counted presses whose
// frame showed as never shown.
const opened: FrameQueue[] = [];
let newest: FrameQueue | undefined;
Object.defineProperty(globalThis, "__frameQueue", {
  configurable: true,
  get: () => opened[opened.length - 1] ?? newest,
});

export class FrameQueue {
  // Settles once the file is read and the decoders are set up, and fails
  // with the reason in a sentence when the file cannot be played here.
  readonly ready: Promise<void>;
  movie: Movie | null = null;
  private given: Movie;
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
    dressWait: 0,
    bare: 0,
    waitedForDress: 0,
    starts: 0,
    keys: 0,
    keysAt: [],
    soundDecoded: 0,
    soundScheduled: 0,
    screen: "",
    errors: [],
  };

  private canvas: HTMLCanvasElement;
  // Whether the canvas has been told its size, see resize, and the frame
  // and size last asked for again, see sharpen.
  private sized = false;
  private sharpened = "";
  // The frame and size said in the log to have come blurred, once, and
  // the frame said to have been asked for again. A window edge dragged
  // with the video paused asks for the same frame again at every size it
  // passes, and only the first is written down, so the log keeps room.
  private blurTold = "";
  private sharpenTold = -1;
  // Settled the first time the canvas is told its size. No frame is asked
  // for before: a frame is decoded at the canvas's size, and a canvas
  // nobody has sized is 300 by 150. The first frame of every workspace was
  // asked for at that, the stylesheet's size a frame later, and stayed
  // blurred until a play asked for frames again, found by Tim.
  private laidOut: Promise<void>;
  private sizeKnown = () => {};
  // The canvas's screen, kept with the canvas, see screen.ts, and null
  // until it is set up. Whether the canvas holds this episode's own frame
  // from a queue before this one, see the constructor.
  private screen: Screen | null = null;
  private known: boolean;
  private listeners = new Set<(s: Shown) => void>();
  private video: VideoTrack | null = null;
  private sound: AudioTrack | null = null;
  private slots: Slot[] = [];
  private soundDecoder: SoundDecoder | null = null;
  // The Go side's frames, from the episode's decoder: every frame drawn,
  // whatever the file. Null only until the queue is open.
  private app: AppFrames | null = null;
  private closed = false;
  private trouble = "";
  // Every step the queue takes on its way to a picture and a play, for
  // the app's log, see lib/said.ts. Set by the video preview straight
  // after the queue is made.
  trace: (line: string) => void = () => {};
  private born = performance.now();
  // Whether the first frame of the queue, and of the play under way, was
  // said in the log yet.
  private firstTold = false;
  private playTold = false;
  // The counts as the play under way began, so its end says how it went,
  // and how even it was, see even.ts.
  private began: { at: number; drawn: number; late: number; lateSound: number; when: number; even: Even } | null = null;

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
  // The ticket of the newest seek, cue or play that has said how it went,
  // see report and standing.
  private toldFor = -1;
  private shown: { frame: Picture; run: number; rank: number } | null = null;
  // Whether the frame on screen is a key frame put up on the way to the
  // frame under the hand, see passing.
  private shownKey = false;
  // The sound card's time at which the program's sample m0 is played.
  private c0: number | null = null;
  private m0 = 0;
  // What is on the sound card, with the sound card's time each stretch
  // ends at by itself, see clip.
  private sources = new Map<AudioBufferSourceNode, number>();
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
  // Counts the hand's asks, a seek or a play, so frames read ahead for
  // where it rested are asked only while it still rests there.
  private gestures = 0;
  private aheadKey = "";
  // The playhead is held under the hand, being dragged: each place it
  // passes is shown on its own, with no play cued from it, see grip.
  private gripped = false;
  // How a drag of the playhead goes, for the log, see grip: how many
  // frames the frame on screen was from each place the hand moved to, the
  // key frames put up before it, and when the hand let go.
  private drag: { behind: number[]; keys: number; letGo: number } | null = null;
  // The first frame of a cue or a play that is waiting for what lies over
  // it, and the one that has it, each by its ticket and rank, see dressed.
  private dressing = "";
  private dressedKey = "";
  private scratch: Float32Array[] = [];

  // path is the episode's, and movie what the engine says of it, see
  // movie.ts. woken is a workspace woken again, see sleeping in
  // Player.svelte: the canvas then holds this episode's own frame, which
  // stays until the first frame of this queue replaces it.
  constructor(
    canvas: HTMLCanvasElement,
    readonly path: string,
    movie: Movie,
    woken = false,
  ) {
    this.canvas = canvas;
    this.known = woken;
    this.given = movie;
    // For the walks, which listen to what it plays, see opened.
    opened.push(this);
    newest = this;
    this.laidOut = new Promise((done) => (this.sizeKnown = done));
    this.ready = this.open().catch((e) => {
      throw new Error(sentence(e));
    });
    // Whoever needs the reason asks ready for it. Nobody asking is not an
    // error of its own.
    this.ready.catch(() => {});
  }

  private async open() {
    const movie = this.given;
    const screen = await screenOf(this.canvas);
    this.trace(`the canvas draws with ${screen.kind}, ${this.ms()}, and plays ${describe(movie)}`);
    if (this.closed) throw new Error("closed");
    this.screen = screen;
    this.stats.screen = screen.kind;
    screen.onRestored = () => this.paint();
    // Black until the first frame, never the last frame of an episode
    // drawn on this canvas before. A workspace woken again keeps its own.
    if (!this.known) this.paint();
    if (this.closed) throw new Error("closed");
    this.movie = movie;
    this.video = movie.video ?? null;
    this.sound = movie.audio ?? null;
    if (!this.video) throw new Error("the video has no picture the video preview can show");
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
    const card = this.audio;
    this.trace(`sound card at ${card.sampleRate} Hz${this.sound && card.sampleRate !== this.sound.sampleRate ? `, not the ${this.sound.sampleRate} Hz asked` : ""}, ${card.state}`);
    card.onstatechange = () => this.trace(`sound card ${card.state}`);
    for (let i = 0; i < 2; i++) this.slots.push(this.makeSlot());
    // A file that cannot be played has said so by now. What can be waits
    // until the canvas has its size, see laidOut.
    await this.laidOut;
    if (this.closed) throw new Error("closed");
    if (this.sound) await this.openSound(this.sound);
    if (this.closed) throw new Error("closed");
    this.trace(`ready, ${this.ms()}`);
  }

  // How long since the queue was made, for the log.
  private ms(): string {
    return `${Math.round(performance.now() - this.born)} ms after it opened`;
  }

  // The sound, decoded by ffmpeg on the Go side whatever it is, plain
  // sound in a MOV too.
  private async openSound(a: AudioTrack) {
    const output = (d: Sound) => this.soundOut(d);
    const dequeue = () => this.pumpSound();
    this.soundDecoder = new GoSound(this.path, a.sampleRate, Math.min(a.channels, 8), output, (e) => this.fault(`The sound stopped decoding: ${e}.`), dequeue);
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
    const v = this.video!;
    const app = new AppFrames(this.path, v, () => this.previewSize(), (why) =>
      this.fault(`The app could not decode the picture: ${why.replace(/\.$/, "")}.`),
    );
    app.trace = (line) => this.trace(line);
    app.onPassing = (rank, frame, seq, key) => this.passing(rank, frame, seq, key);
    app.paused = () => this.state !== "playing" && this.state !== "starting";
    // For the walks, which read how the frames came.
    (window as unknown as { __appFrames?: AppFrames }).__appFrames = app;
    return app;
  }

  // The size a frame is decoded at: the canvas's, no larger than the
  // file's own.
  private previewSize(): { width: number; height: number } {
    const v = this.video!;
    const scale = Math.min(1, this.canvas.width / v.width, this.canvas.height / v.height);
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

  // Whether the picture standing still is the one the queue leaves there:
  // the newest seek or cue has said how it went, and its frame is at the
  // canvas's size or was asked for at it once already, see sharpen. A
  // queue just opened stands only once its first seek has said how it
  // went, so the first frame is waited for too. A seek that put up no
  // frame, the file could not be read, stands with none. A queue that
  // plays has no picture standing still to wait for, and says true. Each
  // time it comes true the listeners are told, or the canvas has changed
  // its size, or the queue has closed, which tells no one: the video
  // preview it belonged to has gone or has a queue of its own by then. For the walks and the
  // benchmark, which read the picture once it stands, see stillBlurred in
  // walks/rules.mjs. It needs the app's own timers to run, the one after
  // a frame is put up that asks for it again among them, see put.
  get standing(): boolean {
    if (this.closed || (this.state !== "paused" && this.state !== "cued")) return true;
    if (this.toldFor !== this.ticket) return false;
    if (!this.shown || this.sharp(this.shown.frame)) return true;
    const { width, height } = this.previewSize();
    return this.sharpened === `${this.shown.rank} ${width} ${height}`;
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
  // plays: nothing that plays changes, because the loop says only where
  // the play ends, program.end. The play works out what follows the
  // clip's end before it gets there, the next time through with the loop
  // on, and keeps whatever it has worked out when the loop changes. Its
  // end is where it stops: the picture at tick, the sound by each stretch
  // on the sound card stopping there, see clip.
  //
  // Started again for the loop, the play threw away what was decoded, and
  // the picture and the sound stopped for a moment at every press. Kept
  // going without the end moving, the play stood at the clip's end while
  // the sound of its start went on for seconds. Both found by Tim.
  private relooped(pieces: Piece[] | null, loop: boolean): boolean {
    if (this.state !== "playing" && this.state !== "starting") return false;
    const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
    if (JSON.stringify([copy, !loop]) !== this.wantedKey) return false;
    if (this.vplan && this.built) {
      if (loop) this.built.loopOn();
      else this.built.loopOff(Math.max(this.reached, this.sounded()));
      if (loop) {
        // A play that had worked out its end works on past it, the sound
        // too, and a stretch on the sound card stopped at the end plays
        // to its own end again.
        this.vplan.relooped();
        this.aplan?.relooped();
        this.soundDone = !this.aplan;
      }
      for (const src of this.sources.keys()) this.clip(src);
      this.pump();
      this.pumpSound();
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
    this.gestures++;
    this.app?.still();
    // A seek is a gesture of its own, so it plays what is asked for now.
    if (program) this.asked = null;
    const changed = program ? this.want(...program) : this.takePending();
    // A play starting from this very moment on this program is the play
    // asked for. A click seeks as the hand goes down and again as it comes
    // up, and starting over would throw away what was decoded between.
    if (!changed && this.state === "starting" && at === this.startFrom) return;
    this.at = at;
    if (this.gripped && this.drag && this.shown && this.video && this.drag.behind.length < 4096) {
      this.drag.behind.push(Math.abs(this.shown.rank - rankAt(this.video.samples, at)));
    }
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
  //
  // The first frame of the play is the one the play's own plan starts
  // with, worked out the way the cue works it out. Worked out apart, from
  // where the playhead lands on the program, a playhead a hair before the
  // end of a piece was inside it here and in the cut for the plan, which
  // takes a piece that short as none. So the cue put up the first frame
  // after the cut. A drag of a cut's left edge leaves the playhead on the
  // edge just before the piece ends there, at every step, and the video
  // preview flickered between the edge's frame and the one after the cut.
  // Found by Tim on youtube.mp4, plan row 2.270.
  private settle(at: number) {
    const ticket = this.ticket;
    this.ready.then(
      () => {
        if (ticket !== this.ticket || this.closed) return;
        const s = this.video!.samples;
        const program = this.program;
        const p0 = program.place(at);
        const plan = new VideoPlan(s, this.video!.frame, program, p0);
        plan.extend(p0 + this.video!.frame);
        const first = plan.need(0);
        const last = program.pieces[program.pieces.length - 1];
        if (first && first.rank === rankAt(s, at) && !this.gripped) this.start(at, true);
        else if (last && at === last.end) void this.still(at, Math.max(last.start, at - this.video!.frame));
        else void this.still(at);
      },
      // The file could not be read: the seek says so with no frame, so
      // the picture stands, see standing.
      () => {
        if (ticket === this.ticket && !this.closed) this.report();
      },
    );
  }

  // The hand takes hold of the playhead, or lets go of it. While it is
  // held, a place shows its frame alone and cues no play: the play's
  // frames after it and its sound were read for every place the drag
  // passed, and thrown away at the next. Where the hand lets go, the
  // play is cued. Plan row 2.269, from the findings of 2.270.
  grip(held: boolean) {
    if (this.gripped === held) return;
    this.gripped = held;
    this.app?.grip(held);
    if (held) this.drag = { behind: [], keys: this.stats.keys, letGo: -1 };
    else if (this.drag) {
      this.drag.letGo = performance.now();
      if (this.shown && this.video && this.shown.rank === rankAt(this.video.samples, this.at)) this.dragged();
    }
    if (!held && !this.closed && (this.state === "paused" || this.state === "cued")) this.settle(this.at);
  }

  // Says how the drag went, once the frame under the hand is up after it
  // let go, issue #281. A click moves the hand once and says nothing.
  private dragged() {
    const d = this.drag;
    this.drag = null;
    if (!d || d.behind.length < 2) return;
    const behind = [...d.behind].sort((a, b) => a - b);
    this.trace(
      `a drag of the playhead let go at ${this.at.toFixed(3)} s, after ${behind.length} moves: the frame on screen ${behind[Math.floor(behind.length / 2)]} frames from the playhead on the median and ${behind[behind.length - 1]} at most as the hand moved, ` +
        `${this.stats.keys - d.keys} key frames put up on the way, the frame under the hand ${Math.round(performance.now() - d.letGo)} ms after letting go`,
    );
  }

  // The moments the hand is likely to step to next while paused, the first
  // frame of the word after the playhead and of the word before: their
  // frames are read while it rests, see AppFrames.ahead. Each is the first
  // frame a play from it shows, the frame the step's cue asks for.
  ahead(times: number[]) {
    if (this.closed || (this.state !== "paused" && this.state !== "cued")) return;
    const gesture = this.gestures;
    // Asked again for the same moments with no gesture between: the frames
    // are on their way or kept already.
    const key = `${gesture} ${times.join(" ")}`;
    if (key === this.aheadKey) return;
    this.aheadKey = key;
    this.ready.then(
      () => {
        if (gesture !== this.gestures || this.closed || !this.app || this.state === "playing" || this.state === "starting") return;
        const s = this.video!.samples;
        const ranks = times.map((at) => {
          const p0 = this.program.place(at);
          const plan = new VideoPlan(s, this.video!.frame, this.program, p0);
          plan.extend(p0 + this.video!.frame);
          return plan.need(0)?.rank ?? rankAt(s, at);
        });
        this.app.ahead(ranks);
      },
      () => {},
    );
  }

  // The clips on the cards, by their starts, the likeliest to be clicked
  // first: the frame each opens on is read once nothing else is wanted and
  // held for a click on its card, see AppFrames.cards.
  cards(starts: number[]) {
    this.cardsGiven = this.ready.then(
      () => {
        if (this.closed || !this.app) return;
        const s = this.video!.samples;
        this.app.cards(starts.map((at) => rankAt(s, at)));
      },
      () => {},
    );
  }
  // The cards given last, once the frames have them, see rested.
  private cardsGiven: Promise<void> = Promise.resolve();

  // Settles once the frames read ahead are in, the cards' among them, for
  // the walks, see AppFrames.rested.
  async rested(): Promise<void> {
    await this.ready;
    await this.cardsGiven;
    await this.app?.rested();
  }

  play() {
    this.trace(`play asked at ${this.at.toFixed(3)} s, ${this.state}, sound card ${this.audio?.state ?? "not there yet"}`);
    if (this.closed || this.state === "playing" || this.state === "starting") return;
    this.gestures++;
    this.app?.still();
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
      this.trace(`playing on from ${this.at.toFixed(3)} s`);
      this.began = { at: this.at, drawn: this.stats.drawn, late: this.stats.late, lateSound: this.stats.lateSound, when: performance.now(), even: new Even() };
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
    this.trace(`pause asked at ${this.at.toFixed(3)} s, ${this.state}${this.state === "playing" ? `, ${this.howItWent()}` : ""}`);
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
    opened.splice(opened.indexOf(this), 1);
    this.idler?.port1.close();
    // A queue closed before its canvas had a size stops waiting for one.
    this.sizeKnown();
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
    // The buffers kept for the next play are let go of with the video
    // preview, see pool.ts.
    framePool.clear();
  }

  // The canvas has this many device pixels, as the stylesheet laid it out.
  // The frame on it is drawn again to fit.
  //
  // A paused frame decoded at another size than the canvas has now is
  // asked for again at the new one, so the picture is as sharp as the
  // canvas lets it be whenever it stands still: the frame on screen after
  // the app grew or shrank, and one asked for just before. A play asks for every frame at the size of
  // the moment anyway. See sharpen.
  resize(width: number, height: number) {
    const w = Math.max(1, Math.round(width));
    const h = Math.max(1, Math.round(height));
    const first = !this.sized;
    this.sized = true;
    this.sizeKnown();
    // Setting the canvas's size clears it, even to the size it has. A
    // workspace woken again keeps its own frame at the size it had, see
    // known.
    if (this.canvas.width === w && this.canvas.height === h && (!first || this.known)) return;
    this.canvas.width = w;
    this.canvas.height = h;
    this.paint();
    this.sharpen();
  }

  // Whether a frame was decoded at the size the canvas asks for now.
  private sharp(f: Picture): boolean {
    if (!this.video) return true;
    const { width, height } = this.previewSize();
    return f.width === width && f.height === height;
  }

  // The frame standing on the canvas, asked for again at the canvas's size
  // where it came at another. It is asked for once for each frame and
  // size, so a decoder that answers at a size of its own is not asked
  // again and again.
  private sharpen() {
    if (this.closed || (this.state !== "paused" && this.state !== "cued") || !this.shown) return;
    const f = this.shown.frame;
    if (this.sharp(f)) return;
    const { width, height } = this.previewSize();
    const key = `${this.shown.rank} ${width} ${height}`;
    if (key === this.sharpened) {
      // Asked for again at this size and come back at another, once the
      // ask has said how it went: it stays as it came.
      if (this.toldFor === this.ticket && this.blurTold !== key) {
        this.blurTold = key;
        this.trace(`frame ${this.shown.rank} came again at ${f.width}x${f.height}, not the ${width}x${height} asked for, and stays`);
      }
      return;
    }
    this.sharpened = key;
    if (this.sharpenTold !== this.shown.rank) {
      this.sharpenTold = this.shown.rank;
      this.trace(`frame ${this.shown.rank} is up at ${f.width}x${f.height}, asked for again at the canvas's ${width}x${height}`);
    }
    this.seek(this.at);
  }

  // The frame on the canvas, in its own pixels, for a colour taken from the
  // picture, or null.
  picture(): Picture | null {
    return this.shown?.frame ?? null;
  }

  private ramp(to: number) {
    const now = this.audio.currentTime;
    this.out.gain.cancelScheduledValues(now);
    this.out.gain.setValueAtTime(this.out.gain.value, now);
    this.out.gain.linearRampToValueAtTime(to, now + FADE);
  }

  private fault(what: string) {
    this.trace(`fault: ${what}`);
    this.stats.errors.push(what);
    console.error("frame queue:", what);
    if (this.closed) return;
    this.trouble = what;
    this.report();
  }

  private emit(s: Shown) {
    for (const fn of this.listeners) fn(s);
  }

  private report(drew = false, passing = false) {
    // A frame passing is put up on the way to the one asked for.
    if (!passing) this.toldFor = this.ticket;
    const playing = this.state === "playing" || this.state === "starting";
    // A play that stopped lets the cards' frames be read again, see
    // AppFrames.paused.
    if (!playing) this.app?.aheadNow();
    this.emit({
      at: this.at,
      frame: this.shown ? this.shownAt(this.shown.rank) : NaN,
      drew,
      playing,
      ended: this.state === "ended",
      drawnAt: performance.now(),
      trouble: this.trouble,
      passing,
    });
  }

  private closeFrame(f: Picture) {
    f.close();
    this.stats.open--;
  }

  // Draws a frame and closes the one it replaces. One passing is put up
  // on the way to the frame asked for, see passing.
  private put(frame: Picture, run: number, rank: number, passing = false) {
    if (!this.firstTold) {
      this.firstTold = true;
      this.trace(`first frame drawn, frame ${rank} of ${frame.width}x${frame.height}${frame.light ? `, ${frame.light}` : ""}, ${this.ms()}`);
    }
    if (this.shown && this.shown.frame !== frame) this.closeFrame(this.shown.frame);
    this.shown = { frame, run, rank };
    this.shownKey = false;
    if (!passing) {
      this.putFor = this.ticket;
      this.shownAsked = this.app?.asked ?? 0;
      if (this.drag && this.drag.letGo >= 0) this.dragged();
      // A frame asked for before the canvas last changed its size comes at
      // the size it was asked at.
      setTimeout(() => this.sharpen());
    }
    this.paint(true);
    this.stats.drawn++;
  }

  // A frame that came for a place a paused drag has passed, while the frame
  // for where the hand is now is still on its way: it is put up meanwhile,
  // unless a frame asked for later is on screen already. Every move of the
  // hand is a seek, which withdraws the frame asked for by the seek before,
  // and a frame takes longer to come than the hand takes to move, 30 to
  // 50 ms against 16. So every frame came for a place already left, none
  // was drawn, and the picture stood still for the whole drag, found by
  // Tim on start.mp4. The playhead is the hand's, so the report says only
  // which frame is up: what stands over the picture, and how far it is
  // turned, follows the frame. Tim saw a part rotated in the clip turn
  // with the hand while the frame from before its cut was still up, #188.
  //
  // A key frame on the way to the frame under the hand comes too, see
  // AppFrames.route, issue #281. It can be up to two seconds from the
  // hand, so it is put up only where it is nearer the playhead than the
  // frame on screen, and once it is up, a frame passing has to be nearer
  // still. It says nothing of the ask it came for, whose own frame still
  // comes and replaces it, so a key frame never stays once the hand rests.
  private passing(rank: number, frame: Picture, seq: number, key = false) {
    if (this.closed || (this.state !== "paused" && this.state !== "cued") || this.putFor === this.ticket) return;
    if (seq <= this.shownAsked) return;
    if ((key || this.shownKey) && this.shown && !this.nearer(rank, this.shown.rank)) return;
    if (!key) this.shownAsked = seq;
    if (this.shown?.rank === rank) return;
    this.stats.open++;
    if (key) {
      this.stats.keys++;
      this.stats.keysAt.push(this.shownAt(rank));
      if (this.stats.keysAt.length > 16) this.stats.keysAt.shift();
    }
    this.put(frame.clone(), -1, rank, true);
    this.shownKey = key;
    this.report(true, true);
  }

  // Whether the frame of rank a is nearer the playhead than that of b.
  private nearer(a: number, b: number): boolean {
    if (!this.video) return false;
    const at = rankAt(this.video.samples, this.at);
    return Math.abs(a - at) < Math.abs(b - at);
  }

  // What lies over the frames, the captions: asked for each frame as it
  // is drawn, by the moment of the episode it starts at, and told of each
  // frame on its way to the screen a few frames ahead, so what lies over it
  // can be there when it is drawn, with the light of the frame, which
  // decides how a colour laid over it is shown. See captions.ts.
  //
  // A frame a gesture put the playhead on, paused, waits to be put up until
  // what lies over it is here, ready, so the picture and its captions come
  // in one frame, see dress. At says with fresh whether the frame is put up
  // for the newest gesture, or is the frame on screen drawn again: until a
  // clip just picked has its frame up, the frame on screen is the clip
  // before's and is drawn with what lay over it then. Issue #383.
  over: {
    at(frame: number, light: Light, fresh: boolean): Over | null;
    coming(frame: number): void;
    ready?(frame: number): Promise<void> | null;
  } | null = null;
  // Starts the wait of a frame for what lies over it, see dress, and says
  // how to call it off: late is told once DRESS_WAIT has passed on the
  // browser's clock. A walk hands in a wait that it ends itself, so a
  // frame whose captions it holds back waits for them on any machine,
  // where the walk's own steps on a busy one used up the 600 ms, issue
  // #468.
  dressWait: (late: () => void) => () => void = (late) => {
    const timer = setTimeout(late, DRESS_WAIT);
    return () => clearTimeout(timer);
  };

  // The frame on screen, see screen.ts. Fresh when it is put up for the
  // newest gesture, see over.
  private paint(fresh = false) {
    const s = this.shown;
    this.screen?.draw(s?.frame ?? null, s && this.over ? this.over.at(this.shownAt(s.rank), s.frame.light, fresh) : null);
  }

  // The frame on screen drawn again, for what lies over it has changed.
  // Before this queue has a frame on screen there is nothing to draw it
  // over: the canvas holds black from open, or the frame of a workspace
  // woken again, which stays until this queue's first frame replaces it.
  // Drawn then, it went black for as long as the first frame took, each
  // time a video whose frame had captions was gone back to.
  repaint() {
    if (!this.closed && this.shown) this.paint();
  }

  // Whether what lies over the frame of rank is here, asked for at once
  // where it is not: a promise of whether the frame is still wanted once
  // it is, or once DRESS_WAIT has passed, or null when there is nothing to
  // wait for. Nothing waits while the hand drags the playhead, where each
  // place is passed in a moment and the picture follows the hand.
  private dress(rank: number): Promise<boolean> | null {
    if (this.gripped || !this.over?.ready || !this.video) return null;
    const came = this.over.ready(this.shownAt(rank));
    if (!came) return null;
    const ticket = this.ticket;
    const asked = performance.now();
    this.stats.waitedForDress++;
    let stop = () => {};
    const late = new Promise<"late">((done) => (stop = this.dressWait(() => done("late"))));
    return Promise.race([came.then(() => "came" as const), late]).then((how) => {
      stop();
      const wanted = ticket === this.ticket && !this.closed;
      if (how === "late" && wanted) {
        this.stats.bare++;
        this.trace(`the captions of frame ${rank} did not come in ${DRESS_WAIT} ms, the frame is put up without them`);
      }
      else if (wanted) this.stats.dressWait = Math.max(this.stats.dressWait, Math.round(performance.now() - asked));
      return wanted;
    });
  }

  // Whether the first frame of the cue or play under way, of rank, may be
  // put up: what lies over it is here, or nothing has to be. Asked the
  // first time, it asks for it and puts the frame up through maybeBegin
  // once it comes.
  private dressed(rank: number): boolean {
    const key = `${this.ticket} ${rank}`;
    if (this.dressedKey === key) return true;
    if (this.dressing === key) return false;
    const waits = this.dress(rank);
    if (!waits) {
      this.dressedKey = key;
      return true;
    }
    this.dressing = key;
    void waits.then((wanted) => {
      if (this.dressing === key) this.dressing = "";
      if (!wanted) return;
      this.dressedKey = key;
      this.maybeBegin();
    });
    return false;
  }

  // The moment the frame starts that a clip starting at a moment opens on,
  // once the video is read: the frame that holds it, which a cue from the
  // clip's start puts up first, see settle. Null before.
  firstFrameAt(at: number): number | null {
    return this.video && !this.closed ? this.shownAt(rankAt(this.video.samples, at)) : null;
  }

  private shownAt(rank: number): number {
    const s = this.video!.samples;
    return s.pts[s.order[rank]] / s.timescale;
  }

  // ---- A paused frame on its own

  // The frame drawn is the one that holds the moment it is given, the
  // playhead unless said otherwise.
  //
  // A seek made while it waits for the video to be read is newer, and the
  // still gives way to it. It took a ticket of its own after the wait, which
  // made the newer seek's own stale, and the still then put the playhead
  // back where it had been asked for: a drag that left the playhead on an
  // edge for a moment kept it there, a frame off the edge's frame.
  private async still(at: number, holds = at) {
    const asked = this.ticket;
    try {
      await this.ready;
    } catch {
      return;
    }
    if (asked !== this.ticket || this.closed) return;
    const ticket = ++this.ticket;
    // A still given up on with no newer seek to take its place still says
    // how it went, with no frame, or the picture would never stand.
    const gaveUp = () => {
      if (ticket === this.ticket && !this.closed) this.report();
    };
    const slot = this.slots[0];
    this.release(slot);
    const s = this.video!.samples;
    const feed = stillFeed(s, holds);
    // What lies over the frame is asked for as the frame is, so the two
    // come in the time the slower takes, not one after the other.
    const dressed = this.dress(feed.rank);
    // Already on screen, at the canvas's size: nothing to decode. What
    // lies over it may be another clip's still, see over.
    if (this.shown && this.shown.rank === feed.rank && this.sharp(this.shown.frame)) {
      if (dressed && !(await dressed)) return gaveUp();
      this.putFor = ticket;
      this.at = at;
      this.paint(true);
      this.report();
      return;
    }
    slot.run = { id: -1 - ticket, key: feed.key, last: feed.last, kFirst: 0, kLast: 0, lastRank: feed.rank };
    slot.firstRank = feed.rank;
    for (let i = feed.key; i <= feed.last; i++) {
      if (ticket !== this.ticket || slot.decoder.state !== "configured") return gaveUp();
      const ts = stamp(0, s.pts[i] / s.timescale);
      slot.fed.set(ts, s.rank[i]);
      const holds = s.rank[i] === feed.rank;
      slot.decoder.decode(ts, s.key[i] === 1, NO_DATA, s.rank[i], holds, feed.rank, holds);
    }
    try {
      await slot.decoder.flush();
    } catch {
      return gaveUp();
    }
    if (ticket !== this.ticket) return;
    // The frame waits for what lies over it, and a seek made meanwhile
    // takes the decoder and closes what it holds.
    if (dressed && !(await dressed)) return gaveUp();
    const got = slot.ready.findIndex((h) => h.rank === feed.rank);
    let drew = false;
    if (got >= 0) {
      const [h] = slot.ready.splice(got, 1);
      this.put(h.frame, -1, h.rank);
      drew = true;
    } else this.paint(true);
    this.release(slot);
    this.at = at;
    this.report(drew);
  }

  // ---- Playing

  private stopPlay() {
    this.ticket++;
    cancelAnimationFrame(this.frameRequest);
    clearTimeout(this.suspendTimer);
    for (const src of this.sources.keys()) {
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
    if (!cue) this.stats.starts++;
    const ticket = this.ticket;
    this.state = cue ? "cued" : "starting";
    if (!cue) {
      this.playTold = false;
      this.trace(`starting a play from ${from.toFixed(3)} s`);
      const asked = performance.now();
      // A play that has not started after a while says what it waits for.
      for (const after of [3, 10])
        setTimeout(() => {
          if (ticket === this.ticket && this.state === "starting" && !this.closed)
            this.trace(`the play from ${from.toFixed(3)} s has not started ${Math.round((performance.now() - asked) / 1000)} s after it was asked: ${this.waitingFor()}`);
        }, after * 1000);
    }
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
        // What lies over the first frame is asked for as it is decoded.
        const first = this.vplan.need(0);
        if (first) this.dressed(first.rank);
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
    // The frame on screen is the one wanted when it is at the canvas's
    // size too: a cue asked for again by sharpen has the same run and rank
    // as the frame it replaces.
    let frameIn =
      !need || (!!this.shown && this.shown.run === need.run && this.shown.rank === need.rank && this.sharp(this.shown.frame));
    // The first frame comes with what lies over it, see dress.
    if (need && !frameIn && slot?.ready[0]?.rank === need.rank && this.dressed(need.rank)) {
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
      if (need && !this.dressed(need.rank)) return;
      this.firstShown = true;
      this.putFor = this.ticket;
      this.drawnFor = this.vplan.moment(0) ?? this.at;
      // Drawn again with what lies over it now, which may be another
      // clip's still, see over.
      this.paint(true);
      if (this.state === "cued") this.report();
    }
    if (this.state === "cued") return;
    const soundIn =
      this.soundDone || this.soundReach - this.m0 >= SOUND_FIRST * this.sampleRate || (this.aplan?.finished && this.soundRun === null);
    if (!frameIn || !soundIn) return;
    if (this.audio.state !== "running") {
      const ticket = this.ticket;
      if (!this.playTold) {
        this.playTold = true;
        this.trace(`the first frame and sound are in, waiting for the sound card, which is ${this.audio.state}`);
      }
      void this.audio.resume().then(
        () => {
          if (ticket === this.ticket) this.maybeBegin();
        },
        (e: unknown) => this.trace(`the sound card would not start: ${e instanceof Error ? e.message : String(e)}`),
      );
      return;
    }
    const rate = this.audio.sampleRate;
    this.c0 = Math.ceil((this.audio.currentTime + START_LEAD) * rate) / rate;
    this.state = "playing";
    this.trace(`playing from ${this.startFrom.toFixed(3)} s`);
    this.began = { at: this.startFrom, drawn: this.stats.drawn, late: this.stats.late, lateSound: this.stats.lateSound, when: performance.now(), even: new Even() };
    if (this.pending) {
      this.unscheduled.push(this.pending);
      this.pending = null;
    }
    for (const c of this.unscheduled) this.schedule(c);
    this.unscheduled = [];
    this.tick(performance.now());
  }

  // How the play under way went, for the log: how long it ran, how many
  // frames it drew and how many came too late to be drawn on time.
  private howItWent(): string {
    const b = this.began;
    if (!b) return "no play under way";
    const ran = (performance.now() - b.when) / 1000;
    // The log never stands in the way of a pause or an end: what it could
    // not work out, it says instead.
    let even: string;
    try {
      even = describeEvenness(b.even.evenness);
    } catch (e) {
      even = `how even it was could not be worked out: ${e instanceof Error ? e.message : String(e)}`;
    }
    return `it ran ${ran.toFixed(1)} s, drew ${this.stats.drawn - b.drawn} frames, ${this.stats.late - b.late} frames late, ${this.stats.lateSound - b.lateSound} pieces of sound late, the frame on screen is for ${this.drawnFor.toFixed(3)} s, ${even}`;
  }

  // What a play that has not started waits for, for the log.
  private waitingFor(): string {
    const need = this.vplan?.need(0);
    const slot = need ? this.slotOf(need.run) : null;
    const frameIn = !need || (!!this.shown && this.shown.run === need.run && this.shown.rank === need.rank);
    const sound = this.soundDone ? "all of it" : `${((this.soundReach - this.m0) / this.sampleRate).toFixed(3)} s of ${SOUND_FIRST} s`;
    return [
      this.vplan ? `first frame ${need ? `${need.rank}, ${frameIn ? "in" : "not in"}` : "none needed"}` : "no plan yet, the video is not read",
      `decoder ${slot ? `holding ${slot.ready.length} frames, ${slot.coming} coming, ${slot.decoder.decodeQueueSize} asked` : "none on it"}`,
      `sound ${sound}, its decoder ${this.soundDecoder ? `${this.soundDecoder.state}, ${this.soundDecoder.decodeQueueSize} asked` : "none"}`,
      `sound card ${this.audio?.state ?? "not there"}`,
      `trouble "${this.trouble}"`,
      `queue ${JSON.stringify(this.stats)}`,
      `frames ${JSON.stringify(this.app?.stats ?? null)}, read on the ${this.app?.puller.where ?? "nothing"}`,
    ].join(", ");
  }

  // Where the program is now, from the sound being heard, at the moment a
  // frame drawn now is on screen, a refresh of the screen from now, see
  // refreshNow in even.ts.
  private position(now: number): number {
    return this.heardOn(now).pos;
  }

  // The same, with where the moment heard came from, for the log.
  private heardOn(now: number): { pos: number; source: Source } {
    if (this.c0 === null || !this.vplan) return { pos: this.vplan?.p0 ?? 0, source: "plain" };
    const stampNow = this.audio.getOutputTimestamp ? this.audio.getOutputTimestamp() : null;
    const latency = (this.audio.outputLatency || 0) + (this.audio.baseLatency || 0);
    const h = heard(
      stampNow && stampNow.contextTime !== undefined && stampNow.performanceTime !== undefined
        ? { contextTime: stampNow.contextTime, performanceTime: stampNow.performanceTime }
        : null,
      this.audio.currentTime,
      latency,
      now + (this.began?.even.refreshNow ?? 1000 / 60),
    );
    return { pos: this.m0 / this.sampleRate + (h.at - this.c0), source: h.source };
  }

  // How far on the program the sound card has played, heard or about to
  // be: what it has played can no longer be stopped. The loop switched off
  // ends the play at the end of the time through this is in, not the one
  // the playhead is in. The playhead moves on only with the page's frames,
  // so on a busy machine the sound had crossed the clip's end while the
  // playhead still stood before it, the stop came too late, and a moment
  // of the clip's start was heard after its end. Plan row 2.253.
  private sounded(): number {
    if (this.c0 === null) return this.reached;
    return this.m0 / this.sampleRate + (this.audio.currentTime - this.c0);
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
    const begun = performance.now();
    const plan = this.vplan;
    // Never back: the sound heard only ever moves on, and a stamp that says
    // a hair less than the last one is the clock's jitter.
    const h = this.heardOn(now);
    const pos = Math.max(h.pos, this.reached);
    this.reached = pos;
    if (this.behind) this.began?.even.behind();
    else this.began?.even.tick(now, pos, h.source);
    plan.extend(pos + READ_AHEAD);
    this.aplan?.extend(pos + READ_AHEAD);
    // The frame due, but never past a piece's first or last frame without
    // drawing it: a frame at the edge of a piece can be due for less than a
    // display frame, and the frame before a cut is the one that says where
    // the cut is.
    const k = Math.max(this.k, Math.min(plan.gridAt(pos), plan.nextEdge(this.k)));
    if (pos >= this.program.end || (plan.finished && k > plan.lastK)) {
      this.end();
      return;
    }
    this.k = k;
    let drew = false;
    let put: number | undefined;
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
            const putting = performance.now();
            this.put(h.frame, need.run, h.rank);
            put = performance.now() - putting;
            this.drawnFor = plan.moment(k) ?? this.drawnFor;
            drew = true;
            this.began?.even.put(now, plan.at(k));
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
    const telling = performance.now();
    this.emit({
      at: this.at,
      frame: this.shown ? this.shownAt(this.shown.rank) : NaN,
      drew,
      playing: true,
      ended: false,
      drawnAt: now,
      trouble: this.trouble,
    });
    const feeding = performance.now();
    // A run whose frames are all behind the playhead is done with.
    for (const s of this.slots) if (s.run && s.run.id >= 0 && s.run.kLast < k) this.release(s);
    plan.forget(k);
    this.aplan?.forget(Math.round(pos * this.sampleRate));
    this.pump();
    this.pumpSound();
    const done = performance.now();
    if (!this.behind && this.began) {
      const i = this.began.even.work({ late: begun - now, put, tell: feeding - telling, feed: done - feeding, all: done - begun });
      this.idle(this.began.even, i, done);
    }
    this.loop();
  }

  // When the page next has nothing to do after an animation frame: a
  // message posted to itself is taken only once the page has drawn what
  // the frame changed and done whatever else was waiting, see Work in
  // even.ts.
  private idler: MessageChannel | null = null;
  private idling: { even: Even; i: number; from: number }[] = [];
  private idle(even: Even, i: number, from: number) {
    if (!this.idler) {
      this.idler = new MessageChannel();
      this.idler.port1.onmessage = () => {
        const w = this.idling.shift();
        w?.even.drawn(w.i, performance.now(), w.from);
      };
    }
    this.idling.push({ even, i, from });
    this.idler.port2.postMessage(0);
  }

  private end() {
    const plan = this.vplan!;
    const last = this.program.pieces[this.program.pieces.length - 1];
    this.state = "ended";
    this.at = last ? last.end : this.at;
    this.k = plan.lastK;
    this.trace(`played to the end, ${this.at.toFixed(3)} s, ${this.howItWent()}`);
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
    // The frame that holds the moment the play starts from. Every other
    // frame is read ahead of it.
    const first = plan.need(0);
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
      const place = wanted && first?.run === run.id && first.rank === rank;
      slot.decoder.decode(ts, s.key[i] === 1, NO_DATA, rank, wanted, slot.firstRank, place);
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

  private frameOut(slot: Slot, frame: Picture) {
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
      if (rank === run.lastRank) {
        slot.ready.push({ frame, rank });
        this.over?.coming(this.shownAt(rank));
      }
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
      this.over?.coming(this.shownAt(rank));
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
    this.sources.set(src, when + c.length / rate);
    this.clip(src);
  }

  // A stretch on the sound card stops where the program ends, or at its
  // own end while the program loops. The sound ends with the last piece
  // faded out to nothing at its edge, see fade, so stopped there it stops
  // in silence, wherever the stretch began. Asked again it moves the stop,
  // which a source allows until it has stopped, so the loop switched on
  // and off a dozen times in the last second leaves the sound as it was.
  private clip(src: AudioBufferSourceNode) {
    const own = this.sources.get(src);
    if (own === undefined || this.c0 === null) return;
    const end = this.program.end;
    const at = Number.isFinite(end) ? this.c0 + (Math.round(end * this.sampleRate) - this.m0) / this.sampleRate : own;
    try {
      src.stop(Math.max(0, Math.min(at, own)));
    } catch {}
  }
}
