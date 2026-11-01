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

// A video's state and the news of its jobs travel apart, and the news
// comes first. Shown at its news, a search was over with its clips still
// to come and its window drawn back where it was for a frame, and Cancel
// came back after Cancelling for as long, found by the review of #665.
// So from a video's first state on, the news of its jobs is shown once a
// state read after it is, and the end lands with what it left, #513.
describe("the news of a video's job", () => {
  test("is shown at once until a state of the video is in", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1, "search", "s"));
    jobs.apply(job("done", 2, "search", "s"));
    expect(jobs.list.map((j) => j.state)).toEqual(["done"]);
  });

  test("waits for a state read after it, and is shown in the order it was sent", () => {
    const jobs = new JobStore();
    const told: string[] = [];
    jobs.watch((_, after) => told.push(`${after.id} ${after.state}`));
    jobs.shown("/v.mp4", 1);
    jobs.apply(job("running", 1, "search", "s"));
    jobs.apply(job("running", 2, "render", "r"));
    jobs.apply(job("done", 4, "render", "r"));
    jobs.apply(job("done", 3, "search", "s"));
    expect(told).toEqual(["s running", "r running"]);
    // Read before the render's end.
    jobs.shown("/v.mp4", 4);
    expect(told).toEqual(["s running", "r running", "s done"]);
    jobs.shown("/v.mp4", 5);
    expect(told).toEqual(["s running", "r running", "s done", "r done"]);
    expect(jobs.list.map((j) => [j.id, j.state])).toEqual([
      ["s", "done"],
      ["r", "done"],
    ]);
  });

  test("keeps a stop pressed until the end it waits for is shown", () => {
    const jobs = new JobStore();
    jobs.shown("/v.mp4", 1);
    jobs.apply(job("running", 1, "search", "s"));
    jobs.stop("/v.mp4", "clips");
    jobs.apply(job("stopped", 2, "search", "s"));
    expect([jobs.search("/v.mp4")?.state, jobs.isStopping("/v.mp4", "clips")]).toEqual(["running", true]);
    jobs.shown("/v.mp4", 3);
    expect([jobs.search("/v.mp4")?.state, jobs.isStopping("/v.mp4", "clips")]).toEqual(["stopped", false]);
  });

  test("shows work started at once, carried on too, and what it says after that with its state", () => {
    const jobs = new JobStore();
    jobs.shown("/v.mp4", 1);
    jobs.apply(job("queued", 5, "search", "s"));
    expect(jobs.search("/v.mp4")?.state).toBe("queued");
    jobs.apply(job("stopped", 6, "search", "s"));
    jobs.shown("/v.mp4", 7);
    jobs.apply(job("running", 8, "search", "s"));
    expect(jobs.search("/v.mp4")?.state).toBe("running");
    jobs.apply(job("running", 9, "search", "s"));
    jobs.apply(job("running", 10, "search", "s"));
    expect(jobs.list[0].seq).toBe(8);
    jobs.shown("/v.mp4", 11);
    expect(jobs.list[0].seq).toBe(10);
  });

  test("is shown at once when the video leaves the library, because no state of it comes again", () => {
    const jobs = new JobStore();
    jobs.shown("/v.mp4", 1);
    jobs.apply(job("running", 1, "search", "s"));
    jobs.apply(job("cancelled", 2, "search", "s"));
    jobs.forget("/v.mp4");
    expect(jobs.list[0].state).toBe("cancelled");
    jobs.apply(job("settled", 3, "search", "s"));
    expect(jobs.list[0].state).toBe("settled");
  });

  test("of another video does not wait for this video's state", () => {
    const jobs = new JobStore();
    jobs.shown("/v.mp4", 1);
    jobs.apply({ ...job("running", 1, "search", "o"), episode: "/other.mp4" });
    jobs.apply({ ...job("done", 2, "search", "o"), episode: "/other.mp4" });
    expect(jobs.list[0].state).toBe("done");
  });
});
