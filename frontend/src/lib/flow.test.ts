import { describe, expect, it, test } from "vitest";
import {
  waitShare,
  Heard,
  heardIn,
  joinParts,
  gapsIn,
  covers,
  type Parts,
  Newest,
  mergeJob,
  nextWindow,
  timesIn,
  followingWindow,
  gridStep,
  onGrid,
  insideClip,
  inEpisode,
  endInEpisode,
  inClip,
  draftCaptions,
  litWord,
} from "./flow";

describe("the window moves on when it chooses for itself", () => {
  const half = 30 * 60;
  const hours = 4 * 3600;

  test("a fresh episode opens on the first half hour", () => {
    const w = nextWindow([{ from: 0, to: hours, times: 0 }], hours, half);
    expect(w).toEqual({ from: 0, to: half });
  });

  test("after the first search it moves past what was searched", () => {
    const passes = [
      { from: 0, to: half, times: 1 },
      { from: half, to: hours, times: 0 },
    ];
    expect(nextWindow(passes, hours, half)).toEqual({ from: half, to: 2 * half });
  });

  test("a gap in the middle is taken before the end", () => {
    const passes = [
      { from: 0, to: half, times: 1 },
      { from: half, to: 2 * half, times: 0 },
      { from: 2 * half, to: 3 * half, times: 1 },
      { from: 3 * half, to: hours, times: 0 },
    ];
    expect(nextWindow(passes, hours, half)).toEqual({ from: half, to: 2 * half });
  });

  test("a part a little longer than the half hour is taken whole", () => {
    const passes = [
      { from: 0, to: 40 * 60, times: 0 },
      { from: 40 * 60, to: hours, times: 1 },
    ];
    expect(nextWindow(passes, hours, half)).toEqual({ from: 0, to: 40 * 60 });
  });

  test("a part well over the half hour gives up only that much", () => {
    const passes = [
      { from: 0, to: 60 * 60, times: 0 },
      { from: 60 * 60, to: hours, times: 1 },
    ];
    expect(nextWindow(passes, hours, half)).toEqual({ from: 0, to: half });
  });

  test("an episode searched end to end starts over at the start", () => {
    expect(nextWindow([{ from: 0, to: hours, times: 1 }], hours, half)).toEqual({ from: 0, to: half });
  });

  test("and the second round walks on the way the first did", () => {
    const passes = [
      { from: 0, to: half, times: 2 },
      { from: half, to: hours, times: 1 },
    ];
    expect(nextWindow(passes, hours, half)).toEqual({ from: half, to: 2 * half });
  });

  test("a short episode is searched whole, again and again", () => {
    const six = 6 * 60;
    expect(nextWindow([{ from: 0, to: six, times: 0 }], six, six)).toEqual({ from: 0, to: six });
    expect(nextWindow([{ from: 0, to: six, times: 3 }], six, six)).toEqual({ from: 0, to: six });
  });

  test("a scrap too short for a clip is passed over", () => {
    const passes = [
      { from: 0, to: 10, times: 0 },
      { from: 10, to: half, times: 1 },
      { from: half, to: hours, times: 0 },
    ];
    expect(nextWindow(passes, hours, half, 20)).toEqual({ from: half, to: 2 * half });
  });

  test("how often a window has been searched is the most of any part in it", () => {
    const passes = [
      { from: 0, to: half, times: 2 },
      { from: half, to: hours, times: 1 },
    ];
    expect(timesIn(passes, half, 2 * half)).toBe(1);
    expect(timesIn(passes, half - 60, 2 * half)).toBe(2);
    expect(timesIn([], 0, half)).toBe(0);
  });
});

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

describe("draftCaptions", () => {
  // Two captions that meet at 2, and a third after a pause.
  const three = [
    { start: 0, end: 2 },
    { start: 2, end: 3.5 },
    { start: 4.5, end: 6 },
  ];

  test("a caption that appears later keeps the one before up until it does", () => {
    const got = draftCaptions(three, { index: 1, edge: "start", at: 2.4 });
    expect(got[0].end).toBe(2.4);
    expect(got[1].start).toBe(2.4);
  });

  test("a caption that appears earlier ends the one before", () => {
    const got = draftCaptions(three, { index: 1, edge: "start", at: 1.6 });
    expect(got[0].end).toBe(1.6);
  });

  test("after a pause, appearing later leaves the one before alone", () => {
    const got = draftCaptions(three, { index: 2, edge: "start", at: 4.8 });
    expect(got[1].end).toBe(3.5);
    expect(got[2].start).toBe(4.8);
  });

  test("going earlier leaves a gap and moves nothing else", () => {
    const got = draftCaptions(three, { index: 0, edge: "end", at: 1.5 });
    expect(got[0].end).toBe(1.5);
    expect(got[1].start).toBe(2);
  });

  test("changes nothing it is handed", () => {
    draftCaptions(three, { index: 1, edge: "start", at: 2.4 });
    expect(three[0].end).toBe(2);
    expect(draftCaptions(three, null)).toBe(three);
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

// The row of a search waiting for the transcript is a view of the window on
// the range picker: how much of the window is transcribed. Measured from the
// start of the episode, a window two hours in began nearly full.
describe("waitShare", () => {
  it("is empty while nothing of the window is heard", () => {
    expect(waitShare(7200, [], 9000)).toBe(0);
    expect(waitShare(7200, [[0, 7200]], 9000)).toBe(0);
  });

  it("is how much of the window is transcribed", () => {
    expect(waitShare(7200, [[0, 8100]], 9000)).toBe(0.5);
    expect(waitShare(1800, [[0, 2500]], 3600)).toBeCloseTo(700 / 1800, 9);
    // Wherever in the window it was heard.
    expect(waitShare(1800, [[3000, 3600]], 3600)).toBeCloseTo(600 / 1800, 9);
  });

  it("is full when all of the window is heard and never past it", () => {
    expect(waitShare(1800, [[0, 3600]], 3600)).toBe(1);
    expect(waitShare(1800, [[0, 4000]], 3600)).toBe(1);
  });
});

describe("after a search the window walks on as long as it was left", () => {
  const six = 6 * 60;
  const short = 5 * 60 + 25;

  test("a minute at four goes on to a minute at five", () => {
    expect(followingWindow(240, 60, six, 20)).toEqual({ from: 240, to: 300 });
  });

  test("the last one is cut at the end of the episode", () => {
    expect(followingWindow(300, 60, short, 20)).toEqual({ from: 300, to: short });
  });

  test("and the one after it is as long as it was set again, from the start", () => {
    expect(followingWindow(short, 60, short, 20)).toEqual({ from: 0, to: 60 });
  });

  test("a scrap too short for a clip is passed over", () => {
    expect(followingWindow(310, 60, short, 20)).toEqual({ from: 0, to: 60 });
  });

  test("a short episode searched whole is searched whole again", () => {
    expect(followingWindow(six, six, six, 20)).toEqual({ from: 0, to: six });
  });
});

describe("every window lands on the range picker's step", () => {
  const hours = 14423;

  test("the step is the smallest round one still eight pixels wide", () => {
    expect(gridStep(hours, 915)).toBe(300);
    expect(gridStep(hours, 1370)).toBe(120);
    expect(gridStep(360, 915)).toBe(5);
  });

  test("the app's own window of 30:02 is 30:00 on the step", () => {
    expect(onGrid({ from: 0, to: 1803 }, 1803, hours, 300, 20)).toEqual({ from: 0, to: 1800 });
  });

  test("a window after one that ended off the step starts on it", () => {
    expect(onGrid({ from: 4202, to: 6004 }, 1802, hours, 300, 20)).toEqual({ from: 4200, to: 6000 });
  });

  test("the last one takes in the scrap after it", () => {
    expect(onGrid({ from: 12600, to: 14400 }, 1800, hours, 300, 20)).toEqual({ from: 12600, to: hours });
  });

  test("never shorter than a step", () => {
    expect(onGrid({ from: 60, to: 62 }, 2, 360, 5, 0)).toEqual({ from: 60, to: 65 });
  });

  test("with no step it is as it was", () => {
    expect(onGrid({ from: 7, to: 70 }, 63, 360, 0, 20)).toEqual({ from: 7, to: 70 });
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
