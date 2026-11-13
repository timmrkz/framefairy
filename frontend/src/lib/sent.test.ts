import { describe, expect, test } from "vitest";

import type { ClipEntry, EpisodeStatus, VideoState } from "./api";
import { SentClips } from "./sent";

// A state sent leaves out the clips the window has as they are, see
// lib/sent.ts, and the window makes it whole again.
describe("a state sent made whole", () => {
  const clip = (key: string, hash: string, more: Partial<ClipEntry> = {}) =>
    ({ key, plan: "p.json", cropLefts: [0], start: 10, end: 30, hash, ...more }) as ClipEntry;
  const state = (rev: number, clips: ClipEntry[], order?: [string, string][], video = "/v.mp4"): VideoState => ({
    video,
    rev,
    status: { source: video } as EpisodeStatus,
    clips,
    coverage: { passes: [] },
    jobs: [],
    order: order?.map(([key, hash]) => ({ key, hash })),
  });
  const kept = (rev: number): VideoState => ({ ...state(rev, []), kept: true });

  // The asks of the Go side to send a video whole, each answered when the
  // test says.
  function asks() {
    const out: { video: string; answer: () => void; answered: Promise<void> }[] = [];
    const ask = (video: string) => {
      let answer!: () => void;
      const answered = new Promise<void>((done) => (answer = done));
      out.push({ video, answer, answered });
      return answered;
    };
    return { out, sent: new SentClips(ask) };
  }

  test("keeps a state read by a call as it is", () => {
    const read = state(1, [clip("a", "")]);
    expect(asks().sent.whole(read)).toBe(read);
  });

  test("fills in the clips left out, the very same clips, in order, with no hash", () => {
    const { sent, out } = asks();
    const first = sent.whole(state(1, [clip("a", "1"), clip("b", "2")], [["a", "1"], ["b", "2"]]))!;
    expect(first.clips.map((c) => c.key)).toEqual(["a", "b"]);
    expect(first.clips[0]).not.toHaveProperty("hash");
    expect(first).not.toHaveProperty("order");
    // b trimmed, and c written before a.
    const next = sent.whole(state(2, [clip("c", "3"), clip("b", "4", { end: 25 })], [["c", "3"], ["a", "1"], ["b", "4"]]))!;
    expect(next.clips.map((c) => [c.key, c.end])).toEqual([["c", 30], ["a", 30], ["b", 25]]);
    expect(next.clips[1]).toBe(first.clips[0]);
    // Nothing changed, so the state holds no clips and no order.
    const same = sent.whole(kept(3))!;
    expect(same.clips).toBe(next.clips);
    expect(same).not.toHaveProperty("kept");
    // a removed.
    const last = sent.whole(state(4, [], [["c", "3"], ["b", "4"]]))!;
    expect(last.clips).toEqual([next.clips[0], next.clips[2]]);
    expect(out).toEqual([]);
  });

  test("asks for every clip once, and again when the whole state was lost", async () => {
    const { sent, out } = asks();
    // The window opened again while the Go side went on.
    expect(sent.whole(state(5, [], [["a", "1"]]))).toBeNull();
    expect(sent.whole(kept(6))).toBeNull();
    expect(out.map((a) => a.video)).toEqual(["/v.mp4"]);
    // The Go side answers the ask, and the whole state it sends is lost.
    // The session heard the answer before the test does.
    out[0].answer();
    await out[0].answered;
    expect(sent.whole(state(7, [], [["a", "1"]]))).toBeNull();
    expect(out.map((a) => a.video)).toEqual(["/v.mp4", "/v.mp4"]);
    // The next whole state is taken, and so are those after it.
    expect(sent.whole(state(8, [clip("a", "1")], [["a", "1"]]))?.clips.map((c) => c.key)).toEqual(["a"]);
    expect(sent.whole(kept(9))?.rev).toBe(9);
  });

  test("keeps the clips of each video apart", () => {
    const { sent } = asks();
    sent.whole(state(1, [clip("a", "1")], [["a", "1"]]));
    expect(sent.whole(state(2, [], [["a", "1"]], "/w.mp4"))).toBeNull();
    expect(sent.whole(state(3, [], [["a", "2"]]))).toBeNull();
  });
});
