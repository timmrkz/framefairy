import { describe, expect, test } from "vitest";
import { onVideo, placeFor, placeOf, playedToEnd, playFrom, type Playhead } from "./playhead";

// A clip of two pieces with a cut between them, at twenty-five frames a
// second, the way the episodes are. Its start falls three quarters of a
// frame into one: the frames start at 1677.60 and 1677.64.
const frame = 0.04;
const pieces = [
  { start: 1677.63, end: 1690 },
  { start: 1695, end: 1712 },
];
const start = pieces[0].start;
const end = pieces[1].end;
const key = "plan/3";

describe("a gesture puts the playhead on the clip or on the video", () => {
  test("picking the clip puts it on the clip, at its start", () => {
    expect(placeOf(pieces, key, start, "clip")).toEqual({ place: "clip", clip: key });
  });

  test("a click on the start edge is on the clip", () => {
    expect(placeOf(pieces, key, start, "clip").place).toBe("clip");
    // And landing on it any other way, since the start edge is the clip's.
    expect(placeOf(pieces, key, start).place).toBe("clip");
  });

  test("a click, a drag or a step inside the clip is on the clip", () => {
    expect(placeOf(pieces, key, 1680).place).toBe("clip");
    expect(placeOf(pieces, key, 1700).place).toBe("clip");
  });

  test("so is one in a cut, which is between the clip's edges", () => {
    expect(placeOf(pieces, key, 1692).place).toBe("clip");
    expect(placeOf(pieces, key, 1690).place).toBe("clip");
  });

  test("anywhere before the clip is on the video", () => {
    expect(placeOf(pieces, key, start - frame).place).toBe("video");
    expect(placeOf(pieces, key, start - 0.001).place).toBe("video");
    expect(placeOf(pieces, key, 0).place).toBe("video");
  });

  test("anywhere after it is on the video", () => {
    expect(placeOf(pieces, key, end + 0.001).place).toBe("video");
    expect(placeOf(pieces, key, end + frame).place).toBe("video");
  });

  test("the end edge is on the clip, at its end", () => {
    expect(placeOf(pieces, key, end, "clip").place).toBe("end");
    expect(placeOf(pieces, key, end).place).toBe("end");
  });

  test("a trim says it is about the clip, wherever the pieces are drawn", () => {
    // The playhead goes with the edge being dragged, and the video preview
    // can still have the pieces from before the last move of the hand.
    expect(placeOf(pieces, key, start - 0.5, "clip").place).toBe("clip");
    expect(placeOf(pieces, key, end + 0.5, "clip").place).toBe("end");
  });

  test("with no clip chosen it is on the video", () => {
    expect(placeOf([], key, 1680)).toEqual(onVideo);
    expect(placeOf(pieces, "", 1680)).toEqual(onVideo);
    expect(placeOf([], "", 1680, "clip")).toEqual(onVideo);
  });

  test("a place is about one clip, and another clip chosen is on the video", () => {
    const p = placeOf(pieces, key, 1680);
    expect(placeFor(p, key)).toBe("clip");
    expect(placeFor(p, "plan/4")).toBe("video");
    expect(placeFor(p, null)).toBe("video");
    expect(placeFor(onVideo, key)).toBe("video");
  });
});

describe("the space bar plays from the place", () => {
  test("on the clip it plays the clip from the playhead", () => {
    expect(playFrom("clip", pieces, start)).toEqual({ clip: true, at: start });
    expect(playFrom("clip", pieces, 1680)).toEqual({ clip: true, at: 1680 });
  });

  test("from inside a cut, from the end of that cut", () => {
    expect(playFrom("clip", pieces, 1692)).toEqual({ clip: true, at: 1695 });
    expect(playFrom("clip", pieces, 1690)).toEqual({ clip: true, at: 1695 });
  });

  test("from the clip's end it starts the clip over", () => {
    expect(playFrom("end", pieces, end)).toEqual({ clip: true, at: start });
    // The video's answer at the end can be a frame before it, which is
    // still the end.
    expect(playFrom("end", pieces, end - frame)).toEqual({ clip: true, at: start });
    expect(playFrom("clip", pieces, end)).toEqual({ clip: true, at: start });
  });

  test("on the video it plays the episode straight on, from anywhere", () => {
    expect(playFrom("video", pieces, start - 10)).toEqual({ clip: false, at: start - 10 });
    // Through the clip and its cuts too.
    expect(playFrom("video", pieces, 1680)).toEqual({ clip: false, at: 1680 });
    expect(playFrom("video", pieces, 1692)).toEqual({ clip: false, at: 1692 });
    expect(playFrom("video", pieces, end + 10)).toEqual({ clip: false, at: end + 10 });
  });

  test("with no pieces there is no clip to play", () => {
    expect(playFrom("clip", [], 1680)).toEqual({ clip: false, at: 1680 });
  });
});

// The bug that made this a state. Picking the clip puts the playhead on
// 1677.63, and the paused video on the Mac answers with where the frame it
// shows begins, 1677.60, which then stood in for the playhead. Measured
// against the clip, that is three quarters of a frame before it, and the
// half frame of room the old rule gave said the episode: the space bar
// played it straight through the cut and past the clip's end.
describe("the video's answer never moves the state", () => {
  const old = (time: number) => time >= start - frame / 2 && time < end - 0.05;
  const answer = 1677.6;

  test("the old distance read the answer as before the clip", () => {
    expect(old(answer)).toBe(false);
  });

  test("the place was set by picking, and the answer plays the clip from its start", () => {
    const p = placeOf(pieces, key, start, "clip");
    expect(playFrom(placeFor(p, key), pieces, answer)).toEqual({ clip: true, at: start });
  });

  test("and an answer a frame late plays the clip from there", () => {
    const p = placeOf(pieces, key, start, "clip");
    expect(playFrom(placeFor(p, key), pieces, start + frame)).toEqual({ clip: true, at: start + frame });
  });

  test("on the video an answer inside the clip is still the video", () => {
    const p = placeOf(pieces, key, start - 1);
    expect(playFrom(placeFor(p, key), pieces, start + frame).clip).toBe(false);
  });
});

describe("a play of the clip that reaches its end", () => {
  const p: Playhead = { place: "clip", clip: key };

  test("stops at its end, on the clip", () => {
    expect(playedToEnd(p, false)).toEqual({ place: "end", clip: key });
  });

  test("so the next press starts it over", () => {
    const after = playedToEnd(p, false);
    // The video stops a little past the end, and on the Mac answers with
    // the start of that last frame.
    expect(playFrom(after.place, pieces, end - 0.02)).toEqual({ clip: true, at: start });
  });

  test("with loop on goes back to its start and stays on the clip", () => {
    expect(playedToEnd(p, true)).toEqual({ place: "clip", clip: key });
  });

  test("loop is about the clip, and the video plays on", () => {
    // playFrom asks nothing about loop, on purpose: on the video the
    // clip's rules are not in play, and loop is one of them.
    expect(playFrom("video", pieces, start - 2)).toEqual({ clip: false, at: start - 2 });
  });
});
