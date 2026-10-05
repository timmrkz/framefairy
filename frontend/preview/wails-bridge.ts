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
