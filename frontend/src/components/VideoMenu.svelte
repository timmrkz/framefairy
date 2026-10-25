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
  // One row is about the words of the video, and says the one thing that
  // can be done with them now, issue #432: Transcribe whole video while
  // the transcript has not reached the end, Stop transcribing while that
  // runs, Continue transcribing once it stopped, by hand or by the app
  // closing, and Export transcript once it is all there. While a search or
  // a clip made by hand transcribes the video, the speech model is theirs,
  // so the first two say the rest comes next, Transcribe the rest next
  // and Continue next, and the click waits for them, then goes on to the
  // end. It said Continue transcribing, as if nothing were transcribing,
  // Tim's report of 25 October, 14.33, and his pick. Each is an
  // action, which closes the menu the way a menu item does on the Mac.
  // How far the work has come is not the menu's to show: the range picker
  // frames the part it works on and the video's dot pulses, Tim's calls
  // on #458.
  // Export hangs a list of its formats on the right of the menu, the way
  // Export To does in Pages, and a format opens the system's save box. Two rows of which one never works
  // were two things to read for one.
  //
  // Everything saves at once, the way it does everywhere in the app: a
  // colour the moment it is clicked, the words when Return is pressed, the
  // field is left or the menu closes. The pill in the row follows the words
  // while they are typed. Escape puts back what was there, and the cross at
  // the end of the field, there while it has words, removes the label.
  import { Popover } from "bits-ui";
  import { tick } from "svelte";
  import Icon from "./Icon.svelte";
  import Swatches from "./Swatches.svelte";
  import type { EpisodeStatus, Job, Label, TranscriptFormat } from "../lib/api";
  import { freshColour } from "../lib/colour";

  // How many characters a label keeps, engine.LabelLength.
  const LabelLength = 40;

  let {
    ep,
    drawn,
    shown,
    anchor,
    worn,
    onopenchange,
    ondraft,
    onsave,
    onreveal,
    onremove,
    work,
    stopping,
    hearsFirst,
    ontranscribe,
    onstoptranscribe,
    onexport,
  }: {
    ep: EpisodeStatus;
    // The label the row draws, which is the one being saved while a save
    // is on its way, so the menu opens on what the row shows.
    drawn: Label | null;
    // Whether the sidebar is open. The menu belongs to it, so it closes
    // when the sidebar does, for whatever reason, rather than stay over
    // a rail with no row left to hang under.
    shown: boolean;
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
    // The video's transcription to its end, running or stopped, or nothing,
    // and whether Stop transcribing was pressed while it still runs.
    work: Job | undefined;
    stopping: boolean;
    // Whether the whole video's transcription would wait now, behind a
    // search or a clip made by hand that transcribes or finds.
    hearsFirst: boolean;
    ontranscribe: () => void;
    onstoptranscribe: () => void;
    onexport: (format: TranscriptFormat) => void;
  } = $props();

  // What the row about the words says, see above. A video whose
  // transcript reaches its end is done, whatever job brought it there. One
  // told to stop is as good as stopped, so the row offers to carry it on.
  const running = $derived((work?.state === "running" || work?.state === "queued") && !stopping);
  const words = $derived(
    running ? "stop" : ep.transcribed ? "export" : work && work.state !== "done" ? "continue" : "transcribe",
  );

  // The list of formats Export transcript hangs on the right of the menu.
  // The pointer on Export hangs it out and leaves the keyboard where it
  // was, so a label being typed is neither saved nor lost by it. A click
  // on Export keeps it out, the way a submenu stays on the Mac. Return,
  // Space or the right arrow on Export hang it out with the keyboard on
  // its first format, the arrows go up and down it, and the left arrow
  // takes the keyboard back to Export. Found by the review of #458: a
  // click put it away, and no key reached it.
  let exporting = $state(false);
  let exportRow = $state<HTMLButtonElement>();
  let formats = $state<HTMLElement>();
  // The pointer anywhere else in the menu takes the formats in again, the
  // way a submenu goes when the pointer leaves its row, unless it is on
  // its way to them: inside the triangle from where it was to the near
  // edge of the formats, the way a submenu on the Mac stays while the
  // pointer cuts across the row under it. Taken in at once, they closed
  // under a pointer moving from Export to Subtitles, found by the second
  // review of #458. aim is where the pointer was last on that way.
  let aim: [number, number] | null = null;
  function towardFormats(e: PointerEvent) {
    if (!exporting) return;
    const at: [number, number] = [e.clientX, e.clientY];
    if ((e.target as Element).closest("[data-export], .formats")) {
      aim = at;
      return;
    }
    const r = formats?.getBoundingClientRect();
    if (aim && r) {
      const edge = r.left >= aim[0] ? r.left : r.right;
      if (within(at, aim, [edge, r.top], [edge, r.bottom])) {
        aim = at;
        return;
      }
    }
    aim = null;
    exporting = false;
  }
  // Whether p lies in the triangle a, b, c, its edges included.
  function within(p: [number, number], a: [number, number], b: [number, number], c: [number, number]): boolean {
    const side = (u: [number, number], v: [number, number]) => (v[0] - u[0]) * (p[1] - u[1]) - (v[1] - u[1]) * (p[0] - u[0]);
    const [x, y, z] = [side(a, b), side(b, c), side(c, a)];
    return (x >= 0 && y >= 0 && z >= 0) || (x <= 0 && y <= 0 && z <= 0);
  }
  const formatItems = () => [...(formats?.querySelectorAll<HTMLElement>("[role=menuitem]") ?? [])];
  async function showFormats(keyboard: boolean) {
    if (ep.missing) return;
    exporting = true;
    if (!keyboard) return;
    await tick();
    formatItems()[0]?.focus();
  }
  function formatKeys(event: KeyboardEvent) {
    if (event.key === "Escape") return;
    // The list lies outside the menu, so the workspace's shortcuts are
    // kept off it here as the menu keeps them off its own rows.
    event.stopPropagation();
    const items = formatItems();
    const at = items.indexOf(document.activeElement as HTMLElement);
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      items[(at + step + items.length) % items.length]?.focus();
    } else if (event.key === "ArrowLeft") {
      event.preventDefault();
      exporting = false;
      exportRow?.focus();
    }
  }
  function exportAs(format: TranscriptFormat) {
    exporting = false;
    close();
    onexport(format);
  }

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
  // apart. A video at the root of a drive has no folder of its own, /x.mp4
  // or C:\x.mp4, and its file name without the ending is the hint, so a
  // colour clicked in an empty field always brings words with it.
  const folder = $derived.by(() => {
    const parts = ep.source.split(/[\\/]/);
    const dir = parts.at(-2) ?? "";
    if (dir && !/^[A-Za-z]:$/.test(dir)) return dir;
    return (parts.at(-1) ?? "").replace(/\.[^.]*$/, "");
  });

  // The menu as it opens, and as it is again once the label is removed.
  function begin(label: Label | null) {
    kept = label ? { ...label } : null;
    text = label?.text ?? "";
    colour = label?.colour ?? "";
  }

  function opened() {
    begin(drawn);
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
    // As many characters as the Go side keeps, counted the way it counts
    // them: an emoji is one, where the field's own limit counted it as two.
    const letters = [...value];
    if (letters.length > LabelLength) {
      value = letters.slice(0, LabelLength).join("");
      if (field) field.value = value;
    }
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

  // Escape in the colour well of a video with no label: what a shade
  // dragged there made is taken back, and the menu is empty again.
  function noColour() {
    text = "";
    colour = "";
    ondraft(null);
    save();
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
    exporting = false;
    open = false;
    onopenchange(false);
  }

  // Escape in the formats: the menu closes with them and puts the label
  // back, what Escape on the menu does through onEscapeKeydown and
  // onOpenChange, which a menu closed from here does not reach.
  function dismiss() {
    putBack = true;
    exporting = false;
    open = false;
    ondraft(kept);
    onopenchange(false);
  }

  $effect(() => {
    if (!shown && open) close();
  });

  // The shortcuts of the workspace stand aside while the menu has the
  // keyboard, the way they do for the colour picker. Escape still reaches
  // the menu's own layer, which closes it. Return keeps the label and
  // closes the menu from wherever the focus is in it but a button, which
  // Return presses: a colour clicked in WebKit leaves the focus on the
  // menu itself, and Tim pressed Return there and nothing closed.
  function keepKeys(event: KeyboardEvent) {
    if (event.key !== "Escape") event.stopPropagation();
    if (event.key === "Enter" && event.target === menu) {
      event.preventDefault();
      close();
    }
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
              class="video-menu panel"
              role="dialog"
              aria-label="More for {ep.name}"
              onkeydown={keepKeys}
              onpointerdown={() => (pressed = true)}
              onpointermove={towardFormats}
            >
              <div class="label">
                <span class="field">
                  <input
                    bind:this={field}
                    type="text"
                    spellcheck="false"
                    autocomplete="off"
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
                    onnone={noColour}
                  />
                </span>
              </div>
              {#if words === "export"}
                <Popover.Root bind:open={exporting}>
                  <Popover.Trigger>
                    {#snippet child({ props })}
                      <!-- The click is the menu's own and only ever hangs
                           the formats out. A click made with the keyboard
                           has no detail, and takes the keyboard to them. -->
                      <button
                        {...props}
                        bind:this={exportRow}
                        class="item"
                        class:lit={exporting}
                        data-export
                        disabled={ep.missing}
                        title="Save the transcript with every word you corrected"
                        onpointerenter={() => showFormats(false)}
                        onclick={(e) => showFormats(e.detail === 0)}
                        onkeydown={(e) => {
                          if (e.key !== "ArrowRight") return;
                          e.preventDefault();
                          showFormats(true);
                        }}
                      >
                        <Icon name="export" />
                        Export transcript
                        <span class="end"><Icon name="chevron" size={12} /></span>
                      </button>
                    {/snippet}
                  </Popover.Trigger>
                  <Popover.Portal>
                    <!-- Where the keyboard goes is said by showFormats, so
                         the list takes none of its own. Escape closes it and
                         the menu with it, the way it closes a submenu on the
                         Mac, see dismiss. It hangs 4 from the menu and keeps
                         4 from the app's edge, as the menu does from its
                         row. -->
                    <Popover.Content
                      side="right"
                      align="start"
                      sideOffset={4}
                      collisionPadding={4}
                      onOpenAutoFocus={(e) => e.preventDefault()}
                      onCloseAutoFocus={(e) => e.preventDefault()}
                      onEscapeKeydown={(e) => {
                        e.preventDefault();
                        dismiss();
                      }}
                    >
                      {#snippet child({ props, wrapperProps, open: listed })}
                        {#if listed}
                          <div {...wrapperProps}>
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <div {...props} bind:this={formats} class="formats panel" role="menu" aria-label="Formats" onkeydown={formatKeys}>
                              <button class="item" role="menuitem" title="The words in paragraphs, to read or paste" onclick={() => exportAs("text")}>
                                Text
                                <span class="end">.txt</span>
                              </button>
                              <button class="item" role="menuitem" title="Subtitles for YouTube or a video editor" onclick={() => exportAs("srt")}>
                                Subtitles
                                <span class="end">.srt</span>
                              </button>
                            </div>
                          </div>
                        {/if}
                      {/snippet}
                    </Popover.Content>
                  </Popover.Portal>
                </Popover.Root>
              {:else}
                <button
                  class="item"
                  disabled={ep.missing}
                  title={words === "stop"
                    ? "Stop transcribing. What is transcribed stays, and it can be carried on"
                    : hearsFirst
                      ? `${words === "continue" ? "Carry on transcribing" : "Transcribe the rest of the video"} once the search or the clip ahead of it is done`
                      : words === "continue"
                        ? "Carry on transcribing from where it stopped"
                        : "Transcribe the whole video, so its transcript can be exported"}
                  onclick={() => {
                    const stop = words === "stop";
                    close();
                    if (stop) onstoptranscribe();
                    else ontranscribe();
                  }}
                >
                  <Icon name="transcript" />
                  {words === "stop"
                    ? "Stop transcribing"
                    : words === "continue"
                      ? hearsFirst
                        ? "Continue next"
                        : "Continue transcribing"
                      : hearsFirst
                        ? "Transcribe the rest next"
                        : "Transcribe whole video"}
                </button>
              {/if}
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
  /* The menu is the panel a list in the app is, .panel in app.css, as wide
     as the row it hangs under. Its rows run from edge to edge with nothing
     above or under them, and its corners clip them. */
  .video-menu {
    width: var(--bits-floating-anchor-width);
    padding: 0;
    overflow: hidden;
  }

  /* The menu takes the focus only to hold it for what is in it, so it is
     never something the keyboard points at, and wears no ring. */
  .video-menu:focus-visible {
    outline: none;
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

  /* A row of the menu is as tall as a row of the list of videos it hangs
     under, 36, so the two read as one list. As tall as a control it looked
     smaller than the rows over it once its lit part reached the menu's
     edges. Its icon stands in the sidebar's column of icons, 9 and the
     menu's border from the row's edge as a foot row's is from the list's,
     its words 8 after it, and it is lit under the pointer. */
  .item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    height: 36px;
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
  .item:focus-visible,
  .item.lit {
    background: var(--ink-3);
    outline: none;
  }

  /* What stands at the end of a row, the chevron of Export or a format's
     ending, in the grey of a secondary word, the way a menu on
     the Mac puts a shortcut there. */
  .item .end {
    display: flex;
    align-items: center;
    margin-left: auto;
    color: var(--muted);
  }

  /* The list of formats hangs beside the menu, its rows the menu's rows
     and running from edge to edge the same way. It is as wide as its
     words and never narrower than 160, so the ending of a format stands
     apart from its name the way a shortcut does in a menu on the Mac. */
  .formats {
    min-width: 160px;
    padding: 0;
    overflow: hidden;
  }

  .formats:focus-visible {
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
    /* No border, which the lit part would be placed inside of: a button's
       border of 1 put it a pixel low in the row, and Tim saw the mark
       longer at the bottom than at the top. */
    border: none;
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
