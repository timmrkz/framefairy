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
    playhead = 0,
    playing = false,
    failed = null,
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
    // Where the playhead is. The list follows it, see below.
    playhead?: number;
    // Whether the video plays. Playing starts with the card of the
    // playhead at the top, wherever the hand had scrolled.
    playing?: boolean;
    // Why the cards could not be read, said in the list itself, the place
    // they would have been in, until the next read works.
    failed?: string | null;
    // How many sentences answer what is typed, for the field to show.
    matches?: number;
    onhover?: (key: string, on: boolean) => void;
    // The part not transcribed yet under the hand, by its place among
    // them, or -1, so the range picker lights the same part.
    onhoverhole?: (index: number) => void;
    // A sentence clicked, with the clip it is named for, if any, or a card
    // walked to with Shift and an arrow, with where it ends.
    onseek: (at: number, clip: string | null, walked?: { end: number }) => void;
  } = $props();

  let list: HTMLOListElement;

  // The column the cards scroll in, which is the clip list's.
  function scroller(): HTMLElement | null {
    return list?.closest<HTMLElement>(".scroll") ?? null;
  }

  // What is typed is a new list, read from its top. A list scrolled half
  // way down before typing showed the answers from half way down. A search
  // let go of, cleared or taken back with Escape, is the playhead's list
  // again, at the card of the playhead: the one it stood at before the
  // search, or the one clicked in it. It went back to the top of the
  // whole transcript, found by Tim.
  let typedWas: string | null = null;
  $effect(() => {
    const now = typed;
    const was = typedWas;
    typedWas = now;
    if (was === null || now === was) return;
    walkedTo = null;
    if (now.trim() || !was.trim()) {
      scroller()?.scrollTo({ top: 0 });
      return;
    }
    untrack(() => follow(true));
  });

  // Playing starts at the card of the playhead. The playhead stood in it
  // already, so nothing moved the list, and the hand had scrolled away:
  // the list only caught up at the next caption, found by Tim.
  $effect(() => {
    if (playing) untrack(() => follow(true));
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

  // The list follows the playhead: the card it comes into stands at the
  // top of the list, whether the hand put it there, on the clip timeline,
  // a caption block or the range picker, or the video plays on into the
  // next caption, found by Tim. Only the playhead moving moves the list,
  // so the hand can scroll it while the video stands. The search opens at
  // the playhead.
  // The row followed is said on the list, for the walks: at the end of
  // the list a row cannot stand at the top, and two rows look the same.
  let followed = $state<string | null | undefined>(undefined);
  $effect(() => {
    void playhead;
    // Cards read anew are followed too: the search opens on the cards it
    // read last, and the ones read as it opens may be cut otherwise.
    void sentences;
    untrack(() => follow(false));
  });

  // The card of the playhead to the top, when the playhead came into
  // another card, or always.
  function follow(always: boolean) {
    const key = rowAt(playhead)?.key ?? null;
    const was = followed;
    followed = key;
    // The search opens at the playhead, or at its top.
    if (!key && (was === undefined || always)) scroller()?.scrollTo({ top: 0 });
    if (!key || (key === was && !always)) return;
    toTop(key);
  }

  // The row the playhead is in. Where nothing is said, the next row, so a
  // clip chosen goes to its first caption, unless the playhead is in a
  // pause between two captions of a clip, where the caption before stays
  // until the next begins. A part not transcribed yet is a row too, so the
  // playhead in one is there and not at the next words heard, found by Tim.
  // Inside the chosen clip, the chosen clip's cards, where another clip
  // over it has cards of the same words.
  function rowAt(t: number): Row | null {
    const chosen = clips.find((c) => c.key === selected);
    const own = chosen && chosen.start <= t && t < chosen.end ? rows.filter((r) => "sentence" in r && r.clip?.key === selected) : [];
    const among = own.length ? own : rows;
    const end = (r: Row) => ("hole" in r ? r.hole[1] : r.sentence.end);
    const i = among.findIndex((r) => end(r) > t);
    if (i < 0) return among.at(-1) ?? null;
    const row = among[i];
    const before = among[i - 1];
    if (row.at > t && before && "sentence" in before && "sentence" in row && before.clip && before.clip.key === row.clip?.key)
      return before;
    return row;
  }

  // Shift and the arrows up and down walk the cards shown, the way they
  // walk the clip list while it is shown, so the answers to a search are
  // gone through from the keyboard, found by Tim. The playhead goes onto
  // each card and the list follows it there. The answers are walked from
  // the first, down, or the last, up, and go round. The whole transcript
  // is walked from the card of the playhead.
  let walkedTo: string | null = null;
  export function walk(step: 1 | -1) {
    if (!rows.length) return;
    let here = -1;
    if (typed.trim()) here = rows.findIndex((r) => r.key === walkedTo);
    else {
      const at = rowAt(playhead);
      here = at ? rows.indexOf(at) : -1;
    }
    const next = here < 0 ? (step > 0 ? 0 : rows.length - 1) : (here + step + rows.length) % rows.length;
    const row = rows[next];
    walkedTo = row.key;
    if ("hole" in row) onseek(row.hole[0], null, { end: row.hole[1] });
    else onseek(row.sentence.start, row.clip?.key ?? null, { end: row.sentence.end });
  }

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

  // The cards that hold what was typed, and only where none does the
  // cards with the word typed with a slip, see near in lib/match.ts. Three
  // letters answer a lot of words the way a font answers its name, and in
  // a video of an hour the cards that hold them were lost among cards
  // that only share their letters, found in the review of #367.
  const rows = $derived.by(() => {
    const out: Row[] = [];
    for (const fuzzy of [false, true]) {
      for (const s of sentences ?? []) {
        const bold = found(s.text, typed, fuzzy);
        // Two clips over each other each have a card of the same words.
        if (bold) out.push({ key: `s${s.start}|${s.clip ?? ""}`, at: s.start, sentence: s, bold, clip: clipOf(s) });
      }
      if (out.length || !typed.trim()) break;
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

<ol class="said" bind:this={list} data-followed={followed}>
  {#if failed}
    <li class="none failed" title={failed}>The transcript could not be read</li>
  {/if}
  {#each rows as row (row.key)}
    {#if "hole" in row}
      {@const index = unheard.indexOf(row.hole)}
      <li
        class="hole"
        data-at={row.hole[0]}
        data-row={row.key}
        title="Nothing was transcribed here yet, so the search cannot look here. Searching a window here transcribes it"
        onpointerenter={() => onhoverhole?.(index)}
        onpointerleave={() => onhoverhole?.(-1)}
      >
        <span class="dots" aria-hidden="true"></span>
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
        <button class="pick" class:inclip={!!row.clip} title="Put the playhead here" onclick={() => {
          // The playhead put on a card is where the list already is.
          followed = row.key;
          onseek(row.sentence.start, row.clip?.key ?? null);
        }}>
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
  {#if sentences && typed.trim() && !answered && !failed}
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

  /* Its dots, faded in from the left, and all of it dotted under the
     hand. The dashed edge stays, so a part only a few pixels wide on the
     range picker still shows, where dots have no room, found by Tim. */
  .dots {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    background: var(--halftone);
    mask-image: linear-gradient(90deg, transparent, #000 70%);
    pointer-events: none;
  }

  li.hole:hover .dots {
    mask-image: none;
  }

  li.hole .meta,
  li.hole .text {
    position: relative;
  }

  li.none {
    display: flex;
    align-items: center;
    justify-content: center;
    height: var(--card-h);
    background: transparent;
    box-shadow: none;
  }

  /* The cards could not be read, in the colour of a warning. */
  li.failed {
    color: var(--warn);
  }
</style>
