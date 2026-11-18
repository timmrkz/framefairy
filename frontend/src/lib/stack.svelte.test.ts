import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";

import { Stack } from "./stack.svelte";

// The column the transcript search scrolls in, the way the browser keeps
// it: the list on the page is as tall as the room the stack gave it the
// last time the page was built, and the column scrolls no further than
// that. A card built is the height it really is, which the stack only
// knows once it has read it.
class Page {
  height: Map<string, number>;
  keys: string[];
  // The room the stack gave the list when the page was last built.
  tall = 0;
  #top = 0;
  readonly box = {
    clientHeight: 500,
    get scrollHeight() {
      return page.tall;
    },
    get scrollTop() {
      return page.#top;
    },
    set scrollTop(v: number) {
      page.#top = Math.max(0, Math.min(v, page.tall - page.box.clientHeight));
    },
    scrollTo({ top }: { top: number }) {
      page.box.scrollTop = top;
    },
  };
  readonly list = {
    querySelectorAll: () => {
      const [first, last] = this.stack.built;
      return this.keys.slice(first, last + 1).map((row) => ({
        dataset: { row },
        getBoundingClientRect: () => ({ height: this.height.get(row)! }),
      }));
    },
  };
  stack: Stack;

  constructor(heights: number[]) {
    this.keys = heights.map((_, i) => `s${i}`);
    this.height = new Map(this.keys.map((k, i) => [k, heights[i]]));
    this.stack = new Stack(() => this.keys);
  }

  // The page built again from the stack, the list as tall as the stack
  // says with the veil over and under it, and the cards just built read,
  // the way the effect in SaidList.svelte does after each build.
  build() {
    this.tall = this.stack.placed.total + 2 * veil;
    this.stack.settle("");
  }

  // Where the card of a key stands, from the top of the column: under the
  // room the stack gave the cards not built over it, and under the cards
  // built over it at the heights they really are. Only a card built
  // stands anywhere.
  at(key: string) {
    const i = this.keys.indexOf(key);
    const [first, last] = this.stack.built;
    if (i < first || i > last) return NaN;
    let top = veil + this.stack.placed.tops[first];
    for (let j = first; j < i; j++) top += this.height.get(this.keys[j])! + gap;
    return top - this.#top;
  }
}

const veil = 16;
const gap = 8;
let page: Page;

beforeEach(() => {
  vi.stubGlobal("getComputedStyle", () => ({ rowGap: `${gap}px`, paddingTop: `${veil}px` }));
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("requestAnimationFrame", () => 0);
});

afterEach(() => vi.unstubAllGlobals());

// Cards of two heights in turn, a short one under a long one, the way a
// card of one line follows a card of two in a search.
const mixed = (n: number) => Array.from({ length: n }, (_, i) => (i % 2 ? 58 : 94));

function open(heights: number[]) {
  page = new Page(heights);
  page.stack.attach(page.list as unknown as HTMLElement, page.box as unknown as HTMLElement);
  page.build();
  return page;
}

describe("a tall stack of cards of different heights", () => {
  // The search opened at the card of the playhead far down a list it had
  // only guessed the height of, and the card stood out of view, 979 px
  // under the top of a column 583 px tall, because the column could not
  // yet scroll as far as the card's place.
  test("turned to a card far down puts it at the top once the list is as tall as its cards", () => {
    const p = open(mixed(200));
    // The list on the page is shorter than the cards say, the way it is
    // when the cards just read made the guess taller.
    p.tall = 4000;
    p.stack.toTop("s150", "");
    p.build();
    p.build();
    expect(p.at("s150")).toBe(veil);
  });

  test("turned to a card near the end stands it as high as the list's end lets it", () => {
    const p = open(mixed(200));
    p.stack.toTop("s199", "");
    p.build();
    p.build();
    expect(p.box.scrollTop).toBe(p.tall - p.box.clientHeight);
    expect(p.at("s199")).toBeLessThan(p.box.clientHeight);
    expect(p.at("s199")).toBeGreaterThan(0);
  });

  test("turned to the foot puts the card's foot over the veil", () => {
    const p = open(mixed(200));
    p.stack.toFoot("s120", "");
    p.build();
    p.build();
    expect(p.at("s120") + p.height.get("s120")!).toBe(p.box.clientHeight - veil);
  });

  // Cards above the one in view laid out at other heights than they were
  // guessed at leave it where the hand left it.
  test("scrolled by the hand keeps the first card in view where it stands as the cards above it are read", () => {
    const p = open(mixed(200));
    p.box.scrollTop = 6000;
    p.stack.scrolled("", null);
    p.build();
    const [first] = p.stack.built;
    const inView = p.keys.find((k) => p.at(k) + p.height.get(k)! > 0)!;
    expect(inView).toBeDefined();
    const was = p.at(inView);
    for (const k of p.keys.slice(first, first + 5)) p.height.set(k, p.height.get(k)! + 40);
    p.build();
    expect(p.at(inView)).toBe(was);
  });

  test("shows a card in view as in view and a card out of it as out", () => {
    const p = open(mixed(200));
    p.stack.toTop("s80", "");
    p.build();
    p.build();
    expect(p.stack.shows("s80")).toEqual({ above: 0, below: veil + p.height.get("s80")! - p.box.clientHeight + veil });
    expect(p.stack.shows("s10")!.above).toBeGreaterThan(0);
  });

  test("builds only the cards in view and a column either side", () => {
    const p = open(mixed(200));
    p.stack.toTop("s100", "");
    p.build();
    const [first, last] = p.stack.built;
    expect(last - first + 1).toBeLessThan(40);
    expect(first).toBeLessThanOrEqual(100);
    expect(last).toBeGreaterThanOrEqual(100);
  });
});
