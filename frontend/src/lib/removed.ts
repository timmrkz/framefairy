// A clip taken out can be put back for a few seconds. The time is the
// episode's, not the screen's: it runs on while another episode or another
// page is open, and a clip removed a moment before is still there to put
// back on coming back. Leaving the episode used to take every way back with
// it, however much of its time was left. It is kept in memory only, because
// it is seconds long and a restart is well past it.
import type { ClipEntry } from "./api";

// How long a clip taken out can be put back, in seconds.
export const secondThoughts = 10;

// A clip taken out, the moment its time runs out, in milliseconds since
// the epoch, the way Date.now() counts, and whether it was the clip chosen
// when it was taken out. If it was, then is the clip chosen in its place,
// empty when none was left, and at is where the playhead stood, so
// putting it back puts both back.
export type Removed = { clip: ClipEntry; until: number; chosen?: boolean; then?: string; at?: number };

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
// it does not stand there until the next search. Its time runs on while
// another episode is open, the way a removed clip's does, so coming back
// does not say it again from the start. By the search's job: the moment
// its row closes, in milliseconds since the epoch, and 0 once it has.
export type Told = Record<string, number>;

const toldAside = new Map<string, Told>();

// The rows an episode leaves the screen with.
export function setAsideTold(path: string, told: Told) {
  if (Object.keys(told).length > 0) toldAside.set(path, told);
  else toldAside.delete(path);
}

// The rows an episode comes back to. Those whose time ran out while it was
// away are closed, so they do not flash up and close again.
export function takeUpTold(path: string, now = Date.now()): Told {
  const kept = toldAside.get(path) ?? {};
  toldAside.delete(path);
  return Object.fromEntries(Object.entries(kept).map(([job, until]) => [job, until > now ? until : 0]));
}

// How much of its time a row saying a search found nothing has used, in
// seconds. A row whose time has not started has used none.
export function toldSpent(until: number | undefined, now = Date.now()): number {
  if (!until) return 0;
  return Math.min(secondThoughts, Math.max(0, secondThoughts - (until - now) / 1000));
}
