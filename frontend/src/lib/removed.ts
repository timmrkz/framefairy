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
