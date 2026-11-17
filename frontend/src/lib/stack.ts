// Where each card of a tall stack stands, so only the cards in view and a
// screen either side are built, and the rest are stood in for by their
// heights, issue #894. The transcript search of a video of four hours has
// over a thousand cards, and building, restyling and laying out every one
// of them took a fifth to a third of a second on opening the search, on
// its first letter and on clearing it.
//
// A card is the height it was last laid out at, once it has been. Before
// that it is guessed at the mean of the cards laid out so far. A guess
// that is off moves where the cards after it stand, never where a card
// lands when it is turned to: the space a card is stood in for by and the
// place it is turned to are worked out from the same heights.

export type Placed = {
  keys: readonly string[];
  // The top of each card, from the top of the first, gaps included.
  tops: Float64Array;
  heights: Float64Array;
  // From the top of the first card to the foot of the last.
  total: number;
};

// The height a card is guessed at before anything is laid out, about a
// card of two lines.
export const firstGuess = 64;

// Where the cards of keys stand, one after the other with gap between
// two, each at the height measured of it or at the guess.
export function place(keys: readonly string[], measured: ReadonlyMap<string, number>, gap: number): Placed {
  let sum = 0;
  for (const h of measured.values()) sum += h;
  const guess = measured.size ? sum / measured.size : firstGuess;
  const tops = new Float64Array(keys.length);
  const heights = new Float64Array(keys.length);
  let at = 0;
  for (const [i, key] of keys.entries()) {
    if (i > 0) at += gap;
    tops[i] = at;
    heights[i] = measured.get(key) ?? guess;
    at += heights[i];
  }
  return { keys, tops, heights, total: at };
}

// The first and the last card that reach into from to to, measured like
// the tops. A gap between two cards belongs to neither. Where no card
// reaches in, first is past last.
export function span(p: Placed, from: number, to: number): [number, number] {
  const n = p.tops.length;
  // The first card whose foot is below from.
  let lo = 0;
  let hi = n;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (p.tops[mid] + p.heights[mid] > from) hi = mid;
    else lo = mid + 1;
  }
  const first = lo;
  // The last card whose top is above to.
  lo = 0;
  hi = n;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (p.tops[mid] < to) lo = mid + 1;
    else hi = mid;
  }
  return [first, lo - 1];
}
