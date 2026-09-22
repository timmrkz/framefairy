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
  // cuts between them, and either edge can be dragged to trim. Edges snap to
  // words the way the render cuts them. Two fingers move along the episode
  // and pinch to zoom, the way an editing timeline does.
  import { onMount } from "svelte";
  import {
    api,
    clock,
    cutAt,
    snapCut,
    snapEnd,
    snapStart,
    wordStep,
    type ClipEntry,
    type Word,
  } from "../lib/api";
  import { insideClip } from "../lib/flow";
  import Info from "./Info.svelte";

  let {
    path,
    clip,
    working = false,
    duration,
    covered = duration,
    time,
    locked = false,
    frame = 1 / 30,
    lit = [],
    onseek,
    ontrim,
    oncut,
    onjoincut,
    onmovecut,
    onwalkclip,
    numbers = $bindable({ start: 0, end: 0, seconds: 0, pieces: 0, saving: false }),
  }: {
    path: string;
    // Without a clip the timeline follows the playhead through the episode.
    clip: ClipEntry | null;
    // True while the transcript is growing. With nothing to draw yet, the
    // track is a place waiting to be filled.
    working?: boolean;
    duration: number;
    // How far the transcript has come. The words and the waveform arrive
    // with it, so the view is taken again as it grows.
    covered?: number;
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
    onseek: (t: number) => void;
    ontrim?: (start: number, end: number) => Promise<void>;
    // The cuts inside the clip: taking a stretch out, putting one back, and
    // moving the edges of one that is already there.
    // toWords says whether the engine should put the edges on the words
    // around them. Alt held while dragging says no: the edges land on the
    // frame they were let go on and stay there.
    oncut?: (from: number, to: number, toWords: boolean) => Promise<void>;
    onjoincut?: (at: number) => Promise<void>;
    onmovecut?: (index: number, from: number, to: number, toWords: boolean) => Promise<void>;
    // Walking the words has run off the end of the clip. The words of the
    // clip beside it are not here to walk on to, they arrive with its
    // captions, so the workspace is asked and it takes it from there.
    onwalkclip?: (back: boolean) => void;
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
      saving: saving || cutSaving,
    };
  });

  // How much of the episode the timeline shows when no clip is selected.
  const loose = 60;

  let track: HTMLDivElement;
  let canvas: HTMLCanvasElement;
  let width = $state(0);
  let view = $state({ from: 0, to: 1 });
  let words = $state<Word[]>([]);
  let keepPause = $state(0.1);
  let peaks = $state<number[]>([]);
  let dragging = $state<null | "start" | "end">(null);
  let draft = $state({ start: 0, end: 0 });
  let saving = $state(false);
  let viewFor = "";
  let loaded = false;
  // How far the transcript had come when this view was read.
  let loadedTo = $state(-1);
  // The stretch the words and the waveform were read for. It is wider than
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
  const start = $derived(dragging || saving ? draft.start : first);
  const end = $derived(dragging || saving ? draft.end : last);
  const span = $derived(Math.max(view.to - view.from, 0.001));

  function x(t: number): number {
    return ((t - view.from) / span) * 100;
  }

  function timeAt(clientX: number): number {
    const box = track.getBoundingClientRect();
    return view.from + ((clientX - box.left) / box.width) * span;
  }

  // Dragging the playhead. The video preview follows the finger, which is
  // the quickest way to find a moment. One seek per frame is enough, and it
  // keeps a four hour episode moving.
  let scrubbing = $state(false);
  let wanted = 0;
  let queued = 0;

  function seekSoon(t: number) {
    wanted = t;
    if (queued) return;
    queued = requestAnimationFrame(() => {
      queued = 0;
      onseek(wanted);
    });
  }

  function scrub(event: PointerEvent) {
    if (event.button !== 0) return;
    // Shift and a drag marks a stretch to take out instead of moving the
    // playhead. Everything else about the track is unchanged.
    if (event.shiftKey && editable && oncut) {
      event.preventDefault();
      drawCut(event);
      return;
    }
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    scrubbing = true;
    seekSoon(timeAt(event.clientX));
    const move = (e: PointerEvent) => seekSoon(timeAt(e.clientX));
    const up = () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      scrubbing = false;
      cancelAnimationFrame(queued);
      queued = 0;
      onseek(wanted);
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // Pieces as the render will play them, with the edges of a drag applied.
  const pieces = $derived.by(() => {
    const out = segments.map((s) => ({ start: s.start, end: s.end }));
    if (!out.length) return out;
    out[0].start = start;
    out[out.length - 1].end = end;
    return out.filter((p) => p.end > p.start);
  });

  // A cut is a stretch the clip leaves out, which is the gap between two
  // pieces. The engine counts them from the first, and so does the timeline,
  // because that is what a move names.
  //
  // A cut being moved and a cut being drawn are held here and drawn from
  // here until the engine answers, so the block follows the hand rather
  // than jumping when the edit lands. Both are snapped the way the engine
  // snaps, so what is drawn is what will be left out.
  let movingCut = $state<null | { index: number; from: number; to: number }>(null);
  let drawnCut = $state<null | { from: number; to: number }>(null);
  // Kept apart from saving, which belongs to the trim and its own draft.
  let cutSaving = $state(false);
  // The closest two edges of a cut may come, so a drag can never turn a cut
  // inside out.
  const leastCut = 0.05;

  // A cut that is not snapped to words is snapped to the picture instead:
  // a video is cut between frames and nowhere else, so an edge in the
  // middle of one is an edge the render has to round anyway. Rounding here
  // means the number on screen is the number that will be used.
  function onFrame(t: number): number {
    return Math.round(t / frame) * frame;
  }

  const editable = $derived(!!clip && !locked && !saving && !cutSaving);

  // The pieces as they are drawn, with a cut being moved or drawn applied.
  // A cut being drawn takes its stretch out of the pieces at once rather
  // than being a block laid over them, so the wash parts under the hand and
  // the count under the timeline follows the drag. What is happening is
  // shown while it happens.
  const drawnPieces = $derived.by(() => {
    const out = pieces.map((p) => ({ start: p.start, end: p.end }));
    const m = movingCut;
    if (m && m.index >= 0 && m.index + 1 < out.length) {
      out[m.index].end = m.from;
      out[m.index + 1].start = m.to;
    }
    const d = drawnCut;
    if (d) {
      const i = out.findIndex((p) => d.from > p.start && d.to < p.end);
      if (i >= 0) {
        const after = { start: d.to, end: out[i].end };
        out[i] = { start: out[i].start, end: d.from };
        out.splice(i + 1, 0, after);
      }
    }
    return out;
  });

  // The clip from its first piece to its last, cuts and all. It is one
  // clip however many holes are in it, and the rules above and below say
  // so by running the whole way.
  const wholeClip = $derived(
    drawnPieces.length
      ? { start: drawnPieces[0].start, end: drawnPieces[drawnPieces.length - 1].end }
      : null,
  );

  // The cuts as they are drawn, in the order the engine counts them.
  const cuts = $derived(
    drawnPieces
      .slice(1)
      .map((p, i) => ({ index: i, from: drawnPieces[i].end, to: p.start }))
      .filter((c) => c.to > c.from),
  );

  // An edge of a cut is dragged the way a clip edge is, and the other edge
  // stays where it was. What the engine would make of the drag is worked
  // out on the way, so the block shows the words it will really take.
  function grabCut(index: number, side: "from" | "to", event: PointerEvent) {
    if (!editable || !onmovecut) return;
    const was = cuts.find((c) => c.index === index);
    if (!was) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const held = { from: was.from, to: was.to };
    // A cut lives between the two pieces it parts, and neither may be
    // squeezed out of existence. The hand is held inside those walls, so
    // the block never shows a clip the engine is about to refuse.
    const wall = {
      least: (pieces[index]?.start ?? 0) + leastCut,
      most: (pieces[index + 1]?.end ?? duration) - leastCut,
    };
    const startX = event.clientX;
    let moved = false;
    // A cut lands on the frame, because a drag says where and growing it
    // out to the words either side puts it somewhere else. Alt asks for
    // words instead, and it is read on every move rather than at the
    // press, so taking alt back part way through a drag goes back to
    // frames and the block says so before the hand lets go.
    let toWords = event.altKey;
    const move = (e: PointerEvent) => {
      if (!moved && Math.abs(e.clientX - startX) > 2) moved = true;
      if (!moved) return;
      toWords = e.altKey;
      const t = Math.min(Math.max(timeAt(e.clientX), wall.least), wall.most);
      const from = side === "from" ? Math.min(t, held.to - leastCut) : held.from;
      const to = side === "to" ? Math.max(t, held.from + leastCut) : held.to;
      const [a, b] = toWords ? snapCut(words, from, to, keepPause) : [onFrame(from), onFrame(to)];
      movingCut = { index, from: a, to: b };
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      const m = movingCut;
      if (!moved || !m) {
        movingCut = null;
        return;
      }
      if (Math.abs(m.from - held.from) < 0.01 && Math.abs(m.to - held.to) < 0.01) {
        movingCut = null;
        return;
      }
      cutSaving = true;
      try {
        undone = null;
        await onmovecut?.(m.index, m.from, m.to, toWords);
      } finally {
        cutSaving = false;
        movingCut = null;
      }
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // Drawing a cut is a drag across the clip with shift held, which is how
  // a stretch is marked in an editing timeline. Without shift the same drag
  // moves the playhead, so nothing that worked before works differently.
  function drawCut(event: PointerEvent) {
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const startX = event.clientX;
    const from = timeAt(startX);
    let moved = false;
    let toWords = event.altKey;
    const move = (e: PointerEvent) => {
      if (!moved && Math.abs(e.clientX - startX) > 2) moved = true;
      if (!moved) return;
      toWords = e.altKey;
      const t = timeAt(e.clientX);
      const near = Math.min(from, t);
      const far = Math.max(from, t);
      // Snapping grows a cut to whole words, so it is never too short for
      // the engine. Frames do not, and a cut of nothing would be refused
      // with the plan untouched and a message for a drag of two pixels. So
      // it is held open at the least a cut may be, the way the walls hold
      // an edge that is being moved. This is the usual way round now, not
      // the exception.
      const [a, b] = toWords
        ? snapCut(words, near, far, keepPause)
        : [onFrame(near), Math.max(onFrame(far), onFrame(near) + leastCut)];
      drawnCut = { from: a, to: b };
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      const d = drawnCut;
      if (!moved || !d) {
        drawnCut = null;
        return;
      }
      cutSaving = true;
      try {
        undone = null;
        await oncut?.(d.from, d.to, toWords);
      } finally {
        cutSaving = false;
        drawnCut = null;
      }
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

  // Shift and a double-click takes a stretch out where you click, the way
  // shift and a drag takes out the stretch you drag across. Shift is the
  // cutting hand on this track either way. Where it goes exactly, and
  // whether it goes at all, is cutAt in lib/api.ts, which has the tests.
  async function cutHere(at: number, wide: number) {
    if (!editable || !oncut) return;
    const where = cutAt(pieces, at, wide, leastCut, frame);
    if (!where) return;
    cutSaving = true;
    try {
      undone = null;
      await oncut(where[0], where[1], false);
    } finally {
      cutSaving = false;
    }
  }

  // The cut that was last put back, so the same double-click in the same
  // place can put it in again. Taking a stretch out is one double-click,
  // and nothing that takes one click may cost more than one to undo. It
  // belongs to the clip it was in, and it is forgotten the moment anything
  // else about that clip's cuts changes, because a stretch put back into a
  // clip that has moved on is not the stretch that was taken out.
  let undone = $state<{ key: string; from: number; to: number } | null>(null);

  // A double-click puts a cut back, the way a double-click undoes an edit
  // point in an editing timeline. It is not a single click, because a cut
  // is easy to land on by accident while scrubbing.
  async function putCutBack(index: number, event: MouseEvent) {
    event.preventDefault();
    if (!editable || !onjoincut) return;
    const cut = cuts.find((c) => c.index === index);
    if (!cut) return;
    cutSaving = true;
    try {
      await onjoincut((cut.from + cut.to) / 2);
      undone = clip ? { key: clip.key, from: cut.from, to: cut.to } : null;
    } finally {
      cutSaving = false;
    }
  }

  // Putting back what was just put back. The stretch is taken out again
  // exactly as it was, edge for edge, so it is sent as it stands rather
  // than snapped afresh: snapping it again would be snapping something
  // already snapped, and on a cut made a frame at a time it would move.
  async function cutAgain(was: { from: number; to: number }) {
    if (!editable || !oncut) return;
    undone = null;
    cutSaving = true;
    try {
      await oncut(was.from, was.to, false);
    } finally {
      cutSaving = false;
    }
  }

  // A view is read with as much again either side of it, so swiping along
  // the episode moves through what is already in hand. The waveform is drawn
  // by time, so what is read and what is shown need not line up.
  //
  // What is drawn and the stretch it was read for are set in the same
  // breath, or the old readings would be drawn against the new stretch for
  // as long as the reading takes, which looks like the waveform jumping
  // about. A reading that comes back after a newer one is dropped.
  let latest = 0;

  async function load(from: number, to: number) {
    view = { from, to };
    loaded = true;
    loadedTo = covered;
    const shown = Math.max(to - from, 0.001);
    const whole = Math.max(duration, to);
    const outer = { from: Math.max(0, from - shown), to: Math.min(whole, to + shown) };
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
      words = w.words ?? [];
      keepPause = w.keepPause;
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
  function fitView(event?: MouseEvent) {
    // Shift is the cutting hand on this track, so a double-click with it
    // held takes a stretch out where it lands and never moves the view. It
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
    if (event && editable && onjoincut && track) {
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
      viewFor = clip.key;
      load(Math.max(0, a - pad), Math.min(duration, b + pad));
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
    const walk = lit.length && insideClip(pieces, time, step) ? lit : words;
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
      const a = clip.segments[0].start;
      const b = clip.segments[clip.segments.length - 1].end;
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

  // An episode being transcribed for the first time has no words and no
  // waveform yet. They arrive piece by piece, so a view that was read before
  // the transcript reached it is read again.
  $effect(() => {
    if (!loaded || covered <= loadedTo + 0.5 || loadedTo >= view.to) return;
    load(view.from, view.to);
  });

  // Where the waveform stops. The readings come from the transcript, so
  // what comes after it is exactly silent. Taking the edge from the same
  // readings the waveform is drawn from means the part that waits can
  // never lie over a waveform that is already there, and never leave a
  // stretch with neither.
  const soundEdge = $derived.by(() => {
    if (!peaks.length) return data.from;
    let last = -1;
    for (let i = peaks.length - 1; i >= 0; i--) {
      if (peaks[i] > -89.5) {
        last = i;
        break;
      }
    }
    if (last < 0) return data.from;
    return data.from + ((last + 1) * (data.to - data.from)) / peaks.length;
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
    ctx.fillStyle = getComputedStyle(canvas).getPropertyValue("--wave").trim() || "#6b7080";

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

  // An edge is dragged to trim. Clicking one without dragging puts the
  // playhead exactly on it, which is how you start a clip over.
  function grab(edge: "start" | "end", event: PointerEvent) {
    if (!clip || locked || saving) return;
    event.preventDefault();
    event.stopPropagation();
    const target = event.currentTarget as HTMLElement;
    target.setPointerCapture(event.pointerId);
    const from = event.clientX;
    draft = { start, end };
    let moved = false;
    const move = (e: PointerEvent) => {
      if (!moved && Math.abs(e.clientX - from) > 2) {
        moved = true;
        if (ontrim) dragging = edge;
      }
      if (!moved || !dragging) return;
      const t = timeAt(e.clientX);
      if (edge === "start") draft.start = Math.min(snapStart(words, t, keepPause), draft.end - 1);
      else draft.end = Math.max(snapEnd(words, t, keepPause), draft.start + 1);
    };
    const up = async () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      const dragged = !!dragging;
      dragging = null;
      if (!dragged) {
        onseek(edge === "start" ? first : last);
        return;
      }
      if (Math.abs(draft.start - first) < 0.01 && Math.abs(draft.end - last) < 0.01) return;
      saving = true;
      try {
        // A clip whose edges have moved is not the clip the stretch was
        // taken out of, so there is nothing to put back any more.
        undone = null;
        await ontrim?.(draft.start, draft.end);
        onseek(draft.start);
      } finally {
        saving = false;
      }
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  }

  // Where in the episode this is. Without them a swipe leaves you nowhere,
  // and the step is round and wide enough that the labels never crowd.
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
        a clip edge to trim it. The words are in the picture, in the caption box, which is where
        they are read and where they are corrected. A hatched block inside a clip is
        a stretch it leaves out. Drag either edge of one to change it, double-click one to put it
        back, and double-click again to take it out once more. Shift is the cutting hand: hold it
        and drag across the clip to take out the stretch you drag over, or hold it and double-click
        to take one out where you click. Cuts land on the frame. Hold alt as well to land on whole
        words instead, which takes the whole pause a cut falls in.
      </Info>
    </span>
    <!-- Nothing to draw yet, so the track says the words are on their way
         rather than looking broken. -->
    {#if working && !peaks.length}
      <div class="asleep waiting"></div>
    {:else if working && soundEdge < view.to}
      <!-- What the transcript has not reached yet is the part that waits,
           the same as on the range picker. It starts where the waveform
           ends, so the two never lie over each other. -->
      <div class="asleep waiting" style="left: {Math.max(x(soundEdge), 0)}%; right: 0"></div>
    {/if}
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
        class="span"
        style="left: {x(wholeClip.start)}%; width: {x(wholeClip.end) - x(wholeClip.start)}%"
      ></div>
    {/if}
    {#each drawnPieces as p, i (i)}
      <div class="piece" style="left: {x(p.start)}%; width: {x(p.end) - x(p.start)}%"></div>
    {/each}
    <!-- A cut is drawn over the pieces rather than between them, so a cut
         being dragged wider is seen taking the piece rather than waiting
         for the piece to give way. -->
    {#each cuts as c (c.index)}
      <div
        class="cut"
        class:editable
        class:drawing={!!drawnCut && Math.abs(c.from - drawnCut.from) < 0.001}
        style="left: {x(c.from)}%; width: {x(c.to) - x(c.from)}%"
        title={editable
          ? "A stretch the clip leaves out. Drag an edge to change it, double-click to put it back."
          : "A stretch the clip leaves out."}
      ></div>
    {/each}
    {#if editable && onmovecut}
      <!-- The handles come after every cut, so a handle is never drawn
           under the next cut's block. -->
      {#each cuts as c (c.index)}
        <div
          class="cutedge"
          class:active={movingCut?.index === c.index}
          style="left: {x(c.from)}%"
          role="slider"
          tabindex="-1"
          aria-label="Where cut {c.index + 1} starts"
          aria-valuenow={c.from}
          onpointerdown={(e) => grabCut(c.index, "from", e)}
        ></div>
        <div
          class="cutedge"
          class:active={movingCut?.index === c.index}
          style="left: {x(c.to)}%"
          role="slider"
          tabindex="-1"
          aria-label="Where cut {c.index + 1} ends"
          aria-valuenow={c.to}
          onpointerdown={(e) => grabCut(c.index, "to", e)}
        ></div>
      {/each}
    {/if}
    {#if clip && !locked}
      <div
        class="edge"
        class:active={dragging === "start"}
        style="left: {x(start)}%"
        role="slider"
        tabindex="-1"
        aria-label="Clip start"
        aria-valuenow={start}
        onpointerdown={(e) => grab("start", e)}
      ></div>
      <div
        class="edge"
        class:active={dragging === "end"}
        style="left: {x(end)}%"
        role="slider"
        tabindex="-1"
        aria-label="Clip end"
        aria-valuenow={end}
        onpointerdown={(e) => grab("end", e)}
      ></div>
    {/if}
  </div>
    {#if time >= view.from && time <= view.to}
      <div class="at" style="left: {x(time)}%">
        <div class="playhead"></div>
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
    --wave: #555a66;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  /* The box the playhead is drawn over, exactly the track and nothing
     more, so a position worked out for the track is right here too. */
  .over {
    position: relative;
  }

  /* Only there once the timeline was moved by hand, which is the one moment
     a way back is worth a control. It lies over the track, the way the
     transcription note lies over the range picker, so no row changes
     height as it comes and goes. */
  .asleep {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    left: 0;
    background: rgba(255, 255, 255, 0.03);
    pointer-events: none;
    z-index: 1;
  }

  .track {
    position: relative;
    /* As tall as the window allows, set by the workspace. */
    height: var(--wave-h, 112px);
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
    border-top: 2px solid var(--accent);
    border-bottom: 2px solid var(--accent);
    pointer-events: none;
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

  /* A stretch the clip leaves out. It is the track's own background and
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
    z-index: 2;
  }

  .edge::after {
    content: "";
    position: absolute;
    left: 5px;
    top: 0;
    bottom: 0;
    width: 2px;
    background: var(--accent);
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
  .playhead {
    position: absolute;
    top: -5px;
    bottom: 0;
    left: -1px;
    width: 2px;
    background: var(--accent-hi);
    pointer-events: none;
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
    cursor: pointer;
    touch-action: none;
  }

  .over.scrubbing .playhead,
  .over.scrubbing .head {
    background: #fff;
  }

  .over.scrubbing .head {
    cursor: grabbing;
  }
</style>
