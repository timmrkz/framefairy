import { describe, expect, test } from "vitest";

import { together, type Member } from "./together";

// A run-down as Busy keeps one: the hand on it or not, and held or running
// as its group says once it is told to look again.
function runDown(name: string) {
  const one = { hand: false, held: false, group: null as ReturnType<typeof together> | null };
  const self: Member = {
    holding: () => one.hand,
    settle: () => {
      one.held = one.group!.holding();
    },
  };
  one.group = together(name, self);
  return {
    get held() {
      return one.held;
    },
    hand(on: boolean) {
      one.hand = on;
      one.group!.settleAll();
    },
    leave: () => one.group!.leave(),
  };
}

describe("run-downs of one removal", () => {
  test("the hand on one holds them all, and leaving lets them all run", () => {
    const row = runDown("away job-1");
    const card = runDown("away job-1");
    row.hand(true);
    expect([row.held, card.held]).toEqual([true, true]);
    row.hand(false);
    expect([row.held, card.held]).toEqual([false, false]);
  });

  test("the hand on one of another removal holds nothing of this one", () => {
    const mine = runDown("away job-2");
    const other = runDown("away job-3");
    other.hand(true);
    expect([mine.held, other.held]).toEqual([false, true]);
    other.hand(false);
  });

  test("the one the hand held them by going lets the rest run", () => {
    const row = runDown("away job-4");
    const card = runDown("away job-4");
    card.hand(true);
    expect(row.held).toBe(true);
    card.leave();
    expect(row.held).toBe(false);
  });

  test("with no name a run-down is alone", () => {
    const a = runDown("");
    const b = runDown("");
    a.hand(true);
    expect([a.held, b.held]).toEqual([true, false]);
  });
});
