// A model being downloaded from the settings, the speech model's and the
// clip model's alike: the job of its kind as the job list has it, Download
// pressed and not answered yet, why the last one stopped, and Cancel
// pressed and not done yet. One of these for each card, so the two cards
// work alike because they are one thing used twice, never two copies
// that drift, see ModelCard.svelte.
import { api, errorText, type Job } from "./api";
import { jobs } from "./state.svelte";

const live = (j: Job) => j.state === "running" || j.state === "queued";

export class Install {
  // Download pressed and the job not reported yet, so a click shows at
  // once.
  starting = $state(false);
  // Why the last download did not start or stopped, said in its line.
  failed = $state("");
  // Cancel pressed. It takes a moment to reach the download.
  cancelling = $state(false);
  // The latest job of this kind, and that job while it runs.
  readonly job = $derived.by(() => jobs.list.filter((j) => j.kind === this.kind).at(-1));
  readonly running = $derived(this.job && live(this.job) ? this.job : undefined);

  // kind is the job's, "model" for speech and "llm" for the clip model.
  // settled is told once each time a download ends, so the models are
  // read again then and not on every job event, which arrive about once
  // a second while one runs. Made while a component is set up, which is
  // where its effects belong.
  private readonly kind: string;

  constructor(kind: string, settled: () => void) {
    this.kind = kind;
    let seen = "";
    $effect(() => {
      const job = this.job;
      const now = job ? `${job.id}:${job.state}` : "";
      if (now === seen) return;
      seen = now;
      if (!job || live(job)) return;
      if (job.state === "failed") this.failed = job.error ?? "The download stopped.";
      settled();
    });
    $effect(() => {
      if (!this.running) this.cancelling = false;
    });
  }

  // Starts the download start makes.
  async start(start: () => Promise<Job>) {
    this.failed = "";
    this.starting = true;
    try {
      const job = await start();
      if (job.state === "failed") this.failed = job.error ?? "The download did not start.";
    } catch (err) {
      this.failed = errorText(err);
    }
    this.starting = false;
  }

  // Stops the one running. What has arrived stays, and Download carries
  // on from it.
  async cancel(onerror: (why: string) => void) {
    const job = this.running;
    if (!job) return;
    this.cancelling = true;
    try {
      await api.cancelJob(job.id);
    } catch (err) {
      onerror(errorText(err));
    }
  }

  // How far the running one has come, or -1 where it cannot say.
  get fraction(): number {
    const p = this.running?.progress;
    return p && p.fraction >= 0 ? p.fraction : -1;
  }
}
