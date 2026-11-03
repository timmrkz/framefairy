import { describe, expect, test, vi } from "vitest";

import type { Job } from "./api";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {}, onVideoState: () => {} }));

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
    jobs.apply(job("stopped", 3));
    expect(jobs.stopping).toEqual([]);
  });

  test("is still stopping when an older news of its end comes after a newer one of it running", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 4));
    jobs.stop("/v.mp4", "whole");
    jobs.apply(job("stopped", 3));
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    jobs.apply(job("stopped", 5));
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
    jobs.apply(job("stopped", 2, "search", "s"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(true);
    jobs.apply(job("stopped", 2, "clip", "c"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(false);
  });

  test("pressed before the Go side answered Search, lasts until the answer and its work have ended", () => {
    const jobs = new JobStore();
    jobs.press("/v.mp4", "clips");
    jobs.stop("/v.mp4", "clips");
    jobs.apply(job("running", 1, "search", "s"));
    jobs.answered("/v.mp4", "clips");
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(true);
    jobs.apply(job("stopped", 2, "search", "s"));
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

// A search or a clip made by hand stops the video's transcription on the
// Go side, which ends once the speech model has heard its piece. The menu
// said Stop transcribing for three and a half seconds after Search, Tim's
// report of 26 October, 14.22.
describe("the work on the clips asked for", () => {
  test("stops the video's transcription on the press, until it ends", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1, "transcribe", "t"));
    jobs.press("/v.mp4", "clips");
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    expect(jobs.stops(job("running", 2, "transcribe", "t"))).toBe(true);
    jobs.apply(job("running", 1, "search", "s"));
    jobs.answered("/v.mp4", "clips");
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    jobs.apply(job("stopped", 2, "transcribe", "t"));
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(false);
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(false);
  });

  test("stops a transcription asked for and not answered yet, and nothing when none runs", () => {
    const jobs = new JobStore();
    jobs.clipsAsked("/v.mp4");
    expect(jobs.stopping).toEqual([]);
    jobs.press("/v.mp4", "whole");
    jobs.clipsAsked("/v.mp4");
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
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
    jobs.apply(job("settled", 2));
    expect(jobs.transcription("/v.mp4")).toBeUndefined();
  });
});

// A search or a clip made by hand says how it ended until it is settled,
// and one done, cancelled or settled says nothing, issue #606.
describe("the work of the clip list", () => {
  test("is a search with something to say, and none once it is settled or cancelled", () => {
    const jobs = new JobStore();
    jobs.apply(job("stopped", 1, "search", "s"));
    expect(jobs.search("/v.mp4")?.state).toBe("stopped");
    jobs.apply(job("settled", 2, "search", "s"));
    expect(jobs.search("/v.mp4")).toBeUndefined();
    jobs.apply(job("cancelled", 3, "search", "t"));
    expect(jobs.search("/v.mp4")).toBeUndefined();
  });

  test("is every clip made by hand on its way or ended before it was done, and no other", () => {
    const jobs = new JobStore();
    for (const [n, state] of ["running", "failed", "interrupted", "stopped", "done", "cancelled", "settled"].entries()) {
      jobs.apply(job(state, n + 1, "clip", `c${n}`));
    }
    expect(jobs.clips("/v.mp4").map((j) => j.state)).toEqual(["running", "failed", "interrupted", "stopped"]);
  });
});

// A video's jobs come in its state, one channel, so its clips, its window
// and the clip chosen change in the same frame as the job that changed
// them, issue #513. A state holds every job of the video there is to say
// anything about, so a job it no longer holds is one the Go side forgot.
describe("the jobs of a video's state", () => {
  const state = (rev: number, jobs: Job[], video = "/v.mp4") =>
    ({ video, rev, jobs, clips: [], coverage: { passes: [] }, status: null, window: null, chosen: "" }) as never;

  test("are kept, each unless a later snapshot of it is", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 4, "search", "s"));
    jobs.take(state(5, [job("running", 3, "search", "s"), job("queued", 5, "render", "r")]));
    expect(jobs.list.map((j) => `${j.id} ${j.seq}`)).toEqual(["s 4", "r 5"]);
  });

  test("go when a newer state no longer holds them, and not for an older state or a newer job", () => {
    const jobs = new JobStore();
    jobs.take(state(5, [job("stopped", 2, "clip", "c"), job("done", 3, "search", "s")]));
    jobs.apply(job("running", 7, "clip", "late"));
    jobs.take(state(4, []));
    expect(jobs.list.map((j) => j.id)).toEqual(["c", "s", "late"]);
    jobs.take(state(6, [job("done", 3, "search", "s")]));
    expect(jobs.list.map((j) => j.id)).toEqual(["s", "late"]);
  });

  test("of one video leave another video's jobs alone", () => {
    const jobs = new JobStore();
    jobs.apply({ ...job("running", 1, "search", "other"), episode: "/w.mp4" });
    jobs.take(state(2, []));
    expect(jobs.list.map((j) => j.id)).toEqual(["other"]);
  });
});
