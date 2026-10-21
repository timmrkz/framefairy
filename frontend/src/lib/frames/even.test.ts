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
  // How the page's clock rounds a time, milliseconds.
  round: (ms: number) => number = (ms) => ms,
): Even {
  const e = new Even();
  let shown = -1;
  for (let i = 0; i * (1 / hz) < seconds; i++) {
    if (skip(i)) continue;
    const at = round((i * 1000) / hz);
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

  test("the wait for the sound to start is said apart and does not count as the clock straying", () => {
    // A play stands on its first frame until the sound card reaches the
    // moment the sound starts, START_LEAD and a refresh or two in queue.ts,
    // 45 ms in the cloud. That stand read as the clock 23 to 47 ms off its
    // line once every play, on Tim's Mac and in the cloud.
    const wait = 0.045;
    const e = play(25, 60, 4, (p) => Math.max(0, p - wait)).evenness;
    expect(e.startWait).toBeCloseTo(45, 0);
    expect(e.stray).toBeLessThan(0.01);
    expect(e.mostStray).toBeLessThan(0.01);
    expect(e.rate).toBeCloseTo(1);
    expect(play(25, 60, 4).evenness.startWait).toBe(0);
  });

  test("a screen that changes its rate during a play counts no refresh as missed", () => {
    // The app runs the screen at 50 Hz while a video of 25 frames a second
    // plays, see MatchScreen in cmd/framefairy-app/match.go, and the
    // screen may take a moment to change: the play starts at 120 Hz.
    const e = new Even();
    let shown = -1;
    const at: number[] = [];
    for (let t = 0; t < 3000; t += 1000 / 120) at.push(t);
    for (let t = at[at.length - 1] + 20; t < 5000; t += 20) at.push(t);
    for (const t of at) {
      e.tick(t, t / 1000, "stamp");
      const k = Math.floor((t / 1000) * 25 + 1e-9);
      if (k !== shown) {
        shown = k;
        e.put(t, k / 25);
      }
    }
    const got = e.evenness;
    expect(got.missed).toBe(0);
    expect(got.uneven).toBe(0);
    expect([...got.held.keys()].sort()).toEqual([2, 4, 5]);
    expect(got.refresh).toBeCloseTo(20);
    expect(got.firstRefresh).toBeCloseTo(1000 / 120);
    expect(describeEvenness(got)).toContain("the screen at 50 Hz, 20.0 ms a refresh, 120 Hz as the play began");
  });

  test("a frame of 30 a second stands 2 refreshes at 60 Hz and 4 at 120 Hz", () => {
    expect([...play(30, 60, 2).evenness.held.keys()]).toEqual([2]);
    expect([...play(30, 120, 2).evenness.held.keys()]).toEqual([4]);
    expect(play(25, 120, 2).evenness.uneven).toBe(0);
  });

  test("a screen of 120 Hz reads as 120 Hz on a clock of whole milliseconds, and 25 frames a second on it as even", () => {
    // WebKit gives the page its times to the whole millisecond, so the
    // animation frames come 8 and 9 ms apart. Tim's Mac on 21 October.
    const e = play(25, 120, 4, undefined, undefined, "held", Math.round).evenness;
    expect(Math.round(1000 / e.refresh)).toBe(120);
    expect([...e.held.keys()].sort()).toEqual([4, 5]);
    expect(e.uneven).toBe(0);
    expect(e.missed).toBe(0);
  });

  test("the refresh as the play goes follows the screen, from 60 Hz until it is measured", () => {
    const e = new Even();
    expect(e.refreshNow).toBeCloseTo(1000 / 60);
    for (let i = 0; i < 40; i++) e.tick(Math.round((i * 1000) / 120), i / 120, "held");
    expect(1000 / e.refreshNow).toBeCloseTo(120, 0);
    // A refresh missed now and then does not move it.
    e.tick(Math.round((41 * 1000) / 120), 41 / 120, "held");
    expect(1000 / e.refreshNow).toBeCloseTo(120, 0);
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

  test("says what the animation frame before a missed refresh spent its time on", () => {
    const e = new Even();
    for (let i = 0; i < 20; i++) {
      // Refresh 10 is missed: the app took 20 ms to be told on refresh 9.
      if (i === 10) continue;
      const at = (i * 1000) / 60;
      e.tick(at, i / 60, "held");
      const tell = i === 9 ? 20 : 1;
      const w = e.work({ late: i === 11 ? 5 : 0.5, put: 2, tell, feed: 0.5, all: 3 + tell });
      e.drawn(w, at + 3 + tell + 2, at + 3 + tell);
    }
    const spent = e.evenness.spent;
    expect(e.evenness.missed).toBe(1);
    expect(spent.tell).toEqual({ typical: 1, most: 20, beforeMissed: 20 });
    expect(spent.put.beforeMissed).toBe(2);
    expect(spent.late.beforeMissed).toBe(5);
    expect(spent.after.typical).toBe(2);
    expect(describeEvenness(e.evenness)).toContain("telling the app 1.0/20.0/20.0");
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
