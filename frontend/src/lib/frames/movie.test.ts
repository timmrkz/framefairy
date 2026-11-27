import { describe, expect, test } from "vitest";
import { movieOf, PACKET, ticks } from "./movie";

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

  // The grid goes with the track, so a stream hands back the grid the
  // page counts on, clock and all.
  test("keeps the grid it counts on", () => {
    const g = { origin: -0.021, start: 0.033367, num: 30000, den: 1001, count: 90 };
    expect(movieOf({ duration: 3, width: 2, height: 2, grid: g }).video!.grid).toEqual(g);
  });
});
