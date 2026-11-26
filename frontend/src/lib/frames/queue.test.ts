import { afterAll, afterEach, beforeAll, describe, expect, test, vi } from "vitest";
import { movieOf } from "./movie";
import { GoPlay, type Taken } from "./play";
import { FrameQueue, type Shown } from "./queue";

// The queue without a browser: a canvas that draws nothing, no sound card
// and no decoders, so the queue never opens and every play stays starting. That is
// enough for what the queue says while a play starts, which is all a click
// while playing sees in its own frame.
function canvas(): HTMLCanvasElement {
  const ctx = { fillStyle: "", fillRect() {}, drawImage() {} };
  return { width: 0, height: 0, getContext: () => ctx } as unknown as HTMLCanvasElement;
}

const g = globalThis as Record<string, unknown>;
const had = g.cancelAnimationFrame;
beforeAll(() => {
  g.cancelAnimationFrame = () => {};
});
afterAll(() => {
  g.cancelAnimationFrame = had;
});

function playing(at: number) {
  const q = new FrameQueue(
    canvas(),
    "/episode.mp4",
    movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
  );
  q.seek(at);
  q.play();
  const said: Shown[] = [];
  q.listen((s) => said.push(s));
  return { q, said };
}

const clip = [
  { start: 57, end: 59.53 },
  { start: 60.31, end: 62.17 },
];

describe("a seek while playing", () => {
  // A click across the edge of the clip changes the moment and the
  // program at once. Told the program first, the queue started the play
  // again from where it was and said so, and that old moment became the
  // playhead the seek was then sent to.
  test("onto another program says only the moment it was sent to", () => {
    const { q, said } = playing(58.2);
    q.seek(55.5, [null, false]);
    expect(said.map((s) => s.at)).toEqual([55.5]);
    expect(said.every((s) => s.playing)).toBe(true);
    said.length = 0;
    q.seek(61, [clip, false]);
    expect(said.map((s) => s.at)).toEqual([61]);
  });

  test("onto the same program says only the moment it was sent to", () => {
    const { q, said } = playing(58.2);
    q.seek(58.9, [null, false]);
    said.length = 0;
    q.seek(66.4, [null, false]);
    expect(said.map((s) => s.at)).toEqual([66.4]);
  });

  // A click seeks as the hand goes down and again as it comes up. The
  // second is the play already starting there, and starting it over would
  // throw away what was decoded for it.
  test("to the moment a play is starting from changes nothing", () => {
    const { q, said } = playing(58.2);
    q.seek(61, [clip, false]);
    said.length = 0;
    q.seek(61, [clip, false]);
    q.seek(61);
    expect(said).toEqual([]);
  });

  test("to the same moment on another program starts it over", () => {
    const { q, said } = playing(58.2);
    q.seek(58.4, [clip, false]);
    said.length = 0;
    q.seek(58.4, [null, false]);
    expect(said.map((s) => s.at)).toEqual([58.4]);
  });
});

// L, or the loop button, while a clip plays. The loop says nothing until
// the end of the clip, and the play started again for it: the picture and
// the sound stopped for a moment every time it was switched.
describe("the loop switched while playing", () => {
  test("goes on as it was, either way", () => {
    const { q, said } = playing(58.2);
    q.seek(58.2, [clip, false]);
    said.length = 0;
    q.setProgram(clip, true);
    q.setProgram(clip, false);
    expect(said).toEqual([]);
  });

});

// A clip edge or a cut dragged while the clip plays, or put back by an
// undo. The play takes in the clip as it is now and goes on, rule 6 of
// docs/VIDEO-PREVIEW.md. It was started again at every step of the hand,
// and Tim saw the picture, the sound and the playhead stutter. Then it kept
// the clip it started with, and an undo while it played was not heard.
describe("the pieces changed while playing", () => {
  const wanted = (q: FrameQueue) => (q as unknown as { wantedKey: string }).wantedKey;

  test("are taken in without starting the play again", () => {
    const { q, said } = playing(58.2);
    q.seek(58.2, [clip, false, "a"]);
    said.length = 0;
    q.setProgram([clip[0]], false, "b");
    q.setProgram([{ start: clip[0].start, end: clip[0].end - 0.5 }], true, "c");
    expect(said).toEqual([]);
    expect(wanted(q)).toBe(JSON.stringify([[{ start: clip[0].start, end: clip[0].end - 0.5 }], true]));
  });

  test("are what the next play plays", () => {
    const { q, said } = playing(58.2);
    q.seek(58.2, [clip, false, "a"]);
    q.setProgram([clip[0]], false, "b");
    said.length = 0;
    q.seek(58.5);
    expect(said.map((s) => s.at)).toEqual([58.5]);
    expect(wanted(q)).toBe(JSON.stringify([[clip[0]], false]));
  });

  // Tim's undo: the end pulled in while the clip played, then put back.
  // The play plays the clip as it is again, to its old end.
  test("and put back by an undo, play as they were", () => {
    const { q, said } = playing(58.2);
    q.seek(58.2, [clip, false, "a"]);
    said.length = 0;
    q.setProgram([clip[0]], false, "b");
    q.setProgram(clip, false, "a");
    expect(said).toEqual([]);
    expect(wanted(q)).toBe(JSON.stringify([clip, false]));
  });
});

// Found by Tim testing #385: a video gone back to from the sidebar went
// black for a frame before its picture came back. Its workspace was woken
// with its own frame on the canvas, and the new queue cleared it, setting
// the canvas to the size it had, and drew it again over nothing when its
// captions came.
describe("a play whose loop was switched off as it went round", () => {
  // L pressed again just before the end: the Go side says the play ends at
  // the end of this time through, but the clock may have gone round by
  // then, with the first frame of the next time through on screen. The
  // playhead read the clip's start, 0.000 s, in a walk in CI.
  test("ends at the end of its last piece", () => {
    vi.stubGlobal("window", { setTimeout: () => 0 });
    try {
      const q = new FrameQueue(
        canvas(),
        "/episode.mp4",
        movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
      );
      const inside = q as unknown as {
        goPlay: unknown;
        due: unknown;
        at: number;
        state: string;
        wanted: { pieces: { start: number; end: number }[]; loop: boolean };
        end(): void;
      };
      inside.wanted = { pieces: [{ start: 0, end: 24.8 }], loop: false };
      inside.state = "playing";
      inside.goPlay = { shots: [], end: 24.8, picturesEnded: true };
      inside.due = { at: 24.8, moment: 0, begins: 0, edge: true };
      inside.at = 0;
      inside.end();
      expect(inside.at).toBeCloseTo(24.8);
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

describe("a workspace woken again", () => {
  function woken() {
    let writes = 0;
    let width = 640;
    let height = 360;
    const c = {
      getContext: () => null,
      get width() {
        return width;
      },
      set width(w: number) {
        writes++;
        width = w;
      },
      get height() {
        return height;
      },
      set height(h: number) {
        writes++;
        height = h;
      },
    } as unknown as HTMLCanvasElement;
    const q = new FrameQueue(
      c,
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
      true,
    );
    const drawn: unknown[] = [];
    (q as unknown as { screen: unknown }).screen = { draw: (f: unknown) => drawn.push(f) };
    return { q, drawn, writes: () => writes };
  }

  test("keeps its frame on a canvas of the size it had", () => {
    const { q, drawn, writes } = woken();
    q.resize(640, 360);
    expect(writes()).toBe(0);
    expect(drawn).toEqual([]);
  });

  test("draws nothing again before it has a frame of its own", () => {
    const { q, drawn } = woken();
    q.repaint();
    expect(drawn).toEqual([]);
  });

  test("still takes a new size, which the canvas had to give up its frame for", () => {
    const { q, writes } = woken();
    q.resize(800, 450);
    expect(writes()).toBe(2);
  });
});

describe("the picture standing still", () => {
  // A paused queue with a frame of 640 by 360 up, from a file of 1920 by
  // 1080, on a canvas of 640 by 360: the frame is as sharp as the canvas
  // lets it be. The queue has no decoders here, so a frame asked for
  // never comes, and the test says how each ask went with report, the way
  // a frame that comes does.
  function standing() {
    let width = 640;
    let height = 360;
    const c = {
      getContext: () => null,
      get width() {
        return width;
      },
      set width(w: number) {
        width = w;
      },
      get height() {
        return height;
      },
      set height(h: number) {
        height = h;
      },
    } as unknown as HTMLCanvasElement;
    const q = new FrameQueue(
      c,
      "/episode.mp4",
      movieOf({ duration: 120, width: 1920, height: 1080, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const inside = q as unknown as {
      video: unknown;
      shown: unknown;
      report(drew?: boolean, passing?: boolean): void;
    };
    inside.video = { width: 1920, height: 1080, samples: { count: 1, timescale: 25, start: 0, step: 1 } };
    const frame = (width: number, height: number) => ({ frame: { width, height, close() {} }, run: -1, rank: 0 });
    inside.shown = frame(640, 360);
    return { q, inside, frame };
  }

  test("stands once the newest seek has said how it went, not while a frame passes on the way", () => {
    const { q, inside } = standing();
    expect(q.standing).toBe(false);
    inside.report();
    expect(q.standing).toBe(true);
    q.seek(1);
    expect(q.standing).toBe(false);
    inside.report(true, true);
    expect(q.standing).toBe(false);
    inside.report(true);
    expect(q.standing).toBe(true);
  });

  test("waits for the first frame of a queue just opened, and stands with none when the seek put none up", () => {
    const { q, inside } = standing();
    inside.shown = null;
    expect(q.standing).toBe(false);
    q.seek(0);
    expect(q.standing).toBe(false);
    inside.report();
    expect(q.standing).toBe(true);
  });

  test("the walks hear the newest queue still open, not one made after it and closed", () => {
    const on = standing().q;
    const slept = standing().q;
    const walks = globalThis as unknown as { __frameQueue?: unknown };
    expect(walks.__frameQueue).toBe(slept);
    slept.close();
    expect(walks.__frameQueue).toBe(on);
    on.close();
  });

  test("waits for the frame asked for again at the canvas's new size", () => {
    const { q, inside, frame } = standing();
    inside.report();
    q.resize(1280, 720);
    expect(q.standing).toBe(false);
    inside.shown = frame(1280, 720);
    inside.report(true);
    expect(q.standing).toBe(true);
  });

  test("asks for the frame at the app's new size once a dragged edge has come to rest, not at every size it passed", () => {
    vi.useFakeTimers();
    vi.stubGlobal("window", globalThis);
    try {
      const { q, inside } = standing();
      inside.report();
      q.resize(1280, 720);
      vi.advanceTimersByTime(150);
      const seeks: number[] = [];
      const real = q.seek.bind(q);
      q.seek = (at: number) => {
        seeks.push(at);
        real(at);
      };
      // The first step from rest asks at once, the steps after it do not.
      for (let w = 1290; w <= 1500; w += 10) q.resize(w, (w * 9) / 16);
      expect(seeks.length).toBe(1);
      expect(q.standing).toBe(false);
      vi.advanceTimersByTime(150);
      expect(seeks.length).toBe(2);
      // A single step from rest asks at once, and not again at rest.
      q.resize(1280, 720);
      expect(seeks.length).toBe(3);
      vi.advanceTimersByTime(150);
      expect(seeks.length).toBe(3);
    } finally {
      vi.unstubAllGlobals();
      vi.useRealTimers();
    }
  });

  test("stands with a frame the decoder gave at a size of its own, since it is not asked for again", () => {
    const { q, inside } = standing();
    inside.report();
    q.resize(1280, 720);
    inside.report(false);
    expect(q.standing).toBe(true);
  });

  // A frame at half the canvas's size, never asked for again: only the
  // state of the queue says it stands.
  test("has nothing to wait for while it plays, at the end, or closed", () => {
    for (const how of ["plays", "ended", "closed"]) {
      const { q, inside, frame } = standing();
      inside.shown = frame(320, 180);
      inside.report();
      expect(q.standing, how).toBe(false);
      if (how === "plays") q.play();
      if (how === "ended") (q as unknown as { state: string }).state = "ended";
      if (how === "closed") q.close();
      expect(q.standing, how).toBe(true);
    }
  });
});

describe("a frame waiting for its captions", () => {
  // A paused queue with a video, whose frame a gesture put the playhead on
  // waits until the captions of its clip are known, see captioned. They
  // come when the test says.
  function waiting() {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const inside = q as unknown as { video: unknown; captioned(): Promise<boolean> | null };
    inside.video = { width: 2, height: 2, samples: { count: 3, timescale: 25, start: 0, step: 1 } };
    let came = () => {};
    q.over = { at: () => null, ready: () => new Promise<void>((done) => (came = done)) };
    return { q, captioned: () => inside.captioned()!, came: () => came() };
  }

  // A clip made with Mark In was sought to the mark and then to its first
  // sentence, and its card was clicked while the frame of the sentence
  // waited for the captions the Go side was slow to give. The frame of the
  // mark came meanwhile, a frame passing, and went up first, so the video
  // preview changed twice for one click, the picture of another moment
  // under the clip's captions and then the clip's own frame.
  test("keeps a frame of an older place off the screen until it is up", async () => {
    const { q, captioned, came } = waiting();
    const inside = q as unknown as { shown: { rank: number } | null; passing(rank: number, frame: unknown, seq: number, key?: boolean): void };
    const picture = () => ({ width: 2, height: 2, over: null, close() {}, clone: () => picture() });
    const wanted = captioned();
    expect(q.stats.waitedForDress).toBe(1);
    inside.passing(2, picture(), 1);
    expect(inside.shown).toBe(null);
    came();
    expect(await wanted).toBe(true);
    // Once nothing waits, a frame passing is put up again, the way it is
    // under a hand that drags the playhead.
    inside.passing(2, picture(), 2);
    expect(inside.shown?.rank).toBe(2);
  });
});

// The frame on screen keeps the dress it came with. A crop moved or a
// word corrected dresses it again, and another clip's dress never lands
// on it: the clip's own frame brings that, issue #383.
describe("the frame on screen dressed again", () => {
  function paused() {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const dressed: number[] = [];
    let key = "";
    const app = {
      dressWith: (a: { framing: string } | null) => (key = a?.framing ?? ""),
      get dressKey() {
        return key;
      },
      dress: async (_: unknown, begins: number) => void dressed.push(begins),
    };
    const shown = { frame: { over: null }, rank: 2, begins: 0.08, at: -Infinity };
    // The frame on screen is the one the last still put up.
    Object.assign(q, { app, shown, putFor: 0 });
    const drawn: unknown[] = [];
    q.painted = (frame) => drawn.push(frame);
    return { q, dressed, drawn, shown };
  }
  const ask = (clip: string, framing: string) => ({ path: "/episode.mp4", clip, framing, drafts: {} });

  test("dresses the frame on screen of the same clip again, and draws it once its dress is here", async () => {
    const { q, dressed, drawn, shown } = paused();
    expect(q.dressWith(ask("a", "f1"))).toBeNull();
    const again = q.dressWith(ask("a", "f2"));
    expect(dressed).toEqual([0.08]);
    await again;
    expect(drawn).toEqual([shown.frame]);
    // The same dress again asks for nothing.
    expect(q.dressWith(ask("a", "f2"))).toBeNull();
  });

  test("leaves the frame on screen as it is for another clip", () => {
    const { q, dressed } = paused();
    q.dressWith(ask("a", "f1"));
    expect(q.dressWith(ask("b", "f3"))).toBeNull();
    expect(dressed).toEqual([]);
  });

  test("leaves the frame on screen to a still on its way, which dresses its own as it puts it up", async () => {
    const { q, dressed } = paused();
    Object.assign(q, { ticket: 1 });
    q.dressWith(ask("a", "f1"));
    expect(q.dressWith(ask("a", "f2"))).toBeNull();
    expect(dressed).toEqual([]);
    const still = q as unknown as { dressedAsNow(f: unknown, begins: number, ticket: number): Promise<boolean> };
    expect(await still.dressedAsNow({ over: { dress: { key: "f1" } } }, 0.4, 1)).toBe(true);
    expect(await still.dressedAsNow({ over: { dress: { key: "f2" } } }, 0.48, 1)).toBe(true);
    expect(dressed).toEqual([0.4]);
    // A still a newer seek took the place of is wanted no more.
    expect(await still.dressedAsNow({ over: { dress: { key: "f2" } } }, 0.4, 0)).toBe(false);
  });

  test("dresses again once, with what is wanted last, for changes that come while one is on its way", async () => {
    const { q, dressed } = paused();
    q.dressWith(ask("a", "f1"));
    const first = q.dressWith(ask("a", "f2"));
    q.dressWith(ask("a", "f3"));
    q.dressWith(ask("a", "f4"));
    await first;
    expect(dressed).toEqual([0.08, 0.08]);
  });
});

describe("sound handed to the sound card late", () => {
  // A queue playing on a sound card whose clock the test sets, at 1000
  // frames a second so a frame is a millisecond, with the play's clock
  // started at 1 s. Each stretch is 100 frames, and what the sound card
  // was told is kept.
  function late() {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const started: [number, number][] = [];
    const card = {
      currentTime: 0,
      sampleRate: 1000,
      createBuffer: () => ({ copyToChannel() {} }),
      createBufferSource: () => ({
        connect() {},
        disconnect() {},
        stop() {},
        start: (when: number, skip = 0) => started.push([when, skip]),
      }),
    };
    const inside = q as unknown as { audio: unknown; out: unknown; c0: number; m0: number; schedule(c: unknown): void };
    inside.audio = card;
    inside.out = {};
    inside.c0 = 1;
    inside.m0 = 0;
    // The whole episode straight on, with no end for the sound to stop at.
    Object.defineProperty(q, "program", { get: () => ({ end: Infinity }) });
    const hand = (out: number) => inside.schedule({ out, length: 100, data: [new Float32Array(100)] });
    return { q, card, hand, started };
  }

  // A busy machine kept the page from handing the sound card its sound
  // in time, and the sound card played nothing until it came. A walk
  // tells that silence from a click by where the queue says it is, issue
  // #466.
  test("says where the sound card played nothing", () => {
    const { q, card, hand, started } = late();
    card.currentTime = 1.05;
    hand(100);
    expect(started).toEqual([[1.1, 0]]);
    card.currentTime = 1.23;
    hand(200);
    expect(started.at(-1)).toEqual([1.23, expect.closeTo(0.03, 9)]);
    card.currentTime = 1.45;
    hand(300);
    hand(400);
    expect(started).toHaveLength(3);
    expect(started.at(-1)).toEqual([1.45, expect.closeTo(0.05, 9)]);
    expect(q.stats.lateSound).toBe(3);
    // The stretch that came too late to start and the one after it, which
    // started late, are one quiet stretch.
    expect(q.stats.lateSoundAt).toEqual([
      [1200, 1230],
      [1300, 1450],
    ]);
  });

  // The record that lets a walk set late sound aside also says whether
  // anything else made it late: a stretch thrown away before it played,
  // or the same sound handed over twice, which a press of L must never
  // do. Either would make the sound after it late on any machine.
  test("says when sound was handed over twice", () => {
    const { q, card, hand } = late();
    card.currentTime = 1;
    hand(0);
    hand(100);
    expect(q.stats.soundTwice).toBe(0);
    hand(50);
    expect(q.stats.soundTwice).toBe(1);
  });

  test("says when sound was thrown away before it played", () => {
    const { q, card, hand } = late();
    card.currentTime = 1;
    hand(0);
    hand(100);
    card.currentTime = 1.15;
    (q as unknown as { stopPlay(): void }).stopPlay();
    // The first stretch had played, the second had not.
    expect(q.stats.soundThrown).toBe(1);
  });

  test("keeps the last 1000", () => {
    const { q, card, hand } = late();
    // Each stretch starts half way through, so none follows on another.
    for (let i = 0; i < 1010; i++) {
      card.currentTime = 1 + (i * 100 + 50) / 1000;
      hand(i * 100);
    }
    expect(q.stats.lateSound).toBe(1010);
    expect(q.stats.lateSoundAt).toHaveLength(1000);
    expect(q.stats.lateSoundAt[0]).toEqual([2000, 2050]);
  });
});

describe("the clock a play runs on", () => {
  function queue() {
    return new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
  }

  test("is the sound card's in the app", () => {
    expect(queue().playClock(3.2, 3.04, () => false)).toBe(3.2);
  });

  // The page the walks run on hands in the walk's clock, see
  // preview/clock.ts, and every queue made after it plays on it.
  test("is the one handed in for every queue made after", () => {
    const app = FrameQueue.clock;
    const told: unknown[] = [];
    FrameQueue.clock = (play) => (heard, unshown, ready) => {
      told.push([play, heard, unshown, ready()]);
      return unshown;
    };
    try {
      const q = queue();
      expect(q.playClock(3.2, 3.04, () => true)).toBe(3.04);
      expect(told).toEqual([[q, 3.2, 3.04, true]]);
    } finally {
      FrameQueue.clock = app;
    }
    expect(queue().playClock(3.2, 3.04, () => false)).toBe(3.2);
  });

  // The play as the clock is told it, at 35.4 s on its clock: the frames
  // come from the Go side and not drawn yet, each by where on the clock
  // it is drawn and where it begins in the video, and whether the Go side
  // has sent the last.
  function standing(shots: { at: number; begins: number }[], picturesEnded = false, sound: { handedTo: number; soundEnded: boolean } | null = null) {
    const inside = queue() as unknown as { reached: number; video: unknown; unshown(play: unknown): { at: number; ready: () => boolean } };
    inside.video = { width: 2, height: 2, samples: { count: 3000, timescale: 25, start: 0, step: 1 } };
    inside.reached = 35.4;
    // A play with sound, started at 35 s of the clock on the sound card's
    // 10 s, which has been handed the sound up to handedTo.
    if (sound) Object.assign(inside, { sound: { sampleRate: 48000 }, c0: 10, m0: 35 * 48000, handedTo: sound.handedTo });
    return inside.unshown({ shots, picturesEnded, soundEnded: sound ? sound.soundEnded : true });
  }

  test("is told of the first frame come and not drawn yet", () => {
    const next = standing([
      { at: 35.6, begins: 103 / 25 },
      { at: 35.8, begins: 104 / 25 },
    ]);
    expect(next.at).toBeCloseTo(35.6);
    expect(next.ready()).toBe(true);
  });

  // A frame still on its way holds the walk's clock where it is, so a
  // busy machine shows every frame, late, rather than skipping one.
  test("stands where it is while the next frame is on its way", () => {
    const next = standing([]);
    expect(next.at).toBe(35.4);
    expect(next.ready()).toBe(false);
  });

  // Once the Go side has sent the last frame no frame comes, and stood
  // on, the play froze with the sound card held and the walk hung, as
  // four did once in pull request #589. The clock is told of no frame, so
  // a walk goes by the sound card's time the way the app plays, and its
  // own checks see what is wrong.
  test("is told of no frame once the Go side sent the last", () => {
    expect(standing([], true).at).toBe(Infinity);
  });

  // The walk's clock goes on as fast as the frames come, and the sound
  // comes as it is pulled, on a busy machine later than the frames. A
  // clock that went on to a frame before its sound was on the sound card
  // played silence there, which a walk heard in CI as a play with sound
  // in 288 of 675 quanta. The play started at 35 s of the clock on the
  // sound card's 10 s, and the frame due is drawn at 35.6 s.
  test("is told the frame is ready only once its sound is on the sound card", () => {
    const at = (handedTo: number, soundEnded = false) => standing([{ at: 35.6, begins: 103 / 25 }], false, { handedTo, soundEnded }).ready();
    expect(at(10.5)).toBe(false);
    expect(at(10.6)).toBe(true);
    expect(at(-Infinity, true)).toBe(true);
  });
});

// The Go side closes a play nobody has pulled from for a while: a cue
// left standing while the app stood paused, or a play paused for long.
describe("a play the Go side closed", () => {
  function closed(state: string) {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const inside = q as unknown as { state: string; goPlay: unknown; at: number; startFrom: number; listenTo(play: unknown): void };
    let shut = 0;
    const play = { shots: [] as unknown[], gone: true, onShots: (came: number) => void came, close: () => shut++, more() {} };
    inside.state = state;
    inside.at = 61;
    inside.goPlay = play;
    inside.listenTo(play);
    play.onShots(0);
    return { inside, shut: () => shut };
  }

  // Cued again, the cue was made again every time it was closed, for as
  // long as the app stood paused.
  test("cued is dropped, and the space bar starts a play of its own", () => {
    const { inside, shut } = closed("cued");
    expect(inside.state).toBe("paused");
    expect(inside.goPlay).toBe(null);
    expect(shut()).toBe(1);
  });

  // A stop the person hears, so a walk counts it.
  test("playing starts again from the playhead, and counts as a start", () => {
    const { inside, shut } = closed("playing");
    expect(inside.state).toBe("starting");
    expect(inside.startFrom).toBe(61);
    expect(shut()).toBe(1);
    expect((inside as unknown as { stats: { starts: number } }).stats.starts).toBe(1);
  });
});

// A clip changed while it plays is taken into the play by the Go side,
// which says where. The window opens no second play: what it holds of the
// clip as it was from there on makes way, and the play goes on.
describe("a change of the clip taken into the play under way", () => {
  function underWay(answer: Taken | null) {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    type Inside = {
      goPlay: unknown;
      state: string;
      c0: number | null;
      m0: number;
      soundTo: number;
      audio: { currentTime: number; sampleRate: number };
      stats: { starts: number };
      sources: Map<{ stop(at: number): void }, number>;
      pump(): void;
      start(at: number): void;
      takeIn(play: unknown, framing: string, loop: boolean): Promise<void>;
    };
    const inside = q as unknown as Inside;
    const said: string[] = [];
    const play = {
      take: async (framing: string, loop: boolean, heard: number) => {
        said.push(`take ${framing} ${loop} ${heard.toFixed(2)}`);
        return answer;
      },
    };
    inside.goPlay = play;
    inside.state = "playing";
    inside.c0 = 0;
    inside.m0 = 61 * 48000;
    inside.soundTo = 62 * 48000;
    inside.audio = { currentTime: 0.1, sampleRate: 48000 };
    inside.pump = () => {};
    inside.start = (at) => said.push(`started again at ${at}`);
    const stops: number[] = [];
    inside.sources.set({ stop: (at) => stops.push(at) }, 0.5);
    inside.sources.set({ stop: (at) => stops.push(at) }, 0.2);
    return { q, inside, play, said, stops };
  }

  test("cuts the sound handed over where the Go side took it in, and starts nothing", async () => {
    const { inside, play, said, stops } = underWay({ gen: 1, at: 61.25, end: 64, lap: 64, video: false });
    await inside.takeIn(play, "b", false);
    expect(said).toEqual(["take b false 61.10"]);
    expect(stops).toEqual([0.25]);
    expect(inside.soundTo).toBe(61.25 * 48000);
    expect(inside.stats.starts).toBe(0);
  });

  test("whose place the sound passed while the Go side answered, starts the play again from the playhead", async () => {
    const { inside, play, said } = underWay({ gen: 1, at: 61.05, end: 64, lap: 64, video: false });
    await inside.takeIn(play, "b", false);
    expect(said.length).toBe(2);
    expect(said[1]).toMatch(/^started again at /);
  });

  test("where the playhead was left outside the clip, goes on as a play of the video and says so", async () => {
    const { q, inside, play } = underWay({ gen: 1, at: 61.25, end: 120, lap: 120, video: true });
    let left = 0;
    q.leftClip = () => left++;
    await inside.takeIn(play, "b", false);
    expect(left).toBe(1);
    expect((q as unknown as { wantedKey: string }).wantedKey).toBe(JSON.stringify([null, false]));
  });
});

// A paused play held the Go side's decoder, so nothing was read ahead
// while it stood. Closed and cued again instead, the space bar after a
// pause started a play of its own, 55 ms later than going on.
describe("a play paused", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  test("gives the Go side's decoder back, keeps the rest, and takes the decoder again as it goes on", () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    vi.stubGlobal("window", globalThis);
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const inside = q as unknown as { state: string; goPlay: unknown; at: number; c0: number | null; audio: unknown; out: unknown };
    const said: string[] = [];
    const gain = { value: 1, cancelScheduledValues() {}, setValueAtTime() {}, linearRampToValueAtTime() {} };
    inside.audio = { currentTime: 3, suspend: () => Promise.resolve(), resume: () => Promise.resolve() };
    inside.out = { gain };
    const play = {
      shots: [],
      held: false,
      close: () => said.push("close"),
      more() {},
      hold() {
        play.held = true;
        said.push("hold");
      },
      go() {
        play.held = false;
        said.push("go");
      },
    };
    inside.goPlay = play;
    inside.state = "playing";
    inside.at = 61;
    inside.c0 = 0;
    // The page's frames are not run here: nothing is drawn.
    (inside as unknown as { loop(): void }).loop = () => {};
    q.pause();
    vi.runAllTimers();
    expect(said).toEqual(["hold"]);
    expect(inside.goPlay).toBe(play);
    q.play();
    expect(said).toEqual(["hold", "go"]);
    expect(inside.state).toBe("playing");
  });
});

// A cut made just as the playhead was put down: the play from the
// playhead was cued on the clip as it was, and the clip changed while the
// Go side opened it. The space bar then started the old cue, and the
// video preview showed the frames of the new cut. Seen in a walk in CI.
describe("a cue the clip changed under", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  test("is cued again on the clip as it is now", async () => {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const inside = q as unknown as {
      app: unknown;
      state: string;
      ticket: number;
      goPlay: unknown;
      shown: { begins: number } | null;
      previewSize(): unknown;
      begin(play: unknown): void;
      cue(at: number, ticket: number): Promise<void>;
    };
    const before = [
      { start: 10.6, end: 12 },
      { start: 13, end: 24.8 },
    ];
    const after = [
      { start: 10.6, end: 10.8 },
      { start: 11.8, end: 12 },
      { start: 13, end: 24.8 },
    ];
    inside.app = {};
    inside.previewSize = () => ({ width: 2, height: 2 });
    inside.shown = { begins: 10.76 };
    inside.begin = (play) => (inside.goPlay = play);
    q.setProgram(before, false);
    inside.state = "paused";
    const asked: { pieces: { start: number; end: number }[] }[] = [];
    const closed: number[] = [];
    let answer: () => void = () => {};
    vi.spyOn(GoPlay, "open").mockImplementation(async (_app, ask) => {
      const n = asked.push(ask);
      const play = { ask, first: 10.76, from: 0, end: 14, close: () => closed.push(n) } as unknown as GoPlay;
      // The first is held until the clip has changed.
      if (n === 1) await new Promise<void>((r) => (answer = r));
      return play;
    });
    const cued = inside.cue(10.776, inside.ticket);
    q.setProgram(after, false);
    answer();
    await cued;
    expect(asked.map((a) => a.pieces)).toEqual([before, after]);
    expect(closed).toEqual([1]);
    expect(inside.state).toBe("cued");
    expect((inside.goPlay as { ask: { pieces: unknown } }).ask.pieces).toEqual(after);
  });
});
