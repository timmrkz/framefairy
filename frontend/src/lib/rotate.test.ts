import { describe, expect, test } from "vitest";

import type { ClipEntry, Rotation, SourceView } from "./api";
import { pictureOf, pieceHolding, rotationAt, shownAt } from "./rotate";

// A portrait video, the way a phone records one standing up.
const phone = { width: 1080, height: 1920, cropWidth: 1080, cropHeight: 1920, turnedCropWidth: 608 } as SourceView;

const clip = {
  key: "c",
  segments: [
    { start: 10, end: 11.1, cropX: null, moved: false, part: 0 },
    { start: 11.1, end: 12, cropX: null, moved: false, part: 0 },
    { start: 13, end: 14, cropX: null, moved: false, part: 1 },
  ],
  rotations: [],
  cropLefts: [0, 0, 0],
} as unknown as ClipEntry;

// How a part turns is the engine's to say, see engine.TurnClip, and the
// state carries it. What stays here is the picture a turn shows.
describe("the picture of a rotated part", () => {
  test("swaps width and height a quarter turn either way, with the crop for it", () => {
    // 1920 wide rotated, a crop of 608 on it.
    expect(pictureOf(phone, 90)).toEqual({ width: 1920, height: 1080, cropWidth: 608 });
    expect(pictureOf(phone, 270)).toEqual({ width: 1920, height: 1080, cropWidth: 608 });
    expect(pictureOf(phone, 180)).toEqual({ width: 1080, height: 1920, cropWidth: 1080 });
  });
});

// The video preview turns the picture by the frame on the canvas, as the
// frame queue reports it, and never by the playhead, which runs ahead of
// its frames. Each report below is one the queue makes while a hand drags
// the playhead over the cut from 12 to 13 s, with the turn the render
// gives that frame: the stretch from the middle of the cut on is rotated.
describe("the turn of the picture", () => {
  const rotations: Rotation[] = [{ from: 12.5, to: null, deg: 90 }];
  const turnOf = (frameStart: number, playhead: number) => rotationAt(rotations, shownAt(frameStart, playhead));

  test("follows the frame on the canvas, not the playhead", () => {
    // Dragged back out of the rotated part: the playhead is in the part
    // before while the last frame of the rotated part is still up.
    expect(turnOf(13.04, 11.9)).toBe(90);
    // Dragged on into the rotated part: a frame of the part before is
    // still up.
    expect(turnOf(11.96, 13.2)).toBe(0);
    // Frames put up on the way, inside the cut: each by its own moment.
    expect(turnOf(12.36, 13.2)).toBe(0);
    expect(turnOf(12.64, 11.9)).toBe(90);
  });

  test("is the playhead's before the first frame", () => {
    expect(turnOf(NaN, 13.2)).toBe(90);
    expect(turnOf(NaN, 11.9)).toBe(0);
  });

  test("goes by the frame's own start at a rate that varies", () => {
    // A phone's frames, a little off any grid, on both sides of the edge
    // of the stretch, and one a hair before it, which the plan keeps to
    // the millisecond.
    const frames = [12.4613, 12.4987, 12.4995, 12.5342, 12.5698];
    expect(frames.map((f) => turnOf(f, 0))).toEqual([0, 0, 90, 90, 90]);
  });

  test("runs to the end of the video where the stretch has no end", () => {
    expect(turnOf(5000, 0)).toBe(90);
    expect(rotationAt([{ from: null, to: 3, deg: 270 }], 0)).toBe(270);
    expect(rotationAt(undefined, 3)).toBe(0);
  });

  test("puts a frame of a cut in no piece", () => {
    expect(pieceHolding(clip.segments, 12.5)).toBe(-1);
    expect(pieceHolding(clip.segments, 12.9995)).toBe(2);
    expect(pieceHolding(clip.segments, 11.0995)).toBe(1);
  });
});
