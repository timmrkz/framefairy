<script lang="ts">
  // Which build of the app this is, and what is happening about the next
  // one, in one card: the build and the channel it follows on top, and
  // under it one line that says where things stand, with the one thing to
  // do about it at its end. A newer build downloads by itself, so that
  // thing is Update once it is here, and Check the rest of the time.
  // Reached from Updates at the foot of the sidebar and from Check for
  // Updates in the app menu. See docs/UPDATES.md.
  import { onMount } from "svelte";
  import { api, errorText, onUpdates, size, type UpdateState } from "../lib/api";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Pick from "../components/Pick.svelte";

  // A check that finds nothing is over in a tenth of a second, which is
  // a flash nobody can read: the button lit and went out and the line
  // under the build changed and changed back. So looking is shown for at
  // least this long, the answer after it.
  const LOOK_AT_LEAST = 1400;

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
    if (!update) return;
    problem = "";
    heard({ ...update, phase: "checking" });
    try {
      await api.checkForUpdates();
    } catch (e) {
      problem = errorText(e);
    }
  }

  async function follow(channel: string) {
    problem = "";
    try {
      await api.followChannel(channel);
    } catch (e) {
      problem = errorText(e);
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

  // A channel is main or a pull request, and on the trigger it is only
  // that: main or #18. The list says the whole title.
  const face = (id: string) => (id.startsWith("pr-") ? `#${id.slice(3)}` : id);
  const channelOptions = $derived.by(() => {
    const listed = (update?.channels ?? []).map((c) => ({ value: c.id, label: c.name, face: face(c.id) }));
    // A pull request that was picked and has since gone stays in the list
    // for as long as it is picked, so the trigger never names nothing.
    const picked = update?.picked ?? "";
    if (picked && !listed.some((o) => o.value === picked)) {
      listed.push({ value: picked, label: `${face(picked)}, merged or closed`, face: face(picked) });
    }
    // A build made by make follows nothing until a channel is picked.
    if (update && !update.channel) listed.unshift({ value: "", label: "Nothing", face: "Nothing" });
    return listed;
  });
  const following = $derived(
    update ? (update.channel ? update.picked || update.follows : update.picked) : "",
  );
  const followingName = $derived(
    update?.channels?.find((c) => c.id === (update?.follows || following))?.name ?? face(update?.follows ?? ""),
  );

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
  const next = $derived(
    update ? `${update.next}${update.nextCommit ? `, commit ${short(update.nextCommit)}` : ""}` : "",
  );

  // Where things stand, in the words of the line under the build: what it
  // is in a few words, then what that means.
  const standing = $derived.by((): { mark: string; head: string; more: string } => {
    const u = update;
    if (!u) return { mark: "", head: "", more: "" };
    if (u.off) return { mark: "off", head: "This build does not update itself", more: u.off };
    switch (u.phase) {
      case "checking":
        return { mark: "look", head: "Looking for a newer build", more: `Of ${followingName || "main"}.` };
      case "downloading": {
        const part = u.total > 0 ? `${Math.floor((u.written / u.total) * 100)} % of ${size(u.total)}.` : "";
        return { mark: "new", head: "A newer build is downloading", more: `${next}. ${part}`.trim() };
      }
      case "ready":
        return {
          mark: "new",
          head: "A newer build is ready",
          more: `${next}. Update restarts the app into it.`,
        };
      case "failed":
        return { mark: "err", head: "The check did not get through", more: `${u.problem} ${when(u.checked)}`.trim() };
      case "current": {
        const gone = u.picked && u.follows !== u.picked ? `${face(u.picked)} was merged or closed, so this follows main. ` : "";
        return {
          mark: "ok",
          head: "Up to date",
          more: `${gone}This is the newest build of ${followingName || "main"}. ${when(u.checked)}`.trim(),
        };
      }
    }
    if (!u.channel && !u.picked) {
      return {
        mark: "idle",
        head: "Built on this Mac",
        more: "It follows no channel. Pick one, and it downloads that channel's newest build.",
      };
    }
    return { mark: "idle", head: "Not checked yet", more: "It looks by itself every ten minutes." };
  });

  const looking = $derived(update?.phase === "checking");
  const downloading = $derived(update?.phase === "downloading");

  onMount(() => {
    const noUpdates = onUpdates(heard);
    api
      .updates()
      .then(heard)
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
    <div class="card">
      <!-- The build, and where the next one comes from. -->
      <div class="build">
        <Icon name="update" size={24} />
        <div class="what">
          <span class="version num selectable">{update.version}</span>
          <span class="muted small num selectable">
            {update.commit ? `Commit ${short(update.commit)}` : "Built on this Mac"}
          </span>
        </div>
        <span class="grow"></span>
        {#if !update.off}
          <label for="channel" class="muted">Follows</label>
          <Pick
            value={following}
            options={channelOptions}
            onpick={follow}
            id="channel"
            label="Channel"
            title="Where the next build comes from: main, or one pull request"
            disabled={channelOptions.length === 0}
          />
        {/if}
      </div>

      <!-- Where things stand, and the one thing to do about it. -->
      <div class="state" class:ready={update.phase === "ready"}>
        <span class="mark {standing.mark}" aria-hidden="true">
          {#if standing.mark === "ok"}
            <Icon name="check" size={14} />
          {:else}
            <span class="dot" class:busy={standing.mark === "look"} class:ready={standing.mark === "new"} class:err={standing.mark === "err"}></span>
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
            >{#if restarting}<Busy />{/if}{restarting ? "Updating" : "Update"}</button
          >
        {:else if !update.off}
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

  /* One card, the way a model is one card in the settings: the same
     border, radius and background. */
  .card {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    background: var(--ink-1);
  }

  .build,
  .state {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px;
  }

  .state {
    border-top: 1px solid var(--line);
  }

  /* The same background a chosen row wears everywhere else in the app,
     for the moment there is something to do. */
  .state.ready {
    background: var(--ink-3);
    border-bottom-left-radius: var(--radius-m);
    border-bottom-right-radius: var(--radius-m);
  }

  .what,
  .words {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .words {
    flex: 1;
  }

  .version {
    font-size: var(--size-l);
    font-weight: 600;
  }

  .head {
    font-weight: 600;
  }

  .grow {
    flex: 1;
  }

  /* The mark before the words is as wide as the icon beside the build, so
     the two lines start their words in one column. */
  .mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    flex: none;
  }

  .mark.ok {
    color: var(--ok);
  }

  /* The list is as wide as what its trigger says, main or #18, and no
     wider, the same as every list in the settings. The titles are in the
     list itself, which grows away from it. */
  .build :global(button.pick) {
    width: max-content;
  }

  /* Room for the longest wording, Downloading, so the row keeps still. */
  .act {
    min-width: 116px;
    flex: none;
  }
</style>
