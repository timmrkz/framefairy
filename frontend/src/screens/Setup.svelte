<script lang="ts">
  // The first run. A new copy of the app on a machine with nothing on it
  // needs two models before it can make anything: one that hears the
  // video and one that picks the clips. Both run on this machine, so both
  // are downloads, and the setup is one page with a card for each.
  //
  // Each card has its own Download, and both can run at once: a person
  // who starts the speech model does not wait for it to finish before
  // starting the clip model. Nothing starts by itself, because a download
  // of gigabytes is something somebody agrees to. Once both are here the
  // setup is over and the app opens on its first page, with nothing to
  // press in between. What else a first run may one day need is another
  // card here.
  //
  // A model in the cloud is not offered here. It needs a key and costs
  // money a video, so it is a choice made later in the settings, by
  // somebody who knows they want it.
  //
  // Nothing here is a box over the workspace. Until both models are here
  // there is no workspace to put a box over, so this is the whole app.
  import { onMount } from "svelte";
  import { api, errorText, languageRow, memorySize, speechRow, type ModelRow, type SetupState } from "../lib/api";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import ModelList from "../components/ModelList.svelte";

  let { ondone }: { ondone: () => void } = $props();

  let setup = $state<SetupState | null>(null);
  let problem = $state("");

  async function reload() {
    try {
      setup = await api.setup();
    } catch (err) {
      problem = errorText(err);
    }
  }

  // The rows the two lists draw, in the same words as the settings. A
  // model this machine cannot hold in its memory is not offered: it would
  // be gigabytes that answer in minutes rather than seconds.
  const speechRows = $derived<ModelRow[]>((setup?.speech ?? []).map((m) => speechRow(m)));
  const clipRows = $derived<ModelRow[]>(
    (setup?.language ?? []).filter((m) => m.fit !== "too big").map((m) => languageRow(m)),
  );

  // One mark for each kind of model, at its head, once one is here.
  const speechReady = $derived(!!setup?.hasSpeech);
  // A machine that finds clips in the cloud, chosen in the settings
  // before, needs no model of its own for that.
  const cloud = $derived(!!setup?.chosen && setup.planner === "api");
  const clipReady = $derived(!!setup?.hasLocalModel || cloud);

  // Once both are here the setup is done, and the app takes over. The
  // answer to how clips are found is written down first, so the app
  // knows it is a decision and does not ask again. One already given,
  // the cloud chosen in the settings before, is left as it is.
  let finishing = false;
  $effect(() => {
    if (!setup || finishing || !speechReady || !clipReady) return;
    finishing = true;
    finish(setup);
  });

  async function finish(now: SetupState) {
    try {
      if (!now.chosen) await api.choosePlanner("local");
      ondone();
    } catch (err) {
      problem = errorText(err);
      finishing = false;
    }
  }

  onMount(reload);
</script>

<section class="setup">
  <header>
    <h1>Frame Fairy</h1>
    <p class="muted lead">Turns a long video into vertical shorts. Two models to download first.</p>
  </header>

  {#if problem}<p class="error selectable">{problem}</p>{/if}

  {#if setup}
    <div class="stage">
      <div class="area asks">
        <span class="ask corner">
          <Info label="About the speech model" side="right">
            Every video is transcribed on this machine, word by word with the time of each word.
            Nothing about the video is sent anywhere. The model is not part of the app, so it is
            downloaded from the people who published it and kept on your machine, in
            <b>~/.framefairy/models</b>.
          </Info>
        </span>
        <div class="headrow">
          <h2>Speech model</h2>
          {#if speechReady}<span class="ready" title="Downloaded"><Icon name="check" /></span>{/if}
        </div>
        <p class="muted">Every video is transcribed on this machine.</p>
        <ModelList models={speechRows} kind="model" oninstall={api.installSpeechModel} onchange={reload} />
      </div>

      <div class="area asks">
        <span class="ask corner">
          <Info label="About the clip model" side="right">
            A language model reads the transcript and picks the moments worth clipping. It runs on
            this machine, is free to run and needs the memory to hold it. Only the words are read.
            A model in the cloud, Anthropic's or OpenAI's, can be chosen later in the settings. A
            model you already have in <b>~/.framefairy/models</b> is used as it is.
          </Info>
        </span>
        <div class="headrow">
          <h2>Clip model</h2>
          {#if clipReady}<span class="ready" title="Downloaded"><Icon name="check" /></span>{/if}
        </div>
        <!-- A model runs from memory, so the machine's own is said out
             loud here. A model's need is only believable next to the
             number it was judged against. -->
        <p class="muted">
          Reads the transcript and picks the moments.
          {setup.memory > 0 ? ` This machine has ${memorySize(setup.memory)} of memory.` : ""}
        </p>
        {#if cloud}
          <p class="muted">Clips are found in the cloud, as the settings say.</p>
        {:else if clipRows.length}
          <ModelList models={clipRows} kind="llm" oninstall={api.installLanguageModel} onchange={reload} />
        {:else}
          <p class="warn">None of the models fits in this machine's memory.</p>
        {/if}
        <!-- A model is the answer and llama-server is what runs it, so one
             without the other makes nothing. This is state and not
             standing help: it is here only while it is true. -->
        {#if !setup.hasServer}
          <p class="warn">
            <b>llama-server</b> is what runs a model here, and it is not on this machine.
          </p>
        {/if}
      </div>
    </div>
  {/if}
</section>

<style>
  /* One column in the middle of the app, and only as tall as what is in
     it. The setup is a few lines and two cards: a column stretched to the
     height of the app would be a strip of nothing under the last card,
     which grows as the app does. So the whole of it stands in the middle
     instead, and an app too short for it lets the cards scroll rather
     than the page. */
  .setup {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    width: 100%;
    max-width: 720px;
    max-height: 100%;
    margin: auto;
    padding: var(--edge);
    /* Whole lines, so the block is a whole number of pixels tall. The
       app's own 1.45 of 13 is 18.85, and a stack of those put every row
       here a third of a pixel off, where a fade paints a mark in one
       place and then another.

       With the block whole, what is left over is split in two, so it
       starts on a whole pixel or on a half, and never anywhere else. A
       half is a whole device pixel wherever the screen has two of them to
       the point, which is every Mac this runs on. */
    line-height: 19px;
  }

  /* Whole heights, because everything below is placed by what the head
     takes. A line box left to the font is a fraction high, and a fraction
     puts the step marks between two pixels, where a fade paints them in
     one place and then another. */
  h1 {
    font-size: var(--size-xl);
    line-height: 28px;
    font-weight: 600;
  }

  /* The head of an area, the same size and weight as the head of the clip
     list and the head of a group of settings. */
  h2 {
    font-size: var(--size-l);
    line-height: 23px;
    font-weight: 600;
  }

  .lead {
    margin-top: 4px;
    line-height: 20px;
  }

  /* The cards are as tall as what is in them, and give way first when
     the app is too short for the whole setup. */
  .stage {
    flex: 0 1 auto;
    min-height: 0;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: var(--edge);
  }

  .area {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .warn {
    color: var(--warn);
  }

  /* The head of a card, and the one mark that says its model is here. */
  .headrow {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .ready {
    display: flex;
    color: var(--ok);
  }

</style>
