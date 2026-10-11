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
// How many calls are on their way, so a walk can wait for the app to
// settle before it compares the screen with the engine.
(window as any).__pending = 0;

let source: EventSource | null = null;
function listen() {
  if (source) return;
  source = new EventSource("/events");
  source.onmessage = (message) => {
    const { name, data } = JSON.parse(message.data) as { name: string; data: unknown };
    for (const fn of listeners.get(name) ?? []) fn({ data });
  };
}

// What the menu bar sends, which a browser has no menu bar for: a walk
// presses Undo and Redo with window.__menu("undo") and window.__menu("redo"),
// the way the preview does.
(window as any).__menu = (what: string) => {
  for (const fn of listeners.get("undo") ?? []) fn({ data: what });
};

export const Call = {
  async ByName(name: string, ...args: unknown[]): Promise<unknown> {
    const call: (typeof calls)[number] = { name: name.split(".").pop() ?? name, args };
    calls.push(call);
    (window as any).__pending++;
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
      (window as any).__pending--;
    }
    if (!answer.ok) {
      call.failed = body.trim();
      throw new Error(call.failed);
    }
    return body ? JSON.parse(body) : null;
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
    await fetch("/drop", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(filenames),
    });
    return true;
  },
});
