// Shared state: the job list, kept current from the Go side's events.
import { api, onJob, type Job, type EngineEvent } from "./api";

class JobStore {
  list = $state<Job[]>([]);
  log = $state<Record<string, EngineEvent[]>>({});

  async start() {
    this.list = (await api.jobs()) ?? [];
    onJob(({ job, event }) => {
      const i = this.list.findIndex((j) => j.id === job.id);
      if (i >= 0) this.list[i] = job;
      else this.list = [...this.list, job];
      if (event && event.kind !== "progress" && event.kind !== "idle") {
        const lines = this.log[job.id] ?? [];
        this.log[job.id] = [...lines.slice(-199), event];
      }
    });
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
  | { name: "empty" };

class Nav {
  view = $state<View>({ name: "empty" });
  go(v: View) {
    this.view = v;
  }
}

export const nav = new Nav();

// How the window itself is arranged. It is a view preference, not anything
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

// The stretch chosen on the range picker, for as long as the app runs.
// Going to the activity page and coming back is no reason to lose it, and
// neither is looking at another episode in between.
class Chosen {
  windows = $state<Record<string, { from: number; to: number }>>({});
  // Whether the app has already looked for clips by itself for an episode.
  // Once for each one while the app runs, so coming back to a workspace
  // never starts a second search of its own.
  looked = $state<Record<string, boolean>>({});

  keep(path: string, from: number, to: number) {
    this.windows[path] = { from, to };
  }

  of(path: string, duration: number): { from: number; to: number } | null {
    const w = this.windows[path];
    if (!w || w.to <= w.from || w.to > duration + 0.5) return null;
    return w;
  }
}

export const chosen = new Chosen();
