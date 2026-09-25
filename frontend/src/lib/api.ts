// Typed calls into the Go side. The names match the methods of the FrameFairy
// service in main.go.
import { Call, Events } from "@wailsio/runtime";
import type { RoomView } from "./room";
export type { RoomView } from "./room";

const call = <T>(method: string, ...args: unknown[]): Promise<T> =>
  Call.ByName(`main.FrameFairy.${method}`, ...args) as Promise<T>;

// Where the training records are and how much there is of them.
// One piece of other people's work the app is made of or brings with it,
// and its licence. The texts are asked for one at a time, by name.
export interface Notice {
  name: string;
  version: string;
  licence: string;
  url: string;
  part: string;
  note?: string;
  texts: string[];
}

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
  // The moments of the episode the render takes a picture of the short at,
  // in time order.
  thumbnails: number[];
}

// One line of a caption, the way the render lays it out.
export interface CaptionLine {
  words: Word[];
}

export interface CaptionCue {
  start: number;
  end: number;
  lines: CaptionLine[];
  // When the word the caption begins on and the word it ends on start in
  // the episode. A caption moved by hand is kept against these.
  first?: number;
  last?: number;
  // The caption appears, or goes, where it was put by hand.
  startMoved?: boolean;
  endMoved?: boolean;
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

// A part of an episode, in seconds. A searched part also says which
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
  // How many clips a search has written to its plan so far.
  found?: number;
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
  // What a render is of, from the moment it is queued. No clips means the
  // whole plan.
  plan?: string;
  clips?: string[];
  last?: EngineEvent;
  progress?: EngineEvent;
  queued: string;
  lane: "transcribe" | "work";
  // Grows with every change to any job. Of two snapshots of a job, the one
  // with the larger number is the later one.
  seq?: number;
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
  installed: boolean;
  // What this machine can do with it: "fits", "tight", "too big", or
  // "unknown" where the machine would not say how much memory it has.
  fit: "fits" | "tight" | "too big" | "unknown";
  // Whether this is the one to offer this machine, which is the largest it
  // can hold. It belongs to the machine and not to the model.
  recommended: boolean;
  // Whether clips are found with it: the file named in the settings, or
  // with none named the only one there is.
  inUse: boolean;
}

// One row of a list of models, as the list draws it. Speech models and
// language models are the same kind of thing on screen, so they are one
// component and each screen says what to put in the row.
export interface ModelRow {
  name: string;
  // What the install of it is called, which is the model's own title. What
  // the row shows may say more, like who made it.
  label: string;
  title: string;
  about: string;
  // What it costs, as one line.
  cost: string;
  installed: boolean;
  // Whether it is the one in use, where there is a choice. Left out where
  // there is none, and then being installed is being in use.
  inUse?: boolean;
  // The room it takes on the disk, said when it is removed.
  room: string;
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
  // Whether llama-server can be found. A model without it is a very large
  // file that answers nothing.
  hasServer: boolean;
  // Whether anybody has answered the one question. Until then the planner
  // is a default rather than a decision.
  chosen: boolean;
  ready: boolean;
  // The job of a model install in hand, so the app, opened again while one
  // runs, picks it up rather than starting a second.
  installing?: string;
}

export const api = {
  version: () => call<string>("Version"),
  platform: () => call<string>("Platform"),
  licences: () => call<Notice[]>("Licences"),
  licenceText: (name: string) => call<string>("LicenceText", name),
  // Where macOS put the title bar and its buttons, in whole CSS pixels, or
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
  // Takes a model off the machine to give its room back. It refuses while
  // one is being installed, or while the work that reads it runs.
  removeSpeechModel: (name: string) => call<void>("RemoveSpeechModel", name),
  removeLanguageModel: (name: string) => call<void>("RemoveLanguageModel", name),
  // Makes an installed model the one clips are found with, and says the
  // path it is found at.
  useLanguageModel: (name: string) => call<string>("UseLanguageModel", name),
  saveAPIKey: (key: string) => call<void>("SaveAPIKey", key),
  choosePlanner: (planner: "local" | "api") => call<void>("ChoosePlanner", planner),
  library: () => call<EpisodeStatus[]>("Library"),
  episode: (path: string) => call<EpisodeStatus>("Episode", path),
  addEpisodes: () => call<string[] | null>("AddEpisodes"),
  // Loads the local model for the first search of an episode while the
  // transcript is still on its way. Nothing comes back and nothing waits.
  warmModel: (path: string, from: number, to: number) => call<void>("WarmModel", path, from, to),
  // Where the transcription stops for now: the end of the window the first
  // search is waiting for. The speech model's chunk is cut exactly there.
  // 0 lets go, and a transcription that had stopped there carries on.
  holdTranscription: (path: string, at: number) => call<void>("HoldTranscription", path, at),
  removeEpisode: (path: string, deleteWork: boolean) =>
    call<void>("RemoveEpisode", path, deleteWork),
  source: (path: string) => call<SourceView>("Source", path),
  clips: (path: string) => call<ClipEntry[]>("Clips", path),
  coverage: (path: string, least: number) => call<CoverageView>("Coverage", path, least),
  // How much of the episode one search can read, and the weight of every
  // line so far, so the range picker knows how far a window may reach.
  room: (path: string) => call<RoomView>("Room", path),
  // Gives a part of an episode back: the clips in it go and the model
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
  // The cuts inside a clip: the parts it leaves out. Making one, moving
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
  // When a caption appears or goes, where the words are a little off from
  // what is heard. word is the word it begins or ends on and at the moment,
  // both in the episode. A moment below nought puts it back where its
  // words put it.
  setCaptionTime: (
    path: string,
    plan: string,
    clip: string,
    word: number,
    edge: "start" | "end",
    at: number,
  ) => call<ClipEntry>("SetCaptionTime", path, plan, clip, word, edge, at),
  // A thumbnail added, moved or removed. A from below nought adds one at
  // to, and a to below nought removes the one at from.
  setThumbnail: (path: string, plan: string, clip: string, from: number, to: number) =>
    call<ClipEntry>("SetThumbnail", path, plan, clip, from, to),
  clipPlayed: (plan: string, clip: string) => call<void>("ClipPlayed", plan, clip),
  removeClip: (path: string, plan: string, clip: string, removed: boolean) =>
    call<ClipEntry>("RemoveClip", path, plan, clip, removed),
  setCrop: (path: string, plan: string, clip: string, at: number, left: number) =>
    call<ClipEntry>("SetCrop", path, plan, clip, at, left),
  resetCrop: (path: string, plan: string, clip: string, at: number) =>
    call<ClipEntry>("ResetCrop", path, plan, clip, at),
  setCaptionStyle: (path: string, plan: string, font: string, size: number) =>
    call<void>("SetCaptionStyle", path, plan, font, size),
  // The colour of the caption text and of the box behind it, as #rrggbb,
  // each with how opaque it is from 0 to 1, and the colour of the pill
  // behind the word being spoken. An empty colour is left alone.
  setCaptionColours: (
    path: string,
    plan: string,
    text: string,
    textOpacity: number,
    box: string,
    boxOpacity: number,
    highlight: string,
    highlightOpacity: number,
  ) =>
    call<void>(
      "SetCaptionColours",
      path,
      plan,
      text,
      textOpacity,
      box,
      boxOpacity,
      highlight,
      highlightOpacity,
    ),
  // The pill behind the word being spoken, and its bounce, on or off for a
  // whole clip set.
  setCaptionHighlight: (path: string, plan: string, on: boolean) =>
    call<void>("SetCaptionHighlight", path, plan, on),
  // Where the captions sit, for every clip of every episode. Dragging the
  // box in the video preview saves it, so the next video starts there too.
  setCaptionsHeight: (path: string, y: number) => call<void>("SetCaptionsHeight", path, y),
  resetCaptionsHeight: (path: string) => call<void>("ResetCaptionsHeight", path),
  // Take back the last thing done to an episode's clips, or do again what
  // was taken back. The answer names the clip it changed.
  undo: (path: string) => call<Undone>("Undo", path),
  redo: (path: string) => call<Undone>("Redo", path),
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

// Where macOS put the title bar and its buttons, in whole CSS pixels. All
// zeros means the system draws its own title bar, which is every other
// system, and then the stylesheet keeps its own numbers.
export interface Chrome {
  // The title bar's height, which is the height of the bar the app draws.
  bar: number;
  // The left edge of the first of those buttons and the right edge of the
  // last, so the name of what is on screen starts after them.
  left: number;
  right: number;
  // The middle of those buttons, from the top of the page.
  middle: number;
}

// The app is told again whenever the answer changes: a resize, either
// way through fullscreen, another display, another scale, light or dark,
// or back from the Dock.
export function onChrome(fn: (c: Chrome) => void): () => void {
  return Events.On("chrome", (ev) => fn(ev.data as Chrome));
}

export interface Undone {
  done: boolean;
  // The key of the clip it changed, as the clip list has it.
  clip?: string;
}

// Undo and Redo in the Edit menu. The menu has the keys, so this is how
// they reach the app.
export function onUndo(fn: (what: "undo" | "redo") => void): () => void {
  return Events.On("undo", (ev) => fn(ev.data as "undo" | "redo"));
}

// Acknowledgements in the Help menu, which is where apps keep the notices
// of the work they are made with.
// Cmd+Q heard by the Go side: "ask" while work runs and the key has to be
// pressed again, "going" once the app is on its way out.
export function onQuit(fn: (what: "ask" | "going") => void): () => void {
  return Events.On("quit", (ev) => fn(ev.data as "ask" | "going"));
}

export function onAcknowledgements(fn: () => void): () => void {
  return Events.On("acknowledgements", () => fn());
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
//
// best is the one the machine is offered, the largest it can hold. Saying
// so never hides the warning: the best of them on a small machine is still
// tight on it, and somebody about to spend a download deserves to know.
export function fitNote(
  fit: LanguageModel["fit"],
  best = false,
): { note: string; warn: boolean } {
  switch (fit) {
    case "fits":
      return { note: best ? "Best for this machine" : "Fits this machine", warn: false };
    case "tight":
      return { note: best ? "The best of these here, and tight on it" : "Tight on this machine", warn: true };
    case "too big":
      return { note: "Too big for this machine", warn: true };
    default:
      return { note: best ? "The safest of these" : "", warn: false };
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
// The colours the captions start out in: white text on a black box that
// lets half the picture through, as the engine's own style has them.
export const captionTextDefault = "#ffffff";
export const captionTextOpacityDefault = 100;
export const captionBoxDefault = "#000000";
export const captionOpacityDefault = 50;
// The pill behind the word being spoken, as the engine's own style has it.
export const captionHighlightDefault = "#942192";
export const captionHighlightOpacityDefault = 100;
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
// that those pixels are worth less than the least still takes a part
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

// Where the playhead lands when it is stepped by words, which is what
// shift and an arrow key do.
//
// A second was what they took before, and a second is nothing in
// particular: it lands in the middle of a word as often as not, it walks
// four words at a time where someone speaks quickly and none at all across
// a pause. A word is the thing the picture is showing. The caption lights
// up the word being spoken, so stepping to the next word steps that light
// one word on, which is what the keys are for.
//
// **It is decided by which word the playhead is in, not by how far it is
// from one.** That is the whole of why this works, and comparing against a
// distance is why it did not. The playhead is not where it was put: a
// video element answers with the frame it is showing, so a playhead sent
// half a frame into a word comes back somewhere else inside that frame,
// and it can come back later than it was sent. Stepping back then measured
// the distance to the word it was already on, found it far enough, and
// sent the playhead to the same place again. That is a key that does
// nothing at all, and there was no way out of it but the mouse.
//
// Deciding by the word cannot do that, because the answer is always a
// different word from the one the playhead is in, however the clock
// rounds.
//
// **It lands a frame into the word, never on its edge.** A word boundary
// is exactly where the question "is this word being spoken" has no steady
// answer: the caption runs on the clip's clock and the playhead on the
// episode's, they are worked out by different arithmetic, and the frame
// the picture settles on is a third answer again. A frame in is inside the
// word by far less than the gap to the next one, so the picture shows the
// same frame and the right word is lit. A word shorter than two frames is
// entered by half of itself.
//
// Null means there is nowhere to go: no words heard here yet, or the
// playhead is already before the first or past the last of them.
// Where the playhead goes to stand on a word: a frame in, never on the
// edge. A word shorter than two frames is entered by half of itself.
export function intoWord(word: { start: number; end: number }, frame: number): number {
  return Math.min(word.start + frame, (word.start + word.end) / 2);
}

export function wordStep(words: Word[], at: number, back: boolean, frame: number): number | null {
  if (!words.length) return null;
  // The last word that has begun, and whether the playhead is still in it.
  let here = -1;
  for (let i = 0; i < words.length; i++) {
    if (at >= words[i].start) here = i;
    else break;
  }
  const inside = here >= 0 && at < words[here].end;
  // Back out of a word is the word before it. Back out of the silence
  // after a word is that word, because it is the one just spoken.
  const target = back ? (inside ? here - 1 : here) : here + 1;
  const word = words[target];
  return word ? intoWord(word, frame) : null;
}

