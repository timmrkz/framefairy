// What the window says when something goes wrong, written into the app's
// own log on the Go side, ~/Library/Logs/Frame Fairy/app.log on a Mac. The
// webview keeps its console to itself, so without this a fault on a Mac
// the cloud cannot run leaves nothing to read. Plan row 2.185, and the
// log of R.8.

import { api } from "./api";

let waiting: string[] = [];
let timer: ReturnType<typeof setTimeout> | null = null;
let last = "";
let repeats = 0;

// Most lines kept between two writes: a loop gone wrong says so, and does
// not fill the disk.
const MOST = 200;

// Adds a line to the log, written within a second with what came with it.
// A line said again straight after itself is counted, not written again.
export function said(line: string) {
  if (line === last) {
    repeats++;
    return;
  }
  if (repeats) waiting.push(`(said ${repeats} more times)`);
  last = line;
  repeats = 0;
  waiting.push(line);
  if (waiting.length > MOST) waiting.splice(0, waiting.length - MOST);
  timer ??= setTimeout(write, 1000);
}

function write() {
  timer = null;
  if (repeats) waiting.push(`(said ${repeats} more times)`);
  repeats = 0;
  last = "";
  const lines = waiting;
  waiting = [];
  if (lines.length) api.said(lines).catch(() => {});
}

function text(args: unknown[]): string {
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

// The console's warnings and errors, and whatever is thrown and not
// caught, go to the log as well as where they went.
export function listen() {
  const warn = console.warn.bind(console);
  const error = console.error.bind(console);
  console.warn = (...args: unknown[]) => {
    warn(...args);
    said(`warning: ${text(args)}`);
  };
  console.error = (...args: unknown[]) => {
    error(...args);
    said(`error: ${text(args)}`);
  };
  window.addEventListener("error", (e) => said(`uncaught: ${text([e.error ?? e.message])}`));
  window.addEventListener("unhandledrejection", (e) => said(`unhandled: ${text([e.reason])}`));
}
