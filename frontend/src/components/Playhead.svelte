<!--
  The playhead, on the range picker and on the clip timeline: one playhead
  seen at two distances, so one component draws it, and a change to how it
  looks or moves is made once and is true in both. Each view says only
  which part of the video it shows, from and span in seconds, and where
  its inside starts, edge.

  At rest it stands at `at`, the moment the view hands it. While the video
  plays it stands where the sound is, --sound in seconds, which the video
  preview writes once a refresh of the screen on every playhead of its
  workspace, see lib/sounding.ts. It then
  lands on a pixel of the screen, --hair, rather than stepping to the
  start of each frame of the video, which on a short video moved it a
  pixel or two 25 times a second, a stutter. Nothing but that one custom
  property is written while it plays, so nothing else in the app is worked
  out again on every refresh. A playhead held by the hand, held, stands
  where the hand is and not where the sound is.

  A line and nothing else, running past the track's top and bottom border
  further than a keyframe or a thumbnail stands out of the clip timeline,
  8 px, so it is seen going on past a mark on its frame and is never taken
  for one. It stops two pixels short of the gap round the track, so it
  never touches what stands beyond it: the video preview, the line under
  the range picker and the controls under the clip timeline. It is dragged
  left and right and taken hold of a few pixels either side of the line,
  and goes white while it is held.
-->
<script lang="ts">
  import { sounding } from "../lib/sounding";

  let {
    at,
    from = 0,
    span,
    edge = 0,
    scrubbing = false,
    held = false,
    z,
    onpointerdown,
  }: {
    // Where it stands at rest, in seconds.
    at: number;
    // The part of the video the view shows, in seconds.
    from?: number;
    span: number;
    // Pixels from the view's box to where its moments start, the border
    // of the range picker's track counted in its marks or not.
    edge?: number;
    // The hand drags it.
    scrubbing?: boolean;
    // The hand holds it, or an edge it stands on, so the sound does not
    // move it.
    held?: boolean;
    // What it stands over in its view.
    z: number;
    onpointerdown?: (event: PointerEvent) => void;
  } = $props();

  const sound = sounding();
  let el = $state<HTMLDivElement>();
  $effect(() => (el && sound ? sound.line(el) : undefined));
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  bind:this={el}
  class="playhead"
  class:scrubbing
  class:held={held || scrubbing}
  style:--at={(at - from) / Math.max(span, 0.001)}
  style:--from={from}
  style:--span={Math.max(span, 0.001)}
  style:--edge="{edge}px"
  style:z-index={z}
  {onpointerdown}
  title="Drag to move the playhead"
></div>

<style>
  .playhead {
    /* Where the sound is, as a share of the view. With no --sound on the
       workspace this is invalid, and the line falls back to --at, see
       left. */
    --x: calc((var(--sound) - var(--from)) / var(--span));
    /* A pixel of the screen while the sound moves it, the same way
       invalid at rest, when it lands on a whole pixel. */
    --step: calc(var(--sound) * 0px + var(--hair));
    position: absolute;
    top: calc(2px - var(--gap));
    bottom: calc(2px - var(--gap));
    left: round(var(--edge) + (100% - 2px) * var(--x, var(--at)), var(--step, 1px));
    width: 2px;
    margin-left: -1px;
    background: var(--accent-hi);
    cursor: ew-resize;
    touch-action: none;
  }

  .held {
    --x: var(--at);
    --step: 1px;
  }

  .playhead::before {
    content: "";
    position: absolute;
    top: 0;
    bottom: 0;
    left: -4px;
    right: -4px;
  }

  .scrubbing {
    background: #fff;
  }
</style>
