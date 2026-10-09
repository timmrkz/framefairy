import { describe, expect, test } from "vitest";
import { Stretch } from "./stretch";

const RATE = 48000;

// A tone in chunks of 1024, the size the frame queue hands the sound card.
function stretched(speed: number, hz: number, seconds: number) {
  const total = Math.round(RATE * seconds);
  const s = new Stretch(2, RATE, speed, 5000);
  const out: number[] = [];
  let expect = 0;
  for (let at = 0; at < total; at += 1024) {
    const n = Math.min(1024, total - at);
    const ch = new Float32Array(n);
    for (let i = 0; i < n; i++) ch[i] = Math.sin((2 * Math.PI * hz * (at + i)) / RATE);
    const got = s.push([ch, ch], n);
    if (!got) continue;
    // Every piece carries on where the one before ended.
    if (got.at !== expect) throw new Error(`output at ${got.at}, expected ${expect}`);
    expect += got.length;
    for (const v of got.data[0]) out.push(v);
  }
  return out;
}

// The pitch, from how often the wave crosses zero going up, in the middle
// of the output away from the fade in at the start.
function pitch(x: number[]): number {
  const from = Math.floor(x.length / 4);
  const to = Math.floor((x.length * 3) / 4);
  let ups = 0;
  for (let i = from + 1; i < to; i++) if (x[i - 1] < 0 && x[i] >= 0) ups++;
  return ups / ((to - from) / RATE);
}

describe("stretch", () => {
  for (const speed of [0.25, 0.5, 0.75, 1.25, 1.5, 1.75, 2]) {
    test(`at ${speed}× the sound keeps its pitch and lasts as long as the play`, () => {
      const out = stretched(speed, 220, 3);
      // What the last windows need of the input after it is still to come
      // when the input stops: about two windows of it.
      const lags = (RATE * 0.06) / speed + 1024 / speed;
      expect(out.length).toBeLessThanOrEqual((3 * RATE) / speed);
      expect((3 * RATE) / speed - out.length).toBeLessThan(lags);
      expect(Math.abs(pitch(out) - 220)).toBeLessThan(220 * 0.03);
      // Laid down without gaps or doubling: the tone keeps its loudness.
      let loudest = 0;
      for (let i = Math.floor(out.length / 4); i < (out.length * 3) / 4; i++) loudest = Math.max(loudest, Math.abs(out[i]));
      expect(loudest).toBeLessThan(1.05);
      expect(loudest).toBeGreaterThan(0.9);
    });
  }
});
