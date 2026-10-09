import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { scrub } from "./scrub";

// A drag surface without a browser: what it is given to listen with, and
// the animation frames run by hand.
type Fn = (e: { clientX: number }) => void;
let frames: (() => void)[] = [];
const g = globalThis as Record<string, unknown>;
const had = [g.requestAnimationFrame, g.cancelAnimationFrame];
beforeEach(() => {
  frames = [];
  g.requestAnimationFrame = (f: () => void) => frames.push(f);
  g.cancelAnimationFrame = (n: number) => {
    if (n) frames[n - 1] = () => {};
  };
});
afterEach(() => {
  [g.requestAnimationFrame, g.cancelAnimationFrame] = had;
});

function press(x: number) {
  const on: Record<string, Fn> = {};
  const target = {
    setPointerCapture() {},
    addEventListener: (k: string, f: Fn) => (on[k] = f),
    removeEventListener() {},
  };
  const sent: number[] = [];
  // What the drag said, seeks and holds in the order they came.
  const told: string[] = [];
  scrub(
    { currentTarget: target, clientX: x, pointerId: 1 } as unknown as PointerEvent,
    (cx) => cx / 10,
    (t) => {
      sent.push(t);
      told.push(`seek ${t}`);
    },
    (held) => told.push(held ? "hold" : "let go"),
  );
  const frame = () => frames.splice(0).forEach((f) => f());
  return { on, sent, told, frame };
}

describe("scrub", () => {
  // While the video plays, the same moment sent again as the hand came up
  // started the play over from the click.
  test("a click let go after its frame seeks once", () => {
    const { on, sent, frame } = press(500);
    frame();
    on.pointerup({ clientX: 500 });
    expect(sent).toEqual([50]);
  });

  test("a click let go before its frame seeks once, as it lets go", () => {
    const { on, sent, frame } = press(500);
    on.pointerup({ clientX: 500 });
    frame();
    expect(sent).toEqual([50]);
  });

  test("a drag ends where it is let go", () => {
    const { on, sent, frame } = press(500);
    frame();
    on.pointermove({ clientX: 520 });
    frame();
    on.pointermove({ clientX: 530 });
    on.pointerup({ clientX: 530 });
    expect(sent).toEqual([50, 52, 53]);
  });

  // A play held under the hand goes on as the hand lets go, so the last
  // seek has to be in before it is told so, or the play went on from the
  // moment before and then jumped to where the hand was let go.
  test("the hand lets go after the last seek", () => {
    const { on, told, frame } = press(500);
    frame();
    on.pointermove({ clientX: 530 });
    on.pointerup({ clientX: 530 });
    expect(told).toEqual(["hold", "seek 50", "seek 53", "let go"]);
  });
});
