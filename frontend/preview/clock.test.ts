import { describe, expect, test } from "vitest";
import { holdSound, walkClock } from "./clock";
import type { Play } from "../src/lib/frames/queue";

// A play whose sound card only says whether it runs, and counts what it
// is told.
function played() {
  const card = { state: "running" as AudioContextState, held: 0, started: 0 };
  const told: (() => void)[] = [];
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
        for (const tell of told) tell();
      },
      addEventListener: (_: string, tell: () => void) => told.push(tell),
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

  // WebKit suspends a sound card itself when its media session is
  // interrupted, and left it suspended in the middle of a play nobody
  // paused. The walk waited six minutes on a play that never moved. The
  // next tick starts it again, once, however long the browser takes.
  test("starts again a sound card the browser suspended while a play goes on", async () => {
    const { card, clock } = played();
    card.state = "suspended";
    expect(clock(3.2, 3.24, false)).toBe(3.2);
    expect(clock(3.2, 3.24, false)).toBe(3.2);
    expect(card).toMatchObject({ state: "running", held: 0, started: 1 });
  });

  // A card WebKit is still bringing back counts as asked: the ticks while
  // it takes its time do not ask again.
  test("asks a slow sound card to start once", async () => {
    const { play, card, clock } = played();
    let started = () => {};
    play.audio.resume = () => {
      card.started++;
      return new Promise<void>((done) => (started = done));
    };
    card.state = "interrupted" as AudioContextState;
    clock(3.2, 3.24, false);
    clock(3.21, 3.24, false);
    expect(card.started).toBe(1);
    started();
    await Promise.resolve();
    await Promise.resolve();
    clock(3.22, 3.24, false);
    expect(card.started).toBe(2);
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

  // Issue #702: the hand held the sound card while the play it had just
  // asked for was still starting it, so there was nothing to hold yet,
  // and the play ran on the sound card's time to the clip's end. The
  // first tick after the app has started it holds it again.
  test("holds the sound card for the hand once the app has started it", async () => {
    const { play, card, clock } = played();
    card.state = "suspended";
    await holdSound(play, "hand", true);
    expect(card.held).toBe(0);
    card.state = "running";
    expect(clock(0.1, 0.12, false)).toBe(0.1);
    expect(card).toMatchObject({ state: "suspended", held: 1, started: 0 });
    await holdSound(play, "hand", false);
    expect(card).toMatchObject({ state: "running", started: 1 });
  });

  // A play on the sound card's own clock has no tick of the walk's. The
  // hand held the card before the space bar, the app started it as the
  // play began, and the play ran to the clip's end while the walk
  // settled. The card is held again the moment it runs.
  test("holds the sound card for the hand once the app has started it, with no tick", async () => {
    const { play, card } = played();
    card.state = "suspended";
    await holdSound(play, "hand", true);
    await play.audio.resume();
    expect(card).toMatchObject({ state: "suspended", held: 1, started: 1 });
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
