<script lang="ts">
  // The whole episode as one slim track: the ruler, the clips as marks and
  // the playhead, which a press anywhere takes hold of. While clips are
  // being found, the window they are found in is drawn over the track.
  // At rest the window is not drawn at all, only marked at its corners: a
  // bar on the top border and one on the bottom border, over them, with a
  // triangle at each end whose tip reaches into the track.
  // Laid over a short episode searched whole, the window hid every clip
  // just found, and a frame round it read as the same window, so nothing
  // of the mark lies over the track but the tips. It says where Search looks
  // next, which the app decides, see nextWindow in lib/flow.ts, or where
  // Continue carries on a search that stopped. Moved off that, the search
  // is set aside and Search starts afresh where it is, see aside in
  // Episode.svelte. A triangle
  // drags its edge, a bar drags the whole window, and a double-click puts
  // it back where the app would have it. While clips are found the mark
  // turns into the window, and back into the mark when they are. Nothing
  // on the range picker says what has been searched: any part can be
  // searched, as often as anyone likes.
  import { onMount } from "svelte";
  import { fade } from "svelte/transition";
  import { clock } from "../lib/api";
  import { gridStep } from "../lib/flow";
  import { hoverClip } from "../lib/hover";
  import Busy from "./Busy.svelte";
  import Info from "./Info.svelte";
  import { scrub as scrubPlayhead } from "../lib/scrub";
  import { fitsAt, pickerSteps, rulerStep, timeWidth } from "../lib/ruler";

  let {
    duration,
    from = 0,
    to = 0,
    shown = false,
    marks = [],
    selected = "",
    hovered = "",
    onhover,
    onmark,
    playhead = -1,
    onseek,
    onhold,
    dimmed = false,
    locked = false,
    onmove,
    onreset,
    grid = $bindable(0),
    least = 0,
    leastSays = "",
    most = Infinity,
    reachSays = "",
    resumes = false,
    unheard = [],
    unheardLit = -1,
    unheardOn = -1,
    giving = -1,
    part = null,
    partWaiting = false,
    partKey = "",
    onhoverunheard,
  }: {
    duration: number;
    // The window of the search in hand, drawn while shown.
    from?: number;
    to?: number;
    shown?: boolean;
    marks?: {
      key: string;
      start: number;
      end: number;
      rendered: boolean;
      // A clip on its way, where its card says it lies, and the key that
      // chooses its card. It keeps its mark when it is written.
      arriving?: boolean;
      pick?: string;
    }[];
    selected?: string;
    // The clip under the hand, here, on the clip timeline or in the clip
    // list. Its mark is lit the way it is under the pointer.
    hovered?: string;
    onhover?: (key: string, on: boolean) => void;
    onmark?: (key: string) => void;
    playhead?: number;
    onseek?: (time: number) => void;
    // The hand has taken hold of the playhead, or let go of it, so a play
    // can wait under the hand, see hold in Player.svelte.
    onhold?: (held: boolean) => void;
    // Whether the playhead is on the video rather than on the chosen clip,
    // so the chosen clip's mark is drawn dimmed, as its frame is.
    dimmed?: boolean;
    // Clips are being found for the window, which wears the shimmer.
    locked?: boolean;
    // The window at rest dragged by its outline, with where its edges are
    // now, while the hand moves, and done when it lets go. What it may be
    // is the workspace's to say, which gives back where it put it.
    onmove?: (from: number, to: number, done: boolean) => void;
    // A double-click on the outline, which puts the window back where the
    // app would have it.
    onreset?: () => void;
    // The round step an edge lands on, see gridStep, 0 until the track has
    // been measured.
    grid?: number;
    // The shortest the window may be, with room for the clips asked for,
    // and the longest, as much as the model reads at once, each with what
    // it is in words, said on the handle that runs into it.
    least?: number;
    leastSays?: string;
    most?: number;
    reachSays?: string;
    // A search stands stopped on the window, so the mark is where Continue
    // carries it on rather than where Search looks.
    resumes?: boolean;
    // The parts of the video not transcribed yet, always, so where a search
    // has to transcribe first, and how much is left to Transcribe whole
    // video, is seen without opening the cards of what was said. Tim
    // picked dots on what is not transcribed over dots on what is: a
    // video transcribed to its end is a plain range picker, issue #432.
    unheard?: [number, number][];
    // The one of them whose card is under the hand, lit a step.
    unheardLit?: number;
    // The one whose card the playhead is in, lit the same way.
    unheardOn?: number;
    // Where the work is hearing now, or -1. The part that starts there
    // gives way as the work hears, so its dots stand still while its edge
    // passes over them.
    giving?: number;
    // The part of the video Transcribe whole video works on, or nothing.
    // It is framed, breathing and shedding the motes, the way the window
    // of a search is while it hears, over the dots of that part, which
    // still say what is not transcribed yet. Tim's pick on #458.
    // When it goes, on Stop transcribing, everything in it stands still
    // where it is and it fades, so nothing moves on after the press.
    // While the work waits its turn, behind a search, a clip made by hand
    // or anything else that has the hearing, it keeps its breath, dimmed,
    // and sheds no motes: the work is on hold, not gone. Tim's report of
    // 23 October. partKey names the work, so the frame of the next work
    // is a frame of its own and never the one still fading.
    part?: [number, number] | null;
    partWaiting?: boolean;
    partKey?: string;
    // The one of them under the hand here, by its place among them, or -1,
    // so its card is lit too, while the cards are open.
    onhoverunheard?: (index: number) => void;
  } = $props();

  // What the mark is, in its titles.
  const says = $derived(resumes ? "Where Continue carries on the search that stopped" : "Where Search looks next");

  let track: HTMLDivElement;
  let width = $state(0);

  // Everything on the track is placed by the stylesheet, in whole pixels.
  // In shares of the width the window and its edges land on halves of a
  // pixel, which is what made one border of the window look thicker than
  // the others, so a place is its share of the track's width rounded,
  // round(share * 100cqw, 1px), see .track and .over. It was worked out
  // here from the width a ResizeObserver read, which comes a frame after
  // the layout, so while the app was resized every mark, the window, the
  // playhead and the minutes moved a frame behind the track they stand
  // on. The stylesheet places them in the same pass as the track.
  // share is where a moment is, as a share of the episode. inner is the
  // track's own width from inside .over, which is its border wider.
  function share(t: number): number {
    return Math.min(Math.max(t, 0), duration) / Math.max(duration, 0.001);
  }
  const inner = "(100cqw - 2px)";
  // Pixels a second, for a drag, which reads it as the hand moves.
  const scale = $derived(width / Math.max(duration, 0.001));
  function on(t: number, of = "100cqw"): string {
    return `round(${share(t)} * ${of}, 1px)`;
  }

  const whole = $derived(from <= 0.5 && to >= duration - 0.5);

  // Where the window is drawn, beside the track rather than in it, in the
  // box the track and its border fill. An edge on a minute is drawn on
  // that minute's line: the line is the pixel after the moment's own, the
  // track's border being the first, so the window reaches over it at its
  // end and starts on it at its start, and both edges sit on their lines
  // alike. It was one short at the end, so the end missed its line and
  // the start did not. A window that starts or ends with the episode
  // reaches the outside of the border and takes its round corner, so it
  // lies on the range picker's own edge rather than inside it.
  function frameOver(a: number, b: number) {
    const first = a <= 0.5;
    const last = b >= duration - 0.5;
    const left = first ? "0px" : `(${on(a, inner)} + 1px)`;
    const right = last ? "100cqw" : `(${on(b, inner)} + 2px)`;
    return { style: `left: calc(${left}); width: max(calc(${right} - ${left}), 0px)`, first, last };
  }
  const frame = $derived(frameOver(from, to));
  // The part Transcribe whole video works on, framed the way the window of
  // a search is while it hears, Tim's pick on #458.
  const partFrame = $derived(part ? frameOver(part[0], part[1]) : null);
  // The part's frame going: its breath and its motes stop where they are,
  // and it fades from however bright the breath had it. The fade is an
  // animation of its own over the breath's, so it starts from there and
  // does not jump to full first. Slower than a toast's, because it is a
  // frame leaving the middle of the workspace, not a note in its corner,
  // and Tim found it went abruptly. It is the part's alone: the search's
  // window goes in the frame its search ends.
  function going(node: Element) {
    node.classList.add("going");
    return fade(node, { duration: 400 });
  }
  onMount(() => {
    // Nothing measured while the workspace is put away, see App.svelte.
    const observer = new ResizeObserver(() => {
      if (track.isConnected && track.clientWidth) width = track.clientWidth;
    });
    observer.observe(track);
    return () => observer.disconnect();
  });

  // Dragging the playhead, the same way as on the clip timeline, see
  // lib/scrub.ts. A press anywhere on the track takes hold of it, and so
  // does the line past the track's border, and the video preview follows
  // the hand.
  let scrubbing = $state(false);

  function scrub(event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    // Keeping the press from selecting text also keeps it from taking the
    // focus away, which every other click in the app does. A field that
    // kept it, Target say, took the I and O meant for the playhead just
    // put here.
    const focused = document.activeElement as HTMLElement | null;
    if (focused && focused !== document.body) focused.blur();
    scrubPlayhead(
      event,
      timeAt,
      (t) => onseek?.(t),
      (held) => {
        scrubbing = held;
        onhold?.(held);
      },
    );
  }

  // An edge dragged lands on a round step, so a window is something that
  // can be said out loud: the smallest round step still about eight pixels
  // wide on the track, five minutes on a four hour episode and five seconds
  // on a six minute one. The ends of the episode win over it. The window
  // snapped this way before it was taken away, and Tim missed it.
  // The workspace places its own windows on the same step, see onGrid in
  // lib/flow.ts, so it is handed back to it.
  $effect(() => {
    grid = width > 0 && duration > 0 ? gridStep(duration, width) : 0;
  });

  function round(t: number): number {
    return grid > 0 ? Math.round(t / grid) * grid : Math.round(t);
  }

  // Dragging the window at rest by its outline: a bar moves all of it, a
  // handle one edge. It follows the hand the whole way, so the Search button's
  // title and the Target's suggestion say where it is while it moves.
  let moving = $state<"" | "move" | "from" | "to">("");

  function grabWindow(what: "move" | "from" | "to", event: PointerEvent) {
    if (event.button !== 0 || !onmove) return;
    event.preventDefault();
    event.stopPropagation();
    const focused = document.activeElement as HTMLElement | null;
    if (focused && focused !== document.body) focused.blur();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startX = event.clientX;
    const was = { from, to };
    const place = (clientX: number): [number, number] => {
      const by = (clientX - startX) / Math.max(scale, 1e-9);
      if (what === "from") return [startAt(was.to, Math.max(round(was.from + by), 0)), was.to];
      if (what === "to") return [was.from, endAt(was.from, Math.min(round(was.to + by), duration))];
      // Moved whole, the start lands on the step and the window keeps
      // its length.
      const size = was.to - was.from;
      const start = Math.min(Math.max(round(was.from + by), 0), Math.max(duration - size, 0));
      return [start, start + size];
    };
    moving = what;
    const move = (e: PointerEvent) => {
      const was = held;
      held = "";
      const placed = place(e.clientX);
      if (held && held !== was) knocked();
      onmove?.(...placed, false);
    };
    const up = (e: PointerEvent) => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      moving = "";
      onmove?.(...place(e.clientX), true);
      held = "";
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // The shortest a window may be, never longer than the episode, and the
  // furthest an end may go from its start, as far as the model reads.
  const shortest = $derived(Math.min(Math.max(least, 0), duration));

  // What holds a handle back while it is dragged, so the marks can say so.
  let held = $state<"" | "least" | "reach">("");

  // An edge held back stops exactly at its limit. The limit is a wall, and
  // a wall wins over the step: stopping at the step before it would give
  // away room the model has, or leave too little for the clips.
  function endAt(start: number, want: number): number {
    const lo = Math.min(start + shortest, duration);
    const hi = Math.min(start + most, duration);
    if (want > hi) {
      held = "reach";
      return hi;
    }
    if (want < lo) {
      held = "least";
      return lo;
    }
    return want;
  }

  function startAt(end: number, want: number): number {
    const lo = Math.max(end - most, 0);
    const hi = Math.max(end - shortest, 0);
    if (want < lo) {
      held = "reach";
      return lo;
    }
    if (want > hi) {
      held = "least";
      return hi;
    }
    return want;
  }

  // The moment a handle runs into a limit, the marks flash twice in the
  // colour of a warning, so a hand that keeps pulling knows it is the
  // limit and not the app that stopped. Once each time it runs in, not for
  // as long as it is held there. The colour changes outright, on and off,
  // rather than fading, so no colour is ever mixed half way between the
  // red and the accent. The window flashed this way before it was taken
  // away, and Tim missed it.
  let flashing = $state(false);
  let flashes: ReturnType<typeof setTimeout>[] = [];
  function knocked() {
    flashes.forEach(clearTimeout);
    flashing = true;
    flashes = [
      setTimeout(() => (flashing = false), 90),
      setTimeout(() => (flashing = true), 170),
      setTimeout(() => (flashing = false), 260),
    ];
  }

  function resetWindow(event: MouseEvent) {
    event.stopPropagation();
    onreset?.();
  }

  // The part not transcribed yet under the hand lights its card in the
  // list, the way a clip's mark lights its card, found by Tim. The parts
  // take no pointer, so a press on one is a press on the track, and which
  // is under the hand is read from where it is. A mark over a part is the
  // mark's.
  let unheardHere = -1;
  function hoverUnheard(event: PointerEvent) {
    const over = event.type === "pointerleave" || !track || (event.target as Element | null)?.closest?.(".clipmark") ? -1 : timeAt(event.clientX);
    const i = over < 0 ? -1 : unheard.findIndex(([a, b]) => a <= over && over < b);
    if (i === unheardHere) return;
    unheardHere = i;
    onhoverunheard?.(i);
  }

  function timeAt(clientX: number): number {
    const box = track.getBoundingClientRect();
    const share = Math.max(0, Math.min(1, (clientX - box.left) / box.width));
    return share * duration;
  }

  // A time every so often, the smallest step that leaves room for the
  // times, and each written where it ends clear of the track's edge, see
  // lib/ruler.ts. Which there are is decided from the width, a frame after
  // a resize, and where they stand by the stylesheet, at once.
  const ticks = $derived.by(() => {
    if (!duration || !width) return [];
    const label = timeWidth(clock(duration));
    const step = rulerStep(duration, width, label, pickerSteps(duration));
    const out: { t: number; label: boolean }[] = [];
    for (let t = step; t < duration; t += step) {
      out.push({ t, label: fitsAt(share(t) * width, timeWidth(clock(t)), width) });
    }
    return out;
  });

</script>

<!-- The playhead is drawn over the track rather than in it. The track
     clips what is inside it, which is what keeps the marks inside its
     rounded corners, and the playhead is the one thing that has to reach
     past them: it runs on past the track's top and bottom border, and at
     the very start or the very end it would otherwise be cut off by the
     corner. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="over" onpointermove={unheard.length && onhoverunheard ? hoverUnheard : undefined} onpointerleave={unheard.length && onhoverunheard ? hoverUnheard : undefined}>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="track asks"
  class:locked
  class:scrubbing
  bind:this={track}
  onpointerdown={scrub}
  aria-label="The range picker"
>
  <!-- What has not been heard yet, dots under the minutes so their lines
       still show through. -->
  {#each unheard as [a, b], i (i)}
    <div
      class="unheard"
      class:lit={i === unheardLit || i === unheardOn}
      class:giving={giving >= 0 && Math.abs(a - giving) < 0.5}
      style="--at: {on(a)}; --to: {on(b)}"
    ></div>
  {/each}
  <!-- Every clip is a mark, and a mark is pressed to work on its clip. -->
  {#each marks as m (m.key)}
    <button
      class="clipmark"
      class:rendered={m.rendered}
      class:selected={(m.pick ?? m.key) === selected}
      class:dim={dimmed && (m.pick ?? m.key) === selected}
      class:lit={m.key === hovered}
      class:waiting={m.arriving}
      style="left: {on(m.start)}; width: max(calc({on(m.end)} - {on(m.start)}), 4px)"
      aria-label="Clip at {clock(m.start)}"
      {@attach hoverClip(m.key, onhover)}
      onpointerdown={(e) => e.stopPropagation()}
      onclick={() => onmark?.(m.pick ?? m.key)}
    ></button>
  {/each}
  <!-- The ruler, in two layers, and they have to be two.

       The line goes under the window, because a minute that falls on the
       window's edge would otherwise paint over half of its border. The
       time goes over everything, because it says where you are on the
       track and nothing laid over the track may take that away.

       A time written inside its own line cannot have both: an element with
       a z-index makes a stacking context, so the time would be held at the
       line's level however high its own is, and the window drawn over it
       would swallow it. -->
  {#each ticks as tick (tick.t)}
    <div class="tick" style="left: {on(tick.t)}"></div>
  {/each}
  {#each ticks as tick (tick.t)}
    {#if tick.label}
      <span class="num time" style="left: {on(tick.t)}">{clock(tick.t)}</span>
    {/if}
  {/each}
  <span class="num time start">0:00</span>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <span class="ask corner" onpointerdown={(e) => e.stopPropagation()}>
    <Info label="What the range picker is" side="right">
      The whole video, with its clips as marks. Press or drag anywhere to move the playhead, and
      press a mark to work on its clip. The marks at the four corners are where Search looks next, or
      where Continue carries on a search that stopped: drag a bar between them to look somewhere
      else, drag a triangle to make it shorter or longer, and double-click to put it back. Moved
      off a search that stopped, Search starts afresh where they are. It is at least as long as its clips need and at most as long as
      the model reads at once, and flashes red when a triangle runs into either. While clips are
      being found, the window is drawn over the track. How far a search has come is on its cards in
      the clip list.
    </Info>
  </span>
</div>
<!-- Only looked at, never taken hold of: the window of a search in hand
     is the search's, and a press on it is a press on the track. Drawn
     beside the track rather than in it, over its border, because the
     track clips what is inside it to its round corners and cut into the
     window and its breath. The part Transcribe whole video works on is
     drawn the same way, by the same lines, because it is the same thing:
     work in hand on a stretch of the video. -->
{#snippet stretch(f: { style: string; first: boolean; last: boolean }, busy: boolean, all: boolean, behind: boolean)}
  <div
    class="window frame"
    class:waiting={busy}
    class:locked={busy}
    class:whole={all}
    class:behind
    class:first={f.first}
    class:last={f.last}
    style={f.style}
  >
    <!-- The motes of work in hand rise through it while its clips are
         found, the ones every control of the app sheds, with no rim and no
         fill: how far the search has come is said on the clip cards, and
         only there. -->
    {#if busy && !behind}<Busy rim={false} />{/if}
  </div>
{/snippet}
{#if shown}
  {@render stretch(frame, locked, whole, false)}
{/if}
{#if partFrame}
  {#key partKey}
    <div class="part" out:going|global>
      {@render stretch(partFrame, true, false, partWaiting)}
    </div>
  {/key}
{/if}
{#if !shown && to > from && duration > 0}
  <!-- The window at rest, marked at its four corners and not drawn over
       the track. A bar on the top border and one on the bottom border
       join the marks, and past them only the tips of the triangles reach
       into the track, so nothing lies over a clip. It is
       drawn beside the track rather than in it, because the track clips
       what is inside it. The four triangles are the handles, the left two
       for where the window starts and the right two for where it ends, and
       a bar moves the whole of it. -->
  <div
    class="aim"
    class:moving={moving !== ""}
    class:flashing
    style={frame.style}
  >
    {#each ["top", "bottom"] as side (side)}
      <div
        class="bar {side}"
        class:held={moving === "move"}
        role="slider"
        tabindex="-1"
        aria-label={says}
        aria-valuenow={from}
        title="{says}, {clock(from)} to {clock(to)}. Drag to look somewhere else, double-click to put it back"
        onpointerdown={(e) => grabWindow("move", e)}
        ondblclick={resetWindow}
      ></div>
      {#each ["from", "to"] as const as edge (edge)}
        <div
          class="tip {side} {edge}"
          role="slider"
          tabindex="-1"
          aria-label={edge === "from" ? "Where the window starts" : "Where the window ends"}
          aria-valuenow={edge === "from" ? from : to}
          title="{edge === 'from' ? 'Where the window starts' : 'Where the window ends'}, {clock(
            edge === 'from' ? from : to,
          )}. Drag to make it shorter or longer, double-click to put it back. At least {leastSays ||
            'room for a clip'}, at most {reachSays || 'all the model reads at once'}"
          onpointerdown={(e) => grabWindow(edge, e)}
          ondblclick={resetWindow}
        ></div>
      {/each}
    {/each}
  </div>
{/if}
{#if playhead >= 0}
  <!-- The playhead of the clip timeline too, drawn by .playhead in
       app.css. -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="playhead"
    class:scrubbing
    style="left: {on(playhead, inner)}"
    onpointerdown={scrub}
    title="Drag to move the playhead"
  ></div>
{/if}
</div>

<style>
  /* A container for its width, so whatever stands on the track is placed
     in shares of it by the stylesheet, see share above. */
  .track {
    position: relative;
    container-type: inline-size;
    /* Half the clip timeline, to within a pixel, set by the workspace so
       the two grow together and fill the height of the app. */
    height: var(--picker-h, 56px);
    flex: none;
    background: var(--well);
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    overflow: hidden;
    cursor: pointer;
    touch-action: none;
  }

  .track.scrubbing {
    cursor: grabbing;
  }

  /* The ruler behind the track, the same as on the clip timeline. It is
     under the window, because a tick that falls on the edge of the window
     would paint over half of its border and leave the window looking as if
     it were behind the track. */
  .tick {
    position: absolute;
    top: 0;
    bottom: 0;
    border-left: 1px solid var(--ink-3);
    pointer-events: none;
    z-index: 2;
  }

  /* The same place and the same colour as the times on the clip
     timeline, and quiet enough to stay behind what the track is about,
     but never behind anything laid over the track. Over the window, which
     is 3, so the window does not swallow the minutes it covers. A layer of its own rather
     than a child of the line, which sits under the window. */
  .time {
    position: absolute;
    z-index: 4;
    top: 3px;
    margin-left: 4px;
    font-size: 11px;
    color: var(--faint);
    white-space: nowrap;
    pointer-events: none;
  }

  /* Clear of the handle of a window that starts with the episode. */
  .start {
    left: 0;
    margin-left: 12px;
  }

  /* One box, the same line on all four sides and round corners, so where
     the window starts and ends is as plain as how tall it is. Nothing else
     is drawn on its edges: a second bar beside the border is what made one
     side look thicker than the others and the corners look broken. */
  .window {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 3;
    /* The motes rise inside it and no further, and lie under nothing
       outside it. */
    isolation: isolate;
    overflow: hidden;
    background: var(--accent-wash);
    /* Only looked at: a press on it is a press on the track, which moves
       the playhead. */
    pointer-events: none;
  }

  .window.whole {
    background: transparent;
    border-color: transparent;
  }

  /* While clips are being found for it, the window cannot be moved. The
     window wears the shimmer, the same light that lies over every place
     in the app waiting to be filled, because that is what this window
     is: the clips in it are on their way. Stripes were tried and they
     tile badly, the diagonal starts over at the edge of the repeat, which
     shows as a seam down the middle of the window. */
  .window.locked,
  .window.locked.whole {
    background-color: var(--accent-wash);
    border-color: var(--accent);
  }

  /* The whole video's part while it waits its turn: the same frame
     and breath at half the colour, the accent at 50 % the app already
     has, and its wash at half of its own. The breath is the frame's
     opacity, so the dimming is done in the colours and the two never
     fight. */
  .window.locked.behind {
    background-color: color-mix(in srgb, var(--accent-wash) 50%, transparent);
    border-color: var(--accent-fill-hi);
  }

  /* What the part's frame fades in: the same box the frames are laid
     out in, so the frame inside is where it would be without it, and over
     the track the same way. */
  .part {
    position: absolute;
    inset: 0;
    z-index: 3;
    pointer-events: none;
  }

  /* Going, nothing in it moves on: the breath and the motes stop where
     they are while it fades. */
  .part.going,
  .part.going :global(*) {
    animation-play-state: paused !important;
  }

  /* On the range picker's own edge, its round corner. */
  .window {
    --left: var(--frame-radius);
    --right: var(--frame-radius);
  }

  .window.first {
    --left: var(--radius-m);
    border-top-left-radius: var(--radius-m);
    border-bottom-left-radius: var(--radius-m);
  }

  .window.last {
    --right: var(--radius-m);
    border-top-right-radius: var(--radius-m);
    border-bottom-right-radius: var(--radius-m);
  }

  /* The motes are cut to the inside of the window's own line, corners
     and all, by a shape of their own. The window's overflow cuts them as
     well, but a mote moves on a layer of its own, and on the Mac a layer
     like that inside a box whose corners are not all alike, round at the
     range picker's end and nearly square at the other, was seen over the
     line at the round bottom corner: Tim saw it start round and break,
     plan row 2.274. The motes rise from the bottom and are gone before
     the top, which is why the top corner stayed round. */
  .window :global(.beam) {
    clip-path: inset(
      0 round max(var(--left) - var(--frame-line), 0px) max(var(--right) - var(--frame-line), 0px)
        max(var(--right) - var(--frame-line), 0px) max(var(--left) - var(--frame-line), 0px)
    );
  }

  /* The window at rest, marked at its corners. A bar three pixels thick
     lies on the top border and on the bottom one, over it, the way the
     clip's frame lies on the clip timeline's, and a triangle hangs from
     each end of it, its tip reaching seven pixels past the bar along the
     window's edge. The bars and the triangles
     are what takes the pointer, each more to the hand than it is drawn,
     and the whole of the mark lights up while any of it is under the
     pointer or held, because it is one thing. */
  /* No layer of its own, so its parts take their own places among the
     track's: the bars under the playhead, whose line would otherwise be
     lost along them, and the triangles over it, because a playhead
     standing on the window's start, where a search leaves it, took every
     press meant for the handles there. */
  .aim {
    position: absolute;
    top: 0;
    bottom: 0;
    pointer-events: none;
    --aim: var(--accent);
  }

  /* Under :where, so being under the pointer weighs no more than being
     held, and the flash after it wins over both: the hand is always on a
     triangle when it runs into a limit. */
  .aim:where(:has(.bar:hover, .tip:hover)),
  .aim.moving {
    --aim: var(--accent-hi);
  }

  .aim.flashing {
    --aim: var(--err);
  }

  .bar {
    position: absolute;
    left: 0;
    right: 0;
    height: 9px;
    z-index: 5;
    pointer-events: auto;
    cursor: grab;
    background: linear-gradient(var(--aim), var(--aim)) no-repeat;
    background-size: 100% 3px;
  }

  .bar.top {
    top: -3px;
    background-position: 0 3px;
  }

  .bar.bottom {
    bottom: -3px;
    background-position: 0 3px;
  }

  .bar.held {
    cursor: grabbing;
  }

  /* A handle, fourteen pixels to the hand around a triangle eight wide
     and ten tall, which starts on the bar and ends seven pixels in. */
  .tip {
    position: absolute;
    width: 14px;
    height: 16px;
    z-index: 7;
    pointer-events: auto;
    cursor: ew-resize;
    touch-action: none;
  }

  .tip::after {
    content: "";
    position: absolute;
    width: 8px;
    height: 10px;
    background: var(--aim);
  }

  .tip.top {
    top: -3px;
  }

  .tip.bottom {
    bottom: -3px;
  }

  .tip.top::after {
    top: 3px;
  }

  .tip.bottom::after {
    bottom: 3px;
  }

  .tip.from {
    left: -6px;
  }

  .tip.to {
    right: -6px;
  }

  .tip.from::after {
    left: 6px;
  }

  .tip.to::after {
    right: 6px;
  }

  .tip.top.from::after {
    clip-path: polygon(0 0, 100% 0, 0 100%);
  }

  .tip.top.to::after {
    clip-path: polygon(0 0, 100% 0, 100% 100%);
  }

  .tip.bottom.from::after {
    clip-path: polygon(0 0, 100% 100%, 0 100%);
  }

  .tip.bottom.to::after {
    clip-path: polygon(100% 0, 100% 100%, 0 100%);
  }

  /* A part not transcribed yet, dots over the track and under the
     minutes and the clips, and nothing else, as Tim picked them. */
  .unheard {
    position: absolute;
    top: 0;
    bottom: 0;
    left: var(--at);
    width: calc(var(--to) - var(--at));
    z-index: 1;
    pointer-events: none;
  }

  /* Under the hand or the keys on its card, the grey of a lit card, with
     the range picker's own border over it, which is the range picker's
     and not the part's. With the border gone the range picker looked
     broken, found by Tim. */
  .unheard.lit {
    background: var(--ink-3);
  }

  /* Its dots, the card's, rising from the foot, so the minutes stand
     clear above them, and with no dashed edge, which Tim found the dots
     did without. They fill the part as whole columns, centred, so the
     space from the last dot to the edge is the same on both sides and
     about half the space between two dots: a pattern that started
     wherever its grid fell looked off, and a row and a column left clear
     round it looked cut off, both found by Tim. The space left over is
     shared out exactly, so a side can take half a pixel, which is a
     whole pixel on a Retina screen. The foot row stands a pixel up from
     the range picker's foot, where it looked clutched to it, found by
     Tim. Lit, the part takes the grey of a card under the hand and keeps
     its fade, found by Tim. */
  .unheard::before {
    --width: calc(var(--to) - var(--at));
    --side: calc((var(--width) - round(down, var(--width), var(--dot-pitch))) / 2);
    content: "";
    position: absolute;
    top: 0;
    bottom: 0;
    left: var(--side);
    right: var(--side);
    background: var(--halftone) right 0 bottom 1px / var(--dot-pitch) var(--dot-pitch);
    mask-image: var(--halftone-fade);
  }

  /* While the work hears into a part, its dots stand still and the edge
     wipes over them, a dot at a time and smoothly. Centred, they moved
     half a pixel with every pixel of the edge and stepped back half a dot
     each time a column went, so the pattern walked back and forth, found
     by Tim. The right edge is where something heard begins or the video
     ends, so it never moves, and the dots are laid from it. Once the work
     stops, the part holds still and its dots are centred again. */
  .unheard.giving::before {
    --side: 0px;
  }

  .unheard.lit::before {
    background-image: var(--halftone-lit);
  }

  /* The box the playhead is drawn over, exactly the track and nothing
     more, so a position worked out for the track is right here too. */
  .over {
    position: relative;
    container-type: inline-size;
  }

  /* Drawn by .playhead in app.css, the same as on the clip timeline. Over
     the window and the clips. */
  .playhead {
    z-index: 6;
  }
</style>
