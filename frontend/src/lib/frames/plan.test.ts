import { describe, expect, test } from "vitest";
import { ticks, type Samples } from "./movie";
import { AudioPlan, fade, FADE, heard, Keeper, Program, stillFeed, VideoPlan } from "./plan";

// A track the way the Go side hands one over: n samples of `step` ticks
// one after another, the first at `shift`.
function track(n: number, timescale: number, step: number, shift = 0): Samples {
  return { count: n, timescale, start: shift, step };
}

// The clip of the proof in the harness: three pieces, two cuts, no edge on
// a frame edge. A plan keeps every edge on a frame's start now, see
// PieceOnFrames in engine/render.go, and edges off them show each rule on
// its own: a piece starts with the frame that holds its start and ends
// with the frame that holds the moment a frame before its end.
const clip = [
  { start: 57, end: 59.53 },
  { start: 60.31, end: 62.17 },
  { start: 63.05, end: 65.0 },
];
const frame = 0.04;
// Ten minutes at 25 a second.
const video = track(15000, 12800, 512);
const shownAt = (r: number) => ticks(video, r) / 12800;

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
  // The loop switched on while the clip plays, let go round, and switched
  // off in the second time through: that play ends after it. The program
  // is that play's, and the next play builds its own, which plays the
  // clip once. Kept, it went on round the end.
  test("with its loop switched while it plays, it is that play's alone", () => {
    const fresh = new Program(clip, false);
    expect(fresh.changedWhilePlaying).toBe(false);
    const played = new Program(clip, false);
    played.loopOn();
    played.loopOff(played.length + 0.5);
    expect(played.end).toBeCloseTo(2 * played.length);
    expect(played.changedWhilePlaying).toBe(true);
    expect(fresh.end).toBeCloseTo(fresh.length);
  });
  // #277: the clip changed while it plays, by a drag, an undo or a redo.
  test("changed under the play, it goes on from the visit in hand with the new pieces", () => {
    const now = [
      { start: 57, end: 60 },
      { start: 61, end: 62.17 },
      { start: 63.05, end: 65.0 },
    ];
    const c = Program.spliced(p, 0, 58, now, false)!;
    expect(c.changedWhilePlaying).toBe(true);
    expect(c.visit(0)).toMatchObject({ v: 0, from: 0, start: 57, end: 60 });
    expect(c.visit(1)).toMatchObject({ v: 1, from: 3, start: 61, end: 62.17 });
    expect(c.visit(2)!.from).toBeCloseTo(4.17);
    expect(c.visit(3)).toBeNull();
    expect(c.end).toBeCloseTo(6.12);
    expect(c.locate(2)).toEqual({ v: 0, at: 59 });
    expect(c.locate(3.5)!.v).toBe(1);
    expect(c.locate(3.5)!.at).toBeCloseTo(61.5);
    expect(c.locate(c.end)).toBeNull();
    // With the loop on it goes round again from the first piece as it is.
    const l = Program.spliced(p, 0, 58, now, true)!;
    expect(l.visit(3)).toMatchObject({ v: 3, start: 57, end: 60 });
    expect(l.visit(3)!.from).toBeCloseTo(6.12);
    // In a cut of the new clip, or outside it, the moment has no piece.
    expect(Program.spliced(p, 0, 58, [{ start: 59, end: 65 }], false)).toBeNull();
  });
  test("left outside the clip, it goes straight on through the episode and stops at its end", () => {
    const r = Program.spliced(new Program(clip, true), 1, 61, [{ start: 0, end: 70 }], false)!;
    expect(r.loop).toBe(false);
    expect(r.visit(0)!.end).toBe(59.53);
    expect(r.visit(1)).toMatchObject({ v: 1, start: 60.31, end: 70 });
    expect(r.visit(1)!.from).toBeCloseTo(2.53);
    expect(r.visit(2)).toBeNull();
    expect(r.end).toBeCloseTo(2.53 + 70 - 60.31);
    expect(r.locate(2.53 + 5)!.at).toBeCloseTo(65.31);
    expect(r.lastPiece).toEqual({ start: 60.31, end: 70 });
  });
  test("changed further on, it parts at the first visit the change reaches", () => {
    const later = Program.spliced(p, 0, 58, [clip[0], clip[1], { start: 63.05, end: 64 }], false)!;
    expect(later.parting).toMatchObject({ v: 2, start: 63.05, end: 64 });
    expect(later.visit(0)).toEqual(p.visit(0));
    expect(later.visit(1)).toEqual(p.visit(1));
    expect(later.end).toBeCloseTo(p.end - 1);
    // A cut moved: the piece before it ends as it did, so it parts there.
    const cut = Program.spliced(p, 0, 58, [clip[0], { start: 60.5, end: 62.17 }, clip[2]], false)!;
    expect(cut.parting).toMatchObject({ v: 0, end: 59.53 });
    expect(cut.visit(1)).toMatchObject({ start: 60.5, from: p.visit(1)!.from });
    // Nothing ahead changed, only the start behind the playhead.
    const behind = Program.spliced(p, 1, 61, [{ start: 56, end: 59.53 }, clip[1], clip[2]], false)!;
    expect(behind.parting!.v).toBe(2);
    expect(behind.end).toBeCloseTo(p.end);
  });
  test("changed twice at the same visit, it keeps what came before", () => {
    const once = Program.spliced(p, 1, 61, [clip[0], { start: 60.31, end: 62 }, clip[2]], false)!;
    const twice = Program.spliced(once, 1, 61.5, [clip[0], { start: 60.31, end: 63.5 }], false)!;
    expect(twice.visit(0)!.end).toBe(59.53);
    expect(twice.visit(1)).toMatchObject({ start: 60.31, end: 63.5 });
    expect(twice.visit(2)).toBeNull();
  });
});

describe("VideoPlan", () => {
  const program = new Program(clip, false);
  const plan = new VideoPlan(video, frame, program, 0);
  plan.extend(Infinity);

  // Every frame of every piece, from the frame that holds its start to the
  // frame that holds the moment a frame before its end, and each drawn
  // where it begins.
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
  const p0 = () => new Program(clip, false);

  test("draws every frame of every piece and never one from inside a cut", () => {
    expect(drawnBy(plan)).toEqual([...frames(57, 59.48), ...frames(60.28, 62.12), ...frames(63.04, 64.96)]);
  });

  test("draws each frame where it begins, carried onto the program", () => {
    // The first piece from the start of the program, a frame each 40 ms.
    expect(plan.at(0)).toBeCloseTo(0);
    expect(plan.at(1)).toBeCloseTo(0.04);
    expect(plan.at(62)).toBeCloseTo(2.48);
    // The second piece begins at 2.53 on the program with the frame that
    // holds 60.31, and the frame of 60.32 begins 10 ms later.
    expect(plan.at(63)).toBeCloseTo(2.53);
    expect(shownAt(plan.need(63)!.rank)).toBeCloseTo(60.28);
    expect(plan.at(64)).toBeCloseTo(2.54);
    // The last frame of the second piece, 62.12, begins 50 ms before the
    // cut, and the third piece's first frame follows it.
    expect(shownAt(plan.need(109)!.rank)).toBeCloseTo(62.12);
    expect(plan.at(109)).toBeCloseTo(2.53 + 1.81);
    expect(plan.at(110)).toBeCloseTo(2.53 + 1.86);
  });

  test("finds the point a position is in, and the one past the end", () => {
    expect(plan.gridAt(0)).toBe(0);
    expect(plan.gridAt(0.0399)).toBe(0);
    expect(plan.gridAt(0.04)).toBe(1);
    expect(plan.gridAt(2.5299)).toBe(62);
    expect(plan.gridAt(2.53)).toBe(63);
    expect(plan.gridAt(2.5399)).toBe(63);
    expect(plan.gridAt(2.54)).toBe(64);
    expect(plan.gridAt(program.length - 0.001)).toBe(plan.lastK);
    expect(plan.gridAt(program.length)).toBe(plan.lastK + 1);
  });

  test("the edges of a piece are never passed over", () => {
    // From inside the first piece the next that must be drawn is its last
    // frame, then the second piece's first, then that one's last.
    expect(plan.nextEdge(10)).toBe(62);
    expect(plan.nextEdge(62)).toBe(63);
    expect(plan.nextEdge(63)).toBe(109);
    expect(plan.nextEdge(plan.lastK)).toBe(plan.lastK + 1);
  });

  // A play begun inside a frame drew the frames on a grid from where it
  // began: each up to a frame late, and the last frame of a piece passed
  // over when the piece ended less than that into it. A millisecond before
  // the next frame is that frame's, see rankAt, so 0.038 is the latest.
  test.each([0.001, 0.013, 0.02, 0.038])("a play begun %f into a frame draws every frame where it begins", (into) => {
    const p0 = program.place(57.4 + into);
    const p = new VideoPlan(video, frame, program, p0);
    p.extend(Infinity);
    expect(p.at(0)).toBeCloseTo(p0);
    expect(shownAt(p.need(0)!.rank)).toBeCloseTo(57.4);
    expect(p.at(1)).toBeCloseTo(0.44);
    expect(drawnBy(p)).toEqual([...frames(57.4, 59.48), ...frames(60.28, 62.12), ...frames(63.04, 64.96)]);
  });

  test("a play begun at a cut starts on the next piece's first frame", () => {
    const p = new VideoPlan(video, frame, program, program.place(60));
    p.extend(Infinity);
    expect(drawnBy(p)).toEqual([...frames(60.28, 62.12), ...frames(63.04, 64.96)]);
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
    expect(drawnBy(p)).toEqual([...frames(10, 10.36), "20.00", "20.04"]);
    expect(p.at(10)).toBeCloseTo(0.4);
    expect(p.at(11)).toBeCloseTo(0.42);
  });

  test("feeds each piece from its first frame, and carries a run on into the piece right after it", () => {
    // Every frame is its own key frame. The first piece from 57, the
    // second from 60.28. The third starts at 63.04, after frames the
    // second never fed, so it starts a run of its own.
    expect(plan.runs.map((r) => [r.key, r.last])).toEqual([
      [1425, 1487],
      [1507, 1553],
      [1576, 1624],
    ]);
    const on = new VideoPlan(video, frame, new Program([{ start: 10, end: 10.4 }, { start: 10.4, end: 10.8 }], false), 0);
    on.extend(Infinity);
    expect(on.runs.map((r) => [r.key, r.last])).toEqual([[250, 269]]);
  });

  test("changed under the play, it draws the piece in hand to its new end and the new pieces after it", () => {
    const p = new Program(clip, false);
    const runOn = new VideoPlan(video, frame, p, 0);
    runOn.extend(3);
    const runs = runOn.runs.length;
    const on = Program.spliced(p, 1, 61, [clip[0], { start: 60.31, end: 64 }], false)!;
    runOn.rewind(on, on.visit(1)!, () => -1)!();
    runOn.extend(Infinity);
    expect(drawnBy(runOn)).toEqual([...frames(57, 59.48), ...frames(60.28, 63.96)]);
    expect(runOn.runs.length).toBe(runs);

    const ahead = new VideoPlan(video, frame, p, 0);
    ahead.extend(Infinity);
    const shorter = Program.spliced(p, 1, 61, [clip[0], { start: 60.31, end: 61.5 }, clip[2]], false)!;
    ahead.rewind(shorter, shorter.visit(1)!, () => -1)!();
    ahead.extend(Infinity);
    expect(drawnBy(ahead)).toEqual([...frames(57, 59.48), ...frames(60.28, 61.44), ...frames(63.04, 64.96)]);
  });

  test("a piece is not run on into frames its decoder was fed past", () => {
    const p = new Program(clip, false);
    const fed = new VideoPlan(video, frame, p, 0);
    fed.extend(3);
    const on = Program.spliced(p, 1, 61, [clip[0], { start: 60.31, end: 64 }], false)!;
    expect(fed.rewind(on, on.visit(1)!, () => Infinity)).toBeNull();
    // Shorter, it draws less of what was fed, which holds.
    const shorter = Program.spliced(p, 1, 61, [clip[0], { start: 60.31, end: 61.5 }], false)!;
    expect(fed.rewind(shorter, shorter.visit(1)!, () => Infinity)).not.toBeNull();
  });

  test("cut short, it keeps what was worked out, so the end put back holds what its decoder was fed", () => {
    const two = [clip[0], clip[1]];
    const p = new Program(two, false);
    const plan = new VideoPlan(video, frame, p, 0);
    plan.extend(Infinity);
    const drawn = drawnBy(plan);
    const short = Program.spliced(p, 1, 61, [clip[0], { start: 60.31, end: 61.5 }], false)!;
    plan.rewind(short, short.visit(1)!, () => Infinity)!();
    plan.extend(Infinity);
    expect(plan.program).toBe(short);
    expect(drawnBy(plan)).toEqual(drawn);
    // Put back by an undo, the end is where the decoder was fed to anyway.
    const back = Program.spliced(short, 1, 61.2, two, false)!;
    expect(plan.rewind(back, back.visit(1)!, () => Infinity)).not.toBeNull();
  });

  test("changed further on, it keeps the runs its decoders were fed for the pieces before", () => {
    const fed = new VideoPlan(video, frame, p0(), 0);
    fed.extend(Infinity);
    const last = fed.runs[fed.runs.length - 1];
    const longer = Program.spliced(fed.program, 0, 58, [clip[0], clip[1], { start: 63.05, end: 66 }], false)!;
    // Every decoder fed, the last only to its first frame: a change to the
    // last piece's end holds.
    fed.rewind(longer, longer.parting!, (r) => (r === last ? 0 : Infinity))!();
    fed.extend(Infinity);
    expect(drawnBy(fed)).toEqual([...frames(57, 59.48), ...frames(60.28, 62.12), ...frames(63.04, 65.96)]);
  });

  test("cut short and then looped, the loop starts where the clip now ends", () => {
    const two = [clip[0], clip[1]];
    const plan = new VideoPlan(video, frame, new Program(two, false), 0);
    plan.extend(Infinity);
    const short = Program.spliced(plan.program, 1, 61, [clip[0], { start: 60.31, end: 61.5 }], false)!;
    plan.rewind(short, short.parting!, () => -1)!();
    expect(plan.cutShort).toBe(true);
    const looped = Program.spliced(short, 1, 61.2, [clip[0], { start: 60.31, end: 61.5 }], true)!;
    plan.rewind(looped, looped.parting!, () => -1)!();
    plan.extend(looped.length + 1);
    expect(plan.cutShort).toBe(false);
    const drawn: string[] = [];
    for (let k = 0; plan.need(k) && drawn.length < frames(57, 59.48).length + frames(60.28, 61.44).length + 1; k++) drawn.push(shownAt(plan.need(k)!.rank).toFixed(2));
    expect(drawn.slice(-2)).toEqual(["61.44", "57.00"]);
  });

  test("a loop starts a run back at the first piece's first frame", () => {
    const loop = new VideoPlan(video, frame, new Program(clip, true), 0);
    loop.extend(new Program(clip, true).length * 2 + 1);
    expect(loop.runs.slice(0, 4).map((r) => r.key)).toEqual([1425, 1507, 1576, 1425]);
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
    expect(mid.runs[0].key).toBe(1525);
  });

  test("keeps exactly the frames it draws and closes the rest unseen", () => {
    const kept: number[] = [];
    for (const run of plan.runs) {
      const keeper = new Keeper(plan, run);
      for (let d = run.key; d <= run.last; d++) if (keeper.offer(d)) kept.push(d);
    }
    const drawn = new Set<number>();
    for (let k = 0; k <= plan.lastK; k++) drawn.add(plan.need(k)!.rank);
    expect(kept).toEqual([...drawn]);
  });
});

describe("VideoPlan on a picture that starts after its sound", () => {
  // In ticks of 1/600 s, 30 frames a second, the first at 0.25 s.
  const late = track(30, 600, 20, 150);

  test("draws the first frame from the start of a piece that begins before it, then each frame where it begins", () => {
    const p = new VideoPlan(late, 1 / 30, new Program([{ start: 0.1, end: 0.4 }], false), 0);
    p.extend(Infinity);
    const drawn: number[] = [];
    for (let k = 0; k <= p.lastK; k++) drawn.push(p.need(k)!.rank);
    // The frames that begin at 0.25, 0.2833, 0.3167 and 0.35, the last
    // the one that holds a frame before the piece's end, 0.3667.
    expect(drawn).toEqual([0, 1, 2, 3]);
    expect(p.at(0)).toBeCloseTo(0);
    expect(p.at(1)).toBeCloseTo(0.28333 - 0.1, 4);
    expect(p.at(3)).toBeCloseTo(0.35 - 0.1, 4);
  });
});

describe("stillFeed", () => {
  test("decodes the frame that holds the moment, and only it", () => {
    expect(stillFeed(video, 63.05)).toEqual({ key: 1576, last: 1576, rank: 1576 });
    expect(stillFeed(video, 0)).toEqual({ key: 0, last: 0, rank: 0 });
  });
});

describe("AudioPlan", () => {
  // Opus at 48 kHz, 960 samples a packet, moved back by its pre-skip of
  // 312, the way ffmpeg writes it.
  const opus = track(30001, 48000, 960, -312);
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

  test("a packet after one that came out as nothing goes where its stamp says", () => {
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

  // A clip with no cut, looped: the next time through starts where the
  // run started, so by its first packet it looked carried on. Those
  // packets were fed long ago, and every one fed after went to both times
  // through, the second a whole clip ahead, so the sound was taken as fed
  // that far and stopped about three seconds before the seam.
  test("changed under the play, the piece in hand sounds on to its new end, with no fade where it used to end", () => {
    const p = new Program(clip, false);
    const ran = new AudioPlan(opus, 48000, p, 0);
    ran.extend(3);
    const on = Program.spliced(p, 1, 61, [clip[0], { start: 60.31, end: 64 }], false)!;
    // Fed into the fade where the piece ended, it no longer holds.
    expect(ran.rewind(on, on.visit(1)!, 121440 + 89280, () => false)).toBeNull();
    // A run whose decoder was flushed is fed no more.
    expect(ran.rewind(on, on.visit(1)!, 121440, () => true)).toBeNull();
    ran.rewind(on, on.visit(1)!, 121440, () => false)!();
    ran.extend(Infinity);
    const st = ran.runs.flatMap((r) => r.stretches);
    expect(st.map((s) => [s.from, s.to, s.out])).toEqual([
      [57 * 48000, 59.53 * 48000, 0],
      [60.31 * 48000, 60.31 * 48000 + 177120, 121440],
    ]);
    const was = 121440 + 89280;
    expect(fade(was - 1, st[1].pieceOut, st[1].pieceEnd, 48000)).toBe(1);
  });

  test("cut short, it keeps what was fed and says the end has no fade", () => {
    const p = new Program(clip, false);
    const ran = new AudioPlan(opus, 48000, p, 0);
    ran.extend(3);
    const was = ran.runs.flatMap((r) => r.stretches.map((s) => ({ ...s })));
    const short = Program.spliced(p, 0, 58, [{ start: 57, end: 58.5 }], false)!;
    ran.rewind(short, short.visit(0)!, 72000 + 24000, () => false)!();
    expect(ran.program).toBe(short);
    expect(ran.cutShort).toBe(true);
    expect(ran.runs.flatMap((r) => r.stretches)).toEqual(was);
    // Weighed against what was fed, the end put back holds, and has its fade again.
    const back = Program.spliced(short, 0, 58.2, clip, false)!;
    ran.rewind(back, back.visit(0)!, 72000 + 24000, () => false)!();
    expect(ran.cutShort).toBe(false);
    // With something after it, a loop or another piece, the end must fade.
    const looped = Program.spliced(p, 0, 58, [{ start: 57, end: 58.5 }], true)!;
    expect(ran.rewind(looped, looped.visit(0)!, 72000 + 24000, () => false)).toBeNull();
  });

  test("changed further on, the sound fed for the pieces before it holds", () => {
    const p = new Program(clip, false);
    const later = Program.spliced(p, 0, 58, [clip[0], clip[1], { start: 63.05, end: 66 }], false)!;
    // Fed past the first cut, into the second piece, a second ahead.
    const fed = Math.round(2.8 * 48000);
    const near = new AudioPlan(opus, 48000, p, 0);
    near.extend(3);
    expect(near.rewind(later, later.parting!, fed, () => false)).not.toBeNull();
    const all = new AudioPlan(opus, 48000, p, 0);
    all.extend(Infinity);
    all.rewind(later, later.parting!, fed, () => false)!();
    all.extend(Infinity);
    expect(all.runs.flatMap((r) => r.stretches).map((s) => s.to)).toEqual([59.53 * 48000, 62.17 * 48000, 66 * 48000]);
  });

  test("a loop back to where the run started starts a run of its own", () => {
    const looped = new AudioPlan(opus, 48000, new Program([{ start: 10, end: 20 }], true), 0);
    looped.extend(15);
    expect(looped.runs).toHaveLength(2);
    expect(looped.runs.map((r) => r.stretches.length)).toEqual([1, 1]);
    let j = 0;
    while (looped.packet(j).at < 15 * 48000) j++;
    const parts = looped.slices(looped.runs[0], j, 960);
    expect(parts).toHaveLength(1);
    expect(parts[0].out).toBeLessThan(10 * 48000);
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

describe("heard", () => {
  test("follows the output timestamp, and the clock less its latency without one", () => {
    expect(heard({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1016).at).toBeCloseTo(10.016);
    expect(heard(null, 10.05, 0.03, 1016).at).toBeCloseTo(10.02);
    expect(heard({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1016).source).toBe("stamp");
    expect(heard(null, 10.05, 0.03, 1016).source).toBe("plain");
    expect(heard({ contextTime: 0, performanceTime: 0 }, 0.5, 0.1, 5).at).toBeCloseTo(0.4);
  });
  test("is never ahead of the clock, so a stamp from before a pause cannot carry play on", () => {
    // Held at 10.05 for 700 ms: the stamp would say 10.716.
    expect(heard({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1716)).toEqual({ at: 10.05, source: "held" });
  });
  test("leaves a stamp on another clock for the clock less its latency", () => {
    expect(heard({ contextTime: 9.17, performanceTime: 21540 }, 9.186, 0.003, 8000).at).toBeCloseTo(9.183);
  });
});
