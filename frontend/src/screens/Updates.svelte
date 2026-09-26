<script lang="ts">
  // Which build of the app this is, where the next one comes from, and the
  // one thing ever left to do about it, the restart. A newer build
  // downloads by itself. Reached from the build at the foot of the
  // sidebar and from Check for Updates in the app menu. See
  // docs/UPDATES.md.
  import { onMount } from "svelte";
  import { api, errorText, onUpdates, type UpdateState } from "../lib/api";
  import Busy from "../components/Busy.svelte";
  import Pick from "../components/Pick.svelte";

  let problem = $state("");

  // Which build is running and where the next one comes from. See
  // docs/UPDATES.md. The Go side sends the whole of it whenever any of it
  // changes, a few times a second while a build downloads.
  let update = $state<UpdateState | null>(null);
  let restarting = $state(false);
  const updating = $derived(update?.phase === "checking" || update?.phase === "downloading");
  const channelOptions = $derived.by(() => {
    const listed = (update?.channels ?? []).map((c) => ({ value: c.id, label: c.name }));
    // A pull request that was picked and has since gone stays in the list
    // for as long as it is picked, so the trigger never names nothing.
    const picked = update?.picked ?? "";
    if (picked && !listed.some((o) => o.value === picked)) {
      listed.push({ value: picked, label: `${picked}, gone` });
    }
    // A build made by make follows nothing until a channel is picked.
    if (update && !update.channel) listed.unshift({ value: "", label: "Nothing" });
    return listed;
  });
  const updateLine = $derived.by(() => {
    const u = update;
    if (!u) return "";
    if (u.off) return u.off;
    switch (u.phase) {
      case "checking":
        return "Looking for a newer build.";
      case "downloading":
        return `Downloading ${u.nextName || u.next}.`;
      case "ready":
        return `${u.nextName || u.next} is ready. Restart to use it.`;
      case "failed":
        return u.problem;
      case "current":
        return u.picked && u.follows !== u.picked
          ? `${u.picked} is gone, so this follows main, and has its newest build.`
          : "This is the newest build of the channel.";
    }
    return u.channel
      ? "Looks by itself every ten minutes."
      : "Built by make, so it looks only when a channel is picked or Check is clicked.";
  });

  async function follow(channel: string) {
    try {
      await api.followChannel(channel);
    } catch (e) {
      problem = errorText(e);
    }
  }

  async function checkForUpdates() {
    try {
      await api.checkForUpdates();
    } catch (e) {
      problem = errorText(e);
    }
  }

  async function restart() {
    restarting = true;
    try {
      await api.restartToUpdate();
    } catch (e) {
      restarting = false;
      problem = errorText(e);
    }
  }

  onMount(() => {
    const noUpdates = onUpdates((u) => (update = u));
    api
      .updates()
      .then((u) => (update = u))
      .catch(() => {});
    return noUpdates;
  });
</script>

<section class="scroll">
  {#if problem}<p class="error selectable">{problem}</p>{/if}
  {#if update}
    <div class="panel">
      <div class="row">
        <h2>This build</h2>
        <span class="grow"></span>
        {#if update.phase === "ready"}
          <button
            class="check primary"
            onclick={restart}
            disabled={restarting}
            title="Quit and come back as the new build. Waits while work runs"
            >{#if restarting}<Busy />{/if}{restarting ? "Restarting" : "Restart"}</button
          >
        {:else}
          <button
            class="check"
            onclick={checkForUpdates}
            disabled={!!update.off || updating}
            title="Look for a newer build of the channel now"
          >
            {#if update.phase === "downloading"}
              <Busy fraction={update.total > 0 ? update.written / update.total : -1} />
            {:else if update.phase === "checking"}
              <Busy />
            {/if}
            {update.phase === "downloading" ? "Downloading" : update.phase === "checking" ? "Checking" : "Check"}
          </button>
        {/if}
      </div>
      <div class="grid">
        <span class="name">Version</span>
        <span class="num selectable"
          >{update.version}{#if update.commit}<span class="muted">, commit {update.commit}</span>{/if}</span
        >
        {#if !update.off}
          <label for="channel">Follows</label>
          <Pick
            value={update.channel ? update.picked || update.follows : update.picked}
            options={channelOptions}
            onpick={follow}
            id="channel"
            label="Channel"
            title="Where the next build comes from: main, or one pull request"
            disabled={channelOptions.length === 0}
          />
        {/if}
        <span></span>
        <span class="muted small selectable" class:error={update.phase === "failed"}>{updateLine}</span>
      </div>
    </div>
  {/if}

</section>

<style>
  /* The same page as the settings: a name and a control beside it, kept
     to a width that reads, in the middle of the app. */
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

  .grid {
    display: grid;
    grid-template-columns: 200px 1fr;
    gap: 8px 16px;
    align-items: center;
  }

  /* Every name in the left column wears the same grey, a label or not. */
  .grid label,
  .grid .name {
    color: var(--muted);
  }

  /* Room for the longest wording, Downloading, so the row keeps still. */
  .check {
    min-width: 116px;
  }

  /* The list is as wide as the longest channel name and no wider, the
     same as every list in the settings. */
  .grid :global(button.pick) {
    width: max-content;
  }
</style>
