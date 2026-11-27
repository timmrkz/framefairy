import { afterEach, describe, expect, test, vi } from "vitest";
import type { AppFrames } from "./app";
import type { Picture } from "./picture";
import { GoPlay, soundOf } from "./play";

// What the Go side sends for a pull of a play's sound: each chunk the
// sample of the clock it starts at, how many moments it holds and the
// changes of the clip taken in, then its samples, the channels side by
// side, see servePlay.
function body(chunks: { out: number; samples: number[][]; gen?: number }[], cut = 0): Uint8Array {
  const size = chunks.reduce((n, c) => n + 24 + c.samples.length * c.samples[0].length * 4, 0);
  const v = new DataView(new ArrayBuffer(size));
  let off = 0;
  for (const c of chunks) {
    v.setFloat64(off, c.out, true);
    v.setFloat64(off + 8, c.samples.length, true);
    v.setFloat64(off + 16, c.gen ?? 0, true);
    off += 24;
    for (const m of c.samples)
      for (const x of m) {
        v.setFloat32(off, x, true);
        off += 4;
      }
  }
  return new Uint8Array(v.buffer, 0, size - cut);
}

describe("the sound of a play", () => {
  test("is one chunk for the chunks that follow one another, each channel on its own", () => {
    const got = soundOf(
      body([
        { out: 100, samples: [[0.1, -0.1], [0.2, -0.2]] },
        { out: 102, samples: [[0.3, -0.3]] },
      ]),
      2,
    );
    expect(got).toHaveLength(1);
    expect(got[0].out).toBe(100);
    expect(got[0].length).toBe(3);
    expect([...got[0].data[0]].map((x) => +x.toFixed(3))).toEqual([0.1, 0.2, 0.3]);
    expect([...got[0].data[1]].map((x) => +x.toFixed(3))).toEqual([-0.1, -0.2, -0.3]);
  });

  // The next piece after a cut, or the clip's start again in a loop.
  test("parts where a chunk does not follow the one before", () => {
    const got = soundOf(
      body([
        { out: 100, samples: [[0.1], [0.2]] },
        { out: 500, samples: [[0.3]] },
      ]),
      1,
    );
    expect(got.map((c) => [c.out, c.length])).toEqual([
      [100, 2],
      [500, 1],
    ]);
  });

  // The sound of the clip as it was and of the clip changed, which the Go
  // side took in where the first ends, is two chunks, so the first can
  // make way.
  test("parts where the changes taken in differ", () => {
    const got = soundOf(
      body([
        { out: 100, samples: [[0.1], [0.2]] },
        { out: 102, samples: [[0.3]], gen: 1 },
      ]),
      1,
    );
    expect(got.map((c) => [c.out, c.length, c.gen])).toEqual([
      [100, 2, 0],
      [102, 1, 1],
    ]);
  });

  test("of a body cut short holds only the moments that came whole", () => {
    const got = soundOf(body([{ out: 0, samples: [[0.1, 0.1], [0.2, 0.2], [0.3, 0.3]] }], 4), 2);
    expect(got[0].length).toBe(2);
  });
});

// A clip changed while it plays is taken in by the Go side, which says
// where: what the window holds of the clip as it was from there on goes,
// and so does what comes of it after.
describe("a change taken into a play", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test("drops the frames and the sound of the clip as it was from where it took over", async () => {
    const answers: unknown[] = [
      { pictures: "p", sound: "s", from: 0, end: 4, length: 4, first: 60, moment: 60 },
      { gen: 1, at: 0.2, end: -1, lap: 3, video: false },
    ];
    const asked: string[] = [];
    vi.stubGlobal("fetch", async (url: string) => {
      asked.push(url);
      if (url.startsWith("/frames/read")) {
        return new Response(
          body([
            { out: 4000, samples: [[0.1], [0.2]] },
            { out: 9599, samples: [[0.1], [0.2], [0.3]] },
            { out: 9600, samples: [[0.4]], gen: 1 },
          ]).slice(),
        );
      }
      return Response.json(answers.shift());
    });
    const closed: number[] = [];
    const app = { path: "/v.mp4", gridText: "g", dressing: (q: URLSearchParams) => q.set("dressKey", "k") } as unknown as AppFrames;
    const play = await GoPlay.open(app, { framing: "a", pieces: [], loop: false, at: 60, rate: 48000, channels: 1, cue: false }, { width: 2, height: 2 });
    if (typeof play === "string") throw new Error(play);
    expect(asked[0]).toContain("program=a");
    expect(asked[0]).toContain("dressKey=k");
    for (const [at, gen] of [
      [0.16, 0],
      [0.2, 0],
      [0.24, 0],
      [0.2, 1],
    ])
      play.shots.push({ at, gen, moment: 60 + at, begins: 60 + at, edge: false, frame: { close: () => closed.push(at) } as unknown as Picture });
    const got = await play.take("b", true, 0.05);
    expect(asked[1]).toBe("/frames/play/take?id=p&heard=0.05&w=2&h=2&loop=1&dressKey=k&framing=b&program=b");
    expect(got?.end).toBe(Infinity);
    expect(play.end).toBe(Infinity);
    expect(play.length).toBe(3);
    expect(play.shots.map((s) => [s.at, s.gen])).toEqual([
      [0.16, 0],
      [0.2, 1],
    ]);
    expect(closed).toEqual([0.2, 0.24]);
    const sound = await play.moreSound();
    expect(sound.map((c) => [c.out, c.length, c.gen])).toEqual([
      [4000, 2, 0],
      [9599, 1, 0],
      [9600, 1, 1],
    ]);
  });
});
