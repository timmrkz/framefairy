import { describe, expect, test } from "vitest";
import { Reach, fitWindow, leastWindow, longestShortest, mostClips, type RoomView } from "./room";

// A line every four seconds, three seconds long, each weighing 100
// characters, for as long as the transcript goes.
function room(chars: number, heardTo: number, rate = 25): RoomView {
  const lines = [];
  for (let at = 0; at + 3 <= heardTo; at += 4) lines.push({ start: at, end: at + 3, chars: 100 });
  return { chars, by: "context", lines, heard: heardTo, rate };
}

describe("how far a window may reach", () => {
  test("a window weighs the lines it touches", () => {
    const r = new Reach(room(1e9, 400), 400);
    expect(r.weight(0, 4)).toBe(100);
    // Touching a line counts it whole.
    expect(r.weight(0, 4.5)).toBe(200);
    expect(r.weight(3.5, 4)).toBe(0);
    expect(r.weight(0, 400)).toBe(10000);
  });

  test("what is not heard yet weighs the rate", () => {
    const r = new Reach(room(1e9, 40), 400);
    expect(r.weight(39, 40)).toBe(0);
    expect(r.weight(100, 110)).toBe(250);
    expect(r.weight(0, 50)).toBe(1000 + 25 * (50 - 40));
  });

  test("the end goes no further than the room", () => {
    // 1,000 characters are ten lines, and the eleventh starts at 40.
    const r = new Reach(room(1000, 400), 400);
    const end = r.longestFrom(0);
    expect(end).toBeGreaterThan(39.99);
    expect(end).toBeLessThanOrEqual(40);
    expect(r.fits(0, end)).toBe(true);
    expect(r.fits(0, end + 0.01)).toBe(false);
    const start = r.earliestTo(400);
    expect(r.fits(start, 400)).toBe(true);
    expect(r.fits(start - 0.01, 400)).toBe(false);
  });

  test("a room that holds the episode holds any window", () => {
    const r = new Reach(room(1e9, 400), 400);
    expect(r.longestFrom(0)).toBe(400);
    expect(r.earliestTo(400)).toBe(0);
    expect(r.anywhere()).toBe(400);
  });

  test("no room yet holds nothing back", () => {
    const r = new Reach(null, 400);
    expect(r.fits(0, 400)).toBe(true);
    expect(r.longestFrom(10)).toBe(400);
    expect(r.anywhere()).toBe(400);
  });

  test("anywhere is the densest stretch, not the end of the episode", () => {
    // Heard for 200 seconds, at 25 a second after: the heard part is the
    // denser, at 100 characters every four seconds.
    const r = new Reach(room(2000, 200), 4000);
    const reach = r.anywhere();
    expect(reach).toBeLessThanOrEqual(80);
    expect(reach).toBeGreaterThan(75);
    // From anywhere a window of that length fits, a start inside a line
    // included.
    for (let s = 0; s + reach <= 4000; s += 0.37) expect(r.fits(s, s + reach)).toBe(true);
  });
});

describe("what the clip settings may ask for", () => {
  test("a window has room for its clips at their shortest", () => {
    expect(leastWindow(12, 20, 3600)).toBe(240);
    expect(leastWindow(1, 5, 3600)).toBe(10);
    // Never longer than the episode.
    expect(leastWindow(12, 20, 100)).toBe(100);
  });

  test("no more clips than fit in the longest window", () => {
    expect(mostClips(1800, 20, 30)).toBe(30);
    expect(mostClips(200, 20, 30)).toBe(10);
    expect(mostClips(10, 20, 30)).toBe(1);
  });

  test("no shortest clip too long for the clips asked for", () => {
    expect(longestShortest(1800, 12, 5, 180)).toBe(150);
    expect(longestShortest(240, 12, 5, 180)).toBe(20);
    expect(longestShortest(30, 12, 5, 180)).toBe(5);
  });
});

describe("a window put back inside what it may be", () => {
  test("too long for the model: the end gives way", () => {
    const r = new Reach(room(1000, 400), 400);
    const w = fitWindow({ from: 100, to: 400 }, r, 20);
    expect(w.from).toBe(100);
    expect(r.fits(w.from, w.to)).toBe(true);
    expect(w.to).toBeGreaterThan(130);
  });

  test("too short for its clips: it grows, backwards at the end", () => {
    const r = new Reach(room(1e9, 400), 400);
    expect(fitWindow({ from: 100, to: 120 }, r, 60)).toEqual({ from: 100, to: 160 });
    expect(fitWindow({ from: 380, to: 400 }, r, 60)).toEqual({ from: 340, to: 400 });
  });

  test("a window that fits stays as it is", () => {
    const r = new Reach(room(1e9, 400), 400);
    expect(fitWindow({ from: 10, to: 300 }, r, 60)).toEqual({ from: 10, to: 300 });
  });
});
