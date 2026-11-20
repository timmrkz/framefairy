import { describe, expect, it, test } from "vitest";
import {
  Newest,
  mergeJob,
  insideClip,
  inEpisode,
  endInEpisode,
  inClip,
  cutsOf,
  wordInEpisode,
  litWord,
  frameAt,
  frameMiddle,
  frameBegins,
  type Piece,
} from "./flow";

describe("only the newest answer counts", () => {
  test("an answer is used when nothing newer has arrived", () => {
    const asks = new Newest();
    const one = asks.send();
    expect(asks.keep(one)).toBe(true);
  });

  test("answers that come back in order are all used", () => {
    const asks = new Newest();
    const tickets = [asks.send(), asks.send(), asks.send()];
    expect(tickets.map((t) => asks.keep(t))).toEqual([true, true, true]);
  });

  test("an answer that comes back late is thrown away", () => {
    const asks = new Newest();
    const slow = asks.send();
    const quick = asks.send();
    expect(asks.keep(quick)).toBe(true);
    expect(asks.keep(slow)).toBe(false);
  });

  test("and so is every older one, however many are in the air", () => {
    const asks = new Newest();
    const tickets = [asks.send(), asks.send(), asks.send(), asks.send(), asks.send()];
    // The last one home first, which is what hammering a key looks like.
    expect(asks.keep(tickets[4])).toBe(true);
    expect(tickets.slice(0, 4).map((t) => asks.keep(t))).toEqual([false, false, false, false]);
  });

  test("the same answer is never used twice", () => {
    const asks = new Newest();
    const one = asks.send();
    expect(asks.keep(one)).toBe(true);
    expect(asks.keep(one)).toBe(false);
  });

  test("an ask that failed does not let an older answer through", () => {
    const asks = new Newest();
    const old = asks.send();
    const failed = asks.send();
    // The newer one came back with nothing, and still closes the door.
    asks.keep(failed);
    expect(asks.keep(old)).toBe(false);
  });

  test("a newer answer is still used after one failed", () => {
    const asks = new Newest();
    const failed = asks.send();
    asks.keep(failed);
    const next = asks.send();
    expect(asks.keep(next)).toBe(true);
  });
});

describe("endInEpisode", () => {
  const two = [
    { start: 10, end: 14 },
    { start: 16, end: 20 },
  ];

  // A caption that goes where the cut begins goes at 14, not at 16: drawn
  // to 16 it lay over the whole cut.
  test("an end at a cut is the end of the piece before", () => {
    expect(endInEpisode(two, 4)).toBe(14);
    expect(inEpisode(two, 4)).toBe(16);
  });

  test("anywhere else it is where inEpisode puts it", () => {
    for (const at of [0, 1.5, 3.99, 4.01, 6, 8]) {
      expect(endInEpisode(two, at)).toBeCloseTo(inEpisode(two, at), 9);
    }
  });

  test("a hair past the cut on the other clock is still at the cut", () => {
    expect(endInEpisode(two, 4 + 1e-9)).toBe(14);
  });

  test("stops at the end of the clip, and gives the time back with no pieces", () => {
    expect(endInEpisode(two, 99)).toBe(20);
    expect(endInEpisode([], 7)).toBe(7);
  });
});

describe("inEpisode", () => {
  // A clip in two pieces with a two second cut between them. The clip's
  // own clock runs 0 to 8, the episode's 10 to 20.
  const two = [
    { start: 10, end: 14 },
    { start: 16, end: 20 },
  ];

  test("walks the pieces, adding the cuts back", () => {
    expect(inEpisode(two, 0)).toBe(10);
    expect(inEpisode(two, 3)).toBe(13);
    expect(inEpisode(two, 4)).toBe(16);
    expect(inEpisode(two, 5)).toBe(17);
  });

  test("stops at the end of the clip", () => {
    expect(inEpisode(two, 8)).toBe(20);
    expect(inEpisode(two, 99)).toBe(20);
  });

  test("gives the time back with no pieces at all", () => {
    expect(inEpisode([], 7)).toBe(7);
  });
});

describe("inClip", () => {
  const two = [
    { start: 10, end: 14 },
    { start: 16, end: 20 },
  ];

  test("is the way back from inEpisode", () => {
    for (const at of [0, 1.5, 3, 4, 5, 7.25, 8]) {
      expect(inClip(two, inEpisode(two, at))).toBeCloseTo(at, 9);
    }
  });

  test("puts a moment the clip cuts out where the clip comes back", () => {
    expect(inClip(two, 15)).toBe(4);
    expect(inClip(two, 5)).toBe(0);
    expect(inClip(two, 99)).toBe(8);
  });
});

describe("cutsOf", () => {
  test("is the gap between two pieces, counted from the first", () => {
    const three = [
      { start: 10, end: 14 },
      { start: 16, end: 20 },
      { start: 21, end: 25 },
    ];
    expect(cutsOf(three)).toEqual([
      { index: 0, from: 14, to: 16 },
      { index: 1, from: 20, to: 21 },
    ]);
  });

  // A crop moved part way through a clip splits a piece in two that touch.
  test("finds no cut between pieces that touch, and keeps the count of the rest", () => {
    const touching = [
      { start: 10, end: 12 },
      { start: 12, end: 14 },
      { start: 16, end: 20 },
    ];
    expect(cutsOf(touching)).toEqual([{ index: 1, from: 14, to: 16 }]);
  });

  test("finds none in a clip of one piece or none", () => {
    expect(cutsOf([{ start: 1, end: 2 }])).toEqual([]);
    expect(cutsOf([])).toEqual([]);
  });
});

describe("insideClip", () => {
  const two = [
    { start: 1677.61, end: 1690 },
    { start: 1700, end: 1712 },
  ];
  // Twenty-five a second, which is what the episodes are.
  const frame = 0.04;

  test("the playhead inside a piece is inside the clip", () => {
    expect(insideClip(two, 1680, frame)).toBe(true);
    expect(insideClip(two, 1705, frame)).toBe(true);
  });

  test("the playhead in a cut is not", () => {
    expect(insideClip(two, 1695, frame)).toBe(false);
  });

  // The one Tim saw. A clip picked from the list puts the playhead on its
  // first second, and the picture answers with the frame it is showing,
  // which begins a hundredth of a second before it. The crop frame went
  // dashed at the start of the clip it belonged to, and one press of an
  // arrow key put it right.
  test("the frame a piece begins in belongs to it", () => {
    // 1677.61 falls a quarter of a frame past one, so the picture settles
    // on 1677.60 and answers with that.
    expect(insideClip(two, 1677.6, frame)).toBe(true);
    expect(insideClip(two, 1677.61 - 0.0000007, frame)).toBe(true);
  });

  test("and the one it ends in, so nothing blinks at the end", () => {
    expect(insideClip(two, 1690.02, frame)).toBe(true);
  });

  test("a whole frame before a clip is still outside it", () => {
    expect(insideClip(two, 1677.61 - 0.05, frame)).toBe(false);
  });

  test("no pieces, nowhere inside", () => {
    expect(insideClip([], 5, frame)).toBe(false);
  });
});

describe("a job's news", () => {
  const job = (state: string, seq?: number) => ({ id: "job-1", state, seq });

  test("a later snapshot replaces an earlier one", () => {
    const list = [job("queued", 1)];
    expect(mergeJob(list, job("running", 4))).toEqual([job("running", 4)]);
  });

  test("an earlier snapshot arriving late is dropped", () => {
    // Queued was sent after the lock was let go, and running and done
    // overtook it. Kept, it showed a finished job as waiting for good.
    let list = [job("queued", 1)];
    for (const got of [job("running", 3), job("done", 7), job("queued", 1)]) {
      list = mergeJob(list, got) ?? list;
    }
    expect(list).toEqual([job("done", 7)]);
  });

  test("a job not heard of before is added", () => {
    expect(mergeJob([job("done", 2)], { id: "job-2", state: "queued", seq: 3 })).toHaveLength(2);
  });

  test("a snapshot without a number is taken as it comes", () => {
    expect(mergeJob([job("running")], job("done"))).toEqual([job("done")]);
  });

  test("the list read again merges with what was heard, in any order", () => {
    const heard = [job("running", 5)];
    const read = [job("queued", 2)];
    let list = heard;
    for (const got of read) list = mergeJob(list, got) ?? list;
    expect(list).toEqual([job("running", 5)]);
  });
});

describe("the word lit in a caption", () => {
  const lines = [
    { words: [{ start: 1.0 }, { start: 1.4 }] },
    { words: [{ start: 2.0 }] },
  ];
  test("is the last word whose start has come, across the lines", () => {
    expect(litWord(lines, 0.9)).toBe(-1);
    expect(litWord(lines, 1.0)).toBe(0);
    expect(litWord(lines, 1.39)).toBe(0);
    expect(litWord(lines, 1.4)).toBe(1);
    expect(litWord(lines, 2.5)).toBe(2);
  });
  // The render keeps a word lit until the next one starts, so a pause
  // between two words, or a word held longer than it was timed, never
  // leaves the caption without its highlight.
  test("stays lit between words, the way the render draws it", () => {
    expect(litWord(lines, 1.95)).toBe(1);
  });
});

// Plan row 2.137: a picture that starts a quarter of a second after its
// sound has its frames from 0.25 on, and frames counted from the start of
// the file stood half a frame from them.
describe("frames are counted from where the picture starts", () => {
  const frame = 0.04;
  test("a moment falls in the picture's own frame", () => {
    expect(frameAt(0.25, frame, 0.25)).toBe(0);
    expect(frameAt(0.288, frame, 0.25)).toBe(0);
    // A hair before the next frame's start is in that frame, the way the
    // engine counts it, see heldMicros.
    expect(frameAt(0.289, frame, 0.25)).toBe(1);
    expect(frameAt(0.29, frame, 0.25)).toBe(1);
    // Counted to the microsecond, as the engine's frameHolding counts:
    // frame 2 at 29.97 starts at 0.0667333 s, so a hair and a microsecond
    // before it is frame 1, and a hair before it frame 2.
    expect(frameAt(0.065733, 1001 / 30000)).toBe(1);
    expect(frameAt(0.065734, 1001 / 30000)).toBe(2);
    expect(frameAt(1.0, frame, 0.25)).toBe(18);
    // Counted from the file, 1.0 is frame 25, a frame the picture has not.
    expect(frameAt(1.0, frame)).toBe(25);
  });
  test("before the picture starts is its first frame", () => {
    expect(frameAt(0.1, frame, 0.25)).toBe(0);
  });
  test("the middle of a frame is in that frame", () => {
    expect(frameMiddle(18, frame, 0.25)).toBeCloseTo(0.99, 9);
    expect(frameAt(frameMiddle(18, frame, 0.25), frame, 0.25)).toBe(18);
  });
  test("the start of a frame is in that frame, not the one before", () => {
    expect(frameBegins(18, frame, 0.25)).toBeCloseTo(0.97, 9);
    for (const k of [0, 1, 18, 749, 4500]) expect(frameAt(frameBegins(k, 1001 / 30000, 0.25), 1001 / 30000, 0.25)).toBe(k);
  });
});

describe("wordInEpisode", () => {
  // A clip of 20 to 21 and 24 to 26, with a word said at 24.5, which is 1.5
  // into the clip. The cut's edges moved put that word elsewhere on the
  // clip's clock and never elsewhere in the episode, so it is one word to
  // the caption block and to the pill, and neither pops for it.
  test("a cut's edges moved leave a word after it the same word", () => {
    const shapes: Piece[][] = [
      [{ start: 20, end: 21 }, { start: 24, end: 26 }],
      [{ start: 20, end: 20.64 }, { start: 24, end: 26 }],
      [{ start: 20, end: 21 }, { start: 24.28, end: 26 }],
      [{ start: 20, end: 20.333 }, { start: 23.917, end: 26 }],
    ];
    const onClip = shapes.map((p) => inClip(p, 24.5));
    expect(new Set(onClip).size).toBe(shapes.length);
    for (const [i, p] of shapes.entries()) expect(wordInEpisode(p, onClip[i])).toBe(24.5);
  });

  // A word a cut takes the start of stands where the cut ends, which on
  // the clip's clock is also where the piece before it ends. It is the
  // piece after's.
  test("a word where a cut ends is the piece after's", () => {
    const pieces: Piece[] = [{ start: 20.1, end: 20.7 }, { start: 24.3, end: 26 }];
    expect(wordInEpisode(pieces, 0.6)).toBe(24.3);
  });
});

