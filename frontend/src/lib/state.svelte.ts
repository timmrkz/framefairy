// Shared state: the job list, kept current from the Go side's events.
import { api, onJob, type Job, type Lane } from "./api";
import { mergeJob } from "./flow";
import type { EpisodeOrder } from "./order";

// How often the list is read again while anything runs. The events keep it
// current, and this is what puts it right when one of them was lost: a job
// that looks as if it runs for ever holds up the first search and says
// Cancelling for good, and only a restart cleared it.
const recheck = 5000;

class JobStore {
  list = $state<Job[]>([]);

  // A snapshot of a job, kept unless the list already has a later one.
  apply(job: Job) {
    const next = mergeJob(this.list, job);
    if (next) this.list = next;
  }

  async start() {
    // Listening starts before the list is read, so nothing that happens
    // while it is read is missed. What was heard in the meantime and what
    // the list says are merged by their numbers.
    onJob(({ job }) => this.apply(job));
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
    const last = this.list.findLast((j) => j.episode === path && j.kind === "search");
    if (!last || last.state === "done" || last.state === "cancelled") return undefined;
    return last;
  }

  // The episode's clips made by hand with I and O that have something to
  // say: on their way, or stopped, cut off or failed. With the search,
  // they are the work of the clip list.
  clips(path: string): Job[] {
    return this.list.filter(
      (j) => j.episode === path && j.kind === "clip" && j.state !== "done" && j.state !== "cancelled",
    );
  }

  get busy(): number {
    return this.list.filter((j) => j.state === "running" || j.state === "queued").length;
  }
}

export const jobs = new JobStore();

export type View =
  | { name: "episode"; path: string }
  | { name: "settings" }
  | { name: "updates" }
  | { name: "acknowledgements" }
  // Report a Problem…, with the video that was in front when it was asked.
  | { name: "report"; video?: string }
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
  // How the sidebar lists the episodes: in the order they were added, the
  // newest last, or by name. The order added is the library's own, so it
  // is the one a new copy of the app starts with.
  order = $state<EpisodeOrder>(readOrder());

  set(open: boolean) {
    this.pinned = open;
    try {
      localStorage.setItem("sidebar", open ? "open" : "closed");
    } catch {
      // A webview without storage simply forgets the choice.
    }
  }

  sortBy(order: EpisodeOrder) {
    this.order = order;
    try {
      localStorage.setItem("episodeOrder", order);
    } catch {
      // Forgotten again, the same way.
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

function readOrder(): EpisodeOrder {
  try {
    return localStorage.getItem("episodeOrder") === "name" ? "name" : "added";
  } catch {
    return "added";
  }
}

export const shell = new Shell();
