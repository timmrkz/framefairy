<script lang="ts">
  import { clock, type Job } from "../lib/api";
  import { jobs } from "../lib/state.svelte";
  import JobProgress from "../components/JobProgress.svelte";

  let open = $state<string | null>(null);

  const ordered = $derived([...jobs.list].reverse());
  const finished = $derived(jobs.list.some((j) => j.state !== "running" && j.state !== "queued"));

  function name(path: string): string {
    return path.split(/[\\/]/).pop() ?? path;
  }

  function outcome(job: Job): string {
    switch (job.state) {
      case "done":
        return "Finished";
      case "failed":
        return "Failed";
      case "cancelled":
        return "Cancelled";
    }
    return "";
  }

  function dot(job: Job): string {
    return { done: "ok", failed: "err", cancelled: "warn", running: "busy", queued: "" }[job.state];
  }
</script>

<section class="scroll">
  <header class="row">
    <span class="grow"></span>
    <button onclick={() => jobs.clear()} disabled={!finished}>Clear finished</button>
  </header>

  <ol>
    {#each ordered as job (job.id)}
      <li>
        {#if job.state === "running" || job.state === "queued"}
          <div class="running">
            <p class="muted">{name(job.episode)}</p>
            <JobProgress {job} />
          </div>
        {:else}
          <button class="line" onclick={() => (open = open === job.id ? null : job.id)}>
            <span class="dot {dot(job)}"></span>
            <span class="label">{job.label}</span>
            <span class="muted">{name(job.episode)}</span>
            <span class="grow"></span>
            <span class="muted">{outcome(job)}</span>
          </button>
          {#if job.error}<p class="error selectable">{job.error}</p>{/if}
        {/if}
        {#if open === job.id}
          <pre class="selectable">{(jobs.log[job.id] ?? [])
              .map((e) => `${clock(e.elapsed)}  ${e.text}`)
              .join("\n")}</pre>
        {/if}
      </li>
    {:else}
      <li class="muted none">Nothing has run yet. Transcriptions, clip searches and renders show up here.</li>
    {/each}
  </ol>
</section>

<style>
  section {
    padding: var(--gap) var(--edge) var(--edge);
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    flex: 1;
    min-height: 0;
  }

  .grow {
    flex: 1;
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  /* One row for one job. The line between two of them runs the whole width
     of the page, and everything inside a row keeps the same distance from
     the edge as the rest of the page. */
  li {
    padding: 8px 0;
    border-top: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  /* Everything on the page starts and ends in the same two places: the
     lines between the jobs, the progress of a running one, the text of a
     finished one and the button at the head. Nothing is a step in from
     anything else. */
  .line {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    height: auto;
    min-height: 36px;
    padding: 0;
    background: transparent;
    border: none;
    border-radius: var(--radius-s);
    text-align: left;
  }

  /* The row lights up under the pointer, in its own place, so the light
     starts and ends where everything else on the page does. */
  .line:hover {
    background: var(--ink-2);
  }

  .label {
    font-weight: 600;
  }

  pre {
    margin: 0;
    padding: 12px;
    background: var(--ink-1);
    border-radius: var(--radius-m);
    font-size: var(--size-s);
    white-space: pre-wrap;
    max-height: 320px;
    overflow: auto;
  }
</style>
