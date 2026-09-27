<script lang="ts">
  // The settings, in the manner of the Updates page: a few groups of cards,
  // each card one subject, each row a mark, what it is in a few words and
  // the one thing to do about it. What a paying customer comes here for is
  // at the top: whether the app is ready, how clips are found, the speech
  // model, where shorts go and the app's colour. What the engine needs in
  // order to run, the paths to its tools and models, is behind Advanced,
  // for the day something has to be pointed somewhere by hand.
  //
  // Everything saves as it is changed, the way System Settings does on the
  // Mac. There is no Save button, so there is nothing to forget to press.
  import { onMount } from "svelte";
  import { wearColour } from "../lib/colour";
  import {
    api,
    errorText,
    languageRow,
    memorySize,
    speechRow,
    type Check,
    type LanguageModel,
    type Settings,
    type SpeechModel,
    type TrainingStatus,
  } from "../lib/api";
  import Confirm from "../components/Confirm.svelte";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import ModelList from "../components/ModelList.svelte";

  let settings = $state<Settings | null>(null);
  let checks = $state<Check[] | null>(null);
  let checking = $state(false);
  let problem = $state("");
  let advanced = $state(false);
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

  const speechRows = $derived(speech.map((m) => speechRow(m)));
  const languageRows = $derived(language.map((m) => languageRow(m, true)));

  // The Anthropic key. It is never read back: the Go side only ever says
  // whether one can be found.
  let key = $state("");
  let hasKey = $state(false);
  let savingKey = $state(false);
  let savedKey = $state(false);

  // Saving as things change. What was last saved, or last read, is kept as
  // text, and a change is only a change when the settings no longer read
  // the same. So reading the settings again, after a model was removed and
  // the Go side let go of it, is not taken for somebody changing them, and
  // does not put back what the Go side just took out.
  let lastSaved = "";
  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  const SAVE_AFTER = 400;

  $effect(() => {
    if (!settings) return;
    const now = JSON.stringify($state.snapshot(settings));
    if (now === lastSaved) return;
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => save(now), SAVE_AFTER);
  });

  async function save(text: string) {
    if (!settings || text === lastSaved) return;
    lastSaved = text;
    problem = "";
    try {
      await api.saveSettings(JSON.parse(text));
      await Promise.all([readTraining(), check()]);
    } catch (err) {
      problem = errorText(err);
    }
  }

  // Taking the settings as the Go side has them, without that counting as
  // a change to save.
  function took(s: Settings) {
    settings = s;
    lastSaved = JSON.stringify(s);
  }

  // The app takes the colour as it is picked, not when it is saved, so
  // what a colour looks like everywhere is what you are looking at while
  // you choose it.
  $effect(() => {
    if (settings?.appColour) wearColour(settings.appColour);
  });

  // The colours the Mac offers as an accent, in its own order, with the
  // app's own first. A person who wants another has the colour well at
  // the end of the row.
  const colours = [
    { hex: "#942192", name: "Purple, the app's own" },
    { hex: "#0a64d6", name: "Blue" },
    { hex: "#c2307a", name: "Pink" },
    { hex: "#d0342c", name: "Red" },
    { hex: "#d9731c", name: "Orange" },
    { hex: "#b98f08", name: "Yellow" },
    { hex: "#38923f", name: "Green" },
    { hex: "#6e7078", name: "Graphite" },
  ];
  const custom = $derived(
    !!settings && !colours.some((c) => c.hex === settings!.appColour?.toLowerCase()),
  );

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
    await Promise.all([readModels(), check()]);
  }

  // The one clips are found with is saved by the Go side at once, so the
  // settings are read again rather than saved over it.
  async function useModel(name: string) {
    await api.useLanguageModel(name);
    await readSettings();
    await check();
  }

  async function readSettings() {
    try {
      const s = await api.getSettings();
      // Only what the Go side changes by itself is taken: the model in use.
      // Everything else on the page is the person's, and may be on its way
      // to being saved.
      if (settings) {
        settings.llmModel = s.llmModel;
        const mine = JSON.parse(lastSaved || "{}");
        mine.llmModel = s.llmModel;
        lastSaved = JSON.stringify(mine);
      } else {
        took(s);
      }
    } catch (err) {
      problem = errorText(err);
    }
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

  async function modelsChanged() {
    await readModels();
    await readSettings();
    await check();
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

  async function chooseOutput() {
    try {
      const dir = await api.chooseFolder("Where shorts are saved");
      if (dir && settings) settings.outputDir = dir;
    } catch (err) {
      problem = errorText(err);
    }
  }

  // Where things stand, in the words of the first card: whether a short
  // can be made, and if not, what is in the way.
  const failed = $derived((checks ?? []).filter((c) => !c.ok));
  const counted = ["No", "One", "Two", "Three", "Four", "Five", "Six"];
  const speechName = $derived(speech.find((m) => m.installed)?.title ?? "");
  const clipsName = $derived(
    settings?.planner === "api"
      ? "the Claude API"
      : (language.find((m) => m.inUse)?.title ?? "a model on this machine"),
  );
  const standing = $derived.by((): { mark: string; head: string; more: string } => {
    if (checks === null) return { mark: "look", head: "Looking at this machine", more: "" };
    if (failed.length === 0) {
      return {
        mark: "ok",
        head: "Ready to make shorts",
        more: speechName
          ? `Speech is heard by ${speechName}, and clips are found with ${clipsName}.`
          : `Clips are found with ${clipsName}.`,
      };
    }
    return {
      mark: "err",
      head: `${counted[failed.length] ?? failed.length} ${failed.length === 1 ? "thing needs" : "things need"} attention`,
      more: "Until then a search or a render may not get far.",
    };
  });

  // What is in the way, in words somebody who only makes shorts can act
  // on. The check's own words are for whoever has to find a file, so they
  // stay under Advanced, and a check this does not know keeps them here too.
  function plainly(c: Check): string {
    switch (c.name) {
      case "ffmpeg":
        return "What reads and renders video is missing. Installing the app again brings it back.";
      case "llama-server":
        return "What runs a model on this machine is missing. Installing the app again brings it back, and until then the Claude API works.";
      case "Speech model":
        return "No speech model is installed. Install one under Speech.";
      case "Claude API key":
        return "There is no API key yet. Paste one under Finding clips.";
      default:
        return c.detail;
    }
  }

  const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

  onMount(() => {
    load();
    return () => clearTimeout(saveTimer);
  });

  async function load() {
    try {
      took(await api.getSettings());
    } catch (err) {
      problem = errorText(err);
    }
    await Promise.all([readTraining(), readModels(), check()]);
  }
</script>

<section class="scroll">
  <div class="page">
    {#if problem}<p class="error selectable">{problem}</p>{/if}

    <!-- Where things stand, and what is in the way. -->
    <div class="card">
      <div class="item">
        <span class="mark {standing.mark}" aria-hidden="true">
          {#if standing.mark === "ok"}
            <Icon name="check" size={16} />
          {:else}
            <span class="dot" class:busy={standing.mark === "look"} class:err={standing.mark === "err"}></span>
          {/if}
        </span>
        <div class="words">
          <span class="head">{standing.head}</span>
          {#if standing.more}<span class="small muted">{standing.more}</span>{/if}
        </div>
        <button class="act" onclick={check} disabled={checking} title="Look for the tools and models again"
          >{#if checking}<Busy />{/if}{checking ? "Checking" : "Check again"}</button
        >
      </div>
      {#each failed as c (c.name)}
        <div class="item">
          <span class="mark"></span>
          <div class="words">
            <span>{c.name}</span>
            <span class="small error selectable">{plainly(c)}</span>
          </div>
        </div>
      {/each}
    </div>

    {#if settings}
      <div class="group asks">
        <div class="headrow">
          <h2>Finding clips</h2>
          <span class="ask">
            <Info label="About finding clips" side="left">
              A language model reads the transcript and picks the moments worth clipping. The Claude
              API works on any machine and costs a few cents an episode. A model on this machine is
              free to run and needs the memory to hold it. Either way the video and the audio stay
              here: only the words are read.
              {#if settings.planner === "local"}
                <br /><br />
                A model runs from memory, and this machine has
                {memory > 0 ? memorySize(memory) : "not said how much"}. A bigger model reads the
                transcript better and needs more of it, and <b>Best for this machine</b> is the
                biggest it can hold. Models are fetched from the people who made them and kept in
                <b>~/.framefairy/models</b>.
              {/if}
            </Info>
          </span>
        </div>
        <div class="card" role="radiogroup" aria-label="How clips are found">
          {#each [{ value: "api", name: "Claude API", about: "Works on any machine. A few cents an episode, paid to Anthropic." }, { value: "local", name: "On this machine", about: "Free to run. Needs the memory to hold a model." }] as way (way.value)}
            {@const on = settings.planner === way.value}
            <div class="item">
              <button
                class="choose"
                role="radio"
                aria-checked={on}
                onclick={() => settings && (settings.planner = way.value === "api" ? "api" : "local")}
              >
                <span class="mark"><span class="radio" class:on></span></span>
                <span class="words">
                  <span class="head">{way.name}</span>
                  <span class="small muted">{way.about}</span>
                </span>
              </button>
            </div>
          {/each}
        </div>

        {#if settings.planner === "local"}
          <ModelList
            models={languageRows}
            kind="llm"
            oninstall={api.installLanguageModel}
            onchange={modelsChanged}
            onremove={api.removeLanguageModel}
            onuse={useModel}
          />
        {:else}
          <div class="card">
            <!-- The key goes in the keychain the moment it is saved, not
                 with the rest of these, because it never lands in the
                 settings file. An app opened from Finder has no shell
                 environment, so this is the only way to give it one. -->
            <div class="item">
              <span class="mark" class:ok={hasKey} aria-hidden="true">
                {#if hasKey}<Icon name="check" />{:else}<span class="dot warn"></span>{/if}
              </span>
              <div class="words">
                <span class="head">API key</span>
                <span class="small muted">
                  {#if savedKey}
                    Saved in the keychain, and nowhere else.
                  {:else if hasKey}
                    In the keychain, and nowhere else. Paste another to replace it.
                  {:else}
                    From console.anthropic.com. It goes in the keychain and nowhere else.
                  {/if}
                </span>
              </div>
              <input
                class="key"
                type="password"
                bind:value={key}
                placeholder={hasKey ? "Replace the key" : "sk-ant-..."}
                aria-label="Anthropic API key"
                autocomplete="off"
                spellcheck="false"
                onkeydown={(e) => e.key === "Enter" && key.trim() && saveKey()}
              />
              <button class="act" disabled={savingKey || !key.trim()} onclick={saveKey}>
                {#if savingKey}<Busy />{/if}
                {savingKey ? "Saving" : "Save key"}
              </button>
            </div>
          </div>
        {/if}
      </div>

      <!-- Every episode is transcribed on this machine, so the model has to
           be on it. It is the same list as the setup, and installing one
           here is the same job. -->
      <div class="group asks">
        <div class="headrow">
          <h2>Speech</h2>
          <span class="ask">
            <Info label="About the speech model" side="left">
              Every episode is transcribed on this machine, word by word with the time of each
              word. Nothing about the episode is sent anywhere. The model is fetched from the
              people who published it and kept in <b>~/.framefairy/models</b>.
            </Info>
          </span>
        </div>
        <ModelList
          models={speechRows}
          kind="model"
          oninstall={api.installSpeechModel}
          onchange={modelsChanged}
          onremove={api.removeSpeechModel}
          removeSays="Every episode is transcribed with it, so the app asks for one again the next time it starts."
        />
      </div>

      <div class="group">
        <h2>Shorts</h2>
        <div class="card">
          <div class="item">
            <span class="mark" aria-hidden="true"><Icon name="folder" /></span>
            <div class="words">
              <span class="head">Save shorts to</span>
              <span class="small muted selectable path">
                {settings.outputDir || "Next to each episode, in its .framefairy folder"}
              </span>
            </div>
            {#if settings.outputDir}
              <button
                class="quiet"
                title="Save each short next to its episode again"
                onclick={() => settings && (settings.outputDir = "")}>Next to episode</button
              >
            {/if}
            <button class="act" onclick={chooseOutput}>Choose…</button>
          </div>
        </div>
      </div>

      <!-- The colour the app picks things out in. The colours burned into a
           short, the words, their box and the pill behind the word being
           spoken, are set together in the captions column of the workspace,
           where the video preview and the clip timeline show them at once. -->
      <div class="group">
        <h2>Appearance</h2>
        <div class="card">
          <div class="item">
            <div class="words">
              <span class="head">Accent colour</span>
              <span class="small muted">What the app picks things out in. The colours of a short are set beside its captions.</span>
            </div>
            <div class="swatches" role="radiogroup" aria-label="Accent colour">
              {#each colours as c (c.hex)}
                {@const on = settings.appColour?.toLowerCase() === c.hex}
                <button
                  class="swatch"
                  class:on
                  role="radio"
                  aria-checked={on}
                  aria-label={c.name}
                  title={c.name}
                  style="--swatch: {c.hex}"
                  onclick={() => settings && (settings.appColour = c.hex)}
                ></button>
              {/each}
              <!-- Any other colour, in the system's own colour picker. -->
              <label class="swatch well" class:on={custom} title="Another colour">
                <input type="color" bind:value={settings.appColour} aria-label="Another colour" />
              </label>
            </div>
          </div>
        </div>
      </div>

      <!-- The records of every episode, in one folder of their own. They are
           not kept with an episode, so letting go of a video leaves them
           alone, and they are thrown away here and nowhere else. -->
      <div class="group">
        <h2>Training data</h2>
        <div class="card">
          <div class="item">
            <div class="words">
              <span class="head num">
                {plural(training.plans, "plan", "plans")}, {plural(training.decisions, "decision", "decisions")}
              </span>
              <span class="small muted">What the model proposed and what became of it, kept on this machine to make it better.</span>
            </div>
            <button
              class="quiet danger bin"
              disabled={training.plans + training.decisions === 0}
              title="Remove every training record"
              aria-label="Remove the training data"
              aria-haspopup="dialog"
              onclick={() => (clearing = true)}
            >
              <Icon name="trash" />
            </button>
          </div>
        </div>
      </div>

      <!-- The machinery: where the tools and the models are, for the day
           one has to be pointed somewhere by hand. Empty uses what the app
           finds by itself, the same as the command line. -->
      <div class="group">
        <button
          class="quiet disclose"
          aria-expanded={advanced}
          onclick={() => (advanced = !advanced)}
        >
          <span class="chevron" class:open={advanced}><Icon name="chevron" size={14} /></span>
          <h2>Advanced</h2>
        </button>
        {#if advanced}
          <div class="card">
            {#if settings.planner === "local"}
              <label class="item field">
                <span class="words">
                  <span class="head">Language model file</span>
                  <span class="small muted">Chosen above, or any .gguf file.</span>
                </span>
                <input type="text" bind:value={settings.llmModel} placeholder="The one in use" spellcheck="false" />
              </label>
              <label class="item field">
                <span class="words">
                  <span class="head">llama-server</span>
                  <span class="small muted">What runs the model.</span>
                </span>
                <input type="text" bind:value={settings.llmServer} placeholder="Beside the app, then the search path" spellcheck="false" />
              </label>
            {:else}
              <label class="item field">
                <span class="words">
                  <span class="head">Claude model</span>
                  <span class="small muted">Which model the API is asked.</span>
                </span>
                <input type="text" bind:value={settings.apiModel} placeholder="claude-sonnet-5" spellcheck="false" />
              </label>
            {/if}
            <label class="item field">
              <span class="words">
                <span class="head">Speech model folder</span>
                <span class="small muted">The installed one, unless named.</span>
              </span>
              <input type="text" bind:value={settings.asrModel} placeholder="~/.framefairy/models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8" spellcheck="false" />
            </label>
            <label class="item field">
              <span class="words">
                <span class="head">ffmpeg</span>
                <span class="small muted">What reads and renders video.</span>
              </span>
              <input type="text" bind:value={settings.ffmpeg} placeholder="Beside the app, then the search path" spellcheck="false" />
            </label>
            <label class="item field">
              <span class="words">
                <span class="head">Training data folder</span>
                <span class="small muted">Where the records go.</span>
              </span>
              <input type="text" bind:value={settings.trainingDir} placeholder="~/.framefairy/training" spellcheck="false" />
            </label>
          </div>

          <!-- Everything the check looked at, found or not, for whoever
               needs to know which ffmpeg or which model it found. -->
          <div class="card">
            {#each checks ?? [] as c (c.name)}
              <div class="item found">
                <span class="mark" class:ok={c.ok} aria-hidden="true">
                  {#if c.ok}<Icon name="check" />{:else}<span class="dot err"></span>{/if}
                </span>
                <div class="words">
                  <span>{c.name}</span>
                  <span class="small selectable" class:muted={c.ok} class:error={!c.ok}>{c.detail}</span>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    {/if}
  </div>

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
  /* The page scrolls as a whole, so its scroll bar is at the edge of the
     app. What is on it keeps to a width that reads, in the middle, the
     same width as the Updates page. */
  section {
    flex: 1;
    min-height: 0;
    width: 100%;
  }

  .page {
    display: flex;
    flex-direction: column;
    /* Between two subjects, which is further than between two cards of
       one subject, so the groups read as groups. */
    gap: var(--edge);
    max-width: 880px;
    margin: 0 auto;
    padding: var(--gap) var(--edge) var(--edge);
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
  }

  /* The head of a group, with the mark that explains the group at its
     right end, which is the group's top right corner. */
  .headrow {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  /* The head of a group, the same size and weight as the head of the clip
     list and of every area. */
  h2 {
    font-size: var(--size-l);
    line-height: 23px;
    font-weight: 600;
  }

  /* A row of a choice: the mark and the words, pressed as one. */
  .choose {
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

  .choose[aria-checked="true"] {
    cursor: default;
  }

  .item:has(.choose[aria-checked="false"]:hover) {
    background: var(--ink-2);
  }

  /* Room for the longest wording, so a row keeps still whatever its
     button says. */
  .act {
    min-width: 116px;
    flex: none;
  }

  .key {
    width: 240px;
    flex: none;
  }

  .path {
    overflow-wrap: anywhere;
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

  /* The accent colours, round, the way the Mac shows them. The chosen one
     carries a ring in its own colour a step out from it. */
  .swatches {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: none;
  }

  .swatch {
    position: relative;
    width: 20px;
    height: 20px;
    padding: 0;
    border-radius: 50%;
    border: none;
    background: var(--swatch);
    flex: none;
  }

  .swatch:hover:not(:disabled) {
    background: var(--swatch);
    filter: brightness(1.15);
  }

  .swatch.on {
    box-shadow:
      0 0 0 2px var(--ink-1),
      0 0 0 4px var(--swatch, var(--accent));
  }

  /* Any other colour: the wheel the Mac draws for the same thing. */
  .well {
    background: conic-gradient(#e0605a, #d9b83c, #5fa37a, #3c9fd9, #8a5cd9, #d95cb5, #e0605a);
    cursor: pointer;
  }

  .well.on {
    --swatch: var(--accent);
  }

  .well input {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    cursor: pointer;
    padding: 0;
    border: none;
  }

  /* Advanced opens and closes like a disclosure in Finder: a chevron that
     turns, and the head of the group beside it. */
  .disclose {
    display: flex;
    align-items: center;
    gap: 6px;
    align-self: flex-start;
    height: auto;
    padding: 0 8px 0 0;
    margin-left: -2px;
    color: var(--text);
  }

  .disclose:hover:not(:disabled) {
    background: transparent;
  }

  .chevron {
    display: flex;
    color: var(--muted);
    transition: transform 0.12s ease;
  }

  .chevron.open {
    transform: rotate(90deg);
  }

  @media (prefers-reduced-motion: reduce) {
    .chevron {
      transition: none;
    }
  }

  /* A path is a name on the left and a field on the right, the field half
     the card, so every field in the card starts in one column. */
  .field .words {
    flex: 0 0 40%;
  }

  .field input {
    flex: 1;
    min-width: 0;
  }

  .found {
    min-height: 0;
  }
</style>
