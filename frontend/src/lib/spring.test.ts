import { describe, expect, test } from "vitest";

import { Spring } from "./spring";

// pull moves s to to in frames of ms milliseconds until it rests, and says
// how far it went at most, how near it came back down, and how long it
// took to reach nine tenths of the way and to rest.
function pull(s: Spring, to: number, ms = 16) {
  const from = s.at;
  let most = s.at;
  let least = s.at;
  let nine = 0;
  let t = 0;
  for (;;) {
    const rests = s.step(to, ms);
    t += ms;
    most = Math.max(most, s.at);
    least = Math.min(least, s.at);
    if (!nine && Math.abs(s.at - from) >= 0.9 * Math.abs(to - from)) nine = t;
    if (rests || t > 10000) return { most, least, nine, rest: t };
  }
}

describe("a spring with a bounce", () => {
  test("goes a little past where it is pulled to, quickly, and settles there", () => {
    const s = new Spring(0.4, 0.4);
    const up = pull(s, 1);
    expect(up.most).toBeGreaterThan(1.05);
    expect(up.most).toBeLessThan(1.15);
    expect(up.nine).toBeLessThan(200);
    expect(up.rest).toBeLessThan(1500);
    expect(s.at).toBe(1);
    expect(s.speed).toBe(0);
  });

  test("goes a little under where it falls back to, and settles there", () => {
    const s = new Spring(0.4, 0.4);
    s.at = 1;
    const down = pull(s, 0);
    expect(down.least).toBeLessThan(-0.05);
    expect(down.least).toBeGreaterThan(-0.15);
    expect(s.at).toBe(0);
  });

  test("moves as far in one long frame as in the frames of a 60 Hz or a 120 Hz screen", () => {
    const long = new Spring(0.4, 0.4);
    long.step(1, 100);
    for (const ms of [1000 / 60, 1000 / 120]) {
      const short = new Spring(0.4, 0.4);
      for (let t = 0; t < 100 - 1e-9; t += ms) short.step(1, Math.min(ms, 100 - t));
      expect(short.at).toBeCloseTo(long.at, 2);
      expect(short.speed).toBeCloseTo(long.speed, 1);
    }
  });

  test("pulled somewhere new while it moves, goes on from where it is", () => {
    const s = new Spring(0.4, 0.4);
    s.step(1, 64);
    const was = s.at;
    s.step(0.3, 4);
    expect(Math.abs(s.at - was)).toBeLessThan(0.05);
  });
});

describe("a spring with no bounce", () => {
  test("never goes past where it is pulled to", () => {
    const s = new Spring(0.4, 0);
    expect(pull(s, 1).most).toBeLessThanOrEqual(1);
  });
});
