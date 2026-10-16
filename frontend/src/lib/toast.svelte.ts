// What went wrong when nothing on screen is about it: a file dropped on
// the app that is not a video, a call to the Go side that failed with no
// row or card of its own, a file Report a Problem turns away. Each is a
// toast, a note that lies over the app in its top right corner and moves
// nothing under it. Plan row 2.268, and Toasts.svelte, the one place they
// are drawn.
//
// Each place that can go wrong has one toast at a time, known by where it
// comes from: a newer reason replaces the older one, and the place takes
// it away when what it was about is done again and goes well. A toast
// about a video shows while that video is on screen and waits while it is
// not. Its time runs down from the moment it shows, the way a removed
// clip's does, and stands still while the pointer is on it. Its cross
// closes it at once.

import { said } from "./said";

export type Toast = {
  id: number;
  // Where it comes from, which a newer reason from the same place replaces.
  from: string;
  text: string;
  // The video it is about, if it is about one.
  video?: string;
  // Told when the person closes it, so the place it came from forgets the
  // reason too and says it again the next time it happens.
  onclose?: () => void;
};

let next = 1;

class Toasts {
  list = $state<Toast[]>([]);

  // Says text from a place, replacing what that place said before. The
  // same words again leave the toast as it is, so a reason said twice does
  // not come in twice. Empty text takes the place's toast away.
  say(from: string, text: string, how: { video?: string; onclose?: () => void } = {}) {
    const words = text.trim();
    if (!words) return this.clear(from);
    const was = this.list.find((t) => t.from === from);
    if (was && was.text === words && was.video === how.video) {
      was.onclose = how.onclose;
      return;
    }
    said(`toast: ${words}`, { level: "warn", video: how.video });
    this.list = [...this.list.filter((t) => t.from !== from), { id: next++, from, text: words, ...how }];
  }

  // Takes a place's toast away, because what it was about went well.
  clear(from: string) {
    if (!this.list.some((t) => t.from === from)) return;
    this.list = this.list.filter((t) => t.from !== from);
  }

  // The person closed it, or its time ran out.
  close(id: number, how: "closed" | "ran out") {
    const t = this.list.find((x) => x.id === id);
    if (!t) return;
    said(`toast ${how}: ${t.text}`, { level: "info", video: t.video });
    this.list = this.list.filter((x) => x.id !== id);
    t.onclose?.();
  }

  // What a place says now, or "".
  text(from: string): string {
    return this.list.find((t) => t.from === from)?.text ?? "";
  }
}

export const toasts = new Toasts();
