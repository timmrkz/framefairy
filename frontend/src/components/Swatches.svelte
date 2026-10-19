<script lang="ts">
  // The accent colours as a row of round colours with the colour well at
  // its end, the way the Mac sets its accent colour. It is the accent in
  // the settings and the colour of a video's label in the sidebar: one row
  // of colours in two places, so they are the same colours and look the
  // same, and a change to either is made here.
  import Colour from "./Colour.svelte";
  import { accentColours } from "../lib/colour";

  let {
    value,
    label,
    oninput = undefined,
    onpick,
    onnone = undefined,
    hang = "start",
  }: {
    // The colour in use, as #rrggbb.
    value: string;
    // What the colours are for, said out loud for anything reading the
    // screen.
    label: string;
    // A colour on its way in the well, while the hand still moves.
    oninput?: (hex: string) => void;
    // A colour picked, to be saved.
    onpick: (hex: string) => void;
    // Escape in the colour well put back no colour at all.
    onnone?: () => void;
    // Which edge of the well its pop-up hangs on, see Colour.svelte.
    hang?: "start" | "end";
  } = $props();

  // The well holds the colour in use when it is none of the round ones. A
  // label with no colour yet has none in use, and the well is not lit.
  const custom = $derived(!!value && !accentColours.some((c) => c.hex === value.toLowerCase()));
</script>

<div class="swatches" role="radiogroup" aria-label={label}>
  {#each accentColours as c (c.hex)}
    {@const on = value?.toLowerCase() === c.hex}
    <button
      class="swatch"
      class:on
      role="radio"
      aria-checked={on}
      aria-label={c.name}
      title={c.name}
      style="--swatch: {c.hex}"
      onclick={() => onpick(c.hex)}
    ></button>
  {/each}
  <!-- Any other colour, in the app's own colour picker. The round colours
       before it are its presets, so it offers none of its own. -->
  <Colour
    face="well"
    on={custom}
    label="Another colour"
    title="Another colour"
    {hang}
    {value}
    oninput={(hex) => hex && oninput?.(hex)}
    onchange={onpick}
    {onnone}
  />
</div>

<style>
  .swatches {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: none;
  }

  /* The colours, round, the way the Mac shows them. The chosen one
     carries a ring in its own colour a step out from it. */
  .swatch {
    position: relative;
    width: 20px;
    height: 20px;
    padding: 0;
    border-radius: 50%;
    border: none;
    background: var(--swatch);
    flex: none;
  }

  .swatch:hover:not(:disabled) {
    background: var(--swatch);
    filter: brightness(1.15);
  }

  .swatch.on {
    box-shadow:
      0 0 0 2px var(--ink-1),
      0 0 0 4px var(--swatch, var(--accent));
  }
</style>
