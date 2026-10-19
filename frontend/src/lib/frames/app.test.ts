import { afterEach, describe, expect, test, vi } from "vitest";
import { AppFrames, AppPictures } from "./app";
import type { VideoTrack } from "./movie";
import { Picture } from "./picture";

// Frames that come at once, each knowing its rank.
function pictures() {
  const out: number[] = [];
  const frames = {
    frame: (rank: number) => Promise.resolve({ rank, close() {} } as unknown as Picture),
  } as unknown as AppFrames;
  const dec = new AppPictures(frames, (f) => out.push((f as unknown as { rank: number }).rank), () => {});
  return { dec, out };
}

const settled = () => new Promise((done) => setTimeout(done, 0));

describe("AppPictures", () => {
  test("a run fed after a flush starts from its own first frame, lower ranks too", async () => {
    // The clip's last piece, then its start the next time through a loop,
    // on the same decoder. The start was held back for good, waiting for
    // the rank after the last piece's end (2.264).
    const { dec, out } = pictures();
    for (const r of [84, 85, 86]) dec.decode(r, false, new Uint8Array(0), r, true, 84);
    await dec.flush();
    expect(out).toEqual([84, 85, 86]);
    for (const r of [0, 1, 2]) dec.decode(r, false, new Uint8Array(0), r, true, 0);
    await settled();
    expect(out).toEqual([84, 85, 86, 0, 1, 2]);
  });

  test("frames come out in the order they are shown, whatever order they are fed in", async () => {
    const { dec, out } = pictures();
    for (const r of [10, 13, 11, 12]) dec.decode(r, false, new Uint8Array(0), r, true, 10);
    await settled();
    expect(out).toEqual([10, 11, 12, 13]);
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
});
