<script lang="ts">
  // Which build of the app this is, and what is happening about the next
  // one, in one card: the build and the channel it follows on top, and
  // under it one line that says where things stand, with the one thing to
  // do about it at its end. A newer build downloads by itself, so that
  // thing is Relaunch once it is here, and Check the rest of the time a
  // channel is followed. A build made on the Mac follows nothing until a
  // channel is chosen, and with nothing to look for there is no Check.
  // Relaunch is Chrome's word for it: a newer version downloads by itself,
  // the mark on Updates says it is here, and one click relaunches into it.
  // Reached from Updates at the foot of the sidebar and from Check for
  // Updates in the app menu. See docs/UPDATES.md.
  import { onDestroy, onMount } from "svelte";
  import { api, errorText } from "../lib/api";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import Pick from "../components/Pick.svelte";
  import { jobs } from "../lib/state.svelte";
  import { channelName, conflicted as conflictedIn, following as followingIn, standing as standingOf, updates } from "../lib/updates.svelte";

  // What is on screen, which the sidebar's dot on Updates is read from
  // too, so the two say the same, see lib/updates.svelte.ts.
  const update = $derived(updates.shown);
  let restarting = $state(false);
  let problem = $state("");
  // Now, a few times a minute, so "just now" becomes a time by itself.
  let now = $state(Date.now());

  // A click shows at once. The Go side's own word that it is looking
  // follows a moment later and changes nothing.
  async function check() {
    if (!update) return;
    problem = "";
    updates.heard({ ...update, phase: "checking" });
    try {
      await api.checkForUpdates();
    } catch (e) {
      problem = errorText(e);
    }
  }

  // A pick shows at once too: the channel in the list and looking under
  // it, before the Go side has said a word. Should it say no, what it
  // says is true is asked for again.
  async function follow(channel: string) {
    if (!update) return;
    problem = "";
    updates.heard({ ...update, picked: channel, gone: "", building: "", next: "", phase: "checking" });
    try {
      await api.followChannel(channel);
    } catch (e) {
      problem = errorText(e);
      updates.ask();
    }
  }

  // What the system said copies with one click, so it can go into a
  // report or a message. The button says it copied in the same frame as
  // the click, and goes back a moment later, the way the key ID in the
  // settings does. Issue #417.
  let copied = $state(false);
  let copyRefused = $state("");
  let copiedTimer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(copiedTimer));
  async function copyDetail(text: string) {
    copied = true;
    copyRefused = "";
    clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => (copied = false), 1500);
    try {
      await api.copy(text);
    } catch (e) {
      copied = false;
      copyRefused = errorText(e);
    }
  }

  async function install() {
    problem = "";
    restarting = true;
    try {
      await api.restartToUpdate();
    } catch (e) {
      restarting = false;
      problem = errorText(e);
    }
  }

  const following = $derived(followingIn(update));
  const followingName = $derived(following ? channelName(following) : "");
  const conflicted = $derived(conflictedIn(update));
  const channelOptions = $derived.by(() => {
    // A pull request that no longer merges into main says so in its name,
    // the way a closed one does. Its build is of the branch as it is, so
    // it is still there to try.
    const listed = (update?.channels ?? []).map((c) => ({
      value: c.id,
      label: c.conflict ? `${channelName(c.id)}, conflicts` : channelName(c.id),
    }));
    // A pull request that was followed and has since gone stays in the
    // list for as long as it is followed, so the trigger never names
    // nothing and says what became of it. Closed only when a list that was
    // read says so: the Go side sets gone from the list alone. With no list
    // read yet, because the check did not get through, the channel is
    // missing from an empty list, and an open pull request was called
    // closed.
    const kept = following;
    if (kept && !listed.some((o) => o.value === kept)) {
      const label = update?.gone === kept ? `${channelName(kept)}, closed` : channelName(kept);
      listed.push({ value: kept, label });
    }
    // Nothing is not in the list. A build made on the Mac follows nothing
    // until a channel is picked, which is a state and not a choice, so the
    // trigger says Choose a channel, and once one is picked another channel
    // is what follows it.
    return listed;
  });

  // Work running or waiting to, which Relaunch waits for: quitting into
  // the new build would stop it half way, issue #382. The Go side refuses
  // a Relaunch pressed all the same. In the words of the work the person
  // started, the first there is.
  const working = $derived.by(() => {
    const job = jobs.list.find((j) => j.state === "running" || j.state === "queued");
    if (!job) return "";
    switch (job.kind) {
      case "search":
        return "the search";
      case "render":
        return "the render";
      case "clip":
        return "the clip being made";
      case "transcribe":
        return "the transcription";
      default:
        return "the download";
    }
  });

  const short = (commit: string) => commit.slice(0, 7);

  // Where things stand, in the words of the line under the build, with
  // the mark the sidebar's dot is made from.
  const standing = $derived(standingOf(update, { working, now }));

  // What the system said when a check did not get through.
  const detail = $derived(update?.phase === "failed" ? update.detail : "");

  const looking = $derived(update?.phase === "checking");
  const downloading = $derived(update?.phase === "downloading");

  onMount(() => {
    // The app listens from its start, see App.svelte. Opening the page
    // asks again, so what it shows is never older than the opening.
    updates.ask();
    const tick = setInterval(() => (now = Date.now()), 15_000);
    return () => clearInterval(tick);
  });
</script>

<section class="scroll">
  {#if problem}<p class="error selectable">{problem}</p>{/if}

  {#if update}
    <div class="card asks">
      <!-- The build, and where the next one comes from. -->
      <div class="item build">
        <Icon name="update" size={24} />
        <div class="words">
          <span class="version num selectable">{update.version}</span>
          <span class="muted small num">
            {#if update.commit}Commit <button
                class="link num"
                style="--custom-contextmenu: commit"
                title="Opens this commit on GitHub"
                onclick={() => api.openCommit().catch((e) => (problem = errorText(e)))}>{short(update.commit)}</button
              >{:else}Built on this Mac{/if}
          </span>
        </div>
        <span class="grow"></span>
        <!-- A word in front of the list, Follows, said nothing the list
             does not say. How updates work is behind the mark beside it,
             which shows while the pointer is on the card. It is not in the
             card's corner, as a mark usually is, because the list is: a
             mark there sat on the list's edge. And a title on the list
             alone was not seen, since macOS shows one only after the
             pointer has rested a second. -->
        {#if !update.off}
          <span class="ask">
            <Info label="How updates work" side="right">
              The list says where this app updates from: main, or one pull request. Every push to
              it makes a new build. The app looks when it starts, when you choose a channel and every
              ten minutes, every twenty seconds while a newer commit is being built, and downloads the
              newest build by itself. <b>Relaunch</b> restarts the app into it, and if you quit instead,
              it goes in on the way out. A build made on this Mac follows nothing until you choose a
              channel. The commit under the version opens on GitHub.
            </Info>
          </span>
          <Pick
            value={following}
            options={channelOptions}
            onpick={follow}
            id="channel"
            label="Channel"
            placeholder="Choose a channel"
            align="right"
            fit
            title="Where this app updates from: main, or one pull request. It downloads the newest build by itself"
            tone={update.phase === "gone" || conflicted ? "warn" : undefined}
            disabled={channelOptions.length === 0}
          />
        {/if}
      </div>

      <!-- Where things stand, and the one thing to do about it. -->
      {#snippet act()}
        {#if update.phase === "ready"}
          <button
            class="act primary"
            onclick={install}
            disabled={restarting || !!working}
            title={working ? `Waits until ${working} is done` : "Quits and comes back as the new build"}
            >{#if restarting}<Busy />{/if}{restarting ? "Relaunching" : "Relaunch"}</button
          >
        {:else if !update.off && following}
          <button
            class="act"
            onclick={check}
            disabled={looking || downloading}
            title="Look for a newer build of the channel now"
          >
            {#if downloading}
              <Busy fraction={update.total > 0 ? update.written / update.total : -1} />
            {:else if looking}
              <Busy />
            {/if}
            {downloading ? "Downloading" : looking ? "Checking" : update.phase === "failed" ? "Try again" : "Check"}
          </button>
        {/if}
      {/snippet}
      <div class="item" class:told={!!detail}>
        <!-- A dot is work, running or waiting to be picked up. Something
             that needs a look is the triangle, the way the settings mark a
             missing key, in the colour of a warning or of a failure. Work
             on a pull request in conflict keeps its dot, in the colour of
             a warning. -->
        <span class="mark {standing.mark}" aria-hidden="true">
          {#if standing.mark === "ok"}
            <Icon name="check" />
          {:else if standing.mark === "warn" || standing.mark === "err"}
            <Icon name="warn" />
          {:else}
            <span
              class="dot"
              class:busy={standing.mark === "look"}
              class:ready={standing.mark === "new"}
              class:warn={conflicted && (standing.mark === "look" || standing.mark === "new")}
            ></span>
          {/if}
        </span>
        <div class="words">
          <span class="head">{standing.head}</span>
          <span class="muted small selectable">{standing.more}</span>
        </div>
        {#if detail}
          <!-- What the system said, apart from the words above it, on one
               line that scrolls sideways, so a long address never wraps
               the row into a wall of text, and Try again on the same row
               as it, the way a field and its button sit in the settings.
               Tim saw it as the reason itself, over three lines, and then
               the field under the words with Try again beside them both,
               lined up with neither, issue #417. -->
          <div class="said">
            <div class="detail">
              <code class="selectable" title="What the system said">{detail}</code>
              <button
                class="quiet copy"
                class:copied
                title={copied ? "Copied" : "Copy what the system said, for a report"}
                aria-label={copied ? "Copied" : "Copy what the system said"}
                onclick={() => copyDetail(detail)}><Icon name={copied ? "check" : "copy"} size={14} /></button
              >
            </div>
            {@render act()}
          </div>
          {#if copyRefused}<span class="small error refused">{copyRefused}</span>{/if}
        {:else}
          {@render act()}
        {/if}
      </div>
    </div>
  {/if}
</section>

<style>
  /* The same page as the settings, kept to a width that reads, in the
     middle of the app. */
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

  /* The build is the head of the page, so its version is set larger. The
     card, its rows and the mark before the words are app.css's .card, the
     same as the settings. */
  .version {
    font-size: var(--size-l);
    font-weight: 600;
  }

  .grow {
    flex: 1;
  }



  .build .words {
    flex: none;
  }

  /* The trigger is as wide as the channel it names and no wider, see fit
     in Pick. The list hangs from its right edge, the edge of the card, and
     grows away from it into the card as far as its longest name needs. */
  .build :global(button.pick) {
    width: max-content;
  }

  .act {
    flex: none;
  }

  /* A row that says what the system said wraps: the mark and the words
     on its first line, as in every other row, and under them the field
     and Try again on a line of their own, starting where the words start,
     past the mark's 24 px and the row's 12 px gap, both from .card in
     app.css, which the walk holds them to. */
  .item.told {
    flex-wrap: wrap;
    row-gap: var(--gap);
  }

  .told > .said,
  .told > .refused {
    flex-basis: calc(100% - 24px - 12px);
    margin-left: calc(24px + 12px);
  }

  .said {
    display: flex;
    align-items: center;
    gap: var(--gap);
    min-width: 0;
  }

  /* What the system said: a field at the height of every control, its
     words at one width the way code is, on one line that scrolls, and
     the button that copies them at its end. */
  .detail {
    display: flex;
    align-items: center;
    flex: 1;
    height: var(--control-h);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--ink-2);
    box-sizing: border-box;
    min-width: 0;
  }

  .detail code {
    flex: 1;
    min-width: 0;
    overflow-x: auto;
    white-space: pre;
    padding: 0 var(--control-pad);
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
    font-size: var(--size-s);
    line-height: calc(var(--control-h) - 2px);
    color: var(--muted);
    /* No scroll bar: with macOS set to show them always, or a mouse
       plugged in, WebKit draws one about 15 px high inside the line and
       pushes the words up. It scrolls with the trackpad, the wheel with
       Shift and a drag across it, and Copy takes the whole of it. */
    scrollbar-width: none;
  }

  .detail code::-webkit-scrollbar {
    display: none;
  }

  /* Square, the way the trash can is, inside the field's own line. */
  .detail .copy {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: calc(var(--control-h) - 2px);
    height: calc(var(--control-h) - 2px);
    padding: 0;
    color: var(--muted);
  }

  .detail .copy.copied {
    color: var(--ok);
  }
</style>
