<script lang="ts">
  // What can be done with a video in the sidebar, behind the three dots at
  // the end of its row, the way Finder offers it on a right-click: the
  // words and the colour of its label, Show in Folder and Remove.
  //
  // The label is how videos with the same file name are told apart, Tim's
  // folders of youtube.mp4s above all. It is never the file's name: the
  // video keeps its name on the disk, and the label is kept in the video's
  // work folder, see engine.Label.
  //
  // The menu hangs under the row and is exactly as wide as it, so it never
  // reaches further over the workspace than the sidebar already does. The
  // well at the end of the colours hangs its pop-up on its right edge for
  // the same reason.
  //
  // Everything saves at once, the way it does everywhere in the app: a
  // colour the moment it is clicked, the words when Return is pressed, the
  // field is left or the menu closes. The pill in the row follows the words
  // while they are typed. Escape puts back what was there.
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
  let colour = $state<string | null>(null);
  // The label as it was when the menu opened, or as it was last saved, so
  // only a change is saved and Escape has something to put back.
  let kept: Label | null = null;
  let putBack = false;
  let field = $state<HTMLInputElement>();
  let menu = $state<HTMLElement>();

  // The folder the video is in, the words a label starts with when a
  // colour is clicked before anything is typed: in a folder of episodes
  // each in its own folder, that is the name that tells them apart.
  const folder = $derived(ep.source.split(/[\\/]/).slice(-2, -1)[0] ?? "");

  function opened() {
    kept = ep.label ? { ...ep.label } : null;
    text = ep.label?.text ?? "";
    colour = ep.label?.colour ?? null;
    putBack = false;
  }

  // The label the menu holds now, or nothing when it has no words.
  function now(): Label | null {
    const words = text.trim();
    return words ? { text: words, colour: colour ?? freshColour(worn) } : null;
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

  function typed(value: string) {
    text = value;
    ondraft(now());
  }

  function pickColour(hex: string, keep = true) {
    colour = hex;
    if (!text.trim()) text = folder;
    ondraft(now());
    if (keep) save();
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
      <button {...props} class="more" title="More" aria-label="More for {ep.name}">
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
      }}
      onEscapeKeydown={() => {
        putBack = true;
      }}
    >
      {#snippet child({ props, wrapperProps, open: shown })}
        {#if shown}
          <div {...wrapperProps}>
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div {...props} bind:this={menu} class="video-menu" aria-label="More for {ep.name}" onkeydown={keepKeys}>
              <div class="label">
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
                      if (!colour && text.trim()) colour = freshColour(worn);
                      save();
                      close();
                    }
                  }}
                  onblur={(e) => {
                    // A colour clicked saves the words with it, so leaving
                    // the field for one saves nothing of its own.
                    if (!menu?.contains(e.relatedTarget as Node | null)) save();
                  }}
                />
                <Swatches
                  label="Label colour"
                  value={colour ?? ""}
                  hang="end"
                  oninput={(hex) => pickColour(hex, false)}
                  onpick={(hex) => pickColour(hex)}
                />
              </div>
              <button
                class="item"
                onclick={() => {
                  close();
                  onreveal();
                }}
                disabled={ep.missing}
              >
                Show in Folder
              </button>
              <button
                class="item danger"
                aria-haspopup="dialog"
                onclick={() => {
                  close();
                  onremove();
                }}
              >
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
     as the row it hangs under. */
  .video-menu {
    z-index: 60;
    width: var(--bits-floating-anchor-width);
    padding: 0 0 4px;
    color: var(--text);
    background: var(--ink-1);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.55);
  }

  /* The label's words over its colours, apart from what is done with the
     video by a line, the way a menu on the Mac is parted. */
  .label {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 8px 8px 10px;
    margin-bottom: 4px;
    border-bottom: 1px solid var(--line);
  }

  .label input {
    width: 100%;
  }

  /* A row of the menu is a row of a list: as tall as a control, the name
     where a list's names start, and lit under the pointer. */
  .item {
    display: flex;
    align-items: center;
    width: 100%;
    height: var(--control-h);
    padding: 0 8px;
    color: var(--text);
    text-align: left;
    background: transparent;
    border: none;
    border-radius: 0;
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
  .item.danger {
    color: var(--err);
  }

  /* The mark is the size of the marks it took the place of, centred in
     the row. */
  .more {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    padding: 0;
    width: 24px;
    height: 24px;
    border-radius: var(--radius-s);
    color: var(--muted);
  }

  .more:hover:not(:disabled),
  .more[aria-expanded="true"] {
    color: var(--text);
    background: var(--ink-3);
  }
</style>
