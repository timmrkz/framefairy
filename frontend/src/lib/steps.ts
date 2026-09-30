// What a search or a render says while it runs, in one place, so every row
// that shows one says it the same way. A search is hearing and then finding,
// a render is rendering, and each waits for its turn in the lane of its step
// first, see docs/JOBS.md. The words are the interface's own, one for each
// step, and never the engine's: the engine reports in its own words and its
// own case, part by part, and a row that passed them on said "loading the
// speech model", "Finding clips" and "2 of 12 found" in turn, in the row
// where the third clip was to appear.
import { clock, type Job, type Lane, type Underway } from "./api";

// What waiting for each lane is waiting to do.
const waitingTo: Record<Lane, string> = {
  hearing: "Waiting to transcribe",
  finding: "Waiting to find clips",
  rendering: "Waiting to render",
  framing: "Waiting to place the crop",
};

const doing: Record<string, string> = {
  hearing: "Transcribing",
  finding: "Finding clips",
  rendering: "Rendering",
  framing: "Placing the crop",
  // A clip of a search's answer well off the length, asked for again.
  fitting: "Fitting to the length",
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
  const left = p && p.remaining > 0 ? `About ${clock(Math.ceil(p.remaining / 5) * 5)} left` : "";
  if (!job.step || job.step === "waiting") {
    return { what: waitingTo[job.lane] ?? "Waiting", left: "", fraction: -1 };
  }
  const what = doing[job.step] ?? sentence(job.label);
  if (job.step === "hearing") return { what, left, fraction: hearing };
  return { what, left, fraction: p && p.fraction >= 0 ? p.fraction : -1 };
}

// The line for a clip on its way, the same for every clip whichever job
// has it: the step the clip itself is in, which for a search's clip is
// placing its crop while the search goes on finding. A clip made by hand
// says how far its own work has come: the words at the playhead heard,
// then its crop placed.
export function arrivalLine(job: Job, clip: Underway): StepLine {
  // The job's progress is the clip's own when the job is the one clip. A
  // search's is the whole search's, and its clips on their way are not
  // that far along, see Carrier.
  if (job.kind === "clip") return stepLine({ ...job, step: clip.step }, job.progress?.fraction ?? -1);
  return { ...stepLine({ ...job, step: clip.step }), left: "", fraction: -1 };
}

// A line of the engine's, where one is shown as it is, as a sentence: the
// engine writes its lines in lower case, the way a log does.
export function sentence(text: string): string {
  const said = text.trim();
  return said ? said[0].toUpperCase() + said.slice(1) : "";
}
