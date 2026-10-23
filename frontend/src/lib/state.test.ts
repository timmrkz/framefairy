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
