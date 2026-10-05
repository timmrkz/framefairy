import { describe, expect, it, test } from "vitest";
import {
  frameStart,
  waitShare,
  Heard,
  heardIn,
  joinParts,
  gapsIn,
  covers,
  type Parts,
  Newest,
  Grace,
  type Timers,
  mergeJob,
  nextWindow,
  timesIn,
  followingWindow,
  gridStep,
  onGrid,
  pictureIsStale,
  stillFits,
  pieceAt,
  insideClip,
  playingAt,
  jumpStep,
  playingPiece,
  shouldChase,
  inEpisode,
  endInEpisode,
  inClip,
  draftCaptions,
  litWord,
} from "./flow";

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

  test("a frame on screen stands until the next one", () => {
    // One frame a second: the playhead at 902.9 is in the frame of 902.
    expect(pictureIsStale({ ready: true, shows: 902, at: 902.9, frame: 1 })).toBe(false);
    expect(pictureIsStale({ ready: true, shows: 902, at: 903.6, frame: 1 })).toBe(true);
    // And never the other way: a frame ahead of the playhead is not its.
    expect(pictureIsStale({ ready: true, shows: 903, at: 902.4, frame: 1 })).toBe(true);
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

// A clock the test moves by hand, so nothing waits for real.
function handClock() {
  let now = 0;
  const waiting: { at: number; run: () => void; live: boolean }[] = [];
  const timers: Timers = {
    later: (run, ms) => {
      const w = { at: now + ms, run, live: true };
      waiting.push(w);
      return w;
    },
    off: (w) => {
      if (w) (w as { live: boolean }).live = false;
    },
  };
  const pass = (ms: number) => {
    now += ms;
    for (const w of waiting) {
      if (w.live && w.at <= now) {
        w.live = false;
        w.run();
      }
    }
  };
  return { timers, pass };
}

// The still the workspace asks the engine for while the video preview
// cannot show the playhead. Every ask that is sent is a frame the engine
// reads from the file with ffmpeg and keeps in the work folder.
describe("a still is asked for only if the picture does not land", () => {
  const asked = () => {
    const sent: number[] = [];
    const { timers, pass } = handClock();
    const stills = new Grace<number>((at) => sent.push(at), 180, timers);
    return { sent, stills, pass };
  };

  test("a picture that does not land gets its still, once", () => {
    const { sent, stills, pass } = asked();
    stills.need(902);
    pass(179);
    expect(sent).toEqual([]);
    pass(1);
    expect(sent).toEqual([902]);
    pass(5000);
    expect(sent).toEqual([902]);
  });

  // The picture landed 20 ms after the seek. The engine used to read the
  // frame 160 ms later anyway, for nothing, because a still is never drawn
  // over a picture that shows the playhead.
  test("a picture that lands before the grace is over needs no still", () => {
    const { sent, stills, pass } = asked();
    stills.need(902);
    pass(20);
    stills.need(null);
    pass(5000);
    expect(sent).toEqual([]);
  });

  test("a newer need takes the place of the one waiting", () => {
    const { sent, stills, pass } = asked();
    stills.need(400);
    pass(100);
    stills.need(411);
    pass(100);
    expect(sent).toEqual([]);
    pass(80);
    expect(sent).toEqual([411]);
  });

  test("a picture that lands after the still was sent takes nothing back", () => {
    const { sent, stills, pass } = asked();
    stills.need(57);
    pass(180);
    stills.need(null);
    pass(5000);
    expect(sent).toEqual([57]);
  });

  test("saying nothing is needed with nothing waiting changes nothing", () => {
    const { sent, stills, pass } = asked();
    stills.need(null);
    pass(500);
    stills.need(3);
    pass(180);
    expect(sent).toEqual([3]);
  });

  test("a picture that lands and is lost again asks again, for where it is now", () => {
    const { sent, stills, pass } = asked();
    stills.need(12);
    pass(50);
    stills.need(null);
    pass(50);
    stills.need(15);
    pass(180);
    expect(sent).toEqual([15]);
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

describe("pictureIsStale", () => {
  // What the video element looks like the moment load() has reset it: it
  // is showing nothing, and the second it last showed is near the
  // playhead, because the seek that would not land was a small one.
  test("a reset element is stale, and says so only if ready is cleared", () => {
    expect(pictureIsStale({ ready: false, shows: -1, at: 12.2 })).toBe(true);
    // Leaving ready set, which is what the player used to do, is the bug:
    // a black element reports a good picture, so no frame is asked for and
    // nothing is drawn over the black.
    expect(pictureIsStale({ ready: true, shows: 12, at: 12.2 })).toBe(false);
  });
});

// A video that has read nothing of the file answers zero for its clock,
// playing or not. The first press of the space bar after a search followed
// it to the start of the episode, and the picture with it, until the file
// came and the video went to the clip. The harness shows this with
// ?unread=12, see frontend/preview/open.mjs.
describe("playingAt, without the frames", () => {
  const playing = { clock: 62.53, empty: false, seeking: false, landed: false };

  test("stands still while the video has read nothing", () => {
    expect(playingAt(62.5, { ...playing, clock: 0, empty: true })).toBe(62.5);
    // However long that lasts, frame after frame.
    let at = 62.5;
    for (let i = 0; i < 100; i++) at = playingAt(at, { ...playing, clock: 0, empty: true });
    expect(at).toBe(62.5);
  });

  test("goes with the clock where the browser cannot say which frame is on screen", () => {
    expect(playingAt(62.5, playing)).toBe(62.53);
    expect(playingAt(62.5, { ...playing, presented: null, now: 10, since: 9 })).toBe(62.53);
  });

  test("to where a seek was sent, and to where a jump landed", () => {
    expect(playingAt(62.5, { ...playing, clock: 70, seeking: true })).toBe(70);
    expect(playingAt(70.2, { ...playing, clock: 70, landed: true })).toBe(70);
  });
});

// On the Mac the clock of a playing video is an estimate that never goes
// back: TimeProgressEstimator in WebKit's MediaPlayerPrivateRemote.cpp
// returns the larger of the wall clock since the last report and what it
// returned last. Playing starts on it before the picture underneath does,
// so it runs ahead, and when a report says where the picture is, it stands
// still until the picture catches up. The playhead stood still with it for
// up to half a second, a moment after the space bar, while the picture and
// the sound played on. The playhead follows the frame on screen instead.
// The harness shows this with ?webkitclock.
describe("playingAt, with the frames", () => {
  // 25 frames a second. The play began at 100 s on the page's clock, from
  // 62.5 in the episode, the frame from 62.48 on screen since long before.
  const frame = 0.04;
  const base = { empty: false, seeking: false, landed: false, rate: 1, frame, since: 100 };
  const paused = { media: 62.48, shown: 40 };

  // A play as the Mac gives it: the picture starts 0.4 s after the space
  // bar, the clock at once, and the first report comes at 0.8 s. Sampled
  // at 60 frames a second for three seconds, the clock stands from the
  // report until the picture has caught up.
  function play(seconds = 3) {
    const out: { now: number; at: number; clock: number }[] = [];
    let at = 62.5;
    let said = -Infinity;
    for (let i = 0; i <= seconds * 60; i++) {
      const now = 100 + i / 60;
      const t = now - 100;
      const picture = t < 0.4 ? 62.5 : 62.5 + (t - 0.4);
      const estimate = t < 0.8 ? 62.5 + t : picture;
      said = Math.max(said, estimate);
      // The frame on screen: the last that started at or before the
      // picture, put up when the picture got to it.
      const media = Math.floor(picture / frame + 1e-6) * frame;
      const presented = t < 0.4 ? paused : { media, shown: 100 + 0.4 + (media - 62.5) };
      at = playingAt(at, { ...base, clock: said, presented, now });
      out.push({ now, at, clock: said });
    }
    return out;
  }

  function longestStill(list: { at: number }[], from = 0): number {
    let longest = 0;
    let run = 0;
    for (let i = from + 1; i < list.length; i++) {
      run = list[i].at === list[i - 1].at ? run + 1 : 0;
      longest = Math.max(longest, run);
    }
    return longest;
  }

  test("the clock that runs ahead and then stands, as the Mac's does", () => {
    const run = play();
    // The clock itself stands still for a third of a second after the
    // report, which is what the playhead used to do.
    expect(longestStill(run.map((r) => ({ at: r.clock })), 1)).toBeGreaterThan(20);
  });

  test("stands until the picture moves, then moves on every frame", () => {
    const run = play();
    const first = run.findIndex((r) => r.at !== run[0].at);
    // Not before the picture has started.
    expect(run[first].now - 100).toBeGreaterThanOrEqual(0.4);
    // And from then on it never stands, through the report and after.
    expect(longestStill(run, first)).toBe(0);
  });

  test("never goes back, and stays with the picture", () => {
    const run = play();
    for (let i = 1; i < run.length; i++) expect(run[i].at).toBeGreaterThanOrEqual(run[i - 1].at);
    const end = run[run.length - 1];
    // Three seconds after the space bar the picture is 2.6 s on.
    expect(end.at).toBeGreaterThan(62.5 + 2.6 - frame);
    expect(end.at).toBeLessThanOrEqual(62.5 + 2.6 + frame);
  });

  test("goes on from the frame by the time since it was put up, at most a frame", () => {
    const presented = { media: 70, shown: 101 };
    expect(playingAt(70, { ...base, clock: 70, presented, now: 101.01 })).toBeCloseTo(70.01, 6);
    expect(playingAt(70, { ...base, clock: 70, presented, now: 101.5 })).toBeCloseTo(70.04, 6);
  });

  test("stands with a picture that stalls, however far the clock goes", () => {
    const presented = { media: 70, shown: 101 };
    let at = 70;
    for (let i = 0; i < 30; i++) at = playingAt(at, { ...base, clock: 70 + i / 60, presented, now: 101 + i / 60 });
    expect(at).toBeCloseTo(70.04, 6);
  });

  test("follows a seek, and a jump over a cut, at once, then the frames from there", () => {
    // While it is on its way and as it lands, the clock says where it was sent.
    expect(playingAt(66.2, { ...base, clock: 75, seeking: true, presented: { media: 66.2, shown: 103 }, now: 103.01, since: 103 })).toBe(75);
    expect(playingAt(66.2, { ...base, clock: 75, landed: true, presented: { media: 66.2, shown: 103 }, now: 103.02, since: 103 })).toBe(75);
    // A frame from before the jump says nothing about where the video is.
    expect(playingAt(75, { ...base, clock: 75.1, presented: { media: 66.2, shown: 103 }, now: 103.05, since: 103.001 })).toBe(75);
    // The first frame from where it landed is followed.
    expect(playingAt(75, { ...base, clock: 75.1, presented: { media: 75, shown: 103.1 }, now: 103.12, since: 103.001 })).toBeCloseTo(75.02, 6);
  });

  test("a loop back to the start goes back with it", () => {
    // The jump back to the clip's start lands like any jump.
    expect(playingAt(88, { ...base, clock: 60, landed: true, presented: { media: 87.96, shown: 110 }, now: 110.1, since: 110.05 })).toBe(60);
    // And from there the frames lead.
    expect(playingAt(60, { ...base, clock: 60.3, presented: { media: 60.2, shown: 110.3 }, now: 110.32, since: 110.05 })).toBeCloseTo(60.22, 6);
  });

  test("takes the clock once the browser has said nothing of a frame of this play for a second", () => {
    const presented = { media: 70, shown: 101 };
    expect(playingAt(70.04, { ...base, clock: 72, presented, now: 102.1 })).toBe(72);
  });

  test("waits longer for the first frame of a play or a seek, with the picture", () => {
    // The clock on the Mac has run on from where the play began while the
    // picture has not, so a second without a frame is not reason enough.
    expect(playingAt(62.5, { ...base, clock: 63.6, presented: paused, now: 101.1 })).toBe(62.5);
    expect(playingAt(62.5, { ...base, clock: 65.9, presented: paused, now: 103.9 })).toBe(62.5);
    // Four seconds of nothing is a browser that has stopped saying.
    expect(playingAt(62.5, { ...base, clock: 66.2, presented: paused, now: 104.1 })).toBe(66.2);
  });

  test("holds within the frame on screen, and for no longer", () => {
    // The frame a clip starts inside begins before the clip, and is the
    // same picture until the next frame.
    expect(playingAt(62.5, { ...base, clock: 62.5, presented: { media: 62.48, shown: 100.5 }, now: 100.5 })).toBe(62.5);
    // Ahead of the frame on screen by more than a frame is a playhead
    // somewhere the picture is not, and it goes to the picture. Held, it
    // stood while the picture played until the picture caught up.
    expect(playingAt(62.6, { ...base, clock: 62.6, presented: { media: 62.48, shown: 100.5 }, now: 100.5 })).toBe(62.48);
  });

  test("a picture more than a frame behind is the video somewhere else, and is followed", () => {
    expect(playingAt(70.2, { ...base, clock: 57, presented: { media: 57, shown: 101 }, now: 101 })).toBe(57);
  });

  // A click on the clip timeline while it plays, as the Mac gives it: the
  // seek lands some time after the click, the clock says where it was sent
  // until then and runs on from there by the wall clock after, the picture
  // starts 0.4 s after the landing, and the first report, 0.8 s after the
  // landing, makes the clock stand until the picture has caught up. With a
  // landing at 0.7 s the first frame comes 1.1 s after the click, past
  // quiet. The playhead took the clock then, a third of a second ahead of
  // the picture, and stood until the picture caught up with it. The
  // harness shows this with ?webkitclock&fps=25&slowseek=700.
  function seekWhilePlaying(land: number, seconds = 3) {
    const out: { now: number; at: number; picture: boolean }[] = [];
    const to = 80;
    const old = { media: 64.48, shown: 99.99 };
    let at = to;
    let said = -Infinity;
    for (let i = 0; i <= seconds * 60; i++) {
      const now = 100 + i / 60;
      const t = now - 100;
      const seeking = t < land;
      const moving = t >= land + 0.4;
      const picture = moving ? to + (t - land - 0.4) : to;
      let clock = to;
      if (!seeking) {
        const estimate = t < land + 0.8 ? to + (t - land) : picture;
        said = Math.max(said, estimate);
        clock = said;
      }
      const media = Math.floor(picture / frame + 1e-6) * frame;
      const presented = moving ? { media, shown: 100 + land + 0.4 + (media - to) } : old;
      at = playingAt(at, { ...base, clock, seeking, presented, now, since: 100 });
      out.push({ now, at, picture: moving });
    }
    return out;
  }

  test("a seek while it plays stands until the picture moves, then moves on every frame", () => {
    for (const land of [0.1, 0.7, 1.5]) {
      const run = seekWhilePlaying(land);
      const first = run.findIndex((r) => r.picture);
      // Not before the picture has started.
      for (let i = 0; i < first; i++) expect(run[i].at).toBe(80);
      // And from then on it stands for no frame, and never goes back.
      expect(longestStill(run, first)).toBe(0);
      for (let i = 1; i < run.length; i++) expect(run[i].at).toBeGreaterThanOrEqual(run[i - 1].at);
    }
  });
});

// A jump over a cut is made by the frame loop, and a pause stops the
// loop. A pause pressed while the video was on its way left the jump
// standing until the next play, and every seek made while paused then
// had a still from the engine drawn over a video that had landed. The
// harness shows this with ?slowseek=150, see frontend/preview/open.mjs,
// because Chromium lands a seek in its small file inside one frame.
describe("jumpStep", () => {
  const playing = { seeking: false, paused: false };

  test("is nothing when no jump was made", () => {
    expect(jumpStep(false, playing)).toBe("none");
    expect(jumpStep(false, { seeking: true, paused: true })).toBe("none");
  });

  test("waits while the playing video is on its way", () => {
    expect(jumpStep(true, { seeking: true, paused: false })).toBe("wait");
  });

  test("has landed once the video is no longer on its way", () => {
    expect(jumpStep(true, playing)).toBe("landed");
    // A pause after the video landed changes nothing: the playhead
    // still goes where it landed.
    expect(jumpStep(true, { seeking: false, paused: true })).toBe("landed");
  });

  test("ends where it stands when the play is paused on the way", () => {
    expect(jumpStep(true, { seeking: true, paused: true })).toBe("paused");
  });

  // The frame loop over a jump, frame by frame, the way Player.svelte
  // runs it: a frame that waits asks for the next one, any other ends
  // the jump. However the pause falls, nothing is left jumping.
  test("never leaves a jump standing once the loop stops", () => {
    const frames = [
      { seeking: true, paused: false },
      { seeking: true, paused: false },
      { seeking: true, paused: true },
    ];
    let jumping = true;
    let running = true;
    for (const f of frames) {
      if (!running) break;
      const step = jumpStep(jumping, f);
      if (step === "wait") continue;
      jumping = false;
      running = !f.paused;
    }
    expect(jumping).toBe(false);
  });
});

describe("playingPiece", () => {
  const three = [
    { start: 10, end: 14 },
    { start: 16, end: 20 },
    { start: 22, end: 26 },
  ];

  test("stays on the piece it is on", () => {
    expect(playingPiece(three, 0, 12)).toBe(0);
    expect(playingPiece(three, 1, 18)).toBe(1);
    expect(playingPiece(three, 2, 24)).toBe(2);
  });

  // A cut put back while the clip plays its last piece leaves the player
  // holding a number past the end. Before this it read undefined and threw
  // inside the frame loop, which stopped the loop for good.
  test("comes back inside when the pieces it was in are gone", () => {
    const two = three.slice(0, 2);
    expect(playingPiece(two, 2, 18)).toBe(1);
    expect(playingPiece([three[0]], 2, 12)).toBe(0);
    expect(playingPiece([three[0]], 1, 12)).toBe(0);
  });

  test("never answers with a piece that is not there", () => {
    for (const count of [1, 2, 3]) {
      const pieces = three.slice(0, count);
      for (const was of [-1, 0, 1, 2, 5]) {
        for (const at of [0, 12, 15, 18, 21, 24, 99]) {
          const got = playingPiece(pieces, was, at);
          expect(got).toBeGreaterThanOrEqual(0);
          expect(got).toBeLessThan(pieces.length);
        }
      }
    }
  });

  test("answers for a clip with no pieces at all", () => {
    expect(playingPiece([], 2, 12)).toBe(0);
  });

  // What the player used to do, and what it does now, side by side. The
  // throw is the bug: it happened inside the frame loop, so the loop ended
  // and the picture stood still on whatever frame it had.
  test("the old way threw where this one does not", () => {
    const two = three.slice(0, 2);
    expect(() => two[2].end).toThrow();
    expect(() => two[playingPiece(two, 2, 18)].end).not.toThrow();
  });
});

describe("pieceAt", () => {
  const two = [
    { start: 10, end: 14 },
    { start: 16, end: 20 },
  ];

  test("gives the piece a time is in", () => {
    expect(pieceAt(two, 11)).toBe(0);
    expect(pieceAt(two, 17)).toBe(1);
  });

  test("gives the piece a time runs into next", () => {
    expect(pieceAt(two, 15)).toBe(1);
    expect(pieceAt(two, 0)).toBe(0);
  });

  test("gives the last piece past the end, and zero with no pieces", () => {
    expect(pieceAt(two, 99)).toBe(1);
    expect(pieceAt([], 99)).toBe(0);
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

describe("shouldChase", () => {
  const paused = { wanted: 100, at: 90, playing: false, tries: 0 };

  test("a seek that never landed is made again", () => {
    expect(shouldChase(paused)).toBe(true);
  });

  test("a seek that landed is not", () => {
    expect(shouldChase({ ...paused, at: 100 })).toBe(false);
    expect(shouldChase({ ...paused, at: 99.7 })).toBe(false);
  });

  test("nothing is chased when nothing was sent", () => {
    expect(shouldChase({ ...paused, wanted: -1 })).toBe(false);
  });

  test("it gives up rather than reading the file over and over", () => {
    expect(shouldChase({ ...paused, tries: 3 })).toBe(false);
  });

  // The one that turned a play into nothing. Playing seeks first, so every
  // play arms a chase, and a seek that changes nothing answers with
  // nothing, so the chase is left running. A second and a bit later the
  // playing clock is a second and a bit further on, which reads exactly
  // like a seek that never landed: the picture was pulled back to where
  // playing began, and on the try after that the file was read again,
  // which empties the element and stops it dead.
  test("a playing picture is never chased", () => {
    expect(shouldChase({ wanted: 100, at: 101.2, playing: true, tries: 0 })).toBe(false);
    expect(shouldChase({ wanted: 100, at: 90, playing: true, tries: 0 })).toBe(false);
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

// The still read while the video preview catches up is the frame the video
// will show there: the one the moment falls in. It was the nearest whole
// second, which from half past on was the next second's frame.
describe("frameStart", () => {
  it("is the frame a moment falls in, never the next one", () => {
    expect(frameStart(12.7, 1)).toBe(12);
    expect(frameStart(12.2, 1)).toBe(12);
    expect(frameStart(0.99, 10)).toBeCloseTo(0.9, 9);
    expect(frameStart(1.0, 25)).toBeCloseTo(1.0, 9);
  });

  it("puts every moment of a frame on the same start", () => {
    const fps = 30000 / 1001;
    for (let n = 0; n < 2000; n += 37) {
      const start = n / fps;
      for (const into of [0, 0.25, 0.5, 0.75, 0.999]) {
        expect(frameStart(start + into / fps, fps)).toBeCloseTo(start, 9);
      }
      // And a start asked for again is the same start, which is what the
      // engine is handed.
      expect(frameStart(frameStart(start, fps), fps)).toBeCloseTo(start, 9);
    }
  });

  it("counts in seconds when the rate is not known", () => {
    expect(frameStart(3.4, 0)).toBe(3);
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

describe("stillFits", () => {
  test("paused, only the still of the frame the playhead is in", () => {
    expect(stillFits(902, 902.01, 30, false)).toBe(true);
    expect(stillFits(902, 902.1, 30, false)).toBe(false);
  });

  // Safari plays on before it shows the frame it plays from. The still of
  // where the play began stays over it for half a second.
  test("playing, the still of where it began stands for half a second", () => {
    expect(stillFits(902, 902.1, 30, true)).toBe(true);
    expect(stillFits(902, 902.5, 30, true)).toBe(true);
    expect(stillFits(902, 902.6, 30, true)).toBe(false);
    // Never a still of a frame ahead of the playhead.
    expect(stillFits(902.5, 902.1, 30, true)).toBe(false);
  });
});
