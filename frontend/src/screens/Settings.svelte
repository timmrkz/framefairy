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
  import { shownFolder, shownPath } from "../lib/paths";
  import {
    cloudModelIn,
    cloudValue,
    finderOptions as finderOptions_,
    finderStanding,
    holdsTheApp,
    modelMark,
    type Standing,
    type Tone,
    providerOf,
  } from "../lib/finding";
  import {
    api,
    clock,
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
    type LogStatus,
    type LicenceLink,
    type LicenceState,
    onLicenceLink,
  } from "../lib/api";
  import ApiKey from "../components/ApiKey.svelte";
  import Confirm from "../components/Confirm.svelte";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import ModelCard from "../components/ModelCard.svelte";
  import { Install } from "../lib/install.svelte";
  import Swatches from "../components/Swatches.svelte";

  let settings = $state<Settings | null>(null);
  let checks = $state<Check[] | null>(null);
  let checking = $state(false);
  let problem = $state("");
  let advanced = $state(false);
  // Where the training records are and how many there are, which is what
  // makes the trash can beside them mean anything.
  let training = $state<TrainingStatus>({ dir: "", plans: 0, decisions: 0 });
  let clearing = $state(false);
  // The app's log, under Advanced: read again whenever Advanced opens, so
  // it says what the log holds now and not what it held at the start.
  let appLog = $state<LogStatus>({ path: "", files: 0, bytes: 0, lines: 0 });
  let removingLog = $state(false);
  const logFolder = $derived(appLog.path.replace(/[\\/][^\\/]*$/, ""));
  // The models that can be installed, and what the machine can hold. The
  // same lists the setup shows, so one can be added or swapped later
  // without going through the setup again.
  let speech = $state<SpeechModel[]>([]);
  let language = $state<LanguageModel[]>([]);
  // What this machine has, in bytes, or zero where it would not say.
  let memory = $state(0);
  // The home folder, so every path on the page is written the one way.
  let home = $state("");
  let modelsDir = $state("");
  const shown = (path: string) => shownPath(path, home);
  // Where the models are kept, said in the info mark of each model's
  // section and nowhere else, with the slash at its end that says it is a
  // folder. Tim had it on a line under every model and found that ugly,
  // 2026-10-28.
  const modelsFolder = $derived(shownFolder(modelsDir, home));



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
  // The key in short, as the Go side read it without the secret.
  let keyHints = $state<Record<string, string>>({});

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
    if (licenceRefused) return licenceRefused + (licence.saved ? `. ${licence.about} on this Mac still unlocks it.` : ".");
    if (fromLink?.what === "unlocked") return `Unlocked from the link. Thank you. ${fromLink.about}.`;
    if (fromLink?.what === "same") return `This key is already on this Mac. ${fromLink.about}.`;
    if (fromLink?.what === "replaced") return `${fromLink.about} from the link, in place of ${small(fromLink.before ?? "")}.`;
    if (licence.saved) return licence.about + ".";
    return "None yet.";
  });

  // The line in parts, with every key ID in it picked out. The kept key's
  // is the one a person may copy and the one that wears the beam: it is
  // what was bought. One it replaced is set the same way, without either.
  // A key ID is a thing shown rather than a word in a sentence, so the
  // full stop after one that ends the line goes.
  const keyIDs = /([0-9A-F]{4}(?:-[0-9A-F]{4}){3})/;
  const keptID = $derived(licence.saved ? (licence.about.match(keyIDs)?.[1] ?? "") : "");
  const licenceParts = $derived.by(() => {
    const parts = licenceLine
      .split(keyIDs)
      .filter((text) => text !== "")
      .map((text) => ({ text, id: keyIDs.test(text), kept: text === keptID }));
    if (parts.length > 1 && parts[parts.length - 1].text === "." && parts[parts.length - 2].id) parts.pop();
    return parts;
  });

  // Copy says it copied in the same frame as the click, and goes back to
  // what it was a moment later. The chip keeps its width throughout.
  let copied = $state(false);
  let copyRefused = $state("");
  let copiedTimer: ReturnType<typeof setTimeout> | undefined;
  // The beam round the key ID rests until the hand comes to it: each
  // time the pointer reaches it, and each copy, it comes round once.
  // Lit all the time it read as a line standing still, 2026-10-27.
  let beams = $state(0);
  async function copyKeyID(id: string) {
    beams++;
    copied = true;
    copyRefused = "";
    clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => (copied = false), 1500);
    try {
      await api.copy(id);
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
      home = state.home ?? "";
      modelsDir = state.modelsDir ?? "";
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

  async function readLog() {
    try {
      appLog = await api.appLog();
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function removeLog() {
    removingLog = false;
    try {
      appLog = await api.removeAppLog();
    } catch (err) {
      problem = errorText(err);
      await readLog();
    }
  }

  async function showLog() {
    try {
      await api.showAppLog();
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
      const dir = await api.chooseFolder("Where clips are saved");
      if (dir && settings) settings.outputDir = dir;
    } catch (err) {
      problem = errorText(err);
    }
  }

  // What the app found, under Advanced: the programs it carries and what
  // it can use. The models and the keys are said in their own sections and
  // only there, so a model is not spoken of in six places, 2026-10-27.
  const saidAbove = (name: string) => ["Speech model", "Language model"].includes(name) || name.endsWith(" API key");
  // One that failed stays, so what is wrong is said here as well as in
  // its own section's line.
  const found = $derived((checks ?? []).filter((c) => !(saidAbove(c.name) && c.ok)));

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
  // What a search, a clip made by hand or the whole video being
  // transcribed is using, see refuseFinding and speechLocked.
  const running = (j: Job) => j.state === "running" || j.state === "queued";
  const searching = $derived(jobs.list.some((j) => j.kind === "search" && running(j)));
  const hearing = $derived(
    searching || jobs.list.some((j) => (j.kind === "clip" || j.kind === "transcribe") && running(j)),
  );
  // The clip model's download, and the speech model's below: one Install
  // each, see install.svelte.ts. When one ends the models are read again:
  // the model it fetched is already the one chosen, so being there is all
  // that changes. One that stopped says why, in the line under the choice.
  const llmInstall = new Install("llm", () => modelsChanged());
  // Whatever is being downloaded, even one started before this page
  // opened, is this page's to show.
  const fetchingModel = $derived(
    llmInstall.running
      ? language.find((m) => m.title === llmInstall.running?.label)
      : llmInstall.starting
        ? inUse
        : undefined,
  );

  const finder = $derived(
    settings?.planner === "api" ? cloudValue(cloudModel) : choosing || (inUse?.name ?? ""),
  );

  const finderOptions = $derived(finderOptions_(cloud, providers, language, cloudModel));

  async function pickFinder(value: string) {
    if (!settings) return;
    llmInstall.failed = "";
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

  // A download of gigabytes starts when Download is pressed, never as a
  // side effect of looking through a list.
  function download() {
    const m = inUse;
    if (m && !m.installed) llmInstall.start(() => api.installLanguageModel(m.name));
  }

  // The speech model, in the shape of the clip model: one list of the
  // speech models, each with its size in the group that says whether it
  // is here, Download beside it when the one in use is not here, and the
  // models on this machine folded under it. There is one speech model
  // today, so the list holds one, and Tim asked for the two to look and
  // work alike all the same, 2026-10-27.
  const speechInUse = $derived(speech.find((m) => m.installed) ?? speech[0]);
  // The biggest first, by the size beside it, as the clip model's list.
  const speechOptions = $derived(
    speech
      .map((m) => ({ m, bytes: m.installed ? m.unpacked : m.download }))
      .sort((a, b) => b.bytes - a.bytes)
      .map(({ m, bytes }) => ({
        value: m.name,
        label: m.title,
        detail: size(bytes),
        group: "On this machine",
        mark: modelMark(m.installed),
      })),
  );
  const speechInstall = new Install("model", () => modelsChanged());
  function downloadSpeech() {
    const m = speechInUse;
    if (m && !m.installed) speechInstall.start(() => api.installSpeechModel(m.name));
  }
  // Choosing the speech model in the list is choosing the one the app
  // installs, so a folder named by hand in the settings before, which
  // there is no field for any more, gives way to it.
  function pickSpeech() {
    if (settings) settings.asrModel = "";
  }
  // The one line under Transcribe with, one line in every state, like the
  // line under Find clips with. A folder named by hand is said, and so is
  // whatever the check found wrong with the model, where it is put right.
  const speechLine = $derived.by((): { text: string; tone: Tone; state: Standing } => {
    const m = speechInUse;
    if (!m) return { text: "No speech model is offered.", tone: "err", state: "err" };
    const job = speechInstall.running;
    if (job || speechInstall.starting) {
      const p = job?.progress;
      const done = p && p.fraction > 0 ? `${size(p.fraction * m.download)} of ${size(m.download)}` : size(m.download);
      const left = p && p.remaining > 0 ? `, ${clock(p.remaining)} left` : "";
      return { text: `Downloading, ${done}${left}`, tone: "", state: "busy" };
    }
    if (speechInstall.failed) return { text: speechInstall.failed, tone: "err", state: "err" };
    const wrong = broken("Speech model")?.detail.split("\n")[0];
    if (wrong) return { text: wrong, tone: "err", state: "err" };
    if (settings?.asrModel) return { text: `From ${shown(settings.asrModel)}.`, tone: "", state: "ok" };
    if (!m.installed) return { text: `Not downloaded yet. ${size(m.download)}.`, tone: "warn", state: "warn" };
    return { text: `By ${m.maker}. ${m.languages}.`, tone: "", state: "ok" };
  });
  const installedSpeech = $derived(speech.filter((m) => m.installed).map((m) => speechRow(m)));
  const installedSpeechSize = $derived(
    size(speech.filter((m) => m.installed).reduce((sum, m) => sum + m.unpacked, 0)),
  );

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
      progress: llmInstall.running?.progress,
      failed: llmInstall.failed,
      modelProblem: broken("Language model")?.detail,
      serverMissing: !!broken("llama-server"),
      searching,
    }),
  );
  const finderLine = $derived(finding);
  const finderState = $derived(finding.state);

  // The models on this machine, folded under the choice.
  const installedRows = $derived(
    language
      .filter((m) => m.installed)
      .map((m) => ({
        ...languageRow(m),
        // Here, what it needs is said by the one it is, and which one it
        // is matters more.
        memory: m.inUse ? "Finds the clips now" : languageRow(m).memory,
        note: "",
        warn: false,
      })),
  );
  const installedSize = $derived(
    size(language.filter((m) => m.installed).reduce((sum, m) => sum + m.download, 0)),
  );

  const plural = (n: number, one: string, many: string) => `${n.toLocaleString("en")} ${n === 1 ? one : many}`;

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
  const speechLocked = "A video is being transcribed with it. It can be removed once that is done";
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
          <h2>Clip model</h2>
          <span class="ask">
            <Info label="About the clip model" side="left">
              A language model reads the transcript and picks the moments worth clipping. A model in
              the cloud, Anthropic's or OpenAI's, works on any machine with a key of your own from
              that company, and costs a few cents a video. A model on this machine is free to run
              and needs the memory to hold it. Either way the video and the audio stay
              here: only the words are read.
              {#if settings.planner === "local"}
                <br /><br />
                A model runs from memory, and this machine has
                {memory > 0 ? memorySize(memory) : "not said how much"}. A bigger model reads the
                transcript better and needs more of it, and the list says beside a model when this
                machine is short of memory for it. A model is downloaded once from its source onto
                your machine, into <b>{modelsFolder}</b>.
              {/if}
            </Info>
          </span>
        </div>
        <ModelCard
          name="finding"
          bind:card={findingCard}
          {shaking}
          onshaken={() => (shaking = false)}
          head="Find clips with"
          line={finderLine}
          install={llmInstall}
          download={notHere && settings.planner === "local"
            ? { title: `Fetch ${inUse?.title}, ${size(inUse?.download ?? 0)}. It finds clips once it is here`, onclick: download }
            : undefined}
          oncancel={() => llmInstall.cancel((why) => (problem = why))}
          pick={{
            value: finder || "Choose a model",
            options: finderOptions,
            onpick: pickFinder,
            label: "Find clips with",
            title: searching ? findingLocked : "What reads the transcript and picks the moments",
            refuse: refuseFinding,
          }}
          rows={settings.planner === "local" ? installedRows : []}
          room={installedSize}
          list={{
            kind: "llm",
            oninstall: api.installLanguageModel,
            onchange: modelsChanged,
            onremove: api.removeLanguageModel,
            locked: searching ? findingLocked : "",
            onlocked: shakeFinding,
          }}
        >
          {#if settings.planner === "api"}
            <ApiKey
              {provider}
              source={keys[provider.name] ?? ""}
              hint={keyHints[provider.name] ?? ""}
              refuse={refuseFinding}
              onchange={() => Promise.all([readModels(), check()])}
            />
          {:else if broken("llama-server")}
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
        </ModelCard>
      </div>

      <!-- Every episode is transcribed on this machine, so the model has to
           be on it. The card is the clip model's, row for row: the choice
           with the line under it, and the models on this machine folded
           under that. Installing one here is the setup's job. -->
      <div class="group asks">
        <div class="headrow">
          <h2>Speech model</h2>
          <span class="ask">
            <Info label="About the speech model" side="left">
              Every video is transcribed on this machine, word by word with the time of each
              word. Nothing about the video is sent anywhere. The model is downloaded once from its
              source onto your machine, into <b>{modelsFolder}</b>.
            </Info>
          </span>
        </div>
        <ModelCard
          name="speech"
          shaking={speechShaking}
          onshaken={() => (speechShaking = false)}
          head="Transcribe with"
          line={speechLine}
          install={speechInstall}
          download={speechInUse && !speechInUse.installed
            ? { title: `Fetch ${speechInUse.title}, ${size(speechInUse.download)}. It transcribes once it is here`, onclick: downloadSpeech }
            : undefined}
          oncancel={() => speechInstall.cancel((why) => (problem = why))}
          pick={{
            value: speechInUse?.name ?? "",
            options: speechOptions,
            onpick: pickSpeech,
            label: "Transcribe with",
            title: "What hears the video and writes down every word",
          }}
          rows={installedSpeech}
          room={installedSpeechSize}
          list={{
            kind: "model",
            oninstall: api.installSpeechModel,
            onchange: modelsChanged,
            onremove: api.removeSpeechModel,
            removeSays: "Every video is transcribed with it, so the app asks for one again the next time it starts.",
            locked: hearing ? speechLocked : "",
            onlocked: shakeSpeech,
          }}
        />
      </div>

      <div class="group">
        <h2>Clips</h2>
        <div class="card">
          <div class="item">
            <span class="mark" aria-hidden="true"><Icon name="folder" /></span>
            <div class="words">
              <span class="head">Save clips to</span>
              <span class="small muted selectable path">
                {settings.outputDir ? shown(settings.outputDir) : "Next to each video"}
              </span>
            </div>
            {#if settings.outputDir}
              <button
                class="quiet"
                title="Save each clip next to its video again"
                onclick={() => settings && (settings.outputDir = "")}>Next to video</button
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
              <span class="small muted">What the app picks things out in. The colours of a clip are set beside its captions.</span>
            </div>
            <Swatches
              label="Accent colour"
              value={settings.appColour ?? ""}
              oninput={(hex) => settings && (settings.appColour = hex)}
              onpick={(hex) => settings && (settings.appColour = hex)}
            />
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
              <span class="head licence-head">{licence.saved ? "Licensed" : "Licence key"}</span>
              <span
                class="small line whole licence-line"
                class:muted={!licenceRefused}
                class:error={!!licenceRefused}
              >
                {#each licenceParts as part, i (i)}
                  {#if part.kept}<button
                      class="key-id kept"
                      class:copied
                      title="Copy the key ID, which is what support asks for"
                      aria-label={copied ? "Copied" : `Copy key ID ${part.text}`}
                      onclick={() => copyKeyID(part.text)}
                      onpointerenter={() => beams++}
                      >{#key beams}{#if beams > 0}<Busy motes={false} seldom once />{/if}{/key}<span>{part.text}</span><Icon
                        name={copied ? "check" : "copy"}
                        size={14}
                      /></button
                    >{:else if part.id}<span class="key-id">{part.text}</span>{:else}{part.text}{/if}
                {/each}
              </span>
              <!-- Why the key ID could not be copied, under it, where it
                   stays to be clicked again. -->
              {#if copyRefused}<span class="small line whole error licence-line">{copyRefused}.</span>{/if}
            </div>
            {#if !licence.saved}
            <input
              class="key licence-key shakes"
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
          onclick={() => {
            advanced = !advanced;
            if (advanced) readLog();
          }}
        >
          <span class="chevron" class:open={advanced}><Icon name="chevron" size={14} /></span>
          <h2>Advanced</h2>
        </button>
        {#if advanced}
          <div class="card">
            <!-- The models are chosen in their own sections, and said
                 nowhere else. A model in the cloud the list does not name
                 yet, a newer one, is the one thing named here by hand. -->
            {#if settings.planner === "api"}
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
                <span class="head">Training data folder</span>
                <span class="small muted">Where the records go.</span>
              </span>
              <input type="text" bind:value={settings.trainingDir} placeholder={shown("~/.framefairy/training")} spellcheck="false" />
            </label>
          </div>

          <!-- The app's own log, the file a report takes along. It is the
               app's and nobody else's, so it can go whole, after asking. -->
          <div class="card">
            <div class="item">
              <div class="words">
                <span class="head">The app's log</span>
                {#if appLog.path}
                  <span class="small muted num">
                    {plural(appLog.files, "file", "files")}, {size(appLog.bytes) || "empty"}, {plural(appLog.lines, "line", "lines")}
                  </span>
                  <span class="small muted selectable path">{shown(appLog.path)}</span>
                {:else}
                  <span class="small muted">Nothing written yet.</span>
                {/if}
              </div>
              <button
                class="quiet bin"
                disabled={!appLog.path}
                title="Show in folder"
                aria-label="Show the app's log in folder"
                onclick={showLog}
              >
                <Icon name="folder" />
              </button>
              <button
                class="quiet danger bin"
                disabled={!appLog.path}
                title="Remove the log, the older files too"
                aria-label="Remove the app's log"
                aria-haspopup="dialog"
                onclick={() => (removingLog = true)}
              >
                <Icon name="trash" />
              </button>
            </div>
          </div>

          <!-- Everything the check looked at, found or not, for whoever
               needs to know which ffmpeg or which model it found. -->
          <div class="card">
            <div class="item">
              <div class="words">
                <span class="head">What the app found</span>
                <span class="small muted">Looked for again whenever a setting changes.</span>
              </div>
              <button
                class="act"
                onclick={check}
                disabled={checking}
                title="Look for the tools and models again"
                >{#if checking}<Busy />{/if}{checking ? "Checking" : "Check again"}</button
              >
            </div>
            {#each found as c (c.name)}
              <div class="item found">
                <span class="mark" class:ok={c.ok} aria-hidden="true">
                  {#if c.ok}<Icon name="check" />{:else}<span class="dot err"></span>{/if}
                </span>
                <div class="words">
                  <span>{c.name}{#if c.version}{" "}<span class="muted num">{c.version}</span>{/if}</span>
                  <span class="small selectable" class:muted={c.ok} class:error={!c.ok}>{c.detail}</span>
                  <!-- Which file, and what it is down to the byte, so it
                       can be compared with the build it came from. -->
                  {#if c.path}<span class="small muted selectable">{shown(c.path)}</span>{/if}
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
        Every plan and every decision recorded so far, in <b>{shown(training.dir)}</b>, goes. The clips
        themselves stay. This cannot be taken back.
      </p>
      {#snippet actions()}
        <button onclick={() => (clearing = false)}>Cancel</button>
        <button class="danger" onclick={clearTraining}>Remove them</button>
      {/snippet}
    </Confirm>
  {/if}

  {#if removingLog}
    <Confirm title="Remove the app's log?" oncancel={() => (removingLog = false)}>
      <p>
        Every file of the log in <b>{logFolder}</b> goes, the older ones too, and nothing else in that
        folder. The app starts a new log with its next line. This cannot be taken back.
      </p>
      {#snippet actions()}
        <button onclick={() => (removingLog = false)}>Cancel</button>
        <button class="danger" onclick={removeLog}>Remove them</button>
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

  .act {
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
    line-height: 28px;
  }

  /* The field, Unlock and the bin are 32 px and centred beside the
     words, so the words must be an even height or the controls stand
     between two pixels and step as they fade, issue #339. Lines of 28 px
     and the gaps of 2 between them are even, and the name's 19 px line is
     not, so the name's lines are 20. A line each, not a pixel under the
     name, because a narrow app wraps the name too. */
  .licence-head {
    line-height: 20px;
  }

  /* A key ID, in one width, on a block of its own, the way code is set in
     a README, in the quieter grey of a line under a name rather than in
     white. The kept key's is a button that copies the key ID, and it
     wears the beam from Busy, the same beam, coming by once in a while
     and turned down: nothing is running, it only says this is what was
     bought. */
  .key-id {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    height: 26px;
    margin: 1px 1px 0;
    padding: 0 9px;
    vertical-align: top;
    /* The edge is drawn inside, the way the clip cards draw theirs, so
       the beam runs on the edge itself. A border lies outside what the
       block clips its beam to, and the beam ran a pixel inside it. */
    border: none;
    box-shadow: inset 0 0 0 1px var(--line);
    border-radius: 6px;
    /* Darker than the card it sits in, the way a key in a README sits in
       a block of code. */
    background: var(--ink-0);
    color: var(--muted);
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
    font-size: var(--size-m);
    line-height: 26px;
  }

  button.key-id {
    cursor: pointer;
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 45%, var(--line));
  }

  button.key-id:hover {
    color: var(--text);
  }

  button.key-id.copied > :global(svg) {
    color: var(--ok);
  }

  /* A licence key is shown as it is, not as dots: it is read off a mail,
     not typed from memory, and a person checks it is the one they meant.
     At one width, the way a key in short is. */
  .licence-key {
    width: 220px;
    flex: none;
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
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
