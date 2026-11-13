// A video's state as the Go side sends it, made whole again. A state sent
// leaves out each clip the window was already sent as it is now and names
// it by its key and its hash in order, or, when not one clip changed,
// leaves out the clips and the order both, see VideoState.Order and Kept
// in cmd/framefairy-app/videostate.go and
// docs/decisions/2026-11-13-a-state-sends-only-the-clips-that-changed.md.
// What every part of the window hears is a whole state, the one shape
// VideoState has, with no order, no kept and no hashes.
import type { ClipEntry, VideoState } from "./api";

// A video's clips as they were last sent here: in order, and by key with
// the hash each was sent with.
type Sent = { clips: ClipEntry[]; byKey: Map<string, { hash: string; clip: ClipEntry }> };

export class SentClips {
  private sent = new Map<string, Sent>();
  // The videos the Go side was asked to send whole, while it has not
  // answered the ask.
  private asking = new Set<string>();

  // ask asks the Go side to send the next state of a video with every
  // clip, see SendWhole.
  constructor(private ask: (video: string) => Promise<unknown>) {}

  // The state with every clip, or null when it names a clip that was not
  // sent here as it is, which happens when the window was opened again
  // while the Go side went on, or a state sent was lost. Then the Go side
  // is asked for every clip, once at a time: a state that still cannot be
  // made whole after the ask was answered asks again, so a lost whole
  // state never leaves the video on the state before it. A clip that did
  // not change is the very same clip as before.
  whole(state: VideoState): VideoState | null {
    const { order, kept, ...rest } = state;
    if (!order && !kept) return state;
    const had = this.sent.get(state.video);
    let now: Sent | null = null;
    if (kept) {
      if (had) now = had;
    } else {
      now = this.fill(state.clips, order!, had);
    }
    if (!now) {
      this.askWhole(state.video);
      return null;
    }
    this.sent.set(state.video, now);
    return { ...rest, clips: now.clips };
  }

  private fill(came: ClipEntry[], order: { key: string; hash: string }[], had: Sent | undefined): Sent | null {
    const sent = new Map(came.map((c) => [c.key, c]));
    const now: Sent = { clips: [], byKey: new Map() };
    for (const { key, hash } of order) {
      let clip: ClipEntry | undefined;
      const fresh = sent.get(key);
      if (fresh) {
        const { hash: _, ...bare } = fresh;
        clip = bare;
      } else if (had?.byKey.get(key)?.hash === hash) {
        clip = had.byKey.get(key)!.clip;
      }
      if (!clip) return null;
      now.byKey.set(key, { hash, clip });
      now.clips.push(clip);
    }
    return now;
  }

  private askWhole(video: string) {
    if (this.asking.has(video)) return;
    this.asking.add(video);
    const done = () => this.asking.delete(video);
    this.ask(video).then(done, done);
  }
}
