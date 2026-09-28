// What a search or a render says while it runs, in one place, so every row
// that shows one says it the same way. A search is hearing and then finding,
// a render is rendering, and each waits for its turn in the lane of its step
// first, see docs/JOBS.md. The words are the interface's own, one for each
// step, and never the engine's: the engine reports in its own words and its
// own case, part by part, and a row that passed them on said "loading the
// speech model", "Finding clips" and "2 of 12 found" in turn, in the row
// where the third clip was to appear.
import { clock, type Job, type Lane } from "./api";

// What waiting for each lane is waiting to do.
const waitingTo: Record<Lane, string> = {
  hearing: "Waiting to transcribe",
  finding: "Waiting to find clips",
  rendering: "Waiting to render",
};

const doing: Record<string, string> = {
  hearing: "Transcribing",
  finding: "Finding clips",
  rendering: "Rendering",
  // A clip made by hand, once it has heard what it needs.
  framing: "Placing the crop",
};

export type StepLine = {
  // The step, in the words above.
  what: string;
  // The time left, when the work can say, rounded to five seconds so it is
  // something to read rather than something that flickers.
  left: string;
  // How far the step has come, from 0 to 1, or -1 when it cannot say.
  fraction: number;
};

// The line for a job that runs. hearing is how far the episode has been heard
// towards the end of the search's window, from 0 to 1, because that is how
// far hearing has come for this search, whatever the speech model says about
// the whole episode.
export function stepLine(job: Job, hearing = -1): StepLine {
  const p = job.progress;
  const left = leftText(p?.remaining ?? 0);
  if (!job.step || job.step === "waiting") {
    return { what: waitingTo[job.lane] ?? "Waiting", left: "", fraction: -1 };
  }
  const what = doing[job.step] ?? sentence(job.label);
  // A search's hearing is how far the episode is heard towards its window,
  // given. Every other step, a clip made by hand's hearing included, is
  // how far the step itself has come.
  if (job.step === "hearing" && hearing >= 0) return { what, left, fraction: hearing };
  return { what, left, fraction: p && p.fraction >= 0 ? p.fraction : -1 };
}

// The time left, the way every row says it, or nothing when it is not known.
export function leftText(remaining: number): string {
  return remaining > 0 ? `About ${clock(Math.ceil(remaining / 5) * 5)} left` : "";
}

// A line of the engine's, where one is shown as it is, as a sentence: the
// engine writes its lines in lower case, the way a log does.
export function sentence(text: string): string {
  const said = text.trim();
  return said ? said[0].toUpperCase() + said.slice(1) : "";
}
