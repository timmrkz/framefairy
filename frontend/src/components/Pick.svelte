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
  import { Command, Popover, Select } from "bits-ui";
  import Icon from "./Icon.svelte";
  import SearchField from "./SearchField.svelte";
  import { matches } from "../lib/match";
  import { said } from "../lib/said";
  import { tick } from "svelte";

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
    // How what is picked stands, where that matters: a choice that cannot
    // work yet wears a frame in the colour of a warning, the way a field
    // that needs something does, so it is seen on the control itself.
    tone = undefined,
    // What the trigger shows: the name of what is picked, or only its icon,
    // for a list in a place with no room for words, the way a toolbar on the
    // Mac shows a sort menu as an icon. The rows still say their names.
    face = "text",
    // The icon an icon face always wears, whatever is picked, for a list
    // whose trigger says what it does rather than what it holds. Without
    // it the face shows the icon of what is picked.
    icon = undefined,
    // Told when the list opens and closes. The list lies over the app,
    // outside whatever the trigger stands in, so a sidebar that closes when
    // the pointer leaves it has to be told the pointer is still its own.
    onopenchange = undefined,
    // Asked as the list is about to open. A choice that may not be changed
    // now says yes, and the list stays shut: the caller shows why, the way
    // the settings shake the card a running search is using.
    refuse = undefined,
    // What the trigger says while nothing in the list is picked. It is a
    // state, not a row: a list never offers nothing as a choice.
    placeholder = "",
    // A trigger as wide as what it says now, rather than as wide as the
    // longest thing it could say. For a list that stands alone at the end
    // of a row, where nothing beside it moves when it changes width, and
    // whose longest name would leave the short ones in a wide empty box:
    // the channel on the Updates page, where PR #175 sat in a box made for
    // PR #20, conflicts.
    fit = false,
    // A list long enough to search, which says this in its search field.
    // It is the same list with a field over its rows that narrows them as
    // it is typed in, by the letters in order rather than the whole word,
    // so amtyp finds American Typewriter. bits-ui brings it too, as the
    // command list shadcn-svelte builds its combobox from, with the
    // scoring the command bars of most apps use.
    search = undefined,
    // Which edge of the trigger the list hangs on. It is the side the
    // trigger reads from, unless the list is wider than the room on that
    // side, which a list of fonts each drawn in its own face is in the
    // settings column: the placement pushed it back from the app's edge,
    // and then it lined up with nothing. So it hangs on the left edge and
    // grows into the workspace, where there is room.
    hang = undefined,
  }: {
    // What is picked. It goes one way only, and nothing here ever writes
    // it back: what the trigger says is what the caller says is true, and
    // that is the whole point. A list that shows what was clicked is a
    // list that lies for as long as the answer is no, and it was lying
    // before this was written: refusing to save a caption face left the
    // trigger naming a face the engine had never taken, and it would have
    // gone on naming it until the face really changed.
    value?: string;
    // A row may say a word beside its name, what it would cost or that it
    // will not run here, and rows may stand in groups under a head, the way
    // a pop-up menu on the Mac puts a separator and a heading between kinds
    // of thing. A group is named on its first row and runs until the next.
    // A row may also be drawn in a face of its own, a font in the list of
    // fonts, at the weight that face has.
    options: {
      value: string;
      label: string;
      detail?: string;
      warn?: boolean;
      group?: string;
      icon?: string;
      font?: string;
      weight?: number;
    }[];
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
    tone?: "warn" | "err";
    face?: "text" | "icon";
    icon?: string;
    onopenchange?: (open: boolean) => void;
    refuse?: () => boolean;
    placeholder?: string;
    fit?: boolean;
    search?: string;
    hang?: "left" | "right";
  } = $props();

  const edge = $derived((hang ?? align) === "right" ? "end" : "start");

  // How far under its field a list hangs. The fields of a column stand 8
  // apart, and a list is its rows and a border above and under them, so a
  // list of one row hung 6 under its field lies exactly over the field
  // under that one. Hung 4 under, it stopped 2 short of that field's
  // bottom, and its border showed under the list's own.
  const drop = 6;

  const picked = $derived(options.find((o) => o.value === value));

  // What the trigger says. A value that is not in the list yet, which is
  // what the moment between opening an episode and its fonts arriving looks
  // like, still reads as itself rather than as nothing.
  // With nothing picked it says the placeholder, in the colour of a unit,
  // the way an empty field shows its own.
  const shown = $derived(options.find((o) => o.value === value)?.label ?? (value || placeholder));
  const unpicked = $derived(!value && !!placeholder);

  // The longest thing the list can ever say. The trigger keeps room for it
  // whatever is picked, so the row does not change width as it is used,
  // exactly as a button that says Play and Pause keeps room for the longer
  // of the two. Measuring text is the stylesheet's work, so the longest
  // label is drawn and hidden rather than measured in JavaScript.
  const longest = $derived(
    options.reduce((most, o) => (o.label.length > most.length ? o.label : most), placeholder),
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
  // A list whose rows say something beside their names reads from the
  // left whatever side it hangs on, the way a pop-up menu on the Mac puts
  // names down the left and what goes with them down the right. Names that
  // end in one line with the trigger's only work while a name is all a row
  // holds.
  const columns = $derived(options.some((o) => o.detail || o.group || o.icon));
  // And a list hung on the left edge reads from the left, whatever the
  // trigger does, so its names start in one line with the edge.
  const fromRight = $derived(align === "right" && edge === "end" && !columns);

  let ticked = $state("");
  $effect(() => {
    ticked = value;
  });

  // Whether the list is open. bits-ui sets it and then says it changed, so
  // a list refused here is still shut when it says so.
  let open = $state(false);
  function opens(next: boolean) {
    if (next && refuse?.()) return;
    open = next;
  }

  // What is typed in the search field, and the row it lights, which starts
  // on what is picked so the arrow keys walk on from there.
  let typed = $state("");
  let lit = $state("");

  // The rows of a list to search that answer what is typed, in their
  // groups and in the order of the whole list, a group being the rows from
  // where it is named until the next. They are never sorted by how well
  // they answer: the command list did that, and with it moved a font of
  // the app's under the head of the installed ones, see matches.
  const sections = $derived.by(() => {
    const out: { group?: string; options: typeof options }[] = [];
    for (const option of options) {
      if (!matches(option.label, typed)) continue;
      const last = out[out.length - 1];
      if (last && last.group === option.group) last.options.push(option);
      else out.push({ group: option.group, options: [option] });
    }
    return out;
  });

  // The lit row stays on a row that is still there as the rows narrow,
  // and goes to the first that is when it is not.
  $effect(() => {
    const rows = sections.flatMap((section) => section.options);
    if (rows.length && !rows.some((option) => option.value === lit)) lit = rows[0].value;
  });

  // The picked row stands in the middle of the list as it opens, the way a
  // pop-up menu on the Mac opens on what is chosen. The list is scrolled
  // and nothing else, so the app behind it does not move.
  function onPicked(list: HTMLElement) {
    const row = list.querySelector<HTMLElement>("[data-chosen]");
    if (row) list.scrollTop = row.offsetTop - (list.clientHeight - row.offsetHeight) / 2;
  }

  // Escape closes the list and lets go of the keyboard, so nothing is left
  // lit and the space bar plays again. The two kinds of list closed
  // differently: a list to search has the keyboard in its field and gave
  // it back to the trigger by script, which the webview leaves unlit,
  // while a plain list never takes it from the trigger, so the Escape was
  // a key pressed on the trigger and lit its ring, issue #370. A field
  // lets go on Escape the same way, see escapeLetsGo.
  let button = $state<HTMLButtonElement>();
  let escaped = false;
  function letGo() {
    if (!escaped) return;
    escaped = false;
    if (document.activeElement === button) button?.blur();
    said(`pick: Escape closed ${label} and let go of it`);
  }

  // A press anywhere else closes the list at once and leaves the keyboard
  // to what was pressed, the way a menu or a popover on the Mac goes when
  // the hand goes elsewhere. The list's own close waits a moment after the
  // press, and when the press moves the workspace, the transcript button
  // opening its search for one, the list is laid out anew in that moment
  // and forgets the press: it stayed open with the keyboard in its field,
  // found by Tim. Nor does the keyboard go back to the trigger after it.
  let pressedAway = false;
  $effect(() => {
    if (!open) return;
    const press = (e: PointerEvent) => {
      const at = e.target as Element | null;
      if (!at?.closest || at.closest(".pick-list") || button?.contains(at)) return;
      // Only a list to search takes the keyboard, and would give it back.
      pressedAway = search !== undefined;
      open = false;
      onopenchange?.(false);
      said(`pick: a press elsewhere closed ${label}`);
    };
    document.addEventListener("pointerdown", press, true);
    return () => document.removeEventListener("pointerdown", press, true);
  });
  function stayAway(e: Event) {
    if (!pressedAway) return false;
    pressedAway = false;
    e.preventDefault();
    return true;
  }

  function choose(next: string) {
    open = false;
    onopenchange?.(false);
    if (next !== value) onpick?.(next);
  }
</script>

{#snippet trigger(props: Record<string, unknown>)}
  <button
    {...props}
    bind:this={button}
    class="pick"
    class:face={face === "icon"}
    class:quiet={face === "icon"}
    class:right={align === "right"}
    class:warn={tone === "warn"}
    class:err={tone === "err"}
    aria-label={label}
    {title}
    {id}
  >
    {#if face === "icon"}
      <Icon name={icon ?? picked?.icon ?? "pick"} />
    {:else}
      <span class="said">
        {#if !fit}<span class="room" aria-hidden="true">{longest}</span>{/if}
        <span
          class:unpicked
          class:shown={!!picked?.font}
          style:font-family={picked?.font}
          style:font-weight={picked?.font ? picked.weight : undefined}>{shown}</span
        >
      </span>
      <span class="mark"><Icon name="pick" size={12} /></span>
    {/if}
  </button>
{/snippet}

<!-- A row read from the left: the tick, the icon, the name and what goes
     with it. The name is drawn in its own face where it has one. -->
{#snippet row(option: (typeof options)[number], selected: boolean)}
  <span class="tick">
    {#if selected}<Icon name="check" size={12} />{/if}
  </span>
  {#if option.icon}
    <span class="glyph"><Icon name={option.icon} /></span>
  {/if}
  <span
    class="what"
    class:shown={!!option.font}
    style:font-family={option.font}
    style:font-weight={option.font ? option.weight : undefined}>{option.label}</span
  >
  {#if option.detail}
    <span class="detail" class:warn={option.warn}>{option.detail}</span>
  {/if}
{/snippet}

{#if search !== undefined}
  <Popover.Root
    bind:open={() => open, opens}
    onOpenChange={(next) => {
      if (next !== open) return;
      if (next) {
        typed = "";
        lit = value;
      }
      onopenchange?.(next);
    }}
  >
    <Popover.Trigger {disabled}>
      {#snippet child({ props })}{@render trigger(props)}{/snippet}
    </Popover.Trigger>
    <Popover.Portal>
      <Popover.Content
        sideOffset={drop}
        align={edge}
        onEscapeKeydown={() => (escaped = true)}
        onCloseAutoFocus={(e) => {
          if (stayAway(e) || !escaped) return;
          e.preventDefault();
          letGo();
        }}
      >
        {#snippet child({ props, wrapperProps, open })}
          {#if open}
            <div {...wrapperProps}>
              <div {...props} class="pick-list searching">
                <Command.Root bind:value={lit} vimBindings={false} shouldFilter={false} {label}>
                  {#snippet child({ props: root })}
                    <div {...root} class="pick-command">
                      <Command.Input bind:value={typed}>
                        {#snippet child({ props: field })}
                          <SearchField {...field} bind:value={typed} placeholder={search} />
                        {/snippet}
                      </Command.Input>
                      <Command.List>
                        {#snippet child({ props: list })}
                          <div {...list} class="pick-rows" use:onPicked>
                            <Command.Viewport>
                              {#snippet child({ props: viewport })}
                                <div {...viewport}>
                                  {#if sections.length === 0}
                                    <div class="pick-none">Nothing matches</div>
                                  {/if}
                                  {#each sections as section, i (section.group ?? "")}
                                    <Command.Group value={section.group ?? "ours"}>
                                      {#snippet child({ props: group })}
                                        <div {...group}>
                                          {#if section.group}
                                            <Command.GroupHeading>
                                              {#snippet child({ props: head })}
                                                <div {...head} class="pick-group" class:first={i === 0}>{section.group}</div>
                                              {/snippet}
                                            </Command.GroupHeading>
                                          {/if}
                                          <Command.GroupItems>
                                            {#snippet child({ props: items })}
                                              <div {...items}>
                                                {#each section.options as option (option.value)}
                                                  <Command.Item
                                                    value={option.value}
                                                    keywords={[option.label]}
                                                    onSelect={() => choose(option.value)}
                                                  >
                                                    {#snippet child({ props: item })}
                                                      <div
                                                        {...item}
                                                        class="pick-row"
                                                        data-chosen={option.value === value ? "" : undefined}
                                                        title={option.label}
                                                      >
                                                        {@render row(option, option.value === value)}
                                                      </div>
                                                    {/snippet}
                                                  </Command.Item>
                                                {/each}
                                              </div>
                                            {/snippet}
                                          </Command.GroupItems>
                                        </div>
                                      {/snippet}
                                    </Command.Group>
                                  {/each}
                                </div>
                              {/snippet}
                            </Command.Viewport>
                          </div>
                        {/snippet}
                      </Command.List>
                    </div>
                  {/snippet}
                </Command.Root>
              </div>
            </div>
          {/if}
        {/snippet}
      </Popover.Content>
    </Popover.Portal>
  </Popover.Root>
{:else}

<Select.Root
  type="single"
  bind:value={ticked}
  items={options}
  {disabled}
  bind:open={() => open, opens}
  onOpenChange={(next) => {
    if (next !== open) return;
    if (next) ticked = value;
    else tick().then(letGo);
    onopenchange?.(next);
  }}
  onValueChange={(v) => onpick?.(v)}
>
  <Select.Trigger>
    {#snippet child({ props })}{@render trigger(props)}{/snippet}
  </Select.Trigger>
  <Select.Portal>
    <!-- Right under the trigger and along its left edge, so the list opens
         where the eye already is. -->
    <Select.Content sideOffset={drop} align={edge} onEscapeKeydown={() => (escaped = true)}>
      {#snippet child({ props, wrapperProps, open })}
        {#if open}
          <div {...wrapperProps}>
            <div {...props} class="pick-list">
              {#each options as option, i (option.value)}
                {#if option.group && option.group !== options[i - 1]?.group}
                  <div class="pick-group" class:first={i === 0}>{option.group}</div>
                {/if}
                <Select.Item value={option.value} label={option.label}>
                  {#snippet child({ props: item, selected })}
                    <!-- Read from the same side the trigger reads from,
                         and the tick on that side too, so a name in the
                         list starts where the name on the trigger starts
                         and the eye runs down one edge. -->
                    <div
                      {...item}
                      class="pick-row"
                      class:right={fromRight}
                      data-chosen={selected ? "" : undefined}
                      title={option.label}
                    >
                      {#if fromRight}
                        <span class="what">{option.label}</span>
                        <span class="tick">
                          {#if selected}<Icon name="check" size={12} />{/if}
                        </span>
                      {:else}
                        {@render row(option, selected)}
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
{/if}

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

  /* Only the icon of what is picked, in a box as wide as it is high plus
     the room the rail gives every icon, and as quiet as the icons beside
     it until the pointer is on it. */
  .pick.face {
    justify-content: center;
    width: 36px;
    padding: 0;
    color: var(--muted);
  }

  .pick.face:hover:not(:disabled),
  .pick.face[aria-expanded="true"] {
    color: var(--text);
  }

  .pick.warn {
    border-color: var(--warn);
  }

  .pick.err {
    border-color: var(--err);
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
    align-self: center;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .room {
    visibility: hidden;
  }

  .unpicked {
    color: var(--muted);
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
       well they were out by exactly that.

       And nothing above the first row or under the last, which Tim saw
       as a strip of nothing between the search field and the rows. The
       rows run from edge to edge, the way the rows of a list in Finder
       do, and the corners are the trigger's, so the list reads as the
       trigger opened. */
    padding: 0;
    overflow-y: auto;
    color: var(--text);
    background: var(--ink-1);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.55);
  }

  /* A row is as tall as a control whatever the list holds. The list is a
     column that bits-ui lays out as a flex box, and a row in a flex box
     gives way when the column is shorter than its rows: in a list of a
     few hundred fonts, every row was squeezed to the height of its line of
     text, 19 pixels where the trigger is 32, which matched nothing on
     screen. */
  .pick-row {
    flex: none;
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--control-h);
    padding: 0 8px;
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

  /* The list to search calls the same row selected, the word command bars
     use for the one the keyboard is on. */
  .searching .pick-row[data-selected] {
    background: var(--ink-3);
  }

  /* What is chosen is in the accent, the way a chosen row is everywhere
     else in the app. */
  .pick-row[data-chosen] {
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

  /* The icon a row shares with the trigger, in the colour of its name. */
  .glyph {
    display: flex;
    flex: none;
  }

  .what {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  /* A name drawn in its own face has its capitals in the middle of its
     box, in the row and in the field alike. The line a face brings is as
     high above its letters and as low under them as its maker chose, so
     Palatino and an italic stood a pixel high where Inter stood in the
     middle. So the face's own line counts for nothing here, line-height
     0, and a strut of no width stands on the baseline instead, as high as
     half the box and half a capital. The baseline is then half a capital
     under the middle of the box, so the capitals are in the middle
     whatever the face. It is plain inline layout that every engine does
     alike, where cutting the line to the capitals with text-box came out
     a pixel apart between two versions of Chromium. The box is as tall as
     the inside of a field, so a face with tall letters is cut there
     rather than pushing the rows apart. */
  .what.shown,
  .said > .shown {
    height: calc(var(--control-h) - 2px);
    line-height: 0;
  }

  .what.shown::before,
  .said > .shown::before {
    content: "";
    display: inline-block;
    height: calc((var(--control-h) - 2px + 1cap) / 2);
  }

  .pick-row.right .what {
    text-align: right;
  }

  /* The word beside a name, what it would cost or that it will not run
     here, in the colour of a unit, apart from the name by the room a
     column needs, so the words line up down the list. */
  .detail {
    flex: none;
    margin-left: 24px;
    color: var(--muted);
    font-size: var(--size-s);
  }

  .detail.warn {
    color: var(--warn);
  }

  /* The head of a group of rows, and the line above it that parts it from
     the group before, the way a pop-up menu on the Mac is parted. */
  .pick-group {
    flex: none;
    padding: 8px 8px 4px 28px;
    border-top: 1px solid var(--line);
    color: var(--muted);
    font-size: var(--size-s);
    font-weight: 600;
    user-select: none;
  }

  .pick-group.first {
    border-top: none;
    padding-top: 4px;
  }

  /* A list to search keeps its field where it is and scrolls its rows
     under it. */
  .pick-list.searching {
    display: flex;
    flex-direction: column;
    padding: 0;
    overflow: hidden;
  }

  .pick-command {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  /* The field is a row of the list, with the magnifier where the ticks
     stand and what is typed where the names start, so the eye runs down
     one edge from the field into the rows. It is a row tall above its
     line, and the line is under that, so it does not stand a pixel
     shorter than the rows it is over. */
  .pick-rows {
    min-height: 0;
    overflow-y: auto;
  }

  /* A row with nothing to tick, so it keeps no room for a tick. It says
     so in the middle, and asks no more width of the list than its field
     has. */
  .pick-none {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    height: var(--control-h);
    padding: 0 8px;
    color: var(--muted);
  }
</style>
