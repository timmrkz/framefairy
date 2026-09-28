// What a search or a render says while it runs, in one place, so every row
// that shows one says it the same way. A search is hearing and then finding,
// a render is rendering, and each waits for its turn in the lane of its step
// first, see docs/JOBS.md. The words are the interface's own, one for each
// step, and never the engine's: the engine reports in its own words and its
// own case, part by part, and a row that passed them on said "loading the
// speech model", "Finding clips" and "2 of 12 found" in turn, in the row
// where the third clip was to appear.
import { clock, type Job, type Lane } from "./api";
import { waitShare, type Span } from "./flow";

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

// How long a part of the episode is, the way a row says it.
export function lengthText(seconds: number): string {
  const s = Math.max(0, seconds);
  if (s < 120) return `${Math.round(s)} s`;
  const minutes = Math.round(s / 60);
  if (minutes < 120) return `${minutes} min`;
  return `${Math.floor(minutes / 60)} h ${minutes % 60} min`;
}

// The part of the episode a piece of work is about, from where to where and
// how long: a search's window, or what a clip made by hand needs heard.
export function partText(part: Span): string {
  return `${clock(part.from)} to ${clock(part.to)}, ${lengthText(part.to - part.from)}`;
}

// The row of a clip on its way. A search's next clip and a clip made by
// hand are the same thing, so they are one row fed by these two functions
// and nothing else: the same words, the same fill, the same time left, the
// same part of the episode when there is no time left to say.
export type WayLine = StepLine & {
  // Told to stop and not stopped yet: it keeps its fill and stands still.
  still?: boolean;
  // Stopped, cut off or failed, with Continue.
  stopped?: boolean;
  // Shown in the frame it is asked for, not held for reading, because a
  // click brought it about.
  now?: boolean;
};

// What a step of it is doing, in the headline. Transcribing says how much
// of the episode it transcribes, so thirty minutes of a window and two of a
// clip made by hand read as the same thing at two sizes.
export function wayWhat(step: string, part: Span, what = doing[step] ?? ""): string {
  return step === "hearing" ? `${what} ${lengthText(part.to - part.from)}` : what;
}

// Under the headline: the time left when the work can say, and where its
// part lies in the episode when it cannot.
export function wayLeft(left: string, part: Span): string {
  return left || `${clock(part.from)} to ${clock(part.to)}`;
}

// While the job runs. heard is how far the episode is heard, and while it
// hears, the fill is how much of its part that covers, empty at the part's
// start and full at its end, the way the range picker draws it.
export function runningLine(job: Job, part: Span, heard: number, stopping: boolean): WayLine {
  const line = stepLine(job, waitShare(part.from, heard, part.to));
  const left = wayLeft(line.left, part);
  if (stopping) return { what: "Stopping", left, fraction: line.fraction, still: true, now: true };
  return { ...line, what: wayWhat(job.step ?? "", part, line.what), left };
}

// Clicked, and the job not there yet: the row already says the step the
// work is about to take, so Continue goes from Stopped straight to it.
export function startingLine(step: string, part: Span, fraction: number): WayLine {
  return { what: wayWhat(step, part), left: wayLeft("", part), fraction, now: true };
}

// Once it stopped: called off with Cancel, cut off by the app closing, or
// failed. The same row either way, with Continue, because what it did
// stays. Only the first word says which it was.
export function stoppedLine(job: Job, part: Span): WayLine {
  if (job.state === "failed") {
    const why = sentence(job.error ?? "") || "No reason was given";
    return { what: "Failed. Click Continue", left: why, fraction: -1, stopped: true };
  }
  const what = job.step === "stopped" ? "Stopped. Click Continue" : "Interrupted. Click Continue";
  return { what, left: partText(part), fraction: -1, stopped: true };
}

// What a row shows, held long enough to be read. The engine reports twice
// a second and work goes through its steps faster than anyone reads, so a
// new headline waits until the one before has been there a second. The
// fill and the time left follow at once, because they are the same thing
// moving on. Work starting or ending, and a line marked now, go straight
// in: what a click brings about shows in the frame of the click.
export type Held = { line: WayLine | null; since: number };

// When what the row wants changes. Anything else waits for heldLater.
export function heldAtOnce(held: Held, want: WayLine | null, now: number): Held {
  if (!want) return held.line ? { line: null, since: held.since } : held;
  const shown = held.line;
  if (!shown || (want.now && (want.what !== shown.what || !!want.still !== !!shown.still))) {
    return { line: want, since: now };
  }
  return held;
}

// On a timer, a few times a second.
export function heldLater(held: Held, want: WayLine | null, now: number): Held {
  const shown = held.line;
  if (!want) return { line: null, since: held.since };
  // Nothing new to say: the row stays as it is.
  if (!want.what && shown) return held;
  if (!shown) return { line: want, since: now };
  if (want.what === shown.what) return { line: want, since: held.since };
  // A step that lasted a moment is not flashed up.
  if (now - held.since < 1000) {
    return {
      line: { ...shown, left: want.left, fraction: Math.max(shown.fraction, want.fraction) },
      since: held.since,
    };
  }
  return { line: want, since: now };
}
