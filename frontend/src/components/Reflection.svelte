<script lang="ts">
  // The faint reflection of the video preview at the foot of the
  // workspace, as if the video shone towards the person in front of it.
  // The frame on screen is made small, upside down, blurred and faded in
  // lib/frames/reflection.ts, and drawn along the bottom edge of the app
  // on a pixel of the canvas for every pixel of the screen, by the
  // graphics chip where there is one, see lib/frames/reflected.ts, where
  // the stylesheet lays it out with the numbers from app.css. It lies
  // behind everything in the workspace, so nothing that is read or worked
  // with changes, and it shows only in the room between them.
  //
  // A new frame is settled on over a quarter of a second, so a cut fades
  // over rather than flashing. The settling is drawn at most STEPS times a
  // second and stops once it has arrived, so a paused video costs nothing.
  import { onMount } from "svelte";
  import type { Picture } from "../lib/frames/picture";
  import { COLS, ROWS, band, clear, ease, mirrored, share, spread, type Band } from "../lib/frames/reflection";
  import { reflectedOn, type Reflected } from "../lib/frames/reflected";

  let { shown = true }: { shown?: boolean } = $props();

  const STEPS = 30;
  let canvas: HTMLCanvasElement;
  // What draws on the canvas, the graphics chip where there is one, from
  // the moment it is ready.
  let reflected: Reflected | null = null;
  // The frame's cells, and what is drawn and what it settles towards, the
  // cells blurred and faded over the band the stylesheet lays out, which
  // the canvas is told of. Clear until the first frame.
  const cells = new Float32Array(COLS * ROWS * 4);
  const now = new Float32Array(COLS * ROWS * 4);
  const goal = new Float32Array(COLS * ROWS * 4);
  let laid: Band | null = null;
  // The canvas's own pixels across and down, as many as the screen gives
  // the band.
  let pixels: [number, number] | null = null;
  // Whether the cells changed since they were spread over the band.
  let fresh = false;
  let request = 0;
  let last = 0;

  // The frame put on the video preview, or null for the black it shows
  // without one, which the reflection fades out on. It is read at once and
  // not kept. It is spread over the band once a step is drawn, not here:
  // a video plays more frames a second than the STEPS drawn, and every
  // frame was spread, which is most of the reflection's work, for a
  // drawing that showed only some of them.
  export function take(frame: Picture | null) {
    const d = frame?.data;
    if (frame && d) mirrored(d, frame.width, frame.height, frame.light, cells);
    else clear(cells);
    fresh = true;
    settle();
  }

  function spreadCells() {
    if (!laid) return;
    spread(cells, laid, goal);
    fresh = false;
  }

  function settle() {
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
    if (fresh) spreadCells();
    const left = ease(now, goal, share(dt / 1000));
    reflected?.draw(now);
    if (left >= 0.5) request = requestAnimationFrame(step);
  }

  // A new band shows at once: only a change of picture is settled on.
  function lay() {
    if (!pixels) return;
    spreadCells();
    now.set(goal);
    reflected?.size(pixels[0], pixels[1], now);
  }

  // The band's size in points, while it is on the page, or null.
  let box: [number, number] | null = null;

  // The canvas gets a pixel for every pixel of the screen under the band,
  // and one pixel while the band is not seen, out of the page with its
  // workspace asleep or hidden with the video's file gone, so a kept
  // workspace keeps a few bytes and not megabytes, which is what the band
  // held before it was drawn on every pixel. Nothing is laid out from it.
  function fit() {
    if (!box || !shown) {
      if (pixels && reflected) reflected.size(1, 1, now);
      pixels = null;
      return;
    }
    laid = band(box[0], box[1]);
    pixels = [Math.max(1, Math.round(box[0] * devicePixelRatio)), Math.max(1, Math.round(box[1] * devicePixelRatio))];
    lay();
  }

  $effect(() => {
    void shown;
    fit();
  });

  // The colour of the app behind the band, which the reflection is laid
  // over, see laid in reflection.ts: --ink-0, as the stylesheet gives it
  // to the canvas, painted on a pixel and read back in sRGB, whatever way
  // the token is written.
  function ground(): [number, number, number] {
    const pixel = document.createElement("canvas");
    pixel.width = pixel.height = 1;
    const g = pixel.getContext("2d", { willReadFrequently: true });
    if (!g) return [0, 0, 0];
    g.fillStyle = getComputedStyle(canvas).color;
    g.fillRect(0, 0, 1, 1);
    const [r, gr, b] = g.getImageData(0, 0, 1, 1).data;
    return [r, gr, b];
  }

  onMount(() => {
    // For the walk "reflection": the canvas is opaque, the reflection
    // laid over the ground, so what is seen of each colour is read from
    // the cells it is drawn from rather than from the canvas.
    (window as { __reflectionCells?: Float32Array }).__reflectionCells = now;
    let gone = false;
    void reflectedOn(canvas, ground()).then((it) => {
      if (gone) return;
      reflected = it;
      lay();
    });
    // The canvas is told how large the stylesheet lays it out, which is
    // how far its blur reaches in cells. A workspace put away, see
    // App.svelte, is out of the page and has no size.
    const watch = new ResizeObserver((entries) => {
      const seen = entries[entries.length - 1].contentRect;
      box = canvas.isConnected && seen.width && seen.height ? [seen.width, seen.height] : null;
      fit();
    });
    watch.observe(canvas);
    // A screen of another scale, the app moved to it or its scale
    // changed, gives the band another number of pixels at the same size
    // in points, which the ResizeObserver does not hear.
    let scale: MediaQueryList | null = null;
    const rescaled = () => {
      scale?.removeEventListener("change", rescaled);
      scale = matchMedia(`(resolution: ${devicePixelRatio}dppx)`);
      scale.addEventListener("change", rescaled);
      fit();
    };
    rescaled();
    return () => {
      gone = true;
      watch.disconnect();
      scale?.removeEventListener("change", rescaled);
      cancelAnimationFrame(request);
    };
  });
</script>

<canvas class="reflection" class:hidden={!shown} bind:this={canvas} width="1" height="1" aria-hidden="true"></canvas>

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
    /* The colour of the app behind the band, which the reflection is
       laid over in its own pixels, read from here as the canvas starts,
       see ground. */
    color: var(--ink-0);
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
