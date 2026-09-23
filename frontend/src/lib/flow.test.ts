import { describe, expect, test } from "vitest";
import {
  Heard,
  Newest,
  mergeJob,
  nextWindow,
  pictureIsStale,
  pieceAt,
  insideClip,
  playingPiece,
  shouldChase,
  saidWord,
  inEpisode,
  inClip,
  draftCaptions,
  shouldLook,
  shouldWarm,
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
  test("not before the transcript reaches the end of the window", () => {
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

  test("with a window that is the whole episode", () => {
    expect(shouldLook({ ...idle, to: 14423, covered: 14423 })).toBe(true);
  });

  test("but not while there is no window yet", () => {
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

describe("the model is loaded for the first search before it starts", () => {
  test("while the transcript is on its way to the end of the window", () => {
    expect(shouldWarm({ ...idle, covered: 1200 })).toBe(true);
  });

  test("not once the search itself can start", () => {
    expect(shouldWarm({ ...idle, covered: 1800 })).toBe(false);
  });

  test("not for an episode that has been searched, or is being", () => {
    expect(shouldWarm({ ...idle, covered: 1200, looked: true })).toBe(false);
    expect(shouldWarm({ ...idle, covered: 1200, plans: 1 })).toBe(false);
    expect(shouldWarm({ ...idle, covered: 1200, busy: true })).toBe(false);
  });

  test("not while there is no window yet", () => {
    expect(shouldWarm({ ...idle, to: 0, covered: 0 })).toBe(false);
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

  test("a part a little longer than the half hour is taken whole", () => {
    const w = nextWindow([{ from: 0, to: 40 * 60 }], [], hours, half);
    expect(w).toEqual({ from: 0, to: 40 * 60 });
  });

  test("a part well over the half hour gives up only that much", () => {
    const w = nextWindow([{ from: 0, to: 60 * 60 }], [], hours, half);
    expect(w).toEqual({ from: 0, to: half });
  });

  test("an episode searched end to end rests on the last part", () => {
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

describe("the transcript's edge only ever moves forward", () => {
  // The edge says how much of the episode has been read. Reading does not
  // unhappen, so no ordering of what the Go side reports may walk it back.
  const ep = "/eps/ep.mp4";

  test("it follows the work while the transcription runs", () => {
    const h = new Heard();
    expect(h.seen(ep, 0, 12, false)).toBe(12);
    expect(h.seen(ep, 0, 30, false)).toBe(30);
    // The save lands, far behind the work, and changes nothing.
    expect(h.seen(ep, 24, 42, false)).toBe(42);
  });

  test("pausing does not rewind it", () => {
    // This is the bug as it was seen. The job reported 300 seconds, the
    // transcript on disk had only been written at 240, and the moment the
    // job stopped the live number was gone.
    const h = new Heard();
    expect(h.seen(ep, 240, 300, false)).toBe(300);
    expect(h.seen(ep, 240, null, false)).toBe(300);
  });

  test("carrying on from a pause does not rewind it either", () => {
    // A transcription that carries on starts again from the end of the
    // saved transcript, which is behind where the work had got to. Those
    // seconds were heard, so the edge stands still until the work passes
    // them rather than jumping back and climbing twice.
    const h = new Heard();
    h.seen(ep, 240, 300, false);
    expect(h.seen(ep, 240, 240, false)).toBe(300);
    expect(h.seen(ep, 240, 280, false)).toBe(300);
    expect(h.seen(ep, 240, 310, false)).toBe(310);
  });

  test("an older answer landing after a newer one does not rewind it", () => {
    // Every refresh is a call of its own and nothing says they come back in
    // the order they went out.
    const h = new Heard();
    expect(h.seen(ep, 600, null, false)).toBe(600);
    expect(h.seen(ep, 120, null, false)).toBe(600);
  });

  test("whatever order the two numbers arrive in, the answer is the same", () => {
    // The job and the episode refresh land independently, so this walks
    // every interleaving of a plausible run and insists the result never
    // decreases and always ends at the furthest either of them reached.
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
        const got = h.seen(ep, s, r, false);
        expect(got, `shift ${shift} step ${i}`).toBeGreaterThanOrEqual(last);
        last = got;
      }
      expect(last, `shift ${shift}`).toBe(most);
    }
  });

  test("another episode starts the mark over", () => {
    const h = new Heard();
    h.seen(ep, 0, 900, false);
    expect(h.seen("/eps/zwei.mp4", 0, 5, false)).toBe(5);
    // And going back does not bring the first one's mark with it.
    expect(h.seen(ep, 0, 3, false)).toBe(3);
  });

  test("a transcript being read again starts the mark over", () => {
    // The file changed, so the old mark is about a transcript that no
    // longer exists.
    const h = new Heard();
    h.seen(ep, 600, null, false);
    expect(h.seen(ep, 0, 10, true)).toBe(10);
    expect(h.seen(ep, 0, 20, false)).toBe(20);
  });

  test("a work folder that is gone starts the mark over", () => {
    const h = new Heard();
    h.seen(ep, 600, null, false);
    expect(h.seen(ep, 0, null, true)).toBe(0);
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

describe("saidWord", () => {
  const two = [
    { start: 10, end: 14 },
    { start: 16, end: 20 },
  ];
  // The episode's words, on the episode's clock. The third one is inside
  // the cut, so no caption word can ever point at it.
  const said = [
    { start: 10.5, end: 11, text: "Und" },
    { start: 13, end: 13.6, text: "da" },
    { start: 14.5, end: 15, text: "hm" },
    { start: 16.2, end: 17, text: "war" },
  ];

  test("finds the word a caption word stands for", () => {
    // On the clip's clock the same words are at 0.5, 3 and 4.2.
    expect(saidWord(two, said, { start: 0.5, end: 1 })?.text).toBe("Und");
    expect(saidWord(two, said, { start: 3, end: 3.6 })?.text).toBe("da");
    expect(saidWord(two, said, { start: 4.2, end: 5 })?.text).toBe("war");
  });

  test("finds the word both halves of a split one came from", () => {
    // "war" corrected to "war es" is drawn as two caption words that
    // together span the one word. Either half corrects the whole.
    expect(saidWord(two, said, { start: 4.2, end: 4.6 })?.text).toBe("war");
    expect(saidWord(two, said, { start: 4.6, end: 5 })?.text).toBe("war");
  });

  test("allows itself the same hair the engine does", () => {
    // A middle a hundredth of a second past the end of "da" is still "da".
    expect(saidWord(two, said, { start: 3.61, end: 3.61 })?.text).toBe("da");
    // A tenth past it is nothing.
    expect(saidWord(two, said, { start: 3.7, end: 3.7 })).toBe(null);
  });

  test("points at nothing when nothing is near", () => {
    expect(saidWord(two, [{ start: 100, end: 101, text: "far" }], { start: 0, end: 1 })).toBe(null);
    expect(saidWord(two, [], { start: 0, end: 1 })).toBe(null);
  });

  test("points at nothing with no pieces to read the clock through", () => {
    expect(saidWord([], said, { start: 10.6, end: 10.9 })).toBe(null);
  });
});

// The one assumption the caption box stands on, end to end.
//
// The engine lays a clip's captions out by walking the pieces, putting
// every word of the episode that falls in one onto the clip's own clock,
// and then splitting any word whose correction reads as several words.
// That is ClipWords in engine/lines.go with SplitCorrected after it. The
// caption box clicks its way back along that path, and nothing checks the
// two agree: TypeScript knows the shape of a word and nothing about where
// it came from.
//
// So the path is walked here, forwards the way the engine walks it and
// backwards the way the app does, and every word has to come home. If
// the engine ever lays them out differently this fails, which is the
// point of writing it down.
describe("a caption word finds its way home", () => {
  type Said = { start: number; end: number; text: string };

  // ClipWords: the words of a clip on the clip's own clock.
  function onClipClock(pieces: { start: number; end: number }[], said: Said[]): Said[] {
    const out: Said[] = [];
    let offset = 0;
    for (const p of pieces) {
      for (const w of said) {
        if (w.start < p.start - 0.02 || w.end > p.end + 0.02) continue;
        out.push({
          start: offset + (Math.max(w.start, p.start) - p.start),
          end: offset + (Math.min(w.end, p.end) - p.start),
          text: w.text,
        });
      }
      offset += p.end - p.start;
    }
    return out;
  }

  // SplitCorrected: a correction that reads as several words is drawn as
  // several, each taking its share of the one moment by how long it is.
  function split(words: Said[]): Said[] {
    const out: Said[] = [];
    for (const w of words) {
      const parts = w.text.split(" ").filter(Boolean);
      if (parts.length < 2) {
        out.push(w);
        continue;
      }
      const letters = parts.reduce((n, part) => n + part.length, 0);
      let from = w.start;
      parts.forEach((part, i) => {
        const to =
          i === parts.length - 1 ? w.end : from + ((w.end - w.start) * part.length) / letters;
        out.push({ start: from, end: to, text: part });
        from = to;
      });
    }
    return out;
  }

  const pieces = [
    { start: 57, end: 69 },
    { start: 70, end: 82 },
  ];
  // Words of the episode: two in the first piece, one in the cut between
  // them that no caption can ever show, three in the second. The last one
  // was corrected into three words, which is what makes the middle rather
  // than the edge the thing to match on.
  const said: Said[] = [
    { start: 57.2, end: 57.6, text: "Und" },
    { start: 68.4, end: 68.9, text: "da" },
    { start: 69.2, end: 69.7, text: "hm" },
    { start: 70.1, end: 70.5, text: "war" },
    { start: 80.0, end: 80.4, text: "es" },
    { start: 81.0, end: 81.9, text: "ein echtes Thema" },
  ];

  test("every caption word points back at the word it came from", () => {
    const drawn = split(onClipClock(pieces, said));
    // Five words are drawn from five words heard, with the corrected one
    // standing as three, and the one inside the cut is not drawn at all.
    expect(drawn.map((w) => w.text)).toEqual([
      "Und",
      "da",
      "war",
      "es",
      "ein",
      "echtes",
      "Thema",
    ]);
    const home = drawn.map((w) => saidWord(pieces, said, w)?.text ?? "-");
    expect(home).toEqual(["Und", "da", "war", "es", ...Array(3).fill("ein echtes Thema")]);
  });

  test("and at the moment the engine keeps it at, to the millisecond", () => {
    // The engine finds the word by its start, within a thousandth and a
    // half of a second. Anything that drifts further is a correction that
    // lands on nothing.
    const drawn = split(onClipClock(pieces, said));
    for (const w of drawn) {
      const home = saidWord(pieces, said, w);
      expect(home).not.toBe(null);
      const its = said.find((x) => x.text === home?.text);
      expect(Math.abs((home?.start ?? -1) - (its?.start ?? -2))).toBeLessThan(0.0015);
    }
  });

  test("a word in a cut is never pointed at", () => {
    const drawn = split(onClipClock(pieces, said));
    expect(drawn.some((w) => saidWord(pieces, said, w)?.text === "hm")).toBe(false);
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
