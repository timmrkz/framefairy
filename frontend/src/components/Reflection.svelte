<script lang="ts">
  // The faint reflection of the video preview at the foot of the
  // workspace, as if the video shone towards the person in front of it.
  // The frame on screen is made small and upside down in
  // lib/frames/reflection.ts, and the stylesheet stretches it along the
  // bottom edge of the app, blurs it and fades it out upwards, with the
  // numbers from app.css. It lies behind everything in the workspace, so
  // nothing that is read or worked with changes, and it shows only in the
  // room between them.
  //
  // A new frame is settled on over a quarter of a second, so a cut fades
  // over rather than flashing. The settling is drawn at most STEPS times a
  // second and stops once it has arrived, so a paused video costs nothing.
  import { onMount } from "svelte";
  import type { Picture } from "../lib/frames/picture";
  import { COLS, ROWS, ease, mirrored, pixels, share } from "../lib/frames/reflection";

  let { shown = true }: { shown?: boolean } = $props();

  const STEPS = 30;
  let canvas: HTMLCanvasElement;
  let g: CanvasRenderingContext2D | null = null;
  let image: ImageData | null = null;
  const now = new Float32Array(COLS * ROWS * 3);
  const goal = new Float32Array(COLS * ROWS * 3);
  let request = 0;
  let last = 0;

  // The frame put on the video preview, or null for the black it shows
  // without one. It is read at once and not kept.
  export function take(frame: Picture | null) {
    const d = frame?.data;
    if (frame && d) mirrored(d, frame.width, frame.height, frame.light, goal);
    else goal.fill(0);
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
      pixels(now, image.data);
      g.putImageData(image, 0, 0);
    }
    if (left >= 0.5) request = requestAnimationFrame(step);
  }

  onMount(() => {
    g = canvas.getContext("2d", { alpha: false });
    image = new ImageData(COLS, ROWS);
    return () => cancelAnimationFrame(request);
  });
</script>

<canvas class="reflection" class:hidden={!shown} bind:this={canvas} width={COLS} height={ROWS} aria-hidden="true"
></canvas>

<style>
  /* Along the bottom edge of the workspace, under the middle of the app,
     which is where the picture stands, see --spans in app.css. The
     workspace starts after the rail. Behind everything in it, which the
     workspace keeps to itself, see section in Episode.svelte. Faded in
     from the left, the right and the top, strongest at the foot, the part
     nearest to the person. */
  .reflection {
    position: absolute;
    z-index: -1;
    bottom: 0;
    left: max(0px, calc(50dvw - var(--reflection-w) / 2 - var(--rail)));
    width: var(--reflection-w);
    height: var(--reflection-h);
    filter: blur(var(--reflection-blur));
    opacity: var(--reflection-strength);
    pointer-events: none;
    -webkit-mask-image:
      linear-gradient(to right, transparent, #000 25%, #000 75%, transparent),
      linear-gradient(to top, #000, rgb(0 0 0 / 45%) 35%, rgb(0 0 0 / 12%) 70%, transparent);
    mask-image:
      linear-gradient(to right, transparent, #000 25%, #000 75%, transparent),
      linear-gradient(to top, #000, rgb(0 0 0 / 45%) 35%, rgb(0 0 0 / 12%) 70%, transparent);
    -webkit-mask-composite: source-in;
    mask-composite: intersect;
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
