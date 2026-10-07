import { describe, expect, test } from "vitest";
import cases from "./frame.cases.json";
import { frameAt, frameOf } from "./flow";
import { edgeRank, rankAt, type Samples } from "./frames/mp4";

// The same table the render is held to, in engine/frameof_test.go: an
// edge means the frame whose start is nearest it, and a moment saved to
// the millisecond on a frame's start is that frame, whichever way the
// millisecond rounded it.

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

describe("the frame a moment means", () => {
  test.each(cases.edges)("at $rate fps from $start, an edge at $at is frame $frame", (c) => {
    const [num, den] = rateOf(c.rate);
    expect(frameOf(c.at, den / num, c.start)).toBe(c.frame);
    expect(edgeRank(samplesAt(num, den, c.start, c.frame + 10), c.at)).toBe(c.frame);
  });

  test.each(cases.stretches)("at $rate fps from $start, every frame saved to the millisecond is that frame", (c) => {
    const [num, den] = rateOf(c.rate);
    const s = samplesAt(num, den, c.start, c.frames);
    const startMs = Math.round(c.start * 1000);
    let below = 0;
    const wrong: string[] = [];
    for (let k = 0; k < c.frames; k++) {
      // Rounded half up, in whole numbers, the way engine/frameof_test.go
      // does it.
      const ms = startMs + Math.floor((2 * k * den * 1000 + num) / (2 * num));
      if ((ms - startMs) * num < k * den * 1000) below++;
      const at = ms / 1000;
      // As an edge, and as the playhead put there: the frame the video
      // preview draws is the frame the render starts on.
      const got = [frameOf(at, den / num, c.start), edgeRank(s, at), frameAt(at, den / num, c.start), rankAt(s, at)];
      if (got.some((g) => g !== k) && wrong.length < 5) wrong.push(`frame ${k} saved as ${at} is ${got.join(", ")}`);
    }
    expect(below).toBe(c.below);
    expect(wrong).toEqual([]);
  });
});
