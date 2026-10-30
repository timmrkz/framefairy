import { describe, expect, it, test } from "vitest";
import {
  nothingToMark,
  Heard,
  heardIn,
  joinParts,
  gapsIn,
  covers,
  type Parts,
  Newest,
  mergeJob,
  timesIn,
  gridStep,
  insideClip,
  inEpisode,
  shareOf,
  endInEpisode,
  inClip,
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

describe("what is heard only ever grows", () => {
  // It says how much of the episode has been read. Reading does not
  // unhappen, so no ordering of what the Go side reports may take any of
  // it back.
  const ep = "/eps/ep.mp4";
  const upTo = (at: number): Parts => (at > 0 ? [[0, at]] : []);

  test("it follows the work while the transcription runs", () => {
    const h = new Heard();
    expect(h.seen(ep, [], [0, 12], false)).toEqual([[0, 12]]);
    expect(h.seen(ep, [], [0, 30], false)).toEqual([[0, 30]]);
    // The save lands, far behind the work, and changes nothing.
    expect(h.seen(ep, upTo(24), [0, 42], false)).toEqual([[0, 42]]);
  });

  test("pausing does not take any of it back", () => {
    // This is the bug as it was seen. The job reported 300 seconds, the
    // transcript on disk had only been written at 240, and the moment the
    // job stopped the live number was gone.
    const h = new Heard();
    expect(h.seen(ep, upTo(240), [0, 300], false)).toEqual([[0, 300]]);
    expect(h.seen(ep, upTo(240), null, false)).toEqual([[0, 300]]);
  });

  test("carrying on from a pause does not either", () => {
    // A transcription that carries on starts again from the end of the
    // saved transcript, which is behind where the work had got to. Those
    // seconds were heard, so the edge stands still until the work passes
    // them rather than jumping back and climbing twice.
    const h = new Heard();
    h.seen(ep, upTo(240), [0, 300], false);
    expect(h.seen(ep, upTo(240), [240, 240], false)).toEqual([[0, 300]]);
    expect(h.seen(ep, upTo(240), [240, 280], false)).toEqual([[0, 300]]);
    expect(h.seen(ep, upTo(240), [240, 310], false)).toEqual([[0, 310]]);
  });

  test("an older answer landing after a newer one does not take any back", () => {
    // Every refresh is a call of its own and nothing says they come back in
    // the order they went out.
    const h = new Heard();
    expect(h.seen(ep, upTo(600), null, false)).toEqual([[0, 600]]);
    expect(h.seen(ep, upTo(120), null, false)).toEqual([[0, 600]]);
  });

  test("a part heard where a clip made by hand needs it stands apart", () => {
    const h = new Heard();
    expect(h.seen(ep, upTo(600), [5000, 5030], false)).toEqual([
      [0, 600],
      [5000, 5030],
    ]);
    // And when the gap between is heard, they are one.
    expect(h.seen(ep, [[0, 5030]], null, false)).toEqual([[0, 5030]]);
  });

  test("the same parts are the same answer", () => {
    const h = new Heard();
    const first = h.seen(ep, upTo(60), null, false);
    expect(h.seen(ep, upTo(60), [10, 20], false)).toBe(first);
  });

  test("whatever order the two arrive in, the answer is the same", () => {
    // The job and the episode refresh land independently, so this walks
    // every interleaving of a plausible run and insists the result never
    // shrinks and always ends at the furthest either of them reached.
    const saved = [0, 0, 60, 60, 120, 180, 180];
    const running = [10, 45, 45, 90, 150, 150, 200];
    for (let shift = 0; shift < saved.length; shift++) {
      const h = new Heard();
      let last = 0;
      let most = 0;
      for (let i = 0; i < saved.length; i++) {
        // The saved number lags by shift steps, which is what a save
        // landing late looks like.
        const s = saved[Math.max(0, i - shift)];
        const r = running[i];
        most = Math.max(most, s, r);
        const got = heardIn(h.seen(ep, upTo(s), [0, r], false), 0, 1e9);
        expect(got, `shift ${shift} step ${i}`).toBeGreaterThanOrEqual(last);
        last = got;
      }
      expect(last, `shift ${shift}`).toBe(most);
    }
  });

  test("another episode starts the mark over", () => {
    const h = new Heard();
    h.seen(ep, [], [0, 900], false);
    expect(h.seen("/eps/zwei.mp4", [], [0, 5], false)).toEqual([[0, 5]]);
    // And going back does not bring the first one's mark with it.
    expect(h.seen(ep, [], [0, 3], false)).toEqual([[0, 3]]);
  });

  test("a transcript being read again starts the mark over", () => {
    // The file changed, so the old mark is about a transcript that no
    // longer exists.
    const h = new Heard();
    h.seen(ep, upTo(600), null, false);
    expect(h.seen(ep, [], [0, 10], true)).toEqual([[0, 10]]);
    expect(h.seen(ep, [], [0, 20], false)).toEqual([[0, 20]]);
  });

  test("a work folder that is gone starts the mark over", () => {
    const h = new Heard();
    h.seen(ep, upTo(600), null, false);
    expect(h.seen(ep, [], null, true)).toEqual([]);
  });
});

describe("parts", () => {
  test("joined in order, touching ones made one", () => {
    expect(joinParts([[40, 50], [0, 10], [10, 20], [45, 60]])).toEqual([
      [0, 20],
      [40, 60],
    ]);
  });

  test("what they hold of a span, and what they leave", () => {
    const parts: Parts = [
      [0, 10],
      [40, 60],
    ];
    expect(heardIn(parts, 5, 50)).toBe(15);
    expect(gapsIn(parts, 5, 70)).toEqual([
      [10, 40],
      [60, 70],
    ]);
    expect(covers(parts, 42, 59)).toBe(true);
    expect(covers(parts, 5, 50)).toBe(false);
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

describe("shareOf", () => {
  // The clip's own clock adds up its pieces, and a share of it lands in
  // the episode past every cut before it.
  test.each([
    ["no cut", [{ start: 10, end: 20 }], [[0, 10], [0.5, 15], [0.9, 19]]],
    // Half way is the first moment after the cut, never inside it.
    ["one cut in the middle", [{ start: 10, end: 14 }, { start: 16, end: 20 }], [[0, 10], [0.3, 12.4], [0.5, 16], [0.9, 19.2]]],
    ["a cut at the start", [{ start: 13, end: 20 }], [[0, 13], [0.5, 16.5], [0.9, 19.3]]],
    ["a cut at the end", [{ start: 10, end: 14 }, { start: 16, end: 17 }], [[0, 10], [0.5, 12.5], [0.9, 16.5]]],
  ] as [string, Piece[], [number, number][]][])("%s", (_, pieces, shares) => {
    for (const [share, at] of shares) expect(shareOf(pieces, share)).toBeCloseTo(at, 9);
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

describe("a dragged window lands on the range picker's step", () => {
  const hours = 14423;

  test("the step is the smallest round one still eight pixels wide", () => {
    expect(gridStep(hours, 915)).toBe(300);
    expect(gridStep(hours, 1370)).toBe(120);
    expect(gridStep(360, 915)).toBe(5);
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
    expect(frameAt(0.289, frame, 0.25)).toBe(0);
    expect(frameAt(0.29, frame, 0.25)).toBe(1);
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

describe("where I and O can make a clip", () => {
  const all: Parts = [[0, 120]];
  test("I makes none at the very end of the video, heard or not", () => {
    expect(nothingToMark(120, false, [], undefined, 120)).toBe(true);
    expect(nothingToMark(119.97, false, [], undefined, 120)).toBe(true);
  });
  test("I makes none on the last frame either, where play stops short of the end", () => {
    expect(nothingToMark(120 - 1 / 15, false, [], undefined, 120)).toBe(true);
    expect(nothingToMark(120 - 1 / 15, false, [[0, 60]], [3, 58], 120)).toBe(true);
    expect(nothingToMark(119.6, false, [[0, 60]], [3, 58], 120)).toBe(true);
  });
  test("O makes none at the very start of the video, heard or not", () => {
    expect(nothingToMark(0, true, [], undefined, 120)).toBe(true);
  });
  test("I after the last word and O before the first make none where it is heard", () => {
    expect(nothingToMark(115, false, all, [3, 110], 120)).toBe(true);
    expect(nothingToMark(110, false, all, [3, 110], 120)).toBe(true);
    expect(nothingToMark(2, true, all, [3, 110], 120)).toBe(true);
  });
  test("I before the last word and O after the first make one", () => {
    expect(nothingToMark(109.9, false, all, [3, 110], 120)).toBe(false);
    expect(nothingToMark(3, true, all, [3, 110], 120)).toBe(false);
    expect(nothingToMark(115, true, all, [3, 110], 120)).toBe(false);
    expect(nothingToMark(2, false, all, [3, 110], 120)).toBe(false);
  });
  test("a video heard to its end with no word in it offers neither", () => {
    expect(nothingToMark(60, false, all, null, 120)).toBe(true);
    expect(nothingToMark(60, true, all, null, 120)).toBe(true);
  });
  test("a part not heard yet is heard first, so I and O are offered there", () => {
    const half: Parts = [[0, 60]];
    expect(nothingToMark(100, false, half, [3, 58], 120)).toBe(false);
    expect(nothingToMark(100, true, [[60, 120]], [70, 110], 120)).toBe(false);
  });
  test("a video of no length offers nothing to decide", () => {
    expect(nothingToMark(0, false, [], undefined, 0)).toBe(false);
  });
});
