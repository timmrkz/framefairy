import { describe, expect, test } from "vitest";
import { AppPictures, type AppFrames } from "./app";
import type { Picture } from "./picture";

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
