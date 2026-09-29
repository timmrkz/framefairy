<script lang="ts">
  // The whole episode as one slim track: the ruler, the clips as marks and
  // the playhead, which a press anywhere takes hold of. While clips are
  // being found, or a search stands stopped, the window it is about is
  // drawn over the track, with how far the episode has been read for it.
  // At rest the window is not drawn over the track, only marked on its top
  // and bottom borders, a bar with a small triangle at each end: laid over
  // a short episode searched whole, the window hid every clip just found.
  // The marks say where New looks next, which the app decides, see
  // nextWindow in lib/flow.ts, and a hand can drag them somewhere else.
  // What has been searched or read so far is the engine's to know, not
  // anything a person has to look after.
  import { onMount } from "svelte";
  import { clock } from "../lib/api";
  import { gapsIn, type Parts } from "../lib/flow";
  import Busy from "./Busy.svelte";
  import Info from "./Info.svelte";
  import { scrub as scrubPlayhead } from "../lib/scrub";

  let {
    duration,
    heard = [[0, duration]],
    live = null,
    from = 0,
    to = 0,
    shown = false,
    marks = [],
    selected = "",
    onmark,
    playhead = -1,
    onseek,
    locked = false,
    transcribing = false,
    holding = false,
    onmove,
  }: {
    duration: number;
    // What of the episode is heard, in parts, the part being heard among
    // them, and that part itself, from where it began to where it has got,
    // or null while nothing is being heard.
    heard?: Parts;
    live?: [number, number] | null;
    // The window of the search in hand, drawn while shown.
    from?: number;
    to?: number;
    shown?: boolean;
    marks?: { key: string; start: number; end: number; rendered: boolean }[];
    selected?: string;
    onmark?: (key: string) => void;
    playhead?: number;
    onseek?: (time: number) => void;
    // Clips are being found for the window, which wears the shimmer.
    locked?: boolean;
    // Whether the episode is being read. Nothing is done about it here:
    // the reading is a step of a search, and is started and called off
    // with New and Cancel in the head of the clip list.
    transcribing?: boolean;
    // The edge is being held where it is, because Cancel was pressed. It
    // stops moving at once rather than sliding on to where the work had
    // got to, which is a second or two of an interface ignoring a click.
    holding?: boolean;
    // The marks of the window at rest dragged, with where its start is
    // now, while the hand moves, and done when it lets go.
    onmove?: (from: number, done: boolean) => void;
  } = $props();

  let track: HTMLDivElement;
  let width = $state(0);

  // Everything on the track is placed in whole pixels. In shares of the
  // width the window and the shade beside it land on halves of a pixel,
  // which is what made one border of the window look thicker than the
  // others.
  const scale = $derived(width / Math.max(duration, 0.001));

  function at(t: number): number {
    return Math.round(Math.min(Math.max(t, 0), duration) * scale);
  }

  const whole = $derived(from <= 0.5 && to >= duration - 0.5);
  // What is not heard yet, drawn only while something hears the episode, a
  // search or a clip made by hand, or while a search stands stopped. At rest
  // what has been heard is the engine's to know, like what has been
  // searched. A gap too short to see is no gap.
  const hearing = $derived(shown || !!live);
  const gaps = $derived(hearing && duration > 0 ? gapsIn(heard, 0, duration).filter(([a, b]) => b - a > 0.5) : []);
  const pending = $derived(gaps.length > 0);
  // The part being heard runs into the gap that starts where it has got.
  // Its stretch of the track is from where it began to the end of that
  // gap, which stays put while it is heard, so its shade and its fill are
  // laid out once and only slide.
  const liveGap = $derived(live ? gaps.find(([a]) => Math.abs(a - live[1]) < 0.05) : undefined);
  const liveSpan = $derived<[number, number] | null>(
    live && liveGap ? [Math.min(live[0], liveGap[0]), liveGap[1]] : null,
  );
  // Each gap in the dark, with the edge its shade starts from.
  const shades = $derived(
    gaps.map(([a, b]) => ({
      from: liveSpan && liveGap && a === liveGap[0] ? liveSpan[0] : a,
      to: b,
      edge: a,
      live: !!liveGap && a === liveGap[0],
      // Only the stretch being heard has an edge, and only until its fill
      // has begun, whose head is the line from then on. A heard part is
      // simply not dark: a still fill over every part heard put the head
      // of a fill, a bright line, at the end of each, and a part a minute
      // long is three pixels of a four hour track, so all it showed was
      // the line.
      waiting: !(liveGap && a === liveGap[0] && live && live[1] <= liveSpan![0] + 0.01),
    })),
  );


  // Whether the edge of the transcript may glide to where it is going. It
  // is off for the first frame, so the edge is simply where it is when the
  // track appears, and on from then on, so every step after that reads as
  // movement rather than a jump.
  let glide = $state(false);

  onMount(() => {
    let armed = 0;
    const arm = () => {
      cancelAnimationFrame(armed);
      armed = requestAnimationFrame(() => (glide = true));
    };
    const observer = new ResizeObserver(() => {
      // A track that changes width moves every edge on it at once, and one
      // that glided while the rest jumped would lag behind the track it
      // belongs to. So a resize puts the edge where it goes and the glide
      // comes back on the next frame.
      glide = false;
      width = track.clientWidth;
      arm();
    });
    observer.observe(track);
    arm();
    return () => {
      cancelAnimationFrame(armed);
      observer.disconnect();
    };
  });

  // Dragging the playhead, the same way as on the clip timeline, see
  // lib/scrub.ts. A press anywhere on the track takes hold of it, and so
  // does its head, and the video preview follows the hand.
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
    scrubPlayhead(event, timeAt, (t) => onseek?.(t), (held) => (scrubbing = held));
  }

  // Dragging the window at rest by its marks. It keeps its length and
  // stays on the episode, and follows the hand the whole way, so the New
  // button's title says where it is while it moves.
  let moving = $state(false);
  const movable = $derived(!shown && to > from && to - from < duration - 0.5);

  function grabWindow(event: PointerEvent) {
    if (event.button !== 0 || !movable) return;
    event.preventDefault();
    event.stopPropagation();
    const focused = document.activeElement as HTMLElement | null;
    if (focused && focused !== document.body) focused.blur();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const size = to - from;
    const startX = event.clientX;
    const was = from;
    const place = (clientX: number) =>
      Math.round(Math.min(Math.max(was + (clientX - startX) / Math.max(scale, 1e-9), 0), duration - size));
    moving = true;
    const move = (e: PointerEvent) => onmove?.(place(e.clientX), false);
    const up = (e: PointerEvent) => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      moving = false;
      onmove?.(place(e.clientX), true);
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  function timeAt(clientX: number): number {
    const box = track.getBoundingClientRect();
    const share = Math.max(0, Math.min(1, (clientX - box.left) / box.width));
    return share * duration;
  }

  // A time label every so often, the smallest step that leaves room for
  // the labels.
  const ticks = $derived.by(() => {
    if (!duration || !width) return [];
    const steps = [60, 300, 600, 900, 1800, 3600, 7200];
    const room = Math.max(1, Math.floor(width / 72));
    const step = steps.find((s) => duration / s <= room) ?? 7200;
    const out: { t: number; label: boolean }[] = [];
    for (let t = step; t < duration; t += step) {
      out.push({ t, label: (t / duration) * width < width - 56 });
    }
    return out;
  });

</script>

<!-- The playhead is drawn over the track rather than in it. The track
     clips what is inside it, which is what keeps the waveform and the
     shades inside its rounded corners, and the playhead is the one thing
     that has to reach past them: its head stands above the track the way
     an editor's does, and at the very start or the very end it would
     otherwise be cut off by the corner. -->
<div class="over">
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="track asks"
  class:locked
  class:scrubbing
  bind:this={track}
  onpointerdown={scrub}
  aria-label="The range picker"
>
  {#if shown}
    <div class="shade" style="left: 0; width: {at(from)}px"></div>
    <div class="shade" style="left: {at(to)}px; right: 0"></div>
  {/if}
  <!-- Over the shade, not under it, so what has no transcript yet reads
       the same wherever it is. The part being heard wears the work in hand
       of Busy.svelte itself, the one every row and button of the app
       wears, with no rim because the track has no edge to run round. Its
       fill slides by the same transform and the same glide as the dark
       ahead of it, so the two never part. Only the part being heard wears
       it: what has been heard is the track without the dark, and stays so. -->
  {#if liveSpan && live}
    <span
      class="busyhost"
      class:glide={glide && !holding}
      class:held={holding}
      style="left: {at(liveSpan[0])}px; width: {at(liveSpan[1]) - at(liveSpan[0])}px"
      ><Busy
        fraction={(live[1] - liveSpan[0]) / Math.max(liveSpan[1] - liveSpan[0], 0.001)}
        rim={false}
        still={!transcribing}
      /></span
    >
  {/if}
  <!-- Each stretch not heard yet is dark from its edge on. The dark is as
       wide as the stretch and slides by transform, so the edge of the part
       being heard is only ever moved, never laid out again. -->
  {#each shades as g (g.from)}
    <div class="gap" style="left: {at(g.from)}px; width: {at(g.to) - at(g.from)}px">
      <div
        class="pending"
        class:waiting={g.waiting}
        class:glide={g.live && glide && !holding}
        class:held={holding}
        style="transform: translateX({at(g.edge) - at(g.from)}px)"
      ></div>
    </div>
  {/each}
  {#if shown}
    <!-- Only looked at, never taken hold of: which part is searched is the
         app's to say, and a press on it is a press on the track. -->
    <div class="window frame" class:waiting={locked} class:whole style="left: {at(from)}px; width: {at(to) - at(from)}px"></div>
  {/if}
  {#if !shown && to > from && duration > 0}
    <!-- The window at rest, marked on the top border and the bottom one,
         the two the same, so either can be taken hold of. Along the edges
         of the track and not across it, so no clip is ever under it. -->
    {#each ["top", "bottom"] as side (side)}
      <div
        class="aim {side}"
        class:movable
        class:moving
        style="left: {at(from)}px; width: {at(to) - at(from)}px"
        role="slider"
        tabindex="-1"
        aria-label="Where New looks next"
        aria-valuenow={from}
        title={movable
          ? `Where New looks next, ${clock(from)} to ${clock(to)}. Drag to look somewhere else`
          : `Where New looks next, ${clock(from)} to ${clock(to)}`}
        onpointerdown={grabWindow}
      ></div>
    {/each}
  {/if}
  <!-- Every clip is a mark, and a mark is pressed to work on its clip. -->
  {#each marks as m (m.key)}
    <button
      class="clipmark"
      class:rendered={m.rendered}
      class:selected={m.key === selected}
      style="left: {at(m.start)}px; width: {Math.max(at(m.end) - at(m.start), 4)}px"
      aria-label="Clip at {clock(m.start)}"
      onpointerdown={(e) => e.stopPropagation()}
      onclick={() => onmark?.(m.key)}
    ></button>
  {/each}
  <!-- The ruler, in two layers, and they have to be two.

       The line goes under the window, because a minute that falls on the
       window's edge would otherwise paint over half of its border. The
       time goes over everything, because it says where you are on the
       track and nothing laid over the track may take that away.

       A time written inside its own line cannot have both: an element with
       a z-index makes a stacking context, so the time would be held at the
       line's level however high its own is, and a window drawn over a
       part already searched would swallow it. -->
  {#each ticks as tick (tick.t)}
    <div class="tick" style="left: {at(tick.t)}px"></div>
  {/each}
  {#each ticks as tick (tick.t)}
    {#if tick.label}
      <span class="num time" style="left: {at(tick.t)}px">{clock(tick.t)}</span>
    {/if}
  {/each}
  <span class="num time start">0:00</span>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <span class="ask corner" onpointerdown={(e) => e.stopPropagation()}>
    <Info label="What the range picker is" side="right">
      The whole episode, with its clips as marks. Press or drag anywhere to move the playhead, and
      press a mark to work on its clip. The marks on the top and bottom border are where New looks
      next, and can be dragged somewhere else. While clips are being found, the part being searched
      is framed, and the dark part of it is not read yet.
    </Info>
  </span>
</div>
{#if playhead >= 0}
  <div class="playhead" class:scrubbing style="left: {at(playhead)}px">
    <!-- The head is its own element, the way it is on the clip timeline,
         because it stands above the track and a head the hand goes
         straight through is not a handle. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="head" onpointerdown={scrub} title="Drag to move the playhead"></div>
  </div>
{/if}
</div>

<style>
  .track {
    position: relative;
    /* Half the clip timeline, set by the workspace so the two grow
       together and fill the height of the app. */
    height: var(--picker-h, 56px);
    flex: none;
    background: var(--ink-1);
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
     is 3, so a window drawn across a part that was searched already
     does not swallow the minutes it covers. A layer of its own rather
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

  .start {
    left: 0;
    margin-left: 6px;
  }

  /* What has not been transcribed yet is darker, and the edge between the
     two is where the transcript has got to.
     The shade alone could never say where that is. Down at this end of the
     scale lightness is compressed: the track is #1d1f23, and laying black
     over it at any strength lands between 1.1 and 1.2 to 1 against it,
     measured off the pixels. Taking it to near black does not help, it only
     turns the far end of the track into a hole. Two large areas that close
     together are one area.
     An edge is a different thing to see. A line carries its contrast in the
     step across it rather than in the area, so one pixel of a grey that is
     plainly lighter says what a whole field of darker grey cannot, and it
     is the edge that shows the movement: what the eye follows as the
     transcript grows is the line, not the shade behind it. */
  /* It is as wide as the whole track and travels by transform, so what is
     drawn is only ever moved and never laid out again. A left that is
     animated is worked out by the main thread on every frame, which is the
     same thread the transcription's own reports land on, so the edge stood
     still for a frame or two and then caught up in a jump: a step, beside a
     mark that glided. Measured while transcribing, the distance between the
     two varied by 1.80 pixels. A transform is carried by the compositor,
     like the mark's, so the two move as one thing.
     The track clips what runs past its right edge. */
  /* A stretch not heard yet. It clips the dark sliding inside it. */
  .gap {
    position: absolute;
    top: 0;
    bottom: 0;
    overflow: hidden;
    pointer-events: none;
  }

  .pending {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    right: 0;
    /* The head of a fill, which is what this edge is: the app fills this
       track from the left as it reads the episode, so the line where it
       has got to is the same bright line with a glow ahead of it that the
       head of every other fill in the app carries. A hairline of --muted
       said the same thing in a colour that means nothing here, and said it
       so quietly that Tim could not see the reading move. */
    /* While the reading runs, the head is the fill's own, the bright line
       with the glow pushed ahead of it that every fill in the app has, so
       this is only the dark ahead of it. A reading that was paused has no
       fill, and a quiet line says where it stopped. */
    border-left: 1px solid var(--line);
    background: linear-gradient(to right, rgba(0, 0, 0, 0.5), rgba(0, 0, 0, 0.34) 28px);
    pointer-events: none;
  }

  /* The transcription hands in a chunk of audio at a time, so the edge
     arrives in steps of about a second. Gliding between them for as long
     as a step usually takes turns the steps into the one movement they
     are. The glide is armed a frame after the first edge is drawn, so
     opening a workspace mid-transcription does not sweep the track. */
  .pending.waiting {
    border-left-color: transparent;
  }

  .pending.glide {
    transition: transform 1s linear;
  }

  /* Cancel was pressed, so the edge stops. Taking the glide away should be
     enough and is not: a transition already on its way carries on to where
     it was going, which is a second of an edge still sliding after the
     press. Saying none outright ends it, and the edge lands on the second
     the work really reached. */
  /* The host of the work in hand drawn over a part of the track. It lies
     under the window, the searched parts and the marks, like the track's
     own shade, and it glides the way the shade's edge does. Square, since
     a part meets the next one inside the track, and the track clips its
     own corners. */
  .busyhost {
    position: absolute;
    top: 0;
    bottom: 0;
    border-radius: 0;
    pointer-events: none;
    --fill-glide: 0s;
  }

  .busyhost.glide {
    --fill-glide: 1s linear;
  }

  .pending.held {
    transition: none;
  }

  .shade {
    position: absolute;
    top: 0;
    bottom: 0;
    background: rgba(12, 12, 14, 0.5);
    pointer-events: none;
  }

  /* One box, the same line on all four sides and round corners, so where
     the window starts and ends is as plain as how tall it is. Nothing else
     is drawn on its edges: a second bar beside the border is what made one
     side look thicker than the others and the corners look broken. */
  /* Over the searched parts, always. A searched part is a fact
     about the episode and the window is what you are doing to it, so the
     window is never partly under one, not even where the two touch
     exactly. */
  .window {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 3;
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
  .track.locked .window,
  .track.locked .window.whole {
    background-color: var(--accent-wash);
    border-color: var(--accent);
  }

  /* The window at rest, marked on a border: a bar along it, as thick as
     the frame's line is twice, and a small triangle at each end pointing
     into the track, so the two ends are plain however short the window
     is. The hold is taller than the mark, so a hand finds it, and it lies
     over the ruler's times, which take no pointer. */
  .aim {
    position: absolute;
    height: 10px;
    z-index: 5;
    --aim: var(--accent);
    background: linear-gradient(var(--aim), var(--aim)) no-repeat;
    background-size: 100% 3px;
  }

  .aim.top {
    top: 0;
    background-position: top;
  }

  .aim.bottom {
    bottom: 0;
    background-position: bottom;
  }

  .aim::before,
  .aim::after {
    content: "";
    position: absolute;
    width: 7px;
    height: 5px;
    background: var(--aim);
  }

  .aim::before {
    left: 0;
  }

  .aim::after {
    right: 0;
  }

  .aim.top::before,
  .aim.top::after {
    top: 3px;
  }

  .aim.bottom::before,
  .aim.bottom::after {
    bottom: 3px;
  }

  .aim.top::before {
    clip-path: polygon(0 0, 100% 0, 0 100%);
  }

  .aim.top::after {
    clip-path: polygon(0 0, 100% 0, 100% 100%);
  }

  .aim.bottom::before {
    clip-path: polygon(0 0, 100% 100%, 0 100%);
  }

  .aim.bottom::after {
    clip-path: polygon(100% 0, 100% 100%, 0 100%);
  }

  .aim.movable {
    cursor: grab;
  }

  .aim.movable:hover,
  .aim.moving {
    --aim: var(--accent-hi);
  }

  .aim.moving {
    cursor: grabbing;
  }

  /* The box the playhead is drawn over, exactly the track and nothing
     more, so a position worked out for the track is right here too. */
  .over {
    position: relative;
  }

  /* The same line and the same head as on the clip timeline: one playhead
     seen at two distances, not two different marks. The head stands above
     the track, which is where an editor puts it and what says this is the
     playhead rather than a line someone drew. */
  .playhead {
    position: absolute;
    top: -5px;
    bottom: 0;
    width: 2px;
    margin-left: -1px;
    background: var(--accent-hi);
    pointer-events: none;
    z-index: 6;
  }

  .head {
    position: absolute;
    top: 0;
    left: -4px;
    width: 10px;
    height: 9px;
    border-radius: 2px 2px 1px 1px;
    background: var(--accent-hi);
    pointer-events: auto;
    cursor: pointer;
    touch-action: none;
  }

  /* Held, the playhead goes white, as it does on the clip timeline. */
  .playhead.scrubbing,
  .playhead.scrubbing .head {
    background: #fff;
  }

  .playhead.scrubbing .head {
    cursor: grabbing;
  }

</style>
