import { describe, expect, test } from "vitest";

import type { ClipEntry, SourceView } from "./api";
import { partOf, pictureOf, rotated } from "./rotate";

// A portrait video, the way a phone records one standing up.
const phone = { width: 1080, height: 1920, cropWidth: 1080, cropHeight: 1920, turnedCropWidth: 608 } as SourceView;

const clip = {
  key: "c",
  segments: [
    { start: 10, end: 11.1, cropX: null, moved: false },
    { start: 11.1, end: 12, cropX: null, moved: false },
    { start: 13, end: 14, cropX: null, moved: false },
  ],
  cropLefts: [0, 0, 0],
} as unknown as ClipEntry;

describe("rotating a part of a clip", () => {
  test("turns the pieces between two cuts together, and no other", () => {
    expect(partOf(clip.segments, 0)).toEqual([0, 1]);
    expect(partOf(clip.segments, 2)).toEqual([2, 2]);
    const once = rotated(clip, phone, 11.5);
    expect(once.segments.map((s) => s.rotate ?? 0)).toEqual([90, 90, 0]);
  });

  test("puts the crop in the middle of the picture rotated", () => {
    const once = rotated(clip, phone, 10.5);
    // 1920 wide rotated, a crop of 608 in the middle.
    expect(once.cropLefts).toEqual([656, 656, 0]);
    expect(pictureOf(phone, 90)).toEqual({ width: 1920, height: 1080, cropWidth: 608 });
  });

  test("comes back after four quarter turns", () => {
    let c = clip;
    for (let i = 0; i < 4; i++) c = rotated(c, phone, 13.5);
    expect(c.segments[2].rotate).toBe(0);
    expect(pictureOf(phone, 180)).toEqual({ width: 1080, height: 1920, cropWidth: 1080 });
  });
});
