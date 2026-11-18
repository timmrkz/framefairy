// A tall list in a scrolled column that builds the cards in view and a
// screen either side at once, and the cards further ahead, over and
// under, while the app is idle, issue #894. A scroll then mostly moves
// through cards built already, the way it does through a list built
// whole, and opening the list or typing in its search builds only what
// is seen. Where the cards stand is lib/stack.ts.
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

import { flushSync, untrack } from "svelte";
import { place, span, type Placed } from "./stack";

export type Shown = { above: number; below: number };

// Work for when the app has nothing else to do: the cards ahead of the
// ones in view, built a few at a time, see grow. A test hands in its own,
// so it decides when that work runs.
export type Later = (work: () => void) => void;
const whenIdle: Later = (work) => {
  if (typeof requestIdleCallback === "function") requestIdleCallback(() => work());
  // WebKit has no idle callback: after the next frame is painted.
  else requestAnimationFrame(() => setTimeout(work, 0));
};

// How far ahead of the cards in view, over and under, cards are built
// when the app is idle, in screens, and how many cards one idle step
// builds. A scroll of twenty screens finds its cards built, and the
// cards are built in few steps, since what a step costs beyond its cards,
// the list looked over and laid out again, is the same however many it
// builds. Measured on the harness's video of four hours: steps of 64
// cards cost a scroll a fifth more than steps of 128, and one step of
// 128 took 10 to 13 ms in the cloud, inside a frame.
const ahead = 20;
const step = 128;

// Whether two lists of keys are one list. The keys are made again
// whenever the rows are, so the same list often comes as a new array.
function same(a: readonly string[], b: readonly string[]) {
  if (a === b) return true;
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

export class Stack {
  #keys: () => readonly string[];
  #measured = new Map<string, number>();
  #heard = $state(0);
  #box: HTMLElement | null = null;
  #cards: ResizeObserver | null = null;
  #keyOf = new WeakMap<Element, string>();
  #list: HTMLElement | null = null;
  #later: Later;
  #growing = false;
  // Scrolls heard, so an idle step knows whether the column moved since
  // it was asked for.
  #scrolls = 0;
  // The cards built by idle steps, for the list they were built for.
  #grown = $state.raw<{ keys: readonly string[]; range: [number, number] } | null>(null);
  #builtWas: [number, number] | null = null;
  // The cards on the page read once, whose later heights the
  // ResizeObserver hears.
  #seenBuilt = new WeakSet<Element>();

  // How far the column is scrolled and how tall it is. At is where the
  // column really is, the one place that says so. Top is the same number
  // told to the page, written only when other cards are needed, see
  // scrolled, so a scroll inside the cards built makes no work.
  top = $state(0);
  #at = 0;
  // The list the stack was last told of: a new search is a new list, and
  // a card kept in one is never put back in another.
  #current: unknown = undefined;
  seen = $state(0);
  // What the list holds over its first card, the line that says the cards
  // could not be read with its gap, and the list's padding, the veil.
  lead = $state(0);
  gap = $state(0);
  veil = $state(0);

  // The card that stays where it stands, how far under the top of the
  // column, and for which list: a new search is a new list and keeps no
  // place from the one before. A card turned to the foot keeps its foot
  // over the veil instead, since its height may be a guess until it is
  // built.
  #anchor: { key: string; at: number; list: unknown; foot?: boolean } | null = null;

  constructor(keys: () => readonly string[], later: Later = whenIdle) {
    this.#keys = keys;
    this.#later = later;
  }

  readonly placed: Placed = $derived.by(() => {
    void this.#heard;
    return place(this.#keys(), this.#measured, this.gap);
  });

  // The cards that must be built now: the ones in view and a screen over
  // and under it.
  // They are worked out from where the column really is, also when the
  // list changed after a scroll top was not told of: from top, the cards
  // in view of a list that changed were not built for a frame, found by
  // the review.
  readonly needed: [number, number] = $derived.by(() => {
    void this.top;
    const from = this.#at - this.veil - this.lead;
    return span(this.placed, from - this.seen, from + 2 * this.seen);
  });

  // The cards built: the ones needed, and the ones idle steps built ahead
  // of them while they reach them. A scroll inside what is built builds
  // nothing and changes nothing on the page but the scroll. Building
  // cards as they came into the screen kept either side cost every wheel
  // step a layout of the cards it built, four times the work of the same
  // scroll through a list built whole, found by Tim.
  //
  // It hands back the same pair while the cards are the same, kept in
  // builtWas: a new pair of the same numbers is a change to Svelte, and
  // everything that reads the cards built would be looked over again.
  readonly built: [number, number] = $derived.by(() => {
    const need = this.needed;
    const g = this.#grown;
    const range = g && this.#regrown(g);
    let got = need;
    if (range && need[0] <= need[1] && range[0] <= need[1] + 1 && range[1] >= need[0] - 1)
      got = [Math.min(range[0], need[0]), Math.max(range[1], need[1])];
    const was = this.#builtWas;
    if (was && was[0] === got[0] && was[1] === got[1]) return was;
    return (this.#builtWas = got);
  });

  // The cards idle steps built, in the list as it is now. A list that
  // changed by a few cards, a sentence read again, a clip made, a word
  // corrected, keeps them from its first to its last, so the cards are not
  // built again. A list that is another list, a search typed or let go
  // of, keeps none.
  #regrown(g: { keys: readonly string[]; range: [number, number] }): [number, number] | null {
    const keys = this.placed.keys;
    if (same(g.keys, keys)) return g.range;
    const x = keys.indexOf(g.keys[g.range[0]]);
    const y = keys.indexOf(g.keys[g.range[1]]);
    if (x < 0 || y < x || y - x > g.range[1] - g.range[0] + 16) return null;
    return [x, y];
  }

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
    this.top = this.#at = box.scrollTop;
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

  // A card built, measured each time its height changes after it was
  // built, see settle for its first height. Put on the card's element with
  // {@attach}, which carries its key in data-row too. It is watched from
  // the next frame: a card built while the ResizeObserver answers, as the
  // list turns to the heights it just heard, is at the depth of the cards
  // it answered for, and the browser leaves it unanswered and logs a loop.
  card(key: string) {
    return (el: Element) => {
      this.#keyOf.set(el, key);
      let gone = false;
      requestAnimationFrame(() => gone || this.#cards?.observe(el));
      return () => {
        gone = true;
        this.#cards?.unobserve(el);
      };
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

  // The cards just built, read once they are on the page and before it is
  // painted, and the card that stays where it stands put back there. Run
  // from an effect each time the cards built or their places change, so a
  // card built stands at its own height in the first frame it is seen,
  // and the list is as tall as the heights say before the card kept is
  // put back, which the column could not scroll to while it was shorter.
  settle(list: unknown) {
    this.#current = list;
    this.#measureBuilt();
    this.hold(list);
    this.#grow();
  }

  // One idle step: the cards built grown towards ahead screens over and
  // under the cards in view, once fewer than half of that are built on
  // either side, and the ones further than twice that let go. So a hand
  // scrolling builds cards once in some screens, in one step of its own
  // between two wheel steps, and every wheel step between finds its
  // cards built. Each step builds and settles its cards, which asks for
  // the next.
  #grow() {
    if (this.#growing || !this.#box) return;
    this.#growing = true;
    const scrolls = this.#scrolls;
    this.#later(() => {
      this.#growing = false;
      if (!this.#box) return;
      // The column moved since: a hand is scrolling, and the step waits
      // for a frame it does not scroll in. WebKit, the app's engine on a
      // Mac, has no idle callback, so the step comes after the next frame
      // whatever happens, and a trackpad scrolls in every frame of a
      // fling. A step of 10 to 18 ms in the cloud landed in a scroll in
      // WebKit, found by the review.
      if (this.#scrolls !== scrolls) return this.#grow();
      const p = this.placed;
      const [a, b] = this.built;
      if (a > b) return;
      const from = this.#at - this.veil - this.lead;
      const reach = (screens: number) => span(p, from - screens * this.seen, from + (screens + 1) * this.seen);
      const [lo, hi] = reach(ahead);
      const [nearLo, nearHi] = reach(ahead / 2);
      const [farLo, farHi] = reach(2 * ahead);
      let [x, y] = [a, b];
      if (x > nearLo) x = Math.max(lo, x - step);
      else if (x < farLo) x = lo;
      if (y < nearHi) y = Math.min(hi, y + step);
      else if (y > farHi) y = hi;
      if (x === a && y === b) return;
      // Top comes along, so what is needed is worked out from where the
      // column is and not from where a scroll last needed other cards.
      // The page is built in this step, not in the frame the hand waits
      // on. Written before the flush: a write inside flushSync's callback
      // is not seen by a derived read outside an effect.
      this.#grown = { keys: p.keys, range: [x, y] };
      this.top = this.#at;
      flushSync();
    });
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
      if (this.#seenBuilt.has(el)) continue;
      const key = el.dataset.row!;
      const h = el.getBoundingClientRect().height;
      if (h === 0) continue;
      this.#seenBuilt.add(el);
      if (this.#measured.get(key) === h) continue;
      this.#measured.set(key, h);
      changed = true;
    }
    if (!changed) return;
    this.#heard++;
    this.hold();
  }

  // The column scrolled by the hand: where it is, and the card that is to
  // stay where it stands, the one preferred when it is in view, or else
  // the first card in view. The heights are the ones settle read as the
  // cards were built, so a scroll lays nothing out.
  //
  // A scroll that stays inside the cards built is not told to the page at
  // all: top changes only when other cards are needed. Every scroll told
  // to the page had Svelte look over everything that reads the cards
  // built, for each wheel step, which a list built whole never did.
  scrolled(list: unknown, prefer: string | null) {
    if (!this.#box) return;
    const t = (this.#at = this.#box.scrollTop);
    this.#scrolls++;
    const from = t - this.veil - this.lead;
    const [need0, need1] = span(this.placed, from - this.seen, from + 2 * this.seen);
    const [b0, b1] = this.built;
    if (need0 <= need1 && (need0 < b0 || need1 > b1)) this.top = t;
    this.#grow();
    this.keep(prefer, list);
  }

  // The card of a key kept where it stands as the cards around it change,
  // when it is in view, or else the first card in view. A card turned to
  // that was in view already is kept this way, so the list keeps no card
  // of the list before it: a search let go of by Escape kept the card of
  // the search, and the next cards measured put that card at the top of
  // the whole transcript, found by the review.
  keep(prefer: string | null, list: unknown) {
    if (!this.#box) return;
    this.#current = list;
    const t = this.#at;
    const from = t - this.veil - this.lead;
    const keys = this.placed.keys;
    const mine = prefer === null ? -1 : keys.indexOf(prefer);
    const at = (i: number) => this.veil + this.lead + this.placed.tops[i] - t;
    const inView = (i: number) => at(i) + this.placed.heights[i] > 0 && at(i) < this.seen;
    let i = mine >= 0 && inView(mine) ? mine : -1;
    if (i < 0) {
      const [first, last] = span(this.placed, from, from + this.seen);
      i = first <= last ? first : -1;
    }
    this.#anchor = i < 0 ? null : { key: keys[i], at: at(i), list };
  }

  // The card that is to stay where it stands put back there, after cards
  // came in or changed their height. Only in the list it was taken in.
  hold(list: unknown = this.#current) {
    const a = this.#anchor;
    if (!a || !this.#box || a.list !== list) return;
    const i = untrack(() => this.placed.keys.indexOf(a.key));
    if (i < 0) return;
    const at = a.foot ? this.#box.clientHeight - this.veil - untrack(() => this.placed.heights[i]) : a.at;
    const want = this.veil + this.lead + untrack(() => this.placed.tops[i]) - at;
    if (Math.abs(want - this.#box.scrollTop) < 0.5) return;
    this.#box.scrollTop = want;
    this.top = this.#at = this.#box.scrollTop;
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
  // what is in view, and stays there. The card is kept at by and not where
  // the scroll left it: the list on the page is as tall as the heights
  // said before this turn, and a card far down stood under the foot of a
  // column that could not yet scroll that far, and was kept there, out of
  // view. Settle puts it at by once the list is as tall as it is now, or
  // as far as the list goes, at its end.
  #toAt(key: string, by: number, list: unknown) {
    if (!this.#box) return;
    this.#current = list;
    this.#measureBuilt();
    const i = this.placed.keys.indexOf(key);
    if (i < 0) return;
    this.#box.scrollTop = this.veil + this.lead + this.placed.tops[i] - by;
    this.top = this.#at = this.#box.scrollTop;
    this.#anchor = { key, at: by, list };
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
    if (this.#anchor) this.#anchor.foot = true;
  }

  // The list at its top, a new list read from its first card.
  toStart(list: unknown) {
    if (!this.#box) return;
    this.#box.scrollTo({ top: 0 });
    this.top = this.#at = this.#box.scrollTop;
    this.#anchor = null;
    this.scrolled(list, null);
  }
}
