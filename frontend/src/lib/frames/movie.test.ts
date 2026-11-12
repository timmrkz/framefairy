import { describe, expect, test } from "vitest";
import { movieOf, PACKET, rankAt, rankOf, ticks } from "./movie";

describe("movieOf", () => {
  test("is the frames the engine names and the sound in packets after one another", () => {
    // 29.97 frames a second, the grid starting inside a picture that
    // starts a quarter of a second after the sound.
    const m = movieOf({
      duration: 7,
      width: 640,
      height: 360,
      grid: { origin: 0, start: 0.016433, num: 30000, den: 1001, count: 210 },
      soundRate: 48000,
      soundChannels: 2,
    });
    const v = m.video!;
    expect(v.samples.count).toBe(210);
    expect(v.frame).toBeCloseTo(1001 / 30000, 12);
    expect(ticks(v.samples, 0) / v.samples.timescale).toBeCloseTo(0.016433, 9);
    expect(ticks(v.samples, 100) / v.samples.timescale).toBeCloseTo(0.016433 + (100 * 1001) / 30000, 9);
    expect(ticks(v.samples, 209) / v.samples.timescale).toBeCloseTo(0.016433 + (209 * 1001) / 30000, 9);
    const a = m.audio!;
    expect([a.sampleRate, a.channels]).toEqual([48000, 2]);
    expect(a.samples.count).toBe(Math.ceil((7 * 48000) / PACKET));
    expect(ticks(a.samples, 3)).toBe(3 * PACKET);
    expect(a.samples.step).toBe(PACKET);
  });

  test("has no picture without a grid and no sound without a rate", () => {
    const m = movieOf({ duration: 3, width: 2, height: 2 });
    expect(m.video).toBeUndefined();
    expect(m.audio).toBeUndefined();
    expect(m.duration).toBe(3);
  });
});

describe("rankAt", () => {
  const s = movieOf({ duration: 4, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 100 } }).video!.samples;
  test("is the frame that holds the moment", () => {
    expect(rankAt(s, 0)).toBe(0);
    expect(rankAt(s, 0.038)).toBe(0);
    expect(rankAt(s, 0.04)).toBe(1);
    // A frame's start kept to the millisecond, a hair before it, is in it.
    expect(rankAt(s, 0.0396)).toBe(1);
    expect(rankAt(s, 59.53 - 57)).toBe(63);
    // A sum of seconds that comes out a hair before a frame is in it.
    expect(rankAt(s, 0.1 + 0.2 - 0.3 + 0.04 - 1e-12)).toBe(1);
    expect(rankAt(s, -5)).toBe(0);
    expect(rankAt(s, 1000)).toBe(99);
  });

  // The grid of a picture that starts after its sound begins inside the
  // file, less than a frame from its start, and every frame of it is a
  // whole number of frames after that, as the engine counts them.
  test("counts from where the grid starts", () => {
    const late = movieOf({ duration: 2, width: 2, height: 2, grid: { origin: 0, start: 0.016433, num: 30000, den: 1001, count: 60 } }).video!.samples;
    const frame = 1001 / 30000;
    expect(rankAt(late, 0)).toBe(0);
    expect(rankAt(late, 0.016433 + 7 * frame)).toBe(7);
    expect(rankAt(late, 0.25)).toBe(7);
    expect(rankAt(late, 0.016433 + 8 * frame - 0.0005)).toBe(8);
  });

  // A grid at a whole rate counts in ticks of a whole frame, and a picture
  // that starts 4 ms after its sound starts the grid there. Every moment of
  // a frame is in that frame, not only its last 4 ms: a moment rounded down
  // to whole ticks showed the frame before for the rest, so a keyframe or
  // a clip's first frame was never the one shown, Tim's report on #752.
  test("counts from a grid that starts inside a tick", () => {
    const late = movieOf({ duration: 30, width: 2, height: 2, grid: { origin: 0, start: 0.004, num: 25, den: 1, count: 750 } }).video!.samples;
    for (const k of [0, 1, 255, 256, 749]) {
      const begins = 0.004 + k * 0.04;
      expect(rankAt(late, begins)).toBe(k);
      expect(rankAt(late, begins + 0.02)).toBe(k);
      expect(rankAt(late, begins + 0.038)).toBe(k);
      // Kept to the millisecond, a hair before it.
      expect(rankAt(late, Math.floor(begins * 1000) / 1000)).toBe(k);
    }
    expect(rankAt(late, 0.002)).toBe(0);
    expect(rankAt(late, 10.263822877763761)).toBe(256);
    // Deep into a long video, 4.8 hours at 25 frames a second.
    const long = movieOf({ duration: 17351, width: 2, height: 2, grid: { origin: 0, start: 0.004, num: 25, den: 1, count: 433754 } }).video!.samples;
    expect(rankAt(long, 2609.044)).toBe(65226);
    expect(rankAt(long, 2609.07)).toBe(65226);
    expect(rankAt(long, 2596.564)).toBe(64914);
  });

  // A frame the Go side handed over is the one whose start its moment is
  // nearest, a rounding early or late.
  // The grid goes with the track, so a stream hands back the grid the
  // page counts on, clock and all.
  test("keeps the grid it counts on", () => {
    const g = { origin: -0.021, start: 0.033367, num: 30000, den: 1001, count: 90 };
    expect(movieOf({ duration: 3, width: 2, height: 2, grid: g }).video!.grid).toEqual(g);
  });

  test("finds the frame a moment the Go side gave stands for", () => {
    const g = { origin: 0, start: 0.016433, num: 30000, den: 1001, count: 60 };
    const late = movieOf({ duration: 2, width: 2, height: 2, grid: g }).video!.samples;
    for (let r = 0; r < 60; r++) {
      const at = g.start + (r * g.den) / g.num;
      for (const off of [-0.0008, 0, 0.0008]) expect(rankOf(late, at + off)).toBe(r);
    }
  });
});
