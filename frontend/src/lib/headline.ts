// What the row of a search says, held long enough to be read. The engine
// reports twice a second and a search goes through its steps faster than
// anyone reads, so a new headline waits until the one before has been
// there for HOLD. The fill and the time left follow at once, because
// they are the same thing moving on. What a click brings about shows at
// once, see atOnce. The time of day is handed in, so a test moves it.

export type Headline = { what: string; left: string; fraction: number; still?: boolean };
// What the work says now, and whether it is to show at once.
export type Wanted = Headline & { now?: boolean };
// The headline on screen, and since when it has said what it says.
export type Held = { shown: Headline | null; since: number };

// How long a headline stands before the next one takes its place, in ms.
export const HOLD = 1000;

// What the row shows the moment the work changes: work starting or
// ending, and a headline marked now, go straight in. Anything else waits
// for the next tick. The same held when nothing changes.
export function atOnce(held: Held, want: Wanted | null, now: number): Held {
  const { shown } = held;
  if (want && (!shown || (want.now && (want.what !== shown.what || !!want.still !== !!shown.still)))) {
    return { shown: { what: want.what, left: want.left, fraction: want.fraction, still: want.still }, since: now };
  }
  if (!want && shown) return { shown: null, since: held.since };
  return held;
}

// What the row shows on a tick of its own time: a headline with nothing
// new to say stays, one that says the same moves its fill on, and a new
// one waits until the one before has stood for HOLD. Meanwhile the fill
// goes on, and never back.
export function onTick(held: Held, want: Wanted | null, now: number): Held {
  const { shown } = held;
  if (!want) return { shown: null, since: held.since };
  if (!want.what && shown) return held;
  const what = want.what;
  if (!shown || what === shown.what) return { shown: { ...want, what }, since: shown ? held.since : now };
  if (now - held.since < HOLD) {
    return { shown: { ...shown, left: want.left, fraction: Math.max(shown.fraction, want.fraction) }, since: held.since };
  }
  return { shown: { ...want, what }, since: now };
}
