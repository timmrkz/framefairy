<script lang="ts">
  import { onMount } from "svelte";
  import { wearColour } from "../lib/colour";
  import {
    api,
    errorText,
    fitNote,
    memorySize,
    size,
    type Check,
    type LanguageModel,
    type ModelRow,
    type Settings,
    type SpeechModel,
    type TrainingStatus,
  } from "../lib/api";
  import Confirm from "../components/Confirm.svelte";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import ModelList from "../components/ModelList.svelte";
  import Pick from "../components/Pick.svelte";

  let settings = $state<Settings | null>(null);
  let checks = $state<Check[]>([]);
  let checking = $state(false);
  let saved = $state(false);
  let problem = $state("");
  // Where the training records are and how many there are, which is what
  // makes the trash can beside them mean anything.
  let training = $state<TrainingStatus>({ dir: "", plans: 0, decisions: 0 });
  let clearing = $state(false);
  // The models that can be installed, and what the machine can hold. The
  // same lists the setup shows, so one can be added or swapped later
  // without going through the setup again.
  let speech = $state<SpeechModel[]>([]);
  let language = $state<LanguageModel[]>([]);
  // What this machine has, in bytes, or zero where it would not say.
  let memory = $state(0);

  const speechRows = $derived<ModelRow[]>(
    speech.map((m) => ({
      name: m.name,
      label: m.title,
      title: m.title,
      about: m.about,
      cost: `${m.languages}. ${size(m.download)} to fetch, ${size(m.unpacked)} on disk`,
      room: size(m.unpacked),
      installed: m.installed,
    })),
  );

  const languageRows = $derived<ModelRow[]>(
    language.map((m) => {
      const { note, warn } = fitNote(m.fit, m.recommended);
      return {
        name: m.name,
        label: m.title,
        title: `${m.title} by ${m.maker}`,
        about: m.about,
        cost: `${size(m.download)} to fetch, ${memorySize(m.needs)} of memory to run`,
        room: size(m.download),
        inUse: m.inUse,
        installed: m.installed,
        note,
        warn,
      };
    }),
  );

  // The Anthropic key. It is never read back: the Go side only ever says
  // whether one can be found.
  let key = $state("");
  let hasKey = $state(false);
  let savingKey = $state(false);
  let savedKey = $state(false);

  async function saveKey() {
    savingKey = true;
    problem = "";
    try {
      await api.saveAPIKey(key);
      key = "";
      savedKey = true;
      setTimeout(() => (savedKey = false), 1800);
    } catch (err) {
      problem = errorText(err);
    }
    savingKey = false;
    await readModels();
  }

  // The one clips are found with is saved at once, and the field below
  // shows it, so the next Save does not put the old one back.
  async function useModel(name: string) {
    const path = await api.useLanguageModel(name);
    if (settings) settings.llmModel = path;
    // The check at the top of the page says which model is in use, or
    // that none is, so it is read again with the choice.
    await check();
  }

  async function readModels() {
    try {
      const state = await api.setup();
      speech = state.speech;
      language = state.language;
      memory = state.memory;
      hasKey = state.hasKey;
    } catch (err) {
      problem = errorText(err);
    }
  }

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
    await readModels();
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
      <h2>This machine</h2>
      <span class="grow"></span>
      <button class="check" onclick={check} disabled={checking}
        >{#if checking}<Busy />{/if}{checking ? "Checking" : "Check again"}</button
      >
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
        <Pick
          value={settings.planner}
          onpick={(v) => {
            if (settings) settings.planner = v === "api" ? "api" : "local";
          }}
          options={[
            { value: "local", label: "On this machine" },
            { value: "api", label: "Claude API" },
          ]}
          id="planner"
          label="Model"
          title="Which model finds the clips"
        />
        {#if settings.planner === "local"}
          <!-- Not a label: what it names is a list, not a field, and the
               field below has the name that belongs to it. -->
          <span>Model</span>
          <div class="models">
            <p class="muted small">
              {memory > 0
                ? `This machine has ${memorySize(memory)} of memory. A model runs from it, so that is what decides which of these it can hold.`
                : "This machine did not say how much memory it has, so nothing below is promised."}
            </p>
            <ModelList
              models={languageRows}
              kind="llm"
              oninstall={api.installLanguageModel}
              onchange={readModels}
              onremove={api.removeLanguageModel}
              onuse={useModel}
            />
          </div>
          <label for="llm">Model file</label>
          <input id="llm" type="text" bind:value={settings.llmModel} placeholder="The only .gguf file in ~/.framefairy/models" />
          <label for="server">llama-server</label>
          <input id="server" type="text" bind:value={settings.llmServer} placeholder="Found beside the app, then on the search path" />
        {:else}
          <label for="api">API model</label>
          <input id="api" type="text" bind:value={settings.apiModel} />
          <!-- The key goes in the keychain the moment it is saved, not
               with the rest of these, because it never lands in the
               settings file. An app opened from Finder has no shell
               environment, so this is the only way to give it one. -->
          <label for="key">API key</label>
          <div class="row">
            <input
              id="key"
              type="password"
              bind:value={key}
              placeholder={hasKey ? "Replace the key" : "sk-ant-..."}
              autocomplete="off"
              spellcheck="false"
            />
            <button class="key" disabled={savingKey || !key.trim()} onclick={saveKey}>
              {#if savingKey}<Busy />{/if}
              {savingKey ? "Saving" : "Save key"}
            </button>
          </div>
          <span></span>
          <span class="muted small">
            {#if savedKey}
              Saved. It is in the keychain and nowhere else.
            {:else if hasKey}
              A key is in place. It is in the keychain and nowhere else.
            {:else}
              It goes in the keychain and nowhere else.
            {/if}
          </span>
        {/if}
      </div>
    </div>

    <!-- Every episode is transcribed on this machine, so the model has to
         be on it. It is the same list as the setup, and installing one
         here is the same job. -->
    <div class="panel">
      <h2>Speech</h2>
      <ModelList
        models={speechRows}
        kind="model"
        oninstall={api.installSpeechModel}
        onchange={readModels}
        onremove={api.removeSpeechModel}
        removeSays="Every episode is transcribed with it, so the app asks for one again the next time it starts."
      />
      <div class="grid">
        <label for="asr">Model folder</label>
        <input id="asr" type="text" bind:value={settings.asrModel} placeholder="~/.framefairy/models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8" />
      </div>
    </div>

    <div class="panel">
      <h2>Rendering</h2>
      <div class="grid">
        <label for="ffmpeg">ffmpeg</label>
        <input id="ffmpeg" type="text" bind:value={settings.ffmpeg} placeholder="Found beside the app, then on the search path" />
        <label for="out">Output folder</label>
        <input id="out" type="text" bind:value={settings.outputDir} placeholder="Next to each episode, in its .framefairy folder" />
      </div>
    </div>

    <!-- The colour the app picks things out in. The colours burned into a
         short, the words, their box and the pill behind the word being
         spoken, are set together in the captions column of the workspace,
         where the video preview and the clip timeline show them at once. -->
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
     middle of the app rather than against the left of it. */
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
  /* Room for the longer wording, so the row keeps still while the tools
     are being looked for. */
  .check {
    min-width: 116px;
  }

  .key {
    min-width: 104px;
  }

  /* The list of models stands in the control column of the grid, where a
     field would, so the name beside it lines up with every other name. */
  .models {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

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

  /* Every field in this column is its own width, no wider than what it
     holds, so the column is a column of fields and not a column of boxes
     stretched across the app. A list keeps room for the longest thing
     it can say, so it is that wide and stays that wide. */
  .grid :global(button.pick) {
    width: max-content;
  }
</style>
