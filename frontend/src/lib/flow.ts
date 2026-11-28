// What the workspace decides by itself, in one place and away from the
// screen, so it can be tested. When a search starts and stops is not among
// it: that is the Go side's, see docs/JOBS.md, and its tests are the path
// tests in cmd/framefairy-app.

import type { Job } from "./api";

// A piece of a clip, in the episode's own seconds.
export type Piece = { start: number; end: number };

// Parts of the episode, from and to in seconds, in order and apart. The
// transcript is heard in parts, wherever a search or a clip made by hand
// needs it first, see docs/ENGINE.md, and the Go side says which.
export type Parts = [number, number][];

// A run of asks where only the newest answer counts.
//
// Several asks can be in the air at once, the clip list read again while
// a search lands its clips, the words of a clip while the arrow keys walk
// the list, and nothing says they come back in the order they went out:
// an answer that took longer lands after a newer one and paints over it,
// and what is on screen is then of something the hand has left. It stays
// wrong, because nothing asks again.
//
// Every ask takes a ticket. An answer is used only if no newer answer has
// been used already.
export class Newest {
  private sent = 0;
  private used = 0;

  // The ticket for an ask about to be sent.
  send(): number {
    return ++this.sent;
  }

  // The ticket the next ask will get, so something can wait for the
  // first answer asked from now on.
  next(): number {
    return this.sent + 1;
  }

  // Whether this answer is still the newest one to arrive. Asking marks it
  // used either way, so an answer older than this one is never taken after
  // it, including the answer to an ask that failed.
  keep(ticket: number): boolean {
    if (ticket <= this.used) return false;
    this.used = ticket;
    return true;
  }
}

// A job as the Go side reports it, by its id and the number of its last
// change. The number grows with every change to any job, and is set where
// the change is made, so of two snapshots of one job the later one has the
// larger number. News is sent after the change is made, so two changes at
// nearly the same moment can arrive the other way round, and a list that
// kept whatever it heard last showed a finished job as waiting, for good.
export type Stamped = { id: string; seq?: number };

// The list with got in it, or null when the list already has a later
// snapshot of that job. A snapshot without a number is taken as it comes.
export function mergeJob<T extends Stamped>(list: T[], got: T): T[] | null {
  const i = list.findIndex((j) => j.id === got.id);
  if (i < 0) return [...list, got];
  if ((got.seq ?? 0) < (list[i].seq ?? 0)) return null;
  const out = list.slice();
  out[i] = got;
  return out;
}

// A job waiting its turn or running.
export const live = (j: Job | undefined) => j?.state === "running" || j?.state === "queued";

// A job that ended before it was done, interrupted, stopped or failed,
// which says so where its work was until Continue carries it on.
export const unfinished = (j: Job | undefined) =>
  j?.state === "interrupted" || j?.state === "stopped" || j?.state === "failed";

// A job that still has something to say: on its way, or ended before it
// was done and not settled yet. One done, cancelled or settled has not.
export const outstanding = (j: Job | undefined) => live(j) || unfinished(j);

// Where a moment of a clip falls in the episode.
//
// The cuts of a clip, the parts it leaves out: the gap between two pieces
// of two parts, see PieceView.Part in engine/shape.go. Two pieces of one
// part, which a crop moved part way through makes, meet with no cut
// between them. The engine counts cuts from the first piece, and so does
// this, because that is what a move names.
export function cutsOf(pieces: (Piece & { part: number })[]): { index: number; from: number; to: number }[] {
  return pieces
    .slice(1)
    .map((p, i) => ({ index: i, from: pieces[i].end, to: p.start, apart: p.part !== pieces[i].part }))
    .filter((c) => c.apart)
    .map(({ index, from, to }) => ({ index, from, to }));
}

// A caption edge being dragged, on the episode's clock, where the hand has
// it, and whether shift puts it on words. Where it lands and what it does to
// the words is the engine's to say, see ShapeCaptionTime in
// engine/episode.go.
export interface CaptionDraft {
  index: number;
  edge: "start" | "end";
  at: number;
  toWords: boolean;
}
