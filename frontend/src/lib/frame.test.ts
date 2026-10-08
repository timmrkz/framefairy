import { describe, expect, test } from "vitest";
import cases from "./frame.cases.json";
import { frameAt } from "./flow";
import { rankAt, type Samples } from "./frames/mp4";

// The same table the render is held to, in engine/frameof_test.go. The
// engine keeps every edge on the start of the frame the render cuts it
// on, to the millisecond, and the frame that holds an edge kept there is
// that frame, whichever way the millisecond rounded it: the frame the
// video preview draws with the playhead on the edge, and the frame its
// playback of a piece starts with. The frame that holds the moment a frame
// before an edge is the frame before it, the last a piece ending there
// shows.

const rateOf = (rate: string): [number, number] => {
  const [n, d = "1"] = rate.split("/");
  return [Number(n), Number(d)];
};

// A picture whose frames come exactly at the rate, from start, the way a
// file's own times say it, on a clock of a thousand ticks to every one of
// the rate's, so the start is on it too.
function samplesAt(num: number, den: number, start: number, count: number): Samples {
  const pts = new Float64Array(count);
  const order = new Uint32Array(count);
  for (let k = 0; k < count; k++) {
    pts[k] = Math.round(start * num * 1000) + k * den * 1000;
    order[k] = k;
  }
  return {
    count,
    timescale: num * 1000,
    offset: new Float64Array(count),
    size: new Uint32Array(count),
    pts,
    duration: new Float64Array(count).fill(den * 1000),
    key: new Uint8Array(count).fill(1),
    order,
    rank: order,
  };
}

describe("the frame that holds an edge", () => {
  test.each(cases.edges)("at $rate fps from $start, an edge kept at $saved is frame $frame", (c) => {
    const [num, den] = rateOf(c.rate);
    const s = samplesAt(num, den, c.start, c.frame + 10);
    expect([frameAt(c.saved, den / num, c.start), rankAt(s, c.saved)]).toEqual([c.frame, c.frame]);
    if (c.frame > 0) {
      const before = c.saved - den / num;
      expect([frameAt(before, den / num, c.start), rankAt(s, before)]).toEqual([c.frame - 1, c.frame - 1]);
    }
  });

  test.each(cases.stretches)("at $rate fps from $start, every frame kept to the millisecond is that frame", (c) => {
    const [num, den] = rateOf(c.rate);
    const s = samplesAt(num, den, c.start, c.frames);
    const startMs = Math.round(c.start * 1000);
    let below = 0;
    const wrong: string[] = [];
    for (let k = 0; k < c.frames; k++) {
      // Rounded half up, in whole numbers, the way the engine keeps it,
      // see keptAt in engine/render.go.
      const ms = startMs + Math.floor((2 * k * den * 1000 + num) / (2 * num));
      if ((ms - startMs) * num < k * den * 1000) below++;
      const at = ms / 1000;
      const got = [frameAt(at, den / num, c.start), rankAt(s, at)];
      const want = [k, k];
      if (k > 0) {
        got.push(frameAt(at - den / num, den / num, c.start), rankAt(s, at - den / num));
        want.push(k - 1, k - 1);
      }
      if (got.join() !== want.join() && wrong.length < 5) wrong.push(`frame ${k} kept at ${at} is ${got.join(", ")}`);
    }
    expect(below).toBe(c.below);
    expect(wrong).toEqual([]);
  });
});
