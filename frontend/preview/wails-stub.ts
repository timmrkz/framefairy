// Stands in for the Wails runtime so the interface can be looked at in a
// plain browser. Only for taking a picture of the layout.
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
    words: words(start, start + 25)
      .filter((w) =>
        segments.some((p) => (w.start + w.end) / 2 >= p.start && (w.start + w.end) / 2 < p.end),
      )
      .map((w) => ({ ...w, text: fixed()[said(w.start)] ?? w.text })),
    rejected: false,
    rendered: rendered ? "/tmp/out.mp4" : undefined,
    captionY: 300,
    captionYMoved: false,
    key: `clips.json/0${n}`,
    plan: "/eps/ep.framefairy/logs/clips.json",
    cropLefts: segments.map((p) => p.cropX),
  };
};

// Where each clip of the ordinary mode starts, so a cut can find the clip
// it was asked about and give the same one back.
const starts: Record<string, [number, string, boolean]> = {
  "01": [57, "Mein Arm ist zersprungen", true],
  "02": [400, "Der Typ vor mir auf einmal", false],
  "03": [902, "Warum ich nie wieder", false],
  "04": [1400, "Ein echtes Thema", false],
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
const captionCues = (id: string) => {
  const c = clipOf(id);
  // Each word also keeps when it starts in the episode, which is what a
  // caption moved by hand is kept against.
  const onClipClock: { start: number; end: number; text: string; said: number }[] = [];
  let offset = 0;
  for (const p of c.segments) {
    for (const w of c.words) {
      if (w.start < p.start - 0.02 || w.end > p.end + 0.02) continue;
      onClipClock.push({
        start: offset + (w.start - p.start),
        end: offset + (w.end - p.start),
        text: w.text,
        said: w.start,
      });
    }
    offset += p.end - p.start;
  }
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
      drawn.push({ start: from, end: to, text: part, said: w.said });
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
  lane: "transcribe",
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
// clock, so a probe can have one running while the other is not.
const llmAt = () => (window as any).__llmAt ?? 0;
const llmRunning = () => llmAt() > 0 && Date.now() - llmAt() < 4000;
const llmDone = () => llmAt() > 0 && Date.now() - llmAt() >= 4000;
const llmJob = () => ({
  id: "l1",
  episode: "",
  kind: "llm",
  label: "Gemma 4 26B A4B by Google",
  state: llmDone() ? "done" : "running",
  queued: "",
  lane: "work",
  progress: llmDone()
    ? undefined
    : {
        stage: "language model",
        text: "fetching",
        fraction: Math.min((Date.now() - llmAt()) / 4000, 1),
        remaining: Math.max(0, Math.round(4 - (Date.now() - llmAt()) / 1000)),
      },
});

export const Call = {
  ByName(name: string, ...args: unknown[]): Promise<unknown> {
    const method = name.split(".").pop();
    // A transcription that really grows, for testing that the first search
    // starts by itself the moment it reaches the end of the window.
    const started = ((window as any).__started ??= Date.now());
    const growing = location.search.includes("growing");
    const grown = Math.min(600 + ((Date.now() - started) / 1000) * 600, 14423);
    // A search that really runs and really finishes, for testing what the
    // workspace does the moment the first clips arrive.
    const found = location.search.includes("found");
    const paused = location.search.includes("paused");
    const carriedOn = () => !!(window as any).__carriedOn;
    const stopped = () => !!(window as any).__stopped;
    const askedAt = () => ((window as any).__planned ?? [])[0]?.wall ?? 0;
    // The search lasts six seconds and its clips land one at a time on the
    // way, the way the engine writes each one the moment it is framed. They
    // land in the order the model wrote them, which is not the order of the
    // episode, so what is new in the list is not always at its end.
    const searchFor = 6000;
    const landOrder = [3, 1, 7, 2, 12, 5, 4, 9, 6, 11, 8, 10];
    const searching = () => found && askedAt() > 0 && Date.now() - askedAt() < searchFor;
    const done = () => found && askedAt() > 0 && Date.now() - askedAt() >= searchFor;
    const landed = () => {
      if (!found || !askedAt()) return [] as number[];
      const since = Date.now() - askedAt();
      return landOrder.filter((_, k) => since >= 700 + k * 350);
    };
    const planJob = (state: string) => ({ id: "p1", episode: "/eps/ep.mp4", kind: "plan", label: "Find clips", state, result: "/eps/ep.framefairy/logs/clips.json", queued: "", lane: "work", progress: state === "running" ? { stage: "plan", text: "Finding clips", fraction: 0.4, remaining: 60 } : undefined });
    switch (method) {
      case "Version":
        return Promise.resolve("0.1.0");
      case "Platform":
        return Promise.resolve(location.search.includes("linux") ? "linux" : "darwin");
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
          installed: !fresh || modelDone(),
        };
        const planner = (window as any).__planner ?? "";
        const key = !!(window as any).__key;
        // Two models from two houses, so the choice the interface has to put
        // is a real one, and so the memory below has something to say. The
        // small one fits any machine, the big one fits none of the ones
        // the harness pretends to be.
        const language = [
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
        ];
        return Promise.resolve({
          speech: [model],
          hasSpeech: model.installed,
          language,
          memory: 34359738368,
          planner: fresh ? planner : "local",
          hasKey: fresh ? key : true,
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
      case "InstallLanguageModel":
        (window as any).__llmAt ??= Date.now();
        return Promise.resolve(llmJob());
      case "WarmModel":
        // A probe reads which windows the model was loaded for.
        ((window as any).__warmed ??= []).push(args.slice(1));
        return Promise.resolve(null);
      case "ChoosePlanner":
        (window as any).__planner = args[0];
        return Promise.resolve(null);
      case "SaveAPIKey":
        (window as any).__key = !!String(args[0] ?? "").trim();
        return Promise.resolve(null);
      case "AddEpisodes":
        return Promise.resolve(null);
      case "Library":
        return Promise.resolve([
          { source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: true, covered: 14423, transcriptStale: false, plans: [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }], rendered: 1, previews: 0, work: true, looked: true },
          { source: "/eps/zwei.mp4", name: "Folge 12, die lange Nacht", size: 1, modified: "", missing: false, transcribed: false, covered: 900, transcriptStale: false, plans: [], rendered: 0, previews: 0, work: true, looked: true },
        ]);
      case "Episode":
        if (found) {
          return Promise.resolve({ source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: true, covered: 14423, transcriptStale: false, plans: landed().length ? [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }] : [], rendered: 0, previews: 0, work: true, looked: askedAt() > 0 });
        }
        if (growing) {
          return Promise.resolve({ source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: false, covered: grown, transcriptStale: false, plans: [], rendered: 0, previews: 0, work: true, looked: false });
        }
        if (location.search.includes("transcribing")) {
          return Promise.resolve({ source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: false, covered: 1200, transcriptStale: false, plans: [], rendered: 0, previews: 0, work: true, looked: true });
        }
        // What pausing leaves behind: clips already found, the episode read
        // only part way, and nothing reading the rest. Until the mark in the
        // clip list head stayed for it, this state had no way out.
        if (paused) {
          return Promise.resolve({ source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: false, covered: 4000, transcriptStale: false, plans: [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }], rendered: 0, previews: 0, work: true, looked: true });
        }
        return Promise.resolve({ source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: true, covered: 14423, transcriptStale: false, plans: [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }], rendered: 1, previews: 0, work: true, looked: true });
      // Every search the interface asks for, so a test can see the first one
      // start by itself.
      case "Plan": {
        const asked = ((window as any).__planned ??= []);
        asked.push({ at: Date.now() - started, wall: Date.now(), req: args[1] });
        return Promise.resolve({ id: "p" + asked.length, episode: "/eps/ep.mp4", kind: "plan", label: "Find clips", state: "running", queued: "", lane: "work" });
      }
      case "Training":
        return Promise.resolve({ dir: "/Users/tim/.framefairy/training", plans: 14, decisions: 62 });
      case "ClearTraining":
        return Promise.resolve(null);
      case "Source":
        return Promise.resolve({ duration: 14423, width: 1920, height: 1080, cropWidth: 608, cropHeight: 1080 });
      case "Clips":
        if (found) {
          const there = new Set(landed());
          return Promise.resolve(
            Array.from({ length: 12 }, (_, i) => clip(i + 1, 40 + i * 140, "Ein Moment " + (i + 1), false)).filter((_, i) => there.has(i + 1)),
          );
        }
        if (growing || location.search.includes("transcribing")) return Promise.resolve([]);
        return Promise.resolve([
          clip(1, 57, "Mein Arm ist zersprungen", true),
          clip(2, 400, "Der Typ vor mir auf einmal", false),
          clip(3, 902, "Warum ich nie wieder", false),
          clip(4, 1400, "Ein echtes Thema", false),
        ]);
      case "Coverage":
        if (found) {
          if (!landed().length) return Promise.resolve({ searched: [], free: [{ from: 0, to: 14423 }] });
          return Promise.resolve({ searched: [{ from: 0, to: 1800, plans: ["/eps/ep.framefairy/logs/clips.json"], clips: 12 }], free: [{ from: 1800, to: 14423 }] });
        }
        if (growing || location.search.includes("transcribing")) return Promise.resolve({ searched: [], free: [{ from: 0, to: 14423 }] });
        return Promise.resolve({
          searched: [{ from: 0, to: 1800, plans: ["/eps/ep.framefairy/logs/clips.json"], clips: 4 }, { from: 5400, to: 7200, plans: ["/eps/ep.framefairy/logs/clips-5400-7200.json"], clips: 6 }],
          free: [{ from: 1800, to: 5400 }, { from: 7200, to: 14423 }],
        });
      case "Captions":
        return Promise.resolve({
          captions: captionCues(String(args[1])),
          style: { font: face(), size: 0.062, lineHeight: 1.16, chosenSize: size(), bold: true, marginV: 0.156, marginH: 0.04, padX: 0.012, padY: 0.008, radius: 0.008, primary: textCss(), box: boxCss(), highlight: true, highlightColour: "#b4236f" },
        });
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
      case "GetSettings":
        return Promise.resolve({ ffmpeg: "", llmServer: "", llmModel: "", asrModel: "", planner: "local", apiModel: "", count: 12, min: 20, max: 30, highlightColour: "#b4236f", outputDir: "", captionY: 240, trainingDir: "" });
      case "RemoveClip": {
        const [, , id, gone] = args as [string, string, string, boolean];
        const n = Number(id);
        const made = clip(n, [57, 400, 902, 1400][n - 1] ?? 60, ["Mein Arm ist zersprungen", "Der Typ vor mir auf einmal", "Warum ich nie wieder", "Ein echtes Thema"][n - 1] ?? "Clip", n === 1);
        return Promise.resolve({ ...made, rejected: !!gone });
      }
      case "RemoveSearch":
        return Promise.resolve(null);
      // A caption moved by hand, kept the way the engine keeps it, against
      // the word and the edge. Below nought puts it back.
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
        return Promise.resolve({ done: true, clip: "clips.json/03" });
      }
      // The cuts inside a clip. They answer with the clip as it now is,
      // the way the Go side does, so the timeline draws where the edges
      // really landed rather than where the hand let go.
      case "CutClip": {
        // The last argument is toWords. Without it the edges stay where
        // they were put, which is how a cut lands on a frame rather than
        // on a word, so the stub has to honour it or the harness cannot
        // tell the two gestures apart.
        const [, , id, from, to, toWords] = args as
          [string, string, string, number, number, boolean];
        const [at] = starts[id] ?? [60];
        const said = words(at, at + 25);
        const [a, b] = toWords ? snapCut(said, from, to) : [from, to];
        return Promise.resolve(recut(id, (list) => applyCut(list, a, b)));
      }
      case "JoinCut": {
        const [, , id, at] = args as [string, string, string, number];
        return Promise.resolve(
          recut(id, (list) => {
            for (let i = 0; i + 1 < list.length; i++) {
              if (at >= list[i].end && at <= list[i + 1].start) {
                const joined = { ...list[i], end: list[i + 1].end };
                return [...list.slice(0, i), joined, ...list.slice(i + 2)];
              }
            }
            return list;
          }),
        );
      }
      case "MoveCut": {
        const [, , id, index, from, to, toWords] = args as
          [string, string, string, number, number, number, boolean];
        const [at] = starts[id] ?? [60];
        const [a, b] = toWords ? snapCut(words(at, at + 25), from, to) : [from, to];
        return Promise.resolve(
          recut(id, (list) => {
            if (index < 0 || index + 1 >= list.length) return list;
            const out = list.map((p) => ({ ...p }));
            out[index].end = a;
            out[index + 1].start = b;
            return out;
          }),
        );
      }
      case "Jobs": {
        const q = location.search;
        if (found) return Promise.resolve(searching() ? [planJob("running")] : done() ? [planJob("done")] : []);
        // Paused, and then asked to carry on: the transcription runs again,
        // which is what the mark in the clip list head has to bring about.
        if (paused) {
          return Promise.resolve(carriedOn()
            ? [{ id: "t1", episode: "/eps/ep.mp4", kind: "transcribe", label: "Transcribe", state: "running", queued: "", lane: "transcribe", progress: { stage: "asr", text: "Listening", fraction: 0.28, remaining: 420 } }]
            : []);
        }
        if (growing) {
          return Promise.resolve([
            { id: "t1", episode: "/eps/ep.mp4", kind: "transcribe", label: "Transcribe", state: "running", queued: "", lane: "transcribe", progress: { stage: "asr", text: "Listening", fraction: grown / 14423, remaining: 600 } },
          ]);
        }
        if (q.includes("transcribing")) {
          // Stopped, so the job is gone and only the saved transcript is
          // left. That is the moment the edge used to jump backwards.
          if (stopped()) return Promise.resolve([]);
          return Promise.resolve([
            {
              id: "t1",
              episode: "/eps/ep.mp4",
              kind: "transcribe",
              label: "Transcribe",
              state: "running",
              queued: "",
              lane: "transcribe",
              // Ahead of the 1200 the episode reports, because the saved
              // transcript is rewritten whole and lands seconds apart.
              progress: { stage: "asr", text: "Listening", fraction: 0.23, remaining: 276, covered: 1800 },
            },
          ]);
        }
        // A render running on the first clip, so the Render button has
        // work of its own to show.
        if (q.includes("rendering")) {
          return Promise.resolve([
            { id: "r1", episode: "/eps/ep.mp4", kind: "render", label: "Render", state: "running", result: "/eps/ep.framefairy/logs/clips.json", queued: "", lane: "work", progress: { stage: "render", text: "Burning in the captions", fraction: 0.58, remaining: 42 } },
          ]);
        }
        // Progress with no number to it comes with the busy job, so
        // ?unknown on its own means the same as ?busy&unknown.
        if (!q.includes("busy") && !q.includes("unknown")) return Promise.resolve([]);
        return Promise.resolve([
          {
            id: "j1",
            episode: "/eps/ep.mp4",
            kind: "plan",
            label: "Find clips",
            state: "running",
            queued: "",
            lane: "work",
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
            kind: "transcribe",
            label: "Transcription",
            state: "done",
            queued: "",
            lane: "transcribe",
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
        if (args[2]) (window as any).__text = hexToCss(String(args[2]), 1);
        if (args[3]) (window as any).__box = hexToCss(String(args[3]), Number(args[4]));
        return Promise.resolve(null);
      case "SetWord": {
        const text = String(args[4]).trim();
        if (!text) return Promise.reject(new Error("a word cannot be empty"));
        if (location.search.includes("refuse")) {
          return Promise.reject(new Error("there is no word at 0:57"));
        }
        fixed()[said(Number(args[3]))] = text;
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
        // The peaks come from the transcript, so while it is being made
        // there is a waveform up to where it got to and silence after.
        const edge = location.search.includes("transcribing") ? 1200 : 14423;
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
          if (t > edge) return -90;
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
      case "Words":
        if (location.search.includes("transcribing")) return Promise.resolve({ words: [], keepPause: 0.1 });
        return Promise.resolve({ words: words(Number(args[1]), Number(args[2])), keepPause: 0.1 });
      case "Still":
        // One file per second, so a test can see the frame follow the
        // playhead.
        return Promise.resolve(`/eps/still-${Math.round(Number(args[1]) || 0)}.jpg`);
      case "CancelJob":
        (window as any).__stopped = true;
        return Promise.resolve(null);
      case "Transcribe":
        (window as any).__carriedOn = true;
        return Promise.resolve({ id: "t1", episode: "/eps/ep.mp4", kind: "transcribe", label: "Transcribe", state: "running", queued: "", lane: "transcribe" });
      default:
        return Promise.resolve(null);
    }
  },
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
    if (name !== "job") return () => {};
    // A transcription that reports where it got to, well ahead of the saved
    // transcript, and that really stops when it is stopped. The job list is
    // only ever kept current by these events, so without them a cancelled
    // job stays in the interface's hands and the pause cannot be tested at
    // all.
    if (location.search.includes("transcribing")) {
      const timer = setInterval(() => {
        const gone = !!(window as any).__stopped;
        fn({
          data: {
            job: {
              id: "t1",
              episode: "/eps/ep.mp4",
              kind: "transcribe",
              label: "Transcribe",
              state: gone ? "cancelled" : "running",
              queued: "",
              lane: "transcribe",
              progress: gone
                ? undefined
                : { stage: "asr", text: "Listening", fraction: 0.23, remaining: 276, covered: 1800 },
            },
            event: { kind: "progress", text: "Listening", elapsed: 1 },
          },
        });
      }, 400);
      return () => clearInterval(timer);
    }
    if (location.search.includes("setup")) {
      const timer = setInterval(() => {
        if (installAt()) {
          fn({ data: { job: modelJob(), event: { kind: "progress", text: "fetching", elapsed: 1 } } });
        }
        if (llmAt()) {
          fn({ data: { job: llmJob(), event: { kind: "progress", text: "fetching", elapsed: 1 } } });
        }
      }, 300);
      return () => clearInterval(timer);
    }
    if (location.search.includes("found")) {
      // Reported the way the engine reports a search: what it is doing, how
      // far it is, and how many clips it has written, which is what tells
      // the interface to read the list again.
      const timer = setInterval(() => {
        const at = ((window as any).__planned ?? [])[0]?.wall ?? 0;
        if (!at) return;
        const since = Date.now() - at;
        const state = since < 6000 ? "running" : "done";
        const found = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11].filter((k) => since >= 700 + k * 350).length;
        const text = found ? `${found} of 12 found` : "Finding clips";
        const progress = { kind: "progress", stage: "plan", text, fraction: Math.min(since / 6000, 0.99), remaining: Math.max((6000 - since) / 1000, 0), found, elapsed: since / 1000, time: "" };
        fn({ data: { job: { id: "p1", episode: "/eps/ep.mp4", kind: "plan", label: "Find clips", state, result: "/eps/ep.framefairy/logs/clips.json", queued: "", lane: "work", progress: state === "running" ? progress : undefined }, event: progress } });
      }, 250);
      return () => clearInterval(timer);
    }
    if (!location.search.includes("growing")) return () => {};
    const started = ((window as any).__started ??= Date.now());
    const timer = setInterval(() => {
      const gone = !!(window as any).__stopped;
      const grown = Math.min(600 + ((Date.now() - started) / 1000) * 600, 14423);
      fn({
        data: {
          job: {
            id: "t1",
            episode: "/eps/ep.mp4",
            kind: "transcribe",
            label: "Transcribe",
            state: gone ? "cancelled" : "running",
            queued: "",
            lane: "transcribe",
            progress: { stage: "asr", text: "Listening", fraction: grown / 14423, remaining: 600, covered: grown },
          },
          event: { kind: "progress", text: "Listening", elapsed: 1 },
        },
      });
    }, 900);
    return () => clearInterval(timer);
  },
};
