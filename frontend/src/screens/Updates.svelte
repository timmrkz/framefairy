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
  import { onMount } from "svelte";
  import { api, errorText, onUpdates, size, type UpdateState } from "../lib/api";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import Pick from "../components/Pick.svelte";

  // A check that finds nothing is over in a tenth of a second, which is
  // a flash nobody can read: the button lit and went out and the line
  // under the build changed and changed back. So looking is shown for at
  // least this long, the answer after it.
  const LOOK_AT_LEAST = 1400;

  // A made-up state to show instead of the Go side's, for the Updates test
  // in the Help menu. A page with one asks the Go side nothing: picking a
  // channel only moves the list, and the buttons do nothing.
  let { sample = undefined }: { sample?: UpdateState } = $props();

  // What the Go side last said, and what is on screen, which trails it
  // while a check is held.
  let latest = $state<UpdateState | null>(null);
  let update = $state<UpdateState | null>(null);
  let lookingSince = 0;
  let held: ReturnType<typeof setTimeout> | undefined;
  let restarting = $state(false);
  let problem = $state("");
  // Now, a few times a minute, so "just now" becomes a time by itself.
  let now = $state(Date.now());

  function heard(u: UpdateState) {
    latest = u;
    if (u.phase === "checking") {
      if (update?.phase !== "checking") lookingSince = Date.now();
      clearTimeout(held);
      update = u;
      return;
    }
    const left = LOOK_AT_LEAST - (Date.now() - lookingSince);
    if (update?.phase === "checking" && left > 0) {
      clearTimeout(held);
      held = setTimeout(() => (update = latest), left);
      return;
    }
    update = u;
  }

  // A click shows at once. The Go side's own word that it is looking
  // follows a moment later and changes nothing.
  async function check() {
    if (!update || sample) return;
    problem = "";
    heard({ ...update, phase: "checking" });
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
    if (sample) {
      update = { ...update, picked: channel, gone: "" };
      return;
    }
    problem = "";
    heard({ ...update, picked: channel, gone: "", building: "", next: "", phase: "checking" });
    try {
      await api.followChannel(channel);
    } catch (e) {
      problem = errorText(e);
      api
        .updates()
        .then(heard)
        .catch(() => {});
    }
  }

  async function install() {
    if (sample) return;
    problem = "";
    restarting = true;
    try {
      await api.restartToUpdate();
    } catch (e) {
      restarting = false;
      problem = errorText(e);
    }
  }

  // A channel is what it is and nothing more: a branch or a pull request
  // by its number. The pull request's title says what it is about, which
  // is not what is being picked here. Customers will see releases the same
  // way, by their version.
  const channelName = (id: string) => (id.startsWith("pr-") ? `PR #${id.slice(3)}` : `Branch ${id}`);
  // The channel followed, as updates.Followed has it: the one picked, or
  // the one a build from a channel came from. Empty for a build made on
  // the Mac until a channel is chosen.
  const following = $derived(update ? update.picked || update.follows || update.gone || update.channel || "" : "");
  const followingName = $derived(following ? channelName(following) : "");
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

  function when(checked: string | undefined): string {
    if (!checked) return "";
    const at = Date.parse(checked);
    // Go's zero time is the year 1.
    if (!isFinite(at) || at < Date.UTC(2000, 0, 1)) return "";
    if (now - at < 60_000) return "Checked just now.";
    const t = new Date(at);
    return `Checked at ${String(t.getHours()).padStart(2, "0")}:${String(t.getMinutes()).padStart(2, "0")}.`;
  }

  const short = (commit: string) => commit.slice(0, 7);
  // A build's version ends in its commit, so the version says it all.
  const next = $derived(update?.next ?? "");

  // A commit pushed to the channel whose build has not come yet. The build
  // there is is still offered, it is the newest there is, but never as
  // the newest commit: a build of the commit before was once tested as
  // the fix it did not have yet.
  const building = $derived(update?.building ? short(update.building) : "");
  const stillBuilding = $derived(building ? ` ${building} is being built.` : "");

  // Where things stand, in the words of the line under the build: what it
  // is in a few words, then what that means.
  const standing = $derived.by((): { mark: string; head: string; more: string } => {
    const u = update;
    if (!u) return { mark: "", head: "", more: "" };
    if (u.off) return { mark: "off", head: "This build does not update itself", more: u.off };
    switch (u.phase) {
      case "checking":
        return { mark: "look", head: "Looking for a newer build", more: following ? `Of ${followingName}.` : "" };
      case "downloading": {
        const part = u.total > 0 ? `${Math.floor((u.written / u.total) * 100)} % of ${size(u.total)}.` : "";
        return { mark: "new", head: "A newer build is downloading", more: `${next}. ${part}${stillBuilding}`.trim() };
      }
      case "ready":
        return {
          mark: "new",
          head: "A newer build is ready",
          more: `${next}. Relaunch to finish updating, or it goes in when you quit.${stillBuilding}`,
        };
      case "failed":
        return { mark: "err", head: "The check did not get through", more: `${u.problem} ${when(u.checked)}`.trim() };
      // The newest build is here, and a newer commit is on its way. Work
      // running somewhere else, so the pulse.
      case "current":
        if (building)
          return {
            mark: "look",
            head: "A newer commit is being built",
            more: `${building}. This is the build before it, the newest of ${followingName} so far. ${when(u.checked)}`.trim(),
          };
        return {
          mark: "ok",
          head: "Up to date",
          more: `This is the newest build of ${followingName}. ${when(u.checked)}`.trim(),
        };
      // The pull request followed was merged or closed. Nothing downloads
      // by itself: which channel to follow next is the person's to say.
      case "gone":
        return {
          mark: "warn",
          head: `${channelName(u.gone)} is closed`,
          more: "Nothing downloads until you choose what to follow next.",
        };
    }
    // A build made on the Mac follows nothing until a channel is chosen,
    // whatever an earlier build picked. Choosing one follows it fully, so
    // looking is the next thing this line says.
    if (!following) {
      return {
        mark: "idle",
        head: "Built on this Mac",
        more: "It stays as it is until you choose a channel to follow.",
      };
    }
    // A build from a channel, in the moment before its first look has
    // answered.
    return { mark: "idle", head: "Not checked yet", more: "It looks when the app starts and every ten minutes." };
  });

  const looking = $derived(update?.phase === "checking");
  const downloading = $derived(update?.phase === "downloading");

  onMount(() => {
    if (sample) {
      update = sample;
      return;
    }
    // Asking also reads the channel list, and what that read found is
    // sent as an event, which can arrive before the answer to the asking
    // does. The answer is then the older of the two, and it emptied the
    // list again, so it only counts while no event has come.
    let told = false;
    const noUpdates = onUpdates((u) => {
      told = true;
      heard(u);
    });
    api
      .updates()
      .then((u) => told || heard(u))
      .catch(() => {});
    const tick = setInterval(() => (now = Date.now()), 15_000);
    return () => {
      noUpdates();
      clearInterval(tick);
      clearTimeout(held);
    };
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
            title="Where this app updates from: main, or one pull request. It downloads the newest build by itself"
            tone={update.phase === "gone" ? "warn" : undefined}
            disabled={channelOptions.length === 0}
          />
        {/if}
      </div>

      <!-- Where things stand, and the one thing to do about it. -->
      <div class="item" class:on={update.phase === "ready"}>
        <span class="mark {standing.mark}" aria-hidden="true">
          {#if standing.mark === "ok"}
            <Icon name="check" size={14} />
          {:else}
            <span
              class="dot"
              class:busy={standing.mark === "look"}
              class:ready={standing.mark === "new"}
              class:err={standing.mark === "err"}
              class:warn={standing.mark === "warn"}
            ></span>
          {/if}
        </span>
        <div class="words">
          <span class="head">{standing.head}</span>
          <span class="muted small selectable">{standing.more}</span>
        </div>
        {#if update.phase === "ready"}
          <button
            class="act primary"
            onclick={install}
            disabled={restarting}
            title="Quits and comes back as the new build. Waits while work runs"
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

  /* The list is as wide as what its trigger says and no wider, the same
     as every list in the settings. It hangs from the trigger's right edge,
     the edge of the card, and grows away from it into the card. */
  .build :global(button.pick) {
    width: max-content;
  }

  /* Room for the longest wording, Downloading, so the row keeps still. */
  .act {
    min-width: 116px;
    flex: none;
  }
</style>
