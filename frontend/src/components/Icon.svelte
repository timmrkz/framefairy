<script lang="ts">
  // The few glyphs the workspace uses, drawn in one place so they share a
  // weight and a size. They take the colour of the text around them.
  let { name, size = 16 }: { name: string; size?: number } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 16 16"
  fill="none"
  stroke="currentColor"
  stroke-width="1.5"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if name === "sidebar"}
    <rect x="2" y="3" width="12" height="10" rx="2" />
    <path d="M6.5 3v10" />
  {:else if name === "plus"}
    <rect x="2.5" y="2.5" width="11" height="11" rx="2.5" />
    <path d="M8 5.5v5M5.5 8h5" />
  {:else if name === "folder"}
    <path d="M1.8 12.6V3.4h4l1.4 1.8h7v7.4a1 1 0 0 1-1 1H2.8a1 1 0 0 1-1-1Z" />
  {:else if name === "trash"}
    <path d="M2.8 4.3h10.4M6.4 4.3V2.9h3.2v1.4M4.6 4.3l.5 8.8h5.8l.5-8.8" />
  {:else if name === "info"}
    <circle cx="8" cy="8" r="6.2" />
    <path d="M8 7.3v4M8 4.9v.8" />
  {:else if name === "activity"}
    <path d="M1.5 8h3l2-4.5 2.6 9L11.3 8h3.2" />
  {:else if name === "close"}
    <path d="M4 4l8 8M12 4l-8 8" />
  {:else if name === "pause"}
    <path d="M6 3.4v9.2M10 3.4v9.2" />
  {:else if name === "play"}
    <path d="M5.2 3.3 12.4 8l-7.2 4.7Z" fill="currentColor" />
  <!-- The loop as every editor draws it: along the top to an arrow on the
       right, back along the bottom to an arrow on the left. -->
  {:else if name === "loop"}
    <path d="M11.33 0.67 14 3.33l-2.67 2.67" />
    <path d="M2 7.33V6a2.67 2.67 0 0 1 2.67-2.67H14" />
    <path d="M4.67 15.33 2 12.67l2.67-2.67" />
    <path d="M14 8.67V10a2.67 2.67 0 0 1-2.67 2.67H2" />
  {:else if name === "locate"}
    <circle cx="8" cy="8" r="3.6" />
    <path d="M8 1.2v2.2M8 12.6v2.2M1.2 8h2.2M12.6 8h2.2" />
  <!-- The way back turns anticlockwise, the way there again turns
       clockwise. One undoes, the other redoes, and which is which has to be
       readable without the tooltip. -->
  {:else if name === "back"}
    <path d="M2.6 8a5.4 5.4 0 1 0 1.7-3.9" />
    <path d="M3.7 1.6v3.2h3.2" />
  {:else if name === "forward"}
    <path d="M13.4 8a5.4 5.4 0 1 1-1.7-3.9" />
    <path d="M12.3 1.6v3.2H9.1" />
  <!-- The mark on a list you pick from, the pair of arrows macOS draws on
       one, so the field says it opens a list without the system drawing it
       in its own place. -->
  {:else if name === "pick"}
    <path d="M5 6.3 8 3.4l3 2.9" />
    <path d="M5 9.7 8 12.6l3-2.9" />
  <!-- Done. Beside a speech model that is on the machine, and anywhere
       else something has arrived. -->
  {:else if name === "check"}
    <path d="M3.2 8.4 6.5 11.7 12.8 4.9" />
  {:else if name === "sliders"}
    <path d="M2 4.5h12M2 11.5h12" />
    <circle cx="5.5" cy="4.5" r="1.9" fill="var(--ink-1)" />
    <circle cx="10.5" cy="11.5" r="1.9" fill="var(--ink-1)" />
  {/if}
</svg>

<style>
  svg {
    flex: none;
    display: block;
  }
</style>
