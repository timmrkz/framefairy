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
  import { onMount, untrack, type Snippet } from "svelte";
  import {
    insideClip,
    jumpStep,
    litWord,
    pictureIsStale,
    playingAt,
    stillFits,
    shouldChase,
    pieceAt as pieceIndex,
    playingPiece,
  } from "../lib/flow";
  import Info from "./Info.svelte";
  import {
    captionYStep,
    mediaURL,
    snapCaptionY,
    type CaptionsView,
    type ClipEntry,
    type SourceView,
  } from "../lib/api";
  import { rgbToHex } from "../lib/colour";

  let {
    path,
    source,
    clip,
    captions = null,
    time = $bindable(0),
    still = "",
    stillAt = -1,
    onplayclip,
    onstill,
    oncaptionmoved,
    oncrop,
    onresetcrop,
    oncaptiony,
    onword,
    locked = false,
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
    // Where the frame the still was read for starts. It is only shown while
    // the playhead is in that frame, never wherever the playhead happens
    // to be when the picture goes stale.
    stillAt?: number;
    onplayclip?: (clip: ClipEntry) => void;
    // Asks for the frame at a moment of the episode, for as long as the
    // app cannot show that moment itself, and says null once it can.
    onstill?: (at: number | null) => void;
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
    // Corrects one word of the episode, by the moment it was spoken. The
    // captions in the video preview are where words are corrected, so this
    // is the one hand that reaches out of the picture.
    onword?: (start: number, text: string) => Promise<void>;
    // True while this clip is being rendered. What is being written into a
    // file cannot be changed while it is written, so its words are only
    // there to read until it is done, the way they were while they were
    // corrected on the playhead.
    locked?: boolean;
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
  // anything at all. While the machine is busy the app often cannot
  // read the file, and then a seek is dropped and the picture stays where
  // it was. The workspace fills in with a frame read from the file.
  let ready = $state(false);
  let shows = $state(-1);
  // One frame of the episode. The playhead can only ever stand on one, so
  // it is the smallest difference between two moments that means anything.
  const frameOf = $derived(source.fps > 0 ? 1 / source.fps : 1 / 30);
  const stale = $derived(pictureIsStale({ ready, shows, at: time, frame: frameOf }));

  // Which frame is really on screen, where the browser can say, and Safari
  // can since 15.4: requestVideoFrameCallback answers with the moment of
  // every frame the video puts up. The video's clock cannot say it. Safari
  // says a seek has landed, seeked, and moves its clock there, before the
  // new frame is on screen: 1 to 15 ms later on an idle Mac, 110 ms for a
  // cold first frame, and longer while the machine is busy, which right
  // after a search it is, placing the crop of every clip. Taken from the
  // clock, the picture read as the playhead's the moment seeked came, the
  // frame the engine read was taken away, and the frame before showed: on
  // the first press of the space bar after a search, a frame from wherever
  // the video preview was before, then the clip. With the frames known,
  // nothing else sets shows, and the clock only stands in where the
  // browser cannot say.
  const framesKnown = typeof HTMLVideoElement !== "undefined" && "requestVideoFrameCallback" in HTMLVideoElement.prototype;
  let watching = 0;
  function watchFrames() {
    if (!framesKnown || !video) return;
    video.cancelVideoFrameCallback(watching);
    watching = video.requestVideoFrameCallback((_, meta) => {
      // A jump the playing clip makes by itself moves the playhead and the
      // picture together, in the frame loop, see tick.
      if (!jumping) shows = meta.mediaTime;
      watchFrames();
    });
  }
  $effect(() => {
    onstill?.(stale ? time : null);
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
    return pieceIndex(pieces, t);
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
      // onloadedmetadata sends it to the playhead, wherever that stands
      // by then. A listener of its own for every seek asked for meanwhile
      // sent it to each of them in turn when the file came, the clip
      // chosen before the last among them, and Chromium could be left
      // seeking for good, with the play waiting on it.
      //
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
      if (!video) return;
      if (!shouldChase({ wanted, at: video.currentTime, playing: !video.paused, tries })) {
        wanted = -1;
        return;
      }
      tries++;
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
  // video element that has not read the file yet refuses to play, which
  // used to mean the first press of the space bar did nothing at all and
  // the second one worked. The press is remembered instead, and the picture
  // starts the moment it can.
  let wantPlay = false;

  // Whether this play is the chosen clip's, with its cuts jumped and a stop
  // at its end, or the episode's, straight on from the playhead.
  let playsClip = false;

  function play() {
    if (!video) return;
    endCorrection();
    // Playing moves the playhead on, so the keyboard's word goes.
    keyed = null;
    walked = 0;
    // The clip plays when the playhead stands in it, when it has just played
    // to its end, which is where the playhead is left, and when it loops.
    // Anywhere else the playhead was put there to look at that part of the
    // episode, clip or no clip, so the episode plays on from there. It used
    // to go back to the start of the chosen clip, and a part of the episode
    // could not be heard at all while a clip was chosen.
    playsClip =
      !!clip &&
      (looping ||
        (time >= clipStart - frameOf / 2 && time < clipEnd - 0.05) ||
        Math.abs(time - clipEnd) <= 0.05 + frameOf);
    if (clip && playsClip) {
      // A clip plays from the playhead while the playhead stands inside it,
      // otherwise from its start.
      //
      // Half a frame of room at that start, because the playhead is not
      // where it was put: picking a clip sends it to the clip's first
      // second and the picture answers with the frame it is showing, which
      // begins a hair before. Read exactly, the playhead was then outside
      // the clip it had just been put at the start of, so every press of
      // the space bar after picking a clip seeked before it played, and a
      // seek is the one thing that can refuse a play.
      if (time < clipStart - frameOf / 2 || time >= clipEnd - 0.05) time = clipStart;
      atPiece = pieceAt(time);
      // And only when the picture really has to move. A seek that changes
      // nothing still interrupts, still answers with nothing, and still
      // leaves a chase running with nothing to answer it.
      if (Math.abs(video.currentTime - time) > frameOf / 2) goTo(time);
      onplayclip?.(clip);
    }
    wantPlay = true;
    video.play().catch(() => {
      if (!wantPlay || !video) return;
      // Two things refuse a play, and they are answered by two different
      // events. A picture that has not read enough of the file yet says so
      // with canplay. A picture interrupted by a seek says so with seeked,
      // and it may never say canplay again, because it could already play
      // and nothing about that changed. Waiting only for canplay is how a
      // press of the space bar was lost and the second one worked.
      const again = () => {
        if (wantPlay && video && video.paused) void video.play().catch(() => {});
      };
      video.addEventListener("canplay", again, { once: true });
      video.addEventListener("seeked", again, { once: true });
    });
    cancelAnimationFrame(frame);
    frame = requestAnimationFrame(tick);
  }

  // A jump the playing clip makes by itself, over a cut or from its end back
  // to its start while it loops. Until the picture has landed, the playhead
  // stays with the picture: moved at once, it would take the crop frame and
  // the captions with it while the old frame is still on screen, and the
  // picture would read as not showing the playhead, which is what calls in
  // a still frame from the file. That still, read for another moment, was
  // the frame that flashed on every loop.
  let jumping = false;

  function tick() {
    frame = 0;
    if (!video) return;
    // Where the video is now. A jump this loop made has just landed when it
    // is no longer seeking, and then the playhead goes wherever it went,
    // back to the start of a loop too. A pause while it was on its way
    // ends it there, and onseeked and ontimeupdate follow it the way they
    // follow any seek made while paused, see jumpStep.
    const step = jumpStep(jumping, video);
    if (step === "wait") {
      frame = requestAnimationFrame(tick);
      return;
    }
    jumping = false;
    if (step === "paused") return;
    const landed = step === "landed";
    if (clip && playsClip && pieces.length) {
      // The episode plays through what the clip cuts out, so the playhead
      // jumps every cut and stops where the clip ends.
      // The pieces change under the player whenever a cut is taken out or
      // put back, so the piece being played can be gone by this frame.
      atPiece = playingPiece(pieces, atPiece, video.currentTime);
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
        jumping = true;
        video.currentTime = pieces[atPiece].start;
        frame = requestAnimationFrame(tick);
        return;
      }
    }
    // Otherwise the playhead only goes forward while the video plays. The
    // clock WebKit hands out while playing is worked out from the wall clock
    // between the reports of the player underneath, and it is set back
    // whenever a report says the picture is behind, which it is while
    // playing starts and whenever the file is slow to read. Followed as it
    // is, the playhead, the lit word, the caption and the crop went back
    // and forth over the picture for as long as playing took to settle.
    //
    // And a video that has read nothing of the file yet has no clock at
    // all, so the playhead stands where it is, see playingAt.
    const empty = video.readyState === HTMLMediaElement.HAVE_NOTHING;
    time = playingAt(time, { clock: video.currentTime, empty, seeking: video.seeking, landed });
    // Where the browser says which frame is on screen, that is the picture,
    // see watchFrames, and the clock is not. Once a jump the playing clip
    // made has landed, the picture goes with the playhead for the frame or
    // two until the next frame is put up, so no still is read for it.
    //
    // Where it cannot say, playing, the picture is where the playhead is,
    // once no seek is on its way, so no still is ever read from the file
    // for it. Not while the video has no frame of where it is, HAVE_NOTHING
    // or HAVE_METADATA: the still is the only picture there is, and it
    // stays until the video has landed where it was sent, see onseeked.
    if (framesKnown) {
      if (landed) shows = time;
    } else if (!video.seeking && video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
      shows = time;
    }
    if (!video.paused) frame = requestAnimationFrame(tick);
  }

  function onError() {
    // Nothing is on screen once it has failed, so the still takes over.
    ready = false;
    shows = -1;
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
      inside: insideClip(clip.segments, time, frameOf),
      moved: clip.segments[piece]?.moved ?? false,
    };
  });

  // The word being corrected: where it stands in the caption, which word of
  // the episode it is, the whole of that word, and the piece of it drawn
  // here. The last two differ only for a word that was split in two.
  let fixing = $state<{ at: number; said: number; whole: string; piece: string } | null>(null);
  // The word a correction is on its way to disk for, so it says it is not
  // settled yet the way everything else in the app does.
  let savingWord = $state<number | null>(null);
  // The moment the caption box is showing while a word is being corrected.
  // Without it the caption would move on under the caret, which is the same
  // reason the lens freezes the row it magnifies.
  let frozen = $state(0);
  const shown = $derived(fixing ? frozen : time);

  // The caption standing at the playhead, with the word being spoken. The
  // engine hands over the lines and the look, so nothing about captions is
  // decided twice.
  const caption = $derived.by(() => {
    if (!clip || !captions?.captions?.length) return null;
    if (shown < clipStart - 0.05 || shown > clipEnd + 0.05) return null;
    const at = inClipTime(shown);
    return captions.captions.find((c) => at >= c.start && at < c.end) ?? null;
  });

  const spoken = $derived(inClipTime(shown));

  // The word lit, counted across the caption's lines, by the rule the
  // render and the clip timeline go by. firstOfRow is where each line's
  // words begin in that count.
  const lit = $derived(caption ? litWord(caption.lines ?? [], spoken) : -1);
  const firstOfRow = $derived.by(() => {
    const out: number[] = [];
    let n = 0;
    for (const line of caption?.lines ?? []) {
      out.push(n);
      n += line.words.length;
    }
    return out;
  });

  // The keyboard's word: the word Shift and an arrow walked to wears the
  // frame the pointer puts on a word, so it is the word Enter opens. It is
  // the same with the highlight on or off, and with it off nothing else in
  // the picture says which word is spoken. It is the caption holding the
  // keyboard: it stays on a word saved with Enter, and goes with anything
  // else, see forget, and when the playhead leaves the word, the clip
  // playing among them.
  let keyed = $state<{ start: number; end: number } | null>(null);
  // When Shift and an arrow were pressed. The next move of the playhead is
  // the walk landing, and its word becomes the keyboard's word.
  let walked = 0;
  // A frame's worth of give at the edges of the word, because the picture
  // answers a seek with the frame it shows, which can begin a hair before
  // the word does.
  const give = 0.05;
  $effect(() => {
    const at = spoken;
    untrack(() => {
      if (walked && Date.now() - walked < 1000) {
        walked = 0;
        const word = wordAt(at);
        keyed = word ? { start: word.start, end: word.end } : null;
        return;
      }
      if (keyed && (at < keyed.start - give || at >= keyed.end + give)) keyed = null;
    });
  });

  // The keyboard's word put where a walk into the clip beside landed. The
  // walk cannot catch that landing itself: the clip changes first, with no
  // words yet, and the playhead only reaches the word once its captions
  // have arrived, which can be any time later. So the workspace says where
  // it put the playhead, and that word takes the frame.
  export function keyAt(t: number) {
    walked = 0;
    const word = wordAt(inClipTime(t));
    keyed = word ? { start: word.start, end: word.end } : null;
  }

  // A colour taken from the picture, for the captions column's colour
  // pickers. While it is asked for, the video preview is a place to point
  // at: a loupe follows the pointer the way the Mac's own colour sampler
  // does, the pixels under it drawn large and the one in its middle the
  // colour, the captions are drawn in that colour as the pointer moves,
  // a click takes it, and Escape or a click anywhere else leaves the
  // colour as it was.
  //
  // What is read is the episode's own frame, from the video or the still
  // drawn over it, not the screen: the crop, its shade and the captions
  // lie over the picture, and a colour read through them would be the
  // app's and not the episode's.
  let sampling = $state<{ over: (hex: string | null) => void; done: (hex: string | null) => void } | null>(null);
  let loupe = $state<{ x: number; y: number; hex: string } | null>(null);
  let lens = $state<HTMLCanvasElement>();
  let stillImage = $state<HTMLImageElement>();

  export function sampleColour(over: (hex: string | null) => void, done: (hex: string | null) => void) {
    finishSampling(null);
    loupe = null;
    sampling = { over, done };
  }

  function finishSampling(hex: string | null) {
    const was = sampling;
    sampling = null;
    loupe = null;
    was?.done(hex);
  }

  // The colour of the frame under a point of the screen, and where that
  // point is in the video preview, or nothing off the picture. The lens
  // is drawn on the way, nine pixels by nine around it.
  function pixelAt(clientX: number, clientY: number): { x: number; y: number; hex: string } | null {
    if (!lens || !screen) return null;
    const source: HTMLVideoElement | HTMLImageElement =
      stillImage && stillImage.complete && stillImage.naturalWidth > 0 ? stillImage : video;
    const w = source instanceof HTMLVideoElement ? source.videoWidth : source.naturalWidth;
    const h = source instanceof HTMLVideoElement ? source.videoHeight : source.naturalHeight;
    if (!w || !h || (source instanceof HTMLVideoElement && source.readyState < 2)) return null;
    // The picture is contained in the screen and keeps its shape, so it is
    // as large as the side that runs out first allows, in the middle.
    const r = screen.getBoundingClientRect();
    const scale = Math.min(r.width / w, r.height / h);
    const left = r.left + (r.width - w * scale) / 2;
    const top = r.top + (r.height - h * scale) / 2;
    const fx = Math.floor((clientX - left) / scale);
    const fy = Math.floor((clientY - top) / scale);
    if (fx < 0 || fy < 0 || fx >= w || fy >= h) return null;
    const ctx = lens.getContext("2d", { willReadFrequently: true });
    if (!ctx) return null;
    ctx.imageSmoothingEnabled = false;
    ctx.clearRect(0, 0, 9, 9);
    try {
      ctx.drawImage(source, fx - 4, fy - 4, 9, 9, 0, 0, 9, 9);
      const [red, green, blue] = ctx.getImageData(4, 4, 1, 1).data;
      return { x: clientX - r.left, y: clientY - r.top, hex: rgbToHex(red, green, blue) };
    } catch {
      return null;
    }
  }

  function sampleMove(event: PointerEvent) {
    const at = pixelAt(event.clientX, event.clientY);
    if (at?.hex !== loupe?.hex) sampling?.over(at?.hex ?? null);
    loupe = at;
  }

  function sampleLeave() {
    if (loupe) sampling?.over(null);
    loupe = null;
  }

  function sampleTake(event: PointerEvent) {
    event.preventDefault();
    event.stopPropagation();
    if (event.button !== 0) return;
    const at = pixelAt(event.clientX, event.clientY);
    if (at) finishSampling(at.hex);
  }

  // Escape leaves the colour as it was, before any other shortcut hears
  // it, and a press anywhere but the picture does the same and goes on to
  // do whatever it was for.
  $effect(() => {
    if (!sampling) return;
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      finishSampling(null);
    };
    const elsewhere = (event: PointerEvent) => {
      if (!(event.target as Element | null)?.closest?.(".sampler")) finishSampling(null);
    };
    window.addEventListener("keydown", escape, true);
    window.addEventListener("pointerdown", elsewhere, true);
    return () => {
      window.removeEventListener("keydown", escape, true);
      window.removeEventListener("pointerdown", elsewhere, true);
    };
  });

  // The word of the caption box at a moment of the clip's clock, among the
  // words that can be corrected.
  function wordAt(at: number): { start: number; end: number } | null {
    for (const line of rows) {
      for (const { word, said } of line) {
        if (said && at >= word.start - give && at < word.end) return word;
      }
    }
    return null;
  }

  // Words are corrected in the picture, where they are read. A correction
  // belongs to the episode, so it takes a clip to know which words these
  // are and somewhere to send it.
  const correctable = $derived(!!clip && !!onword && !locked);

  // Every word of the caption beside the word of the episode it stands for,
  // which the engine says with the word. A correction that reads as two
  // words, or a word shown in halves, is drawn as two, and both point back
  // at the one word they came from.
  const rows = $derived.by(() =>
    (caption?.lines ?? []).map((line) =>
      line.words.map((word) => ({
        word,
        said:
          correctable && word.said !== undefined
            ? { start: word.said, text: word.whole ?? word.text }
            : null,
      })),
    ),
  );

  // A word is in the caption twice when it was split in two, and while one
  // half is being corrected it holds the whole word, so the other half is
  // not drawn at all.
  function doubled(word: { start: number }, said: { start: number } | null): boolean {
    return !!fixing && !!said && said.start === fixing.said && word.start !== fixing.at;
  }

  // A caption word says what it says by hand, not through the template.
  // While a word is being corrected the browser owns what is inside it,
  // and a template that wrote there would take the caret with it. This
  // writes only when the word itself has changed, which never happens
  // while a hand is in it.
  function says(node: HTMLElement, text: string) {
    node.textContent = text;
    return {
      update(next: string) {
        if (node.textContent !== next) node.textContent = next;
      },
    };
  }

  // Taking a word takes the picture with it: a caption that moved on under
  // the caret would leave the hand correcting a word that is no longer
  // there.
  function takeWord(
    node: HTMLElement,
    word: { start: number; text: string },
    said: { start: number; text: string } | null,
  ) {
    if (!said || !correctable) return;
    frozen = time;
    if (video && !video.paused) toggle();
    fixing = { at: word.start, said: said.start, whole: said.text, piece: word.text };
    // The caret is already where the hand put it. It is only moved when
    // what is drawn here is half of the word being corrected: a word split
    // in two is drawn in two halves, and correcting either hands back the
    // whole of it, so the half that was clicked makes way for it.
    if (node.textContent === said.text) return;
    node.textContent = said.text;
    const range = document.createRange();
    range.selectNodeContents(node);
    range.collapse(false);
    const at = window.getSelection();
    at?.removeAllRanges();
    at?.addRange(range);
  }

  async function dropWord(node: HTMLElement, word: { start: number; text: string }) {
    const was = fixing;
    fixing = null;
    if (!was) return;
    // A word is one word on a line, whatever was typed into it: the
    // newlines a paste brings are spaces, and a run of spaces is one.
    const text = (node.textContent ?? "").replace(/\s+/g, " ").trim();
    if (!text || text === was.whole) {
      node.textContent = word.text;
      return;
    }
    savingWord = was.at;
    try {
      await onword?.(was.said, text);
      // What comes back is the engine's answer, drawn by the template.
      // Until it lands the word keeps what was typed, so the correction is
      // never shown coming undone and going in again.
    } catch {
      // The workspace says what went wrong. The word goes back to what it
      // was, so the picture never shows a correction that was refused.
      node.textContent = word.text;
    } finally {
      // Only if it is still this word's turn. Correcting a second word
      // while the first is still on its way leaves two of these running,
      // and the older one finishing would otherwise say the newer one had
      // landed.
      if (savingWord === was.at) savingWord = null;
    }
  }

  // Playing ends a correction. The word lets go of the caret, which saves
  // what was typed the way Enter does, and whatever was selected in the
  // caption box goes with it, so the captions follow the picture again.
  // A click on the video preview lands on the crop frame, and the frame
  // refuses the pointer so a drag moves the crop, which also stops the
  // browser taking the focus away. So the word kept the caret while the
  // video played, with the caption held on the moment it was clicked.
  function endCorrection() {
    const on = document.activeElement;
    if (on instanceof HTMLElement && on.classList.contains("word") && screen?.contains(on)) on.blur();
    const selected = window.getSelection();
    if (selected?.rangeCount && screen?.contains(selected.anchorNode)) selected.removeAllRanges();
  }

  function wordKey(event: KeyboardEvent) {
    const node = event.currentTarget as HTMLElement;
    if (event.key === "Enter") {
      // A caption word is one line and stays one line, so Enter is what
      // finishes it rather than what breaks it.
      event.preventDefault();
      // Handled here, so the Enter that saves a word is not also the Enter
      // that opens the word at the playhead again.
      event.stopPropagation();
      letGo(node);
    } else if (event.key === "Escape") {
      event.preventDefault();
      // Handled here too: Escape leaves the word as it was and goes back to
      // the word chosen, and only a second Escape lets that go.
      event.stopPropagation();
      if (fixing) node.textContent = fixing.piece;
      fixing = null;
      letGo(node);
    }
  }

  // A word let go of with Enter or Escape keeps the keyboard's frame, since
  // the playhead is still on it, and gives up the caret. The caret went on
  // blinking in a word already saved: WebKit keeps the selection in a field
  // the keyboard has left, and draws the caret where it is.
  function letGo(node: HTMLElement) {
    keyed = { start: Number(node.dataset.at), end: Number(node.dataset.to) };
    node.blur();
    const selected = window.getSelection();
    if (selected?.rangeCount && node.contains(selected.anchorNode)) selected.removeAllRanges();
  }

  // Enter opens the word at the playhead for correcting, the way Enter
  // renames the item chosen in the Finder: the whole of it is selected, so
  // typing replaces it and the arrows move inside it. Shift and the arrows
  // walk the playhead from word to word, so the two together correct a
  // caption without the pointer. Between two words, which is where a
  // pause leaves the playhead, it is the word just spoken.
  function openSpoken(): boolean {
    const words = [...(screen?.querySelectorAll<HTMLElement>(".word.correctable") ?? [])];
    let pick: HTMLElement | null = null;
    for (const node of words) {
      if (Number(node.dataset.at) <= spoken + 1e-6) pick = node;
    }
    pick ??= words[0] ?? null;
    if (!pick) return false;
    pick.focus();
    const range = document.createRange();
    range.selectNodeContents(pick);
    const at = window.getSelection();
    at?.removeAllRanges();
    at?.addRange(range);
    return true;
  }

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
    // A word takes the caret where the hand put it, and the browser only
    // does that if nothing refuses the pointer on the way down. So a drag
    // that starts on a word starts without refusing it, and the word is
    // let go of the moment the hand moves instead.
    const word = (event.target as HTMLElement | null)?.closest?.(".word") as HTMLElement | null;
    if (!word) event.preventDefault();
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
      if (word) {
        word.blur();
        // Pressing on a word without refusing the pointer is what puts the
        // caret where the hand went down, and it is also what starts the
        // browser selecting letters. Once the hand is moving this is a
        // drag, so whatever it selected on the way goes.
        window.getSelection()?.removeAllRanges();
      }
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
        // A click on a word is the hand asking to correct it, so the
        // picture stays where it is rather than starting to play.
        if (!word) toggle();
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
  // has the keyboard. The arrows bring the keyboard's word, and Enter opens
  // it.
  function onKey(event: KeyboardEvent) {
    // Any key but the ones that walk and open takes the keyboard's word
    // away, the arrows without Shift and Escape among them. A key held on
    // its own to make a shortcut takes nothing yet.
    if (!["Shift", "Meta", "Control", "Alt", "CapsLock"].includes(event.key)) {
      const walking =
        event.shiftKey &&
        !event.metaKey &&
        !event.ctrlKey &&
        !event.altKey &&
        (event.key === "ArrowLeft" || event.key === "ArrowRight");
      if (!walking && event.key !== "Enter") forget();
    }
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    const on = document.activeElement as HTMLElement | null;
    const tag = on?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || on?.isContentEditable) return;
    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      // The clip timeline moves the playhead. A slider holding the
      // keyboard, an edge of the clip, takes the arrows for itself.
      if (on?.getAttribute("role") === "slider" || document.querySelector("dialog[open]")) return;
      // Shift walks by words, and the word it lands on becomes the
      // keyboard's.
      if (event.shiftKey) walked = Date.now();
      return;
    }
    if (event.key === "Enter") {
      if (event.defaultPrevented || event.shiftKey || event.repeat) return;
      // A button or a link holding the keyboard is pressed by Enter, and a
      // box asking something takes it for its answer.
      if (tag === "BUTTON" || tag === "A" || on?.getAttribute("role") === "option") return;
      if (document.querySelector("dialog[open]")) return;
      if (openSpoken()) event.preventDefault();
      return;
    }
    if (event.code !== "Space" || event.shiftKey) return;
    event.preventDefault();
    toggle();
  }

  // The keyboard's word is the caption holding the keyboard, so whatever
  // takes the keyboard or the pointer anywhere else takes it away: a press
  // of the pointer, wherever it lands, and a field getting the keyboard,
  // the target among them. It stayed through all of them, and read as a
  // word still chosen while the hand was busy somewhere else.
  function forget() {
    keyed = null;
    walked = 0;
  }
  function focusMoves(event: FocusEvent) {
    const to = event.target as HTMLElement | null;
    if (!to?.classList?.contains("word")) forget();
  }

  onMount(() => {
    watchFrames();
    window.addEventListener("keydown", onKey);
    window.addEventListener("pointerdown", forget, true);
    window.addEventListener("focusin", focusMoves);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("pointerdown", forget, true);
      window.removeEventListener("focusin", focusMoves);
      cancelAnimationFrame(frame);
      if (framesKnown) video?.cancelVideoFrameCallback(watching);
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
        sideways to place it, and the black box up or down for the captions. Click a word in the
        caption box to correct it, or walk to it with Shift and the arrows and press Enter: Enter
        saves it, Escape leaves it, and two words split it in two.
      </Info>
    </span>
    <!-- The still of the frame the playhead is in, and while playing of
         where the play began, see stillFits. -->
    {#if still && stale && stillFits(stillAt, time, source.fps, !paused)}
      <img src={still} alt="" bind:this={stillImage} />
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
        // While a seek is on its way the element answers with where it was
        // sent, and the picture is still the frame it had. Taken as the
        // picture, every jump over a cut read as a picture somewhere else,
        // and a still was read from the file in the middle of playing. A
        // jump the playing clip makes is the frame loop's to the end, see
        // onseeked.
        if (!framesKnown && !video.seeking && !jumping) shows = video.currentTime;
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
        if (framesKnown) watchFrames();
        else shows = video.currentTime;
      }}
      onseeked={() => {
        ready = true;
        // Landed is not shown: Safari says seeked before the frame is on
        // screen, see watchFrames, which is what says when it is.
        //
        // A jump the playing clip made by itself moves the playhead and the
        // picture together, in the frame loop. Moved here, the picture was
        // a frame ahead of the playhead and read as stale for that frame.
        if (framesKnown) watchFrames();
        else if (!jumping) shows = video.currentTime;
        wanted = -1;
      }}
      onemptied={() => {
        // The element has just been reset and is showing nothing at all.
        // load() does that, and chase() calls load() when a seek will not
        // land, which is exactly when the machine is busy and the picture
        // matters most.
        //
        // Saying so is what puts the still in its place. pictureIsStale
        // asks whether the picture is ready and what second it is showing,
        // and ready was set in two places and cleared in none, so after a
        // reset the app went on believing a black element was showing
        // the right frame. With the seek that failed anywhere within half
        // a second of the playhead, nothing counted as stale, no frame was
        // asked for, and nothing was drawn over the black. That is a video
        // preview that goes and does not come back, and clicking about
        // near a cut is all it takes, because those are the small seeks.
        ready = false;
        shows = -1;
      }}
      onerror={onError}
      onclick={toggle}
    ></video>
    <!-- The crop is drawn only while the playhead stands in the clip. Anywhere
         else the picture is the episode, not the short, so nothing is laid
         over it. -->
    {#if crop?.inside}
      <div class="shade" style="left: 0; width: {crop.left}%"></div>
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="frame"
        class:lit={dragLeft !== null}
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
    <!-- Captions switched off in the captions column are not drawn, so the
         video preview shows the short as it will be rendered. -->
    {#if caption && captions && captions.style.text !== false}
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
          class:waiting={savingWord !== null}
          class:holding={dragCaptions !== null}
          style="background: {captions.style.boxOn === false ? 'transparent' : captions.style.box};
                 border-radius: {px(captions.style.radius)}px;
                 padding: {px(captions.style.padY)}px {px(captions.style.padX)}px"
          title="Drag the handle up or down to place the captions"
          onpointerdown={grabCaptions}
        >
          <!-- The handle the captions are moved by. It is a ring around
               the box, reaching out over the picture and lying under the
               words, so the whole of the box that is not a word is
               something to take hold of and the words stay one click from
               being corrected.

               It is there because the box stopped being grabbable the day
               the words in it became fields. What was left to take hold of
               was the padding and the spaces between words, which is a few
               pixels of a preview, and nothing said where they were. So
               the handle says where it is: it draws itself the moment the
               pointer comes near, the way the info marks do, and it never
               covers a word.

               Its pointer is the up and down one, not the hand. The crop
               frame it sits inside is moved sideways and wears the hand,
               and two things that move in different directions should not
               say the same thing about themselves. -->
          <span class="hold"></span>
          {#each rows as line, row (row)}
            <div class="line">
              <!-- The key carries the place as well as the moment. Two
                   caption words can stand at the same moment: a word
                   measured as lasting no time at all and corrected into
                   two is split into two halves of nothing, both starting
                   where it did, and a key that was the moment alone would
                   then be the same key twice, which is not a caption that
                   looks wrong but an app that stops. -->
              <!-- Each word says which moment it stands for. Nothing on
                   screen reads it: it is there so a probe can, because
                   which word lights up is decided by comparing two clocks
                   and there is no other way to see the comparison. The
                   playhead landing a thousandth of a millisecond before a
                   word start, and so lighting nothing, was found with
                   these and could not have been found without them. -->
              {#each line as { word, said }, i (`${i}:${word.start}`)}{#if !doubled(word, said)}{#if i > 0}{" "}{/if}<span
                    class="word"
                    class:correctable={!!said}
                    class:fixing={fixing?.at === word.start}
                    class:keyed={!!keyed && !fixing && !!said && keyed.start === word.start}
                    class:now={captions.style.highlight && firstOfRow[row] + i === lit}
                    contenteditable={said ? "plaintext-only" : null}
                    spellcheck="false"
                    data-at={word.start}
                    data-to={word.end}
                    role={said ? "textbox" : null}
                    aria-label={said ? "Correct this word" : null}
                    title={said
                      ? "Click to correct this word. Enter saves it, Escape leaves it. Two words split it in two"
                      : null}
                    style="--pill: {captions.style.highlightColour}"
                    use:says={word.text}
                    onfocusin={(e) => takeWord(e.currentTarget, word, said)}
                    onfocusout={(e) => dropWord(e.currentTarget, word)}
                    onkeydown={wordKey}
                  ></span
                  >{/if}{/each}
            </div>
          {/each}
        </div>
      </div>
    {/if}
    <!-- The video preview giving a colour: over everything else on it, so
         the crop, the captions and the info mark take no clicks while it
         does, with the loupe centred on the pointer in place of the
         pointer itself. -->
    {#if sampling}
      <div
        class="sampler"
        role="application"
        aria-label="Take a colour from the video preview"
        title="Click to take this colour. Escape leaves it as it was"
        onpointermove={sampleMove}
        onpointerleave={sampleLeave}
        onpointerdown={sampleTake}
      >
        <div
          class="loupe"
          class:shown={!!loupe}
          style="left: {loupe?.x ?? 0}px; top: {loupe?.y ?? 0}px; --colour: {loupe?.hex ?? 'transparent'}"
        >
          <canvas bind:this={lens} width="9" height="9"></canvas>
          <span class="aim"></span>
          <span class="said num">{loupe?.hex.slice(1).toUpperCase() ?? ""}</span>
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
     size of the app itself. Nothing here measures anything. */
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
    background: var(--backdrop);
    pointer-events: none;
  }

  /* The crop. Its line and corners are the frame in app.css, the same
     as the window on the range picker and the clip on the clip timeline. */
  .frame {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 2;
    cursor: grab;
    touch-action: none;
  }

  .frame.lit {
    cursor: grabbing;
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

  .box {
    position: relative;
  }

  /* Everything in the box that is not a word takes hold of it, and the
     pointer says which way it goes. The crop frame it sits inside is
     moved sideways and wears the hand, and two things that move in
     different directions should not say the same thing about themselves. */
  .box.draggable {
    pointer-events: auto;
    cursor: ns-resize;
    touch-action: none;
  }

  /* The ring reaches past the box, so there is something to take hold of
     even where a word runs to the end of a line. */
  .box.draggable .hold {
    position: absolute;
    inset: -9px;
    border: 1px solid transparent;
    border-radius: 12px;
  }

  /* It says where it is only while the pointer is near it, like every
     other mark in the app that explains one thing where that thing is. */
  .box.draggable:hover .hold,
  .box.draggable.holding .hold {
    border-color: var(--accent-hi);
  }

  /* The lines stand over the ring rather than under it. The ring is
     placed, and anything placed is painted over anything that is not, so
     without this it would lie across the words and no word could be
     clicked at all. */
  .line {
    position: relative;
  }

  /* A word is corrected where it is read, in the picture, so the word takes
     the pointer for itself whether or not the box around it takes a drag.
     The frame and the caret are the interface's, not the render's: nothing
     here is ever burned into a short. */
  .word.correctable {
    pointer-events: auto;
    cursor: text;
    caret-color: var(--accent-hi);
    outline: 1px solid transparent;
  }

  .word.correctable:hover,
  .word.keyed {
    outline-color: var(--accent-hi);
  }

  /* The word being corrected wears the frame and nothing else. A
     background would be a second pill, in the app's colour, on a word that
     is not the one being spoken, and it would take the pill off the word
     that is: correcting the spoken word swapped the colour the render
     burns in for the app's accent, which is the caption saying something
     about itself that is not true. What the picture shows stays what the
     render will show, and the frame is the only thing the interface adds.

     It is the hover frame twice over, which is the whole difference
     between the two. After Enter the word goes back to the frame of the
     two it had before it was opened: the one under the pointer, or the
     keyboard's, which stays on it while the playhead does. */
  .word.fixing {
    outline-width: 2px;
    outline-color: var(--accent-hi);
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

  /* The pop is in app.css, because the caption block on the clip timeline
     makes the same one. */
  .line span.now {
    background: var(--pill);
    animation: pop 0.22s ease-out;
  }

  @media (prefers-reduced-motion: reduce) {
    .line span.now {
      animation: none;
    }
  }


  /* The video preview giving a colour. The pointer is the loupe. */
  .sampler {
    position: absolute;
    inset: 0;
    z-index: 10;
    cursor: none;
  }

  /* The loupe of the Mac's colour sampler: a round lens over the pointer,
     the pixels under it drawn ten times over and not smoothed, so each one
     is seen, the one in the middle marked, and a ring in the colour it
     would take. Its hex under it, the way the colour panel says it. */
  .loupe {
    position: absolute;
    width: 90px;
    height: 90px;
    margin: -45px 0 0 -45px;
    pointer-events: none;
    visibility: hidden;
  }

  .loupe.shown {
    visibility: visible;
  }

  .loupe canvas {
    display: block;
    width: 90px;
    height: 90px;
    border-radius: 50%;
    image-rendering: pixelated;
    box-shadow:
      0 0 0 4px var(--colour),
      0 0 0 5px rgba(255, 255, 255, 0.85),
      0 4px 14px 5px rgba(0, 0, 0, 0.5);
  }

  .aim {
    position: absolute;
    left: 40px;
    top: 40px;
    width: 10px;
    height: 10px;
    box-sizing: border-box;
    border: 1px solid #fff;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.6);
  }

  .loupe .said {
    position: absolute;
    left: 50%;
    top: 100px;
    transform: translateX(-50%);
    padding: 2px 6px;
    border-radius: var(--radius-s);
    background: rgba(0, 0, 0, 0.7);
    color: #fff;
    font-size: var(--size-s);
    white-space: nowrap;
  }
</style>
