// A clip taken out can be put back for a few seconds. The time is the
// episode's, not the screen's: it runs on while another episode or another
// page is open, and a clip removed a moment before is still there to put
// back on coming back. Leaving the episode used to take every way back with
// it, however much of its time was left. It is kept in memory only, because
// it is seconds long and a restart is well past it.
import type { ClipEntry } from "./api";

// How long a clip taken out can be put back, in seconds.
export const secondThoughts = 10;

// A clip taken out, and the moment its time runs out, in milliseconds
// since the epoch, the way Date.now() counts.
export type Removed = { clip: ClipEntry; until: number };

const aside = new Map<string, Record<string, Removed>>();

// The clips an episode leaves the screen with.
export function setAside(path: string, removed: Record<string, Removed>) {
  if (Object.keys(removed).length > 0) aside.set(path, removed);
  else aside.delete(path);
}

// The clips an episode comes back to: the ones whose time has not run out.
export function takeUp(path: string, now = Date.now()): Record<string, Removed> {
  const kept = aside.get(path) ?? {};
  aside.delete(path);
  return Object.fromEntries(Object.entries(kept).filter(([, r]) => r.until > now));
}

// How much of its time a clip taken out has used, in seconds.
export function spent(r: Removed, now = Date.now()): number {
  return Math.min(secondThoughts, Math.max(0, secondThoughts - (r.until - now) / 1000));
}

// A search that found nothing new says so in the row its next clip would
// have appeared in, for as long as a clip taken out can be put back, and
// then the row closes. It is a piece of news, not a state to report, so
// it does not stand there until the next search. Its time is kept by the
// search's job and runs on while another episode is open, the way a
// removed clip's does, so coming back does not say it again from the
// start. In milliseconds since the epoch, and 0 once the row has closed.
const told = new Map<string, number>();

// When the row saying a search found nothing closes. The first time it is
// asked for starts its time.
export function toldUntil(job: string, now = Date.now()): number {
  let until = told.get(job);
  if (until === undefined) {
    until = now + secondThoughts * 1000;
    told.set(job, until);
  }
  return until;
}

// The row was held under the pointer and runs on with this many seconds
// left.
export function toldHeld(job: string, left: number, now = Date.now()) {
  if (told.get(job)) told.set(job, now + left * 1000);
}

// The row's time ran out and it has closed.
export function toldOver(job: string) {
  told.set(job, 0);
}

// Whether the row has closed. A row held under the pointer past its time
// stays open, it closes when its time runs out after it is let go.
export function toldClosed(job: string): boolean {
  return told.get(job) === 0;
}

// The rows whose time ran out while their episode was not open, closed
// before the episode is shown again.
export function toldSweep(now = Date.now()) {
  for (const [job, until] of told) if (until > 0 && until <= now) told.set(job, 0);
}
