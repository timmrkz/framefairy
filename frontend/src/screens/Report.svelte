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
  // words appear in the field while they are said, where the cursor was.
  // What follows the last pause is still being heard and changes as more
  // of it is, so it is grey and italic and cannot be typed in. What came
  // before the pause is settled, and is written into the field as if it
  // had been typed, so it can be corrected at once and Cmd+Z takes it
  // back. Enter or the button keeps all of it, Escape removes it. The Go
  // side records and writes down, see listen.go.
  let field: HTMLDivElement;
  let listening = $state(false);
  let hearing = $state(false);
  // The recording in hand, and how many of its pieces are in the field.
  let recording = 0;
  let placed = 0;
  // Where what is still being heard is shown, a mark in the text that
  // cannot be typed in, which draws the words after itself. The words
  // written in go just before it.
  let mark: HTMLSpanElement | null = null;
  // Where the dictation began. A range keeps its place as text is put in
  // at its point, so it stays at the start of what was said, and Escape
  // removes from here to the mark.
  let from: Range | null = null;
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

  // The text just before and just after a point in the field.
  function around(at: Range): [string, string] {
    const before = document.createRange();
    before.selectNodeContents(field);
    before.setEnd(at.startContainer, at.startOffset);
    const after = document.createRange();
    after.selectNodeContents(field);
    after.setStart(at.endContainer, at.endOffset);
    return [before.toString(), after.toString()];
  }

  const atMark = () => {
    const at = document.createRange();
    at.setStartBefore(mark!);
    at.collapse(true);
    return at;
  };

  // spaced is the words with a space before them where they would
  // otherwise run into the word before.
  const spaced = (words: string) => {
    const [before] = around(atMark());
    return (before && !/\s$/.test(before) ? " " : "") + words;
  };

  // runsOn says whether the text after the mark starts with a word, which
  // what is still being heard would run into.
  const runsOn = () => /^\S/.test(around(atMark())[1]);

  // The mark goes where the cursor is, or at the end of the text, and the
  // cursor stays after it, so what is typed meanwhile follows what is said.
  function placeMark() {
    let at = cursor && field.contains(cursor.startContainer) ? cursor.cloneRange() : null;
    if (!at) {
      at = document.createRange();
      at.selectNodeContents(field);
    }
    at.collapse(false);
    mark = document.createElement("span");
    mark.className = "hearing";
    mark.contentEditable = "false";
    at.insertNode(mark);
    // In the text before the mark, where there is some, since a point
    // between two nodes would move along with what is put in before it.
    from = document.createRange();
    const prev = mark.previousSibling;
    if (prev?.nodeType === Node.TEXT_NODE) from.setStart(prev, prev.textContent!.length);
    else from.setStartBefore(mark);
    from.collapse(true);
    field.focus();
    const sel = getSelection();
    const after = document.createRange();
    after.setStartAfter(mark);
    sel?.removeAllRanges();
    sel?.addRange(after);
  }

  // write puts settled words in just before the mark as if they had been
  // typed, and gives the cursor back where it was.
  function write(words: string, as = spaced) {
    if (!mark) return;
    if (!field.contains(mark)) {
      // Removed along with the text round it. It goes back at the cursor.
      placeMark();
    }
    const was = document.activeElement as HTMLElement | null;
    const sel = getSelection();
    const keep = sel?.rangeCount && field.contains(sel.anchorNode) ? sel.getRangeAt(0).cloneRange() : null;
    const text = as(words);
    field.focus();
    sel?.removeAllRanges();
    sel?.addRange(atMark());
    if (!document.execCommand("insertText", false, text)) mark.before(text);
    sel?.removeAllRanges();
    if (keep) sel?.addRange(keep);
    if (was && was !== field) was.focus();
    read();
  }

  // catchUp writes in the pieces not yet in the field. Every message has
  // all of them, so one that was missed costs nothing.
  function catchUp(said: string[]) {
    for (; placed < said.length; placed++) write(said[placed]);
  }

  // end takes the mark out, with a space after what was said where the
  // text after it would otherwise run on.
  function end() {
    if (mark && field.contains(mark)) {
      const [before, after] = around(atMark());
      if (before && after && !/^\s/.test(after) && !/\s$/.test(before)) write(" ", (w) => w);
      mark.remove();
    }
    mark = null;
    from = null;
    read();
  }

  onMount(() =>
    onDictation((d) => {
      if (!listening || (recording && d.recording !== recording)) return;
      if (d.error) {
        problem = sentence(d.error);
        listening = false;
        end();
        return;
      }
      catchUp(d.said ?? []);
      if (mark) mark.dataset.hearing = d.hearing ? spaced(d.hearing) + (runsOn() ? " " : "") : "";
    }),
  );

  async function dictate() {
    if (hearing) return;
    if (listening) return keep();
    problem = "";
    listening = true;
    recording = 0;
    placed = 0;
    placeMark();
    try {
      recording = await api.listen();
    } catch (e) {
      listening = false;
      end();
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
      if (mark) mark.dataset.hearing = "";
      end();
      hearing = false;
    }
  }

  // drop stops listening and removes what was said, as one step Cmd+Z
  // takes back.
  function drop() {
    if (!listening) return;
    listening = false;
    api.cancelListening().catch(() => {});
    if (mark && from && field.contains(mark)) {
      const said = from.cloneRange();
      said.setEndBefore(mark);
      if (said.toString()) {
        const sel = getSelection();
        field.focus();
        sel?.removeAllRanges();
        sel?.addRange(said);
        document.execCommand("delete");
      }
    }
    mark?.remove();
    mark = null;
    from = null;
    read();
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
           which a textarea cannot. -->
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
  .item:has(.telling) {
    position: relative;
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

  /* Where a textarea has its placeholder, in the field's own padding. */
  .placeholder {
    position: absolute;
    left: 13px;
    top: 9px;
    line-height: 19px;
    color: var(--muted);
    pointer-events: none;
  }

  /* What is still being heard: grey and slanted, the way claude.ai shows
     it, drawn after the mark and so not text that can be typed in. */
  .telling :global(.hearing)::after {
    content: attr(data-hearing);
    color: var(--muted);
    font-style: italic;
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
