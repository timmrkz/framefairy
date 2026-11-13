import { describe, expect, test } from "vitest";

import type { ClipEntry, EpisodeStatus } from "./api";
import { SentClips, type SentState } from "./sent";

// A state sent leaves out the clips the window has as they are, see
// lib/sent.ts, and the window makes it whole again.
describe("a state sent made whole", () => {
  const clip = (key: string, hash: string, more: Partial<ClipEntry> = {}) =>
    ({ key, plan: "p.json", cropLefts: [0], start: 10, end: 30, hash, ...more }) as ClipEntry & { hash: string };
  const state = (rev: number, clips: (ClipEntry & { hash?: string })[], order?: [string, string][]): SentState => ({
    video: "/v.mp4",
    rev,
    status: { source: "/v.mp4" } as EpisodeStatus,
    clips,
    coverage: { passes: [] },
    jobs: [],
    order: order?.map(([key, hash]) => ({ key, hash })),
  });

  test("keeps a state read by a call as it is", () => {
    const read = state(1, [clip("a", "")]);
    expect(new SentClips().whole(read)).toBe(read);
  });

  test("fills in the clips left out, the very same clips, in order, with no hash", () => {
    const sent = new SentClips();
    const first = sent.whole(state(1, [clip("a", "1"), clip("b", "2")], [["a", "1"], ["b", "2"]]))!;
    expect(first.clips.map((c) => c.key)).toEqual(["a", "b"]);
    expect(first.clips[0]).not.toHaveProperty("hash");
    expect(first).not.toHaveProperty("order");
    // b trimmed, and c written before a.
    const next = sent.whole(state(2, [clip("c", "3"), clip("b", "4", { end: 25 })], [["c", "3"], ["a", "1"], ["b", "4"]]))!;
    expect(next.clips.map((c) => [c.key, c.end])).toEqual([["c", 30], ["a", 30], ["b", 25]]);
    expect(next.clips[1]).toBe(first.clips[0]);
    // Nothing changed, and a removed.
    const last = sent.whole(state(3, [], [["c", "3"], ["b", "4"]]))!;
    expect(last.clips).toEqual([next.clips[0], next.clips[2]]);
    expect(last.clips[1]).toBe(next.clips[2]);
  });

  test("makes nothing of a state that names a clip not sent here as it is", () => {
    const sent = new SentClips();
    // The window opened again while the Go side went on.
    expect(sent.whole(state(5, [], [["a", "1"]]))).toBeNull();
    sent.whole(state(6, [clip("a", "1")], [["a", "1"]]));
    expect(sent.whole(state(7, [], [["a", "2"]]))).toBeNull();
    // Another video's clips are its own.
    expect(sent.whole({ ...state(8, [], [["a", "1"]]), video: "/w.mp4" })).toBeNull();
  });
});
