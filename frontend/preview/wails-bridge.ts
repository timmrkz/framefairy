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
// How many calls are on their way, and how many have come back, so a walk
// can wait for the app to settle before it compares the screen with the
// engine. A call that went and came back between two looks still counts.
// The calls that only write the app's log are left out: they change
// nothing on screen, and the log is written a quarter of a second after a
// line is said, so a walk that waited for them waited a second quarter
// after every step that said one, and longer the busier the machine.
(window as any).__pending = 0;
(window as any).__answered = 0;
const logOnly = new Set(["Said", "Acted"]);

// The app's timers that are to fire in a moment, up to a second, so a walk
// can wait for what the app does after one as it waits for a call: a
// clip timeline that loads its view when a swipe has stopped, a frame put
// up without its captions when they are late, a workspace shown once it
// has had its second to be built. Longer ones are how long something
// stays on screen, a toast or a Copied, which a walk turns on itself with
// holdClock. The timer that writes the log is left out, the same as its
// calls, see said.ts. A timer is counted until it fires or is cleared.
const SOON = 1000;
(window as any).__soon = 0;
const soon = new Set<unknown>();
let faked = false;
const arm = window.setTimeout.bind(window);
const disarm = window.clearTimeout.bind(window);
window.setTimeout = ((fn: TimerHandler, ms = 0, ...args: unknown[]) => {
  if (faked || typeof fn !== "function" || ms > SOON || (fn as any).logOnly) return arm(fn, ms, ...args);
  const id = arm(
    (...given: unknown[]) => {
      soon.delete(id);
      (window as any).__soon = soon.size;
      fn(...given);
    },
    ms,
    ...args,
  );
  soon.add(id);
  (window as any).__soon = soon.size;
  return id;
}) as typeof window.setTimeout;
window.clearTimeout = ((id?: number) => {
  soon.delete(id);
  (window as any).__soon = soon.size;
  disarm(id);
}) as typeof window.clearTimeout;
// Whether something on the page moves for a moment, a row sliding open
// or a clip timeline gliding to its view, which a walk waits for as it
// waits for a timer, because what it reads or clicks next is where the
// movement ends. What moves for longer, or for good, like the shimmer
// over a place waiting to be filled, is not waited for, see SOON.
(window as any).__moving = () =>
  document.getAnimations().some((a) => {
    const end = Number(a.effect?.getComputedTiming().endTime);
    return a.playState === "running" && Number.isFinite(end) && end <= SOON;
  });

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
  (window as any).__soon = 0;
};

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
function had(job: { id?: string; seq?: number } | undefined) {
  if (!job?.id || typeof job.seq !== "number") return;
  seqs[job.id] = Math.max(seqs[job.id] ?? 0, job.seq);
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

export const Call = {
  async ByName(name: string, ...args: unknown[]): Promise<unknown> {
    const call: (typeof calls)[number] = { name: name.split(".").pop() ?? name, args };
    calls.push(call);
    const counted = !logOnly.has(call.name);
    if (counted) (window as any).__pending++;
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
        (window as any).__pending--;
        (window as any).__answered++;
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
