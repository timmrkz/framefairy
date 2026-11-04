// The clips on their way into the list, from every job alike. A job says
// which clips it has on the way, see Underway, and the list shows each in
// its place, wearing what is being done to it, until it is written. A clip
// the model named and a clip made by hand with I or O come in the same way,
// because nothing here asks which it is.
import { clock, type Job, type JobStep } from "./api";
import { live, unfinished } from "./flow";
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
  // Stopped because it failed, which is said in the red of an error, the
  // way the sidebar and the toast say it. Called off or cut off is said in
  // the colour of a warning, because nothing went wrong.
  failed?: boolean;
  // Made by hand with I and O, rather than on the way from a search.
  byHand?: boolean;
  full?: string;
  oncontinue?: () => void;
  // Removes the work that stopped, so it is not carried on: a clip made by
  // hand, issue #257, or the search the clip was on the way from, with
  // every clip it had on the way and the row that says how it ended,
  // issue #431.
  onremove?: () => void;
  // The removal under way: the card says Removed and offers the clip back
  // while its time runs down, see Removed.
  away?: Away;
};

// A card being removed: how much of its time it has used, in seconds,
// what putting it back does, and what a hand on it holding its time and
// letting it go does. The time is the Go side's, which forgets the work
// once it is up, see removal.go, so the card only draws it. together
// names the work it is removed with, so every card of it and its row hold
// and run down as one, see Busy.
export type Away = {
  spent: number;
  together?: string;
  onputback: () => void;
  onhold: (held: boolean) => void;
};

// What the episode does with work that stopped, a search or a clip made
// by hand: remove it, and say how its removal stands. Work that could not
// be removed is Stopped again, and why is a toast.
export type Hand = {
  remove: (job: Job) => void;
  away: (job: Job) => Away | undefined;
};

// arriving is every clip the jobs of an episode have on the way. A job
// that runs shows its clips at work. One that was cut off or failed shows
// the clip it was making where it would have appeared, until it is
// carried on, a new search takes its place, or it is removed, see Hand. stopping is the jobs told to stop that have not said so yet,
// which keep their fill and stop moving.
export function arriving(
  jobs: Job[],
  stopping: (job: Job) => boolean,
  carryOn: (job: Job) => void,
  hand?: Hand,
): Arriving[] {
  const out: Arriving[] = [];
  for (const job of jobs) {
    const running = live(job);
    const stopped = unfinished(job);
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
            : job.state === "stopped"
              ? "Stopped. Click Continue"
              : "Interrupted. Click Continue",
          // Where it lies, never why it failed: why is said whole in a
          // toast as it fails, see Episode, and cut to a line here it was
          // mostly unreadable, issue #450.
          left: clock(clip.start),
          fraction: -1,
          stopped: true,
          failed,
          byHand: job.kind === "clip",
          full: failed ? job.error : undefined,
          oncontinue: () => carryOn(job),
          onremove: hand ? () => hand.remove(job) : undefined,
          away: hand?.away(job),
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

// A card on the way, as the newest state of the video kept has it. The
// card and the clip it becomes come in the same state, so the list hands
// one over to the other in one step, see EpisodeSession.jobs.
export type OnTheWayCard = Arriving;

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
    const own = cards.filter((a) => a.job === job && !a.stopped);
    if (own.some((a) => a.key === this.key)) return this.key;
    let first: OnTheWayCard | undefined;
    for (const a of own) if (!first || place(a) < place(first)) first = a;
    this.key = first?.key ?? "";
    return this.key;
  }
}
