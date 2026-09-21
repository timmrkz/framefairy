<script lang="ts">
  import { onMount } from "svelte";
  import { wearColour } from "../lib/colour";
  import { api, errorText, type Check, type Settings, type TrainingStatus } from "../lib/api";
  import Confirm from "../components/Confirm.svelte";
  import Icon from "../components/Icon.svelte";

  let settings = $state<Settings | null>(null);
  let checks = $state<Check[]>([]);
  let checking = $state(false);
  let saved = $state(false);
  let problem = $state("");
  // Where the training records are and how many there are, which is what
  // makes the trash can beside them mean anything.
  let training = $state<TrainingStatus>({ dir: "", plans: 0, decisions: 0 });
  let clearing = $state(false);

  async function readTraining() {
    try {
      training = await api.training();
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function clearTraining() {
    clearing = false;
    try {
      await api.clearTraining();
      await readTraining();
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function check() {
    checking = true;
    try {
      checks = await api.checkSetup();
    } catch (err) {
      problem = errorText(err);
    }
    checking = false;
  }

  // The app takes the colour as it is picked, not when it is saved, so
  // what a colour looks like everywhere is what you are looking at while
  // you choose it.
  $effect(() => {
    if (settings?.appColour) wearColour(settings.appColour);
  });

  async function save() {
    if (!settings) return;
    problem = "";
    try {
      await api.saveSettings($state.snapshot(settings));
      saved = true;
      setTimeout(() => (saved = false), 1800);
      await readTraining();
      await check();
    } catch (err) {
      problem = errorText(err);
    }
  }

  onMount(async () => {
    settings = await api.getSettings();
    await readTraining();
    await check();
  });
</script>

<section class="scroll">
  <header class="row">
    <span class="grow"></span>
    {#if saved}<span class="muted">Saved</span>{/if}
    <button class="primary" onclick={save} disabled={!settings}>Save</button>
  </header>
  {#if problem}<p class="error selectable">{problem}</p>{/if}

  <div class="panel">
    <div class="row">
      <h2>Setup</h2>
      <span class="grow"></span>
      <button onclick={check} disabled={checking}>{checking ? "Checking" : "Check again"}</button>
    </div>
    <ul>
      {#each checks as c (c.name)}
        <li>
          <span class="dot {c.ok ? 'ok' : 'err'}"></span>
          <span>{c.name}</span>
          <span class="detail selectable" class:error={!c.ok} class:muted={c.ok}>{c.detail}</span>
        </li>
      {/each}
    </ul>
  </div>

  {#if settings}
    <div class="panel">
      <!-- How many clips and how long they are belong to the episode being
           worked on, so they are set in the workspace and nowhere else.
           What is left here is the model that does the finding, which is
           about the machine. -->
      <h2>Finding clips</h2>
      <div class="grid">
        <label for="planner">Model</label>
        <select id="planner" bind:value={settings.planner}>
          <option value="local">On this machine</option>
          <option value="api">Claude API</option>
        </select>
        {#if settings.planner === "local"}
          <label for="llm">Language model file</label>
          <input id="llm" type="text" bind:value={settings.llmModel} placeholder="The only .gguf file in ~/.framefairy/models" />
          <label for="server">llama-server</label>
          <input id="server" type="text" bind:value={settings.llmServer} placeholder="Found on the search path" />
        {:else}
          <label for="api">API model</label>
          <input id="api" type="text" bind:value={settings.apiModel} />
        {/if}
      </div>
    </div>

    <div class="panel">
      <h2>Transcription and rendering</h2>
      <div class="grid">
        <label for="asr">Speech model folder</label>
        <input id="asr" type="text" bind:value={settings.asrModel} placeholder="~/.framefairy/models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8" />
        <label for="ffmpeg">ffmpeg</label>
        <input id="ffmpeg" type="text" bind:value={settings.ffmpeg} placeholder="Found on the search path" />
        <label for="out">Output folder</label>
        <input id="out" type="text" bind:value={settings.outputDir} placeholder="Next to each episode, in its .framefairy folder" />
      </div>
    </div>

    <!-- The two colours together, because the whole point is that they are
         two. One is what the app picks things out in, the other is burned
         into the framefairy, and a taste in one is not a taste in the other.
         They start out the same, so an app nobody has touched looks of a
         piece with what it makes. -->
    <div class="panel">
      <h2>Colours</h2>
      <div class="grid">
        <label for="app-colour">The app</label>
        <div class="row">
          <input id="app-colour" type="color" bind:value={settings.appColour} />
          <input
            type="text"
            class="hex num"
            bind:value={settings.appColour}
            aria-label="The app's colour as hex"
          />
        </div>
        <label for="colour">The word highlight</label>
        <div class="row">
          <input id="colour" type="color" bind:value={settings.highlightColour} />
          <input type="text" class="hex num" bind:value={settings.highlightColour} aria-label="Highlight colour as hex" />
        </div>
      </div>
    </div>

    <!-- The records of every episode, in one folder of their own. They are
         not kept with an episode, so letting go of a video leaves them
         alone, and they are thrown away here and nowhere else. -->
    <div class="panel">
      <h2>Training data</h2>
      <div class="grid">
        <label for="training">Folder</label>
        <input
          id="training"
          type="text"
          bind:value={settings.trainingDir}
          placeholder="~/.framefairy/training"
        />
        <span>Records</span>
        <div class="row">
          <span class="muted num">
            {training.plans}
            {training.plans === 1 ? "plan" : "plans"}, {training.decisions}
            {training.decisions === 1 ? "decision" : "decisions"}
          </span>
          <span class="grow"></span>
          <button
            class="quiet danger drop"
            disabled={training.plans + training.decisions === 0}
            title="Remove every training record"
            aria-haspopup="dialog"
            onclick={() => (clearing = true)}
          >
            <Icon name="trash" />
          </button>
        </div>
      </div>
    </div>
  {/if}

  {#if clearing}
    <Confirm title="Remove the training data?" oncancel={() => (clearing = false)}>
      <p>
        Every plan and every decision recorded so far, in <b>{training.dir}</b>, goes. The clips
        themselves stay. This cannot be taken back.
      </p>
      {#snippet actions()}
        <button onclick={() => (clearing = false)}>Cancel</button>
        <button class="danger" onclick={clearTraining}>Remove them</button>
      {/snippet}
    </Confirm>
  {/if}
</section>

<style>
  /* A setting is a name and a control beside it, which only gets harder to
     read the wider it is. So the page keeps its width and stands in the
     middle of the window rather than against the left of it. */
  section {
    padding: var(--gap) var(--edge) var(--edge);
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: 880px;
    margin: 0 auto;
  }

  h2 {
    font-size: var(--size-l);
    font-weight: 600;
  }

  .grow {
    flex: 1;
  }

  .panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  li {
    display: grid;
    grid-template-columns: 16px 220px 1fr;
    gap: 8px;
    align-items: baseline;
    padding: 6px 0;
  }

  .detail {
    white-space: pre-wrap;
    font-size: var(--size-s);
  }

  /* The one way to throw the records away: a quiet mark that turns red
     under the pointer, the same as the trash can on a clip. */
  .drop {
    display: flex;
    align-items: center;
    justify-content: center;
    width: var(--control-h);
    padding: 0;
    color: var(--muted);
  }

  .drop:hover:not(:disabled) {
    color: var(--err);
  }

  .grid {
    display: grid;
    grid-template-columns: 200px 1fr;
    gap: 8px 16px;
    align-items: center;
  }

  .grid label {
    color: var(--muted);
  }

  input[type="color"] {
    width: 48px;
    padding: 2px;
  }

  .hex {
    width: 110px;
  }
</style>
