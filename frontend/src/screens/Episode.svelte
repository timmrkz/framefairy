<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import { fade } from "svelte/transition";
  import {
    api,
    captionFontDefault,
    captionBoxDefault,
    captionHighlightDefault,
    captionHighlightOpacityDefault,
    captionOpacityDefault,
    captionTextDefault,
    captionTextOpacityDefault,
    captionSizeDefault,
    captionYDefault,
    captionYMax,
    captionYMin,
    captionYStep,
    clock,
    errorText,
    intoWord,
    snapCaptionY,
    type CaptionFont,
    type CaptionsView,
    type CaptionSwitch,
    type ClipEntry,
    type Gesture,
    type Job,
    type CoverageView,
    type KeptWindow,
    type EpisodeStatus,
    type SourceView,
    type Word,
    type Sentence,
    onUndo,
    onLevels,
  } from "../lib/api";
  import { jobs } from "../lib/state.svelte";
  import {
    Heard,
    covers,
    heardIn,
    nothingToMark,
    type Parts,
    inEpisode,
    endInEpisode,
    Newest,
    timesIn,
    frameAt,
    frameMiddle,
  } from "../lib/flow";
  import { faceLook, families, installFonts, nearestFace, styleNames } from "../lib/fonts";
  import { playheadClock, type About } from "../lib/playhead";
  import {
    Reach,
    fitWindow,
    leastWindow,
    longestShortest,
    mostClips,
    type RoomView,
  } from "../lib/room";
  import { suggestedCount } from "../lib/suggest";
  import { captionColours, joinColour, splitColour } from "../lib/colour";
  import { stepLine } from "../lib/steps";
  import { Failures } from "../lib/failed";
  import { asleep, asking, keysElsewhere, typing } from "../lib/keys";
  import {
    secondThoughts,
    setAside,
    setAsideTold,
    spent,
    takeUp,
    takeUpTold,
    toldSpent,
    type Removed,
    type Told,
  } from "../lib/removed";
  import { arriving, OnTheWay, type Arriving, type Away } from "../lib/arriving";
  import { holdAhead } from "../lib/frames/queue";
  import { wordFramesOf, type WordFrame } from "../lib/frames/captions";
  import RangeWindow from "../components/RangeWindow.svelte";
  import Player, { type PlayerOffers } from "../components/Player.svelte";
  import Busy from "../components/Busy.svelte";
  import ClipList from "../components/ClipList.svelte";
  import SaidList from "../components/SaidList.svelte";
  import SearchField from "../components/SearchField.svelte";
  import ClipTimeline, { type ClipNumbers } from "../components/ClipTimeline.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import Confirm from "../components/Confirm.svelte";
  import Pick from "../components/Pick.svelte";
  import Colour from "../components/Colour.svelte";
  import { act, said } from "../lib/said";
  import { pieceHolding, rotated, rotationAt } from "../lib/rotate";
  import { markOn } from "../lib/glide";
  import { unchanged } from "../lib/unchanged";
  import { toasts } from "../lib/toast.svelte";

  let {
    path,
    onchange,
    onready,
    away = false,
    inLibrary,
  }: {
    path: string;
    onchange: () => void;
    // Told once the workspace has the episode on it, see ready.
    onready?: () => void;
    // Another episode has been picked, and this workspace only stays on
    // screen until that one is ready, see App.svelte.
    away?: boolean;
    // The video as the library read it last, which it does each time the
    // app comes to the front, see App.svelte.
    inLibrary?: EpisodeStatus;
  } = $props();

  // The workspace's own element. Another episode's workspace can stand in
  // the app beside this one for a moment, see App.svelte, so whatever this
  // one looks for it looks for in itself, and keys are only taken while it
  // is the one in front, see asleep.
  let root = $state<HTMLElement>();

  // Every step of the workspace opening, for the app's log, see
  // lib/said.ts: what it asked the Go side and what came back, and what
  // it still waits for when it is slow to be ready.
  const built = performance.now();
  const tell = (line: string) =>
    said(`workspace: ${line}, ${Math.round(performance.now() - built)} ms after it was made`, { video: path });

  let status = $state<EpisodeStatus | null>(null);
  let source = $state<SourceView | null>(null);
  // Until the workspace knows where it opens, on its clip or at the start,
  // the video preview draws nothing, see opening in Player.svelte.
  let opened = $state(false);
  let clips = $state<ClipEntry[]>([]);
  let from = $state(0);
  let to = $state(0);
  // The target typed in the workspace, or 0 while it follows the window,
  // and how long the window was it was typed for. It is the target of a
  // window that long and of no other, see typed.
  let target = $state(0);
  let targetWindow = $state(0);
  let min = $state(20);
  let max = $state(30);
  let waiting = $state<string[]>([]);
  // A click has to show at once, long before the job it starts reports in.
  // The store keeps the press, the same for this work as for the whole
  // video heard from its menu.
  const starting = $derived(jobs.isStarting(path, "clips"));
  // The press of Search or Continue the work in hand was asked for by. Cancel
  // names it, so it stops that search whether or not the Go side has heard
  // of it yet, see stopWork.
  let click = "";
  let clicks = 0;
  let time = $state(0);
  // Where the model has already looked. A window is only drawn outside it.
  let coverage = $state<CoverageView>({ passes: [] });
  // How much one search can read, from the engine, and what that makes of
  // the window and the clip settings. The window is no longer than the
  // model reads in one request and no shorter than the clips asked for
  // need at their shortest. The clip count and the shortest length are
  // held to the longest window that can be drawn anywhere on the episode,
  // so whatever they ask for fits wherever the window is.
  let roomView = $state<RoomView | null>(null);
  let selected = $state("");
  // How many times a clip was chosen, so choosing the clip that already
  // was chosen still takes the sentences of what was said to it.
  let chosenTimes = $state(0);
  // The clip under the hand, on its card, on the range picker or on the
  // clip timeline. All three light it, so the same clip is found in the
  // other two at a glance. Going off one thing and onto the next can come
  // in either order, so going off only clears what is still this clip.
  let hovered = $state("");
  function hoverClip(key: string, on: boolean) {
    if (on) hovered = key;
    else if (hovered === key) hovered = "";
  }
  // What the clip list held and which clip was picked when a search began.
  // A search writes each clip to the plan as it is found, so the first one
  // it finds is put on screen the moment it lands, long before the search
  // is over. Only if nobody picked another clip meanwhile: a clip taken
  // away from under the hand that is working on it is worse than one shown
  // a little later. What the list held is state, because the rows still to
  // come are counted from it.
  let listedBefore = $state<Set<string> | null>(null);
  let pickedBefore = "";
  // The clip the search chose by itself, see showFirstFound.
  let chosenByFind = "";
  let shownFirst = false;
  let player = $state<Player>();
  let timeline = $state<ClipTimeline>();
  // The column the video preview and the range picker stand in, which a
  // clip fitted on the clip timeline is centred under.
  let middle = $state<HTMLDivElement>();
  // What the clip timeline has to say about the chosen clip, for the row
  // under it.
  let numbers = $state<ClipNumbers>({ key: "", start: 0, end: 0, seconds: 0, pieces: 0, saving: false });
  // The playback buttons stand in the row above the clip timeline, with
  // Render, so the video preview and the range picker have nothing under
  // them but the line that parts them from it.
  // The workspace measures itself: how much height the app leaves it,
  // and how wide it is. From those two the video preview gets the biggest
  // size it can have, and the columns beside it take everything else.
  // A video whose file is gone says so in a toast, once each time it goes,
  // and its video preview shows nothing of what it showed. It stood in a
  // line over the workspace, which pushed everything down, and Tim saw a
  // toast say it a second time. Issue #311.
  const gone = $derived(
    `The video file is no longer at ${path}. Put it back there, or remove the video from the list.`,
  );
  // When the library says otherwise than the workspace, the workspace reads
  // the video again. Tim put a file back in Finder, the sidebar saw it
  // as the app came to the front, and the video preview went on showing
  // nothing: one part of the app knew and the other did not.
  // Once for each read of the library, so two answers that disagree for a
  // moment do not ask each other again and again.
  $effect(() => {
    if (!inLibrary || away) return;
    const gone = inLibrary.missing;
    const now = untrack(() => status);
    if (!now || gone === now.missing) return;
    untrack(() => {
      tell(`the library says the file is ${gone ? "gone" : "back"}, read again`);
      void load();
    });
  });
  let saidGone = false;
  $effect(() => {
    const now = !!status?.missing;
    if (now === saidGone) return;
    saidGone = now;
    if (now) untrack(() => toasts.say(where(), gone, { video: path }));
  });
  let paused = $state(true);
  let looping = $state(false);
  // Whether the playhead is on the chosen clip or on the video, which the
  // video preview keeps, see lib/playhead.ts. On the video the clip is
  // drawn dimmed on the clip timeline and the range picker.
  let onClip = $state(false);
  let offers = $state<PlayerOffers>({ crop: "", savingCrop: false, hint: "" });

  const duration = $derived(source?.duration ?? 0);
  const reach = $derived(new Reach(roomView, duration));
  const reachAnywhere = $derived(reach.anywhere());
  // The window has room for the clips typed for it. A suggestion follows
  // the window instead, so it only needs room for one.
  // The target typed for this window, or 0 when it follows the window.
  // Three typed for the six minutes of one episode went on asking for
  // three in the half hour of the next, so a number typed is for the
  // window it was typed for: another length follows its own suggestion.
  const typed = $derived(target > 0 && Math.abs(to - from - targetWindow) < 0.5 ? target : 0);
  const leastLong = $derived(leastWindow(typed > 0 ? typed : 1, min, duration));
  const clipsAtMost = $derived(mostClips(reachAnywhere, min, 30));
  // How many clips the window drawn now suggests. The engine works out the
  // same, see engine/suggest.go. What a search looks for is the target
  // typed, or the suggestion while nothing is typed.
  const suggested = $derived(Math.min(suggestedCount(to - from, min, max), clipsAtMost));
  const count = $derived(typed > 0 ? typed : suggested);
  const shortestAtMost = $derived(longestShortest(reachAnywhere, count, 5, 180));
  // The episode's search, as the Go side keeps it: one job from Search to its
  // clips, which hears the episode as far as the window reaches and then
  // finds, see docs/JOBS.md. Everything the workspace shows about finding
  // clips comes from it, and the workspace decides nothing about when a
  // search starts or stops. It is undefined when there is nothing to say.
  const search = $derived(jobs.search(path));
  // A search that fails while the app is open says why in a toast, with
  // Report for what the person cannot put right alone, and so does a clip
  // made by hand with I or O, see lib/failed.ts. Its row or card goes on
  // saying Failed with Continue, after a restart too, and never why, issue
  // #450. Only the jobs' states and ids are read, see the rule on watching
  // jobs.
  const failures = new Failures<Job>((job) => {
    const what = job.kind === "clip" ? "clip made by hand" : "search";
    const why = untrack(() => reasonOf(job.error, what));
    tell(`${what} failed: ${why}`);
    toasts.say(where(), why, { video: path, report: true });
  });
  $effect(() => {
    for (const job of jobs.forEpisode(path)) {
      if (job.kind === "search" || job.kind === "clip") failures.seen(job);
    }
  });
  const searching = $derived(!!search && (search.state === "running" || search.state === "queued"));
  const working = $derived(searching ? search : undefined);
  // The search while it hears the episode. Its progress says how far the
  // audio has been heard.
  const transcribing = $derived(working?.step === "hearing" ? working : undefined);
  const finding = $derived(working?.step === "finding");
  const renderingJob = $derived(jobs.active(path, "rendering"));
  // Whether work is running, as a plain yes or no. The Go side sends a job
  // event about once a second, and every one of them replaces the job
  // object, so anything that watches the job itself is torn down and set
  // up again that often. A timer that watches one never fires at all,
  // which is what stopped the transcript from being read again while it
  // grew, and with it the first search.
  const isFinding = $derived(!!finding);
  // A render is shown by the Render button it was started from, which
  // fills up and becomes Cancel. The head of the clip list is about
  // finding clips, so a render is not its business, and it runs in a lane
  // of its own, so it holds no search up.
  const rendering = $derived(!!renderingJob);
  // While a search runs, the head of the clip list carries it: Search becomes
  // Cancel and the line under the head fills up. Nothing is added to the
  // column and nothing moves.
  // The clips made by hand are work of the clip list too: its Cancel stops
  // them with the search, and its Continue carries them all on.
  const handWork = $derived(jobs.clips(path));
  const handRunning = $derived(handWork.filter((j) => j.state === "running" || j.state === "queued"));
  const handStopped = $derived(handWork.filter((j) => j.state === "interrupted" || j.state === "failed"));
  // The video heard to its end by itself, Transcribe whole video in its
  // menu, while it hears.
  const wholeHearing = $derived.by(() => {
    const j = jobs.transcription(path);
    return j?.state === "running" && j.step === "hearing" ? j : undefined;
  });
  // Whatever hears the episode now, the search, a clip made by hand or the
  // whole video, one at a time. Its progress says which part it hears and
  // how far it has come in it.
  const hearing = $derived(transcribing ?? handRunning.find((j) => j.step === "hearing") ?? wholeHearing);
  // Transcribe whole video while it runs or waits its turn.
  const wholeWork = $derived(jobs.transcribing(path));
  const isTranscribing = $derived(!!hearing);
  const busy = $derived(searching || starting);
  // A search that found its clips is over, and the window it moved on to
  // is on its way from the Go side. The search's frame stays where it is
  // until then, so the range picker never shows the window back where the
  // search was, between the search and the next, see load.
  let movingOn = $state(false);
  // What the search is doing right now, in the step's own words, see
  // lib/steps.ts.
  const doing = $derived(working ? stepLine(working).what : "");
  const leftOfWork = $derived(
    working?.progress && working.progress.remaining > 0
      ? `${clock(working.progress.remaining)} left`
      : "",
  );
  const stopping = $derived(jobs.isStopping(path, "clips"));

  // The pane has one shape, whatever it is busy with. The head says what
  // that is, the button does the one thing there is to do about it, and
  // the button fills up as it goes.
  const needsWords = $derived(
    !!status && !status.missing && (!status.transcribed || status.transcriptStale),
  );
  // The pane is the clip list and nothing else. The transcription is worked
  // from the range picker, at the edge it moves, so none of it belongs in
  // this head: the two ran in lanes of their own and the head carried both,
  // which is how it came to say Transcribing over a list of clips.
  const lane = $derived({ word: "Clips", count: true, job: working ?? null });
  const action = $derived.by(() => {
    // Search is the search's button, so it turns into Cancel for a search and
    // for nothing else: a clip made by hand wears the beam on I or O, which
    // it was started from. It turned into Cancel for those too, under a
    // hand that had only pressed I, and a click there stopped the clip
    // nobody meant to stop. Cancel stops all the work on the clips, the
    // search and every clip on its way, and Continue carries all of it on.
    if (busy) {
      return {
        label: stopping ? "Cancelling" : "Cancel",
        run: stopWork,
        off: stopping,
        primary: false,
        title: "Stop the work on the clips. What was heard and made is kept",
      };
    }
    // Work that was cut off or failed is carried on, not started anew: the
    // button says Continue and takes up the window the search was about,
    // wherever the window on the range picker is now, and every clip made
    // by hand that stopped with it. A clip made by hand that stopped alone
    // is carried on from its own card.
    if (stopped) {
      return {
        label: "Continue",
        run: carryOn,
        off: duration <= 0,
        primary: true,
        title: `Carry on the search of ${stopped.window.toLowerCase()}${handStopped.length ? ", and the clips made by hand" : ""}`,
      };
    }
    return {
      label: "Search",
      run: newClips,
      off: duration <= 0 || to <= 0,
      primary: true,
      title: searchedBefore
        ? `Look again for clips from ${clock(from)} to ${clock(to)}, keeping the ones there are`
        : `Look for clips from ${clock(from)} to ${clock(to)}`,
    };
  });

  // How far whatever the head is about has come.
  // How far the render has come, for the Render button.
  const renderShare = $derived(
    renderingJob?.progress && renderingJob.progress.fraction >= 0 ? renderingJob.progress.fraction : -1,
  );
  const share = $derived(
    lane.job?.progress && lane.job.progress.fraction >= 0 ? lane.job.progress.fraction : -1,
  );

  // Cancel: the search stops, and what it heard of the episode stays, so
  // the next search goes on from there. The edge on the range picker stops
  // where it is, because the engine keeps no piece heard after the stop
  // and reports none, so what was shown is what was saved.
  // It names the episode's work and the press it came after, not a job,
  // so it takes effect the moment it is pressed: it called off the jobs it
  // knew, and between Search and the Go side's answer it knew none, so it
  // stood greyed out and a press there did nothing. The Go side stops the
  // search the press asked for even if Cancel reaches it first.
  function stopWork() {
    if (!busy || stopping) return;
    jobs.stop(path, "clips");
    void api.stopClipWork(path, click);
  }

  // The Render button's Cancel stops the render and nothing else. It
  // called the clip list's Cancel, which stopped the search and every
  // clip on its way, and left the render running.
  let renderStopping = $state(false);
  function stopRender() {
    if (!renderingJob) return;
    renderStopping = true;
    api.cancelJob(renderingJob.id);
  }
  $effect(() => {
    if (!renderingJob) renderStopping = false;
  });

  // Continue, on everything of the clip list that stopped: the search
  // carries on from what it left, on the window it was about, which the
  // range picker shows again, and every clip made by hand that stopped
  // with it. One press, so a Cancel after it stops all of them.
  function carryOn() {
    if (!stopped || !search) return;
    from = stopped.from;
    to = stopped.to;
    const id = search.id;
    const press = begin();
    for (const job of handStopped) void api.continueJob(job.id, press).then((j) => jobs.apply(j));
    void asked(() => api.continueJob(id, press));
  }
  // What the transcript on disk has heard.
  const saved = $derived<Parts>(status?.transcribed ? [[0, duration]] : (status?.heard ?? []));

  // The search through what was said, in the clip column, #366: whether
  // it is open, what is typed in it, and the sentences it reads, read
  // again whenever the transcript on disk hears more.
  let saying = $state(false);
  let sayTyped = $state("");
  let sayMatches = $state(0);
  let sentences = $state<Sentence[] | null>(null);
  // Why the cards could not be read the last time they were asked for,
  // said in the list until the next read works.
  let sentencesFailed = $state<string | null>(null);
  let sayField = $state<HTMLInputElement>();
  let saidList = $state<{ walk: (step: 1 | -1) => void }>();
  const savedKey = $derived(saved.map(([a, b]) => `${a.toFixed(1)}-${b.toFixed(1)}`).join(","));
  // The parts every clip keeps, whose captions are cards of their own, so
  // the cards are read again when a clip changes, see engine.SaidCards.
  const cutsKey = $derived(
    clips
      .filter((c) => !c.rejected)
      .map((c) => `${c.key}:${(c.segments.length ? c.segments : [c]).map((p) => `${p.start.toFixed(2)}-${p.end.toFixed(2)}`).join(" ")}`)
      .join(","),
  );
  // How many times the captions of a clip changed while it stayed the
  // clip they are of: a word corrected or removed, a caption moved, the
  // look of every caption changed. The cards of what was said are those
  // captions, so they are read again. A clip chosen anew is not a change.
  let captionsChanged = $state(0);
  const captionsRead = new Map<string, string>();
  $effect(() => {
    const view = captions;
    const key = captionsOf;
    if (!view || !key) return;
    const words = (view.captions ?? []).map((c) => `${c.start.toFixed(2)} ${c.lines.map((l) => l.words.map((w) => w.text).join(" ")).join(" ")}`).join("|");
    const was = captionsRead.get(key);
    captionsRead.set(key, words);
    if (was !== undefined && was !== words) untrack(() => captionsChanged++);
  });
  // They are read as soon as the video opens and kept, so the transcript
  // button shows them in the frame it is pressed, and read again whenever
  // the transcript hears more. A column left empty until the answer came was
  // seen as a blink by Tim.
  let sentencesAsked = 0;
  let sentencesOf = "";
  $effect(() => {
    const of = path;
    const key = `${savedKey}|${cutsKey}|${captionsChanged}`;
    if (!saved.length) return;
    // While the search is closed, what was read stays until it opens.
    if (!saying && sentencesOf.startsWith(`${of}|`)) return;
    if (sentencesOf === `${of}|${key}`) return;
    const ticket = ++sentencesAsked;
    untrack(() => {
      api
        .sentences(of)
        .then((list) => {
          if (ticket !== sentencesAsked || of !== path) return;
          sentences = list ?? [];
          sentencesOf = `${of}|${key}`;
          sentencesFailed = null;
        })
        .catch((err) => {
          said(`the sentences of the video could not be read: ${errorText(err)}`, { level: "warn" });
          // An old failure never covers a newer answer.
          if (ticket !== sentencesAsked || of !== path) return;
          sentencesFailed = errorText(err);
        });
    });
  });
  // The part not transcribed yet under the hand, in the list, lit on the
  // range picker too.
  let unheardLit = $state(-1);
  // The one whose card the playhead is in, lit on the range picker the
  // same way: a part is lit one way, by the hand or by the keys, found by
  // Tim.
  let unheardOn = $state(-1);
  // The parts of the video the transcript has not heard yet, which the
  // search cannot look in. A gap under a second is between two words.
  // They give way as the work hears, not as it saves, the way the edge of
  // what is heard moves on the range picker: saving comes minutes of
  // audio at a time, and the parts went in thirds of a window, found by
  // Tim. See heard below.
  const unheard = $derived.by(() => {
    if (!(duration > 0)) return [] as [number, number][];
    const out: [number, number][] = [];
    let at = 0;
    for (const [a, b] of [...heard].sort((x, y) => x[0] - y[0])) {
      if (a - at >= 1) out.push([at, a]);
      at = Math.max(at, b);
    }
    if (duration - at >= 1) out.push([at, duration]);
    return out;
  });
  // Stop transcribing pressed and the job not ended yet.
  const wholeStopping = $derived(jobs.isStopping(path, "whole"));
  // The part Transcribe whole video works on, framed on the range picker
  // the way the window of a search is while it hears, Tim's pick on #458:
  // the part it hears, or the one it hears next while it waits its turn.
  // The frame goes on the press of Stop transcribing, fading, see
  // RangeWindow. While the whole video waits its turn, behind a search, a
  // clip made by hand or a model being installed, its frame stays, dimmed,
  // so it is plain the work is only on hold, Tim's report of 23 October.
  // The look comes from its own job and not from whether a search runs:
  // a search with a model in the cloud does not hold up the hearing, found
  // by the second review of #458.
  const wholePart = $derived.by((): [number, number] | null => {
    if (!wholeWork || wholeStopping) return null;
    const edge = wholeHearing && live ? live[1] : -1;
    const gap = (edge >= 0 && unheard.find(([a]) => Math.abs(a - edge) < 0.5)) || unheard[0];
    return gap ?? null;
  });
  // The clip list and the cards of what was said take turns in one
  // scrolled column, so each keeps its own place: the clip list comes back
  // where it was, the chosen clip in view, and the cards open at the
  // playhead, see SaidList. The clip list came back where the cards had
  // been scrolled to, and the cards opened where the clip list had been.
  let clipListTop = 0;
  function openSaying() {
    act("search what was said");
    clipListTop = root?.querySelector<HTMLElement>(".pane .list")?.scrollTop ?? 0;
    saying = true;
    void tick().then(() => sayField?.focus());
  }
  function closeSaying() {
    saying = false;
    sayTyped = "";
    unheardLit = -1;
    void tick().then(() => {
      const list = root?.querySelector<HTMLElement>(".pane .list");
      if (list) list.scrollTop = clipListTop;
      showChosen();
    });
  }
  // Where the playhead and the chosen clip stood when Shift and an arrow
  // first walked the answers to what is typed, and the card it was walked
  // to last. A search let go of, by Escape, by erasing what was typed or
  // by its cross, goes back there, the way a search through a document
  // goes back to where it began, found by Tim. Unless the playhead left
  // the card it was walked to, or a card was clicked, or Enter pressed:
  // then the person went where they meant to go, and stays.
  let sayFrom: { at: number; clip: string } | null = null;
  let sayOn: [number, number] | null = null;
  function walkSaid(step: 1 | -1) {
    saidList?.walk(step);
  }
  let typedBefore = "";
  $effect(() => {
    const now = sayTyped.trim();
    const was = typedBefore;
    typedBefore = now;
    if (now || !was) return;
    untrack(() => {
      const from = sayFrom;
      const on = sayOn;
      sayFrom = null;
      sayOn = null;
      if (!from || !on || time < on[0] - 0.05 || time >= on[1]) return;
      said(`search what was said: let go of, back to ${from.at.toFixed(2)}${from.clip ? ` in ${from.clip}` : ""}`);
      if (from.clip !== selected) {
        if (from.clip) void select(from.clip, false, false);
        else selected = "";
      }
      seekTo(from.at);
    });
  });

  // The space bar in the field with nothing typed plays, and the field
  // lets go of the keyboard for it. Stopping again gives the field the
  // keyboard back, so what is typed next is a search, found by Tim. Not
  // once the keyboard or the hand went somewhere else in between.
  let sayRefocus = false;
  $effect(() => {
    if (!paused || !sayRefocus) return;
    sayRefocus = false;
    if (saying && !typing()) untrack(() => sayField?.focus());
  });
  function sayElsewhere(event: Event) {
    if (event.target !== sayField) sayRefocus = false;
  }

  // Escape takes back one step at a time: what was typed first, so the
  // whole of what was said is there again, and then the search itself.
  function escapeSaying() {
    if (sayTyped) sayTyped = "";
    else closeSaying();
  }
  // Escape steps back out of the search wherever the keyboard is, unless
  // it is in a field of its own: the hand scrolled the sentences and
  // Escape did nothing, because the field had lost the keyboard.
  function sayKey(event: KeyboardEvent) {
    if (event.key !== "Escape" || !saying) return;
    if (event.defaultPrevented || keysElsewhere() || asleep(root)) return;
    event.preventDefault();
    escapeSaying();
  }

  // How far the loudness is measured, which is the waveform. It is measured
  // on its own from the moment the episode is added, ahead of the
  // transcript, and the Go side says it got further about twice a second
  // while it runs, which is when the status is read again.
  const measured = $derived(status?.measuredAll ? duration : (status?.measured ?? 0));
  const measuredParts = $derived<[number, number][]>(
    status?.measuredAll ? [[0, duration]] : (status?.measuredParts ?? []),
  );
  onMount(() =>
    onLevels((p) => {
      if (p !== path || away) return;
      const ticket = statusRead.send();
      api
        .episode(path)
        .then((now) => {
          if (statusRead.keep(ticket)) status = now;
        })
        .catch(() => {});
    }),
  );

  // What of the audio has been heard, which is not the same as what the
  // saved transcript holds. Saving rewrites the whole transcript, so it
  // happens seconds apart and jumps minutes of audio at a time, while every
  // chunk the recogniser finishes says where it got to. The range picker
  // draws this one, so its edge moves with the work. Everything that reads
  // the transcript keeps to saved, because that is what is on disk.
  // The mark the edge is drawn from. It only ever grows, because it says
  // how much of the episode has been read and reading does not unhappen.
  // Pausing showed that plainly: the live number disappears at the one
  // moment the saved one is at its most stale, and the edge walked
  // backwards by however much had not been written down. Heard in flow.ts
  // holds the rule, with the orderings that break it written out beside it.
  const mark = new Heard();
  // What makes the mark meaningless: a transcript that is out of date and
  // will be read again, or a work folder that is no longer there. Not while
  // the episode is still loading, when nothing is known yet.
  const restarted = $derived(!!status && (status.transcriptStale || !status.work));
  // The part being heard, from where it began to where it has got.
  const live = $derived.by((): [number, number] | null => {
    const p = hearing?.progress;
    if (!p?.covered) return null;
    return [p.from ?? 0, p.covered];
  });
  const heard = $derived(mark.seen(path, saved, live, restarted));
  // Where the work hears into a part not transcribed yet, so its dots
  // stand still under the edge. The whole video on hold still holds its
  // part, so the dots do not shift as it waits and carries on, Tim's
  // report of 24 October. Nothing holds the edge on a stop: the engine
  // keeps no piece heard after it, so the edge it last reported is the
  // edge it saved, found by the third review of #458.
  const givingAt = $derived(live ? live[1] : wholePart && !wholeHearing ? wholePart[0] : -1);

  // Whether the episode has been heard to the end of the window. A search
  // of a window not heard yet hears it first, which the title of Search says.
  const heardWindow = $derived(duration > 0 && to > 0 && covers(heard, from, to));
  // The range picker carries the transcription: how far it has come is what
  // the track draws anyway, so there is no bar of its own.
  const waitingOnWords = $derived(
    !!status && !status.missing && (!!transcribing || !status.transcribed || status.transcriptStale),
  );
  const leftToGo = $derived(
    transcribing?.progress && transcribing.progress.remaining > 0
      ? `${clock(transcribing.progress.remaining)} left`
      : "",
  );
  // How many rows the clip list holds open. Nothing is known about the
  // clips before the search answers, but their number is: it is the one
  // asked for. So the list stands in the shape it is about to take, with
  // the light passing over it, rather than as an empty area with a line of
  // text in it. What is going on is in the info mark at the head.
  //
  // A search writes each clip the moment it is found, so while it runs the
  // rows still to come are what it was asked for less what it has taken so
  // far, see searchTook. The clips from earlier searches are in the list
  // too and are not counted against it.
  //
  // With nothing on its way and nothing in the list, the rows stand there
  // all the same, as many as Clips says and following it as it changes,
  // but still: they are what Search will fill, and nothing is filling them
  // yet. An episode whose first search was stopped, by quitting among
  // other things, had an empty column there instead.
  const comingNow = $derived(busy);
  // What the row the next clip will appear in is waiting on. While the
  // transcript has not reached the end of the window, that is the
  // transcript, and how far it has come is how much of the window it
  // covers. While a search runs, it is whatever the search says it is
  // doing.
  //
  // An empty headline means nothing new to say, which is what the moment
  // between two reports of the engine is, and the row keeps what it says.
  const windowText = $derived(`Window ${clock(from)} to ${clock(to)}`);
  // The row the next clip will appear in says which step the search is in,
  // in the words of lib/steps.ts, the same words every row that shows work
  // uses. While it hears, how far it has come is how far it has heard what
  // of the window it has to hear, from 0, see stepLine. It was once how
  // much of the window was heard, which started half full on a window
  // heard half already, issue #444. Nothing more: the clips it has found so far are the cards
  // above the row, and the row said "2 of 12 found" in the place the third
  // clip was to appear.
  const next = $derived.by(() => {
    if (!busy) return null;
    // Clicked, and the job not there yet: the row already says the step
    // the search is about to take, so Continue goes from Stopped straight
    // to Transcribing, with no empty card and no word in between.
    if (!working) {
      const fraction = heardWindow ? -1 : 0;
      // Cancel pressed before the Go side answered: the row says so at
      // once, the same as for a search that runs.
      const left = heardWindow ? windowText : transcribedIn(heard, from, to);
      if (stopping) return { what: "Stopping", left, fraction, still: true, now: true };
      return { what: heardWindow ? "Finding clips" : "Transcribing", left, fraction, now: true };
    }
    const line = stepLine(working);
    // While it hears, or waits to, and cannot say how long it has left, the
    // row says how much of its window is transcribed, as it grows, in the
    // words the row of a search that stopped uses. It said the whole
    // window, Window 0:00 to 30:00, over a window transcribed half already,
    // Tim's report of 25 October.
    const start = working.from ?? 0;
    const end = working.to && working.to > 0 ? working.to : duration;
    const hears = working.lane === "hearing" && end > start;
    const left = line.left || (hears ? transcribedIn(heard, start, end) : windowText);
    // Cancel pressed: the row says so in the same frame, and stands still
    // where it got to, until the search has saved what it did and says it
    // stopped. It went on saying Transcribing for a second after the click.
    if (stopping) {
      return { what: "Stopping", left, fraction: line.fraction, still: true, now: true };
    }
    return { ...line, left };
  });

  // How much of a window the parts given cover, as the rows of a search
  // say it.
  function transcribedIn(parts: Parts, start: number, end: number): string {
    return `${clock(heardIn(parts, start, end))} of ${clock(end - start)} transcribed`;
  }

  // What the row says, held long enough to be read. The engine reports
  // twice a second and a search goes through its steps faster than anyone
  // reads, so a new headline waits until the one before has been there
  // for a while: two and a half seconds, and one for the count of clips
  // found going up. The fill and the time left follow at once, because
  // they are the same thing moving on. It keeps its own time, in onMount,
  // because an effect that reads the job is set up again on every report.
  // The search's work for its own cards, once no row is left to say it,
  // see ClipList.
  const carry = $derived.by(() => {
    if (!working) return null;
    const { fraction, left } = stepLine(working);
    return { job: working.id, fraction, left, still: stopping };
  });

  let shownNext = $state<{ what: string; left: string; fraction: number; still?: boolean } | null>(
    null,
  );
  let shownSince = 0;
  // What a click brings about shows in the same frame as the click, not on
  // the timer's next tick: the row the Stopped note stood in was empty for
  // up to a quarter of a second after Continue, which read as a flash, and
  // Cancel went on saying Transcribing. Work starting or ending, and a
  // headline marked now, go straight in. Only the engine's own moving from
  // one step to the next waits, below.
  $effect(() => {
    const want = next;
    if (want && (!shownNext || (want.now && (want.what !== shownNext.what || !!want.still !== !!shownNext.still)))) {
      shownSince = Date.now();
      shownNext = { what: want.what, left: want.left, fraction: want.fraction, still: want.still };
    } else if (!want && shownNext) {
      shownNext = null;
    }
  });
  onMount(() => {
    const timer = window.setInterval(() => {
      const want = next;
      if (!want) {
        shownNext = null;
        return;
      }
      // Nothing new to say: the row stays as it is.
      if (!want.what && shownNext) return;
      const what = want.what;
      if (!shownNext || what === shownNext.what) {
        if (!shownNext) shownSince = Date.now();
        shownNext = { ...want, what };
        return;
      }
      // A step that lasted a moment, a turn that came at once, is not
      // flashed up: a new headline waits a second.
      const hold = 1000;
      if (Date.now() - shownSince < hold) {
        shownNext = { ...shownNext, left: want.left, fraction: Math.max(shownNext.fraction, want.fraction) };
        return;
      }
      shownSince = Date.now();
      shownNext = { ...want, what };
    }, 250);
    return () => window.clearInterval(timer);
  });
  const waitNote = $derived.by(() => {
    if (!busy) return "";
    if (finding) {
      return `The model is reading the window from ${clock(from)} to ${clock(to)} and choosing ${count} moments from it. Each one appears here and on the range picker as soon as it is found.${leftOfWork ? ` About ${leftOfWork}.` : ""}`;
    }
    const first = transcribing
      ? `The video is being transcribed on this machine, no cloud and no cost.${leftToGo ? ` About ${leftToGo}.` : ""}`
      : heardWindow
        ? "The search waits while another one finds its clips."
        : "The window is not all transcribed yet.";
    return `${first} Clips are found by themselves once the window is transcribed.`;
  });
  // The whole layout is worked out by the browser, in the stylesheet at
  // the foot of this file, from the size of the app and one number that
  // has nothing to do with it: the shape of the episode. It does not
  // change while the app is being resized, so resizing it costs no JavaScript at all and the
  // workspace keeps up with the edge of the app the way a native app
  // does. Measuring a height, working out another height from it and
  // writing that back is a round trip per frame, and the parts that
  // measure each other never settle in one.
  const shape = $derived(`${source?.width || 16} / ${source?.height || 9}`);
  const ratio = $derived((source?.width || 16) / (source?.height || 9));

  const whole = $derived(from <= 0.5 && to >= duration - 0.5);
  const current = $derived(clips.find((c) => c.key === selected) ?? null);
  // The rows of removed clips whose time is over and which are still
  // closing. Until the last of them has closed, the list opens no row
  // still to come: they came at once and stood above the row still
  // closing, which went down the column as it went.
  let closing = $state<string[]>([]);
  // A removed clip stays in the plan, it only leaves the list.
  // The clip just removed keeps its place in the list for a moment, so the
  // rows do not jump and there is somewhere to put it back from.
  const shown = $derived(clips.filter((c) => !c.rejected || c.key in removed));
  // The rows the list holds, counted from the clips it shows. A search
  // waiting for the transcript opens as many more as it will look for, the
  // same as one that runs. It used to open that many in all, so a list that
  // already held as many clips had no row left for the one that says what
  // is going on, and Search looked as if it had done nothing until the
  // transcript was there. Counting the clips of the plan rather than the
  // clips shown also opened a row for every clip removed.
  // How the last search ended, when it stopped before it was done and
  // nothing is running now: cut off, because the app was closed or fell
  // over in the middle, or failed with a reason. It is said in the first
  // row still to come and points to Continue, and nothing starts by
  // itself. A search called off by hand leaves nothing to say.
  // Why a search failed, as a sentence. The engine's reasons begin in
  // lower case, the way an error does in the log.
  function reasonOf(error?: string, what = "search"): string {
    const said = error?.trim() ?? "";
    if (said) return said[0].toUpperCase() + said.slice(1);
    return what === "search" ? "The search failed, and no reason was given." : "The clip could not be made, and no reason was given.";
  }
  const halted = $derived.by(() => {
    if (!search || busy) return null;
    if (search.state !== "interrupted" && search.state !== "failed") return null;
    const start = search.from ?? 0;
    const end = search.to && search.to > 0 ? search.to : duration;
    const window = `Window ${clock(start)} to ${clock(end)}`;
    const span = { from: start, to: end, window };
    if (search.state === "failed") {
      // Why is said in a toast as it fails, see failedNow. A card is no
      // place to carry a reason, and cut to one line it lost the part
      // that said what to do, issue #311.
      return { ...span, what: "Failed. Click Continue", left: window, full: `${window}. ${reasonOf(search.error)}` };
    }
    // Called off with Cancel, or cut off by the app closing: the same row
    // either way, because either way what it did stays and Continue
    // carries it on. Only the first word says which it was.
    const byHand = search.step === "stopped";
    const what = byHand ? "Stopped. Click Continue" : "Interrupted. Click Continue";
    const how = byHand ? "Cancel stopped the search" : "The app was closed";
    // Stopped before it had heard its window, which is the first half of
    // every search of a window not heard yet: Continue hears the rest of
    // it and then finds.
    if (!covers(saved, start, end)) {
      return {
        ...span,
        what,
        left: transcribedIn(saved, start, end),
        full: `${window}. ${how} while the window was transcribed for it. Continue transcribes the rest of it and then finds the clips`,
      };
    }
    return {
      ...span,
      what,
      left: window,
      full: `${window}. ${how} while the clips were found. Continue looks again`,
    };
  });
  // A search that stopped holds the window it was about, and the window
  // can still be moved. Moved anywhere else, the search is set aside: the
  // row that said it stopped closes, Continue becomes Search again, and Search
  // starts afresh in the window as it is now. Moved back, by hand or with
  // Undo, Continue carries the search on as before. Tim cancelled a search
  // to look somewhere else and could not, because the window stood locked
  // on the range picker until the search was carried on. The Go side says
  // it, from the video's one window, so the row and the video's dot in the
  // sidebar say the same, after a restart too, issue #436.
  const aside = $derived(!!halted && !!search?.aside);
  const stopped = $derived(aside ? null : halted);
  // The row as the clip list shows it. It can be removed, issue #431: it
  // says Removed and offers the search back while its time runs down, the
  // way a removed clip's row does, and then the search is forgotten, its
  // record and all, so the row, the window on the range picker and the
  // video's dot in the sidebar go with it. Its clips on the way go with it
  // too, and say Removed in the same moment, because their time is the
  // search's. Failed is said in the red of an error, as the sidebar and
  // the toast say it.
  const stoppedRow = $derived.by(() => {
    if (!stopped || !search) return null;
    const job = search.id;
    return {
      ...stopped,
      failed: search.state === "failed",
      onremove: () => putAside(job),
      away: awayOf(job),
    };
  });
  // Said in the app's log when the window leaves a stopped search or
  // comes back to it, see lib/said.ts.
  let wasAside = false;
  $effect(() => {
    const now = aside;
    if (now === wasAside) return;
    wasAside = now;
    const w = untrack(() => halted);
    if (!w) return;
    said(
      now
        ? `window: moved off the stopped search's window ${clock(w.from)} to ${clock(w.to)}, Search starts afresh`
        : `window: back on the stopped search's window ${clock(w.from)} to ${clock(w.to)}, Continue carries it on`,
      { level: "info", video: path },
    );
  });

  // The clips on their way, from every job of the episode alike: the
  // search's, from the moment the model names each, and the ones made with
  // I and O, from the moment the key is pressed. See lib/arriving.ts.
  const arrivingNow = $derived(
    arriving(
      jobs.forEpisode(path),
      () => stopping,
      (job) => void api.continueJob(job.id, "").then((j) => jobs.apply(j)),
      {
        remove: (job) => putAside(job.id),
        away: (job) => awayOf(job.id),
      },
    ),
  );

  // A clip made by hand that stopped can be removed, so it is not made,
  // issue #257, and so can a search that stopped, issue #431. Its card
  // says Removed and offers it back for as long as a removed clip's row
  // does, and only then is the work called off, and the Go side forgets
  // its record, see cancelSteps. The time is the
  // episode's, kept by the job the way a search's row that found nothing
  // is, see lib/removed.ts: it runs on while another video is open, and
  // work whose time ran out while it was away is called off on coming
  // back. By the job: the moment its time runs out, and 0 once it has.
  let putAway = $state<Told>(untrack(() => takeUpTold(path, Date.now(), "put away")));

  function putAside(job: string) {
    putAway = { ...putAway, [job]: Date.now() + secondThoughts * 1000 };
    said(`clip list: the work of ${job} that stopped removed, ${secondThoughts} s to put it back`, { level: "info", video: path });
  }

  function awayOf(job: string): Away | undefined {
    if (!(job in putAway)) return undefined;
    const until = putAway[job];
    return {
      // Once called off, the row has no time left, and stays until the
      // job says it is gone.
      spent: until === 0 ? secondThoughts : toldSpent(until),
      // A search is removed in its row and in each of its cards on the
      // way at once, and the hand on any of them holds them all.
      together: `away ${job}`,
      onputback: () => {
        if (putAway[job] === 0) return;
        const { [job]: _, ...rest } = putAway;
        putAway = rest;
        said(`clip list: the work of ${job} that stopped put back`, { level: "info", video: path });
      },
      onend: () => callOff(job),
      onresume: (left) => {
        if (putAway[job]) putAway = { ...putAway, [job]: Date.now() + left * 1000 };
      },
    };
  }

  function callOff(job: string) {
    if (putAway[job] === 0) return;
    putAway = { ...putAway, [job]: 0 };
    said(`clip list: the work of ${job} that stopped is called off and forgotten`, { level: "info", video: path });
    send(job);
  }

  // Work that could not be called off is as it was, Stopped with
  // Continue, and why is said whole in a toast, never pressed into its
  // row or card, where it is cut to a line, issue #431.
  function send(job: string) {
    api.cancelJob(job).catch((err) => {
      const { [job]: _, ...rest } = putAway;
      putAway = rest;
      const why = `Could not remove it: ${errorText(err)}`;
      said(`clip list: the work of ${job} that stopped could not be removed, ${why}`, { level: "warn", video: path });
      toasts.say(where(), why, { video: path, report: true });
    });
  }

  // Work whose time ran out while the video was away is called off now.
  for (const [job, until] of Object.entries(untrack(() => putAway))) if (until === 0) send(job);

  // A card whose work was called off goes when its job says so, and what
  // was kept of its time goes with it. So does one whose job runs again.
  // A job not heard of yet, as the video opens, keeps its time.
  $effect(() => {
    const here = new Set(arrivingNow.filter((a) => a.stopped).map((a) => a.job));
    // The search's own row, which stands while it has no clip on the way.
    if (halted && search) here.add(search.id);
    const known = new Set(jobs.forEpisode(path).map((j) => j.id));
    untrack(() => {
      const kept = Object.entries(putAway).filter(([job, until]) => here.has(job) || (until !== 0 && !known.has(job)));
      if (kept.length < Object.keys(putAway).length) putAway = Object.fromEntries(kept);
    });
  });
  // Everything that puts the clip list on screen takes a ticket: a read of
  // the whole list and the answer to an edit alike. A read asked for before
  // an edit can answer after it, and it knows nothing of the edit, so a clip
  // removed a moment ago came back, and a trim looked undone, until the list
  // was read again. Only the newest answer is used.
  const listed = new Newest();
  // A clip that has just been written keeps its card until the list has
  // read it, so the card becomes the clip in one step: never a gap where
  // it was, and never the two of them at once. See OnTheWay in
  // lib/arriving.ts. The card goes in the same step that puts the list on
  // screen, with the first read asked after the clip was written, whichever
  // read answers first.
  const cardsOnTheWay = new OnTheWay();
  let keptRead = $state(0);
  const onTheWay = $derived(cardsOnTheWay.cards(arrivingNow, listed.next(), keptRead));
  $effect(() => {
    void onTheWay;
    if (cardsOnTheWay.unread(listed.next())) void refreshClips(true);
  });
  // The search the list last counted rows for, and how many of its clips
  // the newest read of the list holds: what it had written when the read
  // was asked, because the plan is written before the job says so.
  let searchSeen = $state("");
  let readWritten = $state(0);
  $effect(() => {
    const id = working?.id;
    if (!id || id === untrack(() => searchSeen)) return;
    searchSeen = id;
    readWritten = 0;
  });

  // How many clips the search has written and has on the way. It says so
  // itself, so a clip made by hand while it runs is not counted as one of
  // its own.
  const searchTook = $derived((working?.written ?? 0) + (working?.underway?.length ?? 0));
  // Clips the search has written that no read of the list has brought in
  // yet, and that have no card on the way either: a clip can be written
  // between two job events and never be on the way in either. Each keeps
  // a row of its own until the list has it, during the search and after
  // it ended alike, so the list is never a row short while the read is in
  // the air.
  const lastSearch = $derived(jobs.forEpisode(path).find((j) => j.id === searchSeen));
  const unread = $derived(
    Math.max(
      0,
      (lastSearch?.written ?? 0) - readWritten - onTheWay.filter((a) => a.held && a.job === searchSeen).length,
    ),
  );
  // Every clip the episode had was removed, and nothing is on its way:
  // the first row still to come says so, in the words a row that says how
  // work ended uses, rather than leaving empty rows that look like nothing
  // happened or something broke. It gives no number: the plan keeps every
  // clip ever removed, from earlier searches and earlier days too, so a
  // count of them was more than the list had just shown.
  const emptied = $derived.by(() => {
    if (busy || stopped || shown.length > 0 || onTheWay.length > 0 || closing.length > 0) return null;
    if (clips.length === 0 || !clips.every((c) => c.rejected)) return null;
    return { what: "All clips removed", left: "Search looks for others" };
  });
  // The newest search ended and found nothing beyond the clips there are,
  // which is an answer, not a failure. The row its next clip would have
  // appeared in says so: Tim pressed Search on a video with its one clip, and
  // the row that said Finding closed again with nothing to say what had
  // come of it. It is news rather than a state, so it runs down for as
  // long as a removed clip's row does, in a neutral colour, and closes.
  // Each search that finds nothing says so once, so Search pressed again and
  // again is a row that says Finding and then this, every time.
  //
  // Its time is kept by the search's job, the way a removed clip's is, see
  // told. It starts once the row is on screen, so the row is read here
  // without starting anything. Taken up before anything is drawn, so a
  // row coming back runs on from where it was rather than from full.
  let told = $state<Told>(untrack(() => takeUpTold(path)));
  const foundNothing = $derived.by(() => {
    if (busy || stopped) return null;
    const last = jobs.forEpisode(path).findLast((j) => j.kind === "search");
    // A Continue counts what the search it carries on wrote: one called
    // off once its clips were written had nothing left to write, and its
    // row said Nothing new found, Tim's report of 25 October, 21.40.
    if (last?.state !== "done" || (last.written ?? 0) + (last.carried ?? 0) > 0) return null;
    const job = last.id;
    if (told[job] === 0) return null;
    const fades = {
      spent: toldSpent(told[job]),
      onend: () => {
        told = { ...told, [job]: 0 };
      },
      onresume: (left: number) => {
        told = { ...told, [job]: Date.now() + left * 1000 };
      },
    };
    const start = last.from ?? 0;
    const end = last.to && last.to > 0 ? last.to : duration;
    const window = `Window ${clock(start)} to ${clock(end)}`;
    // Shortest as the search was asked for it, not as it is now.
    if (duration > 0 && duration < (last.min || min)) {
      return {
        job,
        what: "Nothing new found",
        left: "All clips already found",
        fades,
        full: "The video is shorter than Shortest, so it holds one clip, and that clip is already in the list",
      };
    }
    return {
      job,
      what: "Nothing new found",
      left: window,
      full: `${window}. The search found no moments beyond the clips there are`,
      fades,
    };
  });
  // The row's time starts the first time it is shown.
  $effect(() => {
    const job = foundNothing?.job;
    if (job && !(job in told)) told = { ...told, [job]: Date.now() + secondThoughts * 1000 };
  });
  // The news first: with every clip removed, a search that found nothing
  // says so, and the row that stands for an empty list comes after it.
  const note = $derived(foundNothing ?? emptied);
  const coming = $derived(
    shown.length +
      onTheWay.length +
      (busy
        ? // The rows the search itself was asked for, not what the
          // workspace would ask for now: the first search of an episode
          // is asked for by the Go side.
          (working?.whole ? 0 : Math.max(0, (working?.count || count) - searchTook)) + unread
        : shown.length === 0 && onTheWay.length === 0 && closing.length === 0
          ? Math.max(count, unread)
          : // Clips a search wrote before it was cut off stay, and one row
            // after them says what became of the rest.
            unread + (stopped || foundNothing ? 1 : 0)),
  );
  // Whether the window has been searched before, all of it or a part.
  // Search then looks there again, for moments its searches did not bring,
  // and every clip there is stays.
  const searchedBefore = $derived(timesIn(coverage.passes, from, to) > 0);
  // A clip taken out leaves the track at once. Its row stays a moment
  // longer, but that row is what became of it, not a clip.
  //
  // A clip on its way has its mark from the moment its card has its place,
  // where the card says it lies, breathing the way everything not there
  // yet does, and it keeps the mark when it is written: the mark is known
  // by the clip it will be. The marks came only with the written clip, so
  // the range picker and the clip timeline said where a clip lay long
  // after its card did.
  const marks = $derived.by(() => {
    const written = shown
      .filter((c) => !(c.key in removed))
      .map((c) => ({ key: c.key, start: c.start, end: c.end, rendered: c.rendered }));
    const here = new Set(written.map((m) => m.key));
    const coming = onTheWay
      .filter((a) => !(a.clip && here.has(a.clip)) && !a.away)
      .map((a) => {
        const first = a.pieces?.[0]?.[0] ?? a.start;
        const last = a.pieces?.[a.pieces.length - 1]?.[1] ?? a.end;
        return { key: a.clip ?? a.key, start: first, end: last, rendered: false, arriving: true, pick: a.key };
      });
    return [...written, ...coming];
  });
  // Whether the render running is of this clip. The job says what it is
  // of from the moment it is queued: its result only says so once it is
  // over, which is how the button never saw its own render running.
  const renderingCurrent = $derived(
    !!renderingJob &&
      !!current &&
      renderingJob.plan === current.plan &&
      (!renderingJob.clips?.length || renderingJob.clips.includes(current.id)),
  );

  // The free room changes whenever a search finishes, so it is taken again
  // with the rest of the episode.
  // The same for the episode's own state and what has been searched: a
  // read that answers late never puts back what a newer one said.
  const statusRead = new Newest();
  const coverageRead = new Newest();

  async function refreshCoverage() {
    const ticket = coverageRead.send();
    try {
      const now = await api.coverage(path);
      if (coverageRead.keep(ticket)) coverage = now;
    } catch {
      if (coverageRead.keep(ticket)) coverage = { passes: [] };
    }
  }

  // The window put back where the app places it, a double-click on its
  // marks. The Go side places it, as it places every window the app
  // chooses, see chosenWindow.
  async function placeWindowBack() {
    try {
      drawWindow(await api.placeWindow(path));
    } catch (err) {
      oops(errorText(err));
    }
  }

  // The video's window, drawn as the Go side has it. It fits what a search
  // can do with it here, or is fitted and written back, see keepWindow.
  function drawWindow(w: KeptWindow | null) {
    if (!w || w.to <= w.from || duration <= 0) return;
    from = w.from;
    to = Math.min(w.to, duration);
    length = Math.max(w.length, to - from);
    keepWindow();
  }

  // How long the window was made, by the app or by a hand on its marks.
  // A window cut short at the end of the episode is still this long when
  // it starts over, see followingWindow.
  let length = $state(0);

  // The window dragged by its marks on the range picker, moved whole or by
  // one edge. An edge stops where the window would hold too few clips or
  // more than the model reads, which the range picker works out and says,
  // see least and most on it. When the hand lets go it is put inside what
  // a search can do with it there.
  function moveWindow(start: number, end: number, done: boolean) {
    from = start;
    to = end;
    if (done) {
      fit();
      length = to - from;
      rememberWindow(true);
    }
  }

  // The window put back inside what a search can do with it, whenever
  // what that is changes: the room, or the clips asked for. Not while clips
  // are being found for it, because then it is the window being searched.
  // Nor while a search stands stopped on it, because it is the window
  // Continue carries on. A window fitted is written back, so the window
  // drawn is the video's window.
  function keepWindow() {
    if (fit()) rememberWindow();
  }

  function fit(): boolean {
    if (busy || stopped || duration <= 0 || to <= from) return false;
    const kept = fitWindow({ from, to }, reachAnywhere, leastLong, duration);
    const moved = Math.abs(kept.from - from) > 0.001 || Math.abs(kept.to - to) > 0.001;
    if (Math.abs(kept.from - from) > 0.001) from = kept.from;
    if (Math.abs(kept.to - to) > 0.001) to = kept.to;
    return moved;
  }

  // keep is false when another window is on its way, see load, so the
  // window still drawn is not fitted and written back over it.
  async function refreshRoom(keep = true) {
    try {
      roomView = await api.room(path);
    } catch {
      roomView = null;
    }
    if (keep) keepWindow();
  }

  // The window the workspace opens with: the video's window, which the
  // Go side keeps and places, see chosenWindow. A search that runs has
  // its window drawn when it is heard of, see followed, and it is the
  // same window, because every search starts on the video's window.
  async function openWindow(asked: Promise<KeptWindow | null> = api.chosenWindow(path)) {
    const was = path;
    let kept: KeptWindow | null = null;
    try {
      kept = await asked;
    } catch {
      kept = null;
    }
    if (path !== was) return;
    drawWindow(kept);
  }

  // The window is written the moment a hand moves it, or the workspace
  // fits it, so the Go side's window is the window drawn.
  function rememberWindow(byHand = false) {
    if (duration <= 0 || to <= from) return;
    api.chooseWindow(path, from, to, Math.max(length, to - from), byHand).catch(() => {});
  }

  // The clip the workspace opens on: the one this episode was last worked
  // on if it is still there, otherwise the first one. An episode with clips
  // in it and nothing chosen is a workspace with an empty captions column
  // and an empty clip panel, waiting for a click that says nothing.
  //
  // The playhead follows the clip because that is what choosing a clip
  // already does. Where the playhead stood a moment before the app was
  // closed is not kept: no editor keeps it, and the start of the clip is a
  // place that means something.
  async function openOnAClip(key: string) {
    if (selected || !clips.length) return;
    const clip = clips.find((c) => c.key === key) ?? clips[0];
    await select(clip.key);
  }


  // One clip as an edit left it.
  function putClip(updated: ClipEntry) {
    listed.keep(listed.send());
    clips = clips.map((c) => (c.key === updated.key ? updated : c));
  }

  // The list read again. A read for what a search wrote, found, keeps
  // every clip the read says is as it was, the very same clip: a search
  // only adds clips, and a running one reads the list every two seconds.
  // Each read made every clip a new one, the clip on screen too, and its
  // captions were asked for and drawn again each time, the clip timeline
  // and the caption box with them, while nothing about it had changed.
  // Plan row 2.242. Every other read is for an edit of the clips or of
  // their captions, and takes them all as new, so the captions of the clip
  // on screen are read again with them.
  async function refreshClips(found = false) {
    const ticket = listed.send();
    const written = lastSearch?.written ?? 0;
    try {
      const list = (await api.clips(path)) ?? [];
      if (listed.keep(ticket)) {
        // The list, and the cards of the clips it holds going, in one step.
        clips = found ? unchanged(clips, list) : list;
        readWritten = written;
        keptRead = ticket;
      }
    } catch (err) {
      oops(errorText(err));
      if (listed.keep(ticket)) keptRead = ticket;
    }
  }

  // The workspace opening: where the playhead opens comes first, before
  // anything else is asked. The episode, its picture, its clips and the
  // clip it was last worked on are asked at once, the video preview and
  // the playhead are put on that clip the moment they are in, and only
  // then the rest is read, how far it was searched, the room for windows
  // and the window, none of which the video preview waits for. They were
  // read first, one after another, with the video preview waiting.
  async function openEpisode() {
    try {
      const ticket = statusRead.send();
      const [now, picture, , key] = await Promise.all([
        api.episode(path),
        // A file that is missing has no picture to say.
        api.source(path).catch(() => null),
        refreshClips(),
        // An episode that cannot say has simply never been opened.
        api.chosenClip(path).catch(() => ""),
      ]);
      if (statusRead.keep(ticket)) status = now;
      tell(
        `the Go side answered: ${status ? (status.missing ? "the file is missing" : "the file is there") : "no status"}, picture ${picture ? `${picture.width}x${picture.height} at ${picture.fps} a second, ${picture.duration.toFixed(3)} s` : "none"}, ${clips.length} clips, opens on ${key ? `the clip ${key}` : "no clip"}`,
      );
      if (!status) return;
      if (!status.missing) source = picture;
      keepRemovedTrue();
      // The video preview is drawn for the picture before the clip is
      // chosen, so choosing it puts the playhead there.
      await tick();
      await openOnAClip(key);
    } catch (err) {
      oops(errorText(err));
    } finally {
      opened = true;
      tell(`opened, the playhead at ${time.toFixed(3)} s`);
    }
    try {
      if (!status || status.missing) return;
      // Asked together, and used in this order: the window is placed from
      // what was searched and fitted to the room. One after another they
      // kept the workspace behind the one on screen three answers longer.
      const kept = api.chosenWindow(path);
      kept.catch(() => {});
      await Promise.all([refreshCoverage(), refreshRoom()]);
      if (source) await openWindow(kept);
    } catch (err) {
      oops(errorText(err));
    } finally {
      settled = true;
      tell("settled, the window and its room are read");
    }
  }

  // moved is the window a search that ended moved on to, asked for with
  // the rest. It is drawn with the clips the search found, and the window
  // still drawn is not fitted to the new room and written back over it,
  // found by the fourth review of #458.
  async function load(moved?: Promise<KeptWindow | null>) {
    const was = path;
    try {
      const ticket = statusRead.send();
      const now = await api.episode(path);
      if (statusRead.keep(ticket)) status = now;
      if (!status) return;
      // A file that was missing as the workspace opened and is there now.
      const found = !source && !status.missing;
      if (found) source = await api.source(path);
      if (!status.missing) await refreshCoverage();
      if (!status.missing) await refreshRoom(!moved);
      if (found && source) await openWindow();
      const next = moved ? await moved.catch(() => null) : null;
      await refreshClips();
      movingOn = false;
      if (next && path === was) drawWindow(next);
      keepRemovedTrue();
    } catch (err) {
      oops(errorText(err));
    } finally {
      movingOn = false;
    }
  }

  // Putting the playhead somewhere is a jump, not a drift, so the clip
  // timeline goes there too. A view moved by hand otherwise stays where it
  // was put, which is what the crosshair in the row below it is for.
  function seekTo(t: number, about?: About, wordStep = false) {
    player?.seek(t, about, wordStep);
    timeline?.fit(t);
  }

  // The chosen card, brought far enough into the list to be read. Walking
  // the list with the arrows otherwise chooses cards that are not on
  // screen, and the veil over each end means a card only just inside the
  // list is a card half faded away. A card walked to out of view turns
  // the page, the way the transcript does as the video plays: walked
  // down, it comes to the top, and walked up, to the foot. Brought just
  // into view, it went on standing at the foot as the walk went down,
  // found by Tim.
  function showChosen(step: 0 | 1 | -1 = 0) {
    const list = root?.querySelector<HTMLElement>(".pane .list");
    const card = list?.querySelector<HTMLElement>(".pick.current, li.next.current")?.closest("li");
    if (!list || !card) return;
    // The same veil the list fades its ends with, from the stylesheet.
    const veil = 16;
    const box = card.getBoundingClientRect();
    const view = list.getBoundingClientRect();
    const above = view.top + veil - box.top;
    const below = box.bottom - (view.bottom - veil);
    if (above <= 0.5 && below <= 0.5) return;
    if (step > 0 || (!step && above > 0)) list.scrollTop -= above;
    else list.scrollTop += below;
  }

  // seek is off for a clip chosen while the video plays, which carries on
  // where it is.
  // jump is off for a clip chosen by clicking one of its sentences, so the
  // sentences stay where they are rather than go to the clip's first.
  // step is the way the clip list was walked to it, see showChosen.
  async function select(key: string, seek = true, jump = true, step: 0 | 1 | -1 = 0) {
    selected = key;
    if (jump) chosenTimes++;
    // The episode remembers what is being worked on, so opening it again
    // opens on the same clip. Forgetting it is no reason to say anything.
    api.chooseClip(path, key).catch(() => {});
    const clip = clips.find((c) => c.key === key);
    // On the clip, at its start: picking a clip is a gesture about it.
    if (clip && seek) player?.seek(clip.start, "clip");
    // Picking a clip puts the clip timeline back on it, the same as the
    // crosshair in the row below, even when it is the clip that was already
    // selected and the timeline was moved by hand since.
    await tick();
    showChosen(step);
    timeline?.fit();
  }

  // Removing a clip is one click, so putting it back is one click too, for
  // a few seconds. Every clip removed keeps its own row and its own
  // time, and removing another leaves them alone: there was one for the
  // whole list, and the second clip removed took the first one's way back
  // with it. The time is the row's own, see ClipList: it runs down along
  // the foot of the row, stands still under the pointer, and the row says
  // when it is over. It is the episode's time, see lib/removed.ts, so it
  // runs on while another episode is open and the rows are there again on
  // coming back. A list read again, after a search, a render or an undo,
  // leaves them as they are: it used to take every one of them away.
  let removed = $state<Record<string, Removed>>({});

  onMount(() => {
    removed = takeUp(path);
    return () => {
      setAside(path, $state.snapshot(removed));
      setAsideTold(path, $state.snapshot(told));
      setAsideTold(path, $state.snapshot(putAway), "put away");
    };
  });

  // What the clip list is told about the clips removed: how much of its
  // time each has used, for the ones the plan still has removed. An undo
  // that put one back leaves it a clip again, not a row to put back.
  const removedRows = $derived(
    Object.fromEntries(
      clips.filter((c) => c.rejected && c.key in removed).map((c) => [c.key, spent(removed[c.key])]),
    ),
  );

  // The clips an undo or a redo has put back, or removed again, are no
  // longer waiting to be put back.
  function keepRemovedTrue() {
    const still = Object.entries(removed).filter(([k]) => clips.some((c) => c.key === k && c.rejected));
    if (still.length < Object.keys(removed).length) removed = Object.fromEntries(still);
  }

  function held(key: string, left: number) {
    if (!(key in removed)) return;
    removed = { ...removed, [key]: { ...removed[key], until: Date.now() + left * 1000 } };
  }

  function forget(key: string) {
    if (!(key in removed)) return;
    const { [key]: _, ...rest } = removed;
    removed = rest;
    closing = [...closing, key];
  }

  function closed(key: string) {
    closing = closing.filter((k) => k !== key);
  }

  // A clip is always chosen while there is one. Removing the chosen clip
  // chooses the next in the list, or the one before it when it was the
  // last, the way Finder and Mail move the selection on from what was
  // deleted. It used to leave nothing chosen and the clip timeline empty.
  // Found by Tim.
  async function removeClip(clip: ClipEntry) {
    // The clip beside it in the list as it stands before the remove, the
    // same list the shift and arrow keys walk.
    const list = shown.filter((c) => !(c.key in removed));
    const here = list.findIndex((c) => c.key === clip.key);
    const beside = here < 0 ? undefined : (list[here + 1] ?? list[here - 1]);
    const at = time;
    try {
      const updated = await api.removeClip(path, clip.plan, clip.id, true);
      putClip(updated);
      const chosen = selected === updated.key;
      const then = chosen && beside ? beside.key : "";
      if (chosen) {
        said(`clip list: removed ${updated.key}, which was chosen, ${then ? `chose ${then}` : "no clip left to choose"}`, {
          level: "info",
          video: path,
        });
        if (then) select(then);
        else selected = "";
      }
      removed = {
        ...removed,
        [updated.key]: { clip: updated, until: Date.now() + secondThoughts * 1000, chosen, then, at },
      };
    } catch (err) {
      oops(errorText(err));
    }
  }

  // Putting a clip back takes back the remove and nothing more. The clip
  // chosen stays chosen and the play goes on where it is: putting back a
  // clip while another played used to choose the one put back and jump
  // the play to its start. Found by Tim. Only a clip that was the one
  // chosen when it was removed is chosen again, and only while the clip
  // its remove chose instead is still the one chosen, with the playhead
  // back where it was when it was removed.
  async function putClipBack(key: string) {
    const was = removed[key];
    if (!was) return;
    const clip = was.clip;
    try {
      const updated = await api.removeClip(path, clip.plan, clip.id, false);
      putClip(updated);
      forget(key);
      const again = !!was.chosen && selected === (was.then ?? "");
      said(`clip list: put back ${updated.key}, ${again ? "chosen again as it was" : `the clip chosen stays ${selected || "none"}`}, ${paused ? "paused" : "playing"}`, {
        level: "info",
        video: path,
      });
      if (again) {
        void select(updated.key, false);
        if (was.at !== undefined) player?.seek(was.at, "clip");
      }
    } catch (err) {
      oops(errorText(err));
    }
  }

  // A gesture on the clip timeline let go of: an edge trimmed, a part taken
  // out or put back, a cut moved. The engine makes the change it showed
  // while the hand moved, and answers with the clip as it is now.
  async function reshape(clip: ClipEntry, g: Gesture, playhead: [number, number]) {
    try {
      const updated = await api.reshape(path, clip.plan, clip.id, g, playhead);
      putClip(updated);
    } catch (err) {
      oops(errorText(err));
    }
  }

  // One frame of the episode, which is what a thumbnail stands on.
  const frameLen = $derived(source && source.fps > 0 ? 1 / source.fps : 1 / 30);
  // Where the picture's frames begin, see frameAt in lib/flow.ts.
  const frameStart = $derived(source?.videoStart ?? 0);
  // Where the frame on the video preview begins, as its queue drew it, NaN
  // before the first. A mark, a thumbnail and the crop dragged belong to
  // the frame on screen, which is the playhead's own frame but at the end
  // of a clip played through, where the playhead stands past the last
  // frame and the picture shows the last. So that moment, and the playhead
  // only before there is a picture.
  let shownStart = $state(NaN);
  const shownMoment = $derived(Number.isFinite(shownStart) ? shownStart : time);
  // The thumbnail on the frame shown, told by the frame and never by how
  // far the playhead is from one, because the playhead is never quite
  // where it was put.
  const thumbHere = $derived.by(() => {
    if (!current) return null;
    const f = frameAt(shownMoment, frameLen, frameStart);
    return current.thumbnails?.find((t) => frameAt(t, frameLen, frameStart) === f) ?? null;
  });
  // Whether the short shows the frame on the video preview, which is where
  // a thumbnail or a keyframe can be. Told by the frame, as the engine
  // tells it, not by the playhead to the millisecond: a play of the clip
  // leaves the playhead on its end, past its last frame, and steps of a
  // frame leave it a hair short of a frame's start, where the clip's first
  // frame was on screen and the buttons said it was not in the short.
  // Found by Tim on #192.
  const inShort = $derived(!!current && pieceHolding(current.segments, shownMoment) >= 0);

  // A thumbnail added, moved or removed, shown at once and saved straight
  // after. A from below nought adds, a to below nought removes.
  async function setThumbnail(clip: ClipEntry, from: number, to: number) {
    const ms = (t: number) => Math.round(t * 1000);
    const was = clip.thumbnails ?? [];
    const next = was.filter((t) => from < 0 || ms(t) !== ms(from));
    if (to >= 0) next.push(Math.round(to * 1000) / 1000);
    next.sort((a, b) => a - b);
    clips = clips.map((c) => (c.key === clip.key ? { ...c, thumbnails: next } : c));
    try {
      const updated = await api.setThumbnail(path, clip.plan, clip.id, from, to);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      oops(errorText(err));
      clips = clips.map((c) => (c.key === clip.key ? { ...c, thumbnails: was } : c));
    }
  }

  // The keyframe on the frame shown, told by the frame, the way thumbHere
  // is. Plan row 3.26.
  const cropHere = $derived(
    current && source ? markOn(current.cropMarks, shownMoment, source.fps, source.videoStart ?? 0) : null,
  );

  // A keyframe added, moved or removed: the diamond shows at once, and the
  // glide it makes comes back with the clip. A from below nought adds one
  // at to with its crop at x, a to below nought removes the one at from.
  async function setCropMark(clip: ClipEntry, from: number, to: number, x: number) {
    const ms = (t: number) => Math.round(t * 1000);
    const was = clip.cropMarks ?? [];
    const moving = was.find((m) => from >= 0 && ms(m.at) === ms(from));
    const next = was.filter((m) => m !== moving);
    if (to >= 0)
      next.push({
        at: Math.round(to * 1000) / 1000,
        x: moving?.x ?? x,
        turn: moving?.turn ?? rotationAt(clip.rotations, to),
      });
    next.sort((a, b) => a.at - b.at);
    clips = clips.map((c) => (c.key === clip.key ? { ...c, cropMarks: next } : c));
    try {
      const updated = await api.setCropMark(path, clip.plan, clip.id, from, to, x);
      putClip(updated);
    } catch (err) {
      oops(errorText(err));
      clips = clips.map((c) => (c.key === clip.key ? { ...c, cropMarks: was } : c));
    }
  }

  // The button and C: a keyframe on the frame under the playhead, with
  // the crop where it stands there now, or the one there removed, so what
  // one click adds one click takes away.
  function toggleCropMark() {
    if (!current || renderingCurrent) return;
    if (cropHere !== null) {
      void setCropMark(current, cropHere.at, -1, -1);
      return;
    }
    const left = player?.cropLeftNow();
    if (!inShort || left === null || left === undefined) return;
    // The start of the frame, the moment a mark is kept at.
    const at = frameStart + frameAt(shownMoment, frameLen, frameStart) * frameLen;
    void setCropMark(current, -1, at, Math.round(left));
  }

  // The button and T: the frame under the playhead becomes a thumbnail, or
  // stops being one, so what one click adds one click takes away.
  function toggleThumbnail() {
    if (!current || renderingCurrent) return;
    if (thumbHere !== null) {
      void setThumbnail(current, thumbHere, -1);
    } else if (inShort) {
      // The middle of the frame, so the picture and the playhead agree on
      // which frame it is whichever way either rounds.
      void setThumbnail(current, -1, frameMiddle(frameAt(shownMoment, frameLen, frameStart), frameLen, frameStart));
    }
  }

  // I and O make a clip at the playhead, the way In and Out mark a clip in
  // every video editor: I starts it with the sentence the playhead stands
  // in, O ends it there, for a moment noticed once it has passed. It is a
  // job like a search, and any number can be on their way at once, beside
  // a search too. Its card is in the list from the moment it is asked for,
  // see lib/arriving.ts, and the clip is chosen when it is written, by the
  // rule a search's first clip is: unless another has been chosen since,
  // or the video plays.
  //
  // Everything the press changes shows in the frame it lands in: the key's
  // button wears the beam at once, and the clip on its way is chosen, so
  // its card is brought into view with the beam on it and its frame is on
  // the clip timeline, first as long as Shortest from the playhead, then
  // on its sentences as soon as the engine has them. When it is written,
  // the chosen card becomes the clip, if nothing else has been chosen
  // since. Any number can be asked for at once, so the buttons never wait.
  const madeHere = new Map<string, string>();
  let pressed = $state<"in" | "out" | null>(null);
  const makingIn = $derived(pressed === "in" || handRunning.some((j) => !j.backward));
  const makingOut = $derived(pressed === "out" || handRunning.some((j) => !!j.backward));
  async function makeClip(backward: boolean) {
    if (duration <= 0) return;
    pressed = backward ? "out" : "in";
    try {
      const job = await api.makeClip(path, time, backward);
      jobs.apply(job);
      // Asked for here, so a failure says why in a toast however soon it
      // comes, the store's news of it before this answer too.
      const now = jobs.list.find((j) => j.id === job.id) ?? job;
      failures.asked(now);
      if (now.state === "failed") return;
      const key = `${job.id}/1`;
      madeHere.set(job.id, key);
      selected = key;
      await tick();
      showChosen();
    } catch (err) {
      oops(errorText(err));
    } finally {
      pressed = null;
    }
  }
  $effect(() => {
    for (const job of jobs.forEpisode(path)) {
      const before = madeHere.get(job.id);
      if (before === undefined || job.state === "running" || job.state === "queued") continue;
      madeHere.delete(job.id);
      const key = job.result;
      if (job.state !== "done" || !key) continue;
      landing.set(before, key);
      void refreshClips();
    }
  });
  // A clip made here, by its key on its way, and the key it is written
  // under, from the moment its card says it, which is before it is
  // written, or else from its job's result. The chosen card becomes the
  // clip in the same step the list brings it in, whichever read of the
  // list that is. Chosen once its job was over and the read asked for it
  // had come back, it was a frame or more late: the job says it has
  // nothing on the way before it says it is done, a read in between
  // brought the clip in and took the card away, and for those frames no
  // card was chosen and the clip timeline drew it dimmed.
  const landing = new Map<string, string>();
  $effect(() => {
    const made = new Set(madeHere.values());
    for (const a of onTheWay) if (a.clip && made.has(a.key)) landing.set(a.key, a.clip);
  });
  $effect(() => {
    const list = clips;
    for (const [before, key] of landing) {
      if (!list.some((c) => c.key === key)) continue;
      landing.delete(before);
      untrack(() => {
        if (selected !== before) return;
        void select(key, paused);
        // Playing, the playhead stays on the clip it was made on, which is
        // written now under its own key. See claim in Player.svelte.
        if (!paused) player?.claim(before);
      });
    }
  });
  // The clip on its way that is chosen, drawn on the clip timeline the way
  // a clip is: its frame, fitted, which cannot be edited until it is
  // written. Before its sentences are known it is as long as Shortest,
  // from the playhead for I and up to it for O.
  const making = $derived.by((): ClipEntry | null => {
    const a = onTheWay.find((x) => x.key === selected);
    if (!a) return null;
    return { key: a.key, segments: wayPieces(a) } as unknown as ClipEntry;
  });
  // What a clip on its way covers: its pieces, once its pauses are cut,
  // which is before its crop is placed, and until then its frame.
  function wayPieces(a: Arriving): { start: number; end: number }[] {
    if (a.pieces?.length) return a.pieces.map(([start, end]) => ({ start, end }));
    let from = a.start;
    let to = a.end;
    if (to - from < 0.5) {
      const job = handWork.find((j) => a.key.startsWith(`${j.id}/`));
      from = job?.backward ? Math.max(0, a.start - min) : a.start;
      to = job?.backward ? a.start : Math.min(duration, a.start + min);
    }
    return [{ start: from, end: to }];
  }
  // Whether the playhead stands in a clip, from its start to its end, cuts
  // included: a clip in the list or one on its way, chosen or not, playing
  // or not. I and O make no clip there. The clip is already there, and its
  // edges are dragged to where it should be. So a play of a clip never
  // has its clip changed under it by I or O.
  const inClip = $derived.by(() => {
    const t = time;
    const covers = (pieces: { start: number; end: number }[]) =>
      pieces.length > 0 &&
      t >= Math.min(...pieces.map((p) => p.start)) &&
      t <= Math.max(...pieces.map((p) => p.end));
    return clips.some((c) => !c.rejected && covers(c.segments)) || onTheWay.some((a) => covers(wayPieces(a)));
  });
  // Where I or O would make no clip, because nothing is said after the
  // playhead or before it, they are not offered, so a card that can only
  // fail is never made, found by Tim on #456.
  const noIn = $derived(nothingToMark(time, false, saved, status?.said, duration));
  const noOut = $derived(nothingToMark(time, true, saved, status?.said, duration));
  // The clip on its way that was just asked for is followed until it is
  // written: brought into view once its card has slid open, and again
  // whenever it moves in the list, and with the video paused the playhead
  // is put where its frame starts, so the picture is of the clip being
  // made. Brought into view in the frame it was asked for only, the card
  // was still sliding open, at no height, and came to rest half under the
  // foot of the list. Where the playhead is not put anywhere, while the
  // video plays, it is on the clip from the moment the clip is there,
  // see claim in Player.svelte.
  let followingMade = "";
  $effect(() => {
    const m = making;
    if (!m || ![...madeHere.values()].includes(m.key)) return;
    const start = m.segments[0].start;
    const mark = `${m.key}:${start}`;
    if (mark === followingMade) return;
    const first = !followingMade.startsWith(`${m.key}:`);
    followingMade = mark;
    untrack(() => {
      // The seek is a gesture about a clip the video preview does not have
      // yet, so the playhead is put on it after.
      if (paused && Math.abs(time - start) > 0.05) {
        player?.seek(start);
        player?.claim();
      } else if (first) player?.claim();
    });
    // After the card has slid open and the list has moved it, 200 ms and
    // 180 ms in ClipList.svelte.
    setTimeout(showChosen, 240);
  });

  function inOutKey(event: KeyboardEvent) {
    const out = event.key === "o" || event.key === "O";
    if (!out && event.key !== "i" && event.key !== "I") return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
    if (event.defaultPrevented || event.repeat) return;
    if (keysElsewhere() || asleep(root)) return;
    if (inClip || (out ? noOut : noIn)) return;
    event.preventDefault();
    void makeClip(out);
  }

  // L switches loop on and off, the loop button's key, the way T is the
  // thumbnail's and I and O make a clip.
  function loopKey(event: KeyboardEvent) {
    if (event.key !== "l" && event.key !== "L") return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
    if (event.defaultPrevented || event.repeat) return;
    const on = document.activeElement as HTMLElement | null;
    const tag = on?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || on?.isContentEditable) return;
    if (document.querySelector("dialog[open]") || asleep(root)) return;
    if (!current) return;
    event.preventDefault();
    looping = !looping;
  }

  function rotateKey(event: KeyboardEvent) {
    if (event.key !== "r" && event.key !== "R") return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
    if (event.defaultPrevented || event.repeat) return;
    if (keysElsewhere() || asleep(root)) return;
    if (!current) return;
    event.preventDefault();
    void rotatePart();
  }

  // C puts a keyframe on the frame under the playhead, or removes the one
  // there, the keyframe button's key, the way T is the thumbnail's.
  function cropMarkKey(event: KeyboardEvent) {
    if (event.key !== "c" && event.key !== "C") return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
    if (event.defaultPrevented || event.repeat) return;
    if (keysElsewhere() || asleep(root)) return;
    if (!current) return;
    event.preventDefault();
    toggleCropMark();
  }

  function thumbnailKey(event: KeyboardEvent) {
    if (event.key !== "t" && event.key !== "T") return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
    if (event.defaultPrevented || event.repeat) return;
    if (keysElsewhere() || asleep(root)) return;
    if (!current) return;
    event.preventDefault();
    toggleThumbnail();
  }

  // Corrections are made one word after another, and each one is a call
  // and a reading of every clip after it. Two of them are in the air the
  // moment a second word is clicked before the first has landed, which is
  // all it takes: clicking a word commits the one before it. Nothing says
  // they come back in the order they went out, and the loser paints the
  // clip list with what it read before the newer correction was written,
  // which is the older word back on screen.
  const words = new Newest();

  async function setWord(clip: ClipEntry, start: number, text: string) {
    const ticket = words.send();
    try {
      const updated = await api.setWord(path, clip.plan, clip.id, start, text);
      // A correction is the clip's own, so no other clip changed.
      if (!words.keep(ticket)) return;
      putClip(updated);
    } catch (err) {
      words.keep(ticket);
      oops(errorText(err));
      throw err;
    }
  }

  // A word removed, or a caption removed whole, is removed from this clip
  // alone, so only this clip changes. It takes the same ticket as a word
  // corrected, which it comes after in the caption box.
  async function removeWords(clip: ClipEntry, remove: () => Promise<ClipEntry>) {
    const ticket = words.send();
    try {
      const updated = await remove();
      if (!words.keep(ticket)) return;
      putClip(updated);
    } catch (err) {
      words.keep(ticket);
      oops(errorText(err));
      throw err;
    }
  }
  const removeWord = (clip: ClipEntry, start: number, part: number | null) =>
    removeWords(clip, () => api.removeWord(path, clip.plan, clip.id, start, part ?? -1));
  const removeCaption = (clip: ClipEntry, first: number) =>
    removeWords(clip, () => api.removeCaption(path, clip.plan, clip.id, first));

  async function setCrop(clip: ClipEntry, at: number, left: number) {
    try {
      const updated = await api.setCrop(path, clip.plan, clip.id, at, left);
      putClip(updated);
    } catch (err) {
      oops(errorText(err));
    }
  }

  // The face and the size of the captions belong to the whole clip set, so
  // this is the one place they are set, right next to the clip.
  async function setCaptionStyle(font: string, size: number) {
    if (!current) return;
    try {
      await api.setCaptionStyle(path, current.plan, font, size);
      await refreshClips();
    } catch (err) {
      oops(errorText(err));
    }
  }

  // The captions of the selected clip, as the render will draw them. Every
  // edit replaces the clip, so this follows along by itself.
  let captions = $state<CaptionsView | null>(null);
  // The clip the captions above are of. Until a clip just chosen has its
  // own, the video preview draws none rather than the last clip's at the
  // new clip's moments.
  let captionsOf = $state<string | null>(null);

  // The clips on the cards, the likeliest to be clicked first: the card
  // under the pointer, then the cards nearest the one chosen. Their
  // captions are asked for one at a time once the workspace is on screen,
  // and the video preview reads the frame each opens on and lays out its
  // captions over it, so a card clicked puts up its clip at once, see
  // Player.ahead. A clip a search finds or Mark In makes joins them as it
  // comes. On a video of hours a card clicked for the first time took half
  // a second, issue #383, and Tim asked for every clip to be ready.
  // Their captions, by the clip as it is: every edit replaces the clip, so
  // what is kept for one is what asking again would give, see captions.
  const viewsAhead = new WeakMap<ClipEntry, CaptionsView>();
  const viewsAsked = new WeakSet<ClipEntry>();
  let viewAsking = false;
  // Told when a captions call ends, see rested.
  let viewsTold: (() => void)[] = [];
  // The words either side of the playhead the clip timeline asked to be
  // read ahead last, which go first.
  let wordsAhead: number[] = [];
  function readAhead(times = wordsAhead) {
    wordsAhead = times;
    untrack(() => {
      // Asleep, the video preview has no frames to read them into, and a
      // search running on the video in the background would ask the
      // captions of every clip it finds. Woken, it reads them again.
      if (!player || !ready || away) return;
      const list = shown.filter((c) => !c.rejected);
      const here = Math.max(0, list.findIndex((c) => c.key === current?.key));
      const cards = list
        .map((c, i) => ({ c, far: c.key === hovered ? -1 : Math.abs(i - here) + (i < here ? 0.5 : 0) }))
        .filter(({ c }) => c.key !== current?.key)
        .sort((a, b) => a.far - b.far)
        .map(({ c }) => c);
      // One captions call at a time, the likeliest card first.
      const next = viewAsking ? undefined : cards.find((c) => !viewsAsked.has(c));
      if (next) {
        viewAsking = true;
        viewsAsked.add(next);
        api
          .captions(path, next.plan, next.id)
          .then((view) => {
            if (view) viewsAhead.set(next, view);
          })
          .then(
            () => {
              viewAsking = false;
              readAhead();
            },
            // Asked again when the cards next change, not at once, so a
            // Go side that keeps failing is not asked over and over.
            () => {
              viewAsking = false;
              viewsAsked.delete(next);
            },
          )
          .finally(() => {
            const told = viewsTold;
            viewsTold = [];
            for (const done of told) done();
          });
      }
      player.ahead(times, cards.map((c) => ({ clip: c, captions: viewsAhead.get(c) ?? null })));
    });
  }
  // Settles once the captions of every card were asked for and what the
  // video preview reads ahead is in, for the walks, which wait on it
  // rather than on the clock.
  async function rested(): Promise<void> {
    for (;;) {
      await new Promise<void>((done) => {
        if (!viewAsking) done();
        else viewsTold.push(done);
      });
      await player?.rested();
      if (!viewAsking) return;
    }
  }
  $effect(() => {
    if (away) return;
    (window as unknown as { __readAheadRested?: () => Promise<void> }).__readAheadRested = rested;
  });

  // Read again as the cards change, and as the pointer comes to one: the
  // card under it is the likeliest to be clicked. And as the workspace
  // wakes: its video preview opens with frames of its own, which have
  // none of the cards', found by the review of #385.
  $effect(() => {
    void [hovered, shown, current, ready, away];
    readAhead();
  });

  // Whether the workspace has the episode on it: the window on the range
  // picker, a picture on the canvas, the clip timeline drawn and the
  // captions of the clip it opened on. Opening another episode keeps the
  // workspace before it on screen until then, and the two change places in
  // one frame, see App.svelte. It was built in front of the person, empty
  // first and then a part at a time as each answer came in, and Tim saw
  // the workspace flicker every time he picked an episode.
  let settled = $state(false);
  let pictured = $state(false);
  let drawn = $state(false);
  const ready = $derived(
    settled && (!source || (pictured && drawn && (!current || !!captions))),
  );
  let toldReady = false;
  $effect(() => {
    if (!ready || toldReady) return;
    toldReady = true;
    tell("ready, its video is on it");
    untrack(() => onready?.());
  });
  // A workspace slow to be ready says what it still waits for.
  onMount(() => {
    const timers = [3, 10].map((after) =>
      setTimeout(() => {
        if (toldReady) return;
        tell(
          `not ready ${after} s after it was made: ${[
            settled ? "" : "the window is not read yet",
            !source ? "" : pictured ? "" : "no picture on the video preview",
            !source || drawn ? "" : "the clip timeline is not drawn",
            !source || !current || captions ? "" : "the clip's captions are not read",
          ]
            .filter(Boolean)
            .join(", ")}${away ? ", asleep" : ""}${toasts.text(where()) ? `, problem: ${toasts.text(where())}` : ""}`,
        );
      }, after * 1000),
    );
    return () => timers.forEach(clearTimeout);
  });
  // What went wrong here is a toast about this video, issue #327. It
  // lies over the workspace and moves nothing, where it was once a line
  // over the workspace that pushed it down. Each time is a toast of its
  // own, and it is told to the log with what the workspace was doing.
  const where = () => `video:${path}`;
  function oops(text: string) {
    tell(`problem: ${text}`);
    toasts.say(where(), text, { video: path });
  }
  onMount(() => () => toasts.clear(where()));
  // A workspace kept while another episode is in front is asleep, see
  // App.svelte: its video preview lets go of everything it holds, so an
  // episode left while it plays goes quiet the moment another is picked,
  // and it reads nothing from the Go side by itself. Waking, it reads the
  // episode again in the background, behind what it already shows, for
  // whatever changed while it slept, a setting among them.
  let slept = false;
  $effect(() => {
    tell(away ? "asleep" : "awake");
  });
  $effect(() => {
    if (away) {
      slept = true;
      return;
    }
    if (!slept) return;
    slept = false;
    untrack(() => {
      void load();
      readSettings();
    });
  });

  // The captions as the engine has them while a caption edge is dragged on
  // the clip timeline. The caption box in the video preview follows it on
  // the way, so what is seen while dragging is what will be saved.
  let captionShape = $state<{ cues: NonNullable<CaptionsView["captions"]>; track?: string } | null>(null);
  // A colour being picked is drawn in the video preview while it is picked,
  // and saved when the hand lets go of it.
  let colourDraft = $state<{
    primary?: string;
    box?: string;
    highlight?: string;
    highlightOn?: boolean;
    textOn?: boolean;
    boxOn?: boolean;
  } | null>(null);
  // The captions the draft was made against. The draft is let go of when
  // they come back changed, so the caption box never flashes back to the
  // colour it had while the saved colour is on its way.
  let colourHeld: CaptionsView | null = null;
  // Watched, not only read: the draft is let go of once the saving is over
  // and the captions have come back. Read without being watched, the draft
  // outlived the save, and the next colour drawn was let go of instead, the
  // moment it was drawn, because the captions it was held against were the
  // ones from before the save. The colour field drew again with every
  // movement and hid it. A pick from the video preview draws once per
  // colour, and showed nothing.
  let colourSaving = $state(false);
  $effect(() => {
    if (colourDraft && !colourSaving && captions !== colourHeld) colourDraft = null;
  });
  // The clip as a drag on the clip timeline is shaping it: its pieces and
  // the captions the engine made for them. The video preview shows that
  // clip while the hand moves, the same one the timeline shows.
  let reshaped = $state<{
    cues: NonNullable<CaptionsView["captions"]>;
    pieces: { start: number; end: number }[];
    track?: string;
  } | null>(null);
  // Each piece keeps the framing of the piece it came from, the way the
  // engine keeps it when the edit lands.
  const shownClip = $derived.by(() => {
    if (!current || !reshaped) return current;
    const old = current.segments;
    const overlap = (a: { start: number; end: number }, b: { start: number; end: number }) =>
      Math.min(a.end, b.end) - Math.max(a.start, b.start);
    const from = reshaped.pieces.map((p) => {
      let best = 0;
      old.forEach((o, i) => {
        if (overlap(p, o) > overlap(p, old[best])) best = i;
      });
      return best;
    });
    const lefts = current.cropLefts;
    return {
      ...current,
      segments: reshaped.pieces.map((p, i) => ({ ...old[from[i]], start: p.start, end: p.end })),
      cropLefts: from.map((i) => lefts[i]),
    };
  });
  const shownCaptions = $derived.by(() => {
    if (!captions) return captions;
    let view = captions;
    if (reshaped) view = { ...view, captions: reshaped.cues, track: reshaped.track, pieces: reshaped.pieces };
    if (captionShape) view = { ...view, captions: captionShape.cues, track: captionShape.track };
    if (colourDraft) {
      view = {
        ...view,
        style: {
          ...view.style,
          primary: colourDraft.primary ?? view.style.primary,
          box: colourDraft.box ?? view.style.box,
          highlightColour: colourDraft.highlight ?? view.style.highlightColour,
          highlight: colourDraft.highlightOn ?? view.style.highlight,
          text: colourDraft.textOn ?? view.style.text,
          boxOn: colourDraft.boxOn ?? view.style.boxOn,
        },
      };
    }
    return view;
  });
  // The colours and switches being drafted, as the video preview asks the
  // Go side to draw them, see lib/frames/captions.ts. Only what is being
  // changed: the rest is drawn as it is saved.
  const captionDrafts = $derived.by(() => {
    const d = colourDraft;
    const out: Record<string, string> = {};
    if (!d) return out;
    for (const [name, css] of [
      ["text", d.primary],
      ["box", d.box],
      ["highlight", d.highlight],
    ] as const) {
      if (css === undefined) continue;
      const c = splitColour(css);
      out[name] = c.hex;
      out[`${name}Opacity`] = String(c.alpha);
    }
    for (const [name, on] of [
      ["textOn", d.textOn],
      ["boxOn", d.boxOn],
      ["highlightOn", d.highlightOn],
    ] as const) {
      if (on !== undefined) out[name] = on ? "1" : "0";
    }
    return out;
  });
  // The colours as the controls show them.
  const textSplit = $derived(splitColour(shownCaptions?.style.primary ?? ""));
  const textColour = $derived(textSplit.hex);
  const textOpacity = $derived(Math.round(textSplit.alpha * 100));
  const boxColour = $derived(splitColour(shownCaptions?.style.box ?? "rgba(0, 0, 0, 0.5)"));
  const boxOpacity = $derived(Math.round(boxColour.alpha * 100));

  const highlightSplit = $derived(splitColour(shownCaptions?.style.highlightColour ?? ""));
  const highlightColour = $derived(highlightSplit.hex);
  const highlightOpacity = $derived(Math.round(highlightSplit.alpha * 100));

  // Whether the word being spoken sits on its pill and bounces. Off, the
  // captions are the box and the words.
  const highlightOn = $derived(shownCaptions?.style.highlight ?? true);
  // Whether captions are burned in at all, and whether the box is drawn
  // behind them. Off, nothing of the captions renders, and a short can be
  // made without any.
  const textOn = $derived(shownCaptions?.style.text ?? true);
  const boxOn = $derived(shownCaptions?.style.boxOn ?? true);
  // What the column shows for the box and the highlight. With the text off
  // nothing of the captions is drawn, so they read as off too, the way the
  // video preview shows them. Each keeps its own setting underneath, and
  // turning the text on again brings them back as they were.
  const boxShown = $derived(textOn && boxOn);
  const highlightShown = $derived(textOn && highlightOn);

  // A click on Box or Highlight. With the text off it says the captions are
  // wanted, with this part of them, so both come on.
  async function flipPart(which: "box" | "highlight", on: boolean) {
    if (!textOn) {
      await setCaptionSwitch("text", true);
      if (!on) await setCaptionSwitch(which, true);
      return;
    }
    await setCaptionSwitch(which, !on);
  }

  function drawColour(part: {
    primary?: string;
    box?: string;
    highlight?: string;
    highlightOn?: boolean;
    textOn?: boolean;
    boxOn?: boolean;
  }) {
    if (!colourDraft) colourHeld = captions;
    colourDraft = { ...colourDraft, ...part };
  }

  // A colour that was only being looked at, a pick from the video preview
  // that was left, goes back to what is saved.
  function dropColourDraft() {
    if (!colourSaving) colourDraft = null;
  }

  // A colour picker asking the video preview for a colour.
  function sampleColour(over: (hex: string | null) => void, done: (hex: string | null) => void) {
    if (!player) return done(null);
    player.sampleColour(over, done);
  }

  // A colour and how much of it is seen, for the text and for the box, the
  // share as a percentage. An empty colour is left as it is.
  async function setCaptionColours(
    text: string,
    textShare: number,
    box: string,
    boxShare: number,
    highlight = "",
    highlightShare = 100,
  ) {
    if (!current) return;
    captionsWere = null;
    colourSaving = true;
    try {
      await api.setCaptionColours(
        path,
        current.plan,
        text,
        textShare / 100,
        box,
        boxShare / 100,
        highlight,
        highlightShare / 100,
      );
      await refreshClips();
    } catch (err) {
      oops(errorText(err));
      colourDraft = null;
    } finally {
      colourSaving = false;
    }
  }

  // A switch of the captions column on or off, shown at once and saved
  // straight after: the text, the box, or the highlight.
  async function setCaptionSwitch(which: CaptionSwitch, on: boolean) {
    if (!current) return;
    captionsWere = null;
    drawColour(which === "text" ? { textOn: on } : which === "box" ? { boxOn: on } : { highlightOn: on });
    colourSaving = true;
    try {
      await api.setCaptionSwitch(path, current.plan, which, on);
      await refreshClips();
    } catch (err) {
      oops(errorText(err));
      colourDraft = null;
    } finally {
      colourSaving = false;
    }
  }

  // The text's colour or how much of it is seen, and the same for the box.
  // Choosing either while it is off says it is wanted, so it comes back on
  // with it, the way the highlight does below.
  async function setTextColour(colour: string, share: number) {
    if (captions?.style.text === false) await setCaptionSwitch("text", true);
    await setCaptionColours(colour, share, "", boxOpacity);
  }
  async function setBoxColour(colour: string, share: number) {
    if (captions?.style.text === false) await setCaptionSwitch("text", true);
    if (captions?.style.boxOn === false) await setCaptionSwitch("box", true);
    await setCaptionColours("", textOpacity, colour, share);
  }

  // The pill's colour or how much of it is seen. Choosing either while the
  // highlight is off says the pill is wanted, so it comes back on with it.
  async function setHighlightColour(colour: string, share: number) {
    if (captions?.style.text === false) await setCaptionSwitch("text", true);
    if (captions?.style.highlight === false) await setCaptionSwitch("highlight", true);
    await setCaptionColours("", textOpacity, "", boxOpacity, colour, share);
  }

  // When a caption appears or goes, moved where the words are a little off
  // from what is heard. It answers whether it was saved, so the timeline
  // knows to keep the edge where it was let go until the captions come back.
  async function setCaptionTime(
    clip: ClipEntry,
    word: number,
    edge: "start" | "end",
    at: number,
    toWords: boolean,
  ): Promise<boolean> {
    try {
      const updated = await api.setCaptionTime(path, clip.plan, clip.id, word, edge, at, toWords);
      putClip(updated);
      return true;
    } catch (err) {
      oops(errorText(err));
      return false;
    }
  }

  // The words the caption lights up, put back on the episode's clock.
  //
  // This is the list shift and an arrow key walk, and it is not the list
  // of words the transcript holds. A correction that reads as two words
  // is two words in the caption and one in the transcript, so a word
  // added by hand stood in no list the timeline had and the keys stepped
  // straight past it. A word a cut takes out is the other way round: in
  // the transcript, never in the caption, and landing on it lit nothing.
  //
  // Reading the caption is what makes both right at once, and it will go
  // on being right, because it is the same list either way: whatever
  // lights up is what these keys walk.
  //
  // And each word is where the picture lights it, by the frames the engine
  // reads it lit on, see WordFrame. A caption's times are on the clip's
  // clock, and a piece of the short starts on the frame nearest its
  // start, so a word walked to by its own time was now and then not lit
  // yet on the frame the walk landed on, while the keyboard's frame was
  // already round it. A word lit on no frame is not walked to. By the
  // words' own times only until the frames are known.
  const lit = $derived.by(() => {
    const pieces = current?.segments ?? [];
    if (!pieces.length || !captions?.captions?.length) return [];
    const frames = framedTrack ? (wordFrames?.frames ?? null) : null;
    const out: Word[] = [];
    for (const [i, cue] of captions.captions.entries()) {
      let k = 0;
      for (const line of cue.lines) {
        for (const word of line.words) {
          const f = frames?.[i]?.[k++];
          if (f && f.from < 0) continue;
          out.push({
            start: f ? f.from : inEpisode(pieces, word.start),
            end: f ? f.to : endInEpisode(pieces, word.end),
            text: word.text,
            framed: !!f,
            cue: i,
          });
        }
      }
    }
    return out;
  });
  // Where libass puts each word of the captions shown and when the picture
  // lights it, see WordFrame, asked for once for each track: what the hand
  // drafts moves no word but for the height, and the height is the caption
  // box's own. A drag of an edge makes new captions on every step, so an
  // ask the captions moved on from is called off, and the Go side stops
  // measuring for it. The video preview places the words of the caption
  // box by them, and lit walks by them.
  //
  // Nothing is asked while the hand still shapes the clip or a caption on
  // the clip timeline: nobody walks or types into captions that change on
  // every step, and the captions the hand leaves are asked for once it
  // has let go and they are saved. Asked on every step, they held a
  // connection to the Go side beside the video preview's three layers,
  // and on a busy machine the answers to the drag queued behind them, so
  // an edge taken again while the drag before it saved stood still.
  let wordFrames = $state<{ key: string; frames: WordFrame[][] | null } | null>(null);
  $effect(() => {
    if (reshaped || captionShape) return;
    const track = shownCaptions?.track;
    const key = track ? `${path}\n${track}` : "";
    if (!key || untrack(() => wordFrames?.key) === key) return;
    const off = new AbortController();
    void wordFramesOf(path, track!, off.signal, (text, video) => said(text, { level: "warn", video }))
      .then((frames) => {
        if (!off.signal.aborted) wordFrames = { key, frames };
      })
      // An ask that broke off is an answer with no frames too, so the words
      // are walked by their own times and a clip walked to lands on its
      // word, rather than waiting on an answer that is not coming.
      .catch((e) => {
        if (off.signal.aborted) return;
        said(`captions: where the words stand did not come, ${errorText(e)}`, { level: "warn", video: path });
        wordFrames = { key, frames: null };
      });
    return () => off.abort();
  });
  // The track of the captions on hand once the engine has answered where
  // the picture lights their words, whether it said or not, and the same
  // once it said, and until then nothing.
  const answeredTrack = $derived(
    captions?.track && wordFrames?.key === `${path}\n${captions.track}` ? captions.track : undefined,
  );
  const framedTrack = $derived(answeredTrack && wordFrames?.frames ? answeredTrack : undefined);
  let fonts = $state<CaptionFont[]>([]);
  // The fonts as Pages offers them, a list of families and a list of the
  // styles of the one picked, and the face the captions are in now.
  const fontFamilies = $derived(families(fonts));
  const captionFace = $derived(fonts.find((f) => f.name === captions?.style.font));
  const captionFamily = $derived(
    fontFamilies.find((f) => f.name === (captionFace?.family || captionFace?.name)),
  );
  const captionStyles = $derived(captionFamily ? styleNames(captionFamily) : new Map<string, string>());

  // Where the captions sit, for every clip of every episode. Dragging the
  // box in the video preview is the same thing as typing the number in the
  // settings column, and both stay for the next video.
  let captionY = $state(captionYDefault);
  // What the captions looked like before they were put back, so the way
  // back is a way there and back again. Putting them back is one click,
  // and one click has to be undoable as easily. It is dropped the moment
  // any of the three is set to anything else, because then there is
  // nothing to return to.
  let captionsWere = $state<{
    font: string;
    size: number;
    y: number;
    text: string;
    textOpacity: number;
    box: string;
    opacity: number;
    highlight: string;
    highlightOpacity: number;
    highlightOn: boolean;
    textOn: boolean;
    boxOn: boolean;
  } | null>(null);
  // The captions as they are now, and whether that is how they start out.
  // The mark beside the head is about the group, not about one row of it.
  const captionsNow = $derived({
    // The face the video asks for, even one this machine lacks, so putting
    // the captions back as they start out is offered for it too.
    font: captions?.style.missing || (captions?.style.font ?? captionFontDefault),
    size: Math.round(captions?.style.chosenSize ?? captionSizeDefault),
    y: Math.round(captionY),
    text: splitColour(captions?.style.primary ?? "").hex,
    textOpacity: Math.round(splitColour(captions?.style.primary ?? "").alpha * 100),
    highlight: splitColour(captions?.style.highlightColour ?? "").hex,
    highlightOpacity: Math.round(splitColour(captions?.style.highlightColour ?? "").alpha * 100),
    highlightOn: captions?.style.highlight ?? true,
    textOn: captions?.style.text ?? true,
    boxOn: captions?.style.boxOn ?? true,
    box: splitColour(captions?.style.box ?? "").hex,
    opacity: Math.round(splitColour(captions?.style.box ?? "rgba(0, 0, 0, 0.5)").alpha * 100),
  });
  const coloursMoved = $derived(
    captionsNow.text !== captionTextDefault ||
      captionsNow.textOpacity !== captionTextOpacityDefault ||
      captionsNow.highlight !== captionHighlightDefault ||
      captionsNow.highlightOpacity !== captionHighlightOpacityDefault ||
      captionsNow.box !== captionBoxDefault ||
      captionsNow.opacity !== captionOpacityDefault,
  );
  const captionsMoved = $derived(
    captionsNow.font !== captionFontDefault ||
      captionsNow.size !== captionSizeDefault ||
      captionsNow.y !== captionYDefault ||
      !captionsNow.highlightOn ||
      !captionsNow.textOn ||
      !captionsNow.boxOn ||
      coloursMoved,
  );

  // Everything about the captions back to how it starts out: the face, the
  // size and the height together, because the mark is beside the head of
  // the whole group and not beside one row of it.
  async function putCaptionsBack() {
    const was = captionsNow;
    if (captionsNow.font !== captionFontDefault || captionsNow.size !== captionSizeDefault) {
      await setCaptionStyle(captionFontDefault, captionSizeDefault);
    }
    if (captionsNow.y !== captionYDefault) await resetCaptionsHeight();
    if (!captionsNow.highlightOn) await setCaptionSwitch("highlight", true);
    if (!captionsNow.textOn) await setCaptionSwitch("text", true);
    if (!captionsNow.boxOn) await setCaptionSwitch("box", true);
    if (coloursMoved) {
      await setCaptionColours(
        captionTextDefault,
        captionTextOpacityDefault,
        captionBoxDefault,
        captionOpacityDefault,
        captionHighlightDefault,
        captionHighlightOpacityDefault,
      );
    }
    captionsWere = was;
  }

  // And back to what they were, which is the same one click the other way.
  async function putCaptionsAsTheyWere() {
    const was = captionsWere;
    if (!was) return;
    captionsWere = null;
    if (was.font !== captionsNow.font || was.size !== captionsNow.size) {
      await setCaptionStyle(was.font, was.size);
    }
    if (was.y !== captionsNow.y) await setCaptionsHeight(was.y);
    if (was.highlightOn !== captionsNow.highlightOn) await setCaptionSwitch("highlight", was.highlightOn);
    if (was.textOn !== captionsNow.textOn) await setCaptionSwitch("text", was.textOn);
    if (was.boxOn !== captionsNow.boxOn) await setCaptionSwitch("box", was.boxOn);
    if (
      was.text !== captionsNow.text ||
      was.textOpacity !== captionsNow.textOpacity ||
      was.box !== captionsNow.box ||
      was.opacity !== captionsNow.opacity ||
      was.highlight !== captionsNow.highlight ||
      was.highlightOpacity !== captionsNow.highlightOpacity
    ) {
      await setCaptionColours(
        was.text,
        was.textOpacity,
        was.box,
        was.opacity,
        was.highlight,
        was.highlightOpacity,
      );
    }
  }

  async function setCaptionsHeight(y: number) {
    const was = captionY;
    captionsWere = null;
    captionY = snapCaptionY(y);
    try {
      await api.setCaptionsHeight(path, captionY);
      // The clips come again, so the captions of the one on screen are read
      // again with them.
      await refreshClips();
    } catch (err) {
      captionY = was;
      oops(errorText(err));
    }
  }

  async function resetCaptionsHeight() {
    const was = captionY;
    captionY = captionYDefault;
    try {
      await api.resetCaptionsHeight(path);
      await refreshClips();
    } catch (err) {
      captionY = was;
      oops(errorText(err));
    }
  }

  // R and the Rotate button: the part of the clip between two cuts that
  // the playhead stands in turns a quarter to the left, the way Preview's
  // Rotate Left does, for a phone that was turned while it recorded. It
  // turns in the video preview in the frame it is pressed in, by the
  // engine's own rule in lib/rotate.ts, and the clip the engine answers
  // with takes its place. Plan row 3.25.
  async function rotatePart() {
    if (!current || renderingCurrent || !inShort || !source) return;
    const clip = current;
    const at = time;
    clips = clips.map((c) => (c.key === clip.key ? rotated(c, source!, at) : c));
    try {
      const updated = await api.rotatePart(path, clip.plan, clip.id, at);
      putClip(updated);
    } catch (err) {
      oops(errorText(err));
      clips = clips.map((c) => (c.key === clip.key ? clip : c));
    }
  }

  async function resetCrop(clip: ClipEntry, at: number) {
    try {
      const updated = await api.resetCrop(path, clip.plan, clip.id, at);
      putClip(updated);
    } catch (err) {
      oops(errorText(err));
    }
  }

  // The shortest cannot be longer than the longest, so whichever was just
  // typed pushes the other along rather than leaving a search that can find
  // nothing.
  function keepOrder(typed: "min" | "max") {
    if (typed === "min" && min > shortestAtMost) min = shortestAtMost;
    if ((min > 0) && (max > 0) && min > max) {
      if (typed === "min") max = min;
      else min = max;
    }
    saveSearch();
  }

  // The numbers a search is made with live in the workspace, next to the
  // episode they are about, and there is nowhere else to set them. So they
  // are kept the moment they change, like everything else in the
  // workspace, and the next episode opens with them.
  function saveSearch() {
    api.setSearch(target, targetWindow, min, max).catch((err) => oops(errorText(err)));
    keepWindow();
  }

  // A number typed past what the window can hold is taken back to the
  // most it can, the way a field's own arrows stop there. A field cleared
  // follows the window again, and shows its suggestion.
  function keepTarget(e: Event) {
    const entry = (e.currentTarget as HTMLInputElement).value.trim();
    target = entry === "" ? 0 : Math.min(Math.max(Math.round(Number(entry)) || 1, 1), clipsAtMost);
    targetWindow = target > 0 ? to - from : 0;
    (e.currentTarget as HTMLInputElement).value = String(target > 0 ? target : suggested);
    saveSearch();
  }

  function newClips() {
    findClips(false);
  }

  // A click shows at once: the list opens its rows and the head says
  // Cancel before the job has reported in. It gives the press its name.
  function begin(): string {
    jobs.press(path, "clips");
    click = `${Date.now()}-${++clicks}`;
    listedBefore = new Set(clips.map((c) => c.key));
    pickedBefore = selected;
    shownFirst = false;
    chosenByFind = "";
    return click;
  }

  // Search. The search hears the window first if the episode has not been
  // heard that far, all of it on the Go side, see docs/JOBS.md.
  async function findClips(replan: boolean) {
    const press = begin();
    await asked(() =>
      api.search(
        path,
        {
          From: whole ? 0 : from,
          To: whole ? 0 : to,
          Count: count,
          Min: min,
          Max: max,
          Replan: replan,
        },
        press,
      ),
    );
  }

  // Search and Continue alike. The Go side answers with the search, and one
  // it would not start, while the app closes or the episode is being
  // removed, comes back failed already. It never runs, so nothing would
  // ever end the start the click showed: the head stood on Cancel, greyed
  // out, over Finding clips. Its row says why instead, with Continue, the
  // way any search that failed does.
  async function asked(ask: () => Promise<Job>) {
    try {
      const job = await ask();
      jobs.apply(job);
    } catch (err) {
      oops(errorText(err));
    } finally {
      jobs.answered(path, "clips");
    }
  }

  // A search says how many clips it has written each time one lands, and
  // the list is read again then, rather than on the next tick of a timer.
  // The effect runs on every job event, so it only acts on a change.
  let foundHeard = 0;
  $effect(() => {
    const found = working?.written ?? 0;
    if (found === foundHeard) return;
    foundHeard = found;
    if (found > 0) refreshClips(true).then(() => showFirstFound());
  });

  // Undo and Redo, from the Edit menu and its keys. A field being typed in,
  // a word in the caption box or a number beside the clip, has an undo of
  // its own for the text in it, and keeps it: the key goes to the field
  // while it is being typed in and to the episode otherwise. Taking
  // something back shows what came back, so the clip it changed is chosen
  // and nothing changes where nobody is looking.
  let undoing = false;
  async function undo(what: "undo" | "redo") {
    // The menu tells every workspace standing in the app. Only the one in
    // front is the one the person means.
    if (asleep(root)) return;
    if (typing()) {
      document.execCommand(what);
      return;
    }
    // Nothing changes behind the box asking something.
    if (asking()) return;
    // One at a time. A key held down would otherwise ask for the same step
    // twice before the first answer is back.
    if (undoing) return;
    undoing = true;
    try {
      const done = what === "undo" ? await api.undo(path) : await api.redo(path);
      if (!done?.done) return;
      // A step that moved the window puts it where it was. It is kept on
      // the Go side already.
      if (done.window && !busy) {
        from = done.window.from;
        to = Math.min(done.window.to, duration);
        length = Math.max(done.window.length, to - from);
      }
      await load();
      // The caption height is kept in the settings, so an undo of a drag
      // is read back from there.
      const settings = await api.getSettings();
      captionY = settings.captionY || captionYDefault;
      onchange();
      const clip = done.clip ? clips.find((c) => c.key === done.clip) : undefined;
      if (clip && clip.key !== selected) await select(clip.key);
      // A dragged edge took the playhead along, so taking it back takes
      // the playhead back too. After the clip is chosen, because choosing
      // one puts the playhead on it. Not while the episode plays: only the
      // space bar and a click on the clip timeline change a play, see
      // edgesWhilePlaying in ClipTimeline.svelte.
      if (done.playhead !== undefined && paused) player?.seek(done.playhead);
    } catch (err) {
      oops(errorText(err));
    } finally {
      undoing = false;
    }
  }
  onMount(() => onUndo(undo));

  // The first clip a search finds, shown as soon as it is in the list.
  // Not while the video plays: choosing a clip puts the playhead on it, and
  // a picture that jumps from where it was playing to a clip nobody asked
  // for is the search taking the video away from the hand. The clips land
  // in the list and on both tracks either way, and are one click off.
  //
  // When the search is over, the earliest of what it found is the one
  // chosen, the first of its clips in the list, as long as the clip chosen
  // is still the one the search chose. The model names its clips strongest
  // first, not in the order of the episode, so the first to land was often
  // the last in the list, and it stayed chosen with the playhead on it.
  function showFirstFound(ended = false) {
    if (!listedBefore || (shownFirst && !ended)) return;
    const known = listedBefore;
    const found = clips.filter((c) => !known.has(c.key));
    if (!found.length) return;
    shownFirst = true;
    if ((selected !== pickedBefore && selected !== chosenByFind) || !paused) return;
    // The list is in the order of the episode, and so is what is new in
    // it, so this is the earliest of what has landed.
    if (found[0].key === selected) return;
    chosenByFind = found[0].key;
    select(found[0].key);
  }

  // The job has reported in, so the placeholder can give way to it.
  $effect(() => {
    if (working) untrack(() => jobs.answered(path, "clips"));
  });

  // The window on the range picker is the window being searched, for as
  // long as a search runs. A search the Go side started by itself, the
  // first one of an episode just added, has a window the workspace did not
  // draw, so the range picker goes to it. And its rows are counted from
  // the list as it was when it started, the same as for one started here.
  let followed = "";
  $effect(() => {
    if (!working || !source || followed === working.id) return;
    followed = working.id;
    const start = working.from ?? 0;
    const end = working.to && working.to > 0 ? working.to : duration;
    if (end > start) {
      from = start;
      to = end;
    }
    if (!listedBefore) {
      listedBefore = new Set(clips.map((c) => c.key));
      pickedBefore = selected;
      shownFirst = false;
      chosenByFind = "";
    }
  });

  // The clip a render was asked for, from the click until the job is
  // there, so the button fills from the moment it is pressed.
  let renderAsked = $state("");
  $effect(() => {
    if (renderingJob) renderAsked = "";
  });

  async function render(clip: ClipEntry) {
    renderAsked = clip.key;
    try {
      const job = await api.render(path, { Plan: clip.plan, Clips: [clip.id], Preview: false });
      waiting = [...waiting, job.id];
    } catch (err) {
      renderAsked = "";
      oops(errorText(err));
    }
  }

  // The captions of the clip on its way that is chosen, laid out as they
  // will be once it is written, from the moment its pieces are known. Asked
  // for once for every change of its pieces, not on every job event.
  const makingCaptions = $derived.by(() => {
    const a = making ? onTheWay.find((x) => x.key === making.key) : undefined;
    return a?.pieces?.length ? `${a.job}\n${a.n}\n${JSON.stringify(a.pieces)}` : "";
  });
  $effect(() => {
    const clip = current;
    if (!clip) {
      const [job, n] = makingCaptions.split("\n");
      if (!makingCaptions) {
        captions = null;
        return;
      }
      let dropped = false;
      api
        .arrivingCaptions(job, Number(n))
        .then((view) => {
          if (!dropped && view) captions = view;
        })
        .catch(() => {});
      return () => (dropped = true);
    }
    let dropped = false;
    // Read while the hand rested, see readAhead: drawn at once, and asked
    // for again below in case anything changed that does not replace the
    // clip.
    const early = viewsAhead.get(clip);
    if (early) {
      captions = early;
      captionsOf = clip.key;
    }
    api
      .captions(path, clip.plan, clip.id)
      .then((view) => {
        if (dropped || (early && view?.track === early.track)) return;
        captions = view;
        captionsOf = clip.key;
      })
      .catch(() => {
        if (dropped || early) return;
        // None to draw, and none on the way, so nothing waits for them.
        captions = null;
        captionsOf = clip.key;
      });
    return () => (dropped = true);
  });

  // The model answers a search with the whole set at once, and the clips
  // land in the plan file there and then. The list is taken again while a
  // search runs, so they show up the moment they exist instead of when the
  // job has wrapped up.


  // While the transcript grows, the covered part of the timeline grows.


  // And when the whole video's work ends, which no search's hearing says:
  // the workspace kept what it read last, so stopped on its last pieces
  // the video was heard to its end and the dots stayed, found by the
  // second review of #458.
  const wholeOn = $derived(!!wholeWork);
  let wasWhole = false;
  $effect(() => {
    const now = wholeOn;
    if (wasWhole && !now) untrack(() => load());
    wasWhole = now;
  });

  // How far the episode has been heard is read again when the hearing
  // stops, so the range picker's edge lands on what was saved.
  let wasTranscribing = false;
  $effect(() => {
    const now = !!transcribing;
    if (wasTranscribing && !now) {
      onchange();
      load();
    }
    wasTranscribing = now;
  });

  // A search that ends: the list shows what it found, and the window moves
  // on. How it ended otherwise is said by the search itself, in the row its
  // next clip would have appeared in. Watched by whether one runs, not by
  // the job, which is replaced on every report.
  let searchRan = "";
  $effect(() => {
    const id = working?.id ?? "";
    if (id) {
      searchRan = id;
      return;
    }
    if (!searchRan) return;
    const ended = jobs.list.find((j) => j.id === searchRan);
    searchRan = "";
    untrack(() => jobs.answered(path, "clips"));
    onchange();
    // The window walks on from where the search ended, as long as it was,
    // which the Go side did before the search said it was done. Asked for
    // with the rest and drawn with the clips found, so the window does not
    // move one answer after the first clip is picked.
    const moved = ended?.state === "done" ? api.chosenWindow(path) : undefined;
    moved?.catch(() => {});
    movingOn = !!moved;
    load(moved).then(() => {
      if (ended?.state !== "done") {
        listedBefore = null;
        return;
      }
      // The earliest clip it found, unless something else has been picked
      // since the search chose one, which is where the hand is now.
      showFirstFound(true);
      // A window searched again comes back under the names it had, so
      // nothing in the list is new. Its first clip, as long as nobody has
      // picked another.
      if (!shownFirst && selected === pickedBefore && paused && ended.result) {
        const first = clips.find((c) => c.plan === ended.result);
        if (first) select(first.key);
      }
      listedBefore = null;
    });
  });

  // Finished renders this screen started: the list shows what was
  // rendered, and a render that failed says why over the workspace.
  $effect(() => {
    if (!waiting.length) return;
    const finished = jobs.list.filter(
      (j) => waiting.includes(j.id) && j.state !== "queued" && j.state !== "running",
    );
    if (!finished.length) return;
    waiting = waiting.filter((id) => !finished.some((j) => j.id === id));
    onchange();
    load().then(() => {
      for (const job of finished) {
        if (job.state === "failed") oops(job.error ?? "The render failed.");
      }
    });
  });

  // Playing moves nothing. An editor's timeline follows its playhead while
  // it plays, and that is a setting there, off as often as on, because a
  // view somebody put somewhere is a view they meant. Here it was neither
  // asked for nor announced: scroll to where you want to look, press the
  // space bar, and the track jumped somewhere else before a frame had
  // played. Finding the playhead is what the crosshair under the track is
  // for, and going back to the clip is what clicking its card does. Two
  // controls that say what they do, and no third that does it uninvited.

  // Shift and the arrows up and down walk the clip list, the way shift and
  // the arrows left and right walk its words. Shift is what means a clip
  // or a caption throughout: without it the arrows move the playhead a
  // frame, with it they move it a word and a clip. Each step takes the
  // next clip and puts the playhead at its start, which is where a short
  // begins and so the one frame worth seeing first.
  function walkClips(event: KeyboardEvent) {
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    if (!event.shiftKey) return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.defaultPrevented) return;
    // A box asking something, and an edge being nudged, take the arrows
    // for themselves.
    if (keysElsewhere() || asleep(root)) return;
    if (document.activeElement?.getAttribute("role") === "slider") return;
    // While the transcript is shown in the column, its cards are walked.
    if (saying) {
      event.preventDefault();
      walkSaid(event.key === "ArrowDown" ? 1 : -1);
      return;
    }
    const list = shown.filter((c) => !(c.key in removed));
    if (!list.length) return;
    event.preventDefault();
    const here = list.findIndex((c) => c.key === selected);
    const step = event.key === "ArrowDown" ? 1 : -1;
    // With nothing chosen, down takes the first and up the last. The list
    // goes round: down from the last card is the first, and up from the
    // first is the last.
    const next = here < 0 ? (step > 0 ? 0 : list.length - 1) : here + step;
    const clip = list[(next + list.length) % list.length];
    if (!clip) return;
    select(clip.key, true, true, step);
    seekTo(clip.start, "clip");
  }

  // Walking the words runs off the end of a clip into the one beside it:
  // shift and the left arrow from the first word of a clip is the last
  // word of the one before it, and the right arrow from the last word is
  // the first word of the next. A clip's words are not there to walk on
  // to, they arrive with its captions, so the clip is chosen and where to
  // land in it is held until they do.
  //
  // It is held by the clip it is about. A clip whose captions never arrive
  // would otherwise leave this standing, and the next clip chosen by any
  // other means would be jumped about in for no reason anyone could see.
  let landOn = $state<{ key: string; last: boolean } | null>(null);

  function walkClip(back: boolean) {
    const list = shown.filter((c) => !(c.key in removed));
    const here = list.findIndex((c) => c.key === selected);
    if (here < 0) return;
    const next = list[here + (back ? -1 : 1)];
    if (!next) return;
    // The captions on hand belong to the clip being left, so they are put
    // down before the new clip is chosen. Until the new ones arrive there
    // are no words, which is what the landing below waits for.
    captions = null;
    landOn = { key: next.key, last: back };
    select(next.key);
  }

  $effect(() => {
    const want = landOn;
    if (!want || current?.key !== want.key || !lit.length) return;
    // Where the picture lights the words, once the engine has said.
    if (captions?.track && !answeredTrack) return;
    landOn = null;
    const word = want.last ? lit[lit.length - 1] : lit[0];
    const at = intoWord(word, source && source.fps > 0 ? 1 / source.fps : 1 / 30);
    seekTo(at, undefined, true);
    // The word landed on is the keyboard's word, as any word walked to is.
    untrack(() => player?.keyAt(at));
  });

  onMount(() => {
    window.addEventListener("keydown", walkClips);
    window.addEventListener("keydown", thumbnailKey);
    window.addEventListener("keydown", cropMarkKey);
    window.addEventListener("keydown", rotateKey);
    window.addEventListener("keydown", loopKey);
    window.addEventListener("keydown", inOutKey);
    window.addEventListener("keydown", sayKey);
    window.addEventListener("focusin", sayElsewhere);
    window.addEventListener("pointerdown", sayElsewhere, true);
    return () => {
      window.removeEventListener("focusin", sayElsewhere);
      window.removeEventListener("pointerdown", sayElsewhere, true);
      window.removeEventListener("keydown", walkClips);
      window.removeEventListener("keydown", sayKey);
      window.removeEventListener("keydown", thumbnailKey);
      window.removeEventListener("keydown", cropMarkKey);
      window.removeEventListener("keydown", rotateKey);
      window.removeEventListener("keydown", loopKey);
      window.removeEventListener("keydown", inOutKey);
    };
  });

  // One timer for as long as the workspace is open, and it decides what to
  // do each time it fires. It is not an effect: an effect that mentions
  // running work is torn down and set up again on every job event, about
  // once a second, so its timer would never fire at all.
  onMount(() => {
    let ticks = 0;
    const timer = setInterval(() => {
      // Asleep, nothing is read: waking reads it all once.
      if (away) return;
      ticks++;
      if (isTranscribing) {
        // While the transcript grows, how far it has come is read again:
        // the track draws it, and the first search waits for it.
        const ticket = statusRead.send();
        api
          .episode(path)
          .then((now) => {
            if (statusRead.keep(ticket)) status = now;
          })
          .catch(() => {});
        // What the new lines weigh, less often: the room only needs them
        // to say how far a window may reach, and the rest of the episode
        // is weighed at a rate that errs towards less until then.
        if (ticks % 5 === 0) refreshRoom();
      }
      if (isFinding) {
        // Each clip lands in the plan the moment it is framed, while the
        // model is still writing the next, so the list is read again as
        // the search runs and the first one is put on screen.
        refreshClips(true).then(() => showFirstFound());
      }
    }, 2000);
    return () => clearInterval(timer);
  });

  onMount(() => {
    installFonts()
      .then((list) => (fonts = list))
      .catch(() => {});
    tell(`made${away ? ", asleep" : ""}`);
    holdAhead(path);
    readSettings();
    openEpisode();
  });

  function readSettings() {
    api.getSettings().then((settings) => {
      target = settings.target || 0;
      targetWindow = settings.targetWindow || 0;
      min = settings.min || 20;
      max = settings.max || 30;
      captionY = settings.captionY || captionYDefault;
    });
  }
</script>

{#snippet strip()}
  <RangeWindow
    {duration}
    {from}
    {to}
    shown={(busy || movingOn) && !stopping}
    resumes={!!stopped}
    {marks}
    {selected}
    {hovered}
    onhover={hoverClip}
    onmark={select}
    playhead={time}
    onseek={seekTo}
    onhold={(held) => player?.hold(held)}
    dimmed={!onClip}
    locked={busy}
    {unheard}
    unheardLit={saying ? unheardLit : -1}
    unheardOn={saying ? unheardOn : -1}
    giving={givingAt}
    part={wholePart}
    partWaiting={!wholeHearing}
    partKey={wholeWork?.id ?? ""}
    onhoverunheard={saying ? (i) => (unheardLit = i) : undefined}
    onmove={moveWindow}
    onreset={() => void placeWindowBack()}
    least={leastLong}
    leastSays={typed > 0 ? `room for ${typed} clips of ${min} s` : `room for a clip of ${min} s`}
    most={reachAnywhere}
    reachSays={roomView?.by === "budget"
      ? "all the budget pays for"
      : roomView?.by === "memory"
        ? "all this computer's memory holds"
        : "all the model reads at once"}
  />
{/snippet}



<!-- The one number the stylesheet needs and cannot know: the shape of
     this episode. It stays put while the app is resized, so the whole
     layout below is the browser's own work from there on. -->
<section bind:this={root} style="--ar: {ratio}">
  {#if source}
    <div class="stage">
      <div class="settings">
        <div class="group asks">
          <div class="row grouphead">
            <h3>New clips</h3>
            <span class="ask">
              <Info label="What finding clips does">
                The model reads the window chosen on the <b>range picker</b> and answers with the
                moments worth clipping. A part it has read is marked there. <b>Target</b> is how
                many clips it looks for. It gives fewer when fewer moments are strong enough.
                <b>Shortest</b> and <b>Longest</b> are how long each clip may run, in seconds,
                once the pauses and asides it leaves out are gone.
              </Info>
            </span>
          </div>
          <label class="setting">
            <span>Target</span>
            <span class="field"
              ><input
                class="num"
                type="number"
                min="1"
                max={clipsAtMost}
                title={comingNow
                  ? "The search on its way looks for this many. Change it for the next one"
                  : `How many clips the model looks for. Until you change it, it follows the window: ${suggested} for this one, 6 for half an hour and by the square root of its length for others. Type a number to set your own for this window, and clear it to follow the window again. A window of another length follows its own. The model gives fewer when fewer moments are strong enough. At most ${clipsAtMost}, as many as fit at ${min} s each in the longest window the model can read`}
                disabled={comingNow}
                value={comingNow && working?.count ? working.count : typed > 0 ? typed : suggested}
                onchange={keepTarget}
              /></span
            >
          </label>
          <label class="setting">
            <span>Shortest</span>
            <span class="field"
              ><input
                class="num"
                type="number"
                min="5"
                max={shortestAtMost}
                title={comingNow
                  ? "The search on its way asks for clips this long. Change it for the next one"
                  : `At most ${shortestAtMost} s, so ${count} clips of it fit in the longest window the model can read`}
                disabled={comingNow}
                bind:value={min}
                onchange={() => keepOrder("min")}
              /><span class="unit">s</span></span
            >
          </label>
          <label class="setting">
            <span>Longest</span>
            <span class="field"
              ><input
                class="num"
                type="number"
                min="5"
                max="180"
                title={comingNow ? "The search on its way asks for clips this long. Change it for the next one" : undefined}
                disabled={comingNow}
                bind:value={max}
                onchange={() => keepOrder("max")}
              /><span class="unit">s</span></span
            >
          </label>
        </div>
        {#if captions}
          <div class="group asks">
            <div class="row grouphead">
              <h3>Captions</h3>
              <span class="ask">
                <Info label="What the caption settings do">
                  The face, the size and the colours for every clip of this episode:
                  the words, the box behind them and the pill behind the word being spoken. A word too long
                  for a line is hyphenated onto the next. The number beside a colour is how much of it is seen: 100 is
                  solid, and a box at 0 leaves only the words. <b>Height</b> is how far above the bottom of the short the captions
                  sit, for every episode: drag the caption box in the video preview, or type it here.
                  With the captions any other way than they start out, the mark in this corner turns
                  anticlockwise and puts them back. Once they are back it turns clockwise instead and
                  puts them where you had them, so nothing is lost by trying.
                </Info>
              </span>
              <span class="grow"></span>
              <!-- The way back and the way there again, in one place: the
                   mark turns the captions back to where they belong, and
                   once they are there it turns the other way and offers the
                   height they had. It is not there when there is nothing to
                   undo either way. -->
              {#if captionsMoved}
                <button
                  class="quiet glyph"
                  title="Put the captions back as they start out: {captionFontDefault}, {captionSizeDefault}, {captionYDefault} from the bottom, white on a half clear black box"
                  aria-label="Put the captions back"
                  onclick={putCaptionsBack}
                >
                  <Icon name="back" />
                </button>
              {:else if captionsWere}
                <button
                  class="quiet glyph"
                  title="Put the captions back as you had them: {captionsWere.font}, {captionsWere.size}, {captionsWere.y} from the bottom, with their colours"
                  aria-label="Put the captions back as you had them"
                  onclick={putCaptionsAsTheyWere}
                >
                  <Icon name="forward" />
                </button>
              {/if}
            </div>
            <label class="setting" for="caption-font">
              <span>Font</span>
              <!-- The list of families stands in the same box as the numbers,
                   with its own mark where their unit is, so every setting
                   in the column ends in the same place whatever draws it. -->
              <span class="field">
                <!-- The families of the app, then the ones installed on
                     the machine under a head of their own, the way a
                     pop-up menu on the Mac parts its kinds of thing, each
                     drawn in its own regular face, with a field to search
                     them. A family picked comes in the style nearest the
                     one the captions are in. A face the video asks for and
                     the machine does not have is named on the field in the
                     colour of a warning, and the captions are drawn in the
                     app's own meanwhile. -->
                <Pick
                  value={captions.style.missing || captionFamily?.name || captions.style.font}
                  options={fontFamilies.map((family) => ({
                    value: family.name,
                    label: family.name,
                    group: family.machine ? "Installed" : undefined,
                    ...faceLook(nearestFace(family)),
                  }))}
                  onpick={(name) => {
                    const family = fontFamilies.find((f) => f.name === name);
                    if (family) setCaptionStyle(nearestFace(family, captionFace).name, 0);
                  }}
                  id="caption-font"
                  label="Font"
                  search="Search fonts"
                  hang="left"
                  title={captions.style.missing
                    ? `${captions.style.missing} is not installed on this computer, so the captions are drawn in ${captions.style.font}`
                    : "The font the captions are written in"}
                  tone={captions.style.missing ? "warn" : undefined}
                  align="right"
                />
              </span>
            </label>
            <label class="setting" for="caption-style">
              <span>Style</span>
              <!-- The faces of the family, Regular, Bold, Condensed Light,
                   each drawn as itself. A face the machine does not have
                   has no family here to offer the styles of. -->
              <span class="field">
                <Pick
                  value={captions.style.missing ? "" : captions.style.font}
                  options={(captionFamily?.faces ?? []).map((face) => ({
                    value: face.name,
                    label: captionStyles.get(face.name) ?? face.name,
                    ...faceLook(face),
                  }))}
                  onpick={(name) => setCaptionStyle(name, 0)}
                  id="caption-style"
                  label="Style"
                  placeholder={captions.style.missing ? "Not installed" : ""}
                  disabled={!!captions.style.missing}
                  hang="left"
                  title="How heavy, how wide and how slanted the font is"
                  align="right"
                />
              </span>
            </label>
            <label class="setting">
              <span>Size</span>
              <span class="field">
                <input
                  class="num"
                  type="number"
                  min="24"
                  max="200"
                  step="4"
                  value={Math.round(captions.style.chosenSize)}
                  onchange={(e) => setCaptionStyle("", Number(e.currentTarget.value))}
                />
              </span>
            </label>
            <!-- The colours follow the hand in the video preview while they
                 are picked, and are saved when the picker lets go. -->
            <!-- The text and how much of it is seen, the same pair as the
                 box under it. -->
            <!-- Its name is its switch, the way Highlight's is: off, no
                 captions are burned in at all, and a short can be made
                 without any. -->
            <div class="setting">
              <button
                class="name"
                class:off={!textOn}
                aria-pressed={textOn}
                title={textOn
                  ? "The captions are burned into the short. Click to turn them off and render without captions"
                  : "Off: the short is rendered without captions. Click to burn them in again"}
                onclick={() => setCaptionSwitch("text", !textOn)}>Text</button
              >
              <span class="field pair" class:off={!textOn}>
                <Colour
                  title="The colour the captions are written in"
                  label="Text colour"
                  value={textColour}
                  presets={captionColours}
                  oninput={(hex) =>
                    hex ? drawColour({ primary: joinColour(hex, textOpacity / 100), textOn: true }) : dropColourDraft()}
                  onchange={(hex) => setTextColour(hex, textOpacity)}
                  onsample={sampleColour}
                />
                <input
                  class="num"
                  type="number"
                  min="0"
                  max="100"
                  step="5"
                  title="How much of the caption text is seen. 100 is solid, less lets the picture through"
                  aria-label="Text opacity"
                  value={textOpacity}
                  oninput={(e) => {
                    const v = Math.min(100, Math.max(0, Number(e.currentTarget.value)));
                    if (Number.isFinite(v)) drawColour({ primary: joinColour(textColour, v / 100), textOn: true });
                  }}
                  onchange={(e) =>
                    setTextColour(textColour, Math.min(100, Math.max(0, Number(e.currentTarget.value) || 0)))}
                /><span class="unit">%</span>
              </span>
            </div>
            <!-- The box and how much of it is seen, side by side, because
                 they are one thing: what is behind the words. -->
            <div class="setting">
              <button
                class="name"
                class:off={!boxShown}
                aria-pressed={boxShown}
                title={!textOn
                  ? "Off with the text. Click to turn the captions on again, with the box"
                  : boxOn
                    ? "The captions sit on a box. Click to turn it off, so the words stand on the picture"
                    : "Off: the words stand on the picture. Click to put the box behind them again"}
                onclick={() => flipPart("box", boxOn)}>Box</button
              >
              <span class="field pair" class:off={!boxShown}>
                <Colour
                  title="The colour of the box behind the captions"
                  label="Box colour"
                  value={boxColour.hex}
                  presets={captionColours}
                  oninput={(hex) =>
                    hex ? drawColour({ box: joinColour(hex, boxOpacity / 100), boxOn: true }) : dropColourDraft()}
                  onchange={(hex) => setBoxColour(hex, boxOpacity)}
                  onsample={sampleColour}
                />
                <input
                  class="num"
                  type="number"
                  min="0"
                  max="100"
                  step="5"
                  title="How much of the box behind the captions is seen. 0 leaves only the words, 100 hides the picture behind them"
                  aria-label="Box opacity"
                  value={boxOpacity}
                  oninput={(e) => {
                    const v = Math.min(100, Math.max(0, Number(e.currentTarget.value)));
                    if (Number.isFinite(v)) drawColour({ box: joinColour(boxColour.hex, v / 100), boxOn: true });
                  }}
                  onchange={(e) =>
                    setBoxColour(boxColour.hex, Math.min(100, Math.max(0, Number(e.currentTarget.value) || 0)))}
                /><span class="unit">%</span>
              </span>
            </div>
            <!-- The pill behind the word being spoken, beside the other two
                 colours of the captions and the same pair as they are, so
                 every colour the short and the clip timeline show is set
                 in one place and in one way. Its name is also its switch,
                 the way an entry in a chart's legend turns its line on and
                 off: at rest it reads like every other name, under the hand
                 it lifts like a quiet button, and off it is struck through
                 and its colour steps back. Nothing moves either way. -->
            <div class="setting">
              <button
                class="name"
                class:off={!highlightShown}
                aria-pressed={highlightShown}
                title={!textOn
                  ? "Off with the text. Click to turn the captions on again, with the highlight"
                  : highlightOn
                  ? "The word being spoken sits on a pill that bounces. Click to turn it off, so the captions are the box and the words"
                  : "Off: the captions are the box and the words. Click to light up the word being spoken again"}
                onclick={() => flipPart("highlight", highlightOn)}>Highlight</button
              >
              <span class="field pair" class:off={!highlightShown}>
                <Colour
                  title="The colour of the pill behind the word being spoken"
                  label="Highlight colour"
                  value={highlightColour}
                  presets={captionColours}
                  oninput={(hex) =>
                    hex
                      ? drawColour({ highlight: joinColour(hex, highlightOpacity / 100), highlightOn: true })
                      : dropColourDraft()}
                  onchange={(hex) => setHighlightColour(hex, highlightOpacity)}
                  onsample={sampleColour}
                />
                <input
                  class="num"
                  type="number"
                  min="0"
                  max="100"
                  step="5"
                  title="How much of the pill behind the word being spoken is seen. 100 is solid, less lets the box and the picture through"
                  aria-label="Highlight opacity"
                  value={highlightOpacity}
                  oninput={(e) => {
                    const v = Math.min(100, Math.max(0, Number(e.currentTarget.value)));
                    if (Number.isFinite(v))
                      drawColour({ highlight: joinColour(highlightColour, v / 100), highlightOn: true });
                  }}
                  onchange={(e) =>
                    setHighlightColour(
                      highlightColour,
                      Math.min(100, Math.max(0, Number(e.currentTarget.value) || 0)),
                    )}
                /><span class="unit">%</span>
              </span>
            </div>
            <label class="setting">
              <span>Height</span>
              <span class="field">
                <input
                  class="num"
                  type="number"
                  min={captionYMin}
                  max={captionYMax}
                  step={captionYStep}
                  value={Math.round(captionY)}
                  onchange={(e) => setCaptionsHeight(Number(e.currentTarget.value))}
                /><span class="unit">px</span>
              </span>
            </label>
          </div>
        {/if}
      </div>
      <div class="middle" bind:this={middle}>
        <Player
          bind:this={player}
          bind:paused
          bind:looping
          bind:onClip
          bind:offers
          {path}
          {source}
          missing={!!status?.missing}
          clip={shownClip}
          making={making?.key}
          opening={!opened}
          bind:pictured
          bind:frameStart={shownStart}
          sleeping={away}
          bind:time
          captions={shownClip && captionsOf === shownClip.key ? shownCaptions : null}
          captionsDue={!!shownClip && captionsOf !== shownClip.key}
          {wordFrames}
          drafts={captionDrafts}
          onplayclip={(c) => api.clipPlayed(c.plan, c.id).catch(() => {})}
          oncrop={(at, left) => (current ? setCrop(current, at, left) : Promise.resolve())}
          onresetcrop={(at) => (current ? resetCrop(current, at) : Promise.resolve())}
          oncropmark={(from, to, x) => (current ? setCropMark(current, from, to, x) : Promise.resolve())}
          oncaptiony={(y) => setCaptionsHeight(y)}
          onword={(start, text) => (current ? setWord(current, start, text) : Promise.resolve())}
          onremoveword={(start, part) => (current ? removeWord(current, start, part) : Promise.resolve())}
          locked={renderingCurrent}
          oncaptionmoved={(y) => {
            captionsWere = null;
            captionY = y;
          }}
          {strip}
        />
      </div>
      <aside>
        <div class="pane asks">
          <div class="row listhead">
            {#if saying}
              <!-- The search through what was said takes the place of the
                   head, the way the field over the font list does, and
                   Escape or the cross gives the head back. -->
              <SearchField
                framed
                bind:field={sayField}
                bind:value={sayTyped}
                placeholder="Search transcript"
                aria-label="Search transcript"
                onkeydown={(e) => {
                  if (e.key === "Escape") {
                    e.preventDefault();
                    e.stopPropagation();
                    escapeSaying();
                  }
                  // A space at the start of nothing is no search, so with
                  // nothing typed the space bar plays and stops, and the
                  // field lets go of the keyboard, found by Tim. The
                  // player hears the same key on the window after this,
                  // with the keyboard back in the workspace.
                  if (e.code === "Space" && !e.shiftKey && !sayTyped.trim()) {
                    e.preventDefault();
                    sayField?.blur();
                    sayRefocus = paused;
                  }
                  // Shift and the arrows up and down walk the cards from
                  // the field too, where they would select its text.
                  if ((e.key === "ArrowDown" || e.key === "ArrowUp") && e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey) {
                    e.preventDefault();
                    e.stopPropagation();
                    walkSaid(e.key === "ArrowDown" ? 1 : -1);
                  }
                  // Enter stays at the card walked to.
                  if (e.key === "Enter" && sayFrom) {
                    e.preventDefault();
                    said("search what was said: Enter stays at the card walked to");
                    sayFrom = null;
                  }
                }}
              >
                {#if sayTyped.trim()}
                  <!-- How many cards match, the number fading in anew
                       as it changes, the way a toast comes. -->
                  <span class="saycount muted num" title="Cards that match">
                    {#key sayMatches}<span in:fade={{ duration: 150 }}>{sayMatches}</span>{/key}
                  </span>
                {/if}
                <button class="glyph quiet" title="Close the search, Escape does the same" aria-label="Close the search" onclick={closeSaying}>
                  <Icon name="close" size={12} />
                </button>
              </SearchField>
            {:else}
            <h2>{lane.word}</h2>
            {#if lane.count}
              <span class="muted num">{shown.length}</span>
            {/if}
            <!-- Nothing about the transcription here. It has a place of
                 its own now, at the edge it moves, on the range picker.
                 A thirteen pixel mark in the head of a list about something
                 else was a thing nobody could name. -->
            <span class="ask">
              <Info label={waitNote ? "What is happening" : "What the clip list is"} side="right">
                {#if waitNote}
                  {waitNote}
                {:else}
                  Every clip found, in the order they were spoken, and on the
                  <b>range picker</b> as marks. Click one to work on it, the trash can takes it
                  out. <b>Search</b> looks in the window chosen on the <b>range picker</b>.
                {/if}
              </Info>
            </span>
            <span class="grow"></span>
            <button class="glyph" title="Search the transcript" aria-label="Search the transcript" onclick={openSaying}>
              <Icon name="transcript" />
            </button>
            <button
              class="search"
              class:primary={action.primary}
              onclick={action.run}
              disabled={action.off}
              title={`${action.title}${busy && leftOfWork ? `, ${leftOfWork}` : ""}`}
            >
              <!-- A search says how it is going in the row its next clip
                   will appear in, where it can say what it is doing as
                   well, so the button only offers to stop it. -->
              {action.label}
            </button>
            {/if}
          </div>
          <div class="scroll list">
            {#if saying}
              <SaidList
                bind:this={saidList}
                sentences={saved.length ? sentences : []}
                typed={sayTyped}
                {unheard}
                {unheardLit}
                clips={shown}
                {selected}
                chosen={chosenTimes}
                bind:matches={sayMatches}
                bind:holeOn={unheardOn}
                onhover={hoverClip}
                onhoverhole={(i) => (unheardLit = i)}
                playhead={time}
                playing={!paused}
                failed={sentencesFailed}
                onseek={(at, clip, walked) => {
                  if (walked && sayTyped.trim()) {
                    sayFrom ??= { at: time, clip: selected };
                    sayOn = [at, walked.end];
                  } else if (!walked) sayFrom = null;
                  // A sentence of a clip makes that clip the one worked on,
                  // the way its mark does, found by Tim.
                  if (clip && clip !== selected) void select(clip, false, false);
                  seekTo(intoWord({ start: at, end: at + 0.2 }, source && source.fps > 0 ? 1 / source.fps : 1 / 30));
                }}
              />
            {:else}
            <ClipList
              clips={shown}
              live={current ? numbers : null}
              arriving={onTheWay}
              {selected}
              {hovered}
              onhover={hoverClip}
              {coming}
              at={stopped ? stopped.to : to}
              waiting={comingNow}
              next={shownNext}
              {carry}
              stopped={stoppedRow}
              removed={removedRows}
              emptied={note}
              onforget={forget}
              onheld={held}
              onclosed={closed}
              onselect={select}
              onremove={removeClip}
              onputback={putClipBack}
            />
            {/if}
          </div>
        </div>
      </aside>
    </div>

    <!-- data-framed names the track of captions whose words the clip
         timeline puts the playhead into by the frames the picture lights
         them on, see lit, and data-answered the track the engine has
         answered for, with frames or none. A walk clicks a caption block
         once a click goes by the frames, issue #290. Nothing on screen
         reads them. -->
    <div class="detail" data-framed={framedTrack} data-answered={answeredTrack}>
      <ClipTimeline
        bind:this={timeline}
        {path}
        waiting={!opened}
        clip={current ?? making}
        {duration}
        heard={saved}
        {measured}
        {measuredParts}
        {time}
        shown={shownMoment}
        locked={renderingCurrent || (!current && !!making)}
        arriving={!current && !!making}
        frame={source.fps > 0 ? 1 / source.fps : 1 / 30}
        frameStart={source.videoStart ?? 0}
        {lit}
        bind:numbers
        bind:drawn
        onseek={(t, about, wordStep) => player?.seek(t, about, wordStep)}
        onahead={readAhead}
        onhold={(held) => player?.hold(held)}
        dimmed={!onClip}
        onreshape={(g, playhead) => (current ? reshape(current, g, playhead) : Promise.resolve())}
        onwalkclip={walkClip}
        {looping}
        playing={!paused}
        thumbnails={current?.thumbnails ?? []}
        onthumbnail={(from, to) => (current ? setThumbnail(current, from, to) : Promise.resolve())}
        cropMarks={current?.cropMarks ?? []}
        oncropmark={(from, to) => (current ? setCropMark(current, from, to, -1) : Promise.resolve())}
        {marks}
        {hovered}
        onhover={hoverClip}
        onmark={select}
        centre={() => {
          const box = middle?.getBoundingClientRect();
          return box && box.width > 0 ? box.left + box.width / 2 : undefined;
        }}
        captions={captions?.captions ?? []}
        captionPieces={captions?.pieces}
        captionLook={shownCaptions
          ? {
              text: shownCaptions.style.primary,
              box: shownCaptions.style.box,
              highlight: shownCaptions.style.highlight
                ? shownCaptions.style.highlightColour
                : "transparent",
            }
          : null}
        oncaptiontime={(word, edge, at, toWords) =>
          current ? setCaptionTime(current, word, edge, at, toWords) : Promise.resolve(false)}
        onremovecaption={(first) => (current ? removeCaption(current, first) : Promise.resolve())}
        oncaptionshape={(shape) => (captionShape = shape)}
        onshape={(next) => (reshaped = next)}
      />
      <!-- One row under the clip up close, so the range picker and the
           waveform stand together: what plays on the left, what the
           selected clip is in the middle, and what becomes of it on the
           right. -->
      <div class="row">
        <!-- What plays, as the three marks anyone knows: the triangle, the
             two bars, and the tape going round. The row keeps its width
             whatever they say, because an icon is an icon wide. -->
        <button
          class="glyph"
          onclick={() => player?.toggle()}
          aria-label={paused ? "Play" : "Pause"}
          title={paused
            ? "Play from the playhead. The space bar does the same"
            : "Pause. The space bar does the same"}
        >
          <Icon name={paused ? "play" : "pause"} />
        </button>
        {#if current}
          <button
            class="glyph"
            class:on={looping}
            aria-pressed={looping}
            aria-label="Loop the clip"
            onclick={() => (looping = !looping)}
            title={looping
              ? "Stop looping, so the clip stops at its end. L does the same"
              : "Play the clip again at its end. L does the same"}
          >
            <Icon name={looping ? "loop-off" : "loop"} />
          </button>
        {/if}
        {#if current}
          <!-- The frame under the playhead as a thumbnail. It is not a
               mode, so it never looks pressed: its picture says what a
               click does, a plus to add one and a minus when the
               playhead stands on one. -->
          <button
            class="glyph"
            aria-label={thumbHere !== null ? "Remove this thumbnail" : "Make this frame a thumbnail"}
            disabled={renderingCurrent || (thumbHere === null && !inShort)}
            onclick={toggleThumbnail}
            title={thumbHere !== null
              ? "Remove the thumbnail at the playhead. T does the same"
              : inShort
                ? "Make the frame under the playhead a thumbnail. Render writes it beside the short. T does the same"
                : "Put the playhead in the clip to make a thumbnail of the frame there"}
          >
            <Icon name={thumbHere !== null ? "thumbnail-remove" : "thumbnail-add"} />
          </button>
          <!-- A keyframe on the frame under the playhead, made the way a
               thumbnail is: a plus to put one with the crop where it stands,
               a minus when the playhead stands on one. With two or more the
               crop glides from each to the next. Plan row 3.26. -->
          <button
            class="glyph"
            aria-label={cropHere !== null ? "Remove this keyframe" : "Put a keyframe on this frame"}
            disabled={renderingCurrent || (cropHere === null && !inShort)}
            onclick={toggleCropMark}
            title={cropHere !== null
              ? "Remove the keyframe at the playhead. C does the same"
              : inShort
                ? "Put a keyframe on the frame under the playhead, with the crop where it stands. With two or more the crop moves from each to the next. C does the same"
                : "Put the playhead in the clip to put a keyframe on the frame there"}
          >
            <Icon name={cropHere !== null ? "keyframe-remove" : "keyframe-add"} />
          </button>
          <!-- Rotate Left, the way Preview and Photos rotate: the part of
               the clip between two cuts the playhead stands in, a quarter
               turn to the left a click, four clicks round. -->
          <button
            class="glyph"
            aria-label="Rotate this part of the clip to the left"
            disabled={renderingCurrent || !inShort}
            onclick={rotatePart}
            title={inShort
              ? "Rotate the part of the clip between two cuts under the playhead a quarter turn to the left. R does the same"
              : "Put the playhead in the clip to rotate the part of it there"}
          >
            <Icon name="rotate" />
          </button>
        {/if}
        <!-- Mark In and Mark Out, the way every editor marks a clip: the
             two square brackets on the buttons, and I and O on the keys.
             Each makes a clip at the playhead, and only where no clip is:
             inside one, its edges are dragged instead. -->
        <button
          class="glyph"
          disabled={duration <= 0 || inClip || noIn}
          onclick={() => makeClip(false)}
          aria-label="Mark In"
          title={makingIn
            ? "Making a clip"
            : inClip
              ? "The playhead is in a clip. Drag its edges to change it"
              : noIn
                ? "Nothing is said after the playhead, so no clip can start here"
                : "Mark In: start a clip with the sentence under the playhead, as long as Shortest. I does the same"}
          >{#if makingIn}<Busy />{/if}<Icon name="mark-in" /></button
        >
        <button
          class="glyph"
          disabled={duration <= 0 || inClip || noOut}
          onclick={() => makeClip(true)}
          aria-label="Mark Out"
          title={makingOut
            ? "Making a clip"
            : inClip
              ? "The playhead is in a clip. Drag its edges to change it"
              : noOut
                ? "Nothing is said before the playhead, so no clip can end here"
                : "Mark Out: end a clip with the sentence under the playhead, grown back to Shortest. O does the same"}
          >{#if makingOut}<Busy />{/if}<Icon name="mark-out" /></button
        >
        <!-- One job, whatever is chosen: go to the playhead. Going back to
             the clip is what clicking it in the list does. -->
        <button
          class="glyph"
          onclick={() => timeline?.toPlayhead()}
          aria-label="Go to the playhead"
          title="Go to the playhead"
        >
          <Icon name="locate" />
        </button>
        <!-- Where the playhead is, to the frame, the way an editor shows
             it beside its transport: a field as tall as the buttons, as
             wide at the start of the video as at its end. What the clip
             is, its start, length and pieces, its card in the list says,
             so the row says it no more. Asked for by Tim, issue #361. -->
        <output
          class="timecode num selectable"
          class:long={duration >= 3600}
          aria-label="Playhead"
          title="Where the playhead is: hours, minutes, seconds and the millisecond its frame starts on"
          >{playheadClock(time, frameLen, frameStart, duration >= 3600)}</output
        >
        {#if numbers.saving}<span class="muted">Saving</span>{/if}
        <span class="grow"></span>
        {#if offers.hint}<span class="muted num">{offers.hint}</span>{/if}
        {#if offers.crop === "auto"}
          <button
            onclick={() => player?.resetCrop()}
            disabled={offers.savingCrop}
            title="Let the app place the crop again">Automatic crop</button
          >
        {:else if offers.crop === "back"}
          <button
            onclick={() => player?.cropBack()}
            disabled={offers.savingCrop}
            title="Back to where you had it">Put the crop back</button
          >
        {/if}
        {#if current}
          {#if current.short}
            <button onclick={() => api.reveal(current.short!)}>Show in folder</button>
          {/if}
          <!-- The one act that matters, so it says how it is going in the
               button it was started from, and becomes the way to stop it,
               the same as Search does for a search. -->
          {@const inHand = renderingCurrent || renderAsked === current.key}
          <button
            class="primary render"
            onclick={() => (inHand ? stopRender() : render(current))}
            disabled={inHand ? renderStopping || !renderingCurrent : !!working}
            title={inHand
              ? `Stop the render${leftOfWork ? `, ${leftOfWork}` : ""}`
              : working
                ? "Render once the work running now is done"
                : "Write the short, and its thumbnails beside it"}
            >{#if inHand}<Busy fraction={renderingCurrent ? renderShare : -1} />{/if}{inHand
              ? renderStopping
                ? "Cancelling"
                : "Cancel"
              : current.rendered
                ? "Render again"
                : "Render"}</button
          >
        {/if}
      </div>
    </div>
  {/if}
</section>

<style>
  /* The whole workspace, worked out here rather than in JavaScript.
     Everything below is one expression the browser evaluates in the same
     pass as the resize, so dragging the edge of the app moves the
     workspace with it instead of a frame or two behind it.

     One number comes from outside: --ar, the shape of the episode. It
     does not change because the app was resized. The rest is the size of the app
     itself and the tokens from app.css. */
  section {
    /* The height left in the app: everything but the bar at the top, the
       space above the workspace and the edge at the foot. */
    --space: calc(100dvh - var(--bar-h) - var(--gap) - var(--edge));
    /* The middle column, the rest of the app's width once the two sides,
       --spans from app.css, have theirs, with a space either side of it. */
    --mid-w: calc(100dvw - var(--spans) - 2 * var(--gap));
    /* Everything down the height that is neither the picture nor a track:
       four spaces, one under the picture, two around the line that parts
       the workspace from the clip up close, one over the row at the foot,
       the line itself, and the row. */
    --down: calc(4 * var(--gap) + 1px + var(--row-h));
    /* How tall the picture would be if it were as wide as the middle
       column. */
    --widest: calc(var(--mid-w) / var(--ar));
    /* The picture takes the height first, until it is that wide, leaving
       the tracks at least their smallest. What it cannot use goes to the
       two tracks, the range picker a third of it and the clip timeline
       the rest, so nothing is left over at the foot of the app. */
    --pic-h: max(200px, min(var(--widest), calc(var(--space) - var(--down) - 1.5 * var(--wave-min))));
    --tracks: calc(var(--space) - var(--down) - var(--pic-h));
    --picker-h: calc(var(--tracks) / 3);
    --wave-h: calc(var(--tracks) - var(--picker-h));
    --pic-w: calc(var(--pic-h) * var(--ar));

    padding: var(--gap) var(--edge) var(--edge) var(--gap);
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    flex: 1;
    min-height: 0;
    overflow: auto;
  }

  /* Every height a whole number of pixels, where the browser can do the
     rounding. A track half a pixel tall puts everything below it half a
     pixel out, and a line, a label or a mark drawn between two pixels is
     soft, and is painted in a different place once a fade puts it on a
     surface of its own.

     And every size rounded so it only ever moves the way the edge of the
     app does. Each is rounded down once, and what is left of it goes to
     the one size worked out after it, never back to one before. The clip
     timeline was rounded down to an even number, so the range picker,
     half of it, was whole too, and the picture took what was left. That
     could only change in steps of three pixels, so as the app shrank the
     picture shrank a pixel, another, and then grew three, and the range
     picker under it and the clip list beside it went with it: the shaking
     Tim recorded, a sawtooth of two, two and six on his screen. Now the
     picture is rounded first, from the app alone, and the range picker
     and the clip timeline share what it leaves, a third and the rest,
     within a pixel of one half the other. */
  @supports (height: round(down, 3px, 2px)) {
    section {
      /* The height the tracks leave at their smallest is whole by its
         sum, and only the browser's arithmetic makes it 462.99999, which
         rounded down took a pixel from the picture on one step in a few
         hundred and gave it to the clip timeline. So it is rounded to the
         nearest pixel, and only the height the shape of the picture
         gives, which is a fraction, is rounded down. A size worked out by
         multiplying or dividing comes out a hair under a whole pixel just
         as often, 539.99999 for a middle column of 960, so a hundredth of
         a pixel is added before it is rounded down: far more than the
         arithmetic is ever off by, and far less than anything seen. */
      --pic-h: max(
        200px,
        min(
          round(down, calc(var(--widest) + 0.01px), 1px),
          round(calc(var(--space) - var(--down) - 1.5 * var(--wave-min)), 1px)
        )
      );
      --tracks: round(calc(var(--space) - var(--down) - var(--pic-h)), 1px);
      --picker-h: round(down, calc(var(--tracks) / 3), 1px);
      /* The picture's width is its height times its shape, rounded down,
         except where the width is what holds it in: there it is the
         middle column's width exactly. A width worked out from a height
         that was itself rounded down came a pixel or two short of the
         column, and the picture, centred in what was left, stood 0, 1, 0,
         1 pixels in as the app was dragged. The second term is the
         column's width while the picture's height is the one the column
         allows, and far below anything else otherwise, so the larger of
         the two is the column where the width holds the picture and its
         shape everywhere else. And never wider than the column, which a
         picture held at its smallest height on a narrow app would be. */
      --pic-w: min(
        var(--mid-w),
        max(
          round(down, calc(var(--pic-h) * var(--ar) + 0.01px), 1px),
          calc(
            var(--mid-w) -
              max(
                round(down, calc(var(--widest) + 0.01px), 1px) - var(--pic-h),
                var(--pic-h) - round(down, calc(var(--widest) + 0.01px), 1px)
              ) *
              100000
          )
        )
      );
    }
  }

  /* Nothing in the column may shrink below its own height, or the player
     would reach into the timeline under it. */
  section > :global(*) {
    flex-shrink: 0;
  }


  /* The playhead's time, in a field of its own beside the buttons: as
     tall as a control, with the border a field has, and as wide as the
     time with the most figures this video can have, so the row never
     moves as the playhead does. Tabular figures, so each figure stands
     where the one before it stood. The border, the background and the
     corners are a field's, from app.css, and the room either side of
     the time is a field's padding, 12px, with its border. */
  .timecode {
    box-sizing: border-box;
    flex: none;
    height: var(--control-h);
    width: calc(9ch + 26px);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--ink-2);
    white-space: nowrap;
  }

  .timecode.long {
    width: calc(11ch + 26px);
  }

  /* What plays and what the timeline looks at say it with a mark rather
     than a word: the three of them are known everywhere, and a mark cannot
     change length as you use it. They are as tall as every other control in
     the row and as wide as they are tall. */
  /* Square, as tall as every control and as wide, and never squeezed: in
     a row narrower than its controls a button gave way first, and stood
     taller than it was wide. Found by Tim on #192. */
  .detail .row .glyph {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: var(--control-h);
    padding: 0;
  }

  h2 {
    font-size: var(--size-l);
    font-weight: 600;
  }

  .grow {
    flex: 1;
  }

  /* Three columns: the settings the sidebar lies over when it opens, the
     video preview, and the clips. The two beside the picture are the same
     width, from the app's width alone, and the middle one takes the rest,
     so a wider app makes all three wider and a taller or shorter one moves
     none of them. */
  .stage {
    display: grid;
    grid-template-columns: var(--side-w) 1fr var(--clips-w);
    gap: var(--gap);
    align-items: stretch;
    /* Exactly the picture, the space under it and the range picker. The
       columns beside them are that tall too, and the line under the
       workspace sits right below the range picker. */
    height: calc(var(--pic-h) + var(--gap) + var(--picker-h));
    flex: none;
    min-height: 0;
  }

  /* The room the video preview has. It is measured, and what the player
     makes of it is what the whole workspace is tall. */
  .middle {
    min-width: 0;
  }

  .settings {
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-width: 0;
    padding-right: var(--gap);
    border-right: 1px solid var(--line);
    /* On an app too short for every setting, the last of them would lie
       under the clip panel with no way to reach it. It scrolls instead,
       and only then. */
    overflow-y: auto;
    scrollbar-width: none;
    /* A column that scrolls cuts off whatever reaches past its edge, and
       the lift under the Highlight name reaches seven pixels to the left of
       the names. The column takes eight pixels of the edge beside it and
       gives them back as padding, so the names stay where they are and the
       lift is whole. */
    margin-left: -8px;
    padding-left: 8px;
  }

  /* A name and a small field beside it read worse the further apart they
     are, so the settings take a little of a wider app and leave the
     rest. The column itself still grows, which is what keeps the video
     preview where it is. */
  .group {
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-width: 320px;
  }

  /* As tall as a control, so the head is a whole number of pixels high
     and the mark in it lands on a whole pixel. An element painted between
     two pixels is painted differently once it is faded in, because a fade
     puts it on a surface of its own and that surface starts on a whole
     pixel, so the mark appeared to step as it arrived. */
  .grouphead {
    gap: 4px;
    height: var(--control-h);
  }

  /* A mark that undoes something sits at the end of the row the thing is
     in, and is only there while there is something to undo. */
  .grouphead .glyph {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    padding: 0;
    color: var(--muted);
  }

  .grouphead .glyph:hover:not(:disabled) {
    color: var(--text);
  }

  /* The head of a group of settings is the head of an area of the
     workspace, the same as the head of the clip list, so it is the same
     size. Two heads of the same kind at two sizes read as a hierarchy that
     is not there. */
  h3 {
    font-size: var(--size-l);
    font-weight: 600;
    margin: 0;
  }

  /* The name on the left, the control on the right, every control the same
     width, so the column reads as one list. */
  .setting {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--muted);
  }

  .setting > span:first-child {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .field {
    position: relative;
    flex: none;
  }

  /* A name that is also a switch. At rest it is every other name: the same
     place, colour and weight, so the column reads as one. Under the hand it
     lifts like a quiet button, the room for that taken out of the gap
     before it so the word itself never moves. Off, it is struck through,
     the way a legend shows a line that is hidden. */
  .setting > button.name {
    flex: 1;
    min-width: 0;
    height: var(--control-h);
    /* Six pixels of lift and the button's own border, which is there but
       unseen, so the word starts where the names above it start. */
    margin-left: -7px;
    padding: 0 6px;
    border-color: transparent;
    background: transparent;
    color: inherit;
    text-align: left;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .setting > button.name:hover {
    background: var(--lift);
    color: var(--text);
  }

  .setting > button.name.off {
    text-decoration: line-through;
    color: var(--faint);
  }

  /* A colour that has nothing to do while its switch is off. */
  .field.off {
    opacity: 0.45;
  }

  .field.off input {
    cursor: default;
  }

  .field,
  .setting input {
    width: 116px;
  }

  .setting input {
    color: var(--text);
  }

  /* What a search on its way was asked for is held while it runs: the
     number went to the model with the prompt, so changing it changes
     nothing the model does. Dimmed the way a button that cannot be pressed
     is, in app.css, the field and its unit together. The input alone left
     the unit bright beside a dimmed number. */
  .setting .field:has(input:disabled) {
    opacity: 0.45;
  }

  .setting input:disabled {
    cursor: default;
  }

  /* The stepper the system draws inside a number field would stand between
     the number and its unit, so the field is plain. The number is typed, or
     nudged with the arrow keys. */
  .setting input[type="number"] {
    appearance: textfield;
  }

  .setting input::-webkit-outer-spin-button,
  .setting input::-webkit-inner-spin-button {
    appearance: none;
    margin: 0;
  }

  /* Every number in the column ends in the same place, whether it wears a
     unit or not, so the numbers read as one column. */
  .setting input {
    text-align: right;
    padding-right: 29px;
  }

  /* The box and its opacity share the width one field has, the colour
     first and the number after it, which still ends where every other
     number in the column ends. */
  .field.pair {
    display: flex;
    gap: 4px;
  }

  .pair input.num {
    flex: 1;
    min-width: 0;
  }

  /* The unit stands in a place of its own, as wide as the longest one and
     read from its left, so every unit is the same one space from its
     number. */
  .unit {
    position: absolute;
    top: 0;
    right: 8px;
    width: 16px;
    line-height: var(--control-h);
    pointer-events: none;
  }

  /* The clip list is exactly as tall as the video preview with the track
     and the controls under it, and scrolls inside that. Its own length never
     stretches the block, which would leave a gap beside the preview. */
  aside {
    position: relative;
    border-left: 1px solid var(--line);
    min-height: 240px;
  }

  .pane {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    left: var(--gap);
    display: flex;
    flex-direction: column;
  }

  /* No box around the clips, and no line under the head. The list simply
     goes under a veil at each end: a card that scrolls out of sight fades
     away rather than being cut off at an edge, which is what tells you
     there is more of it. The space inside the scroller is what the veil
     eats, so at rest the first and last card are whole. */
  .list {
    flex: 1;
    min-height: 0;
    --veil: 16px;
    mask-image: linear-gradient(
      to bottom,
      transparent 0,
      #000 var(--veil),
      #000 calc(100% - var(--veil)),
      transparent 100%
    );
    -webkit-mask-image: linear-gradient(
      to bottom,
      transparent 0,
      #000 var(--veil),
      #000 calc(100% - var(--veil)),
      transparent 100%
    );
  }

  .detail {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    padding-top: var(--gap);
    border-top: 1px solid var(--line);
  }

  /* As tall as the layout counted on, whether a clip is chosen or not, so
     choosing one never moves the two tracks above it. */
  .detail .row {
    height: var(--row-h);
  }


  /* A whole number of pixels high, so everything in it sits on whole
     pixels. No line under it: the veil over the top of the list is what
     parts the head from the clips now. */
  .listhead {
    position: relative;
    gap: 6px;
    height: var(--control-h);
  }

  /* What is running fills the button it was started from, behind its own
     words. The control that started the work is the one that should say
     how the work is going, and it leaves the head with nothing drawn
     across it. Busy.svelte is the whole of it, here and everywhere else
     something runs. */

  .listhead .grow {
    flex: 1;
  }

  .saycount {
    flex: none;
    line-height: 16px;
  }

  label {
    color: var(--muted);
  }

  label input {
    color: var(--text);
  }


</style>
