import { describe, expect, test } from "vitest";
import type { Job } from "./api";
import { sentence, stepLine } from "./steps";

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
