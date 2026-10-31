// A video's session in the workspace, issue #606. It says when the
// video's work moves on, once, as the job store keeps each snapshot of a
// job, so no effect in the workspace has to keep a value from before and
// compare it with the next. It is where the video's state Go sends is to
// be kept, see docs/decisions/2026-10-30-a-videos-state-in-one-session.md.
import type { Job } from "./api";
import type { JobStore } from "./state.svelte";

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

const live = (j: Job | undefined) => j?.state === "running" || j?.state === "queued";

export class EpisodeSession {
  // The work asked for in this workspace that is not over yet.
  private asking = new Set<string>();
  // The failures said, so the answer to a call that comes after its news
  // says nothing again.
  private told = new Set<string>();
  private off: () => void;

  constructor(
    readonly path: string,
    private store: JobStore,
    private moves: WorkMoves,
  ) {
    this.off = store.watch((before, after) => this.seen(before, after));
  }

  // Work asked for here, a clip made by hand or a render, by the job the
  // Go side answered with. What the Go side says of a job travels apart
  // from the answer to the call, so a job can be over in the store before
  // the answer comes back, its news heard first on a busy machine, issue
  // #508. Then it is said at once, from what the store knows of it.
  asked(job: Job) {
    const now = this.store.list.find((j) => j.id === job.id) ?? job;
    if (live(now)) {
      this.asking.add(job.id);
      return;
    }
    if (now.state === "failed") this.fail(now);
    this.over(now);
  }

  // The workspace is gone, and hears nothing more.
  close() {
    this.off();
  }

  private seen(before: Job | undefined, after: Job) {
    if (after.episode !== this.path) return;
    const was = live(before);
    const now = live(after);
    if (was && after.state === "failed") this.fail(after);
    switch (after.kind) {
      case "search":
        if (now && (after.written ?? 0) > 0 && after.written !== (before?.written ?? 0)) this.moves.wrote(after);
        if (was && before?.step === "hearing" && !(now && after.step === "hearing")) this.moves.heard(after);
        if (was && !now) this.moves.searched(after);
        break;
      case "transcribe":
        if (was && !now) this.moves.transcribed(after);
        break;
    }
    if (!now && this.asking.delete(after.id)) this.over(after);
  }

  private fail(job: Job) {
    if (job.kind !== "search" && job.kind !== "clip") return;
    if (this.told.has(job.id)) return;
    this.told.add(job.id);
    this.moves.failed(job);
  }

  private over(job: Job) {
    if (job.kind === "clip") this.moves.made(job);
    else if (job.kind === "render") this.moves.rendered(job);
  }
}
