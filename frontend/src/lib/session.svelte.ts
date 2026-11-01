// A video's session in the workspace, issue #606. It keeps the video's
// state as Go sends it, the newest there is, and it says when the video's
// work moves on, once, as the job store keeps each snapshot of a job, so
// no effect in the workspace has to keep a value from before and compare
// it with the next. See
// docs/decisions/2026-10-30-a-videos-state-in-one-session.md.
import type { ClipEntry, CoverageView, EpisodeStatus, Job, VideoState } from "./api";
import { live } from "./flow";
import { said } from "./said";
import type { JobStore } from "./state.svelte";
import { unchanged } from "./unchanged";

// What the workspace does when the video's work moves on. When one
// snapshot says more than one of them, they come in this order.
export interface WorkMoves {
  // A search or a clip made by hand failed: one seen running, or one
  // asked for here. Each is said once, issue #450. One seen failed only
  // as the app opened said why already, the time it failed.
  failed(job: Job): void;
  // The search wrote another clip to the plan while it runs.
  wrote(search: Job): void;
  // The search has stopped hearing the video: it went on to find clips,
  // or it ended while it heard.
  heard(search: Job): void;
  // The search is over, however it ended.
  searched(search: Job): void;
  // Transcribe whole video, from the video's menu, is over.
  transcribed(job: Job): void;
  // A clip made by hand that was asked for here is over, however it
  // ended.
  made(clip: Job): void;
  // A render that was asked for here is over.
  rendered(render: Job): void;
}

export class EpisodeSession {
  // The video's state, from the newest VideoState kept: null until the
  // first is in.
  status = $state<EpisodeStatus | null>(null);
  clips = $state<ClipEntry[]>([]);
  coverage = $state<CoverageView>({ passes: [] });
  // The rev of the newest state kept, and how many clips each job had
  // written in it. A state read after a job's news, rev past its seq,
  // holds every clip the job had said it wrote by then.
  rev = $state(0);
  private written = $state<Record<string, number>>({});
  // The rev of the newest answer to an edit: a state read before it
  // knows nothing of the edit, and would put back what it changed.
  private floor = 0;
  // Changes shown before the Go side has answered them, and the newest
  // state that came in the meantime, kept until they are answered.
  private holds = 0;
  private waiting: VideoState | null = null;
  private waitingFresh = false;
  // The work asked for in this workspace that is not over yet.
  private asking = new Set<string>();
  // The failures said, so the answer to a call that comes after its news
  // says nothing again.
  private told = new Set<string>();
  // The news of the video's jobs whose moves wait for a state read after
  // it, in the order it came, see seen. late is news that should have come
  // after its state and came before it.
  private waits: { before: Job | undefined; after: Job; late: boolean }[] = [];
  private late = false;
  private moves: WorkMoves = { failed() {}, wrote() {}, heard() {}, searched() {}, transcribed() {}, made() {}, rendered() {} };

  constructor(
    readonly path: string,
    private store: JobStore,
  ) {}

  // The workspace is on the page: from now on the session hears every
  // snapshot of a job the store keeps and says what the video's work did
  // with moves, and takes every state of the video hear brings, onVideoState
  // in lib/api.ts. A move is made once the state kept was read after the
  // job's news, so it reads what the work left, see seen. The Go side
  // sends that state before the news that moves the work on, so a move
  // waits for nothing unless an edit holds the states, see take. The
  // function it returns is for when the workspace is gone, and then it
  // hears nothing more.
  open(moves: WorkMoves, hear: (fn: (state: VideoState) => void) => () => void = () => () => {}): () => void {
    this.moves = moves;
    const off = this.store.watch((before, after) => this.seen(before, after));
    const unheard = hear((state) => {
      if (state.video === this.path) this.take(state);
    });
    return () => {
      off();
      unheard();
    };
  }

  // A state of the video, sent by the Go side or read by the workspace.
  // It is kept when it is newer than every state and every edit's answer
  // kept before it, and fresh makes every clip in it a new one, so the
  // captions of the clip on screen are read again after an edit of them,
  // the clips kept when the state is older than them.
  // A state sent as work goes on keeps each clip it holds unchanged the
  // very same clip, so nothing about the clip on screen is drawn again.
  // Its jobs go to the store whether it is kept or not, those the store
  // has no newer snapshot of, so the jobs on screen are never older than
  // the state, and the moves they make wait for a state that is kept.
  // Whether it was kept.
  take(state: VideoState, fresh = false): boolean {
    let kept = false;
    if (this.holds > 0) {
      if (!this.waiting || state.rev > this.waiting.rev) this.waiting = state;
      this.waitingFresh ||= fresh;
    } else if (state.rev > this.rev && state.rev > this.floor) {
      this.rev = state.rev;
      this.status = same(this.status, state.status);
      this.clips = fresh ? state.clips : unchanged(this.clips, state.clips);
      this.coverage = same(this.coverage, state.coverage);
      this.written = Object.fromEntries(state.jobs.map((j) => [j.id, j.written ?? 0]));
      kept = true;
      this.decideWaiting();
    } else if (fresh) {
      // A newer state came first, and still the captions are read again.
      this.clips = this.clips.map((c) => ({ ...c }));
    }
    for (const job of state.jobs) {
      const had = this.store.list.find((j) => j.id === job.id);
      if (!had || (job.seq ?? 0) > (had.seq ?? 0)) this.store.apply(job);
    }
    return kept;
  }

  // How many clips the job had written in the newest state kept, which
  // are in its clips.
  wrote(job: string): number {
    return this.written[job] ?? 0;
  }

  // One clip as an edit left it, from the edit's answer. No state read
  // before the edit is kept after it.
  put(clip: ClipEntry) {
    this.floor = Math.max(this.floor, clip.rev ?? 0);
    this.clips = this.clips.map((c) => (c.key === clip.key ? clip : c));
  }

  // One clip changed on screen at once, the way a hand left it, while
  // write saves it. Its answer takes the change's place, and a write that
  // is refused puts the clip back as it was and throws. The states that
  // come while it writes are kept aside, because each was read before the
  // edit and would undo it on screen for a moment. The newest of them is
  // taken once it has answered, unless the answer is newer.
  async edit(key: string, to: (clip: ClipEntry) => ClipEntry, write: () => Promise<ClipEntry>): Promise<void> {
    const was = this.clips.find((c) => c.key === key);
    this.holds++;
    this.clips = this.clips.map((c) => (c.key === key ? to(c) : c));
    try {
      this.put(await write());
    } catch (err) {
      if (was) this.clips = this.clips.map((c) => (c.key === key ? was : c));
      throw err;
    } finally {
      this.holds--;
      const next = this.waiting;
      if (this.holds === 0 && next) {
        const fresh = this.waitingFresh;
        this.waiting = null;
        this.waitingFresh = false;
        this.take(next, fresh);
      }
    }
  }

  // Work asked for here, a clip made by hand or a render, by the job the
  // Go side answered with. What the Go side says of a job travels apart
  // from the answer to the call, so a job can be over in the store before
  // the answer comes back, its news heard first on a busy machine, issue
  // #508. Then it is said from what the store knows of it, once a state
  // read after that is in. The answer alone does not say it is over: that
  // is said once the store has its end.
  asked(job: Job) {
    this.asking.add(job.id);
    const now = this.store.list.find((j) => j.id === job.id);
    if (now && !live(now)) {
      this.waits.push({ before: undefined, after: now, late: false });
      this.decideWaiting();
    }
  }

  // A snapshot of a job the store kept. Its moves are made once the state
  // kept was read after it, its rev past the job's seq, so a move reads
  // the clips and the status the work left: the first clip found chosen,
  // the window moved on. Until then they wait, in the order the news
  // came: while an edit holds the states, when a state read before an
  // edit's answer is not kept, and when news went out before its state.
  private seen(before: Job | undefined, after: Job) {
    if (after.episode !== this.path) return;
    const late = this.rev > 0 && this.holds === 0 && this.rev >= this.floor && !this.knows(after);
    this.waits.push({ before, after, late });
    this.decideWaiting();
  }

  // Whether the state kept was read after this news of the job.
  private knows(job: Job): boolean {
    return (job.seq ?? 0) < this.rev;
  }

  private decideWaiting() {
    while (this.waits.length > 0 && this.knows(this.waits[0].after)) {
      const { before, after, late } = this.waits.shift()!;
      this.decide(before, after, late);
    }
  }

  private decide(before: Job | undefined, after: Job, late: boolean) {
    this.late = late;
    try {
      this.movesOf(before, after);
    } finally {
      this.late = false;
    }
  }

  private movesOf(before: Job | undefined, after: Job) {
    const was = live(before);
    const now = live(after);
    const asked = !now && this.asking.delete(after.id);
    if ((was || asked) && after.state === "failed") this.fail(after);
    switch (after.kind) {
      case "search":
        if (now && (after.written ?? 0) > 0 && after.written !== (before?.written ?? 0)) this.make("wrote", after);
        if (was && before?.step === "hearing" && !(now && after.step === "hearing")) this.make("heard", after);
        if (was && !now) this.make("searched", after);
        break;
      case "transcribe":
        if (was && !now) this.make("transcribed", after);
        break;
    }
    if (asked) this.over(after);
  }

  // A move that goes wrong is said in the app's log and stops nothing
  // else: not the moves after it, and not the rest of the state taken. A
  // move whose news came before its state is said there too: the Go side
  // sends the state first for every news that moves the work on, by
  // movesOn in cmd/framefairy-app/jobs.go, so one that did not means the
  // two sides' rules no longer agree, see moves.cases.json. The move still
  // reads the right state, only a moment later.
  private make(move: keyof WorkMoves, job: Job) {
    if (this.late) {
      said(`session: the move ${move} of the job ${job.id} waited for its state, which came after its news`, { level: "warn", video: this.path });
    }
    try {
      this.moves[move](job);
    } catch (err) {
      said(`session: the move ${move} of the job ${job.id} went wrong: ${err}`, { level: "error", video: this.path });
    }
  }

  private fail(job: Job) {
    if (job.kind !== "search" && job.kind !== "clip") return;
    if (this.told.has(job.id)) return;
    this.told.add(job.id);
    this.make("failed", job);
  }

  private over(job: Job) {
    if (job.kind === "clip") this.make("made", job);
    else if (job.kind === "render") this.make("rendered", job);
  }
}

// The value kept, when the one that came says the same, so nothing that
// reads it runs again for a state that changed elsewhere.
function same<T>(was: T, now: T): T {
  return was !== null && JSON.stringify(was) === JSON.stringify(now) ? was : now;
}
