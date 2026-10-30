import { describe, expect, test, vi } from "vitest";

import type { Job } from "./api";
import { EpisodeSession, type WorkMoves } from "./session.svelte";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {} }));

const job = (id: string, kind: string, state: string, seq: number, more: Partial<Job> = {}): Job =>
  ({ id, episode: "/v.mp4", kind, state, seq, ...more }) as Job;

// Every move the session says, in the order it said them, as "move id".
function session(jobs = new JobStore()) {
  const heard: string[] = [];
  const say = (move: string) => (j: Job) => heard.push(`${move} ${j.id}`);
  const moves: WorkMoves = {
    wrote: say("wrote"),
    transcribed: say("transcribed"),
    heard: say("heard"),
    searched: say("searched"),
    rendered: say("rendered"),
  };
  return { jobs, heard, session: new EpisodeSession("/v.mp4", jobs, moves) };
}

// When a video's work moves on, the workspace shows it: the clips a search
// wrote land in the list, the window walks on when the search is over, and
// the range picker's edge lands on what was heard. Each was an effect in
// Episode.svelte that kept a value from before, issue #606. Said twice, the
// list is read twice and the window walks on twice. Not said, the clips
// found stay out of the list until something else reads it.
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

  test("says a search over however it ended, a failure and a cancel alike", () => {
    const { jobs, heard } = session();
    jobs.apply(job("a", "search", "queued", 1, { step: "waiting" }));
    jobs.apply(job("a", "search", "failed", 2, { step: "waiting" }));
    jobs.apply(job("b", "search", "running", 1, { step: "finding" }));
    jobs.apply(job("b", "search", "cancelled", 2, { step: "finding" }));
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

  test("says a clip made by hand is none of these", () => {
    const { jobs, heard } = session();
    jobs.apply(job("c", "clip", "running", 1, { step: "hearing", written: 0 }));
    jobs.apply(job("c", "clip", "done", 2, { step: "finding", written: 1 }));
    expect(heard).toEqual([]);
  });

  test("says only the renders asked for in this workspace, once each", () => {
    const { jobs, heard, session: here } = session();
    jobs.apply(job("mine", "render", "queued", 1));
    jobs.apply(job("other", "render", "queued", 1));
    here.rendering(job("mine", "render", "queued", 1));
    jobs.apply(job("mine", "render", "running", 2));
    jobs.apply(job("other", "render", "done", 2));
    jobs.apply(job("mine", "render", "failed", 3, { error: "the disk is full" }));
    jobs.apply(job("mine", "render", "failed", 4, { error: "the disk is full" }));
    expect(heard).toEqual(["rendered mine"]);
  });

  test("says a render that was over before its answer came, at once, with what the store knows of it", () => {
    const jobs = new JobStore();
    const got: Job[] = [];
    const here = new EpisodeSession("/v.mp4", jobs, {
      wrote: () => {},
      transcribed: () => {},
      heard: () => {},
      searched: () => {},
      rendered: (j) => got.push(j),
    });
    jobs.apply(job("r", "render", "running", 1));
    jobs.apply(job("r", "render", "done", 2, { result: "short.mp4" }));
    here.rendering(job("r", "render", "queued", 0));
    expect(got.map((j) => [j.state, j.result])).toEqual([["done", "short.mp4"]]);
    jobs.apply(job("r", "render", "done", 3, { result: "short.mp4" }));
    expect(got).toHaveLength(1);
  });

  test("says nothing of another video's work", () => {
    const { jobs, heard, session: here } = session();
    here.rendering(job("r", "render", "queued", 1, { episode: "/other.mp4" }));
    for (const kind of ["search", "transcribe", "render"]) {
      jobs.apply(job(kind, kind, "running", 1, { episode: "/other.mp4", step: "hearing" }));
      jobs.apply(job(kind, kind, "done", 2, { episode: "/other.mp4", step: "finding", written: 1 }));
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
});
