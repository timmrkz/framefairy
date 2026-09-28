import { describe, expect, test } from "vitest";
import type { Job } from "./api";
import { type Held, heldAtOnce, heldLater, partText, runningLine, sentence, stepLine, stoppedLine } from "./steps";

function job(over: Partial<Job>): Job {
  return { id: "j", episode: "e", kind: "search", label: "Find clips", state: "running", queued: "", lane: "finding", ...over };
}

describe("a search and a render say which step they are in", () => {
  test("each step in its own words, whatever the engine reports", () => {
    const busy = { kind: "progress", text: "2 of 12 found", fraction: 0.4, remaining: 61, elapsed: 1, time: "" };
    expect(stepLine(job({ step: "finding", progress: busy }))).toEqual({
      what: "Finding clips",
      left: "About 1:05 left",
      fraction: 0.4,
    });
    const heard = { kind: "progress", text: "transcribing", fraction: 0.9, remaining: 0, elapsed: 1, time: "" };
    expect(stepLine(job({ step: "hearing", lane: "hearing", progress: heard }), 0.25)).toEqual({
      what: "Transcribing",
      left: "",
      fraction: 0.25,
    });
    expect(stepLine(job({ kind: "render", step: "rendering", lane: "rendering" })).what).toBe("Rendering");
  });

  test("waiting says what it waits to do, by the lane it waits for", () => {
    expect(stepLine(job({ step: "waiting", lane: "hearing" })).what).toBe("Waiting to transcribe");
    expect(stepLine(job({ step: "waiting", lane: "finding" })).what).toBe("Waiting to find clips");
    expect(stepLine(job({ kind: "render", step: "waiting", lane: "rendering" })).what).toBe("Waiting to render");
  });

  test("no count of clips found, which the cards already show", () => {
    const found = { kind: "progress", text: "7 of 12 found", fraction: 0.8, remaining: 0, elapsed: 1, time: "" };
    expect(stepLine(job({ step: "finding", progress: found })).what).not.toMatch(/found/);
  });
});

test("a line of the engine's reads as a sentence", () => {
  expect(sentence("fetching 12 MB of 500 MB")).toBe("Fetching 12 MB of 500 MB");
  expect(sentence("  ")).toBe("");
});

describe("a search's next clip and a clip made by hand are one row", () => {
  const window = { from: 600, to: 2400 };
  const part = { from: 90, to: 210 };
  const heard = (covered: number) => ({ kind: "progress", text: "transcribing", fraction: 0.1, remaining: 0, elapsed: 1, time: "", covered });

  test("each says the part it transcribes, from where to where and how long", () => {
    const search = runningLine(job({ step: "hearing", progress: heard(1500) }), window, 1500, false);
    const hand = runningLine(job({ kind: "hand", step: "hearing", progress: heard(150) }), part, 150, false);
    expect(search).toEqual({ what: "Transcribing 30 min", left: "10:00 to 40:00", fraction: 0.5 });
    expect(hand).toEqual({ what: "Transcribing 2 min", left: "1:30 to 3:30", fraction: 0.5 });
  });

  test("Cancel makes both say Stopping in the same words, standing where they got to", () => {
    const search = runningLine(job({ step: "hearing" }), window, 1500, true);
    const hand = runningLine(job({ kind: "hand", step: "hearing" }), part, 150, true);
    expect(search.what).toBe("Stopping");
    expect(hand.what).toBe("Stopping");
    expect([search.still, search.now, hand.still, hand.now]).toEqual([true, true, true, true]);
  });

  test("stopped, both say so the same way, with their part", () => {
    const search = stoppedLine(job({ state: "interrupted", step: "stopped" }), window);
    const hand = stoppedLine(job({ kind: "hand", state: "interrupted", step: "stopped" }), part);
    expect(search).toEqual({ what: "Stopped. Click Continue", left: partText(window), fraction: -1, stopped: true });
    expect(hand).toEqual({ what: "Stopped. Click Continue", left: partText(part), fraction: -1, stopped: true });
    expect(stoppedLine(job({ kind: "hand", state: "failed", error: "no words" }), part).left).toBe("No words");
  });

  test("a length reads in seconds, minutes, then hours", () => {
    expect(partText({ from: 0, to: 22 })).toBe("0:00 to 0:22, 22 s");
    expect(partText({ from: 0, to: 5400 })).toBe("0:00 to 1:30:00, 90 min");
    expect(partText({ from: 0, to: 14400 })).toBe("0:00 to 4:00:00, 4 h 0 min");
  });
});

describe("a row is held long enough to be read", () => {
  const line = (what: string, fraction = 0.2, now = false) => ({ what, left: "", fraction, now });

  test("a click shows at once", () => {
    const held = { line: line("Transcribing"), since: 0 };
    expect(heldAtOnce(held, line("Stopping", 0.2, true), 100).line?.what).toBe("Stopping");
    expect(heldAtOnce(held, line("Finding clips"), 100).line?.what).toBe("Transcribing");
  });

  test("the engine's next step waits a second, while the fill moves on", () => {
    let held: Held = { line: line("Transcribing"), since: 0 };
    held = heldLater(held, line("Placing the crop", 0.4), 500);
    expect(held.line).toEqual({ what: "Transcribing", left: "", fraction: 0.4, now: false });
    held = heldLater(held, line("Placing the crop", 0.5), 1200);
    expect(held.line?.what).toBe("Placing the crop");
  });
});
