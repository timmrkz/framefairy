<script lang="ts">
  // A list of models that can be installed, and installing one. The speech
  // models and the models that find clips are the same kind of thing on
  // screen, so they are one list: two lists of the same kind would have to
  // look the same anyway.
  //
  // No model ships with the app, so every copy fetches what it needs. The
  // list is a card of rows, one model each, one line each, read like a
  // table without its lines: the name, who made it, the memory it needs,
  // its size and what can be done with it, each in a column of its own, so
  // the same fact stands in the same place in every row. What the model is
  // made of is for whoever asks, in the title of its name.
  //
  // Which model finds clips is chosen in the settings from one list of all
  // of them, so this list only installs and removes. Rows with a round mark
  // for the model in use were tried here first, and a second card of
  // choices under the card choosing between the API and this machine read
  // as two things that had nothing to do with each other.
  //
  // An install is work in hand like any other, so it wears what all work
  // wears, in the control it was started from: the beam round the button
  // and the fill for how far it has come, and the button says Cancel while
  // it runs. The row does not move and does not change its shape: its
  // maker stays, its memory turns into how far the download has come and
  // its size into how long is left, each in its own column and lined up
  // the way the rest of that column is, so a row on its way reads like
  // the rows around it.
  //
  // An installed model can be removed again, to give its room back. It
  // asks first, because getting it back means fetching gigabytes again.
  import { api, clock, errorText, type Job, type ModelRow } from "../lib/api";
  import { jobs } from "../lib/state.svelte";
  import { sentence } from "../lib/steps";
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
    // How to remove one, where models can be removed. The setup has
    // nothing to remove.
    onremove,
    // What removing a model means beyond the room it gives back, said in
    // the box that asks first.
    removeSays = "",
    // Rows only, for a card that already holds the list: the models on
    // this machine, opened under the choice of which one finds clips.
    bare = false,
    // Why no model of the list may be removed now, or empty when one may:
    // a search is hearing or finding with it. The bin then asks nothing
    // and onlocked shows the no, where the list stands.
    locked = "",
    onlocked = undefined,
  }: {
    models: ModelRow[];
    kind: "model" | "llm";
    oninstall: (name: string) => Promise<Job>;
    onchange: () => void;
    onremove?: (name: string) => Promise<void>;
    removeSays?: string;
    bare?: boolean;
    locked?: string;
    onlocked?: () => void;
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

  // Cancelling takes a moment to reach the download, so the button says so
  // at once. The part that arrived stays, and the next Download carries on
  // from it.
  let cancelling = $state("");
  function cancel(job: Job) {
    cancelling = job.id;
    api.cancelJob(job.id);
  }

  // What a running download says in place of the memory it needs: how
  // far it has come, the way the size is said. A step with no bytes to count, the
  // unpacking, says what it is doing.
  function said(job: Job): string {
    if (job.state === "queued") return "Waiting for the download before it";
    const text = job.progress?.text ?? job.last?.text ?? "";
    const far = /[\d.]+ [KMG]B of [\d.]+ [KMG]B/.exec(text);
    return far ? far[0] : sentence(text) || "Starting";
  }

  // And in place of its size, how long is left.
  function left(job: Job): string {
    return job.progress && job.progress.remaining > 0 ? `${clock(job.progress.remaining)} left` : "";
  }

  const share = (job: Job) =>
    job.progress && job.progress.fraction >= 0 ? job.progress.fraction : -1;

  // The model the box is asking about.
  let removing = $state<ModelRow | null>(null);

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
<ul class="models" class:card={!bare} class:bare>
  {#each models as model (model.name)}
    <!-- Which row an install belongs to. The job carries the name the Go
         side gave it, which is the model's own title, so rows are matched
         on the job's name and not on what they show. -->
    {@const mine = running?.label === model.label ? running : undefined}
    {@const failed = !mine && job && job.label === model.label && job.state === "failed"}
    <li class="item">
      <!-- A model on its way wears the fill the way a clip card on its way
           does, across the whole row, the one place the work shows. -->
      {#if mine}<Busy fraction={share(mine)} />{/if}
      <span class="head name" title={model.about}>{model.title}</span>
      <span class="small muted cell">{model.maker}</span>
      {#if mine}
        {@const far = said(mine)}
        <span class="small muted num cell" title={far}>{far}</span>
        <span class="small muted num size cell">{left(mine)}</span>
      {:else}
        <span class="small cell" class:muted={!model.warn} class:warn={model.warn} title={model.note || undefined}
          >{model.memory}</span
        >
        <span class="small muted num size cell">{model.size}</span>
      {/if}
      <span class="end">
        {#if !model.installed}
          {#if mine}
            <button
              class="act"
              title="Stop the download. What has arrived stays, and Download carries on from it"
              disabled={cancelling === mine.id}
              onclick={() => cancel(mine)}
            >
              {cancelling === mine.id ? "Cancelling" : "Cancel"}
            </button>
          {:else}
            <button
              class="act"
              class:primary={model.best && !running}
              title="Download it and keep it on this machine"
              disabled={asked === model.name || !!running}
              onclick={() => install(model.name)}
            >
              {#if asked === model.name}<Busy />{/if}
              {asked === model.name ? "Starting" : "Download"}
            </button>
          {/if}
        {:else if onremove}
          <!-- Quiet until it is reached, then the colour of what it does,
               the way the trash can on a clip is. -->
          <button
            class="quiet danger bin"
            title={locked || "Remove it from this machine, to give its room back"}
            aria-label="Remove {model.title}"
            aria-haspopup="dialog"
            disabled={!!running}
            onclick={() => (locked ? onlocked?.() : (removing = model))}
          >
            <Icon name="trash" />
          </button>
        {/if}
      </span>
      {#if failed}
        <span class="small error selectable whole">{job?.error}</span>
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
  }

  /* Rows in a card that is already there: the line above the first one
     parts it from the row before, as between any two rows of a card. */
  ul.bare > li:first-child {
    border-top: 1px solid var(--line);
  }

  /* One grid for the whole list, and every row lays its cells on it, so
     the maker, the memory and the size of each row stand in the columns
     of the rows above and below, a table without its lines. The name
     takes what the other columns leave. */
  ul.models {
    display: grid;
    grid-template-columns:
      minmax(0, 1fr) var(--maker-w) var(--memory-w)
      var(--size-w) var(--end-w);
    /* Each column is as wide as what it holds in any list of models, a
       maker, the memory a model needs, a size and the Download button,
       and no wider for what one list holds. Two lists one above the
       other, the speech model and the clip model, then line up as one
       table, to the pixel: a column as wide as its widest word was a
       fraction of a pixel wider in one list than in the other. */
    --maker-w: 64px;
    --memory-w: 168px;
    --size-w: 72px;
    --end-w: 98px;
    column-gap: 24px;
  }

  ul.models > li {
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
    align-items: center;
  }

  /* Every cell of a row is one line, so every row is as high as the
     next. A word too long for its column is cut short rather than taking
     a second line. */
  .name,
  .cell {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .size {
    text-align: right;
  }

  /* What a model needs, when this machine has little more than that. */
  .warn {
    color: var(--warn);
  }

  /* The last column holds a button or nothing, and is as wide as the
     widest of its buttons, so the sizes before it line up. */
  .end {
    display: flex;
    justify-content: flex-end;
  }

  /* Why a download failed, on a line of its own under the row. */
  .whole {
    grid-column: 1 / -1;
  }

  .act {
    flex: none;
  }
</style>
