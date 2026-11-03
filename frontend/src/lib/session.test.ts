import { describe, expect, test, vi } from "vitest";

import type { ClipEntry, EpisodeStatus, Job, VideoState, WindowView } from "./api";
import { said } from "./said";
import { EpisodeSession, type WorkMoves } from "./session.svelte";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {}, onVideoState: () => {} }));
vi.mock("./said", () => ({ said: vi.fn() }));

const none: WorkMoves = {
  chose: () => {},
  heard: () => {},
  searched: () => {},
  transcribed: () => {},
  rendered: () => {},
};

const job = (id: string, kind: string, state: string, seq: number, more: Partial<Job> = {}): Job =>
  ({ id, episode: "/v.mp4", kind, state, seq, ...more }) as Job;

// Every move the session says, in the order it said them, as "move id".
function session(jobs = new JobStore()) {
  const heard: string[] = [];
  const say = (move: string) => (j: Job) => heard.push(`${move} ${j.id}`);
  const moves: WorkMoves = {
    chose: (key: string, before: string, card: string) => heard.push(`chose ${key} ${before || "none"}${card ? ` from ${card}` : ""}`),
    heard: say("heard"),
    searched: say("searched"),
    transcribed: say("transcribed"),
    rendered: say("rendered"),
  };
  const here = new EpisodeSession("/v.mp4", jobs);
  const close = here.open(moves);
  return { jobs, heard, session: here, close };
}

// When a video's work moves on, the workspace shows it: the window walks
// on when the search is over, the range picker's edge lands on what was
// heard. The clips a search wrote and
// the clip made by hand come in the video's state, with the clip the Go
// side chose. Each was an
// effect in Episode.svelte that kept a value from before, issue #606. Said
// twice, the list is read twice and the window walks on twice. Not said,
// the clips found stay out of the list until something else reads it. A
// failure is told by the Go side, see notice.go.
describe("a video's session", () => {
  test("says nothing of a search's clips as they are written, only that it is over", () => {
    const { jobs, heard } = session();
    jobs.apply(job("s", "search", "running", 1, { step: "finding" }));
    jobs.apply(job("s", "search", "running", 2, { step: "finding", written: 1 }));
    jobs.apply(job("s", "search", "running", 3, { step: "finding", written: 1 }));
    jobs.apply(job("s", "search", "running", 4, { step: "finding", written: 2 }));
    jobs.apply(job("s", "search", "done", 5, { step: "finding", written: 3 }));
    expect(heard).toEqual(["searched s"]);
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
    jobs.apply(job("s", "search", "stopped", 2, { step: "stopped" }));
    expect(heard).toEqual(["heard s", "searched s"]);
  });

  test("says a search over however it ended", () => {
    const { jobs, heard } = session();
    jobs.apply(job("a", "search", "queued", 1, { step: "waiting" }));
    jobs.apply(job("a", "search", "failed", 2, { step: "waiting" }));
    jobs.apply(job("b", "search", "running", 1, { step: "finding" }));
    jobs.apply(job("b", "search", "cancelled", 2, { step: "finding" }));
    expect(heard).toEqual(["searched a", "searched b"]);
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

  test("says a clip made by hand is none of a search's moves", () => {
    const { jobs, heard, session: here } = session();
    jobs.apply(job("c", "clip", "running", 1, { step: "hearing", written: 0 }));
    jobs.apply(job("c", "clip", "done", 2, { step: "finding", written: 1 }));
    jobs.apply(job("d", "clip", "running", 1, { step: "hearing" }));
    here.asked(job("d", "clip", "running", 1));
    jobs.apply(job("d", "clip", "running", 2, { step: "finding" }));
    jobs.apply(job("d", "clip", "done", 3, { step: "finding", result: "e/3" }));
    jobs.apply(job("d", "clip", "done", 4, { step: "finding", result: "e/3" }));
    expect(heard).toEqual([]);
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
    const here = new EpisodeSession("/v.mp4", jobs);
    here.open({ ...none, rendered: (j) => got.push(j) });
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
    gone.close();
    jobs.apply(job("s", "search", "done", 2, { step: "finding" }));
    expect(gone.heard).toEqual([]);
    expect(stays.heard).toEqual(["searched s"]);
  });

  // The store is what every part of the app hears its jobs from, so one
  // workspace that goes wrong must not leave the others behind.
  test("a workspace whose move goes wrong stops neither the other workspaces nor the next news, and is in the log", () => {
    const jobs = new JobStore();
    new EpisodeSession("/v.mp4", jobs).open({
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

// The workspace shows the video's state as the Go side sends it, issue
// #606. A state read before another, or before an edit was written, comes
// last now and then on a busy machine, and taken it put back a clip just
// removed or a trim just made until the next read, the family of #323 and
// #572.
describe("a video's state in its session", () => {
  const clip = (key: string, more: Partial<ClipEntry> = {}): ClipEntry =>
    ({ key, plan: "/v.mp4.framefairy/logs/clips.json", cropLefts: [0], start: 10, end: 30, ...more }) as ClipEntry;
  const state = (rev: number, more: Partial<VideoState> = {}): VideoState => ({
    video: "/v.mp4",
    rev,
    status: { source: "/v.mp4", measured: rev } as EpisodeStatus,
    clips: [clip("a")],
    coverage: { passes: [] },
    jobs: [],
    ...more,
  });

  test("keeps the newest state, whichever order they come in", () => {
    const here = new EpisodeSession("/v.mp4", new JobStore());
    expect(here.take(state(5))).toBe(true);
    expect(here.take(state(3))).toBe(false);
    expect(here.status?.measured).toBe(5);
    expect(here.take(state(6))).toBe(true);
    expect([here.rev, here.status?.measured]).toEqual([6, 6]);
  });

  test("a read asked to be fresh makes the clips new, though a newer state came first", () => {
    const here = new EpisodeSession("/v.mp4", new JobStore());
    here.take(state(5));
    const was = here.clips[0];
    // The state the read asked for after a caption style edit comes after
    // one the Go side sent.
    expect(here.take(state(6))).toBe(true);
    expect(here.clips[0]).toBe(was);
    expect(here.take(state(4), true)).toBe(false);
    expect(here.clips[0]).not.toBe(was);
    expect(here.clips[0]).toEqual(was);
  });

  test("takes no state read before an edit's answer, and the first read after it", () => {
    const here = new EpisodeSession("/v.mp4", new JobStore());
    here.take(state(5));
    here.put(clip("a", { rejected: true, rev: 8 }));
    // Read before the clip was removed, so it holds it as it was.
    expect(here.take(state(7))).toBe(false);
    expect(here.clips[0].rejected).toBe(true);
    expect(here.take(state(9, { clips: [clip("a", { rejected: true })] }))).toBe(true);
    expect(here.clips[0].rejected).toBe(true);
  });

  test("shows an edit at once, holds what comes while it is written, and takes the newest after", async () => {
    const here = new EpisodeSession("/v.mp4", new JobStore());
    here.take(state(5));
    let answer!: (c: ClipEntry) => void;
    const edited = here.edit("a", (c) => ({ ...c, thumbnails: [12] }), () => new Promise((done) => (answer = done)));
    expect(here.clips[0].thumbnails).toEqual([12]);
    // A state read before the edit was written comes while it is.
    here.take(state(6));
    expect(here.clips[0].thumbnails).toEqual([12]);
    expect(here.status?.measured).toBe(5);
    answer(clip("a", { thumbnails: [12], rev: 7 }));
    await edited;
    expect(here.clips[0].thumbnails).toEqual([12]);
    // The state held was older than the answer, so it is not taken.
    expect(here.status?.measured).toBe(5);
    expect(here.take(state(8, { clips: [clip("a", { thumbnails: [12] })] }))).toBe(true);
  });

  test("puts the clip back when the edit is refused, and takes the state that came meanwhile", async () => {
    const here = new EpisodeSession("/v.mp4", new JobStore());
    here.take(state(5));
    const edited = here.edit("a", (c) => ({ ...c, thumbnails: [12] }), async () => {
      here.take(state(6));
      throw new Error("the plan is gone");
    });
    await expect(edited).rejects.toThrow("the plan is gone");
    expect(here.clips[0].thumbnails).toBeUndefined();
    expect(here.status?.measured).toBe(6);
  });

  test("keeps a clip that did not change the very same clip, unless it is read for an edit", () => {
    const here = new EpisodeSession("/v.mp4", new JobStore());
    here.take(state(5));
    const was = here.clips[0];
    here.take(state(6, { clips: [clip("a"), clip("b", { start: 40 })] }));
    expect(here.clips[0]).toBe(was);
    here.take(state(7, { clips: [clip("a"), clip("b", { start: 40 })] }), true);
    expect(here.clips[0]).not.toBe(was);
  });

  test("hands its jobs to the store unless the store has newer news of them, and counts what each wrote", () => {
    const jobs = new JobStore();
    const { heard, session: here } = session(jobs);
    jobs.apply(job("s", "search", "running", 4, { step: "finding", written: 2 }));
    here.take(state(5, { jobs: [job("s", "search", "running", 3, { step: "finding", written: 1 })] }));
    expect(jobs.list[0].written).toBe(2);
    expect(here.wrote("s")).toBe(1);
    here.take(state(7, { jobs: [job("s", "search", "done", 6, { step: "finding", written: 3 })] }));
    expect([jobs.list[0].state, here.wrote("s")]).toEqual(["done", 3]);
    expect(heard).toEqual(["searched s"]);
  });

  test("shows the clip a state says is chosen, and says the app chose another", () => {
    const { heard, session: here } = session();
    here.take(state(5, { chosen: "p/a" }));
    here.take(state(6, { chosen: "p/a" }));
    here.take(state(7, { chosen: "p/b" }));
    expect(here.chosen).toBe("p/b");
    expect(heard).toEqual(["chose p/a none", "chose p/b p/a"]);
  });

  test("keeps a hand's pick on screen until its answer, the state read after it, and says nothing of it", async () => {
    const { heard, session: here } = session();
    here.take(state(5, { chosen: "p/a" }));
    let answer: (s: VideoState) => void = () => {};
    const picked = here.pick("p/b", () => new Promise((r) => (answer = r)));
    expect(here.chosen).toBe("p/b");
    // Read before the Go side had the pick.
    here.take(state(6, { chosen: "p/a" }));
    expect(here.chosen).toBe("p/b");
    answer(state(7, { chosen: "p/b" }));
    await picked;
    expect([here.chosen, here.rev]).toEqual(["p/b", 7]);
    expect(heard).toEqual(["chose p/a none"]);
  });

  test("draws the clip a card picked was written as, when its answer says so", async () => {
    const { heard, session: here } = session();
    here.take(state(5, { chosen: "p/a" }));
    let answer: (s: VideoState) => void = () => {};
    const picked = here.pick("job/1", () => new Promise((r) => (answer = r)));
    answer(state(6, { chosen: "hand/h01", chosenCard: "job/1" }));
    await picked;
    expect(here.chosen).toBe("hand/h01");
    expect(heard).toEqual(["chose p/a none", "chose hand/h01 job/1 from job/1"]);
  });

  test("shows what the app chose while a pick was on its way once the pick is answered, when it chose after it", async () => {
    const { heard, session: here } = session();
    here.take(state(5, { chosen: "p/a" }));
    let answer: (s: VideoState) => void = () => {};
    const picked = here.pick("p/b", () => new Promise((r) => (answer = r)));
    // A search's first clip, chosen after the pick was answered at 6.
    here.take(state(7, { chosen: "p/c" }));
    expect(here.chosen).toBe("p/b");
    answer(state(6, { chosen: "p/b" }));
    await picked;
    expect(here.chosen).toBe("p/c");
    expect(heard).toEqual(["chose p/a none", "chose p/c p/b"]);
  });

  test("draws the window its state carries, and keeps a hand's place on screen until its answer", async () => {
    const { session: here } = session();
    const win = (from: number, to: number) => ({ from, to, length: to - from, most: 1800, least: 60 }) as WindowView;
    here.take(state(5, { window: win(0, 600) }));
    expect([here.window?.from, here.window?.to]).toEqual([0, 600]);
    let answer: (s: VideoState) => void = () => {};
    const placed = here.place({ from: 600, to: 1200, length: 600 }, () => new Promise((r) => (answer = r)));
    expect([here.window?.from, here.window?.to]).toEqual([600, 1200]);
    // Read before the Go side had the window moved.
    here.take(state(6, { window: win(0, 600) }));
    expect([here.window?.from, here.window?.to]).toEqual([600, 1200]);
    // The Go side fitted it as it kept it.
    answer(state(7, { window: win(600, 1100) }));
    await placed;
    expect([here.window?.from, here.window?.to, here.window?.least]).toEqual([600, 1100, 60]);
  });

  test("hears the states of its own video while it is open", () => {
    const jobs = new JobStore();
    const here = new EpisodeSession("/v.mp4", jobs);
    let send: (s: VideoState) => void = () => {};
    const close = here.open(none, (fn) => {
      send = fn;
      return () => (send = () => {});
    });
    send(state(5, { video: "/other.mp4" }));
    expect(here.rev).toBe(0);
    send(state(5));
    expect(here.rev).toBe(5);
    close();
    send(state(6));
    expect(here.rev).toBe(5);
  });
});
