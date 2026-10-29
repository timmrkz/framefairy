import { afterAll, afterEach, beforeAll, describe, expect, test, vi } from "vitest";
import { movieOf } from "./movie";
import { FrameQueue, GoSound, GoSoundStream, SOUND_CHUNK, soundChunks, type Shown } from "./queue";

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
    q.seek(58.2, [clip, false]);
    said.length = 0;
    q.setProgram([clip[0]], false);
    q.setProgram([{ start: clip[0].start, end: clip[0].end - 0.5 }], true);
    expect(said).toEqual([]);
    expect(wanted(q)).toBe(JSON.stringify([[{ start: clip[0].start, end: clip[0].end - 0.5 }], true]));
  });

  test("are what the next play plays", () => {
    const { q, said } = playing(58.2);
    q.seek(58.2, [clip, false]);
    q.setProgram([clip[0]], false);
    said.length = 0;
    q.seek(58.5);
    expect(said.map((s) => s.at)).toEqual([58.5]);
    expect(wanted(q)).toBe(JSON.stringify([[clip[0]], false]));
  });

  // Tim's undo: the end pulled in while the clip played, then put back.
  // The play plays the clip as it is again, to its old end.
  test("and put back by an undo, play as they were", () => {
    const { q, said } = playing(58.2);
    q.seek(58.2, [clip, false]);
    said.length = 0;
    q.setProgram([clip[0]], false);
    q.setProgram(clip, false);
    expect(said).toEqual([]);
    expect(wanted(q)).toBe(JSON.stringify([clip, false]));
  });
});

describe("soundChunks", () => {
  // What the Go side sends for a pull of a sound stream: chunks of
  // SOUND_CHUNK moments, each after the moment it starts at, the last of a
  // stream shorter.
  const chunk = (at: number, values: number[]) => {
    const b = new Uint8Array(8 + values.length * 4);
    const v = new DataView(b.buffer);
    v.setFloat64(0, at, true);
    values.forEach((x, i) => v.setFloat32(8 + i * 4, x, true));
    return b;
  };
  const join = (...parts: Uint8Array[]) => {
    const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
    let at = 0;
    for (const p of parts) {
      out.set(p, at);
      at += p.length;
    }
    return out;
  };
  test("takes the samples of whole chunks and of a short last one, without the moments", () => {
    const first = Array.from({ length: SOUND_CHUNK * 2 }, (_, i) => i / 4096);
    const last = [0.5, -0.5, 0.25, -0.25];
    const got = soundChunks(join(chunk(1, first), chunk(1 + SOUND_CHUNK / 48000, last)), 2);
    expect(got.length).toBe(first.length + last.length);
    expect(Array.from(got.subarray(0, 3))).toEqual(first.slice(0, 3).map((x) => Math.fround(x)));
    expect(Array.from(got.subarray(first.length))).toEqual(last);
  });
  test("an answer with no chunk holds no samples", () => {
    expect(soundChunks(new Uint8Array(0), 2).length).toBe(0);
  });
});

describe("GoSoundStream", () => {
  // The Go side stood in for: a stream of moments numbered from where it
  // was opened, one chunk a read, in one channel, so a moment says where
  // it is. It closes the stream after the first read, the way it closes
  // one nobody has read from for 20 seconds.
  test("a stream the Go side closed is opened again where it got to, with nothing lost or twice", async () => {
    const opened: number[] = [];
    const streams = new Map<string, { at: number; reads: number }>();
    const was = globalThis.fetch;
    globalThis.fetch = (async (input: string) => {
      const url = new URL(input, "http://x");
      if (url.pathname === "/frames/sound") {
        const at = Math.round(Number(url.searchParams.get("from")) * 48000);
        opened.push(at);
        const id = String(opened.length);
        streams.set(id, { at, reads: 0 });
        return new Response(JSON.stringify({ id }));
      }
      const s = streams.get(url.searchParams.get("id") ?? "");
      if (url.pathname === "/frames/read") {
        if (!s || s.reads === 1) return new Response("gone", { status: 404 });
        s.reads++;
        const b = new Uint8Array(8 + SOUND_CHUNK * 4);
        const v = new DataView(b.buffer);
        for (let i = 0; i < SOUND_CHUNK; i++) v.setFloat32(8 + i * 4, s.at + i, true);
        s.at += SOUND_CHUNK;
        return new Response(b);
      }
      return new Response(null, { status: 204 });
    }) as typeof fetch;
    try {
      const stream = new GoSoundStream("/ep.mp4", 4800, 48000, 1);
      const first = await stream.take(1000);
      const second = await stream.take(1000);
      expect(opened).toEqual([4800, 4800 + SOUND_CHUNK]);
      expect(first[0]).toBe(4800);
      expect(Array.from(second.subarray(0, 3))).toEqual([5800, 5801, 5802]);
      expect(second[999]).toBe(6799);
    } finally {
      globalThis.fetch = was;
    }
  });
});

describe("GoSound", () => {
  test("a stream that fails is dropped, and the next packet is read from a stream of its own", async () => {
    // The Go side stood in for: moments numbered from where a stream was
    // opened, one channel, and the first stream fails on every read.
    const opened: number[] = [];
    const was = globalThis.fetch;
    globalThis.fetch = (async (input: string) => {
      const url = new URL(input, "http://x");
      if (url.pathname === "/frames/sound") {
        opened.push(Math.round(Number(url.searchParams.get("from")) * 48000));
        return new Response(JSON.stringify({ id: String(opened.length) }));
      }
      if (url.pathname === "/frames/read") {
        const id = Number(url.searchParams.get("id"));
        if (id === 1) return new Response("broken", { status: 500 });
        const b = new Uint8Array(8 + SOUND_CHUNK * 4);
        const v = new DataView(b.buffer);
        for (let i = 0; i < SOUND_CHUNK; i++) v.setFloat32(8 + i * 4, opened[id - 1] + i, true);
        return new Response(b);
      }
      return new Response(null, { status: 204 });
    }) as typeof fetch;
    try {
      const out: { timestamp: number; first: number }[] = [];
      const errors: string[] = [];
      const sound = new GoSound(
        "/ep.mp4",
        48000,
        1,
        (s) => {
          const one = new Float32Array(s.numberOfFrames);
          s.copyTo(one, { planeIndex: 0, format: "f32-planar" });
          out.push({ timestamp: s.timestamp, first: one[0] });
        },
        (e) => errors.push(e),
        () => {},
      );
      sound.decode(1, new Uint8Array(0), 0, 960);
      sound.decode(2, new Uint8Array(0), 960, 960);
      sound.decode(3, new Uint8Array(0), 1920, 960);
      await sound.flush();
      expect(errors).toEqual(["the sound stopped, it answered 500"]);
      expect(opened).toEqual([0, 960]);
      expect(out).toEqual([
        { timestamp: 2, first: 960 },
        { timestamp: 3, first: 1920 },
      ]);
    } finally {
      globalThis.fetch = was;
    }
  });
});

// Found by Tim testing #385: a video gone back to from the sidebar went
// black for a frame before its picture came back. Its workspace was woken
// with its own frame on the canvas, and the new queue cleared it, setting
// the canvas to the size it had, and drew it again over nothing when its
// captions came.
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
    inside.video = { width: 1920, height: 1080, samples: { pts: [0], order: [0], timescale: 25 } };
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
  afterEach(() => {
    vi.useRealTimers();
  });

  // A paused queue with a video, whose frame a gesture put the playhead on
  // waits for what lies over it, see dress. The captions come when the
  // test says, or never, and the wait ends when the test says, or after
  // the 600 ms of the browser's clock, which the test moves.
  function waiting() {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const inside = q as unknown as { video: unknown; dress(rank: number): Promise<boolean> | null };
    inside.video = { width: 2, height: 2, samples: { pts: [0, 1, 2], order: [0, 1, 2], timescale: 25 } };
    let came = () => {};
    q.over = {
      at: () => null,
      coming() {},
      ready: () => new Promise<void>((done) => (came = done)),
    };
    const said: string[] = [];
    q.trace = (line) => said.push(line);
    return { q, dress: () => inside.dress(1)!, came: () => came(), said };
  }

  // A clip clicked whose captions never come still shows its picture: Tim
  // saw a picture stand still while its captions were slow, issue #383,
  // and a picture that stands still is worse than captions a moment late.
  test("is put up without them once its wait runs out", async () => {
    const { q, dress, said } = waiting();
    let late = () => {};
    q.dressWait = (l) => {
      late = l;
      return () => {};
    };
    const wanted = dress();
    expect(q.stats.waitedForDress).toBe(1);
    late();
    expect(await wanted).toBe(true);
    expect(q.stats.bare).toBe(1);
    expect(said.some((l) => l.includes("did not come"))).toBe(true);
  });

  test("is put up with them when they come first, and its wait is called off", async () => {
    const { q, dress, came } = waiting();
    let calledOff = 0;
    q.dressWait = () => () => calledOff++;
    const wanted = dress();
    came();
    expect(await wanted).toBe(true);
    expect(q.stats.bare).toBe(0);
    expect(calledOff).toBe(1);
  });

  test("waits 600 ms of the browser's clock unless a wait is handed in", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const { q, dress } = waiting();
    const wanted = dress();
    let ended = false;
    void wanted.then(() => (ended = true));
    await vi.advanceTimersByTimeAsync(599);
    expect(ended).toBe(false);
    expect(q.stats.bare).toBe(0);
    await vi.advanceTimersByTimeAsync(1);
    expect(ended).toBe(true);
    expect(q.stats.bare).toBe(1);
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

describe("a frame decoded for a play", () => {
  // A play asks for what lies over each frame as it comes out of the
  // decoder, before its turn on screen, so its captions are on their way
  // while the frames before it are shown, issue #288. The order is read,
  // not the time it takes, so no machine is too busy for it, issue #338.
  test("asks for what lies over it before it is shown", () => {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const inside = q as unknown as { video: unknown; frameOut(slot: unknown, frame: unknown): void };
    inside.video = { width: 2, height: 2, samples: { pts: [0, 1, 2], order: [0, 1, 2], timescale: 25 } };
    const asked: number[] = [];
    const drawn: number[] = [];
    q.over = {
      at: (frame) => {
        drawn.push(frame);
        return null;
      },
      coming: (frame) => asked.push(frame),
    };
    const slot = { fed: new Map([[80000, 2]]), run: { id: 1, lastRank: 2 }, firstRank: 0, coming: 1, keeper: { offer: () => true }, ready: [] as unknown[] };
    inside.frameOut(slot, { timestamp: 80000, close() {} });
    expect(slot.ready).toHaveLength(1);
    expect(asked).toEqual([2 / 25]);
    expect(drawn).toEqual([]);
  });
});

describe("the walk's clock", () => {
  // A sound card that only says whether it runs, and counts what it is
  // told.
  function clocked() {
    const q = new FrameQueue(
      canvas(),
      "/episode.mp4",
      movieOf({ duration: 120, width: 2, height: 2, grid: { origin: 0, start: 0, num: 25, den: 1, count: 3000 } }),
    );
    const card = { state: "running" as AudioContextState, held: 0, started: 0 };
    q.audio = {
      get state() {
        return card.state;
      },
      suspend: async () => {
        card.held++;
        card.state = "suspended";
      },
      resume: async () => {
        card.started++;
        card.state = "running";
      },
    } as unknown as AudioContext;
    (q as unknown as { state: string }).state = "playing";
    return { q, card };
  }
  afterEach(() => {
    delete g.__playClock;
  });

  test("is the sound card's when no walk asks for it", () => {
    const { q, card } = clocked();
    expect(q.playClock(3.2, 3.04)).toBe(3.2);
    expect(card.held).toBe(0);
  });

  // Issue #588: standing on the frame alone left the sound running on
  // ahead of the picture.
  test("stands on a frame not up yet, and holds the sound card until it is", () => {
    g.__playClock = "frames";
    const { q, card } = clocked();
    expect(q.playClock(3.2, 3.04)).toBe(3.04);
    expect(q.playClock(3.21, 3.04)).toBe(3.04);
    expect(card).toMatchObject({ state: "suspended", held: 1, started: 0 });
    expect(q.playClock(3.21, 3.24)).toBe(3.21);
    expect(card).toMatchObject({ state: "running", held: 1, started: 1 });
    expect(q.playClock(3.22, 3.24)).toBe(3.22);
    expect(card.started).toBe(1);
  });

  // The app holds the sound card itself when it pauses, and the clock
  // lets go only of a hold it made.
  test("leaves a sound card held by someone else held", () => {
    g.__playClock = "frames";
    const { q, card } = clocked();
    card.state = "suspended";
    expect(q.playClock(3.2, 3.24)).toBe(3.2);
    expect(card).toMatchObject({ state: "suspended", held: 0, started: 0 });
  });

  // A walk holds the sound card while its hand drags, issue #583. The
  // review of #588 found the clock let go of the hand's hold when the
  // frame it waited for came up during the drag.
  test("lets the sound card go only once neither the frame nor the hand holds it", async () => {
    g.__playClock = "frames";
    const { q, card } = clocked();
    expect(q.playClock(3.2, 3.04)).toBe(3.04);
    await q.holdSound("hand", true);
    expect(q.playClock(3.2, 3.24)).toBe(3.2);
    expect(card).toMatchObject({ state: "suspended", started: 0 });
    await q.holdSound("hand", false);
    expect(card).toMatchObject({ state: "running", started: 1 });

    await q.holdSound("hand", true);
    expect(q.playClock(3.3, 3.28)).toBe(3.28);
    expect(q.playClock(3.3, 3.32)).toBe(3.3);
    expect(card).toMatchObject({ state: "suspended", started: 1 });
    await q.holdSound("hand", false);
    expect(card).toMatchObject({ state: "running", started: 2 });
  });

  // A play started again starts the sound card itself, see maybeBegin,
  // while the clock may still count itself a holder. The next frame not
  // up holds it again.
  test("holds the sound card again once the app has started it", () => {
    g.__playClock = "frames";
    const { q, card } = clocked();
    q.playClock(3.2, 3.04);
    card.state = "running";
    expect(q.playClock(3.3, 3.04)).toBe(3.04);
    expect(card).toMatchObject({ state: "suspended", held: 2 });
  });
});
