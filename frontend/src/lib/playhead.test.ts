import { describe, expect, test } from "vitest";
import { arrowStep, inEpisode, onVideo, placeFor, placeOf, playedToEnd, playFrom, playheadClock, SKIP, type ArrowKey, type Playhead } from "./playhead";

// A clip of two pieces with a cut between them, at twenty-five frames a
// second, the way the episodes are. Its start falls three quarters of a
// frame into one: the frames start at 1677.60 and 1677.64.
const fps = 25;
const frame = 1 / fps;
const pieces = [
  { start: 1677.63, end: 1690 },
  { start: 1695, end: 1712 },
];
const start = pieces[0].start;
const end = pieces[1].end;
const key = "plan/3";

// Where the engine says the playhead is on the clip, see
// SourceInfo.Places: on it from a hair before its first frame, at its end
// from a hair before its last, and off it again from a hair before the
// frame after. Which frames those are is the engine's, tested there.
const clip = { segments: pieces, places: [1677.599, 1711.959, 1711.999] as [number, number, number] };

describe("a gesture puts the playhead on the clip or on the video", () => {
  test("picking the clip puts it on the clip, at its start", () => {
    expect(placeOf(clip, key, start, "clip")).toEqual({ place: "clip", clip: key });
  });

  test("a click, a drag or a step inside the clip is on the clip, a cut too", () => {
    for (const at of [1677.6, start, 1680, 1690, 1692, 1700]) expect(placeOf(clip, key, at).place).toBe("clip");
  });

  test("before the frame the clip opens with is on the video", () => {
    for (const at of [0, 1677.56, 1677.598]) expect(placeOf(clip, key, at).place).toBe("video");
  });

  test("the clip's last frame and its end are the end, and after it the video", () => {
    for (const at of [1711.96, end - 0.01, end]) expect(placeOf(clip, key, at).place).toBe("end");
    expect(placeOf(clip, key, end, "clip").place).toBe("end");
    expect(placeOf(clip, key, end + 0.001).place).toBe("video");
    expect(placeOf(clip, key, end + frame).place).toBe("video");
  });

  test("a trim of the end on frames leaves the end, so the space bar starts over", () => {
    // The playhead goes with the edge, a frame before the end, see
    // docs/APP.md. That is the clip's last frame, so it is the end, and
    // the next press of the space bar plays the clip from its start.
    const p = placeOf(clip, key, end - frame, "clip");
    expect(p.place).toBe("end");
    expect(playFrom(p.place, pieces, end - frame)).toEqual({ clip: true, at: start });
  });

  test("a trim says it is about the clip, wherever the pieces are drawn", () => {
    // The playhead goes with the edge being dragged, and the video preview
    // can still have the pieces from before the last move of the hand.
    expect(placeOf(clip, key, start - 0.5, "clip").place).toBe("clip");
    expect(placeOf(clip, key, end + 0.5, "clip").place).toBe("end");
  });

  test("a step while the episode plays on the video stays on the video", () => {
    // Skipping ahead through a clip the play started outside of does not
    // light the clip up, not inside it, not at its end, not in a cut.
    for (const at of [start, 1680, end]) expect(placeOf(clip, key, at, "video")).toEqual(onVideo);
  });

  test("with no clip chosen, or none the engine placed, it is on the video", () => {
    expect(placeOf(null, key, 1680)).toEqual(onVideo);
    expect(placeOf(clip, "", 1680)).toEqual(onVideo);
    expect(placeOf({ segments: pieces }, key, 1680, "clip")).toEqual(onVideo);
  });

  test("a place is about one clip, and another clip chosen is on the video", () => {
    const p = placeOf(clip, key, 1680);
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
    const p = placeOf(clip, key, start, "clip");
    expect(playFrom(placeFor(p, key), pieces, answer)).toEqual({ clip: true, at: start });
  });

  test("and an answer a frame late plays the clip from there", () => {
    const p = placeOf(clip, key, start, "clip");
    expect(playFrom(placeFor(p, key), pieces, start + frame)).toEqual({ clip: true, at: start + frame });
  });

  test("on the video an answer inside the clip is still the video", () => {
    const p = placeOf(clip, key, start - 1);
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

describe("the playhead stays inside the episode", () => {
  test("a moment past the end the file's tables give is the end", () => {
    expect(inEpisode(349.524, 349.465)).toBe(349.465);
  });
  test("a moment inside is kept, and none is before the start", () => {
    expect(inEpisode(12.5, 349.465)).toBe(12.5);
    expect(inEpisode(-0.01, 349.465)).toBe(0);
  });
  test("a duration not yet known keeps the moment", () => {
    expect(inEpisode(3, 0)).toBe(3);
  });
});

describe("the time field says where the playhead's frame begins", () => {
  test("to the millisecond", () => {
    expect(playheadClock(1677.6, true)).toBe("0:27:57.600");
    expect(playheadClock(0.25 + (10 * 1001) / 30000, false)).toBe("00:00.584");
  });

  test("as wide at the start of a video as at its end", () => {
    expect(playheadClock(0, true)).toBe("0:00:00.000");
    expect(playheadClock(3 * 3600 + 59.96, true)).toBe("3:00:59.960");
    expect(playheadClock(0, false)).toBe("00:00.000");
    expect(playheadClock(59 * 60 + 59.96, false)).toBe("59:59.960");
  });

  test("a millisecond that rounds up carries into the second", () => {
    // 1/30 s frames: frame 1799 starts at 59.9667, frame 1800 at 60.
    expect(playheadClock(1799 / 30, false)).toBe("00:59.967");
    expect(playheadClock(59.9999, false)).toBe("01:00.000");
  });
});

describe("an arrow key on the clip timeline", () => {
  // A clip of two pieces at 25 frames a second, 10 to 14 and 16 to 20,
  // with a cut from 14 to 16. The cut took "geschnitten" out, from 15 to
  // 15.5, so the caption lights only these.
  const frame = 0.04;
  const clip = { start: 10, end: 20 };
  // Each lands a frame in, as the engine says, see LandOn.
  const lit = [
    { start: 10.5, end: 11, text: "Und", land: 10.54, since: 10.5 },
    { start: 18, end: 18.6, text: "da", land: 18.04, since: 18 },
  ];
  const key = (o: Partial<ArrowKey>): ArrowKey => ({
    time: 12,
    stepped: 12.04,
    back: false,
    shift: false,
    through: false,
    clip,
    lit,
    looping: false,
    ...o,
  });

  test("steps to the frame beside without Shift, where the engine's table says it begins", () => {
    expect(arrowStep(key({}))).toEqual({ to: 12.04, word: false });
    expect(arrowStep(key({ back: true, stepped: 11.96 }))).toEqual({ to: 11.96, word: false });
  });

  // #377: outside a clip Shift skips, transcribed or not.
  // Outside the clip the playhead is on the video, which the place says.
  test("with Shift skips SKIP seconds on the video, through the clip too, and with no clip", () => {
    expect(arrowStep(key({ shift: true, time: 30, through: true }))).toEqual({ to: 30 + SKIP, word: false });
    expect(arrowStep(key({ shift: true, time: 30, back: true, through: true }))).toEqual({ to: 30 - SKIP, word: false });
    expect(arrowStep(key({ shift: true, time: 12, through: true }))).toEqual({ to: 12 + SKIP, word: false });
    expect(arrowStep(key({ shift: true, time: 12, clip: null }))).toEqual({ to: 12 + SKIP, word: false });
  });

  // The picture answers a playhead put on a clip's first second with the
  // frame it shows, which begins a little before. That is still the clip.
  test("with Shift walks the words a frame before the clip's start, as it does inside it", () => {
    expect(arrowStep(key({ shift: true, time: 10 - frame / 2 }))).toEqual({ to: 10.54, word: true });
  });

  test("with Shift walks the words the caption lights inside a piece", () => {
    expect(arrowStep(key({ shift: true, time: 12 }))).toEqual({ to: 18.04, word: true });
    // Back out of a word is the word before it, and back out of the
    // silence after a word is that word, the one just spoken.
    expect(arrowStep(key({ shift: true, time: 18.3, back: true }))).toEqual({ to: 10.54, word: true });
    expect(arrowStep(key({ shift: true, time: 19, back: true }))).toEqual({ to: 18.04, word: true });
  });

  // Tim: the steps are for the words the video preview shows, and a word
  // a cut took out is not shown, so from a cut a step goes to the word
  // the short says next, or said last, and never into the cut.
  test("with Shift from a cut walks to the words the caption lights either side, never the words cut out", () => {
    expect(arrowStep(key({ shift: true, time: 14.5 }))).toEqual({ to: 18.04, word: true });
    expect(arrowStep(key({ shift: true, time: 15.2 }))).toEqual({ to: 18.04, word: true });
    expect(arrowStep(key({ shift: true, time: 15.2, back: true }))).toEqual({ to: 10.54, word: true });
  });

  test("out of the caption's words goes round with loop on, and hands over to the clip beside without", () => {
    expect(arrowStep(key({ shift: true, time: 19, looping: true }))).toEqual({ to: 10.54, word: true });
    expect(arrowStep(key({ shift: true, time: 10.2, back: true, looping: true }))).toEqual({ to: 18.04, word: true });
    expect(arrowStep(key({ shift: true, time: 19 }))).toBe("clip");
  });

  test("with Shift before the captions arrive steps a second", () => {
    expect(arrowStep(key({ shift: true, time: 12, lit: [] }))).toEqual({ to: 13, word: false });
    expect(arrowStep(key({ shift: true, time: 15.2, lit: [], back: true }))).toEqual({ to: 14.2, word: false });
  });
});
