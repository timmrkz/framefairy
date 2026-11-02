import { afterEach, describe, expect, test, vi } from "vitest";
import { AppFrames, AppPictures } from "./app";
import type { VideoTrack } from "./movie";
import { Picture } from "./picture";

// Frames that come at once, each knowing its rank. out(n) settles once
// the decoder has put out n frames, heard from its own output.
function pictures() {
  const shown: number[] = [];
  let waiting: { n: number; done: () => void }[] = [];
  const frames = {
    frame: (rank: number) => Promise.resolve({ rank, close() {} } as unknown as Picture),
  } as unknown as AppFrames;
  const put = (f: unknown) => {
    shown.push((f as { rank: number }).rank);
    const due = waiting.filter((w) => shown.length >= w.n);
    waiting = waiting.filter((w) => shown.length < w.n);
    for (const w of due) w.done();
  };
  const dec = new AppPictures(frames, put, () => {});
  const out = (n: number) =>
    new Promise<number[]>((done) => {
      if (shown.length >= n) return done(shown);
      waiting.push({ n, done: () => done(shown) });
    });
  return { dec, shown, out };
}

describe("AppPictures", () => {
  test("a run fed after a flush starts from its own first frame, lower ranks too", async () => {
    // The clip's last piece, then its start the next time through a loop,
    // on the same decoder. The start was held back for good, waiting for
    // the rank after the last piece's end (2.264).
    const { dec, shown, out } = pictures();
    for (const r of [84, 85, 86]) dec.decode(r, false, new Uint8Array(0), r, true, 84);
    await dec.flush();
    expect(shown).toEqual([84, 85, 86]);
    for (const r of [0, 1, 2]) dec.decode(r, false, new Uint8Array(0), r, true, 0);
    expect(await out(6)).toEqual([84, 85, 86, 0, 1, 2]);
  });

  test("frames come out in the order they are shown, whatever order they are fed in", async () => {
    const { dec, out } = pictures();
    for (const r of [10, 13, 11, 12]) dec.decode(r, false, new Uint8Array(0), r, true, 10);
    expect(await out(4)).toEqual([10, 11, 12, 13]);
  });
});

describe("AppFrames", () => {
  afterEach(() => vi.unstubAllGlobals());

  test("the cards' frames stay kept while a play reads more frames than are kept besides", () => {
    // Nothing on the Go side answers: a frame that is not kept is never
    // handed over.
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    const n = 200;
    const order = Uint32Array.from({ length: n }, (_, i) => i);
    const samples = {
      count: n,
      timescale: 25,
      pts: Float64Array.from(order),
      duration: new Float64Array(n).fill(1),
      key: new Uint8Array(n).fill(1),
      order,
      rank: order,
    };
    const track: VideoTrack = { kind: "video", width: 4, height: 2, samples, frame: 1 / 25 };
    const app = new AppFrames("/v.mp4", track, () => ({ width: 4, height: 2 }), () => {});
    const picture = (rank: number) => new Picture(new Uint8Array(4 * 2 * 4), 4, 2, rank * 40000);
    app.cards([150]);
    app.keep(150, picture(150));
    for (let r = 0; r < 100; r++) app.keep(r, picture(r));
    void app.frame(150, 0);
    expect(app.stats.kept).toBe(1);
    // The first frame of the play was let go of.
    void app.frame(0, 0);
    expect(app.stats.kept).toBe(1);
    app.close();
  });

  // Found by the review of #385: a card's frame was read while a clip
  // played, a second decoder seeking in the file the play decodes.
  test("are read only while the video preview stands still", () => {
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    const { app } = frames(() => ({ width: 4, height: 2 }));
    let paused = false;
    app.paused = () => paused;
    app.cards([150]);
    expect(app.stats.ahead).toBe(0);
    paused = true;
    app.aheadNow();
    expect(app.stats.ahead).toBe(1);
    app.close();
  });

  // Issue #596: soon after a video opens its cards are still being read,
  // and a card's seek stood between a drag and the frame under the hand.
  test("wait while the hand holds the playhead, and are read once it lets go", () => {
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    const { app } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.grip(true);
    app.cards([150]);
    app.aheadNow();
    expect(app.stats.ahead).toBe(0);
    app.grip(false);
    expect(app.stats.ahead).toBe(1);
    app.close();
  });

  // Found by the review of #385: the budget was counted where the cards
  // were given, at the size of that moment, and nowhere when it changed.
  test("keep as many as their budget holds at the size frames are read at", () => {
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    let size = { width: 4, height: 2 };
    const { app, picture } = frames(() => size);
    // Two frames of 4 by 2, or four of 2 by 2.
    app.cardsBytes = 2 * 4 * 2 * 4;
    app.cards([150, 151, 152]);
    for (const r of [150, 151, 152]) app.keep(r, picture(r, 4, 2));
    for (let r = 0; r < 100; r++) app.keep(r, picture(r, 4, 2));
    for (const r of [150, 151, 152]) void app.frame(r, 0);
    expect(app.stats.keptRanks.slice(-app.stats.kept)).toEqual([150, 151]);
    size = { width: 2, height: 2 };
    app.keep(152, picture(152, 2, 2));
    for (let r = 0; r < 100; r++) app.keep(r, picture(r, 2, 2));
    void app.frame(152, 0);
    expect(app.stats.keptRanks.at(-1)).toBe(152);
    app.close();
  });
});

// Frames of a 200 frame track on a Go side that never answers, so a frame
// not kept is never handed over.
function frames(size: () => { width: number; height: number }) {
  const n = 200;
  const order = Uint32Array.from({ length: n }, (_, i) => i);
  const samples = {
    count: n,
    timescale: 25,
    pts: Float64Array.from(order),
    duration: new Float64Array(n).fill(1),
    key: new Uint8Array(n).fill(1),
    order,
    rank: order,
  };
  const track: VideoTrack = { kind: "video", width: 4, height: 2, samples, frame: 1 / 25 };
  const app = new AppFrames("/v.mp4", track, size, () => {});
  const picture = (rank: number, w: number, h: number) => new Picture(new Uint8Array(w * h * 4), w, h, rank * 40000);
  return { app, picture };
}

// A stream on a Go side the test answers by hand: each pull waits until
// the test hands it the ranks it brings, each frame's first byte its rank,
// and the times the Go side tells with a stream's first frame.
function goSide() {
  vi.stubGlobal("fetch", (url: string) =>
    Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ id: `s${url.length}` }), text: () => Promise.resolve("") }),
  );
  const { app } = frames(() => ({ width: 4, height: 2 }));
  app.paused = () => false;
  const asked: { url: string; give: (ranks: number[], times?: string) => void; shut: () => void }[] = [];
  const came: (() => void)[] = [];
  (app as unknown as { puller: unknown }).puller = {
    pull: (url: string) =>
      new Promise((done) => {
        asked.push({
          url,
          give: (ranks, times = "") =>
            done({
              status: 200,
              end: false,
              error: "",
              closed: false,
              times,
              light: "",
              spare: [],
              frames: ranks.map((r) => ({ at: r / 25, frame: new Picture(new Uint8Array(4 * 2 * 4).fill(r), 4, 2, r * 40000) })),
            }),
          // Closed on the Go side, to make room for newer streams.
          shut: () => done({ status: 200, end: false, error: "", closed: true, times: "", light: "", spare: [], frames: [] }),
        });
        for (const c of came.splice(0)) c();
      }),
    close() {},
  };
  // Settles once the Go side has been asked n times in all.
  const pulls = (n: number) =>
    new Promise<void>((done) => {
      const look = () => (asked.length >= n ? done() : came.push(look));
      look();
    });
  const rankOf = async (p: Promise<Picture | null>) => (await p)?.data?.[0] ?? null;
  return { app, asked, pulls, rankOf };
}

describe("a stream asked for a frame while a pull is on its way", () => {
  afterEach(() => vi.unstubAllGlobals());

  // The pull on its way skips to a frame further on, so the frame asked
  // for in between never comes from it. It was handed the frame before
  // it that was kept instead, under its own rank, and the video preview
  // showed the frame before the one it said, issue #358.
  test("still gets its own frame when the pull skips past it", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    const ten = app.frame(10, 0);
    await pulls(1);
    asked[0].give([10]);
    expect(await rankOf(ten)).toBe(10);
    const twenty = app.frame(20, 0);
    await pulls(2);
    expect(asked[1].url).toContain("skip=");
    const fourteen = app.frame(14, 0);
    asked[1].give([19, 20]);
    expect(await rankOf(twenty)).toBe(20);
    // Either the frame comes as it is, or it is asked of the Go side again.
    const first = await Promise.race([rankOf(fourteen), pulls(3).then(() => "asked again")]);
    expect(first).toBe("asked again");
    asked[2].give([14]);
    expect(await rankOf(fourteen)).toBe(14);
    app.close();
  });
});

describe("a stream asked for a frame while its first frame comes for nobody", () => {
  afterEach(() => vi.unstubAllGlobals());

  // A stream opened for a place the hand left reads its first frame all
  // the same, see firstFrame. A frame asked of it meanwhile, one the
  // stream reaches, was never pulled: pull returned at once since the
  // stream was pulling, and the pull of the first frame returned after
  // it. The frame never came, the cue that waited for it never said it
  // stood, and a walk hung after a drag of an edge, issue #556.
  test("still gets the frame asked of it meanwhile", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    const withdrawn = new Set<{ gone?: boolean }>();
    void app.frame(10, 0, withdrawn as never, true);
    // The hand moves on before the stream is open, as a decoder reset
    // withdraws what it asked.
    for (const w of withdrawn) w.gone = true;
    await pulls(1);
    expect(asked[0].url).toContain("n=1");
    const twelve = app.frame(12, 0);
    asked[0].give([10]);
    await pulls(2);
    asked[1].give([12]);
    expect(await rankOf(twelve)).toBe(12);
    app.close();
  });

  // The same, with the stream closed on the Go side before its first
  // frame came, the way it makes room for the newer streams of a drag.
  // The frame asked meanwhile ended as the nearest frame there was,
  // nothing, where a pull hands it to another stream.
  test("still gets the frame asked of it meanwhile when the stream is closed", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    const withdrawn = new Set<{ gone?: boolean }>();
    void app.frame(10, 0, withdrawn as never, true);
    for (const w of withdrawn) w.gone = true;
    await pulls(1);
    const twelve = app.frame(12, 0);
    asked[0].shut();
    await pulls(2);
    asked[1].give([12]);
    expect(await rankOf(twelve)).toBe(12);
    app.close();
  });
});

describe("a place dragged to, opened with its key frame first", () => {
  afterEach(() => vi.unstubAllGlobals());

  // A press at frame 10 and a drag on to frame 150, whose stream is asked
  // for its key frame first, in a pull of one frame.
  async function dragged() {
    const { app, asked, pulls, rankOf } = goSide();
    const passed: number[] = [];
    app.onPassing = (rank, _frame, _seq, key) => key && passed.push(rank);
    app.grip(true);
    // Where the press landed opens with no key frame first.
    void app.frame(10, 0, undefined, true);
    await pulls(1);
    asked[0].give([10]);
    const under = rankOf(app.frame(150, 0, undefined, true));
    await pulls(2);
    expect(asked[1].url).toContain("n=1");
    return { app, asked, pulls, passed, under };
  }

  test("puts up the key frame the decoder sought, then the frame under the hand", async () => {
    const { app, asked, pulls, passed, under } = await dragged();
    asked[1].give([140], "1,2,3,0,0");
    await pulls(3);
    asked[2].give([141, 142, 143, 144, 145, 146, 147, 148, 149, 150]);
    expect(await under).toBe(150);
    expect([app.stats.keyFirst, app.stats.keyCame, app.stats.keyWentOn]).toEqual([1, 1, 0]);
    expect(app.stats.wentOnRanks).toEqual([]);
    expect(app.stats.opens.at(-1)?.on).toBe(false);
    expect(passed).toEqual([140]);
    app.close();
  });

  // Issue #511: the decoder reads on to the frame asked for, without a
  // seek, when a cursor already stands between the key frame and it, and
  // then hands over no key frame. The walk of a far drag took that for a
  // key frame the app failed to put up.
  test("says the decoder read on to the frame under the hand, with no key frame to put up", async () => {
    const { app, asked, passed, under } = await dragged();
    asked[1].give([150], "1,2,3,0,1");
    expect(await under).toBe(150);
    expect([app.stats.keyFirst, app.stats.keyCame, app.stats.keyWentOn]).toEqual([1, 0, 1]);
    expect(app.stats.wentOnRanks).toEqual([150]);
    expect(app.stats.opens.at(-1)?.on).toBe(true);
    expect(passed).toEqual([]);
    app.close();
  });
});

describe("a slow drag to the left", () => {
  afterEach(() => vi.unstubAllGlobals());

  // Issue #596: a slow drag of the playhead to the left stood for each
  // step, up to seconds, where one to the right kept up. A stream only
  // reads on, so every step back was a stream of its own, a seek with its
  // key frame first, which the frame on screen was nearer than.
  test("brings the frames before the place it steps back to, so the next steps find them kept", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    app.grip(true);
    // Where the press landed.
    const pressed = rankOf(app.frame(100, 0, undefined, true));
    await pulls(1);
    asked[0].give([100]);
    expect(await pressed).toBe(100);
    // A frame back: one stream, from 15 frames before, every frame of it
    // handed over, with no key frame first and no skip.
    const back = rankOf(app.frame(99, 0, undefined, true));
    await pulls(2);
    expect(asked[1].url).toContain("n=16");
    expect(asked[1].url).not.toContain("skip=");
    asked[1].give(Array.from({ length: 16 }, (_, i) => 84 + i));
    expect(await back).toBe(99);
    // The next 14 steps back come from the frames kept.
    for (let r = 98; r >= 85; r--) expect(await rankOf(app.frame(r, 0, undefined, true))).toBe(r);
    expect(asked.length).toBe(2);
    expect([app.stats.streams, app.stats.keyFirst]).toEqual([2, 0]);
    app.close();
  });

  // Found by the review of #677: the stream opened for a step back handed
  // over every frame between for as long as it lived, so a frame asked of
  // it later, after the drag, came with the twenty before it.
  test("hands over every frame on its first pull only, and skips after", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    app.grip(true);
    void app.frame(100, 0, undefined, true);
    await pulls(1);
    asked[0].give([100]);
    void app.frame(99, 0, undefined, true);
    await pulls(2);
    expect(asked[1].url).toContain("n=16");
    asked[1].give(Array.from({ length: 16 }, (_, i) => 84 + i));
    app.grip(false);
    // A still further on goes to the stream of the press, which the Go
    // side closes on the way, so it goes on to the stream of the step
    // back: the frames before it are skipped there, but for the one just
    // before it.
    const later = rankOf(app.frame(110, 0, undefined, true));
    await pulls(3);
    asked[2].shut();
    await pulls(4);
    expect(asked[3].url).toContain("n=2");
    expect(asked[3].url).toContain("skip=");
    asked[3].give([109, 110]);
    expect(await later).toBe(110);
    app.close();
  });

  // A far step back is a hand moving fast, and a far step comes quicker
  // with its key frame first, as before, issue #281.
  test("leaves a far step back to its key frame first", async () => {
    const { app, asked, pulls } = goSide();
    app.grip(true);
    void app.frame(100, 0, undefined, true);
    await pulls(1);
    asked[0].give([100]);
    void app.frame(90, 0, undefined, true);
    await pulls(2);
    expect(asked[1].url).toContain("n=1");
    expect(app.stats.keyFirst).toBe(1);
    app.close();
  });
});
