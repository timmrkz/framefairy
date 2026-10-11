import { describe, expect, test } from "vitest";
import { movieOf, PACKET, rankAt, rankOf } from "./movie";

describe("movieOf", () => {
  test("is the frames the engine names and the sound in packets after one another", () => {
    // 29.97 frames a second, the grid starting inside a picture that
    // starts a quarter of a second after the sound.
    const m = movieOf({
      duration: 7,
      width: 640,
      height: 360,
      grid: { start: 0.016433, num: 30000, den: 1001, count: 210 },
      soundRate: 48000,
      soundChannels: 2,
    });
    const v = m.video!;
    expect(v.samples.count).toBe(210);
    expect(v.frame).toBeCloseTo(1001 / 30000, 12);
    expect(v.samples.pts[0] / v.samples.timescale).toBeCloseTo(0.016433, 9);
    expect(v.samples.pts[100] / v.samples.timescale).toBeCloseTo(0.016433 + (100 * 1001) / 30000, 9);
    // Every frame of the grid can be asked for on its own, in the order
    // it is shown.
    expect(v.samples.key.every((k) => k === 1)).toBe(true);
    expect(v.samples.order[57]).toBe(57);
    expect(v.samples.rank[57]).toBe(57);
    const a = m.audio!;
    expect([a.sampleRate, a.channels]).toEqual([48000, 2]);
    expect(a.samples.count).toBe(Math.ceil((7 * 48000) / PACKET));
    expect(a.samples.pts[3]).toBe(3 * PACKET);
    expect(a.samples.duration[3]).toBe(PACKET);
  });

  test("has no picture without a grid and no sound without a rate", () => {
    const m = movieOf({ duration: 3, width: 2, height: 2 });
    expect(m.video).toBeUndefined();
    expect(m.audio).toBeUndefined();
    expect(m.duration).toBe(3);
  });
});

describe("rankAt", () => {
  const s = movieOf({ duration: 4, width: 2, height: 2, grid: { start: 0, num: 25, den: 1, count: 100 } }).video!.samples;
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
    const late = movieOf({ duration: 2, width: 2, height: 2, grid: { start: 0.016433, num: 30000, den: 1001, count: 60 } }).video!.samples;
    const frame = 1001 / 30000;
    expect(rankAt(late, 0)).toBe(0);
    expect(rankAt(late, 0.016433 + 7 * frame)).toBe(7);
    expect(rankAt(late, 0.25)).toBe(7);
    expect(rankAt(late, 0.016433 + 8 * frame - 0.0005)).toBe(8);
  });

  // A frame the Go side handed over is the one whose start its moment is
  // nearest, a rounding early or late.
  test("finds the frame a moment the Go side gave stands for", () => {
    const g = { start: 0.016433, num: 30000, den: 1001, count: 60 };
    const late = movieOf({ duration: 2, width: 2, height: 2, grid: g }).video!.samples;
    for (let r = 0; r < 60; r++) {
      const at = g.start + (r * g.den) / g.num;
      for (const off of [-0.0008, 0, 0.0008]) expect(rankOf(late, at + off)).toBe(r);
    }
  });
});
