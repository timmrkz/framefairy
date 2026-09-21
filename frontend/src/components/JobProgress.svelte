<script lang="ts">
  import { api, clock, type Job } from "../lib/api";

  let { job }: { job: Job } = $props();

  // Stopping a job takes a moment to reach the work itself, so the button
  // says so at once rather than looking like nothing happened.
  let stopping = $state(false);
  const word = $derived(job.kind === "transcribe" ? "Pause" : "Cancel");
  // What it says while the stop is on its way, the same wording as the
  // transcription note on the range picker.
  const onItsWay = $derived(job.kind === "transcribe" ? "Pausing" : "Cancelling");

  function stop() {
    stopping = true;
    api.cancelJob(job.id);
  }

  const fraction = $derived(job.progress && job.progress.fraction >= 0 ? job.progress.fraction : -1);
  const line = $derived(
    job.state === "queued"
      ? "Waiting for the job before it"
      : (job.progress?.text ?? job.last?.stage ?? job.last?.text ?? "Starting"),
  );
  const left = $derived(
    job.progress && job.progress.remaining > 0 ? `${clock(job.progress.remaining)} left` : "",
  );
</script>

<!-- Stacked, because this sits in the clip column as often as in a wide
     row, and it has to read the same in both. -->
<div class="job">
  <div class="row top">
    <span class="label grow">{job.label}</span>
    <span class="muted num">{left}</span>
  </div>
  <div class="bar" class:unknown={fraction < 0}>
    <i style="width: {Math.max(fraction, 0) * 100}%"></i>
  </div>
  <div class="row bottom">
    <span class="muted grow line">{line}</span>
    <button class="stop" onclick={stop} disabled={stopping}
      >{stopping ? onItsWay : word}</button
    >
  </div>
</div>

<style>
  .job {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .label {
    font-weight: 600;
  }

  .grow {
    flex: 1;
    min-width: 0;
  }

  .line {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Room for the longer wording, so the row keeps still. */
  .stop {
    min-width: 104px;
  }
</style>
