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
