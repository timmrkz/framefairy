// The frame queue: the episode played onto one canvas with the sound
// card's clock, so a clip plays through its cuts the way the render does.
// Every frame and every sample comes from ffmpeg on the Go side, the
// episode's decoder, see docs/VIDEO-PREVIEW.md. The browser decodes
// nothing, and decides nothing about which frames a play shows, where it
// cuts or how it fades: a play is the Go side's, see play.ts and
// docs/decisions/2026-11-19-go-plays-the-video-preview.md. What is left
// here is what a person sees and hears.
//
// How it runs, in short:
//
// - Paused, the frame that holds the playhead is asked for where the hand
//   put it, a look, see AppFrames, and drawn. The play from there is then
//   cued: its first frames and its first sound come while it waits, so
//   the space bar starts it at once.
// - Playing, the Go side's frames come a few at a time, each stamped with
//   where on the play's clock it is drawn, and its sound a second ahead,
//   cut and faded, scheduled on the AudioContext at the exact place on the
//   clock it belongs, so the stretches meet without a gap.
// - The sound card's clock drives everything. A frame is drawn when the
//   sound heard has reached its place, on the animation frame before it
//   is due on screen. The playhead is the moment the frame due stands
//   for, moved on with the sound, reported to whoever listens on every
//   animation frame, so it moves smoothly and stands in the frame drawn.
// - The clip changed while it plays is a new play that takes over a
//   moment ahead of the sound heard, so the picture and the sound go on
//   with no stop, see changed.
// - Frames are closed the moment they are replaced or not needed, and a
//   play holds a few, so memory stays flat over any length.
// - Nothing polls. Work is started by frames and sound arriving, and by
//   the animation frame that draws while playing.
import { AppFrames, type Waiter } from "./app";
import { afterRest } from "../rest";
import type { Picture } from "./picture";
import type { Over } from "./captions";
import type { Light } from "./light";
import { screenOf, type Screen } from "./screen";
import { describeEvenness, Even, type Source } from "./even";
import { frames as framePool } from "./pool";
import { rankAt, rankOf, ticks, type AudioTrack, type Movie, type VideoTrack } from "./movie";
import { FADE, heard } from "./heard";
import { GoPlay, type Chunk, type Piece, type Shot } from "./play";

export type { Piece } from "./play";

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

// Where a play is on the program, see tick: from where the sound card says
// it is, heard, and where the first frame of the play not on screen yet
// starts, unshown, and whether that frame is ready, decoded and waiting
// with what lies over it here and its sound handed over, see unshown. Ready is asked only by a clock
// that wants it, so the app's, which goes by the sound card alone, costs
// nothing for it.
export type PlayClock = (heard: number, unshown: number, ready: () => boolean) => number;
// What a clock may reach of the queue it runs: the sound card, and whether
// a play goes on.
export type Play = { readonly audio: AudioContext; readonly playing: boolean };

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
  // Stretches of sound scheduled after their time had come, and where the
  // sound card played nothing for them: its frames from where each should
  // have started to where it did start, or to its own end when it came too
  // late to start at all, the stretches that follow on one another as one.
  // A walk tells that from a click, issue #466. The last 1000 are kept, a
  // play of several minutes on a machine too busy to keep up.
  lateSound: number;
  lateSoundAt: [number, number][];
  // Stretches taken off the sound card before they had played to their
  // end, by a play that stopped or started over, and stretches handed to
  // it over sound it had already been handed. A press of L does neither,
  // so a walk that sets late sound aside knows it was not L's, issue #466.
  soundThrown: number;
  soundTwice: number;
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
  // Frames put up on the way to the frame under a hand that drags the
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

// How many frames of a play may have come and wait to be drawn.
const DEPTH = 4;
// How much sound is pulled ahead of what is heard, seconds.
const SOUND_AHEAD = 1;
// How far ahead the sound is pulled while the page's frames stop, behind
// another window, where a timer may run only once a second, see keepOn.
const SOUND_BEHIND = 3;
// Sound pulled before a play starts, seconds.
const SOUND_FIRST = 0.15;
// How far ahead of the sound heard a play taking over from the one before
// starts, seconds: its first frames are kept on the Go side, and its
// sound has to come before that place is heard, see changed.
const TAKE_OVER = 0.15;
// How many times a take over is tried with its place moved on, see
// takeOver, before the play starts again from the playhead.
const TAKE_OVER_TRIES = 3;
// How far ahead of the sound card's clock a play's first sound is put,
// seconds. The sound card renders a buffer at a time, and a stretch put
// where it has rendered already starts late and out of step with the
// picture. A buffer is 512 to 1024 samples on the Mac, 11 to 21 ms at
// 48 kHz, and 25 ms is a whole one of the larger with room left. It was
// 50, and the harness played to the sample from 2, so this is the Mac's
// margin and not the harness's. Every millisecond here is a millisecond
// from the press to the picture moving.
const START_LEAD = 0.025;
// The longest the picture may stand while the hand drags the playhead on,
// in milliseconds, the mark docs/VIDEO-PREVIEW.md sets, for the log of a
// drag. Only said, never acted on.
const DRAG_FRAME_MS = 50;
// The longest a frame a gesture put the playhead on waits for what lies
// over it, the captions, milliseconds, see dress. A layer comes in tens of
// milliseconds, and one that did not come is asked for once more after a
// quarter of a second, see AGAIN in captions.ts. Past this the frame is put
// up without it: a picture that stands still because its captions are slow
// is worse than captions that come a moment after it.
const DRESS_WAIT = 600;

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

// Paused with nothing prepared, paused with a cue, starting a play until
// its first frame and sound are in hand, playing, and stopped at the end
// of the program.
type State = "paused" | "cued" | "starting" | "playing" | "ended";

// What the queue plays, in a few words, for the log.
function describe(m: Movie): string {
  const v = m.video;
  const a = m.audio;
  const picture = v
    ? `${v.width}x${v.height}, ${v.samples.count} frames of ${(v.frame * 1000).toFixed(2)} ms, the first at ${(ticks(v.samples, 0) / v.samples.timescale).toFixed(3)} s`
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
    lateSoundAt: [],
    soundThrown: 0,
    soundTwice: 0,
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
  // The frame asked for again once the canvas's size has come to rest. A
  // window edge dragged asked for it at every size it passed, each a
  // seek, with a stream of frames and one of sound opened on the Go side.
  private sharpenSoon = 0;
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
  // The Go side's frames, from the episode's decoder: every frame drawn,
  // whatever the file. Null only until the queue is open.
  private app: AppFrames | null = null;
  // For the walks, which read how the frames came, see awake in
  // Player.svelte.
  get appFrames(): AppFrames | null {
    return this.app;
  }
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
  // episode straight on, and whether it loops.
  private wanted: { pieces: Piece[] | null; loop: boolean } = { pieces: null, loop: false };
  private wantedKey = "";
  private asked: [Piece[] | null, boolean] | null = null;

  private state: State = "paused";
  // Where the playhead is, in the episode.
  private at = 0;
  // The play under way or cued, the Go side's, see play.ts.
  private goPlay: GoPlay | null = null;
  // Where the play began, in the episode, and the furthest the playhead
  // has got on the play's clock, so it never goes back while playing.
  private startFrom = 0;
  private reached = 0;
  // The frame of the play due last: where on the clock it is drawn and
  // the moment it stands for, which the playhead moves on from.
  private due: { at: number; moment: number } | null = null;
  // The moment of the episode the frame drawn last was drawn for.
  private drawnFor = 0;
  // The ticket of the seek or play whose own frame is on the canvas, and
  // the newest ask of the Go side's frames made when it was put up.
  private putFor = -1;
  private shownAsked = 0;
  // The ticket of the newest seek, cue or play that has said how it went,
  // see report and standing.
  private toldFor = -1;
  // The frame on the canvas: the frame of the grid it is, where it begins
  // in the episode, and where on the clock of the play under way it was
  // drawn, -Infinity for one a play did not draw.
  private shown: { frame: Picture; rank: number; begins: number; at: number } | null = null;
  // The sound card's time at which the clock's sample m0 is played.
  private c0: number | null = null;
  private m0 = 0;
  // What is on the sound card, with the sound card's time each stretch
  // ends at by itself, see clip.
  private sources = new Map<AudioBufferSourceNode, number>();
  // Where the sound handed to the sound card in this play reaches, in its
  // seconds, and the sound come before the clock started.
  private handedTo = -Infinity;
  private unscheduled: Chunk[] = [];
  // How far on the clock the sound come so far reaches, in samples.
  private soundTo = 0;
  private frameRequest = 0;
  private suspendTimer = 0;
  // Every play, cue and seek gets a ticket, and work from an older one is
  // dropped when it comes back.
  private ticket = 0;
  // Counts the hand's asks, a seek or a play, so frames read ahead for
  // where it rested are asked only while it still rests there.
  private gestures = 0;
  private aheadKey = "";
  // The frame a still waits for, withdrawn by a newer seek, see still.
  private stillAsks = new Set<Waiter>();
  // The playhead is held under the hand, being dragged: each place it
  // passes is shown on its own, with no play cued from it, see grip.
  private gripped = false;
  // How a drag of the playhead goes, for the log, see grip: how many
  // frames the frame on screen was from each place the hand moved to, the
  // key frames put up before it, and when the hand let go. waits are how
  // long the picture stood while the hand had moved on from it, each from
  // the move that left it behind to the next frame put up, and waitFrom
  // when the one going on now began, or -1. A stutter is a long wait,
  // which how far behind the frame was does not show, issue #596. ways
  // is how the frames of the moves came, from AppFrames.stats at the
  // press.
  private drag: {
    behind: number[];
    keys: number;
    letGo: number;
    waits: number[];
    waitFrom: number;
    ways: { kept: number; looks: number; near: number; overtaken: number };
  } | null = null;
  // The first frame of a play that is waiting for what lies over it, and
  // the one that has it, each by its ticket and where it begins, see
  // dressed.
  private dressing = "";
  private dressedKey = "";
  // The ticket whose frame is decoded or on its way and waits for what
  // lies over it, see dress, and -1 when none waits. A frame passing is
  // not put up meanwhile, see passing.
  private dressFor = -1;


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
      this.audio = FrameQueue.soundCard(this.sound ? { sampleRate: this.sound.sampleRate } : {});
    } catch {
      this.audio = FrameQueue.soundCard({});
    }
    this.out = this.audio.createGain();
    this.out.connect(this.audio.destination);
    const card = this.audio;
    this.trace(`sound card at ${card.sampleRate} Hz${this.sound && card.sampleRate !== this.sound.sampleRate ? `, not the ${this.sound.sampleRate} Hz asked` : ""}, ${card.state}`);
    card.onstatechange = () => this.trace(`sound card ${card.state}`);
    // A file that cannot be played has said so by now. What can be waits
    // until the canvas has its size, see laidOut.
    await this.laidOut;
    if (this.closed) throw new Error("closed");
    this.trace(`ready, ${this.ms()}`);
  }

  // How long since the queue was made, for the log.
  private ms(): string {
    return `${Math.round(performance.now() - this.born)} ms after it opened`;
  }

  // The Go side's frames for this episode, decoded at the size of the
  // canvas and no larger than the file's own.
  private fromApp(): AppFrames {
    const v = this.video!;
    const app = new AppFrames(this.path, v, () => this.previewSize(), (why) =>
      this.fault(`The app could not decode the picture: ${why.replace(/\.$/, "")}.`),
    );
    app.trace = (line) => this.trace(line);
    app.onPassing = (rank, frame, seq) => this.passing(rank, frame, seq);
    app.paused = () => this.state !== "playing" && this.state !== "starting";
    app.shownRank = () => this.shown?.rank ?? -1;
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
    return this.audio?.sampleRate ?? this.sound?.sampleRate ?? 48000;
  }

  // The channels the Go side mixes the sound to, as many as the episode
  // has, up to eight.
  private get channels(): number {
    return Math.min(this.sound?.channels ?? 2, 8);
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
  // benchmark, which read the picture once it stands, see stillFrame in
  // walks/rules.mjs. It needs the app's own timers to run, the one after
  // a frame is put up that asks for it again among them, see put.
  get standing(): boolean {
    if (this.closed || (this.state !== "paused" && this.state !== "cued")) return true;
    if (this.toldFor !== this.ticket) return false;
    if (!this.shown || this.sharp(this.shown.frame)) return true;
    const { width, height } = this.previewSize();
    return this.sharpened === `${this.shown.rank} ${width} ${height}`;
  }

  // Where the play under way ends on its clock, Infinity while it loops,
  // and where one time through it ends, for the walks.
  get program(): { end: number; length: number; loop: boolean } {
    const p = this.goPlay;
    if (!p) return { end: Infinity, length: 0, loop: this.wanted.loop };
    return { end: p.end, length: p.length + p.shift, loop: p.end === Infinity };
  }

  // The pieces play plays: the clip's, or the whole episode straight on.
  private pieces(): Piece[] {
    return this.wanted.pieces ?? [{ start: 0, end: this.movie?.duration ?? 0 }];
  }

  // What play plays: the pieces of a clip, cuts jumped, or null for the
  // whole episode straight on, and whether it loops. The same program
  // again changes nothing. A new one while playing is taken into the play
  // ahead of the playhead, see changed, and a cue made for the old one is
  // made again.
  setProgram(pieces: Piece[] | null, loop: boolean) {
    if (this.relooped(pieces, loop)) return;
    if (this.state === "playing" || this.state === "starting") {
      const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
      if (JSON.stringify([copy, loop]) === this.wantedKey) {
        this.asked = null;
        return;
      }
      // The clip's pieces changed under a play of the clip are taken in,
      // see changed. A play of the video and a play of a clip do not turn
      // into each other here: a clip made with I or O while the video
      // plays is the next play's, and the clip timeline says when the
      // playhead was left outside the clip, see playOn. So is a program
      // where the playhead is in none of the pieces now.
      if (!pieces || !this.wanted?.pieces || this.changed(pieces, loop) === "outside") this.asked = [pieces, loop];
      return;
    }
    this.asked = null;
    if (!this.want(pieces, loop)) return;
    if (this.state === "cued" || this.goPlay) this.seek(this.at);
  }

  // The program asked for while a play ran, for the next play, see
  // setProgram. Taken as the program, it says whether it is another.
  private takePending(): boolean {
    const p = this.asked;
    this.asked = null;
    return p ? this.want(...p) : false;
  }

  // Only the loop switched, with the same pieces, while a play starts or
  // plays. Switched off, the play ends at the end of the time through the
  // sound card has played into, see GoPlay.loop: the picture at tick, the
  // sound by each stretch on the sound card stopping there, see clip.
  // Switched on, a play that had worked out its end goes on round, which
  // a play taking over does, see changed. Started again for the loop, the
  // play threw away what was decoded, and the picture and the sound
  // stopped for a moment at every press, found by Tim.
  private relooped(pieces: Piece[] | null, loop: boolean): boolean {
    if (this.state !== "playing" && this.state !== "starting") return false;
    const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
    if (JSON.stringify([copy, !loop]) !== this.wantedKey) return false;
    const play = this.goPlay;
    if (play && loop) return this.changed(pieces, loop) !== "outside";
    this.wanted = { pieces: copy, loop };
    this.wantedKey = JSON.stringify([copy, loop]);
    // A play still on its way from the Go side is opened again with what
    // is wanted, see start.
    if (play) {
      this.looped = play.loop(false, Math.max(this.reached, this.sounded())).then(() => {
        if (play !== this.goPlay) return;
        for (const src of this.sources.keys()) this.clip(src);
      });
    }
    return true;
  }

  // The clip changed under a running play, by a drag, an undo or a redo,
  // or its loop switched on: a play of the new pieces takes over at once,
  // a moment ahead of the sound heard, from the moment the play under way
  // stands at there, and the picture and the sound go on with no stop,
  // Tim's rule on #277 and his report on it. Its first frames are the ones
  // the play before decoded and the Go side kept. A clip edge or a cut
  // dragged while the clip played used to start the play again at every
  // step, and the picture, the sound and the playhead stuttered with the
  // hand, so that is the one thing this never does.
  //
  // Says "outside" and changes nothing where the playhead is in none of
  // the new pieces.
  private changed(pieces: Piece[] | null, loop: boolean): "taken" | "outside" | "started" {
    const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
    const list = copy ?? [{ start: 0, end: this.movie?.duration ?? 0 }];
    if (!list.some((p) => this.at >= p.start && this.at < p.end)) return "outside";
    this.want(pieces, loop);
    const play = this.goPlay;
    // Still on its way from the Go side: it is opened again with what is
    // wanted, see start.
    if (!play) return "taken";
    if (this.c0 === null) {
      // Not playing yet: the play starts again with what is wanted.
      this.start(this.startFrom, "again");
      return "started";
    }
    this.takenOver = this.takeOver(play, Math.max(this.reached, this.sounded()) + TAKE_OVER, list, loop);
    return "taken";
  }

  // A play of the pieces takes over from the one under way at a place on
  // its clock, see changed. Where the Go side says the new pieces do not
  // hold the moment that place stands for, the play goes on as it was,
  // and the pieces are the next play's.
  //
  // The new play comes as a cue and takes the decoder only once it is the
  // one drawn: taken at once, the Go side stopped the play under way
  // while the window still pulled from it, and the window, told it was
  // closed, started the play again, the stop this exists to prevent.
  private async takeOver(was: GoPlay, at: number, pieces: Piece[], loop: boolean, tries = 1): Promise<void> {
    if (!this.app) return;
    const key = this.wantedKey;
    const next = await GoPlay.open(
      this.app,
      { pieces, loop, at: this.at, rate: this.sampleRate, channels: this.channels, cue: true, after: { play: was, at } },
      this.previewSize(),
    );
    if (typeof next === "string") {
      this.fault(`The play could not take the change in: ${next.replace(/\.$/, "")}.`);
      return;
    }
    if (!next) {
      if (was === this.goPlay && key === this.wantedKey) this.asked = [this.wanted.pieces, loop];
      return;
    }
    if (was !== this.goPlay || this.closed || key !== this.wantedKey || this.c0 === null) {
      next.close();
      return;
    }
    // The sound went past the place while the Go side answered: the
    // place is moved on, a few times, before the play starts again.
    if (at <= this.sounded()) {
      next.close();
      this.trace(`the take over at ${at.toFixed(3)} s came after the sound had passed it, try ${tries}`);
      if (tries < TAKE_OVER_TRIES) return this.takeOver(was, Math.max(this.reached, this.sounded()) + TAKE_OVER, pieces, loop, tries + 1);
      this.start(this.at);
      return;
    }
    this.trace(`the clip changed while it plays, a play of it takes over at ${next.moment.toFixed(3)} s, ${((at - this.sounded()) * 1000).toFixed(0)} ms ahead of the sound`);
    // The frames and the sound of the play before from that place on make
    // way for the new play's.
    for (const s of was.shots.splice(0)) {
      if (s.at < at) was.shots.push(s);
      else this.closeFrame(s.frame);
    }
    const stop = this.c0 + (Math.round(at * this.sampleRate) - this.m0) / this.sampleRate;
    for (const [src, end] of this.sources) {
      if (end <= stop) continue;
      try {
        src.stop(Math.max(stop, this.audio.currentTime));
      } catch {}
      this.sources.set(src, stop);
    }
    this.handedTo = Math.min(this.handedTo, stop);
    this.soundTo = Math.min(this.soundTo, Math.round(at * this.sampleRate));
    // What the play before has come up to that place is drawn first.
    next.shots.unshift(...was.shots.splice(0));
    // The new play takes the decoder before the old one lets it go, so
    // nothing is read ahead in between.
    next.go();
    was.close();
    this.goPlay = next;
    if (!this.sound) next.soundEnded = true;
    this.listenTo(next);
    this.pump();
  }

  // The playhead was left outside the clip while the clip plays, by an
  // edge of the clip dragged past it or an undo, so the play goes on as a
  // play of the video, Tim's rule on #277: from the piece the playhead is
  // in straight on to the episode's end, taken in the way any change of
  // the clip is, see changed.
  playOn() {
    if (this.state !== "playing" && this.state !== "starting") return;
    this.trace(`the playhead was left outside the clip at ${this.at.toFixed(3)} s, the play goes on as a play of the video`);
    if (this.changed(null, false) !== "outside") return;
    // The playhead in none of the pieces of the program it plays, past the
    // episode's end, which a moment of the episode never is.
    this.want(null, false);
    this.start(this.at);
  }

  // Takes a program as the one play plays, and says whether it is another
  // than the one before. Nothing is started.
  private want(pieces: Piece[] | null, loop: boolean): boolean {
    const copy = pieces ? pieces.map((p) => ({ start: p.start, end: p.end })) : null;
    const key = JSON.stringify([copy, loop]);
    if (key === this.wantedKey) return false;
    this.wantedKey = key;
    this.wanted = { pieces: copy, loop };
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
      const behind = Math.abs(this.shown.rank - rankAt(this.video.samples, at));
      this.drag.behind.push(behind);
      if (behind && this.drag.waitFrom < 0) this.drag.waitFrom = performance.now();
    }
    if (this.state === "playing" || this.state === "starting") {
      this.start(at);
      return;
    }
    this.stopPlay();
    this.state = "paused";
    this.settle(at);
  }


  // A paused playhead: the frame that holds it is asked for and drawn, see
  // still, and the play from it is cued once it is up, see cue. On the
  // clip's end itself it is the clip's last frame, the one that holds the
  // moment a frame before the end, where a play of the clip stops and the
  // short ends, see end, and nothing is cued.
  private settle(at: number) {
    const ticket = this.ticket;
    this.ready.then(
      () => {
        if (ticket !== this.ticket || this.closed) return;
        const last = this.pieces().reduce<Piece | null>((a, p) => (!a || p.end > a.end ? p : a), null);
        if (last && at === last.end) void this.still(at, Math.max(last.start, at - this.video!.frame));
        else void this.still(at, at, !this.gripped);
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
    if (held) this.drag = { behind: [], keys: this.stats.keys, letGo: -1, waits: [], waitFrom: -1, ways: this.ways() };
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
    const waits = [...d.waits].sort((a, b) => a - b);
    const now = this.ways();
    const was = d.ways;
    this.trace(
      `a drag of the playhead let go at ${this.at.toFixed(3)} s, after ${behind.length} moves: the frame on screen ${behind[Math.floor(behind.length / 2)]} frames from the playhead on the median and ${behind[behind.length - 1]} at most as the hand moved, ` +
        `${this.stats.keys - d.keys} frames put up on the way, the frame under the hand ${Math.round(performance.now() - d.letGo)} ms after letting go, ` +
        `the picture waited ${Math.round(waits[Math.floor(waits.length / 2)] ?? 0)} ms on the median and ${Math.round(waits[waits.length - 1] ?? 0)} ms at most for a new frame, ${waits.filter((w) => w > DRAG_FRAME_MS).length} of ${waits.length} waits over ${DRAG_FRAME_MS} ms, ` +
        `the frames from ${now.kept - was.kept} kept, ${now.looks - was.looks} looks, ${now.near - was.near} frames on the way, ${now.overtaken - was.overtaken} looks overtaken`,
    );
  }

  // The counts of how AppFrames found its frames, see drag.
  private ways() {
    const s = this.app?.stats;
    return { kept: s?.kept ?? 0, looks: s?.looks ?? 0, near: s?.near ?? 0, overtaken: s?.overtaken ?? 0 };
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
        this.app.ahead(times.map((at) => rankAt(s, at)));
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
  // Settles once the Go side has said where the play ends after the loop
  // was switched off, see relooped, for the walks: until then the play
  // under way still loops, see program.
  looped: Promise<void> = Promise.resolve();
  // Settles once the play of a changed clip has taken over from the one
  // under way, or the queue has given up and started it again, see
  // changed, for the walks: until then the sound must not pass the place
  // it takes over at.
  takenOver: Promise<void> = Promise.resolve();

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
    this.app?.hold();
    // The sound card starts on the gesture that asked for it, or a browser
    // keeps it silent.
    void this.audio?.resume();
    // What was asked for while the last play ran is what this one plays,
    // so a play paused or cued on the program before starts again.
    const changed = this.takePending();
    // Paused in the middle of a play: the play is all still there, the
    // sound card was only held.
    if (!changed && this.goPlay && this.c0 !== null && this.state === "paused") {
      clearTimeout(this.suspendTimer);
      if (this.goPlay.held) this.goPlay.go();
      this.state = "playing";
      this.app?.began();
      this.trace(`playing on from ${this.at.toFixed(3)} s`);
      this.began = { at: this.at, drawn: this.stats.drawn, late: this.stats.late, lateSound: this.stats.lateSound, when: performance.now(), even: new Even() };
      this.ramp(1);
      this.report();
      this.loop();
      return;
    }
    // Cued: everything is there but the decoder and the clock.
    if (!changed && this.state === "cued" && this.goPlay) {
      this.state = "starting";
      this.stats.starts++;
      this.startFrom = this.at;
      this.goPlay.go();
      this.report();
      this.pump();
      this.maybeBegin();
      return;
    }
    let from = this.at;
    if (this.state === "ended") from = this.firstStart();
    this.start(from);
  }

  // Where the clip starts, which a play past its end starts from.
  private firstStart(): number {
    return Math.min(...this.pieces().map((p) => p.start));
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
    // The play gives the Go side's decoder back meanwhile, so the frames
    // of the cards and of the words beside are read while the hand rests,
    // and takes it again as it goes on, see GoPlay.hold. Kept, it held the
    // decoder, and nothing was read ahead while it stood paused.
    this.ramp(0);
    this.suspendTimer = window.setTimeout(() => void this.audio.suspend(), FADE * 1000 + 10);
    this.goPlay?.hold();
    this.hold();
    this.report();
  }

  // Paused, the picture is the frame that holds the playhead. The playhead
  // runs up to a frame ahead of the frame drawn for it, and that frame has
  // nearly always come already. Where it has not, the playhead goes back
  // to the moment the frame on screen was drawn for.
  private hold() {
    const play = this.goPlay;
    if (!this.shown || !play) return;
    let i = -1;
    while (i + 1 < play.shots.length && play.shots[i + 1].at <= this.reached) i++;
    if (i < 0) {
      if (this.due && this.shown.at !== this.due.at) this.at = this.drawnFor;
      return;
    }
    this.draw(play, i);
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    opened.splice(opened.indexOf(this), 1);
    this.idler?.port1.close();
    clearTimeout(this.sharpenSoon);
    // A queue closed before its canvas had a size stops waiting for one.
    this.sizeKnown();
    this.stopPlay();
    this.app?.close();
    if (this.shown) this.closeFrame(this.shown.frame);
    this.shown = null;
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
    // The frame on screen is drawn to fit at once. It is asked for at the
    // new size at once when the size was at rest, so one step costs no
    // wait, and again once the hand that sizes the app has come to rest,
    // not at every size a dragged edge passes.
    const resting = this.sharpenSoon === 0;
    clearTimeout(this.sharpenSoon);
    if (first || resting) this.sharpen();
    if (first) return;
    this.app?.resizing(true);
    this.sharpenSoon = afterRest(() => {
      this.sharpenSoon = 0;
      this.app?.resizing(false);
      this.sharpen();
    }, 150);
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
      frame: this.shown ? this.shown.begins : NaN,
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
  // on the way to the frame asked for, see passing. At is where on the
  // clock of the play under way it is drawn, -Infinity for one a play did
  // not draw.
  private put(frame: Picture, rank: number, begins: number, at: number, passing = false) {
    if (!this.firstTold) {
      this.firstTold = true;
      this.trace(`first frame drawn, frame ${rank} of ${frame.width}x${frame.height}${frame.light ? `, ${frame.light}` : ""}, ${this.ms()}`);
    }
    if (this.shown && this.shown.frame !== frame) this.closeFrame(this.shown.frame);
    if (this.gripped && this.drag && this.video && this.shown?.rank !== rank) {
      const now = performance.now();
      if (this.drag.waitFrom >= 0 && this.drag.waits.length < 4096) this.drag.waits.push(now - this.drag.waitFrom);
      this.drag.waitFrom = rank === rankAt(this.video.samples, this.at) ? -1 : now;
    }
    this.shown = { frame, rank, begins, at };
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
  // The Go side sends a frame on the way to the frame under the hand, the
  // first its decoder reads there, where it is nearer the hand than the
  // frame on screen, see engine.Schedule. It says nothing of the ask it
  // came for, whose own frame still comes and replaces it, so it never
  // stays once the hand rests.
  //
  // Nor while the frame of the newest gesture waits for what lies over it.
  // A clip card clicked puts up the clip's first frame with its captions,
  // in one change, and a frame of an older place came in between: a clip
  // made with Mark In was sought to the mark and then to its first
  // sentence, the mark's frame came while the clip's waited for its
  // captions, and it went up first, the picture of another moment under
  // the clip's captions, then the clip's own frame.
  private passing(rank: number, frame: Picture, seq: number) {
    if (this.closed || (this.state !== "paused" && this.state !== "cued") || this.putFor === this.ticket) return;
    if (this.dressFor === this.ticket) return;
    if (seq <= this.shownAsked) return;
    if (this.shown?.rank === rank) return;
    this.stats.open++;
    this.stats.keys++;
    this.stats.keysAt.push(this.shownAt(rank));
    if (this.stats.keysAt.length > 16) this.stats.keysAt.shift();
    this.put(frame.clone(), rank, this.shownAt(rank), -Infinity, true);
    this.report(true, true);
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
  // The clock this queue plays on, see PlayClock. The app's is the sound
  // card's, so a machine too slow to decode every frame in time shows
  // fewer of them, which is right for the app. What runs the queue hands
  // in a clock of its own by clock, before the queue is made: the page
  // the walks run on hands in the walk's, see preview/clock.ts, which the
  // app never builds.
  static clock: (play: Play) => PlayClock = () => (heard) => heard;
  // The sound card a queue plays on, made as it opens. The app's is the
  // system's. What runs the queue hands in one of its own the same way as
  // clock: the page the walks run on hands in a card whose time the walk's
  // clock moves, see preview/clock.ts.
  static soundCard: (options: AudioContextOptions) => AudioContext = (options) => new AudioContext(options);
  playClock: PlayClock = FrameQueue.clock(this);
  get playing(): boolean {
    return this.state === "playing";
  }

  // Told of every frame put on screen, or of null for black, by what
  // shows it again somewhere else: the reflection at the foot of the
  // workspace, see Reflection.svelte. The frame is only lent, read at once
  // and never kept, the way the screen reads it.
  painted: ((frame: Picture | null) => void) | null = null;

  // The frame on screen, see screen.ts. Fresh when it is put up for the
  // newest gesture, see over.
  private paint(fresh = false) {
    const s = this.shown;
    this.screen?.draw(s?.frame ?? null, s && this.over ? this.over.at(s.begins, s.frame.light, fresh) : null);
    this.painted?.(s?.frame ?? null);
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

  // Whether what lies over the frame that begins at begins is here, asked
  // for at once where it is not: a promise of whether the frame is still wanted once
  // it is, or once DRESS_WAIT has passed, or null when there is nothing to
  // wait for. Nothing waits while the hand drags the playhead, where each
  // place is passed in a moment and the picture follows the hand.
  private dress(begins: number): Promise<boolean> | null {
    if (this.gripped || !this.over?.ready || !this.video) return null;
    const came = this.over.ready(begins);
    if (!came) return null;
    const ticket = this.ticket;
    const asked = performance.now();
    this.stats.waitedForDress++;
    this.dressFor = ticket;
    let stop = () => {};
    const late = new Promise<"late">((done) => (stop = this.dressWait(() => done("late"))));
    return Promise.race([came.then(() => "came" as const), late]).then((how) => {
      stop();
      if (this.dressFor === ticket) this.dressFor = -1;
      const wanted = ticket === this.ticket && !this.closed;
      if (how === "late" && wanted) {
        this.stats.bare++;
        this.trace(`the captions of the frame at ${begins.toFixed(3)} s did not come in ${DRESS_WAIT} ms, the frame is put up without them`);
      }
      else if (wanted) this.stats.dressWait = Math.max(this.stats.dressWait, Math.round(performance.now() - asked));
      return wanted;
    });
  }

  // Whether the first frame of the cue or play under way, at begins, may be
  // put up: what lies over it is here, or nothing has to be. Asked the
  // first time, it asks for it and puts the frame up through maybeBegin
  // once it comes.
  private dressed(begins: number): boolean {
    const key = `${this.ticket} ${begins}`;
    if (this.dressedKey === key) return true;
    if (this.dressing === key) return false;
    const waits = this.dress(begins);
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
    return ticks(s, rank) / s.timescale;
  }


  // ---- A paused frame on its own

  // The frame drawn is the one that holds the moment it is given, the
  // playhead unless said otherwise. With cue, the play from the playhead
  // is cued once the frame is up, see cue.
  //
  // A seek made while it waits for the video to be read is newer, and the
  // still gives way to it. It took a ticket of its own after the wait, which
  // made the newer seek's own stale, and the still then put the playhead
  // back where it had been asked for: a drag that left the playhead on an
  // edge for a moment kept it there, a frame off the edge's frame.
  private async still(at: number, holds = at, cue = false) {
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
    // The frame the still before waits for is wanted no more.
    for (const w of this.stillAsks) w.gone = true;
    this.stillAsks.clear();
    const rank = rankAt(this.video!.samples, holds);
    const begins = this.shownAt(rank);
    // What lies over the frame is asked for as the frame is, so the two
    // come in the time the slower takes, not one after the other.
    const dressed = this.dress(begins);
    // Already on screen, at the canvas's size: nothing to ask for. What
    // lies over it may be another clip's still, see over.
    if (this.shown && this.shown.rank === rank && this.sharp(this.shown.frame)) {
      if (dressed && !(await dressed)) return gaveUp();
      this.putFor = ticket;
      this.at = at;
      this.paint(true);
      this.report();
      if (cue) this.cue(at, ticket);
      return;
    }
    const frame = await this.app!.frame(rank, Math.round(begins * 1e6), this.stillAsks);
    if (frame) {
      this.stats.open++;
      this.stats.mostOpen = Math.max(this.stats.mostOpen, this.stats.open);
      this.stats.decoded++;
      this.over?.coming(begins);
    }
    if (ticket !== this.ticket) {
      if (frame) this.closeFrame(frame);
      return;
    }
    // The frame waits for what lies over it, and a seek made meanwhile
    // takes its place.
    if (dressed && !(await dressed)) {
      if (frame) this.closeFrame(frame);
      return gaveUp();
    }
    if (frame) this.put(frame, rank, begins, -Infinity);
    else this.paint(true);
    this.at = at;
    this.report(!!frame);
    if (cue && frame) this.cue(at, ticket);
  }

  // The play from a paused playhead, prepared and waiting for the space
  // bar: its first frames, kept on the Go side by the still, and its
  // first sound come while it waits, and the Go side's decoder stays the
  // hand's until it starts, see GoPlay.go. A playhead in a cut of the clip
  // or past its end starts a play on another frame than the one up, and
  // cues nothing.
  private async cue(at: number, ticket: number): Promise<void> {
    if (!this.app || this.state !== "paused") return;
    const key = this.wantedKey;
    const play = await GoPlay.open(
      this.app,
      { pieces: this.pieces(), loop: this.wanted.loop, at, rate: this.sampleRate, channels: this.channels, cue: true },
      this.previewSize(),
    );
    if (!play || typeof play === "string") return;
    // The clip or its loop changed while the Go side opened the cue, a
    // cut made as the playhead was put down: it is cued again for what is
    // wanted now. Kept, the space bar played the clip as it was, and the
    // video preview showed the frames of the new cut.
    if (key !== this.wantedKey) {
      play.close();
      if (ticket === this.ticket && !this.closed) return this.cue(at, ticket);
      return;
    }
    if (ticket !== this.ticket || this.closed || this.state !== "paused" || this.goPlay || play.first !== this.shown?.begins || play.from >= play.end) {
      play.close();
      return;
    }
    this.state = "cued";
    this.startFrom = at;
    this.begin(play);
  }

  // ---- Playing

  private stopPlay() {
    this.ticket++;
    cancelAnimationFrame(this.frameRequest);
    clearTimeout(this.suspendTimer);
    // A stretch stops where the play ends, see clip, so one stopped after
    // that threw nothing away.
    const now = this.sources.size ? this.audio.currentTime : 0;
    const end = this.goPlay?.end ?? Infinity;
    const ends = this.c0 !== null && Number.isFinite(end) ? this.c0 + (Math.round(end * this.sampleRate) - this.m0) / this.sampleRate : Infinity;
    for (const [src, stops] of this.sources) {
      if (Math.min(stops, ends) > now) this.stats.soundThrown++;
      try {
        src.stop();
      } catch {}
      src.disconnect();
    }
    this.sources.clear();
    this.handedTo = -Infinity;
    if (this.goPlay) {
      for (const s of this.goPlay.shots.splice(0)) this.closeFrame(s.frame);
      this.goPlay.close();
      this.goPlay = null;
    }
    this.unscheduled = [];
    this.due = null;
    this.c0 = null;
  }

  // A play from a moment of the episode. A play started again for what is
  // wanted now, before anything was heard, is no new start for the
  // counts.
  private start(from: number, how: "play" | "again" = "play") {
    this.stopPlay();
    if (how === "play") this.stats.starts++;
    const ticket = this.ticket;
    this.state = "starting";
    this.playTold = false;
    this.trace(`starting a play from ${from.toFixed(3)} s`);
    const asked = performance.now();
    // A play that has not started after a while says what it waits for.
    for (const after of [3, 10])
      setTimeout(() => {
        if (ticket === this.ticket && this.state === "starting" && !this.closed)
          this.trace(`the play from ${from.toFixed(3)} s has not started ${Math.round((performance.now() - asked) / 1000)} s after it was asked: ${this.waitingFor()}`);
      }, after * 1000);
    this.startFrom = from;
    this.at = from;
    void this.audio?.resume();
    // Started again for what is wanted now, it says nothing new.
    if (how === "play") this.report();
    this.ready.then(
      async () => {
        if (ticket !== this.ticket || this.closed || !this.app) return;
        const key = this.wantedKey;
        const play = await GoPlay.open(
          this.app,
          { pieces: this.pieces(), loop: this.wanted.loop, at: from, rate: this.sampleRate, channels: this.channels, cue: false },
          this.previewSize(),
        );
        if (ticket !== this.ticket || this.closed) {
          if (play && typeof play !== "string") play.close();
          return;
        }
        // The clip or its loop changed while the Go side opened the play:
        // it is opened again for what is wanted now.
        if (key !== this.wantedKey) {
          if (play && typeof play !== "string") play.close();
          this.start(from, "again");
          return;
        }
        if (!play || typeof play === "string") {
          this.fault(`The play could not start: ${String(play).replace(/^the play could not start: /, "").replace(/\.$/, "")}.`);
          return;
        }
        if (play.from >= play.end) {
          // Nothing to play from here: a play from past the clip's end
          // plays the clip from its start.
          play.close();
          const first = this.firstStart();
          if (from !== first) this.start(first, "again");
          else {
            this.state = "ended";
            this.report();
          }
          return;
        }
        this.begin(play);
      },
      () => {},
    );
  }

  // Takes a play the Go side opened as the one under way or cued, and
  // asks for its first frames and sound.
  private begin(play: GoPlay) {
    this.goPlay = play;
    this.listenTo(play);
    if (!this.sound) play.soundEnded = true;
    this.out.gain.cancelScheduledValues(this.audio.currentTime);
    this.out.gain.setValueAtTime(1, this.audio.currentTime);
    this.reached = play.from;
    this.m0 = Math.round(play.from * this.sampleRate);
    this.soundTo = this.m0;
    // What lies over the first frame is asked for as it comes.
    if (play.first >= 0 && this.state === "starting") this.dressed(play.first);
    this.pump();
  }

  // Hears of the frames that came for a play.
  private listenTo(play: GoPlay) {
    play.onShots = (came) => {
      if (play !== this.goPlay || this.closed) return;
      if (play.gone) return this.lost();
      this.stats.open += came;
      this.stats.mostOpen = Math.max(this.stats.mostOpen, this.stats.open);
      this.stats.decoded += came;
      for (const s of play.shots.slice(-came)) this.over?.coming(s.begins);
      if (play.trouble && play.picturesEnded) this.fault(`The app could not decode the picture: ${play.trouble.replace(/\.$/, "")}.`);
      if (this.state === "starting" || this.state === "cued") this.maybeBegin();
      this.pump();
    };
  }

  // The Go side closed the play under way: a play paused or cued and left
  // standing is closed once nobody has pulled from it for a while. One
  // playing starts again from the playhead, a moment's stop the person
  // hears, so it counts as a start and a walk sees it, and one
  // paused or cued is dropped, so the space bar starts a play of its own.
  // Cued again, a cue left standing was made again and again for as long
  // as the app stood paused.
  private lost() {
    this.trace(`the play was closed on the Go side, ${this.state} at ${this.at.toFixed(3)} s`);
    if (this.state === "playing" || this.state === "starting") {
      this.start(this.at);
      return;
    }
    if (this.state === "ended") return;
    this.stopPlay();
    this.state = "paused";
    this.report();
  }

  // Puts the first frame of a play up, and starts the sound card's clock
  // once the first frame and the first of the sound are in hand and play
  // was asked for. The first frame is up once due is set: the frame a
  // cue was made on, which is on screen already, or the play's own first.
  private maybeBegin() {
    const play = this.goPlay;
    if ((this.state !== "starting" && this.state !== "cued") || !play) return;
    let frameIn = this.due !== null;
    const first = play.shots[0];
    if (!frameIn && !first) frameIn = play.picturesEnded;
    else if (!frameIn && this.shown && this.shown.begins === first.begins && this.sharp(this.shown.frame)) {
      // The frame on screen at the canvas's size is the first, the frame
      // a cue was made on: it stays, and the one that came is let go.
      play.shots.shift();
      this.closeFrame(first.frame);
      this.shown.at = first.at;
      this.due = first;
      this.drawnFor = first.moment;
      frameIn = true;
    } else if (!frameIn && this.state === "starting" && this.dressed(first.begins)) {
      // The first frame comes with what lies over it, see dress.
      this.draw(play, 0);
      frameIn = true;
      this.report(true);
    }
    if (this.state === "cued") return;
    const soundIn = play.soundEnded || this.soundTo - this.m0 >= SOUND_FIRST * this.sampleRate;
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
    this.app?.began();
    this.trace(`playing from ${this.startFrom.toFixed(3)} s`);
    this.began = { at: this.startFrom, drawn: this.stats.drawn, late: this.stats.late, lateSound: this.stats.lateSound, when: performance.now(), even: new Even() };
    for (const c of this.unscheduled) this.schedule(c);
    this.unscheduled = [];
    this.tick(performance.now());
  }

  // Draws shot i of the play, the ones before it closed unseen.
  private draw(play: GoPlay, i: number) {
    const passed = play.shots.splice(0, i + 1);
    const s = passed.pop()!;
    for (const p of passed) {
      this.stats.unseen++;
      this.closeFrame(p.frame);
    }
    this.put(s.frame, rankOf(this.video!.samples, s.begins), s.begins, s.at);
    this.due = s;
    this.drawnFor = s.moment;
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
    const play = this.goPlay;
    const sound = !play ? "none" : play.soundEnded ? "all of it" : `${((this.soundTo - this.m0) / this.sampleRate).toFixed(3)} s of ${SOUND_FIRST} s`;
    return [
      play ? `first frame ${play.first >= 0 ? `at ${play.first.toFixed(3)} s, ${play.shots.length} frames come` : "none needed"}` : "no play from the Go side yet",
      `sound ${sound}`,
      `sound card ${this.audio?.state ?? "not there"}`,
      `trouble "${this.trouble}"`,
      `queue ${JSON.stringify(this.stats)}`,
      `frames ${JSON.stringify(this.app?.stats ?? null)}, read on the ${this.app?.puller.where ?? "nothing"}`,
    ].join(", ");
  }

  // Where the clock is now, from the sound being heard, at the moment a
  // frame drawn now is on screen, a refresh of the screen from now, see
  // refreshNow in even.ts.
  private position(now: number): number {
    return this.heardOn(now).pos;
  }

  // The same, with where the moment heard came from, for the log.
  private heardOn(now: number): { pos: number; source: Source } {
    if (this.c0 === null || !this.goPlay) return { pos: this.goPlay?.from ?? 0, source: "plain" };
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

  // How far on the clock the sound card has played, heard or about to
  // be: what it has played can no longer be stopped. The loop switched off
  // ends the play at the end of the time through this is in, not the one
  // the playhead is in. The playhead moves on only with the page's frames,
  // so on a busy machine the sound had crossed the clip's end while the
  // playhead still stood before it, the stop came too late, and a moment
  // of the clip's start was heard after its end. Plan row 2.253.
  sounded(): number {
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
    const play = this.goPlay;
    if (this.state !== "playing" || !play) return;
    const begun = performance.now();
    // Never back: the sound heard only ever moves on, and a stamp that says
    // a hair less than the last one is the clock's jitter.
    const h = this.heardOn(now);
    const next = this.unshown(play);
    const pos = Math.max(this.playClock(h.pos, next.at, next.ready), this.reached);
    this.reached = pos;
    if (this.behind) this.began?.even.behind();
    else this.began?.even.tick(now, pos, h.source);
    if (pos >= play.end) {
      this.end();
      return;
    }
    // The frame due, but never past the first or last frame of a piece
    // without drawing it: a frame at the edge of a piece can be due for
    // less than a display frame, and the frame before a cut is the one
    // that says where the cut is.
    let due = -1;
    for (let i = 0; i < play.shots.length && play.shots[i].at <= pos; i++) {
      due = i;
      if (play.shots[i].edge) break;
    }
    let drew = false;
    let put: number | undefined;
    if (due >= 0) {
      const putting = performance.now();
      const at = play.shots[due].at;
      this.draw(play, due);
      put = performance.now() - putting;
      drew = true;
      this.began?.even.put(now, at);
    } else if (!play.shots.length && !play.picturesEnded && pos >= (this.due?.at ?? play.from) + this.video!.frame) {
      // The next frame is due by now and has not come.
      this.stats.late++;
    }
    if (this.due) this.at = this.due.moment + Math.max(0, pos - this.due.at);
    const telling = performance.now();
    this.emit({
      at: this.at,
      frame: this.shown ? this.shown.begins : NaN,
      drew,
      playing: true,
      ended: false,
      drawnAt: now,
      trouble: this.trouble,
    });
    const feeding = performance.now();
    this.pump();
    const done = performance.now();
    if (!this.behind && this.began) {
      const i = this.began.even.work({ late: begun - now, put, tell: feeding - telling, feed: done - feeding, all: done - begun });
      this.idle(this.began.even, i, done);
    }
    this.loop();
  }

  // Where the first frame of the play not on screen yet is drawn on the
  // clock, see playClock, and whether it is ready: come and waiting with
  // what lies over it here, so a clock that goes on to it at once shows it
  // with its captions, the way the app does, which has the frame's time to
  // fetch them, and with the sound up to its place on the sound card, so a
  // clock that goes on as fast as the frames come never plays past the
  // sound, which the app's sound card plays as it comes. A layer still on
  // its way is asked for first. A frame still on its way from the Go side
  // holds the clock where it is, and once the Go side has sent the last,
  // the play goes by the sound card's time the way it does in the app.
  private unshown(play: GoPlay): { at: number; ready: () => boolean } {
    const s = play.shots[0];
    if (!s) return { at: play.picturesEnded ? Infinity : this.reached, ready: () => false };
    const sounded = () => play.soundEnded || (this.c0 !== null && this.m0 / this.sampleRate + (this.handedTo - this.c0) >= s.at);
    const ready = () => !(this.video && this.over?.ready?.(s.begins)) && sounded();
    return { at: s.at, ready };
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

  // The play reached its end. Stopped at the end, the picture is the last
  // frame of what played: nearly always it has come, and where it has
  // not, it is asked for on its own, as a paused frame is.
  private end() {
    const play = this.goPlay!;
    this.state = "ended";
    let last = -1;
    while (last + 1 < play.shots.length && play.shots[last + 1].at < play.end) last++;
    if (last >= 0) this.draw(play, last);
    // The loop switched off just before the end can be said by the Go
    // side after the clock has gone round: the frame on screen is then of
    // the next time through, and the playhead read the clip's start. The
    // play ends where the time through ends, at its last piece's end.
    const round = this.due !== null && this.due.at >= play.end;
    if (round) {
      const p = this.wanted.pieces;
      this.at = p?.length ? p[p.length - 1].end : (this.movie?.duration ?? this.at);
    } else if (this.due) this.at = this.due.moment + Math.max(0, play.end - this.due.at);
    this.trace(`played to the end, ${this.at.toFixed(3)} s, ${this.howItWent()}`);
    this.report(last >= 0);
    // The sound card is held once the last of the sound has been heard.
    // Anything that plays or seeks after the end clears this first.
    this.suspendTimer = window.setTimeout(() => {
      if (this.state === "ended") void this.audio.suspend();
    }, 300);
    if ((round || (last < 0 && play.picturesEnded === false)) && this.video) void this.still(this.at, Math.max(0, this.at - this.video.frame));
  }

  // ---- Pulling the pictures and the sound

  // Asks the Go side for the frames and the sound the play needs next: a
  // few frames ahead, and the sound a second ahead of what is heard.
  private pump() {
    const play = this.goPlay;
    if (!play || (this.state !== "starting" && this.state !== "playing" && this.state !== "cued")) return;
    play.more(DEPTH);
    if (play.soundEnded || play.pullingSoundNow) return;
    const rate = this.sampleRate;
    const heard = this.c0 === null ? this.m0 : Math.round(this.position(performance.now()) * rate);
    const ahead = (this.c0 === null ? SOUND_FIRST * 2 : this.behind ? SOUND_BEHIND : SOUND_AHEAD) * rate;
    if (this.soundTo - heard >= ahead) return;
    void play.moreSound().then((chunks) => {
      if (play !== this.goPlay || this.closed) return;
      if (play.gone) return this.lost();
      for (const c of chunks) {
        if (c.out + c.length <= this.soundTo) continue;
        this.stats.soundDecoded++;
        this.soundTo = c.out + c.length;
        this.hand(c);
      }
      if (play.trouble && play.soundEnded && this.sound) this.fault(`The sound stopped decoding: ${play.trouble.replace(/\.$/, "")}.`);
      if (this.state === "starting" || this.state === "cued") this.maybeBegin();
      this.pump();
    });
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
    if (when < this.handedTo - 1 / rate) this.stats.soundTwice++;
    this.handedTo = Math.max(this.handedTo, when + c.length / rate);
    const now = this.audio.currentTime;
    if (when < now) {
      this.stats.lateSound++;
      const quiet: [number, number] = [Math.round(when * rate), Math.round(Math.min(now, when + c.length / rate) * rate)];
      const last = this.stats.lateSoundAt.at(-1);
      if (last && Math.abs(quiet[0] - last[1]) <= 1) last[1] = quiet[1];
      else this.stats.lateSoundAt.push(quiet);
      if (this.stats.lateSoundAt.length > 1000) this.stats.lateSoundAt.shift();
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

  // A stretch on the sound card stops where the play ends, or at its own
  // end while the play loops. The sound ends with the last piece faded out
  // to nothing at its edge, so stopped there it stops in silence, wherever
  // the stretch began. Asked again it moves the stop, which a source
  // allows until it has stopped, so the loop switched on and off a dozen
  // times in the last second leaves the sound as it was.
  private clip(src: AudioBufferSourceNode) {
    const own = this.sources.get(src);
    if (own === undefined || this.c0 === null) return;
    const end = this.goPlay?.end ?? Infinity;
    const at = Number.isFinite(end) ? this.c0 + (Math.round(end * this.sampleRate) - this.m0) / this.sampleRate : own;
    try {
      src.stop(Math.max(0, Math.min(at, own)));
    } catch {}
  }
}
