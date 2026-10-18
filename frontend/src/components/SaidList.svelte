<script lang="ts">
  // What the video says, sentence by sentence, as cards in the clip column
  // while what was said is searched, #366. A card is laid out the way a
  // clip's card is, its time at the head and the sentence under it. A
  // sentence a clip holds wears the chosen clip's edge, the clip's name and
  // white words, and every other one a softer grey, so the clips stand out
  // of the transcript they came from. Where nothing was transcribed yet, a dashed
  // card says so in its place, because the search cannot look there.
  import { clock, type ClipEntry, type Sentence } from "../lib/api";
  import { found } from "../lib/match";
  import { hoverClip } from "../lib/hover";
  import { untrack } from "svelte";

  let {
    sentences,
    typed = "",
    unheard = [],
    clips = [],
    selected = "",
    chosen = 0,
    matches = $bindable(0),
    onhover,
    onhoverhole,
    onseek,
  }: {
    // Null until the first answer, so a list on its way is not a list
    // that found nothing.
    sentences: Sentence[] | null;
    typed?: string;
    unheard?: [number, number][];
    clips?: ClipEntry[];
    // The chosen clip, which the list goes to when it is chosen somewhere
    // else. The hand on a sentence of a clip lights the clip everywhere
    // else, and only the card under the hand is lit in the list, never
    // every sentence of the clip with it, found by Tim.
    selected?: string;
    // Counts every time a clip is chosen, the one chosen already too.
    chosen?: number;
    // How many sentences answer what is typed, for the field to show.
    matches?: number;
    onhover?: (key: string, on: boolean) => void;
    // The part not transcribed yet under the hand, by its place among
    // them, or -1, so the range picker lights the same part.
    onhoverhole?: (index: number) => void;
    // A sentence clicked, with the clip it is named for, if any.
    onseek: (at: number, clip: string | null) => void;
  } = $props();

  let list: HTMLOListElement;

  // The column the cards scroll in, which is the clip list's.
  function scroller(): HTMLElement | null {
    return list?.closest<HTMLElement>(".scroll") ?? null;
  }

  // What is typed is a new list, read from its top. A list scrolled half
  // way down before typing showed the answers from half way down.
  let typedWas: string | null = null;
  $effect(() => {
    const now = typed;
    if (typedWas !== null && now !== typedWas) scroller()?.scrollTo({ top: 0 });
    typedWas = now;
  });

  // A clip chosen on the range picker or the clip timeline while the
  // search is open puts its first sentence at the top of the list, the
  // way choosing it brings its card into view in the clip list. The first
  // card it keeps, which may be named for another clip over it.
  // Choosing it again does it again, after the hand scrolled away.
  let chosenWas: number | null = null;
  $effect(() => {
    const times = chosen;
    const key = selected;
    const was = chosenWas;
    chosenWas = times;
    // Only a clip chosen, not a clip that became the chosen one some
    // other way, like a sentence of it clicked here.
    if (was === null || was === times) return;
    const clip = untrack(() => clips.find((c) => c.key === key));
    if (!clip) return;
    const parts = clip.segments.length ? clip.segments : [{ start: clip.start, end: clip.end }];
    const first = untrack(() =>
      rows.find((r) => "sentence" in r && parts.some((p) => p.start <= (r.sentence.start + r.sentence.end) / 2 && (r.sentence.start + r.sentence.end) / 2 < p.end)),
    );
    if (!first || !("sentence" in first)) return;
    const card = list?.querySelector<HTMLElement>(`li[data-at="${first.sentence.start}"]`);
    const box = scroller();
    if (!card || !box) return;
    // Where the card is, read once and never laid out from: every card
    // above it is laid out at its own height, so the place is exact, and
    // the card stands where the first card of the list stands, under the
    // veil the list fades its ends with.
    const head = list.firstElementChild as HTMLElement;
    const top = card.getBoundingClientRect().top - head.getBoundingClientRect().top;
    box.scrollTo({ top });
  });

  type Row =
    | { key: string; at: number; sentence: Sentence; bold: [number, number][]; clip: ClipEntry | null }
    | { key: string; at: number; hole: [number, number] };

  // The clip that keeps the card. A card is cut at the edges of what the
  // clips keep, see engine.Sentences, so it is all in a clip or all out,
  // and its middle says which.
  function clipOf(s: Sentence): ClipEntry | null {
    const at = (s.start + s.end) / 2;
    for (const c of clips) {
      const parts = c.segments.length ? c.segments : [{ start: c.start, end: c.end }];
      if (parts.some((p) => p.start <= at && at < p.end)) return c;
    }
    return null;
  }

  const rows = $derived.by(() => {
    const out: Row[] = [];
    for (const s of sentences ?? []) {
      const bold = found(s.text, typed);
      if (bold) out.push({ key: `s${s.start}`, at: s.start, sentence: s, bold, clip: clipOf(s) });
    }
    // Where nothing was transcribed yet is said while the whole of what
    // was said is shown. It is no answer to what is typed, found by Tim.
    if (sentences && !typed.trim()) for (const h of unheard) out.push({ key: `h${h[0]}`, at: h[0], hole: h });
    return out.sort((a, b) => a.at - b.at);
  });

  const answered = $derived(rows.filter((r) => "sentence" in r).length);
  $effect(() => {
    matches = answered;
  });

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

<ol class="said" bind:this={list}>
  {#each rows as row (row.key)}
    {#if "hole" in row}
      {@const index = unheard.indexOf(row.hole)}
      <li
        class="hole"
        title="Nothing was transcribed here yet, so the search cannot look here. Searching a window here transcribes it"
        onpointerenter={() => onhoverhole?.(index)}
        onpointerleave={() => onhoverhole?.(-1)}
      >
        <span class="meta muted num">{clock(row.hole[0])} to {clock(row.hole[1])}</span>
        <span class="text">Not transcribed yet</span>
      </li>
    {:else}
      <li
        data-at={row.sentence.start}
        data-clip={row.clip?.key}
        {@attach row.clip ? hoverClip(row.clip.key, onhover) : undefined}
      >
        <button class="pick" class:inclip={!!row.clip} title="Put the playhead here" onclick={() => onseek(row.sentence.start, row.clip?.key ?? null)}>
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
    <li class="none muted">No matches</li>
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
    /* Laid out whole, every card, though only a few are in view. Cards
       out of view laid out at a guessed height put the place a clip is
       gone to off once they were laid out, and WebKit keeps no anchor
       that would have made up for it, found by Tim. */
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
    color: color-mix(in srgb, var(--text) 50%, var(--muted));
    line-height: 18px;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .text b {
    font-weight: 600;
  }

  /* A sentence a clip holds: the chosen clip's edge, in the app's own
     colour, its name, and its words in white. The edge is the button's,
     so the grey it takes under the hand does not cover it. */
  .pick.inclip {
    box-shadow: inset 3px 0 var(--accent);
  }

  .inclip .text {
    color: var(--text);
  }

  .clipname {
    color: var(--accent-lit);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Where nothing was transcribed yet, a card like any other with its
     edge dashed, the edge the range picker draws round the same part.
     Under the hand it fills with the grey of every card under the hand,
     which the dashes go into, the way a card's edge does. */
  li.hole {
    justify-content: center;
    box-shadow: none;
    border: 1px dashed var(--ink-3);
  }

  li.hole:hover {
    background: var(--ink-2);
    border-color: var(--ink-2);
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
