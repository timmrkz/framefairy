<script lang="ts">
  import { onMount, tick } from "svelte";
  import {
    api,
    captionFontDefault,
    captionSizeDefault,
    captionYDefault,
    captionYMax,
    captionYMin,
    captionYStep,
    clock,
    errorText,
    mediaURL,
    snapCaptionY,
    type CaptionFont,
    type CaptionsView,
    type ClipEntry,
    type CoverageView,
    type EpisodeStatus,
    type SourceView,
    type WindowView,
  } from "../lib/api";
  import { chosen, jobs } from "../lib/state.svelte";
  import { Heard, Newest, nextWindow, shouldLook, shouldTranscribe } from "../lib/flow";
  import { installFonts } from "../lib/fonts";
  import RangeWindow from "../components/RangeWindow.svelte";
  import Player, { type PlayerOffers } from "../components/Player.svelte";
  import ClipList from "../components/ClipList.svelte";
  import ClipTimeline, { type ClipNumbers } from "../components/ClipTimeline.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import Confirm from "../components/Confirm.svelte";

  let { path, onchange }: { path: string; onchange: () => void } = $props();

  // Long episodes open on their first half hour, short ones whole.
  const firstLook = 30 * 60;

  let status = $state<EpisodeStatus | null>(null);
  let source = $state<SourceView | null>(null);
  let clips = $state<ClipEntry[]>([]);
  let from = $state(0);
  let to = $state(0);
  let count = $state(12);
  let min = $state(20);
  let max = $state(30);
  let problem = $state("");
  let waiting = $state<string[]>([]);
  // A click has to show at once, long before the job it starts reports in.
  let starting = $state(false);
  // The frame the video preview shows while it cannot show the playhead
  // itself.
  let still = $state("");
  let time = $state(0);
  // Where the model has already looked. A window is only drawn outside it.
  let coverage = $state<CoverageView>({ searched: [], free: [] });
  let selected = $state("");
  let player = $state<Player>();
  let timeline = $state<ClipTimeline>();
  // What the clip timeline has to say about the chosen clip, for the row
  // under it.
  let numbers = $state<ClipNumbers>({ start: 0, end: 0, seconds: 0, pieces: 0, saving: false });
  // The playback buttons stand in the row above the clip timeline, with
  // Render, so the video preview and the range picker have nothing under
  // them but the line that parts them from it.
  // The workspace measures itself: how much height the window leaves it,
  // and how wide it is. From those two the video preview gets the biggest
  // size it can have, and the columns beside it take everything else.
  // What sits above the workspace when anything does: an error line. It is
  // nothing at all most of the time, and it never changes because the
  // window changed, so reading it costs nothing while the window is being
  // dragged.
  let aboveH = $state(0);
  let paused = $state(true);
  let looping = $state(false);
  let offers = $state<PlayerOffers>({ crop: "", savingCrop: false, hint: "" });

  const duration = $derived(source?.duration ?? 0);
  const transcribing = $derived(jobs.active(path, "transcribe"));
  const working = $derived(jobs.active(path, "work"));
  const finding = $derived(working?.kind === "plan");
  // Whether work is running, as a plain yes or no. The Go side sends a job
  // event about once a second, and every one of them replaces the job
  // object, so anything that watches the job itself is torn down and set
  // up again that often. A timer that watches one never fires at all,
  // which is what stopped the transcript from being read again while it
  // grew, and with it the first search.
  const isTranscribing = $derived(!!transcribing);
  const isFinding = $derived(!!finding);
  // Anything in the work lane, a search or a render. While it runs, the
  // head of the clip list carries it: New becomes Cancel and the line under
  // the head fills up. Nothing is added to the column and nothing moves.
  const busy = $derived(!!working || starting);
  const leftOfWork = $derived(
    working?.progress && working.progress.remaining > 0
      ? `${clock(working.progress.remaining)} left`
      : "",
  );
  let stopping = $state(false);

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
  const lane = $derived({ word: "Clips", count: true, job: busy ? working : null });
  const action = $derived.by(() => {
    if (busy) {
      return {
        label: stopping ? "Cancelling" : "Cancel",
        icon: "close",
        run: stopWork,
        off: stopping || (starting && !working),
        primary: false,
        title: `${finding || starting ? "Stop looking for clips" : "Stop the render"}${leftOfWork ? `, ${leftOfWork}` : ""}`,
      };
    }
    return {
      label: "New",
      icon: "plus",
      run: newClips,
      off: !readyToLook,
      primary: true,
      title: !readyToLook
        ? `The transcript reaches ${clock(covered)}. Clips can be looked for once it reaches ${clock(to)}`
        : covering
          ? "Look at this stretch again, removing the clips it has"
          : "Look for clips in the chosen stretch",
    };
  });

  // How far whatever the head is about has come.
  const share = $derived(
    lane.job?.progress && lane.job.progress.fraction >= 0 ? lane.job.progress.fraction : -1,
  );

  $effect(() => {
    if (!working) stopping = false;
  });

  function stopWork() {
    if (!working) return;
    stopping = true;
    api.cancelJob(working.id);
  }
  const covered = $derived(status?.transcribed ? duration : (status?.covered ?? 0));

  // A transcription that was stopped part way through. The episode still
  // wants words, some of them are already read, and nothing is reading the
  // rest. It is the state pausing leaves behind, and until it had a name
  // there was no way back out of it: the mark that stops the transcription
  // was only there while it ran, and once a clip existed the head never
  // went back to being about words, so nothing ever offered to carry on.
  const partly = $derived(needsWords && covered > 0 && !transcribing && !starting);
  // How far the audio has been heard, which is not the same as how far the
  // saved transcript reaches. Saving rewrites the whole transcript, so it
  // happens seconds apart and jumps minutes of audio at a time, while every
  // chunk the recogniser finishes says where it got to. The range picker
  // draws this one, so its edge moves with the work. Everything that reads
  // the transcript keeps to covered, because that is what is on disk: a
  // search that started on this number would read a transcript that stops
  // short of the stretch it was asked for.
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
  const heard = $derived(
    mark.seen(path, covered, transcribing?.progress?.covered ?? null, restarted),
  );
  // Where the edge was when pause was pressed, or null while it is free to
  // move. Held rather than followed, because what the work reports after
  // the press is work nobody asked for any more.
  let stoppedAt = $state<number | null>(null);
  const shownHeard = $derived(stoppedAt ?? heard);

  // A search reads the transcript off disk, so it can only run where the
  // saved transcript reaches. It goes by covered and not by heard for that
  // reason: heard runs ahead of what has been written down, and a search
  // started on it would read a transcript that stops short of the stretch
  // it was asked for. shouldLook keeps to covered for the same reason.
  const readyToLook = $derived(duration > 0 && to > 0 && covered >= to - 0.5);
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
  // There is nothing to list until the transcript reaches the end of the
  // chosen stretch. The list waits with a row of placeholders and the info
  // mark beside the head says why, the same mark that tells what the list
  // is once there are clips in it.
  const stillWaiting = $derived(!busy && waitingOnWords && covered < to - 0.5);
  // How many rows the clip list holds open. Nothing is known about the
  // clips before the search answers, but their number is: it is the one
  // asked for. So the list stands in the shape it is about to take, with
  // the light passing over it, rather than as an empty area with a line of
  // text in it. What is going on is in the info mark at the head.
  const coming = $derived(stillWaiting || finding || starting ? count : 0);
  const waitNote = $derived.by(() => {
    if (finding || starting) {
      return `The model is reading the stretch from ${clock(from)} to ${clock(to)} and choosing ${count} moments from it. They appear here and on the range picker as soon as it answers.${leftOfWork ? ` About ${leftOfWork}.` : ""}`;
    }
    if (!stillWaiting) return "";
    const first = transcribing
      ? `The episode is being transcribed on this machine, no cloud and no cost.${leftToGo ? ` About ${leftToGo}.` : ""}`
      : "The episode is not transcribed to the end of the chosen stretch yet.";
    return `${first} Clips are found by themselves once the transcript reaches ${clock(to)}, and the window on the range picker can be moved and resized while it runs.`;
  });
  // The stretch stays with the episode while the app runs, so leaving the
  // workspace and coming back does not throw away what was chosen.
  $effect(() => {
    if (duration > 0 && to > from) chosen.keep(path, from, to);
  });
  // The whole layout is worked out by the browser, in the stylesheet at
  // the foot of this file, from the size of the window and two numbers
  // that have nothing to do with the window: the shape of the episode and
  // how tall anything above the workspace is. Neither changes while the
  // window is being dragged, so dragging it costs no JavaScript at all and
  // the workspace keeps up with the edge of the window the way a native
  // one does. Measuring a height, working out another height from it and
  // writing that back is a round trip per frame, and the parts that
  // measure each other never settle in one.
  const shape = $derived(`${source?.width || 16} / ${source?.height || 9}`);
  const ratio = $derived((source?.width || 16) / (source?.height || 9));

  // A window that cannot read the episode file, which is what happens
  // while the machine is busy, drops a seek and leaves the picture on a
  // frame that has nothing to do with the playhead. Whenever the video
  // preview says it cannot show the playhead, the frame under it is read
  // from the file instead. The engine keeps one frame per second of an
  // episode, so going back over a stretch costs nothing.
  let asking = 0;
  // Which ask the picture is from. Several are in the air whenever the
  // playhead is moved quickly, and an answer that took longer to read
  // would otherwise land after a newer one and paint over it, leaving the
  // picture on somewhere the playhead has left, for good, because nothing
  // asks again.
  const stills = new Newest();
  // The second the picture on screen is of, which is not the same as the
  // second last asked for. Going to one clip, then another, then back to
  // the first used to skip the last ask, because it matched what had been
  // asked for, and leave the second clip's frame on screen.
  let showing = -1;

  function askStill(at: number) {
    if (!source || status?.missing) return;
    const second = Math.round(Math.max(at, 0));
    if (second === showing) return;
    clearTimeout(asking);
    asking = window.setTimeout(() => {
      const ticket = stills.send();
      api
        .still(path, second, 960)
        .then((file) => {
          if (!stills.keep(ticket)) return;
          still = mediaURL(file);
          showing = second;
        })
        .catch(() => {
          // A frame that cannot be read is not worth a message. The
          // picture keeps the one it has, and the second it is of is
          // left alone so it can be asked for again.
          stills.keep(ticket);
        });
    }, 180);
  }
  const whole = $derived(from <= 0.5 && to >= duration - 0.5);
  const current = $derived(clips.find((c) => c.key === selected) ?? null);
  // A removed clip stays in the plan, it only leaves the list.
  // The clip just removed keeps its place in the list for a moment, so the
  // rows do not jump and there is somewhere to put it back from.
  const shown = $derived(clips.filter((c) => !c.rejected || c.key === removed?.key));
  // What the window lies over. A window may be drawn anywhere, so looking
  // again at material that was searched is allowed, it only asks first and
  // takes the clips it finds there with it.
  const covering = $derived(coverage.searched.some((w) => w.to > from && w.from < to));
  const inWindow = $derived(
    clips.filter((c) => !c.rejected && c.end > from && c.start < to).length,
  );
  // A clip taken out leaves the track at once. Its row stays a moment
  // longer, but that row is what became of it, not a clip.
  const marks = $derived(
    shown
      .filter((c) => c.key !== removed?.key)
      .map((c) => ({ key: c.key, start: c.start, end: c.end, rendered: !!c.rendered })),
  );
  const renderingCurrent = $derived(
    !!working && working.kind === "render" && !!current && working.result === current.plan,
  );

  // The free room changes whenever a search finishes, so it is taken again
  // with the rest of the episode.
  async function refreshCoverage() {
    try {
      coverage = await api.coverage(path, min);
    } catch {
      coverage = { searched: [], free: [] };
    }
  }

  // The window moves on to the first stretch nobody has looked at. What was
  // chosen before does not come into it: after a search that stretch is a
  // wall, and a window left on it hides the clip marks it just made.
  function moveWindowOn() {
    const next = nextWindow(coverage.free, coverage.searched, duration, firstLook);
    from = next.from;
    to = next.to;
  }

  // The window the workspace opens with: the one this episode had a moment
  // ago if it still fits, otherwise wherever a window goes by itself.
  function openWindow() {
    const kept = chosen.of(path, duration);
    if (kept) {
      from = kept.from;
      to = kept.to;
      return;
    }
    moveWindowOn();
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
  async function openOnAClip() {
    if (selected || !clips.length) return;
    let key = "";
    try {
      key = await api.chosenClip(path);
    } catch {
      // An episode that cannot say has simply never been opened.
    }
    const clip = clips.find((c) => c.key === key) ?? clips[0];
    await select(clip.key);
  }

  async function refreshClips() {
    try {
      clips = (await api.clips(path)) ?? [];
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function load() {
    try {
      removed = null;
      status = await api.episode(path);
      const first = !source && !status.missing;
      if (first) {
        source = await api.source(path);
      }
      if (!status.missing) await refreshCoverage();
      if (first && source) {
        openWindow();
      }
      await refreshClips();
      if (first) await openOnAClip();
      // A video added a moment ago: nothing searched, nothing found, and the
      // transcript still on its way. That one gets its first clips without
      // being asked.
      if (
        first &&
        shouldTranscribe({ missing: status.missing, work: status.work, busy: !!jobs.active(path) })
      ) {
        api.transcribe(path).catch((err) => (problem = errorText(err)));
      }
    } catch (err) {
      problem = errorText(err);
    }
  }

  // Putting the playhead somewhere is a jump, not a drift, so the clip
  // timeline goes there too. A view moved by hand otherwise stays where it
  // was put, which is what the crosshair in the row below it is for.
  function seekTo(t: number) {
    player?.seek(t);
    timeline?.fit(t);
  }

  // The chosen card, brought far enough into the list to be read. Walking
  // the list with the arrows otherwise chooses cards that are not on
  // screen, and the veil over each end means a card only just inside the
  // list is a card half faded away.
  function showChosen() {
    const list = document.querySelector<HTMLElement>(".pane .list");
    const card = list?.querySelector<HTMLElement>(".pick.current")?.closest("li");
    if (!list || !card) return;
    // The same veil the list fades its ends with, from the stylesheet.
    const veil = 16;
    const box = card.getBoundingClientRect();
    const view = list.getBoundingClientRect();
    if (box.top < view.top + veil) list.scrollTop -= view.top + veil - box.top;
    else if (box.bottom > view.bottom - veil) list.scrollTop += box.bottom - view.bottom + veil;
  }

  async function select(key: string) {
    selected = key;
    // The episode remembers what is being worked on, so opening it again
    // opens on the same clip. Forgetting it is no reason to say anything.
    api.chooseClip(path, key).catch(() => {});
    const clip = clips.find((c) => c.key === key);
    if (clip) player?.seek(clip.start);
    // Picking a clip puts the clip timeline back on it, the same as the
    // crosshair in the row below, even when it is the clip that was already
    // selected and the timeline was moved by hand since.
    await tick();
    showChosen();
    timeline?.fit();
  }

  // Pausing takes a moment to reach the work itself, so the button says so
  // at once rather than looking like nothing happened.
  let pausing = $state(false);

  function pauseTranscribing() {
    if (!transcribing) return;
    // The edge stops where it is, on the click. The recogniser is part way
    // through a chunk and keeps reporting until it hears the stop, so
    // without this the edge carries on for a second or two after the press
    // and the click looks like it missed.
    stoppedAt = heard;
    pausing = true;
    api.cancelJob(transcribing.id);
  }

  // Carrying on lets the edge go again. Asking for it here rather than
  // through the action keeps the two halves of the one control together.
  function carryOnTranscribing() {
    stoppedAt = null;
    api.transcribe(path).catch((err) => (problem = errorText(err)));
  }

  $effect(() => {
    if (!transcribing) pausing = false;
  });

  // Removing a clip is one click, so putting it back is one click too, for
  // as long as the list is on screen.
  let removed = $state<ClipEntry | null>(null);
  // How long the row stays behind before the list closes over it.
  const secondThoughts = 10000;
  let forgetting = 0;

  function forgetSoon() {
    clearTimeout(forgetting);
    forgetting = setTimeout(() => (removed = null), secondThoughts);
  }

  async function removeClip(clip: ClipEntry) {
    problem = "";
    try {
      const updated = await api.removeClip(path, clip.plan, clip.id, true);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
      if (selected === updated.key) selected = "";
      removed = updated;
      forgetSoon();
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function putClipBack() {
    const clip = removed;
    if (!clip) return;
    clearTimeout(forgetting);
    problem = "";
    try {
      const updated = await api.removeClip(path, clip.plan, clip.id, false);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
      removed = null;
      select(updated.key);
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function trim(clip: ClipEntry, start: number, end: number) {
    problem = "";
    try {
      const updated = await api.trimClip(path, clip.plan, clip.id, start, end);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      problem = errorText(err);
    }
  }

  // The cuts inside a clip. All three answer with the clip as it is now,
  // because the engine puts the edges on words and the timeline has to draw
  // where they landed, not where the hand let go.
  async function cut(clip: ClipEntry, from: number, to: number, toWords: boolean) {
    problem = "";
    try {
      const updated = await api.cutClip(path, clip.plan, clip.id, from, to, toWords);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function joinCut(clip: ClipEntry, at: number) {
    problem = "";
    try {
      const updated = await api.joinCut(path, clip.plan, clip.id, at);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function moveCut(
    clip: ClipEntry,
    index: number,
    from: number,
    to: number,
    toWords: boolean,
  ) {
    problem = "";
    try {
      const updated = await api.moveCut(path, clip.plan, clip.id, index, from, to, toWords);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      problem = errorText(err);
    }
  }

  async function setWord(clip: ClipEntry, start: number, text: string) {
    problem = "";
    try {
      const updated = await api.setWord(path, clip.plan, clip.id, start, text);
      // Other clips with the same word changed too.
      await refreshClips();
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      problem = errorText(err);
      throw err;
    }
  }

  async function setCrop(clip: ClipEntry, at: number, left: number) {
    problem = "";
    try {
      const updated = await api.setCrop(path, clip.plan, clip.id, at, left);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      problem = errorText(err);
    }
  }

  // The face and the size of the captions belong to the whole clip set, so
  // this is the one place they are set, right next to the clip.
  async function setCaptionStyle(font: string, size: number) {
    if (!current) return;
    problem = "";
    try {
      await api.setCaptionStyle(path, current.plan, font, size);
      await refreshClips();
    } catch (err) {
      problem = errorText(err);
    }
  }

  // The captions of the selected clip, as the render will draw them. Every
  // edit replaces the clip, so this follows along by itself.
  let captions = $state<CaptionsView | null>(null);
  let fonts = $state<CaptionFont[]>([]);

  // Where the captions sit, for every clip of every episode. Dragging the
  // box in the video preview is the same thing as typing the number in the
  // settings column, and both stay for the next video.
  let captionY = $state(captionYDefault);
  // What the captions looked like before they were put back, so the way
  // back is a way there and back again. Putting them back is one click,
  // and one click has to be undoable as easily. It is dropped the moment
  // any of the three is set to anything else, because then there is
  // nothing to return to.
  let captionsWere = $state<{ font: string; size: number; y: number } | null>(null);
  // The captions as they are now, and whether that is how they start out.
  // The mark beside the head is about the group, not about one row of it.
  const captionsNow = $derived({
    font: captions?.style.font ?? captionFontDefault,
    size: Math.round(captions?.style.chosenSize ?? captionSizeDefault),
    y: Math.round(captionY),
  });
  const captionsMoved = $derived(
    captionsNow.font !== captionFontDefault ||
      captionsNow.size !== captionSizeDefault ||
      captionsNow.y !== captionYDefault,
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
  }

  async function setCaptionsHeight(y: number) {
    problem = "";
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
      problem = errorText(err);
    }
  }

  async function resetCaptionsHeight() {
    problem = "";
    const was = captionY;
    captionY = captionYDefault;
    try {
      await api.resetCaptionsHeight(path);
      await refreshClips();
    } catch (err) {
      captionY = was;
      problem = errorText(err);
    }
  }

  async function resetCrop(clip: ClipEntry, at: number) {
    problem = "";
    try {
      const updated = await api.resetCrop(path, clip.plan, clip.id, at);
      clips = clips.map((c) => (c.key === updated.key ? updated : c));
    } catch (err) {
      problem = errorText(err);
    }
  }

  // The shortest cannot be longer than the longest, so whichever was just
  // typed pushes the other along rather than leaving a search that can find
  // nothing.
  function keepOrder(typed: "min" | "max") {
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
    api.setSearch(count, min, max).catch((err) => (problem = errorText(err)));
  }

  // Clips for a stretch that already has some are asked for again, which
  // replaces what is there. That is not something to do by accident.
  let confirmReplace = $state(false);

  // Removing what the window covers: the clips in the stretch go and the
  // model may read it again. Also not something to do by accident.
  let removingSearch = $state<{ from: number; to: number } | null>(null);

  async function removeRange(stretch: { from: number; to: number }, thenSearch: boolean) {
    removingSearch = null;
    confirmReplace = false;
    problem = "";
    try {
      await api.removeSearch(path, stretch.from, stretch.to);
      removed = null;
      await load();
      if (!clips.some((c) => c.key === selected)) selected = "";
      await refreshCoverage();
      onchange();
      // A search of its own follows when the stretch was given back in
      // order to look at it again. The window stays where it is then.
      if (thenSearch) await findClips(true);
      else openWindow();
    } catch (err) {
      problem = errorText(err);
    }
  }

  function newClips() {
    if (covering) {
      confirmReplace = true;
      return;
    }
    findClips(false);
  }

  async function findClips(replan: boolean) {
    problem = "";
    starting = true;
    try {
      const job = await api.plan(path, {
        From: whole ? 0 : from,
        To: whole ? 0 : to,
        Count: count,
        Min: min,
        Max: max,
        Replan: replan,
      });
      waiting = [...waiting, job.id];
    } catch (err) {
      problem = errorText(err);
      starting = false;
    }
  }

  // The job has reported in, so the placeholder can give way to it.
  $effect(() => {
    if (working) starting = false;
  });

  // An episode that was just added finds its first clips by itself. The
  // The first search starts the moment the transcript covers the chosen
  // stretch, so adding a video is all it takes to end up with clips. The
  // rule itself is in lib/flow.ts, with its tests.
  $effect(() => {
    if (!source || !status) return;
    const look = shouldLook({
      covered,
      to,
      plans: status.plans?.length ?? 0,
      clips: clips.length,
      busy: busy || waiting.length > 0,
      // Ever searched, by the app or by hand, in this run or an earlier
      // one. Removing the clips again does not make it a new episode.
      looked: status.looked || !!chosen.looked[path],
    });
    if (!look) return;
    chosen.looked[path] = true;
    findClips(false);
  });

  async function render(clip: ClipEntry) {
    problem = "";
    const job = await api.render(path, { Plan: clip.plan, Clips: [clip.id], Preview: false });
    waiting = [...waiting, job.id];
  }

  $effect(() => {
    const clip = current;
    if (!clip) {
      captions = null;
      return;
    }
    let dropped = false;
    api
      .captions(clip.plan, clip.id)
      .then((view) => {
        if (!dropped) captions = view;
      })
      .catch(() => {
        if (!dropped) captions = null;
      });
    return () => (dropped = true);
  });

  // The model answers a search with the whole set at once, and the clips
  // land in the plan file there and then. The list is taken again while a
  // search runs, so they show up the moment they exist instead of when the
  // job has wrapped up.


  // While the transcript grows, the covered part of the timeline grows.


  let wasTranscribing = false;
  $effect(() => {
    const now = !!transcribing;
    if (wasTranscribing && !now) {
      onchange();
      load();
      const last = jobs.list.findLast((j) => j.episode === path && j.kind === "transcribe");
      if (last?.state === "failed") problem = last.error ?? "The transcription failed.";
    }
    wasTranscribing = now;
  });

  // Finished jobs this screen started: show new clips, or the reason it failed.
  $effect(() => {
    if (!waiting.length) return;
    const finished = jobs.list.filter(
      (j) => waiting.includes(j.id) && j.state !== "queued" && j.state !== "running",
    );
    if (!finished.length) return;
    waiting = waiting.filter((id) => !finished.some((j) => j.id === id));
    starting = false;
    for (const job of finished) {
      if (job.state === "failed") problem = job.error ?? "The job failed.";
    }
    onchange();
    load().then(() => {
      const plan = finished.find((j) => j.kind === "plan" && j.state === "done")?.result;
      if (!plan) return;
      // The stretch just searched is a wall now, so the window moves on to
      // the next one nobody has looked at. It would otherwise sit on the
      // clips it just found, lying over their marks as an X-ray and
      // offering to throw them away.
      moveWindowOn();
      const first = clips.find((c) => c.plan === plan);
      if (first) select(first.key);
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

  // The arrows up and down walk the clip list, the way the arrows left and
  // right walk the episode on the clip timeline. Each step takes the next
  // clip and puts the playhead at its start, so the whole list can be gone
  // through without reaching for the pointer.
  function walkClips(event: KeyboardEvent) {
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    if (event.metaKey || event.ctrlKey || event.altKey || event.defaultPrevented) return;
    const on = document.activeElement as HTMLElement | null;
    const tag = on?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || on?.isContentEditable) return;
    // A box asking something, and an edge being nudged, take the arrows
    // for themselves.
    if (on?.getAttribute("role") === "slider") return;
    if (document.querySelector("dialog[open]")) return;
    const list = shown.filter((c) => c.key !== removed?.key);
    if (!list.length) return;
    event.preventDefault();
    const here = list.findIndex((c) => c.key === selected);
    const step = event.key === "ArrowDown" ? 1 : -1;
    // With nothing chosen, down takes the first and up the last.
    const next = here < 0 ? (step > 0 ? 0 : list.length - 1) : here + step;
    const clip = list[Math.max(0, Math.min(next, list.length - 1))];
    if (!clip) return;
    select(clip.key);
    seekTo(clip.start);
  }

  onMount(() => {
    window.addEventListener("keydown", walkClips);
    return () => window.removeEventListener("keydown", walkClips);
  });

  // One timer for as long as the workspace is open, and it decides what to
  // do each time it fires. It is not an effect: an effect that mentions
  // running work is torn down and set up again on every job event, about
  // once a second, so its timer would never fire at all.
  onMount(() => {
    const timer = setInterval(() => {
      if (isTranscribing) {
        // While the transcript grows, how far it has come is read again:
        // the track draws it, and the first search waits for it.
        api
          .episode(path)
          .then((now) => (status = now))
          .catch(() => {});
      }
      if (isFinding) {
        // The model answers with the whole set at once and the clips land
        // in the plan there and then, so they show up while the search is
        // still wrapping up.
        refreshClips();
      }
    }, 2000);
    return () => clearInterval(timer);
  });

  onMount(() => {
    installFonts()
      .then((list) => (fonts = list))
      .catch(() => {});
    api.getSettings().then((settings) => {
      count = settings.count || 12;
      min = settings.min || 20;
      max = settings.max || 30;
      captionY = settings.captionY || captionYDefault;
    });
    load();
    return () => clearTimeout(forgetting);
  });
</script>

{#snippet strip()}
  <RangeWindow
    {duration}
    covered={shownHeard}
    bind:from
    bind:to
    {marks}
    {selected}
    onmark={select}
    playhead={time}
    onseek={seekTo}
    searched={coverage.searched}
    onremove={(stretch) => (removingSearch = stretch)}
    locked={finding || starting}
    onmoved={(edge) => seekTo(edge === "to" ? Math.max(to - 1, 0) : from)}
    transcribing={isTranscribing}
    {partly}
    {leftToGo}
    holding={stoppedAt !== null}
    ontranscription={() => (transcribing ? pauseTranscribing() : carryOnTranscribing())}
  />
{/snippet}



<!-- Two numbers the stylesheet needs and cannot know: the shape of this
     episode, and how tall whatever is above the workspace came out. Both
     stay put while the window is dragged, so the whole layout below is the
     browser's own work from there on. -->
<section
  style="--ar: {ratio}; --above: {aboveH > 0 ? `calc(${Math.ceil(aboveH)}px + var(--gap))` : '0px'}"
>
  {#if confirmReplace}
    <Confirm
      title="Look at {clock(from)} to {clock(to)} again?"
      oncancel={() => (confirmReplace = false)}
    >
      <p>
        Part of this stretch has been searched already. The
        {inWindow}
        {inWindow === 1 ? "clip" : "clips"} in it are removed first, with every trim, crop and
        caption place you gave them, and the model reads the stretch as if for the first time.
        Clips outside it stay as they are, and clips you have rendered stay as files on disk.
      </p>
      {#snippet actions()}
        <button onclick={() => (confirmReplace = false)}>Cancel</button>
        <button class="danger" onclick={() => removeRange({ from, to }, true)}>Look again</button>
      {/snippet}
    </Confirm>
  {/if}

  {#if removingSearch}
    {@const stretch = removingSearch}
    <Confirm
      title="Remove the clips in {clock(stretch.from)} to {clock(stretch.to)}?"
      oncancel={() => (removingSearch = null)}
    >
      <p>
        The {inWindow}
        {inWindow === 1 ? "clip" : "clips"} in this stretch leave the list, with every trim, crop
        and caption place you gave them. Clips you have rendered stay as files on disk. Afterwards
        the stretch is free again, and the model will read it as if for the first time. What was
        searched outside it stays searched.
      </p>
      {#snippet actions()}
        <button onclick={() => (removingSearch = null)}>Cancel</button>
        <button class="danger" onclick={() => removeRange(stretch, false)}>Remove</button>
      {/snippet}
    </Confirm>
  {/if}

  <!-- Everything that stands above the workspace, together, so its height
       is one number the layout can take off the window. It is not there at
       all most of the time, and an empty row would still cost a space. -->
  {#if status?.missing || problem}
    <div class="above" bind:clientHeight={aboveH}>
      {#if status?.missing}
        <p class="error">
          The file is no longer at {path}. Move it back, or remove it from the list.
        </p>
      {/if}
      {#if problem}
        <p class="error selectable">{problem}</p>
      {/if}
    </div>
  {/if}

  {#if source}
    <div class="stage">
      <div class="settings">
        <div class="group asks">
          <div class="row grouphead">
            <h3>New clips</h3>
            <span class="ask">
              <Info label="What finding clips does">
                The model reads the stretch chosen on the <b>range picker</b> and answers with the
                moments worth clipping. A stretch it has read is marked there.
              </Info>
            </span>
          </div>
          <label class="setting">
            <span>Clips</span>
            <span class="field"
              ><input
                class="num"
                type="number"
                min="1"
                max="30"
                bind:value={count}
                onchange={saveSearch}
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
                max="180"
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
                  The face and the size for every clip of this episode. A line that will not fit is
                  drawn smaller. <b>Height</b> is how far above the bottom of the short the captions
                  sit, for every episode: drag the black box in the picture, or type it here. With
                  the captions anywhere but their usual {captionYDefault}, the mark in this corner
                  turns anticlockwise and puts them back. Once they are back it turns clockwise
                  instead and puts them where you had them, so nothing is lost by trying.
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
                  title="Put the captions back as they start out: {captionFontDefault}, {captionSizeDefault}, {captionYDefault} from the bottom"
                  aria-label="Put the captions back"
                  onclick={putCaptionsBack}
                >
                  <Icon name="back" />
                </button>
              {:else if captionsWere}
                <button
                  class="quiet glyph"
                  title="Put the captions back as you had them: {captionsWere.font}, {captionsWere.size}, {captionsWere.y} from the bottom"
                  aria-label="Put the captions back as you had them"
                  onclick={putCaptionsAsTheyWere}
                >
                  <Icon name="forward" />
                </button>
              {/if}
            </div>
            <label class="setting">
              <span>Font</span>
              <!-- The list of faces stands in the same box as the numbers,
                   with its own mark where their unit is, so every setting
                   in the column ends in the same place whatever draws it. -->
              <span class="field">
                <select
                  value={captions.style.font}
                  onchange={(e) => setCaptionStyle(e.currentTarget.value, 0)}
                >
                  {#each fonts as font (font.name)}
                    <option value={font.name}>{font.name}</option>
                  {/each}
                </select>
                <span class="unit mark"><Icon name="pick" size={12} /></span>
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
      <div class="middle">
        <Player
          bind:this={player}
          bind:paused
          bind:looping
          bind:offers
          {path}
          {source}
          clip={current}
          bind:time
          {still}
          {captions}
          onplayclip={(c) => api.clipPlayed(c.plan, c.id).catch(() => {})}
          onstill={askStill}
          oncrop={(at, left) => (current ? setCrop(current, at, left) : Promise.resolve())}
          onresetcrop={(at) => (current ? resetCrop(current, at) : Promise.resolve())}
          oncaptiony={(y) => setCaptionsHeight(y)}
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
                  out. <b>New</b> looks in the stretch chosen on the <b>range picker</b>.
                {/if}
              </Info>
            </span>
            <span class="grow"></span>
            <button
              class="new"
              class:primary={action.primary}
              onclick={action.run}
              disabled={action.off}
              title={`${action.title}${leftOfWork ? `, ${leftOfWork}` : ""}`}
              aria-haspopup={action.label === "New" && covering ? "dialog" : undefined}
            >
              <!-- How the work is going, inside the button the work was
                   started from and behind its own words. The control that
                   started it is the one that should say how it is doing,
                   and nothing is drawn across the list for it. -->
              {#if lane.job}
                {#if share < 0}
                  <!-- Work that cannot say how far along it is. -->
                  <span class="going round"><i class="sweep"></i></span>
                {:else}
                  <!-- Work that can: it fills, with a bright edge at the
                       front so where it has got to is a line and not just
                       where one shade becomes another. -->
                  <span class="going fills">
                    <i style="width: {share * 100}%"></i>
                  </span>
                {/if}
              {/if}
              <Icon name={action.icon} />
              {action.label}
            </button>
          </div>
          <div class="scroll list">
            <ClipList
              clips={shown}
              {selected}
              {coming}
              removed={removed?.key ?? ""}
              onselect={select}
              onremove={removeClip}
              onputback={putClipBack}
            />
          </div>
        </div>
      </aside>
    </div>

    <div class="detail">
      <ClipTimeline
        bind:this={timeline}
        {path}
        clip={current}
        {duration}
        {covered}
        {time}
        working={!!transcribing}
        locked={renderingCurrent}
        frame={source.fps > 0 ? 1 / source.fps : 1 / 30}
        bind:numbers
        onseek={(t) => player?.seek(t)}
        ontrim={(start, end) => (current ? trim(current, start, end) : Promise.resolve())}
        oncut={(from, to, toWords) =>
          current ? cut(current, from, to, toWords) : Promise.resolve()}
        onjoincut={(at) => (current ? joinCut(current, at) : Promise.resolve())}
        onmovecut={(index, from, to, toWords) =>
          current ? moveCut(current, index, from, to, toWords) : Promise.resolve()}
        onword={(start, text) => (current ? setWord(current, start, text) : Promise.resolve())}
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
            title="Play the clip again at its end"
          >
            <Icon name="loop" />
          </button>
        {/if}
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
        {#if current}
          <div class="titles">
            <h2 class="selectable">{current.title || current.slug}</h2>
            {#if current.reason}<p class="muted selectable">{current.reason}</p>{/if}
          </div>
          <span class="num">{clock(numbers.start)} to {clock(numbers.end)}</span>
          <span class="muted num"
            >{numbers.seconds} s, {numbers.pieces}
            {numbers.pieces === 1 ? "piece" : "pieces"}</span
          >
        {/if}
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
          {#if current.rendered}
            <button onclick={() => api.reveal(current.rendered!)}>Show in folder</button>
          {/if}
          <button class="primary" onclick={() => render(current)} disabled={!!working}
            >{renderingCurrent ? "Rendering" : current.rendered ? "Render again" : "Render"}</button
          >
        {/if}
      </div>
    </div>
  {/if}
</section>

<style>
  /* The whole workspace, worked out here rather than in JavaScript.
     Everything below is one expression the browser evaluates in the same
     pass as the resize, so dragging the window edge moves the workspace
     with it instead of a frame or two behind it.

     Two numbers come from outside: --ar, the shape of the episode, and
     --above, the height of anything standing over the workspace. Neither
     changes because the window changed. The rest is the window itself and
     the tokens from app.css. */
  section {
    /* The height the window leaves: everything but the bar at the top, the
       space above the workspace and the edge at the foot. */
    --space: calc(100dvh - var(--bar-h) - var(--gap) - var(--edge) - var(--above));
    /* And the width: everything but the rail and the two edges. */
    --stage-w: calc(100dvw - var(--rail) - 2 * var(--edge));
    /* The columns beside the picture at their smallest, with a space on
       either side of it. */
    --sides: calc(var(--settings-w) + 280px + 2 * var(--gap));
    /* Everything down the height that is neither the picture nor a track:
       four spaces, one under the picture, two around the line that parts
       the workspace from the clip up close, one over the row at the foot,
       the line itself, and the row. */
    --down: calc(4 * var(--gap) + 1px + var(--row-h));
    /* How tall the picture would be if it were as wide as it may be. */
    --widest: calc((var(--stage-w) - var(--sides)) / var(--ar));
    /* The picture takes the height first, until it is that wide. What it
       cannot use goes to the two tracks, the range picker always half the
       clip timeline, so nothing is left over at the foot of the window. */
    --wave-h: max(var(--wave-min), calc((var(--space) - var(--down) - var(--widest)) / 1.5));
    --picker-h: calc(var(--wave-h) / 2);
    --pic-h: max(200px, calc(var(--space) - var(--down) - 1.5 * var(--wave-h)));
    --pic-w: calc(var(--pic-h) * var(--ar));

    padding: var(--gap) var(--edge) var(--edge);
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
     surface of its own. The clip timeline goes down to an even number so
     the range picker, which is half of it, is whole as well. */
  @supports (height: round(down, 3px, 2px)) {
    section {
      --wave-h: max(
        var(--wave-min),
        round(down, calc((var(--space) - var(--down) - var(--widest)) / 1.5), 2px)
      );
      --pic-w: round(down, calc(var(--pic-h) * var(--ar)), 1px);
    }
  }

  /* Nothing in the column may shrink below its own height, or the player
     would reach into the timeline under it. */
  section > :global(*) {
    flex-shrink: 0;
  }


  .titles {
    min-width: 0;
  }

  /* What plays and what the timeline looks at say it with a mark rather
     than a word: the three of them are known everywhere, and a mark cannot
     change length as you use it. They are as tall as every other control in
     the row and as wide as they are tall. */
  .detail .row .glyph {
    display: flex;
    align-items: center;
    justify-content: center;
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
     video preview, and the clips. The middle one is exactly as wide as the
     picture may be, which the workspace works out, and the two beside it
     share whatever is left. That way a wider window makes the settings and
     the clip list wider instead of leaving a strip of nothing. */
  .stage {
    display: grid;
    /* The middle column is exactly as wide as the picture may be, and the
       two beside it share whatever is left, so a wider window makes them
       wider instead of leaving a strip of nothing beside the picture. */
    grid-template-columns: minmax(var(--settings-w), 1fr) var(--pic-w) minmax(280px, 1fr);
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
  }

  /* A name and a small field beside it read worse the further apart they
     are, so the settings take a little of a wider window and leave the
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

  .field,
  .setting input,
  .setting select {
    width: 116px;
  }

  .setting input,
  .setting select {
    color: var(--text);
  }

  /* Every setting in the column ends in the same place, the face along
     with the numbers. A list a person picks from is laid out by the
     system, so it is told where to put its text rather than where to put
     its box, and the mark the system would draw at its own inset is left
     out and drawn in the column the units stand in instead. Otherwise the
     face ends 17px further right than every number above it. */
  .setting select {
    appearance: none;
    text-align: right;
    text-align-last: right;
    padding-right: 29px;
  }

  /* The mark sits where a unit sits and is read from the same left edge,
     so it is one space after the face just as s is one space after a
     number. It is the colour of a unit, not of the text, because it says
     what the field is rather than what it holds. */
  .unit.mark {
    display: flex;
    align-items: center;
    height: var(--control-h);
    line-height: normal;
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

  /* Wide enough for the longest word it ever says, Cancelling, so the row
     does not move when a search starts or is stopped. */
  .new {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    min-width: 114px;
    padding: 0 10px;
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
     across it. */
  .listhead .new {
    position: relative;
    overflow: hidden;
  }

  .listhead .going {
    position: absolute;
    inset: 0;
    z-index: 0;
    border-radius: inherit;
    overflow: hidden;
    pointer-events: none;
  }

  /* What fills. The filled part is the app's colour at a wash, and its
     front edge is a line of the colour itself, so how far it has come is
     something to look at rather than a change of shade to squint at. A
     sheen passes over what is done, which is what says it is still going
     when the number has not moved for a while. */
  .listhead .fills i {
    display: block;
    position: relative;
    height: 100%;
    background: var(--accent-wash);
    box-shadow: inset -2px 0 var(--accent-hi);
    transition: width 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  }

  .listhead .fills i::after {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      transparent 0%,
      rgba(255, 255, 255, 0.14) 50%,
      transparent 100%
    );
    animation: passing 2.4s ease-in-out infinite;
  }

  /* Work that cannot say how far along it is: a band of the app's colour
     travels across the button, over and over. Brighter than the light that
     passes over a place waiting to be filled, and in the app's own colour,
     because something is happening here and only its end is unknown. */
  .listhead .round .sweep {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    width: 45%;
    background: linear-gradient(
      90deg,
      transparent 0%,
      var(--accent-wash) 30%,
      var(--accent) 60%,
      var(--accent-hi) 76%,
      transparent 100%
    );
    animation: travelling 1.6s cubic-bezier(0.45, 0, 0.55, 1) infinite;
  }

  @keyframes travelling {
    from {
      transform: translateX(-100%);
    }
    to {
      transform: translateX(223%);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .listhead .round .sweep,
    .listhead .fills i::after {
      animation: none;
    }
  }

  /* The words and the mark stay over the fill. */
  .listhead .new > :global(svg),
  .listhead .new {
    position: relative;
  }

  .listhead .going i {
    display: block;
    height: 100%;
    background: var(--accent-wash);
    transition: width 0.2s linear;
  }


  .listhead .grow {
    flex: 1;
  }

  label {
    color: var(--muted);
  }

  label input {
    color: var(--text);
  }


</style>
