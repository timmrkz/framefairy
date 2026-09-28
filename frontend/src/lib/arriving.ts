// The clips on their way into the list, from every job alike. A job says
// which clips it has on the way, see Underway, and the list shows each in
// its place, wearing what is being done to it, until it is written. A clip
// the model named and a clip made by hand with I or O come in the same way,
// because nothing here asks which it is.
import { clock, type Job } from "./api";
import { arrivalLine } from "./steps";

export type Arriving = {
  // The job's id and which of its clips this is, so it keeps its place
  // while its step and its edges change.
  key: string;
  // Where it lies, which is where it goes in the list.
  start: number;
  // What it is called, once that is known.
  title: string;
  // What is being done to it, how long is left, and how far it has come.
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
          start: clip.start,
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
        start: clip.start,
        title: clip.title ?? "",
        what: stopping(job) ? "Stopping" : line.what,
        left: line.left,
        fraction: line.fraction,
        still: stopping(job),
      });
    }
  }
  return out;
}
