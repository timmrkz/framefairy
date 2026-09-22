// Typed calls into the Go side. The names match the methods of the FrameFairy
// service in main.go.
import { Call, Events } from "@wailsio/runtime";

const call = <T>(method: string, ...args: unknown[]): Promise<T> =>
  Call.ByName(`main.FrameFairy.${method}`, ...args) as Promise<T>;

// Where the training records are and how much there is of them.
export interface TrainingStatus {
  dir: string;
  plans: number;
  decisions: number;
}

export interface Settings {
  ffmpeg: string;
  llmServer: string;
  llmModel: string;
  asrModel: string;
  planner: "local" | "api";
  apiModel: string;
  count: number;
  min: number;
  max: number;
  // The colour of the pill behind the word being spoken, in the rendered
  // short. It belongs to the short.
  highlightColour: string;
  // The colour the app itself is picked out in. It belongs to the app, and
  // everything the app draws in its own colour follows it.
  appColour: string;
  outputDir: string;
  // The one folder the training records of every episode go in. Empty
  // means the default, ~/.framefairy/training.
  trainingDir: string;
  // Where the captions sit, as the distance from the bottom of a 1080x1920
  // frame. One place for every clip of every episode.
  captionY: number;
}

// One of the faces the captions can be written in, built into the program.
export interface CaptionFont {
  name: string;
  about: string;
  file: string;
}

export interface Check {
  name: string;
  ok: boolean;
  detail: string;
}

export interface PlanSummary {
  path: string;
  name: string;
  from: number;
  to: number;
  clips: number;
  model: string;
  modified: string;
}

export interface EpisodeStatus {
  source: string;
  name: string;
  size: number;
  modified: string;
  missing: boolean;
  transcribed: boolean;
  covered: number;
  transcriptStale: boolean;
  plans: PlanSummary[] | null;
  rendered: number;
  previews: number;
  // Whether the episode has a work folder at all. Without one there is
  // nothing of it to keep or to throw away.
  work: boolean;
  // Whether anyone has ever searched this episode for clips. It stays true
  // when the clips are removed again.
  looked: boolean;
}

export interface Word {
  start: number;
  end: number;
  text: string;
}

export interface Segment {
  start: number;
  end: number;
  cropX: number | null;
  moved: boolean;
}

export interface ClipView {
  id: string;
  slug: string;
  basename: string;
  title: string;
  reason: string;
  duration: number;
  start: number;
  end: number;
  segments: Segment[];
  words: Word[] | null;
  rejected: boolean;
  rendered?: string;
  preview?: string;
  // Where the caption line of this clip sits, as the distance from the
  // bottom of a 1080x1920 frame, when it was placed by hand.
  captionY: number;
  captionYMoved: boolean;
}

// One line of a caption, the way the render lays it out.
export interface CaptionLine {
  words: Word[];
}

export interface CaptionCue {
  start: number;
  end: number;
  lines: CaptionLine[];
}

// The caption look, with every measure as a share of the frame height.
export interface CaptionStyle {
  font: string;
  // The em size to draw at, as a share of the frame height, and the space
  // from one line to the next as a share of that size.
  size: number;
  lineHeight: number;
  // The size the caption style asks for, which is what the control shows.
  chosenSize: number;
  bold: boolean;
  marginV: number;
  marginH: number;
  padX: number;
  padY: number;
  radius: number;
  primary: string;
  box: string;
  highlight: boolean;
  highlightColour: string;
}

export interface CaptionsView {
  captions: CaptionCue[] | null;
  style: CaptionStyle;
}

// A stretch of an episode, in seconds. A searched stretch also says which
// plans cover it and how many clips they hold, so it can be let go of.
export interface WindowView {
  from: number;
  to: number;
  plans?: string[];
  clips?: number;
}

// Where the model has already looked, and what is left to look at.
export interface CoverageView {
  searched: WindowView[];
  free: WindowView[];
}

export interface SourceView {
  duration: number;
  width: number;
  height: number;
  cropWidth: number;
  cropHeight: number;
  // The episode's frame rate. One step of the arrow keys on the clip
  // timeline is one frame of it.
  fps: number;
}

export interface ClipEntry extends ClipView {
  key: string;
  plan: string;
  cropLefts: number[];
}

export interface PlanView {
  summary: PlanSummary;
  clips: ClipView[] | null;
}

export interface EngineEvent {
  kind: string;
  stage?: string;
  text: string;
  fraction: number;
  remaining: number;
  // The second of the episode the work has reached, where that means
  // anything. A transcription sets it on every chunk it hears.
  covered?: number;
  duration?: number;
  elapsed: number;
  time: string;
}

export type JobState = "queued" | "running" | "done" | "failed" | "cancelled";

export interface Job {
  id: string;
  episode: string;
  kind: "transcribe" | "plan" | "render" | "model" | "llm";
  label: string;
  state: JobState;
  error?: string;
  result?: string;
  last?: EngineEvent;
  progress?: EngineEvent;
  queued: string;
  lane: "transcribe" | "work";
}

export interface PlanRequest {
  From: number;
  To: number;
  Count: number;
  Min: number;
  Max: number;
  Replan: boolean;
}

export interface RenderRequest {
  Plan: string;
  Clips: string[];
  Preview: boolean;
}

// One speech model that can be installed, as the setup and the settings
// see it. The engine holds the list, so a new model is one entry there and
// nothing here changes.
export interface SpeechModel {
  name: string;
  title: string;
  about: string;
  languages: string;
  // What the download costs and what it costs on disk once it is there,
  // in bytes. Both are said before anything starts.
  download: number;
  unpacked: number;
  url: string;
  // The one the app offers when nobody has chosen.
  recommended: boolean;
  installed: boolean;
}

// One model that can find clips. A model runs from memory, so what decides
// whether a machine can have one is not the download but what it takes to
// run.
export interface LanguageModel {
  name: string;
  title: string;
  // Who made it. A list of models is read by maker first: Google's Gemma
  // and Meta's Llama are the same kind of thing from two houses.
  maker: string;
  about: string;
  // Bytes to fetch, and bytes of memory to run.
  download: number;
  needs: number;
  url: string;
  recommended: boolean;
  installed: boolean;
  // What this machine can do with it: "fits", "tight", "too big", or
  // "unknown" where the machine would not say how much memory it has.
  fit: "fits" | "tight" | "too big" | "unknown";
}

// One row of a list of models, as the list draws it. Speech models and
// language models are the same kind of thing on screen, so they are one
// component and each screen says what to put in the row.
export interface ModelRow {
  name: string;
  title: string;
  about: string;
  // What it costs, as one line.
  cost: string;
  installed: boolean;
  // A word about this model on this machine, where there is one.
  note?: string;
  // Whether that word is a warning rather than a fact.
  warn?: boolean;
}

// What a new copy of the app still needs before it can make a short. Two
// things are needed and only one of them is a question: speech is always
// local and finding clips is a choice between an API key and a local
// model.
export interface SetupState {
  speech: SpeechModel[];
  hasSpeech: boolean;
  language: LanguageModel[];
  // What this machine has, in bytes, or zero where it would not say.
  memory: number;
  planner: "local" | "api" | "";
  // Whether a key can be found. It never carries the key itself.
  hasKey: boolean;
  hasLocalModel: boolean;
  // Whether anybody has answered the one question. Until then the planner
  // is a default rather than a decision.
  chosen: boolean;
  ready: boolean;
  // The job of a model install in hand, so a window opened again while one
  // runs picks it up rather than starting a second.
  installing?: string;
}

export const api = {
  version: () => call<string>("Version"),
  platform: () => call<string>("Platform"),
  // Where macOS put the window's own furniture, in whole CSS pixels, or
  // zeros where the system draws its own title bar.
  chrome: () => call<Chrome>("Chrome"),
  // The clip of an episode that was last worked on, so opening it again
  // opens on the same one. It is the clip's key, or an empty string.
  chosenClip: (path: string) => call<string>("ChosenClip", path),
  chooseClip: (path: string, key: string) => call<void>("ChooseClip", path, key),
  getSettings: () => call<Settings>("GetSettings"),
  training: () => call<TrainingStatus>("Training"),
  clearTraining: () => call<void>("ClearTraining"),
  saveSettings: (s: Settings) => call<void>("SaveSettings", s),
  // How many clips a search looks for and how long they may be. They are
  // set in the workspace, beside the episode they are about, and kept for
  // the next one.
  setSearch: (count: number, min: number, max: number) =>
    call<void>("SetSearch", count, min, max),
  checkSetup: () => call<Check[]>("CheckSetup"),
  // What a new copy of the app still needs, and the three ways to answer
  // it. Setup only reads, the other three change something.
  setup: () => call<SetupState>("Setup"),
  installSpeechModel: (name: string) => call<Job>("InstallSpeechModel", name),
  installLanguageModel: (name: string) => call<Job>("InstallLanguageModel", name),
  saveAPIKey: (key: string) => call<void>("SaveAPIKey", key),
  choosePlanner: (planner: "local" | "api") => call<void>("ChoosePlanner", planner),
  library: () => call<EpisodeStatus[]>("Library"),
  episode: (path: string) => call<EpisodeStatus>("Episode", path),
  addEpisodes: () => call<string[] | null>("AddEpisodes"),
  removeEpisode: (path: string, deleteWork: boolean) =>
    call<void>("RemoveEpisode", path, deleteWork),
  source: (path: string) => call<SourceView>("Source", path),
  clips: (path: string) => call<ClipEntry[]>("Clips", path),
  coverage: (path: string, least: number) => call<CoverageView>("Coverage", path, least),
  // Gives a stretch of an episode back: the clips in it go and the model
  // may read it again. It answers with how many clips went.
  removeSearch: (path: string, from: number, to: number) =>
    call<number>("RemoveSearch", path, from, to),
  captions: (plan: string, clip: string) => call<CaptionsView>("Captions", plan, clip),
  fonts: () => call<CaptionFont[]>("Fonts"),
  waveform: (path: string, from: number, to: number, buckets: number) =>
    call<number[]>("Waveform", path, from, to, buckets),
  transcribe: (path: string) => call<Job>("Transcribe", path),
  plan: (path: string, req: PlanRequest) => call<Job>("Plan", path, req),
  render: (path: string, req: RenderRequest) => call<Job>("Render", path, req),
  still: (path: string, at: number, width: number) => call<string>("Still", path, at, width),
  words: (path: string, from: number, to: number) =>
    call<{ words: Word[] | null; keepPause: number }>("Words", path, from, to),
  trimClip: (path: string, plan: string, clip: string, start: number, end: number) =>
    call<ClipEntry>("TrimClip", path, plan, clip, start, end),
  // The cuts inside a clip: the stretches it leaves out. Making one, moving
  // one and putting one back. The edges land on words, so what comes back
  // is what to draw, never what was asked for.
  // toWords puts the edges on the words around them, which is what nearly
  // every cut wants. Without it they stay exactly where the hand put them,
  // a frame at a time, and a cut may then stop inside a word.
  cutClip: (path: string, plan: string, clip: string, from: number, to: number, toWords: boolean) =>
    call<ClipEntry>("CutClip", path, plan, clip, from, to, toWords),
  joinCut: (path: string, plan: string, clip: string, at: number) =>
    call<ClipEntry>("JoinCut", path, plan, clip, at),
  moveCut: (
    path: string,
    plan: string,
    clip: string,
    index: number,
    from: number,
    to: number,
    toWords: boolean,
  ) => call<ClipEntry>("MoveCut", path, plan, clip, index, from, to, toWords),
  setWord: (path: string, plan: string, clip: string, start: number, text: string) =>
    call<ClipEntry>("SetWord", path, plan, clip, start, text),
  clipPlayed: (plan: string, clip: string) => call<void>("ClipPlayed", plan, clip),
  removeClip: (path: string, plan: string, clip: string, removed: boolean) =>
    call<ClipEntry>("RemoveClip", path, plan, clip, removed),
  setCrop: (path: string, plan: string, clip: string, at: number, left: number) =>
    call<ClipEntry>("SetCrop", path, plan, clip, at, left),
  resetCrop: (path: string, plan: string, clip: string, at: number) =>
    call<ClipEntry>("ResetCrop", path, plan, clip, at),
  setCaptionStyle: (path: string, plan: string, font: string, size: number) =>
    call<void>("SetCaptionStyle", path, plan, font, size),
  // Where the captions sit, for every clip of every episode. Dragging the
  // box in the video preview saves it, so the next video starts there too.
  setCaptionsHeight: (path: string, y: number) => call<void>("SetCaptionsHeight", path, y),
  resetCaptionsHeight: (path: string) => call<void>("ResetCaptionsHeight", path),
  jobs: () => call<Job[]>("Jobs"),
  cancelJob: (id: string) => call<void>("CancelJob", id),
  clearJobs: () => call<void>("ClearJobs"),
  readPlan: (path: string) => call<PlanView>("ReadPlan", path),
  reveal: (path: string) => call<void>("Reveal", path),
};

export interface JobUpdate {
  job: Job;
  event?: EngineEvent;
}

export function onJob(fn: (u: JobUpdate) => void): () => void {
  return Events.On("job", (ev) => fn(ev.data as JobUpdate));
}

// Where macOS put the window's own furniture, in whole CSS pixels. All
// zeros means the system draws its own title bar, which is every other
// system, and then the stylesheet keeps its own numbers.
export interface Chrome {
  // The title bar's height, which is the height of the bar the app draws.
  bar: number;
  // The left edge of the first window button and the right edge of the
  // last, so the name of what is on screen starts after them.
  left: number;
  right: number;
  // The middle of the window buttons, from the top of the page.
  middle: number;
}

// The window is told again whenever the answer changes: a resize, either
// way through fullscreen, another display, another scale, light or dark,
// or back from the Dock.
export function onChrome(fn: (c: Chrome) => void): () => void {
  return Events.On("chrome", (ev) => fn(ev.data as Chrome));
}

export function onEpisodeChanged(fn: (path: string) => void): () => void {
  return Events.On("episode", (ev) => fn(ev.data as string));
}

export function mediaURL(path: string): string {
  return `/media/?path=${encodeURIComponent(path)}`;
}

export function clock(seconds: number): string {
  if (!isFinite(seconds) || seconds < 0) seconds = 0;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  const mm = String(m).padStart(h ? 2 : 1, "0");
  const ss = String(s).padStart(2, "0");
  return h ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

// What a machine can do with a model, in words, and whether that is a
// warning. One place, because the setup and the settings say the same
// thing and two wordings of one fact read as two facts.
export function fitNote(fit: LanguageModel["fit"]): { note: string; warn: boolean } {
  switch (fit) {
    case "fits":
      return { note: "Fits this machine", warn: false };
    case "tight":
      return { note: "Tight on this machine, and it will slow everything else down", warn: true };
    case "too big":
      return { note: "Too big for this machine", warn: true };
    default:
      return { note: "", warn: false };
  }
}

// A size in bytes the way a download is always quoted: metric, a thousand
// bytes to the kilobyte, and never more than one decimal.
export function size(bytes: number): string {
  if (!isFinite(bytes) || bytes <= 0) return "";
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  return `${Math.round(bytes / 1e6)} MB`;
}

// Memory, which is counted the other way. A machine sold as 32 GB has
// 34,359,738,368 bytes of it, so quoting that as 34.4 GB would be true and
// unrecognisable: nobody would find it on the box their machine came in.
// A download counts in thousands and memory counts in 1024s, because that
// is how each of them is quoted everywhere else.
export function memorySize(bytes: number): string {
  if (!isFinite(bytes) || bytes <= 0) return "";
  const gb = bytes / (1 << 30);
  if (gb < 1) return `${Math.round(bytes / (1 << 20))} MB`;
  return `${gb >= 10 ? Math.round(gb) : Math.round(gb * 10) / 10} GB`;
}

export function errorText(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  try {
    return JSON.stringify(err);
  } catch {
    return String(err);
  }
}

// Where the captions may sit, as the distance from the bottom of a
// 1080x1920 frame. The same steps the engine snaps to.
// Where the captions sit when nobody has placed them, as the distance from
// the bottom of a 1080x1920 frame. The same number as DefaultStyle in the
// engine.
// What the captions look like when nobody has changed them. The same
// values as DefaultStyle in the engine, so putting them back here and
// rendering with no settings at all come to the same picture.
export const captionFontDefault = "Inter Black";
export const captionSizeDefault = 96;
export const captionYDefault = 300;
export const captionYStep = 40;
export const captionYMin = 120;
export const captionYMax = 1560;

export function snapCaptionY(y: number): number {
  if (!isFinite(y)) return 300;
  const step = Math.round(y / captionYStep) * captionYStep;
  return Math.max(captionYMin, Math.min(step, captionYMax));
}

// The same snapping the engine applies when a clip edge is moved: a little
// before the nearest word, never into the word before it.
export function snapStart(words: Word[], at: number, keepPause: number): number {
  if (!words.length) return at;
  let best = 0;
  words.forEach((w, i) => {
    if (Math.abs(w.start - at) < Math.abs(words[best].start - at)) best = i;
  });
  let edge = words[best].start - keepPause;
  if (best > 0) edge = Math.max(edge, words[best - 1].end);
  return Math.max(0, edge);
}

export function snapEnd(words: Word[], at: number, keepPause: number): number {
  if (!words.length) return at;
  let best = 0;
  words.forEach((w, i) => {
    if (Math.abs(w.end - at) < Math.abs(words[best].end - at)) best = i;
  });
  let edge = words[best].end + keepPause;
  if (best + 1 < words.length) edge = Math.min(edge, words[best + 1].start);
  return edge;
}

// The same snapping the engine applies to a cut, so the block the hand
// draws is the block the render will leave out. It is not the trim's
// snapping: a cut takes words away, so it swallows every word it touches
// and then leaves keepPause of air on each side that stays. A cut over a
// pause takes the whole pause, a cut over speech takes whole words.
// Where a cut goes when a double-click says take one out here. Three
// things decide it.
//
// It lands on frames. A double-click says where, exactly, and growing it
// out to the words either side would put it somewhere else, which is the
// whole complaint about cuts landing on words.
//
// It is as wide as it is asked to be, and what asks is the timeline, which
// works that width out from a fixed number of pixels at whatever zoom it
// is on. A cut nobody can see is a cut nobody can change. Below the least
// a cut may be it is held open at that, so a timeline zoomed in far enough
// that those pixels are worth less than the least still takes a stretch
// out rather than doing nothing.
//
// And it stays inside the piece it falls in, with room left on both sides,
// because a piece squeezed to nothing is a clip the engine refuses. A
// click outside every piece, or in a piece with no room to spare, makes no
// cut at all rather than a cut somewhere else.
export function cutAt(
  pieces: { start: number; end: number }[],
  at: number,
  wide: number,
  least: number,
  frame: number,
): [number, number] | null {
  const on = (t: number) => (frame > 0 ? Math.round(t / frame) * frame : t);
  const piece = pieces.find((p) => at >= p.start && at < p.end);
  if (!piece) return null;
  const low = piece.start + least;
  const high = piece.end - least;
  if (high - low < least) return null;
  // As wide as it was asked to be, never less than the least a cut may be,
  // never more than the piece has room for.
  let width = Math.min(Math.max(wide, least), high - low);
  // And a whole number of frames wide, rounded up. Rounding the two ends
  // on their own instead brings them closer than they were asked to be,
  // and at 25 frames a second a frame is 40 milliseconds against a least
  // of 50, so two ends far enough apart came back 40 apart and the engine
  // refused the cut: a double-click that did nothing at all, at some zooms
  // and not others.
  if (frame > 0) width = Math.min(Math.ceil(width / frame) * frame, high - low);
  let a = on(at - width / 2);
  let b = a + width;
  // Inside the piece, keeping the width. An end held at the piece's own
  // edge is no longer on a frame, which is right: that edge is where the
  // engine put it.
  if (a < low) {
    a = low;
    b = a + width;
  }
  if (b > high) {
    b = high;
    a = Math.max(b - width, low);
  }
  if (b - a < least) return null;
  return [a, b];
}

export function snapCut(
  words: Word[],
  from: number,
  to: number,
  keepPause: number,
): [number, number] {
  let swallowedFrom = Infinity;
  let swallowedTo = -Infinity;
  for (const w of words) {
    if (w.end > from && w.start < to) {
      swallowedFrom = Math.min(swallowedFrom, w.start);
      swallowedTo = Math.max(swallowedTo, w.end);
    }
  }
  let start = Math.min(from, swallowedFrom);
  let end = Math.max(to, swallowedTo);

  let before = -Infinity;
  let after = Infinity;
  for (const w of words) {
    if (w.end <= start) before = Math.max(before, w.end);
    if (w.start >= end) after = Math.min(after, w.start);
  }
  if (before !== -Infinity) start = Math.min(before + keepPause, swallowedFrom);
  if (after !== Infinity) end = Math.max(after - keepPause, swallowedTo);
  return [Math.max(0, start), end];
}
