<script lang="ts">
  import { flip } from "svelte/animate";
  import { fade } from "svelte/transition";
  import Busy from "./Busy.svelte";
  import Icon from "./Icon.svelte";
  import { toasts, type Toast } from "../lib/toast.svelte";

  // The toasts, in the top right corner of the window they are about,
  // under its bar, the corner macOS brings its own notes in at. They lie
  // over whatever is there and move nothing, so the workspace stands where
  // it stood when one comes and goes. Plan row 2.268, see toast.svelte.ts.
  //
  // A toast stays until it is read. The pointer on it reads it: from then
  // on its time runs down in the fill a removed clip's runs down in,
  // standing still while the pointer is on it, and it goes when the time
  // is over. The cross closes it at once. Before the pointer has been on
  // it, nothing takes it away but the place it came from, so a toast that
  // came while the person looked elsewhere is still there when they look.
  //
  // shown says which toasts belong in this window now: a toast about a
  // video waits while another video is on screen.
  let { shown = () => true }: { shown?: (t: Toast) => boolean } = $props();

  // How long a toast stays once it has been read and the pointer has
  // left it, in seconds.
  const afterRead = 6;

  let read = $state<Record<number, boolean>>({});
  const list = $derived(toasts.list.filter((t) => shown(t)));
</script>

<div class="toasts">
  {#each list as t (t.id)}
    <div
      class="toast busyhost"
      role="alert"
      data-id={t.id}
      onpointerenter={() => (read[t.id] = true)}
      animate:flip={{ duration: 150 }}
      out:fade={{ duration: 150 }}
    >
      <span class="mark"><Icon name="warn" /></span>
      <p class="text selectable">{t.text}</p>
      <button class="quiet bin close" aria-label="Close" title="Close" onclick={() => toasts.close(t.id, "closed")}>
        <Icon name="close" />
      </button>
      {#if read[t.id]}
        <Busy drain={afterRead} onend={() => toasts.close(t.id, "read")} />
      {/if}
    </div>
  {/each}
</div>

<style>
  /* Out of the flow and over everything but a box that asks first, which
     the browser puts on a layer above the page. Whole pixels from the
     corner, so a toast fading in does not step. */
  .toasts {
    position: fixed;
    top: calc(var(--bar-h) + var(--gap));
    right: var(--gap);
    z-index: 50;
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: min(340px, calc(100vw - 2 * var(--gap)));
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 6px 6px 6px 12px;
    border: 1px solid color-mix(in srgb, var(--err) 45%, var(--line));
    border-radius: var(--radius-m);
    background: var(--ink-2);
    color: var(--text);
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
    pointer-events: auto;
    animation: arrive 0.18s ease-out;
    /* The time running down wears the colour of what went wrong, the way
       a removed row's does. */
    --lit: var(--err);
    --wash-from: color-mix(in srgb, var(--err) 10%, transparent);
    --wash-to: color-mix(in srgb, var(--err) 20%, transparent);
  }

  /* Whole lines, beside a button the height of every control. */
  .mark {
    display: flex;
    align-items: center;
    height: var(--control-h);
    color: var(--err);
  }

  .text {
    flex: 1;
    min-width: 0;
    margin: 0;
    padding: 6px 0;
    line-height: 20px;
    overflow-wrap: anywhere;
  }

  .close {
    color: var(--muted);
  }

  @keyframes arrive {
    from {
      opacity: 0;
      transform: translateY(-8px);
    }
  }
</style>
