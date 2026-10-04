// The clips on their way into the list, from every job alike. A job says
// which clips it has on the way, see Underway, and the list shows each in
// its place, wearing what is being done to it, until it is written. A clip
// the model named and a clip made by hand with I or O come in the same way,
// because nothing here asks which it is.
import { clock, type Job, type JobStep } from "./api";
import { arrivalLine } from "./steps";

export type Arriving = {
  // The job's id and which of its clips this is, so it keeps its place
  // while its step and its edges change.
  key: string;
  // The job it is on the way from and which of its clips it is.
  job: string;
  n: number;
  // The key its clip will have in the list, once the engine has given it
  // an id. The list keeps one row from the card to the clip by it.
  clip?: string;
  // What of it is kept, once its pauses are cut.
  pieces?: [number, number][];
  // Where it lies, which is where it goes in the list, and where it ends.
  // Until its sentences are known the two are the same moment.
  start: number;
  end: number;
  // What it is called, once that is known.
  title: string;
  // What is being done to it, in the engine's word and in the words shown,
  // how long is left, and how far it has come.
  step?: JobStep;
  what: string;
  left: string;
  fraction: number;
  still?: boolean;
  // A clip on the way whose job stopped before it was written: called off
  // with Cancel, cut off by the app closing, or failed. It stays where it
  // would have appeared and says so there, and a click on it, or Continue
  // at the head of the list, carries it on.
  stopped?: boolean;
  full?: string;
  oncontinue?: () => void;
};

// arriving is every clip the jobs of an episode have on the way. A job
// that runs shows its clips at work. One that was cut off or failed shows
// the clip it was making where it would have appeared, until it is
// carried on or put away in Activity. stopping is the jobs told to stop
// that have not said so yet, which keep their fill and stop moving.
export function arriving(
  jobs: Job[],
  stopping: (job: Job) => boolean,
  carryOn: (job: Job) => void,
): Arriving[] {
  const out: Arriving[] = [];
  for (const job of jobs) {
    const running = job.state === "running" || job.state === "queued";
    const stopped = job.state === "interrupted" || job.state === "failed";
    if (!running && !stopped) continue;
    for (const clip of job.underway ?? []) {
      const key = `${job.id}/${clip.n}`;
      if (stopped) {
        const failed = job.state === "failed";
        out.push({
          key,
          job: job.id,
          n: clip.n,
          start: clip.start,
          end: clip.end,
          title: clip.title ?? "",
          // In the words a search that stopped uses, in its row.
          what: failed
            ? "Failed. Click Continue"
            : job.step === "stopped"
              ? "Stopped. Click Continue"
              : "Interrupted. Click Continue",
          left: failed && job.error ? job.error : clock(clip.start),
          fraction: -1,
          stopped: true,
          full: failed ? job.error : undefined,
          oncontinue: () => carryOn(job),
        });
        continue;
      }
      const line = arrivalLine(job, clip);
      out.push({
        key,
        job: job.id,
        n: clip.n,
        pieces: clip.pieces,
        clip: clip.clip,
        start: clip.start,
        end: clip.end,
        title: clip.title ?? "",
        step: clip.step,
        what: stopping(job) ? "Stopping" : line.what,
        left: line.left,
        fraction: line.fraction,
        still: stopping(job),
      });
    }
  }
  return out;
}

// A card on the way, or one whose clip was just written and that no read
// of the list holds yet.
export type OnTheWayCard = Arriving & { held?: boolean };

// The cards the list shows on the way: every card the jobs have on the
// way, and every card that left that list without being stopped, until a
// read of the clip list asked after it left has answered. That read holds
// its clip, so the card hands over to it in one step.
//
// It is worked out in the same pass that builds the list, never after it.
// An effect held the card one render late, so for that render the card was
// gone, a row still to come stood in its place, and then the card slid
// back open before it became its clip: card, gap, card, clip, for every
// clip, which at the end of a search read as the whole list blinking.
export class OnTheWay {
  private live = new Map<string, Arriving>();
  private held = new Map<string, Arriving & { after: number }>();

  // now is what the jobs have on the way, next the ticket the next read of
  // the list will get, and read the ticket of the newest read kept.
  cards(now: Arriving[], next: number, read: number): OnTheWayCard[] {
    const here = new Set(now.map((a) => a.key));
    for (const [key, a] of this.live) {
      if (!here.has(key) && !a.stopped && !this.held.has(key)) this.held.set(key, { ...a, after: next });
    }
    for (const key of here) this.held.delete(key);
    for (const [key, a] of this.held) if (a.after <= read) this.held.delete(key);
    this.live = new Map(now.map((a) => [a.key, a]));
    return [...now, ...[...this.held.values()].map(({ after: _, ...a }) => ({ ...a, held: true }))];
  }

  // Whether a card is held that no read has been asked for since it left.
  unread(next: number): boolean {
    return [...this.held.values()].some((a) => a.after >= next);
  }
}

// Which one card of a search wears the search's fill: how far the whole
// search has come and, once no row is left to say it, the time left. One
// card, never all of them: every card of the search wore it once, three
// fills and three "About 0:10 left" side by side for one piece of work.
// Every card keeps its beam, because each is still being worked on. It is
// the first of the search's cards in the list, and it keeps the fill for
// as long as it is on the way, so a card named above it does not take it
// away. A card whose clip is written has done its work and passes it on
// to the first card left, so the fill starts at the first card of the
// batch and ends at the last.
//
// First is by where a card stands in the list, which place says: its
// moment in the episode unless the list keeps the search's cards in the
// places they came into, see ClipList.
export class Carrier {
  private key = "";

  pick(cards: OnTheWayCard[], job: string, place: (a: OnTheWayCard) => number = (a) => a.start): string {
    const own = cards.filter((a) => a.job === job && !a.stopped && !a.held);
    if (own.some((a) => a.key === this.key)) return this.key;
    let first: OnTheWayCard | undefined;
    for (const a of own) if (!first || place(a) < place(first)) first = a;
    this.key = first?.key ?? "";
    return this.key;
  }
}
