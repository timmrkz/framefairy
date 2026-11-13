import { describe, expect, test } from "vitest";

import { atOnce, onTick, type Held } from "./headline";

const none: Held = { shown: null, since: 0 };
const reading = { what: "Reading", left: "about a minute", fraction: 0.1 };
const writing = { what: "Writing", left: "about 40 s", fraction: 0.3 };

describe("the row of a search", () => {
  test("shows work that starts at once, and nothing once it ends", () => {
    const started = atOnce(none, reading, 5000);
    expect(started).toEqual({ shown: reading, since: 5000 });
    expect(atOnce(started, null, 5100).shown).toBeNull();
  });

  test("holds a new headline until the one before has stood a second", () => {
    let held = onTick(none, reading, 0);
    held = onTick(held, writing, 999);
    // The headline waits, and the fill goes on.
    expect(held.shown).toEqual({ ...reading, left: writing.left, fraction: 0.3 });
    held = onTick(held, writing, 1000);
    expect(held).toEqual({ shown: writing, since: 1000 });
  });

  test("never takes the fill back while a headline waits", () => {
    let held = onTick(none, { ...reading, fraction: 0.5 }, 0);
    held = onTick(held, { ...writing, fraction: 0.2 }, 500);
    expect(held.shown?.fraction).toBe(0.5);
  });

  test("moves the fill of the same headline on at once, and keeps its time", () => {
    let held = onTick(none, reading, 0);
    held = onTick(held, { ...reading, fraction: 0.4 }, 300);
    expect(held).toEqual({ shown: { ...reading, fraction: 0.4 }, since: 0 });
  });

  test("keeps what it says while the work has nothing new to say", () => {
    const held = onTick(none, reading, 0);
    expect(onTick(held, { what: "", left: "", fraction: 0.9 }, 2000)).toBe(held);
  });

  test("puts a headline marked now in at once, and a change of still", () => {
    const held = onTick(none, reading, 0);
    const stopping = { what: "Stopping", left: "", fraction: 0.1, still: true, now: true };
    expect(atOnce(held, stopping, 100)).toEqual({ shown: { what: "Stopping", left: "", fraction: 0.1, still: true }, since: 100 });
    // Not marked now, it waits for the tick.
    expect(atOnce(held, writing, 100)).toBe(held);
  });
});
