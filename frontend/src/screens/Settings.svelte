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
  import { jobs, nav } from "../lib/state.svelte";
  import {
    cloudModelIn,
    cloudValue,
    finderOptions as finderOptions_,
    finderStanding,
    holdsTheApp,
    providerOf,
  } from "../lib/finding";
  import {
    api,
    errorText,
    languageRow,
    memorySize,
    size,
    speechRow,
    type Check,
    type CloudModel,
    type Provider,
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
  import Pick from "../components/Pick.svelte";

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


  // The companies in the cloud, the models of theirs the app offers by
  // name, and whether a key of each can be found. A key is never read
  // back: the Go side only ever says whether there is one.
  let providers = $state<Provider[]>([]);
  let cloud = $state<CloudModel[]>([]);
  let keys = $state<Record<string, boolean>>({});
  // The company of the model named, as the Go side last said. A model the
  // app offers is known here at once; one written in by hand is known once
  // the settings are saved and read again.
  let providerSaid = $state("anthropic");
  let key = $state("");
  let savingKey = $state(false);
  let savedKey = $state(false);

  // The model in the cloud, its company, and whether that company's key is
  // here. Which company decides whose key the row asks for.
  const cloudModel = $derived(settings?.apiModel || cloud[0]?.model || "claude-sonnet-5");
  const provider = $derived(providerOf(cloudModel, cloud, providers, providerSaid));
  const hasKey = $derived(!!keys[provider.name]);

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
      await Promise.all([readTraining(), readModels(), check()]);
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
      await api.saveAPIKey(provider.name, key);
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
  // The list says which model is in use from the models as the Go side
  // last reported them, so they are read again too. Reading only the
  // settings left the list on the model before, and a choice looked as if
  // it had done nothing.
  async function useModel(name: string) {
    try {
      await api.useLanguageModel(name);
    } catch (err) {
      problem = errorText(err);
    }
    await Promise.all([readModels(), readSettings()]);
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
      providers = state.providers ?? [];
      cloud = state.cloud ?? [];
      keys = state.keys ?? {};
      providerSaid = state.provider || "anthropic";
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

  // What each check that failed is about, so it can be said in the row
  // where it is put right rather than in a list of its own at the top.
  const failed = $derived((checks ?? []).filter((c) => !c.ok));
  const broken = (name: string) => failed.find((c) => c.name === name);
  // A check this page has no row for is said at the top, so nothing that
  // is in the way goes unsaid.
  const homeless = $derived(
    failed.filter(
      (c) =>
        !["ffmpeg", "llama-server", "Speech model", "Language model"].includes(c.name) &&
        !c.name.endsWith(" API key"),
    ),
  );

  // What finds the clips, as one choice: a model in the cloud, from any of
  // the companies the app knows, or one of the models that run here, in
  // one list, the way the apps that offer models
  // put them, grouped by where they run. So the list holds any number of
  // models in the room of one control.
  //
  // Choosing a model chooses it, whether it is on this machine or not. One
  // that is not here yet is the choice all the same, and it cannot find
  // clips until it is downloaded, which is a step of its own: a download of
  // gigabytes starts when Download is pressed, never as a side effect of
  // looking through a list. The mark, the frame round the list and the
  // line under it all say so. Keeping the model before it in use meanwhile
  // made the list say one model while the searches ran on another.
  const inUse = $derived(language.find((m) => m.inUse));
  // What was just chosen, shown until the Go side's answer is read, so a
  // click shows at once.
  let choosing = $state("");
  const notHere = $derived(!!inUse && !inUse.installed);
  let fetchFailed = $state("");
  const llmJob = $derived(jobs.list.filter((j) => j.kind === "llm").at(-1));
  const llmRunning = $derived(
    llmJob && (llmJob.state === "running" || llmJob.state === "queued") ? llmJob : undefined,
  );
  // Download pressed and the job not reported yet.
  let starting = $state(false);
  // Whatever is being downloaded, even one started before this page
  // opened, is this page's to show.
  const fetchingModel = $derived(
    llmRunning ? language.find((m) => m.title === llmRunning.label) : starting ? inUse : undefined,
  );

  const finder = $derived(
    settings?.planner === "api" ? cloudValue(cloudModel) : choosing || (inUse?.name ?? ""),
  );

  const finderOptions = $derived(finderOptions_(cloud, providers, language, cloudModel));

  async function pickFinder(value: string) {
    if (!settings) return;
    fetchFailed = "";
    const inCloud = cloudModelIn(value);
    if (inCloud !== null) {
      settings.planner = "api";
      settings.apiModel = inCloud;
      return;
    }
    settings.planner = "local";
    choosing = value;
    await useModel(value);
    choosing = "";
  }

  async function download() {
    if (!inUse || inUse.installed) return;
    fetchFailed = "";
    // A click shows at once: the row says it is downloading before the
    // first job event arrives.
    starting = true;
    try {
      const job = await api.installLanguageModel(inUse.name);
      if (job.state === "failed") fetchFailed = job.error ?? "The download did not start.";
    } catch (err) {
      fetchFailed = errorText(err);
    }
    starting = false;
  }

  // When a download ends the models are read again: the model it fetched
  // is already the one chosen, so being there is all that changes. One
  // that stopped says why, where it was asked for. Read when the job
  // settles, not on every job event, which arrive about once a second
  // while it runs.
  let settled = "";
  $effect(() => {
    const job = llmJob;
    const now = job ? `${job.id}:${job.state}` : "";
    if (now === settled) return;
    settled = now;
    if (!job || job.state === "running" || job.state === "queued") return;
    if (job.state === "failed") fetchFailed = job.error ?? "The download stopped.";
    modelsChanged();
  });

  let cancelling = $state(false);
  async function cancelFetch() {
    if (!llmRunning) return;
    cancelling = true;
    try {
      await api.cancelJob(llmRunning.id);
    } catch (err) {
      problem = errorText(err);
    }
  }
  $effect(() => {
    if (!llmRunning) cancelling = false;
  });

  // The one line under Find clips with. One line and never two: it is cut
  // short with the rest in its title rather than wrapped, because a line
  // that wraps when a button arrives beside it moves the whole card. So
  // every wording here is short enough to fit beside the list and a
  // button, and what does not fit is for the info mark.
  const finding = $derived(
    finderStanding({
      planner: settings?.planner === "api" ? "api" : "local",
      provider,
      hasKey,
      inUse,
      fetching: fetchingModel,
      progress: llmRunning?.progress,
      failed: fetchFailed,
      modelProblem: broken("Language model")?.detail,
      serverMissing: !!broken("llama-server"),
    }),
  );
  const finderLine = $derived(finding);
  const finderState = $derived(finding.state);

  // The models on this machine, which is only about the room they take,
  // so it stays folded until somebody wants some of it back.
  let downloads = $state(false);
  const installedRows = $derived(
    language
      .filter((m) => m.installed)
      .map((m) => ({
        ...languageRow(m),
        cost: `By ${m.maker}. ${size(m.download)} on disk`,
        note: m.inUse ? "Finds the clips now" : "",
        warn: false,
      })),
  );
  const installedSize = $derived(
    size(language.filter((m) => m.installed).reduce((sum, m) => sum + m.download, 0)),
  );

  const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

  // The settings are not left while what finds the clips cannot find any:
  // a model chosen and not downloaded, a key missing, nothing chosen. Going
  // on from there is going on to an app that fails every search, far from
  // the one place it can be put right. So a move away is refused, and the
  // card that needs the answer comes into view and shakes, the way the
  // Mac's own password field does when it will not let somebody in, with
  // the keyboard on the list where the answer is. A download on its way
  // is not held: it is being put right. Nor is a page still reading what
  // it has to show.
  let findingCard = $state<HTMLElement>();
  let shaking = $state(false);
  let loaded = $state(false);
  const held = $derived(holdsTheApp(finderState, loaded));
  function hold(): boolean {
    if (!held) return false;
    findingCard?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    findingCard?.querySelector<HTMLElement>("button.pick")?.focus();
    // Off and on again, so a second try shakes it again.
    shaking = false;
    requestAnimationFrame(() => (shaking = true));
    return true;
  }

  onMount(() => {
    load();
    nav.hold = hold;
    return () => {
      clearTimeout(saveTimer);
      if (nav.hold === hold) nav.hold = null;
    };
  });

  async function load() {
    try {
      took(await api.getSettings());
    } catch (err) {
      problem = errorText(err);
    }
    await Promise.all([readTraining(), readModels(), check()]);
    loaded = true;
  }
</script>

<section class="scroll">
  <div class="page">
    {#if problem}<p class="error selectable">{problem}</p>{/if}

    <!-- What is in the way and has no row of its own below. Everything
         else is said in the row where it is put right. -->
    {#each homeless as c (c.name)}
      <div class="card">
        <div class="item">
          <span class="mark" aria-hidden="true"><span class="dot err"></span></span>
          <div class="words">
            <span class="head">{c.name}</span>
            <span class="small error selectable">{c.detail}</span>
          </div>
        </div>
      </div>
    {/each}

    {#if settings}
      <div class="group asks">
        <div class="headrow">
          <h2>Finding clips</h2>
          <span class="ask">
            <Info label="About finding clips" side="left">
              A language model reads the transcript and picks the moments worth clipping. A model in
              the cloud, Anthropic's or OpenAI's, works on any machine with a key of your own from
              that company, and costs a few cents an episode. A model on this machine is free to run
              and needs the memory to hold it. Either way the video and the audio stay
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
        <div
          class="card finding"
          class:shaking
          bind:this={findingCard}
          onanimationend={() => (shaking = false)}
        >
          <!-- The one decision, and under it the rows that depend on it, in
               the same card, the way a pop-up in the Mac's own settings
               changes the rows beneath it. -->
          <div class="item">
            <span class="mark {finderState}" aria-hidden="true">
              {#if finderState === "busy"}
                <span class="dot busy"></span>
              {:else if finderState === "ok"}
                <Icon name="check" />
              {:else}
                <Icon name="warn" />
              {/if}
            </span>
            <div class="words">
              <span class="head">Find clips with</span>
              <span
                class="small line"
                class:muted={!finderLine.tone}
                class:warn={finderLine.tone === "warn"}
                class:error={finderLine.tone === "err"}
                title={finderLine.text}>{finderLine.text}</span
              >
            </div>
            {#if llmRunning}
              <!-- The download carries the beam and the fill in the one
                   control that can do something about it. -->
              <button
                class="act"
                title="Stop the download. What has arrived stays, and Download carries on from it"
                disabled={cancelling}
                onclick={cancelFetch}
              >
                <Busy
                  fraction={llmRunning.progress && llmRunning.progress.fraction >= 0
                    ? llmRunning.progress.fraction
                    : -1}
                />
                {cancelling ? "Cancelling" : "Cancel"}
              </button>
            {:else if notHere && settings.planner === "local"}
              <button
                class="act"
                title="Fetch {inUse?.title}, {size(inUse?.download ?? 0)}. It finds clips once it is here"
                disabled={starting}
                onclick={download}
                >{#if starting}<Busy />{/if}{starting ? "Starting" : "Download"}</button
              >
            {/if}
            <Pick
              value={finder || "Choose a model"}
              options={finderOptions}
              onpick={pickFinder}
              label="Find clips with"
              align="right"
              tone={finderState === "warn" || finderState === "err" ? finderState : undefined}
              disabled={!!llmRunning || starting}
              title="What reads the transcript and picks the moments"
            />
          </div>

          {#if settings.planner === "api"}
            <!-- The key goes in the keychain the moment it is saved, not
                 with the rest of these, because it never lands in the
                 settings file. An app opened from Finder has no shell
                 environment, so this is the only way to give it one. -->
            <div class="item">
              <span class="mark" class:ok={hasKey} class:warn={!hasKey} aria-hidden="true">
                {#if hasKey}<Icon name="check" />{:else}<Icon name="warn" />{/if}
              </span>
              <div class="words">
                <span class="head">{provider.title} API key</span>
                <span class="small line" class:muted={hasKey} class:warn={!hasKey}>
                  {savedKey ? "Saved in the keychain." : hasKey ? "In the keychain." : `None yet. Get one at ${provider.keysAt}.`}
                </span>
              </div>
              <input
                class="key"
                type="password"
                bind:value={key}
                placeholder={hasKey ? "Replace the key" : provider.name === "openai" ? "sk-proj-..." : "sk-ant-..."}
                aria-label="{provider.title} API key"
                title="It goes in the keychain and nowhere else"
                autocomplete="off"
                spellcheck="false"
                onkeydown={(e) => e.key === "Enter" && key.trim() && saveKey()}
              />
              <button class="act" disabled={savingKey || !key.trim()} onclick={saveKey}>
                {#if savingKey}<Busy />{/if}
                {savingKey ? "Saving" : "Save key"}
              </button>
            </div>
          {:else}
            {#if broken("llama-server")}
              <!-- A model is the answer and llama-server is what runs it,
                   so one without the other finds nothing. -->
              <div class="item">
                <span class="mark err" aria-hidden="true"><Icon name="warn" /></span>
                <div class="words">
                  <span class="head">llama-server</span>
                  <span
                    class="small error line"
                    title="What runs a model on this machine is missing. Installing the app again brings it back, and until then a model in the cloud works."
                    >Missing. Installing the app again brings it back.</span
                  >
                </div>
              </div>
            {/if}
            {#if installedRows.length > 0}
              <!-- The models on this machine, which is only about the room
                   they take, folded until somebody wants some of it back. -->
              <button class="item disclose-row" aria-expanded={downloads} onclick={() => (downloads = !downloads)}>
                <span class="mark" aria-hidden="true"><Icon name="folder" /></span>
                <span class="words">
                  <span class="head">Downloaded models</span>
                  <span class="small muted">
                    {plural(installedRows.length, "model", "models")} on this machine, {installedSize}.
                  </span>
                </span>
                <span class="chevron" class:open={downloads}><Icon name="chevron" size={14} /></span>
              </button>
              {#if downloads}
                <ModelList
                  bare
                  models={installedRows}
                  kind="llm"
                  oninstall={api.installLanguageModel}
                  onchange={modelsChanged}
                  onremove={api.removeLanguageModel}
                />
              {/if}
            {/if}
          {/if}
        </div>
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
                {settings.outputDir || "Next to each episode"}
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
          {#if broken("ffmpeg")}
            <div class="item">
              <span class="mark err" aria-hidden="true"><Icon name="warn" /></span>
              <div class="words">
                <span class="head">ffmpeg</span>
                <span
                  class="small error line"
                  title="What reads and renders video is missing. Installing the app again brings it back."
                  >Missing. Installing the app again brings it back.</span
                >
              </div>
            </div>
          {/if}
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
                  <span class="head">Model in the cloud</span>
                  <span class="small muted">Which model the API is asked.</span>
                </span>
                <input type="text" bind:value={settings.apiModel} placeholder={cloud[0]?.model ?? "claude-sonnet-5"} spellcheck="false" />
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
            <div class="item">
              <div class="words">
                <span class="head">What the app found</span>
                <span class="small muted">Looked for again whenever a setting changes.</span>
              </div>
              <button class="act" onclick={check} disabled={checking} title="Look for the tools and models again"
                >{#if checking}<Busy />{/if}{checking ? "Checking" : "Check again"}</button
              >
            </div>
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

  /* Not yet: the card that needs an answer shakes from side to side and
     settles, the way the Mac's password field does, a few times and less
     each time. Without motion it is lit for a moment instead. */
  .finding.shaking {
    animation: shake 0.45s cubic-bezier(0.36, 0.07, 0.19, 0.97);
  }

  @keyframes shake {
    20% {
      transform: translateX(-8px);
    }
    40% {
      transform: translateX(8px);
    }
    60% {
      transform: translateX(-5px);
    }
    80% {
      transform: translateX(3px);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .finding.shaking {
      animation: lit 0.6s ease;
    }
  }

  @keyframes lit {
    30% {
      border-color: var(--warn);
    }
  }

  /* A line under a name is one line. It is cut short, with the whole of it
     in its title, rather than wrapped, because a line that wraps when a
     button arrives beside it moves the card. */
  .line {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .warn {
    color: var(--warn);
  }

  .mark.warn {
    color: var(--warn);
  }

  .mark.err {
    color: var(--err);
  }

  /* A row that opens what is under it: the whole row is the button, and
     the chevron at its end turns, the way a row in the Mac's own settings
     leads on. */
  .disclose-row {
    width: 100%;
    height: auto;
    border: none;
    border-radius: 0;
    background: transparent;
    text-align: left;
    white-space: normal;
  }

  .disclose-row:hover:not(:disabled) {
    background: var(--ink-2);
  }

  /* The list that says what finds the clips is as wide as the longest
     name in it, and hangs from the card's right edge. */
  .card :global(button.pick) {
    width: max-content;
    flex: none;
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
