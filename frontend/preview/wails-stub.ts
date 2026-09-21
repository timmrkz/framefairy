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

const clip = (n: number, start: number, title: string, rendered: boolean) => ({
  id: `0${n}`,
  slug: `clip-${n}`,
  basename: `0${n}_clip-${n}`,
  title,
  reason: "A short, complete memory of defending oneself using Judo.",
  duration: 24,
  start,
  end: start + 24,
  segments: [
    { start, end: start + 12, cropX: 420, moved: false },
    { start: start + 13, end: start + 25, cropX: 420, moved: false },
  ],
  words: words(start, start + 25),
  rejected: false,
  rendered: rendered ? "/tmp/out.mp4" : undefined,
  captionY: 300,
  captionYMoved: false,
  key: `clips.json/0${n}`,
  plan: "/eps/ep.framefairy/logs/clips.json",
  cropLefts: [420, 420],
});

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
      case "Jobs": {
        const q = location.search;
        if (found) return Promise.resolve(searching() ? [planJob("running")] : done() ? [planJob("done")] : []);
        if (growing) {
          return Promise.resolve([
            { id: "t1", episode: "/eps/ep.mp4", kind: "transcribe", label: "Transcribe", state: "running", queued: "", lane: "transcribe", progress: { stage: "asr", text: "Listening", fraction: grown / 14423, remaining: 600 } },
          ]);
        }
        if (q.includes("transcribing")) {
          return Promise.resolve([
            {
              id: "t1",
              episode: "/eps/ep.mp4",
              kind: "transcribe",
              label: "Transcribe",
              state: "running",
              queued: "",
              lane: "transcribe",
              progress: { stage: "asr", text: "Listening", fraction: 0.23, remaining: 276 },
            },
          ]);
        }
        if (!q.includes("busy")) return Promise.resolve([]);
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
        const buckets = Number(args[3]) || 900;
        const from = Number(args[1]) || 0;
        const to = Number(args[2]) || 14423;
        // The peaks come from the transcript, so while it is being made
        // there is a waveform up to where it got to and silence after.
        const edge = location.search.includes("transcribing") ? 1200 : 14423;
        return Promise.resolve(Array.from({ length: buckets }, (_, i) => (from + ((i + 0.5) * (to - from)) / buckets > edge ? -90 : -60 + 45 * Math.abs(Math.sin(i / 7)) * Math.abs(Math.cos(i / 31)))));
      }
      case "Words":
        if (location.search.includes("transcribing")) return Promise.resolve({ words: [], keepPause: 0.1 });
        return Promise.resolve({ words: words(Number(args[1]), Number(args[2])), keepPause: 0.1 });
      case "Still":
        // One file per second, so a test can see the frame follow the
        // playhead.
        return Promise.resolve(`/eps/still-${Math.round(Number(args[1]) || 0)}.jpg`);
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
      const grown = Math.min(600 + ((Date.now() - started) / 1000) * 600, 14423);
      fn({
        data: {
          job: {
            id: "t1",
            episode: "/eps/ep.mp4",
            kind: "transcribe",
            label: "Transcribe",
            state: "running",
            queued: "",
            lane: "transcribe",
            progress: { stage: "asr", text: "Listening", fraction: grown / 14423, remaining: 600 },
          },
          event: { kind: "progress", text: "Listening", elapsed: 1 },
        },
      });
    }, 900);
    return () => clearInterval(timer);
  },
};
