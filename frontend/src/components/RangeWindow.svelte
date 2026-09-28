<script lang="ts">
  // The whole episode as one slim track: the ruler, the clips as marks and
  // the playhead, which a press anywhere takes hold of. While clips are
  // being found, or a search stands stopped, the window it is about is
  // drawn over the track, with how far the episode has been read for it.
  // At rest there is no window: which part of the episode a search reads
  // is the app's to decide, see engine/suggest.go, and what has been
  // searched or read so far is the engine's to know, not anything a person
  // has to look after. Tim found the window at rest a leftover that got in
  // the way: laid over a short episode searched whole, it hid every clip
  // just found.
  import { onMount } from "svelte";
  import { clock } from "../lib/api";
  import Busy from "./Busy.svelte";
  import Info from "./Info.svelte";
  import { scrub as scrubPlayhead } from "../lib/scrub";

  let {
    duration,
    covered = duration,
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
  }: {
    duration: number;
    covered?: number;
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
  // How far the episode has been read is shown for the search in hand
  // only, inside what it is reading.
  const pending = $derived(shown && duration > 0 && covered < duration - 0.5);

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
    scrubPlayhead(event, timeAt, (t) => onseek?.(t), (held) => (scrubbing = held));
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
    <!-- Over the shade, not under it, so what has no transcript yet reads
         the same wherever it is. What the episode has not been read to is a
         place waiting to be filled, so while the reading runs it wears the
         shimmer, the same breath every such place in the app wears, and the
         line where the reading has got to is the head of a fill, the same
         head every fill carries. Only while it runs: an episode read half
         way and left alone is not work in hand, and a track that breathed at
         it would say there was.
         Nothing new is drawn here: the track says what the rest of the app
         says, in the words the rest of the app uses. -->
    <!-- What has been read wears the fill, and the motes rise through the
         track: the work in hand of Busy.svelte itself, the one every row and
         button of the app wears, with no rim because the track has no edge to
         run round. Not a copy of it, which is what this was and what looked
         different. Its fill slides by the same transform and the same glide
         as the shade ahead of it, so the two never part. A reading that has
         stopped, called off or waiting while another search finds, keeps
         its fill and stands still, the way Busy draws any work that is not
         moving, so what has been read never disappears from the track and
         comes back. -->
    {#if pending && covered > 0}
      <span
        class="busyhost"
        class:glide={glide && !holding}
        class:held={holding}
      ><Busy fraction={duration > 0 ? covered / duration : 0} rim={false} still={!transcribing} /></span>
    {/if}
    {#if pending}
      <div
        class="pending"
        class:waiting={covered > 0}
        class:glide={glide && !holding}
        class:held={holding}
        style="transform: translateX({at(covered)}px)"
      ></div>
    {/if}
    <!-- Only looked at, never taken hold of: which part is searched is the
         app's to say, and a press on it is a press on the track. -->
    <div class="window frame" class:waiting={locked} class:whole style="left: {at(from)}px; width: {at(to) - at(from)}px"></div>
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
      press a mark to work on its clip. While clips are being found, the part being searched is
      framed, and the dark part of it is not read yet.
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
  /* The host of the work in hand drawn over the track. It lies under the
     window, the searched parts and the marks, like the track's own shade,
     and it glides the way the shade's edge does. */
  .busyhost {
    position: absolute;
    inset: 0;
    border-radius: inherit;
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
