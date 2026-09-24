// Shared state: the job list, kept current from the Go side's events.
import { api, onJob, type Job, type EngineEvent } from "./api";
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

  active(path: string, lane?: "transcribe" | "work"): Job | undefined {
    return this.list.find(
      (j) =>
        j.episode === path &&
        (j.state === "running" || j.state === "queued") &&
        (!lane || j.lane === lane),
    );
  }

  get busy(): number {
    return this.list.filter((j) => j.state === "running" || j.state === "queued").length;
  }

  async clear() {
    await api.clearJobs();
    this.list = this.list.filter((j) => j.state === "running" || j.state === "queued");
  }
}

export const jobs = new JobStore();

export type View =
  | { name: "episode"; path: string }
  | { name: "jobs" }
  | { name: "settings" }
  | { name: "acknowledgements" }
  | { name: "empty" };

class Nav {
  view = $state<View>({ name: "empty" });
  go(v: View) {
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
  // Whether the app has already looked for clips by itself for an episode.
  // Once for each one while the app runs, so coming back to a workspace
  // never starts a second search of its own.
  looked = $state<Record<string, boolean>>({});
  // Whether the model has been loaded ahead of that first search.
  warmed = $state<Record<string, boolean>>({});
  // Where the transcription was told to stop for that first search.
  held = $state<Record<string, number>>({});

  keep(path: string, from: number, to: number) {
    this.windows[path] = { from, to };
  }

  // Forgets an episode that was removed.
  forget(path: string) {
    delete this.windows[path];
    delete this.looked[path];
    delete this.warmed[path];
    delete this.held[path];
  }

  of(path: string, duration: number): { from: number; to: number } | null {
    const w = this.windows[path];
    if (!w || w.to <= w.from || w.to > duration + 0.5) return null;
    return w;
  }
}

export const chosen = new Chosen();
