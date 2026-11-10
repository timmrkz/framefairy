// The walk's clock, which a play runs on in the page the walks run on, see
// install, and the sound card it moves. Both are a test's own, so they
// live here with the walks and the app never builds them: the app plays on
// the system's sound card and its clock, see FrameQueue.clock and
// FrameQueue.soundCard.
//
// A play goes from frame to frame as fast as the page draws them: the
// walk's clock moves the walk's sound card on to the next frame the moment
// that frame is decoded and its captions are here, and stands while either
// is on its way. So a
// walk sees every frame of a play on any machine, the sound stays with the
// picture, and a play of 3 s of a 5 fps video takes the 15 refreshes that
// draw its frames, not 3 s. By the system's sound card a busy machine
// showed 2 frames of 9, issue #534, standing on frames alone left the sound
// running on ahead, so a loop switched off or a cut made while playing went
// by the sound's moment, not the picture's, issue #588, and every play took
// as long as it lasts.
import { FrameQueue, type Play, type PlayClock } from "../src/lib/frames/queue";

// The queue makes its clock as it is made and its sound card as it
// opens, so the card is looked up on each tick.
export function walkClock(play: Play): PlayClock {
  return (heard, unshown, ready) => {
    const card = play.audio;
    if (!(card instanceof WalkCard)) throw new Error("the walk's clock moves only the walk's sound card, see install");
    if (card.state !== "running") return Math.min(heard, unshown);
    // A frame ready, decoded and waiting with its captions, goes up on
    // this tick, so the card goes on to its moment. A frame the queue says
    // is nowhere, see unshown in queue.ts, never comes, so the card goes by
    // the time that passed, as the system's does. A frame still on its way
    // holds the card, and the time it stands is not counted later.
    if (unshown === Infinity) return heard + card.follow();
    if (unshown > heard && ready()) return Math.min(heard + card.run(unshown - heard), unshown);
    card.stand();
    return Math.min(heard, unshown);
  };
}

// A sound card whose time moves only when the walk's clock moves it, see
// walkClock, or by the time that passes while nothing is coming. It does
// what the frame queue asks of the system's: it runs, is held and closed,
// and takes stretches of sound and a gain, which it plays nowhere. A
// stretch ends once the card's time has passed it, the way the system's
// says so. What listens to the sound plays on the system's card instead,
// see soundCard in walks/common/steps.mjs.
export class WalkCard extends EventTarget {
  readonly sampleRate: number;
  readonly baseLatency = 0;
  readonly outputLatency = 0;
  readonly destination = new WalkNode();
  state: AudioContextState = "suspended";
  onstatechange: ((ev: Event) => unknown) | null = null;
  private time = 0;
  // When the time last moved or stood, on the page's clock, for follow.
  private moved = 0;
  private playing = new Set<WalkSource>();

  constructor(options: AudioContextOptions = {}) {
    super();
    this.sampleRate = options.sampleRate ?? 48000;
  }

  get currentTime(): number {
    return this.time;
  }

  // Moves the time on by seconds while the card runs, and says by how
  // much it moved.
  run(seconds: number): number {
    if (this.state !== "running" || !(seconds > 0)) return 0;
    this.time += seconds;
    this.moved = performance.now();
    for (const s of this.playing) s.reached(this.time);
    return seconds;
  }

  // Moves the time on by the time that passed on the page's clock since it
  // last moved, stood or started.
  follow(): number {
    return this.run((performance.now() - this.moved) / 1000);
  }

  // Stands where it is: the time that passes until the next move is
  // counted from now, so a frame that held the card is not made up for
  // once the next one is nowhere.
  stand() {
    this.moved = performance.now();
  }

  resume(): Promise<void> {
    return this.become("running");
  }

  suspend(): Promise<void> {
    return this.become("suspended");
  }

  close(): Promise<void> {
    return this.become("closed");
  }

  // Does what was asked the way the system's card does, in a task of its
  // own: it settles the call's promise, then takes the new state and says
  // so, so what the call's promise waits for sees the card running, and a
  // call made as the card says it runs comes in a task after. Done at once,
  // the hand that holds the card while a play starts, see holdSound, held
  // it again before the play saw it run, the play asked for it again, and
  // the two went back and forth for good. A message, not a timer, so a
  // clock the walk holds holds no card.
  private asked: (() => void)[] = [];
  private later = new MessageChannel();
  private become(state: AudioContextState): Promise<void> {
    this.later.port1.onmessage = () => this.asked.shift()?.();
    return new Promise((done) => {
      this.asked.push(() => {
        done();
        if (this.state === state || this.state === "closed") return;
        this.state = state;
        if (state === "running") this.moved = performance.now();
        const e = new Event("statechange");
        this.onstatechange?.(e);
        this.dispatchEvent(e);
      });
      this.later.port2.postMessage(0);
    });
  }

  createGain(): GainNode {
    const gain = {
      value: 1,
      cancelScheduledValues: () => gain,
      setValueAtTime: (v: number) => ((gain.value = v), gain),
      linearRampToValueAtTime: (v: number) => ((gain.value = v), gain),
    };
    return Object.assign(new WalkNode(), { gain }) as unknown as GainNode;
  }

  createBuffer(channels: number, length: number, rate: number): AudioBuffer {
    return { numberOfChannels: channels, length, sampleRate: rate, duration: length / rate, copyToChannel() {} } as unknown as AudioBuffer;
  }

  createBufferSource(): AudioBufferSourceNode {
    return new WalkSource(this, this.playing) as unknown as AudioBufferSourceNode;
  }
}

class WalkNode {
  connect<T>(to: T): T {
    return to;
  }
  disconnect() {}
}

// A stretch of sound on the walk's sound card. It ends at its own end or
// where it was stopped, once the card's time has passed that.
class WalkSource extends WalkNode {
  buffer: AudioBuffer | null = null;
  onended: (() => void) | null = null;
  private ends = Infinity;
  private stops = Infinity;
  private started = false;
  private done = false;

  constructor(
    private card: WalkCard,
    private playing: Set<WalkSource>,
  ) {
    super();
  }

  start(when = 0, offset = 0) {
    if (this.started) throw new DOMException("a stretch of sound starts once", "InvalidStateError");
    this.started = true;
    const from = Math.max(when, this.card.currentTime);
    this.ends = from + Math.max(0, (this.buffer?.duration ?? 0) - offset);
    this.playing.add(this);
    this.reached(this.card.currentTime);
  }

  stop(when = 0) {
    if (!this.started) throw new DOMException("a stretch of sound stops only once it started", "InvalidStateError");
    this.stops = when;
    this.reached(this.card.currentTime);
  }

  reached(time: number) {
    if (this.done || time < Math.min(this.ends, this.stops)) return;
    this.done = true;
    this.playing.delete(this);
    queueMicrotask(() => this.onended?.());
  }
}

// Holds a play's sound card by the walk's hand while it drags, "hand",
// see holdSound in walks/bridge.mjs and issue #583, or lets it go. The
// card runs again only once nobody holds it, and only while a play goes
// on: a play the app pauses or ends holds it itself, and a play that
// starts starts it. A play asked for starts the sound card a moment after
// its Pause button shows, so a hand that held it in between held a card
// that was not running yet, issue #702: the card is held again the moment
// it says it runs. Without it a play the hand held from before the space
// bar ran on while the walk settled, to the clip's end.
//
// Let go, the card runs for as long as a play goes on, whoever stopped
// it: WebKit suspends the system's card on its own when its media session
// is interrupted, and left it suspended, see suspendPlayback in WebKit's
// AudioContext.cpp. Settles once the sound card has done what it was
// told.
const holders = new WeakMap<Play, Set<string>>();
const starting = new WeakMap<Play, Promise<void>>();
export function holdSound(play: Play, who: string, held: boolean): Promise<void> {
  let by = holders.get(play);
  if (!by) {
    const holding = (by = new Set());
    holders.set(play, holding);
    play.audio.addEventListener("statechange", () => {
      if (holding.size && play.audio.state === "running") void play.audio.suspend();
    });
  }
  if (held) by.add(who);
  else by.delete(who);
  if (by.size) return play.audio.state === "running" ? play.audio.suspend() : Promise.resolve();
  if (!play.playing || play.audio.state === "running" || play.audio.state === "closed") return Promise.resolve();
  // Asked once, not again on every tick while the browser takes its time.
  let start = starting.get(play);
  if (!start) {
    start = play.audio.resume().finally(() => starting.delete(play));
    starting.set(play, start);
  }
  return start;
}

// Hands every queue the page makes the walk's clock and sound card, unless
// the walk asked for the system's with window.__playClock, see open in
// walks/bridge.mjs. window.__holdSound(held) holds the sound card of the
// queue on the video preview by the walk's hand, or lets it go. Called by
// wails-bridge.ts before the app makes its first queue.
export function install() {
  const page = window as unknown as { __playClock?: string; __frameQueue?: Play; __holdSound?: (held: boolean) => Promise<void> };
  if (page.__playClock !== "sound") {
    FrameQueue.soundCard = (options) => new WalkCard(options) as unknown as AudioContext;
    FrameQueue.clock = walkClock;
  }
  page.__holdSound = (held) => {
    if (!page.__frameQueue) throw new Error("there is no video preview whose sound card the hand could hold");
    return holdSound(page.__frameQueue, "hand", held);
  };
}
