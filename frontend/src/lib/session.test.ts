import { describe, expect, test, vi } from "vitest";

import type { Job } from "./api";
import { said } from "./said";
import { EpisodeSession, type WorkMoves } from "./session.svelte";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {} }));
vi.mock("./said", () => ({ said: vi.fn() }));

const none: WorkMoves = {
  failed: () => {},
  wrote: () => {},
  heard: () => {},
  searched: () => {},
  transcribed: () => {},
  made: () => {},
  rendered: () => {},
};

const job = (id: string, kind: string, state: string, seq: number, more: Partial<Job> = {}): Job =>
  ({ id, episode: "/v.mp4", kind, state, seq, ...more }) as Job;

// Every move the session says, in the order it said them, as "move id".
function session(jobs = new JobStore()) {
  const heard: string[] = [];
  const say = (move: string) => (j: Job) => heard.push(`${move} ${j.id}`);
  const moves: WorkMoves = {
    failed: say("failed"),
    wrote: say("wrote"),
    heard: say("heard"),
    searched: say("searched"),
    transcribed: say("transcribed"),
    made: say("made"),
    rendered: say("rendered"),
  };
  return { jobs, heard, session: new EpisodeSession("/v.mp4", jobs, moves) };
}

// When a video's work moves on, the workspace shows it: the clips a search
// wrote land in the list, the window walks on when the search is over, and
// the range picker's edge lands on what was heard, a clip made by hand
// lands where its card was, and a failure says why in a toast. Each was an
// effect in Episode.svelte that kept a value from before, issue #606. Said
// twice, the list is read twice, the window walks on twice and the toast
// comes twice. Not said, the clips found stay out of the list until
// something else reads it, and a failure says nothing.
describe("a video's session", () => {
  test("says a search's clips as they are written, once each, and none after it is over", () => {
    const { jobs, heard } = session();
    jobs.apply(job("s", "search", "running", 1, { step: "finding" }));
    jobs.apply(job("s", "search", "running", 2, { step: "finding", written: 1 }));
    jobs.apply(job("s", "search", "running", 3, { step: "finding", written: 1 }));
    jobs.apply(job("s", "search", "running", 4, { step: "finding", written: 2 }));
    jobs.apply(job("s", "search", "done", 5, { step: "finding", written: 3 }));
    expect(heard).toEqual(["wrote s", "wrote s", "searched s"]);
  });

  test("says the search stopped hearing when it goes on to find, and is over only when it is", () => {
    const { jobs, heard } = session();
    jobs.apply(job("s", "search", "running", 1, { step: "hearing" }));
    jobs.apply(job("s", "search", "running", 2, { step: "hearing" }));
    jobs.apply(job("s", "search", "running", 3, { step: "finding" }));
    expect(heard).toEqual(["heard s"]);
    jobs.apply(job("s", "search", "done", 4, { step: "finding" }));
    expect(heard).toEqual(["heard s", "searched s"]);
  });

  test("says a search stopped while it heard has stopped hearing first, then that it is over", () => {
    const { jobs, heard } = session();
    jobs.apply(job("s", "search", "running", 1, { step: "hearing" }));
    jobs.apply(job("s", "search", "interrupted", 2, { step: "hearing" }));
    expect(heard).toEqual(["heard s", "searched s"]);
  });

  test("says a search over however it ended, and that it failed first when it did", () => {
    const { jobs, heard } = session();
    jobs.apply(job("a", "search", "queued", 1, { step: "waiting" }));
    jobs.apply(job("a", "search", "failed", 2, { step: "waiting" }));
    jobs.apply(job("b", "search", "running", 1, { step: "finding" }));
    jobs.apply(job("b", "search", "cancelled", 2, { step: "finding" }));
    expect(heard).toEqual(["failed a", "searched a", "searched b"]);
  });

  test("says a search is over when it ends after the next one began", () => {
    const { jobs, heard } = session();
    jobs.apply(job("a", "search", "running", 1, { step: "finding" }));
    jobs.apply(job("b", "search", "queued", 1, { step: "waiting" }));
    jobs.apply(job("a", "search", "done", 2, { step: "finding" }));
    expect(heard).toEqual(["searched a"]);
    jobs.apply(job("b", "search", "done", 2, { step: "finding" }));
    expect(heard).toEqual(["searched a", "searched b"]);
  });

  test("says nothing for an older snapshot that comes after a newer one", () => {
    const { jobs, heard } = session();
    jobs.apply(job("s", "search", "running", 4, { step: "hearing", written: 0 }));
    jobs.apply(job("s", "search", "done", 5, { step: "finding", written: 2 }));
    jobs.apply(job("s", "search", "running", 3, { step: "hearing", written: 1 }));
    expect(heard).toEqual(["heard s", "searched s"]);
  });

  test("says nothing for a search it only ever heard of as over", () => {
    const { jobs, heard } = session();
    jobs.apply(job("s", "search", "interrupted", 1, { step: "hearing", written: 2 }));
    jobs.apply(job("s", "search", "interrupted", 2, { step: "hearing", written: 2, aside: true }));
    expect(heard).toEqual([]);
  });

  test("says the whole video heard is over, once", () => {
    const { jobs, heard } = session();
    jobs.apply(job("t", "transcribe", "queued", 1));
    jobs.apply(job("t", "transcribe", "running", 2, { step: "hearing" }));
    jobs.apply(job("t", "transcribe", "interrupted", 3, { step: "hearing" }));
    jobs.apply(job("t", "transcribe", "interrupted", 4, { step: "hearing" }));
    expect(heard).toEqual(["transcribed t"]);
  });

  test("says a clip made by hand is none of a search's moves, and made only when it was asked for here", () => {
    const { jobs, heard, session: here } = session();
    jobs.apply(job("c", "clip", "running", 1, { step: "hearing", written: 0 }));
    jobs.apply(job("c", "clip", "done", 2, { step: "finding", written: 1 }));
    jobs.apply(job("d", "clip", "running", 1, { step: "hearing" }));
    here.asked(job("d", "clip", "running", 1));
    jobs.apply(job("d", "clip", "running", 2, { step: "finding" }));
    jobs.apply(job("d", "clip", "done", 3, { step: "finding", result: "e/3" }));
    jobs.apply(job("d", "clip", "done", 4, { step: "finding", result: "e/3" }));
    expect(heard).toEqual(["made d"]);
  });

  test("says a failure once it was seen running, and only once", () => {
    const { jobs, heard } = session();
    jobs.apply(job("a", "clip", "queued", 1));
    jobs.apply(job("a", "clip", "running", 2));
    jobs.apply(job("a", "clip", "failed", 3));
    jobs.apply(job("a", "clip", "failed", 4));
    expect(heard).toEqual(["failed a"]);
  });

  test("says no failure of a job only ever seen failed, as the app opened", () => {
    const { jobs, heard } = session();
    jobs.apply(job("a", "clip", "failed", 1));
    jobs.apply(job("b", "search", "failed", 1));
    expect(heard).toEqual([]);
  });

  test("says the failure of a clip asked for here that failed in the store before its call answered", () => {
    const { jobs, heard, session: here } = session();
    // The events came first, all at once, so the store was failed already.
    jobs.apply(job("a", "clip", "failed", 1));
    here.asked(job("a", "clip", "queued", 0));
    expect(heard).toEqual(["failed a", "made a"]);
  });

  test("says the failure of a clip whose call answers that it failed, the same way as one that fails later", () => {
    const { jobs, heard, session: here } = session();
    here.asked(job("a", "clip", "failed", 1));
    jobs.apply(job("a", "clip", "failed", 1));
    jobs.apply(job("b", "clip", "queued", 1));
    here.asked(job("b", "clip", "queued", 1));
    jobs.apply(job("b", "clip", "failed", 2));
    expect(heard).toEqual(["failed a", "made a", "failed b", "made b"]);
  });

  test("says a failure seen running and said before the call answered not again", () => {
    const { jobs, heard, session: here } = session();
    jobs.apply(job("a", "clip", "running", 1));
    jobs.apply(job("a", "clip", "failed", 2));
    here.asked(job("a", "clip", "queued", 0));
    expect(heard).toEqual(["failed a", "made a"]);
  });

  test("says no failure of a job that ended any other way and was read again as failed", () => {
    const { jobs, heard } = session();
    for (const state of ["done", "cancelled", "interrupted"]) {
      jobs.apply(job(state, "clip", "running", 1));
      jobs.apply(job(state, "clip", state, 2));
      jobs.apply(job(state, "clip", "failed", 3));
    }
    expect(heard).toEqual([]);
  });

  test("says no failure of work that is not a search or a clip made by hand", () => {
    const { jobs, heard } = session();
    jobs.apply(job("t", "transcribe", "running", 1));
    jobs.apply(job("t", "transcribe", "failed", 2));
    jobs.apply(job("r", "render", "running", 1));
    jobs.apply(job("r", "render", "failed", 2));
    expect(heard).toEqual(["transcribed t"]);
  });

  test("says only the renders asked for in this workspace, once each", () => {
    const { jobs, heard, session: here } = session();
    jobs.apply(job("mine", "render", "queued", 1));
    jobs.apply(job("other", "render", "queued", 1));
    here.asked(job("mine", "render", "queued", 1));
    jobs.apply(job("mine", "render", "running", 2));
    jobs.apply(job("other", "render", "done", 2));
    jobs.apply(job("mine", "render", "failed", 3, { error: "the disk is full" }));
    jobs.apply(job("mine", "render", "failed", 4, { error: "the disk is full" }));
    expect(heard).toEqual(["rendered mine"]);
  });

  test("says a render that was over before its answer came, at once, with what the store knows of it", () => {
    const jobs = new JobStore();
    const got: Job[] = [];
    const here = new EpisodeSession("/v.mp4", jobs, { ...none, rendered: (j) => got.push(j) });
    jobs.apply(job("r", "render", "running", 1));
    jobs.apply(job("r", "render", "done", 2, { result: "short.mp4" }));
    here.asked(job("r", "render", "queued", 0));
    expect(got.map((j) => [j.state, j.result])).toEqual([["done", "short.mp4"]]);
    jobs.apply(job("r", "render", "done", 3, { result: "short.mp4" }));
    expect(got).toHaveLength(1);
  });

  test("says nothing of another video's work", () => {
    const { jobs, heard, session: here } = session();
    here.asked(job("r", "render", "queued", 1, { episode: "/other.mp4" }));
    for (const kind of ["search", "transcribe", "render", "clip"]) {
      jobs.apply(job(kind, kind, "running", 1, { episode: "/other.mp4", step: "hearing" }));
      jobs.apply(job(kind, kind, "failed", 2, { episode: "/other.mp4", step: "finding", written: 1 }));
    }
    jobs.apply(job("r", "render", "done", 3, { episode: "/other.mp4" }));
    expect(heard).toEqual([]);
  });

  test("says nothing once the workspace is gone, and the other workspaces still hear", () => {
    const jobs = new JobStore();
    const gone = session(jobs);
    const stays = session(jobs);
    jobs.apply(job("s", "search", "running", 1, { step: "finding" }));
    gone.session.close();
    jobs.apply(job("s", "search", "done", 2, { step: "finding" }));
    expect(gone.heard).toEqual([]);
    expect(stays.heard).toEqual(["searched s"]);
  });

  // The store is what every part of the app hears its jobs from, so one
  // workspace that goes wrong must not leave the others behind.
  test("a workspace whose move goes wrong stops neither the other workspaces nor the next news, and is in the log", () => {
    const jobs = new JobStore();
    new EpisodeSession("/v.mp4", jobs, {
      ...none,
      searched: () => {
        throw new Error("the list is gone");
      },
    });
    const stays = session(jobs);
    jobs.apply(job("s", "search", "running", 1, { step: "finding" }));
    jobs.apply(job("s", "search", "done", 2, { step: "finding" }));
    jobs.apply(job("t", "transcribe", "running", 1));
    jobs.apply(job("t", "transcribe", "done", 2));
    expect(stays.heard).toEqual(["searched s", "transcribed t"]);
    expect(jobs.list.map((j) => [j.id, j.state])).toEqual([
      ["s", "done"],
      ["t", "done"],
    ]);
    expect(said).toHaveBeenCalledWith(expect.stringContaining("the list is gone"), { level: "error", video: "/v.mp4" });
  });
});
