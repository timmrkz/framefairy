<script lang="ts">
  // The faint reflection of the video preview at the foot of the
  // workspace, as if the video shone towards the person in front of it.
  // The frame on screen is made small, upside down, blurred and faded in
  // lib/frames/reflection.ts, and the stylesheet only stretches it along
  // the bottom edge of the app, with the numbers from app.css. It lies
  // behind everything in the workspace, so nothing that is read or worked
  // with changes, and it shows only in the room between them.
  //
  // A new frame is settled on over a quarter of a second, so a cut fades
  // over rather than flashing. The settling is drawn at most STEPS times a
  // second and stops once it has arrived, so a paused video costs nothing.
  import { onMount } from "svelte";
  import type { Picture } from "../lib/frames/picture";
  import { COLS, ROWS, band, clear, ease, mirrored, painted, share, spread, type Band } from "../lib/frames/reflection";

  let { shown = true }: { shown?: boolean } = $props();

  const STEPS = 30;
  let canvas: HTMLCanvasElement;
  let g: CanvasRenderingContext2D | null = null;
  let image: ImageData | null = null;
  // The frame's cells, and what is drawn and what it settles towards, the
  // cells blurred and faded over the band the stylesheet lays out, which
  // the canvas is told of. Clear until the first frame.
  const cells = new Float32Array(COLS * ROWS * 4);
  const now = new Float32Array(COLS * ROWS * 4);
  const goal = new Float32Array(COLS * ROWS * 4);
  let laid: Band | null = null;
  let request = 0;
  let last = 0;

  // The frame put on the video preview, or null for the black it shows
  // without one, which the reflection fades out on. It is read at once and
  // not kept. The same frame put up again, for what lies over it changed,
  // is the same reflection, and is not read again.
  let taken: Picture | null | undefined;
  export function take(frame: Picture | null) {
    if (frame === taken) return;
    taken = frame;
    const d = frame?.data;
    if (frame && d) mirrored(d, frame.width, frame.height, frame.light, cells);
    else clear(cells);
    settle();
  }

  // A new band shows at once, on the next step: only a change of picture
  // is settled on.
  function settle(at = false) {
    if (laid) spread(cells, laid, goal);
    if (at) now.set(goal);
    if (!request) {
      last = performance.now();
      request = requestAnimationFrame(step);
    }
  }

  function step(t: number) {
    request = 0;
    const dt = t - last;
    if (dt < 1000 / STEPS) {
      request = requestAnimationFrame(step);
      return;
    }
    last = t;
    const left = ease(now, goal, share(dt / 1000));
    if (g && image) {
      painted(now, image.data);
      g.putImageData(image, 0, 0);
    }
    if (left >= 0.5) request = requestAnimationFrame(step);
  }

  onMount(() => {
    g = canvas.getContext("2d");
    image = new ImageData(COLS, ROWS);
    // The canvas is told how large the stylesheet lays it out, which is
    // how far its blur reaches in cells. Nothing is laid out from it.
    const watch = new ResizeObserver((entries) => {
      const box = entries[entries.length - 1].contentRect;
      // Nothing measured while the workspace is put away, see App.svelte.
      if (!canvas.isConnected || !box.width || !box.height) return;
      laid = band(box.width, box.height);
      settle(true);
    });
    watch.observe(canvas);
    return () => {
      watch.disconnect();
      cancelAnimationFrame(request);
    };
  });
</script>

<canvas class="reflection" class:hidden={!shown} bind:this={canvas} width={COLS} height={ROWS} aria-hidden="true"
></canvas>

<style>
  /* Along the bottom edge of the workspace, under the middle of the app,
     which is where the picture stands, see --spans in app.css. The
     workspace starts after the rail. Behind everything in it, which the
     workspace keeps to itself, see section in Episode.svelte. No filter
     and no mask: the blur and the fade are drawn into its cells, see
     reflection.ts. */
  .reflection {
    position: absolute;
    z-index: -1;
    bottom: 0;
    left: max(0px, calc(50dvw - var(--reflection-w) / 2 - var(--rail)));
    width: var(--reflection-w);
    height: var(--reflection-h);
    pointer-events: none;
    will-change: transform;
  }

  /* On a whole pixel, see Everything lands on a whole pixel in CLAUDE.md. */
  @supports (left: round(down, 3px, 2px)) {
    .reflection {
      left: max(0px, round(down, calc(50dvw - var(--reflection-w) / 2 - var(--rail)), 1px));
      width: round(down, var(--reflection-w), 1px);
    }
  }

  .reflection.hidden {
    visibility: hidden;
  }
</style>
