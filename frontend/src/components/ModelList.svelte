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
  import type { Snippet } from "svelte";
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
    // The info mark in the corner of the list, where the list needs one.
    info,
  }: {
    models: ModelRow[];
    kind: "model" | "llm";
    oninstall: (name: string) => Promise<Job>;
    onchange: () => void;
    auto?: boolean;
    onremove?: (name: string) => Promise<void>;
    onuse?: (name: string) => Promise<void>;
    removeSays?: string;
    info?: Snippet;
  } = $props();

  // Whether the rows are a choice, one of them the one in use.
  const pickable = $derived(!!onuse);

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
<ul class="card asks">
  {#if info}<span class="ask corner">{@render info()}</span>{/if}
  {#each models as model (model.name)}
    <!-- Which row an install belongs to. The job carries the name the Go
         side gave it, which is the model's own title, so rows are matched
         on the job's name and not on what they show. -->
    {@const mine = running?.label === model.label ? running : undefined}
    {@const on = using ? using === model.name : !!model.inUse}
    <li class="item">
      {#if pickable}
        <!-- The mark and the words are one button, the way a radio button
             and its label are one thing on the Mac. A model that is not
             there yet cannot be chosen, so its ring is faint and pressing
             it does nothing. It is not disabled, because a disabled
             button fades its words, and the words are what say what the
             model costs before anybody fetches it. -->
        <button
          class="choose"
          role="radio"
          aria-checked={on}
          aria-disabled={!model.installed}
          title={model.installed ? model.about : `${model.about} Install it to use it.`}
          onclick={() => model.installed && !on && use(model.name)}
        >
          <span class="mark"><span class="radio" class:on class:faint={!model.installed}></span></span>
          {@render words(model, mine)}
        </button>
      {:else}
        <span class="mark ok">{#if model.installed}<Icon name="check" />{/if}</span>
        <span class="lone" title={model.about}>{@render words(model, mine)}</span>
      {/if}
      {#if !model.installed}
        {#if mine}
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
          <button
            class="act"
            title="Fetch it and keep it on this machine"
            disabled={asked === model.name || !!running}
            onclick={() => install(model.name)}
          >
            {#if asked === model.name}<Busy />{/if}
            {asked === model.name ? "Starting" : "Install"}
          </button>
        {/if}
      {:else if onremove}
        <!-- Quiet until it is reached, then the colour of what it does,
             the way the trash can on a clip is. -->
        <button
          class="quiet danger bin"
          title="Remove it from this machine, to give its room back"
          aria-label="Remove {model.title}"
          aria-haspopup="dialog"
          disabled={!!running}
          onclick={() => (removing = model)}
        >
          <Icon name="trash" />
        </button>
      {/if}
    </li>
  {/each}
</ul>

{#snippet words(model: ModelRow, mine: Job | undefined)}
  <span class="words">
    <span class="head">{model.title}</span>
    {#if mine}
      <span class="small muted num">{said(mine)}</span>
    {:else}
      <span class="small" class:muted={!model.warn} class:warn={model.warn}>
        {model.cost}.{model.note ? ` ${model.note}.` : ""}
      </span>
    {/if}
    {#if !mine && job && job.label === model.label && job.state === "failed"}
      <span class="small error selectable">{job.error}</span>
    {/if}
  </span>
{/snippet}

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
  }

  /* The mark and the words of a row that can be chosen, pressed as one.
     A button with nothing of a button about it: the row is the control. */
  .choose,
  .lone {
    display: flex;
    align-items: center;
    gap: 12px;
    flex: 1;
    min-width: 0;
    height: auto;
    padding: 0;
    border: none;
    background: transparent;
    text-align: left;
    white-space: normal;
  }

  .choose:hover:not(:disabled) {
    background: transparent;
  }

  .choose[aria-disabled="true"],
  .choose[aria-checked="true"] {
    cursor: default;
  }

  /* A row that can be chosen answers the pointer the way a row of the
     clip list does. */
  li:has(.choose[aria-checked="false"][aria-disabled="false"]:hover) {
    background: var(--ink-2);
  }

  /* A model that is not there yet has a ring nobody can fill. */
  .radio.faint {
    border-color: var(--line);
  }

  .warn {
    color: var(--warn);
  }

  /* Room for the longest wording, Cancelling, so the row keeps still
     whatever the button says. */
  .act {
    min-width: 104px;
    flex: none;
  }

  /* The trash can: square at the height of every control. */
  .bin {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--control-h);
    padding: 0;
    flex: none;
  }
</style>
