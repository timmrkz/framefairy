// What the app still has on its way after a step, for a walk or a probe
// to wait on, in the page against the real Go side, wails-bridge.ts, and
// in the one against the stand-in, wails-stub.ts. Imported before
// anything of the app, so the app's timers are the ones counted here.
//
// A walk waits until the app is done before it compares the screen with
// the engine, and a probe before it measures: the calls out, the timers
// set to fire in a moment, and what moves on the page for a moment.
// __settled answers when none is left, woken by the call coming back,
// the timer firing or being cleared and the movement ending, never by
// looking again and again.
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
import "./standing";

const SOON = 1000;
const logOnly = new Set(["Said", "Acted"]);
let out = 0;
let back = 0;

// A call a walk holds on the Go side until it lets it go, see __hold, is
// set aside: it is not out, so the walk can settle on everything else
// while it is held. holdNext is how many calls of a name from now on are
// held, and held the calls set aside.
const holdNext = new Map<string, number>();
const held = new Set<{ name: string; counted: boolean }>();

// A call of the app's to the Go side is out, by its name: what is handed
// back says it came back, answered or failed.
export function callOut(name: string): () => void {
  if (logOnly.has(name)) return () => {};
  const call = { name, counted: true };
  const next = holdNext.get(name) ?? 0;
  if (next > 0) {
    holdNext.set(name, next - 1);
    call.counted = false;
    held.add(call);
  } else out++;
  return () => {
    if (call.counted) out--;
    held.delete(call);
    back++;
    wake();
  };
}

// Holds the next call of a name on the Go side until __letHeld, the way a
// busy machine delivers it late, see /hold in bridge_test.go.
(window as any).__hold = async (name: string) => {
  holdNext.set(name, (holdNext.get(name) ?? 0) + 1);
  await fetch(`/hold?call=${encodeURIComponent(name)}&gate=shut&times=${holdNext.get(name)}`);
};

// Lets every call held go on to the Go side, and counts them as out again,
// so the walk's next settle waits for them to come back. Hands back the
// names of the holds no call came for, which held nothing.
(window as any).__letHeld = async () => {
  const names = new Set([...holdNext.keys(), ...[...held].map((c) => c.name)]);
  const empty = [...holdNext].filter(([, n]) => n > 0).map(([name]) => name);
  holdNext.clear();
  for (const c of held) {
    if (!c.counted) {
      c.counted = true;
      out++;
    }
  }
  for (const name of names) await fetch(`/hold?call=${encodeURIComponent(name)}&gate=open`);
  return empty;
};
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

