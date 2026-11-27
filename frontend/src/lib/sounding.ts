import { getContext, setContext } from "svelte";

// Where the sound is while a video plays, in seconds, the one moment every
// playhead of a workspace stands on then, see Playhead.svelte. The video
// preview tells it once a refresh of the screen, from the sound card's
// clock, and NaN once the play stops. It writes --sound on each playhead
// and on nothing else: written on the workspace, it would be inherited by
// every element in it, and the browser would work all their styles out
// again on every refresh to pass one number down to two lines.
export class Sounding {
  private lines = new Set<HTMLElement>();
  private at = NaN;

  // A playhead that reads it, until the function it returns is called.
  line(el: HTMLElement): () => void {
    this.lines.add(el);
    this.write(el);
    return () => {
      this.lines.delete(el);
      el.style.removeProperty("--sound");
    };
  }

  set(at: number) {
    if (at === this.at || (Number.isNaN(at) && Number.isNaN(this.at))) return;
    this.at = at;
    for (const el of this.lines) this.write(el);
  }

  private write(el: HTMLElement) {
    if (Number.isFinite(this.at)) el.style.setProperty("--sound", String(this.at));
    else el.style.removeProperty("--sound");
  }
}

const key = Symbol("sounding");

// Made by the workspace, for the playheads and the video preview in it.
export function workspaceSounding(): Sounding {
  return setContext(key, new Sounding());
}

// The workspace's, or none outside one, as in a preview of one part.
export function sounding(): Sounding | undefined {
  return getContext<Sounding | undefined>(key);
}
