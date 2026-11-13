import { describe, expect, test, vi } from "vitest";

import type { Job } from "./api";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {}, onVideoState: () => {} }));

const job = (state: string, seq: number, kind = "transcribe", id = "job-5"): Job =>
  ({ id, episode: "/v.mp4", kind, state, seq }) as Job;

const asked = () => Promise.resolve(null);

// A call the test answers when it likes, the way a busy machine answers
// late.
function late<T>() {
  let answer!: (v: T) => void;
  const promise = new Promise<T>((done) => (answer = done));
  return { promise, answer };
}

// A Stop or Cancel stops what shows the work in the frame it is pressed,
// so the store keeps the press until the Go side says the work is
// stopping or has ended. Found by the second review of #458, which saw it
// covered only by a walk, and kept once for every kind of work since the
// third.
describe("a stop pressed for a video's work", () => {
  test("is stopping until the transcription ends, and not after", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1));
    jobs.stopJob("/v.mp4", "whole", asked);
    jobs.stopJob("/v.mp4", "whole", asked);
    expect(jobs.stopping).toEqual([{ path: "/v.mp4", work: "whole", what: "" }]);
    jobs.apply(job("running", 2));
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    expect(jobs.stops(job("running", 2))).toBe(true);
    jobs.apply(job("stopped", 3));
    expect(jobs.stopping).toEqual([]);
  });

  test("gives way to the Go side once it says the work is stopping", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1));
    jobs.stopJob("/v.mp4", "whole", asked);
    jobs.apply({ ...job("running", 2), stopping: true });
    expect(jobs.stopping).toEqual([]);
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    jobs.apply(job("stopped", 3));
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(false);
  });

  test("is still stopping when an older news of its end comes after a newer one of it running", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 4));
    jobs.stopJob("/v.mp4", "whole", asked);
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
    jobs.stopJob("/v.mp4", "clips", asked);
    expect(jobs.stops(job("running", 2, "search", "s"))).toBe(true);
    expect(jobs.stops(job("running", 2, "transcribe", "t"))).toBe(false);
    jobs.apply(job("stopped", 2, "search", "s"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(true);
    jobs.apply(job("stopped", 2, "clip", "c"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(false);
  });

  test("pressed before the Go side answered Search, lasts until the answer and its work have ended", async () => {
    const jobs = new JobStore();
    const search = late<Job>();
    const started = jobs.startJob("/v.mp4", "clips", () => search.promise, "search");
    expect(jobs.isStarting("/v.mp4", "clips", "search")).toBe(true);
    jobs.stopJob("/v.mp4", "clips", asked);
    jobs.apply(job("running", 1, "search", "s"));
    search.answer(job("running", 1, "search", "s"));
    await started;
    expect(jobs.isStarting("/v.mp4", "clips")).toBe(false);
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(true);
    jobs.apply(job("stopped", 2, "search", "s"));
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(false);
  });

  test("is over when the work is started again", () => {
    const jobs = new JobStore();
    jobs.apply(job("running", 1));
    jobs.stopJob("/v.mp4", "whole", asked);
    void jobs.startJob("/v.mp4", "whole", () => late<Job>().promise);
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(false);
    expect(jobs.isStarting("/v.mp4", "whole")).toBe(true);
  });
});

// A start is shown from the press until the Go side answers, and what
// tells two presses of one kind of work apart keeps them apart: I and O,
// or the clip a render is of.
describe("a start pressed for a video's work", () => {
  test("lasts until the answer, which the list then holds, and a refusal ends it too", async () => {
    const jobs = new JobStore();
    const render = late<Job>();
    const started = jobs.startJob("/v.mp4", "render", () => render.promise, "clips.json/03");
    expect(jobs.isStarting("/v.mp4", "render", "clips.json/03")).toBe(true);
    expect(jobs.isStarting("/v.mp4", "render", "clips.json/04")).toBe(false);
    render.answer(job("running", 1, "render", "r"));
    await started;
    expect(jobs.isStarting("/v.mp4", "render")).toBe(false);
    expect(jobs.list.map((j) => j.id)).toEqual(["r"]);
    await expect(jobs.startJob("/v.mp4", "clips", () => Promise.reject(new Error("no")), "in")).rejects.toThrow("no");
    expect(jobs.isStarting("/v.mp4", "clips")).toBe(false);
  });
});

// A search or a clip made by hand stops the video's transcription on the
// Go side, which says the transcription is stopping from that moment and
// until it ends, once the speech model has heard its piece. The menu said
// Stop transcribing for three and a half seconds after Search, Tim's
// report of 26 October, 14.22, and the interface once guessed it on the
// press, a rule of its own.
describe("a transcription the Go side says is stopping", () => {
  test("is stopping until it ends", () => {
    const jobs = new JobStore();
    jobs.apply({ ...job("running", 1, "transcribe", "t"), stopping: true });
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(true);
    expect(jobs.stops(jobs.list[0])).toBe(true);
    expect(jobs.isStopping("/v.mp4", "clips")).toBe(false);
    jobs.apply(job("stopped", 2, "transcribe", "t"));
    expect(jobs.isStopping("/v.mp4", "whole")).toBe(false);
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

// Every state of a video reaches the store twice, from its own listener
// and passed on by the video's session, issue #805. The second is the
// same state and changes nothing.
describe("a state of a video heard twice", () => {
  test("is taken once", () => {
    const jobs = new JobStore();
    const state = { video: "/v.mp4", rev: 4, jobs: [job("running", 3)] } as never;
    jobs.take(state);
    const list = jobs.list;
    jobs.take(state);
    expect(jobs.list).toBe(list);
    // A newer state of the same jobs is still taken.
    jobs.take({ video: "/v.mp4", rev: 5, jobs: [job("done", 5)] } as never);
    expect(jobs.list.map((j) => j.state)).toEqual(["done"]);
  });
});
