// Shared state: the job list, kept current from the Go side's events.
// A video's jobs come in its state, onVideoState, one channel, and a job
// of no video, a model being downloaded, on the job event.
import { untrack } from "svelte";
import { api, onJob, onVideoState, type Job, type Lane, type VideoState } from "./api";
import { live, mergeJob, outstanding } from "./flow";
import { said } from "./said";
import type { EpisodeOrder } from "./order";

// The kinds of work a person starts and stops for a video: the whole
// video heard, from its menu, the clip list's work, the search with the
// clips made by hand, from Search, Continue, Cancel, I and O, and a render,
// from Render.
export type Work = "whole" | "clips" | "render";

const works: Work[] = ["whole", "clips", "render"];

function doing(work: Work, j: Job): boolean {
  if (work === "whole") return j.kind === "transcribe";
  if (work === "render") return j.kind === "render";
  return j.kind === "search" || j.kind === "clip";
}

// A press of a video's work the Go side has not answered yet. what tells
// two presses of one kind of work apart, the clip a render is of, or I and
// O for a clip made by hand.
type Press = { path: string; work: Work; what: string };

const of = (path: string, work: Work) => (p: Press) => p.path === path && p.work === work;

export class JobStore {
  list = $state<Job[]>([]);
  // What a person pressed that the Go side has not answered yet, the same
  // for every kind of work, so no control keeps a flag of its own. A start
  // lasts until its call answers with the job, see startJob. A stop lasts
  // until the Go side says its work is stopping, which it says from the
  // moment it is told, or has ended, see stopJob: the video's dot, its
  // menu, the clip list's head and the range picker all stop on the
  // press, and not when the work ends a second or two later.
  starting = $state.raw<Press[]>([]);
  stopping = $state.raw<Press[]>([]);

  // Who hears each snapshot the list keeps, with the one it replaced, see
  // EpisodeSession in lib/session.svelte.ts.
  private watchers = new Set<(before: Job | undefined, after: Job) => void>();

  // The rev of the newest state of each video the jobs were taken from.
  private revs = new Map<string, number>();

  // The jobs of a video, from a state of it the Go side sent or a call
  // answered with. Each is kept unless the list has a later snapshot of
  // it. A state newer than every one before it holds every job of the
  // video there is to say anything about, so a job of the video it no
  // longer holds, older than it, is one the Go side forgot, and goes.
  take(state: VideoState) {
    for (const job of state.jobs) {
      const had = this.list.find((j) => j.id === job.id);
      if (!had || (job.seq ?? 0) > (had.seq ?? 0)) this.apply(job);
    }
    if (state.rev <= (this.revs.get(state.video) ?? 0)) return;
    this.revs.set(state.video, state.rev);
    const held = new Set(state.jobs.map((j) => j.id));
    const kept = this.list.filter((j) => j.episode !== state.video || held.has(j.id) || (j.seq ?? 0) > state.rev);
    if (kept.length < this.list.length) {
      this.list = kept;
      this.settle();
    }
  }

  // A snapshot of a job, kept unless the list already has a later one.
  apply(job: Job) {
    const before = this.list.find((j) => j.id === job.id);
    const next = mergeJob(this.list, job);
    if (next) this.list = next;
    this.settle();
    if (next) untrack(() => this.tell(before, job));
  }

  // Heard outside any effect that applied it, so what a watcher reads
  // does not make that effect run again. A watcher that goes wrong is said
  // in the app's log and stops nothing else: not the watchers after it,
  // which are the other workspaces, and not the rest of a list being read.
  private tell(before: Job | undefined, after: Job) {
    for (const watch of this.watchers) {
      try {
        watch(before, after);
      } catch (err) {
        said(`jobs: what the job ${after.id} moved on to could not be shown: ${err}`, { level: "error", video: after.episode });
      }
    }
  }

  // Hears every snapshot kept from now on, until the function it returns
  // is called.
  watch(fn: (before: Job | undefined, after: Job) => void): () => void {
    this.watchers.add(fn);
    return () => this.watchers.delete(fn);
  }

  isStarting(path: string, work: Work, what?: string): boolean {
    return this.starting.some((p) => of(path, work)(p) && (what === undefined || p.what === what));
  }

  // Whether a stop was pressed for the work, or the Go side says it is
  // stopping.
  isStopping(path: string, work: Work): boolean {
    return this.stopping.some(of(path, work)) || this.list.some((j) => j.episode === path && live(j) && doing(work, j) && !!j.stopping);
  }

  // Whether the job stops: the Go side says so, or a stop was pressed for
  // the work it does.
  stops(j: Job): boolean {
    return !!j.stopping || works.some((w) => doing(w, j) && this.stopping.some(of(j.episode, w)));
  }

  // Starts a video's work: the press shows from this frame, until the Go
  // side answers with the job, which the list then holds. A start is a new
  // press, so a stop before it is over. It answers with the job, and a
  // call that failed throws, for the caller to say why.
  async startJob(path: string, work: Work, call: () => Promise<Job>, what = ""): Promise<Job> {
    const press = { path, work, what };
    this.starting = [...this.starting, press];
    if (this.stopping.some(of(path, work))) this.stopping = this.stopping.filter((p) => !of(path, work)(p));
    try {
      const job = await call();
      this.apply(job);
      return job;
    } finally {
      this.starting = this.starting.filter((p) => p !== press);
      this.settle();
    }
  }

  // Stops a video's work: the stop shows from this frame, until the Go
  // side says the work is stopping or has ended.
  stopJob(path: string, work: Work, call: () => Promise<unknown>) {
    if (!this.stopping.some(of(path, work))) this.stopping = [...this.stopping, { path, work, what: "" }];
    call().catch((err) => said(`jobs: the stop of the ${work} work could not be asked for: ${err}`, { level: "warn", video: path }));
  }

  // A stop lasts while a start of its work is out, or work of it runs that
  // the Go side does not say is stopping yet.
  private settle() {
    const left = this.stopping.filter(
      (p) => this.starting.some(of(p.path, p.work)) || this.list.some((j) => j.episode === p.path && live(j) && doing(p.work, j) && !j.stopping),
    );
    if (left.length < this.stopping.length) this.stopping = left;
  }

  async start() {
    // Listening starts before the list is read, so nothing that happens
    // while it is read is missed. What was heard in the meantime and what
    // the list says are merged by their numbers. Nothing is read again
    // after: every state of a video holds all of its jobs, so a state that
    // was missed is put right by the next, and nothing has to be polled.
    onJob(({ job }) => this.apply(job));
    onVideoState((state) => this.take(state));
    await this.resync();
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
        live(j) &&
        (!lane || j.lane === lane),
    );
  }

  // The episode's search, the newest there is: running, or stopped with
  // something to say, cut off or failed. One done, cancelled or settled
  // has nothing to say, and is not it.
  search(path: string): Job | undefined {
    const last = this.list.findLast((j) => j.episode === path && j.kind === "search");
    if (!outstanding(last)) return undefined;
    return last;
  }

  // The episode's clips made by hand with I and O that have something to
  // say: on their way, or stopped, cut off or failed. With the search,
  // they are the work of the clip list.
  clips(path: string): Job[] {
    return this.list.filter(
      (j) => j.episode === path && j.kind === "clip" && outstanding(j),
    );
  }

  // The video's transcription to its end, Transcribe whole video in its
  // menu, the newest there is: running, or stopped, cut off or failed,
  // which its menu offers to carry on. One that is done has nothing to say,
  // and neither has one settled, carried on by the next or forgotten with
  // its video when the video left the library.
  transcription(path: string): Job | undefined {
    const last = this.list.findLast((j) => j.episode === path && j.kind === "transcribe");
    if (!outstanding(last)) return undefined;
    return last;
  }

  // The video's transcription to its end while it runs or waits its turn.
  transcribing(path: string): Job | undefined {
    const work = this.transcription(path);
    return work && live(work) ? work : undefined;
  }

  get busy(): number {
    return this.list.filter(live).length;
  }
}

export const jobs = new JobStore();

export type View =
  | { name: "episode"; path: string }
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
  // How the sidebar lists the episodes: in the order they were added, the
  // newest last, or by name. The order added is the library's own, so it
  // is the one a new copy of the app starts with.
  order = $state<EpisodeOrder>(readOrder());
  // How many boxes asking first are open, Confirm.svelte, so a video
  // dropped on the app is not added under a question.
  asking = $state(0);

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
