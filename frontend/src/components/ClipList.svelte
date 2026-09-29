<script lang="ts">
  import { flip } from "svelte/animate";
  import { slide } from "svelte/transition";
  import { clock, type ClipEntry } from "../lib/api";
  import type { Arriving } from "../lib/arriving";

  import Busy from "./Busy.svelte";
  import Icon from "./Icon.svelte";

  let {
    clips,
    arriving = [],
    selected,
    removed = "",
    coming = 0,
    waiting = true,
    next = null,
    stopped = null,
    onselect,
    onremove,
    onputback,
  }: {
    clips: ClipEntry[];
    // The clips on their way, whoever proposed them, each in its place in
    // the episode among the clips that are there, wearing the work that is
    // being done to it. A clip made by hand is one from the moment I or O
    // is pressed, and a clip the model named is one from the moment it is
    // named. Each hands over to the clip itself, in the same place, when
    // it is written.
    arriving?: Arriving[];
    selected: string;
    // How many clips are on the way: the number the search was asked for,
    // while the transcript is still coming or the search is running. That
    // many rows wait in place, so the list is already the shape it is
    // about to be, and the info mark at the head says what is going on.
    coming?: number;
    // Whether anything is on its way to fill those rows. They breathe while
    // it is and stand still while nothing is, the way paused work does.
    waiting?: boolean;
    // What the first of those rows is waiting on, while it waits: what is
    // being done, how long is left, and how far it has come, -1 when that
    // is not known. That row wears the work running, because it is where
    // the next clip will appear.
    // still is work that has been told to stop and has not said so yet:
    // it keeps its fill and stops moving, the way Busy draws it.
    next?: { what: string; left: string; fraction: number; still?: boolean } | null;
    // How the last search ended, when it stopped before it was done and
    // nothing is running now. It is said in the row its next clip would
    // have appeared in, the same row that says what a search is doing
    // while it runs, so the end of the story is where the story was told.
    // It stays until a search starts: after a restart too, because the
    // engine keeps it with the episode.
    stopped?: { what: string; left: string; full: string } | null;
    // The clip just taken out. It keeps its place in the list for a moment,
    // showing what happened to it and offering it back, so the rows do not
    // jump out from under the pointer.
    removed?: string;
    onselect: (key: string) => void;
    // Takes a clip out of the list. It stays in the plan, so it can come
    // back.
    onremove?: (clip: ClipEntry) => void;
    onputback?: () => void;
  } = $props();

  // A search writes each clip the moment it is found, so the list fills in
  // one row at a time and the rows still to come shrink as it does. What
  // is handed in already counts the clips that are there.
  const ghosts = $derived.by(() => {
    const n = Math.max(0, coming - clips.length - arriving.length);
    return Array.from({ length: n }, (_, i) => i);
  });

  // The clips there are and the clips on the way, in the order they are
  // spoken, which is the order of the range picker and the clip timeline.
  //
  // A clip on its way and the clip it becomes are one row: the row keeps
  // the card's key from the moment the card knows which clip it will be,
  // and the clip takes the row over when the list has it. They were two
  // rows, one going and one coming, and between the two the list was a
  // row short for a moment, which the browser answered by pulling a list
  // that was scrolled down back up, every card jumping at once. The keys
  // are kept for as long as the list is, so a clip never changes rows.
  type Row = { key: string; start: number; clip?: ClipEntry; arriving?: Arriving };
  const rowOf = new Map<string, string>();
  const rows = $derived.by((): Row[] => {
    for (const a of arriving) if (a.clip && !rowOf.has(a.clip)) rowOf.set(a.clip, a.key);
    const here = new Set(clips.map((c) => c.key));
    const all: Row[] = [
      ...clips.map((clip) => ({ key: rowOf.get(clip.key) ?? clip.key, start: clip.start, clip })),
      ...arriving
        .filter((a) => !(a.clip && here.has(a.clip)))
        .map((a) => ({ key: a.key, start: a.start, arriving: a })),
    ];
    return all.sort((a, b) => a.start - b.start);
  });

  // A clip taken out slides away. A clip on the way does not: it becomes
  // the clip itself, in the same place and the same frame, so the list
  // never shows the two of them at once.
  function leave(node: Element, row: Row) {
    return row.clip ? slide(node, { duration: 200 }) : { duration: 0 };
  }

  // A clip on the way that makes the list longer comes in the way a clip
  // that is taken out goes, so a card appearing among the others is seen
  // appearing. One that takes the place of a row still to come is already
  // there, the size it will be: it slid open from nothing while the row it
  // took went at once, so for a moment the list was a row short and every
  // card under it jumped up and slid back down.
  let settled = 0;
  $effect(() => {
    settled = rows.length + ghosts.length;
  });
  function enter(node: Element, row: Row) {
    const grows = rows.length + ghosts.length > settled;
    return row.arriving && grows ? slide(node, { duration: 200 }) : { duration: 0 };
  }

  // The rows still to come are after the clips there are, so in a list
  // longer than its column a search began out of sight: all anyone saw
  // was New turning into Cancel. The row the next clip will appear in, the
  // one wearing the work, is what is being done, so it is kept in view:
  // brought to the top of the column when the search starts, with the rows
  // still to come under it, and brought back every time a clip lands,
  // wherever the list has been scrolled to in the meantime. It comes back
  // with part of the row after it showing, so it is plain there is more to
  // come, and the clip that just landed is right above it. Only when a clip
  // lands: following every report would fight a hand that is scrolling.
  let nextRow = $state<HTMLLIElement>();
  let hadNext = false;
  $effect(() => {
    const has = !!next && !!nextRow;
    if (has && !hadNext) nextRow!.scrollIntoView({ block: "start", behavior: "smooth" });
    hadNext = has;
  });
  //
  // The last clip a search finds leaves no row to come after it, so there
  // is nothing to follow: that clip itself is brought into view, to the
  // foot of the list when it lands below. The search may already have
  // reported that it is done by the time its last clip is in the list, so
  // a landing a few seconds after the search is still one of its own.
  let list = $state<HTMLOListElement>();
  let known: Set<string> | null = null;
  let searchedUntil = 0;
  $effect(() => {
    if (next) searchedUntil = Infinity;
    else if (searchedUntil === Infinity) searchedUntil = Date.now() + 5000;
  });
  $effect(() => {
    const keys = clips.map((c) => c.key);
    const before = known;
    known = new Set(keys);
    if (!before || !list || Date.now() > searchedUntil) return;
    const landed = keys.filter((k) => !before.has(k));
    if (!landed.length) return;
    const row = nextRow;
    const own = list;
    // After whatever else the landing moves. The first clip found is
    // chosen, and the workspace brings the chosen card into view at once,
    // which stops a smooth scroll that began before it.
    setTimeout(() => {
      const target =
        row?.isConnected
          ? row
          : own.querySelector<HTMLElement>(`li[data-key="${CSS.escape(landed[landed.length - 1])}"]`);
      target?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    });
  });
</script>

<ol bind:this={list}>
  {#each rows as row (row.key)}
    {@const clip = row.clip}
    {@const a = row.arriving}
    <li
      animate:flip={{ duration: 180 }}
      in:enter={row}
      out:leave={row}
      data-key={clip?.key ?? a?.key}
      class:next={!!a}
      class:current={!!a && a.key === selected}
      class:stopped={!!a?.stopped}
      aria-live={a ? "polite" : undefined}
    >
      {#if a}
        {#if a.stopped}
          <button class="pick carry" title={a.full} onclick={() => a.oncontinue?.()}>
            <span class="title">{a.what}</span>
            <span class="meta muted num">{a.left}</span>
          </button>
        {:else}
          <Busy fraction={a.fraction} still={a.still} />
          <span class="title">{a.title || a.what}</span>
          <span class="meta muted num">{a.title ? a.what : clock(a.start)}{a.left ? `, ${a.left}` : ""}</span>
        {/if}
      {:else if clip && clip.key === removed}
        <div class="gone">
          <Icon name="trash" />
          <span class="what">Removed</span>
          <span class="grow"></span>
          <button class="quiet back" onclick={() => onputback?.()}>Put it back</button>
        </div>
      {:else if clip}
        <button
          class="pick"
          class:current={clip.key === selected}
          onclick={() => onselect(clip.key)}
        >
          <span class="title">{clip.title || clip.slug}</span>
          <span class="meta muted num">
            {clock(clip.start)}, {Math.round(clip.duration)} s
            {#if clip.rendered}<span class="dot ok" title="This clip is rendered"></span>{/if}
          </span>
        </button>
        {#if onremove}
          <button
            class="drop quiet danger"
            title="Take this clip out of the list"
            aria-label="Remove {clip.title || clip.slug}"
            onclick={() => onremove(clip)}
          >
            <Icon name="trash" />
          </button>
        {/if}
      {/if}
    </li>
  {/each}
  <!-- The rows that are not there yet, after whatever is. They are as many
       as the search was asked for, so the list does not fill out from four
       rows to twelve, and the shimmer over them is the only thing that
       says it is still working. Each row is a step further into the
       shimmer's round than the one above, so every card is tipped at its
       own angle and the light runs down the column rather than the whole
       column being one sheet. The step is a twelfth of the round, and
       twelve is what a search is asked for, so a full list covers the
       round exactly once. -->
  {#each ghosts as row (row)}
    {#if row === 0 && next}
      <li class="ghost next" aria-live="polite" bind:this={nextRow}>
        <Busy fraction={next.fraction} still={next.still} />
        <span class="title">{next.what}</span>
        <span class="meta muted num">{next.left}</span>
      </li>
    {:else if row === 0 && stopped}
      <li class="ghost next stopped" title={stopped.full}>
        <span class="title">{stopped.what}</span>
        <span class="meta muted num">{stopped.left}</span>
      </li>
    {:else}
      <li class="ghost" class:waiting style="--wait-in: {row * 800}ms"></li>
    {/if}
  {/each}
</ol>

<style>
  /* Every clip is a card of its own with air around it, rather than a row
     in a box. A list of things you pick one of reads as a stack of things,
     not as a block with lines drawn in it, and a card can be picked up by
     the eye at a glance. The space at the top and the bottom is what the
     veil over the ends of the list fades, so at rest nothing is faded and
     a card only goes under it once the list is scrolled. */
  ol {
    list-style: none;
    margin: 0;
    /* No space at the sides: a card ends exactly where the head above it
       and the button beside that end, which is what makes a column read as
       one column. */
    padding: var(--veil, 16px) 0;
    display: flex;
    flex-direction: column;
    gap: var(--gap);
  }

  /* The edge is drawn inside the card rather than around it, so the card's
     content is a whole number of pixels wide whatever the column does. */
  li {
    position: relative;
    /* Brought into view clear of the veil over either end of the list. */
    scroll-margin: var(--veil, 16px) 0;
    border-radius: var(--radius-m);
    background: var(--ink-1);
    box-shadow: inset 0 0 0 1px var(--ink-3);
    overflow: hidden;
  }

  /* The trash can waits until the pointer is on the row, so the list stays
     a list of clips and not a row of buttons. */
  .drop {
    position: absolute;
    top: 50%;
    right: 6px;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    padding: 0;
    border: none;
    border-radius: var(--radius-s);
    background: transparent;
    color: var(--muted);
    opacity: 0;
    pointer-events: none;
  }

  li:hover .drop,
  .drop:focus-visible {
    opacity: 1;
    pointer-events: auto;
  }



  .pick {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    justify-content: center;
    gap: 2px;
    width: 100%;
    height: 56px;
    padding: 0 10px;
    border: none;
    border-radius: 0;
    background: transparent;
    text-align: left;
  }

  li:hover .pick {
    background: var(--ink-2);
  }

  /* The chosen clip, in the same grey every chosen row in the app uses,
     with the app's own colour down its edge so it is plain which card it
     is without reading any of them. */
  .pick.current,
  li:hover .pick.current,
  .next.current {
    background: var(--ink-3);
    box-shadow: inset 3px 0 var(--accent);
  }

  /* What is left of a clip that was taken out: the same row, in its place,
     saying what happened and offering it back. It slides in from the right
     the way the clip went, so the eye sees it leave rather than find a gap
     later. */
  .gone {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 56px;
    padding: 0 8px;
    background: rgba(224, 96, 90, 0.14);
    color: var(--err);
    animation: swept 0.18s ease-out;
  }

  .gone .what {
    font-weight: 600;
  }

  .gone .grow {
    flex: 1;
  }

  .back {
    color: var(--text);
  }

  @keyframes swept {
    from {
      transform: translateX(24px);
      opacity: 0;
    }
    to {
      transform: none;
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .gone {
      animation: none;
    }
  }

  .title {
    font-weight: 600;
    width: 100%;
    padding-right: 26px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  /* A card that is not there yet: the size of a clip, wearing the same
     shimmer as every other place in the app waiting to be filled. */
  .ghost {
    height: 56px;
    background: var(--ink-2);
  }

  /* A row still to come answers the pointer the way a clip's row does,
     a step brighter, because it is already a row of the list. The shimmer
     is the row's own breath and goes on under it, so the two are seen
     together. */
  .ghost:hover {
    background: var(--ink-3);
  }

  .next:hover {
    background: var(--ink-2);
  }

  /* The row the next clip will appear in, saying what it is waiting on,
     laid out as a clip's row is, a line of what and a line of how long. */
  .next {
    /* As tall as a clip, whatever it says, so a clip on its way and the
       clip it becomes are the same card. */
    height: 56px;
    /* Part of the row after it shows below it when it is brought into
       view: a third of a row, past the veil over the foot of the list. */
    scroll-margin-bottom: calc(var(--veil, 16px) + var(--gap) + 20px);
    /* And the clip that landed just above it, which is the one that was
       chosen, stays in view with it. */
    scroll-margin-top: calc(var(--veil, 16px) + var(--gap) + 56px);
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 2px;
    padding: 0 10px;
    background: var(--ink-1);
  }
  /* The last row has no row after it to show, so it asks for the veil
     only. Asking for a third of a row that is not there scrolled the list
     into its own padding as a clip landed, and the next change snapped it
     back, every card jumping at once. */
  .next:not(:has(+ li)) {
    scroll-margin-bottom: var(--veil, 16px);
  }


  /* A search that stopped before it was done: the same row, still, in the
     colour of a warning. Not the red of what was taken away, because
     nothing was lost: New looks again. */
  .stopped .title {
    color: var(--warn);
  }

  /* A clip on the way that stopped is carried on with a click, the one
     thing it offers, laid out as the row it would have been. */
  .carry {
    padding: 0;
  }

  .carry:hover {
    background: transparent;
  }

  /* One line, as tall as every other row, whatever the reason says. The
     whole of it is in the row's title. */
  .stopped .meta {
    display: block;
    width: 100%;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
</style>
