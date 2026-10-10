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
  // button, one playhead: playing starts where the playhead stands. With
  // the playhead on the selected clip it jumps the clip's cuts and stops
  // where the clip ends, and on the video it plays the episode straight on.
  // While the playhead is on the clip and inside it, its captions are drawn
  // inside the crop the way the render will burn them in. On the video the
  // video preview shows the video and nothing else.
  import { onMount, untrack, type Snippet } from "svelte";
  import { insideClip, litWord, type Piece } from "../lib/flow";
  import { inEpisode, onVideo, placeFor, placeOf, playedToEnd, playFrom, type About, type Place, type Playhead } from "../lib/playhead";
  import { FrameQueue, type Shown } from "../lib/frames/queue";
  import type { Movie } from "../lib/frames/mp4";
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
  import { asleep, keysElsewhere } from "../lib/keys";

  let {
    path,
    source,
    clip,
    captions = null,
    time = $bindable(0),
    onplayclip,
    oncaptionmoved,
    oncrop,
    onresetcrop,
    oncaptiony,
    onword,
    locked = false,
    strip,
    paused = $bindable(true),
    looping = $bindable(false),
    onClip = $bindable(false),
    offers = $bindable({ crop: "", savingCrop: false, hint: "" } as PlayerOffers),
    opening = false,
    pictured = $bindable(false),
    sleeping = false,
  }: {
    path: string;
    source: SourceView;
    clip: ClipEntry | null;
    // The captions of the selected clip, on the clip's own clock.
    captions?: CaptionsView | null;
    time?: number;
    // While the workspace has not yet decided where the playhead opens,
    // on the clip it opens on, nothing is drawn: the episode's first
    // frames were drawn and then left for the clip, a flicker Tim saw as
    // every episode opened. The queue opens the file meanwhile, so the
    // first frame is no later for it.
    opening?: boolean;
    onplayclip?: (clip: ClipEntry) => void;
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
    // Whether the playhead is on the chosen clip, so the space bar plays
    // the clip, or on the video, so it plays the episode straight on and
    // the clip is drawn dimmed wherever it is drawn. See lib/playhead.ts.
    onClip?: boolean;
    offers?: PlayerOffers;
    // Whether the canvas holds a picture of the episode yet, or has said
    // why it cannot. The workspace is not put in front of the person
    // before, see ready in Episode.svelte.
    pictured?: boolean;
    // The workspace is kept for coming back to while another episode is
    // in front, see App.svelte. Asleep, the video preview holds nothing
    // but the frame on its canvas and the file's index: no decoder on the
    // Go side, no frames kept, no sound card.
    sleeping?: boolean;
  } = $props();

  let screen: HTMLDivElement;
  // How tall the picture came out, which the captions are drawn against.
  // Reading it changes nothing, so it never sets the layout going again.
  let shell: HTMLDivElement;

  let dragLeft = $state<number | null>(null);
  let savingCrop = $state(false);
  let dragCaptions = $state<number | null>(null);
  let savingCaptions = $state(false);

  // Going back to the automatic crop is one click, so taking that back has
  // to be one click too. What a reset threw away is kept until the clip
  // changes or it is placed by hand again.
  let undoCrop = $state<{ key: string; piece: number; at: number; left: number } | null>(null);

  // The episode, played from its own file onto the canvas by the frame
  // queue, see lib/frames/. Every frame drawn, playing or paused, comes
  // from it, and while it plays so does the playhead.
  let canvas: HTMLCanvasElement;
  let queue: FrameQueue | null = null;
  // Why the episode cannot be shown, in a sentence, where the picture
  // would be. And why it plays without sound, or stopped decoding.
  let failed = $state("");
  let trouble = $state("");
  // How many device pixels the stylesheet gave the canvas, read from it as
  // it changes, so a new queue is told at once.
  let pixels: [number, number] = [0, 0];
  // One frame of the episode. The playhead can only ever stand on one, so
  // it is the smallest difference between two moments that means anything.
  const oneFrame = $derived(source.fps > 0 ? 1 / source.fps : 1 / 30);

  // A queue while the workspace is awake. Going to sleep closes it with
  // everything it holds, its decoders and its sound card among them, and
  // keeps only the file's index, so waking reads nothing from the file and
  // the frame on the canvas stays until the new queue draws the same one.
  //
  // Waking, the queue is opened once the workspace is on screen, in the
  // task after the frame that shows it: what is on the canvas is already
  // the frame it would draw, and starting a sound card and a decoder in
  // that frame held it back. Going to sleep, it is closed the same way.
  let known: Movie | null = null;
  $effect(() => {
    const url = mediaURL(path);
    if (sleeping) return;
    let q: FrameQueue | null = null;
    const stop = known ? afterPaint(() => (q = open(url))) : null;
    if (!stop) q = untrack(() => open(url));
    return () => {
      stop?.();
      if (!q) return;
      const going: FrameQueue = q;
      known = going.movie ?? known;
      if (!untrack(() => paused)) going.pause();
      paused = true;
      nailed = false;
      if (queue === going) queue = null;
      setTimeout(() => going.close());
    };
  });

  // Runs fn in the task after the next frame is painted. What it answers
  // with calls it off, whichever of the two waits is under way.
  function afterPaint(fn: () => void): () => void {
    let timer = 0;
    const frame = requestAnimationFrame(() => {
      timer = window.setTimeout(() => untrack(fn));
    });
    return () => {
      cancelAnimationFrame(frame);
      clearTimeout(timer);
    };
  }

  function open(url: string): FrameQueue {
    failed = "";
    trouble = "";
    const q = new FrameQueue(canvas, url, known);
    queue = q;
    if (pixels[0]) q.resize(pixels[0], pixels[1]);
    q.listen((s) => {
      if (queue === q) heard(s);
    });
    q.ready.catch((e: Error) => {
      if (queue !== q) return;
      failed = e.message;
      paused = true;
      pictured = true;
    });
    q.setProgram(...programOf(place));
    sought = null;
    if (!untrack(() => opening)) {
      q.seek(time);
      sought = q;
    }
    return q;
  }

  // The queue that has had its first seek. One opened while the workspace
  // was still opening has its first seek once it knows where, from a
  // gesture or from here.
  let sought: FrameQueue | null = null;
  $effect(() => {
    if (opening || !queue || sought === queue) return;
    sought = queue;
    queue.seek(untrack(() => time));
  });

  // What the queue says: where the playhead is and whether it plays. While
  // it plays the playhead is the sound being heard, on every animation
  // frame. A play of the clip that reaches its end leaves the playhead on
  // the clip, at its end, see lib/playhead.ts.
  function heard(s: Shown) {
    trouble = s.trouble;
    // A picture: the video preview has what it is going to show. A file
    // that cannot be opened at all says so where the picture would be, see
    // open. Trouble on the way is no picture: the sound stopping said the
    // video preview was ready before its first frame, and a workspace built
    // as the app starts went to sleep with nothing on its canvas.
    if (s.drew) pictured = true;
    if (s.ended) {
      paused = true;
      if (playedClip) {
        time = clipEnd;
        placed = playedToEnd(placed, false);
      } else {
        time = inEpisode(s.at, source.duration);
      }
      playedClip = false;
      return;
    }
    if (s.playing) {
      time = inEpisode(s.at, source.duration);
      return;
    }
    // Paused: where the queue stopped, or where it was sent, which is
    // where the playhead already is. A correction holds the caption on its
    // own moment, frozen, so this changes nothing under the caret.
    time = inEpisode(s.at, source.duration);
  }

  // What play plays for a place: on the video the whole episode straight
  // on, on the clip its pieces with the cuts jumped, looping or not.
  function programOf(where: Place): [Piece[] | null, boolean] {
    return where === "video" || !pieces.length ? [null, false] : [pieces, looping];
  }

  // The canvas is told its device pixels as the stylesheet lays it out.
  // Nothing is laid out from the answer.
  function watchSize(node: HTMLCanvasElement) {
    const seen = (entries: ResizeObserverEntry[]) => {
      const e = entries[entries.length - 1];
      // Nothing measured while the workspace is put away, see App.svelte.
      if (!node.isConnected || !e.contentRect.width) return;
      const device = e.devicePixelContentBoxSize?.[0];
      const ratio = window.devicePixelRatio || 1;
      pixels = device
        ? [device.inlineSize, device.blockSize]
        : [Math.round(e.contentRect.width * ratio), Math.round(e.contentRect.height * ratio)];
      queue?.resize(pixels[0], pixels[1]);
    };
    const watch = new ResizeObserver(seen);
    try {
      watch.observe(node, { box: "device-pixel-content-box" });
    } catch {
      watch.observe(node);
    }
    return { destroy: () => watch.disconnect() };
  }

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

  // Where the playhead is, on the clip or on the video, as the last gesture
  // that put it somewhere left it, see lib/playhead.ts. Only a gesture sets
  // it, through seek, and a play of the clip that reaches its end.
  let placed = $state<Playhead>(onVideo);
  const place = $derived(placeFor(placed, clip?.key));
  $effect(() => {
    onClip = place !== "video";
  });

  // The queue plays what the place says, and a clip whose pieces change
  // while it plays, a cut made, moved or put back, or loop switched on or
  // off, goes on from the playhead on what it is now. The same program
  // again changes nothing.
  $effect(() => {
    const program = programOf(place);
    untrack(() => queue?.setProgram(...program));
  });

  // Every gesture that puts the playhead somewhere comes through here, and
  // says whether it is about the chosen clip, see placeOf. Paused, the
  // frame that holds the moment is drawn, exactly. Playing, the play goes
  // on from there, the clip's or the episode's by where it landed.
  //
  // The moment and the program go to the queue in one call. In two, a click
  // across the clip's edge while playing was lost: the new program started
  // the play again from where it was, the queue said so at once, that set
  // the playhead back, and the seek after it went to the playhead.
  export function seek(t: number, about?: About) {
    const at = Math.max(0, Math.min(t, source.duration));
    time = at;
    placed = placeOf(pieces, clip?.key ?? "", at, 1 / oneFrame, about, source.videoStart ?? 0);
    if (!queue) return;
    if (!paused) playedClip = placed.place !== "video";
    queue.seek(at, programOf(placeFor(placed, clip?.key)));
    sought = queue;
  }

  // A press on the clip timeline or the range picker while it plays holds
  // the play under the hand, like a nail, for as long as the button is
  // down: the playhead goes to the press and stays there, follows the hand
  // once it moves, and the play goes on from where the hand lets go. The
  // play stays a play the whole time, the button says Pause, and only the
  // sound is held, the way a pause holds it. Before, the play went on from
  // the press at once and ran away from the hand before a drag could start.
  // Found by Tim while testing 2.156, plan row 2.158.
  let nailed = false;

  export function hold(held: boolean) {
    if (!queue) return;
    if (held) {
      if (paused || nailed) return;
      nailed = true;
      queue.pause();
      return;
    }
    if (!nailed) return;
    nailed = false;
    // The space bar may have paused it under the hand.
    if (!paused) queue.play();
  }

  export function toggle() {
    if (!queue || failed) return;
    nailed = false;
    if (paused) play();
    else {
      queue.pause();
      paused = true;
    }
  }

  // Whether the play under way is the chosen clip's, so its end leaves the
  // playhead on the clip, at its end.
  let playedClip = false;

  // Called from the space bar, the play button or a click on the picture,
  // so the sound card starts inside the gesture that asked for it.
  function play() {
    if (!queue) return;
    endCorrection();
    // Playing moves the playhead on, so the keyboard's word goes.
    keyed = null;
    walked = 0;
    // On the clip, the clip plays from the playhead, from the end of a cut
    // the playhead is in, and from its start where the playhead is at its
    // end. On the video, the episode plays straight on from the playhead,
    // clip or no clip, so any part of it can be heard while a clip is
    // chosen. See playFrom.
    const from = playFrom(place, pieces, time);
    playedClip = !!clip && from.clip;
    if (clip && from.clip) {
      placed = { place: "clip", clip: clip.key };
      queue.setProgram(pieces, looping);
      // Only where the play does not start at the playhead, from the
      // clip's start or the end of a cut. Anywhere else the play is cued
      // already, its first frames and sound decoded.
      if (from.at !== time) {
        time = from.at;
        queue.seek(time);
      }
      onplayclip?.(clip);
    } else {
      queue.setProgram(null, false);
    }
    queue.play();
    paused = false;
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
      // On the video the clip's rules are not in play, and the video
      // preview shows the episode alone, the clip's moments included.
      inside: place !== "video" && insideClip(clip.segments, time, oneFrame),
      moved: clip.segments[piece]?.moved ?? false,
    };
  });

  // The word being corrected: where it stands in the caption, which word of
  // the episode it is, the whole of that word, and the piece of it drawn
  // here. The last two differ only for a word that was split in two.
  let fixing = $state<{
    at: number;
    said: number;
    part: number | null;
    whole: string;
    piece: string;
  } | null>(null);
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
  // decided twice. None on the video: the episode playing on through the
  // clip is the episode, not the short, and Tim saw its captions stay up.
  const caption = $derived.by(() => {
    if (!clip || !captions?.captions?.length || place === "video") return null;
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
  // What is read is the episode's own frame, the one the queue drew, not
  // the screen: the crop, its shade and the captions lie over the picture,
  // and a colour read through them would be the app's and not the
  // episode's.
  let sampling = $state<{ over: (hex: string | null) => void; done: (hex: string | null) => void } | null>(null);
  let loupe = $state<{ x: number; y: number; hex: string } | null>(null);
  let lens = $state<HTMLCanvasElement>();

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
    const source = queue?.picture();
    if (!lens || !screen || !source) return null;
    const w = source.displayWidth;
    const h = source.displayHeight;
    if (!w || !h) return null;
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
      if (event.key !== "Escape" || asleep(shell)) return;
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
  // at the one word they came from. A correction of several words also
  // says which of them each is, part, so each is a word of its own in the
  // caption box: text is that one word, the one corrected or removed
  // when this is, and the others stay as they are. A word typed in after
  // another, the way a removed word is put back, is one of these.
  type Heard = { start: number; whole: string; part: number | null; text: string };
  const rows = $derived.by(() =>
    (caption?.lines ?? []).map((line) =>
      line.words.map((word) => {
        if (!correctable || word.said === undefined) return { word, said: null };
        const whole = word.whole ?? word.text;
        const part = word.part ?? null;
        const text = part === null ? whole : (whole.split(" ")[part] ?? whole);
        return { word, said: { start: word.said, whole, part, text } as Heard };
      }),
    ),
  );

  // What a heard word reads as with one of its words changed, or taken
  // out when the text is empty.
  function withPart(said: Heard, text: string): string {
    if (said.part === null) return text;
    const words = said.whole.split(" ");
    words.splice(said.part, 1, ...(text ? [text] : []));
    return words.join(" ");
  }

  // A word is in the caption twice when the captions show it in halves,
  // and while one half is being corrected it holds the whole word, so the
  // other half is not drawn at all.
  function doubled(word: { start: number }, said: Heard | null): boolean {
    return (
      !!fixing &&
      !!said &&
      said.start === fixing.said &&
      said.part === fixing.part &&
      word.start !== fixing.at
    );
  }

  // A caption word says what it says by hand, not through the template.
  // While a word is being corrected the browser owns what is inside it,
  // and a template that wrote there would take the caret with it. This
  // writes whenever the captions come back from the engine, and never
  // into a word a hand is in. It wrote only when the word's own text
  // changed, so a word typed in after another, "weil" made "weil ein",
  // went on reading "weil ein" with "ein" drawn after it as well: the
  // first word is still "weil", so nothing told it to let go of what was
  // typed.
  function says(node: HTMLElement, [text]: [string, unknown]) {
    node.textContent = text;
    return {
      update([next]: [string, unknown]) {
        if (document.activeElement !== node && node.textContent !== next) node.textContent = next;
      },
    };
  }

  // Taking a word takes the picture with it: a caption that moved on under
  // the caret would leave the hand correcting a word that is no longer
  // there.
  function takeWord(
    node: HTMLElement,
    word: { start: number; text: string },
    said: Heard | null,
  ) {
    if (!said || !correctable) return;
    frozen = time;
    if (!paused) toggle();
    fixing = { at: word.start, said: said.start, part: said.part, whole: said.text, piece: word.text };
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

  // The words removed whose removal is still on its way to the engine. A
  // removed word leaves the caption box with the key that removed it, and
  // the engine's answer, the captions without it, takes over when it
  // lands. A word is the moment it was heard, and which of its words it
  // is when a correction made it several.
  const goneKey = (said: Heard) => (said.part === null ? `${said.start}` : `${said.start}:${said.part}`);
  let gone = $state<Set<string>>(new Set());
  $effect(() => {
    void captions;
    untrack(() => {
      if (gone.size) gone = new Set();
    });
  });

  // Removing one of the words a correction made removes that word and
  // keeps the others. It took the whole heard word, so removing a word
  // typed in after another took the other with it.
  async function removeWord(said: Heard) {
    // The frame goes with the word, since there is nothing left to frame.
    keyed = null;
    walked = 0;
    const key = goneKey(said);
    gone = new Set([...gone, key]);
    try {
      await onword?.(said.start, withPart(said, ""));
    } catch {
      // The workspace says what went wrong, and the word comes back.
      const back = new Set(gone);
      back.delete(key);
      gone = back;
    }
  }

  async function dropWord(node: HTMLElement, word: { start: number; text: string }, said: Heard | null) {
    const was = fixing;
    fixing = null;
    if (!was) return;
    // A word is one word on a line, whatever was typed into it: the
    // newlines a paste brings are spaces, and a run of spaces is one.
    const text = (node.textContent ?? "").replace(/\s+/g, " ").trim();
    // A word emptied is a word removed. The text goes back in first, so
    // the word is never drawn empty while it leaves.
    if (!text) {
      node.textContent = word.text;
      if (was.whole && said) removeWord(said);
      return;
    }
    if (text === was.whole) {
      node.textContent = word.text;
      return;
    }
    savingWord = was.at;
    try {
      await onword?.(was.said, said ? withPart(said, text) : text);
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
  //
  // A word in the keyboard's frame is the word Enter opens, wherever the
  // playhead is, because the frame is what says which word that is. A
  // word clicked and let go of with Enter or Escape keeps the frame and
  // leaves the playhead where it was, and Enter opened the word at the
  // playhead instead, a word with no frame on it. A walk found it.
  function openSpoken(): boolean {
    const words = [...(screen?.querySelectorAll<HTMLElement>(".word.correctable") ?? [])];
    let pick: HTMLElement | null = null;
    if (keyed) pick = words.find((node) => Number(node.dataset.at) === keyed!.start) ?? null;
    if (!pick) {
      for (const node of words) {
        if (Number(node.dataset.at) <= spoken + 1e-6) pick = node;
      }
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

  // A share of the height of the video preview, as the stylesheet works it
  // out: .screen is a container, and 100cqh is its height in the same pass
  // as the layout. It was the height a ResizeObserver read, which comes a
  // frame after the layout, so while the app was resized the captions were
  // a frame behind the picture under them, and shook.
  function px(share: number): string {
    return `calc(${share} * 100cqh)`;
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
    const scale = 1920 / Math.max(screen.clientHeight, 1);
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
    if (asleep(shell)) return;
    // Delete removes the keyboard's word, the one in the frame, unless a
    // field has the keyboard and the key is its own.
    if (removeKeyed(event)) return;
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
    // The box asking something takes the space bar for the button that
    // has the keyboard, and the arrows and Enter for its answer. The space
    // bar played the episode behind it and pressed nothing.
    if (keysElsewhere()) return;
    const on = document.activeElement as HTMLElement | null;
    const tag = on?.tagName;
    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      // The clip timeline moves the playhead. A slider holding the
      // keyboard, an edge of the clip, takes the arrows for itself.
      if (on?.getAttribute("role") === "slider") return;
      // Shift walks by words, and the word it lands on becomes the
      // keyboard's.
      if (event.shiftKey) walked = Date.now();
      return;
    }
    if (event.key === "Enter") {
      if (event.defaultPrevented || event.shiftKey || event.repeat) return;
      // A button or a link holding the keyboard is pressed by Enter.
      if (tag === "BUTTON" || tag === "A" || on?.getAttribute("role") === "option") return;
      if (openSpoken()) event.preventDefault();
      return;
    }
    if (event.code !== "Space" || event.shiftKey) return;
    event.preventDefault();
    toggle();
  }

  // On the Mac the key marked delete is Backspace to the browser, and the
  // forward delete key is Delete. Either removes the word in the frame.
  function removeKeyed(event: KeyboardEvent): boolean {
    if (event.key !== "Backspace" && event.key !== "Delete") return false;
    if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey || event.repeat) return false;
    if (!keyed || fixing) return false;
    if (keysElsewhere()) return false;
    const at = keyed.start;
    const found = rows.flat().find(({ word, said }) => !!said && word.start === at);
    if (!found?.said || gone.has(goneKey(found.said))) return false;
    event.preventDefault();
    removeWord(found.said);
    return true;
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
    window.addEventListener("keydown", onKey);
    window.addEventListener("pointerdown", forget, true);
    window.addEventListener("focusin", focusMoves);
    return () => {
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("pointerdown", forget, true);
      window.removeEventListener("focusin", focusMoves);
    };
  });
</script>

<!-- The video preview takes everything the workspace has left, in height
     and in width, and keeps the shape of the episode. What is under it, the
     range picker and the controls, is measured rather than guessed at, so
     nothing is left over and nothing has to be scrolled to. -->
<div class="player" bind:this={shell}>
  <!-- data-playhead says where the playhead is. Nothing on screen reads
       it: it is there so a probe can follow the playhead across a cut on
       every frame, the way the caption words say their moments. -->
  <!-- The viewer: as wide as the middle column and as tall as the picture
       may be, black, with the picture in the middle of it. The columns
       beside it take their width from the app's width alone, so on an app
       too short for the picture to fill the column the rest is black at
       its sides, the way a video player shows a picture of another shape.
       Everything drawn over the picture stands in .screen, the picture's
       own box, so the crop and the captions are in shares of the picture
       and not of the viewer. -->
  <div class="viewer">
  <div class="screen asks" bind:this={screen} data-playhead={time}>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <span class="ask corner" onpointerdown={(e) => e.stopPropagation()}>
      <Info label="What you can do with the picture" side="right">
        The space bar plays and pauses, and so does a click on the picture. With the playhead on
        the chosen clip it plays the clip, its cuts jumped, and anywhere else the video straight
        on, with nothing of the clip over it. Drag the crop frame
        sideways to place it, and the black box up or down for the captions. Click a word in the
        caption box to correct it, or walk to it with Shift and the arrows and press Enter: Enter
        saves it, Escape leaves it, and two words split it in two. Delete removes the word in the
        frame, and so does saving it empty.
      </Info>
    </span>
    <!-- The picture. The queue draws every frame onto it, the size the
         stylesheet makes it, in the episode's own shape. -->
    <canvas bind:this={canvas} use:watchSize onclick={toggle}></canvas>
    {#if failed}
      <p class="failed selectable">{failed}</p>
    {:else if trouble}
      <p class="trouble selectable">{trouble}</p>
    {/if}
    <!-- The crop is drawn only while the playhead stands in the clip and is
         on it. Anywhere else, and on the video playing on through the clip,
         the picture is the episode, not the short, so nothing is laid over
         it. -->
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
        style="left: {box.left}%; width: {box.width}%; --step: {px(captionYStep / 1920)}"
      ></div>
    {/if}
    <!-- Captions switched off in the captions column are not drawn, so the
         video preview shows the short as it will be rendered. -->
    {#if caption && captions && captions.style.text !== false}
      <div
        class="captions"
        style="left: {box.left}%; width: {box.width}%;
               bottom: max({px(captionY / 1920 - captions.style.padY)}, 0px);
               padding: 0 {px(captions.style.marginH)};
               font-family: '{captions.style.font}', system-ui, sans-serif;
               font-weight: {captions.style.bold ? 800 : 500};
               font-size: {px(captions.style.size)};
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
                 border-radius: {px(captions.style.radius)};
                 padding: {px(captions.style.padY)} {px(captions.style.padX)}"
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
              {#each line as { word, said }, i (`${i}:${word.start}`)}{#if !doubled(word, said) && !(said && gone.has(goneKey(said)))}{#if i > 0}{" "}{/if}<span
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
                      ? "Click to correct this word. Enter saves it, Escape leaves it, and delete removes it. Two words split it in two"
                      : null}
                    style="--pill: {captions.style.highlightColour}"
                    use:says={[word.text, captions]}
                    onfocusin={(e) => takeWord(e.currentTarget, word, said)}
                    onfocusout={(e) => dropWord(e.currentTarget, word, said)}
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
  </div>
  <div class="under">
    {@render strip?.()}
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
  /* A container for its size, so what is drawn over the picture is sized
     in shares of it by the stylesheet, see px. */
  .viewer {
    display: flex;
    height: var(--pic-h);
    background: #000;
    border-radius: var(--radius-m);
    overflow: hidden;
  }

  /* In the middle of the viewer, on a whole pixel: half of an odd
     difference is a half, and a picture between two pixels is soft. */
  .screen {
    position: relative;
    container-type: size;
    flex: none;
    margin-left: calc((var(--mid-w) - var(--pic-w)) / 2);
    height: var(--pic-h);
    width: var(--pic-w);
    background: #000;
    overflow: hidden;
  }

  @supports (width: round(down, 3px, 2px)) {
    .screen {
      margin-left: round(down, calc((var(--mid-w) - var(--pic-w)) / 2), 1px);
    }
  }

  canvas {
    position: absolute;
    inset: 0;
    display: block;
    width: 100%;
    height: 100%;
  }

  /* Why the episode cannot be shown, where the picture would be, and why
     it plays without sound, at the foot of the picture. */
  .failed,
  .trouble {
    position: absolute;
    left: var(--edge);
    right: var(--edge);
    z-index: 4;
    margin: 0;
    text-align: center;
    color: var(--muted);
  }

  .failed {
    top: 0;
    bottom: 0;
    display: grid;
    place-items: center;
  }

  .trouble {
    bottom: var(--edge);
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
