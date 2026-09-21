<script lang="ts">
  import { flip } from "svelte/animate";
  import { slide } from "svelte/transition";
  import { clock, type ClipEntry } from "../lib/api";

  import Icon from "./Icon.svelte";

  let {
    clips,
    selected,
    removed = "",
    coming = 0,
    onselect,
    onremove,
    onputback,
  }: {
    clips: ClipEntry[];
    selected: string;
    // How many clips are on the way: the number the search was asked for,
    // while the transcript is still coming or the search is running. That
    // many rows wait in place, so the list is already the shape it is
    // about to be, and the info mark at the head says what is going on.
    coming?: number;
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

  // A search hands in all its clips at once, so in practice this is either
  // all of them or none. Counting what is there anyway means a list that
  // is partly filled never shows more rows than the search will hold.
  const ghosts = $derived.by(() => {
    const n = Math.max(0, coming - clips.length);
    return Array.from({ length: n }, (_, i) => i);
  });
</script>

<ol>
  {#each clips as clip (clip.key)}
    <li animate:flip={{ duration: 180 }} out:slide={{ duration: 200 }}>
      {#if clip.key === removed}
        <div class="gone">
          <Icon name="trash" />
          <span class="what">Removed</span>
          <span class="grow"></span>
          <button class="quiet back" onclick={() => onputback?.()}>Put it back</button>
        </div>
      {:else}
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
       rows to twelve, and the light passing over them is the only thing
       that says it is still working. -->
  {#each ghosts as row (row)}
    <li class="ghost waiting"></li>
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
  li:hover .pick.current {
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

  /* A card that is not there yet: the size of a clip, with the same light
     passing over it as over the places under the video preview. */
  .ghost {
    height: 56px;
    background: var(--ink-2);
  }
</style>
