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
    type Job,
    type Provider,
    type LanguageModel,
    type Settings,
    type SpeechModel,
    type TrainingStatus,
    type LicenceLink,
    type LicenceState,
    onLicenceLink,
  } from "../lib/api";
  import Confirm from "../components/Confirm.svelte";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import ModelList from "../components/ModelList.svelte";
  import Colour from "../components/Colour.svelte";
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
  let keys = $state<Record<string, string>>({});
  // The company of the model named, as the Go side last said. A model the
  // app offers is known here at once; one written in by hand is known once
  // the settings are saved and read again.
  let providerSaid = $state("anthropic");
  let key = $state("");
  let savingKey = $state(false);
  let savedKey = $state(false);
  // Why the company refused the key just typed, said in the key's own row
  // until it is typed again.
  let keyRefused = $state("");
  // The key in short, as the Go side read it without the secret, and
  // whether the box that asks before it is removed is open.
  let keyHints = $state<Record<string, string>>({});
  // The field a key is typed in, and whether it is shaking off a key that
  // was refused.
  let keyField = $state<HTMLInputElement>();
  let keyShaking = $state(false);
  let removingKey = $state(false);

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

  // Only what changed goes to the Go side, key by key. The whole object
  // went once, and it put back whatever the Go side had changed since the
  // page last read the settings.
  async function save(text: string) {
    if (!settings || text === lastSaved) return;
    const before: Record<string, unknown> = lastSaved ? JSON.parse(lastSaved) : {};
    const after: Record<string, unknown> = JSON.parse(text);
    lastSaved = text;
    problem = "";
    const changed = Object.fromEntries(
      Object.entries(after).filter(([key, value]) => JSON.stringify(value) !== JSON.stringify(before[key])),
    );
    try {
      await api.saveSettings(changed);
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

  // The Go side asks the company whether it takes the key before keeping
  // it, so a key it refuses is found here, where it was typed.
  async function saveKey() {
    if (refuseFinding()) return;
    savingKey = true;
    keyRefused = "";
    try {
      await api.saveAPIKey(provider.name, key);
      key = "";
      savedKey = true;
      setTimeout(() => (savedKey = false), 1800);
    } catch (err) {
      // The Go side's errors start small, to sit inside a sentence.
      const said = errorText(err);
      keyRefused = said.charAt(0).toUpperCase() + said.slice(1);
      // The way the Mac's password fields take a wrong password: the field
      // shakes, keeps what was typed, selected, with the keyboard in it,
      // so the next paste replaces it. Save waits for something new.
      savingKey = false;
      keyShaking = false;
      requestAnimationFrame(() => {
        keyShaking = true;
        keyField?.focus();
        keyField?.select();
      });
    }
    savingKey = false;
    await Promise.all([readModels(), check()]);
  }

  // An empty key takes the saved one off the machine. It cannot be taken
  // back, the key is not kept anywhere else, so it asks first.
  async function removeKey() {
    removingKey = false;
    keyRefused = "";
    try {
      await api.saveAPIKey(provider.name, "");
    } catch (err) {
      const said = errorText(err);
      keyRefused = said.charAt(0).toUpperCase() + said.slice(1);
    }
    await Promise.all([readModels(), check()]);
  }

  // The licence key, kept in the keychain like an API key and never read
  // back: the Go side only says whether there is one and how it was
  // described when it was kept. A link from the mail a key came in has
  // already been dealt with by the Go side when it arrives here, see
  // linkOutcome in licence.go: its key is kept, in place of any kept
  // before, or it was refused. The row says which. The field is there only
  // while no key is kept, for a key that comes as text rather than as a
  // link, and the key itself is never shown: the key ID names it.
  let licence = $state<LicenceState>({ saved: false, about: "", waiting: false });
  let licenceKey = $state("");
  let fromLink = $state<LicenceLink | null>(null);
  let unlocking = $state(false);
  let licenceRefused = $state("");
  let licenceField = $state<HTMLInputElement>();
  let licenceCard = $state<HTMLElement>();
  let licenceShaking = $state(false);
  let removingLicence = $state(false);

  // The words under the head: a refusal, what a link did, the key kept,
  // or that there is none.
  const licenceLine = $derived.by(() => {
    if (copyRefused) return copyRefused + ".";
    if (licenceRefused) return licenceRefused + (licence.saved ? `. ${licence.about} on this Mac still unlocks it.` : ".");
    if (fromLink?.what === "unlocked") return `Unlocked from the link. Thank you. ${fromLink.about}.`;
    if (fromLink?.what === "same") return `This key is already on this Mac. ${fromLink.about}.`;
    if (fromLink?.what === "replaced") return `${fromLink.about} from the link, in place of ${small(fromLink.before ?? "")}.`;
    if (licence.saved) return licence.about + ".";
    return "None yet.";
  });

  // The line in parts, with every key ID in it picked out. The kept key's
  // is the one a person may copy and the one that shines: it is what was
  // bought. One it replaced is set the same way, without either.
  const keyIDs = /([0-9A-F]{4}(?:-[0-9A-F]{4}){3})/;
  const keptID = $derived(licence.saved ? (licence.about.match(keyIDs)?.[1] ?? "") : "");
  const licenceParts = $derived(
    licenceLine
      .split(keyIDs)
      .filter((text) => text !== "")
      .map((text) => ({ text, id: keyIDs.test(text), kept: text === keptID })),
  );

  // Copy says it copied in the same frame as the click, and goes back to
  // what it was a moment later. The chip keeps its width throughout.
  let copied = $state(false);
  let copyRefused = $state("");
  let copiedTimer: ReturnType<typeof setTimeout> | undefined;
  async function copyLicence() {
    copied = true;
    copyRefused = "";
    clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => (copied = false), 1500);
    try {
      await api.copyLicence();
    } catch (err) {
      copied = false;
      copyRefused = sentence(errorText(err));
    }
  }

  async function readLicence() {
    try {
      licence = await api.licence();
    } catch (err) {
      problem = errorText(err);
    }
  }

  function sentence(said: string): string {
    return said.charAt(0).toUpperCase() + said.slice(1);
  }

  function small(said: string): string {
    return said.charAt(0).toLowerCase() + said.slice(1);
  }

  // What a link did, with the row in view.
  async function takeLink() {
    let l: LicenceLink | null = null;
    try {
      l = await api.takeLicenceLink();
    } catch (err) {
      problem = errorText(err);
    }
    if (!l?.what) return;
    licenceRefused = l.what === "refused" ? sentence(l.reason) : "";
    licenceKey = "";
    fromLink = l.what === "refused" ? null : l;
    await readLicence();
    licenceCard?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }

  // Refused the way the API key is: the field shakes and keeps the key,
  // selected, so the next paste replaces it.
  async function unlock() {
    unlocking = true;
    licenceRefused = "";
    try {
      licence = await api.saveLicence(licenceKey);
      licenceKey = "";
      fromLink = null;
    } catch (err) {
      licenceRefused = sentence(errorText(err));
      licenceShaking = false;
      requestAnimationFrame(() => {
        licenceShaking = true;
        licenceField?.focus();
        licenceField?.select();
      });
    }
    unlocking = false;
  }

  // The key is in the mail it came in, so removing it here can be undone
  // by unlocking again, but only with that mail at hand. So it asks first.
  async function removeLicence() {
    removingLicence = false;
    licenceRefused = "";
    fromLink = null;
    try {
      licence = await api.saveLicence("");
    } catch (err) {
      licenceRefused = sentence(errorText(err));
    }
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
      keyHints = state.keyHints ?? {};
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
  // What a search or a clip made by hand is using, see refuseFinding.
  const running = (j: Job) => j.state === "running" || j.state === "queued";
  const searching = $derived(jobs.list.some((j) => j.kind === "search" && running(j)));
  const hearing = $derived(searching || jobs.list.some((j) => j.kind === "clip" && running(j)));
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
      searching,
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
  // holds it too, because the model is no use until the last of it has
  // arrived. Only a page still reading what it has to show does not.
  let findingCard = $state<HTMLElement>();
  let shaking = $state(false);
  let speechShaking = $state(false);
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

  // A search hears the episode with the speech model and then finds clips
  // with the language model, the one chosen when it began, and a clip made
  // by hand hears with the speech model too. What they use stays as it is
  // until they are done: choosing another model, removing one or the key
  // it reads is refused, at once, from the hearing on, and the card it
  // belongs to shakes, the way it does when the settings may not be left.
  // The Go side refuses the same, see lockedBy in setup.go.
  const findingLocked = "A search is using it. It can be changed once the search is done";
  const speechLocked = "An episode is being transcribed with it. It can be removed once that is done";
  function shakeFinding() {
    shaking = false;
    requestAnimationFrame(() => (shaking = true));
  }
  function shakeSpeech() {
    speechShaking = false;
    requestAnimationFrame(() => (speechShaking = true));
  }
  // For the list of what finds clips, which asks before it opens.
  function refuseFinding(): boolean {
    if (searching) shakeFinding();
    return searching;
  }

  onMount(() => {
    load();
    nav.hold = hold;
    const noLink = onLicenceLink(takeLink);
    return () => {
      noLink();
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
    await Promise.all([readTraining(), readModels(), check(), readLicence()]);
    loaded = true;
    // After the page is drawn, so the row is there to come into view.
    await takeLink();
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
              {#if finderState === "busy" || finderLine.using}
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
              refuse={refuseFinding}
              title={searching ? findingLocked : "What reads the transcript and picks the moments"}
            />
          </div>

          {#if settings.planner === "api"}
            <!-- The key goes in the keychain the moment it is saved, not
                 with the rest of these, because it never lands in the
                 settings file. An app opened from Finder has no shell
                 environment, so this is the only way to give it one. -->
            <div class="item">
              <span class="mark" class:ok={hasKey && !keyRefused} class:err={!!keyRefused} class:warn={!hasKey && !keyRefused} aria-hidden="true">
                {#if hasKey && !keyRefused}<Icon name="check" />{:else}<Icon name="warn" />{/if}
              </span>
              <div class="words">
                <span class="head">{provider.title} API key</span>
                <!-- A saved key is shown in short, the way the companies
                     list keys, and where it comes from is in the title. -->
                <span
                  class="small line"
                  class:muted={hasKey && !keyRefused}
                  class:hint={hasKey && !keyRefused && !savedKey && !!keyHints[provider.name]}
                  class:warn={!hasKey && !keyRefused}
                  class:error={!!keyRefused}
                  title={keyRefused ||
                    (keys[provider.name] === "environment"
                      ? `From ${provider.env}`
                      : hasKey
                        ? "In the keychain, where only this app may read it"
                        : undefined)}
                >
                  {#if keyRefused}{keyRefused}{:else if savedKey}Saved in the keychain.{:else if keyHints[
                      provider.name
                    ]}{keyHints[provider.name]}{:else if keys[provider.name] === "environment"}From {provider.env}.{:else if hasKey}In the keychain.{:else}None
                    yet. Get one at <button
                      class="link"
                      title="Opens the page where {provider.title} makes keys"
                      onclick={() => api.openKeysPage(provider.name).catch(() => {})}
                      >{provider.keysAt}</button
                    >.{/if}
                </span>
              </div>
              <input
                class="key"
                class:shaking={keyShaking}
                onanimationend={() => (keyShaking = false)}
                type="password"
                bind:this={keyField}
                bind:value={key}
                oninput={() => (keyRefused = "")}
                placeholder={hasKey ? "New key" : `${provider.name === "openai" ? "sk-proj" : "sk-ant"}-...`}
                aria-label="{provider.title} API key"
                title="It goes in the keychain and nowhere else"
                autocomplete="off"
                spellcheck="false"
                onkeydown={(e) => e.key === "Enter" && key.trim() && !keyRefused && saveKey()}
              />
              <!-- A key that was refused is refused again, so Save waits
                   until the field holds something else. -->
              <button class="act" disabled={savingKey || !key.trim() || !!keyRefused} onclick={saveKey}>
                {#if savingKey}<Busy />{/if}
                {savingKey ? "Checking" : "Save key"}
              </button>
              <!-- Only a key the app keeps can be removed here. One in the
                   environment is the terminal's to take away. The trash can
                   keeps its place without one, so the field and the button
                   do not move when a key is saved or removed. -->
              {#if keys[provider.name] === "keychain"}
                <button
                  class="quiet danger bin"
                  title="Remove the key from this machine"
                  aria-label="Remove the {provider.title} API key"
                  aria-haspopup="dialog"
                  disabled={savingKey}
                  onclick={() => refuseFinding() || (removingKey = true)}
                >
                  <Icon name="trash" />
                </button>
              {:else}
                <span class="bin-room" aria-hidden="true"></span>
              {/if}
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
                  locked={searching ? findingLocked : ""}
                  onlocked={shakeFinding}
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
        <div class="speech" class:shaking={speechShaking} onanimationend={() => (speechShaking = false)}>
          <ModelList
            models={speechRows}
            kind="model"
            oninstall={api.installSpeechModel}
            onchange={modelsChanged}
            onremove={api.removeSpeechModel}
            removeSays="Every episode is transcribed with it, so the app asks for one again the next time it starts."
            locked={hearing ? speechLocked : ""}
            onlocked={shakeSpeech}
          />
        </div>
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
              <!-- Any other colour, in the app's own colour picker. The
                   round colours before it are its presets, so it offers
                   none of its own. -->
              <Colour
                face="well"
                on={custom}
                label="Another colour"
                title="Another colour"
                value={settings.appColour ?? ""}
                oninput={(hex) => hex && settings && (settings.appColour = hex)}
                onchange={(hex) => settings && (settings.appColour = hex)}
              />
            </div>
          </div>
        </div>
      </div>

      <!-- The licence, kept in the keychain like an API key. A key comes
           in the mail it was bought with, and the Unlock button in that
           mail unlocks the app and opens this row to say so. -->
      <div class="group">
        <div class="headrow">
          <h2>Licence</h2>
          <span class="ask">
            <Info label="About the licence" side="left">
              The key comes in the mail you bought Frame Fairy with. Unlock in that mail unlocks
              the app, or paste the key into the field here. It is checked on this machine, without
              asking anybody, and kept in the keychain. The key ID names it, and it is what
              support asks for.
            </Info>
          </span>
        </div>
        <div class="card" bind:this={licenceCard}>
          <div class="item">
            <span class="mark" class:ok={licence.saved && !licenceRefused} class:err={!!licenceRefused} aria-hidden="true">
              {#if licenceRefused}<Icon name="warn" />{:else if licence.saved}<Icon name="check" />{/if}
            </span>
            <div class="words">
              <span class="head">{licence.saved ? "Licensed" : "Licence key"}</span>
              <span
                class="small line whole licence-line"
                class:muted={!licenceRefused && !copyRefused}
                class:error={!!licenceRefused || !!copyRefused}
              >
                {#each licenceParts as part, i (i)}
                  {#if part.kept}<button
                      class="key-id kept"
                      class:copied
                      title="Copy the licence key, for a password manager or another Mac"
                      aria-label={copied ? "Copied" : `Copy the licence key, key ID ${part.text}`}
                      onclick={copyLicence}
                      ><span>{part.text}</span><Icon name={copied ? "check" : "copy"} size={12} /></button
                    >{:else if part.id}<span class="key-id">{part.text}</span>{:else}{part.text}{/if}
                {/each}
              </span>
            </div>
            {#if !licence.saved}
            <input
              class="key licence-key"
              class:shaking={licenceShaking}
              onanimationend={() => (licenceShaking = false)}
              type="text"
              bind:this={licenceField}
              bind:value={licenceKey}
              oninput={() => {
                licenceRefused = "";
                fromLink = null;
              }}
              placeholder="FF1-..."
              aria-label="Licence key"
              title="It goes in the keychain and nowhere else"
              autocomplete="off"
              spellcheck="false"
              onkeydown={(e) => e.key === "Enter" && licenceKey.trim() && !licenceRefused && unlock()}
            />
            <button
              class="act unlock"
              disabled={unlocking || !licenceKey.trim() || !!licenceRefused}
              onclick={unlock}
            >
              {#if unlocking}<Busy />{/if}
              {unlocking ? "Checking" : "Unlock"}
            </button>
            {/if}
            {#if licence.saved}
              <button
                class="quiet danger bin"
                title="Remove the licence key from this machine"
                aria-label="Remove the licence key"
                aria-haspopup="dialog"
                disabled={unlocking}
                onclick={() => (removingLicence = true)}
              >
                <Icon name="trash" />
              </button>
            {:else}
              <span class="bin-room" aria-hidden="true"></span>
            {/if}
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
                  <span>{c.name}{#if c.version}{" "}<span class="muted num">{c.version}</span>{/if}</span>
                  <span class="small selectable" class:muted={c.ok} class:error={!c.ok}>{c.detail}</span>
                  <!-- Which file, and what it is down to the byte, so it
                       can be compared with the build it came from. -->
                  {#if c.path}<span class="small muted selectable">{c.path}</span>{/if}
                  {#if c.sha256}<span class="small muted selectable sum" title="SHA-256">SHA-256 {c.sha256}</span>{/if}
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

  {#if removingLicence}
    <Confirm title="Remove the licence key?" oncancel={() => (removingLicence = false)}>
      <p>
        The key leaves this machine's keychain. It stays valid, and Unlock in the mail it came in
        brings it back.
      </p>
      {#snippet actions()}
        <button onclick={() => (removingLicence = false)}>Cancel</button>
        <button class="danger" onclick={removeLicence}>Remove</button>
      {/snippet}
    </Confirm>
  {/if}

  {#if removingKey}
    <Confirm title="Remove the {provider.title} API key?" oncancel={() => (removingKey = false)}>
      <p>
        The key leaves this machine's keychain, and clips cannot be found in the cloud until a key
        is saved again. The key itself stays valid at {provider.keysAt}.
      </p>
      {#snippet actions()}
        <button onclick={() => (removingKey = false)}>Cancel</button>
        <button class="danger" onclick={removeKey}>Remove</button>
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

  /* Narrow, since what is typed shows as dots, and the room goes to the
     key in short beside it. */
  .key {
    width: 150px;
    flex: none;
  }

  /* The room of the trash can, where there is nothing to remove. */
  .bin-room {
    width: var(--control-h);
    flex: none;
  }

  /* A key in short reads as a key: the figures and letters at one width. */
  .hint {
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
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
  .finding.shaking,
  .speech.shaking,
  .key.shaking {
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
    .finding.shaking,
    .speech.shaking,
    .key.shaking {
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

  /* The licence row's line is read whole, and wraps. It says why a key
     was refused or what a link did, and cut short it read as a key that
     would not unlock for no reason: "Only a build made from the code on
     this machine takes ...". The card grows by a line rather than hiding
     the reason. */
  .line.whole {
    white-space: normal;
  }

  /* The licence row's line holds key IDs set like code in a README, so
     it is given a whole-pixel line of its own, with room for them. */
  .licence-line {
    line-height: 22px;
  }

  /* A key ID, in one width, on a block of its own, the way code is set in
     a README. The kept key's is a button that copies the key itself, and a
     light passes over it now and then: it is what was bought. That light
     is not one of the five ways work in hand is shown, because nothing is
     running. It passes once, rests for seconds and never pulses, so it is
     not read as anything waiting. */
  .key-id {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 20px;
    margin: 1px 1px 0;
    padding: 0 6px;
    vertical-align: top;
    border: 1px solid var(--line);
    border-radius: 5px;
    background: var(--ink-2);
    color: var(--text);
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
    font-size: var(--size-s);
    line-height: 18px;
  }

  button.key-id {
    position: relative;
    overflow: hidden;
    cursor: pointer;
    border-color: color-mix(in srgb, var(--accent) 55%, var(--line));
  }

  button.key-id:hover {
    background: var(--ink-3);
  }

  button.key-id :global(svg) {
    color: var(--muted);
  }

  button.key-id.copied :global(svg) {
    color: var(--ok);
  }

  button.key-id::after {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(100deg, transparent 30%, color-mix(in srgb, var(--accent-lit) 35%, transparent) 50%, transparent 70%);
    transform: translateX(-100%);
    animation: glint 7s ease-in-out 0.6s infinite;
    pointer-events: none;
  }

  @keyframes glint {
    0% {
      transform: translateX(-100%);
    }
    20%,
    100% {
      transform: translateX(100%);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    button.key-id::after {
      animation: none;
      display: none;
    }
  }

  /* A licence key is shown as it is, not as dots: it is read off a mail,
     not typed from memory, and a person checks it is the one they meant.
     At one width, the way a key in short is. */
  .licence-key {
    width: 220px;
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
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

  /* A checksum is read a character at a time, in figures of one width,
     and broken anywhere rather than pushing the card wider. */
  .sum {
    font-variant-numeric: tabular-nums;
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
    overflow-wrap: anywhere;
  }
</style>
