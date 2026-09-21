<script lang="ts" module>
  // What the row above the clip timeline has to offer for the video
  // preview. The buttons sit in that row now, so what they are for has to
  // travel out of here.
  export type PlayerOffers = {
    crop: "" | "auto" | "back";
    savingCrop: boolean;
    // What the caption box says about itself while it is dragged.
    hint: string;
  };
</script>

<script lang="ts">
  // The episode, with the crop of the selected clip laid over it. One Play
  // button, one playhead: playing starts where the playhead stands, and with
  // a clip selected it jumps the clip's cuts and stops where the clip ends.
  // While the playhead is inside a clip, its captions are drawn inside the
  // crop the way the render will burn them in.
  import { onMount, type Snippet } from "svelte";
  import { pictureIsStale } from "../lib/flow";
  import Info from "./Info.svelte";
  import {
    captionYStep,
    mediaURL,
    snapCaptionY,
    type CaptionsView,
    type ClipEntry,
    type SourceView,
  } from "../lib/api";

  let {
    path,
    source,
    clip,
    captions = null,
    time = $bindable(0),
    still = "",
    onplayclip,
    onstill,
    oncaptionmoved,
    oncrop,
    onresetcrop,
    oncaptiony,
    strip,
    paused = $bindable(true),
    looping = $bindable(false),
    offers = $bindable({ crop: "", savingCrop: false, hint: "" } as PlayerOffers),
  }: {
    path: string;
    source: SourceView;
    clip: ClipEntry | null;
    // The captions of the selected clip, on the clip's own clock.
    captions?: CaptionsView | null;
    time?: number;
    still?: string;
    onplayclip?: (clip: ClipEntry) => void;
    // Asks for the frame at a moment of the episode, for as long as the
    // window cannot show that moment itself.
    onstill?: (at: number) => void;
    // Where the caption box is while it is being dragged, so the setting
    // beside it says what you are doing as you do it. Letting go saves,
    // this only shows.
    oncaptionmoved?: (y: number) => void;
    oncrop?: (at: number, left: number) => Promise<void>;
    onresetcrop?: (at: number) => Promise<void>;
    // Puts the captions at a distance from the bottom of a 1080x1920
    // frame. There is one place for every clip of every episode, so this
    // saves a setting rather than editing the clip.
    oncaptiony?: (y: number) => Promise<void>;
    // The range picker sits under the video preview, so it is exactly as
    // wide as it is.
    strip?: Snippet;
    // Play and Pause stand in the row above the clip timeline, with Render.
    paused?: boolean;
    looping?: boolean;
    offers?: PlayerOffers;
  } = $props();

  let screen: HTMLDivElement;
  // How tall the picture came out, which the captions are drawn against.
  // Reading it changes nothing, so it never sets the layout going again.
  let screenHeight = $state(0);
  let shell: HTMLDivElement;

  let dragLeft = $state<number | null>(null);
  let savingCrop = $state(false);
  let dragCaptions = $state<number | null>(null);
  let savingCaptions = $state(false);

  // Going back to the automatic crop is one click, so taking that back has
  // to be one click too. What a reset threw away is kept until the clip
  // changes or it is placed by hand again.
  let undoCrop = $state<{ key: string; piece: number; at: number; left: number } | null>(null);

  let video: HTMLVideoElement;
  let failed = $state("");
  // The moment the picture is really showing, and whether it shows
  // anything at all. While the machine is busy the window often cannot
  // read the file, and then a seek is dropped and the picture stays where
  // it was. The workspace fills in with a frame read from the file.
  let ready = $state(false);
  let shows = $state(-1);
  const stale = $derived(pictureIsStale({ ready, shows, at: time }));
  $effect(() => {
    if (stale) onstill?.(time);
  });
  let atPiece = 0;
  let frame = 0;

  // The row above the clip timeline draws the buttons, so it is told what
  // there is to offer.
  $effect(() => {
    offers = {
      crop: !clip ? "" : crop?.moved ? "auto" : undoCrop?.key === clip.key && undoCrop.piece === piece ? "back" : "",
      savingCrop,
      hint: dragCaptions !== null ? `Captions ${Math.round(captionY)} from the bottom` : "",
    };
  });

  const pieces = $derived(clip?.segments ?? []);
  const clipStart = $derived(pieces.length ? pieces[0].start : 0);
  const clipEnd = $derived(pieces.length ? pieces[pieces.length - 1].end : 0);

  // The captions run on the clip's clock, which has the cuts taken out.
  function inClipTime(t: number): number {
    let sum = 0;
    for (const p of pieces) {
      if (t < p.start) return sum;
      if (t < p.end) return sum + (t - p.start);
      sum += p.end - p.start;
    }
    return sum;
  }

  function pieceAt(t: number): number {
    const index = pieces.findIndex((p) => t < p.end);
    return index < 0 ? Math.max(pieces.length - 1, 0) : index;
  }

  // A video that has not read its own index yet drops a seek on the floor,
  // which used to leave the playhead somewhere the picture never went. The
  // moment it knows its length, it is sent there.
  export function seek(t: number) {
    if (!video) return;
    time = Math.max(0, Math.min(t, source.duration));
    atPiece = pieceAt(time);
    goTo(time);
  }

  // Where the picture was asked to go, while it is still on its way there.
  let wanted = -1;
  let tries = 0;
  let chasing = 0;

  function goTo(t: number) {
    if (!video) return;
    if (video.readyState === 0 || !Number.isFinite(video.duration)) {
      video.addEventListener("loadedmetadata", () => goTo(t), { once: true });
      // An element that has not started reading the file does not start by
      // itself, so it is asked to. NETWORK_EMPTY and NETWORK_NO_SOURCE are
      // the two states where nothing is on its way.
      if (video.networkState === 0 || video.networkState === 3) video.load();
      return;
    }
    wanted = t;
    tries = 0;
    put(t);
    chase();
  }

  function put(t: number) {
    try {
      video.currentTime = t;
    } catch {
      // A webview that refuses the seek keeps the frame it has. It is
      // asked again below, and the time under the video preview is the
      // truth either way.
    }
  }

  // A seek that is never answered leaves the picture on the frame it had,
  // which reads as a broken video preview. The machine is busy while it
  // transcribes, so a seek that has not landed is made again, and then the
  // file is read once more before giving up.
  function chase() {
    clearTimeout(chasing);
    chasing = window.setTimeout(() => {
      if (!video || wanted < 0) return;
      if (Math.abs(video.currentTime - wanted) < 0.5) {
        wanted = -1;
        return;
      }
      tries++;
      if (tries > 2) {
        wanted = -1;
        return;
      }
      if (tries === 2) {
        const t = wanted;
        video.addEventListener("loadedmetadata", () => put(t), { once: true });
        video.load();
      } else {
        put(wanted);
      }
      chase();
    }, 1200);
  }

  export function toggle() {
    if (!video || failed) return;
    if (video.paused) {
      play();
    } else {
      wantPlay = false;
      video.pause();
    }
  }

  // True from the moment playing is asked for until it is stopped again. A
  // window that has not read the file yet refuses to play, which used to
  // mean the first press of the space bar did nothing at all and the
  // second one worked. The press is remembered instead, and the picture
  // starts the moment it can.
  let wantPlay = false;

  function play() {
    if (!video) return;
    if (clip) {
      // A clip plays from the playhead while the playhead stands inside it,
      // otherwise from its first word.
      if (time < clipStart || time >= clipEnd - 0.05) time = clipStart;
      atPiece = pieceAt(time);
      goTo(time);
      onplayclip?.(clip);
    }
    wantPlay = true;
    video.play().catch(() => {
      if (!wantPlay || !video) return;
      video.addEventListener(
        "canplay",
        () => {
          if (wantPlay) void video.play().catch(() => {});
        },
        { once: true },
      );
    });
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(tick);
  }

  function tick() {
    frame = 0;
    if (!video) return;
    if (clip && pieces.length) {
      // The episode plays through what the clip cuts out, so the playhead
      // jumps every cut and stops where the clip ends.
      const piece = pieces[atPiece];
      // Nothing is decided while a jump is still being made, or a stale
      // position could be read as the end of the piece jumped to.
      if (!video.seeking && video.currentTime >= piece.end - 0.02) {
        atPiece++;
        if (atPiece >= pieces.length) {
          if (!looping) {
            wantPlay = false;
            video.pause();
            time = clipEnd;
            return;
          }
          atPiece = 0;
        }
        video.currentTime = pieces[atPiece].start;
      }
    }
    time = video.currentTime;
    if (!video.paused) frame = requestAnimationFrame(tick);
  }

  function onError() {
    const code = video?.error?.code ?? 0;
    const names: Record<number, string> = {
      1: "loading was stopped",
      2: "a network error",
      3: "the video could not be decoded",
      4: "the format or the way it is served is not supported",
    };
    failed = `The episode cannot play in the app: ${names[code] ?? "unknown error"} (code ${code}).`;
  }

  // The piece under the playhead, or the one nearest to it. Nearest, not
  // first, so nothing jumps when the playhead sits on a cut or rests on the
  // last frame of the clip.
  const piece = $derived.by(() => {
    if (!clip) return -1;
    const here = clip.segments.findIndex((s) => time >= s.start && time < s.end);
    if (here >= 0) return here;
    let best = 0;
    let near = Infinity;
    clip.segments.forEach((s, i) => {
      const gap = time < s.start ? s.start - time : time - s.end;
      if (gap < near) {
        near = gap;
        best = i;
      }
    });
    return best;
  });

  // Its crop, as shares of the video preview, with a drag applied.
  const crop = $derived.by(() => {
    if (!clip || piece < 0 || !source.width || !source.cropWidth) return null;
    const auto = clip.cropLefts[piece] ?? (source.width - source.cropWidth) / 2;
    const left = dragLeft ?? auto;
    return {
      left: (left / source.width) * 100,
      width: (source.cropWidth / source.width) * 100,
      // The last frame of a piece still belongs to it, so the frame does
      // not blink when a clip plays to its end.
      inside: clip.segments.some((s) => time >= s.start && time <= s.end),
      moved: clip.segments[piece]?.moved ?? false,
    };
  });

  // The caption standing at the playhead, with the word being spoken. The
  // engine hands over the lines and the look, so nothing about captions is
  // decided twice.
  const caption = $derived.by(() => {
    if (!clip || !captions?.captions?.length) return null;
    if (time < clipStart - 0.05 || time > clipEnd + 0.05) return null;
    const at = inClipTime(time);
    return captions.captions.find((c) => at >= c.start && at < c.end) ?? null;
  });

  const spoken = $derived(inClipTime(time));

  // The frame the captions sit in: the crop, or the whole preview when there
  // is none to place them in.
  const box = $derived(crop ?? { left: 0, width: 100 });

  function px(share: number): number {
    return share * screenHeight;
  }

  // The caption line, as the distance from the bottom of a 1080x1920 frame,
  // with a drag in progress applied.
  const captionY = $derived(dragCaptions ?? (captions ? captions.style.marginV * 1920 : 300));

  // Dragging the caption box moves it up and down in steps, so the captions
  // of an episode stay in line with each other.
  function grabCaptions(event: PointerEvent) {
    if (!oncaptiony || savingCaptions || !captions) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const from = event.clientY;
    const start = captions.style.marginV * 1920;
    const scale = 1920 / Math.max(screenHeight, 1);
    let moved = false;
    const move = (e: PointerEvent) => {
      if (Math.abs(e.clientY - from) > 2) moved = true;
      if (!moved) return;
      dragCaptions = snapCaptionY(start + (from - e.clientY) * scale);
      oncaptionmoved?.(dragCaptions);
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      const to = dragCaptions;
      if (!moved || to === null) {
        dragCaptions = null;
        toggle();
        return;
      }
      savingCaptions = true;
      try {
        await oncaptiony(to);
      } finally {
        dragCaptions = null;
        savingCaptions = false;
      }
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // The moment the crop change applies to: the playhead when it is inside
  // the clip, otherwise the start of the piece shown.
  function cropMoment(): number {
    if (!clip) return time;
    const seg = clip.segments[piece];
    return time >= seg.start && time < seg.end ? time : seg.start;
  }

  function dragCrop(event: PointerEvent) {
    if (!clip || !oncrop || savingCrop) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startX = event.clientX;
    const start = clip.cropLefts[piece] ?? (source.width - source.cropWidth) / 2;
    const scale = source.width / screen.clientWidth;
    const most = source.width - source.cropWidth;
    let moved = false;
    const move = (e: PointerEvent) => {
      if (Math.abs(e.clientX - startX) > 2) moved = true;
      if (!moved) return;
      dragLeft = Math.round(Math.max(0, Math.min(most, start + (e.clientX - startX) * scale)));
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      if (!moved || dragLeft === null) {
        dragLeft = null;
        toggle();
        return;
      }
      savingCrop = true;
      try {
        await oncrop(cropMoment(), dragLeft);
        undoCrop = null;
      } finally {
        dragLeft = null;
        savingCrop = false;
      }
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  export async function resetCrop() {
    if (!clip || !onresetcrop) return;
    const at = cropMoment();
    const was = piece;
    const had = clip.cropLefts[was];
    savingCrop = true;
    try {
      await onresetcrop(at);
      if (had !== undefined && had !== null) undoCrop = { key: clip.key, piece: was, at, left: had };
    } finally {
      savingCrop = false;
    }
  }

  export async function cropBack() {
    const back = undoCrop;
    if (!back || !oncrop) return;
    savingCrop = true;
    try {
      await oncrop(back.at, back.left);
      undoCrop = null;
    } finally {
      savingCrop = false;
    }
  }

  // The space bar plays and pauses, as in every video tool, unless a field
  // has the keyboard.
  function onKey(event: KeyboardEvent) {
    if (event.code !== "Space" || event.metaKey || event.ctrlKey || event.altKey) return;
    const on = document.activeElement as HTMLElement | null;
    const tag = on?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || on?.isContentEditable) return;
    event.preventDefault();
    toggle();
  }

  onMount(() => {
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      cancelAnimationFrame(frame);
    };
  });
</script>

<!-- The video preview takes everything the workspace has left, in height
     and in width, and keeps the shape of the episode. What is under it, the
     range picker and the controls, is measured rather than guessed at, so
     nothing is left over and nothing has to be scrolled to. -->
<div class="player" bind:this={shell}>
  <div class="screen asks" bind:this={screen} bind:clientHeight={screenHeight}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <span class="ask corner" onpointerdown={(e) => e.stopPropagation()}>
      <Info label="What you can do with the picture" side="right">
        The space bar plays and pauses, and so does a click on the picture. Drag the crop frame
        sideways to place it, and the black box up or down for the captions.
      </Info>
    </span>
    {#if still && stale}
      <img src={still} alt="" />
    {/if}
    <!-- svelte-ignore a11y_media_has_caption -->
    <video
      bind:this={video}
      src={mediaURL(path)}
      preload="metadata"
      bind:paused
      ontimeupdate={() => {
        // Only an element that has read the file has a time worth
        // following. One that has nothing reports zero, and that would
        // throw away the playhead the moment it is put somewhere.
        if (video.readyState === 0) return;
        shows = video.currentTime;
        if (video.paused) time = video.currentTime;
      }}
      onloadedmetadata={() => {
        // Nothing is decoded until the picture is sent somewhere, so an
        // episode that is opened and not played would stay black. A hair
        // past the start counts as somewhere.
        goTo(time > 0 ? time : 0.05);
      }}
      onloadeddata={() => {
        ready = true;
        shows = video.currentTime;
      }}
      onseeked={() => {
        ready = true;
        shows = video.currentTime;
        wanted = -1;
      }}
      onerror={onError}
      onclick={toggle}
    ></video>
    {#if crop}
      <div class="shade" style="left: 0; width: {crop.left}%"></div>
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="frame"
        class:outside={!crop.inside}
        class:dragging={dragLeft !== null}
        style="left: {crop.left}%; width: {crop.width}%"
        title="Drag sideways to place the crop"
        onpointerdown={dragCrop}
      ></div>
      <div class="shade" style="left: {crop.left + crop.width}%; right: 0"></div>
    {/if}
    {#if dragCaptions !== null}
      <div
        class="grid"
        style="left: {box.left}%; width: {box.width}%; --step: {px(captionYStep / 1920)}px"
      ></div>
    {/if}
    {#if caption && captions}
      <div
        class="captions"
        style="left: {box.left}%; width: {box.width}%;
               bottom: {Math.max(px(captionY / 1920 - captions.style.padY), 0)}px;
               padding: 0 {px(captions.style.marginH)}px;
               font-family: '{captions.style.font}', system-ui, sans-serif;
               font-weight: {captions.style.bold ? 800 : 500};
               font-size: {px(captions.style.size)}px;
               line-height: {captions.style.lineHeight};
               color: {captions.style.primary}"
      >
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="box"
          class:draggable={!!oncaptiony}
          style="background: {captions.style.box};
                 border-radius: {px(captions.style.radius)}px;
                 padding: {px(captions.style.padY)}px {px(captions.style.padX)}px"
          title="Drag up or down to place the captions"
          onpointerdown={grabCaptions}
        >
          {#each caption.lines as line, row (row)}
            <div class="line">
              {#each line.words as word, i (word.start)}{#if i > 0}{" "}{/if}<span
                  class:now={captions.style.highlight &&
                    spoken >= word.start &&
                    spoken < word.end}
                  style="--pill: {captions.style.highlightColour}">{word.text}</span
                >{/each}
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
  <div class="under">
    {@render strip?.()}
    {#if failed}<p class="error selectable">{failed}</p>{/if}
  </div>
</div>

<style>
  .player {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    min-width: 0;
    width: 100%;
  }

  /* The range picker and the controls under the picture. */
  .under {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
  }

  /* As tall and as wide as the workspace worked out, which it did from the
     window itself. Nothing here measures anything. */
  .screen {
    position: relative;
    height: var(--pic-h);
    width: var(--pic-w);
    background: #000;
    border-radius: var(--radius-m);
    overflow: hidden;
  }

  video,
  img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: contain;
  }

  img {
    z-index: 1;
    pointer-events: none;
  }

  .shade {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 2;
    background: rgba(0, 0, 0, 0.55);
    pointer-events: none;
  }

  .frame {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 2;
    border: 2px solid var(--accent);
    cursor: grab;
    touch-action: none;
  }

  .frame.dragging {
    cursor: grabbing;
    border-color: var(--accent-hi);
  }

  .frame.outside {
    border-style: dashed;
    border-color: var(--muted);
  }

  /* The captions, drawn where the render burns them into the crop. */
  .captions {
    position: absolute;
    z-index: 3;
    display: flex;
    justify-content: center;
    pointer-events: none;
  }

  /* The box is as wide as its widest line and no wider, exactly like the
     box the render draws around the text it measured. The engine breaks the
     lines so that they fit inside the frame. */
  .box {
    width: fit-content;
    text-align: center;
  }

  .box.draggable {
    pointer-events: auto;
    cursor: grab;
    touch-action: none;
  }

  /* The steps the caption line snaps to, shown while it is dragged. */
  .grid {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 3;
    pointer-events: none;
    background: repeating-linear-gradient(
      to top,
      rgba(255, 255, 255, 0.16) 0 1px,
      transparent 1px var(--step)
    );
  }

  .line {
    white-space: nowrap;
  }

  /* The pill around the spoken word is drawn over the line without moving
     anything, the way the render draws it, so the padding is taken back out
     of the layout again. */
  .line span {
    display: inline-block;
    padding: 0.1em 0.14em;
    margin: 0 -0.14em;
    border-radius: 0.2em;
  }

  .line span.now {
    background: var(--pill);
    animation: pop 0.22s ease-out;
  }

  @keyframes pop {
    from {
      transform: scale(0.88);
    }
    55% {
      transform: scale(1.12);
    }
    to {
      transform: scale(1);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .line span.now {
      animation: none;
    }
  }

</style>
