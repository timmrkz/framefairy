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
  import { api, clock, snapEnd, snapStart, type ClipEntry, type Word } from "../lib/api";
  import Icon from "./Icon.svelte";
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
    onseek,
    ontrim,
    onword,
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
    onseek: (t: number) => void;
    ontrim?: (start: number, end: number) => Promise<void>;
    onword?: (start: number, text: string) => Promise<void>;
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
      seconds: Math.round(pieces.reduce((sum, p) => sum + p.end - p.start, 0)),
      pieces: pieces.length,
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
    // The words are wanted around the playhead, for the lens and for
    // snapping an edge. Zoomed out to a four hour episode that would be
    // forty thousand of them for every swipe, so they are asked for by the
    // ten minutes around where the work is.
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
    const whole = Math.max(duration, loose);
    const from = Math.max(0, Math.min(at - loose * 0.25, whole - loose));
    load(from, Math.min(from + loose, whole));
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
    const whole = Math.max(duration, loose);
    const from = Math.max(0, Math.min(time - loose / 2, whole - loose));
    load(from, Math.min(from + loose, whole));
  }

  // Where the timeline sits when nobody has moved it: the clip with a little
  // room around it, or the minute around the playhead.
  function fitView() {
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
  // takes a second at a time.
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
    const step = (event.shiftKey ? 1 : Math.max(frame, 1 / 240)) * (event.key === "ArrowLeft" ? -1 : 1);
    event.preventDefault();
    onseek(Math.max(0, Math.min(time + step, duration)));
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
      const shown = span * Math.exp(event.deltaY * 0.01);
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
  // screen per column of the picture, each the loudest reading that falls
  // in it, each a whole pixel wide and a whole pixel tall.
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

    // The loudest reading in each column, in decibels. Nothing at all where
    // no reading falls, so a column past the end of the transcript stays
    // empty rather than reading as silence.
    const loudest = new Float32Array(w).fill(-Infinity);
    const step = (data.to - data.from) / Math.max(peaks.length, 1);
    const scale = w / span;
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

  // The lens is not on screen until it is asked for: the magnifier on the
  // playhead shows the words, and a second click takes them away again. The
  // magnifier is a button and nothing else, so using it never moves the
  // playhead.
  let lensOpen = $state(false);

  let editing = $state<number | null>(null);
  let editText = $state("");
  let frozen = $state(0);

  // The lens can only ever show what has been heard. While an episode is
  // still being transcribed the playhead goes anywhere, and past the end of
  // the transcript there are no words to magnify, so the magnifier is off
  // there rather than opening on nothing.
  const heard = $derived(covered > 0 && time <= covered + 0.5);

  const lensShown = $derived((lensOpen && heard) || editing !== null);

  // Escape takes the lens away too, the same as a second click on the
  // magnifier. It only ever closes the lens when the lens is the top thing
  // on screen: a word being corrected, a box asking something and an info
  // bubble all answer Escape themselves and keep it.
  $effect(() => {
    if (!lensOpen) return;
    const key = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return;
      const on = document.activeElement as HTMLElement | null;
      const tag = on?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || on?.isContentEditable) return;
      if (document.querySelector("dialog[open]") || document.querySelector(".bubble")) return;
      lensOpen = false;
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  });

  // The lens: a pill that rides the playhead and shows the words around it
  // big enough to read and to correct. It is the only place the words are
  // spelled out, so there is no wall of text under the timeline.
  const lensWidth = 340;

  // The words are laid out in a row of their own, each as wide as it really
  // is, and the row is shifted so that the word being spoken sits in the
  // middle of the pill.
  let ruler: CanvasRenderingContext2D | null = null;

  function textWidth(text: string): number {
    if (!ruler && track) {
      const style = getComputedStyle(track);
      ruler = document.createElement("canvas").getContext("2d");
      if (ruler) ruler.font = `${style.fontSize} ${style.fontFamily}`;
    }
    return ruler ? ruler.measureText(text).width : text.length * 7.2;
  }

  const laid = $derived.by(() => {
    let at = 0;
    return words.map((w) => {
      const size = Math.round(textWidth(w.text)) + 10;
      const item = { word: w, left: at, size };
      at += size + 6;
      return item;
    });
  });

  // Where the lens is reading, in that row. It walks from one word to the
  // next while the word is spoken, so the row glides instead of jumping.
  const centre = $derived(editing !== null ? frozen : time);

  const reading = $derived.by(() => {
    if (!laid.length) return 0;
    const index = laid.findIndex((it) => centre < it.word.end);
    if (index < 0) {
      const last = laid[laid.length - 1];
      return last.left + last.size / 2;
    }
    const here = laid[index];
    const next = laid[index + 1];
    const from = Math.max(here.word.start, index > 0 ? laid[index - 1].word.end : 0);
    const to = next ? next.word.start : here.word.end;
    const part = Math.max(0, Math.min(1, (centre - from) / Math.max(to - from, 0.001)));
    const a = here.left + here.size / 2;
    const b = next ? next.left + next.size / 2 : a;
    return a + (b - a) * part;
  });

  // Only what fits in the pill, plus a little either side.
  const lensWords = $derived(
    laid
      .map((it) => ({ ...it, at: it.left - reading + lensWidth / 2 }))
      .filter((it) => it.at > -it.size - 8 && it.at < lensWidth + 8),
  );

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

  function beginEdit(w: Word) {
    // A correction belongs to a clip, so without one the words are only
    // there to read.
    if (!clip || !onword || locked) return;
    frozen = time;
    editing = w.start;
    editText = w.text;
  }

  async function commitEdit() {
    if (editing === null) return;
    const at = editing;
    const original = words.find((w) => w.start === at)?.text;
    const text = editText.trim();
    editing = null;
    if (!text || text === original) return;
    await onword?.(at, text);
    const w = words.find((w) => w.start === at);
    if (w) w.text = text;
  }

  function editKey(e: KeyboardEvent) {
    if (e.key === "Enter") {
      e.preventDefault();
      commitEdit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      editing = null;
    }
  }

  function focus(node: HTMLInputElement) {
    node.focus();
    node.select();
  }
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
        and a double-click to fit the clip. The arrow keys step a frame, with shift a second. Drag
        a clip edge to trim it, and the magnifier shows the words. A hatched block inside a clip is
        dead air the engine cut out.
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
    {#each pieces as p, i (i)}
      <div class="piece" style="left: {x(p.start)}%; width: {x(p.end) - x(p.start)}%"></div>
      {#if i > 0}
        <div class="cut" style="left: {x(pieces[i - 1].end)}%; width: {x(p.start) - x(pieces[i - 1].end)}%"></div>
      {/if}
    {/each}
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
      <div class="at" class:shown={lensShown} style="left: {x(centre)}%">
        <div class="playhead"></div>
        <button
          class="grip"
          aria-label="The words at the playhead"
          aria-expanded={lensShown}
          disabled={!heard}
          title={heard
            ? lensShown
              ? "Hide the words"
              : "The words spoken here"
            : "The transcript does not reach this far yet"}
          onpointerdown={(e) => e.stopPropagation()}
          onclick={() => (lensOpen = !lensOpen)}
        >
          <Icon name="lens" size={13} />
        </button>
        {#if lensWords.length}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div class="lens" onpointerdown={(e) => e.stopPropagation()}>
            {#each lensWords as it (it.word.start)}
              {#if editing === it.word.start}
                <input
                  class="edit"
                  type="text"
                  bind:value={editText}
                  use:focus
                  onkeydown={editKey}
                  onblur={commitEdit}
                  style="left: {it.at}px; width: {Math.max(editText.length, 3) + 2}ch"
                  aria-label="Correct the word"
                />
              {:else}
                <button
                  class="word"
                  class:now={time >= it.word.start && time < it.word.end}
                  class:plain={!clip}
                  style="left: {it.at}px; width: {it.size}px"
                  disabled={locked || !clip}
                  title={clip
                    ? "Correct this word. Two words split it in two"
                    : "Pick a clip to correct its words"}
                  onclick={() => beginEdit(it.word)}>{it.word.text}</button
                >
              {/if}
            {/each}
          </div>
        {/if}
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

  .piece {
    position: absolute;
    top: 0;
    bottom: 0;
    background: var(--accent-wash);
    border-top: 2px solid var(--accent);
    border-bottom: 2px solid var(--accent);
    pointer-events: none;
  }

  .cut {
    position: absolute;
    top: 0;
    bottom: 0;
    background: repeating-linear-gradient(135deg, transparent 0 5px, rgba(255, 255, 255, 0.06) 5px 10px);
    pointer-events: none;
  }

  /* The playhead, with the grip that holds the words. */
  .at {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 0;
    z-index: 3;
  }

  /* Under the head of the playhead, not over it: the head says where the
     playhead is and the magnifier says what is said there, and neither
     should be drawn across the other. */
  .grip {
    position: absolute;
    top: 10px;
    left: -11px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 20px;
    padding: 0;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--ink-2);
    color: var(--muted);
    cursor: pointer;
    z-index: 5;
  }

  .at.shown .grip {
    background: var(--ink-3);
    color: var(--text);
    border-color: var(--accent);
  }

  /* Past the end of the transcript there is nothing to magnify, so the
     magnifier says so by looking spent rather than by opening on nothing. */
  .grip:disabled {
    color: var(--ink-3);
    border-color: var(--ink-3);
    cursor: default;
  }

  /* The lens magnifies the words around the playhead, which is where they
     are corrected. It is not there until it is reached for. */
  .lens {
    position: absolute;
    top: 27px;
    left: -170px;
    width: 340px;
    height: 30px;
    background: var(--ink-2);
    border: 1px solid var(--line);
    border-radius: 15px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.45);
    overflow: hidden;
    z-index: 4;
    opacity: 0;
    visibility: hidden;
    pointer-events: none;
  }

  .at.shown .lens {
    opacity: 1;
    visibility: visible;
    pointer-events: auto;
  }

  .lens::after {
    content: "";
    position: absolute;
    left: 50%;
    top: 0;
    bottom: 0;
    width: 1px;
    background: var(--line);
    pointer-events: none;
  }

  .word,
  .edit {
    position: absolute;
    top: 3px;
    height: 22px;
    padding: 0 5px;
    font-size: var(--size-m);
    line-height: 20px;
    border-radius: var(--radius-s);
    text-align: center;
    white-space: nowrap;
    z-index: 1;
  }

  .word {
    border: 1px solid transparent;
    background: transparent;
    cursor: text;
    overflow: hidden;
  }

  /* Without a clip the words are only there to read, so they do not look
     like something to click. */
  .word.plain:disabled {
    opacity: 1;
  }

  .word:hover:not(:disabled) {
    background: var(--ink-3);
    border-color: var(--line);
  }

  .word.now {
    color: var(--accent-hi);
    font-weight: 600;
  }

  .edit {
    border: 1px solid var(--accent);
    background: var(--ink-1);
    width: auto;
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

  .playhead::before {
    content: "";
    position: absolute;
    top: 0;
    left: -4px;
    width: 10px;
    height: 9px;
    border-radius: 2px 2px 1px 1px;
    background: var(--accent-hi);
  }

  .over.scrubbing .playhead,
  .over.scrubbing .playhead::before {
    background: #fff;
  }
</style>
