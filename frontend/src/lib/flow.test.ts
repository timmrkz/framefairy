import { describe, expect, it, test } from "vitest";
import {
  Newest,
  mergeJob,
  insideClip,
  cutsOf,
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

describe("cutsOf", () => {
  test("is the gap between two pieces, counted from the first", () => {
    const three = [
      { start: 10, end: 14, part: 0 },
      { start: 16, end: 20, part: 1 },
      { start: 21, end: 25, part: 2 },
    ];
    expect(cutsOf(three)).toEqual([
      { index: 0, from: 14, to: 16 },
      { index: 1, from: 20, to: 21 },
    ]);
  });

  // A crop moved part way through a clip splits a piece in two that touch,
  // and the engine says they are of one part.
  test("finds no cut between pieces of one part, and keeps the count of the rest", () => {
    const touching = [
      { start: 10, end: 12, part: 0 },
      { start: 12, end: 14, part: 0 },
      { start: 16, end: 20, part: 1 },
    ];
    expect(cutsOf(touching)).toEqual([{ index: 1, from: 14, to: 16 }]);
  });

  test("finds none in a clip of one piece or none", () => {
    expect(cutsOf([{ start: 1, end: 2, part: 0 }])).toEqual([]);
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

