<script lang="ts">
  // A list of models that can be installed, and installing one. The speech
  // models and the models that find clips are the same kind of thing on
  // screen, so they are one list: two lists of the same kind would have to
  // look the same anyway.
  //
  // No model ships with the app, so every copy fetches what it needs. The
  // rows say what a model is, what it costs and, where the machine has
  // something to say about it, whether it will run here.
  import { api, errorText, size, type Job, type ModelRow } from "../lib/api";
  import { jobs } from "../lib/state.svelte";
  import Busy from "./Busy.svelte";
  import Icon from "./Icon.svelte";
  import JobProgress from "./JobProgress.svelte";

  let {
    models,
    // Which jobs belong to this list. A speech model and a model that
    // finds clips install in different lanes, so they are different kinds
    // of work and one never shows in the other's rows.
    kind,
    // How to start one.
    oninstall,
    // Called when an install settles, so whoever asked can read again what
    // is installed.
    onchange,
    // Whether the one model on the list may start by itself. It may in the
    // setup, where a single speech model is not a choice and the app does
    // what it can do by itself. It may not where somebody has to pick, and
    // never in the settings, where nobody asked for anything.
    auto = false,
  }: {
    models: ModelRow[];
    kind: "model" | "llm";
    oninstall: (name: string) => Promise<Job>;
    onchange: () => void;
    auto?: boolean;
  } = $props();

  // The model an install was just asked for, so the row says so before the
  // first job event arrives. A click shows at once.
  let asked = $state("");
  let problem = $state("");

  // The install as the job list has it. The list is only ever brought up
  // to date by events, which is where the progress comes from.
  const job = $derived(jobs.list.filter((j) => j.kind === kind).at(-1));
  const running = $derived(
    job && (job.state === "running" || job.state === "queued") ? job : undefined,
  );

  // Read again when an install settles, and not on every job event: the Go
  // side sends one about once a second while anything runs, and asking
  // that often would be a call a second for an answer that cannot have
  // changed.
  let settled = "";
  $effect(() => {
    const now = job ? `${job.id}:${job.state}` : "";
    if (now === settled) return;
    settled = now;
    if (job && job.state !== "running" && job.state !== "queued") {
      asked = "";
      onchange();
    }
  });

  // Fetching a model by itself happens once, however often this list is
  // read again. A download that failed is offered again by hand rather
  // than started again by the app, which would be a loop nobody could get
  // out of.
  let startedOne = false;
  $effect(() => {
    if (!auto || startedOne || running || models.length !== 1) return;
    const only = models[0];
    if (only.installed) return;
    startedOne = true;
    install(only.name);
  });

  async function install(name: string) {
    asked = name;
    problem = "";
    try {
      const started = await oninstall(name);
      if (started.state === "failed") problem = started.error ?? "";
    } catch (err) {
      problem = errorText(err);
    }
    onchange();
  }
</script>

{#if problem}<p class="error selectable">{problem}</p>{/if}
<ul>
  {#each models as model (model.name)}
    <!-- Which row an install belongs to. The job carries the model's title
         as its label, which is the one thing both sides name the same
         way. -->
    {@const mine = running?.label === model.title ? running : undefined}
    <li class:on={model.installed}>
      <div class="row head">
        <span class="title grow">{model.title}</span>
        {#if model.installed}
          <span class="done row"><Icon name="check" />Installed</span>
        {:else if mine}
          <span class="muted">Installing</span>
        {:else}
          <button class="act" disabled={asked === model.name || !!running} onclick={() => install(model.name)}>
            {#if asked === model.name}<Busy />{/if}
            {asked === model.name ? "Starting" : "Install"}
          </button>
        {/if}
      </div>
      <p class="muted about">{model.about}</p>
      <p class="small" class:muted={!model.warn} class:warn={model.warn}>
        {model.cost}{model.note ? `. ${model.note}` : ""}
      </p>
      {#if mine}
        <JobProgress job={mine} named={false} />
      {:else if job && job.label === model.title && job.state === "failed"}
        <p class="error selectable">{job.error}</p>
      {/if}
    </li>
  {/each}
</ul>

<style>
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
    /* Whole lines. The app's own 1.45 of 13 is 18.85, and a stack of
       those leaves every row below a fraction off a whole pixel. */
    line-height: 19px;
  }

  li {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 12px;
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    background: var(--ink-1);
  }

  /* The same background a chosen row wears everywhere else in the app. */
  li.on {
    border-color: var(--accent);
    background: var(--ink-3);
  }

  /* As tall as the control in it, so a row with a button and a row with a
     word are the same height and nothing moves as one becomes the other. */
  .head {
    min-height: var(--control-h);
  }

  .title {
    font-weight: 600;
  }

  .grow {
    flex: 1;
    min-width: 0;
  }

  .about {
    font-size: var(--size-m);
  }

  .small {
    font-size: var(--size-s);
  }

  .done {
    color: var(--ok);
    gap: 6px;
  }

  .warn {
    color: var(--warn);
  }

  /* Room for the longer wording, so the row keeps still while an install
     is being started. */
  .act {
    min-width: 92px;
  }
</style>
