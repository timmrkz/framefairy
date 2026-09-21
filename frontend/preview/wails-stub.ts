// Stands in for the Wails runtime so the interface can be looked at in a
// plain browser. Only for taking a picture of the layout.
const words = (from: number, to: number) => {
  const list: { start: number; end: number; text: string }[] = [];
  const sample = "Und da war irgendein Typ auf einmal vor mir und ich habe mich gewehrt weil das ein echtes Thema war".split(" ");
  let at = from;
  let i = 0;
  while (at < to) {
    const len = 0.28 + (i % 5) * 0.08;
    list.push({ start: at, end: at + len, text: sample[i % sample.length] });
    at += len + 0.06;
    i++;
  }
  return list;
};

type Piece = { start: number; end: number; cropX: number; moved: boolean };

// A clip's pieces, once a cut has changed them. The Go side keeps them in
// the plan, so the preview keeps them here, or a cut would come undone the
// moment the clip list is read again.
const held = (): Record<string, Piece[]> => ((window as any).__pieces ??= {});

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

// Taking a stretch out of a set of pieces. A piece the cut straddles becomes
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
    words: words(start, start + 25).filter((w) =>
      segments.some((p) => (w.start + w.end) / 2 >= p.start && (w.start + w.end) / 2 < p.end),
    ),
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

const captionWords = [
  { start: 0.2, end: 0.6, text: "Und" },
  { start: 0.6, end: 1.0, text: "da" },
  { start: 1.0, end: 1.6, text: "war" },
  { start: 1.6, end: 2.4, text: "irgendein" },
];

export const Call = {
  ByName(name: string, ...args: unknown[]): Promise<unknown> {
    const method = name.split(".").pop();
    // A transcription that really grows, for testing that the first search
    // starts by itself the moment it reaches the end of the chosen stretch.
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
    const searching = () => found && askedAt() > 0 && Date.now() - askedAt() < 2500;
    const done = () => found && askedAt() > 0 && Date.now() - askedAt() >= 2500;
    const planJob = (state: string) => ({ id: "p1", episode: "/eps/ep.mp4", kind: "plan", label: "Find clips", state, result: "/eps/ep.framefairy/logs/clips.json", queued: "", lane: "work", progress: state === "running" ? { stage: "plan", text: "Reading the transcript", fraction: 0.4, remaining: 60 } : undefined });
    switch (method) {
      case "Version":
        return Promise.resolve("0.1.0");
      case "Library":
        return Promise.resolve([
          { source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: true, covered: 14423, transcriptStale: false, plans: [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }], rendered: 1, previews: 0, work: true, looked: true },
          { source: "/eps/zwei.mp4", name: "Folge 12, die lange Nacht", size: 1, modified: "", missing: false, transcribed: false, covered: 900, transcriptStale: false, plans: [], rendered: 0, previews: 0, work: true, looked: true },
        ]);
      case "Episode":
        if (found) {
          return Promise.resolve({ source: "/eps/ep.mp4", name: "Mein Arm ist zersprungen", size: 1, modified: "", missing: false, transcribed: true, covered: 14423, transcriptStale: false, plans: done() ? [{ path: "/eps/ep.framefairy/logs/clips.json", name: "clips.json", from: 0, to: 1800, clips: 12, model: "gemma", modified: "" }] : [], rendered: 0, previews: 0, work: true, looked: askedAt() > 0 });
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
      // Every search this window asks for, so a test can see the first one
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
          if (!done()) return Promise.resolve([]);
          return Promise.resolve(Array.from({ length: 12 }, (_, i) => clip(i + 1, 40 + i * 140, "Ein Moment " + (i + 1), false)));
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
          if (!done()) return Promise.resolve({ searched: [], free: [{ from: 0, to: 14423 }] });
          return Promise.resolve({ searched: [{ from: 0, to: 1800, plans: ["/eps/ep.framefairy/logs/clips.json"], clips: 12 }], free: [{ from: 1800, to: 14423 }] });
        }
        if (growing || location.search.includes("transcribing")) return Promise.resolve({ searched: [], free: [{ from: 0, to: 14423 }] });
        return Promise.resolve({
          searched: [{ from: 0, to: 1800, plans: ["/eps/ep.framefairy/logs/clips.json"], clips: 4 }, { from: 5400, to: 7200, plans: ["/eps/ep.framefairy/logs/clips-5400-7200.json"], clips: 6 }],
          free: [{ from: 1800, to: 5400 }, { from: 7200, to: 14423 }],
        });
      case "Captions":
        return Promise.resolve({
          captions: [{ start: 0, end: 4, lines: [{ words: captionWords.slice(0, 2) }, { words: captionWords.slice(2) }] }],
          style: { font: "Inter Black", size: 0.062, lineHeight: 1.16, chosenSize: 100, bold: true, marginV: 0.156, marginH: 0.04, padX: 0.012, padY: 0.008, radius: 0.008, primary: "#ffffff", box: "rgba(0,0,0,0.85)", highlight: true, highlightColour: "#b4236f" },
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
      // Where macOS put the window's own furniture. Zeros stand for the
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
              text: "Reading the transcript",
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
      case "Waveform": {
        const from = Number(args[1]) || 0;
        const to = Number(args[2]) || 14423;
        // Never more buckets than there are measurements, the way the Go
        // side answers. Without this the stub hands back five buckets
        // carrying the same ten milliseconds and the window has no way to
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
  // The Go side sends a job event every second while work runs, and a
  // window that re-subscribes to anything on every one of those events
  // never gets anything done. The growing mode sends them, so a test can
  // tell.
  On(name: string, fn: (ev: unknown) => void): () => void {
    if (name !== "job") return () => {};
    // A transcription that reports where it got to, well ahead of the saved
    // transcript, and that really stops when it is stopped. The job list is
    // only ever kept current by these events, so without them a cancelled
    // job stays in the window's hands and the pause cannot be tested at all.
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
    if (location.search.includes("found")) {
      const timer = setInterval(() => {
        const at = ((window as any).__planned ?? [])[0]?.wall ?? 0;
        if (!at) return;
        const state = Date.now() - at < 2500 ? "running" : "done";
        fn({ data: { job: { id: "p1", episode: "/eps/ep.mp4", kind: "plan", label: "Find clips", state, result: "/eps/ep.framefairy/logs/clips.json", queued: "", lane: "work", progress: state === "running" ? { stage: "plan", text: "Reading the transcript", fraction: 0.4, remaining: 60 } : undefined }, event: { kind: "progress", text: "Reading", elapsed: 1 } } });
      }, 500);
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
