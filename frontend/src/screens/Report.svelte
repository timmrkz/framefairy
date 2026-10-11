<script lang="ts">
  import { onDestroy } from "svelte";
  import { api } from "../lib/api";
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

  // Dictate, plan row 2.189: the app records what the person says and
  // the speech model on this machine writes it into the field, where the
  // cursor was, as if it had been typed, so Cmd+Z takes it back. The Go
  // side records, see listen.go.
  let field: HTMLTextAreaElement;
  let listening = $state(false);
  let hearing = $state(false);

  const sentence = (e: unknown) => {
    const said = e instanceof Error ? e.message : String(e);
    return said.charAt(0).toUpperCase() + said.slice(1);
  };

  async function dictate() {
    if (hearing) return;
    problem = "";
    if (!listening) {
      listening = true;
      try {
        await api.listen();
      } catch (e) {
        listening = false;
        problem = sentence(e);
      }
      return;
    }
    listening = false;
    hearing = true;
    try {
      const text = await api.stopListening();
      if (text) put(text);
    } catch (e) {
      problem = sentence(e);
    } finally {
      hearing = false;
    }
  }

  // put writes what was said where the cursor was, or over what was
  // selected, with a space either side where the words would otherwise
  // run together. The field keeps where its cursor was while the button
  // has the focus, so text typed while listening is taken into account.
  function put(text: string) {
    const start = field.selectionStart;
    const end = field.selectionEnd;
    const before = what.slice(0, start);
    const after = what.slice(end);
    const words = (before && !/\s$/.test(before) ? " " : "") + text + (after && !/^\s/.test(after) ? " " : "");
    field.focus();
    field.setSelectionRange(start, end);
    // As if typed, so the field's own undo takes it back. Where the
    // browser will not, it is written in.
    if (!document.execCommand("insertText", false, words)) {
      what = before + words + after;
    }
  }

  // A recording is not left running behind another page.
  onDestroy(() => {
    if (listening) api.stopListening().catch(() => {});
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

<section class="scroll">
  <div class="card">
    <div class="item">
      <textarea
        class="selectable"
        bind:this={field}
        bind:value={what}
        placeholder="What happened, and what were you doing when it did?"
        aria-label="What happened"
        spellcheck="true"
      ></textarea>
    </div>
    <div class="item">
      <!-- Room for the longest wording, Writing, so the row keeps still. -->
      <button
        class="act dictate"
        onclick={dictate}
        disabled={hearing || making}
        aria-pressed={listening}
        title={listening
          ? "Stops listening and writes down what you said"
          : "Say what happened. The speech model on this Mac writes it into the field, where you can correct it"}
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
  textarea {
    flex: 1;
    height: calc(10 * 19px + 2 * 8px + 2px);
    box-sizing: border-box;
    resize: none;
    font: inherit;
    line-height: 19px;
    color: inherit;
    background: var(--ink-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    padding: 8px 12px;
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
