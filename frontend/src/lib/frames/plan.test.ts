import { describe, expect, test } from "vitest";
import type { Samples } from "./mp4";
import { AudioPlan, fade, FADE, heardAt, Keeper, Program, spans, stillFeed, VideoPlan } from "./plan";

// A track the way a file holds one, without the file: n samples of `step`
// ticks, key frames every `gop`, and, for a picture with B-frames, the
// decode order of each group of four I/P B B shown as I B B P.
function track(n: number, timescale: number, step: number, gop: number, opts: { bframes?: boolean; shift?: number } = {}): Samples {
  const pts = new Float64Array(n);
  const duration = new Float64Array(n).fill(step);
  const key = new Uint8Array(n);
  const order = new Uint32Array(n);
  const rank = new Uint32Array(n);
  for (let i = 0; i < n; i++) {
    let shown = i;
    if (opts.bframes) {
      // Decode order 0 3 1 2 | 4 7 5 6 ...: the P of each four first.
      const base = i - (i % 4);
      shown = base + [0, 3, 1, 2][i % 4];
    }
    pts[i] = shown * step + (opts.shift ?? 0);
    key[i] = i % gop === 0 ? 1 : 0;
  }
  for (let i = 0; i < n; i++) order[i] = i;
  order.sort((a, b) => pts[a] - pts[b]);
  for (let r = 0; r < n; r++) rank[order[r]] = r;
  return {
    count: n,
    timescale,
    offset: Float64Array.from({ length: n }, (_, i) => i * 1000),
    size: new Uint32Array(n).fill(1000),
    pts,
    duration,
    key,
    order,
    rank,
  };
}

// The clip of the proof in the harness: three pieces, two cuts, no edge on
// a frame edge.
const clip = [
  { start: 57, end: 59.53 },
  { start: 60.31, end: 62.17 },
  { start: 63.05, end: 65.0 },
];
const frame = 0.04;
// Ten minutes at 25 a second, a key frame every four seconds.
const video = track(15000, 12800, 512, 100);
const shownAt = (r: number) => video.pts[video.order[r]] / 12800;

describe("Program", () => {
  const p = new Program(clip, false);
  test("puts the pieces end to end", () => {
    expect(p.length).toBeCloseTo(2.53 + 1.86 + 1.95);
    expect(p.locate(0)).toEqual({ v: 0, at: 57 });
    expect(p.locate(2.54)!.v).toBe(1);
    expect(p.locate(2.54)!.at).toBeCloseTo(60.32);
    expect(p.locate(p.length)).toBeNull();
  });
  test("places a moment in a cut at the start of the piece after it", () => {
    expect(p.place(58)).toBeCloseTo(1);
    expect(p.place(60)).toBeCloseTo(2.53);
    expect(p.place(10)).toBe(0);
    expect(p.place(70)).toBeCloseTo(p.length);
  });
  test("a loop goes round again on the same clock", () => {
    const l = new Program(clip, true);
    const again = l.locate(l.length + 0.5)!;
    expect(again.v).toBe(3);
    expect(again.at).toBeCloseTo(57.5);
    expect(l.visit(4)!.from).toBeCloseTo(l.length + 2.53);
  });
});

describe("VideoPlan", () => {
  const program = new Program(clip, false);
  const plan = new VideoPlan(video, frame, program, 0);
  plan.extend(Infinity);

  // Every frame of every piece, from the frame that holds its start to the
  // frame that holds the moment just before its end, and each drawn where
  // it begins.
  const frames = (from: number, to: number) => {
    const out: string[] = [];
    for (let t = from; t <= to + 1e-9; t += frame) out.push(t.toFixed(2));
    return out;
  };
  const drawnBy = (p: VideoPlan) => {
    const out: string[] = [];
    for (let k = 0; k <= p.lastK; k++) out.push(shownAt(p.need(k)!.rank).toFixed(2));
    return out;
  };

  test("draws every frame of every piece and never one from inside a cut", () => {
    expect(drawnBy(plan)).toEqual([...frames(57, 59.52), ...frames(60.28, 62.16), ...frames(63.04, 64.96)]);
  });

  test("draws each frame where it begins, carried onto the program", () => {
    // The first piece from the start of the program, a frame each 40 ms.
    expect(plan.at(0)).toBeCloseTo(0);
    expect(plan.at(1)).toBeCloseTo(0.04);
    expect(plan.at(63)).toBeCloseTo(2.52);
    // The second piece begins at 2.53 on the program with the frame that
    // holds 60.31, and the frame of 60.32 begins 10 ms later.
    expect(plan.at(64)).toBeCloseTo(2.53);
    expect(shownAt(plan.need(64)!.rank)).toBeCloseTo(60.28);
    expect(plan.at(65)).toBeCloseTo(2.54);
    // The last frame of the second piece, 62.16, begins 10 ms before the
    // cut, and the third piece's first frame follows it.
    expect(shownAt(plan.need(111)!.rank)).toBeCloseTo(62.16);
    expect(plan.at(111)).toBeCloseTo(2.53 + 1.85);
    expect(plan.at(112)).toBeCloseTo(2.53 + 1.86);
  });

  test("finds the point a position is in, and the one past the end", () => {
    expect(plan.gridAt(0)).toBe(0);
    expect(plan.gridAt(0.0399)).toBe(0);
    expect(plan.gridAt(0.04)).toBe(1);
    expect(plan.gridAt(2.5299)).toBe(63);
    expect(plan.gridAt(2.53)).toBe(64);
    expect(plan.gridAt(2.5399)).toBe(64);
    expect(plan.gridAt(program.length - 0.001)).toBe(plan.lastK);
    expect(plan.gridAt(program.length)).toBe(plan.lastK + 1);
  });

  test("the edges of a piece are never passed over", () => {
    // From inside the first piece the next that must be drawn is its last
    // frame, then the second piece's first, then that one's last.
    expect(plan.nextEdge(10)).toBe(63);
    expect(plan.nextEdge(63)).toBe(64);
    expect(plan.nextEdge(64)).toBe(111);
    expect(plan.nextEdge(plan.lastK)).toBe(plan.lastK + 1);
  });

  // A play begun inside a frame drew the frames on a grid from where it
  // began: each up to a frame late, and the last frame of a piece passed
  // over when the piece ended less than that into it.
  test.each([0.001, 0.013, 0.02, 0.039])("a play begun %f into a frame draws every frame where it begins", (into) => {
    const p0 = program.place(57.4 + into);
    const p = new VideoPlan(video, frame, program, p0);
    p.extend(Infinity);
    expect(p.at(0)).toBeCloseTo(p0);
    expect(shownAt(p.need(0)!.rank)).toBeCloseTo(57.4);
    expect(p.at(1)).toBeCloseTo(0.44);
    expect(drawnBy(p)).toEqual([...frames(57.4, 59.52), ...frames(60.28, 62.16), ...frames(63.04, 64.96)]);
  });

  test("a play begun at a cut starts on the next piece's first frame", () => {
    const p = new VideoPlan(video, frame, program, program.place(60));
    p.extend(Infinity);
    expect(drawnBy(p)).toEqual([...frames(60.28, 62.16), ...frames(63.04, 64.96)]);
    expect(p.at(1)).toBeCloseTo(2.54);
  });

  test("a play begun at the end of the clip has nothing to draw", () => {
    const p = new VideoPlan(video, frame, program, program.length);
    p.extend(Infinity);
    expect(p.lastK).toBe(-1);
    expect(p.gridAt(program.length)).toBe(0);
  });

  test("a piece ending on a frame's start ends on the frame before", () => {
    const p = new VideoPlan(video, frame, new Program([{ start: 10, end: 10.4 }, { start: 20.02, end: 20.1 }], false), 0);
    p.extend(Infinity);
    expect(drawnBy(p)).toEqual([...frames(10, 10.36), "20.00", "20.04", "20.08"]);
    expect(p.at(10)).toBeCloseTo(0.4);
    expect(p.at(11)).toBeCloseTo(0.42);
  });

  test("feeds each piece from the key frame before it, and carries a run on inside what it decoded", () => {
    // The first piece from 56, the second from 60, and the third, which
    // starts at 63.05 inside the group of pictures the second is in, goes on
    // from where the second stopped rather than from 60 again.
    expect(plan.runs.map((r) => [r.key, r.last])).toEqual([
      [1400, 1488],
      [1500, 1624],
    ]);
  });

  test("a loop starts a run back at the first piece's key frame", () => {
    const loop = new VideoPlan(video, frame, new Program(clip, true), 0);
    loop.extend(new Program(clip, true).length * 2 + 1);
    expect(loop.runs.slice(0, 4).map((r) => r.key)).toEqual([1400, 1500, 1400, 1500]);
    // The seam of the loop is a cut: the clip's last frame, then its first.
    const drawn: string[] = [];
    for (let k = 0; k < 170; k++) drawn.push(shownAt(loop.need(k)!.rank).toFixed(2));
    const seam = drawn.indexOf("64.96");
    expect(drawn.slice(seam, seam + 2)).toEqual(["64.96", "57.00"]);
    expect(loop.at(seam + 1)).toBeCloseTo(program.length);
  });

  test("a play from the middle starts on the frame under the playhead", () => {
    const p0 = program.place(61.003);
    const mid = new VideoPlan(video, frame, program, p0);
    mid.extend(Infinity);
    expect(shownAt(mid.need(0)!.rank)).toBeCloseTo(61.0);
    expect(mid.runs[0].key).toBe(1500);
  });

  test("keeps exactly the frames it draws and closes the rest unseen", () => {
    const kept: number[] = [];
    for (const run of plan.runs) {
      const keeper = new Keeper(plan, run);
      for (let d = run.key; d <= run.last; d++) if (keeper.offer(video.rank[d])) kept.push(video.rank[d]);
    }
    const drawn = new Set<number>();
    for (let k = 0; k <= plan.lastK; k++) drawn.add(plan.need(k)!.rank);
    expect(kept).toEqual([...drawn]);
  });

  test("with B-frames the last sample fed is the last one any drawn frame needs", () => {
    const b = track(15000, 12800, 512, 100, { bframes: true });
    const p = new VideoPlan(b, frame, new Program([{ start: 10.01, end: 10.5 }], false), 0);
    p.extend(Infinity);
    const run = p.runs[0];
    const needed = new Set<number>();
    for (let k = 0; k <= p.lastK; k++) needed.add(b.order[p.need(k)!.rank]);
    expect(run.key).toBe(200);
    expect(run.last).toBe(Math.max(...needed));
    // Shown 10.48 is the P decoded at 263 after the Bs before it, so 262
    // and 263 both have to go in.
    expect(run.last).toBeGreaterThanOrEqual(262);
  });
});

describe("stillFeed", () => {
  test("decodes from the key frame before to the frame that holds the moment", () => {
    expect(stillFeed(video, 63.05)).toEqual({ key: 1500, last: 1576, rank: 1576 });
    const b = track(15000, 12800, 512, 100, { bframes: true });
    // 10.04 is a B shown second and decoded third.
    const f = stillFeed(b, 10.05);
    expect(b.pts[b.order[f.rank]] / 12800).toBeCloseTo(10.04);
    expect(f.key).toBe(200);
    expect(f.last).toBe(b.order[f.rank]);
  });
});

describe("AudioPlan", () => {
  // Opus at 48 kHz, 960 samples a packet, moved back by its pre-skip of
  // 312, the way ffmpeg writes it.
  const opus = track(30001, 48000, 960, 1, { shift: -312 });
  const program = new Program(clip, false);
  const plan = new AudioPlan(opus, 48000, program, 0);
  plan.extend(Infinity);

  test("cuts at the sample and puts the stretches end to end", () => {
    const st = plan.runs.flatMap((r) => r.stretches);
    expect(st.map((s) => [s.from, s.to, s.out])).toEqual([
      [57 * 48000, 59.53 * 48000, 0],
      [60.31 * 48000, 60.31 * 48000 + 89280, 121440],
      [63.05 * 48000, 63.05 * 48000 + 93600, 210720],
    ]);
    // Nothing between them: each starts where the last ended.
    for (let i = 1; i < st.length; i++) expect(st[i].out).toBe(st[i - 1].out + (st[i - 1].to - st[i - 1].from));
  });

  test("starts every run far enough before for the decoder to settle", () => {
    for (const run of plan.runs) {
      const first = plan.packet(run.first);
      expect(first.at).toBeLessThanOrEqual(run.stretches[0].from - 4800 + 960);
    }
    expect(plan.runs.length).toBe(3);
  });

  test("a packet across a cut gives each side only its own samples", () => {
    const run = plan.runs[0];
    // The packet that holds 59.53.
    let j = 0;
    while (plan.packet(j).at + 960 <= 59.53 * 48000) j++;
    const parts = plan.slices(run, j, 960);
    expect(parts).toHaveLength(1);
    const p = plan.packet(j);
    expect(p.at + parts[0].to).toBe(59.53 * 48000);
    expect(parts[0].out + (parts[0].to - parts[0].from)).toBe(121440);
  });

  test("a short packet is the end of what it holds", () => {
    const run = plan.runs[0];
    // A packet well inside the first piece.
    let j = 0;
    while (plan.packet(j).at < 58 * 48000) j++;
    const [full] = plan.slices(run, j, 960);
    const [short] = plan.slices(run, j, 648);
    expect(full.to - full.from).toBe(960);
    // 312 left out of the front: what came out is the last 648.
    expect(short.to - short.from).toBe(648);
    expect(short.out).toBe(full.out + 312);
  });

  // AAC at 48 kHz decodes 1024 samples a packet, whatever the file says.
  // Tim's start.mp4 says its packets last 1008, 1056 and 1008 in turn, on
  // a clock of the sample rate. A track that counts 600 ticks a second,
  // the way QuickTime writes a movie, starts each packet on the nearest
  // tick, 12.8 ticks a packet. Placed by its stamp, a packet missed the
  // one before at most packets, by 16 samples in the first file.
  const aac = (timescale: number, at: (i: number) => number) => {
    const n = 6000;
    const t = track(n, timescale, 1, 1);
    for (let i = 0; i < n; i++) {
      t.pts[i] = at(i);
      t.duration[i] = at(i + 1) - at(i);
    }
    return t;
  };
  test.each([
    ["in turns of 1008, 1056 and 1008", aac(48000, (i) => i * 1024 + [0, -16, 16][i % 3])],
    ["on a clock of 600 ticks a second", aac(600, (i) => Math.round((i * 1024 * 600) / 48000))],
  ])("what the decoder puts out follows on to the sample, with packets %s", (_, t) => {
    const p = new AudioPlan(t, 48000, new Program([{ start: 10, end: 20 }], false), 0);
    p.extend(Infinity);
    const [run] = p.runs;
    let missed = 0;
    let end = -1;
    for (let j = run.first; j <= run.last; j++) {
      const byStamp = p.slices(run, j, 1024).at(-1);
      for (const s of p.decoded(run, j, 1024)) {
        if (end >= 0) expect(s.out).toBe(end);
        end = s.out + (s.to - s.from);
      }
      if (byStamp && byStamp.out + (byStamp.to - byStamp.from) !== end) missed++;
    }
    // The ten seconds are heard whole, to the sample, and placed by their
    // stamps they would have missed.
    expect(end - run.stretches[0].out).toBe(10 * 48000);
    expect(missed).toBeGreaterThan(100);
  });

  test("a packet more than half a packet off goes where its stamp says", () => {
    const run = { ...plan.runs[0], stretches: plan.runs[0].stretches, end: undefined };
    let j = 0;
    while (plan.packet(j).at < 58 * 48000) j++;
    const [first] = plan.decoded(run, j, 960);
    // The next one came out as nothing: the one after is a packet on.
    const [after] = plan.decoded(run, j + 2, 960);
    expect(after.out).toBe(first.out + 2 * 960);
  });

  test("two pieces closer than the decoder's lead share a run", () => {
    const close = new AudioPlan(opus, 48000, new Program([{ start: 10, end: 11 }, { start: 11.05, end: 12 }], false), 0);
    close.extend(Infinity);
    expect(close.runs).toHaveLength(1);
    expect(close.runs[0].stretches).toHaveLength(2);
  });
});

describe("fade", () => {
  test("is the render's fade at each edge of a piece", () => {
    const r = 48000;
    const f = FADE * r;
    expect(fade(0, 0, 48000, r)).toBeCloseTo(0.5 / f);
    expect(fade(f, 0, 48000, r)).toBe(1);
    expect(fade(24000, 0, 48000, r)).toBe(1);
    expect(fade(47999, 0, 48000, r)).toBeCloseTo(0.5 / f);
  });
});

describe("spans", () => {
  test("reads what lies close together in one request and nothing twice", () => {
    const got = spans(
      [
        { from: 0, to: 100 },
        { from: 150, to: 200 },
        { from: 10_000_000, to: 10_000_100 },
        { from: 500, to: 600 },
      ],
      [{ from: 450, to: 700 }],
      1000,
    );
    expect(got).toEqual([
      { from: 0, to: 200 },
      { from: 10_000_000, to: 10_000_100 },
    ]);
  });
  test("never reads more than the most in one", () => {
    const wants = Array.from({ length: 100 }, (_, i) => ({ from: i * 100, to: i * 100 + 100 }));
    for (const s of spans(wants, [], 1000, 2500)) expect(s.to - s.from).toBeLessThanOrEqual(2500);
  });
});

describe("heardAt", () => {
  test("follows the output timestamp, and the clock less its latency without one", () => {
    expect(heardAt({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1016)).toBeCloseTo(10.016);
    expect(heardAt(null, 10.05, 0.03, 1016)).toBeCloseTo(10.02);
    expect(heardAt({ contextTime: 0, performanceTime: 0 }, 0.5, 0.1, 5)).toBeCloseTo(0.4);
  });
  test("is never ahead of the clock, so a stamp from before a pause cannot carry play on", () => {
    // Held at 10.05 for 700 ms: the stamp would say 10.716.
    expect(heardAt({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1716)).toBe(10.05);
  });
  test("leaves a stamp on another clock for the clock less its latency", () => {
    expect(heardAt({ contextTime: 9.17, performanceTime: 21540 }, 9.186, 0.003, 8000)).toBeCloseTo(9.183);
  });
});
