<script lang="ts">
  // A small mark that explains one thing when you want it explained. The
  // workspace says what a control is for here, instead of keeping a line of
  // help on screen at all times.
  import type { Snippet } from "svelte";
  import Icon from "./Icon.svelte";

  let {
    label,
    side = "left",
    children,
  }: {
    label: string;
    // Which side of the mark the bubble grows from, so it stays on screen.
    side?: "left" | "right";
    children: Snippet;
  } = $props();

  let open = $state(false);
  let nearMark = $state(false);
  let nearBubble = $state(false);
  let root: HTMLSpanElement;
  let bubble = $state<HTMLSpanElement>();
  // The bubble is only in the page while it is up. Hidden, it would still
  // count towards how tall the workspace is, and the workspace is meant to
  // fit the window.
  const shown = $derived(open || nearMark || nearBubble);

  // The bubble hangs from the window itself rather than from the mark. An
  // info mark sits in the top right corner of the area it explains, and
  // those areas clip what is inside them, so a bubble that stayed there
  // would be cut off at the edge of a track only a few pixels tall. Out
  // here nothing can clip it and nothing can lie over it.
  function loose(node: HTMLElement) {
    document.body.appendChild(node);
    return {
      destroy() {
        node.remove();
      },
    };
  }

  // Where it ended up having to go: under the mark, above it when there is
  // no room below, and moved sideways when it would leave the window. A
  // bubble that runs off the screen is a bubble nobody can read. It stays
  // out of sight until it has been measured and put somewhere, so it never
  // shows in the corner of the window first.
  let at = $state<{ top: number; left: number } | null>(null);
  let above = $state(false);

  $effect(() => {
    if (!shown || !bubble) {
      at = null;
      return;
    }
    const gap = 6;
    const edge = 8;
    const mark = root.getBoundingClientRect();
    const box = bubble.getBoundingClientRect();
    let top = mark.bottom + gap;
    above = top + box.height > window.innerHeight - edge && mark.top - box.height - gap > edge;
    if (above) top = mark.top - box.height - gap;
    let left = side === "right" ? mark.right + 8 - box.width : mark.left - 8;
    left = Math.min(left, window.innerWidth - edge - box.width);
    at = { top: Math.max(edge, top), left: Math.max(edge, left) };
  });

  // Opened by a click, it stays until you click somewhere else or press
  // Escape. The text inside can be read and copied for as long as it does,
  // so a click in the bubble is not a click somewhere else.
  $effect(() => {
    if (!open) return;
    const away = (event: PointerEvent) => {
      const on = event.target as Node;
      if (!root.contains(on) && !bubble?.contains(on)) open = false;
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") open = false;
    };
    window.addEventListener("pointerdown", away);
    window.addEventListener("keydown", key);
    return () => {
      window.removeEventListener("pointerdown", away);
      window.removeEventListener("keydown", key);
    };
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<span
  class="info"
  bind:this={root}
  onpointerenter={() => (nearMark = true)}
  onpointerleave={() => (nearMark = false)}
>
  <button class="mark" aria-label={label} aria-expanded={shown} onclick={() => (open = !open)}>
    <Icon name="info" size={15} />
  </button>
</span>
{#if shown}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <span
    class="bubble selectable"
    class:placed={!!at}
    class:above
    bind:this={bubble}
    use:loose
    style="top: {at?.top ?? 0}px; left: {at?.left ?? 0}px"
    onpointerenter={() => (nearBubble = true)}
    onpointerleave={() => (nearBubble = false)}
  >{@render children()}</span>
{/if}

<style>
  /* Exactly as tall as the mark in it, rather than as tall as the line of
     text it stands in. A box of 23 and a third pixels centred in a row
     puts the mark between two pixels, and a mark between two pixels is
     painted in one place while it fades and another once it is there. */
  .info {
    position: relative;
    display: inline-flex;
    align-items: center;
    height: 20px;
  }

  .mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: transparent;
    color: var(--muted);
  }

  .mark:hover:not(:disabled),
  .info:hover .mark {
    background: transparent;
    color: var(--text);
  }

  /* Over everything, because it is the answer to a question just asked.
     The gap between the mark and the bubble belongs to the bubble, so
     moving the pointer into it does not lose the hover. */
  .bubble {
    position: fixed;
    z-index: 200;
    width: 260px;
    padding: 8px 10px;
    background: var(--ink-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    box-shadow: 0 6px 18px rgba(0, 0, 0, 0.5);
    color: var(--text);
    font-size: var(--size-s);
    line-height: 1.5;
    text-align: left;
    white-space: normal;
    cursor: text;
    max-height: calc(100vh - 16px);
    overflow: auto;
    visibility: hidden;
  }

  .bubble.placed {
    visibility: visible;
  }

  .bubble::before {
    content: "";
    position: absolute;
    left: 0;
    right: 0;
    top: -8px;
    height: 8px;
  }

  .bubble.above::before {
    top: auto;
    bottom: -8px;
  }

</style>
