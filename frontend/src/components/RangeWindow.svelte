<script lang="ts">
  // The whole episode as one slim track. The window you drag chooses the
  // stretch the model reads. Clip marks, the playhead and time labels sit
  // inside the track. A click without dragging moves the player.
  import { onMount } from "svelte";
  import { clock, type WindowView } from "../lib/api";
  import Icon from "./Icon.svelte";
  import Info from "./Info.svelte";

  let {
    duration,
    covered = duration,
    from = $bindable(0),
    to = $bindable(0),
    onmoved,
    marks = [],
    selected = "",
    onmark,
    playhead = -1,
    onseek,
    searched = [],
    onremove,
    locked = false,
    transcribing = false,
    partly = false,
    leftToGo = "",
    holding = false,
    ontranscription,
  }: {
    duration: number;
    covered?: number;
    from: number;
    to: number;
    onmoved?: (edge: "from" | "to") => void;
    marks?: { key: string; start: number; end: number; rendered: boolean }[];
    selected?: string;
    onmark?: (key: string) => void;
    playhead?: number;
    onseek?: (time: number) => void;
    // The stretches already searched for clips, in order and merged. They
    // are drawn as marks on the track. A window may be drawn over them,
    // and what that means is decided by whoever acts on the window.
    searched?: WindowView[];
    // Removing what the window covers, which lets the clips in it go and
    // leaves the stretch free to be searched again. The caller asks first.
    onremove?: (stretch: { from: number; to: number }) => void;
    locked?: boolean;
    // How the reading of the episode stands. The transcript's edge is drawn
    // here already, so the one thing to do about it belongs here too rather
    // than in the head of a list about clips.
    transcribing?: boolean;
    // Stopped part way: some of it read, nothing reading the rest.
    partly?: boolean;
    leftToGo?: string;
    // The edge is being held where it is, because pause was pressed. It
    // stops moving at once rather than sliding on to where the work had
    // got to, which is a second or two of an interface ignoring a click.
    holding?: boolean;
    // Pause it while it runs, carry on while it is stopped. One control,
    // because there is only ever one thing to do.
    ontranscription?: () => void;
  } = $props();

  // The control at the transcript's edge is only there while the pointer is
  // on the track, the same as the info marks: what a thing is for is shown
  // when it is being looked at, and a track with nothing happening on it
  // carries nothing.
  let near = $state(false);

  let track: HTMLDivElement;
  let width = $state(0);
  const minimum = 10;
  // A press that travels less than this is a click. A trackpad rarely holds
  // still to the pixel, and on a four hour episode one pixel is half a
  // minute, so a wobble used to draw a window nobody asked for.
  const slack = 6;

  // Whether the window lies over material that has been searched already.
  // A window may be drawn anywhere, so this is what the buttons around it
  // go by: what it covers there can be removed, and searching it again
  // asks first.
  const covering = $derived(searched.some((w) => w.to > from && w.from < to));

  // Everything on the track is placed in whole pixels. In shares of the
  // width the window and the shade beside it land on halves of a pixel,
  // which is what made one border of the window look thicker than the
  // others.
  const scale = $derived(width / Math.max(duration, 0.001));

  function at(t: number): number {
    return Math.round(Math.min(Math.max(t, 0), duration) * scale);
  }

  // Edges land on a round step, so a stretch is something you can say out
  // loud. The step is the smallest round one that is still about eight
  // pixels wide, which keeps it useful for a fifteen minute episode and for
  // a four hour one.
  const grid = $derived.by(() => {
    const steps = [1, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800];
    const least = (duration / Math.max(width, 1)) * 8;
    return steps.find((step) => step >= least) ?? 1800;
  });

  function round(t: number): number {
    return Math.round(t / grid) * grid;
  }

  const whole = $derived(from <= 0.5 && to >= duration - 0.5);
  const pending = $derived(duration > 0 && covered < duration - 0.5);

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

  // Double-clicking the track takes the window back to the whole episode.
  function reset() {
    if (locked) return;
    from = 0;
    to = duration;
    onmoved?.("from");
  }

  function clamp(t: number): number {
    return Math.max(0, Math.min(t, duration));
  }

  function timeAt(clientX: number): number {
    const box = track.getBoundingClientRect();
    const share = Math.max(0, Math.min(1, (clientX - box.left) / box.width));
    return share * duration;
  }

  // While a stretch is being drawn or moved it says what it is, because a
  // step the pointer lands on is worth seeing in seconds.
  let showing = $state(false);

  // True while the pointer is on the window or on the button beside it.
  // What can be taken away waits until the window is under the pointer,
  // the way the trash can on a clip row does.
  let overWindow = $state(false);

  // True while an edge is under the pointer or holds the keyboard focus.
  // The edges are the box itself, so the box lights up instead of growing a
  // second bar beside its own border.
  let grip = $state(false);

  function drag(kind: "from" | "to" | "move" | "new", event: PointerEvent) {
    event.preventDefault();
    event.stopPropagation();
    const startX = timeAt(event.clientX);
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startClientX = event.clientX;
    const startFrom = from;
    const startTo = to;
    const span = startTo - startFrom;
    const grab = startX - startFrom;
    let dragged = false;
    let moved: "from" | "to" = kind === "to" ? "to" : "from";
    const move = (e: PointerEvent) => {
      if (Math.abs(e.clientX - startClientX) > slack) dragged = true;
      if (!dragged || locked) return;
      showing = true;
      const here = timeAt(e.clientX);
      // Edges land on the grid, and the ends of the episode win over it.
      const t = clamp(round(here));
      if (kind === "from") from = Math.max(0, Math.min(t, to - minimum));
      else if (kind === "to") to = Math.min(duration, Math.max(t, from + minimum));
      else if (kind === "move") {
        from = Math.max(0, Math.min(round(here - grab), duration - span));
        to = from + span;
      } else {
        const a = Math.min(startX, t);
        const b = Math.max(startX, t);
        if (b - a >= minimum) {
          from = a;
          to = b;
          moved = t > startX ? "to" : "from";
        }
      }
    };
    const up = (e: PointerEvent) => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      showing = false;
      // A press that moved a few pixels without changing the window is a
      // click, not a drag, so the player goes to where it was let go.
      if (from === startFrom && to === startTo) {
        onseek?.(timeAt(e.clientX));
        return;
      }
      if (locked) return;
      onmoved?.(moved);
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  function nudge(kind: "from" | "to", event: KeyboardEvent) {
    if (locked) return;
    const step = grid * (event.shiftKey ? 5 : 1);
    let delta = 0;
    if (event.key === "ArrowLeft") delta = -step;
    else if (event.key === "ArrowRight") delta = step;
    else return;
    event.preventDefault();
    if (kind === "from") from = Math.max(0, Math.min(from + delta, to - minimum));
    else to = Math.min(duration, Math.max(to + delta, from + minimum));
    onmoved?.(kind);
  }

  // Time labels: a round step that leaves room for every label.
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
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="over"
  onpointerenter={() => (near = true)}
  onpointerleave={() => (near = false)}
>
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="track asks"
  class:locked
  bind:this={track}
  onpointerdown={(e) => drag("new", e)}
  ondblclick={reset}
  aria-label="The range picker"
>

  <div class="shade" style="left: 0; width: {at(from)}px"></div>
  <div class="shade" style="left: {at(to)}px; right: 0"></div>
  <!-- Over the shade, not under it, so what has no transcript yet reads
       the same wherever it is. It is darker and nothing else: the edge
       moves as the transcript grows, and the only thing on the track that
       moves of its own accord is the window while a search runs. -->
  {#if pending}
    <div class="pending" class:glide={glide && !holding} class:held={holding} style="transform: translateX({at(covered)}px)"></div>
  {/if}
  <!-- The one thing to do about the reading of the episode, at the edge the
       reading moves. It waits for the pointer to be on the track, the way
       every other mark here does, so a track nobody is looking at carries
       nothing. -->
  {#if pending && near && (transcribing || partly) && ontranscription}
    <button
      class="reading"
      class:glide={glide && !holding}
      class:held={holding}
      style="transform: translateX({at(covered)}px)"
      onpointerdown={(e) => e.stopPropagation()}
      ondblclick={(e) => e.stopPropagation()}
      onclick={ontranscription}
      aria-label={transcribing ? "Pause the transcription" : "Carry on transcribing"}
      title={transcribing
        ? `Reading the episode${leftToGo ? `, ${leftToGo}` : ""}. Pause it, and it carries on where it stopped`
        : `The episode is read as far as ${clock(covered)}. Carry on from there`}
    >
      <Icon name={transcribing ? "pause" : "play"} size={12} />
    </button>
  {/if}
  {#each searched as w, i (i)}
    <div
      class="done"
      style="left: {at(w.from)}px; width: {at(w.to) - at(w.from)}px"
      title="Searched already. Draw a window over it to look again or to remove the clips in it"
    ></div>
  {/each}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="window"
    class:whole
    class:grip
    class:xray={covering}
    style="left: {at(from)}px; width: {at(to) - at(from)}px"
    title="Drag it along the range picker"
    onpointerdown={(e) => drag("move", e)}
    onpointerenter={() => (overWindow = true)}
    onpointerleave={() => (overWindow = false)}
  ></div>
  <!-- The window is what is acted on, so what can be taken away sits in
       it: the clips of the searched material it covers. A window too narrow
       to hold the button wears it just outside its end. -->
  {#if onremove && !locked && covering}
    <button
      class="free quiet danger"
      class:shown={overWindow}
      class:beside={at(to) - at(from) < 40}
      style="left: {at(to)}px"
      title="Remove the clips in this stretch, so the model can read it again"
      aria-label="Remove the clips from {clock(from)} to {clock(to)}"
      aria-haspopup="dialog"
      onpointerdown={(e) => e.stopPropagation()}
      onpointerenter={() => (overWindow = true)}
      onpointerleave={() => (overWindow = false)}
      onclick={() => onremove?.({ from, to })}
    >
      <Icon name="trash" size={12} />
    </button>
  {/if}
  <!-- A clip the window lies over is on its way out, so it is not drawn:
       what the window shows is what the range picker would look like with
       that stretch given back. -->
  {#each marks as m (m.key)}
    {#if !(covering && m.end > from && m.start < to)}
    <button
      class="mark"
      class:rendered={m.rendered}
      class:selected={m.key === selected}
      style="left: {at(m.start)}px; width: {Math.max(at(m.end) - at(m.start), 4)}px"
      aria-label="Clip at {clock(m.start)}"
      onpointerdown={(e) => e.stopPropagation()}
      onclick={() => onmark?.(m.key)}
    ></button>
    {/if}
  {/each}
  <!-- The ruler, in two layers, and they have to be two.

       The line goes under the window, because a minute that falls on the
       window's edge would otherwise paint over half of its border. The
       time goes over everything, because it says where you are on the
       track and nothing laid over the track may take that away.

       A time written inside its own line cannot have both: an element with
       a z-index makes a stacking context, so the time would be held at the
       line's level however high its own is, and a window drawn over a
       stretch already searched would swallow it. -->
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
  <span
    class="ask corner"
    onpointerdown={(e) => e.stopPropagation()}
    ondblclick={(e) => e.stopPropagation()}
  >
    <!-- Until the episode is read to the end that is the thing to say, and
         it is said first, because it is what everything else is waiting on.
         Short either way: a bubble nobody finishes is a bubble nobody
         reads. -->
    <Info label="What the range picker is" side="right">
      {#if pending}
        The whole episode. The dark part is not read yet, and the line is how far it has got. The
        mark on the line pauses the reading or carries it on. Clips can be looked for once the line
        passes the window.
      {:else}
        The whole episode. Drag for a stretch to search, or drag the window and its edges.
        Double-click for all of it. A shaded stretch has been searched, and the marks in it are its
        clips.
      {/if}
    </Info>
  </span>
  {#if showing}
    <div class="said"><div class="row num">{clock(from)} to {clock(to)}</div></div>
  {/if}
  <div
      class="handle"
      style="left: {at(from)}px"
      role="slider"
      tabindex="0"
      aria-label="Start of the stretch"
      aria-valuemin={0}
      aria-valuemax={duration}
      aria-valuenow={from}
      aria-valuetext={clock(from)}
      onpointerdown={(e) => drag("from", e)}
      onkeydown={(e) => nudge("from", e)}
      onpointerenter={() => (grip = true)}
      onpointerleave={() => (grip = false)}
      onfocus={() => (grip = true)}
      onblur={() => (grip = false)}
    ></div>
    <div
      class="handle"
      style="left: {at(to)}px"
      role="slider"
      tabindex="0"
      aria-label="End of the stretch"
      aria-valuemin={0}
      aria-valuemax={duration}
      aria-valuenow={to}
      aria-valuetext={clock(to)}
      onpointerdown={(e) => drag("to", e)}
      onkeydown={(e) => nudge("to", e)}
      onpointerenter={() => (grip = true)}
      onpointerleave={() => (grip = false)}
      onfocus={() => (grip = true)}
      onblur={() => (grip = false)}
    ></div>
</div>
{#if playhead >= 0}
  <div class="playhead" style="left: {at(playhead)}px"></div>
{/if}
</div>

<style>
  .track {
    position: relative;
    /* Half the clip timeline, set by the workspace so the two grow
       together and fill the window. */
    height: var(--picker-h, 56px);
    flex: none;
    background: var(--ink-1);
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    overflow: hidden;
    cursor: crosshair;
    touch-action: none;
  }

  .track.locked,
  .track.locked .window {
    cursor: default;
  }

  /* Removing what the window covers. It sits in the corner of the window,
     or just outside a window too narrow to hold it. */
  /* What can be taken away waits until the window is under the pointer,
     the way the trash can on a clip row does. */
  .free {
    position: absolute;
    top: 4px;
    opacity: 0;
    pointer-events: none;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    margin-left: -24px;
    padding: 0;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--ink-2);
    color: var(--muted);
    z-index: 5;
  }

  .free.shown,
  .free:focus-visible {
    opacity: 1;
    pointer-events: auto;
  }

  .free.beside {
    margin-left: 4px;
  }

  .free:hover:not(:disabled),
  .free:focus-visible {
    color: var(--err);
    border-color: var(--err);
  }

  /* What has been searched already. A window is never drawn over it, so it
     reads as the wall it is. */
  .done {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 1;
    background: var(--ink-3);
    border-left: 1px solid var(--line);
    border-right: 1px solid var(--line);
    opacity: 0.75;
    pointer-events: none;
  }

  /* The ruler behind the track, the same as on the clip timeline. It is
     over the searched stretch, so the minutes can still be read there,
     and under the window, because a tick that falls on the edge of the
     window would paint over half of its border and leave the window
     looking as if it were behind the wall it sits on. */
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
     is 3, so a window drawn across a stretch that was searched already
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
  .pending {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    right: 0;
    border-left: 1px solid var(--muted);
    /* Deepest against the line and easing back to the flat shade, so the
       edge reads as the front of something moving rather than as the side
       of a block. */
    background: linear-gradient(to right, rgba(0, 0, 0, 0.55), rgba(0, 0, 0, 0.34) 28px);
    pointer-events: none;
  }

  /* The transcription hands in a chunk of audio at a time, so the edge
     arrives in steps of about a second. Gliding between them for as long
     as a step usually takes turns the steps into the one movement they
     are. The glide is armed a frame after the first edge is drawn, so
     opening a workspace mid-transcription does not sweep the track. */
  .pending.glide {
    transition: transform 1s linear;
  }

  /* Pause was pressed, so the edge stops. Taking the glide away should be
     enough and is not: a transition already on its way carries on to where
     it was going, which is a second of an edge still sliding after the
     press. Saying none outright ends it, and the edge lands on the second
     the work really reached. */
  .pending.held,
  .reading.held {
    transition: none;
  }

  /* At the transcript's edge, on the dark side of it, so it never covers
     the waveform or a clip mark. It sits on the line rather than beside it,
     because what it is about is the line.
     It travels with the edge, so it takes the same glide: a control that
     jumped while the line it belongs to slid would read as two things. */
  /* It travels by transform rather than by left. A left that is animated
     lands on a fraction of a pixel on most frames, the button is laid out
     afresh at every one of them, and the two bars of the pause mark inside
     it are drawn a little differently each time: the mark wobbles while it
     slides. A transform moves what has already been drawn, so the mark is
     rasterised once and carried, and it holds still. */
  .reading {
    position: absolute;
    top: 50%;
    left: 3px;
    margin-top: -10px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--ink-2);
    color: var(--text);
    cursor: pointer;
    z-index: 7;
  }

  .reading:hover {
    background: var(--ink-3);
    border-color: var(--muted);
  }

  .reading.glide {
    transition: transform 1s linear;
  }

  /* In the middle of the track, over everything, and only what is inside it
     takes the pointer, so the track can still be dragged around it. */
  .said {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
    z-index: 5;
  }

  .said .row {
    gap: 8px;
    height: 36px;
    padding: 0 6px 0 12px;
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    background: rgba(12, 12, 14, 0.88);
    pointer-events: auto;
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
  /* Over the searched stretches, always. A searched stretch is a fact
     about the episode and the window is what you are doing to it, so the
     window is never partly under one, not even where the two touch
     exactly. */
  .window {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 3;
    background: var(--accent-wash);
    border: 2px solid var(--accent);
    border-radius: var(--radius-s);
    cursor: grab;
  }

  .window.whole {
    background: transparent;
    border-color: transparent;
  }

  /* A window drawn over material that was searched already is a window
     onto what that stretch would be without it: the track as it looks
     where nobody has looked yet, with the clips inside it gone. So what
     the button in its corner does is plain before it is pressed. */
  .window.xray {
    background: var(--ink-1);
  }

  /* An edge under the pointer lights the whole box, which is the edge you
     are about to take hold of. */
  .window.grip,
  .window.whole.grip {
    border-color: var(--accent-hi);
  }

  /* While clips are being found for it, the window cannot be moved. A soft
     light passes through it, once every couple of seconds, so the stretch
     says work is in hand without a pattern to read. Stripes were tried and
     they tile badly: the diagonal starts over at the edge of the repeat,
     which shows as a seam down the middle of the window. */
  .track.locked .window,
  .track.locked .window.whole {
    background-color: rgba(180, 35, 111, 0.18);
    border-color: var(--accent);
    overflow: hidden;
  }

  .track.locked .window::after {
    content: "";
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    width: 45%;
    background: linear-gradient(
      90deg,
      rgba(208, 53, 127, 0) 0%,
      rgba(230, 120, 180, 0.38) 50%,
      rgba(208, 53, 127, 0) 100%
    );
    animation: pass 2.2s ease-in-out infinite;
  }

  @keyframes pass {
    from {
      transform: translateX(-100%);
    }
    to {
      transform: translateX(222%);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .track.locked .window::after {
      animation: none;
      width: 100%;
      background: rgba(208, 53, 127, 0.18);
    }
  }

  .mark {
    position: absolute;
    bottom: 5px;
    height: 12px;
    min-width: 4px;
    padding: 0;
    border: none;
    border-radius: 2px;
    background: var(--accent);
    z-index: 2;
    cursor: pointer;
  }

  .mark:hover:not(:disabled) {
    background: var(--accent-hi);
  }

  .mark.rendered {
    background: var(--ok);
  }

  .mark.selected {
    background: #fff;
    bottom: 3px;
    height: 16px;
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

  .playhead::before {
    content: "";
    position: absolute;
    top: 0;
    left: -4px;
    width: 10px;
    height: 9px;
    border-radius: 2px 2px 1px 1px;
    background: var(--accent-hi);
  }

  /* Room to take hold of an edge, and nothing to look at. What you see is
     the border of the window, which is exactly where the edge is. */
  .handle {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 10px;
    margin-left: -5px;
    cursor: ew-resize;
    z-index: 4;
  }

  .track.locked .handle {
    cursor: default;
  }
</style>
