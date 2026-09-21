import { describe, expect, test } from "vitest";
import {
  Newest,
  nextWindow,
  pictureIsStale,
  shouldLook,
  shouldTranscribe,
  type SearchState,
} from "./flow";

const idle: SearchState = {
  covered: 0,
  to: 1800,
  plans: 0,
  clips: 0,
  busy: false,
  looked: false,
};

describe("a new episode transcribes itself", () => {
  test("with no work folder beside it", () => {
    expect(shouldTranscribe({ missing: false, work: false, busy: false })).toBe(true);
  });

  test("but not while something of it is already running", () => {
    expect(shouldTranscribe({ missing: false, work: false, busy: true })).toBe(false);
  });

  test("and not when it has a work folder already", () => {
    expect(shouldTranscribe({ missing: false, work: true, busy: false })).toBe(false);
  });

  test("and never when the file is gone", () => {
    expect(shouldTranscribe({ missing: true, work: false, busy: false })).toBe(false);
  });
});

describe("the first clips are found by themselves", () => {
  test("not before the transcript reaches the end of the stretch", () => {
    expect(shouldLook({ ...idle, covered: 1200 })).toBe(false);
  });

  test("as soon as it does", () => {
    expect(shouldLook({ ...idle, covered: 1800 })).toBe(true);
  });

  test("within the half second the frames are counted in", () => {
    expect(shouldLook({ ...idle, covered: 1799.7 })).toBe(true);
  });

  test("also when the episode was transcribed before it was added", () => {
    expect(shouldLook({ ...idle, covered: 14423 })).toBe(true);
  });

  test("with a stretch that is the whole episode", () => {
    expect(shouldLook({ ...idle, to: 14423, covered: 14423 })).toBe(true);
  });

  test("but not while there is no stretch yet", () => {
    expect(shouldLook({ ...idle, to: 0, covered: 14423 })).toBe(false);
  });

  test("not when the episode has been searched already", () => {
    expect(shouldLook({ ...idle, covered: 1800, plans: 1 })).toBe(false);
  });

  test("not when it has clips already", () => {
    expect(shouldLook({ ...idle, covered: 1800, clips: 4 })).toBe(false);
  });

  test("not while a search or a render is running", () => {
    expect(shouldLook({ ...idle, covered: 1800, busy: true })).toBe(false);
  });

  test("and only once for an episode", () => {
    expect(shouldLook({ ...idle, covered: 1800, looked: true })).toBe(false);
  });

  test("never again for an episode that was searched and emptied since", () => {
    // The plans and the clips are gone, but the episode remembers that
    // somebody looked, so the machine is not spent on it unasked.
    expect(shouldLook({ ...idle, covered: 1800, plans: 0, clips: 0, looked: true })).toBe(false);
  });
});

describe("the picture agrees with the playhead", () => {
  test("or the frame is read from the file instead", () => {
    expect(pictureIsStale({ ready: true, shows: 12, at: 902 })).toBe(true);
  });

  test("and it does when the window is at the playhead", () => {
    expect(pictureIsStale({ ready: true, shows: 902, at: 902 })).toBe(false);
  });

  test("within half a second, because a seek lands on a frame", () => {
    expect(pictureIsStale({ ready: true, shows: 902.2, at: 902 })).toBe(false);
  });

  test("a window that has decoded nothing is always stale", () => {
    expect(pictureIsStale({ ready: false, shows: -1, at: 0 })).toBe(true);
    expect(pictureIsStale({ ready: true, shows: -1, at: 0 })).toBe(true);
  });
});

describe("the window moves on when it chooses for itself", () => {
  const half = 30 * 60;
  const hours = 4 * 3600;

  test("a fresh episode opens on the first half hour", () => {
    const w = nextWindow([{ from: 0, to: hours }], [], hours, half);
    expect(w).toEqual({ from: 0, to: half });
  });

  test("after the first search it moves past what was searched", () => {
    const w = nextWindow([{ from: half, to: hours }], [{ from: 0, to: half }], hours, half);
    expect(w).toEqual({ from: half, to: 2 * half });
  });

  test("so it never lies on the clips that were just found", () => {
    const searched = [{ from: 0, to: half }];
    const w = nextWindow([{ from: half, to: hours }], searched, hours, half);
    expect(searched.some((s) => s.to > w.from && s.from < w.to)).toBe(false);
  });

  test("a gap in the middle is taken before the end", () => {
    const w = nextWindow(
      [
        { from: half, to: 2 * half },
        { from: 3 * half, to: hours },
      ],
      [
        { from: 0, to: half },
        { from: 2 * half, to: 3 * half },
      ],
      hours,
      half,
    );
    expect(w).toEqual({ from: half, to: 2 * half });
  });

  test("a stretch a little longer than the half hour is taken whole", () => {
    const w = nextWindow([{ from: 0, to: 40 * 60 }], [], hours, half);
    expect(w).toEqual({ from: 0, to: 40 * 60 });
  });

  test("a stretch well over the half hour gives up only that much", () => {
    const w = nextWindow([{ from: 0, to: 60 * 60 }], [], hours, half);
    expect(w).toEqual({ from: 0, to: half });
  });

  test("an episode searched end to end rests on the last stretch", () => {
    const searched = [
      { from: 0, to: half },
      { from: half, to: hours },
    ];
    expect(nextWindow([], searched, hours, half)).toEqual({ from: half, to: hours });
  });

  test("an episode with nothing anywhere takes the whole of it", () => {
    expect(nextWindow([], [], hours, half)).toEqual({ from: 0, to: hours });
  });

  test("a scrap of free room too short to search is passed over", () => {
    const w = nextWindow(
      [
        { from: half, to: half + 0.2 },
        { from: 2 * half, to: hours },
      ],
      [{ from: 0, to: half }],
      hours,
      half,
    );
    expect(w).toEqual({ from: 2 * half, to: 3 * half });
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
