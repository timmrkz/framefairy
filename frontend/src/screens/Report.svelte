<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import { api, onDictation, onLoudness } from "../lib/api";
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
  // heard and changes as more of it is, so it is plain text in a mark
  // that cannot be typed in. What came before the pause is settled and is text
  // in the field, still grey, which can be corrected while the person
  // talks on. Enter or the button keeps all of it, written in as one step
  // Cmd+Z takes back, and Escape removes it. The Go side records and
  // writes down, see listen.go.
  let field: HTMLDivElement;
  let listening = $state(false);
  let hearing = $state(false);
  // The recording in hand, how many of its pieces are in the field, and
  // how many times words being heard were settled here by a click.
  let recording = 0;
  let placed = 0;
  let asked = 0;
  // What has been said, in the text where the cursor was, and the mark at
  // its end that draws what is still being heard. The settled words go
  // in by hand rather than as typing, so they are no steps of their own
  // for Cmd+Z, and Cmd+Z and Cmd+Y while talking undo and redo only what
  // was typed. What is still being heard is plain text in the mark, which
  // cannot be typed in. It was drawn by the mark's ::after from an
  // attribute, and on Tim's Mac the end of a long report was left painted
  // where it had been, grey under white, in lines that did not line up.
  let said: HTMLSpanElement | null = null;
  let mark: HTMLSpanElement | null = null;
  // Where the cursor was in the field, kept while the button has the
  // focus, since the field forgets it then on some systems.
  let cursor: Range | null = null;

  const sentence = (e: unknown) => {
    const said = e instanceof Error ? e.message : String(e);
    return said.charAt(0).toUpperCase() + said.slice(1);
  };

  // The field's text, as it is sent. While somebody dictates it holds
  // what is still being heard too, and Make Report waits until that is
  // kept.
  const read = () => {
    what = field.innerText.replace(/\n$/, "");
  };

  function remember() {
    const sel = getSelection();
    if (sel?.rangeCount && field?.contains(sel.anchorNode)) cursor = sel.getRangeAt(0).cloneRange();
  }

  // How loud the microphone is, which the button moves with, and why the
  // last recording stopped by itself.
  let loud = $state(0);
  let note = $state("");
  // Why the microphone hears nothing, while it records.
  let unheard = $state("");

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

  // What has been settled so far, without what is still being heard.
  function settled(): string {
    const at = document.createRange();
    at.selectNodeContents(said!);
    at.setEndBefore(mark!);
    return at.toString();
  }

  // add puts a settled piece in at the end of what has been said.
  function add(piece: string) {
    if (!said || !mark) return;
    if (!field.contains(said)) begin();
    const before = settled();
    const text = (before.trim() && !/\s$/.test(before) ? " " : "") + piece;
    const last = mark.previousSibling;
    if (last?.nodeType === Node.TEXT_NODE) (last as Text).appendData(text);
    else mark.before(text);
  }

  // show draws what is still being heard after what was settled, and
  // keeps the end of it in view, however long the report has grown.
  function show(now: string) {
    if (!said || !mark) return;
    const rest = around(said)[1];
    const before = settled().trim();
    const text = (now && before ? " " : "") + now + (/^\S/.test(rest) && (now || before) ? " " : "");
    if (mark.textContent !== text) mark.textContent = text;
    // At the end of the report, down to its padding. Tim talked past the
    // field's last line and could not see what was written down.
    if (rest.trim()) mark.scrollIntoView({ block: "nearest", inline: "nearest" });
    else field.scrollTop = field.scrollHeight;
    read();
  }

  // freeze settles the words still being heard where they are, when the
  // pointer goes down in them or just after them, before the browser puts
  // the cursor there. They become text like any settled piece, so the
  // cursor goes where it was put, and a word can be selected and
  // corrected while the person talks on. Tim could not click into them.
  // The Go side is told, and what it says until it has taken that in is
  // passed over, since it would show those words a second time.
  function freeze(e: PointerEvent) {
    if (!listening || !recording || !said || !mark || !field.contains(said)) return;
    const now = mark.textContent!.trim();
    if (!now) return;
    if (!mark.contains(e.target as Node)) {
      const at = document.caretRangeFromPoint?.(e.clientX, e.clientY);
      if (!at) return;
      const from = document.createRange();
      from.setStartBefore(mark);
      from.setEndAfter(said);
      if (from.comparePoint(at.startContainer, at.startOffset) !== 0) return;
    }
    add(now);
    api.settleListening(recording, placed, now).catch(() => {});
    placed++;
    asked++;
    show("");
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
    onLoudness((n) => {
      if (listening && (!recording || n.recording === recording)) loud = n.level;
    }),
  );

  onMount(() =>
    onDictation((d) => {
      if (!listening || (recording && d.recording !== recording)) return;
      if (d.error || d.ended) {
        if (d.error) problem = sentence(d.error);
        if (d.ended) note = sentence(d.ended) + ".";
        listening = false;
        loud = 0;
        catchUp(d.said ?? []);
        finish(true);
        return;
      }
      unheard = d.unheard ? sentence(d.unheard) : "";
      if ((d.asked ?? 0) < asked) return;
      catchUp(d.said ?? []);
      show(d.hearing);
    }),
  );

  async function dictate() {
    if (hearing) return;
    if (listening) return keep();
    problem = "";
    note = "";
    unheard = "";
    listening = true;
    recording = 0;
    placed = 0;
    asked = 0;
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
    loud = 0;
    hearing = true;
    try {
      // It may have stopped by itself just before, and then says why.
      const d = await api.stopListening();
      if (d.recording === recording) {
        catchUp(d.said ?? []);
        if (d.ended) note = sentence(d.ended) + ".";
      }
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
    loud = 0;
    api.cancelListening().catch(() => {});
    finish(false);
  }

  function key(e: KeyboardEvent) {
    if (!listening) return;
    if (e.key === "Enter" && !e.isComposing && !e.shiftKey && !e.altKey && !e.metaKey && !e.ctrlKey) {
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
    note = "";
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
          onpointerdown={freeze}
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
      <button
        class="act dictate"
        class:listening
        onclick={dictate}
        disabled={hearing || making}
        style:--loud={loud}
        title={listening
          ? "Keeps what you said. Enter does the same, and Escape removes it"
          : "Say what happened. The speech model on this computer writes it into the field as you talk, and you can correct it while it does"}
      >
        {#if listening || hearing}<Busy />{/if}
        <Icon name="mic" />
        {listening ? "Stop" : hearing ? "Writing…" : "Dictate"}
      </button>
      <div class="words">
        {#if problem}
          <span class="error selectable">{problem}</span>
        {:else if listening && unheard}
          <span class="error selectable">{unheard}</span>
        {:else if listening}
          <span>Listening…</span>
        {:else if hearing}
          <span>Writing down what you said…</span>
        {:else if note}
          <span>{note}</span>
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
     claude.ai shows it. What is still being heard is in a mark that
     cannot be typed in. */
  .telling :global(.dictated) {
    color: var(--muted);
    font-style: italic;
  }

  /* Room for the longest wording, Make Report, so the row keeps still,
     and Dictate the same size beside it, with room for Writing… and the
     microphone. */
  .act {
    min-width: 116px;
    flex: none;
  }

  /* The microphone moves with the voice it hears, so the person sees they
     are heard. It grows by up to a third, about its middle, which moves
     nothing else in the row. */
  .act.dictate :global(svg) {
    transform: scale(calc(1 + var(--loud, 0) / 3));
    transition: transform 100ms linear;
  }

  /* While it listens the microphone wears the accent, so it reads as on
     at a glance. Tim missed a colour on it. */
  .act.dictate.listening :global(svg) {
    color: var(--accent);
  }

  .act.dictate {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }
</style>
