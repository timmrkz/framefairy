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
  // The search wrote another clip to the plan while it runs.
  wrote(search: Job): void;
  // Transcribe whole video, from the video's menu, is over.
  transcribed(job: Job): void;
  // The search has stopped hearing the video: it went on to find clips,
  // or it ended while it heard.
  heard(search: Job): void;
  // The search is over, however it ended.
  searched(search: Job): void;
  // A render the workspace asked for is over.
  rendered(render: Job): void;
}

const live = (j: Job | undefined) => j?.state === "running" || j?.state === "queued";

export class EpisodeSession {
  // The renders asked for in this workspace that are not over yet.
  private renders = new Set<string>();
  private off: () => void;

  constructor(
    readonly path: string,
    private store: JobStore,
    private moves: WorkMoves,
  ) {
    this.off = store.watch((before, after) => this.seen(before, after));
  }

  // A render asked for here, by the job the Go side answered with. One
  // that is over before the answer came is said at once.
  rendering(job: Job) {
    const now = this.store.list.find((j) => j.id === job.id) ?? job;
    if (live(now)) this.renders.add(job.id);
    else this.moves.rendered(now);
  }

  // The workspace is gone, and hears nothing more.
  close() {
    this.off();
  }

  private seen(before: Job | undefined, after: Job) {
    if (after.episode !== this.path) return;
    const was = live(before);
    const now = live(after);
    switch (after.kind) {
      case "search":
        if (now && (after.written ?? 0) > 0 && after.written !== (before?.written ?? 0)) this.moves.wrote(after);
        if (was && before?.step === "hearing" && !(now && after.step === "hearing")) this.moves.heard(after);
        if (was && !now) this.moves.searched(after);
        break;
      case "transcribe":
        if (was && !now) this.moves.transcribed(after);
        break;
      case "render":
        if (!now && this.renders.delete(after.id)) this.moves.rendered(after);
        break;
    }
  }
}
