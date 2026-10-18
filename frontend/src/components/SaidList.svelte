<script lang="ts">
  // What the video says, sentence by sentence, as cards in the clip column
  // while what was said is searched, #366. A card is laid out the way a
  // clip's card is, its time at the head and the sentence under it. A
  // sentence a clip holds wears the chosen clip's edge, the clip's name and
  // white words, and every other one a softer grey, so the clips stand out
  // of the transcript they came from. Where nothing was heard yet, a dashed
  // card says so in its place, because the search cannot look there.
  import { clock, type ClipEntry, type Sentence } from "../lib/api";
  import { found } from "../lib/match";

  let {
    sentences,
    typed = "",
    unheard = [],
    clips = [],
    onseek,
  }: {
    // Null until the first answer, so a list on its way is not a list
    // that found nothing.
    sentences: Sentence[] | null;
    typed?: string;
    unheard?: [number, number][];
    clips?: ClipEntry[];
    onseek: (at: number) => void;
  } = $props();

  type Row =
    | { key: string; at: number; sentence: Sentence; bold: [number, number][]; clip: ClipEntry | null }
    | { key: string; at: number; hole: [number, number] };

  // The first clip that keeps any of the sentence.
  function clipOf(s: Sentence): ClipEntry | null {
    for (const c of clips) {
      const parts = c.segments.length ? c.segments : [{ start: c.start, end: c.end }];
      if (parts.some((p) => p.start < s.end && p.end > s.start)) return c;
    }
    return null;
  }

  const rows = $derived.by(() => {
    const out: Row[] = [];
    for (const s of sentences ?? []) {
      const bold = found(s.text, typed);
      if (bold) out.push({ key: `s${s.start}`, at: s.start, sentence: s, bold, clip: clipOf(s) });
    }
    if (sentences) for (const h of unheard) out.push({ key: `h${h[0]}`, at: h[0], hole: h });
    return out.sort((a, b) => a.at - b.at);
  });

  const answered = $derived(rows.some((r) => "sentence" in r));

  // The sentence in pieces, bold where it answers what was typed.
  function pieces(text: string, bold: [number, number][]) {
    const out: { text: string; bold: boolean }[] = [];
    let at = 0;
    for (const [a, b] of bold) {
      if (a > at) out.push({ text: text.slice(at, a), bold: false });
      out.push({ text: text.slice(a, b), bold: true });
      at = b;
    }
    if (at < text.length) out.push({ text: text.slice(at), bold: false });
    return out;
  }
</script>

<ol class="said">
  {#each rows as row (row.key)}
    {#if "hole" in row}
      <li class="hole" title="Nothing was heard here yet, so the search cannot look here. Searching a window here hears it">
        <span class="meta muted num">{clock(row.hole[0])} to {clock(row.hole[1])}</span>
        <span class="text">Not heard yet</span>
      </li>
    {:else}
      <li class:inclip={!!row.clip}>
        <button class="pick" title="Put the playhead here" onclick={() => onseek(row.sentence.start)}>
          <span class="meta muted num">
            {clock(row.sentence.start)}
            {#if row.clip}<span class="clipname">{row.clip.title || row.clip.slug}</span>{/if}
          </span>
          <span class="text">
            {#each pieces(row.sentence.text, row.bold) as p, i (i)}{#if p.bold}<b>{p.text}</b>{:else}{p.text}{/if}{/each}
          </span>
        </button>
      </li>
    {/if}
  {/each}
  {#if sentences && typed.trim() && !answered}
    <li class="none muted">Nothing said matches</li>
  {/if}
</ol>

<style>
  /* The clip list's own column of cards, see ClipList.svelte. */
  ol {
    list-style: none;
    margin: 0;
    padding: var(--veil, 16px) 0;
    display: flex;
    flex-direction: column;
    gap: var(--gap);
  }

  li {
    position: relative;
    flex: none;
    border-radius: var(--radius-m);
    background: var(--ink-1);
    box-shadow: inset 0 0 0 1px var(--ink-3);
    overflow: hidden;
    /* A long video is thousands of sentences, and only the ones in view
       are laid out and drawn. */
    content-visibility: auto;
    contain-intrinsic-size: auto var(--card-h);
  }

  .pick,
  .hole {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    width: 100%;
    min-height: var(--card-h);
    padding: 10px;
    text-align: left;
  }

  .pick {
    height: auto;
    justify-content: center;
    border: none;
    border-radius: 0;
    background: transparent;
  }

  li:hover .pick {
    background: var(--ink-2);
  }

  .meta {
    display: flex;
    align-items: center;
    gap: 6px;
    max-width: 100%;
    line-height: 18px;
  }

  /* What was said, a step softer than the words of a clip. */
  .text {
    width: 100%;
    color: color-mix(in srgb, var(--text) 75%, var(--muted));
    line-height: 18px;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .text b {
    font-weight: 600;
  }

  /* A sentence a clip holds: the chosen clip's edge, in the app's own
     colour, its name, and its words in white. */
  li.inclip {
    box-shadow:
      inset 3px 0 var(--accent),
      inset 0 0 0 1px var(--ink-3);
  }

  li.inclip .text {
    color: var(--text);
  }

  .clipname {
    color: var(--accent-lit);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Where nothing was heard yet, the dashed edge the range picker draws
     round the same part. */
  li.hole {
    justify-content: center;
    box-shadow: none;
    border: 1px dashed var(--faint);
  }

  li.hole .text {
    color: var(--muted);
  }

  li.none {
    display: flex;
    align-items: center;
    justify-content: center;
    height: var(--card-h);
    background: transparent;
    box-shadow: none;
  }
</style>
