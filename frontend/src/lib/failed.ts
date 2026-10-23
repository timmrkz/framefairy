// Which failures of a search or of a clip made by hand with I or O say why
// in a toast: those of jobs seen running while the app is open, and of
// clips asked for here, each once, issue #450. One seen failed only as the
// app opened said why already, the time it failed.
//
// What the Go side says of a job travels apart from the answer to the call
// that asked for it, so a clip that fails at once can be failed in the
// store before its call has answered, and the failure is said from
// whichever comes. Nothing is kept of a job that ended any other way.
import type { Job } from "./api";

type Seen = Pick<Job, "id" | "state">;

export class Failures<J extends Seen> {
  #running = new Set<string>();
  #told = new Set<string>();
  #say: (job: J) => void;

  constructor(say: (job: J) => void) {
    this.#say = say;
  }

  // A job as the store shows it now.
  seen(job: J) {
    if (job.state === "running" || job.state === "queued") {
      this.#running.add(job.id);
      return;
    }
    if (this.#running.delete(job.id) && job.state === "failed") this.#tell(job);
  }

  // A job asked for here, as the store has it once the call has answered.
  asked(job: J) {
    if (job.state === "failed") this.#tell(job);
    else this.seen(job);
  }

  #tell(job: J) {
    if (this.#told.has(job.id)) return;
    this.#told.add(job.id);
    this.#say(job);
  }
}
