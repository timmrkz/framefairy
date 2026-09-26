<script lang="ts">
  import { onMount } from "svelte";
  import {
    api,
    clock,
    onChrome,
    onEpisodeChanged,
    onAcknowledgements,
    onShowUpdates,
    onUpdates,
    type UpdateState,
    onQuit,
    type Chrome,
    type EpisodeStatus,
    errorText,
  } from "./lib/api";
  import { chosen, jobs, nav, shell } from "./lib/state.svelte";
  import Icon from "./components/Icon.svelte";
  import Confirm from "./components/Confirm.svelte";
  import Busy from "./components/Busy.svelte";
  import { installFonts } from "./lib/fonts";
  import { wearColour } from "./lib/colour";
  import Episode from "./screens/Episode.svelte";
  import Jobs from "./screens/Jobs.svelte";
  import SettingsScreen from "./screens/Settings.svelte";
  import Acknowledgements from "./screens/Acknowledgements.svelte";
  import UpdatesScreen from "./screens/Updates.svelte";
  import Setup from "./screens/Setup.svelte";

  // The sidebar is a rail until the pointer reaches it, and stays open when
  // it is pinned. Open, it lies over the workspace rather than pushing it,
  // so the video preview never changes size while you reach for an episode.
  let near = $state(false);
  const open = $derived(shell.pinned || near);

  // The button does what it says at once: open, it closes the sidebar, even
  // with the pointer still on it. Hover opens it again once the pointer has
  // left and come back.
  function toggleBar() {
    shell.set(!open);
    if (open) near = false;
  }

  let episodes = $state<EpisodeStatus[]>([]);
  // What is on screen, said once, in the bar at the top. The
  // screens do not write their own name any more.
  const title = $derived.by(() => {
    if (nav.view.name === "episode") {
      const path = nav.view.path;
      return episodes.find((ep) => ep.source === path)?.name ?? "";
    }
    if (nav.view.name === "jobs") return "Activity";
    if (nav.view.name === "settings") return "Settings";
    if (nav.view.name === "updates") return "Updates";
    if (nav.view.name === "acknowledgements") return "Acknowledgements";
    return "Frame Fairy";
  });

  // Whether a newer build of the app is ready, which the rail marks on
  // Settings, where the restart is. See docs/UPDATES.md.
  let update = $state<UpdateState | null>(null);
  const updateReady = $derived(update?.phase === "ready");
  let problem = $state("");

  // The first run. A new copy of the app cannot transcribe without a
  // speech model and cannot find clips until somebody has said how, so
  // until both are answered the setup is the whole app: no sidebar, no
  // workspace, nothing to press that would not work.
  //
  // Null until the Go side has answered, so the workspace never flashes up
  // for a moment before the setup covers it.
  let settingUp = $state<boolean | null>(null);

  async function refresh() {
    try {
      episodes = await api.library();
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function add() {
    try {
      const added = await api.addEpisodes();
      await refresh();
      if (added && added.length) nav.go({ name: "episode", path: added[0] });
    } catch (err) {
      // One file that could not be added does not mean none of them were,
      // so the list is read again either way.
      problem = errorText(err);
      await refresh();
    }
  }

  // Cmd+Q heard: the app dims at once and says what happens next. See
  // quit.go for why the first press only asks while work runs.
  let leaving = $state<"ask" | "going" | null>(null);
  const quitKey = /Mac/.test(navigator.userAgent) ? "⌘Q" : "Ctrl+Q";

  // Removing an episode belongs to the episode in the list, not to the
  // workspace, which is about the clips.
  let removing = $state<EpisodeStatus | null>(null);

  // Which answer is being carried out. Removing waits for the episode's
  // work to stop, which takes a moment while a search runs, so the button
  // says so at once and the box takes no second click.
  let removingHow = $state<"keep" | "delete" | null>(null);

  async function remove(ep: EpisodeStatus, deleteWork: boolean) {
    if (removingHow) return;
    removingHow = deleteWork ? "delete" : "keep";
    try {
      await api.removeEpisode(ep.source, deleteWork);
      // The workspace goes first. Left open, it went on reading the
      // episode it showed, which was no longer in the library, and every
      // read came back as "file does not exist".
      if (nav.view.name === "episode" && nav.view.path === ep.source) nav.go({ name: "empty" });
      // An episode added again is a new one: its window starts from the
      // beginning and it looks for its first clips by itself.
      chosen.forget(ep.source);
      removing = null;
      await refresh();
    } catch (err) {
      removing = null;
      problem = errorText(err);
    } finally {
      removingHow = null;
    }
  }

  function selected(path: string): boolean {
    const v = nav.view;
    return v.name === "episode" && v.path === path;
  }

  function summary(ep: EpisodeStatus): string {
    if (ep.missing) return "File not found";
    const parts: string[] = [];
    if (ep.transcribed) parts.push("Transcribed");
    else if (ep.transcriptStale) parts.push("Changed since transcription");
    else if (ep.covered > 0) parts.push(`Transcribed to ${clock(ep.covered)}`);
    else parts.push("Not transcribed");
    const plans = ep.plans?.length ?? 0;
    if (plans) parts.push(plans === 1 ? "1 plan" : `${plans} plans`);
    if (ep.rendered) parts.push(`${ep.rendered} rendered`);
    return parts.join(", ");
  }

  // The folder beside the episode that holds everything the app made of
  // it. The remove box names it, so it is clear what is kept or deleted
  // and where to find it later.
  function workFolder(path: string): string {
    const base = path.split(/[\\/]/).pop() ?? path;
    return `${base.replace(/\.[^.]+$/, "")}.framefairy`;
  }

  function dotFor(ep: EpisodeStatus): string {
    if (jobs.active(ep.source)) return "busy";
    if (ep.missing) return "err";
    if (ep.transcriptStale) return "warn";
    if (ep.plans?.length) return "ok";
    return "";
  }

  // A pointer that lands anywhere else takes the keyboard with it. Clicking
  // the arrows of a number field leaves that field holding the keyboard, and
  // then the space bar types into it instead of playing the clip, however
  // much else you did in between.
  function handOverFocus(event: PointerEvent) {
    const on = document.activeElement as HTMLElement | null;
    if (!on || on === document.body) return;
    if (event.target instanceof Node && on.contains(event.target)) return;
    on.blur();
  }

  // Where macOS put the title bar and its buttons. All zeros away from
  // macOS, where the system draws its own title bar, and then the bar is
  // an ordinary header and the stylesheet keeps its own numbers.
  //
  // Nothing here works a size out from a measurement of the page. This is
  // the one thing the stylesheet cannot know, asked for once and told
  // again when it changes, and handed over as four numbers it lays itself
  // out from.
  let chrome = $state<Chrome>({ bar: 0, left: 0, right: 0, middle: 0 });
  // The height is given whenever there is one. Where the buttons are is
  // given only when there are buttons, so that in native fullscreen, where
  // macOS takes them away, the stylesheet falls back to centring the name
  // in the bar rather than being told to centre it on nothing.
  const shellStyle = $derived.by(() => {
    const parts: string[] = [];
    if (chrome.bar > 0) parts.push(`--bar-h: ${chrome.bar}px`);
    if (chrome.right > 0) {
      parts.push(`--lights-l: ${chrome.left}px`);
      parts.push(`--lights-r: ${chrome.right}px`);
      parts.push(`--lights-y: ${chrome.middle}px`);
    }
    return parts.join("; ");
  });

  onMount(() => {
    jobs.start();
    installFonts().catch(() => {});
    api
      .chrome()
      .then((c) => (chrome = c))
      .catch(() => {});
    const noChrome = onChrome((c) => (chrome = c));
    // The app wears the colour it was given before anything is drawn in it.
    api
      .getSettings()
      .then((s) => wearColour(s.appColour))
      .catch(() => {});
    api
      .setup()
      .then((s) => (settingUp = !s.chosen || !s.hasSpeech))
      // A machine that cannot answer is not a machine to hold in a setup
      // screen it can never leave.
      .catch(() => (settingUp = false));
    refresh();
    const off = onEpisodeChanged(() => refresh());
    const noAcknowledgements = onAcknowledgements(() => nav.go({ name: "acknowledgements" }));
    api
      .updates()
      .then((u) => (update = u))
      .catch(() => {});
    const noUpdates = onUpdates((u) => (update = u));
    // Check for Updates in the app menu shows the answer where it is kept.
    const noShowUpdates = onShowUpdates(() => nav.go({ name: "updates" }));
    // The question lasts as long as the Go side waits for the second
    // press, quitAgain in quit.go.
    let asked: ReturnType<typeof setTimeout> | undefined;
    const dismiss = (e: KeyboardEvent) => {
      if (e.key === "Escape" && leaving === "ask") leaving = null;
    };
    window.addEventListener("keydown", dismiss);
    const noQuit = onQuit((what) => {
      clearTimeout(asked);
      leaving = what;
      if (what === "ask") asked = setTimeout(() => (leaving = null), 3000);
    });
    window.addEventListener("pointerdown", handOverFocus);
    return () => {
      clearTimeout(asked);
      window.removeEventListener("keydown", dismiss);
      noQuit();
      window.removeEventListener("pointerdown", handOverFocus);
      noChrome();
      noAcknowledgements();
      noUpdates();
      noShowUpdates();
      off();
    };
  });
</script>

<div class="shell" style={shellStyle}>
  <!-- The bar across the top: the close, minimise and zoom buttons, and
       the name of what is on screen beside them. It is the whole width of
       the app, so the sidebar opens under it and never covers the name. -->
  <!-- The bar is the title bar. macOS lays that out and puts its three
       buttons on its middle, and the app takes the height it was given
       rather than asking for one.
       This used to say that nothing the app can set moves those buttons,
       which is false and cost a great deal: Electron apps move them and
       VS Code does, and chrome_darwin.go already holds the handle, since
       it asks the app's window for standardWindowButton: and reads the
       frame.
       What was really wrong was asking for a toolbar, which makes the bar
       taller and makes macOS inset the buttons to centre them in it. See
       the WebviewWindowOptions in main.go.
       Then the buttons are on the bar's middle because the bar is what
       they were centred in. The name goes in a box whose middle is their
       middle, which is the same thing said the other way round. Where the
       system draws its own title bar there is nothing to agree with and
       the stylesheet keeps its own numbers. -->
  <header class="bar">
    <span class="name"><span class="what">{title}</span></span>
  </header>

  <div class="body" class:alone={settingUp !== false}>
  {#if settingUp === null}
    <!-- Nothing, for the moment it takes to ask. The bar is already there,
         so the app is not blank. -->
  {:else if settingUp}
    <Setup
      ondone={() => {
        settingUp = false;
        refresh();
      }}
    />
  {:else}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <aside
    class:open
    onpointerenter={() => (near = true)}
    onpointerleave={() => (near = false)}
    onfocusin={() => (near = true)}
    onfocusout={() => (near = false)}
  >
    <div class="head row">
      <button
        class="quiet glyph"
        onclick={toggleBar}
        aria-pressed={open}
        title={open ? "Close the sidebar" : "Keep the sidebar open"}
      >
        <Icon name="sidebar" />
      </button>
      <h2>Episodes</h2>
    </div>
    <ul class="scroll">
      {#each episodes as ep (ep.source)}
        <li>
          <button
            class="episode"
            class:current={selected(ep.source)}
            onclick={() => nav.go({ name: "episode", path: ep.source })}
          >
            <span class="dot {dotFor(ep)}"></span>
            <span class="name" title={summary(ep)}>{ep.name}</span>
          </button>
          <span class="tools">
            <button
              class="glyph small"
              title="Show in folder"
              aria-label="Show {ep.name} in folder"
              disabled={ep.missing}
              onclick={() => api.reveal(ep.source)}
            >
              <Icon name="folder" />
            </button>
            <button
              class="glyph small quiet danger"
              title="Remove"
              aria-label="Remove {ep.name}"
              aria-haspopup="dialog"
              onclick={() => (removing = ep)}
            >
              <Icon name="trash" />
            </button>
          </span>
        </li>
      {:else}
        <li class="empty muted">
          Add an episode with the plus below to start. Any mp4, mov, m4v or mkv works.
        </li>
      {/each}
    </ul>
    <div class="foot">
      <button
        class="quiet nav"
        onclick={add}
        title="Add an episode. Any mp4, mov, m4v or mkv works"
      >
        <Icon name="plus" />
        <span class="label">Add</span>
      </button>
      <button
        class="quiet nav"
        class:current={nav.view.name === "jobs"}
        onclick={() => nav.go({ name: "jobs" })}
        title={jobs.busy
          ? jobs.busy === 1
            ? "Activity, one job running"
            : `Activity, ${jobs.busy} jobs running`
          : "Activity"}
      >
        <span class="mark">
          <Icon name="activity" />
          <!-- Work in hand is one dot on the icon, the same dot as beside
               an episode in the list and pulsing the same way. It sits
               over the icon, so nothing on the rail moves when a job
               starts or ends. -->
          {#if jobs.busy}<span class="dot busy"></span>{/if}
        </span>
        <span class="label">Activity</span>
      </button>
      <button
        class="quiet nav"
        class:current={nav.view.name === "settings"}
        onclick={() => nav.go({ name: "settings" })}
        title="Settings"
      >
        <Icon name="sliders" />
        <span class="label">Settings</span>
      </button>
      <!-- The build that is running, which is also the way to its updates.
           It says which build this is, so a report from testing always
           names what was tested. -->
      <button
        class="quiet nav"
        class:current={nav.view.name === "updates"}
        onclick={() => nav.go({ name: "updates" })}
        title={updateReady
          ? `Updates. ${update?.nextName || update?.next} is ready, restart to use it`
          : `Updates. This is ${update?.version ?? "the app"}`}
      >
        <span class="mark">
          <Icon name="update" />
          <!-- A new build ready is a dot that stands still, in the same
               place as the dot for work in hand. It is not work running,
               so it does not pulse. -->
          {#if updateReady}<span class="dot ready"></span>{/if}
        </span>
        <span class="label">Updates</span>
        <span class="label muted small num build">{update?.version ?? ""}</span>
      </button>
    </div>
  </aside>

  <main>
    {#if problem}
      <p class="error banner selectable">{problem}</p>
    {/if}
    {#if nav.view.name === "episode"}
      {#key nav.view.path}
        <Episode path={nav.view.path} onchange={refresh} />
      {/key}
    {:else if nav.view.name === "jobs"}
      <Jobs />
    {:else if nav.view.name === "updates"}
      <UpdatesScreen />
    {:else if nav.view.name === "settings"}
      <SettingsScreen />
    {:else if nav.view.name === "acknowledgements"}
      <Acknowledgements />
    {:else}
      <div class="welcome">
        <h1>Pick an episode</h1>
        <p class="muted">
          Choose one on the left, or add a new one. The app transcribes it on this machine, finds
          the moments worth clipping and renders them as vertical shorts.
        </p>
      </div>
    {/if}
  </main>
  {/if}
  </div>

  {#if leaving}
    <!-- Over everything, the moment the key is heard. A click or Escape
         takes the question away, the same as waiting does. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="leaving"
      role="status"
      onpointerdown={() => leaving === "ask" && (leaving = null)}
    >
      <p>{leaving === "ask" ? `Press ${quitKey} again to quit` : "Quitting"}</p>
    </div>
  {/if}

  {#if removing}
    {@const ep = removing}
    <Confirm title="Remove {ep.name}?" oncancel={() => !removingHow && (removing = null)}>
      {#if ep.work}
        <p>
          The transcript, the clip sets and the rendered clips are in
          <b>{workFolder(ep.source)}</b>, beside the episode. Keeping that folder means adding the
          episode again picks up where this left off, deleting it starts from the beginning. The
          episode file itself always stays.
        </p>
      {:else}
        <p>
          There is nothing beside the episode to throw away: it has no
          <b>{workFolder(ep.source)}</b> folder. Only the episode leaves the list, the file itself
          stays where it is.
        </p>
      {/if}
      <!-- Left to right, the safest answer to the one that cannot be taken
           back, which is where the eye and the hand both expect it. The
           keyboard starts on Cancel: what is irreversible is marked as
           what it is rather than made the answer Enter gives. No answer is
           picked out: this box asks which of three things to do and the
           app has no opinion on two of them, so all three look alike and
           only the one that cannot be taken back is marked. The other
           boxes have two answers and no highlight either. -->
      {#snippet actions()}
        <button onclick={() => (removing = null)} disabled={!!removingHow}>Cancel</button>
        {#if ep.work}
          <button class="answer" onclick={() => remove(ep, false)} disabled={!!removingHow}
            >{#if removingHow === "keep"}<Busy />{/if}{removingHow === "keep"
              ? "Removing"
              : "Keep"}</button
          >
        {/if}
        <button class="answer danger" onclick={() => remove(ep, true)} disabled={!!removingHow}
          >{#if removingHow === "delete"}<Busy />{/if}{removingHow === "delete"
            ? "Removing"
            : "Remove"}</button
        >
      {/snippet}
    </Confirm>
  {/if}
</div>

<style>
  /* Cmd+Q heard. The dim is the one a box over the workspace brings,
     Confirm.svelte, so the app looks the same whenever it is waiting on
     an answer. The words are the size of a box's title. */
  .leaving {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: grid;
    place-items: center;
    background: rgba(0, 0, 0, 0.55);
  }

  /* The words on a dark ground of their own, the way macOS shows the
     volume, because the dim alone leaves them over the captions in the
     video preview. No line round it and nothing to press: a box over the
     workspace is Confirm.svelte, and it only asks about what cannot be
     taken back. */
  .leaving p {
    margin: 0;
    padding: 12px 20px;
    border-radius: var(--radius-m);
    background: var(--ink-0);
    font-size: var(--size-l);
    font-weight: 600;
  }

  /* Room for Removing, so the row of answers does not move when one is
     clicked. */
  .answer {
    min-width: 96px;
  }

  /* The rail keeps its own column, so nothing of the workspace ever hides
     under it. The sidebar grows over the workspace from there. */
  .shell {
    position: relative;
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  /* The bar belongs to the app as a whole: it is what you drag the app
     by, it holds the close, minimise and zoom buttons on macOS, and it
     says what is on screen. */
  /* Exactly as tall as the title bar macOS laid out, which the shell sets
     from what macOS answers, or the token where the system draws its own
     bar. The line at the foot is drawn inside that height rather than
     under it, so the bar is the title bar and nothing else.
     The line was taken out once and put back. Read off VS Code and
     Terminal: both draw one under their title bar, and VS Code's is
     brighter against its own bar than ours is against ours. It was never
     what made the app look unlike theirs. */
  .bar {
    position: relative;
    flex: none;
    height: var(--bar-h);
    box-shadow: inset 0 -1px var(--line);
    background: var(--ink-1);
    --wails-draggable: drag;
  }

  /* The name stands in a box that reaches from the top of the bar to as
     far below the buttons' middle as that middle is below the top, so the
     middle of the box is the middle of the buttons. It starts one space
     after the last of them. Nothing is moved by a transform, so nothing
     is put on a compositing surface of its own and the whole pixel rule
     holds. Where there are no buttons in the page the box is the
     bar, and the name is centred in it. */
  .bar .name {
    position: absolute;
    top: 0;
    left: calc(var(--lights-r, 0px) + var(--gap));
    right: var(--gap);
    height: calc(2 * var(--lights-y, calc(var(--bar-h) / 2)));
    display: flex;
    align-items: center;
    overflow: hidden;
  }

  /* The name itself. Two things have to be true of it at once and they
     pull against each other.

     A line box keeps room below the baseline for the letters that go
     there and room above the capitals that no capital uses, and there is
     more of the first than of the second, so a name centred by its line
     box reads high. Trimming the box to the capitals and the baseline is
     what puts the letters themselves on the middle.

     But the box that is trimmed is also the box that is clipped, and the
     name is clipped so a long one ends in an ellipsis rather than running
     under the sidebar. Trimmed and clipped together is what cut the tail
     off the y in YouTube.mp4.

     So the box is trimmed to the capitals and then padded back out. The
     padding is even, so the middle of the box is still the middle of the
     capitals, and it is deeper than any letter goes, so nothing is cut.
     Clipping happens at the padding, which is the whole of the name
     again. */
  .bar .what {
    min-width: 0;
    font-size: var(--size-m);
    font-weight: 600;
    line-height: 1;
    text-box: trim-both cap alphabetic;
    padding: 6px 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Where the browser cannot trim a text box there is nothing to pad back
     out, and a plain line box centred on the buttons is a small fraction
     high rather than wrong. */
  @supports not (text-box: trim-both cap alphabetic) {
    .bar .what {
      line-height: normal;
      padding: 0;
    }
  }

  /* Where the system draws its own title bar the name keeps the left of
     the bar, because there is nothing at the left to make room for. */
  .shell:not([style*="--lights-r"]) .bar .name {
    left: var(--gap);
  }

  .body {
    position: relative;
    display: grid;
    grid-template-columns: var(--rail) 1fr;
    flex: 1;
    min-height: 0;
  }

  /* The setup has the whole app, rail and all: there is nothing on the
     rail worth reaching for until it is done.

     The row is as tall as the app, not as tall as the setup. Left
     implicit it is sized to what is in it, which in an app too short for
     the whole setup makes the row taller than the app, and then the foot
     of the setup is below the bottom edge with nothing to scroll: the
     page itself never scrolls. minmax(0, 1fr) is the row being the app's
     own height and what is in it being allowed to give way. */
  .body.alone {
    grid-template-columns: 1fr;
    grid-template-rows: minmax(0, 1fr);
  }

  aside {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    z-index: 10;
    width: var(--rail);
    display: flex;
    flex-direction: column;
    background: var(--ink-1);
    /* The line at the edge is drawn inside rather than as a border, so the
       rail is 44px of room for the icons and not 43. That way an icon and
       its highlight sit on whole pixels in the middle of the rail. */
    box-shadow: inset -1px 0 var(--line);
    min-height: 0;
    overflow: hidden;
    transition: width 0.14s ease;
  }

  aside.open {
    width: 272px;
    box-shadow:
      inset -1px 0 var(--line),
      8px 0 24px rgba(0, 0, 0, 0.45);
  }

  @media (prefers-reduced-motion: reduce) {
    aside {
      transition: none;
    }
  }

  /* What there is no room for on the rail waits until the sidebar opens.
     An icon on the rail has to be exactly where its row is when the
     sidebar opens, or the sidebar opens and moves it out from under the
     pointer that came for it. So a row keeps its height and only its
     words go, and an episode keeps its row and shows only its lamp. */
  aside:not(.open) h2,
  aside:not(.open) .label {
    display: none;
  }

  /* On the rail an episode is its lamp and nothing else. What it is called
     and what it has wait for the room, and so do the two marks, which need
     a pointer on the row anyway. The row keeps its height, so the lamp is
     in the same place shut and open and nothing moves as the sidebar
     goes over. An episode nobody has added yet has nothing to show on the
     rail at all. */
  aside:not(.open) .episode .name,
  aside:not(.open) .tools,
  aside:not(.open) li.empty {
    display: none;
  }

  /* The sidebar slides open, so for as long as it moves its contents are
     narrower than the room they will have. Nothing in it may reflow on the
     way, or a line wraps, a row grows and the icons jump out from under the
     pointer that came for them. Every line in the sidebar stays one line and
     what does not fit yet is clipped by the sidebar. */
  .head h2,
  .nav .label {
    white-space: nowrap;
  }

  .nav .label {
    min-width: 0;
    overflow: hidden;
  }

  /* The list keeps its 4 at the sides on the rail, so a lamp stands in the
     same column shut and open and does not step across as the sidebar goes
     over. Only the scrolling goes, because a rail has nothing to scroll. */
  aside:not(.open) ul {
    overflow: hidden;
  }

  /* Never squeezed by the rail, or the glyph would sit somewhere else on
     the rail than it does when the sidebar is open. */
  .glyph {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 36px;
    padding: 0;
    color: var(--muted);
  }

  .glyph:hover:not(:disabled) {
    color: var(--text);
  }

  /* 4px each side of a 36px box leaves the icon in the middle of the rail,
     and the same box holds the rows in the foot. */
  .head {
    justify-content: space-between;
    padding: 8px 12px 8px 4px;
  }

  .head h2 {
    flex: 1;
  }

  h2 {
    font-size: var(--size-l);
    font-weight: 600;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0 4px;
    flex: 1;
    min-height: 0;
  }

  li {
    position: relative;
  }

  /* A whole number of pixels high, and the same number whether the sidebar
     is shut or open: the 36 the rail gives every icon, so a row shut is an
     icon's box around its lamp and a row open is the same box with the
     name beside it. Saying the height rather than letting the text say it
     means the row does not shrink to its lamp when the name goes, and a
     lamp that dropped up the list as the sidebar opened is the one thing a
     sidebar sliding over must not do.

     It said the episode's state on a second line, and 54 was the height of
     two. One line is what a row of this kind holds everywhere else on the
     rail, and the state is in the row's own title for whoever wants it.

     The left padding is the foot's, so the lamp stands in the same column
     as every icon on the rail: 4 from the list, 1 of border and 9 here put
     the lamp's box at 14, and an 8 wide lamp centred in a 16 wide box
     leaves it on 22, which is the middle of the rail. */
  .episode {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    height: 36px;
    padding: 0 8px 0 9px;
    text-align: left;
    border-color: transparent;
    background: transparent;
    margin-bottom: 2px;
  }

  /* An icon's box around a lamp, so it is centred on the icons' column. */
  .episode .dot {
    margin: 0 4px;
  }

  /* An even number, so the name sits on a whole pixel in a row of an even
     height. Thirteen point five at the usual line height is nineteen, and
     nineteen in thirty-six leaves half a pixel above and below. */
  .episode .name {
    line-height: 20px;
  }

  .episode.current,
  .nav.current {
    background: var(--ink-3);
  }

  /* Room for the two marks, so a long name never runs under them. */
  .episode .name {
    min-width: 0;
    padding-right: 48px;
  }

  /* What you do with an episode itself waits until the pointer is on its
     row, the way the trash can does on a clip. */
  .tools {
    position: absolute;
    /* Centred on the row: a 24 tall mark in a 36 tall row. */
    top: 6px;
    right: 6px;
    display: flex;
    gap: 2px;
    opacity: 0;
    pointer-events: none;
  }

  li:hover .tools,
  .tools:focus-within {
    opacity: 1;
    pointer-events: auto;
  }

  .glyph.small {
    width: 24px;
    height: 24px;
    border-radius: var(--radius-s);
  }

  .glyph.small:hover:not(:disabled) {
    background: var(--ink-3);
  }

  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .small {
    font-size: var(--size-s);
  }

  .empty {
    padding: 8px;
  }

  .foot {
    border-top: 1px solid var(--line);
    padding: 8px 4px;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 2px;
  }

  /* The icon of a foot row sits where the sidebar glyph at the head sits,
     so the rail is one column of icons and every one of them is in the
     same place when the sidebar opens over it. */
  .nav {
    text-align: left;
    display: flex;
    align-items: center;
    /* 9px and the button's own 1px border put the icon where the glyph at
       the head has it, in the middle of the rail. */
    gap: 8px;
    padding: 0 9px;
  }

  .nav .label {
    flex: 1;
  }

  .mark {
    position: relative;
    display: flex;
    flex: none;
  }

  .mark .busy,
  .mark .ready {
    position: absolute;
    top: -2px;
    right: -3px;
    width: 7px;
    height: 7px;
  }

  /* The build stands at the far end of its row, where a count stands in
     a list. */
  .nav .build {
    margin-left: auto;
  }

  /* The sidebar is out of the flow, so the workspace has to be told to
     stay in the second column and leave the rail alone. */
  main {
    grid-column: 2;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  .banner {
    padding: var(--gap) var(--edge);
  }

  /* In the middle of the workspace, so the sidebar can lie over the left of
     it and the words are still there to read. */
  .welcome {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 24px 48px 64px;
    text-align: center;
  }

  .welcome p {
    max-width: 52ch;
  }

  .welcome h1 {
    font-size: var(--size-xl);
    font-weight: 600;
  }
</style>
