<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Notice } from "../lib/api";
  import Icon from "../components/Icon.svelte";

  // Acknowledgements, from the Help menu: every piece of other people's
  // work Frame Fairy is made of or brings with it, grouped by where it is,
  // each opening to its licence. The list is built into the app from
  // notices/, see docs/THIRD_PARTY.md.
  let notices = $state<Notice[]>([]);
  let problem = $state("");
  let open = $state<string | null>(null);
  // The texts, by file name, once read. Several notices share one.
  let texts = $state<Record<string, string>>({});

  const parts = $derived.by(() => {
    const groups: { part: string; notices: Notice[] }[] = [];
    for (const n of notices) {
      const last = groups.at(-1);
      if (last?.part === n.part) last.notices.push(n);
      else groups.push({ part: n.part, notices: [n] });
    }
    return groups;
  });

  function key(n: Notice): string {
    return `${n.part}/${n.name}`;
  }

  async function toggle(n: Notice) {
    if (open === key(n)) {
      open = null;
      return;
    }
    open = key(n);
    for (const name of n.texts) {
      if (texts[name] !== undefined) continue;
      try {
        texts[name] = await api.licenceText(name);
      } catch (e) {
        texts[name] = String(e);
      }
    }
  }

  onMount(async () => {
    try {
      notices = await api.licences();
    } catch (e) {
      problem = String(e);
    }
  });
</script>

<section class="scroll">
  <div class="page">
    {#if problem}<p class="error selectable">{problem}</p>{/if}
    {#each parts as group (group.part)}
      <div class="group">
        <h2>{group.part}</h2>
        <ol class="card">
          {#each group.notices as n (key(n))}
            <li>
              <button class="item disclose-row" onclick={() => toggle(n)} aria-expanded={open === key(n)}>
                <span class="words">
                  <span class="head">{n.name}</span>
                  <span class="small muted num">{n.version}</span>
                </span>
                <span class="muted">{n.licence}</span>
                <span class="chevron" class:open={open === key(n)}><Icon name="chevron" size={14} /></span>
              </button>
              {#if open === key(n)}
                <div class="text">
                  {#if n.note}<p class="muted selectable">{n.note}</p>{/if}
                  <p class="muted selectable">{n.url}</p>
                  {#each n.texts as name (name)}
                    <pre class="selectable">{texts[name] ?? ""}</pre>
                  {/each}
                </div>
              {/if}
            </li>
          {/each}
        </ol>
      </div>
    {/each}
  </div>
</section>

<style>
  /* The same page as the settings: it keeps its width and stands in the
     middle of the app, each group a head over a card, and each row of a
     card the settings' row, which opens to its licence the way Downloaded
     models opens in the settings. */
  section {
    flex: 1;
    min-height: 0;
    width: 100%;
  }

  .page {
    display: flex;
    flex-direction: column;
    gap: var(--edge);
    max-width: 880px;
    margin: 0 auto;
    padding: var(--gap) var(--edge) var(--edge);
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
  }

  h2 {
    font-size: var(--size-l);
    line-height: 23px;
    font-weight: 600;
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  li + li {
    border-top: 1px solid var(--line);
  }

  .disclose-row {
    width: 100%;
    height: auto;
    border: none;
    border-radius: 0;
    background: transparent;
    text-align: left;
    white-space: normal;
  }

  .disclose-row:hover:not(:disabled) {
    background: var(--ink-2);
  }

  .head {
    overflow-wrap: anywhere;
  }

  .chevron {
    display: flex;
    color: var(--muted);
    transition: transform 0.12s ease;
  }

  .chevron.open {
    transform: rotate(90deg);
  }

  @media (prefers-reduced-motion: reduce) {
    .chevron {
      transition: none;
    }
  }

  /* What a row opens to stands under its words, in from the card's edges
     as far as the row's own words. */
  .text {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 0 16px 16px;
  }

  .text p {
    font-size: var(--size-s);
    overflow-wrap: anywhere;
  }

  pre {
    margin: 0;
    padding: 12px;
    background: var(--ink-0);
    border-radius: var(--radius-s);
    font-size: var(--size-s);
    white-space: pre-wrap;
    max-height: 320px;
    overflow: auto;
  }
</style>
