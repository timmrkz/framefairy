import { describe, expect, test, vi } from "vitest";

import type { Job } from "./api";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {} }));

const job = (state: string, seq: number): Job => ({ id: "job-5", episode: "/v.mp4", kind: "transcribe", state, seq }) as Job;

// Stop transcribing stops what shows the work in the frame it is pressed,
// so the store keeps the press until the job says it ended. Found by the
// second review of #458, which saw it covered only by a walk.
describe("a transcription Stop was pressed for", () => {
  test("is stopping until it ends, and not after", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1));
    jobs.stop("job-5");
    jobs.stop("job-5");
    expect(jobs.stopping).toEqual(["job-5"]);
    jobs.apply(job("running", 2));
    expect(jobs.stopping).toEqual(["job-5"]);
    jobs.apply(job("interrupted", 3));
    expect(jobs.stopping).toEqual([]);
  });

  test("is still stopping when an older news of its end comes after a newer one of it running", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 4));
    jobs.stop("job-5");
    jobs.apply(job("interrupted", 3));
    expect(jobs.stopping).toEqual(["job-5"]);
    jobs.apply(job("interrupted", 5));
    expect(jobs.stopping).toEqual([]);
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
