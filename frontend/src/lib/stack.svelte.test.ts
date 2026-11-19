import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";

import { span } from "./stack";
import { Stack } from "./stack.svelte";

// The column the transcript search scrolls in, the way the browser keeps
// it: the list on the page is as tall as the room the stack gave it the
// last time the page was built, and the column scrolls no further than
// that. A card built is the height it really is, which the stack only
// knows once it has read it.
class Page {
  height: Map<string, number>;
  keys = $state.raw<string[]>([]);
  // The cards on the page, one element each, kept while built.
  #els = new Map<string, { dataset: { row: string }; getBoundingClientRect: () => { height: number } }>();
  // The work left for when the app is idle, see rest.
  idle: (() => void)[] = [];
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
      return this.keys.slice(first, last + 1).map((row) => this.el(row));
    },
  };
  stack: Stack;

  constructor(heights: number[]) {
    this.keys = heights.map((_, i) => `s${i}`);
    this.height = new Map(this.keys.map((k, i) => [k, heights[i]]));
    this.stack = new Stack(
      () => this.keys,
      (work) => this.idle.push(work),
    );
  }

  el(row: string) {
    let el = this.#els.get(row);
    if (!el) {
      el = { dataset: { row }, getBoundingClientRect: () => ({ height: this.height.get(row)! }) };
      this.#els.set(row, el);
      this.stack.card(row)(el as unknown as Element);
    }
    return el;
  }

  // Cards built changed their height, told the way the ResizeObserver
  // tells it, after the layout.
  resized(keys: string[]) {
    observed.at(-1)!(keys.map((k) => ({ target: this.el(k), borderBoxSize: [{ blockSize: this.height.get(k)! }] })));
  }

  // The app idle until it has nothing left to do: each idle step that
  // builds cards has the page built again after it, the way the effect
  // in SaidList.svelte runs only when the cards built change.
  rest() {
    for (let n = 0; n < 100 && this.idle.length; n++) {
      const was = this.stack.built;
      this.idle.shift()!();
      if (this.stack.built !== was) this.build();
    }
    expect(this.idle).toEqual([]);
  }

  // The page built again from the stack, the list as tall as the stack
  // says with the veil over and under it, and the cards just built read,
  // the way the effect in SaidList.svelte does after each build. The
  // effect runs again as long as what it read makes the place or the
  // cards built change.
  build() {
    for (let n = 0; n < 10; n++) {
      const was = JSON.stringify([this.stack.placed.total, this.stack.built, this.box.scrollTop]);
      this.tall = this.stack.placed.total + 2 * veil;
      this.stack.settle("");
      if (JSON.stringify([this.stack.placed.total, this.stack.built, this.box.scrollTop]) === was) return;
    }
    throw new Error("the page never settled");
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
// What each ResizeObserver made hears, the cards' the last of them.
const observed: ((entries: unknown[]) => void)[] = [];

beforeEach(() => {
  vi.stubGlobal("getComputedStyle", () => ({ rowGap: `${gap}px`, paddingTop: `${veil}px` }));
  observed.length = 0;
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(heard: (entries: unknown[]) => void) {
        observed.push(heard);
      }
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
    const above = p.keys.slice(first, first + 5);
    for (const k of above) p.height.set(k, p.height.get(k)! + 40);
    p.resized(above);
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

// The cards further away are built while the app is idle, so a scroll
// moves through cards built already, the way it moves through a list
// built whole. Building them as they came into the screen kept either
// side cost every wheel step a layout, found by Tim.
describe("a tall stack building ahead while the app is idle", () => {
  test("builds the cards far over and under the ones in view, and a scroll inside them changes nothing", () => {
    const p = open(mixed(2000));
    p.stack.toTop("s1000", "");
    p.build();
    const needed = p.stack.built;
    p.rest();
    const built = p.stack.built;
    expect(built[0]).toBeLessThan(needed[0] - 50);
    expect(built[1]).toBeGreaterThan(needed[1] + 50);
    const top = p.stack.top;
    p.box.scrollTop += 3 * p.box.clientHeight;
    p.stack.scrolled("", null);
    expect(p.stack.top).toBe(top);
    expect(p.stack.built).toBe(built);
  });

  test("a scroll past the cards built builds the ones it needs at once", () => {
    const p = open(mixed(2000));
    p.rest();
    p.box.scrollTop = 120000;
    p.stack.scrolled("", null);
    p.build();
    const first = p.keys.find((k) => p.at(k) + p.height.get(k)! > 0);
    expect(first).toBeDefined();
    expect(p.at(first!)).toBeLessThan(p.box.clientHeight);
  });

  test("lets go of the cards far behind", () => {
    const p = open(mixed(2000));
    p.rest();
    p.box.scrollTop = 120000;
    p.stack.scrolled("", null);
    p.build();
    p.rest();
    const [first, last] = p.stack.built;
    expect(first).toBeGreaterThan(0);
    expect(last - first).toBeLessThan(1000);
  });

  test("a new list builds only the cards seen, and the rest while idle", () => {
    const p = open(mixed(2000));
    p.rest();
    p.keys = p.keys.filter((_, i) => i % 3);
    p.stack.toStart("e");
    p.build();
    const [first, last] = p.stack.built;
    expect(first).toBe(0);
    expect(last).toBeLessThan(40);
    p.rest();
    expect(p.stack.built[1]).toBeGreaterThan(100);
  });

  // Found by the review: the card a search kept in its own list was put
  // back in the whole list once the idle steps measured the cards they
  // built, so a search let go of by Escape stood on the wrong card.
  test("a search let go of keeps the card of the playhead where it stands as cards are built around it", () => {
    const p = open(mixed(2000));
    const all = p.keys;
    p.keys = all.filter((_, i) => i % 3);
    p.stack.toStart("searched");
    p.keys = all;
    p.stack.keep("s0", "");
    p.build();
    expect(p.at("s0")).toBe(veil);
    p.rest();
    expect(p.at("s0")).toBe(veil);
  });

  // Found by the review: a list that changed after a scroll inside the
  // cards built built the cards of the place the last needed cards were
  // worked out from, and the cards in view were missing for a frame.
  test("a list that changes by a card after a scroll keeps the cards in view built, and the ones built ahead", () => {
    const p = open(mixed(2000));
    p.stack.toTop("s1000", "");
    p.build();
    p.rest();
    p.box.scrollTop += 6 * p.box.clientHeight;
    p.stack.scrolled("", null);
    const ahead = p.stack.built;
    p.height.set("s2000", 58);
    p.keys = [...p.keys, "s2000"];
    p.build();
    const [first, last] = span(p.stack.placed, p.box.scrollTop - veil, p.box.scrollTop - veil + p.box.clientHeight);
    expect(p.stack.built[0]).toBeLessThanOrEqual(first);
    expect(p.stack.built[1]).toBeGreaterThanOrEqual(last);
    expect(p.stack.built).toEqual(ahead);
  });

  // WebKit has no idle callback, so a step came right after a frame,
  // in the middle of a scroll, found by the review.
  test("a step asked for waits while the hand scrolls, and builds once a frame passes without a scroll", () => {
    const p = open(mixed(2000));
    p.stack.toTop("s1000", "");
    p.build();
    const needed = p.stack.built;
    expect(p.idle.length).toBe(1);
    p.box.scrollTop += 100;
    p.stack.scrolled("", null);
    p.build();
    const scrolled = p.stack.built;
    p.idle.shift()!();
    expect(p.stack.built).toBe(scrolled);
    expect(scrolled[1] - scrolled[0]).toBeLessThan(40);
    expect(p.idle.length).toBe(1);
    p.rest();
    expect(p.stack.built[0]).toBeLessThan(needed[0] - 50);
  });
});
