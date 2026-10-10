<script lang="ts">
  import { api } from "../lib/api";
  import Busy from "../components/Busy.svelte";
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
  <div class="card asks">
    <span class="ask corner">
      <Info label="What a report holds" side="right">
        What you write here, the app's log of what it did, which build this is and what Mac it runs on, and your
        settings. Never a video, its words or its captions, and your home folder is written as ~. It is saved on the
        Desktop as one zip file, which Finder shows, and you can look inside before you send it.
      </Info>
    </span>
    <div class="item words-row">
      <textarea
        class="selectable"
        bind:value={what}
        placeholder="What happened, and what were you doing when it did?"
        aria-label="What happened"
        spellcheck="true"
      ></textarea>
    </div>
    <div class="item">
      <div class="words">
        {#if problem}
          <span class="error selectable">{problem}</span>
        {:else if made}
          <span class="selectable">Saved on the Desktop as {made}</span>
        {:else}
          <span class="muted">Saved on the Desktop as one zip file</span>
        {/if}
      </div>
      <button class="act primary" onclick={make} disabled={making} title="Saves the report on the Desktop and shows it in Finder">
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

  /* The info mark sits in the card's top right corner, see .ask.corner
     in app.css, and the field keeps clear of it. */
  .card {
    position: relative;
  }

  .words-row {
    padding-right: 28px;
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
</style>
