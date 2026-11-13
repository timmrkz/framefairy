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
// says so. Asked to, it keeps what it plays of them from then on, sample
// by sample, see record and heard, so a walk listens to a play on the
// walk's clock rather than for as long as the play lasts.
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
  // Every gain of the card, told what it was at each time the card moved.
  readonly params = new Set<WalkParam>();
  // What the card played since record, from the frame it began at.
  private recording: { since: number; parts: Float32Array[] } | null = null;

  constructor(options: AudioContextOptions = {}) {
    super();
    this.sampleRate = options.sampleRate ?? 48000;
  }

  get currentTime(): number {
    return this.time;
  }

  // The card's frame at its time now, the way the system's counts it.
  get frame(): number {
    return Math.round(this.time * this.sampleRate);
  }

  // Keeps what the card plays from now on, for heard.
  record() {
    this.recording = { since: this.frame, parts: [] };
  }

  // What the card played since record, a frame at a time from the frame
  // it began at: channel 0, as a recorder on the system's card reads it.
  heard(): { frame: number; data: Float32Array } {
    const parts = this.recording?.parts ?? [];
    const data = new Float32Array(parts.reduce((n, p) => n + p.length, 0));
    let at = 0;
    for (const p of parts) {
      data.set(p, at);
      at += p.length;
    }
    return { frame: this.recording?.since ?? this.frame, data };
  }

  // Moves the time on by seconds while the card runs, and says by how
  // much it moved. What it plays on the way is the sum of the stretches
  // playing, each through the gains it is connected to as they are now,
  // the way the system's card works out each moment as it comes to it: a
  // stretch starts and stops no earlier than the card's time, so nothing
  // after can change what it played.
  run(seconds: number): number {
    if (this.state !== "running" || !(seconds > 0)) return 0;
    const from = this.frame;
    this.time += seconds;
    this.moved = performance.now();
    if (this.recording && this.frame > from) this.recording.parts.push(this.play(from, this.frame));
    for (const p of this.params) p.reached(this.time);
    for (const s of this.playing) s.reached(this.time);
    return seconds;
  }

  private play(from: number, to: number): Float32Array {
    const data = new Float32Array(to - from);
    for (const s of this.playing) {
      const samples = s.samples;
      const lo = Math.max(from, s.from);
      const hi = Math.min(to, s.to);
      if (!samples || hi <= lo) continue;
      // Only what reaches the card's output is heard.
      const gains: WalkGain[] = [];
      let n: WalkNode | null = s.next;
      for (; n && n !== this.destination; n = n.next) if (n instanceof WalkGain) gains.push(n);
      if (!n) continue;
      for (let f = lo; f < hi; f++) {
        let x = samples[s.offset + f - s.from] ?? 0;
        for (const g of gains) x *= g.gain.at(f / this.sampleRate);
        data[f - from] += x;
      }
    }
    return data;
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
    return new WalkGain(this) as unknown as GainNode;
  }

  // A buffer that keeps its first channel, which is all heard reads.
  createBuffer(channels: number, length: number, rate: number): AudioBuffer {
    const buffer = {
      numberOfChannels: channels,
      length,
      sampleRate: rate,
      duration: length / rate,
      samples: null as Float32Array | null,
      copyToChannel(from: Float32Array, channel: number) {
        if (channel === 0) buffer.samples = Float32Array.from(from.subarray(0, length));
      },
    };
    return buffer as unknown as AudioBuffer;
  }

  createBufferSource(): AudioBufferSourceNode {
    return new WalkSource(this, this.playing) as unknown as AudioBufferSourceNode;
  }
}

// A node of the walk's sound card, which knows the one it is connected to,
// so heard can tell which gains a stretch passed through. A node let go of
// keeps it: the frame queue lets go of a stretch only once it has ended,
// and what was heard before stays as it was heard.
class WalkNode {
  next: WalkNode | null = null;
  connect<T>(to: T): T {
    if (to instanceof WalkNode) this.next = to;
    return to;
  }
  disconnect() {}
}

class WalkGain extends WalkNode {
  readonly gain: WalkParam;
  constructor(card: WalkCard) {
    super();
    this.gain = new WalkParam(card, 1);
  }
}

// A gain's value over the card's time, set at a moment or ramped to one in
// a straight line, the way the system's card has it. Read, it says what it
// was when the card last moved, as the system's does, so a change taken
// away goes on from the value heard and not from the one it would have
// had without the change.
class WalkParam {
  private events: { t: number; v: number; ramp: boolean }[] = [];
  private last: number;
  constructor(
    private card: WalkCard,
    private first: number,
  ) {
    this.last = first;
    card.params.add(this);
  }

  get value(): number {
    return this.last;
  }

  set value(v: number) {
    this.setValueAtTime(v, this.card.currentTime);
    this.last = v;
  }

  reached(t: number) {
    this.last = this.at(t);
  }

  setValueAtTime(v: number, t: number) {
    return this.add({ t, v, ramp: false });
  }

  linearRampToValueAtTime(v: number, t: number) {
    return this.add({ t, v, ramp: true });
  }

  cancelScheduledValues(t: number) {
    this.events = this.events.filter((e) => e.t < t);
    return this;
  }

  private add(e: { t: number; v: number; ramp: boolean }) {
    const i = this.events.findIndex((x) => x.t > e.t);
    this.events.splice(i < 0 ? this.events.length : i, 0, e);
    return this;
  }

  at(t: number): number {
    let v = this.first;
    let from = 0;
    for (const e of this.events) {
      if (e.t <= t) {
        v = e.v;
        from = e.t;
      } else return e.ramp ? v + ((e.v - v) * (t - from)) / (e.t - from) : v;
    }
    return v;
  }

}

// A stretch of sound on the walk's sound card. It ends at its own end or
// where it was stopped, once the card's time has passed that. from and to
// are the card's frames it plays between, and offset the first of its
// samples played. The frame queue asks for a card at the rate of its
// sound, which the walk's card always has, so a frame of the card is a
// sample of a stretch.
class WalkSource extends WalkNode {
  buffer: AudioBuffer | null = null;
  onended: (() => void) | null = null;
  from = 0;
  to = 0;
  offset = 0;
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

  get samples(): Float32Array | null {
    return (this.buffer as unknown as { samples?: Float32Array | null } | null)?.samples ?? null;
  }

  start(when = 0, offset = 0) {
    if (this.started) throw new DOMException("a stretch of sound starts once", "InvalidStateError");
    this.started = true;
    const from = Math.max(when, this.card.currentTime);
    this.ends = from + Math.max(0, (this.buffer?.duration ?? 0) - offset);
    const rate = this.card.sampleRate;
    this.from = Math.round(from * rate);
    this.offset = Math.round(offset * (this.buffer?.sampleRate ?? rate));
    this.to = this.from + Math.max(0, (this.buffer?.length ?? 0) - this.offset);
    this.playing.add(this);
    this.reached(this.card.currentTime);
  }

  // Moves the stop, as the system's card allows until the stretch has
  // ended. A stop asked for a moment already past stops it now.
  stop(when = 0) {
    if (!this.started) throw new DOMException("a stretch of sound stops only once it started", "InvalidStateError");
    if (this.done) return;
    this.stops = Math.max(when, this.card.currentTime);
    this.to = Math.min(this.from + Math.max(0, (this.buffer?.length ?? 0) - this.offset), Math.max(this.from, Math.round(this.stops * this.card.sampleRate)));
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
// bar ran on while the walk settled, to the clip's end. And a hold asked
// that way lands only after the card says so, so the hand can let go in
// between and find the card running: the card is started again the moment
// it says it was held with nobody holding it while a play goes on, or the
// play stood for good, as one HEVC playback walk did.
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
      const state = play.audio.state;
      if (holding.size && state === "running") void play.audio.suspend();
      else if (!holding.size && state === "suspended") void letGo(play);
    });
  }
  if (held) by.add(who);
  else by.delete(who);
  if (by.size) return play.audio.state === "running" ? play.audio.suspend() : Promise.resolve();
  return letGo(play);
}

// Starts a sound card nobody holds while a play goes on, asked once, not
// again on every tick while the browser takes its time.
function letGo(play: Play): Promise<void> {
  if (!play.playing || play.audio.state === "running" || play.audio.state === "closed") return Promise.resolve();
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
