import { describe, expect, test } from "vitest";
import { columnLevels, HeldLoudness, laid, LoudnessLooks, loudnessOf, silent, type Loudness } from "./loudness";

const FRAME = 0.01;

// The bytes the Go side sends for these readings, laid out the way
// engine.Loudness lays them out: a head with the count, 10 ms a reading,
// -90 dB and two steps a decibel, the readings, then each level the
// loudest of two of the one before, down to one. Whether Go lays them out
// so is held by the walks, which draw what the real Go side sends and
// find no waveform at all when the two differ, see loudnessOf.
function sent(readings: number[], floor = -90, steps = 2): Uint8Array {
  const levels = [readings];
  while (levels[levels.length - 1].length > 1) {
    const below = levels[levels.length - 1];
    levels.push(Array.from({ length: Math.ceil(below.length / 2) }, (_, i) => Math.max(below[2 * i], below[2 * i + 1] ?? 0)));
  }
  const bytes = new Uint8Array(8 + levels.reduce((sum, l) => sum + (readings.length ? l.length : 0), 0));
  const head = new DataView(bytes.buffer);
  head.setUint32(0, readings.length, true);
  head.setUint16(4, FRAME * 1e6, true);
  head.setInt8(6, floor);
  head.setUint8(7, steps);
  let at = 8;
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

  test("takes bytes that do not add up exactly for no loudness", () => {
    expect(loudnessOf(new Uint8Array([3, 0, 0]))).toBe(silent);
    const whole = sent([10, 20, 30]);
    expect(loudnessOf(whole.subarray(0, whole.length - 1))).toBe(silent);
    const longer = new Uint8Array(whole.length + 1);
    longer.set(whole);
    expect(loudnessOf(longer)).toBe(silent);
    expect(loudnessOf(sent([])).count).toBe(0);
  });

  test("reads how loud a step is from the head, not from a copy of its own", () => {
    // A byte of 20 is 10 dB up from the floor at two steps a decibel, and
    // 5 dB up at four.
    expect(columnLevels(loudnessOf(sent([20])), 0, FRAME, 1)[0]).toBe(-80);
    expect(columnLevels(loudnessOf(sent([20], -60, 4)), 0, FRAME, 1)[0]).toBe(-55);
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

describe("HeldLoudness", () => {
  // The Go side's answers, held until the test lets each one go.
  function heldGo() {
    const asked: { path: string; answer: (l: Loudness) => void }[] = [];
    const ask = (path: string) => new Promise<Loudness>((answer) => asked.push({ path, answer }));
    return { asked, ask };
  }
  // Lets the answers given so far be read, a few turns of promises, with
  // no clock.
  const settle = async () => {
    for (let i = 0; i < 5; i++) await Promise.resolve();
  };

  test("reads when the stamp changes and only then, one reading at a time", async () => {
    const go = heldGo();
    const got: Loudness[] = [];
    const held = new HeldLoudness((_, l) => got.push(l), go.ask);
    held.want("a.mp4", "1", true);
    held.want("a.mp4", "1", true);
    expect(go.asked.length).toBe(1);
    // Two more stamps while the first is out are one reading once it is back.
    held.want("a.mp4", "2", true);
    held.want("a.mp4", "3", true);
    expect(go.asked.length).toBe(1);
    const first = loudnessOf(sent([1]));
    go.asked[0].answer(first);
    await settle();
    expect(got).toEqual([first]);
    expect(go.asked.length).toBe(2);
    go.asked[1].answer(loudnessOf(sent([2])));
    await settle();
    expect(go.asked.length).toBe(2);
    expect(got.length).toBe(2);
  });

  test("a clip timeline out of the page reads nothing until it is back", async () => {
    const go = heldGo();
    const held = new HeldLoudness(() => {}, go.ask);
    held.want("a.mp4", "1", false);
    held.want("a.mp4", "2", false);
    expect(go.asked.length).toBe(0);
    held.want("a.mp4", "2", true);
    expect(go.asked.map((a) => a.path)).toEqual(["a.mp4"]);
  });

  test("an answer for a video no longer shown is dropped", async () => {
    const go = heldGo();
    const got: string[] = [];
    const held = new HeldLoudness((path) => got.push(path), go.ask);
    held.want("a.mp4", "1", true);
    held.want("b.mp4", "1", true);
    go.asked[0].answer(silent);
    await settle();
    expect(got).toEqual([]);
    expect(go.asked.map((a) => a.path)).toEqual(["a.mp4", "b.mp4"]);
  });

  test("a reading that failed is not asked again until the stamp changes", async () => {
    let asked = 0;
    const held = new HeldLoudness(
      () => {},
      () => {
        asked++;
        return Promise.reject(new Error("gone"));
      },
    );
    held.want("a.mp4", "1", true);
    await settle();
    await settle();
    expect(asked).toBe(1);
    held.want("a.mp4", "2", true);
    await settle();
    expect(asked).toBe(2);
  });
});

// A piece as engine.LoudnessSchedule answers a look: the head of the
// loudness, the version, the first reading and the one after the last, a
// byte that says whether the view is measured whole, three of nothing,
// then each level's bytes of those readings, cut from what Go holds.
function piece(all: number[], first: number, last: number, version: number, whole: boolean): Uint8Array {
  const held = loudnessOf(sent(all));
  const parts: Uint8Array[] = [];
  if (last > first) {
    held.levels.forEach((level, k) => parts.push(level.slice(Math.floor(first / 2 ** k), Math.floor((last - 1) / 2 ** k) + 1)));
  }
  const bytes = new Uint8Array(24 + parts.reduce((sum, p) => sum + p.length, 0));
  bytes.set(sent(all).subarray(0, 8));
  const head = new DataView(bytes.buffer);
  head.setUint32(8, version, true);
  head.setUint32(12, first, true);
  head.setUint32(16, last, true);
  bytes[20] = whole ? 1 : 0;
  let at = 24;
  for (const p of parts) {
    bytes.set(p, at);
    at += p.length;
  }
  return bytes;
}

describe("laid", () => {
  // A piece laid over what the window holds gives the waveform of all
  // the readings, the levels above the piece included, the same as the
  // whole read again.
  test("a piece over what is held is the whole read again", () => {
    const before = speech(1000, 3);
    const after = before.slice();
    const fresh = speech(130, 9);
    after.splice(300, fresh.length, ...fresh);
    const held = loudnessOf(sent(before));
    const got = laid(held, piece(after, 300, 430, 7, false));
    expect(got?.version).toBe(7);
    expect(got?.whole).toBe(false);
    const want = loudnessOf(sent(after));
    expect(got?.loudness.levels.map((l) => Array.from(l))).toEqual(want.levels.map((l) => Array.from(l)));
    for (const [from, to, columns] of [[0, 10, 200], [2.5, 4.5, 300], [3, 3.6, 900]] as const) {
      expect(columnLevels(got!.loudness, from, to, columns)).toEqual(columnLevels(want, from, to, columns));
    }
  });

  // A window holding a loudness of another length is sent all of it,
  // which is the loudness.
  test("all of a loudness of another length is the loudness", () => {
    const all = speech(777, 5);
    const got = laid(loudnessOf(sent(speech(10, 1))), piece(all, 0, 777, 2, true));
    expect(got?.loudness.count).toBe(777);
    expect(got?.loudness.levels.map((l) => Array.from(l))).toEqual(loudnessOf(sent(all)).levels.map((l) => Array.from(l)));
  });

  // Bytes that do not add up are no piece, and what is held stays.
  test("a piece cut short changes nothing", () => {
    const held = loudnessOf(sent(speech(100, 2)));
    const was = Array.from(held.levels[0]);
    expect(laid(held, piece(speech(100, 4), 10, 50, 1, false).subarray(0, 40))).toBeNull();
    expect(Array.from(held.levels[0])).toEqual(was);
  });
});

describe("LoudnessLooks", () => {
  // Every move of the view is a look with what the window holds. A view
  // not measured whole is looked at again with the version that came,
  // until it is, and an answer older than what is held is dropped.
  test("a view is looked at again until it is measured whole", async () => {
    let held = loudnessOf(sent(speech(500, 1)));
    const asked: URLSearchParams[] = [];
    const answers = [piece(speech(500, 2), 100, 200, 3, false), piece(speech(500, 3), 100, 200, 4, true)];
    let got = 0;
    let both: () => void;
    const answered = new Promise<void>((r) => (both = r));
    const looks = new LoudnessLooks(
      () => held,
      (_, l) => {
        held = l;
        if (++got === 2) both();
      },
      async (q) => {
        asked.push(q);
        return answers.shift() ?? null;
      },
    );
    looks.look("/v.mp4", 1, 2);
    await answered;
    expect(asked.map((q) => [q.get("from"), q.get("to"), q.get("has"), q.get("count")])).toEqual([
      ["1", "2", "0", "500"],
      ["1", "2", "3", "500"],
    ]);
    expect(Number(asked[1].get("n"))).toBeGreaterThan(Number(asked[0].get("n")));
    expect(Array.from(held.levels[0].subarray(100, 200))).toEqual(speech(500, 3).slice(100, 200));
  });
});
