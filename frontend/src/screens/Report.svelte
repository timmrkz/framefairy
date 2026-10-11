<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { api, onDictation } from "../lib/api";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";

  // Report a Problem…, from the Help menu: what the person says happened,
  // and one button that puts it on the Desktop with the app's log, as the
  // one file they send us. The Go side writes it and shows it in Finder,
  // see report.go and docs/LOGGING.md. Plan row 2.188.
  let { video }: { video?: string } = $props();

  let what = $state("");
  let making = $state(false);
  // The name of the last report made, and why one could not be.
  let made = $state("");
  let problem = $state("");

  // Dictate, plan row 2.189, the way dictation on claude.ai works: the
  // words appear in the field while they are said, where the cursor was,
  // grey until they are kept. What follows the last pause is still being
  // heard and changes as more of it is, so it is drawn by a mark and
  // cannot be typed in. What came before the pause is settled and is text
  // in the field, still grey, which can be corrected while the person
  // talks on. Enter or the button keeps all of it, written in as one step
  // Cmd+Z takes back, and Escape removes it. The Go side records and
  // writes down, see listen.go.
  let field: HTMLDivElement;
  let listening = $state(false);
  let hearing = $state(false);
  // The recording in hand, and how many of its pieces are in the field.
  let recording = 0;
  let placed = 0;
  // What has been said, in the text where the cursor was, and the mark at
  // its end that draws what is still being heard. The settled words go
  // in by hand rather than as typing, so they are no steps of their own
  // for Cmd+Z, and Cmd+Z and Cmd+Y while talking undo and redo only what
  // was typed.
  let said: HTMLSpanElement | null = null;
  let mark: HTMLSpanElement | null = null;
  // Where the cursor was in the field, kept while the button has the
  // focus, since the field forgets it then on some systems.
  let cursor: Range | null = null;

  const sentence = (e: unknown) => {
    const said = e instanceof Error ? e.message : String(e);
    return said.charAt(0).toUpperCase() + said.slice(1);
  };

  // The field's text, as it is sent. What is still being heard is drawn,
  // not written, so it is not in it.
  const read = () => {
    what = field.innerText.replace(/\n$/, "");
  };

  function remember() {
    const sel = getSelection();
    if (sel?.rangeCount && field?.contains(sel.anchorNode)) cursor = sel.getRangeAt(0).cloneRange();
  }

  // The text of the field before and after a node in it.
  function around(node: Node): [string, string] {
    const before = document.createRange();
    before.selectNodeContents(field);
    before.setEndBefore(node);
    const after = document.createRange();
    after.selectNodeContents(field);
    after.setStartAfter(node);
    return [before.toString(), after.toString()];
  }

  // begin puts what will be said where the cursor is, or at the end of
  // the text, and the cursor after it.
  function begin() {
    let at = cursor && field.contains(cursor.startContainer) ? cursor.cloneRange() : null;
    if (!at) {
      at = document.createRange();
      at.selectNodeContents(field);
    }
    at.collapse(false);
    said = document.createElement("span");
    said.className = "dictated";
    mark = document.createElement("span");
    mark.className = "hearing";
    mark.contentEditable = "false";
    said.append(mark);
    at.insertNode(said);
    // A space from the word before, where it would run into it.
    if (/\S$/.test(around(said)[0])) said.prepend(" ");
    field.focus();
    const after = document.createRange();
    after.setStartAfter(said);
    getSelection()?.removeAllRanges();
    getSelection()?.addRange(after);
  }

  // add puts a settled piece in at the end of what has been said.
  function add(piece: string) {
    if (!said || !mark) return;
    if (!field.contains(said)) begin();
    const text = (said.textContent!.trim() && !/\s$/.test(said.textContent!) ? " " : "") + piece;
    const last = mark.previousSibling;
    if (last?.nodeType === Node.TEXT_NODE) (last as Text).appendData(text);
    else mark.before(text);
  }

  // show draws what is still being heard after what was settled, and
  // keeps the end of it in view, however long the report has grown.
  function show(now: string) {
    if (!said || !mark) return;
    const rest = around(said)[1];
    mark.dataset.hearing =
      (now && said.textContent!.trim() ? " " : "") + now + (/^\S/.test(rest) && (now || said.textContent!.trim()) ? " " : "");
    // At the end of the report, down to its padding. Tim talked past the
    // field's last line and could not see what was written down.
    if (rest.trim()) mark.scrollIntoView({ block: "nearest", inline: "nearest" });
    else field.scrollTop = field.scrollHeight;
    read();
  }

  // catchUp writes in the pieces not yet in the field. Every message has
  // all of them, so one that was missed costs nothing.
  function catchUp(pieces: string[]) {
    for (; placed < pieces.length; placed++) add(pieces[placed]);
  }

  // finish ends the dictation. Kept, what was said, with whatever was
  // corrected in it, goes into the text as if it had been typed, as one
  // step Cmd+Z takes back, and the cursor after it.
  function finish(kept: boolean) {
    const was = said;
    said = mark = null;
    if (!was || !field.contains(was)) return read();
    mark = was.querySelector(".hearing");
    mark?.remove();
    mark = null;
    const text = was.textContent!.trim();
    const [before, after] = around(was);
    const at = document.createRange();
    at.setStartBefore(was);
    at.collapse(true);
    const parent = was.parentNode!;
    const index = Array.prototype.indexOf.call(parent.childNodes, was);
    was.remove();
    if (kept && text) {
      const words = (/\S$/.test(before) ? " " : "") + text + (/^\S/.test(after) ? " " : "");
      field.focus();
      at.setStart(parent, index);
      at.collapse(true);
      getSelection()?.removeAllRanges();
      getSelection()?.addRange(at);
      if (!document.execCommand("insertText", false, words)) at.insertNode(document.createTextNode(words));
    }
    read();
  }

  onMount(() =>
    onDictation((d) => {
      if (!listening || (recording && d.recording !== recording)) return;
      if (d.error) {
        problem = sentence(d.error);
        listening = false;
        finish(true);
        return;
      }
      catchUp(d.said ?? []);
      show(d.hearing);
    }),
  );

  async function dictate() {
    if (hearing) return;
    if (listening) return keep();
    problem = "";
    listening = true;
    recording = 0;
    placed = 0;
    begin();
    try {
      recording = await api.listen();
    } catch (e) {
      listening = false;
      finish(false);
      problem = sentence(e);
    }
  }

  // keep stops listening and keeps everything that was said, the rest
  // written in once the speech model has it.
  async function keep() {
    if (!listening) return;
    listening = false;
    hearing = true;
    try {
      const d = await api.stopListening();
      if (d.recording === recording) catchUp(d.said ?? []);
    } catch (e) {
      problem = sentence(e);
    } finally {
      finish(true);
      hearing = false;
    }
  }

  // drop stops listening and removes what was said.
  function drop() {
    if (!listening) return;
    listening = false;
    api.cancelListening().catch(() => {});
    finish(false);
  }

  function key(e: KeyboardEvent) {
    if (!listening) return;
    if (e.key === "Enter" && !e.shiftKey && !e.altKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      keep();
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      drop();
    }
  }

  // A recording is not left running behind another page.
  onDestroy(() => {
    if (listening) api.cancelListening().catch(() => {});
  });

  async function make() {
    if (making) return;
    making = true;
    problem = "";
    try {
      const path = await api.reportProblem(what, video ?? "");
      made = path.split(/[\\/]/).pop() ?? path;
    } catch (e) {
      problem = e instanceof Error ? e.message : String(e);
    } finally {
      making = false;
    }
  }
</script>

<svelte:window onkeydowncapture={key} />

<section class="scroll">
  <div class="card">
    <div class="item">
      <!-- A field of plain text that can hold the words still being heard,
           which a textarea cannot. The placeholder lies over it in a box
           of their own, so it lines up with the field's text. -->
      <div class="box">
        <div
          class="telling selectable"
          bind:this={field}
          contenteditable="plaintext-only"
          role="textbox"
          tabindex="0"
          aria-multiline="true"
          aria-label="What happened"
          spellcheck="true"
          oninput={read}
          onkeyup={remember}
          onmouseup={remember}
          onblur={remember}
        ></div>
        {#if !what && !listening}
          <span class="placeholder" aria-hidden="true">What happened, and what were you doing when it did?</span>
        {/if}
      </div>
    </div>
    <div class="item">
      <!-- Room for the longest wording, Writing, so the row keeps still. -->
      <button
        class="act dictate"
        onclick={dictate}
        disabled={hearing || making}
        aria-pressed={listening}
        title={listening
          ? "Keeps what you said. Enter does the same, and Escape removes it"
          : "Say what happened. The speech model on this Mac writes it into the field as you talk, and you can correct it while it does"}
      >
        {#if listening || hearing}<Busy />{/if}
        <Icon name="mic" />
        {listening ? "Stop" : hearing ? "Writing…" : "Dictate"}
      </button>
      <div class="words">
        {#if problem}
          <span class="error selectable">{problem}</span>
        {:else if listening}
          <span>Listening…</span>
        {:else if hearing}
          <span>Writing down what you said…</span>
        {:else if made}
          <span class="selectable">Saved on the Desktop as {made}</span>
        {:else}
          <span class="muted">Saved on the Desktop as one zip file</span>
        {/if}
      </div>
      <!-- Beside the button rather than in the card's corner, where it
           pushed the field off the middle of its row, and always there,
           as Tim asked, since what goes in a report is worth seeing
           before it is made. -->
      <span class="ask always">
        <Info label="What a report holds" side="right">
          What you write here, the app's log of what it did, which build this is and what Mac it runs on, and your
          settings. Never a video, its words or its captions, and your home folder is written as ~. It is saved on the
          Desktop as one zip file, which Finder shows, and you can look inside before you send it.
        </Info>
      </span>
      <button class="act primary" onclick={make} disabled={making || listening || hearing} title="Saves the report on the Desktop and shows it in Finder">
        {#if making}<Busy />{/if}
        {making ? "Making…" : "Make Report"}
      </button>
    </div>
  </div>
</section>

<style>
  /* The same page as the settings and the updates, kept to a width that
     reads, in the middle of the app. */
  section {
    padding: var(--gap) var(--edge) var(--edge);
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: 880px;
    margin: 0 auto;
  }

  .always {
    opacity: 1;
    pointer-events: auto;
  }

  /* A field of ten whole lines of the card's 19 px, with the padding of a
     control round them. */
  .box {
    position: relative;
    flex: 1;
    display: flex;
  }

  .telling {
    flex: 1;
    height: calc(10 * 19px + 2 * 8px + 2px);
    box-sizing: border-box;
    overflow-y: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font: inherit;
    line-height: 19px;
    color: inherit;
    background: var(--ink-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    padding: 8px 12px;
  }

  /* Where a textarea has its placeholder, in the field's own border and
     padding, 1 px and 12 px across, 1 px and 8 px down. */
  .placeholder {
    position: absolute;
    left: 13px;
    top: 9px;
    line-height: 19px;
    color: var(--muted);
    pointer-events: none;
  }

  /* What is being said: grey and slanted until it is kept, the way
     claude.ai shows it. What is still being heard is drawn after the mark
     and so is not text that can be typed in. */
  .telling :global(.dictated) {
    color: var(--muted);
    font-style: italic;
  }

  .telling :global(.hearing)::after {
    content: attr(data-hearing);
  }

  /* Room for the longer wording, Make Report, so the row keeps still. */
  .act {
    min-width: 116px;
    flex: none;
  }

  /* Room for Writing… beside the microphone. */
  .act.dictate {
    min-width: 108px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }
</style>
