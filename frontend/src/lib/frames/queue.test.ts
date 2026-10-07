import { afterAll, beforeAll, describe, expect, test } from "vitest";
import { FrameQueue, SOUND_CHUNK, soundChunks, type Shown } from "./queue";

// The queue without a browser: a canvas that draws nothing and no decoders,
// so the file is never opened and every play stays starting. That is
// enough for what the queue says while a play starts, which is all a click
// while playing sees in its own frame.
function canvas(): HTMLCanvasElement {
  const ctx = { fillStyle: "", fillRect() {}, drawImage() {} };
  return { width: 0, height: 0, getContext: () => ctx } as unknown as HTMLCanvasElement;
}

const g = globalThis as Record<string, unknown>;
const had = g.cancelAnimationFrame;
beforeAll(() => {
  g.cancelAnimationFrame = () => {};
});
afterAll(() => {
  g.cancelAnimationFrame = had;
});

function playing(at: number) {
  const q = new FrameQueue(canvas(), "/episode.mp4");
  q.seek(at);
  q.play();
  const said: Shown[] = [];
  q.listen((s) => said.push(s));
  return { q, said };
}

const clip = [
  { start: 57, end: 59.53 },
  { start: 60.31, end: 62.17 },
];

describe("a seek while playing", () => {
  // A click across the edge of the clip changes the moment and the
  // program at once. Told the program first, the queue started the play
  // again from where it was and said so, and that old moment became the
  // playhead the seek was then sent to.
  test("onto another program says only the moment it was sent to", () => {
    const { q, said } = playing(58.2);
    q.seek(55.5, [null, false]);
    expect(said.map((s) => s.at)).toEqual([55.5]);
    expect(said.every((s) => s.playing)).toBe(true);
    said.length = 0;
    q.seek(61, [clip, false]);
    expect(said.map((s) => s.at)).toEqual([61]);
  });

  test("onto the same program says only the moment it was sent to", () => {
    const { q, said } = playing(58.2);
    q.seek(58.9, [null, false]);
    said.length = 0;
    q.seek(66.4, [null, false]);
    expect(said.map((s) => s.at)).toEqual([66.4]);
  });

  // A click seeks as the hand goes down and again as it comes up. The
  // second is the play already starting there, and starting it over would
  // throw away what was decoded for it.
  test("to the moment a play is starting from changes nothing", () => {
    const { q, said } = playing(58.2);
    q.seek(61, [clip, false]);
    said.length = 0;
    q.seek(61, [clip, false]);
    q.seek(61);
    expect(said).toEqual([]);
  });

  test("to the same moment on another program starts it over", () => {
    const { q, said } = playing(58.2);
    q.seek(58.4, [clip, false]);
    said.length = 0;
    q.seek(58.4, [null, false]);
    expect(said.map((s) => s.at)).toEqual([58.4]);
  });
});

describe("soundChunks", () => {
  // What the Go side sends for a pull of a sound stream: chunks of
  // SOUND_CHUNK moments, each after the moment it starts at, the last of a
  // stream shorter.
  const chunk = (at: number, values: number[]) => {
    const b = new Uint8Array(8 + values.length * 4);
    const v = new DataView(b.buffer);
    v.setFloat64(0, at, true);
    values.forEach((x, i) => v.setFloat32(8 + i * 4, x, true));
    return b;
  };
  const join = (...parts: Uint8Array[]) => {
    const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
    let at = 0;
    for (const p of parts) {
      out.set(p, at);
      at += p.length;
    }
    return out;
  };
  test("takes the samples of whole chunks and of a short last one, without the moments", () => {
    const first = Array.from({ length: SOUND_CHUNK * 2 }, (_, i) => i / 4096);
    const last = [0.5, -0.5, 0.25, -0.25];
    const got = soundChunks(join(chunk(1, first), chunk(1 + SOUND_CHUNK / 48000, last)), 2);
    expect(got.length).toBe(first.length + last.length);
    expect(Array.from(got.subarray(0, 3))).toEqual(first.slice(0, 3).map((x) => Math.fround(x)));
    expect(Array.from(got.subarray(first.length))).toEqual(last);
  });
  test("an answer with no chunk holds no samples", () => {
    expect(soundChunks(new Uint8Array(0), 2).length).toBe(0);
  });
});
