import { describe, expect, test } from "vitest";

import { firstGuess, place, span } from "./stack";

// The cards of the transcript search are placed by their heights, so only
// the ones in view are built, and a card turned to lands where its place
// says, see lib/stack.ts.
describe("the cards of a tall stack", () => {
  test("stand one after the other, a gap between two", () => {
    const p = place(["a", "b", "c"], new Map([["a", 40], ["b", 58], ["c", 76]]), 8);
    expect([...p.tops]).toEqual([0, 48, 114]);
    expect([...p.heights]).toEqual([40, 58, 76]);
    expect(p.total).toBe(190);
  });

  test("not laid out yet are guessed at the mean of the ones laid out", () => {
    const p = place(["a", "b", "c", "d"], new Map([["a", 40], ["c", 60], ["gone", 80]]), 8);
    expect([...p.heights]).toEqual([40, 60, 60, 60]);
    expect([...p.tops]).toEqual([0, 48, 116, 184]);
  });

  test("with none laid out are guessed at a card of two lines", () => {
    const p = place(["a", "b"], new Map(), 10);
    expect([...p.heights]).toEqual([firstGuess, firstGuess]);
    expect(p.total).toBe(2 * firstGuess + 10);
  });

  test("of an empty list take no room", () => {
    const p = place([], new Map([["a", 40]]), 8);
    expect(p.total).toBe(0);
    expect(span(p, 0, 1000)).toEqual([0, -1]);
  });

  // Four cards of 40 with gaps of 10: 0 to 40, 50 to 90, 100 to 140 and
  // 150 to 190.
  const four = place(["a", "b", "c", "d"], new Map([["a", 40], ["b", 40], ["c", 40], ["d", 40]]), 10);

  test("in view are the ones that reach into it", () => {
    expect(span(four, 0, 190)).toEqual([0, 3]);
    expect(span(four, 60, 110)).toEqual([1, 2]);
    expect(span(four, 39, 51)).toEqual([0, 1]);
  });

  test("a view inside one card holds that card alone", () => {
    expect(span(four, 105, 135)).toEqual([2, 2]);
  });

  test("a view inside a gap holds no card", () => {
    expect(span(four, 41, 49)).toEqual([1, 0]);
  });

  test("a card ending where the view begins, or beginning where it ends, is not in it", () => {
    expect(span(four, 40, 100)).toEqual([1, 1]);
  });

  test("a view over the top or past the foot holds the cards at that end", () => {
    expect(span(four, -500, 20)).toEqual([0, 0]);
    expect(span(four, 170, 900)).toEqual([3, 3]);
    expect(span(four, 300, 900)).toEqual([4, 3]);
  });

  test("of a video of four hours are found in a long stack", () => {
    const keys = Array.from({ length: 1443 }, (_, i) => `s${i}`);
    const p = place(keys, new Map(keys.map((k, i) => [k, i % 2 ? 76 : 58])), 8);
    // Two cards are 58 + 8 + 76 + 8 = 150 high, so card 1000 starts at
    // 75000 and card 1001 at 75066.
    expect(p.tops[1000]).toBe(75000);
    expect(span(p, 75000, 75066)).toEqual([1000, 1000]);
    expect(span(p, 75000, 75067)).toEqual([1000, 1001]);
  });
});
