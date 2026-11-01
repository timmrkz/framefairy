// Shared state: the job list, kept current from the Go side's events.
import { untrack } from "svelte";
import { api, onJob, onVideoState, type Job, type Lane, type VideoState } from "./api";
import { live, mergeJob, outstanding } from "./flow";
import { said } from "./said";
import type { EpisodeOrder } from "./order";

// How often the list is read again while anything runs. The events keep it
// current, and this is what puts it right when one of them was lost: a job
// that looks as if it runs for ever holds up the first search and says
// Cancelling for good, and only a restart cleared it.
const recheck = 5000;

// The two kinds of work a person starts and stops for a video: the whole
// video heard, from its menu, and the clip list's work, the search with
// the clips made by hand, from Search, Continue and Cancel.
export type Work = "whole" | "clips";

function doing(work: Work, j: Job): boolean {
  return work === "whole" ? j.kind === "transcribe" : j.kind === "search" || j.kind === "clip";
}

const key = (path: string, work: Work) => `${work} ${path}`;

export class JobStore {
  list = $state<Job[]>([]);
  // What a person pressed that the Go side has not caught up with yet, the
  // same for both kinds of work. starting is a start whose job has not
  // answered. stopping is a stop whose work has not said it ended: the
  // video's dot, its menu, the clip list's head and the range picker all
  // stop on the press, and not when the work ends a second or two later.
  // It was kept twice, once in the store for the transcription and once in
  // the workspace for the search, found by the third review of #458.
  starting = $state<string[]>([]);
  stopping = $state<string[]>([]);

  // Who hears each snapshot the list keeps, with the one it replaced, see
  // EpisodeSession in lib/session.svelte.ts.
  private watchers = new Set<(before: Job | undefined, after: Job) => void>();

  // The newest state heard of each video, by its rev, and the news of its
  // jobs that waits for a state read after it, the newest of each job.
  // A video's state and the news of its jobs travel apart, and the news
  // comes first. A job's end shown at its news, with the clips it left
  // still to come in the state, was a search over with no clip chosen yet
  // and its window drawn back where it was for a frame, #513. So the list
  // shows the news of a video's job only once a state read after it is
  // shown, and the end, its clips and what follows land in one step.
  private revs = new Map<string, number>();
  private waiting = new Map<string, Job>();
  // The videos a workspace is open on, and how many. Their states are the
  // session's to show, and it says which it showed with shown, see
  // EpisodeSession.take. The states of the rest are shown as they come.
  private followed = new Map<string, number>();

  // A snapshot of a job, kept unless the list already has a later one.
  // The news of a video's job waits for a state of the video read after
  // it, unless it says work started: a press shows at once, and a start
  // leaves nothing in the state that has to come with it.
  apply(job: Job) {
    const rev = this.revs.get(job.episode);
    const had = this.list.find((j) => j.id === job.id);
    const waits =
      rev !== undefined && (job.seq ?? 0) >= rev && (this.waiting.has(job.id) || !(live(job) && !live(had)));
    if (!waits) return this.keep(job);
    const was = this.waiting.get(job.id);
    if (!was || (job.seq ?? 0) > (was.seq ?? 0)) this.waiting.set(job.id, job);
  }

  // A state of the video read at rev is shown: the news of its jobs that
  // came before it was read is let through, in the order it was sent.
  shown(path: string, rev: number) {
    const was = this.revs.get(path);
    if (was !== undefined && rev <= was) return;
    this.revs.set(path, rev);
    this.release(path, rev);
  }

  // A workspace is open on the video, until the function it returns is
  // called.
  follow(path: string): () => void {
    this.followed.set(path, (this.followed.get(path) ?? 0) + 1);
    return () => {
      const n = (this.followed.get(path) ?? 1) - 1;
      if (n > 0) this.followed.set(path, n);
      else this.followed.delete(path);
    };
  }

  // A state of a video no workspace is open on, for the sidebar and the
  // rest of the app.
  private heard(state: VideoState) {
    if (this.followed.has(state.video)) return;
    this.shown(state.video, state.rev);
    for (const job of state.jobs) this.apply(job);
  }

  // The video left the library, and no state of it comes again. What
  // its jobs said last is shown now.
  forget(path: string) {
    this.revs.delete(path);
    this.release(path, Infinity);
  }

  // The news of the video's jobs from before rev, in the order it was sent.
  private release(path: string, rev: number) {
    const due = [...this.waiting.values()]
      .filter((j) => j.episode === path && (j.seq ?? 0) < rev)
      .sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0));
    for (const job of due) {
      this.waiting.delete(job.id);
      this.keep(job);
    }
  }

  private keep(job: Job) {
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

  isStarting(path: string, work: Work): boolean {
    return this.starting.includes(key(path, work));
  }

  isStopping(path: string, work: Work): boolean {
    return this.stopping.includes(key(path, work));
  }

  // Whether a stop was pressed for the work this job does.
  stops(j: Job): boolean {
    return (["whole", "clips"] as const).some((w) => doing(w, j) && this.isStopping(j.episode, w));
  }

  // A start pressed. It is a new press, so a stop before it is over. The
  // work on the clips stops the video's transcription, see clipsAsked.
  press(path: string, work: Work) {
    const k = key(path, work);
    if (!this.starting.includes(k)) this.starting = [...this.starting, k];
    if (this.stopping.includes(k)) this.stopping = this.stopping.filter((s) => s !== k);
    if (work === "clips") this.clipsAsked(path);
  }

  // Work on the video's clips asked for, a search or a clip made by hand.
  // The Go side stops the video's transcription for it, stopTranscribing
  // in export.go, and the transcription ends once the speech model has
  // heard the piece it is in, seconds later. So the stop is said on the
  // press, the way Stop transcribing says it: the menu said Stop
  // transcribing for three and a half seconds after Search, Tim's report
  // of 26 October, 14.22.
  clipsAsked(path: string) {
    if (this.isStarting(path, "whole") || this.list.some((j) => j.episode === path && live(j) && doing("whole", j))) this.stop(path, "whole");
  }

  // The Go side answered the start, with its job or with a refusal.
  answered(path: string, work: Work) {
    const k = key(path, work);
    if (this.starting.includes(k)) this.starting = this.starting.filter((s) => s !== k);
    this.settle();
  }

  // A stop pressed, for whatever of the work runs or is on its way.
  stop(path: string, work: Work) {
    const k = key(path, work);
    if (!this.stopping.includes(k)) this.stopping = [...this.stopping, k];
  }

  // A stop lasts until nothing of its work runs or is asked for.
  private settle() {
    const ended = this.stopping.filter((k) => {
      const [work, ...rest] = k.split(" ");
      const path = rest.join(" ");
      return !this.starting.includes(k) && !this.list.some((j) => j.episode === path && live(j) && doing(work as Work, j));
    });
    if (ended.length) this.stopping = this.stopping.filter((k) => !ended.includes(k));
  }

  async start() {
    // Listening starts before the list is read, so nothing that happens
    // while it is read is missed. What was heard in the meantime and what
    // the list says are merged by their numbers.
    onJob(({ job }) => this.apply(job));
    onVideoState((state) => this.heard(state));
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
