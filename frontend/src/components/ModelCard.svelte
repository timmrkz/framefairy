<script lang="ts">
  // The card of one kind of model in the settings, the speech model's and
  // the clip model's: the choice, with the mark before it and the one line
  // under it, Download beside it when the one chosen is not here, Cancel
  // with the beam and the fill while it comes, and under it, folded, the
  // models of this kind on this machine with the room they take, the way a
  // pop-up in the Mac's own settings changes the rows beneath it. Tim
  // found the two drawn two ways, 2026-10-27, and asked for them to look
  // and work alike, so they are one card used twice, never two copies.
  // What only one kind has, the key of a company in the cloud and
  // llama-server, are rows the caller hands in.
  import type { Snippet } from "svelte";
  import type { Job, ModelRow } from "../lib/api";
  import type { Standing, Tone } from "../lib/finding";
  import type { Install } from "../lib/install.svelte";
  import Busy from "./Busy.svelte";
  import Icon from "./Icon.svelte";
  import ModelList from "./ModelList.svelte";
  import Pick from "./Pick.svelte";

  let {
    // The card's own name, for the walks and the shake: "finding" for the
    // clip model and "speech" for the speech model.
    name,
    card = $bindable(),
    shaking,
    onshaken,
    // What the choice is, Find clips with or Transcribe with.
    head,
    // The line under it, and how the choice stands, which the mark before
    // it and the frame round the list both show. using is a choice that
    // works and that work running elsewhere is using now.
    line,
    install,
    // Download, where the one chosen is not here: its title and what it
    // does. Left out while there is nothing to fetch.
    download = undefined,
    oncancel,
    pick,
    // The models of this kind on this machine, folded, and the room they
    // take together. No fold without them.
    rows,
    room,
    list,
    children = undefined,
  }: {
    name: string;
    card?: HTMLElement;
    shaking: boolean;
    onshaken: () => void;
    head: string;
    line: { text: string; tone: Tone; state: Standing; using?: boolean };
    install: Install;
    download?: { title: string; onclick: () => void };
    oncancel: () => void;
    pick: {
      value: string;
      options: { value: string; label: string; detail?: string; warn?: boolean; group?: string }[];
      onpick?: (value: string) => void;
      label: string;
      title?: string;
      refuse?: () => boolean;
    };
    rows: ModelRow[];
    room: string;
    list: {
      kind: "model" | "llm";
      oninstall: (name: string) => Promise<Job>;
      onchange: () => void;
      onremove: (name: string) => Promise<void>;
      removeSays?: string;
      locked?: string;
      onlocked?: () => void;
    };
    children?: Snippet;
  } = $props();

  // The models on this machine are only about the room they take, so they
  // stay folded until somebody wants some of it back.
  let open = $state(false);
  const plural = (n: number) => `${n.toLocaleString("en")} ${n === 1 ? "model" : "models"}`;
</script>

<div class="card shakes {name}" class:shaking bind:this={card} onanimationend={onshaken}>
  <div class="item">
    <span class="mark {line.state}" aria-hidden="true">
      {#if line.state === "busy" || line.using}
        <span class="dot busy"></span>
      {:else if line.state === "ok"}
        <Icon name="check" />
      {:else}
        <Icon name="warn" />
      {/if}
    </span>
    <div class="words">
      <span class="head">{head}</span>
      <span
        class="small line"
        class:muted={!line.tone}
        class:warn={line.tone === "warn"}
        class:error={line.tone === "err"}
        title={line.text}>{line.text}</span
      >
    </div>
    {#if install.running}
      <!-- The download carries the beam and the fill in the one control
           that can do something about it. -->
      <button
        class="act"
        title="Stop the download. What has arrived stays, and Download carries on from it"
        disabled={install.cancelling}
        onclick={oncancel}
      >
        <Busy fraction={install.fraction} />
        {install.cancelling ? "Cancelling" : "Cancel"}
      </button>
    {:else if download}
      <button class="act" title={download.title} disabled={install.starting} onclick={download.onclick}
        >{#if install.starting}<Busy />{/if}{install.starting ? "Starting" : "Download"}</button
      >
    {/if}
    <Pick
      value={pick.value}
      options={pick.options}
      onpick={pick.onpick}
      label={pick.label}
      align="right"
      tone={line.state === "warn" || line.state === "err" ? line.state : undefined}
      disabled={!!install.running || install.starting}
      refuse={pick.refuse}
      title={pick.title}
    />
  </div>
  {@render children?.()}
  {#if rows.length > 0}
    <button class="item disclose-row" aria-expanded={open} onclick={() => (open = !open)}>
      <span class="mark" aria-hidden="true"><Icon name="folder" /></span>
      <span class="words">
        <span class="head">Downloaded models</span>
        <span class="small muted">{plural(rows.length)} on this machine, {room}.</span>
      </span>
      <span class="chevron" class:open><Icon name="chevron" size={14} /></span>
    </button>
    {#if open}
      <ModelList
        bare
        models={rows}
        kind={list.kind}
        oninstall={list.oninstall}
        onchange={list.onchange}
        onremove={list.onremove}
        removeSays={list.removeSays}
        locked={list.locked}
        onlocked={list.onlocked}
      />
    {/if}
  {/if}
</div>

<style>
  .act {
    flex: none;
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

  /* The list is as wide as the longest name in it, and hangs from the
     card's right edge. */
  .card :global(button.pick) {
    width: max-content;
    flex: none;
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
</style>
