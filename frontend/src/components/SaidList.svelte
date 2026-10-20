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
    unheardLit = -1,
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
    // The part not transcribed yet under the hand on the range picker,
    // whose card is lit with it.
    unheardLit?: number;
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

  // The card that stays where it stands as cards come in, see below.
  let anchor: { key: string; top: number; typed: string } | null = null;

  // The column the cards scroll in, which is the clip list's.
  function scroller(): HTMLElement | null {
    return list?.closest<HTMLElement>(".scroll") ?? null;
  }

  // The anchor put back where it stood. It runs before the effects that
  // move the list, so they move it from where the hand left it.
  $effect(() => {
    void rows;
    untrack(() => {
      const was = anchor;
      anchor = null;
      if (!was || was.typed !== typed) return;
      const card = list?.querySelector<HTMLElement>(`li[data-row="${CSS.escape(was.key)}"]`);
      const box = scroller();
      if (card && box) box.scrollTop += card.getBoundingClientRect().top - was.top;
    });
  });

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
  // search is open brings its first sentence into view, the way choosing
  // it brings its card into view in the clip list. The first card it
  // keeps, which may be named for another clip over it. Choosing it again
  // does it again, after the hand scrolled away.
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
    if (first) turnTo(first.key);
  });

  // The list follows the playhead the way the clip timeline does, by
  // turning the page: the card the playhead comes into is the current
  // one, marked the way the chosen clip is in the clip list, and while it
  // is in view the list stands still. A card out of view, or going out of
  // it at the foot as the video plays on, comes to the top, found by Tim.
  // That is so wherever the playhead was put, on the clip timeline, a
  // caption block or the range picker. Only the playhead moving moves the
  // list, so the hand can scroll it while the video stands. The search
  // opens at the playhead.
  // The row followed is said on the list, for the walks: at the end of
  // the list a row cannot stand at the top, and two rows look the same.
  let followed = $state<string | null | undefined>(undefined);
  // The playhead put somewhere while the video stands brings its card into
  // view even when it stays in the same card, the way choosing a clip
  // again does: the hand may have scrolled the card away.
  let playheadWas: number | null = null;
  $effect(() => {
    const at = playhead;
    // Cards read anew are followed too: the search opens on the cards it
    // read last, and the ones read as it opens may be cut otherwise.
    void sentences;
    const put = playheadWas !== null && at !== playheadWas && !untrack(() => playing);
    playheadWas = at;
    untrack(() => follow(false, put));
  });

  // The card of the playhead into view, when the playhead came into
  // another card, or always. A card walked to turns the page the way
  // playing does, in the way it was walked, see pageTo.
  let walking: 0 | 1 | -1 = 0;
  function follow(always: boolean, put = false) {
    const step = walking;
    walking = 0;
    const key = rowAt(playhead)?.key ?? null;
    const was = followed;
    followed = key;
    // The search opens at the playhead, or at its top.
    if (!key && (was === undefined || always)) scroller()?.scrollTo({ top: 0 });
    if (!key || (key === was && !always && !put)) return;
    if (step) pageTo(key, step);
    else turnTo(key);
  }

  // The card the playhead is in, marked as the current one. In a search,
  // only an answer the playhead is in, so a mark never says the playhead
  // is somewhere it is not.
  const current = $derived.by(() => {
    const row = rows.find((r) => r.key === followed);
    if (!row || !typed.trim()) return followed;
    const end = "hole" in row ? row.hole[1] : row.sentence.end;
    return row.at <= playhead + 0.001 && playhead < end ? followed : null;
  });

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
    let i = among.findIndex((r) => end(r) > t);
    if (i < 0) return among.at(-1) ?? null;
    // Two rows can hold the same moment at their edge: a card heard up to
    // the edge of what was heard can run on a little past it, into the part
    // not transcribed yet after it. The playhead is then in the row that
    // began last, so a step onto that part lands in it. In the card before,
    // Shift and the down arrow stood still there and the up arrow went past
    // it, found by Tim.
    while (among[i + 1] && among[i + 1].at <= t && end(among[i + 1]) > t) i++;
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
  // The walk goes on from the current card, so the one marked is where
  // the next step starts from.
  export function walk(step: 1 | -1) {
    if (!rows.length) return;
    const here = rows.findIndex((r) => r.key === current);
    const next = here < 0 ? (step > 0 ? 0 : rows.length - 1) : (here + step + rows.length) % rows.length;
    const row = rows[next];
    walking = step;
    if ("hole" in row) onseek(row.hole[0], null, { end: row.hole[1] });
    else onseek(row.sentence.start, row.clip?.key ?? null, { end: row.sentence.end });
  }

  // Whether the card of a row is in view, inside the veil the list fades
  // its ends with.
  function shows(key: string): { card: HTMLElement; box: HTMLElement; veil: number; above: number; below: number } | null {
    const card = list?.querySelector<HTMLElement>(`li[data-row="${CSS.escape(key)}"]`);
    const box = scroller();
    if (!card || !box) return null;
    const veil = parseFloat(getComputedStyle(list).paddingTop) || 0;
    const r = card.getBoundingClientRect();
    const v = box.getBoundingClientRect();
    return { card, box, veil, above: v.top + veil - r.top, below: r.bottom - (v.bottom - veil) };
  }

  // The page turned to the card of a row, unless it is in view.
  function turnTo(key: string) {
    const at = shows(key);
    if (at && at.above <= 0.5 && at.below <= 0.5) return;
    toTop(key);
  }

  // The page turned to a card walked to, unless it is in view: walked
  // down, it comes to the top, the way the next card does as the video
  // plays, and walked up, to the foot, so the page before is shown. It
  // was brought just into view, and went on standing at the foot as the
  // walk went down, found by Tim.
  function pageTo(key: string, step: 1 | -1) {
    const at = shows(key);
    if (!at || (at.above <= 0.5 && at.below <= 0.5)) return;
    if (step > 0) toTop(key);
    else at.box.scrollTop += at.below;
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
    // Known by their place among them, not by where they start, which
    // moves as the work hears.
    if (sentences && !typed.trim()) for (const [i, h] of unheard.entries()) out.push({ key: `h${i}`, at: h[0], hole: h });
    return out.sort((a, b) => a.at - b.at);
  });

  // Cards that come in above the ones in view, as the transcript grows
  // over the part the playhead stands in, leave the ones in view where
  // they stand. WebKit keeps no anchor of its own, so the card of the
  // playhead was pushed out at the foot while it stayed the same card,
  // and nothing brought it back. The anchor is the card of the playhead
  // when it is in view, or else the first card in view. A new search is
  // a new list and keeps nothing.
  $effect.pre(() => {
    void rows;
    anchor = untrack(() => {
      const box = scroller();
      if (!box || !list) return null;
      const v = box.getBoundingClientRect();
      const shown = (li: HTMLElement) => {
        const r = li.getBoundingClientRect();
        return r.bottom > v.top && r.top < v.bottom;
      };
      const mine = followed ? list.querySelector<HTMLElement>(`li[data-row="${CSS.escape(followed)}"]`) : null;
      const card = mine && shown(mine) ? mine : [...list.querySelectorAll<HTMLElement>("li[data-row]")].find(shown);
      return card ? { key: card.dataset.row!, top: card.getBoundingClientRect().top, typed } : null;
    });
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
        class:lit={index === unheardLit}
        class:current={row.key === current}
        data-at={row.hole[0]}
        data-row={row.key}
        title="Nothing was transcribed here yet, so the search cannot look here. Searching a window here transcribes it"
        onpointerenter={() => onhoverhole?.(index)}
        onpointerleave={() => onhoverhole?.(-1)}
      >
        <span class="face" aria-hidden="true">
          <span class="dots"></span>
          <span class="ring"></span>
        </span>
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
        <button class="pick" class:inclip={!!row.clip} class:clip-edge={!!row.clip} class:current={row.key === current} title="Put the playhead here" onclick={() => {
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

  /* A caption of a clip: the chosen clip's edge, .clip-edge in app.css,
     its name, and its words a step below white. The edge is the
     button's, so the grey it takes under the hand does not cover it. */
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
     edge dashed, see .ring. Under the hand it fills with the grey of
     every card under the hand, which the dashes go into, the way a
     card's edge does. */
  li.hole {
    justify-content: center;
    background: transparent;
    box-shadow: none;
    border: 1px solid transparent;
  }

  li.hole:hover,
  li.hole.lit {
    background: var(--ink-2);
  }

  /* The card the playhead is in, the way the chosen clip's card is in the
     clip list, so a walk through three answers shows where it is, found
     by Tim. */
  .pick.current,
  li:hover .pick.current,
  li.hole.current {
    background: var(--ink-3);
  }

  li.hole .text {
    color: var(--muted);
  }

  /* Its dots and its dashes, on one grid of --dot-pitch: whole columns
     and rows of dots, centred in the card with --room at least on every
     side, and a dash on the edge over each column and beside each row.
     Dashes the browser drew went their own way round the dots, dots on
     to the edge sat too close to the dashes, and dots a whole tile in
     looked cut off, all found by Tim. The grid is worked out from the
     card's own size, the face being a container of it, so the dots and
     the dashes are laid from the same numbers and the same either side.
     A side can take half a pixel, which is a whole pixel on a Retina
     screen. The dots rise from the foot on nothing else. Under the hand
     the card fills with a card's grey, the dots go up a step and keep
     their fade, and the dashes go into the grey. */
  .face {
    --room: 2px;
    --columns: round(down, 100cqw - 2 * var(--room), var(--dot-pitch));
    --rows: round(down, 100cqh - 2 * var(--room), var(--dot-pitch));
    --left: calc((100cqw - var(--columns)) / 2);
    --top: calc((100cqh - var(--rows)) / 2);
    position: absolute;
    inset: -1px;
    border-radius: inherit;
    pointer-events: none;
    container-type: size;
  }

  .dots {
    position: absolute;
    left: var(--left);
    top: var(--top);
    width: var(--columns);
    height: var(--rows);
    background: var(--halftone) 0 0 / var(--dot-pitch) var(--dot-pitch);
    mask-image: var(--halftone-fade);
  }

  li.hole:hover .dots,
  li.hole.lit .dots,
  li.hole.current .dots {
    background-image: var(--halftone-lit);
  }

  /* The edge is solid and its mask makes the dashes, a run of them as
     long as the dots' columns along the top and the foot, and as long as
     their rows down the sides, so no dash falls outside the grid. */
  .ring {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    border: 1px solid var(--ink-3);
    mask-image: var(--halftone-dash-x), var(--halftone-dash-y);
    mask-size:
      var(--columns) 100%,
      100% var(--rows);
    mask-position:
      var(--left) 0,
      0 var(--top);
    mask-repeat: no-repeat;
  }

  li.hole:hover .ring,
  li.hole.lit .ring {
    border-color: var(--ink-2);
  }

  li.hole.current .ring {
    border-color: var(--ink-3);
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
