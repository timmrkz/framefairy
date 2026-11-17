import { describe, expect, test } from "vitest";
import { columnLevels, FRAME, loudnessOf, readLoudness, silent } from "./loudness";

// The bytes the Go side sends for these readings, made the way
// engine.Loudness makes them: the count, the readings, then each level the
// loudest of two of the one before, down to one.
function sent(readings: number[]): Uint8Array {
  const levels = [readings];
  while (levels[levels.length - 1].length > 1) {
    const below = levels[levels.length - 1];
    levels.push(Array.from({ length: Math.ceil(below.length / 2) }, (_, i) => Math.max(below[2 * i], below[2 * i + 1] ?? 0)));
  }
  const bytes = new Uint8Array(4 + levels.reduce((sum, l) => sum + (readings.length ? l.length : 0), 0));
  new DataView(bytes.buffer).setUint32(0, readings.length, true);
  let at = 4;
  for (const level of readings.length ? levels : []) {
    bytes.set(level, at);
    at += level.length;
  }
  return bytes;
}

// Readings that rise and fall the way speech does, with a grain, so a
// column that took the wrong reading shows.
function speech(n: number, seed: number): number[] {
  let s = seed;
  return Array.from({ length: n }, () => {
    s = (s * 1103515245 + 12345) % 2147483648;
    return s % 181;
  });
}

describe("loudnessOf", () => {
  test("reads the count and every level", () => {
    const l = loudnessOf(sent([0, 60, 180, 90, 0]));
    expect(l.count).toBe(5);
    expect(l.levels.map((level) => [...level])).toEqual([[0, 60, 180, 90, 0], [60, 180, 0], [180, 0], [180]]);
  });

  test("takes bytes that do not add up for no loudness", () => {
    expect(loudnessOf(new Uint8Array([3, 0, 0]))).toBe(silent);
    expect(loudnessOf(sent([10, 20, 30]).subarray(0, 8))).toBe(silent);
    expect(loudnessOf(sent([])).count).toBe(0);
  });

  test("reads what Wails hands over, the bytes as base64", () => {
    const bytes = sent([0, 180, 90]);
    const l = readLoudness(btoa(String.fromCharCode(...bytes)));
    expect([...l.levels[0]]).toEqual([0, 180, 90]);
  });
});

describe("columnLevels", () => {
  test("zoomed out, every column is the loudest reading in it, at any zoom", () => {
    const readings = speech(100_000, 7);
    const l = loudnessOf(sent(readings));
    // From a few readings a column to all thousand seconds over 900
    // columns, at views that start anywhere.
    for (const [from, to, columns] of [
      [0, 1000, 900],
      [3.217, 3.217 + 9 * FRAME * 2.5, 9],
      [12.5, 812.25, 1601],
      [999, 1000, 60],
    ]) {
      const got = columnLevels(l, from, to, columns);
      const scale = columns / (to - from);
      for (let x = 0; x < columns; x++) {
        const first = Math.floor((from + x / scale) / FRAME);
        const last = Math.min(Math.ceil((from + (x + 1) / scale) / FRAME - 1e-9) - 1, readings.length - 1);
        const want = Math.max(...readings.slice(first, last + 1)) / 2 - 90;
        if (got[x] !== want) throw new Error(`${from} to ${to} over ${columns}: column ${x} is ${got[x]} dB, want ${want}`);
      }
    }
  });

  test("zoomed in, the outline runs between the readings", () => {
    // Two readings, -90 and 0 dB, over 80 columns: each has 40 to itself,
    // and the line between them runs from the middle of the one to the
    // middle of the other.
    const got = columnLevels(loudnessOf(sent([0, 180])), 0, 2 * FRAME, 80);
    expect(got[0]).toBe(-90);
    expect(got[20]).toBe(-90);
    expect(got[40]).toBeCloseTo(-45, 5);
    expect(got[60]).toBe(0);
    expect(got[79]).toBe(0);
  });

  test("draws nothing where no reading falls, and nothing of no loudness", () => {
    const got = columnLevels(loudnessOf(sent([100, 100, 100, 100])), 0, 8 * FRAME, 4);
    expect([...got]).toEqual([-40, -40, -Infinity, -Infinity]);
    expect([...columnLevels(silent, 0, 10, 3)]).toEqual([-Infinity, -Infinity, -Infinity]);
  });
});
