<!--
  The field a person types a search into: a magnifier, what is typed, and
  whatever the place adds after it. The font list has it over its rows,
  the transcript has it in the head of the clip column, so a change to how
  a search field looks is made here once and is true in both.

  `framed` gives it an edge of its own all round, lit in the accent while
  it has the keyboard, for a field that stands in a head among buttons.
  Without it the field is the top of a list and only a line parts it from
  the rows. What comes after the text, the count and the cross, are the
  children.
-->
<script lang="ts">
  import type { Snippet } from "svelte";
  import type { HTMLInputAttributes } from "svelte/elements";
  import Icon from "./Icon.svelte";

  let {
    value = $bindable(""),
    field = $bindable<HTMLInputElement>(),
    framed = false,
    children,
    ...rest
  }: Omit<HTMLInputAttributes, "value"> & {
    value?: string;
    field?: HTMLInputElement;
    framed?: boolean;
    children?: Snippet;
  } = $props();
</script>

<label class="search" class:framed>
  <Icon name="search" size={12} />
  <input {...rest} bind:this={field} bind:value />
  {@render children?.()}
</label>

<style>
  .search {
    flex: none;
    display: flex;
    align-items: center;
    gap: 8px;
    height: calc(var(--control-h) + 1px);
    padding: 0 8px;
    border-bottom: 1px solid var(--line);
    color: var(--muted);
    cursor: text;
  }

  /* In a head it is one control of the row, as tall as the others, and
     takes what the row leaves. */
  .framed {
    flex: 1;
    height: var(--control-h);
    padding: 0 0 0 var(--control-pad);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--ink-1);
  }

  .framed:focus-within {
    border-color: var(--accent);
  }

  /* A button inside sits within the edge, not over it. */
  .framed :global(.glyph) {
    height: calc(var(--control-h) - 2px);
  }

  /* It asks nothing of the width around it, which is the rows' or the
     head's to set. */
  input {
    flex: 1;
    width: 0;
    min-width: 0;
    height: 100%;
    padding: 0;
    color: var(--text);
    font: inherit;
    background: transparent;
    border: none;
    outline: none;
  }
</style>
