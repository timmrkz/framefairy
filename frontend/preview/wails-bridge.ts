// Takes the place of the Wails runtime when the interface runs against the
// real Go side through the bridge, see cmd/framefairy-app/bridge_test.go.
// A call is a POST to /call, and what the Go side tells the interface
// comes as server-sent events on /events. Nothing here knows what any
// call or event means: that is the Go side's, which is the point of it.

type Listener = (ev: { data: unknown }) => void;
const listeners = new Map<string, Set<Listener>>();

// The calls made and how each ended, for a walk to read.
const calls: { name: string; args: unknown[]; failed?: string }[] = [];
(window as any).__calls = calls;
// What the app still has on its way after a step, so a walk can wait
// until it is done before it compares the screen with the engine: the
// calls out, the timers set to fire in a moment, and what moves on the
// page for a moment. __settled answers when none is left, woken by the
// call coming back, the timer firing or being cleared and the movement
// ending, never by looking again and again.
//
// The calls that only write the app's log are left out: they change
// nothing on screen. So is the timer that writes the log, see said.ts.
// A timer counts when it is to fire within a second: a clip timeline
// that loads its view when a swipe has stopped, a frame put up without
// its captions when they are late, a workspace shown once it has had its
// second to be built. Longer ones are how long something stays on
// screen, a toast or a Copied, which a walk turns on itself with
// holdClock. A movement counts the same way: a row sliding open or a clip
// timeline gliding to its view, not the shimmer over a place waiting to
// be filled, which moves for good.
const SOON = 1000;
const logOnly = new Set(["Said", "Acted"]);
let out = 0;
let back = 0;
const soon = new Set<unknown>();
let faked = false;
let woken: (() => void)[] = [];
function wake() {
  const now = woken;
  woken = [];
  for (const fn of now) fn();
}

const arm = window.setTimeout.bind(window);
const disarm = window.clearTimeout.bind(window);
window.setTimeout = ((fn: TimerHandler, ms = 0, ...args: unknown[]) => {
  if (faked || typeof fn !== "function" || ms > SOON || (fn as any).logOnly) return arm(fn, ms, ...args);
  const id = arm(
    (...given: unknown[]) => {
      soon.delete(id);
      fn(...given);
      wake();
    },
    ms,
    ...args,
  );
  soon.add(id);
  return id;
}) as typeof window.setTimeout;
window.clearTimeout = ((id?: number) => {
  if (soon.delete(id)) queueMicrotask(wake);
  disarm(id);
}) as typeof window.clearTimeout;

// Playwright's fake clock, put in while the page is open, takes the place
// of these two for the app, and keeps them for its own timers, which run
// it as fast as the real one and are cleared where this never sees it.
// So the walk stops counting then, see fakeClock in walks/bridge.mjs: the
// app's timers are the fake clock's from then on, which a walk turns on
// itself. After a load the fake clock comes first, with the browser's
// own timers, and these wrap what it gives the app.
(window as any).__forgetSoon = () => {
  faked = true;
  soon.clear();
  wake();
};

const moving = () =>
  document.getAnimations().filter((a) => {
    const end = Number(a.effect?.getComputedTiming().endTime);
    return a.playState === "running" && Number.isFinite(end) && end <= SOON;
  });

// Answers once nothing is on its way. When nothing is, it lets two of the
// browser's frames go by, so a call the app makes as it draws is out by
// then, and answers if nothing came or came back in between, or looks
// again. The
// frames are the browser's own, kept before a fake clock comes, see
// fakeClock. While the app's time is held, held, its timers fire only
// when the walk turns the clock on, so they are not waited for.
(window as any).__settled = (held: boolean) =>
  new Promise<void>((done) => {
    const frame: (fn: () => void) => void = (window as any).__real?.frame ?? requestAnimationFrame;
    const busy = () => out > 0 || (!held && soon.size > 0);
    const look = () => {
      if (busy()) {
        woken.push(look);
        return;
      }
      const going = moving();
      if (going.length) {
        Promise.all(going.map((a) => a.finished.catch(() => {}))).then(look);
        return;
      }
      const seen = back;
      frame(() => frame(() => (back === seen && !busy() && !moving().length ? done() : look())));
    };
    look();
  });

// Settled once the interface is listening, so a walk asks for a mark only
// when the interface can hear it, after a reload too.
let opened = () => {};
(window as any).__listening = new Promise<void>((done) => (opened = done));

let source: EventSource | null = null;
function listen() {
  if (source) return;
  source = new EventSource("/events");
  source.onopen = () => opened();
  source.onmessage = (message) => {
    const { name, data } = JSON.parse(message.data) as { name: string; data: unknown };
    if (paused) paused.push({ name, data });
    else tell(name, data);
  };
}
// How many times the interface has been told something of each name, and
// the last mark it heard, which the bridge sends behind everything it told
// the interface before, see /mark in bridge_test.go. A walk that has seen
// a mark come knows the count is whole up to it. window.__heardMark(n)
// settles once mark n has come.
const heard: Record<string, number> = {};
(window as any).__heard = heard;
let mark = 0;
let waiting: { n: number; done: () => void }[] = [];
(window as any).__heardMark = (n: number) =>
  new Promise<void>((done) => (mark >= n ? done() : waiting.push({ n, done })));
// The newest change of each job the interface has had, by the job's seq,
// from its news and from the lists of jobs it asked for. The Go side sends
// a job's news after it lets go of the queue, so a list asked for can show
// a change whose news is still on its way, and a walk waits for the seq
// the list showed, see caughtUp in walks/bridge.mjs.
const seqs: Record<string, number> = {};
(window as any).__seqs = seqs;
// window.__heardSeqs([[id, seq], ...]) settles once every job named has
// had that change or a newer one.
let due: { jobs: [string, number][]; done: () => void }[] = [];
const hasAll = (jobs: [string, number][]) => jobs.every(([id, seq]) => (seqs[id] ?? 0) >= seq);
(window as any).__heardSeqs = (jobs: [string, number][]) =>
  new Promise<void>((done) => (hasAll(jobs) ? done() : due.push({ jobs, done })));
function had(job: { id?: string; seq?: number } | undefined) {
  if (!job?.id || typeof job.seq !== "number") return;
  seqs[job.id] = Math.max(seqs[job.id] ?? 0, job.seq);
  const now = due.filter((d) => hasAll(d.jobs));
  due = due.filter((d) => !hasAll(d.jobs));
  for (const d of now) d.done();
}
function tell(name: string, data: unknown) {
  heard[name] = (heard[name] ?? 0) + 1;
  if (name === "job") had((data as { job?: { id?: string; seq?: number } })?.job);
  if (name === "mark") {
    mark = Math.max(mark, data as number);
    const now = waiting.filter((w) => w.n <= mark);
    waiting = waiting.filter((w) => w.n > mark);
    for (const w of now) w.done();
  }
  for (const fn of listeners.get(name) ?? []) fn({ data });
}

// What the Go side tells the interface waits, in order, while a walk
// aims at something on screen that an event could change: it reads where
// a word is with window.__pauseEvents(true), clicks there, and lets what
// waited through with window.__pauseEvents(false). A person clicks what
// they see, in the same frame, but a walk reads the page and clicks it in
// two trips, and the words being heard while dictating changed between
// the two, so the click landed in other words.
let paused: { name: string; data: unknown }[] | null = null;
(window as any).__pauseEvents = (on: boolean) => {
  if (on) {
    paused ??= [];
    return;
  }
  const waited = paused ?? [];
  paused = null;
  for (const { name, data } of waited) tell(name, data);
};

// What the menu bar sends, which a browser has no menu bar for: a walk
// presses Undo and Redo with window.__menu("undo") and window.__menu("redo"),
// the way the preview does.
(window as any).__menu = (what: string) => tell("undo", what);

// window.__nextCall(after, match) settles once a call numbered after or
// later, from 0, is one match says yes to, seen as the call is made,
// before it is answered.
type Made = (typeof calls)[number];
let calling: { after: number; match: (c: Made) => boolean; done: () => void }[] = [];
(window as any).__nextCall = (after: number, match: (c: Made) => boolean) =>
  new Promise<void>((done) => (calls.slice(after).some(match) ? done() : calling.push({ after, match, done })));

export const Call = {
  async ByName(name: string, ...args: unknown[]): Promise<unknown> {
    const call: Made = { name: name.split(".").pop() ?? name, args };
    calls.push(call);
    const now = calling.filter((w) => w.match(call));
    calling = calling.filter((w) => !now.includes(w));
    for (const w of now) w.done();
    const counted = !logOnly.has(call.name);
    if (counted) out++;
    let answer: Response;
    let body: string;
    try {
      answer = await fetch("/call", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, args }),
      });
      body = await answer.text();
    } finally {
      if (counted) {
        out--;
        back++;
        wake();
      }
    }
    if (!answer.ok) {
      call.failed = body.trim();
      throw new Error(call.failed);
    }
    const answered = body ? JSON.parse(body) : null;
    if (call.name === "Jobs" && Array.isArray(answered)) answered.forEach(had);
    return answered;
  },
};

export const Events = {
  On(name: string, fn: Listener): () => void {
    listen();
    let set = listeners.get(name);
    if (!set) listeners.set(name, (set = new Set()));
    set.add(fn);
    return () => set.delete(fn);
  },
};

// What the runtime does with a file dragged from the system over the app,
// which a browser cannot be handed: on macOS and Linux the Go side calls
// these, see window.ts in @wailsio/runtime 3.0.0-beta.23, the version the
// app pins. A drag starts with handleDragEnter, and handleDragOver lights
// nothing before it. While a file is held over a drop target, the nearest
// element marked data-file-drop-target wears file-drop-target-active, and
// a drop that lands on one goes to the Go side, which adds what it can,
// see drop.go. A drop anywhere else is ignored, as the runtime ignores it,
// and so is everything while the app's window takes no drops, which
// EnableFileDrop in main.go says. A walk drags and drops with these.
const TARGET = "data-file-drop-target";
const ACTIVE = "file-drop-target-active";
const wails = ((window as any)._wails ??= {});
wails.flags = { ...(wails.flags ?? {}), enableFileDrop: true };
let dragging = false;
let lit: Element | null = null;
function unlight() {
  dragging = false;
  lit?.classList.remove(ACTIVE);
  lit = null;
}
const target = (x: number, y: number) => document.elementFromPoint(x, y)?.closest(`[${TARGET}]`) ?? null;
Object.assign(wails, {
  handleDragEnter() {
    if (wails.flags.enableFileDrop === false) return;
    dragging = true;
  },
  handleDragOver(x: number, y: number) {
    if (!dragging || wails.flags.enableFileDrop === false) return;
    const over = target(x, y);
    if (over !== lit) lit?.classList.remove(ACTIVE);
    over?.classList.add(ACTIVE);
    lit = over;
  },
  handleDragLeave: unlight,
  async handlePlatformFileDrop(filenames: string[], x: number, y: number): Promise<boolean> {
    if (wails.flags.enableFileDrop === false) return false;
    const on = target(x, y);
    unlight();
    if (!on) return false;
    // The report window attaches pictures dropped on it, see bugs.go, and
    // the app's window adds videos.
    const report = new URLSearchParams(location.search).get("window") === "report";
    await fetch(report ? "/drop-report" : "/drop", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(filenames),
    });
    return true;
  },
});
