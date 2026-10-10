// What the window says, written into the app's own log on the Go side,
// ~/Library/Logs/Frame Fairy/app.log on a Mac. The webview keeps its
// console to itself, so without this a fault on a Mac the cloud cannot run
// leaves nothing to read. Plan row 2.185, and the log of R.8.
//
// It says enough that a fault is found by reading it, not by asking the
// person to try again: every step a video takes from its workspace opening
// to its first frame and its play, every call to the Go side that fails or
// is slow, every time the window stood still, and what the space bar met.
// Each line has its level and, where there is one, the video it is about,
// see docs/LOGGING.md.

import { api, tellTo } from "./api";

// error: something failed. warn: something went wrong and the app carried
// on. info: what the person did, and what started. debug: the steps
// between. trace: what comes with every frame, written only while the log
// is detailed.
export type Level = "error" | "warn" | "info" | "debug" | "trace";
export type Said = { level?: Level; video?: string };

type Line = { at: number; text: string; act?: string } & Said;

// The person's last act: a click, a key, a video picked. Every line said
// after it carries its id, and so does every line the Go side writes for
// it, so what one act set off is read together. See docs/LOGGING.md.
let acts = 0;
let acting = "";

// The video in front, if there is one.
function inFront(): string | undefined {
  return document.querySelector<HTMLElement>(".workspace.front[data-path]")?.dataset.path || undefined;
}

// Starts an act: says what the person did, at once, and gives its id.
export function act(what: string, video = inFront()): string {
  acting = `a${(++acts).toString(36)}`;
  api.acted({ id: acting, at: Date.now(), what, video }).catch(() => {});
  return acting;
}

let waiting: Line[] = [];
let timer: ReturnType<typeof setTimeout> | null = null;
let last = "";
let repeats = 0;
let repeatedAt = 0;

// Most lines kept between two writes: a loop gone wrong says so, and does
// not fill the disk.
const MOST = 400;

// Adds a line to the log, written within a quarter of a second with what
// came with it, stamped with the moment it was said. It is a debug line
// unless it says otherwise. A line said again straight after itself is
// counted, not written again.
export function said(text: string, how: Said = {}) {
  const at = Date.now();
  const key = `${acting} ${how.level ?? ""} ${how.video ?? ""} ${text}`;
  if (key === last) {
    repeats++;
    repeatedAt = at;
    return;
  }
  if (repeats) waiting.push({ at: repeatedAt, text: `(said ${repeats} more times)` });
  last = key;
  repeats = 0;
  waiting.push({ at, text, act: acting || undefined, ...how });
  if (waiting.length > MOST) waiting.splice(0, waiting.length - MOST);
  timer ??= setTimeout(write, 250);
}

function write() {
  timer = null;
  if (repeats) waiting.push({ at: repeatedAt, text: `(said ${repeats} more times)` });
  repeats = 0;
  last = "";
  const lines = waiting;
  waiting = [];
  if (lines.length) api.said(lines).catch(() => {});
}

// Anything as text for a line of the log.
export function text(args: unknown[]): string {
  return args
    .map((a) => {
      if (a instanceof Error) return `${a.message}${a.stack ? `\n${a.stack}` : ""}`;
      if (typeof a === "string") return a;
      try {
        return JSON.stringify(a);
      } catch {
        return String(a);
      }
    })
    .join(" ");
}

// The element that has the keyboard, in a few words.
function described(el: Element | null): string {
  if (!el || el === document.body) return "nothing";
  const h = el as HTMLElement;
  const name = h.getAttribute("aria-label") || h.title || (h.textContent ?? "").trim().slice(0, 30);
  return `${el.tagName.toLowerCase()}${h.className && typeof h.className === "string" ? `.${h.className.split(" ")[0]}` : ""}${name ? ` "${name}"` : ""}`;
}

// The workspaces as they stand: which one is in front, which are behind,
// and which take no keys.
export function workspaces(): string {
  const all = [...document.querySelectorAll<HTMLElement>(".workspace[data-path]")];
  if (!all.length) return "no workspace";
  return all
    .map((w) => {
      const name = (w.dataset.path ?? "").split("/").pop();
      const how = [w.classList.contains("front") ? "front" : "behind", w.closest("[inert]") ? "inert" : "", w.isConnected ? "" : "put away"];
      return `${name} (${how.filter(Boolean).join(", ")})`;
    })
    .join(", ");
}

// How long the window may stand still before it is said, in ms: a timer
// that should fire every quarter of a second came this much late.
const STOOD = 400;

// The control an act was on, by what the interface calls it, and never by
// the words of a video: a word clicked in the caption box is named as the
// caption box, so the log holds no transcript.
function control(target: Element | null): string {
  const el = target?.closest<HTMLElement>("button, a, input, textarea, select, canvas, label, [role], [contenteditable], [title], [aria-label]") ?? (target as HTMLElement | null);
  if (!el || el === document.body) return "nothing";
  const labelled = el.matches("button, a, label, [role=button], [role=menuitem], [role=option], [role=tab], [role=checkbox], [role=switch]");
  const name = el.getAttribute("aria-label") || el.title || (labelled ? (el.textContent ?? "").trim().slice(0, 30) : "");
  const kind = typeof el.className === "string" ? el.className.split(" ").find((c) => c && !c.startsWith("svelte-")) : "";
  return `${el.tagName.toLowerCase()}${kind ? `.${kind}` : ""}${name ? ` "${name}"` : ""}`;
}

// Whether a key is typing into a field, which is not an act of its own:
// the field the person typed into is, once, when they clicked or tabbed
// into it.
function typing(e: KeyboardEvent): boolean {
  const el = e.target as HTMLElement | null;
  const field = !!el && (el.isContentEditable || el.tagName === "INPUT" || el.tagName === "TEXTAREA");
  return field && !e.metaKey && !e.ctrlKey && (e.key.length === 1 || ["Backspace", "Delete", "ArrowLeft", "ArrowRight", "Shift"].includes(e.key));
}

// A key as the person pressed it: ⌘Z, Space, ArrowLeft.
function pressed(e: KeyboardEvent): string {
  const key = e.key === " " ? "Space" : e.key;
  return `${e.metaKey ? "⌘" : ""}${e.ctrlKey ? "⌃" : ""}${e.altKey ? "⌥" : ""}${e.shiftKey && key.length > 1 ? "⇧" : ""}${key}`;
}

// The console's warnings and errors, and whatever is thrown and not
// caught, go to the log as well as where they went. So does every time the
// window stood still, and what the space bar met. Every click and every
// key that is not typing is an act.
export function listen() {
  window.addEventListener("pointerdown", (e) => act(`pressed ${control(e.target as Element | null)}`), true);
  window.addEventListener(
    "keydown",
    (e) => {
      if (e.repeat || ["Shift", "Meta", "Control", "Alt", "CapsLock"].includes(e.key) || typing(e)) return;
      act(`${pressed(e)} on ${control(document.activeElement)}`);
    },
    true,
  );
  tellTo((line) => said(line, { level: "warn" }));
  const warn = console.warn.bind(console);
  const error = console.error.bind(console);
  console.warn = (...args: unknown[]) => {
    warn(...args);
    said(text(args), { level: "warn" });
  };
  console.error = (...args: unknown[]) => {
    error(...args);
    said(text(args), { level: "error" });
  };
  window.addEventListener("error", (e) => said(`uncaught: ${text([e.error ?? e.message])}`, { level: "error" }));
  window.addEventListener("unhandledrejection", (e) => said(`unhandled: ${text([e.reason])}`, { level: "error" }));
  said(`window: started, ${navigator.userAgent}, ${window.devicePixelRatio}x, ${innerWidth}x${innerHeight}, WebGPU ${"gpu" in navigator ? "there" : "not there"}`, { level: "info" });
  // A window that stands still is a window doing something too long: the
  // timer comes late by as much.
  let tick = performance.now();
  setInterval(() => {
    const now = performance.now();
    const late = now - tick - 250;
    tick = now;
    if (late > STOOD && !document.hidden) said(`window: stood still for ${Math.round(late)} ms`, { level: "warn" });
  }, 250);
  document.addEventListener("visibilitychange", () => said(`window: ${document.hidden ? "hidden" : "shown"}`));
  // The space bar, before anything takes it: where the keyboard was, and
  // which workspace could have heard it.
  window.addEventListener(
    "keydown",
    (e) => {
      if (e.code !== "Space" || e.repeat) return;
      const asking = !!document.querySelector("dialog[open]");
      said(`space bar: keyboard on ${described(document.activeElement)}${asking ? ", a box is asking" : ""}, workspaces ${workspaces()}`);
    },
    true,
  );
}
