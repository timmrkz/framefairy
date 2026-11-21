import { describe, expect, test } from "vitest";
import { soundOf } from "./play";

// What the Go side sends for a pull of a play's sound: each chunk the
// sample of the clock it starts at and how many moments it holds, then
// its samples, the channels side by side, see servePlay.
function body(chunks: { out: number; samples: number[][] }[], cut = 0): Uint8Array {
  const size = chunks.reduce((n, c) => n + 16 + c.samples.length * c.samples[0].length * 4, 0);
  const v = new DataView(new ArrayBuffer(size));
  let off = 0;
  for (const c of chunks) {
    v.setFloat64(off, c.out, true);
    v.setFloat64(off + 8, c.samples.length, true);
    off += 16;
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

  // A play that takes over counts on the clock of the play before it.
  test("is moved on the clock by the play's shift", () => {
    expect(soundOf(body([{ out: 100, samples: [[0.1]] }]), 1, 48000)[0].out).toBe(48100);
  });

  test("of a body cut short holds only the moments that came whole", () => {
    const got = soundOf(body([{ out: 0, samples: [[0.1, 0.1], [0.2, 0.2], [0.3, 0.3]] }], 4), 2);
    expect(got[0].length).toBe(2);
  });
});
