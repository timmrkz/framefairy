import { describe, expect, test, vi } from "vitest";

import type { ClipEntry, EpisodeStatus, Job, VideoState, WindowView } from "./api";
import { EpisodeSession } from "./session.svelte";
import { JobStore } from "./state.svelte";

vi.mock("./api", () => ({ api: {}, onJob: () => {}, onVideoState: () => {} }));
vi.mock("./said", () => ({ said: vi.fn() }));

const none = () => {};

const job = (id: string, kind: string, state: string, seq: number, more: Partial<Job> = {}): Job =>
  ({ id, episode: "/v.mp4", kind, state, seq, ...more }) as Job;

// Every clip the session says the app chose, in the order it said them.
function session(jobs = new JobStore()) {
  const heard: string[] = [];
  const here = new EpisodeSession("/v.mp4", jobs);
  const close = here.open((key: string, before: string, card: string) => heard.push(`chose ${key} ${before || "none"}${card ? ` from ${card}` : ""}`));
  return { jobs, heard, session: here, close };
}

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
    const { session: here } = session(jobs);
    jobs.apply(job("s", "search", "running", 4, { step: "finding", written: 2 }));
    here.take(state(5, { jobs: [job("s", "search", "running", 3, { step: "finding", written: 1 })] }));
    expect(jobs.list[0].written).toBe(2);
    expect(here.wrote("s")).toBe(1);
    here.take(state(7, { jobs: [job("s", "search", "done", 6, { step: "finding", written: 3 })] }));
    expect([jobs.list[0].state, here.wrote("s")]).toEqual(["done", 3]);
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
