<script lang="ts">
  // What the video says, as cards in the clip column while what was said
  // is searched, #366. A card is laid out the way a clip's card is, its
  // time at the head and its words under it. A clip's cards are its
  // captions, one card a caption, with the words the video preview shows,
  // see engine.SaidCards. They wear the chosen clip's edge, the clip's
  // name and brighter words, and what no clip keeps reads in sentences of
  // a darker grey, so the clips stand out of the transcript they came
  // from. Where nothing was transcribed yet, a dashed
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
    sought = { at: 0, n: 0 },
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
    // Where the playhead was last put, and how many times it was. The list
    // goes to the card there, the way it goes to a clip chosen.
    sought?: { at: number; n: number };
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
    const first = untrack(() => rows.find((r) => "sentence" in r && r.clip?.key === key));
    if (first) toTop(first.key);
  });

  // The playhead put anywhere puts the card it is in at the top, or the
  // next card after it where nothing is said, the way a clip chosen does,
  // found by Tim. Only a gesture: the list does not follow a playing video.
  let soughtWas: number | null = null;
  $effect(() => {
    const { at, n } = sought;
    const was = soughtWas;
    soughtWas = n;
    if (was === null || was === n) return;
    // Of two clips over each other, the card of the chosen one.
    const card = untrack(() => {
      const after = rows.filter((r) => "sentence" in r && r.sentence.end > at);
      const first = after[0];
      if (!first || !("sentence" in first)) return first;
      return after.find((r) => "sentence" in r && r.sentence.start === first.sentence.start && r.clip?.key === selected) ?? first;
    });
    if (card) toTop(card.key);
  });

  // The card of a row, at the top of the list. Where it is
  // is read once and never laid out from: every card above it is laid out
  // at its own height, so the place is exact, and the card stands where
  // the first card of the list stands, under the veil the list fades its
  // ends with.
  function toTop(key: string) {
    const card = list?.querySelector<HTMLElement>(`li[data-row="${CSS.escape(key)}"]`);
    const box = scroller();
    if (!card || !box) return;
    const head = list.firstElementChild as HTMLElement;
    box.scrollTo({ top: card.getBoundingClientRect().top - head.getBoundingClientRect().top });
  }

  type Row =
    | { key: string; at: number; sentence: Sentence; bold: [number, number][]; clip: ClipEntry | null }
    | { key: string; at: number; hole: [number, number] };

  // The clip that shows the card, which the Go side names, see
  // engine.SaidCards.
  function clipOf(s: Sentence): ClipEntry | null {
    return s.clip ? (clips.find((c) => c.key === s.clip) ?? null) : null;
  }

  const rows = $derived.by(() => {
    const out: Row[] = [];
    for (const s of sentences ?? []) {
      const bold = found(s.text, typed);
      // Two clips over each other each have a card of the same words.
      if (bold) out.push({ key: `s${s.start}|${s.clip ?? ""}`, at: s.start, sentence: s, bold, clip: clipOf(s) });
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
        data-row={row.key}
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
    /* Laid out whole, every card, though only a few are in view. Cards
       out of view laid out at a guessed height put the place a clip is
       gone to off once they were laid out, and WebKit keeps no anchor
       that would have made up for it, found by Tim. */
  }

  /* The space between two cards belongs to them, the half above to the
     card above and the half below to the card below, so the hand going
     from one card to the next is never on no card at all. Between two
     cards of one clip, the clip went dark everywhere else for a moment
     and lit again, a flicker, found by Tim. */
  li:not(.none)::before,
  li:not(.none)::after {
    content: "";
    position: absolute;
    left: 0;
    right: 0;
    height: calc(var(--gap) / 2);
  }

  li::before {
    bottom: 100%;
  }

  li::after {
    top: 100%;
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
    border-radius: inherit;
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

  /* What no clip keeps, in the grey of a label, two steps below the words
     of a clip, found by Tim. */
  .text {
    width: 100%;
    color: var(--muted);
    line-height: 18px;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  /* What answers what is typed, in the white of a clip's words and
     heavier, so it stands out of every card, a clip's too. */
  .text b {
    color: var(--text);
    font-weight: 600;
  }

  /* A caption of a clip: the chosen clip's edge, in the app's own
     colour, its name, and its words a step below white. The edge is the button's,
     so the grey it takes under the hand does not cover it. */
  .pick.inclip {
    box-shadow: inset 3px 0 var(--accent);
  }

  .inclip .text {
    color: color-mix(in srgb, var(--text) 60%, var(--muted));
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
