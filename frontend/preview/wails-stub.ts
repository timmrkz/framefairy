// Stands in for the Wails runtime so the interface can be looked at in a
// plain browser. Only for taking a picture of the layout.
import noticeList from "../../notices/notices.json";

const noticeTexts = import.meta.glob("../../notices/texts/*.txt", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

// The frame rate of the episode the harness plays, see open.mjs.
const harnessFps = 25;

// The words of the whole episode, on one clock, made once.
//
// They used to be made from wherever a call asked to start, so a call
// about the ten minutes around the playhead and a call about a clip's
// twenty-five seconds answered with words at different moments. Nothing
// looked wrong: both lists read the same sentence and both drew fine. But
// the Go side reads one transcript, so the word a clip is built from and
// the word the timeline steps to are the same word, and here they were
// not. Stepping by words lit the wrong word in the caption box and it
// took a probe to see, because the two lists only disagree once something
// crosses from one to the other.
let every: { start: number; end: number; text: string }[] | null = null;

const allWords = () => {
  if (every) return every;
  const sample = "Und da war irgendein Typ auf einmal vor mir und ich habe mich gewehrt weil das ein echtes Thema war".split(" ");
  const list: { start: number; end: number; text: string }[] = [];
  let at = 0;
  let i = 0;
  while (at < 14423) {
    const len = 0.28 + (i % 5) * 0.08;
    list.push({ start: at, end: at + len, text: sample[i % sample.length] });
    at += len + 0.06;
    i++;
  }
  every = list;
  return list;
};

// Whether a piece holds some of a word's sound, the engine's HoldsWord:
// more than a frame of it.
const holds = (p: { start: number; end: number }, w: { start: number; end: number }) =>
  Math.min(p.end, w.end) - Math.max(p.start, w.start) > 0.02;

// The words of the episode, for a probe that has to name one by when it is
// said, to correct it or to find it on the clip timeline.
(window as any).__allWords = () => allWords();

const words = (from: number, to: number) =>
  allWords().filter((w) => w.end > from && w.start < to);

type Piece = { start: number; end: number; cropX: number; moved: boolean };

// A clip's pieces, once a cut has changed them. The Go side keeps them in
// the plan, so the preview keeps them here, or a cut would come undone the
// moment the clip list is read again.
const held = (): Record<string, Piece[]> => ((window as any).__pieces ??= {});

// The corrections, kept the same way the cuts are: the Go side writes them
// down for the whole episode, so a word corrected once stays corrected
// however often the clip is read again.
const fixed = (): Record<string, string> => ((window as any).__fixed ??= {});
const said = (start: number) => start.toFixed(3);

const pieces = (n: number, start: number): Piece[] =>
  held()[`0${n}`] ?? [
    { start, end: start + 12, cropX: 420, moved: false },
    { start: start + 13, end: start + 25, cropX: 420, moved: false },
  ];

// The same snapping the engine does, so what the preview gives back is what
// the app would really be handed: a cut swallows every word it touches and
// then leaves a tenth of a second of air on each side that stays.
const snapCut = (list: { start: number; end: number }[], from: number, to: number): [number, number] => {
  const keepPause = 0.1;
  let swallowedFrom = Infinity;
  let swallowedTo = -Infinity;
  for (const w of list) {
    if (w.end > from && w.start < to) {
      swallowedFrom = Math.min(swallowedFrom, w.start);
      swallowedTo = Math.max(swallowedTo, w.end);
    }
  }
  let a = Math.min(from, swallowedFrom);
  let b = Math.max(to, swallowedTo);
  let before = -Infinity;
  let after = Infinity;
  for (const w of list) {
    if (w.end <= a) before = Math.max(before, w.end);
    if (w.start >= b) after = Math.min(after, w.start);
  }
  if (before !== -Infinity) a = Math.min(before + keepPause, swallowedFrom);
  if (after !== Infinity) b = Math.max(after - keepPause, swallowedTo);
  return [Math.max(0, a), b];
};

// Taking a part out of a set of pieces. A piece the cut straddles becomes
// two, and both keep the framing, exactly as the engine does it.
const applyCut = (list: Piece[], from: number, to: number): Piece[] => {
  const out: Piece[] = [];
  for (const p of list) {
    if (from <= p.start && to >= p.end) continue;
    if (to <= p.start || from >= p.end) {
      out.push(p);
      continue;
    }
    if (from > p.start) out.push({ ...p, end: from });
    if (to < p.end) out.push({ ...p, start: to });
  }
  return out;
};

const clip = (n: number, start: number, title: string, rendered: boolean) => {
  const segments = pieces(n, start);
  const first = segments.length ? segments[0].start : start;
  const last = segments.length ? segments[segments.length - 1].end : start;
  return {
    id: `0${n}`,
    slug: `clip-${n}`,
    basename: `0${n}_clip-${n}`,
    title,
    reason: "A short, complete memory of defending oneself using Judo.",
    duration: segments.reduce((sum, p) => sum + p.end - p.start, 0),
    start: first,
    end: last,
    segments,
    rejected: false,
    rendered: rendered ? "/tmp/out.mp4" : undefined,
    captionY: 300,
    captionYMoved: false,
    // Only the moments inside a kept piece, the way the engine reads them.
    thumbnails: (((window as any).__thumbs ??= {})[`0${n}`] ?? [])
      .filter((t: number) => segments.some((p) => t >= p.start && t < p.end))
      .sort((a: number, b: number) => a - b),
    // Where the edges were before anything changed them, kept the first
    // time anything does, the way the engine's editPieces keeps them.
    found: (((window as any).__found ??= {})[`0${n}`] ?? [first, last]) as [number, number],
    key: `clips.json/0${n}`,
    plan: "/eps/ep.framefairy/logs/clips.json",
    cropLefts: segments.map((p) => p.cropX),
  };
};

// Where each clip of the ordinary mode starts, so a cut can find the clip
// it was asked about and give the same one back.
const starts: Record<string, [number, string, boolean]> = {
  "01": [57, "Mein Arm ist zersprungen", true],
  // ?overlap puts the second clip over the end of the first, the way two
  // clips of a short episode can lie over each other.
  "02": [location.search.includes("overlap") ? 75 : 400, "Der Typ vor mir auf einmal", false],
  "03": [902, "Warum ich nie wieder", false],
  "04": [1400, "Ein echtes Thema", false],
};

type StubGesture = {
  kind: string;
  edge: string;
  index: number;
  from: number;
  to: number;
  toWords: boolean;
};

// The stand-in for engine/shape.go, see the Shape case.
const gestured = (now: Piece[], g: StubGesture, at: number): { pieces: Piece[]; playhead: number } | null => {
  // The episode's frames, as the Go side puts every edge on them.
  const frame = 1 / harnessFps;
  const on = (t: number) => Math.round(t / frame) * frame;
  const said = words(at - 60, at + 90);
  const least = 0.05;
  if (!now.length) return null;
  if (g.kind === "trim") {
    let start = now[0].start;
    let end = now[now.length - 1].end;
    let playhead = -1;
    if (g.edge === "start") {
      const w = said.reduce((b, x) => (Math.abs(x.start - g.from) < Math.abs(b.start - g.from) ? x : b), said[0]);
      start = g.toWords && w ? Math.max(w.start - 0.1, said[said.indexOf(w) - 1]?.end ?? 0) : Math.max(0, on(g.from));
      start = Math.min(start, end - 1);
      playhead = g.toWords && w ? Math.min(w.start + frame, (w.start + w.end) / 2) : start;
    } else {
      const w = said.reduce((b, x) => (Math.abs(x.end - g.from) < Math.abs(b.end - g.from) ? x : b), said[0]);
      end = g.toWords && w ? Math.min(w.end + 0.1, said[said.indexOf(w) + 1]?.start ?? Infinity) : on(g.from);
      end = Math.max(end, start + 1);
      playhead = g.toWords && w ? Math.max(w.end - frame, (w.start + w.end) / 2) : Math.max(end - frame, start);
    }
    const kept = now.filter((p) => p.end > start && p.start < end).map((p) => ({ ...p }));
    if (!kept.length) return null;
    kept[0].start = start;
    kept[kept.length - 1].end = end;
    return { pieces: kept, playhead };
  }
  if (g.kind === "cut" || g.kind === "restore") {
    const near = Math.min(g.from, g.to);
    const far = Math.max(g.from, g.to);
    const [a, b] =
      g.kind === "restore"
        ? [near, far]
        : g.toWords
          ? snapCut(said, near, far)
          : [on(near), Math.max(on(far), on(near) + least)];
    const out = applyCut(now, a, b);
    return { pieces: out, playhead: -1 };
  }
  if (g.kind === "move") {
    const i = g.index;
    if (i < 0 || i + 1 >= now.length) return null;
    let [a, b] = g.toWords ? snapCut(said, g.from, g.to) : [on(g.from), on(g.to)];
    // Moving one edge never moves the other, the engine's rule.
    if (g.edge === "from") {
      b = now[i + 1].start;
      a = Math.min(Math.max(a, now[i].start + least), b - least);
    } else if (g.edge === "to") {
      a = now[i].end;
      b = Math.max(Math.min(b, now[i + 1].end - least), a + least);
    } else {
      a = Math.min(Math.max(a, now[i].start + least), now[i + 1].end - 2 * least);
      b = Math.max(Math.min(b, now[i + 1].end - least), a + least);
    }
    const out = now.map((p) => ({ ...p }));
    out[i].end = a;
    out[i + 1].start = b;
    return { pieces: out, playhead: -1 };
  }
  if (g.kind === "join") {
    for (let i = 0; i + 1 < now.length; i++) {
      if (g.from >= now[i].end && g.from <= now[i + 1].start) {
        const joined = { ...now[i], end: now[i + 1].end };
        return { pieces: [...now.slice(0, i), joined, ...now.slice(i + 2)], playhead: -1 };
      }
    }
    return null;
  }
  return null;
};

const recut = (id: string, change: (list: Piece[]) => Piece[]) => {
  const n = Number(id);
  const [at, title, rendered] = starts[id] ?? [60, "Clip", false];
  held()[id] = change(pieces(n, at));
  return clip(n, at, title, rendered);
};

// The caption look, as far as anything can change it here: the face and the
// size the interface asked for last. Without these the interface could ask
// for a face all day and always be told Inter Black, so a probe about
// picking one would pass whatever the picking did.
const face = () => (window as any).__face ?? "Inter Black";
const size = () => (window as any).__size ?? 100;
// The caption colours, reported the way the engine reports them: the text
// opaque white and the box black and half clear until they are changed.
const textCss = () => (window as any).__text ?? "rgba(255, 255, 255, 1)";
const boxCss = () => (window as any).__box ?? "rgba(0, 0, 0, 0.498)";
const pillCss = () => (window as any).__pill ?? "rgba(148, 33, 146, 1)";
const hexToCss = (hex: string, alpha: number) =>
  `rgba(${parseInt(hex.slice(1, 3), 16)}, ${parseInt(hex.slice(3, 5), 16)}, ${parseInt(hex.slice(5, 7), 16)}, ${Math.round(alpha * 255) / 255})`;

// The clip a call names, whatever has been done to it since.
const clipOf = (id: string) => {
  const [at, title, rendered] = starts[id] ?? [60, "Clip", false];
  return clip(Number(id), at, title, rendered);
};

// The captions of a clip, built the way the engine builds them, because
// the caption box is where words are corrected and a word in it has to be
// able to say which word of the episode it is. Made up cues could never
// answer that, so a probe about correcting a word would pass whatever the
// interface did.
//
// Two things the engine does and this does with it: the words are put on
// the clip's own clock, with the cuts taken out of it, and a correction
// that reads as two words is drawn as two, each taking its share of the
// one moment they both came from.
const captionCues = (id: string, draft?: { start: number; end: number }[]) => {
  // With pieces being dragged on the clip timeline, the captions are of
  // those, the way the engine's ShapeClipView makes them.
  const segments = draft ?? clipOf(id).segments;
  return cuesOf(segments, id);
};
// The captions of any pieces, a saved clip's, a dragged one's or one on
// its way. A clip keeps no words of its own. What it says is read off the
// episode's words, corrected, the way the engine's Said reads it.
const cuesOf = (segments: { start: number; end: number }[], id = "") => {
  const c = {
    segments,
    words: allWords()
      .filter((w) => segments.some((p) => holds(p, w)))
      .map((w) => ({ ...w, text: fixed()[said(w.start)] ?? w.text }))
      // A word corrected to nothing is removed, the way Transcript.Correct
      // leaves it out.
      .filter((w) => w.text !== ""),
  };
  // Each word also keeps when it starts in the episode, which is what a
  // caption moved by hand is kept against.
  const onClipClock: { start: number; end: number; text: string; said: number; whole: string; part?: number }[] = [];
  // The engine's ClipWords: a word is captioned in the piece that holds
  // the most of it, from the edge on when an edge cuts into it.
  const offsets: number[] = [];
  let sum = 0;
  for (const p of c.segments) {
    offsets.push(sum);
    sum += p.end - p.start;
  }
  for (const w of c.words) {
    let best = -1;
    let most = 0;
    c.segments.forEach((p, i) => {
      const held = Math.min(p.end, w.end) - Math.max(p.start, w.start);
      if (holds(p, w) && held > most) {
        best = i;
        most = held;
      }
    });
    if (best < 0) continue;
    // Clamped only at a cut, never at the clip's first or last edge, the
    // way the engine keeps it.
    const p = c.segments[best];
    const from = best > 0 ? Math.max(w.start, p.start) : w.start;
    const to = best < c.segments.length - 1 ? Math.min(w.end, p.end) : w.end;
    onClipClock.push({
      start: offsets[best] + (from - p.start),
      end: offsets[best] + (to - p.start),
      text: w.text,
      said: w.start,
      whole: w.text,
    });
  }
  onClipClock.sort((x, y) => x.start - y.start);
  const drawn: typeof onClipClock = [];
  for (const w of onClipClock) {
    const parts = w.text.split(" ").filter(Boolean);
    if (parts.length < 2) {
      drawn.push(w);
      continue;
    }
    const letters = parts.reduce((n, part) => n + part.length, 0);
    let from = w.start;
    parts.forEach((part, i) => {
      const to =
        i === parts.length - 1 ? w.end : from + ((w.end - w.start) * part.length) / letters;
      drawn.push({ start: from, end: to, text: part, said: w.said, whole: w.whole, part: i });
      from = to;
    });
  }
  // Eight words to a cue in two lines of four, which is near enough to
  // what the engine's line breaking gives for a box to be looked at.
  const cues = [];
  for (let i = 0; i < drawn.length; i += 8) {
    const eight = drawn.slice(i, i + 8);
    // A cue stays up a little past its last word, and never past the start
    // of the one after it. The engine clamps it the same way, and without
    // the clamp two cues cover the same moment: the interface takes the first
    // that covers it, so the caption box went on showing the cue before
    // while the playhead stood in a word of the cue after, and that word
    // lit nothing at all.
    const next = drawn[i + 8];
    const first = eight[0].said;
    const last = eight[eight.length - 1].said;
    cues.push({
      start: eight[0].start,
      end: Math.min(eight[eight.length - 1].end + 0.4, next ? next.start : Infinity),
      lines: [{ words: eight.slice(0, 4) }, { words: eight.slice(4) }].filter(
        (line) => line.words.length > 0,
      ),
      first,
      last,
      startMoved: false,
      endMoved: false,
    });
  }
  // Captions moved by hand, put where they were put. The engine's rules on
  // clamping are its own and are tested there. This is enough for a probe
  // to see a moved caption come back moved.
  const moved = ((window as any).__captionTimes ??= {}) as Record<string, number>;
  const onClip = (at: number) => {
    let sum = 0;
    for (const p of c.segments) {
      if (at < p.start) return sum;
      if (at <= p.end) return sum + at - p.start;
      sum += p.end - p.start;
    }
    return sum;
  };
  cues.forEach((cue, i) => {
    const start = moved[`${id}:${cue.first}:start`];
    if (start !== undefined) {
      cue.start = onClip(start);
      cue.startMoved = true;
      if (cues[i - 1] && cues[i - 1].end > cue.start) cues[i - 1].end = cue.start;
    }
    const end = moved[`${id}:${cue.last}:end`];
    if (end !== undefined) {
      cue.end = onClip(end);
      cue.endMoved = true;
    }
  });
  return cues;
};

// The speech model install of the setup mode: it starts when it is asked
// for and finishes four seconds later, reporting as it goes. A mode that
// sends no events can never show work starting or stopping, whatever its
// Jobs call answers.
const installAt = () => (window as any).__installAt ?? 0;
const modelRunning = () => installAt() > 0 && Date.now() - installAt() < 4000;
const modelDone = () => installAt() > 0 && Date.now() - installAt() >= 4000;
const modelJob = () => ({
  id: "m1",
  episode: "",
  kind: "model",
  label: "Parakeet TDT 0.6B v3",
  state: modelDone() ? "done" : "running",
  queued: "",
  lane: "hearing",
  progress: modelDone()
    ? undefined
    : {
        stage: "speech model",
        text: "fetching",
        fraction: Math.min((Date.now() - installAt()) / 4000, 1),
        remaining: Math.max(0, Math.round(4 - (Date.now() - installAt()) / 1000)),
      },
});

// The same for a model that finds clips, on its own lane and its own
// clock, so a probe can have one running while the other is not. In the
// settings mode it takes twenty seconds, long enough to watch and to
// cancel. The job is named the way the Go side names it, by the model's
// own title and without its maker, which is not what the row shows: a
// stub that named it the way the row does could never show the row
// failing to find its own install, which it did.
const llmAt = () => (window as any).__llmAt ?? 0;
const llmFor = () => (location.search.includes("models") ? 20000 : 4000);
const llmCancelled = () => (window as any).__llmCancelled ?? 0;
const llmRunning = () => llmAt() > 0 && !llmCancelled() && Date.now() - llmAt() < llmFor();
const llmDone = () => llmAt() > 0 && !llmCancelled() && Date.now() - llmAt() >= llmFor();
const llmJob = () => {
  const share = Math.min((Date.now() - llmAt()) / llmFor(), 1);
  const total = (window as any).__llmSize ?? 15.5;
  return {
    id: "l1",
    episode: "",
    kind: "llm",
    label: (window as any).__llmTitle ?? "Gemma 4 26B A4B",
    state: llmCancelled() ? "cancelled" : llmDone() ? "done" : "running",
    queued: "",
    lane: "finding",
    progress:
      llmDone() || llmCancelled()
        ? undefined
        : {
            stage: "language model",
            text: `fetching ${(total * share).toFixed(1)} GB of ${total.toFixed(1)} GB at 42 MB a second`,
            fraction: share,
            remaining: Math.max(0, Math.round((llmFor() - (Date.now() - llmAt())) / 1000)),
          },
  };
};

// ---------------------------------------------------------------------------
// The searches, the way the Go side runs them, see docs/JOBS.md: one job
// from New to its clips. It hears the episode from where the transcript
// ends to the end of its window, at 600 seconds of audio a second, and then
// finds, and its twelve clips land one at a time on the way. Cancel stops
// it where it is, and what it heard stays. Every search the interface asks
// for is kept on window.__searches, so a probe can read what was asked.
//
// ?interrupted and ?failed start with a search whose record says it was
// cut off or failed, the way the app reads it at start. With ?waiting the
// one cut off had heard the episode to 15 minutes of its half hour.
// ?growing is an episode just added: its first search is running from the
// moment the window opens, started by the Go side, hearing from 10 minutes.
// ?lagging saves the transcript every 8 s of work, the way the engine does,
// while the job reports every chunk it hears.
// ---------------------------------------------------------------------------
type FakeSearch = {
  id: string;
  n: number;
  from: number;
  to: number;
  heardFrom: number;
  wall: number;
  cancelledAt?: number;
  // A search that stopped before this run of the app: cut off, or failed.
  stopped?: "interrupted" | "failed";
  step?: string;
  error?: string;
  settled?: boolean;
  said?: string;
  // How many clips it was asked for. It finds that many and no more, the
  // way the Go side does.
  count?: number;
};
const fullLength = 14423;

// The loudness of the episode, measured on its own the way the Go side
// measures it when an episode is added. ?measuring measures it from the
// moment the page opens, the four hours in twelve seconds, and says it got
// further every half second with a levels event. Like the Go side it
// measures what the clip timeline last asked the waveform of first, then
// on from there, then from the start, and jumps when the view moves to a
// part not measured yet. ?unmeasured never measures it, an episode added
// before the measuring existed, so the waveform is the transcript's alone.
// Otherwise it is measured already.
const levelsRate = fullLength / 12;
let levelParts: [number, number][] = [];
let levelFocus: [number, number] = [0, 0];
let levelAt = -1;
let levelClock = Date.now();
function levelGap(from: number): number | null {
  let at = Math.max(0, from);
  for (const [a, b] of levelParts) if (at >= a && at < b) at = b;
  return at < fullLength - 0.005 ? at : null;
}
function levelNext(): number | null {
  const [from, to] = levelFocus;
  return (to > from ? levelGap(from) : null) ?? levelGap(0);
}
function levelShown(at: number): boolean {
  const [from, to] = levelFocus;
  return to > from && at >= from && at < to;
}
function levelsAdvance() {
  if (!location.search.includes("measuring")) return;
  const now = Date.now();
  let budget = ((now - levelClock) / 1000) * levelsRate;
  levelClock = now;
  while (budget > 0) {
    const want = levelNext();
    if (want === null) return;
    if (levelAt < 0 || levelGap(levelAt) !== levelAt || (levelShown(want) && !levelShown(levelAt))) {
      levelAt = want;
    }
    const stop = Math.min(fullLength, ...levelParts.map(([a]) => a).filter((a) => a > levelAt));
    const take = Math.min(budget, stop - levelAt);
    levelParts.push([levelAt, levelAt + take]);
    levelParts.sort((x, y) => x[0] - y[0]);
    levelParts = levelParts.reduce<[number, number][]>((out, p) => {
      const last = out[out.length - 1];
      if (last && p[0] <= last[1] + 1e-6) last[1] = Math.max(last[1], p[1]);
      else out.push([p[0], p[1]]);
      return out;
    }, []);
    levelAt += take;
    budget -= take;
  }
}
function measuredParts(): [number, number][] {
  if (location.search.includes("unmeasured")) return [];
  if (!location.search.includes("measuring")) return [[0, fullLength]];
  levelsAdvance();
  return levelParts.map(([a, b]) => [a, b]);
}
function measuredNow(): number {
  return measuredParts().reduce((sum, [a, b]) => sum + b - a, 0);
}
// With ?hear=100 it hears a hundred seconds of audio a second instead, so
// a probe has time to look at the hearing.
const hearsPerSecond = Number(/hear=(\d+)/.exec(location.search)?.[1] ?? 600);
const landEvery = () => (location.search.includes("slow") ? 2000 : 350);
const findFor = (s?: FakeSearch) => 700 + landsOf(s).length * landEvery() + 1000;
const landOrder = [3, 1, 7, 2, 12, 5, 4, 9, 6, 11, 8, 10];
// The clips a search lands, in the order it lands them.
const landsOf = (s?: FakeSearch) => landOrder.slice(0, s?.count ?? 12);
const fakeSearches = (): FakeSearch[] => {
  const w = window as any;
  if (!w.__searches) {
    w.__searches = [];
    const q = location.search;
    const now = Date.now();
    if (q.includes("interrupted") || q.includes("failed")) {
      w.__searches.push({
        id: "s0", n: 0, from: 0, to: 1800, heardFrom: baseCovered(), wall: now,
        stopped: q.includes("failed") ? "failed" : "interrupted",
        step: q.includes("failed") ? "failed" : q.includes("waiting") ? "hearing" : "finding",
        error: q.includes("failed") ? "the language model could not be loaded: not enough memory" : undefined,
      });
    }
    if (q.includes("growing")) {
      // ?asked=4 is a first search the Go side asked for another number
      // than the workspace would, the way a target typed for a window of
      // this length, or none, can make it.
      const asked = Number(/asked=(\d+)/.exec(q)?.[1] ?? 0) || undefined;
      w.__searches.push({ id: "s1", n: 1, from: 0, to: 1800, heardFrom: 600, wall: (w.__started ??= now), count: asked });
    }
  }
  return w.__searches;
};
// How far the episode was heard before any search of this run.
function baseCovered(): number {
  const q = location.search;
  if (q.includes("growing")) return 600;
  if (q.includes("waiting")) return 900;
  if (q.includes("transcribing")) return 1200;
  if (q.includes("paused")) return q.includes("short") ? 2500 : 4000;
  return fullLength;
}
// Where a search is at a moment: its step, how far the episode has been
// heard, and how many clips have landed.
function searchAt(s: FakeSearch, now = Date.now()) {
  const end = s.to > 0 ? s.to : fullLength;
  if (s.stopped) {
    return { state: s.settled ? "cancelled" : s.stopped, step: s.step, covered: s.heardFrom, found: 0, since: 0 };
  }
  // Called off with Cancel, it stops where it is and says so, with
  // Continue, the way the Go side does, after the moment the real one takes
  // to save what it heard: it goes on reporting that it runs for 800 ms.
  // Stopping at once hid that the row went on saying Transcribing for as
  // long as that moment lasts.
  const byHand = s.cancelledAt !== undefined && now - s.cancelledAt >= 800;
  const at = Math.min(now, s.cancelledAt ?? now);
  const since = at - s.wall;
  // ?transcribing hears and never gets anywhere: it reports 1800 while the
  // saved transcript says 1200, which is what the edge on the range picker
  // is measured against.
  const still = location.search.includes("transcribing");
  const hearFor = still ? Infinity : (Math.max(end - s.heardFrom, 0) / hearsPerSecond) * 1000;
  const covered = still ? s.heardFrom : Math.min(end, s.heardFrom + (since / 1000) * hearsPerSecond);
  const stoppedState = s.settled ? "cancelled" : "interrupted";
  if (since < hearFor) {
    return byHand
      ? { state: stoppedState, step: "stopped", covered: Math.max(covered, s.heardFrom), found: 0, since }
      : { state: "running", step: "hearing", covered: Math.max(covered, s.heardFrom), found: 0, since };
  }
  const finding = since - hearFor;
  const found = landsOf(s).filter((_, k) => finding >= 700 + k * landEvery()).length;
  if (finding < findFor(s)) {
    return byHand
      ? { state: stoppedState, step: "stopped", covered: end, found, since: finding }
      : { state: "running", step: "finding", covered: end, found, since: finding };
  }
  return { state: "done", step: "", covered: end, found: landsOf(s).length, since: finding };
}
function searchJob(s: FakeSearch) {
  const now = searchAt(s);
  const base = { id: s.id, episode: "/eps/ep.mp4", kind: "search", label: "Find clips", state: now.state, step: now.step, record: "search", from: s.from, to: s.to, count: s.count, queued: "", lane: now.step === "hearing" ? "hearing" : "finding", error: s.error, result: now.state === "done" ? "/eps/ep.framefairy/logs/clips.json" : undefined };
  if (now.state !== "running") return base;
  if (now.step === "hearing") {
    // How far and how long, to the end of the window it hears for, the way
    // the engine reports it. A made-up time left here hid that the engine
    // said the time to the end of the whole episode.
    const covered = location.search.includes("transcribing") ? 1800 : now.covered;
    const end = s.to > 0 ? s.to : fullLength;
    const share = Math.min(Math.max((covered - s.heardFrom) / Math.max(end - s.heardFrom, 1), 0), 1);
    const remaining = Math.max(end - covered, 0) / hearsPerSecond;
    return { ...base, progress: { kind: "progress", stage: "asr", text: "Listening", fraction: share, remaining, from: s.heardFrom, covered, elapsed: 1, time: "" } };
  }
  const lasts = findFor(s);
  const lands = landsOf(s);
  const text = now.found ? `${now.found} of ${lands.length} found` : "Finding clips";
  // The next clip is named a moment before it lands, and is on its way
  // until it does, the way the plan builder says so while it places the
  // crop.
  // Named two and a half landings before it lands, so up to three are on
  // their way at once, the way several framers work on a real machine.
  const k = now.found;
  const underway = lands
    .map((n, j) => ({ n, j }))
    .filter(({ j }) => j >= k && now.since >= Math.max(300, 700 + j * landEvery() - landEvery() * 2.5))
    .map(({ n, j }) => ({ n: j + 1, start: placeOf(s, n), end: placeOf(s, n) + 25, title: "Ein Moment " + n, step: "framing", clip: `clips.json/0${n + (s.n - 1) * 12 + (freshList() ? 0 : 4)}` }));
  // What it has written goes with what it has on the way, in one event,
  // the way the Go side sends them.
  return { ...base, underway, written: now.found, whole: now.found + underway.length >= lands.length, progress: { kind: "progress", stage: "plan", text, fraction: Math.min(now.since / lasts, 0.99), remaining: Math.max((lasts - now.since) / 1000, 0), found: now.found, elapsed: now.since / 1000, time: "" } };
}
// How far the saved transcript reaches. With ?lagging it is saved every 8 s
// of work, minutes of audio apart, while the job reports every chunk.
function savedCovered(): number {
  let covered = baseCovered();
  for (const s of fakeSearches()) {
    const heard = searchAt(s).covered;
    if (location.search.includes("lagging") && !s.stopped) {
      const since = Math.max(Math.min(Date.now(), s.cancelledAt ?? Date.now()) - s.wall, 0);
      covered = Math.max(covered, Math.min(heard, s.heardFrom + Math.floor(since / 8000) * 8 * hearsPerSecond));
    } else {
      covered = Math.max(covered, heard);
    }
  }
  return Math.min(covered, fullLength);
}
// The clips the searches of this run have found, numbered after the ones
// the episode had, in the order they land.
function foundClips(): { n: number; start: number }[] {
  const out: { n: number; start: number }[] = [];
  for (const s of fakeSearches()) {
    if (s.stopped || !s.n) continue;
    const now = searchAt(s);
    const landed = now.step === "finding" || now.state === "done" ? landsOf(s).slice(0, now.found) : [];
    out.push(...landed.map((k) => ({ n: k + (s.n - 1) * 12, start: placeOf(s, k) })));
  }
  return out;
}
// Where the k-th of the twelve moments a search can find starts: in its
// window, the way the model can only name what the window holds, a
// twelfth of the window apart.
function placeOf(s: FakeSearch, k: number): number {
  const end = s.to > 0 ? s.to : fullLength;
  return Math.max(s.from, s.from + ((end - s.from) * (k - 0.5)) / 12 - 12.5);
}
// Clips made by hand with I and O, the way the Go side makes them: a job
// each, any number at once, which hears first where the transcript does
// not reach, places the crop, and lands its clip in the clips made by hand.
// Its clip is on its way from the moment it is asked for, at the playhead,
// and where its sentences are once it has them. ?slowhand takes its time,
// so a probe can look at a clip on its way.
const clock = (t: number) => {
  const s = Math.floor(t);
  return `${Math.floor(s / 3600)}:${String(Math.floor((s % 3600) / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
};
type FakeHand = { id: string; n: number; at: number; backward: boolean; wall: number; hears: boolean; cancelledAt?: number; settled?: boolean };
const hands = (): FakeHand[] => ((window as any).__hands ??= []);
// About as long as the real one takes: the speech model loads and hears
// the minute, then the crop is placed over it. A stub quicker than the
// app hid that nothing showed for the seconds the app takes.
const handTakes = () => (location.search.includes("slowhand") ? 5000 : 3000);
function handAt(h: FakeHand, now = Date.now()) {
  // Called off, it says so where its clip would have been, the way a
  // search does, after the moment the real one takes to stop.
  if (h.settled) return { state: "cancelled", step: "", start: h.at, end: h.at, since: 0, share: -1 };
  if (h.cancelledAt !== undefined && now - h.cancelledAt >= 400) {
    return { state: "interrupted", step: "stopped", start: h.at, end: h.at, since: 0, share: -1 };
  }
  now = Math.min(now, h.cancelledAt ?? now);
  const since = now - h.wall;
  const hearFor = h.hears ? handTakes() : 0;
  const start = Math.max(0, h.backward ? h.at - 25 : h.at - 2);
  if (since < hearFor) return { state: "running", step: "hearing", start: h.at, end: h.at, since, share: since / hearFor };
  if (since < hearFor + handTakes()) {
    // Its sentences are known a moment into the step.
    // Placing the crop says how far it is once it is under way, a quarter
    // of a second after it has the clip's pieces, the way the engine's
    // report ticks.
    const known = since - hearFor > 200;
    const placing = since - hearFor - 450;
    const share = placing > 0 ? Math.min(placing / (handTakes() - 450), 0.99) : -1;
    return { state: "running", step: "framing", start: known ? start : h.at, end: known ? start + 25 : h.at, since, share };
  }
  return { state: "done", step: "", start, end: start + 25, since, share: 1 };
}
// The part a clip made by hand has the engine hear, the minute around the
// playhead, see ClipRequest.reach, and what of it is heard so far.
function handReach(h: FakeHand): [number, number] {
  const r = 16;
  return h.backward
    ? [Math.max(0, h.at - 30 - r), Math.min(fullLength, h.at + r)]
    : [Math.max(0, h.at - r), Math.min(fullLength, h.at + 30 + r)];
}
function handHeard(h: FakeHand): [number, number] | null {
  if (!h.hears) return null;
  const now = handAt(h);
  const [a, b] = handReach(h);
  if (now.step === "hearing") return [a, a + (b - a) * now.share];
  if (now.state === "running" || now.state === "done") return [a, b];
  return null;
}
// What the transcript on disk has heard: from the start as far as the
// searches got, and the parts the clips made by hand had heard.
function heardParts(): [number, number][] {
  const parts: [number, number][] = [[0, savedCovered()]];
  for (const h of hands()) {
    const p = handHeard(h);
    if (p && p[1] > p[0]) parts.push(p);
  }
  parts.sort((x, y) => x[0] - y[0]);
  const out: [number, number][] = [];
  for (const [a, b] of parts) {
    const last = out[out.length - 1];
    if (last && a <= last[1]) last[1] = Math.max(last[1], b);
    else if (b > a) out.push([a, b]);
  }
  return out;
}
function handJob(h: FakeHand) {
  const now = handAt(h);
  const reach = handReach(h);
  const running = now.state === "running";
  return {
    id: h.id, episode: "/eps/ep.mp4", kind: "clip", label: "Make a clip", state: now.state,
    step: now.step, record: `clip-${h.n}`, at: h.at, backward: h.backward, queued: "",
    lane: now.step === "hearing" ? "hearing" : "framing",
    result: now.state === "done" ? `clips-hand.json/h0${h.n}` : undefined,
    underway: now.state === "interrupted" ? [{ n: 1, start: h.at, end: h.at, step: "stopped" }] : running ? [{ n: 1, start: now.start, end: now.end, title: now.step === "framing" && now.start !== now.end ? `Von ${clock(now.start)} an` : undefined, step: now.step, pieces: now.step === "framing" && now.start !== now.end ? handPieces(h, now.start) : undefined, clip: now.step === "framing" ? `clips-hand.json/h0${h.n}` : undefined }] : undefined,
    progress: running && now.step === "hearing"
      ? { kind: "progress", stage: "asr", text: "Listening", fraction: now.share, remaining: (handTakes() - now.since) / 1000, from: reach[0], covered: reach[0] + (reach[1] - reach[0]) * now.share, elapsed: 1, time: "" }
      : running && now.step === "framing" && now.share >= 0
        ? { kind: "progress", text: "placing the crop", fraction: now.share, remaining: (handTakes() * (1 - now.share)) / 1000, elapsed: 1, time: "" }
        : undefined,
  };
}
// What a clip made by hand keeps once its pauses are cut, a moment into
// placing its crop: the pieces of the clip it lands as.
function handMade(h: FakeHand, start: number) {
  return clip(20 + h.n, start, `Von ${clock(start)} an`, false);
}
function handPieces(h: FakeHand, start: number): [number, number][] {
  return handMade(h, start).segments.map((p: { start: number; end: number }) => [p.start, p.end]);
}
// The clips made by hand that have landed.
function handClips() {
  return hands()
    .filter((h) => handAt(h).state === "done")
    .map((h) => {
      const at = handAt(h);
      const made = handMade(h, at.start);
      return { ...made, id: `h0${h.n}`, slug: `hand-${h.n}`, key: `clips-hand.json/h0${h.n}`, plan: "/eps/ep.framefairy/logs/clips-hand.json" };
    });
}

// Whether the episode's list starts empty, and the searches of this run
// fill it.
function freshList(): boolean {
  const q = location.search;
  return q.includes("found") || q.includes("growing") || q.includes("interrupted") || q.includes("failed") || q.includes("transcribing");
}
function askSearch(from: number, to: number, count?: number): FakeSearch {
  const list = fakeSearches();
  for (const s of list) if (s.stopped || s.cancelledAt !== undefined) s.settled = true;
  const n = list.filter((s) => s.n > 0).length + 1;
  const made = { id: `s${n}`, n, from, to, heardFrom: savedCovered(), wall: Date.now(), count };
  list.push(made);
  return made;
}

export const Call = {
  ByName(name: string, ...args: unknown[]): Promise<unknown> {
    const method = name.split(".").pop();
    const q = location.search;
    // Episodes whose list starts empty: the searches of this run fill it.
    const fresh = freshList();
    const covered = savedCovered();
    const found = foundClips();
    const plans = found.length || !fresh ? [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }] : [];
    switch (method) {
      case "Version":
        return Promise.resolve("0.1.0");
      case "Updates":
        return Promise.resolve({ ...updNow() });
      case "FollowChannel":
        updNow().picked = args[0] as string;
        updSend();
        updFetch(args[0] as string);
        return Promise.resolve(null);
      case "CheckForUpdates":
        // The channel followed, as updates.Followed has it: the one picked,
        // or the one the build came from.
        updFetch(updNow().picked || updNow().follows || updNow().channel);
        return Promise.resolve(null);
      case "RestartToUpdate":
        (window as any).__restarted = true;
        return Promise.resolve(null);
      case "StayOpen":
        (window as any).__stayed = ((window as any).__stayed ?? 0) + 1;
        return Promise.resolve(null);
      case "OpenCommit":
        (window as any).__openedCommit = true;
        return Promise.resolve(null);
      case "Platform":
        return Promise.resolve(location.search.includes("linux") ? "linux" : "darwin");
      // The real notices, the ones the app builds in, so the page is looked
      // at with what it will really show.
      case "Licences":
        return Promise.resolve(noticeList);
      case "LicenceText":
        return Promise.resolve(noticeTexts[`../../notices/texts/${String(args[0])}`] ?? "");
      // The first run. Nothing installed, nothing chosen, and an install
      // that really runs and really finishes, so the whole walkthrough can
      // be reached. Every other mode is a machine that is already set up,
      // or the setup would cover the workspace in every probe there is.
      case "Setup": {
        const fresh = location.search.includes("setup");
        const model = {
          name: "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8",
          title: "Parakeet TDT 0.6B v3",
          about: "NVIDIA's recogniser, quantised. Fast on any machine and accurate on speech.",
          languages: "25 European languages, German and English among them",
          download: 487170055,
          unpacked: 671239000,
          url: "https://example.invalid/parakeet.tar.bz2",
          recommended: true,
          installed: (!fresh || modelDone()) && !((window as any).__removed ?? []).includes("sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8"),
        };
        const planner = (window as any).__planner ?? "";
        const key = !!(window as any).__key;
        // Two models from two houses, so the choice the interface has to put
        // is a real one, and so the memory below has something to say. The
        // small one fits any machine, the big one fits none of the ones
        // the harness pretends to be.
        // ?models is the settings of a machine that has tried a second
        // model: two installed, one of them in use, and a third to fetch.
        const trying = location.search.includes("models");
        const removed: string[] = (window as any).__removed ?? [];
        const used: string = (window as any).__used ?? "gemma-4-26B_q4_0-it.gguf";
        const language = trying
          ? [
              {
                name: "gemma-4-26B_q4_0-it.gguf",
                title: "Gemma 4 26B A4B",
                maker: "Google",
                about: "Quantised to four bits, and only four billion of its twenty six are used per token.",
                download: 14439363584,
                needs: 21474836480,
                url: "https://example.invalid/gemma.gguf",
                fit: "fits",
                recommended: true,
              },
              {
                name: "gemma-4-12b-it-qat-q4_0.gguf",
                title: "Gemma 4 12B",
                maker: "Google",
                about: "Twelve billion parameters, trained to be quantised to four bits.",
                download: 6975879296,
                needs: 10737418240,
                url: "https://example.invalid/gemma12.gguf",
                fit: "fits",
                recommended: false,
              },
              {
                name: "Ministral-3-8B-Instruct-2512-Q4_K_M.gguf",
                title: "Ministral 3 8B",
                maker: "Mistral",
                about: "Eight billion parameters, quantised to four bits. The smallest of these.",
                download: 5198911904,
                needs: 15032385536,
                url: "https://example.invalid/ministral.gguf",
                fit: "fits",
                recommended: false,
              },
            ].map((m, i) => {
              const installed = !removed.includes(m.name) && (i < 2 || llmDone());
              // Chosen is chosen, downloaded or not, the way the Go side
              // reports it.
              return { ...m, installed, inUse: used === m.name };
            })
          : [
          {
            name: "gemma-4-26B_q4_0-it.gguf",
            title: "Gemma 4 26B A4B",
            maker: "Google",
            about: "Quantised to four bits, and only four billion of its twenty six are used per token.",
            download: 15461882265,
            needs: 19327352832,
            url: "https://example.invalid/gemma.gguf",
            installed: !fresh || llmDone(),
            fit: "fits",
            recommended: true,
          },
          {
            name: "small-q4.gguf",
            title: "Something Small",
            maker: "Another House",
            about: "Half the size and most of the way there.",
            download: 4900000000,
            needs: 42949672960,
            url: "https://example.invalid/small.gguf",
            installed: false,
            fit: "too big",
            recommended: false,
          },
        ].map((m) => ({ ...m, inUse: m.installed }));
        return Promise.resolve({
          speech: [model],
          hasSpeech: model.installed,
          language,
          memory: 34359738368,
          planner: fresh ? planner : "local",
          // Two companies, and a key for each or not. ?nokey is a machine
          // with none, and a fresh one has none until one is saved. ?envkey
          // has Anthropic's in the environment.
          ...(() => {
            const apiModel: string = (window as any).__settings?.apiModel || (window as any).__apiModel || "claude-sonnet-5";
            const provider = apiModel.startsWith("gpt-") ? "openai" : "anthropic";
            const stored: Record<string, boolean> = (window as any).__keys ?? {};
            const none = fresh || location.search.includes("nokey");
            const env = location.search.includes("envkey") && !stored.anthropic;
            const where = (here: boolean) => (here ? "keychain" : "");
            const keys: Record<string, string> = {
              anthropic: env ? "environment" : where(stored.anthropic === false ? false : none ? !!stored.anthropic || !!key : true),
              openai: where(!!stored.openai),
            };
            return {
              apiModel,
              provider,
              providers: [
                { name: "anthropic", title: "Anthropic", env: "ANTHROPIC_API_KEY", keysAt: "platform.claude.com" },
                { name: "openai", title: "OpenAI", env: "OPENAI_API_KEY", keysAt: "platform.openai.com" },
              ],
              cloud: [
                { model: "claude-sonnet-5", title: "Claude Sonnet 5", provider: "anthropic" },
                { model: "gpt-6-sol", title: "GPT-6 Sol", provider: "openai" },
              ],
              keys,
              // Each key in short, the way the companies list them.
              keyHints: {
                anthropic: keys.anthropic === "environment" ? "sk-ant-api03...WXYZ" : keys.anthropic ? "sk-ant-api03...MwAA" : "",
                openai: keys.openai ? "sk-proj-7Fq2...k9Qa" : "",
              },
              hasKey: !!keys[provider],
            };
          })(),
          hasLocalModel: fresh ? llmDone() : true,
          // ?noserver is the machine with a model and nothing to run it,
          // which is the state the local way has to say something about.
          hasServer: !location.search.includes("noserver"),
          chosen: fresh ? !!planner : true,
          ready: !fresh,
          installing: modelRunning() ? "m1" : "",
        });
      }
      case "InstallSpeechModel":
        (window as any).__installAt ??= Date.now();
        return Promise.resolve(modelJob());
      case "InstallLanguageModel": {
        const titles: Record<string, [string, number]> = {
          "Ministral-3-8B-Instruct-2512-Q4_K_M.gguf": ["Ministral 3 8B", 5.2],
          "gemma-4-12b-it-qat-q4_0.gguf": ["Gemma 4 12B", 7.0],
        };
        const [title, total] = titles[String(args[0])] ?? ["Gemma 4 26B A4B", 15.5];
        (window as any).__llmTitle = title;
        (window as any).__llmSize = total;
        (window as any).__llmCancelled = 0;
        (window as any).__llmAt = Date.now();
        return Promise.resolve(llmJob());
      }
      case "RemoveLanguageModel":
      case "RemoveSpeechModel":
        ((window as any).__removed ??= []).push(String(args[0]));
        if ((window as any).__used === args[0]) (window as any).__used = "";
        return Promise.resolve(null);
      case "UseLanguageModel":
        (window as any).__used = String(args[0]);
        return Promise.resolve(`/Users/tim/.framefairy/models/${String(args[0])}`);
      case "ChoosePlanner":
        (window as any).__planner = args[0];
        return Promise.resolve(null);
      case "OpenKeysPage":
        (window as any).__opened = String(args[0]);
        return Promise.resolve(null);
      case "SaveAPIKey": {
        // A key that ends in "bad" is one the company refuses, after the
        // moment it takes to ask, and one that does not look like theirs is
        // said to be no key of theirs.
        const typed = String(args[1] ?? "").trim();
        if (typed.endsWith("bad")) {
          const openai = String(args[0]) === "openai";
          const [title, prefix] = openai ? ["OpenAI", "sk-"] : ["Anthropic", "sk-ant-"];
          const why = typed.startsWith(prefix) ? `${title} did not accept this key.` : `not an ${title} key. Those start with ${prefix}.`;
          return new Promise((_, reject) => setTimeout(() => reject(new Error(why)), 600));
        }
        ((window as any).__keys ??= {})[String(args[0])] = !!typed;
        return new Promise((resolve) => setTimeout(() => resolve(null), 600));
      }
      // The licence key. ?link starts with a key from a framefairy:// link
      // having come, and window.__unlockLink(key) opens one while the app
      // runs. A link does what linkOutcome in licence.go does: it unlocks
      // an app with no key, and waits for Unlock to replace one. A key that
      // ends in "bad" is refused, from a link at once and when pasted after
      // the moment it takes.
      case "Licence":
        return Promise.resolve(licenceNow());
      case "SaveLicence": {
        const typed = String(args[0] ?? "").trim();
        if (typed.endsWith("bad")) {
          return new Promise((_, reject) =>
            setTimeout(() => reject(new Error("this key was not signed by Frame Fairy")), 400),
          );
        }
        licence.key = typed;
        licence.saved = !!typed;
        licence.about = typed ? describeKey(typed) : "";
        return new Promise((resolve) => setTimeout(() => resolve(licenceNow()), 400));
      }
      case "Copy":
        (window as any).__copied = String(args[0]);
        return location.search.includes("denied") ? Promise.reject(new Error("the clipboard did not take it")) : Promise.resolve();
      case "TakeLicenceLink": {
        const l = licence.link ?? { what: "", key: "", about: "", reason: "" };
        licence.link = null;
        return Promise.resolve(l);
      }
      case "ChooseCloudModel":
        (window as any).__apiModel = String(args[0]);
        if ((window as any).__settings) (window as any).__settings.apiModel = String(args[0]);
        return Promise.resolve(null);
      // Add brings in one more episode, whose name sorts between the two
      // already there, so a probe can tell the order added from the order
      // by name. The library keeps the order added, the way the Go side does.
      case "AddEpisodes": {
        const n = ((window as any).__added = ((window as any).__added ?? 0) + 1);
        return Promise.resolve([`/eps/neu${n}.mp4`]);
      }
      case "Library":
        return Promise.resolve([
          { source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: true, covered: 14423, heard: [[0, 14423]], transcriptStale: false, plans: [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }], rendered: 1, previews: 0, work: true, everSearched: true },
          { source: "/eps/zwei.mp4", name: "Folge 12, die lange Nacht", size: 1, modified: "", missing: false, transcribed: false, covered: 900, heard: [[0, 900]], transcriptStale: false, plans: [], rendered: 0, previews: 0, work: true, everSearched: true },
          ...Array.from({ length: (window as any).__added ?? 0 }, (_, k) => ({ source: `/eps/neu${k + 1}.mp4`, name: `Folge ${k + 3}, der Morgen danach`, size: 1, modified: "", missing: false, transcribed: false, covered: 0, heard: [], transcriptStale: false, plans: [], rendered: 0, previews: 0, work: true, everSearched: false })),
        ]);
      case "Episode":
        return Promise.resolve({ source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: covered >= fullLength, covered, heard: heardParts(), measured: measuredNow(), measuredParts: measuredParts(), measuredAll: measuredNow() >= fullLength - 0.01, transcriptStale: false, plans, rendered: fresh ? 0 : 1, previews: 0, work: true, everSearched: true });
      // New. A probe reads what was asked for on window.__searches.
      case "Search": {
        const req = args[1] as { From: number; To: number; Count?: number };
        // The Go side refuses a search it cannot start, while the app
        // closes or the episode is being removed: the job comes back
        // failed, and its event goes out like any other.
        if (location.search.includes("closing")) {
          const list = ((window as any).__refused ??= []) as any[];
          const job = { id: `refused-${list.length + 1}`, episode: "/eps/ep.mp4", kind: "search", label: "Find clips", state: "failed", error: "the app is closing", from: req.From, to: req.To, count: req.Count, queued: "", lane: "finding" };
          list.push(job);
          return Promise.resolve(job);
        }
        const made = askSearch(req.From, req.To, req.Count);
        return Promise.resolve(searchJob(made));
      }
      case "MakeClip": {
        const at = Number(args[1]);
        const list = hands();
        const made = { id: `h${list.length + 1}`, n: list.length + 1, at, backward: !!args[2], wall: Date.now(), hears: false };
        const reach = handReach(made);
        made.hears = !heardParts().some(([a, b]) => a <= reach[0] && b >= reach[1]);
        list.push(made);
        return Promise.resolve(handJob(made));
      }
      case "Continue": {
        const hand = hands().find((h) => h.id === args[0] && handAt(h).state === "interrupted");
        if (hand) {
          hand.settled = true;
          const list = hands();
          const again = { id: `h${list.length + 1}`, n: list.length + 1, at: hand.at, backward: hand.backward, wall: Date.now(), hears: false };
          list.push(again);
          return Promise.resolve(handJob(again));
        }
        const was = fakeSearches().find((s) => s.id === args[0] && (s.stopped || s.cancelledAt !== undefined) && !s.settled);
        if (!was) return Promise.resolve({ id: "x", episode: "", kind: "continue", label: "Continue", state: "failed", queued: "", lane: "finding" });
        return Promise.resolve(searchJob(askSearch(was.from, was.to)));
      }
      case "Training":
        return Promise.resolve({ dir: "/Users/tim/.framefairy/training", plans: 14, decisions: 62 });
      case "ClearTraining":
        return Promise.resolve(null);
      case "Source":
        // The frame rate of the episode the harness plays, see harnessFps.
        // Without it the app took 30, and the frame on screen was a second
        // behind a playing clock half the time.
        return Promise.resolve({ duration: 14423, width: 1920, height: 1080, cropWidth: 608, cropHeight: 1080, fps: harnessFps });
      case "Clips": {
        const made = [
          ...found.map((f) => clip(f.n + (fresh ? 0 : 4), f.start, "Ein Moment " + f.n, false)),
          ...handClips(),
        ];
        // ?lagclips answers every list 300 ms late, the way a busy machine
        // does, so a card on its way has to hold its place until the list
        // has the clip it became.
        // ?crossclips answers one read late and the next at once, so a
        // read asked later can answer first, the way two reads in the air
        // together do on a busy machine.
        const reads = ((window as any).__reads = ((window as any).__reads ?? 0) + 1);
        const late = location.search.includes("lagclips") ? 300 : location.search.includes("crossclips") && reads % 2 ? 400 : 0;
        const answer = <T,>(list: T): Promise<T> =>
          late ? new Promise((done) => setTimeout(() => done(list), late)) : Promise.resolve(list);
        if (fresh) return answer(made.sort((x, y) => x.start - y.start));
        // A list that is slow to come, the way it is while the machine is
        // busy, and that knows nothing of what was done since it was asked
        // for. It lands after an edit made in the meantime, and whatever it
        // says must not undo that edit on screen.
        if (location.search.includes("slowclips") && (window as any).__listed) {
          const asked = [
            clip(1, 57, "Mein Arm ist zersprungen", true),
            clip(2, 400, "Der Typ vor mir auf einmal", false),
            clip(3, 902, "Warum ich nie wieder", false),
            clip(4, 1400, "Ein echtes Thema", false),
          ];
          return new Promise((done) => setTimeout(() => done(asked), 1200));
        }
        (window as any).__listed = true;
        return answer([
          clip(1, 57, "Mein Arm ist zersprungen", true),
          clip(2, starts["02"][0], "Der Typ vor mir auf einmal", false),
          clip(3, 902, "Warum ich nie wieder", false),
          clip(4, 1400, "Ein echtes Thema", false),
          ...made,
        ]
          .map((c) => ({ ...c, rejected: !!(window as any).__gone?.[Number(c.id)] }))
          .sort((x, y) => x.start - y.start));
      }
      case "Coverage": {
        // The episode in parts by how many searches have read them, the
        // way the Go side counts them.
        const view = (passes: { from: number; to: number; times: number }[]) => ({ passes });
        if (fresh) {
          if (!found.length) return Promise.resolve(view([{ from: 0, to: 14423, times: 0 }]));
          return Promise.resolve(view([{ from: 0, to: 1800, times: 1 }, { from: 1800, to: 14423, times: 0 }]));
        }
        return Promise.resolve(
          view([
            { from: 0, to: 1800, times: 1 },
            { from: 1800, to: 5400, times: 0 },
            { from: 5400, to: 7200, times: 1 },
            { from: 7200, to: 14423, times: 0 },
          ]),
        );
      }
      case "Room":
        // ?uneven is an episode read for its first hour, lighter there than
        // the rest is weighed, the way a real one is: the longest window
        // that fits is longer from the start than further on, which is
        // what Tim ran into moving a window along.
        if (location.search.includes("uneven")) {
          const lines = [];
          for (let at = 0; at + 3 <= 3600; at += 4) lines.push({ start: at, end: at + 3, chars: 80 });
          return Promise.resolve({ chars: 250000, by: "memory", lines, heard: [[0, 3600]], rate: 30 });
        }
        // A model that reads everything, or with ?small one that reads
        // about 35 minutes at a time, the way Qwen3 14B does, and with
        // ?memory a machine whose memory holds about 1.3 hours.
        return Promise.resolve({
          chars: location.search.includes("small") ? 52000 : location.search.includes("memory") ? 117000 : 600000,
          by: location.search.includes("memory") ? "memory" : "context",
          lines: [],
          heard: [],
          rate: 25,
        });
      case "ArrivingCaptions": {
        const h = hands().find((x) => x.id === args[0]);
        const now = h && handAt(h);
        if (!h || !now || now.step !== "framing" || now.start === now.end) return Promise.resolve(null);
        return Promise.resolve({
          captions: cuesOf(handMade(h, now.start).segments),
          style: { font: face(), size: 0.062, lineHeight: 1.16, chosenSize: size(), bold: true, marginV: 0.156, marginH: 0.04, padX: 0.012, padY: 0.008, radius: 0.008, primary: textCss(), box: boxCss(), highlight: (window as any).__highlight ?? true, highlightColour: pillCss(), text: (window as any).__text_on ?? true, boxOn: (window as any).__box_on ?? true },
        });
      }
      case "Captions":
        // ?slowcaptions answers a second and a half late, the way a busy
        // machine can, which is longer than a walk into the next clip
        // waits for anything else.
        return (location.search.includes("slowcaptions") ? new Promise((done) => setTimeout(done, 1500)) : Promise.resolve()).then(() => ({
          captions: captionCues(String(args[2])),
          style: { font: face(), size: 0.062, lineHeight: 1.16, chosenSize: size(), bold: true, marginV: 0.156, marginH: 0.04, padX: 0.012, padY: 0.008, radius: 0.008, primary: textCss(), box: boxCss(), highlight: (window as any).__highlight ?? true, highlightColour: pillCss(), text: (window as any).__text_on ?? true, boxOn: (window as any).__box_on ?? true },
        }));
      case "Fonts":
        return Promise.resolve([
          { name: "Inter Black", about: "", file: "Inter-Black.ttf" },
          { name: "Anton", about: "", file: "Anton-Regular.ttf" },
          { name: "Archivo Black", about: "", file: "ArchivoBlack-Regular.ttf" },
        ]);
      // What the app remembers of an episode between runs. The preview
      // starts each time with nothing chosen, so a probe sees what a first
      // opening looks like unless it says otherwise.
      // Where macOS put the title bar and its buttons. Zeros stand for the
      // systems that draw their own title bar, which is what a probe sees
      // unless it asks for ?mac, and those numbers are what a 2026 Mac
      // with the automatic toolbar style answers.
      case "Chrome":
        if (location.search.includes("full")) {
          // Native fullscreen: the title bar is gone and the buttons with
          // it, and the bar keeps the height it had.
          return Promise.resolve({ bar: 52, left: 0, right: 0, middle: 0 });
        }
        return Promise.resolve(
          location.search.includes("mac")
            ? { bar: 52, left: 20, right: 92, middle: 26 }
            : { bar: 0, left: 0, right: 0, middle: 0 },
        );
      case "ChosenClip":
        return Promise.resolve((window as any).__chosen ?? "");
      case "ChooseClip":
        (window as any).__chosen = args[1];
        return Promise.resolve();
      // Kept across a reload, the way the Go side keeps it across a
      // restart, so a probe can reload the page and see it come back.
      case "ChosenWindow":
        try {
          return Promise.resolve(JSON.parse(sessionStorage.getItem("__window") ?? "null"));
        } catch {
          return Promise.resolve(null);
        }
      case "ChooseWindow": {
        const now = { from: args[1], to: args[2], length: args[3] };
        let was = null;
        try {
          was = JSON.parse(sessionStorage.getItem("__window") ?? "null");
          sessionStorage.setItem("__window", JSON.stringify(now));
        } catch {
          /* nothing to keep it in */
        }
        // A window moved by hand is a step, the way the Go side keeps it.
        if (args[4] && JSON.stringify(was) !== JSON.stringify(now)) {
          ((window as any).__windowSteps ??= []).push([was, now]);
          (window as any).__windowRedo = [];
        }
        return Promise.resolve();
      }
      case "GetSettings":
        return Promise.resolve({ llmModel: "", asrModel: "", planner: (window as any).__planner || "local", apiModel: "claude-sonnet-5", target: 0, min: 20, max: 30, highlightColour: "#b4236f", appColour: "#942192", outputDir: "", captionY: 240, trainingDir: "", ...((window as any).__settings ?? {}) });
      // What the settings page saves, kept, so a probe can read what was
      // saved and a page opened again reads it back.
      // Only what changed comes, and it is laid over what is kept, the way
      // the Go side does it.
      case "SaveSettings":
        (window as any).__settings = { ...((window as any).__settings ?? {}), ...(args[0] as object) };
        (window as any).__saves = ((window as any).__saves ?? 0) + 1;
        (window as any).__saved = [...((window as any).__saved ?? []), args[0]];
        if ((args[0] as any).planner) (window as any).__planner = (args[0] as any).planner;
        return Promise.resolve(null);
      case "ChooseFolder":
        return Promise.resolve("/Users/tim/Movies/Shorts");
      // The machine's checks, the way CheckSetup makes them. ?broken is a
      // machine that has lost llama-server.
      case "CheckSetup": {
        const broken = location.search.includes("broken");
        const api = ((window as any).__settings?.planner ?? (window as any).__planner) === "api";
        const out = [
          { name: "ffmpeg", ok: true, version: "8.1.3", detail: "The one this app was built with: its SHA-256 matches the one built in.", path: "/Applications/Frame Fairy.app/Contents/MacOS/ffmpeg", sha256: "e1ba46e814d383b96be780d1a7c49e1c4eecef1619615a241974dc122e2f1a2e" },
          { name: "ffprobe", ok: true, version: "8.1.3", detail: "The one this app was built with: its SHA-256 matches the one built in.", path: "/Applications/Frame Fairy.app/Contents/MacOS/ffprobe", sha256: "5b0c9e2d7a41f8836c2e0b1d94f7a6c35e8d21b0a97f4c6e3d58b2a1f0c9e7d4" },
          { name: "Video decoding", ok: true, detail: "VideoToolbox, falling back to the processor for a file it does not take. The log of a search says which one it used" },
          { name: "Caption fonts", ok: true, detail: "built in: Inter Black, Montserrat ExtraBold" },
          { name: "Speech model", ok: true, detail: "/Users/tim/.framefairy/models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8" },
        ];
        // The same key the setup answers about: there on a machine that is
        // set up, and on a fresh one once it is saved. ?nokey takes it away.
        const model: string = (window as any).__settings?.apiModel || (window as any).__apiModel || "claude-sonnet-5";
        const openai = model.startsWith("gpt-");
        const stored: Record<string, boolean> = (window as any).__keys ?? {};
        const none = location.search.includes("nokey") || location.search.includes("setup");
        const key = openai ? !!stored.openai : !none || !!stored.anthropic;
        const who = openai ? "OpenAI" : "Anthropic";
        if (api) out.push({ name: `${who} API key`, ok: key, detail: key ? "found" : `no ${who} API key found. Either set ${openai ? "OPENAI_API_KEY" : "ANTHROPIC_API_KEY"}, or store it in the keychain` });
        else {
          out.push(broken
            ? { name: "llama-server", ok: false, detail: "llama-server beside the program is not the one this program was built with, so it is not run. Its SHA-256 is 05ea47886ba6d8a0e9fb93f7d7f68f64b992eefb78e5628e79709c2154e9d71b and should be 9d2c41f0e6b8a3d75c1e04f92b6a8d3e7f05c1b29a4e68d3f7b0c25e19a4d6f8. Installing the app again puts the right one back.", path: "/Applications/Frame Fairy.app/Contents/MacOS/llama-server", sha256: "05ea47886ba6d8a0e9fb93f7d7f68f64b992eefb78e5628e79709c2154e9d71b" }
            : { name: "llama-server", ok: true, version: "b11105", detail: "The one this app was built with: its SHA-256 matches the one built in.", path: "/Applications/Frame Fairy.app/Contents/MacOS/llama-server", sha256: "9d2c41f0e6b8a3d75c1e04f92b6a8d3e7f05c1b29a4e68d3f7b0c25e19a4d6f8" });
          // A model chosen before its download is not here yet, which the
          // check says in the Go side's words.
          const used: string = (window as any).__used ?? "gemma-4-26B_q4_0-it.gguf";
          const here = !location.search.includes("models") || used !== "Ministral-3-8B-Instruct-2512-Q4_K_M.gguf" || llmDone();
          out.push(here
            ? { name: "Language model", ok: true, detail: `/Users/tim/.framefairy/models/${used}` }
            : { name: "Language model", ok: false, detail: "Ministral 3 8B is not downloaded yet." });
        }
        return new Promise((r) => setTimeout(() => r(out), 300));
      }
      case "RemoveClip": {
        const [, , id, gone] = args as [string, string, string, boolean];
        const n = Number(id);
        // Kept, the way the plan keeps it, so a list read again has the
        // clip removed still.
        ((window as any).__gone ??= {})[n] = !!gone;
        const made = clip(n, [57, 400, 902, 1400][n - 1] ?? 60, ["Mein Arm ist zersprungen", "Der Typ vor mir auf einmal", "Warum ich nie wieder", "Ein echtes Thema"][n - 1] ?? "Clip", n === 1);
        return Promise.resolve({ ...made, rejected: !!gone });
      }
      // A caption moved by hand, kept the way the engine keeps it, against
      // the word and the edge. Below nought puts it back.
      case "SetThumbnail": {
        const [, , id, from, to] = args as [string, string, string, number, number];
        const all = ((window as any).__thumbs ??= {}) as Record<string, number[]>;
        const list = (all[id] ??= []);
        const ms = (t: number) => Math.round(t * 1000);
        if (from >= 0) {
          const at = list.findIndex((t) => ms(t) === ms(from));
          if (at < 0) return Promise.reject(new Error("the clip has no thumbnail there"));
          list.splice(at, 1);
        }
        if (to >= 0) {
          if (list.some((t) => ms(t) === ms(to))) {
            return Promise.reject(new Error("the clip already has a thumbnail there"));
          }
          list.push(Math.round(to * 1000) / 1000);
        }
        return Promise.resolve(clipOf(id));
      }
      case "SetCaptionTime": {
        const [, , id, word, edge, at] = args as [string, string, string, number, string, number];
        const moved = ((window as any).__captionTimes ??= {}) as Record<string, number>;
        const key = `${id}:${word}:${edge}`;
        if (at < 0) delete moved[key];
        else moved[key] = at;
        const n = Number(id);
        return Promise.resolve(clip(n, [57, 400, 902, 1400][n - 1] ?? 60, ["Mein Arm ist zersprungen", "Der Typ vor mir auf einmal", "Warum ich nie wieder", "Ein echtes Thema"][n - 1] ?? "Clip", n === 1));
      }
      // Every undo and redo the interface asks for, so a probe can see which
      // reached the episode and which stayed in a field being typed in. The
      // clip it names is the third, so a probe can see the interface go
      // there.
      case "Undo":
      case "Redo": {
        ((window as any).__undone ??= []).push(method);
        // The window's steps first, the way they come last on the Go side
        // when a probe has just moved it.
        const back = method === "Undo";
        const steps = ((window as any).__windowSteps ??= []);
        const redo = ((window as any).__windowRedo ??= []);
        const from = back ? steps : redo;
        const step = from.pop();
        const shaped = ((window as any).__reshapeSteps ??= []);
        const unshaped = ((window as any).__reshapeRedo ??= []);
        const reshaped = (back ? shaped : unshaped).pop();
        if (!step && reshaped) {
          (back ? unshaped : shaped).push(reshaped);
          held()[reshaped.id] = back ? reshaped.was : reshaped.is;
          const moved = reshaped.playhead && reshaped.playhead[0] !== reshaped.playhead[1];
          return Promise.resolve({
            done: true,
            clip: `${reshaped.plan.split("/").pop()}/${reshaped.id}`,
            playhead: moved ? reshaped.playhead[back ? 0 : 1] : undefined,
          });
        }
        if (step) {
          (back ? redo : steps).push(step);
          const put = back ? step[0] : step[1];
          try {
            sessionStorage.setItem("__window", JSON.stringify(put));
          } catch {
            /* nothing to keep it in */
          }
          return Promise.resolve({ done: true, window: put ?? undefined });
        }
        return Promise.resolve({ done: true, clip: "clips.json/03" });
      }
      case "Jobs": {
        const searches = [...fakeSearches().map(searchJob), ...hands().map(handJob)];
        // A render running on the first clip, so the Render button has
        // work of its own to show.
        if (q.includes("rendering")) {
          return Promise.resolve([
            { id: "r1", episode: "/eps/ep.mp4", kind: "render", label: "Render", state: "running", plan: "/eps/ep.framefairy/logs/clips.json", clips: ["01"], queued: "", lane: "rendering", progress: { stage: "render", text: "Burning in the captions", fraction: 0.58, remaining: 42 } },
            ...searches,
          ]);
        }
        // Progress with no number to it comes with the busy job, so
        // ?unknown on its own means the same as ?busy&unknown.
        if (!q.includes("busy") && !q.includes("unknown")) return Promise.resolve(searches);
        return Promise.resolve([
          {
            id: "j1",
            episode: "/eps/ep.mp4",
            kind: "search",
            label: "Find clips",
            state: "running",
            step: "finding",
            record: "search",
            from: 0,
            to: 1800,
            queued: "",
            lane: "finding",
            progress: {
              stage: "plan",
              text: "Finding clips",
              fraction: q.includes("unknown") ? -1 : 0.42,
              remaining: 130,
            },
          },
          {
            id: "j0",
            episode: "/eps/youtube.mp4",
            kind: "search",
            label: "Find clips",
            state: "done",
            queued: "",
            lane: "finding",
          },
        ]);
      }
      // A correction belongs to the episode and is applied to every clip
      // that holds the word, which here is every clip that reads it back
      // through fixed().
      case "SetCaptionStyle":
        if (location.search.includes("refuse")) {
          return Promise.reject(new Error("that face is not installed"));
        }
        if (args[2]) (window as any).__face = String(args[2]);
        if (Number(args[3]) > 0) (window as any).__size = Number(args[3]);
        return Promise.resolve(null);
      case "SetCaptionColours":
        // A probe reads what was saved from here.
        ((window as any).__colours ??= []).push(args.slice(2));
        if (args[2]) (window as any).__text = hexToCss(String(args[2]), Number(args[3]));
        if (args[4]) (window as any).__box = hexToCss(String(args[4]), Number(args[5]));
        if (args[6]) (window as any).__pill = hexToCss(String(args[6]), Number(args[7] ?? 1));
        return Promise.resolve(null);
      // The switches of the captions column. A probe reads them back from
      // here.
      case "SetCaptionSwitch": {
        const key = ({ highlight: "__highlight", text: "__text_on", box: "__box_on" } as Record<string, string>)[String(args[2])];
        if (!key) return Promise.reject(new Error("no such switch"));
        (window as any)[key] = Boolean(args[3]);
        return Promise.resolve(null);
      }
      case "SetWord": {
        // Nothing removes the word, the way SetWordText does.
        const text = String(args[4]).replace(/\s+/g, " ").trim();
        if (location.search.includes("refuse")) {
          return Promise.reject(new Error("there is no word at 0:57"));
        }
        // A removed word typed back in beside its neighbour goes back where
        // it was heard, the way the engine's putBack does it.
        const all = allWords();
        const i = all.findIndex((w) => said(w.start) === said(Number(args[3])));
        const put = new Map<number, string>([[i, text]]);
        const reads = (j: number) => (fixed()[said(all[j].start)] ?? all[j].text).split(" ").filter(Boolean);
        const gone = (j: number) => j >= 0 && j < all.length && reads(j).length === 0;
        const was = i >= 0 ? reads(i) : [];
        const now = text.split(" ").filter(Boolean);
        const same = (a: string[], b: string[]) => a.join(" ") === b.join(" ");
        if (was.length > 0 && now.length > was.length) {
          const after = same(now.slice(0, was.length), was) && gone(i + 1);
          const before = !after && same(now.slice(now.length - was.length), was) && gone(i - 1);
          if (after || before) {
            const extra = after ? now.slice(was.length) : now.slice(0, now.length - was.length);
            const slots: number[] = [];
            for (let j = after ? i + 1 : i - 1; gone(j) && slots.length < extra.length; j += after ? 1 : -1) slots.push(j);
            const last = slots.length - 1;
            put.set(i, was.join(" "));
            slots.forEach((j, k) => {
              if (after) put.set(j, k < last ? extra[k] : extra.slice(last).join(" "));
              else put.set(j, k < last ? extra[extra.length - 1 - k] : extra.slice(0, extra.length - last).join(" "));
            });
          }
        }
        for (const [j, says] of put) {
          const key = j >= 0 ? said(all[j].start) : said(Number(args[3]));
          if (j >= 0 && says === all[j].text) delete fixed()[key];
          else fixed()[key] = says;
        }
        return Promise.resolve(clipOf(String(args[2])));
      }
      case "Waveform": {
        const from = Number(args[1]) || 0;
        const to = Number(args[2]) || 14423;
        // Never more buckets than there are measurements, the way the Go
        // side answers. Without this the stub hands back five buckets
        // carrying the same ten milliseconds and the interface has no way to
        // know it.
        const buckets = Math.min(Number(args[3]) || 900, Math.max(Math.ceil((to - from) / 0.01), 1));
        // The peaks come from the loudness measured on its own, or from the
        // transcript where only that has heard, the way the Go side
        // answers. What the waveform is asked for is where the measuring
        // goes first.
        levelsAdvance();
        levelFocus = [from, to];
        const parts = measuredParts();
        const heardTo = location.search.includes("transcribing") ? 1200 : 14423;
        const known = (t: number) => t <= heardTo || parts.some(([a, b]) => t >= a && t < b);
        // Loudness is measured every ten milliseconds and no finer, and
        // a bucket is the loudest measurement that falls in it. That is
        // engine.FrameSeconds and Transcript.Peaks, and the stub has to do
        // the same or it cannot be zoomed in past the measurement, which
        // is the one place the waveform looked like a display with too few
        // pixels. It used to answer with exactly as many smooth values as
        // it was asked for, whatever the zoom, so the fault could not
        // happen here at all.
        const frame = 0.01;
        // Speech, near enough: syllables about four a second inside a
        // slower rise and fall, and a grain on top because loudness is not
        // smooth from one ten milliseconds to the next. The grain is the
        // part that matters here. A signal that barely moves between
        // neighbouring frames cannot show a staircase however coarsely it
        // is drawn, so a stub without it says every drawing is fine.
        const loud = (t: number) => {
          if (!known(t)) return -90;
          const said = Math.abs(Math.sin(t * 6.3)) * Math.abs(Math.cos(t * 0.7));
          const grain = Math.abs(Math.sin(t * 997));
          return -60 + 45 * said * (0.55 + 0.45 * grain);
        };
        const step = (to - from) / buckets;
        return Promise.resolve(
          Array.from({ length: buckets }, (_, i) => {
            const first = Math.floor((from + i * step) / frame);
            const last = Math.max(first + 1, Math.ceil((from + (i + 1) * step) / frame - 1e-9));
            let peak = -90;
            for (let k = first; k < last; k++) peak = Math.max(peak, loud(k * frame));
            return peak;
          }),
        );
      }
      // What a gesture makes of a clip, and saving it. The engine works this
      // out in engine/shape.go. This is a small stand-in with the same
      // shape of answer: edges on frames, or on the nearest word with shift,
      // a clip at least a second long, and a cut at least 50 ms wide.
      case "Shape":
      case "Reshape": {
        const [, plan, id, g, playhead] = args as [string, string, string, StubGesture, [number, number]];
        const [at, title, rendered] = starts[id] ?? [60, "Clip", false];
        const now = pieces(Number(id), at);
        const out = gestured(now, g, at);
        if (!out) return Promise.reject(new Error("that would leave the clip with nothing in it"));
        if (method === "Reshape") {
          // A step undo takes back, with the playhead the way the Go side
          // keeps it, see history.go.
          ((window as any).__reshapeSteps ??= []).push({ plan, id, was: now, is: out.pieces, playhead });
          (window as any).__reshapeRedo = [];
          const found = ((window as any).__found ??= {}) as Record<string, [number, number]>;
          if (!found[id] && now.length) found[id] = [now[0].start, now[now.length - 1].end];
          held()[id] = out.pieces;
          return Promise.resolve(clip(Number(id), at, title, rendered));
        }
        return Promise.resolve({
          pieces: out.pieces.map((p) => ({ start: p.start, end: p.end })),
          playhead: out.playhead,
          captions: { captions: captionCues(id, out.pieces), style: {} },
        });
      }
      case "Words":
        if (location.search.includes("transcribing")) return Promise.resolve([]);
        return Promise.resolve(
          words(Number(args[1]), Number(args[2]))
            .map((w) => ({ ...w, text: fixed()[said(w.start)] ?? w.text }))
            .filter((w) => w.text !== ""),
        );
      // Removing waits for the episode's work to stop, which takes a
      // moment while a search runs. Every call is counted, so a probe can
      // see whether a second click sent a second removal.
      case "RemoveEpisode":
        (window as any).__removals = ((window as any).__removals ?? 0) + 1;
        return new Promise((r) => setTimeout(() => r(null), 1500));
      case "StopClipWork": {
        // Cancel at the head of the clip list, on every search and every
        // clip made by hand of the episode that runs, the way CancelJob
        // does it on one.
        (window as any).__stopped = true;
        const now = Date.now();
        for (const s of fakeSearches()) {
          if (!s.stopped && s.cancelledAt === undefined && searchAt(s, now).state === "running") {
            ((window as any).__cancels ??= []).push(s.id);
            s.cancelledAt = now;
          }
        }
        for (const h of hands()) {
          if (h.cancelledAt === undefined && handAt(h, now).state === "running") {
            ((window as any).__cancels ??= []).push(h.id);
            h.cancelledAt = now;
          }
        }
        return Promise.resolve(null);
      }
      case "CancelJob": {
        (window as any).__stopped = true;
        ((window as any).__cancels ??= []).push(args[0]);
        if (args[0] === "l1" && llmRunning()) (window as any).__llmCancelled = Date.now();
        const hand = hands().find((h) => h.id === args[0]);
        if (hand && handAt(hand).state === "interrupted") hand.settled = true;
        else if (hand && hand.cancelledAt === undefined) hand.cancelledAt = Date.now();
        const s = fakeSearches().find((f) => f.id === args[0]);
        if (s?.stopped) s.settled = true;
        else if (s && s.cancelledAt === undefined) s.cancelledAt = Date.now();
        return Promise.resolve(null);
      }
      default:
        return Promise.resolve(null);
    }
  },
};

// Updates, the way updates.go reports them. The build running is pull
// request 18's, or with ?makebuild one made by make, which follows nothing
// until a channel is picked, whatever a build before it picked, and with
// ?updatesoff one with no update key.
// ?prgone is pull request 18's build after the pull request was merged:
// it is not on the list any more, and nothing downloads until another
// channel is picked.
// ?listfails is pull request 18's build, picked by hand, when the channel
// list could not be read since the app started: no channels at all, and a
// check that failed. The pull request is still open, so it must not read
// as closed.
// Picking a channel checks, downloads over two seconds with the fill, and
// says ready, the way the Go side does.
const updListeners = new Set<(ev: unknown) => void>();
const updChannels = [
  { id: "main", name: "main", version: "0.3.0-main.9f8e7d6" },
  { id: "pr-20", name: "#20 Captions follow whoever speaks", version: "0.3.0-pr20.c0ffee1" },
  { id: "pr-18", name: "#18 How the app updates itself", version: "0.3.0-pr29.db33a28" },
];
let upd: any = null;
// The download in hand, stopped when another channel is picked, the way
// the Go side stops it.
let updTimers: ReturnType<typeof setTimeout>[] = [];
const updNow = () => {
  if (upd) return upd;
  const local = location.search.includes("makebuild");
  const gone = !local && location.search.includes("prgone");
  const fails = location.search.includes("listfails");
  upd = {
    version: local ? "0.3.0-local" : "0.3.0-pr29.db33a28",
    commit: local ? "" : "a1b2c3d4e5f6",
    channel: local ? "" : "pr-18",
    off: location.search.includes("updatesoff")
      ? "This build has no update key yet, so it cannot tell a build of ours from anybody else's."
      : "",
    channels: fails ? [] : gone ? updChannels.filter((c) => c.id !== "pr-18") : updChannels,
    picked: fails ? "pr-18" : "",
    follows: local || gone || fails ? "" : "pr-18",
    gone: gone ? "pr-18" : "",
    // ?unbuilt: a push to the channel whose build has not come yet.
    building: location.search.includes("unbuilt") ? "6ceea6d1f2a3" : "",
    // A channel gone is the phase gone, whatever else, the way settle in
    // updates.go keeps it.
    phase: gone ? "gone" : local ? "" : fails ? "failed" : "current",
    next: "",
    nextName: "",
    nextCommit: "",
    checked: local ? "0001-01-01T00:00:00Z" : new Date(Date.now() - 7 * 60_000).toISOString(),
    written: 0,
    total: 0,
    problem: fails ? "The channel list answered 404 Not Found." : "",
  };
  return upd;
};
const updSend = () => updListeners.forEach((fn) => fn({ data: { ...upd } }));
const updFetch = (channel: string) => {
  updTimers.forEach((t) => clearTimeout(t));
  updTimers = [];
  // A list that cannot be read fails the check and says nothing about the
  // channel, the way checkOnce in updates.go does.
  if (location.search.includes("listfails")) {
    Object.assign(upd, { phase: "checking" });
    updSend();
    updTimers.push(setTimeout(() => {
      Object.assign(upd, { phase: "failed", problem: "The channel list answered 404 Not Found.", checked: new Date().toISOString() });
      updSend();
    }, 80));
    return;
  }
  // Following nothing, a build made by make before a channel is picked,
  // has nothing to look for, the way check in updates.go does.
  if (!channel) return;
  const ch = upd.channels.find((c: any) => c.id === channel);
  if (!ch) {
    Object.assign(upd, { phase: "gone", gone: channel, follows: "", next: "", written: 0, total: 0 });
    updSend();
    return;
  }
  Object.assign(upd, { follows: ch.id, gone: "", next: "", nextName: "", written: 0, total: 0 });
  // A check that finds nothing is over at once, the way the real one is
  // when the list is cached, which is what the page has to hold on to.
  upd.phase = "checking";
  updSend();
  if (ch.version === upd.version) {
    updTimers.push(setTimeout(() => {
      Object.assign(upd, { phase: "current", next: "", checked: new Date().toISOString() });
      updSend();
    }, 80));
    return;
  }
  updTimers.push(setTimeout(() => {
    Object.assign(upd, {
      phase: "downloading",
      next: ch.version,
      nextName: ch.name,
      nextCommit: "9f8e7d6c5b4a",
      total: 46e6,
      written: 0,
      checked: new Date().toISOString(),
    });
    updSend();
    const step = () => {
      upd.written = Math.min(upd.total, upd.written + 4.6e6);
      if (upd.written >= upd.total) upd.phase = "ready";
      else updTimers.push(setTimeout(step, 200));
      updSend();
    };
    updTimers.push(setTimeout(step, 200));
  }, 500));
};

type StubLink = { what: string; about: string; before?: string; reason: string };
const licence = {
  key: "",
  saved: false,
  about: "",
  link: null as StubLink | null,
};
// A key in a few words, the way DescribeLicence puts it, told apart by
// its last two letters, in hexadecimal as a key ID is.
const describeKey = (k: string) =>
  `Key ID ${[...k.slice(-2)].map((c) => c.charCodeAt(0).toString(16).padStart(2, "0")).join("").toUpperCase()}-0304-0506-0708`;
// What a link does with its key, linkOutcome in licence.go.
const linkOutcome = (k: string): StubLink => {
  if (k.endsWith("bad")) return { what: "refused", about: "", reason: "this key was not signed by Frame Fairy" };
  if (licence.saved && licence.key === k) return { what: "same", about: licence.about, reason: "" };
  const before = licence.saved ? licence.about : "";
  licence.key = k;
  licence.saved = true;
  licence.about = describeKey(k);
  if (!before) return { what: "unlocked", about: licence.about, reason: "" };
  return { what: "replaced", about: licence.about, before, reason: "" };
};
if (location.search.includes("link")) licence.link = linkOutcome("FF1-AQIDBAUGBwgJCgsMDQ4PEABBbm5h");
const licenceNow = () => ({ saved: licence.saved, about: licence.about, waiting: !!licence.link });
const licenceListeners = new Set<(ev: unknown) => void>();
(window as any).__unlockLink = (key: string) => {
  licence.link = linkOutcome(key);
  for (const fn of licenceListeners) fn({ data: null });
};

export const Events = {
  // The Go side sends a job event every second while work runs, and an
  // interface that re-subscribes to anything on every one of those events
  // never gets anything done. The growing mode sends them, so a test can
  // tell.
  On(name: string, fn: (ev: unknown) => void): () => void {
    // There is no menu bar here, so a probe presses Undo and Redo with
    // window.__menu("undo") and window.__menu("redo").
    if (name === "undo") {
      (window as any).__menu = (what: string) => fn({ data: what });
      return () => delete (window as any).__menu;
    }
    // Cmd+Q heard, with window.__quit("ask") or window.__quit("going").
    if (name === "quit") {
      (window as any).__quit = (what: string) => fn({ data: what });
      return () => delete (window as any).__quit;
    }
    // And Help, Acknowledgements with window.__help().
    if (name === "acknowledgements") {
      (window as any).__help = () => fn({ data: null });
      return () => delete (window as any).__help;
    }
    // The colour test in Help, with window.__colourTest().
    if (name === "colourtest") {
      (window as any).__colourTest = () => fn({ data: null });
      return () => delete (window as any).__colourTest;
    }
    if (name === "updates") {
      updListeners.add(fn);
      return () => updListeners.delete(fn);
    }
    // Check for Updates in the app menu, with window.__checkForUpdates().
    if (name === "show-updates") {
      (window as any).__checkForUpdates = () => {
        updFetch(updNow().picked || updNow().follows || updNow().channel);
        fn({ data: null });
      };
      return () => delete (window as any).__checkForUpdates;
    }
    if (name === "licence-link") {
      licenceListeners.add(fn);
      return () => licenceListeners.delete(fn);
    }
    if (name === "levels") {
      if (!location.search.includes("measuring")) return () => {};
      const timer = setInterval(() => {
        fn({ data: "/eps/ep.mp4" });
        if (measuredNow() >= fullLength - 0.01) clearInterval(timer);
      }, 500);
      return () => clearInterval(timer);
    }
    if (name !== "job") return () => {};
    // Every search reports the way the Go side reports one: about four
    // times a second while it runs, and once more when it ends. The job list
    // is only ever kept current by these events, so without them a search
    // called off stays in the interface's hands.
    const told = new Map<string, string>();
    const searchTimer = setInterval(() => {
      for (const job of [...fakeSearches().map(searchJob), ...hands().map(handJob), ...((window as any).__refused ?? [])] as any[]) {
        const s = { id: job.id };
        const key = `${job.state}`;
        if (job.state !== "running" && told.get(s.id) === key) continue;
        told.set(s.id, key);
        if (job.progress) {
          ((window as any).__heardSent ??= []).push({ at: Date.now() - ((window as any).__started ?? Date.now()), covered: job.progress.covered });
        }
        fn({ data: { job, event: job.progress ?? { kind: "idle", text: "", elapsed: 0 } } });
      }
    }, 250);
    if (location.search.includes("setup") || location.search.includes("models")) {
      const timer = setInterval(() => {
        if (installAt()) {
          fn({ data: { job: modelJob(), event: { kind: "progress", text: "fetching", elapsed: 1 } } });
        }
        if (llmAt()) {
          fn({ data: { job: llmJob(), event: { kind: "progress", text: "fetching", elapsed: 1 } } });
        }
      }, 300);
      return () => {
        clearInterval(timer);
        clearInterval(searchTimer);
      };
    }
    return () => clearInterval(searchTimer);
  },
};
