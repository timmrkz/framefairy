<script lang="ts" module>
  // What the row under the clip timeline says about the chosen clip.
  export type ClipNumbers = {
    start: number;
    end: number;
    seconds: number;
    pieces: number;
    saving: boolean;
  };
</script>

<script lang="ts">
  // The episode up close, always: the waveform and where the playhead
  // stands. With a clip selected it is that clip, with its pieces and the
  // cuts between them, and either edge can be dragged to trim. Edges land
  // on the frame, and with shift on words the way the render cuts them. Two fingers move along the episode
  // and pinch to zoom, the way an editing timeline does.
  import { onMount } from "svelte";
  import {
    api,
    clock,
    intoWord,
    wordStep,
    type Gesture,
    type ClipEntry,
    type Word,
    type CaptionCue,
  } from "../lib/api";
  import {
    draftCaptions,
    heardIn,
    inClip,
    inEpisode,
    insideClip,
    litWord,
    type CaptionDraft,
    type Parts,
  } from "../lib/flow";
  import { scrub as scrubPlayhead } from "../lib/scrub";
  import { hoverClip } from "../lib/hover";
  import Info from "./Info.svelte";
  import Icon from "./Icon.svelte";

  let {
    path,
    clip,
    duration,
    heard = [[0, duration]],
    measured = 0,
    measuredParts = [],
    time,
    locked = false,
    frame = 1 / 30,
    lit = [],
    onseek,
    dimmed = false,
    onreshape,
    onwalkclip,
    captions = [],
    arriving = false,
    captionLook = null,
    oncaptiontime,
    oncaptiondraft,
    onshape,
    thumbnails = [],
    onthumbnail,
    marks = [],
    hovered = "",
    onhover,
    onmark,
    centre,
    numbers = $bindable({ start: 0, end: 0, seconds: 0, pieces: 0, saving: false }),
  }: {
    path: string;
    // Without a clip the timeline follows the playhead through the episode.
    clip: ClipEntry | null;
    duration: number;
    // What the transcript has heard, in parts. The words arrive with it,
    // so the view is taken again as more of it is heard.
    heard?: Parts;
    // How much of the loudness is measured, which is the waveform, and
    // which parts, from and to. It is measured on its own from the moment
    // the episode is added, ahead of the transcript and where this
    // timeline looks first, so a view with a gap in it is taken again as
    // it grows.
    measured?: number;
    measuredParts?: [number, number][];
    time: number;
    locked?: boolean;
    // One frame of the episode, which is what an arrow key is worth.
    frame?: number;
    // The words the caption lights up, on the episode's clock. Shift and
    // an arrow key step through these, because these are the words anyone
    // can see. They are not the words of the transcript: a correction that
    // reads as two words is two here and one there, so a word added by
    // hand is in this list and in no other, and a word a cut takes out is
    // in the transcript and never in this one.
    lit?: Word[];
    // Puts the playhead at a moment. A gesture about the clip itself, an
    // edge, a trim or a caption, says so, and the playhead is then on the
    // clip whatever the moment, see placeOf in lib/playhead.ts.
    onseek: (t: number, about?: "clip") => void;
    // Whether the playhead is on the video rather than on the clip, so the
    // clip's frame is drawn dimmed: its rules are not in play.
    dimmed?: boolean;
    // A gesture let go of: an edge of the clip trimmed, a part taken out,
    // the edges of a cut moved, a cut put back. The engine makes the change
    // it showed while the hand moved, see engine/shape.go.
    // It comes with where the playhead stood when the hand took hold and
    // where the gesture left it, so an undo puts both back.
    onreshape?: (g: Gesture, playhead: [number, number]) => Promise<void>;
    // Walking the words has run off the end of the clip. The words of the
    // clip beside it are not here to walk on to, they arrive with its
    // captions, so the workspace is asked and it takes it from there.
    onwalkclip?: (back: boolean) => void;
    // The clip's captions, on the clip's clock, as the render shows them.
    // They are drawn along the foot of the track, and either edge of one
    // can be dragged where the words are a little off from what is heard.
    captions?: CaptionCue[];
    // The clip is on its way, being made: its caption blocks come in one
    // after another as they arrive, so the clip is built in front of the
    // person rather than appearing whole.
    arriving?: boolean;
    // The colours the captions are burned in, as the video preview draws
    // them: the words, the box behind them and the pill behind the word
    // being spoken. A block wears all three, so a colour picked for the
    // short is seen here too.
    captionLook?: { text: string; box: string; highlight: string } | null;
    // A caption edge let go of: the word it begins or ends on and the new
    // moment, both in the episode, or a moment below nought to put it back
    // where its words put it. True when it was saved.
    oncaptiontime?: (word: number, edge: "start" | "end", at: number) => Promise<boolean>;
    // A caption edge while it is being dragged, on the clip's clock, so
    // the caption box in the video preview can follow it. Null once the
    // captions have come back with it saved.
    oncaptiondraft?: (draft: CaptionDraft | null) => void;
    // The clip as a drag of an edge or a cut has it, its pieces and the
    // captions the engine made for them, or null when no drag is shaping
    // it. The video preview shows the same clip as the timeline on the way.
    onshape?: (shape: { cues: CaptionCue[]; pieces: { start: number; end: number }[] } | null) => void;
    // The clip's thumbnails, the moments of the episode the render takes a
    // picture of the short at. Each is a mark along the foot of the track.
    thumbnails?: number[];
    // A thumbnail dragged to another frame, from where it was to where it
    // was let go, both in the episode.
    onthumbnail?: (from: number, to: number) => Promise<void>;
    // Every clip of the episode, the way the range picker has them, so
    // the timeline zoomed out shows where the others are. The chosen one
    // is the frame and has no mark.
    marks?: {
      key: string;
      start: number;
      end: number;
      rendered: boolean;
      // A clip on its way, where its card says it lies, and the key that
      // chooses its card. It keeps its mark when it is written.
      arriving?: boolean;
      pick?: string;
    }[];
    // A mark clicked, which chooses that clip, as on the range picker.
    onmark?: (key: string) => void;
    // Where across the app a fitted clip stands, the middle of the video
    // preview and the range picker. The track runs under all three columns
    // and the clip list is wider than the settings, so the middle of the
    // track is not the middle of the picture above it.
    centre?: () => number | undefined;
    // The clip under the hand, here, on the range picker or in the clip
    // list. Its mark is lit the way it is under the pointer, and when it is
    // the clip up close, its frame is.
    hovered?: string;
    onhover?: (key: string, on: boolean) => void;
    // What the clip is, for the row under the timeline: its edges as they
    // are dragged, how long it comes out and in how many pieces, and
    // whether an edit is still on its way to disk.
    numbers?: ClipNumbers;
  } = $props();

  // The numbers travel out of here rather than standing over the
  // waveform, so the range picker and the waveform have nothing between
  // them.
  $effect(() => {
    numbers = {
      start,
      end,
      seconds: Math.round(drawnPieces.reduce((sum, p) => sum + p.end - p.start, 0)),
      pieces: drawnPieces.length,
      saving,
    };
  });

  // How much of the episode the timeline shows when no clip is selected.
  const loose = 60;

  let track: HTMLDivElement;
  let canvas: HTMLCanvasElement;
  let width = $state(0);
  let view = $state({ from: 0, to: 1 });
  let words = $state<Word[]>([]);
  let peaks = $state<number[]>([]);
  let viewFor = "";
  let loaded = false;
  // How much of the view the transcript had heard when it was read, how
  // much of the loudness was measured, and whether all of what was read.
  let loadedHeard = $state(-1);
  let loadedMeasured = $state(0);
  let loadedWhole = $state(false);
  // The part the words and the waveform were read for. It is wider than
  // the view, so a swipe has somewhere to go before anything is read again.
  let data = $state({ from: 0, to: 1 });
  // True while the view is where a hand put it. Until it is let go of, the
  // clip and the playhead stop pulling it back.
  let held = $state(false);
  // The closest the timeline goes, in seconds.
  const nearest = 2;
  // How much of the episode is asked for in words at a time.
  const spoken = 600;

  const segments = $derived(clip?.segments ?? []);
  const first = $derived(segments.length ? segments[0].start : 0);
  const last = $derived(segments.length ? segments[segments.length - 1].end : 0);
  const span = $derived(Math.max(view.to - view.from, 0.001));

  function x(t: number): number {
    return ((t - view.from) / span) * 100;
  }

  function timeAt(clientX: number): number {
    const box = track.getBoundingClientRect();
    return view.from + ((clientX - box.left) / box.width) * span;
  }

  // Dragging the playhead, the same way as on the range picker, see
  // lib/scrub.ts.
  let scrubbing = $state(false);

  // One seek a frame while an edge is dragged, for the playhead that goes
  // with it, the same pace the playhead itself is dragged at.
  let wanted = 0;
  let queued = 0;

  let wantedAbout: "clip" | undefined;

  function seekSoon(t: number, about?: "clip") {
    wanted = t;
    wantedAbout = about;
    if (queued) return;
    queued = requestAnimationFrame(() => {
      queued = 0;
      onseek(wanted, wantedAbout);
    });
  }

  function scrub(event: PointerEvent) {
    if (event.button !== 0) return;
    // Shift and a drag marks a part to take out instead of moving the
    // playhead. Everything else about the track is unchanged.
    if (event.shiftKey && editable && onreshape) {
      event.preventDefault();
      drawCut(event);
      return;
    }
    scrubPlayhead(event, timeAt, onseek, (held) => (scrubbing = held));
  }

  // A gesture on the clip, and what the engine makes of it. The hand says
  // where it is and the engine says where that lands, see engine/shape.go:
  // on a frame or on a word, how far an edge may go, where the playhead
  // stands, which pieces the clip is left with and what its captions say.
  // Nothing about that is decided here. The timeline draws the answer, so
  // what is drawn is what the same gesture saves when the hand lets go.
  //
  // One question at a time, and always about where the hand is now: a drag
  // moves faster than the answers come, and the ones in between are of
  // places the hand has already left. After the hand lets go the answer
  // stays until the saved clip and its captions come back, which are the
  // same, so nothing jumps on the way.
  let hand = $state<Gesture | null>(null);
  let saving = $state(false);
  let shaped = $state<null | {
    pieces: { start: number; end: number }[];
    cues: CaptionCue[];
    held: CaptionCue[];
  }>(null);
  let nextGesture: Gesture | null = null;
  let asking = false;
  let shapeFor = 0;
  // Where the playhead stood when the hand took hold, and where the
  // engine's answers have put it since. Kept here rather than read from
  // time when the hand lets go, because the last seek may not have
  // landed yet.
  let heldAt = 0;
  let leftAt = 0;

  function gesture(g: Omit<Gesture, "frame">): Gesture {
    return { ...g, frame };
  }

  function shape(g: Gesture) {
    if (!hand) heldAt = leftAt = time;
    hand = g;
    nextGesture = g;
    void ask();
  }

  async function ask() {
    if (asking || !nextGesture || !clip) return;
    const g = nextGesture;
    const asked = clip;
    const mine = shapeFor;
    const held = captions;
    nextGesture = null;
    asking = true;
    try {
      const answer = await api.shape(path, asked.plan, asked.id, g);
      if (mine === shapeFor && clip?.key === asked.key && answer) {
        shaped = { pieces: answer.pieces, cues: answer.captions?.captions ?? [], held };
        if (answer.playhead >= 0) {
          leftAt = answer.playhead;
          // The playhead goes with the edge being dragged, so it is on
          // the clip, even a hair outside the pieces drawn so far.
          seekSoon(answer.playhead, "clip");
        }
      }
    } catch {
      // A gesture the engine says no to shows the last one it said yes to.
      // Letting go says what is wrong.
    } finally {
      asking = false;
      void ask();
    }
  }

  // The hand let go. The same gesture is saved, and a gesture that never
  // moved anything is let go of with nothing saved.
  async function letGo(save: boolean) {
    const g = hand;
    hand = null;
    nextGesture = null;
    if (!save || !g || !onreshape) {
      shapeFor++;
      shaped = null;
      return;
    }
    await reshape(g, [heldAt, leftAt]);
  }

  // A click on a cut's button moves no playhead, so it stays where it is.
  async function reshape(g: Gesture, playhead: [number, number] = [time, time]) {
    if (!onreshape) return;
    saving = true;
    try {
      await onreshape(g, playhead);
    } finally {
      saving = false;
    }
  }

  const reshaping = $derived(!!hand || saving);
  const samePieces = (a: { start: number; end: number }[], b: { start: number; end: number }[]) =>
    a.length === b.length &&
    a.every((p, i) => Math.abs(p.start - b[i].start) < 0.002 && Math.abs(p.end - b[i].end) < 0.002);
  $effect(() => {
    if (shaped && !reshaping && (captions !== shaped.held || !samePieces(segments, shaped.pieces))) {
      shaped = null;
    }
  });
  // Another clip chosen is another clip's gesture. The key and not the
  // clip, because an edit hands back the same clip as a new object, and
  // that must not let go of the answer while its captions are on their way.
  const clipKey = $derived(clip?.key);
  $effect(() => {
    void clipKey;
    shapeFor++;
    shaped = null;
    hand = null;
  });
  const shapedShown = $derived(!!shaped && (reshaping || captions === shaped.held));

  // The pieces as they are drawn: the engine's answer to the gesture in
  // hand, or the clip as it is saved. A cut being drawn takes its part out
  // of the pieces at once rather than being a block laid over them, so the
  // wash parts under the hand and the count under the timeline follows the
  // drag. What is happening is shown while it happens.
  const drawnPieces = $derived(
    shapedShown && shaped ? shaped.pieces : segments.map((s) => ({ start: s.start, end: s.end })),
  );
  const start = $derived(drawnPieces.length ? drawnPieces[0].start : 0);
  const end = $derived(drawnPieces.length ? drawnPieces[drawnPieces.length - 1].end : 0);
  const editable = $derived(!!clip && !locked && !saving);

  // The clip from its first piece to its last, cuts and all. It is one
  // clip however many holes are in it, and the rules above and below say
  // so by running the whole way.
  const wholeClip = $derived(drawnPieces.length ? { start, end } : null);

  // A cut is a part the clip leaves out, which is the gap between two
  // pieces. The engine counts them from the first, and so does the timeline,
  // because that is what a move names.
  const cuts = $derived(
    drawnPieces
      .slice(1)
      .map((p, i) => ({ index: i, from: drawnPieces[i].end, to: p.start }))
      .filter((c) => c.to > c.from),
  );
  // A cut the saved clip does not have yet is one being drawn.
  const savedCuts = $derived(
    segments.slice(1).map((p, i) => ({ from: segments[i].end, to: p.start })),
  );
  function drawing(c: { from: number; to: number }): boolean {
    return hand?.kind === "cut" && !savedCuts.some((s) => Math.abs(s.from - c.from) < 0.002);
  }

  // An edge of the clip is dragged to trim it. Clicking one without
  // dragging puts the playhead exactly on it, which is how you start a clip
  // over. It lands on the frame, and shift puts it on a word, read on every
  // move, so letting go of shift part way through goes back to frames.
  function grab(edge: "start" | "end", event: PointerEvent) {
    if (!editable || !onreshape) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const from = event.clientX;
    let moved = false;
    const move = (e: PointerEvent) => {
      if (!moved && Math.abs(e.clientX - from) > 2) moved = true;
      if (!moved) return;
      shape(gesture({ kind: "trim", edge, index: 0, from: timeAt(e.clientX), to: 0, toWords: e.shiftKey }));
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      if (!moved) {
        onseek(edge === "start" ? first : last, "clip");
        return;
      }
      undone = null;
      await letGo(true);
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // An edge of a cut is dragged the way a clip edge is, and the other edge
  // stays where it was. Shift puts the edges on words, the same as on a
  // clip edge.
  function grabCut(index: number, side: "from" | "to", event: PointerEvent) {
    if (!editable || !onreshape) return;
    const was = cuts.find((c) => c.index === index);
    if (!was) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startX = event.clientX;
    let moved = false;
    const move = (e: PointerEvent) => {
      if (!moved && Math.abs(e.clientX - startX) > 2) moved = true;
      if (!moved) return;
      const t = timeAt(e.clientX);
      shape(
        gesture({
          kind: "move",
          edge: "",
          index,
          from: side === "from" ? t : was.from,
          to: side === "to" ? t : was.to,
          toWords: e.shiftKey,
        }),
      );
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      undone = null;
      await letGo(moved);
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // Drawing a cut is a drag across the clip with shift held, which is how
  // a part is marked in an editing timeline. Without shift the same drag
  // moves the playhead, so nothing that worked before works differently.
  // Shift already draws, so alt as well puts the edges on words.
  function drawCut(event: PointerEvent) {
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startX = event.clientX;
    const from = timeAt(startX);
    let moved = false;
    const move = (e: PointerEvent) => {
      if (!moved && Math.abs(e.clientX - startX) > 2) moved = true;
      if (!moved) return;
      shape(gesture({ kind: "cut", edge: "", index: 0, from, to: timeAt(e.clientX), toWords: e.altKey }));
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      undone = null;
      await letGo(moved);
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // How wide a cut taken out by a double-click starts, in pixels of the
  // track rather than in seconds.
  //
  // Seconds are the wrong measure for something whose whole job is to be
  // seen and taken hold of. A quarter of a second is wider than the whole
  // view with the timeline zoomed right in, so the cut arrives with both
  // its edges off screen and nothing to drag. On a four hour episode
  // zoomed right out it is a hair nobody can see, let alone grab. The
  // same number cannot be right at both ends of a range that wide, and
  // the thing that has to stay the same is what the hand sees.
  //
  // Forty, because the two edge handles are twelve wide each and centred
  // on the edges, so forty apart leaves clear block between them: both
  // edges can be told apart and either one grabbed without catching the
  // other.
  const cutAtOnce = 40;

  // Shift and a double-click takes a part out where you click, the way
  // shift and a drag takes out the part you drag across. Shift is the
  // cutting hand on this track either way. Where it goes exactly, and
  // whether there is room for it, is the engine's.
  async function cutHere(at: number, wide: number) {
    if (!editable) return;
    undone = null;
    await reshape(gesture({ kind: "cut", edge: "", index: 0, from: at - wide / 2, to: at + wide / 2, toWords: false }));
  }

  // The cut that was last put back, so the same double-click in the same
  // place can put it in again. Taking a part out is one double-click,
  // and nothing that takes one click may cost more than one to undo. It
  // belongs to the clip it was in, and it is forgotten the moment anything
  // else about that clip's cuts changes, because a part put back into a
  // clip that has moved on is not the part that was taken out.
  let undone = $state<{ key: string; from: number; to: number } | null>(null);

  // A double-click puts a cut back, the way a double-click undoes an edit
  // point in an editing timeline. It is not a single click, because a cut
  // is easy to land on by accident while scrubbing.
  async function putCutBack(index: number, event: MouseEvent) {
    event.preventDefault();
    if (!editable) return;
    const cut = cuts.find((c) => c.index === index);
    if (!cut) return;
    await reshape(gesture({ kind: "join", edge: "", index: 0, from: (cut.from + cut.to) / 2, to: 0, toWords: false }));
    undone = clip ? { key: clip.key, from: cut.from, to: cut.to } : null;
  }

  // Putting back what was just put back. The part is taken out again
  // exactly as it was, edge for edge.
  async function cutAgain(was: { from: number; to: number }) {
    if (!editable) return;
    undone = null;
    await reshape(gesture({ kind: "restore", edge: "", index: 0, from: was.from, to: was.to, toWords: false }));
  }

  // A view is read with as much again either side of it, so swiping along
  // the episode moves through what is already in hand. The waveform is drawn
  // by time, so what is read and what is shown need not line up.
  //
  // What is drawn and the part it was read for are set in the same
  // breath, or the old readings would be drawn against the new part for
  // as long as the reading takes, which looks like the waveform jumping
  // about. A reading that comes back after a newer one is dropped.
  let latest = 0;

  async function load(from: number, to: number) {
    view = { from, to };
    loaded = true;
    loadedHeard = heardIn(heard, from, to);
    const shown = Math.max(to - from, 0.001);
    const whole = Math.max(duration, to);
    const outer = { from: Math.max(0, from - shown), to: Math.min(whole, to + shown) };
    loadedMeasured = measured;
    loadedWhole = measuredParts.some(
      ([a, b]) => a <= outer.from + 0.02 && b >= Math.min(outer.to, duration) - 0.02,
    );
    const wide = (outer.to - outer.from) / shown;
    const buckets = Math.min(4000, Math.max(100, Math.round((width || 900) * wide)));
    // The words are wanted around the playhead, for snapping an edge to
    // them. Zoomed out to a four hour episode that would be forty thousand
    // of them for every swipe, so they are asked for by the ten minutes
    // around where the work is.
    const middle = time >= outer.from && time <= outer.to ? time : (from + to) / 2;
    const said = {
      from: Math.max(outer.from, middle - spoken / 2),
      to: Math.min(outer.to, middle + spoken / 2),
    };
    const mine = ++latest;
    try {
      const [w, p] = await Promise.all([
        api.words(path, said.from, said.to),
        api.waveform(path, outer.from, outer.to, buckets),
      ]);
      if (mine !== latest) return;
      words = w ?? [];
      peaks = p ?? [];
      data = outer;
    } catch {
      if (mine !== latest) return;
      words = [];
      peaks = [];
      data = outer;
    }
  }

  // Brings a moment into the view without changing how much of the episode
  // the view shows. How close the timeline stands is the hand's: a pinch
  // sets it, and fitting a clip sets it, and nothing else may. Following
  // the playhead is not a reason to change it. It used to be: pressing the
  // space bar took the view back to a minute wide, so zooming in and
  // playing threw the zoom away every time.
  //
  // A view the moment is already well inside does not move at all, or it
  // would shuffle along every time the playhead crossed the middle.
  function bring(at: number) {
    const shown = span;
    const whole = Math.max(duration, shown);
    if (at > view.from + shown * 0.1 && at < view.from + shown * 0.9) return;
    const from = Math.max(0, Math.min(at - shown * 0.25, whole - shown));
    load(from, from + shown);
  }

  // Clicking a clip in the list puts the timeline back on it, even when it
  // is the clip that is already selected and the view was moved by hand.
  // Given a moment, the timeline goes there instead: putting the playhead
  // somewhere on the range picker is asking to look there, clip or no clip.
  export function fit(at?: number) {
    if (at === undefined) {
      fitView();
      return;
    }
    // With a clip chosen, looking somewhere else is a move by hand: the
    // view stays where it was put until the crosshair in the row below
    // takes it back to the clip. With no clip it simply follows the
    // playhead.
    held = !!clip;
    viewFor = clip?.key ?? "";
    bring(at);
  }

  // The crosshair in the row below goes to the playhead. Always, clip or no
  // clip.
  //
  // Going back to the clip is what clicking the clip in the list does, and
  // it does it even for the clip already chosen, so the crosshair does not
  // need to do it as well. A control that went to the clip with one chosen
  // and to the playhead without could not be relied on for either.
  //
  // The playhead lands in the middle, because the whole reason to ask is to
  // see where it is. Everywhere else it sits a quarter in, where there is
  // room ahead of it for what is coming.
  //
  // With a clip chosen this counts as putting the view somewhere by hand,
  // or the effect below would pull it straight back to the clip. With no
  // clip the view follows the playhead by itself and has to keep doing so.
  export function toPlayhead() {
    held = !!clip;
    viewFor = clip?.key ?? "";
    const whole = Math.max(duration, span);
    const from = Math.max(0, Math.min(time - span / 2, whole - span));
    load(from, from + span);
  }

  // Where the timeline sits when nobody has moved it: the clip with a little
  // room around it, or the minute around the playhead.
  // A double-click fits the clip to the view, unless it lands on a cut, in
  // which case it puts that cut back.
  //
  // It has to be decided here rather than on the cut itself. The track takes
  // the pointer on the way down so a drag keeps working when it leaves the
  // track, and while a pointer is captured every click and double-click is
  // dealt to the element holding it. A handler on the cut is never reached.
  // How far across the track the middle of the video preview lies, from
  // nought at its left edge to one at its right. A measurement read when a
  // clip is fitted, never turned into a size.
  function centreOn(): number | undefined {
    const x = centre?.();
    const box = track?.getBoundingClientRect();
    if (x === undefined || !box || box.width <= 0) return undefined;
    const at = (x - box.left) / box.width;
    return at > 0.2 && at < 0.8 ? at : undefined;
  }

  function fitView(event?: MouseEvent) {
    // Shift is the cutting hand on this track, so a double-click with it
    // held takes a part out where it lands and never moves the view. It
    // used to fit the clip instead, which is why holding shift and
    // double-clicking read as nothing happening: the one gesture that
    // looked like it ought to cut only zoomed.
    if (event?.shiftKey) {
      event.preventDefault();
      const at = timeAt(event.clientX);
      // What those forty pixels are worth in seconds, here, at this zoom.
      // timeAt is linear and unclamped, so the difference is the same
      // anywhere on the track, and reading it costs no layout: it is a
      // measurement read, never turned back into a size.
      const wide = Math.abs(timeAt(event.clientX + cutAtOnce) - at);
      // Inside a cut there is nothing left to take out.
      if (!cuts.some((c) => at >= c.from && at <= c.to)) void cutHere(at, wide);
      return;
    }
    if (event && editable && onreshape && track) {
      const at = timeAt(event.clientX);
      const hit = cuts.find((c) => at >= c.from && at <= c.to);
      if (hit) {
        putCutBack(hit.index, event);
        return;
      }
      // Nothing to put back here, but this is where something was just put
      // back. The same gesture in the same place takes it out again.
      const back = undone;
      if (back && back.key === clip?.key && at >= back.from && at <= back.to) {
        event.preventDefault();
        cutAgain(back);
        return;
      }
    }
    held = false;
    if (clip && clip.segments.length) {
      const a = clip.segments[0].start;
      const b = clip.segments[clip.segments.length - 1].end;
      const pad = Math.max(8, (b - a) * 0.3);
      const shown = b - a + 2 * pad;
      // The clip stands under the video preview, so the frame here lines up
      // with the window on the range picker and the crop above it. Never so
      // far over that less than half the room is left on either side.
      const at = centreOn();
      const from = at === undefined
        ? a - pad
        : Math.min(a - pad / 2, Math.max(b + pad / 2 - shown, (a + b) / 2 - at * shown));
      viewFor = clip.key;
      load(Math.max(0, from), Math.min(duration, from + shown));
      return;
    }
    const whole = Math.max(duration, loose);
    const from = Math.max(0, Math.min(time - loose * 0.25, whole - loose));
    viewFor = "";
    load(from, Math.min(from + loose, whole));
  }

  // The arrow keys step the playhead a frame at a time, as in every video
  // tool, unless a field or an edge of the clip has the keyboard. Shift
  // steps by words, which is the thing the picture is showing: the caption
  // lights up the word being spoken, so this walks that light one word at
  // a time. Where nothing has been heard yet there are no words to walk,
  // and shift takes a second, which is all it ever took before.
  function onKey(event: KeyboardEvent) {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.defaultPrevented) return;
    const on = document.activeElement as HTMLElement | null;
    const tag = on?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || on?.isContentEditable) return;
    // The edges of a clip and the window on the range picker take the
    // arrows for themselves while they hold the keyboard.
    if (on?.getAttribute("role") === "slider") return;
    if (document.querySelector("dialog[open]")) return;
    const back = event.key === "ArrowLeft";
    event.preventDefault();
    const put = (t: number) => onseek(Math.max(0, Math.min(t, duration)));
    if (!event.shiftKey) {
      put(time + Math.max(frame, 1 / 240) * (back ? -1 : 1));
      return;
    }
    // Inside the clip, the words the caption lights up. Outside it there
    // is no caption at all, so the words that were heard are the only ones
    // there are.
    //
    // Inside by a whole frame either side, because the playhead is not
    // where it was put: picking a clip sends it to the clip's first second
    // and the picture answers with the frame it is showing, which begins a
    // little before that. Read exactly, the playhead was then outside the
    // clip it had just been put at the start of, so shift and an arrow
    // walked the transcript instead and the first press landed wherever
    // the word before the clip happened to be. That is the jump out of the
    // clip that could not be made to happen twice: it only happens on the
    // first press after picking one.
    const step = Math.max(frame, 1 / 240);
    const walk = lit.length && insideClip(drawnPieces, time, step) ? lit : words;
    const to = wordStep(walk, time, back, step);
    if (to !== null) {
      put(to);
      return;
    }
    // Out of words. At the ends of a clip that means the clip beside it,
    // because the words go on even where this clip does not.
    if (walk === lit) onwalkclip?.(back);
    else if (!walk.length) put(time + (back ? -1 : 1));
  }

  // Reading takes a call each, so it waits until the fingers come to rest.
  let settle = 0;

  function readSoon() {
    clearTimeout(settle);
    settle = setTimeout(() => load(view.from, view.to), 150);
  }

  // Puts the view somewhere, kept inside the episode and between the
  // closest the timeline goes and the whole of it.
  function showing(from: number, to: number) {
    const whole = Math.max(duration, loose);
    const shown = Math.min(Math.max(to - from, nearest), whole);
    const at = Math.min(Math.max(from, 0), whole - shown);
    view = { from: at, to: at + shown };
  }

  // Two fingers on the trackpad: a swipe moves along the episode, a pinch
  // zooms around the pointer. A pinch reaches a page as a wheel with the
  // control key held, which is how every trackpad says it.
  function wheel(event: WheelEvent) {
    if (!duration) return;
    const zooming = event.ctrlKey || event.metaKey;
    const along = event.deltaX + (event.shiftKey ? event.deltaY : 0);
    if (!zooming && Math.abs(along) < 0.5) return;
    event.preventDefault();
    held = true;
    if (zooming) {
      const at = timeAt(event.clientX);
      const share = Math.min(Math.max((at - view.from) / span, 0), 1);
      // The closest the timeline goes is a wall, not a slope. The width is
      // held between the closest and the whole episode here, before the
      // edges are worked out from it, because working them out from a
      // width that is then held somewhere else leaves the width standing
      // at the wall and the edges still moving: a pinch that could go no
      // closer slid the view sideways instead of doing nothing. At the
      // wall the width does not change, so neither edge moves either.
      const most = Math.max(duration, loose);
      const shown = Math.min(Math.max(span * Math.exp(event.deltaY * 0.01), nearest), most);
      showing(at - share * shown, at + (1 - share) * shown);
    } else {
      const shift = (along / Math.max(width, 1)) * span;
      showing(view.from + shift, view.to + shift);
    }
    readSoon();
  }

  // A new clip gets a new view. A trimmed clip keeps it unless an edge left
  // it. With no clip the view follows the playhead, so there is always a
  // timeline saying where you are in the episode. A view moved by hand is
  // left alone until the clip changes or the playhead leaves it.
  $effect(() => {
    if (clip && clip.segments.length) {
      if (viewFor !== clip.key) {
        fitView();
        return;
      }
      if (held) return;
      // Inside the episode, the way the view is, or a clip that reached
      // past either end was never inside the view and was read again and
      // again, without end.
      const a = Math.max(0, clip.segments[0].start);
      const b = Math.min(duration, clip.segments[clip.segments.length - 1].end);
      const pad = Math.max(8, (b - a) * 0.3);
      if (a < view.from || b > view.to) {
        load(Math.max(0, a - pad), Math.min(duration, b + pad));
      }
      return;
    }
    // A view moved by hand stays where it was put, wherever the playhead
    // goes. The crosshair, a double-click or another clip lets go of it.
    if (held) return;
    const shown = view.to - view.from;
    const middle = time > view.from + shown * 0.15 && time < view.from + shown * 0.85;
    const whole = Math.max(duration, loose);
    const from = Math.max(0, Math.min(time - loose * 0.25, whole - loose));
    // Nothing to do while the playhead is well inside the window, and
    // nothing to do when the window it wants is the one already loaded.
    if (loaded && viewFor === "" && (middle || Math.abs(from - view.from) < 0.5)) return;
    // A view that is already about the playhead just moves along with it.
    // Only a view that is about something else, or none at all, is laid
    // out afresh.
    if (loaded && viewFor === "") {
      bring(time);
      return;
    }
    fitView();
  });

  // A new episode's waveform arrives in seconds and its words as it is
  // heard, both piece by piece, so a view read before either reached it is
  // read again: the words when the transcript grows past where it stood,
  // the waveform when more is measured and what was read had a gap in it.
  // The waveform is measured where this timeline looks first, so a view
  // it jumped to fills in before the rest.
  $effect(() => {
    if (!loaded) return;
    const more = heardIn(heard, view.from, view.to) > loadedHeard + 0.5;
    const grew = measured > loadedMeasured + 0.05 && !loadedWhole;
    if (more || grew) load(view.from, view.to);
  });

  // The waveform, drawn the way an editor draws one: one column of the
  // screen per column of the picture, each a whole pixel wide and a whole
  // pixel tall. Zoomed out a column is the loudest reading that falls in
  // it. Zoomed in past the measurement the outline runs between the
  // readings instead, or the track steps in blocks eight pixels wide.
  //
  // It used to draw one bar per reading, wherever that reading landed,
  // which at most zooms is a bar a fraction of a pixel wide at a fractional
  // position. A bar like that is spread across two pixels at part strength,
  // and how much of it falls in which pixel changes with every step of the
  // zoom, so the whole waveform brightened and dimmed as it was zoomed.
  // Zoomed out it was worse: several readings landed in one pixel and were
  // drawn over each other, so some pixels took two part-strength bars and
  // came out brighter than their neighbours. Nothing here is ever drawn
  // between two pixels, so there is nothing left to shimmer.
  $effect(() => {
    void peaks;
    void width;
    void view;
    if (!canvas || !width) return;
    const ratio = window.devicePixelRatio || 1;
    const w = Math.max(1, Math.round(width * ratio));
    const h = Math.max(1, Math.round(canvas.clientHeight * ratio));
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d")!;
    ctx.clearRect(0, 0, w, h);
    ctx.fillStyle = getComputedStyle(canvas).getPropertyValue("--wave").trim();

    // The level to draw in each column, in decibels. Nothing at all where
    // no reading falls, so a column past the end of the transcript stays
    // empty rather than reading as silence.
    const loudest = new Float32Array(w).fill(-Infinity);
    const step = (data.to - data.from) / Math.max(peaks.length, 1);
    const scale = w / span;
    // How many columns one reading has to itself.
    const each = step * scale;
    if (each > 1.5) {
      // Zoomed in past the measurement. Loudness is measured every ten
      // milliseconds and that is all there is, so a second of it on a
      // retina screen is a hundred readings across eight hundred pixels:
      // eight pixels of exactly one height, then a step, then eight more.
      // That is what made the track look like a display with too few
      // pixels, and no amount of asking for more buckets can fix it,
      // because there is nothing finer to ask for.
      //
      // So the outline runs between the readings rather than standing
      // still at each one. It invents no detail: it is the same readings,
      // joined instead of squared off, which is what every editor draws
      // once the zoom passes what it measured.
      for (let x = 0; x < w; x++) {
        // Where this column sits between two readings, taking a reading to
        // stand at the middle of the ten milliseconds it measured.
        const at = (view.from + x / scale - data.from) / step - 0.5;
        const i = Math.floor(at);
        if (i < -1 || i >= peaks.length) continue;
        const a = peaks[Math.max(i, 0)];
        const b = peaks[Math.min(i + 1, peaks.length - 1)];
        loudest[x] = a + (b - a) * (at - i);
      }
    } else {
      // Zoomed out, where several readings fall in one column. The loudest
      // of them wins, which is what a waveform is for: a peak that was
      // averaged away is a peak nobody can see.
      for (let i = 0; i < peaks.length; i++) {
        const at = data.from + i * step;
        const first = Math.floor((at - view.from) * scale);
        if (first >= w) break;
        const last = Math.max(first, Math.ceil((at + step - view.from) * scale) - 1);
        if (last < 0) continue;
        for (let x = Math.max(first, 0); x <= Math.min(last, w - 1); x++) {
          if (peaks[i] > loudest[x]) loudest[x] = peaks[i];
        }
      }
    }

    const pad = Math.round(2 * ratio);
    const room = Math.max(1, h - 2 * pad);
    for (let x = 0; x < w; x++) {
      if (loudest[x] === -Infinity) continue;
      const level = Math.max(0, Math.min(1, (loudest[x] + 60) / 60));
      const tall = Math.max(1, Math.round(level * room));
      ctx.fillRect(x, Math.round((h - tall) / 2), 1, tall);
    }
  });

  onMount(() => {
    const observer = new ResizeObserver(() => (width = track.clientWidth));
    observer.observe(track);
    // Not the listener Svelte would attach: a wheel we act on is ours, and
    // a passive one cannot say so.
    track.addEventListener("wheel", wheel, { passive: false });
    window.addEventListener("keydown", onKey);
    return () => {
      observer.disconnect();
      track.removeEventListener("wheel", wheel);
      window.removeEventListener("keydown", onKey);
      clearTimeout(settle);
    };
  });

  // The captions along the foot of the track. A caption appears when its
  // first word is said and goes when the next appears, or a moment after
  // its last word when a pause follows, and where that is a little off
  // from what is heard, either edge is dragged. The two sides of the line
  // between two captions are two edges: the left side is where the one
  // before goes, the right side where the one after appears. Dragging the
  // first leaves a gap, dragging the second takes the one before along.
  let capDraft = $state<CaptionDraft | null>(null);
  let capMoving = $state(false);
  let capSaving = $state(false);
  // The captions a draft was made against. The draft is let go of when
  // they come back changed, so the block never jumps back to where it was
  // while the saved captions are on their way.
  let capHeld: CaptionCue[] | undefined;

  // The clip as a gesture shapes it goes to the video preview too, so it
  // shows the same clip as the timeline while the hand moves.
  $effect(() => {
    onshape?.(shapedShown && shaped ? { cues: shaped.cues, pieces: shaped.pieces } : null);
  });
  const shownCues = $derived(shapedShown && shaped ? shaped.cues : captions);
  const cuePieces = $derived(shapedShown && shaped ? shaped.pieces : segments);

  const clipLength = $derived(cuePieces.reduce((sum, p) => sum + p.end - p.start, 0));

  const captionBlocks = $derived.by(() => {
    if (!clip || !shownCues?.length || !cuePieces.length) return [];
    const list = shapedShown ? shownCues : draftCaptions(shownCues, capDraft);
    return list.map((c, i) => ({
      i,
      c: shownCues[i],
      from: inEpisode(cuePieces, c.start),
      to: inEpisode(cuePieces, Math.min(c.end, clipLength)),
    }));
  });

  // The other clips in view, as marks across the middle of the track.
  // Clips may overlap, and inside the chosen clip its own captions are
  // what is worked on, so another clip's mark gives way there: drawn over
  // the captions, it hid them, and the caption the video preview shows,
  // in the accent over a mark in the accent, could not be seen at all.
  // What is left of a mark on either side still says where that clip is,
  // and the range picker shows the two lying over each other.
  const shownMarks = $derived.by(() => {
    const out: {
      id: string;
      key: string;
      at: number;
      start: number;
      end: number;
      rendered: boolean;
      arriving?: boolean;
      pick?: string;
    }[] = [];
    for (const m of marks) {
      if (m.key === clip?.key || m.pick === clip?.key || m.end <= view.from || m.start >= view.to) {
        continue;
      }
      const pieces = wholeClip
        ? [
            { start: m.start, end: Math.min(m.end, wholeClip.start) },
            { start: Math.max(m.start, wholeClip.end), end: m.end },
          ]
        : [{ start: m.start, end: m.end }];
      pieces.forEach((p, i) => {
        if (p.end - p.start > 0.001) out.push({ ...m, ...p, id: `${m.key}-${i}`, at: m.start });
      });
    }
    return out;
  });

  // A caption block is detail for working inside a clip, and it is drawn
  // only while it can be read as a block: while the caption in the middle
  // of the list is at least as wide as a block is tall. Narrower, its line
  // has no room inside its padding and the blocks run into one smear, which
  // says nothing about the captions and covers the waveform. Editors do the
  // same with titles on a timeline: the detail comes in as there is room
  // for it. The middle caption rather than the narrowest, so one short
  // "Ja." does not take them all away.
  const captionLeast = 16;
  const captionsReadable = $derived.by(() => {
    if (!captionBlocks.length || !width) return false;
    const wide = captionBlocks.map((b) => ((b.to - b.from) / span) * width).sort((a, b) => a - b);
    return wide[Math.floor(wide.length / 2)] >= captionLeast;
  });

  // The last word of a caption to have begun, counted through its lines,
  // or -1 before its first. The block of the caption shown is drawn afresh
  // whenever it changes, so it pops on every word the way the pill does in
  // the video preview, where each word lights up in turn, and not again in
  // the pause after a word. Walking the words with shift and the arrow
  // keys pops both at once.
  const spokenAt = $derived(clip && cuePieces.length ? inClip(cuePieces, time) : -1);
  function wordNow(c: CaptionCue): number {
    return litWord(c.lines ?? [], spokenAt);
  }

  // Where a click on a caption puts the playhead: a frame into its first
  // word, on the clip's own clock and then in the episode, the same step
  // the arrow keys take into a word. A word is lit from its start, and a
  // moment read back through the episode can land a hair before it.
  function firstWordOf(c: CaptionCue): number | null {
    const word = c.lines?.[0]?.words?.[0];
    if (!word || !cuePieces.length) return null;
    return inEpisode(cuePieces, intoWord(word, frame));
  }

  $effect(() => {
    if (capDraft && !capMoving && !capSaving && captions !== capHeld) {
      capDraft = null;
      oncaptiondraft?.(null);
    }
  });

  function grabCaption(i: number, edge: "start" | "end", event: PointerEvent) {
    if (!clip || locked || capSaving || !oncaptiontime || !captions?.length) return;
    const c = captions[i];
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const from = event.clientX;
    capHeld = captions;
    let moved = false;
    const move = (e: PointerEvent) => {
      if (!moved && Math.abs(e.clientX - from) > 2) {
        moved = true;
        capMoving = true;
      }
      if (!moved) return;
      // On the clip's clock, a whole frame at a time, and never over the
      // caption beside it or past its own other edge.
      let at = Math.round(inClip(segments, timeAt(e.clientX)) / frame) * frame;
      const shortest = 0.1;
      if (edge === "start") {
        const low = i > 0 ? captions[i - 1].start + shortest : 0;
        at = Math.min(Math.max(at, low), c.end - shortest);
      } else {
        const high = i + 1 < captions.length ? captions[i + 1].start : clipLength;
        at = Math.min(Math.max(at, c.start + shortest), high);
      }
      capDraft = { index: i, edge, at };
      oncaptiondraft?.(capDraft);
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      capMoving = false;
      const d = capDraft;
      const word = edge === "start" ? c.first : c.last;
      if (!moved || !d || word === undefined) {
        // A click on an edge puts the playhead on it, the way a clip edge
        // does, which is how a caption is heard from where it appears.
        capDraft = null;
        oncaptiondraft?.(null);
        if (!moved) onseek(inEpisode(segments, edge === "start" ? c.start : Math.min(c.end, clipLength)), "clip");
        return;
      }
      capSaving = true;
      const saved = await oncaptiontime(word, edge, inEpisode(segments, d.at));
      capSaving = false;
      if (!saved) {
        capDraft = null;
        oncaptiondraft?.(null);
      }
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // A double-click on an edge put there by hand puts it back where its
  // words put it.
  function resetCaption(i: number, edge: "start" | "end") {
    const c = captions?.[i];
    if (!c || !oncaptiontime || locked) return;
    const word = edge === "start" ? c.first : c.last;
    const moved = edge === "start" ? c.startMoved : c.endMoved;
    if (word === undefined || !moved) return;
    oncaptiontime(word, edge, -1);
  }

  // Where in the episode this is. Without them a swipe leaves you nowhere,
  // and the step is round and wide enough that the labels never crowd.
  // A thumbnail being dragged. It is drawn where the hand is, the playhead
  // goes with it so the video preview shows the frame under the hand, and
  // it stays where it was let go until the clip comes back with it saved.
  let thumbDrag = $state<null | { from: number; to: number }>(null);
  const thumbMarks = $derived(
    thumbnails.map((at) => ({
      at,
      shown: thumbDrag && thumbDrag.from === at ? thumbDrag.to : at,
    })),
  );
  // The frame the playhead is in, which is how a mark knows it is the one
  // on screen. A state, never a distance.
  const playFrame = $derived(Math.floor(time / frame));

  // The middle of the frame a moment falls in, inside a piece the clip
  // keeps: a moment that was cut out is not in the short, so a mark held
  // over a cut stays at the edge of the piece nearest the hand.
  function thumbFrame(t: number): number {
    const mid = (u: number) => (Math.floor(u / frame) + 0.5) * frame;
    let best = mid(t);
    let gap = Infinity;
    for (const p of segments) {
      const least = mid(p.start + frame / 2);
      const most = mid(p.end - frame / 2);
      if (most < least) continue;
      const at = Math.min(Math.max(mid(t), least), most);
      if (Math.abs(at - t) < gap) {
        gap = Math.abs(at - t);
        best = at;
      }
    }
    return best;
  }

  function grabThumb(at: number, event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startX = event.clientX;
    let moved = false;
    const move = (e: PointerEvent) => {
      if (locked || !onthumbnail) return;
      if (!moved && Math.abs(e.clientX - startX) > 2) moved = true;
      if (!moved) return;
      const to = thumbFrame(timeAt(e.clientX));
      thumbDrag = { from: at, to };
      seekSoon(to);
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      const d = thumbDrag;
      // A click puts the playhead on it, so the video preview shows it.
      if (!moved || !d) {
        thumbDrag = null;
        onseek(at);
        return;
      }
      cancelAnimationFrame(queued);
      queued = 0;
      onseek(d.to);
      if (Math.floor(d.to / frame) === Math.floor(d.from / frame)) {
        thumbDrag = null;
        return;
      }
      try {
        await onthumbnail?.(d.from, d.to);
      } finally {
        thumbDrag = null;
      }
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  const ticks = $derived.by(() => {
    if (!width || span <= 0) return [];
    const steps = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600];
    const room = Math.max(1, Math.floor(width / 96));
    const step = steps.find((s) => span / s <= room) ?? 3600;
    const out: number[] = [];
    for (let t = Math.ceil(view.from / step) * step; t <= view.to; t += step) {
      out.push(Math.round(t));
    }
    return out;
  });

</script>

<div class="clip-timeline">
  <!-- Nothing above the waveform: the range picker sits right over it, and
       what the clip is and what to do with it are in the row below. The
       playhead is drawn over the track rather than in it: the track clips
       what is inside it, which is what keeps the waveform inside its
       rounded corners, and the playhead is the one thing that has to reach
       past them. -->
  <div class="over" class:scrubbing>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="track asks"
    class:scrubbing
    bind:this={track}
    onpointerdown={scrub}
    ondblclick={fitView}
    aria-label="The clip timeline"
  >
    <canvas bind:this={canvas}></canvas>
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <span
      class="ask corner"
      onpointerdown={(e) => e.stopPropagation()}
      ondblclick={(e) => e.stopPropagation()}
    >
      <Info label="What the clip timeline is" side="right">
        The episode up close. Drag to move the playhead, two fingers to travel, a pinch to zoom,
        and a double-click to fit the clip. The arrow keys step a frame. With shift they step a
        word, so the caption in the picture lights up the next one, and past the last word of a
        clip they carry on into the one beside it. Shift with the arrows up and down takes the
        next clip and starts it from the top. Drag
        a clip edge to trim it, frame by frame, or with shift held from word to word. The words are in the picture, in the caption box, which is where
        they are read and where they are corrected. A hatched block inside a clip is
        a part it leaves out. Drag either edge of one to change it, with shift for whole words, double-click one to put it
        back, and double-click again to take it out once more. Shift is the cutting hand: hold it
        and drag across the clip to take out the part you drag over, or hold it and double-click
        to take one out where you click. Cuts land on the frame. Hold alt as well as shift to land
        on whole words instead, which takes the whole pause a cut falls in. Along the foot are the captions,
        each from where it appears to where it goes, and the one the video preview is showing is
        lit. Where one is a little early or late against what you hear, drag its edge: the left
        side of a gap between two captions is where the one before goes, the right side where the
        one after appears. A double-click on an edge moved by hand puts it back. The
        small pictures along the bottom are the thumbnails, the frames Render writes beside the
        short. The thumbnail button under the timeline, or T, makes the frame under the playhead
        one, and takes it away again. Drag one to another frame.
      </Info>
    </span>
    <!-- The ruler in two layers, the same as on the range picker: the line
         under what is drawn on the track, the time over it. A time written
         inside its own line is held at the line's level, because an element
         with a z-index makes a stacking context, and then anything laid
         over the track swallows it. -->
    {#each ticks as t (t)}
      <div class="tick" style="left: {x(t)}%"></div>
    {/each}
    {#each ticks as t (t)}
      <span class="num time" style="left: {x(t)}%">{clock(t)}</span>
    {/each}
    <!-- The clip, whole, from its first piece to its last. The rules above
         and below run the length of it whatever is cut out in between,
         because the holes are inside one clip and not between several: a
         rule that broke at every cut would read as three clips standing in
         a row. -->
    {#if wholeClip}
      <div
        class="span frame"
        class:lit={!!clip && clip.key === hovered}
        class:dim={dimmed}
        style="left: {x(wholeClip.start)}%; width: {x(wholeClip.end) - x(wholeClip.start)}%"
      ></div>
    {/if}
    {#each drawnPieces as p, i (i)}
      <div
        class="piece"
        class:first={i === 0}
        class:last={i === drawnPieces.length - 1}
        style="left: {x(p.start)}%; width: {x(p.end) - x(p.start)}%"
      ></div>
    {/each}
    <!-- A cut is drawn over the pieces rather than between them, so a cut
         being dragged wider is seen taking the piece rather than waiting
         for the piece to give way. -->
    {#each cuts as c (c.index)}
      <div
        class="cut"
        class:editable
        class:drawing={drawing(c)}
        style="left: {x(c.from)}%; width: {x(c.to) - x(c.from)}%"
        title={editable
          ? "A part the clip leaves out. Drag an edge to change it, double-click to put it back."
          : "A part the clip leaves out."}
      ></div>
    {/each}
    {#if editable && onreshape}
      <!-- The handles come after every cut, so a handle is never drawn
           under the next cut's block. -->
      {#each cuts as c (c.index)}
        <div
          class="cutedge"
          class:active={hand?.kind === "move" && hand.index === c.index}
          style="left: {x(c.from)}%"
          role="slider"
          tabindex="-1"
          aria-label="Where cut {c.index + 1} starts"
          aria-valuenow={c.from}
          onpointerdown={(e) => grabCut(c.index, "from", e)}
        ></div>
        <div
          class="cutedge"
          class:active={hand?.kind === "move" && hand.index === c.index}
          style="left: {x(c.to)}%"
          role="slider"
          tabindex="-1"
          aria-label="Where cut {c.index + 1} ends"
          aria-valuenow={c.to}
          onpointerdown={(e) => grabCut(c.index, "to", e)}
        ></div>
      {/each}
    {/if}
    <!-- The other clips, the same marks the range picker draws, so the
         timeline zoomed out shows where they all are. A click chooses one. -->
    {#each shownMarks as m (m.id)}
      <button
        class="clipmark"
        class:rendered={m.rendered}
        class:lit={m.key === hovered}
        class:waiting={m.arriving}
        style="left: {x(m.start)}%; width: {x(m.end) - x(m.start)}%"
        aria-label="Clip at {clock(m.at)}"
        title="Choose the clip at {clock(m.at)}"
        {@attach hoverClip(m.key, onhover)}
        onpointerdown={(e) => e.stopPropagation()}
        onclick={() => onmark?.(m.pick ?? m.key)}
      ></button>
    {/each}
    {#if clip && !locked}
      <div
        class="edge"
        class:active={hand?.kind === "trim" && hand.edge === "start"}
        style="left: {x(start)}%"
        role="slider"
        tabindex="-1"
        aria-label="Clip start"
        aria-valuenow={start}
        onpointerdown={(e) => grab("start", e)}
      ></div>
      <div
        class="edge"
        class:active={hand?.kind === "trim" && hand.edge === "end"}
        style="left: {x(end)}%"
        role="slider"
        tabindex="-1"
        aria-label="Clip end"
        aria-valuenow={end}
        onpointerdown={(e) => grab("end", e)}
      ></div>
    {/if}
    {#if captionsReadable}
      <!-- The captions across the middle of the track, each a block from
           where it appears to where it goes. -->
      <div
        class="captions"
        class:arriving
        style="--cap-text: {captionLook?.text ?? 'var(--text)'}; --cap-box: {captionLook?.box ??
          'transparent'}; --cap-pill: {captionLook?.highlight ?? 'var(--accent)'}"
      >
        {#each captionBlocks as b (b.i)}
          <!-- A click puts the playhead on the caption's first word, where
               the video preview shows it spoken. That is where it appears,
               except for the first caption of a clip, which is on screen
               from the clip's first frame, before its first word. -->
          <!-- A block takes no focus. The keys walk the words wherever the
               focus is, and a block that kept it from a click wore the
               focus ring the moment a key was pressed, round a caption the
               video preview had long left. -->
          <!-- Drawn afresh on every word of the caption shown, which is what
               starts its pop again. A key on the list would not do it: the
               list is only looked at again when the captions change. -->
          {#key time >= b.from && time < b.to ? wordNow(b.c) : -2}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_interactive_supports_focus -->
            <div
              class="caption"
              class:showing={time >= b.from && time < b.to}
              style="left: {x(b.from)}%; width: calc({Math.max(x(b.to) - x(b.from), 0)}% - 2px); --i: {b.i}"
              role="button"
              title="Put the playhead where this caption appears"
              onpointerdown={(e) => e.stopPropagation()}
              onclick={() => onseek(firstWordOf(b.c) ?? b.from, "clip")}
            ><i></i></div>
          {/key}
        {/each}
      </div>
      {#if !locked && oncaptiontime && !shapedShown}
        {#each captionBlocks as b (b.i)}
          <div
            class="capedge start"
            class:active={capDraft?.index === b.i && capDraft.edge === "start"}
            style="left: {x(b.from)}%"
            role="slider"
            tabindex="-1"
            aria-label="When caption {b.i + 1} appears"
            aria-valuenow={b.c.start}
            title={b.c.startMoved
              ? "When this caption appears, put here by hand. Drag to move it, double-click to put it back where its words put it."
              : "When this caption appears. Drag to move it."}
            onpointerdown={(e) => grabCaption(b.i, "start", e)}
            ondblclick={(e) => {
              e.stopPropagation();
              resetCaption(b.i, "start");
            }}
          ></div>
          <div
            class="capedge end"
            class:active={capDraft?.index === b.i && capDraft.edge === "end"}
            style="left: {x(b.to)}%"
            role="slider"
            tabindex="-1"
            aria-label="When caption {b.i + 1} goes"
            aria-valuenow={b.c.end}
            title={b.c.endMoved
              ? "When this caption goes, put here by hand. Drag to move it, double-click to put it back where its words put it."
              : "When this caption goes. Drag to move it."}
            onpointerdown={(e) => grabCaption(b.i, "end", e)}
            ondblclick={(e) => {
              e.stopPropagation();
              resetCaption(b.i, "end");
            }}
          ></div>
        {/each}
      {/if}
    {/if}
    {#if clip}
      <!-- The thumbnails along the foot of the track, each a picture in the
           shape of a short, over the captions, lit when the playhead is on
           its frame. -->
      {#each thumbMarks as m, i (m.at)}
        <div
          class="thumb"
          class:here={Math.floor(m.shown / frame) === playFrame}
          class:active={thumbDrag?.from === m.at}
          style="left: {x(m.shown)}%"
          role="slider"
          tabindex="-1"
          aria-label="Thumbnail {i + 1}"
          aria-valuenow={m.shown}
          title={locked
            ? "A thumbnail. Click to see it"
            : "A thumbnail. Click to see it, drag it to another frame. The thumbnail button or T removes it"}
          onpointerdown={(e) => grabThumb(m.at, e)}
        >
          <Icon name="thumbnail" size={18} />
        </div>
      {/each}
    {/if}
  </div>
    {#if time >= view.from && time <= view.to}
      <div class="at" style="left: {x(time)}%">
        <!-- The line takes the drag as well as its head, a few pixels
             either side of it, so the playhead is taken hold of wherever
             the hand finds it, the same as on the range picker. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="playhead" onpointerdown={scrub} title="Drag to move the playhead"></div>
        <!-- The head is its own element rather than something drawn on the
             line, because it stands above the track and the track is what
             takes the drag. Drawn but not grabbable, its top five pixels
             did nothing: the eye saw a handle and the hand went through it.
             It scrubs the same way the track does, so where a drag starts
             makes no difference to what it does. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="head"
          onpointerdown={scrub}
          ondblclick={fitView}
          title="Drag to move the playhead"
        ></div>
      </div>
    {/if}
  </div>
</div>

<style>
  .clip-timeline {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  /* The box the playhead is drawn over, exactly the track and nothing
     more, so a position worked out for the track is right here too. */
  .over {
    position: relative;
  }

  .track {
    position: relative;
    /* As tall as the height of the app allows, set by the workspace. */
    height: var(--wave-h, 112px);
    background: var(--well);
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    overflow: hidden;
    cursor: pointer;
    touch-action: none;
  }

  .track.scrubbing {
    cursor: grabbing;
  }

  canvas {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
  }

  .tick {
    position: absolute;
    top: 0;
    bottom: 0;
    border-left: 1px solid var(--ink-3);
    pointer-events: none;
    z-index: 1;
  }

  /* At the top, the same as on the range picker: two tracks that both say
     where you are say it in the same place and in the same colour, and
     that colour keeps the times behind the waveform they are about. In a
     layer of its own, over everything drawn on the track, because a time
     nothing can read is worse than no time at all. */
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

  /* The clip, whole. Two rules, above and below, from its first piece to
     its last. They are what says this is one clip: they run the length of
     it whether or not there are holes in it, and the vertical edges at
     either end close it. Before this, every piece carried its own rules
     and a clip with a cut in it read as two clips side by side. */
  .span {
    position: absolute;
    top: 0;
    bottom: 0;
    /* Its line and corners are the frame in app.css, the same as the crop
       in the video preview and the window on the range picker. */
    pointer-events: none;
    /* Over the time lines, the cuts and the captions, so the clip is one
       solid frame that nothing on the track crosses. */
    z-index: 2;
  }

  /* What the clip keeps. The wash is the one thing that says which parts
     of the span are in it, so nothing else needs to. */
  .piece {
    position: absolute;
    top: 0;
    bottom: 0;
    background: var(--accent-wash);
    pointer-events: none;
  }

  /* The wash at the clip's two ends takes the frame's corners, so none of
     it shows outside them. */
  .piece.first {
    border-radius: var(--frame-radius) 0 0 var(--frame-radius);
  }

  .piece.last {
    border-radius: 0 var(--frame-radius) var(--frame-radius) 0;
  }

  .piece.first.last {
    border-radius: var(--frame-radius);
  }

  /* A part the clip leaves out. It is the track's own background and
     nothing else: what is not in the clip looks like everything else that
     is not in the clip, which is the plainest way to say it. It used to
     be hatched, and a hatch over a waveform is a second pattern laid on a
     first, which is why the track read as a display with too few pixels.
     The two edit points carry the meaning instead, the way an editor
     marks the place two shots were joined. */
  .cut {
    position: absolute;
    top: 0;
    bottom: 0;
    border-left: 1px solid var(--accent-hi);
    border-right: 1px solid var(--accent-hi);
    pointer-events: none;
    z-index: 1;
  }

  /* A cut the hand can reach takes the pointer, so a double-click can put
     it back. It does not stop the drag underneath: the pointer event goes
     on up to the track, which is what moves the playhead, so scrubbing
     across a cut works exactly as it did. */
  .cut.editable {
    pointer-events: auto;
  }

  /* The cut the hand is drawing, before the engine has been asked. It is
     already a cut, because the wash parts under the hand as it moves, so
     all it needs is to stand out as the one being worked on. */
  .cut.drawing {
    border-left-width: 2px;
    border-right-width: 2px;
    pointer-events: none;
  }

  /* The edges of a cut are grabbed the way the clip's own edges are, so
     they are the same width and the same shape. They are drawn in the
     accent's lighter shade rather than the accent, because what they move
     is the hole and not the clip. */
  .cutedge {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 12px;
    margin-left: -6px;
    cursor: ew-resize;
    z-index: 2;
  }

  .cutedge::after {
    content: "";
    position: absolute;
    left: 5px;
    top: 0;
    bottom: 0;
    width: 2px;
    background: var(--accent-hi);
    opacity: 0.55;
  }

  .cutedge:hover::after,
  .cutedge.active::after {
    left: 4px;
    width: 4px;
    opacity: 1;
  }

  /* The captions in a band across the middle of the track, where the
     waveform is at its quietest, with room above and below them. The band
     is 24 pixels, a block of 16 with 4 above and below it, and it lands on
     a whole pixel whatever height the track is. */
  .captions {
    position: absolute;
    left: 0;
    right: 0;
    top: round(down, calc(50% - 12px), 1px);
    height: 24px;
    pointer-events: none;
    /* Over everything else on the track, the clip frame, the time lines
       and their times and the playhead, the way the captions lie over the
       picture in the short, so a box that lets the picture through lets
       the track through here and nothing is drawn across a caption. */
    z-index: 5;
  }

  /* A block is the caption in the colours the short burns it in: the box
     colour, as see-through as the box is in the short, over the waveform
     the way the box lies over the picture, and a bar in the colour of the
     words. The colours are the short's and never change with the state,
     because a dimmed colour is another colour. At rest the bar is a
     hairline, and under the pointer it grows.

     The caption the video preview is showing wears the highlight colour
     over its box, the pill over the box the way the short draws it, each
     as see-through as it is set to be, so it shows two colours, the
     highlight and the words. It bounces into place with the same pop the
     pill makes in the video preview.

     A block is drawn a pixel short of its time at each end, so two
     captions that meet show a gap. */
  .caption {
    position: absolute;
    top: 4px;
    height: 16px;
    margin-left: 1px;
    box-sizing: border-box;
    padding: 0 6px;
    display: flex;
    align-items: center;
    background: var(--cap-box);
    border-radius: 3px;
    pointer-events: auto;
    cursor: pointer;
  }

  .caption.showing {
    z-index: 1;
    background:
      linear-gradient(var(--cap-pill), var(--cap-pill)),
      var(--cap-box);
    /* The pop the word makes in the video preview, as tall as the band the
       block stands in at its peak, 16 pixels grown to 24, and hardly wider,
       so it never runs into the caption beside it. */
    --pop-from-x: 0.97;
    --pop-from-y: 0.7;
    --pop-peak-x: 1.03;
    --pop-peak-y: 1.5;
    animation: pop 0.22s ease-out;
  }

  .caption i {
    display: block;
    flex: 1;
    min-width: 0;
    height: 2px;
    border-radius: 1px;
    background: var(--cap-text);
    transition: height 0.16s cubic-bezier(0.34, 1.56, 0.64, 1);
  }

  .caption:hover i,
  .caption.showing i {
    height: 4px;
    border-radius: 2px;
  }

  @media (prefers-reduced-motion: reduce) {
    .caption i {
      transition: none;
    }

    .caption.showing {
      animation: none;
    }
  }

  /* An edge is grabbed on its own side of the gap, so where two captions
     meet the left side is the one before going and the right side the one
     after appearing. It stands in the band, as tall as it. */
  .capedge {
    position: absolute;
    top: round(down, calc(50% - 12px), 1px);
    height: 24px;
    width: 8px;
    cursor: ew-resize;
    z-index: 6;
  }

  .capedge.end {
    margin-left: -8px;
  }

  /* The handle: a white line with a dark edge, which shows on any colour
     a caption can have. */
  .capedge::after {
    content: "";
    position: absolute;
    top: 4px;
    bottom: 4px;
    width: 2px;
    background: var(--text);
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.7);
    border-radius: 1px;
    opacity: 0;
  }

  .capedge.start::after {
    left: 1px;
  }

  .capedge.end::after {
    right: 1px;
  }

  /* The handle shows while the pointer is on it and while it is dragged,
     and goes again when the hand lets go. */
  .capedge:hover::after,
  .capedge.active::after {
    opacity: 1;
  }

  /* A thumbnail: the picture the thumbnail button wears, standing on the
     foot of the track in the app's colour. It sits on a chip of the app's
     colour while the playhead is on its frame, under the hand or being
     dragged. Whole pixels throughout. */
  .thumb {
    position: absolute;
    bottom: 3px;
    width: 22px;
    height: 22px;
    margin-left: -11px;
    display: grid;
    place-items: center;
    border-radius: 4px;
    color: var(--accent-hi);
    cursor: ew-resize;
    touch-action: none;
    z-index: 6;
  }

  .thumb :global(svg) {
    pointer-events: none;
  }

  .thumb:hover,
  .thumb.here,
  .thumb.active {
    background: var(--accent);
    color: #fff;
  }

  /* The playhead. */
  .at {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 0;
    z-index: 3;
  }

  .edge {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 12px;
    margin-left: -6px;
    cursor: ew-resize;
    /* Over the handles of the captions. The first caption is on screen
       from the clip's first frame, so its handle stood on the clip's start
       edge, and a hand that reached for the clip in the band of the
       captions moved the caption instead. The clip edge is the one a hand
       there means. */
    z-index: 7;
  }

  /* The frame draws the clip's sides, so the edge only shows itself when
     it is reached or dragged, wider and brighter. */
  .edge::after {
    content: "";
    position: absolute;
    left: 5px;
    top: 0;
    bottom: 0;
    width: 2px;
    background: transparent;
    border-radius: 2px;
  }

  .edge:hover::after,
  .edge.active::after {
    left: 4px;
    width: 4px;
    background: var(--accent-hi);
  }

  /* A thin line with a head at the top, the way every editor draws one.
     The head is what says this is the playhead and not a mark or an edge,
     and it is what the eye finds when the line itself is lost in a
     waveform. */
  /* The head stands above the track, which is where an editor puts it and
     what says this is the playhead rather than a line someone drew. */
  /* It is dragged left and right, and says so under the pointer, head and
     line alike, the way a clip's edges do. A press on the empty track also
     moves it, and there the pointer is the hand of anything pressed. */
  .playhead {
    position: absolute;
    top: -5px;
    bottom: 0;
    left: -1px;
    width: 2px;
    background: var(--accent-hi);
    cursor: ew-resize;
    touch-action: none;
  }

  .playhead::before {
    content: "";
    position: absolute;
    top: 0;
    bottom: 0;
    left: -4px;
    right: -4px;
  }

  /* The same head, in the same place, as the line used to draw on itself.
     It is an element of its own now so it can take the drag: it stands
     above the track, the track is what scrubs, and a head that is drawn
     outside it is a handle the hand goes straight through. */
  .head {
    position: absolute;
    top: -5px;
    left: -5px;
    width: 10px;
    height: 9px;
    border-radius: 2px 2px 1px 1px;
    background: var(--accent-hi);
    cursor: ew-resize;
    touch-action: none;
  }

  .over.scrubbing .playhead,
  .over.scrubbing .head {
    background: #fff;
  }

  /* The caption blocks of a clip on its way come in one after another, in
     the order they are said, the way a clip is built. */
  .captions.arriving .caption {
    animation: arrive 180ms ease-out both;
    animation-delay: calc(var(--i) * 70ms);
  }

  @keyframes arrive {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }
</style>
