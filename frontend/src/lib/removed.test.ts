import { describe, expect, test } from "vitest";

import type { ClipEntry } from "./api";
import {
  secondThoughts,
  setAside,
  spent,
  takeUp,
  setAsideTold,
  takeUpTold,
  toldSpent,
} from "./removed";

const clip = (key: string) => ({ key }) as ClipEntry;

describe("a clip taken out", () => {
  test("is still there to put back on coming back to its episode", () => {
    setAside("a.mp4", { one: { clip: clip("one"), until: 5000 } });
    const back = takeUp("a.mp4", 2000);
    expect(Object.keys(back)).toEqual(["one"]);
    expect(spent(back.one, 2000)).toBe(secondThoughts - 3);
  });

  test("is gone when its time ran out while the episode was away", () => {
    setAside("a.mp4", { one: { clip: clip("one"), until: 5000 }, two: { clip: clip("two"), until: 9000 } });
    expect(Object.keys(takeUp("a.mp4", 6000))).toEqual(["two"]);
  });

  test("belongs to its own episode, and is taken up once", () => {
    setAside("a.mp4", { one: { clip: clip("one"), until: 5000 } });
    expect(takeUp("b.mp4", 1000)).toEqual({});
    expect(Object.keys(takeUp("a.mp4", 1000))).toEqual(["one"]);
    expect(takeUp("a.mp4", 1000)).toEqual({});
  });

  test("has used between none and all of its time", () => {
    const r = { clip: clip("one"), until: 20000 };
    expect(spent(r, 20000 - secondThoughts * 1000)).toBe(0);
    expect(spent(r, 0)).toBe(0);
    expect(spent(r, 25000)).toBe(secondThoughts);
  });
});

describe("a search that found nothing", () => {
  test("says so on coming back for the time it has left", () => {
    setAsideTold("a.mp4", { s1: 5000 });
    const back = takeUpTold("a.mp4", 2000);
    expect(back).toEqual({ s1: 5000 });
    expect(toldSpent(back.s1, 2000)).toBe(secondThoughts - 3);
  });

  test("is closed on coming back when its time ran out while away, and stays closed", () => {
    setAsideTold("a.mp4", { s1: 5000, s2: 9000, s3: 0 });
    expect(takeUpTold("a.mp4", 6000)).toEqual({ s1: 0, s2: 9000, s3: 0 });
  });

  test("belongs to its own episode, and is taken up once", () => {
    setAsideTold("a.mp4", { s1: 5000 });
    expect(takeUpTold("b.mp4", 1000)).toEqual({});
    expect(takeUpTold("a.mp4", 1000)).toEqual({ s1: 5000 });
    expect(takeUpTold("a.mp4", 1000)).toEqual({});
  });

  test("has used none of its time before it starts, and never more than all", () => {
    expect(toldSpent(undefined, 1000)).toBe(0);
    expect(toldSpent(1000 + secondThoughts * 1000, 1000)).toBe(0);
    expect(toldSpent(1000, 50000)).toBe(secondThoughts);
  });
});

describe("a clip made by hand that stopped and was removed", () => {
  test("keeps its time apart from a search's rows, and its run out time comes back as 0", () => {
    setAsideTold("a.mp4", { s1: 5000 });
    setAsideTold("a.mp4", { h1: 5000, h2: 9000 }, "put away");
    expect(takeUpTold("a.mp4", 6000, "put away")).toEqual({ h1: 0, h2: 9000 });
    expect(takeUpTold("a.mp4", 1000)).toEqual({ s1: 5000 });
    expect(takeUpTold("a.mp4", 1000, "put away")).toEqual({});
  });
});
