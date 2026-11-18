// A tall list in a scrolled column that builds only the cards in view and
// a screen either side, issue #894. Where the cards stand is lib/stack.ts.
// This is what keeps it in step with the page: the heights the cards are
// laid out at, how far the column is scrolled and how tall it is, and the
// card the hand is looking at, which stays where it stands as cards come
// in, are laid out or change their height.
//
// The list is a flex column with a gap between two cards and the veil as
// its padding. The space of the cards not built is the margin over the
// first card built and under the last, so the list holds no element of its
// own for it and the cards are its only children.
//
// The heights read back are turned into the room the cards not built take,
// which the layout rules in CLAUDE.md keep to a minimum. They are read
// through a ResizeObserver, whose answer comes after the layout and before
// the paint, and the column is put back in the same pass, so the cards in
// view never stand in the wrong place in a frame that is shown. Nothing
// in view is sized from them: a card built lays itself out.

import { untrack } from "svelte";
import { place, span, type Placed } from "./stack";

export type Shown = { above: number; below: number };

export class Stack {
  #keys: () => readonly string[];
  #measured = new Map<string, number>();
  #heard = $state(0);
  #box: HTMLElement | null = null;
  #cards: ResizeObserver | null = null;
  #keyOf = new WeakMap<Element, string>();
  #list: HTMLElement | null = null;

  // How far the column is scrolled and how tall it is.
  top = $state(0);
  seen = $state(0);
  // What the list holds over its first card, the line that says the cards
  // could not be read with its gap, and the list's padding, the veil.
  lead = $state(0);
  gap = $state(0);
  veil = $state(0);

  // The card that stays where it stands, how far under the top of the
  // column, and for which list: a new search is a new list and keeps no
  // place from the one before.
  #anchor: { key: string; at: number; list: unknown } | null = null;

  constructor(keys: () => readonly string[]) {
    this.#keys = keys;
  }

  readonly placed: Placed = $derived.by(() => {
    void this.#heard;
    return place(this.#keys(), this.#measured, this.gap);
  });

  // The cards built: the ones in view and a screen over and under it.
  readonly built: [number, number] = $derived.by(() => {
    const from = this.top - this.veil - this.lead;
    return span(this.placed, from - this.seen, from + 2 * this.seen);
  });

  // The room of the cards not built, over the first card built and under
  // the last.
  readonly over = $derived(this.built[0] <= this.built[1] ? this.placed.tops[this.built[0]] : 0);
  readonly under = $derived.by(() => {
    const [, last] = this.built;
    if (last < 0) return 0;
    return this.placed.total - this.placed.tops[last] - this.placed.heights[last];
  });

  // The list and the column it scrolls in, once both are on the page.
  // The gap and the veil come from the stylesheet and are read once.
  attach(list: HTMLElement, box: HTMLElement): () => void {
    this.#box = box;
    this.#list = list;
    const style = getComputedStyle(list);
    this.gap = parseFloat(style.rowGap) || 0;
    this.veil = parseFloat(style.paddingTop) || 0;
    this.top = box.scrollTop;
    this.seen = box.clientHeight;
    const column = new ResizeObserver(() => (this.seen = box.clientHeight));
    column.observe(box);
    this.#cards = new ResizeObserver((entries) => this.#laidOut(entries));
    return () => {
      column.disconnect();
      this.#cards?.disconnect();
      this.#cards = null;
      this.#box = this.#list = null;
    };
  }

  // A card built, measured each time its height changes. Put on the
  // card's element with {@attach}, which carries its key in data-row too.
  card(key: string) {
    return (el: Element) => {
      this.#keyOf.set(el, key);
      this.#cards?.observe(el);
      return () => this.#cards?.unobserve(el);
    };
  }

  // The line over the first card, measured the same way.
  head(el: Element) {
    const heard = new ResizeObserver(([e]) => (this.lead = e.borderBoxSize[0].blockSize + this.gap));
    heard.observe(el);
    return () => {
      heard.disconnect();
      this.lead = 0;
    };
  }

  #laidOut(entries: ResizeObserverEntry[]) {
    let changed = false;
    for (const e of entries) {
      const key = this.#keyOf.get(e.target);
      const h = e.borderBoxSize[0].blockSize;
      if (key === undefined || h === 0 || this.#measured.get(key) === h) continue;
      this.#measured.set(key, h);
      changed = true;
    }
    if (!changed) return;
    this.#heard++;
    this.hold();
  }

  // The cards built read now, before a place is worked out from the
  // heights, so a card built since the last answer of the ResizeObserver
  // is at the height it is laid out at and not at the guess. The page then
  // turns from where the cards really stand. It lays out the cards built,
  // a screen and two more, never the whole list. They are read from the
  // list, by their key in data-row, since a card just built may not have
  // been handed to card yet: Svelte runs the effects that turn the page
  // before the attachments of the cards it built in the same pass.
  #measureBuilt() {
    if (!this.#list) return;
    let changed = false;
    for (const el of this.#list.querySelectorAll<HTMLElement>(":scope > li[data-row]")) {
      const key = el.dataset.row!;
      const h = el.getBoundingClientRect().height;
      if (h === 0 || this.#measured.get(key) === h) continue;
      this.#measured.set(key, h);
      changed = true;
    }
    if (!changed) return;
    this.#heard++;
    this.hold();
  }

  // The column scrolled by the hand: where it is, and the card that is to
  // stay where it stands, the one preferred when it is in view, or else
  // the first card in view.
  scrolled(list: unknown, prefer: string | null) {
    if (!this.#box) return;
    this.#measureBuilt();
    this.top = this.#box.scrollTop;
    const keys = this.placed.keys;
    const mine = prefer === null ? -1 : keys.indexOf(prefer);
    const at = (i: number) => this.veil + this.lead + this.placed.tops[i] - this.top;
    const inView = (i: number) => at(i) + this.placed.heights[i] > 0 && at(i) < this.seen;
    let i = mine >= 0 && inView(mine) ? mine : -1;
    if (i < 0) {
      const [first, last] = span(this.placed, this.top - this.veil - this.lead, this.top - this.veil - this.lead + this.seen);
      i = first <= last ? first : -1;
    }
    this.#anchor = i < 0 ? null : { key: keys[i], at: at(i), list };
  }

  // The card that is to stay where it stands put back there, after cards
  // came in or changed their height. Only in the list it was taken in.
  hold(list?: unknown) {
    const a = this.#anchor;
    if (!a || !this.#box || (list !== undefined && a.list !== list)) return;
    const i = untrack(() => this.placed.keys.indexOf(a.key));
    if (i < 0) return;
    const want = this.veil + this.lead + untrack(() => this.placed.tops[i]) - a.at;
    if (Math.abs(want - this.#box.scrollTop) < 0.5) return;
    this.#box.scrollTop = want;
    this.top = this.#box.scrollTop;
  }

  // Where the card of a key stands against the column: how far its top is
  // over the top of what is in view, and its foot under the foot of it,
  // inside the veil, so both are 0 or less when it is in view. Read from
  // the heights, so nothing but the cards built is laid out for it.
  shows(key: string): Shown | null {
    if (!this.#box) return null;
    this.#measureBuilt();
    const i = this.placed.keys.indexOf(key);
    if (i < 0) return null;
    const top = this.#box.scrollTop;
    const cardTop = this.veil + this.lead + this.placed.tops[i] - top;
    return { above: this.veil - cardTop, below: cardTop + this.placed.heights[i] - (this.#box.clientHeight - this.veil) };
  }

  // The column scrolled so the card of a key stands at by, from the top of
  // what is in view, and stays there.
  #toAt(key: string, by: number, list: unknown) {
    if (!this.#box) return;
    this.#measureBuilt();
    const i = this.placed.keys.indexOf(key);
    if (i < 0) return;
    this.#box.scrollTop = this.veil + this.lead + this.placed.tops[i] - by;
    this.top = this.#box.scrollTop;
    this.#anchor = { key, at: this.veil + this.lead + this.placed.tops[i] - this.top, list };
  }

  // The card of a key at the top of the column, where the first card
  // stands when the list is at its top, under the veil.
  toTop(key: string, list: unknown) {
    this.#toAt(key, this.veil, list);
  }

  // The card of a key at the foot of the column, over the veil.
  toFoot(key: string, list: unknown) {
    this.#measureBuilt();
    const i = this.placed.keys.indexOf(key);
    if (i < 0 || !this.#box) return;
    this.#toAt(key, this.#box.clientHeight - this.veil - this.placed.heights[i], list);
  }

  // The list at its top, a new list read from its first card.
  toStart(list: unknown) {
    if (!this.#box) return;
    this.#box.scrollTo({ top: 0 });
    this.top = this.#box.scrollTop;
    this.#anchor = null;
    this.scrolled(list, null);
  }
}
