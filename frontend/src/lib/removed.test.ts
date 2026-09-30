import { describe, expect, test } from "vitest";

import type { ClipEntry } from "./api";
import { secondThoughts, setAside, spent, takeUp } from "./removed";

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
