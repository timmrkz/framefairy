import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";

vi.mock("./said", () => ({ said: vi.fn() }));

// A sound card of the test's own: it keeps every sound it is handed and
// how each was started, and plays none.
class Card {
  static made: Card[] = [];
  readonly sampleRate = 48000;
  state = "suspended";
  readonly destination = {};
  started: { buffer: { data: Float32Array }; how: unknown[] }[] = [];
  constructor() {
    Card.made.push(this);
  }
  resume() {
    this.state = "running";
    return Promise.resolve();
  }
  suspend() {
    this.state = "suspended";
    return Promise.resolve();
  }
  createBuffer(_channels: number, length: number) {
    const data = new Float32Array(length);
    return { data, getChannelData: () => data };
  }
  createBufferSource() {
    const card = this;
    return {
      buffer: null as unknown as { data: Float32Array },
      onended: null,
      connect() {},
      start(...how: unknown[]) {
        card.started.push({ buffer: this.buffer, how });
      },
    };
  }
}

// The pitch of a stretch of sound, in Hz, from how often it crosses
// nought.
function pitch(d: Float32Array, rate: number, from: number, to: number): number {
  let crossings = 0;
  for (let i = Math.round(from * rate) + 1; i < Math.round(to * rate); i++) if (d[i - 1] < 0 !== d[i] < 0) crossings++;
  return crossings / 2 / (to - from);
}

// How loud a stretch of sound is at its loudest.
function loudest(d: Float32Array, rate: number, from: number, to: number): number {
  let most = 0;
  for (let i = Math.round(from * rate); i < Math.round(to * rate); i++) most = Math.max(most, Math.abs(d[i]));
  return most;
}

let chime: typeof import("./chime");

beforeEach(async () => {
  Card.made = [];
  vi.stubGlobal("AudioContext", Card);
  vi.resetModules();
  chime = await import("./chime");
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// Issue #654: each note was started at a moment of its own, a hundredth
// of a second ahead, so a window that got to them after their end played
// nothing. The pair is one sound, started at once, so it plays whole and
// in order however late the window is to start it.
describe("the start and stop sounds", () => {
  test("are each one sound of both notes, started once and at once", () => {
    chime.wake();
    chime.chime("on");
    chime.chime("off");
    const [card] = Card.made;
    expect(card.started).toHaveLength(2);
    for (const { how } of card.started) expect(how).toEqual([]);
  });

  test("go up for on and down for off, the second note 75 ms after the first", () => {
    chime.wake();
    chime.chime("on");
    chime.chime("off");
    const [card] = Card.made;
    const rate = card.sampleRate;
    const [up, down] = card.started.map((s) => s.buffer.data);
    // E5 then B5 going up, B5 then E5 going down, each heard on its own
    // before the other comes in or after it has gone.
    expect(pitch(up, rate, 0.005, 0.07)).toBeCloseTo(659, -1);
    expect(pitch(up, rate, 0.115, 0.18)).toBeCloseTo(988, -1);
    expect(pitch(down, rate, 0.005, 0.07)).toBeCloseTo(988, -1);
    expect(pitch(down, rate, 0.115, 0.18)).toBeCloseTo(659, -1);
    // The second note starts at 75 ms: before it the sound is the first
    // note falling, after it it rises again.
    expect(loudest(up, rate, 0.075, 0.09)).toBeGreaterThan(loudest(up, rate, 0.06, 0.075));
  });

  test("sound off only after on, and on only once", () => {
    chime.wake();
    chime.chime("off");
    chime.chime("on");
    chime.chime("on");
    expect(Card.made[0].started).toHaveLength(1);
  });

  test("make no sound before the hand woke them", () => {
    chime.chime("on");
    expect(Card.made).toHaveLength(0);
  });
});
