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
      at = clock(card.currentTime, k * 0.2, true);
      expect(at).toBe(k * 0.2);
    }
    expect(card.currentTime).toBeCloseTo(3, 9);
  });

  // Issue #534: on the system's card a busy machine showed 2 frames of 9.
  test("stands on a frame still being decoded, and the sound card with it", async () => {
    const { card, clock } = await played();
    expect(clock(0, 0.2, false)).toBe(0);
    expect(clock(0, 0.2, false)).toBe(0);
    expect(card.currentTime).toBe(0);
    expect(clock(0, 0.2, true)).toBe(0.2);
  });

  // A frame due before the moment heard is drawn first, never passed.
  test("stands on a frame due before the moment heard", async () => {
    const { card, clock } = await played();
    card.run(0.25);
    expect(clock(0.25, 0.2, true)).toBe(0.2);
    expect(card.currentTime).toBe(0.25);
  });

  // A frame the queue says is nowhere, see unshown in queue.ts, never
  // comes, so the play goes by the time that passes, the way the system's
  // card goes.
  test("goes by the time that passed for a frame that is nowhere", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { card, clock } = await played();
    vi.advanceTimersByTime(100);
    expect(clock(0, Infinity, false)).toBeCloseTo(0.1, 9);
    expect(card.currentTime).toBeCloseTo(0.1, 9);
    // What a frame took the card on to is not counted again.
    clock(card.currentTime, 0.3, true);
    vi.advanceTimersByTime(50);
    expect(clock(card.currentTime, Infinity, false)).toBeCloseTo(0.35, 9);
  });

  // Issue #583: a walk holds the sound card while its hand drags, and the
  // play stands, picture and sound, however long the hand takes.
  test("moves nothing while the hand holds the sound card", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { play, card, clock } = await played();
    await holdSound(play, "hand", true);
    vi.advanceTimersByTime(5000);
    expect(clock(0, 0.2, true)).toBe(0);
    expect(clock(0, Infinity, false)).toBe(0);
    expect(card.currentTime).toBe(0);
    await holdSound(play, "hand", false);
    expect(card.state).toBe("running");
    // The time held is not made up for once let go.
    expect(clock(0, Infinity, false)).toBe(0);
    expect(clock(0, 0.2, true)).toBe(0.2);
  });

  // The queue makes its clock before it opens its sound card.
  test("moves the sound card the play has when it ticks", async () => {
    const play = { playing: true, audio: undefined as unknown as AudioContext };
    const clock = walkClock(play);
    const card = new WalkCard({ sampleRate: 1000 });
    play.audio = card as unknown as AudioContext;
    await card.resume();
    expect(clock(0, 0.2, true)).toBe(0.2);
  });

  test("refuses a sound card the walk does not move", () => {
    const play = { playing: true, audio: {} as AudioContext } satisfies Play;
    expect(() => walkClock(play)(0, 0.2, true)).toThrow("the walk's clock moves only the walk's sound card");
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
  // and the play ran on to the clip's end. The card is held again the
  // moment it says it runs.
  test("holds the sound card for the hand once the app has started it", async () => {
    const card = new WalkCard({ sampleRate: 1000 });
    const play = { playing: true, audio: card as unknown as AudioContext } satisfies Play;
    await holdSound(play, "hand", true);
    await card.resume();
    await said();
    expect(card.state).toBe("suspended");
    await holdSound(play, "hand", false);
    expect(card.state).toBe("running");
  });
});
