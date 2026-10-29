import { describe, expect, test } from "vitest";
import { holdSound, walkClock } from "./clock";
import type { Play } from "../src/lib/frames/queue";

// A play whose sound card only says whether it runs, and counts what it
// is told.
function played() {
  const card = { state: "running" as AudioContextState, held: 0, started: 0 };
  const play = {
    playing: true as boolean,
    audio: {
      get state() {
        return card.state;
      },
      suspend: async () => {
        card.held++;
        card.state = "suspended";
      },
      resume: async () => {
        card.started++;
        card.state = "running";
      },
    } as unknown as AudioContext,
  } satisfies Play;
  return { play, card, clock: walkClock(play) };
}

describe("the walk's clock", () => {
  // Issue #588: standing on the frame alone left the sound running on
  // ahead of the picture.
  test("stands on a frame not up yet, and holds the sound card until it is", () => {
    const { card, clock } = played();
    expect(clock(3.2, 3.04, false)).toBe(3.04);
    expect(clock(3.21, 3.04, false)).toBe(3.04);
    expect(card).toMatchObject({ state: "suspended", held: 1, started: 0 });
    expect(clock(3.21, 3.24, false)).toBe(3.21);
    expect(card).toMatchObject({ state: "running", held: 1, started: 1 });
    expect(clock(3.22, 3.24, false)).toBe(3.22);
    expect(card.started).toBe(1);
  });

  // The app holds the sound card itself when it pauses, and the clock
  // lets go only of a hold it made.
  test("leaves a sound card held by someone else held", () => {
    const { card, clock } = played();
    card.state = "suspended";
    expect(clock(3.2, 3.24, false)).toBe(3.2);
    expect(card).toMatchObject({ state: "suspended", held: 0, started: 0 });
  });

  // A paused play holds its sound card, and the clock's last hold let go
  // after the pause leaves it held.
  test("lets the sound card go only while a play goes on", () => {
    const { play, card, clock } = played();
    clock(3.2, 3.04, false);
    play.playing = false;
    clock(3.2, 3.24, false);
    expect(card).toMatchObject({ state: "suspended", started: 0 });
  });

  // A walk holds the sound card while its hand drags, issue #583. The
  // review of #588 found the clock let go of the hand's hold when the
  // frame it waited for came up during the drag.
  test("lets the sound card go only once neither the frame nor the hand holds it", async () => {
    const { play, card, clock } = played();
    expect(clock(3.2, 3.04, false)).toBe(3.04);
    await holdSound(play, "hand", true);
    expect(clock(3.2, 3.24, false)).toBe(3.2);
    expect(card).toMatchObject({ state: "suspended", started: 0 });
    await holdSound(play, "hand", false);
    expect(card).toMatchObject({ state: "running", started: 1 });

    await holdSound(play, "hand", true);
    expect(clock(3.3, 3.28, false)).toBe(3.28);
    expect(clock(3.3, 3.32, false)).toBe(3.3);
    expect(card).toMatchObject({ state: "suspended", started: 1 });
    await holdSound(play, "hand", false);
    expect(card).toMatchObject({ state: "running", started: 2 });
  });

  // A frame the queue says is nowhere, see unshown in queue.ts, holds
  // nothing, so the play goes by the sound card's time.
  test("does not hold the sound card for a frame that is nowhere", () => {
    const { card, clock } = played();
    expect(clock(35.7, Infinity, false)).toBe(35.7);
    expect(card.held).toBe(0);
  });

  // A frame decoded and waiting goes up on this tick, so the sound card is
  // not held for it. Held for every frame, it stopped and started five
  // times a second in the walks, and each frame stood a refresh or two
  // longer than it lasts.
  test("leaves the sound card running for a frame decoded and waiting", () => {
    const { card, clock } = played();
    expect(clock(35.61, 35.6, true)).toBe(35.6);
    expect(card).toMatchObject({ state: "running", held: 0 });
  });

  // A play started again starts the sound card itself, see maybeBegin in
  // queue.ts, while the clock may still count itself a holder. The next
  // frame not up holds it again.
  test("holds the sound card again once the app has started it", () => {
    const { card, clock } = played();
    clock(3.2, 3.04, false);
    card.state = "running";
    expect(clock(3.3, 3.04, false)).toBe(3.04);
    expect(card).toMatchObject({ state: "suspended", held: 2 });
  });
});
