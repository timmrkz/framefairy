import { describe, expect, test, vi } from "vitest";

import type { Job } from "./api";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {} }));

const job = (state: string, seq: number, kind = "transcribe", id = "job-5"): Job =>
  ({ id, episode: "/v.mp4", kind, state, seq }) as Job;

// A Stop or Cancel stops what shows the work in the frame it is pressed,
// so the store keeps the press until the work says it ended. Found by the
// second review of #458, which saw it covered only by a walk, and kept
// once for both kinds of work since the third.
describe("a stop pressed for a video's work", () => {
  test("is stopping until the transcription ends, and not after", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1));
    jobs.stop("/v.mp4", "whole");
    jobs.stop("/v.mp4", "whole");
    expect(jobs.stopping).toEqual(["whole /v.mp4"]);
    jobs.apply(job("running", 2));
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    expect(jobs.stops(job("running", 2))).toBe(true);
    jobs.apply(job("interrupted", 3));
    expect(jobs.stopping).toEqual([]);
  });

  test("is still stopping when an older news of its end comes after a newer one of it running", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 4));
    jobs.stop("/v.mp4", "whole");
    jobs.apply(job("interrupted", 3));
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    jobs.apply(job("interrupted", 5));
    expect(jobs.stopping).toEqual([]);
  });

  test("stops the search and the clips made by hand, and lasts until the last of them ends", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1, "search", "s"));
    jobs.apply(job("queued", 1, "clip", "c"));
    jobs.apply(job("running", 1, "transcribe", "t"));
    jobs.stop("/v.mp4", "clips");
    expect(jobs.stops(job("running", 2, "search", "s"))).toBe(true);
    expect(jobs.stops(job("running", 2, "transcribe", "t"))).toBe(false);
    jobs.apply(job("interrupted", 2, "search", "s"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(true);
    jobs.apply(job("interrupted", 2, "clip", "c"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(false);
  });

  test("pressed before the Go side answered Search, lasts until the answer and its work have ended", () => {
    const jobs = new JobStore();
    jobs.press("/v.mp4", "clips");
    jobs.stop("/v.mp4", "clips");
    jobs.apply(job("running", 1, "search", "s"));
    jobs.answered("/v.mp4", "clips");
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(true);
    jobs.apply(job("interrupted", 2, "search", "s"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(false);
  });

  test("is over when the work is started again", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1));
    jobs.stop("/v.mp4", "whole");
    jobs.press("/v.mp4", "whole");
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(false);
    expect(jobs.isStarting("/v.mp4", "whole")).toBe(true);
  });
});

// A transcription settled, because the video left the library or Continue
// started the next one, is nothing for the menu to carry on. Tim removed a
// video, added the same file again, and its menu offered Continue
// transcribing, his report of 24 October on #458.
describe("the video's transcription", () => {
  test("is not one once it is settled", () => {
    const jobs = new JobStore();
    jobs.apply(job("interrupted", 1));
    expect(jobs.transcription("/v.mp4")?.state).toBe("interrupted");
    jobs.apply(job("cancelled", 2));
    expect(jobs.transcription("/v.mp4")).toBeUndefined();
  });
});
