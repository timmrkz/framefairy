<script lang="ts">
  // The first run. A new copy of the app on a machine with nothing on it
  // needs two things, and only one of them is a question.
  //
  // Speech is always local and there is one model today, so the app says
  // what it is about to do and does it. Finding clips is a real choice
  // between an Anthropic API key and a local model, with different costs
  // either way, so it is asked once here and can be changed later in the
  // settings.
  //
  // Nothing here is a box over the workspace. Until the question is
  // answered there is no workspace to put a box over, so this is the
  // window.
  import { onMount } from "svelte";
  import { api, errorText, type SetupState } from "../lib/api";
  import { jobs } from "../lib/state.svelte";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import SpeechModels from "../components/SpeechModels.svelte";

  let { ondone }: { ondone: () => void } = $props();

  let setup = $state<SetupState | null>(null);
  let mac = $state(true);
  let problem = $state("");
  let step = $state(0);

  const names = ["Speech", "Finding clips"];

  // The typed key, and whether it is on its way to the keychain. It is
  // never read back: the Go side only ever says whether one can be found.
  let key = $state("");
  let saving = $state(false);
  let savedKey = $state(false);

  async function reload() {
    try {
      setup = await api.setup();
    } catch (err) {
      problem = errorText(err);
    }
  }

  // Whether the speech model is still coming. The job list is shared, so
  // the last step reads it for itself rather than being told.
  const job = $derived(jobs.list.filter((j) => j.kind === "model").at(-1));
  const installing = $derived(
    job && (job.state === "running" || job.state === "queued") ? job : undefined,
  );

  async function choose(planner: "local" | "api") {
    problem = "";
    try {
      await api.choosePlanner(planner);
    } catch (err) {
      problem = errorText(err);
    }
    await reload();
  }

  async function saveKey() {
    saving = true;
    problem = "";
    try {
      await api.saveAPIKey(key);
      key = "";
      savedKey = true;
      setTimeout(() => (savedKey = false), 1800);
    } catch (err) {
      problem = errorText(err);
    }
    saving = false;
    await reload();
  }

  // The way out. Adding an episode is the next thing anybody does, so the
  // last button does that as well as closing the setup: cancelling the
  // file picker leaves them in the empty workspace, which says the same
  // thing again.
  async function finish() {
    ondone();
    try {
      await api.addEpisodes();
    } catch {
      // The workspace reads the library itself. A picker nobody chose
      // anything in is not a failure worth a message.
    }
  }

  // While the model is still coming there is nothing to be done with the
  // app, so the last button waits and shows how far it has got. It is
  // never a wall: an install that stopped leaves the button free, and the
  // row says what happened and offers to try again.
  const waiting = $derived(!!installing);
  const fraction = $derived(
    installing?.progress && installing.progress.fraction >= 0 ? installing.progress.fraction : -1,
  );

  onMount(async () => {
    api
      .platform()
      .then((p) => (mac = p === "darwin"))
      .catch(() => {});
    await reload();
  });
</script>

<section class="setup">
  <header>
    <h1>Frame Fairy</h1>
    <p class="muted lead">
      Turns a podcast episode into vertical shorts. Two things to set up first.
    </p>
  </header>

  <!-- Where you are, and nothing you press: going back is the button in
       the foot, and one thing is done in one way. -->
  <ol class="steps" aria-label="Setup">
    {#each names as name, i (name)}
      <li class:here={i === step} class:past={i < step}>
        <span class="mark num">{i + 1}</span>
        <span>{name}</span>
      </li>
    {/each}
  </ol>

  {#if problem}<p class="error selectable">{problem}</p>{/if}

  {#if setup}
    <div class="stage">
      {#if step === 0}
        <div class="area asks">
          <span class="ask corner">
            <Info label="About the speech model" side="right">
              Every episode is transcribed on this machine, word by word with the time of each
              word. Nothing about the episode is sent anywhere. The model is not part of the app,
              so it is fetched from its own home the first time and kept in
              <b>~/.framefairy/models</b>.
            </Info>
          </span>
          <h2>Speech</h2>
          <p class="muted">Every episode is transcribed on this machine.</p>
          <SpeechModels models={setup.speech} onchange={reload} auto />
        </div>
      {:else}
        <div class="area asks">
          <span class="ask corner">
            <Info label="About finding clips" side="right">
              A language model reads the transcript and picks the moments worth clipping. The
              Claude API works on any machine and costs a few cents an episode. A model on this
              machine is free to run and needs the memory to hold it. Either way the video and the
              audio stay here: only the words are read.
            </Info>
          </span>
          <h2>Finding clips</h2>
          <p class="muted">A language model reads the transcript and picks the moments.</p>
          <ul class="ways">
            <li class:on={setup.chosen && setup.planner === "api"}>
              <button class="pick" onclick={() => choose("api")}>
                <span class="title">Claude API</span>
                <span class="muted about">Works on any machine. A few cents an episode.</span>
              </button>
              {#if setup.chosen && setup.planner === "api"}
                <div class="more">
                  {#if setup.hasKey}
                    <p class="done row"><Icon name="check" />A key is in place.</p>
                  {:else if !mac}
                    <p class="warn">
                      This machine has no keychain, so the key comes from
                      <b>ANTHROPIC_API_KEY</b> in the environment the app starts in.
                    </p>
                  {/if}
                  {#if mac}
                    <div class="row">
                      <input
                        type="password"
                        bind:value={key}
                        placeholder={setup.hasKey ? "Replace the key" : "sk-ant-..."}
                        aria-label="Anthropic API key"
                        autocomplete="off"
                        spellcheck="false"
                      />
                      <button class="act" disabled={saving || !key.trim()} onclick={saveKey}>
                        {#if saving}<Busy />{/if}
                        {saving ? "Saving" : "Save"}
                      </button>
                    </div>
                    <p class="muted small">
                      {savedKey ? "Saved." : "It goes in the keychain and nowhere else."}
                    </p>
                  {/if}
                </div>
              {/if}
            </li>
            <li class:on={setup.chosen && setup.planner === "local"}>
              <button class="pick" onclick={() => choose("local")}>
                <span class="title">On this machine</span>
                <span class="muted about">Free to run. Needs the memory to hold the model.</span>
              </button>
              {#if setup.chosen && setup.planner === "local"}
                <div class="more">
                  {#if setup.hasLocalModel}
                    <p class="done row"><Icon name="check" />A model file is in place.</p>
                  {:else}
                    <p class="warn">
                      No model file yet. Put a <b>.gguf</b> file in
                      <b>~/.framefairy/models</b> and have <b>llama-server</b> on the machine.
                      The settings say where to look.
                    </p>
                  {/if}
                </div>
              {/if}
            </li>
          </ul>
        </div>
      {/if}
    </div>

    <footer class="row">
      <button class="act" disabled={step === 0} onclick={() => (step = step - 1)}>Back</button>
      <span class="grow"></span>
      {#if step === 0}
        <button class="primary go" onclick={() => (step = 1)}>Continue</button>
      {:else}
        <button class="primary go" disabled={waiting || !setup.chosen} onclick={finish}>
          {#if waiting}<Busy {fraction} />{/if}
          {waiting ? "Fetching the speech model" : "Add an episode"}
        </button>
      {/if}
    </footer>
  {/if}
</section>

<style>
  /* One column in the middle of the window, and only as tall as what is
     in it. The setup is a few lines and two cards: a column stretched to
     the height of the window would be a strip of nothing between the last
     card and the foot, which grows as the window does. So the whole of it
     stands in the middle instead, and a window too short for it lets the
     step scroll rather than the page. */
  .setup {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    width: 100%;
    max-width: 640px;
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

  /* Where you are. Two marks and their names, the numbers in the same
     tabular figures as every other number in the app. */
  .steps {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    gap: var(--gap);
    color: var(--muted);
  }

  .steps li {
    display: flex;
    align-items: center;
    gap: 6px;
    height: 24px;
  }

  .mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    border: 1px solid var(--line);
    font-size: var(--size-s);
  }

  .steps li.here {
    color: var(--text);
  }

  .steps li.here .mark {
    border-color: var(--accent);
    background: var(--accent-wash);
  }

  .steps li.past .mark {
    border-color: var(--accent);
  }

  /* The step is as tall as what is in it, and gives way first when the
     window is too short for the whole setup. */
  .stage {
    flex: 0 1 auto;
    min-height: 0;
    overflow: auto;
  }

  .area {
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .ways li {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 12px;
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    background: var(--ink-1);
  }

  /* The same background a selected row wears everywhere else in the app. */
  .ways li.on {
    border-color: var(--accent);
    background: var(--ink-3);
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

  .done {
    color: var(--ok);
    gap: 6px;
  }

  .warn {
    color: var(--warn);
  }

  /* A way of finding clips is picked by pressing the whole card, so the
     hit area is the thing it stands for rather than a small circle beside
     it. It is a button with nothing of a button about it. */
  .pick {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
    width: 100%;
    height: auto;
    padding: 0;
    border: none;
    background: transparent;
    text-align: left;
    white-space: normal;
  }

  .pick:hover:not(:disabled) {
    background: transparent;
    color: var(--text);
  }

  .more {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding-top: 2px;
  }

  footer {
    flex: none;
    border-top: 1px solid var(--line);
    padding-top: var(--gap);
  }

  /* Room for the longer wording and then some, so the button is the same
     size while the model is coming and when it has come. The wording is
     measured in the harness, where the font is not the one macOS uses, so
     the number has headroom rather than being the measurement. */
  .go {
    min-width: 216px;
  }

  .act {
    min-width: 92px;
  }
</style>
