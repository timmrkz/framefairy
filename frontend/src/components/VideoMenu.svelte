<script lang="ts">
  // What can be done with a video in the sidebar, behind the three dots at
  // the end of its row, the way Finder offers it on a right-click: the
  // words and the colour of its label, Show in folder and Remove.
  //
  // The label is how videos with the same file name are told apart, Tim's
  // folders of youtube.mp4s above all. It is never the file's name: the
  // video keeps its name on the disk, and the label is kept in the video's
  // work folder, see engine.Label.
  //
  // The menu hangs under the row and is exactly as wide as it, so it never
  // reaches further over the workspace than the sidebar already does. The
  // well at the end of the colours hangs its pop-up on its right edge for
  // the same reason. Its icons stand in the sidebar's column of icons and
  // its words where the sidebar's words start.
  //
  // The menu shows the label as it is. A video with no label has an empty
  // field with no colour chosen, and the name of its folder in grey in the
  // field, which is what tells a folder of youtube.mp4s apart: Tab takes
  // it, the way Tab takes a suggestion in a field on the Mac. The moment
  // the field has words, by Tab or by typing, a colour is chosen, one no
  // other video wears yet, and the pill shows in the row. An empty field
  // is no label, so the field, the colours and the row never disagree, and
  // a menu opened for Show in folder leaves the video as it was.
  //
  // Everything saves at once, the way it does everywhere in the app: a
  // colour the moment it is clicked, the words when Return is pressed, the
  // field is left or the menu closes. The pill in the row follows the words
  // while they are typed. Escape puts back what was there, and the cross at
  // the end of the field, there while it has words, removes the label.
  import { Popover } from "bits-ui";
  import Icon from "./Icon.svelte";
  import Swatches from "./Swatches.svelte";
  import type { EpisodeStatus, Label } from "../lib/api";
  import { freshColour } from "../lib/colour";

  let {
    ep,
    anchor,
    worn,
    onopenchange,
    ondraft,
    onsave,
    onreveal,
    onremove,
  }: {
    ep: EpisodeStatus;
    // The row the menu hangs under and takes its width from.
    anchor: HTMLElement | undefined;
    // The colours the other videos' labels wear, so a new label starts in
    // one of its own.
    worn: string[];
    // Told when the menu opens and closes. It lies over the app, outside
    // the sidebar, so a sidebar that closes when the pointer leaves it has
    // to be told the pointer is still its own.
    onopenchange: (open: boolean) => void;
    // The label as it is being made, to be drawn and not yet saved.
    ondraft: (label: Label | null) => void;
    // The label to keep, nothing for none.
    onsave: (label: Label | null) => void;
    onreveal: () => void;
    onremove: () => void;
  } = $props();

  let open = $state(false);
  let text = $state("");
  let colour = $state("");
  // The label as it was when the menu opened, or as it was last saved, so
  // only a change is saved and Escape has something to put back.
  let kept = $state<Label | null>(null);
  let putBack = false;
  // A press inside the menu. WebKit gives a button no focus when it is
  // clicked, so leaving the field for a colour says nothing of where the
  // focus went, and the press is how it is known to stay in the menu.
  let pressed = false;
  let field = $state<HTMLInputElement>();
  let menu = $state<HTMLElement>();

  // The folder the video is in, the words a label starts with: in a folder
  // of episodes each in its own folder, that is the name that tells them
  // apart.
  const folder = $derived(ep.source.split(/[\\/]/).slice(-2, -1)[0] ?? "");

  // The menu as it opens, and as it is again once the label is removed.
  function begin(label: Label | null) {
    kept = label ? { ...label } : null;
    text = label?.text ?? "";
    colour = label?.colour ?? "";
  }

  function opened() {
    begin(ep.label ?? null);
    putBack = false;
    pressed = false;
  }

  // The label the menu holds now, or nothing when it has no words.
  function now(): Label | null {
    const words = text.trim();
    return words ? { text: words, colour: colour || freshColour(worn) } : null;
  }

  function same(a: Label | null, b: Label | null) {
    return a?.text === b?.text && a?.colour === b?.colour;
  }

  function save() {
    if (putBack) return;
    const label = now();
    if (same(label, kept)) return;
    kept = label;
    onsave(label);
  }

  // Words in the field and a colour go together: the first word chooses
  // a colour and an empty field has none.
  function typed(value: string) {
    text = value;
    if (!value.trim()) colour = "";
    else if (!colour) colour = freshColour(worn);
    ondraft(now());
  }

  // Tab in an empty field takes the folder's name, with the caret after
  // it so typing goes on from there. Anywhere else Tab moves on, as usual.
  function complete(event: KeyboardEvent) {
    if (text || !folder || event.shiftKey) return;
    event.preventDefault();
    typed(folder);
    field?.setSelectionRange(folder.length, folder.length);
  }

  function pickColour(hex: string, keep = true) {
    if (!text.trim()) text = folder;
    colour = hex;
    ondraft(now());
    if (keep) save();
  }

  // The cross at the end of the field: the label goes, and the menu is as
  // it was before the video had one, ready for another.
  function removeLabel() {
    if (kept) onsave(null);
    begin(null);
    ondraft(null);
    field?.focus();
    field?.select();
  }

  function close() {
    save();
    open = false;
    onopenchange(false);
  }

  // The shortcuts of the workspace stand aside while the menu has the
  // keyboard, the way they do for the colour picker. Escape still reaches
  // the menu's own layer, which closes it.
  function keepKeys(event: KeyboardEvent) {
    if (event.key !== "Escape") event.stopPropagation();
  }
</script>

<svelte:window onpointerup={() => (pressed = false)} onpointercancel={() => (pressed = false)} />

<Popover.Root
  bind:open
  onOpenChange={(now) => {
    if (now) opened();
    else if (putBack) ondraft(kept);
    else save();
    onopenchange(now);
  }}
>
  <Popover.Trigger>
    {#snippet child({ props })}
      <button {...props} class="more quiet" title="More" aria-label="More for {ep.name}">
        <Icon name="more" />
      </button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Portal>
    <Popover.Content
      customAnchor={anchor}
      side="bottom"
      align="end"
      sideOffset={2}
      collisionPadding={4}
      onOpenAutoFocus={(e) => {
        e.preventDefault();
        field?.focus();
        field?.select();
      }}
      onEscapeKeydown={() => {
        putBack = true;
      }}
    >
      {#snippet child({ props, wrapperProps, open: shown })}
        {#if shown}
          <div {...wrapperProps}>
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              {...props}
              bind:this={menu}
              class="video-menu"
              aria-label="More for {ep.name}"
              onkeydown={keepKeys}
              onpointerdown={() => (pressed = true)}
            >
              <div class="label">
                <span class="field">
                  <input
                    bind:this={field}
                    type="text"
                    spellcheck="false"
                    autocomplete="off"
                    maxlength="40"
                    aria-label="Label"
                    title="A label tells this video apart from others with the same name. It stays in the app, the file keeps its name"
                    placeholder={folder || "Label"}
                    value={text}
                    oninput={(e) => typed(e.currentTarget.value)}
                    onkeydown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        close();
                      } else if (e.key === "Tab") complete(e);
                    }}
                    onblur={(e) => {
                      // A colour clicked saves the words with it, so leaving
                      // the field for one saves nothing of its own.
                      if (!pressed && !menu?.contains(e.relatedTarget as Node | null)) save();
                    }}
                  />
                  {#if text.trim()}
                    <button class="clear quiet" title="Remove label" aria-label="Remove label" onclick={removeLabel}>
                      <Icon name="close" size={12} />
                    </button>
                  {/if}
                </span>
                <span class="colours">
                  <Swatches
                    label="Label colour"
                    value={colour}
                    hang="end"
                    oninput={(hex) => pickColour(hex, false)}
                    onpick={(hex) => pickColour(hex)}
                  />
                </span>
              </div>
              <button
                class="item"
                onclick={() => {
                  close();
                  onreveal();
                }}
                disabled={ep.missing}
              >
                <Icon name="folder" />
                Show in folder
              </button>
              <button
                class="item danger"
                aria-haspopup="dialog"
                onclick={() => {
                  close();
                  onremove();
                }}
              >
                <Icon name="trash" />
                Remove
              </button>
            </div>
          </div>
        {/if}
      {/snippet}
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>

<style>
  /* The menu is the panel a list in the app is, see Pick.svelte, as wide
     as the row it hangs under. Its rows run from edge to edge with nothing
     above or under them, and its corners clip them. */
  .video-menu {
    z-index: 60;
    width: var(--bits-floating-anchor-width);
    padding: 0;
    overflow: hidden;
    color: var(--text);
    background: var(--ink-1);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.55);
  }

  /* The label's words over its colours, apart from what is done with the
     video by a line, the way a menu on the Mac is parted. The field and
     the colours start in the column of the icons below them. */
  .label {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 8px 8px 10px 9px;
    border-bottom: 1px solid var(--line);
  }

  /* The cross sits inside the end of the field, the way it does in a
     search field on the Mac, and the words stop short of it. */
  .field {
    position: relative;
    display: flex;
    min-width: 0;
  }

  .field input {
    width: 100%;
  }

  .field:has(.clear) input {
    padding-right: 28px;
  }

  .clear {
    position: absolute;
    top: 50%;
    right: 4px;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    color: var(--muted);
  }

  .clear:hover:not(:disabled) {
    color: var(--text);
  }

  /* A row of the menu is a row of the sidebar's foot: as tall as a
     control, its icon in the sidebar's column of icons, 9 and the menu's
     border from the row's edge as a foot row's is from the list's, its
     words 8 after it, and lit under the pointer. */
  .item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: var(--control-h);
    padding: 0 9px;
    color: var(--text);
    text-align: left;
    background: transparent;
    border: none;
    border-radius: 0;
  }

  .item :global(svg) {
    color: var(--muted);
  }

  .item:hover:not(:disabled),
  .item:focus-visible {
    background: var(--ink-3);
    outline: none;
  }

  .item:disabled {
    color: var(--muted);
  }

  /* Remove wears the colour of a warning, the way it does in the box that
     asks before it, so it is read before it is clicked. */
  .item.danger,
  .item.danger :global(svg) {
    color: var(--err);
  }

  /* The mark is the sort mark at the head of the list: as big, lit the
     same way under the pointer, and in the same column, see .tools in
     App.svelte. It takes the pointer over the row's whole height, and
     what is lit is a control's height in the middle of the 36 the row's
     button has, where the sort mark's is in the head. */
  .more {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 36px;
    height: 100%;
    padding: 0 0 2px;
    color: var(--muted);
    background: transparent;
    border-color: transparent;
  }

  .more::before {
    content: "";
    position: absolute;
    top: calc((36px - var(--control-h)) / 2);
    left: 0;
    right: 0;
    height: var(--control-h);
    border-radius: var(--radius-s);
  }

  .more:hover:not(:disabled),
  .more[aria-expanded="true"] {
    color: var(--text);
    background: transparent;
  }

  .more:hover:not(:disabled)::before {
    background: var(--lift);
  }

  .more :global(svg) {
    position: relative;
  }
</style>
