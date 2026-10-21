import { describe, expect, test } from "vitest";
import { describeEvenness, Even, type Source } from "./even";

// A play of a video at fps on a screen at hz, seconds long, the way the
// frame queue feeds it: on every animation frame the clock says where the
// sound is, and the frame that holds that place is put up when it is not
// up yet. clock turns the true place into the one the clock says, skip
// says which animation frames the page missed.
function play(
  fps: number,
  hz: number,
  seconds: number,
  clock: (pos: number) => number = (p) => p,
  skip: (i: number) => boolean = () => false,
  source: Source = "stamp",
): Even {
  const e = new Even();
  let shown = -1;
  for (let i = 0; i * (1 / hz) < seconds; i++) {
    if (skip(i)) continue;
    const at = (i * 1000) / hz;
    const pos = clock(i / hz);
    e.tick(at, pos, source);
    const k = Math.floor(pos * fps + 1e-9);
    if (k !== shown) {
      shown = k;
      e.put(at, k / fps);
    }
  }
  return e;
}

describe("Even", () => {
  test("a frame of 25 a second stands 2 or 3 refreshes at 60 Hz, which is even", () => {
    const e = play(25, 60, 4).evenness;
    expect(e.refresh).toBeCloseTo(1000 / 60);
    expect(e.missed).toBe(0);
    expect(e.uneven).toBe(0);
    expect([...e.held.keys()].sort()).toEqual([2, 3]);
    expect(e.stray).toBeLessThan(0.01);
    expect(e.rate).toBeCloseTo(1);
  });

  test("a frame of 30 a second stands 2 refreshes at 60 Hz and 4 at 120 Hz", () => {
    expect([...play(30, 60, 2).evenness.held.keys()]).toEqual([2]);
    expect([...play(30, 120, 2).evenness.held.keys()]).toEqual([4]);
    expect(play(25, 120, 2).evenness.uneven).toBe(0);
  });

  test("a refresh the page missed is counted, and the frame due on it stood a refresh too short", () => {
    // Frame 15 is due on refresh 30, missed, so it goes up a refresh late:
    // frame 14 stands 3 refreshes and frame 15 one.
    const e = play(30, 60, 2, undefined, (i) => i === 30).evenness;
    expect(e.missed).toBe(1);
    expect(e.missedRefreshes).toBe(1);
    expect(e.longest).toBeCloseTo(2000 / 60);
    expect(e.held.get(3)).toBe(1);
    expect(e.held.get(1)).toBe(1);
    expect(e.uneven).toBe(2);
    // A refresh missed between two frames changes nothing anyone sees.
    const between = play(30, 60, 2, undefined, (i) => i === 31).evenness;
    expect(between.missed).toBe(1);
    expect(between.uneven).toBe(0);
  });

  test("a clock that steps by the sound card's buffers puts frames up unevenly", () => {
    // 1024 samples at 48 kHz, the clock moving on only as each is played.
    const step = 1024 / 48000;
    const e = play(25, 60, 4, (p) => Math.floor(p / step) * step).evenness;
    expect(e.stray).toBeGreaterThan(4);
    expect(e.mostStray).toBeGreaterThan(9);
    expect(e.uneven).toBeGreaterThan(0);
    expect(e.missed).toBe(0);
  });

  test("nothing is measured across a time the app was behind another window", () => {
    const e = new Even();
    e.tick(0, 0, "stamp");
    e.put(0, 0);
    e.tick(16.7, 0.0167, "stamp");
    e.behind();
    e.tick(1016.7, 1.0167, "stamp");
    e.put(1016.7, 1);
    e.tick(1033.3, 1.0333, "stamp");
    const got = e.evenness;
    expect(got.missed).toBe(0);
    expect(got.frames).toBe(0);
    expect(got.longest).toBeCloseTo(16.7);
  });

  test("says where the moment heard came from", () => {
    const e = new Even();
    e.tick(0, 0, "stamp");
    e.tick(16, 0.016, "held");
    e.tick(33, 0.033, "plain");
    e.tick(50, 0.05, "held");
    expect(e.evenness.sources).toEqual({ stamp: 1, held: 2, plain: 1 });
  });

  test("reads in one line in the log", () => {
    const line = describeEvenness(play(25, 60, 1).evenness);
    expect(line).toContain("the screen at 60 Hz, 16.7 ms a refresh");
    expect(line).toContain("0 animation frames late by 0 refreshes in all");
    expect(line).toMatch(/frames stood for so many refreshes: \d+ for 2, \d+ for 3/);
    expect(line).toContain("0 frames a refresh longer or shorter than they last");
    expect(line).toContain("from the output timestamp 60 times, held to the sound card's time 0");
    expect(describeEvenness(new Even().evenness)).toBe("no animation frame came");
  });
});
