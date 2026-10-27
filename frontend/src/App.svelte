<script lang="ts">
  import { onMount, tick, untrack } from "svelte";
  import {
    api,
    clock,
    onEpisodeChanged,
    onDropped,
    onAcknowledgements,
    onReportWindow,
    onUndo,
    onShowUpdates,
    onLicenceLink,
    onQuit,
    type EpisodeStatus,
    type Job,
    type Label,
    type Added,
    type TranscriptFormat,
    errorText,
  } from "./lib/api";
  import { jobs, nav, shell } from "./lib/state.svelte";
  import { railDot, standing, updates } from "./lib/updates.svelte";
  import { sortEpisodes, type EpisodeOrder } from "./lib/order";
  import Icon from "./components/Icon.svelte";
  import Bar from "./components/Bar.svelte";
  import Landing from "./components/Landing.svelte";
  import { titleBar } from "./lib/chrome.svelte";
  import Confirm from "./components/Confirm.svelte";
  import Toasts from "./components/Toasts.svelte";
  import { toasts } from "./lib/toast.svelte";
  import Busy from "./components/Busy.svelte";
  import Pick from "./components/Pick.svelte";
  import VideoMenu from "./components/VideoMenu.svelte";
  import { installFonts } from "./lib/fonts";
  import { wearColour } from "./lib/colour";
  import Episode from "./screens/Episode.svelte";
  import SettingsScreen from "./screens/Settings.svelte";
  import Acknowledgements from "./screens/Acknowledgements.svelte";
  import UpdatesScreen from "./screens/Updates.svelte";
  import Setup from "./screens/Setup.svelte";
  import { said } from "./lib/said";
  import { typing } from "./lib/keys";

  // The sidebar is a rail until the pointer reaches it, and stays open when
  // it is pinned. Open, it lies over the workspace rather than pushing it,
  // so the video preview never changes size while you reach for an episode.
  let near = $state(false);
  // The sort list and a video's menu lie over the app, outside the
  // sidebar, so moving onto one is leaving the sidebar as far as the
  // pointer can tell, and so is the keyboard going into it. The sidebar
  // stays open while one of its own is. They are counted, not a yes or
  // no: a video's menu opened while another's is open may hear itself open
  // before the other hears itself close, and a yes and no let the second
  // close the sidebar under the menu that was opening, which Tim saw as
  // the menu squeezed into the rail.
  let holding = $state(false);
  let holds = 0;
  const open = $derived(shell.pinned || near || holding);
  let aside = $state<HTMLElement>();

  // What opened or closed the sidebar last, for the log, so a report
  // says what did it.
  let why = "";
  function reach(to: boolean, cause: string) {
    if (near === to) return;
    why = cause;
    near = to;
  }

  // What the sidebar knew of the pointer is out of date once its list
  // closes: while the list was open the pointer was on the list, not on
  // the sidebar. Falling back on that closed the sidebar the moment Escape
  // or a click shut the list. So it stays open and asks the next move of
  // the pointer where it really is, and only a move outside closes it.
  let asking = false;

  function hold(isOpen: boolean) {
    holds = Math.max(0, holds + (isOpen ? 1 : -1));
    holding = holds > 0;
    if (!holding) {
      reach(true, "its list closed");
      asking = true;
    }
  }

  // Another window taking the focus takes the pointer with it, the report
  // window above all, which Bugs opens. The app's window hears nothing of
  // the pointer going, so the sidebar it was on stayed open until Tim
  // clicked back in and moved. It closes when the app's window loses the
  // focus, and the next move of the pointer, once it is back, says where
  // the pointer is.
  function away() {
    if (holding) return;
    reach(false, "the app lost the focus");
    asking = true;
  }

  // Only the keyboard going into the sidebar opens it. Focus that comes
  // back to the button last clicked, when the app's window has the focus
  // again, is not the person reaching for the sidebar.
  function focusIn(e: FocusEvent) {
    if ((e.target as Element).matches?.(":focus-visible")) reach(true, "the keyboard went into it");
  }

  // The sidebar opens when the hand moves the pointer onto it, and for
  // nothing else. Tim had the pointer on the sidebar, swiped up for
  // Mission Control, and saw it open and close over and over until he
  // clicked back into the app. WebKit hands on every mouse entered and
  // exited macOS sends as a move of the pointer, and a sidebar that opens
  // and closes under a still pointer can have the system send another, so
  // the sidebar answered itself. Those moves move nothing: what the hand
  // does on the trackpad or the mouse is in a move's movement, and theirs
  // is none. Only the very first move is taken without it, since the
  // pointer had no place before it to have moved from. And the sidebar
  // only answers the pointer while the app is in front, the way the Mac's
  // own windows do: a move while another app is in front is not taken
  // in at all, or the sidebar took the pointer for already there and
  // stayed a rail when the app came back to the front. WebKit hands the
  // app such moves only on a Mac that shows scroll bars always, see
  // trackingAreaOptions in WebKit's WebViewImpl.mm. Where the pointer was
  // at its last real move, so it opens on the way in and not again while
  // it stays.
  let inside = false;
  let moved = false;
  function pointerMoved(e: PointerEvent) {
    if (moved && !e.movementX && !e.movementY) return;
    if (!document.hasFocus()) return;
    moved = true;
    const now = !!aside?.contains(e.target as Node);
    const came = now && !inside;
    inside = now;
    if (holding) return;
    if (asking) {
      asking = false;
      reach(now, now ? "the pointer is on it" : "the pointer is away from it");
    } else if (came) reach(true, "the pointer moved onto it");
  }

  function pointerLeft() {
    inside = false;
    reach(false, "the pointer left it");
  }

  // Escape closes one thing at a time, the way a menu on the Mac does: the
  // list first, and the sidebar that hovering opened only on the next press.
  // Read before the list hears it, so the press that closes the list is not
  // also the press that closes the sidebar. A field being typed in is a
  // layer too: the press lets go of it, see escapeLetsGo.
  function escape(e: KeyboardEvent) {
    if (e.key !== "Escape" || holding || shell.pinned || !near || removing || typing()) return;
    reach(false, "Escape");
    asking = false;
  }

  // The button does what it says at once: open, it closes the sidebar, even
  // with the pointer still on it. Hover opens it again once the pointer has
  // left and come back.
  function toggleBar() {
    why = open ? "its button closed it" : "its button pinned it";
    shell.set(!open);
    if (open) near = false;
  }

  // Every time the sidebar opens or closes, and what did it, so a report
  // shows a sidebar that went back and forth and why.
  let wasOpen: boolean | null = null;
  $effect(() => {
    const now = open;
    untrack(() => {
      if (wasOpen !== null && wasOpen !== now) said(`sidebar: ${now ? "opened" : "closed"}, ${why || "pinned"}`);
      wasOpen = now;
      why = "";
    });
  });

  let episodes = $state<EpisodeStatus[]>([]);
  // A label being made in a video's menu, drawn in its row and the title
  // bar while it is typed, before it is saved. Null is a label being taken
  // away. A read of the library while the words are typed does not put
  // the old label back, because what is drawn is the draft until the
  // answer to saving it is in.
  let drafts = $state<Record<string, Label | null>>({});
  // Which draft of a video is the latest, by number. A save lets go of the
  // draft it drew only if no newer one came since. The draft itself cannot
  // tell: a label put into the state comes back as a proxy of itself, so
  // it is never the label it was, and the row kept drawing a label the Go
  // side did not have, after a save that failed above all.
  const made: Record<string, number> = {};
  function draft(source: string, label: Label | null): number {
    drafts[source] = label;
    made[source] = (made[source] ?? 0) + 1;
    return made[source];
  }
  const labelOf = (ep: EpisodeStatus): Label | null =>
    ep.source in drafts ? drafts[ep.source] : (ep.label ?? null);
  // The rows of the sidebar, which a video's menu hangs under.
  const rows = $state<Record<string, HTMLElement>>({});

  // How many labels of a video are on their way to the Go side, and the
  // last of them. They go one after another, in the order they were made,
  // so the label the Go side keeps is the last one made: a colour clicked
  // as the field is left is two saves, and two calls at once may land in
  // either order.
  const saving: Record<string, number> = {};
  const queued: Record<string, Promise<void>> = {};

  function saveLabel(ep: EpisodeStatus, label: Label | null) {
    const mine = draft(ep.source, label);
    saving[ep.source] = (saving[ep.source] ?? 0) + 1;
    const save = async () => {
      try {
        const kept = await api.setLabel(ep.source, label?.text ?? "", label?.colour ?? "");
        episodes = episodes.map((e) => (e.source === ep.source ? { ...e, label: kept ?? undefined } : e));
      } catch (err) {
        oops(errorText(err));
      }
      if (--saving[ep.source] === 0) {
        delete saving[ep.source];
        delete queued[ep.source];
      }
      // Kept or not, the row now draws what the Go side keeps, unless a
      // newer draft is on screen.
      if (made[ep.source] === mine) delete drafts[ep.source];
    };
    queued[ep.source] = (queued[ep.source] ?? Promise.resolve()).then(save);
  }

  // How far Transcribe whole video has come is shown on the range picker
  // and by nothing else, Tim's calls on #458: the video's dot
  // pulses while it runs, the way it does for all work. The store keeps
  // that true from the frame of the click: a start on its way pulses it,
  // and work a stop was pressed for does not, unless other work runs.
  const live = (j: Job) => j.state === "running" || j.state === "queued";
  function working(path: string): boolean {
    return (
      jobs.isStarting(path, "whole") ||
      jobs.isStarting(path, "clips") ||
      jobs.list.some((j) => j.episode === path && live(j) && !jobs.stops(j))
    );
  }

  // Whether a search or a clip made by hand of the video runs or is on
  // its way, so Transcribe video waits until it is done: the two go one at
  // a time, Tim's report of 26 October, 12.27.
  function clipsBusy(path: string): boolean {
    return (
      jobs.isStarting(path, "clips") ||
      jobs.list.some((j) => j.episode === path && (j.kind === "search" || j.kind === "clip") && live(j) && !jobs.stops(j))
    );
  }

  // Why a transcription failed, said in a toast with Report, whether the
  // job failed as it ran or was refused as it was asked for.
  function sayFailed(j: Job) {
    const said = j.error?.trim() ?? "";
    const why = said ? said[0].toUpperCase() + said.slice(1) : "The transcription failed, and no reason was given.";
    toasts.say("transcribe whole video", why, { video: j.episode, report: true });
  }

  // Transcribe video in a video's menu.
  // The job it answers with is taken at once, so the row says Transcribing
  // without waiting for the job's first news.
  async function transcribe(ep: EpisodeStatus) {
    said(`video menu: transcribe whole video`, { video: ep.source });
    jobs.press(ep.source, "whole");
    try {
      const job = await api.transcribe(ep.source);
      jobs.apply(job);
      // Refused, while the app relaunches or the video is being removed,
      // it never ran, so the effect below never sees it fail. Found by
      // the review of #458.
      if (job.state === "failed") sayFailed(job);
    } catch (err) {
      toasts.say("transcribe whole video", errorText(err), { video: ep.source });
    } finally {
      jobs.answered(ep.source, "whole");
    }
  }

  // A transcription that fails says why in a toast, the way a search does,
  // and its row says Transcribe video again. One seen failed only as the
  // app opened has said why already. Only the state and id are read, see
  // the rule on watching jobs.
  const transcribingSeen = new Set<string>();
  $effect(() => {
    for (const j of jobs.list) {
      if (j.kind !== "transcribe") continue;
      if (live(j)) transcribingSeen.add(j.id);
      else if (transcribingSeen.delete(j.id) && j.state === "failed") sayFailed(j);
    }
  });

  // Export transcript: the save box is the system's, so nothing is said
  // here but a failure.
  async function exportTranscript(ep: EpisodeStatus, format: TranscriptFormat) {
    try {
      const saved = await api.exportTranscript(ep.source, format);
      said(`video menu: export transcript as ${format}, ${saved ? "saved" : "cancelled"}`, { video: ep.source });
    } catch (err) {
      toasts.say("export transcript", errorText(err), { video: ep.source });
    }
  }

  // A menu closed: what it drew and did not save is let go, unless a save
  // is still on its way, whose answer lets go of it.
  function settle(source: string) {
    if (!(source in saving)) delete drafts[source];
  }
  // Whether the library has been read once.
  let libraryRead = $state(false);
  // The list as the sidebar shows it. The trigger is the sort mark Apple's
  // own apps put on a sort menu, and says what it does rather than which
  // order is on: a clock alone read as anything but sorting. The rows say
  // which, with a tick on the one that is on.
  const listed = $derived(sortEpisodes(episodes, shell.order));
  let list = $state<HTMLUListElement>();
  const orders = [
    { value: "added", label: "Added", icon: "sort-added" },
    { value: "name", label: "Name", icon: "sort-name" },
  ];
  // The workspaces of the episodes opened last are kept, so going back to
  // one is one frame: it stands where it stood, with everything it had on
  // it. Only the one in front is awake. The others are asleep, out of the
  // page, see shelf, and hold their page and the file's index and nothing
  // costly, no decoder and no sound card, see sleeping in Player.svelte. Tim found
  // every episode picked took half a second to be built again from
  // nothing, a chain of questions to the Go side, the file's index and
  // ffmpeg starting, all of it thrown away again on leaving.
  //
  // An episode not kept is built behind the one on screen, and the two
  // change places in one frame once the new one has its episode on it, see
  // ready in Episode.svelte, or after a second at most, so a slow disk
  // never keeps the old episode up. It was torn down and built again in
  // front of the person, empty first and then a part at a time, and Tim saw
  // that as a flicker on every episode picked.
  const KEPT = 10;
  const asked = $derived(nav.view.name === "episode" ? nav.view.path : "");
  // The one in front, and the ones kept, the last opened first.
  let onScreen = $state("");
  let kept = $state<string[]>([]);
  // The kept ones that have had their episode on them once.
  let readied = $state<Record<string, true>>({});
  $effect(() => {
    const coming = asked;
    if (!coming) return;
    // So the app opens on it again as it starts, see Opened in chosen.go.
    api.openEpisode(coming).catch(() => {});
    untrack(() => {
      kept = [coming, ...kept.filter((p) => p !== coming)].slice(0, KEPT);
      for (const p of Object.keys(readied)) if (!kept.includes(p)) delete readied[p];
      if (warming && !kept.includes(warming)) warming = "";
      if (readied[coming]) onScreen = coming;
    });
  });
  // As the app starts, it opens on the episode that was open when it was
  // left, the way a Mac app comes back with the document it had open, and
  // the workspaces of the episodes opened before it are built, so the
  // first one picked is on screen in one frame too. One at a time, behind
  // whatever is on screen, and only while no episode that was picked is
  // being built: each is built awake, so it has its frame on its canvas,
  // and goes to sleep once it is ready, or after three seconds.
  let toWarm = $state<string[]>([]);
  let warming = $state("");
  // Whether the app knows yet where it was left. Until it does, the page
  // for no episode stays blank rather than show for a moment.
  let placed = $state(false);
  onMount(() => {
    api
      .opened(KEPT)
      .then((list) => {
        toWarm = list;
        if (list.length && nav.view.name === "empty") nav.go({ name: "episode", path: list[0] });
      })
      .catch(() => {})
      .finally(() => (placed = true));
  });
  $effect(() => {
    if (warming || (asked && asked !== onScreen) || kept.length >= KEPT) return;
    const known = new Set(episodes.map((e) => e.source));
    const next = toWarm.find((p) => known.has(p) && !kept.includes(p));
    if (!next) return;
    untrack(() => {
      toWarm = toWarm.filter((p) => p !== next);
      warming = next;
      kept = [...kept, next];
    });
  });
  $effect(() => {
    const now = warming;
    if (!now) return;
    const late = setTimeout(() => {
      if (warming === now) warming = "";
    }, 3000);
    return () => clearTimeout(late);
  });
  // Which workspace is asked for, in front and being built, for the app's
  // log, see lib/said.ts.
  const base = (p: string) => p.split("/").pop() ?? "";
  $effect(() => {
    said(`app: ${asked ? `${base(asked)} asked for` : "no video asked for"}, ${onScreen ? `${base(onScreen)} in front` : "nothing in front"}${warming ? `, ${base(warming)} being built` : ""}, ${untrack(() => kept.length)} kept`, { level: "info", video: asked || undefined });
  });
  // An episode removed from the library takes its workspace with it.
  $effect(() => {
    const known = new Set(episodes.map((e) => e.source));
    untrack(() => {
      if (kept.some((p) => !known.has(p) && p !== asked)) kept = kept.filter((p) => known.has(p) || p === asked);
    });
  });
  $effect(() => {
    const coming = asked;
    if (!coming || coming === onScreen) return;
    const late = setTimeout(() => {
      if (asked === coming) onScreen = coming;
    }, 1000);
    return () => clearTimeout(late);
  });
  // A kept workspace that sleeps is taken out of the page and put back as
  // it was. Out of the page, it costs no layout when the app is resized,
  // and nothing that looks the workspace up by the document finds it. It
  // keeps everything it holds, its canvases with what is drawn on them
  // among it. Its slot stays where it was for it to go back into.
  function shelf(node: HTMLElement, away: boolean) {
    const home = node.parentElement!;
    let kept: number | null = null;
    const put = (now: boolean) => {
      if (now && node.isConnected) {
        // Taken out of the page, a list forgets how far it was scrolled.
        kept = node.querySelector<HTMLElement>(".pane .list")?.scrollTop ?? null;
        node.remove();
      } else if (!now && !node.isConnected) {
        home.append(node);
        const list = node.querySelector<HTMLElement>(".pane .list");
        if (list && kept !== null) list.scrollTop = kept;
      }
    };
    put(away);
    return { update: put };
  }

  // The one asked for comes first in the document, so whatever looks the
  // workspace up finds the episode that was picked, and the one going is
  // drawn over it until they change places.
  const workspaces = $derived(asked ? [asked, ...kept.filter((p) => p !== asked)] : kept);

  // What is on screen, said once, in the bar at the top. The
  // screens do not write their own name any more.
  const title = $derived.by(() => {
    if (nav.view.name === "episode") {
      const path = nav.view.path;
      return episodes.find((ep) => ep.source === path)?.name ?? "";
    }
    if (nav.view.name === "settings") return "Settings";
    if (nav.view.name === "updates") return "Updates";
    if (nav.view.name === "acknowledgements") return "Acknowledgements";
    return "Frame Fairy";
  });

  // The label of the video on screen, beside its name in the bar.
  const titleLabel = $derived.by(() => {
    if (nav.view.name !== "episode") return null;
    const path = nav.view.path;
    const ep = episodes.find((e) => e.source === path);
    return ep ? labelOf(ep) : null;
  });

  // Where the app's updates stand, which the rail marks on Updates with
  // the dot the Updates page's own mark makes, issue #406. See
  // docs/UPDATES.md.
  const update = $derived(updates.shown);
  const updateDot = $derived(railDot(update));
  const updateStanding = $derived(standing(update));
  // Whether the report window is open, which lights Bugs.
  let reporting = $state(false);
  const updateReady = $derived(update?.phase === "ready");
  // What went wrong with the library or with files given to the app,
  // which has no row of its own: a toast, issue #327. Each time is a
  // toast of its own, so two files dropped one after the other are said
  // both.
  const oops = (text: string) => toasts.say("app", text);

  // The first run. A new copy of the app cannot transcribe without a
  // speech model and cannot find clips until somebody has said how, so
  // until both are answered the setup is the whole app: no sidebar, no
  // workspace, nothing to press that would not work.
  //
  // Null until the Go side has answered, so the workspace never flashes up
  // for a moment before the setup covers it.
  let settingUp = $state<boolean | null>(null);

  // Which read of the library is the latest. Reads overlap, the app coming
  // to the front as Add's box closes among them, and one that answers
  // after a later one holds an older library: a video just added, missing
  // from it, would have closed again as it opened.
  let reads = 0;
  async function refresh() {
    const mine = ++reads;
    try {
      const now = await api.library();
      if (mine !== reads) return;
      closeLeft(now);
      episodes = now;
    } catch (err) {
      if (mine !== reads) return;
      oops(errorText(err));
    }
    libraryRead = true;
  }

  // A video that left the library while it was on screen, its folder
  // deleted outside the app, closes the way one removed with Remove does:
  // the video opened before it comes back, or the page for none. Only one
  // the library had and no longer has: a video just added is asked for
  // before the read that has it answers.
  function closeLeft(now: EpisodeStatus[]) {
    const v = nav.view;
    if (v.name !== "episode") return;
    const has = (list: EpisodeStatus[], path: string) => list.some((e) => e.source === path);
    if (has(now, v.path) || !has(episodes, v.path)) return;
    const before = kept.find((p) => p !== v.path && has(now, p));
    said("app: the video on screen left the library", { level: "info", video: v.path });
    nav.go(before ? { name: "episode", path: before } : { name: "empty" });
  }

  // Which files the app takes as videos, said where Add is. The Go side
  // has the one list, so the words follow it, see VideoNames.
  let videoNames = $state("");

  async function add() {
    try {
      await took(await api.addEpisodes());
    } catch (err) {
      oops(errorText(err));
      await refresh();
    }
  }

  // What came of files given to the app, with Add or a drop, the same
  // either way: the list is read again and the video to open opens, from
  // whatever page the app was on. Why a file cannot be added is a toast,
  // and nothing given, Add cancelled, says nothing.
  async function took(a: Added) {
    if (a.error) oops(a.error);
    await refresh();
    if (!a.open) return;
    said("app: a video added opens", { level: "info", video: a.open });
    nav.go({ name: "episode", path: a.open });
    // Added last, it is at the foot of a list that may be longer than
    // the sidebar, so it is brought into view to be seen arriving.
    await tick();
    list?.querySelector(".episode.current")?.scrollIntoView({ block: "nearest" });
  }

  // A video dropped anywhere on the app is added the way Add adds one.
  // The whole app is where it may land, marked for the runtime with
  // data-file-drop-target, once the setup is done: before that there is
  // no list to add to. Not while a box asks first or the app asks about
  // quitting either, so nothing is added under a question. The Go side
  // adds it and says what came of it, see drop.go.
  $effect(() => {
    const open = settingUp === false && !leaving && shell.asking === 0;
    document.documentElement.toggleAttribute("data-file-drop-target", open);
  });

  // Cmd+Q heard: the app dims at once and says what happens next. See
  // quit.go for why the first press only asks while work runs.
  let leaving = $state<"ask" | "going" | null>(null);
  const quitKey = /Mac/.test(navigator.userAgent) ? "⌘Q" : "Ctrl+Q";
  // The question taken away, with Escape or a click. The Go side hears it
  // too, so the next Cmd+Q asks again: the question went from the screen
  // and the press still counted, so the next one quit on what looked like
  // the first press.
  function stayOpen() {
    if (leaving !== "ask") return;
    leaving = null;
    api.stayOpen().catch(() => {});
  }

  // Removing an episode belongs to the episode in the list, not to the
  // workspace, which is about the clips.
  let removing = $state<EpisodeStatus | null>(null);

  // Which answer is being carried out. Removing waits for the episode's
  // work to stop, which takes a moment while a search runs, so the button
  // says so at once and the box takes no second click.
  let removingHow = $state<"keep" | "delete" | null>(null);

  async function remove(ep: EpisodeStatus, deleteWork: boolean) {
    if (removingHow) return;
    removingHow = deleteWork ? "delete" : "keep";
    try {
      await api.removeEpisode(ep.source, deleteWork);
      // The workspace goes first. Left open, it went on reading the
      // episode it showed, which was no longer in the library, and every
      // read came back as "file does not exist".
      // The episode opened before it comes back, the way closing a tab
      // shows the one before it.
      if (nav.view.name === "episode" && nav.view.path === ep.source) {
        const before = kept.find((p) => p !== ep.source);
        nav.go(before ? { name: "episode", path: before } : { name: "empty" });
      }
      removing = null;
      await refresh();
    } catch (err) {
      removing = null;
      oops(errorText(err));
    } finally {
      removingHow = null;
    }
  }

  function selected(path: string): boolean {
    const v = nav.view;
    return v.name === "episode" && v.path === path;
  }

  function summary(ep: EpisodeStatus): string {
    if (ep.missing) return "File not found";
    const parts: string[] = [];
    if (ep.transcribed) parts.push("Transcribed");
    else if (ep.transcriptStale) parts.push("Changed since transcription");
    else if (ep.heard?.length === 1 && ep.covered > 0) parts.push(`Transcribed to ${clock(ep.covered)}`);
    else if (ep.heard?.length) parts.push("Transcribed in parts");
    else parts.push("Not transcribed");
    const plans = ep.plans?.length ?? 0;
    if (plans) parts.push(plans === 1 ? "1 plan" : `${plans} plans`);
    if (ep.rendered) parts.push(`${ep.rendered} rendered`);
    return parts.join(", ");
  }

  // The folder beside the episode that holds everything the app made of
  // it. The remove box names it, so it is clear what is kept or deleted
  // and where to find it later.
  function workFolder(path: string): string {
    const base = path.split(/[\\/]/).pop() ?? path;
    return `${base.replace(/\.[^.]+$/, "")}.framefairy`;
  }

  function dotFor(ep: EpisodeStatus): string {
    if (working(ep.source)) return "busy";
    if (ep.missing) return "err";
    // A search that stopped says so here as well as in its row, so it can
    // be found from the sidebar: stopped or cut off in the colour of a
    // warning, failed in the colour of an error. One set aside has no row,
    // so it says nothing here either, issue #436.
    const found = jobs.search(ep.source);
    const search = found?.aside ? undefined : found;
    if (search?.state === "failed") return "err";
    if (search?.state === "interrupted") return "warn";
    if (ep.transcriptStale) return "warn";
    if (ep.plans?.length) return "ok";
    return "";
  }

  // A pointer that lands anywhere else takes the keyboard with it. Clicking
  // the arrows of a number field leaves that field holding the keyboard, and
  // then the space bar types into it instead of playing the clip, however
  // much else you did in between.
  function handOverFocus(event: PointerEvent) {
    const on = document.activeElement as HTMLElement | null;
    if (!on || on === document.body) return;
    if (event.target instanceof Node && on.contains(event.target)) return;
    on.blur();
  }

  // A button pressed with the pointer does not keep the keyboard, the way
  // a button on the Mac does not. WebKit on the Mac gives it none, and
  // Chromium, on Windows and in the preview, does, and then Enter, which
  // opens the word at the playhead, pressed the button again instead: a
  // click on Highlight, then Enter, switched the highlight back on. A
  // button pressed with the keyboard keeps it, and so does one that opens
  // a list, which hands the keyboard on to the list itself.
  function letButtonGo(event: MouseEvent) {
    if (event.detail === 0) return;
    const button = (event.target as Element | null)?.closest?.("button");
    if (!button || button !== document.activeElement) return;
    if (button.hasAttribute("aria-haspopup")) return;
    button.blur();
  }


  // A key from a link that waits goes to the settings, once the setup of a
  // new copy is out of the way.
  function showLicenceLink() {
    api
      .licence()
      .then((l) => {
        if (l.waiting && settingUp === false) nav.go({ name: "settings" });
      })
      .catch(() => {});
  }

  onMount(() => {
    jobs.start();
    installFonts().catch(() => {});
    const noChrome = titleBar.follow("app");
    // The app wears the colour it was given before anything is drawn in it.
    api
      .getSettings()
      .then((s) => wearColour(s.appColour))
      .catch(() => {});
    api
      .setup()
      .then((s) => (settingUp = !s.chosen || !s.hasSpeech))
      // A machine that cannot answer is not a machine to hold in a setup
      // screen it can never leave.
      .catch(() => (settingUp = false))
      .then(showLicenceLink);
    refresh();
    api
      .videoNames()
      .then((names) => (videoNames = names))
      .catch(() => {});
    const off = onEpisodeChanged(() => refresh());
    // The library is read again whenever the app comes back to the front,
    // which is where a person comes back from who deleted a video's folder
    // in Finder: the Go side lets that video go as it answers, see
    // letGoOfRemoved in library.go. The rest of the library is the app's
    // own, so nothing else can have changed it while the app was behind.
    const comeBack = () => refresh();
    window.addEventListener("focus", comeBack);
    const noDropped = onDropped(took);
    const noAcknowledgements = onAcknowledgements(() => nav.go({ name: "acknowledgements" }));
    // Undo and Redo in the Edit menu go to the workspace in front, which
    // hands them to a field being typed in. Elsewhere, in the settings,
    // no workspace is in front, so the field's own undo answers here, or
    // Cmd+Z in it would do nothing.
    const noUndo = onUndo((what) => {
      if (nav.view.name !== "episode" && typing()) document.execCommand(what);
    });
    // Bugs is lit while the report window is open, see bugs.go.
    const noReport = onReportWindow((open) => (reporting = open));
    api
      .reportWindowOpen()
      .then((open) => (reporting = open))
      .catch(() => {});
    // Where updates stand, heard from the start for the dot on Updates and
    // kept for the Updates page, see lib/updates.svelte.ts.
    const noUpdates = updates.listen();
    // Check for Updates in the app menu shows the answer where it is kept.
    const noShowUpdates = onShowUpdates(() => nav.go({ name: "updates" }));
    // A framefairy://unlock link, from the mail a licence came in, opens
    // the settings, where its key waits in the Licence row. One that
    // opened the app came before this page could listen, so it is asked
    // for as well.
    const noLicenceLink = onLicenceLink(() => nav.go({ name: "settings" }));
    // The question lasts as long as the Go side waits for the second
    // press, quitAgain in quit.go.
    let asked: ReturnType<typeof setTimeout> | undefined;
    const dismiss = (e: KeyboardEvent) => {
      if (e.key === "Escape") stayOpen();
    };
    window.addEventListener("keydown", dismiss);
    const noQuit = onQuit((what) => {
      clearTimeout(asked);
      leaving = what;
      if (what === "ask") asked = setTimeout(() => (leaving = null), 3000);
    });
    window.addEventListener("pointerdown", handOverFocus);
    window.addEventListener("click", letButtonGo);
    return () => {
      clearTimeout(asked);
      window.removeEventListener("keydown", dismiss);
      noQuit();
      window.removeEventListener("pointerdown", handOverFocus);
      window.removeEventListener("click", letButtonGo);
      noChrome();
      noAcknowledgements();
      noReport();
      noUndo();
      noUpdates();
      noShowUpdates();
      noLicenceLink();
      noDropped();
      window.removeEventListener("focus", comeBack);
      off();
    };
  });
</script>

<svelte:window onpointermove={pointerMoved} onkeydowncapture={escape} onblur={away} />

<div class="shell" style={titleBar.style}>
  <Bar {title} label={titleLabel} />

  <div class="body" class:alone={settingUp !== false}>
  {#if settingUp === null}
    <!-- Nothing, for the moment it takes to ask. The bar is already there,
         so the app is not blank. -->
  {:else if settingUp}
    <Setup
      ondone={() => {
        settingUp = false;
        refresh();
        showLicenceLink();
      }}
    />
  {:else}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <aside
    bind:this={aside}
    class:open
    onpointerleave={pointerLeft}
    onfocusin={focusIn}
    onfocusout={() => reach(false, "the keyboard left it")}
  >
    <div class="head row">
      <button
        class="quiet glyph"
        onclick={toggleBar}
        aria-pressed={open}
        title={open ? "Close the sidebar" : "Keep the sidebar open"}
      >
        <Icon name="sidebar" />
      </button>
      <h2>Videos</h2>
      {#if episodes.length > 1}
        <span class="order">
          <Pick
            value={shell.order}
            options={orders}
            label="Sort videos by"
            title={shell.order === "name" ? "Sort videos, now by name" : "Sort videos, now by when they were added"}
            align="right"
            face="icon"
            icon="sort"
            onopenchange={hold}
            onpick={(v) => shell.sortBy(v as EpisodeOrder)}
          />
        </span>
      {/if}
    </div>
    <ul class="scroll" bind:this={list}>
      {#each listed as ep (ep.source)}
        {@const label = labelOf(ep)}
        <li bind:this={rows[ep.source]}>
          <button
            class="episode"
            class:current={selected(ep.source)}
            onclick={() => nav.go({ name: "episode", path: ep.source })}
          >
            <span class="dot {dotFor(ep)}"></span>
            <span class="line" title={label ? `${label.text}. ${summary(ep)}` : summary(ep)}>
              <span class="name">{ep.name}</span>
              {#if label}<span class="tag" style="--tag: {label.colour}">{label.text}</span>{/if}
            </span>
          </button>
          <span class="tools">
            <VideoMenu
              {ep}
              shown={open}
              anchor={rows[ep.source]}
              worn={episodes.filter((e) => e.source !== ep.source).map((e) => labelOf(e)?.colour ?? "")}
              onopenchange={(isOpen) => {
                hold(isOpen);
                if (!isOpen) settle(ep.source);
              }}
              drawn={label}
              ondraft={(l) => draft(ep.source, l)}
              onsave={(l) => saveLabel(ep, l)}
              onreveal={() => api.reveal(ep.source)}
              onremove={() => (removing = ep)}
              work={jobs.transcription(ep.source)}
              stopping={jobs.isStopping(ep.source, "whole")}
              clipsBusy={clipsBusy(ep.source)}
              ontranscribe={() => transcribe(ep)}
              onstoptranscribe={() => {
                const work = jobs.transcribing(ep.source);
                if (!work) return;
                jobs.stop(ep.source, "whole");
                said("video menu: stop transcribing", { video: ep.source });
                void api.cancelJob(work.id);
              }}
              onexport={(format) => exportTranscript(ep, format)}
            />
          </span>
        </li>
      {/each}
    </ul>
    <div class="foot">
      <button
        class="quiet nav"
        onclick={add}
        title={videoNames ? `Add a video. Any ${videoNames} works` : "Add a video"}
      >
        <Icon name="plus" />
        <span class="label">Add</span>
      </button>
      <button
        class="quiet nav"
        class:current={nav.view.name === "settings"}
        onclick={() => nav.go({ name: "settings" })}
        title="Settings"
      >
        <Icon name="sliders" />
        <span class="label">Settings</span>
      </button>
      <!-- Report a Problem, in a window of its own, one at a time, so a
           person can talk about a problem while going through it here.
           Lit while that window is open. Plan row 2.244. -->
      <button class="quiet nav bugs" class:current={reporting} onclick={() => api.openReport("bugs").catch(() => {})} title="Report a problem">
        <Icon name="report" />
        <span class="label">Bugs</span>
      </button>
      <!-- The build that is running, which is also the way to its updates.
           It says which build this is, so a report from testing always
           names what was tested. -->
      <button
        class="quiet nav"
        class:current={nav.view.name === "updates"}
        onclick={() => nav.go({ name: "updates" })}
        title={updateReady
          ? `Updates. ${update?.nextName || update?.next} is ready, relaunch to finish updating`
          : updateDot
            ? `Updates. This is ${update?.version ?? "the app"}. ${updateStanding.head}. ${updateStanding.more}`.trim()
            : `Updates. This is ${update?.version ?? "the app"}`}
      >
        <span class="mark">
          <Icon name="update" />
          <!-- Whatever the Updates page marks, the rail marks too, with a
               dot in the place of the dot for work in hand, in the colour
               the page wears it in: work on a newer build pulses, a build
               ready stands still in the accent, and a pull request closed
               or in conflict, or a check that did not get through, stands
               still in the colour of its triangle. Up to date is no dot. -->
          {#if updateDot}<span class="dot {updateDot}"></span>{/if}
        </span>
        <span class="label">Updates</span>
        <span class="label muted small num build">{update?.version ?? ""}</span>
      </button>
    </div>
  </aside>

  <main>
    <!-- The kept workspaces stay while another screen is open, out of the
         way, so coming back from the settings is one frame as well. -->
    {#if workspaces.length}
      <div class="workspaces" class:gone={!asked}>
        {#each workspaces as path (path)}
          <div class="slot">
            <div
              class="workspace"
              data-path={path}
              class:behind={path !== onScreen}
              class:front={path === onScreen}
              inert={path !== onScreen || path !== asked}
              use:shelf={path !== onScreen && path !== asked && path !== warming}
            >
              <Episode
                {path}
                away={path !== asked && path !== warming}
                inLibrary={episodes.find((e) => e.source === path)}
                onchange={refresh}
                onready={() => {
                  readied[path] = true;
                  if (path === warming) warming = "";
                  if (path === asked) onScreen = path;
                }}
              />
            </div>
          </div>
        {/each}
      </div>
    {/if}
    {#if nav.view.name === "episode"}
      <!-- The workspaces above. -->
    {:else if nav.view.name === "updates"}
      <UpdatesScreen />
    {:else if nav.view.name === "settings"}
      <SettingsScreen />
    {:else if nav.view.name === "acknowledgements"}
      <Acknowledgements />
    {:else if !placed || !libraryRead}
      <!-- Where the app was left is on its way. -->
    {:else if !episodes.length}
      <div class="welcome">
        <h1>Add a video</h1>
        <p class="muted">
          The app transcribes it on this machine, finds the moments worth clipping and renders
          them as vertical clips.{videoNames ? ` Any ${videoNames} works.` : ""}
        </p>
        <button class="primary" onclick={add}>Add</button>
      </div>
    {:else}
      <div class="welcome">
        <h1>Pick a video</h1>
        <p class="muted">Choose one on the left, or add a new one.</p>
      </div>
    {/if}
  </main>
  {/if}
  </div>

  <!-- Where a video dragged over the app would land: all of it. -->
  <Landing />

  {#if leaving}
    <!-- Over everything, the moment the key is heard. A click or Escape
         takes the question away, the same as waiting does. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="leaving"
      role="status"
      onpointerdown={stayOpen}
    >
      <p>{leaving === "ask" ? `Press ${quitKey} again to quit` : "Quitting"}</p>
    </div>
  {/if}

  <!-- What went wrong with nothing on screen about it, over the app. A
       toast about a video waits while another is on screen. -->
  <Toasts shown={(t) => !t.video || t.video === asked} />

  {#if removing}
    {@const ep = removing}
    <Confirm title="Remove {ep.name}?" oncancel={() => !removingHow && (removing = null)}>
      {#if ep.work}
        <p>
          The transcript, the clip sets and the rendered clips are in
          <b>{workFolder(ep.source)}</b>, beside the video. Keeping that folder means adding the
          video again picks up where this left off, deleting it starts from the beginning. The
          video file itself always stays.
        </p>
      {:else}
        <p>
          There is nothing beside the video to throw away: it has no
          <b>{workFolder(ep.source)}</b> folder. Only the video leaves the list, the file itself
          stays where it is.
        </p>
      {/if}
      <!-- Left to right, the safest answer to the one that cannot be taken
           back, which is where the eye and the hand both expect it. The
           keyboard starts on Cancel: what is irreversible is marked as
           what it is rather than made the answer Enter gives. No answer is
           picked out: this box asks which of three things to do and the
           app has no opinion on two of them, so all three look alike and
           only the one that cannot be taken back is marked. The other
           boxes have two answers and no highlight either. -->
      {#snippet actions()}
        <button onclick={() => (removing = null)} disabled={!!removingHow}>Cancel</button>
        {#if ep.work}
          <button class="answer" onclick={() => remove(ep, false)} disabled={!!removingHow}
            >{#if removingHow === "keep"}<Busy />{/if}{removingHow === "keep"
              ? "Removing"
              : "Keep"}</button
          >
        {/if}
        <button class="answer danger" onclick={() => remove(ep, true)} disabled={!!removingHow}
          >{#if removingHow === "delete"}<Busy />{/if}{removingHow === "delete"
            ? "Removing"
            : "Remove"}</button
        >
      {/snippet}
    </Confirm>
  {/if}
</div>

<style>
  /* Cmd+Q heard. The dim is the one a box over the workspace brings,
     Confirm.svelte, so the app looks the same whenever it is waiting on
     an answer. The words are the size of a box's title. */
  .leaving {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: grid;
    place-items: center;
    background: var(--backdrop);
  }

  /* The words on a dark ground of their own, the way macOS shows the
     volume, because the dim alone leaves them over the captions in the
     video preview. No line round it and nothing to press: a box over the
     workspace is Confirm.svelte, and it only asks about what cannot be
     taken back. */
  .leaving p {
    margin: 0;
    padding: 12px 20px;
    border-radius: var(--radius-m);
    background: var(--ink-0);
    font-size: var(--size-l);
    font-weight: 600;
  }

  /* The rail keeps its own column, so nothing of the workspace ever hides
     under it. The sidebar grows over the workspace from there. */
  .shell {
    position: relative;
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .body {
    position: relative;
    display: grid;
    grid-template-columns: var(--rail) 1fr;
    flex: 1;
    min-height: 0;
  }

  /* The setup has the whole app, rail and all: there is nothing on the
     rail worth reaching for until it is done.

     The row is as tall as the app, not as tall as the setup. Left
     implicit it is sized to what is in it, which in an app too short for
     the whole setup makes the row taller than the app, and then the foot
     of the setup is below the bottom edge with nothing to scroll: the
     page itself never scrolls. minmax(0, 1fr) is the row being the app's
     own height and what is in it being allowed to give way. */
  .body.alone {
    grid-template-columns: 1fr;
    grid-template-rows: minmax(0, 1fr);
  }

  aside {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    z-index: 10;
    width: var(--rail);
    display: flex;
    flex-direction: column;
    background: var(--ink-1);
    /* The line at the edge is drawn inside rather than as a border, so the
       rail is 44px of room for the icons and not 43. That way an icon and
       its highlight sit on whole pixels in the middle of the rail. */
    box-shadow: inset -1px 0 var(--line);
    min-height: 0;
    overflow: hidden;
    transition: width 0.14s ease;
  }

  /* The rail and the settings column of the workspace, so the open
     sidebar covers that column and nothing else. */
  aside.open {
    width: calc(var(--span-l) + var(--gap));
    box-shadow:
      inset -1px 0 var(--line),
      8px 0 24px rgba(0, 0, 0, 0.45);
    /* The shadow falls on the workspace to the right and nowhere else. Its
       blur reached up over the title bar too, which Tim saw as a shadow
       cast on the bar the sidebar hangs from. Only the right side is let
       out, as far as the blur goes. */
    clip-path: inset(0 -40px 0 0);
  }

  @media (prefers-reduced-motion: reduce) {
    aside {
      transition: none;
    }
  }

  /* What there is no room for on the rail waits until the sidebar opens.
     An icon on the rail has to be exactly where its row is when the
     sidebar opens, or the sidebar opens and moves it out from under the
     pointer that came for it. So a row keeps its height and only its
     words go, and an episode keeps its row and shows only its lamp. */
  aside:not(.open) h2,
  aside:not(.open) .order,
  aside:not(.open) .label {
    display: none;
  }

  /* On the rail an episode is its lamp and nothing else. What it is called
     and what it has wait for the room, and so do the two marks, which need
     a pointer on the row anyway. The row keeps its height, so the lamp is
     in the same place shut and open and nothing moves as the sidebar
     goes over. */
  aside:not(.open) .episode .line,
  aside:not(.open) .tools {
    display: none;
  }

  /* The sidebar slides open, so for as long as it moves its contents are
     narrower than the room they will have. Nothing in it may reflow on the
     way, or a line wraps, a row grows and the icons jump out from under the
     pointer that came for them. Every line in the sidebar stays one line and
     what does not fit yet is clipped by the sidebar. */
  .head h2,
  .nav .label {
    white-space: nowrap;
  }

  .nav .label {
    min-width: 0;
    overflow: hidden;
  }

  /* The list keeps its 4 at the sides on the rail, so a lamp stands in the
     same column shut and open and does not step across as the sidebar goes
     over. Only the scrolling goes, because a rail has nothing to scroll. */
  aside:not(.open) ul {
    overflow: hidden;
  }

  /* Never squeezed by the rail, or the glyph would sit somewhere else on
     the rail than it does when the sidebar is open. */
  .glyph {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 36px;
    padding: 0;
    color: var(--muted);
  }

  .glyph:hover:not(:disabled) {
    color: var(--text);
  }

  /* 4px each side of a 36px box leaves the icon in the middle of the rail,
     and the same box holds the rows in the foot. */
  .head {
    justify-content: space-between;
    padding: 8px 12px 8px 4px;
  }

  .head h2 {
    flex: 1;
  }

  /* The sort choice keeps its size and gives way to nothing: the name of
     the area takes what is left. */
  .order {
    flex: none;
  }

  h2 {
    font-size: var(--size-l);
    font-weight: 600;
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0 4px;
    flex: 1;
    min-height: 0;
  }

  /* The 2 between one row and the next belongs to the row above, so the
     pointer is always on a row on its way down the list, and no row goes
     dark for the moment it crosses the gap. */
  li {
    position: relative;
    padding-bottom: 2px;
  }

  /* The row is lit while the pointer is anywhere on it, the three dots
     included: they lie over the row, and lighting the button alone made
     the row go dark each time the pointer reached them. */
  li:hover .episode {
    background: var(--ink-3);
  }

  /* A whole number of pixels high, and the same number whether the sidebar
     is shut or open: the 36 the rail gives every icon, so a row shut is an
     icon's box around its lamp and a row open is the same box with the
     name beside it. Saying the height rather than letting the text say it
     means the row does not shrink to its lamp when the name goes, and a
     lamp that dropped up the list as the sidebar opened is the one thing a
     sidebar sliding over must not do.

     It said the episode's state on a second line, and 54 was the height of
     two. One line is what a row of this kind holds everywhere else on the
     rail, and the state is in the row's own title for whoever wants it.

     The left padding is the foot's, so the lamp stands in the same column
     as every icon on the rail: 4 from the list, 1 of border and 9 here put
     the lamp's box at 14, and an 8 wide lamp centred in a 16 wide box
     leaves it on 22, which is the middle of the rail. */
  .episode {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    height: 36px;
    padding: 0 8px 0 9px;
    text-align: left;
    border-color: transparent;
    background: transparent;
  }

  /* An icon's box around a lamp, so it is centred on the icons' column. */
  .episode .dot {
    margin: 0 4px;
  }

  /* An even number, so the name sits on a whole pixel in a row of an even
     height. Thirteen point five at the usual line height is nineteen, and
     nineteen in thirty-six leaves half a pixel above and below. */
  .episode .name {
    line-height: 20px;
  }

  .episode.current,
  .nav.current {
    background: var(--ink-3);
  }

  /* The name and the label after it, on one line. The label sits right
     after the name rather than at the far edge, so the two read as one
     thing, and the label keeps its words while a long name gives way.
     Room is kept for the three dots, so neither ever runs under them: the
     mark is 36 wide and 8 from the row's edge, the row's own padding is 8,
     and 6 more is the gap between them. */
  .episode .line {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    min-width: 0;
    padding-right: 42px;
  }

  .episode .name {
    min-width: 0;
  }

  /* What you do with an episode itself waits until the pointer is on its
     row, the way the trash can does on a clip. */
  .tools {
    position: absolute;
    /* The three dots stand under the sort mark at the head of the list,
       one column of marks of one size: the head keeps 12 at its right and
       the list 4, so 8 more here. They take the pointer over the whole
       height of the row, gap included, so a pointer going down the
       column of dots goes from one to the next and never off them, and
       they are drawn a control's height in the middle of the 36, see
       VideoMenu.svelte. */
    top: 0;
    bottom: 0;
    right: 8px;
    display: flex;
    gap: 2px;
    opacity: 0;
    pointer-events: none;
  }

  li:hover .tools,
  .tools:focus-within,
  .tools:has(:global([aria-expanded="true"])) {
    opacity: 1;
    pointer-events: auto;
  }

  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .small {
    font-size: var(--size-s);
  }

  .foot {
    border-top: 1px solid var(--line);
    padding: 8px 4px;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 2px;
  }

  /* The icon of a foot row sits where the sidebar glyph at the head sits,
     so the rail is one column of icons and every one of them is in the
     same place when the sidebar opens over it. */
  .nav {
    text-align: left;
    display: flex;
    align-items: center;
    /* 9px and the button's own 1px border put the icon where the glyph at
       the head has it, in the middle of the rail. */
    gap: 8px;
    padding: 0 9px;
  }

  .nav .label {
    flex: 1;
  }

  .mark {
    position: relative;
    display: flex;
    flex: none;
  }

  /* The dot on Updates, in the corner of its icon, whatever its colour. */
  .mark .dot {
    position: absolute;
    top: -2px;
    right: -3px;
    width: 7px;
    height: 7px;
  }

  /* The build stands at the far end of its row, where a count stands in
     a list, and whole: it names what is being tested, so it keeps its own
     width and Updates has what is left. Shared out evenly, the two had
     half the row each, and 0.3.0-pr29.db33a28 lost its last letters. */
  .nav .build {
    flex: none;
    margin-left: auto;
  }

  /* The sidebar is out of the flow, so the workspace has to be told to
     stay in the second column and leave the rail alone. */
  main {
    position: relative;
    grid-column: 2;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  /* The workspace in front and the one being opened behind it stand in
     the same place, one cell of a grid, so the one behind is laid out to
     the pixel as it will be shown and changing places moves nothing. */
  .workspaces {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template: minmax(0, 1fr) / minmax(0, 1fr);
  }

  /* Kept while another screen is open, where they stand when shown and
     unseen, so one being built as the app starts is laid out to the pixel
     and its canvases keep their size. */
  .workspaces.gone {
    position: absolute;
    inset: 0;
    visibility: hidden;
  }

  .workspace {
    grid-area: 1 / 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }

  .workspace.behind {
    visibility: hidden;
  }

  .workspace.front {
    z-index: 1;
  }

  /* What a workspace stands in. An empty one, its workspace put away,
     takes no room. */
  .slot {
    display: contents;
  }

  /* In the middle of the workspace, so the sidebar can lie over the left of
     it and the words are still there to read. */
  .welcome {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 24px 48px 64px;
    text-align: center;
  }

  .welcome p {
    max-width: 52ch;
  }

  .welcome button {
    margin-top: var(--gap);
  }

  .welcome h1 {
    font-size: var(--size-xl);
    font-weight: 600;
  }
</style>
