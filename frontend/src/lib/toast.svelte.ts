// What went wrong when nothing on screen is about it: a file dropped on
// the app that is not a video, a call to the Go side that failed with no
// row or card of its own, a file Report a Problem turns away. Each is a
// toast, a note that lies over the app in its top right corner and moves
// nothing under it. Issue #327, and Toasts.svelte, the one place they
// are drawn.
//
// Each time something goes wrong is a toast of its own, the way macOS
// stacks its notes: the newest comes in on top and pushes the others
// down, and each runs its own time. Tim dropped one picture after another
// and saw only the last, because a newer reason from the same place once
// replaced the older. The same words again from the same place while
// they show do not stack, they come in on top with their time started
// again. A toast about a video shows while that video is on screen and
// waits while it is not. Its time runs down from the moment it shows, the
// way a removed clip's does, and stands still while the pointer is on it.
// Its cross closes it at once.

import { untrack } from "svelte";
import { said } from "./said";

export type Toast = {
  id: number;
  // Where it comes from: the app, the report window, or a video.
  from: string;
  text: string;
  // The video it is about, if it is about one.
  video?: string;
};

let next = 1;

class Toasts {
  // Newest first.
  list = $state<Toast[]>([]);

  // Says text from a place, on top of what is there. It reads and writes
  // the list untracked, since a place may say something from inside an
  // effect, which would otherwise run again on every toast.
  say(from: string, text: string, how: { video?: string } = {}) {
    const words = text.trim();
    if (!words) return;
    untrack(() => {
      said(`toast: ${words}`, { level: "warn", video: how.video });
      const others = this.list.filter((t) => !(t.from === from && t.text === words));
      this.list = [{ id: next++, from, text: words, ...how }, ...others];
    });
  }

  // Takes a place's toasts away, because what they were about is gone.
  clear(from: string) {
    untrack(() => {
      if (!this.list.some((t) => t.from === from)) return;
      this.list = this.list.filter((t) => t.from !== from);
    });
  }

  // The person closed it, or its time ran out.
  close(id: number, how: "closed" | "ran out") {
    const t = this.list.find((x) => x.id === id);
    if (!t) return;
    said(`toast ${how}: ${t.text}`, { level: "info", video: t.video });
    this.list = this.list.filter((x) => x.id !== id);
  }

  // The newest a place said that still shows, or "".
  text(from: string): string {
    return this.list.find((t) => t.from === from)?.text ?? "";
  }
}

export const toasts = new Toasts();
