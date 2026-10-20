<script lang="ts">
  import { flip } from "svelte/animate";
  import { fade } from "svelte/transition";
  import Busy from "./Busy.svelte";
  import Icon from "./Icon.svelte";
  import { secondThoughts } from "../lib/removed";
  import { toasts, type Toast } from "../lib/toast.svelte";
  import { api } from "../lib/api";

  // The toasts, in the top right corner of the window they are about,
  // under its bar, the corner macOS brings its own notes in at. They lie
  // over whatever is there and move nothing, so the workspace stands where
  // it stood when one comes and goes. Issue #327, see toast.svelte.ts.
  //
  // Its time runs down from the moment it comes, in the fill a removed
  // clip's runs down in, and as long, the way a toast does everywhere
  // else. It once had eight seconds of its own, and Tim asked why it was
  // not the ten everything else that runs out has. It stands
  // still while the pointer is on it, so a toast being read is not taken
  // away, and the cross closes it at once. It once waited for the pointer
  // before its time began, and Tim found a toast that never went.
  //
  // shown says which toasts belong in this window now: a toast about a
  // video waits while another video is on screen.
  let { shown = () => true }: { shown?: (t: Toast) => boolean } = $props();

  const list = $derived(toasts.list.filter((t) => shown(t)));
</script>

<div class="toasts">
  {#each list as t (t.id)}
    <div
      class="toast busyhost"
      role="alert"
      data-id={t.id}
      animate:flip={{ duration: 150 }}
      out:fade={{ duration: 150 }}
    >
      <span class="mark"><Icon name="warn" /></span>
      <div class="body">
        <p class="text selectable">{t.text}</p>
        {#if t.report}
          <!-- Report a Problem, with the toast's words already written in
               it, for a failure the person cannot put right alone. -->
          <button
            class="report"
            title="Opens Report a Problem with this message written in it"
            onclick={() => {
              api.reportAbout(t.text).catch(() => {});
              toasts.close(t.id, "closed");
            }}>Report</button
          >
        {/if}
      </div>
      <button class="quiet bin close" aria-label="Close" title="Close" onclick={() => toasts.close(t.id, "closed")}>
        <Icon name="close" />
      </button>
      <Busy drain={secondThoughts} onend={() => toasts.close(t.id, "ran out")} />
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
    /* As wide as a short reason with a file's name in it on two lines.
       340 px was a little too wide for Tim. One width for every toast, so
       a stack reads as one column. */
    width: min(300px, calc(100vw - 2 * var(--gap)));
    pointer-events: none;
  }

  /* The red of what went wrong laid over the dark of the app, the way a
     removed row wears it, and no line round it. A grey note with a thin
     red line was a look the app has nowhere else, and Tim found it wrong.
     Solid, since it lies over whatever is under it. */
  .toast {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 6px 6px 6px 12px;
    border-radius: var(--radius-m);
    background: color-mix(in srgb, var(--err) 16%, var(--ink-1));
    color: color-mix(in srgb, var(--err) 30%, var(--text));
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

  .body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
  }

  /* Under the words, where a note on macOS has its button, and as wide
     as its word. */
  .report {
    margin-bottom: 4px;
  }

  .text {
    min-width: 0;
    margin: 0;
    padding: 6px 0;
    line-height: 20px;
    overflow-wrap: anywhere;
  }

  .close {
    color: inherit;
  }

  @keyframes arrive {
    from {
      opacity: 0;
      transform: translateY(-8px);
    }
  }
</style>
