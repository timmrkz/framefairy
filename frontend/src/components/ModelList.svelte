<script lang="ts">
  // A list of models that can be installed, and installing one. The speech
  // models and the models that find clips are the same kind of thing on
  // screen, so they are one list: two lists of the same kind would have to
  // look the same anyway.
  //
  // No model ships with the app, so every copy fetches what it needs. The
  // rows say what a model is, what it costs and, where the machine has
  // something to say about it, whether it will run here.
  //
  // An install is work in hand like any other, so it wears what all work
  // wears, in the control it was started from: the beam round the button
  // and the fill for how far it has come, and the button says Cancel while
  // it runs. The row does not move and does not light up as a whole. What
  // it says under the name turns from what the model costs into how far
  // the download has come, how fast, and how long is left, in the same one
  // line, so the row keeps its height.
  //
  // An installed model can be removed again, to give its room back. It
  // asks first, because getting it back means fetching gigabytes again.
  import { api, clock, errorText, type Job, type ModelRow } from "../lib/api";
  import { jobs } from "../lib/state.svelte";
  import Busy from "./Busy.svelte";
  import Confirm from "./Confirm.svelte";
  import Icon from "./Icon.svelte";

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
    // How to remove one, where models can be removed. The setup has
    // nothing to remove.
    onremove,
    // How to make an installed one the one in use, where there is a choice
    // between several: the models that find clips.
    onuse,
    // What removing a model means beyond the room it gives back, said in
    // the box that asks first.
    removeSays = "",
  }: {
    models: ModelRow[];
    kind: "model" | "llm";
    oninstall: (name: string) => Promise<Job>;
    onchange: () => void;
    auto?: boolean;
    onremove?: (name: string) => Promise<void>;
    onuse?: (name: string) => Promise<void>;
    removeSays?: string;
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

  // Cancelling takes a moment to reach the download, so the button says so
  // at once. The part that arrived stays, and the next Install carries on
  // from it.
  let cancelling = $state("");
  function cancel(job: Job) {
    cancelling = job.id;
    api.cancelJob(job.id);
  }

  // What a running install says under the name: what the download says,
  // how far, how fast, and how long is left.
  function said(job: Job): string {
    if (job.state === "queued") return "Waiting for the install before it";
    const text = job.progress?.text ?? job.last?.text ?? "";
    const left =
      job.progress && job.progress.remaining > 0 ? `, ${clock(job.progress.remaining)} left` : "";
    const line = text ? text[0].toUpperCase() + text.slice(1) : "Starting";
    return line + left;
  }

  const share = (job: Job) =>
    job.progress && job.progress.fraction >= 0 ? job.progress.fraction : -1;

  // The model the box is asking about, and the one being made the one in
  // use, so its button says so at once.
  let removing = $state<ModelRow | null>(null);
  let using = $state("");

  async function remove(model: ModelRow) {
    removing = null;
    problem = "";
    try {
      await onremove?.(model.name);
    } catch (err) {
      problem = errorText(err);
    }
    onchange();
  }

  async function use(name: string) {
    using = name;
    problem = "";
    try {
      await onuse?.(name);
    } catch (err) {
      problem = errorText(err);
    }
    using = "";
    onchange();
  }

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
    <!-- Which row an install belongs to. The job carries the name the Go
         side gave it, which is the model's own title, and a row may show
         more than that, the maker too, so rows are matched on the job's
         name and not on what they show. -->
    {@const mine = running?.label === model.label ? running : undefined}
    {@const chosen = model.inUse ?? model.installed}
    <li class:on={chosen}>
      <div class="row head">
        <span class="title grow">{model.title}</span>
        {#if model.installed}
          {#if chosen}
            <!-- A state and not a control, so it does nothing when clicked,
                 but it keeps the frame of the button it stands in for, so
                 the row reads the same whatever the model is. -->
            <span class="act state"><Icon name="check" />{model.inUse === undefined ? "Installed" : "In use"}</span>
          {:else}
            <button
              class="act"
              title="Find clips with this model"
              disabled={using !== ""}
              onclick={() => use(model.name)}
            >
              {#if using === model.name}<Busy />{/if}
              {using === model.name ? "Choosing" : "Use"}
            </button>
          {/if}
        {:else if mine}
          <!-- The button the install was started from carries it: the beam
               and the fill, and the one thing to do about it. -->
          <button
            class="act"
            title="Stop the download. What has arrived stays, and Install carries on from it"
            disabled={cancelling === mine.id}
            onclick={() => cancel(mine)}
          >
            <Busy fraction={share(mine)} />
            {cancelling === mine.id ? "Cancelling" : "Cancel"}
          </button>
        {:else}
          <button class="act" disabled={asked === model.name || !!running} onclick={() => install(model.name)}>
            {#if asked === model.name}<Busy />{/if}
            {asked === model.name ? "Starting" : "Install"}
          </button>
        {/if}
        <!-- Last in the row, and always there. Every row of a list whose
             models can be removed keeps its place, so the buttons beside
             it stand in one column whether a model is there or not. -->
        {#if onremove}
          {#if model.installed}
            <button
              class="bin"
              title="Remove it from this machine, to give its room back"
              aria-label="Remove {model.title}"
              aria-haspopup="dialog"
              disabled={!!running}
              onclick={() => (removing = model)}
            >
              <Icon name="trash" size={14} />
            </button>
          {:else}
            <span class="bin" aria-hidden="true"></span>
          {/if}
        {/if}
      </div>
      <p class="muted about">{model.about}</p>
      {#if mine}
        <p class="small muted num">{said(mine)}</p>
      {:else}
        <p class="small" class:muted={!model.warn} class:warn={model.warn}>
          {model.cost}{model.note ? `. ${model.note}` : ""}
        </p>
      {/if}
      {#if !mine && job && job.label === model.label && job.state === "failed"}
        <p class="error selectable">{job.error}</p>
      {/if}
    </li>
  {/each}
</ul>

{#if removing}
  {@const gone = removing}
  <Confirm title="Remove {gone.title}?" oncancel={() => (removing = null)}>
    <p>
      It leaves this machine and gives back {gone.room}. Using it again means fetching it
      again.{removeSays ? ` ${removeSays}` : ""}
    </p>
    {#snippet actions()}
      <button onclick={() => (removing = null)}>Cancel</button>
      <button class="danger" onclick={() => remove(gone)}>Remove</button>
    {/snippet}
  </Confirm>
{/if}

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

  .warn {
    color: var(--warn);
  }

  /* Room for the longest wording, Cancelling, so the row keeps still
     whatever the button says. The word that stands in for it once the
     model is there takes the same room, so the trash can beside it stays
     where it is. */
  .act {
    min-width: 104px;
    justify-content: center;
  }

  /* What a model is once it is there, in the frame of the button it
     stands in for: the same size, the same line, in the colour of what is
     done. */
  .state {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: var(--control-h);
    box-sizing: border-box;
    padding: 0 12px;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    color: var(--ok);
    white-space: nowrap;
  }

  /* The trash can: a button of the row, square at the height of every
     control, quiet until it is reached and then the colour of what it
     does, the way the trash can on a clip is. */
  .bin {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--control-h);
    height: var(--control-h);
    box-sizing: border-box;
    padding: 0;
    flex: none;
    color: var(--muted);
  }

  button.bin:hover:not(:disabled) {
    background: var(--lift-err);
    color: var(--err);
  }

  span.bin {
    visibility: hidden;
  }
</style>
