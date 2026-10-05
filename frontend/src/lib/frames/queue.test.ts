import { afterAll, beforeAll, describe, expect, test } from "vitest";
import { FrameQueue, type Shown } from "./queue";

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
