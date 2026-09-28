// Shared state: the job list, kept current from the Go side's events.
import { api, onJob, type Job, type EngineEvent, type Lane } from "./api";
import { mergeJob } from "./flow";

// How often the list is read again while anything runs. The events keep it
// current, and this is what puts it right when one of them was lost: a job
// that looks as if it runs for ever holds up the first search and says
// Cancelling for good, and only a restart cleared it.
const recheck = 5000;

class JobStore {
  list = $state<Job[]>([]);
  log = $state<Record<string, EngineEvent[]>>({});

  // A snapshot of a job, kept unless the list already has a later one.
  apply(job: Job) {
    const next = mergeJob(this.list, job);
    if (next) this.list = next;
  }

  async start() {
    // Listening starts before the list is read, so nothing that happens
    // while it is read is missed. What was heard in the meantime and what
    // the list says are merged by their numbers.
    onJob(({ job, event }) => {
      this.apply(job);
      if (event && event.kind !== "progress" && event.kind !== "idle") {
        const lines = this.log[job.id] ?? [];
        this.log[job.id] = [...lines.slice(-199), event];
      }
    });
    await this.resync();
    setInterval(() => {
      if (this.busy > 0) void this.resync();
    }, recheck);
  }

  async resync() {
    try {
      for (const job of (await api.jobs()) ?? []) this.apply(job);
    } catch {
      // The Go side will answer next time. Nothing here is worth an error.
    }
  }

  forEpisode(path: string): Job[] {
    return this.list.filter((j) => j.episode === path);
  }

  active(path: string, lane?: Lane): Job | undefined {
    return this.list.find(
      (j) =>
        j.episode === path &&
        (j.state === "running" || j.state === "queued") &&
        (!lane || j.lane === lane),
    );
  }

  // The episode's search, the newest there is: running, or stopped with
  // something to say, cut off or failed. One called off by hand or done
  // has nothing to say, and is not it.
  search(path: string): Job | undefined {
    return this.latest(path, "search");
  }

  // The same for the episode's clip made by hand: running, or stopped with
  // Continue.
  hand(path: string): Job | undefined {
    return this.latest(path, "hand");
  }

  private latest(path: string, kind: Job["kind"]): Job | undefined {
    const last = this.list.findLast((j) => j.episode === path && j.kind === kind);
    if (!last || last.state === "done" || last.state === "cancelled") return undefined;
    return last;
  }

  get busy(): number {
    return this.list.filter((j) => j.state === "running" || j.state === "queued").length;
  }

  async clear() {
    await api.clearJobs();
    this.list = this.list.filter((j) => j.state === "running" || j.state === "queued" || stays(j));
  }
}

export const jobs = new JobStore();

// A search or a render that stopped, cut off or failed, and has not been
// carried on or called off yet. It is not finished, so clearing the
// finished leaves it: it says so where its work was until it is acted on.
export function stays(j: Job): boolean {
  return j.state === "interrupted" || (j.state === "failed" && !!j.record);
}

export type View =
  | { name: "episode"; path: string }
  | { name: "jobs" }
  | { name: "settings" }
  | { name: "updates" }
  | { name: "acknowledgements" }
  | { name: "empty" };

class Nav {
  view = $state<View>({ name: "empty" });
  // A page that must not be left yet. It is asked before every move to
  // another page, and a page that says so keeps the app where it is and
  // shows what has to be put right first. The settings hold the app while
  // what finds the clips cannot, because every search after would fail
  // somewhere far from the one place it can be fixed.
  hold: ((to: View) => boolean) | null = null;
  go(v: View) {
    if (this.hold && v.name !== this.view.name && this.hold(v)) return;
    this.view = v;
  }
}

export const nav = new Nav();

// How the app itself is arranged. It is a view preference, not anything
// about an episode, so it lives in the webview and not in the settings file.
class Shell {
  pinned = $state(read());

  set(open: boolean) {
    this.pinned = open;
    try {
      localStorage.setItem("sidebar", open ? "open" : "closed");
    } catch {
      // A webview without storage simply forgets the choice.
    }
  }
}

function read(): boolean {
  try {
    return localStorage.getItem("sidebar") !== "closed";
  } catch {
    return true;
  }
}

export const shell = new Shell();

// The window chosen on the range picker, for as long as the app runs.
// Going to the activity page and coming back is no reason to lose it, and
// neither is looking at another episode in between.
class Chosen {
  windows = $state<Record<string, { from: number; to: number }>>({});

  keep(path: string, from: number, to: number) {
    this.windows[path] = { from, to };
  }

  // Forgets an episode that was removed.
  forget(path: string) {
    delete this.windows[path];
  }

  of(path: string, duration: number): { from: number; to: number } | null {
    const w = this.windows[path];
    if (!w || w.to <= w.from || w.to > duration + 0.5) return null;
    return w;
  }
}

export const chosen = new Chosen();
