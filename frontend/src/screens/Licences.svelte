<script lang="ts">
  import { onMount } from "svelte";
  import { api, type Notice } from "../lib/api";

  // Every piece of other people's work Frame Fairy is made of or brings
  // with it, grouped by where it is, each opening to its licence. The list
  // is built into the app from notices/, see docs/THIRD_PARTY.md.
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
  {#if problem}<p class="error selectable">{problem}</p>{/if}
  {#each parts as group (group.part)}
    <div class="panel">
      <h2>{group.part}</h2>
      <ol>
        {#each group.notices as n (key(n))}
          <li>
            <button class="line" onclick={() => toggle(n)} aria-expanded={open === key(n)}>
              <span class="name">{n.name}</span>
              <span class="muted num">{n.version}</span>
              <span class="grow"></span>
              <span class="muted">{n.licence}</span>
            </button>
            {#if open === key(n)}
              {#if n.note}<p class="muted selectable">{n.note}</p>{/if}
              <p class="muted selectable">{n.url}</p>
              {#each n.texts as name (name)}
                <pre class="selectable">{texts[name] ?? ""}</pre>
              {/each}
            {/if}
          </li>
        {/each}
      </ol>
    </div>
  {/each}
</section>

<style>
  /* The same page as the settings: it keeps its width and stands in the
     middle of the app, and its groups have the settings' heads. */
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

  h2 {
    font-size: var(--size-l);
    font-weight: 600;
  }

  .panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .grow {
    flex: 1;
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  /* A row opens to its licence, the way a finished job opens to its log
     on the activity page, and looks the same doing it. */
  li {
    padding: 8px 0;
    border-top: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .line {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    height: auto;
    min-height: 36px;
    padding: 0;
    background: transparent;
    border: none;
    border-radius: var(--radius-s);
    text-align: left;
  }

  .line:hover {
    background: var(--ink-2);
  }

  .name {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  p {
    overflow-wrap: anywhere;
  }

  pre {
    margin: 0;
    padding: 12px;
    background: var(--ink-1);
    border-radius: var(--radius-m);
    font-size: var(--size-s);
    white-space: pre-wrap;
    max-height: 320px;
    overflow: auto;
  }
</style>
