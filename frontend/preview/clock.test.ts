import { afterEach, describe, expect, test, vi } from "vitest";
import { holdSound, walkClock, WalkCard } from "./clock";
import type { Play } from "../src/lib/frames/queue";

afterEach(() => {
  vi.useRealTimers();
});

// A play going on, on the walk's sound card, running from 0 s.
async function played() {
  const card = new WalkCard({ sampleRate: 1000 });
  const play = { playing: true as boolean, audio: card as unknown as AudioContext } satisfies Play;
  await card.resume();
  return { play, card, clock: walkClock(play) };
}

// The frame after the one on screen, ready, decoded and waiting with its
// captions, or still on its way, see unshown in queue.ts.
const ready = () => true;
const onItsWay = () => false;

// Lets what the sound card says after a call land, the way it does on
// the system's card: once the call has returned.
const said = () => new Promise<void>((done) => queueMicrotask(done));

describe("the walk's clock", () => {
  // A play of 3 s of a 5 fps video takes the 15 ticks that draw its
  // frames, and the sound card goes with the picture.
  test("goes on to a frame decoded and waiting at once", async () => {
    const { card, clock } = await played();
    let at = 0;
    for (let k = 1; k <= 15; k++) {
      at = clock(card.currentTime, k * 0.2, ready);
      expect(at).toBe(k * 0.2);
    }
    expect(card.currentTime).toBeCloseTo(3, 9);
  });

  // Issue #534: on the system's card a busy machine showed 2 frames of 9.
  test("stands on a frame still being decoded, and the sound card with it", async () => {
    const { card, clock } = await played();
    expect(clock(0, 0.2, onItsWay)).toBe(0);
    expect(clock(0, 0.2, onItsWay)).toBe(0);
    expect(card.currentTime).toBe(0);
    expect(clock(0, 0.2, ready)).toBe(0.2);
  });

  // A frame due before the moment heard is drawn first, never passed.
  test("stands on a frame due before the moment heard", async () => {
    const { card, clock } = await played();
    card.run(0.25);
    expect(clock(0.25, 0.2, ready)).toBe(0.2);
    expect(card.currentTime).toBe(0.25);
  });

  // A frame the queue says is nowhere, see unshown in queue.ts, never
  // comes, so the play goes by the time that passes, the way the system's
  // card goes.
  test("goes by the time that passed for a frame that is nowhere", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { card, clock } = await played();
    vi.advanceTimersByTime(100);
    expect(clock(0, Infinity, onItsWay)).toBeCloseTo(0.1, 9);
    expect(card.currentTime).toBeCloseTo(0.1, 9);
    // What a frame took the card on to is not counted again.
    clock(card.currentTime, 0.3, ready);
    vi.advanceTimersByTime(50);
    expect(clock(card.currentTime, Infinity, onItsWay)).toBeCloseTo(0.35, 9);
  });

  // The time the card stood on a frame is not made up for by the next
  // frame that is nowhere, which would pass the frames between.
  test("goes by the time since it last stood for a frame that is nowhere", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { card, clock } = await played();
    vi.advanceTimersByTime(500);
    expect(clock(0, 0.2, onItsWay)).toBe(0);
    vi.advanceTimersByTime(16);
    expect(clock(0, Infinity, onItsWay)).toBeCloseTo(0.016, 9);
    expect(card.currentTime).toBeCloseTo(0.016, 9);
  });

  // Issue #583: a walk holds the sound card while its hand drags, and the
  // play stands, picture and sound, however long the hand takes.
  test("moves nothing while the hand holds the sound card", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { play, card, clock } = await played();
    await holdSound(play, "hand", true);
    vi.advanceTimersByTime(5000);
    expect(clock(0, 0.2, ready)).toBe(0);
    expect(clock(0, Infinity, onItsWay)).toBe(0);
    expect(card.currentTime).toBe(0);
    await holdSound(play, "hand", false);
    expect(card.state).toBe("running");
    // The time held is not made up for once let go.
    expect(clock(0, Infinity, onItsWay)).toBe(0);
    expect(clock(0, 0.2, ready)).toBe(0.2);
  });

  // The card says it is held a task after the hand asks. A play that ran
  // on meanwhile, as fast as its frames came, ran a walk that held it at
  // each press of L past the clip's end.
  test("stands from the tick the hand asks, before the card says it is held", async () => {
    const { play, card, clock } = await played();
    void holdSound(play, "hand", true);
    expect(card.state).toBe("running");
    expect(clock(0, 0.2, ready)).toBe(0);
    expect(card.currentTime).toBe(0);
  });

  // The queue makes its clock before it opens its sound card.
  test("moves the sound card the play has when it ticks", async () => {
    const play = { playing: true, audio: undefined as unknown as AudioContext };
    const clock = walkClock(play);
    const card = new WalkCard({ sampleRate: 1000 });
    play.audio = card as unknown as AudioContext;
    await card.resume();
    expect(clock(0, 0.2, ready)).toBe(0.2);
  });

  test("refuses a sound card the walk does not move", () => {
    const play = { playing: true, audio: {} as AudioContext } satisfies Play;
    expect(() => walkClock(play)(0, 0.2, ready)).toThrow("the walk's clock moves only the walk's sound card");
  });
});

describe("the walk's sound card", () => {
  test("runs, is held and is closed, and says so once the call returned", async () => {
    const card = new WalkCard({ sampleRate: 48000 });
    const states: AudioContextState[] = [];
    card.onstatechange = () => states.push(card.state);
    expect([card.state, card.sampleRate]).toEqual(["suspended", 48000]);
    const resumed = card.resume();
    expect(card.state).toBe("suspended");
    await resumed;
    await card.resume();
    await card.suspend();
    await card.close();
    await card.resume();
    expect(states).toEqual(["running", "suspended", "closed"]);
  });

  // The frame queue counts sound it took off the card before it had
  // played, and forgets a stretch once the card says it ended.
  test("ends a stretch once its time has passed, or where it was stopped", async () => {
    const card = new WalkCard({ sampleRate: 1000 });
    await card.resume();
    const ended: string[] = [];
    const stretch = (name: string, seconds: number) => {
      const src = card.createBufferSource();
      src.buffer = card.createBuffer(1, seconds * 1000, 1000);
      src.connect(card.createGain()).connect(card.destination);
      src.onended = () => ended.push(name);
      return src;
    };
    const a = stretch("a", 0.5);
    const b = stretch("b", 0.5);
    const c = stretch("c", 0.5);
    a.start(0);
    b.start(0.5);
    c.start(0.5, 0.25);
    b.stop(0.6);
    card.run(0.5);
    await said();
    expect(ended).toEqual(["a"]);
    card.run(0.25);
    await said();
    expect(ended).toEqual(["a", "b", "c"]);
  });

  test("ends a stretch stopped before its time at once", async () => {
    const card = new WalkCard({ sampleRate: 1000 });
    const src = card.createBufferSource();
    src.buffer = card.createBuffer(1, 1000, 1000);
    let ended = 0;
    src.onended = () => ended++;
    expect(() => src.stop()).toThrow("a stretch of sound stops only once it started");
    src.start(2);
    expect(() => src.start(2)).toThrow("a stretch of sound starts once");
    src.stop();
    await said();
    expect(ended).toBe(1);
  });
});

// What the walk's sound card says it played, which "heard" and "stopped
// at its end" listen to in walks/playback/steps.mjs.
describe("what the walk's sound card played", () => {
  // A stretch of the samples given, at 1000 Hz, through the card's gain.
  async function playing() {
    const card = new WalkCard({ sampleRate: 1000 });
    await card.resume();
    const out = card.createGain();
    out.connect(card.destination);
    const stretch = (samples: number[]) => {
      const src = card.createBufferSource();
      src.buffer = card.createBuffer(1, samples.length, 1000);
      src.buffer.copyToChannel(Float32Array.from(samples), 0);
      src.connect(out);
      return src;
    };
    return { card, out, stretch };
  }
  const played = (card: WalkCard) => Array.from(card.heard().data);

  test("is every stretch it played since it recorded, each at its own frame", async () => {
    const { card, stretch } = await playing();
    stretch([9, 9]).start(0);
    card.run(0.002);
    card.record();
    stretch([1, 2, 3]).start(0.003);
    stretch([4, 5]).start(0.006);
    card.run(0.008);
    expect(card.heard()).toEqual({ frame: 2, data: Float32Array.from([0, 1, 2, 3, 4, 5, 0, 0]) });
  });

  test("plays only what reaches its output", async () => {
    const { card, stretch } = await playing();
    card.record();
    stretch([1, 1]).start(0);
    const astray = card.createBufferSource();
    astray.buffer = card.createBuffer(1, 2, 1000);
    astray.buffer.copyToChannel(Float32Array.from([5, 5]), 0);
    astray.connect(card.createGain());
    astray.start(0);
    card.run(0.002);
    expect(played(card)).toEqual([1, 1]);
  });

  test("plays nothing while it is held", async () => {
    const { card, stretch } = await playing();
    card.record();
    stretch([1, 1, 1, 1]).start(0);
    card.run(0.002);
    await card.suspend();
    card.run(0.002);
    await card.resume();
    card.run(0.001);
    expect(played(card)).toEqual([1, 1, 1]);
  });

  // The frame queue starts a stretch handed over late from where the
  // card is, skipping what it missed, and stops one at the program's end.
  test("starts a stretch from its offset and ends it where it was stopped, moved or not", async () => {
    const { card, stretch } = await playing();
    card.record();
    card.run(0.002);
    stretch([1, 2, 3, 4, 5]).start(0, 0.002);
    const b = stretch([6, 7, 8, 9]);
    b.start(0.005);
    b.stop(0.006);
    b.stop(0.007);
    card.run(0.008);
    expect(played(card)).toEqual([0, 0, 3, 4, 5, 6, 7, 0, 0, 0]);
  });

  // The frame queue ramps its gain from where it is when a play pauses
  // or goes on: it takes the ramp to come away and starts its own from
  // the value read.
  test("goes through the gain as it was set and ramped, and reads what it was last", async () => {
    const { card, out, stretch } = await playing();
    card.record();
    out.gain.setValueAtTime(1, 0.004);
    out.gain.linearRampToValueAtTime(0, 0.008);
    stretch(Array(10).fill(1)).start(0);
    card.run(0.002);
    expect(out.gain.value).toBe(1);
    card.run(0.004);
    expect(out.gain.value).toBe(0.5);
    out.gain.cancelScheduledValues(0.006);
    expect(out.gain.value).toBe(0.5);
    out.gain.setValueAtTime(out.gain.value, 0.006);
    out.gain.linearRampToValueAtTime(1, 0.008);
    card.run(0.004);
    expect(played(card)).toEqual([1, 1, 1, 1, 1, 0.75, 0.5, 0.75, 1, 1]);
  });
});

describe("the hand on the sound card", () => {
  // A paused play holds its sound card, and the hand let go after the
  // pause leaves it held.
  test("lets the sound card go only while a play goes on", async () => {
    const { play, card } = await played();
    await holdSound(play, "hand", true);
    play.playing = false;
    await holdSound(play, "hand", false);
    expect(card.state).toBe("suspended");
  });

  // WebKit suspends the system's card when its media session is
  // interrupted, and left it suspended in a play nobody paused. The hand
  // let go starts it again.
  test("starts again a sound card suspended while a play goes on", async () => {
    const { play, card } = await played();
    await card.suspend();
    await holdSound(play, "hand", false);
    expect(card.state).toBe("running");
  });

  // A card the browser is still bringing back counts as asked.
  test("asks a slow sound card to start once", async () => {
    const { play, card } = await played();
    await card.suspend();
    let asked = 0;
    let started = () => {};
    card.resume = () => {
      asked++;
      return new Promise<void>((done) => (started = done));
    };
    void holdSound(play, "hand", false);
    void holdSound(play, "hand", false);
    expect(asked).toBe(1);
    started();
    await said();
    await said();
    void holdSound(play, "hand", false);
    expect(asked).toBe(2);
  });

  // Issue #702: the hand held the sound card while the play it had just
  // asked for was still starting it, so there was nothing to hold yet,
  // and the play ran on to the clip's end. Then the card was held again
  // the moment it said it ran, a few milliseconds of sound later, and
  // Chromium moved the card's time on over the hold while the sound it
  // was playing went on where it was, so the sound clicked 85 ms into a
  // play. The app's start waits for the hand, and the card never runs
  // while it is held.
  test("starts a held sound card only once the hand lets go", async () => {
    const card = new WalkCard({ sampleRate: 1000 });
    const play = { playing: true, audio: card as unknown as AudioContext } satisfies Play;
    await holdSound(play, "hand", true);
    const states: AudioContextState[] = [];
    card.addEventListener("statechange", () => states.push(card.state));
    let started = false;
    const start = card.resume().then(() => (started = true));
    // The card does what it is asked in order, so once a hold asked after
    // the app's start has landed, the card has done what it would with
    // the start.
    await card.suspend();
    expect(states).toEqual([]);
    expect(started).toBe(false);
    await holdSound(play, "hand", false);
    await start;
    // The play that asked for it sees it run.
    expect(card.state).toBe("running");
    expect(states).toEqual(["running"]);
  });

  // A card started while the hand holds it, by anything but the app's
  // ask, is held again the moment it says it runs.
  test("holds again a sound card started some other way while the hand holds it", async () => {
    const card = new WalkCard({ sampleRate: 1000 });
    const play = { playing: true, audio: card as unknown as AudioContext } satisfies Play;
    await holdSound(play, "hand", true);
    const heldAgain = new Promise((done) => card.addEventListener("statechange", () => card.state === "suspended" && done(null)));
    await WalkCard.prototype.resume.call(card);
    await heldAgain;
    await holdSound(play, "hand", false);
    expect(card.state).toBe("running");
  });

  // The hold asked as the card said it runs lands a task later, so the
  // hand let go in between found the card running and left it, the hold
  // landed with nobody holding, and the play stood for good.
  test("starts the card again when a hold lands after the hand let go", async () => {
    const card = new WalkCard({ sampleRate: 1000 });
    const play = { playing: true, audio: card as unknown as AudioContext } satisfies Play;
    await holdSound(play, "hand", true);
    const states: AudioContextState[] = [];
    card.addEventListener("statechange", () => states.push(card.state));
    const back = new Promise((done) => card.addEventListener("statechange", () => states.length === 3 && done(null)));
    await WalkCard.prototype.resume.call(card);
    await holdSound(play, "hand", false);
    await back;
    expect(states).toEqual(["running", "suspended", "running"]);
  });
});
