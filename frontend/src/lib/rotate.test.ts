import { describe, expect, test } from "vitest";

import type { ClipEntry, SourceView } from "./api";
import { partOf, pictureOf, pieceHolding, rotated, shownMoment } from "./rotate";

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

describe("the moment the picture shows", () => {
  const pieces = [
    { start: 0, end: 2 },
    { start: 3, end: 5, rotate: 90 },
  ];
  test("is the playhead while its frame is on the canvas", () => {
    expect(shownMoment(3.01, 3, 0.04)).toBe(3.01);
    expect(shownMoment(3.01, NaN, 0.04)).toBe(3.01);
  });
  test("is the frame on the canvas while the playhead's frame is on its way", () => {
    // Scrubbed back from the rotated part into the cut: the canvas still
    // has the rotated part's first frame, so the picture stays rotated.
    const at = shownMoment(2.5, 3, 0.04);
    expect(pieceHolding(pieces, at)).toBe(1);
    // Scrubbed on into the part: the frame of the cut is still there, so
    // the picture is not yet rotated.
    expect(pieceHolding(pieces, shownMoment(3.2, 2.96, 0.04))).toBe(-1);
  });
});
