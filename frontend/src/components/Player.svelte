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
  // inside the crop the way the render will burn them in: by libass, from
  // the short's own caption file, laid over each frame before it is shown,
  // see lib/frames/captions.ts. On the video the video preview shows the
  // video and nothing else.
  import { onMount, untrack, type Snippet } from "svelte";
  import { said } from "../lib/said";
  import { insideClip, wordInEpisode, type Piece } from "../lib/flow";
  import { inEpisode, onVideo, placeFor, placeOf, playedToEnd, playFrom, type About, type Place, type Playhead } from "../lib/playhead";
  import { FrameQueue, type Shown } from "../lib/frames/queue";
  import { movieOf } from "../lib/frames/movie";
  import { CaptionLayers, quarters, type CaptionAsk, type Over, type Showing, type WordFrame } from "../lib/frames/captions";
  import { shownColour, type Light } from "../lib/frames/light";
  import Info from "./Info.svelte";
  import {
    captionYStep,
    snapCaptionY,
    type CaptionsView,
    type ClipEntry,
    type CropMark,
    type SourceView,
  } from "../lib/api";
  import { rgbToHex } from "../lib/colour";
  import { asleep, keysElsewhere } from "../lib/keys";
  import { pictureOf, pieceHolding, rotationAt, shownAt } from "../lib/rotate";
  import { glidingLeft } from "../lib/glide";

  let {
    path,
    source,
    clip,
    making,
    captions = null,
    captionsDue = false,
    wordFrames = null,
    drafts = {},
    time = $bindable(0),
    onplayclip,
    oncaptionmoved,
    oncrop,
    onresetcrop,
    oncropmark,
    oncaptiony,
    onword,
    onremoveword,
    locked = false,
    strip,
    paused = $bindable(true),
    looping = $bindable(false),
    onClip = $bindable(false),
    offers = $bindable({ crop: "", savingCrop: false, hint: "" } as PlayerOffers),
    opening = false,
    pictured = $bindable(false),
    frameStart = $bindable(NaN),
    sleeping = false,
  }: {
    path: string;
    source: SourceView;
    clip: ClipEntry | null;
    // The key of the clip on its way that is chosen, which the video
    // preview has nothing of yet but the place of the playhead, see claim.
    making?: string;
    // The captions of the selected clip, on the clip's own clock.
    captions?: CaptionsView | null;
    // Whether the captions of the selected clip are on their way, so a
    // frame of it waits for them, see readyAt.
    captionsDue?: boolean;
    // Where libass puts each word of the captions of a track, see
    // WordFrame, null while they are on their way or cannot be said.
    wordFrames?: { key: string; frames: WordFrame[][] | null } | null;
    // What the hand is changing of their colours and switches on the way,
    // as /frames/captions takes it, see cmd/framefairy-app/captions.go.
    // Only what is being changed: the rest is the captions' own.
    drafts?: Record<string, string>;
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
    // Puts a keyframe back at a moment with its crop, the way the clip
    // timeline puts one, for the marks a reset took away.
    oncropmark?: (from: number, to: number, x: number) => Promise<void>;
    // Puts the captions at a distance from the bottom of a 1080x1920
    // frame. There is one place for every clip of every episode, so this
    // saves a setting rather than editing the clip.
    oncaptiony?: (y: number) => Promise<void>;
    // Corrects one word of the episode, by the moment it was spoken. The
    // captions in the video preview are where words are corrected, so this
    // is the one hand that reaches out of the picture.
    onword?: (start: number, text: string) => Promise<void>;
    // Removes one word from this clip's captions alone, by the moment it
    // was spoken and which of the words a correction made of it it is, or
    // all of it with null. Every other clip still shows it.
    onremoveword?: (start: number, part: number | null) => Promise<void>;
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
    // Where the frame on the canvas begins, in the episode, as the queue
    // says it drew it. NaN before the first. The frame a mark is put on,
    // see shownMoment in Episode.svelte.
    frameStart?: number;
    // The workspace is kept for coming back to while another episode is
    // in front, see App.svelte. Asleep, the video preview holds nothing
    // but the frame on its canvas and what it plays: no decoder on the
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
  // to be one click too. What a reset threw away, the crop and the crop
  // marks of the angle, is kept until the clip changes or it is placed by
  // hand again.
  let undoCrop = $state<{
    key: string;
    piece: number;
    at: number;
    left: number | null;
    marks: CropMark[];
  } | null>(null);

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
  // the frame on the canvas stays until the new queue draws the same one.
  // The new queue counts on the frames the engine names now, from source,
  // never on the ones the last queue played: the file may have been
  // written over in between, and the Go side streams on what the page
  // counts on, see Stream in lib/frames/app.ts.
  //
  // Waking, the queue is opened once the workspace is on screen, in the
  // task after the frame that shows it: what is on the canvas is already
  // the frame it would draw, and starting a sound card and a decoder in
  // that frame held it back. Going to sleep, it is closed the same way.
  let slept = false;
  $effect(() => {
    const video = path;
    if (sleeping) return;
    let q: FrameQueue | null = null;
    const stop = slept ? afterPaint(() => (q = open(video))) : null;
    if (!stop) q = untrack(() => open(video));
    return () => {
      stop?.();
      if (!q) return;
      const going: FrameQueue = q;
      said(`video preview: closed, ${untrack(() => sleeping) ? "the workspace sleeps" : "the workspace went"}`, { video: path });
      slept = true;
      if (!untrack(() => paused)) going.pause();
      paused = true;
      nailed = false;
      if (queue === going) queue = null;
      held = null;
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

  function open(video: string): FrameQueue {
    failed = "";
    trouble = "";
    // What the engine says of the video: its frames, the short's, and its
    // sound. The video preview reads nothing of the file itself.
    const woken = slept;
    const q = new FrameQueue(canvas, video, movieOf(source), woken);
    q.trace = (line) => said(`video preview: ${line}`, { video: path });
    q.over = { at: overAt, coming: comingAt, ready: readyAt };
    queue = q;
    const opened = queueOpened;
    queueOpened = [];
    for (const done of opened) done();
    if (pixels[0]) q.resize(pixels[0], pixels[1]);
    q.listen((s) => {
      if (queue === q) heard(s);
    });
    // What the video preview had when it still showed no picture, for the
    // app's log, see lib/said.ts. Plan row 2.185.
    said(`video preview: opening${woken ? ", woken" : ""}, canvas ${canvas.width}x${canvas.height}, playhead ${time.toFixed(3)} s${untrack(() => opening) ? ", the workspace still opening" : ""}`, { video: path });
    for (const after of [5, 15])
      setTimeout(() => {
        if (queue !== q || pictured || sleeping) return;
        const frames = (window as unknown as { __appFrames?: { stats: unknown } }).__appFrames?.stats;
        said(
          `video preview: no picture ${after} s after it opened, playhead ${time.toFixed(3)}, ${paused ? "paused" : "playing"}, opening ${opening}, trouble "${trouble}", queue ${JSON.stringify(q.stats)}, movie ${JSON.stringify(q.movie && { duration: q.movie.duration, video: q.movie.video && { width: q.movie.video.width, height: q.movie.video.height, frame: q.movie.video.frame, samples: q.movie.video.samples.count } })}, frames ${JSON.stringify(frames)}`,
          { level: "warn", video: path },
        );
      }, after * 1000);
    q.ready.then(
      () => {
        if (queue === q) ahead(...askedAhead);
      },
      () => {},
    );
    q.ready.catch((e: Error) => {
      said(`video preview: could not be opened: ${e.message}`, { level: "error", video: path });
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
    frameStart = s.frame;
    if (s.passing) {
      if (s.drew) pictured = true;
      return;
    }
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

  // The captions run on the clip's clock, which has the cuts taken out: the
  // clock of the pieces they were made for. A clip that was changed has its
  // new pieces a moment before its new captions, and the captions read on
  // the new pieces' clock showed and lit another word for that moment.
  const captionPieces = $derived(captions?.pieces?.length ? captions.pieces : pieces);
  function inClipTime(t: number): number {
    let sum = 0;
    for (const p of captionPieces) {
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
  const place = $derived(placeFor(placed, clip?.key ?? making));
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
  //
  // A step from word to word bounces the word it lands on over the frame,
  // and any other way of getting there shows the frame alone, see
  // CaptionLayers.sought.
  export function seek(t: number, about?: About, wordStep = false) {
    const at = Math.max(0, Math.min(t, source.duration));
    holdOverlay();
    layers.sought(wordStep);
    time = at;
    placed = placeOf(pieces, clip?.key ?? "", at, 1 / oneFrame, about, source.videoStart ?? 0);
    if (!queue) return;
    if (!paused) playedClip = placed.place !== "video";
    queue.seek(at, programOf(placeFor(placed, clip?.key)));
    sought = queue;
  }

  // The moments the hand is likely to step to next, paused, whose frames
  // are read while it rests, see FrameQueue.ahead: the words either side
  // of the playhead. And the clips on the cards, the likeliest to be
  // clicked first: the frame each opens on is read once nothing else is
  // wanted, with the layer of its captions, and both are kept for a click
  // on its card, see FrameQueue.cards and CaptionLayers.next. How many
  // frames that is, the frames decide, see AppFrames.cards. Every layer is
  // kept, being only the box its words are drawn in, a few hundred
  // kilobytes. On a video of hours a card clicked for the first time took
  // half a second, issue #383.
  //
  // What was asked last is asked again of a queue as it opens, once it
  // knows the video: a workspace woken asks before its queue is there.
  let askedAhead: [number[], { clip: ClipEntry; captions: CaptionsView | null }[]] = [[], []];
  export function ahead(times: number[], clips: { clip: ClipEntry; captions: CaptionsView | null }[] = []) {
    askedAhead = [times, clips];
    if (!queue) return;
    queue.ahead(times);
    const starts: number[] = [];
    const wants: Parameters<typeof layers.next>[0] = [];
    // The layers of the frames a word step goes to, first: a step waits
    // for its frame's captions before it is put up, see FrameQueue.dress,
    // and each step lights another word, so its layer is a new one. Found
    // by the review of #385.
    const ask = captionAsk;
    for (const at of ask ? times : []) {
      const frame = queue.firstFrameAt(at);
      const f = frame !== null && framesClip(frame) ? framingOf(frame) : null;
      const size = f && layerSize(f);
      if (frame !== null && ask && size) wants.push({ ask, frame, size });
    }
    for (const { clip: of, captions: view } of clips) {
      const frame = queue.firstFrameAt(of.start);
      if (frame === null) break;
      const card = askOf(of, view);
      const f = card ? framingOf(frame, of) : null;
      const size = f && layerSize(f);
      starts.push(of.start);
      if (card && size) wants.push({ ask: card, frame, size });
    }
    queue.cards(starts);
    layers.next(wants);
  }

  // Settles once what was read ahead is in, the frames and the layers of
  // their captions, for the walks, which wait on it rather than on the
  // clock.
  export async function rested(): Promise<void> {
    // Woken, the queue opens in the task after the frame that shows the
    // workspace again.
    if (!queue && !sleeping) await new Promise<void>((done) => queueOpened.push(done));
    await Promise.all([queue?.rested().catch(() => {}), layers.rested()]);
  }
  let queueOpened: (() => void)[] = [];

  // A clip made with I or O is made around the playhead, so the playhead
  // is on it from the moment it is there: making it is a gesture about
  // it, and it is not dimmed. This moves nothing and changes no running
  // play, rule 6 of docs/VIDEO-PREVIEW.md. The play goes on as it began,
  // and the clip's pieces are what the next play plays. Before, the clip
  // made while the episode played was chosen with no gesture on it, so
  // it stood dimmed over the very moment it was made from. Found by Tim.
  //
  // A clip on its way has no pieces here yet, so the playhead is simply
  // on it. With from, only where the playhead is on the clip of that key:
  // a clip on its way that is written comes back under a key of its own,
  // and the playhead stays on it.
  export function claim(from?: string) {
    const key = clip?.key ?? making;
    if (!key || (from !== undefined && placed.clip !== from)) return;
    placed = clip
      ? placeOf(pieces, key, time, 1 / oneFrame, "clip", source.videoStart ?? 0)
      : { place: "clip", clip: key };
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
    queue.grip(held);
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
    if (!queue || failed) {
      said(`video preview: play or pause asked, and ${failed ? `the video could not be opened: ${failed}` : `there is nothing to play yet${sleeping ? ", the workspace sleeps" : ""}`}`, { level: "warn", video: path });
      return;
    }
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

  // The moment the frame on the canvas starts, which the picture is turned
  // for and its crop stands for, see lib/rotate.ts.
  const frameAt = $derived(shownAt(frameStart, time));
  // How far that frame is turned, by the clip's rotations: the picture is
  // drawn rotated, and the crop and the captions stand on it rotated. On
  // the video, the episode shows as filmed. Plan row 3.25.
  const turn = $derived(clip && place !== "video" ? rotationAt(clip.rotations, frameAt) : 0);
  // The piece that frame belongs to, or -1 for a frame of a cut or from
  // outside the clip.
  const framed = $derived(clip && place !== "video" ? pieceHolding(clip.segments, frameAt) : -1);
  // The picture the piece shows, rotated or not, and its crop.
  const picture = $derived(pictureOf(source, turn));
  // The piece whose crop stands on the picture: the one the frame shows,
  // or with a frame of a cut on the canvas the one nearest the playhead.
  const cropped = $derived(framed >= 0 ? framed : piece);

  // For the app's log, see docs/LOGGING.md: the pieces of the clip in
  // front each time they change, with how each is turned and whether its
  // crop was placed, and every time the picture turns, with the frame it
  // turned for and the playhead. A part that turned back halfway, or a
  // picture turned for the wrong frame, is then read in the log rather
  // than guessed at from a screenshot, #188.
  let saidPieces = "";
  $effect(() => {
    if (!clip) return;
    const pieces = clip.segments
      .map(
        (s) =>
          `${s.start.toFixed(3)}-${s.end.toFixed(3)}${s.rotate ? ` turned ${s.rotate}` : ""}${s.moved ? " placed" : ""}${s.marks?.length ? ` gliding over ${s.marks.map((m) => `${m.at.toFixed(3)}@${m.x}`).join(" ")}` : ""}`,
      )
      .join(", ");
    const turns = (clip.rotations ?? []).map((r) => `${r.from ?? "start"}-${r.to ?? "end"} ${r.deg}`).join(", ");
    const line = `video preview: clip ${clip.id} has the pieces ${pieces}${turns ? `, rotated ${turns}` : ""}`;
    if (line === saidPieces) return;
    saidPieces = line;
    said(line, { video: untrack(() => path) });
  });
  // The turn is said once it has held a moment, so a playhead dragged
  // back and forth over a cut says where it came to rest, not every frame.
  let saidTurn = 0;
  $effect(() => {
    const now = turn;
    if (now === saidTurn) return;
    const settled = setTimeout(() => {
      const was = saidTurn;
      saidTurn = now;
      said(
        `video preview: the picture turned from ${was} to ${now} for the frame at ${frameStart.toFixed(3)} s, piece ${framed} of clip ${clip?.id ?? "none"}, playhead ${time.toFixed(3)} s, ${paused ? "paused" : "playing"}`,
        { video: path },
      );
    }, 300);
    return () => clearTimeout(settled);
  });

  // The left edge of the crop on the frame on the canvas, in pixels of the
  // picture: gliding over the piece's keyframes frame by frame while the
  // clip plays, the way the render crops it, or standing still. Plan row
  // 3.26.
  const cropLeft = $derived.by(() => {
    if (!clip || cropped < 0 || !picture.width || !picture.cropWidth) return null;
    return leftOn(clip, cropped, frameAt, picture, place !== "video");
  });

  // The left edge of the crop of a piece on the frame that starts at a
  // moment, in pixels of the picture, gliding or standing still.
  function leftOn(
    c: ClipEntry,
    at: number,
    frame: number,
    pic: { width: number; cropWidth: number },
    glides: boolean,
  ): number {
    const gliding = glides
      ? glidingLeft(c.segments[at]?.marks, frame, source.fps, source.videoStart ?? 0, pic.cropWidth, pic.width)
      : null;
    return gliding ?? c.cropLefts[at] ?? (pic.width - pic.cropWidth) / 2;
  }

  // The crop's left edge as the video preview shows it now, which a crop
  // mark put on this frame keeps.
  export function cropLeftNow(): number | null {
    return cropLeft;
  }

  // Its crop, as shares of the video preview, with a drag applied.
  const crop = $derived.by(() => {
    if (!clip || cropped < 0 || !picture.width || !picture.cropWidth || cropLeft === null) return null;
    const left = dragLeft ?? cropLeft;
    return {
      left: (left / picture.width) * 100,
      width: (picture.cropWidth / picture.width) * 100,
      // On the video the clip's rules are not in play, and the video
      // preview shows the episode alone, the clip's moments included.
      inside: place !== "video" && insideClip(clip.segments, time, oneFrame),
      moved: !!clip.segments[cropped]?.moved || !!clip.segments[cropped]?.marks?.length,
    };
  });

  // The video preview shows one moment at a time: a frame, and the crop
  // frame, its shade and the captions that belong to that frame. A clip
  // picked moved the crop frame at once and took the captions off the
  // frame on screen, the picture came a few frames later, and its captions
  // a few after that. Tim saw the three come one after another and wanted
  // them at once, issue #383. So from a seek to another clip until that
  // clip's frame is put up, with its captions, see readyAt, everything
  // over the picture stays what it was, the clip before's, and the three
  // change together in the frame the queue puts the new frame up in, see
  // overAt. Shown is what was on screen in the last frame drawn: the crop
  // as shares of the picture, the turn, the crop's left edge for the
  // probes, the captions laid over the canvas, and whose clip.
  type Overlaid = { key: string; crop: typeof crop; turn: number; left: number | null };
  let held = $state.raw<(Overlaid & { over: Over | null; since: number }) | null>(null);
  let last: Overlaid = { key: "", crop: null, turn: 0, left: null };
  let drawnOver: Over | null = null;
  $effect.pre(() => {
    const now = { key: clip?.key ?? "", crop, turn, left: crop?.inside ? (dragLeft ?? cropLeft) : null };
    if (!held) last = now;
  });
  // A seek to another clip than the one on screen holds what is over the
  // picture. A seek before the frame of the last came keeps the hold it
  // has: what is on screen is still the clip before's.
  function holdOverlay() {
    if (held || !queue || last.key === (clip?.key ?? "")) return;
    held = { ...last, over: drawnOver, since: performance.now() };
  }
  const shownCrop = $derived(held ? held.crop : crop);
  const shownTurn = $derived(held ? held.turn : turn);

  // The word being corrected: where it stands in the caption, which word of
  // the episode it is, the whole of that word, and the piece of it drawn
  // here. The last two differ only for a word that was split in two.
  let fixing = $state<{
    at: number;
    said: number;
    part: number | null;
    whole: string;
    piece: string;
    pad?: number;
  } | null>(null);
  // The word a correction is on its way to disk for, so it says it is not
  // settled yet the way everything else in the app does.
  let savingWord = $state<number | null>(null);
  // The moment the caption box is showing while a word is being corrected.
  // Without it the caption would move on under the caret, which is the same
  // reason the lens freezes the row it magnifies.
  let frozen = $state(0);
  const shown = $derived(fixing ? frozen : time);

  // What the layer on screen shows, see Showing in lib/frames/captions.ts:
  // the caption and the word libass drew on the frame the canvas shows,
  // for the captions of a track. The caption box shows them, so it changes
  // caption and word on the frame the picture does, playing or paused. A
  // word being corrected keeps what was on screen when it was opened.
  // And how large libass draws the pill of that word on the frame, 1 at
  // rest, which the frame round the word grows by, so it stays on the
  // pill's edge through the pop, as it did when the browser drew both.
  let drawn = $state<(Showing & { track: string; scale: number }) | null>(null);
  let frozenDrawn = $state<(Showing & { track: string; scale: number }) | null>(null);
  function onScreen(track: string | null, showing: Showing | undefined, scale = 1) {
    const now = track && showing ? { track, ...showing, scale } : null;
    if (
      now?.track === drawn?.track &&
      now?.caption === drawn?.caption &&
      now?.word === drawn?.word &&
      now?.scale === drawn?.scale
    )
      return;
    drawn = now;
  }
  const showing = $derived.by(() => {
    const d = fixing ? frozenDrawn : drawn;
    return d && captions?.track === d.track ? d : null;
  });

  // The caption standing at the playhead, with the word being spoken. The
  // engine hands over the lines and the look, so nothing about captions is
  // decided twice. None on the video: the episode playing on through the
  // clip is the episode, not the short, and Tim saw its captions stay up.
  const caption = $derived.by(() => {
    if (!clip || !captions?.captions?.length || place === "video") return null;
    if (shown < clipStart - 0.05 || shown > clipEnd + 0.05) return null;
    if (showing) return captions.captions[showing.caption] ?? null;
    const at = inClipTime(shown);
    return captions.captions.find((c) => at >= c.start && at < c.end) ?? null;
  });

  const spoken = $derived(inClipTime(shown));

  // The word lit, counted across the caption's lines: the one the layer on
  // screen lights, and none until a layer of these captions has come. The
  // playhead's word lit here while the picture drew no captions yet, after
  // a clip was picked or an edit landed, which is a second say in what is
  // lit. firstOfRow is where each line's words begin in that count.
  const lit = $derived(!caption || !showing ? -1 : showing.word);
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
  // Its frame shows on the frame the picture lights the word on, see
  // showing: a walk moves the playhead at once, but the picture lights the
  // word only once the frame there and its captions are on screen, and the
  // frame went to the next word a frame or more before the pill did, which
  // Tim saw on #201.
  // A word is the keyboard's while the picture lights it: from and to, on
  // the episode's clock, see WordFrame, or by its own time on the clip's
  // clock where those are not known.
  let keyed = $state<{ start: number; end: number; from?: number; to?: number } | null>(null);
  // When Shift and an arrow were pressed. The next move of the playhead is
  // the walk landing, and its word becomes the keyboard's word.
  let walked = 0;
  // A frame's worth of give at the edges of the word, because the picture
  // answers a seek with the frame it shows, which can begin a hair before
  // the word does.
  const give = 0.05;
  $effect(() => {
    const at = shown;
    untrack(() => {
      if (walked && Date.now() - walked < 1000) {
        walked = 0;
        keyed = keyOf(at);
        return;
      }
      if (!keyed) return;
      const out =
        keyed.from !== undefined && keyed.to !== undefined
          ? at < keyed.from - give || at >= keyed.to
          : inClipTime(at) < keyed.start - give || inClipTime(at) >= keyed.end + give;
      if (out) keyed = null;
    });
  });

  // The word the picture lights at a moment of the episode, as the
  // keyboard's word, by when the engine says each word is lit, see
  // WordFrame, in whichever caption it is: the caption box may still hold
  // the caption before while the picture there is on its way. By the
  // word's own time where those are not known. Shift and an arrow walk to
  // where the picture lights a word, see lit in Episode.svelte, so the
  // keyboard's word is the word walked to.
  function keyOf(at: number): { start: number; end: number; from?: number; to?: number } | null {
    const frames = captions?.track && wordFrames?.key === `${path}\n${captions.track}` ? wordFrames.frames : null;
    if (frames) {
      for (const [i, cue] of (captions?.captions ?? []).entries()) {
        const words = (cue.lines ?? []).flatMap((line) => line.words);
        for (const [k, word] of words.entries()) {
          const f = frames[i]?.[k];
          if (f && f.from >= 0 && at >= f.from - give && at < f.to) return { start: word.start, end: word.end, from: f.from, to: f.to };
        }
      }
    }
    const word = wordAt(inClipTime(at));
    return word ? { start: word.start, end: word.end } : null;
  }

  // A word is corrected at the moment it was opened at, and a correction
  // ends when the playhead leaves that moment, however it is moved: the
  // word lets go of the caret the way a click elsewhere takes it, which
  // saves what was typed, and the captions follow the playhead again. A
  // click on a thumbnail's camera or a keyframe's diamond moves the
  // playhead without taking the focus, so the word stayed open and the
  // caption stayed on the old moment while the frame moved on, which Tim
  // saw on #201.
  $effect(() => {
    const at = time;
    untrack(() => {
      if (fixing && Math.abs(at - frozen) > give) endCorrection();
    });
  });

  // The keyboard's word put where a walk into the clip beside landed. The
  // walk cannot catch that landing itself: the clip changes first, with no
  // words yet, and the playhead only reaches the word once its captions
  // have arrived, which can be any time later. So the workspace says where
  // it put the playhead, and that word takes the frame.
  export function keyAt(t: number) {
    walked = 0;
    keyed = keyOf(t);
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
    const w = source.width;
    const h = source.height;
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
    const piece = source.imageData(fx - 4, fy - 4, 9, 9);
    ctx.putImageData(piece, 0, 0);
    const [red, green, blue] = piece.data.subarray(4 * 9 * 4 + 4 * 4);
    return { x: clientX - r.left, y: clientY - r.top, hex: rgbToHex(red, green, blue) };
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

  // How far the pill reaches past the letters of a word on either side, in
  // ems, so the box a word is typed in is its pill as long as it holds the
  // word and grows by exactly what is typed. libass makes the pill the
  // letters' ink and a margin, the browser's box is the letters' advance,
  // which is a little wider, so the margin is what is left of the pill
  // around the advance. Read once, as the word is opened, from the pill
  // and the letters both as the screen shows them, so the pop's scale
  // drops out. Nothing when the words are not laid out by libass.
  function sides(node: HTMLElement): number | undefined {
    if (!laidWords || !node.firstChild) return undefined;
    const letters = document.createRange();
    letters.selectNodeContents(node);
    const pill = node.getBoundingClientRect().width;
    const style = getComputedStyle(node);
    const size = parseFloat(style.fontSize) * (pill / parseFloat(style.width));
    if (!pill || !size) return undefined;
    return Math.max(0, (pill - letters.getBoundingClientRect().width) / 2 / size);
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
    frozenDrawn = drawn;
    if (!paused) toggle();
    fixing = { at: word.start, said: said.start, part: said.part, whole: said.text, piece: word.text, pad: sides(node) };
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
      await onremoveword?.(said.start, said.part);
    } catch {
      // The workspace says what went wrong, and the word comes back.
      const back = new Set(gone);
      back.delete(key);
      gone = back;
    }
  }

  async function dropWord(node: HTMLElement, word: { start: number; text: string }, said: Heard | null) {
    // However the word is left, it lets go of the caret. WebKit keeps the
    // selection in a field the keyboard has left and draws the caret
    // there, and the keys then typed into a word nobody had opened. Tim
    // found the caret blinking in a word after Report a problem took the
    // keyboard from it, when only Enter and Escape let go of it.
    const selected = window.getSelection();
    if (selected?.rangeCount && node.contains(selected.anchorNode)) selected.removeAllRanges();
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
  // the playhead is still on it, and gives up the caret, see dropWord.
  function letGo(node: HTMLElement) {
    keyed = { start: Number(node.dataset.at), end: Number(node.dataset.to) };
    node.blur();
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
  const box = $derived(shownCrop ?? { left: 0, width: 100 });

  // The captions in the picture are the short's: libass draws them on the
  // Go side from the caption file the render burns in, for each frame at
  // the moment the short shows it, and the screen lays them over the frame
  // before it gives it its look, see lib/frames/captions.ts. The caption
  // box above is what the hand takes hold of, and draws nothing of the
  // captions itself, except while a word is being corrected: what is typed
  // is not captions yet, so the caption box draws it, and the picture's
  // captions wait until it is saved.
  // Paused, the word a word step lands on bounces over the still frame,
  // the screen redrawing it on each of its own frames, see Lift in
  // lib/frames/captions.ts.
  const layers = new CaptionLayers(
    () => queue?.repaint(),
    { paused: () => paused },
    (text, video) => said(text, { level: "warn", video }),
  );
  // For the walks, which wait until the captions over the frame are drawn.
  (window as unknown as { __captionLayers?: CaptionLayers }).__captionLayers = layers;
  // While a word is typed, libass draws the caption without it and the
  // caption box draws only what is typed, where libass puts the word, see
  // typed. Before libass knows where the words stand, the caption box
  // draws the whole caption itself.
  const captionAsk = $derived.by((): CaptionAsk | null => {
    const ask = clip && askOf(clip, captions);
    if (!ask || !captions?.captions) return null;
    if (fixing && (!laidWords || typed < 0)) return null;
    if (dragCaptions !== null) ask.drafts.y = String(dragCaptions);
    if (fixing && caption) ask.typing = `${captions.captions.indexOf(caption)} ${typed}`;
    return ask;
  });
  // The captions of a clip as the Go side draws them over its frames, with
  // the drafts of the clip set, or null when it has none to draw.
  function askOf(of: ClipEntry, view: CaptionsView | null | undefined): CaptionAsk | null {
    if (!view?.track || !view.captions?.length || view.style.text === false) return null;
    return { path, clip: of.key, track: view.track, drafts: { ...drafts } };
  }
  // The word being typed, counted across the lines of its caption.
  const typed = $derived.by(() => {
    if (!fixing) return -1;
    for (const [row, line] of rows.entries()) {
      const i = line.findIndex(({ word }) => word.start === fixing!.at);
      if (i >= 0) return firstOfRow[row] + i;
    }
    return -1;
  });
  // The words of the caption shown where libass put them, each as a box in
  // the caption box, from the line the captions stand on, as shares of
  // the short: x and w of its width, top and h of its height, top down
  // from the line. Nothing while the words of this caption are not known
  // yet. While a word is typed, it is drawn here in its place, on the
  // baseline libass set it on, base, see captionAsk.
  type LaidWord = { x: number; w: number; top: number; h: number; r: number; base: number };
  const laidWords = $derived.by((): { words: LaidWord[]; x: number; w: number; top: number; h: number } | null => {
    if (!caption || !captions?.track || wordFrames?.key !== `${path}\n${captions.track}` || !wordFrames.frames)
      return null;
    const frames = wordFrames.frames[(captions.captions ?? []).indexOf(caption)];
    const count = (caption.lines ?? []).reduce((n, line) => n + line.words.length, 0);
    if (!frames || frames.length !== count || !count) return null;
    // The line the captions were measured standing on, which the caption
    // box stands on wherever it is dragged.
    const line = 1 - captions.style.marginV;
    const words = frames.map((f) => ({ x: f.x, w: f.w, top: f.y - line, h: f.h, r: f.r, base: f.base - line }));
    const x = Math.min(...words.map((f) => f.x));
    const top = Math.min(...words.map((f) => f.top));
    const w = Math.max(...words.map((f) => f.x + f.w)) - x;
    const h = Math.max(...words.map((f) => f.top + f.h)) - top;
    return { words, x, w, top, h };
  });

  // Where a word stands in the caption box laid out by libass, and the
  // corners of its pill as the engine drew them, see engine.pillRadius,
  // so the frame round the word has the pill's corners. Left and width
  // are the pill's at rest, which a word being typed grows from, and base
  // is how far down the pill its letters stand, which it is set on.
  function wordPlace(laid: { x: number; w: number; top: number }, f: LaidWord): string {
    return `--left: ${((f.x - laid.x) / laid.w) * 100}%; --width: ${(f.w / laid.w) * 100}%;
      --base: ${px(f.base - f.top)}; top: ${px(f.top - laid.top)}; height: ${px(f.h)};
      line-height: ${px(f.h)}; border-radius: ${px(f.r)};`;
  }

  // Everything the captions over a frame are worked out from, so a change
  // to any of them draws the frame on screen again.
  $effect(() => {
    layers.set(captionAsk);
    void [place, dragLeft, clip, source];
    untrack(() => queue?.repaint());
  });

  // The crop of the frame that starts at a moment: how far it is turned,
  // and where the crop stands and how wide it is, as shares of the picture
  // as shown. The same rule as the crop frame's, worked out for the frame
  // itself, since it is asked for as the frame is drawn, before the
  // workspace hears which frame that is.
  function framingOf(frame: number, of = clip): { turn: number; left: number; width: number } | null {
    if (!of) return null;
    const turn = rotationAt(of.rotations, frame);
    const held = pieceHolding(of.segments, frame);
    const at = held >= 0 ? held : of === clip ? piece : 0;
    const pic = pictureOf(source, turn);
    if (at < 0 || !pic.width || !pic.cropWidth) return null;
    const left = (of === clip ? dragLeft : null) ?? leftOn(of, at, frame, pic, true);
    return { turn, left: left / pic.width, width: pic.cropWidth / pic.width };
  }

  // The size the captions of a frame are drawn at: the crop as the screen
  // shows it, in device pixels.
  function layerSize(f: { turn: number; width: number }): [number, number] | null {
    const [cw, ch] = pixels;
    const [w, h] = quarters(f.turn) % 2 ? [ch, cw] : [cw, ch];
    const size: [number, number] = [Math.round(f.width * w), h];
    return size[0] >= 1 && size[1] >= 1 ? size : null;
  }

  // Whether the frame that starts at a moment is a frame of the short:
  // the playhead is on the clip, and the frame holds a moment of it.
  function framesClip(frame: number): boolean {
    return !!clip && place !== "video" && frame + oneFrame > clipStart + 1e-6 && frame < clipEnd - 1e-6;
  }

  // The light of the frame on screen. The colours the browser draws over
  // it, the box a word is typed in, are shown in its look, see shownColour.
  let shownLight = $state<Light>("");
  // The colours of the captions as the frame on screen shows them.
  const shownCaptions = $derived(
    captions
      ? {
          primary: shownColour(shownLight, captions.style.primary),
          pill: shownColour(shownLight, captions.style.highlightColour),
          box: shownColour(shownLight, captions.style.box),
        }
      : null,
  );

  // The captions over the frame the canvas draws, and what they show for
  // the caption box, see onScreen. Fresh is a frame put up for the newest
  // gesture, which ends a hold, and anything else the frame on screen drawn
  // again, which during a hold is drawn with what lay over it before.
  function overAt(frame: number, light: Light, fresh = false): Over | null {
    if (held) {
      if (!fresh) return held.over;
      said(`video preview: clip ${clip?.id ?? "none"} has its frame up at ${frame.toFixed(3)} s, ${Math.round(performance.now() - held.since)} ms after it was picked`, { video: path });
      held = null;
    }
    if (light !== shownLight) shownLight = light;
    const ask = captionAsk;
    const f = ask && framesClip(frame) ? framingOf(frame) : null;
    const size = f && layerSize(f);
    const layer = ask && size ? layers.at(frame, size) : null;
    onScreen(layer?.track ?? null, layer?.showing, layer?.scale);
    drawnOver = f && size && layer ? { layer, size, left: f.left, width: f.width, turn: f.turn } : null;
    return drawnOver;
  }

  // What lies over the frame that starts at a moment is here, or is on its
  // way and the promise says when: the captions of the clip, asked of the
  // Go side when the clip was picked, then the layer of the frame. Null
  // when nothing is to be laid over it.
  function readyAt(frame: number): Promise<void> | null {
    if (captionsDue && clip && place !== "video")
      return new Promise((done) => {
        dueWaiters.push(() => {
          const next = readyAt(frame);
          if (next) void next.then(done);
          else done();
        });
      });
    layers.set(captionAsk);
    const f = captionAsk && framesClip(frame) ? framingOf(frame) : null;
    const size = f && layerSize(f);
    return size ? layers.ready(frame, size) : null;
  }
  let dueWaiters: (() => void)[] = [];
  $effect(() => {
    if (captionsDue && clip) return;
    void captionAsk;
    const waiting = dueWaiters;
    dueWaiters = [];
    untrack(() => waiting.forEach((w) => w()));
  });

  function comingAt(frame: number) {
    if (!captionAsk || !framesClip(frame)) return;
    const f = framingOf(frame);
    const size = f && layerSize(f);
    if (size) layers.coming(frame, size);
  }

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

  // The moment the crop change applies to: the start of the frame on the
  // canvas, whose crop is the one dragged, when it is in the clip,
  // otherwise the start of the piece shown. Not the playhead, which a play
  // of the clip leaves at its end, past its last frame, where the canvas
  // still shows that frame.
  function cropMoment(): number {
    if (!clip) return time;
    if (pieceHolding(clip.segments, frameAt) >= 0) return frameAt;
    return clip.segments[piece].start;
  }

  function dragCrop(event: PointerEvent) {
    if (!clip || !oncrop || savingCrop) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startX = event.clientX;
    const start = cropLeft ?? (picture.width - picture.cropWidth) / 2;
    const scale = picture.width / screen.clientWidth;
    const most = picture.width - picture.cropWidth;
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
    const had = clip.cropLefts[was] ?? null;
    const marked = clip.cropMarks ?? [];
    savingCrop = true;
    try {
      await onresetcrop(at);
      const still = clip?.cropMarks ?? [];
      const marks = marked.filter((m) => !still.some((k) => k.at === m.at));
      if (had !== null || marks.length) undoCrop = { key: clip!.key, piece: was, at, left: had, marks };
    } finally {
      savingCrop = false;
    }
  }

  export async function cropBack() {
    const back = undoCrop;
    if (!back || !oncrop) return;
    savingCrop = true;
    try {
      if (back.left !== null) await oncrop(back.at, back.left);
      for (const m of back.marks) await oncropmark?.(-1, m.at, m.x);
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
       every frame, the way the caption words say their moments. data-crop
       is the crop's left edge in pixels of the picture, for the same, and
       data-shown where the frame on the picture begins, the frame a mark
       is put on. -->
  <!-- The viewer: as wide as the middle column and as tall as the picture
       may be, black, with the picture in the middle of it. The columns
       beside it take their width from the app's width alone, so on an app
       too short for the picture to fill the column the rest is black at
       its sides, the way a video player shows a picture of another shape.
       Everything drawn over the picture stands in .screen, the picture's
       own box, so the crop and the captions are in shares of the picture
       and not of the viewer. -->
  <div class="stack">
  <div class="viewer">
  <div
    class="screen asks"
    class:turned={shownTurn !== 0}
    class:sideways={shownTurn % 180 !== 0}
    bind:this={screen}
    data-playhead={time}
    data-shown={frameStart}
    data-turn={shownTurn}
    data-crop={held ? held.left : crop?.inside ? (dragLeft ?? cropLeft) : null}
    style="--turn: {-shownTurn}deg"
  >
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
    <!-- What the crop takes hold of, and the shade over the rest of the
         picture. The shade is the shadow of the inside of the frame, so
         it follows the frame's rounded corners and lies under its line,
         with nothing of the picture left bright between the two. The line
         itself is drawn in .atop, above the viewer's rounded corners. -->
    {#if shownCrop?.inside}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div
        class="crop"
        class:held={dragLeft !== null}
        style="left: {shownCrop.left}%; width: {shownCrop.width}%"
        title="Drag sideways to place the crop"
        onpointerdown={dragCrop}
      ></div>
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
        class:typing={!!fixing}
        class:highlighted={!!captions.style.highlight}
        class:laid={!!laidWords}
        style:--pop={showing?.scale ?? 1}
        style="{laidWords
          ? `left: calc(${box.left}% + ${laidWords.x * box.width}%); width: ${laidWords.w * box.width}%;
             bottom: ${px(captionY / 1920 - laidWords.top - laidWords.h)}; height: ${px(laidWords.h)};`
          : `left: ${box.left}%; width: ${box.width}%;
             bottom: max(${px(captionY / 1920 - captions.style.padY)}, 0px);
             padding: 0 ${px(captions.style.marginH)};`}
               --size: {px(captions.style.size)};
               font-family: '{captions.style.font}', system-ui, sans-serif;
               font-weight: {captions.style.weight ?? (captions.style.bold ? 700 : 400)};
               --thicken: {captions.style.thicken ?? 0};
               font-size: {px(captions.style.size)};
               line-height: {captions.style.lineHeight};
               --typed: {shownCaptions?.primary};
               color: {fixing && !laidWords ? shownCaptions?.primary : 'transparent'}"
      >
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="box"
          class:draggable={!!oncaptiony}
          class:waiting={savingWord !== null}
          class:holding={dragCaptions !== null}
          style="background: {captions.style.boxOn === false || !fixing || laidWords ? 'transparent' : shownCaptions?.box};
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
                   looks wrong but an app that stops. The moment is where
                   the word starts in the episode, see wordInEpisode: on
                   the clip's clock a cut before it moved it, and the pill
                   popped on every answer while the cut was dragged. -->
              <!-- Each word says which moment it stands for. Nothing on
                   screen reads it: it is there so a probe can, because
                   which word lights up is decided by comparing two clocks
                   and there is no other way to see the comparison. The
                   playhead landing a thousandth of a millisecond before a
                   word start, and so lighting nothing, was found with
                   these and could not have been found without them. -->
              {#each line as { word, said }, i (`${i}:${wordInEpisode(captionPieces, word.start)}`)}{#if !doubled(word, said) && !(said && gone.has(goneKey(said)))}{#if i > 0}{" "}{/if}<span
                    class="word"
                    class:correctable={!!said}
                    class:fixing={fixing?.at === word.start}
                    class:keyed={!!keyed && !fixing && !!said && keyed.start === word.start && !!showing && firstOfRow[row] + i === lit}
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
                    style="--pill: {shownCaptions?.pill};{fixing?.at === word.start && fixing.pad !== undefined
                      ? `--pad: ${fixing.pad}em;`
                      : ''}{laidWords?.words[firstOfRow[row] + i]
                      ? wordPlace(laidWords, laidWords.words[firstOfRow[row] + i])
                      : ''}"
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
  <!-- The crop frame's line, over the viewer rather than in it. The
       viewer's rounded corners clip the picture and everything on it, and
       the frame is not on the picture: it says what of the picture is
       chosen, so it is drawn whole wherever the crop stands, at the
       picture's edge too. A box of the picture's own size and place, in
       the same cell as the viewer, takes no clicks: the crop under it is
       what the hand moves. -->
  <div class="atop">
    <div class="lid" class:sideways={shownTurn % 180 !== 0}>
      {#if shownCrop?.inside}
        <div class="frame" class:lit={dragLeft !== null} style="left: {shownCrop.left}%; width: {shownCrop.width}%"></div>
      {/if}
    </div>
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
  /* The viewer and the crop frame's line over it, in one cell, so the
     two are laid out by the same rules and stand in the same place. */
  .stack {
    display: grid;
  }

  .viewer,
  .atop {
    grid-area: 1 / 1;
    display: flex;
    height: var(--pic-h);
  }

  .viewer {
    background: #000;
    border-radius: var(--radius-m);
    overflow: hidden;
  }

  /* Over the crop and under the captions, the way the frame always lay:
     the same layer as the crop, and after it. */
  .atop {
    z-index: 2;
    pointer-events: none;
  }

  /* In the middle of the viewer, on a whole pixel: half of an odd
     difference is a half, and a picture between two pixels is soft. The
     lid is the same box over the viewer, for the crop frame's line. */
  .screen,
  .lid {
    position: relative;
    container-type: size;
    flex: none;
    margin-left: calc((var(--mid-w) - var(--pic-w)) / 2);
    height: var(--pic-h);
    width: var(--pic-w);
  }

  .screen {
    background: #000;
    overflow: hidden;
  }

  @supports (width: round(down, 3px, 2px)) {
    .screen,
    .lid {
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

  .turned canvas {
    transform: rotate(var(--turn));
  }

  /* A piece rotated a quarter turn shows the picture the other way up: as
     big as it can be in the viewer, the viewer's own shape kept, the way a
     video player shows a picture of another shape. Its box is the
     picture's box turned and made smaller by the shorter side over the
     longer, so it still fits, and in the middle of the viewer on a whole
     pixel. Everything over the picture stands in it, the crop and the
     captions, rotated with it. The canvas is the episode's frame lying as
     the file has it, as tall as the box is wide, turned into the box.

     The box is rounded first, never larger than the viewer, and centred
     from its rounded size. Centred from the size before rounding, a
     picture whose turned box filled the viewer's height to a hair over
     got a margin a hair below nought, rounded down to a whole pixel
     above the viewer, issue #264. A hundredth of a pixel is added before
     rounding down, as for the picture's own size in Episode.svelte. */
  .screen.sideways,
  .lid.sideways {
    --fit: min(var(--ar), calc(1 / var(--ar)));
    --side-w: min(var(--pic-w), round(down, calc(var(--pic-h) * var(--fit) + 0.01px), 1px));
    --side-h: min(var(--pic-h), round(down, calc(var(--pic-w) * var(--fit) + 0.01px), 1px));
    width: var(--side-w);
    height: var(--side-h);
    margin-left: round(down, calc((var(--mid-w) - var(--side-w)) / 2), 1px);
    margin-top: round(down, calc((var(--pic-h) - var(--side-h)) / 2), 1px);
  }

  .sideways canvas {
    inset: auto;
    left: round(down, calc((100cqw - 100cqh) / 2), 1px);
    top: round(down, calc((100cqh - 100cqw) / 2), 1px);
    width: 100cqh;
    height: 100cqw;
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

  /* The crop, as the hand takes hold of it: the frame's box, with nothing
     drawn but the shade around it. */
  .crop {
    position: absolute;
    top: 0;
    bottom: 0;
    z-index: 2;
    cursor: grab;
    touch-action: none;
  }

  .crop.held {
    cursor: grabbing;
  }

  /* The shade over the picture beside the crop is the shadow of the
     inside of the frame: the box inside its line, with the corners of the
     inside of its line, so it reaches into the frame's rounded corners
     and its edge lies under the line, where nothing shows between them.
     The screen clips it to the picture. */
  .crop::before {
    content: "";
    position: absolute;
    inset: var(--frame-line);
    border-radius: max(0px, calc(var(--frame-radius) - var(--frame-line)));
    box-shadow: 0 0 0 100vmax var(--backdrop);
    pointer-events: none;
  }

  /* The crop frame's line. Its line and corners are the frame in app.css,
     the same as the window on the range picker and the clip on the clip
     timeline. */
  .frame {
    position: absolute;
    top: 0;
    bottom: 0;
  }

  /* The caption box, where the render burns the captions into the crop.
     The captions themselves are in the picture, drawn by libass, so the box
     is see-through and draws only what is the interface's: the frame round
     a word and the handle. While a word is being corrected it draws the
     words itself, see typing. */
  .captions {
    position: absolute;
    z-index: 3;
    display: flex;
    justify-content: center;
    pointer-events: none;
    /* The letters are as heavy as libass draws them: the face at its own
       weight, never a bold the browser fakes, grown by the share of the em
       libass grows them by, see engine.Thickening. A stroke round each
       letter in its own colour grows it half the width on either side and
       leaves the advances alone, as libass does. The Mac faked the bold of
       Archivo Black its own way, and Tim saw the word typed lighter and
       narrower than libass draws it. */
    font-synthesis: none;
    -webkit-text-stroke-width: calc(var(--size) * var(--thicken, 0));
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
    /* The frame stands across the pill's edge, a pixel of the screen out
       and one in. libass draws the edge where it falls, between two
       pixels as often as not, and the row it half covers comes out half
       dark, while the browser puts a frame on whole pixels. As an outline
       round the outside of the box, that row showed between the frame
       and the pill or just inside the frame, and Tim saw the pill a pixel
       short at the top or the bottom. Across the edge, the frame covers
       that row wherever it falls. An outline cannot be moved by half a
       point, its offset is rounded, so the frame is a shadow. */
    --frame: transparent;
    --frame-width: var(--hair);
    box-shadow:
      0 0 0 var(--frame-width) var(--frame),
      inset 0 0 0 var(--frame-width) var(--frame);
  }

  .word.correctable:hover,
  .word.keyed {
    --frame: var(--accent-hi);
  }

  /* With the highlight off the word being corrected wears the hover
     frame twice over, which is the whole difference between the two.
     After Enter the word goes back to the frame of the two it had before
     it was opened: the one under the pointer, or the keyboard's, which
     stays on it while the playhead does. */
  .word.fixing {
    --frame-width: calc(2 * var(--hair));
    --frame: var(--accent-hi);
  }

  /* With the highlight on, the word being corrected is a pill in the
     highlight colour, whether or not it is the word lit, and wears no
     frame. Tim asked for one colour behind the letters typed: a word not
     lit was typed on the dark box of the caption, which read as a black
     field, and the frame round a lit word, in the app's lighter purple,
     read as a second shade of the pill. The caret says the word is open. */
  .highlighted .word.fixing {
    --frame: transparent;
  }

  /* A word in the caption box wears no focus ring. The frame says which
     word the keyboard is on and the caret that it is open, and the ring
     every other control wears, app.css, stood a pixel off the pill, so
     Tim saw a dark line round the word typed, the caption's box showing
     through between the two. */
  .word:focus-visible {
    outline: none;
  }

  /* The words where libass put them, see laidWords: the caption box is
     the box round all of them, and each word is placed in it on the pill
     libass draws behind it at rest, so the frame round a word is the
     pill's edge and the word stands in its middle. The spaces between the
     words take no room, since the words are placed and not laid out. */
  .captions.laid {
    display: block;
  }

  .laid .box {
    width: 100%;
    height: 100%;
    font-size: 0;
  }

  .laid .line {
    display: contents;
  }

  .laid .line span {
    position: absolute;
    left: var(--left);
    width: var(--width);
    margin: 0;
    padding: 0;
    font-size: var(--size);
  }

  /* The word being spoken is framed on its pill, which libass grows round
     its middle in the pop, so the frame grows with it, by the scale the
     layer on screen says, see onScreen. At rest it is 1. */
  .laid .line span.now {
    transform: scale(var(--pop, 1));
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
  .typing:not(.laid) .line span.now {
    background: var(--pill);
    animation: pop 0.22s ease-out;
  }

  @media (prefers-reduced-motion: reduce) {
    .typing:not(.laid) .line span.now {
      animation: none;
    }
  }

  /* Libass draws the caption without the word being typed, and the word
     is drawn here, in its place, in the caption's colour, on its pill
     while it is the word lit. The browser drew the whole caption while a
     word was typed, larger than libass draws it, so the caption grew as a
     word was opened. The box is the pill as long as it holds the word,
     and grows round the pill's middle by what is typed, see sides: with
     the pill drawn by libass, a word typed longer ran out of it. On a
     frame of the pop it keeps the frame's scale, so opening the word
     moves nothing. */
  .laid .line span.fixing {
    left: calc(var(--left) + var(--width) / 2);
    width: max-content;
    min-width: var(--width);
    padding: 0 var(--pad, 0.14em);
    translate: -50% 0;
    color: var(--typed);
    /* Nothing of the word is drawn outside its pill. A selection is as
       tall as the face's line, which reaches past the pill, and Tim saw
       its blue stand over the pill in square corners. */
    overflow: clip;
  }

  .highlighted.laid .line span.fixing {
    background: var(--pill);
  }

  /* The letters typed stand on the baseline libass set the word on. In
     the middle of the pill's height, where the browser puts a line, they
     stood a few pixels lower than libass draws them, and Tim saw a word
     drop as he opened it. The box is trimmed to the top of a capital and
     the baseline, and its top padding puts the baseline where the engine
     measured it, see engine.WordFrame. */
  @supports (text-box: trim-both cap alphabetic) {
    .laid .line span.fixing {
      padding-top: calc(var(--base) - 1cap);
      text-box: trim-both cap alphabetic;
    }
  }

  /* The caret is the colour of the text it stands in, as in a text field
     on the Mac, so it shows on any pill the text itself shows on. In the
     accent it was purple on purple. */
  .word.correctable.fixing {
    caret-color: var(--typed);
  }

  /* Selected letters wear the Mac's own selection colour, Highlight, which
     follows the colour picked in the system settings. WebKit thins an
     opaque selection colour with white, and over the pill that came out
     the pill's colour again, so a selection could not be seen. A colour
     that is not quite opaque is left as it is, hence the 99 percent. The
     first line is for a webview without color-mix. */
  .word.correctable::selection {
    background: rgba(63, 99, 139, 0.99);
    background: color-mix(in srgb, Highlight 99%, transparent);
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
