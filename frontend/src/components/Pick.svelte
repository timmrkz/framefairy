<script lang="ts">
  // A list to pick from, wearing the app's own clothes.
  //
  // A plain <select> is drawn by the system: on macOS the webview hands the
  // whole list to AppKit, which paints it white with a blue row and a tick,
  // in a box that has nothing to do with the dark workspace around it. The
  // one thing a stylesheet cannot reach was the one thing that looked
  // wrong.
  //
  // So the list is the app's own, built on bits-ui, which is the headless
  // half of what shadcn-svelte is made of: it brings the keyboard, the
  // roles, the focus, the typeahead and the floating placement, and brings
  // no look at all. Everything seen here is this file and the tokens in
  // app.css.
  //
  // This is the one list to pick from in the app. A second one would be a
  // second way of doing the same thing, which is the difference between two
  // controls that match and two that nearly match.
  //
  // Every element is drawn here rather than left to bits-ui, through the
  // child snippets it offers for exactly this. A class handed to a
  // component is a string and not an attribute, so the stylesheet would
  // have had to reach it with :global, and a global name is a name the
  // whole app has to keep clear: the clip list already has a pick of its
  // own, and two rules under one name is how the title bar once ended up
  // wearing the progress track's rounded corners.
  import { Select } from "bits-ui";
  import Icon from "./Icon.svelte";

  let {
    value = "",
    options,
    onpick,
    label,
    title = "",
    id = undefined,
    // Which way the trigger reads. The settings column ends every row in
    // the same place, so the face ends where the numbers do, on the right.
    // Everywhere else a field reads from the left.
    align = "left",
    disabled = false,
  }: {
    // What is picked. It goes one way only, and nothing here ever writes
    // it back: what the trigger says is what the caller says is true, and
    // that is the whole point. A list that shows what was clicked is a
    // list that lies for as long as the answer is no, and it was lying
    // before this was written: refusing to save a caption face left the
    // trigger naming a face the engine had never taken, and it would have
    // gone on naming it until the face really changed.
    value?: string;
    options: { value: string; label: string }[];
    // What to do with a pick. A caller that keeps the value itself sets it
    // here, and a caller that sends it to the engine draws whatever comes
    // back. Either way the trigger only ever says what came back.
    onpick?: (value: string) => void;
    // What the list is for, said out loud for anything reading the screen.
    label: string;
    title?: string;
    id?: string;
    align?: "left" | "right";
    disabled?: boolean;
  } = $props();

  // What the trigger says. A value that is not in the list yet, which is
  // what the moment between opening an episode and its fonts arriving looks
  // like, still reads as itself rather than as nothing.
  const shown = $derived(options.find((o) => o.value === value)?.label ?? value);

  // The longest thing the list can ever say. The trigger keeps room for it
  // whatever is picked, so the row does not change width as it is used,
  // exactly as a button that says Play and Pause keeps room for the longer
  // of the two. Measuring text is the stylesheet's work, so the longest
  // label is drawn and hidden rather than measured in JavaScript.
  const longest = $derived(
    options.reduce((most, o) => (o.label.length > most.length ? o.label : most), ""),
  );

  // Which row the list itself thinks is ticked. It is not the same thing
  // as what is true: between a click and an answer the list has moved on
  // and the answer may yet be no. So it follows the truth whenever the
  // truth changes, and it is set back to the truth every time the list
  // opens, which is the only moment it is ever read.
  // It starts empty rather than at the value, because reading a prop once
  // at the top is reading the value it had at that moment and nothing
  // after. The two lines below are the whole of how it is kept: it follows
  // the truth whenever the truth changes, and it is set back to the truth
  // every time the list opens.
  let ticked = $state("");
  $effect(() => {
    ticked = value;
  });
</script>

<Select.Root
  type="single"
  bind:value={ticked}
  items={options}
  {disabled}
  onOpenChange={(open) => {
    if (open) ticked = value;
  }}
  onValueChange={(v) => onpick?.(v)}
>
  <Select.Trigger>
    {#snippet child({ props })}
      <button
        {...props}
        class="pick"
        class:right={align === "right"}
        aria-label={label}
        {title}
        {id}
      >
        <span class="said">
          <span class="room" aria-hidden="true">{longest}</span>
          <span>{shown}</span>
        </span>
        <span class="mark"><Icon name="pick" size={12} /></span>
      </button>
    {/snippet}
  </Select.Trigger>
  <Select.Portal>
    <!-- Right under the trigger and along its left edge, so the list opens
         where the eye already is. -->
    <Select.Content sideOffset={4} align={align === "right" ? "end" : "start"}>
      {#snippet child({ props, wrapperProps, open })}
        {#if open}
          <div {...wrapperProps}>
            <div {...props} class="pick-list">
              {#each options as option (option.value)}
                <Select.Item value={option.value} label={option.label}>
                  {#snippet child({ props: row, selected })}
                    <!-- Read from the same side the trigger reads from,
                         and the tick on that side too, so a name in the
                         list starts where the name on the trigger starts
                         and the eye runs down one edge. -->
                    <div
                      {...row}
                      class="pick-row"
                      class:right={align === "right"}
                      title={option.label}
                    >
                      {#if align === "right"}
                        <span class="what">{option.label}</span>
                        <span class="tick">
                          {#if selected}<Icon name="check" size={12} />{/if}
                        </span>
                      {:else}
                        <span class="tick">
                          {#if selected}<Icon name="check" size={12} />{/if}
                        </span>
                        <span class="what">{option.label}</span>
                      {/if}
                    </div>
                  {/snippet}
                </Select.Item>
              {/each}
            </div>
          </div>
        {/if}
      {/snippet}
    </Select.Content>
  </Select.Portal>
</Select.Root>

<style>
  /* The trigger is a button, so it already has the height, the border, the
     background and the radius every control in the app has, and it hovers
     and dims the way they all do. All it adds is a row: what is picked, and
     the mark that says this is a list. The right padding comes in because
     the mark is narrower than a letter is. */
  .pick {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding-right: 8px;
    color: var(--text);
    text-align: left;
  }

  /* The same ring the rest of the app gives whatever has the keyboard,
     drawn inside the box rather than around it so a field in a column does
     not grow by two pixels when it is tabbed to. */
  .pick:focus-visible {
    outline: 2px solid var(--accent-hi);
    outline-offset: -1px;
  }

  /* And no ring at all while the list is open. The keyboard never leaves
     the trigger, so the ring stayed lit under an open list and said a
     second time, louder, what the lit row in the list was already saying.
     Two marks for one thing, and the bigger one on the thing that is not
     being walked.

     This is every list in the app at once, because there is only one list
     in the app. */
  .pick[aria-expanded="true"]:focus-visible {
    outline: none;
  }

  /* The two lie on top of each other, so the box is as wide as the longest
     thing the list can say and what is picked is drawn in it. */
  .said {
    display: grid;
    flex: 1;
    min-width: 0;
  }

  .said > span {
    grid-area: 1 / 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .room {
    visibility: hidden;
  }

  /* Ending the row on the right is what the settings column asks for, and
     the mark stays where it is: it says what the field is, so it belongs
     with the units above and below it, not against the text. */
  .pick.right .said {
    text-align: right;
  }

  /* The mark is the colour of a unit, not of the text, because it says what
     the field is rather than what it holds. */
  .mark {
    display: flex;
    align-items: center;
    flex: none;
    color: var(--muted);
  }

  /* The list itself. It is the panel colour rather than the field colour,
     because it lies over the workspace rather than sitting in it.

     **It is as wide as what it holds, and it grows away from the edge it
     is hung on.** A name is read in the list, so the list is never the
     reason a name is cut short.

     Which is the width it is measured at, and that is the whole of why
     this works. The placement decides which way a list grows from the
     width it measures, and it measures before the stylesheet has run its
     rules. Saying `width: var(--bits-floating-anchor-width)` and then
     `min-width: max-content` is measure first and widen after: the
     placement worked out where the trigger's right edge put a list of the
     trigger's width, the list then came out wider, and every pixel of the
     difference went out to the right, over the edge every field in that
     column ends on. `max-content` is the width before anything is
     measured, so the placement measures what it will get and hangs the
     right edge where it was told.

     The minimum keeps a list of short names from standing narrower than
     the trigger it belongs to. It is the one number the stylesheet cannot
     work out for itself, so bits-ui measures the trigger and hands it over
     as a custom property. It only ever applies where the names are
     shorter than the trigger, where the difference is a pixel or two.

     The most is what the placement says there is room for, so a name
     longer than the room that is left cannot push the list off the side
     of the app. That is the one case a name is still cut short, and the
     whole of it is in the row's title. */
  .pick-list {
    z-index: 60;
    width: max-content;
    min-width: var(--bits-floating-anchor-width);
    max-width: calc(var(--bits-floating-available-width, 100vw) - 8px);
    max-height: 320px;
    /* Nothing at the sides. A row carries the whole inset by itself, so
       the list's own padding cannot be added on top of it: the trigger
       ends its name 29 pixels short of its right edge, being 8 of padding,
       a 12 wide mark and the 8 between them, and a row ends its name the
       same 29 short of the list's, being the border, the same 8, the same
       12 and the same 8. The list stands on the trigger's right edge, so
       the two names end in one line. With 4 pixels of padding here as
       well they were out by exactly that. */
    padding: 4px 0;
    overflow-y: auto;
    color: var(--text);
    background: var(--ink-1);
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.55);
  }

  .pick-row {
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--control-h);
    padding: 0 8px;
    border-radius: var(--radius-s);
    cursor: pointer;
    user-select: none;
  }

  /* The row under the pointer and the row the keyboard is on are one thing,
     so they look like one thing. bits-ui calls it highlighted and marks it
     on the element, which is the only way the two can never disagree: a
     hover of its own would light a second row the moment the hand moved
     while the arrow keys were walking the list. */
  .pick-row[data-highlighted] {
    background: var(--ink-3);
  }

  /* What is chosen is in the accent, the way a chosen row is everywhere
     else in the app. */
  .pick-row[data-selected] {
    color: var(--accent-lit);
  }

  .pick-row[data-disabled] {
    color: var(--muted);
    cursor: default;
  }

  /* The tick keeps its place whether or not it is there, so no row moves
     sideways as the chosen one changes. */
  .tick {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 12px;
    color: var(--accent-lit);
  }

  .what {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .pick-row.right .what {
    text-align: right;
  }
</style>
