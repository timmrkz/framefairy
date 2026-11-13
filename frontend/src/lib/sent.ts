// A video's state as the Go side sends it, made whole again. A state sent
// leaves out each clip the window was already sent as it is now and names
// it by its key and its hash in order, see VideoState.Order in
// cmd/framefairy-app/videostate.go and
// docs/decisions/2026-11-13-a-state-sends-only-the-clips-that-changed.md.
// What every part of the window hears is a whole state, the one shape
// VideoState has.
import type { ClipEntry, VideoState } from "./api";

// A clip as it is sent: with its hash, while it was sent whole.
type SentClip = ClipEntry & { hash?: string };

// A state as it is sent: its clips are those that changed, and order is
// every clip, by its key and its hash. A state read by a call holds
// every clip and no order.
export type SentState = Omit<VideoState, "clips"> & {
  clips: SentClip[];
  order?: { key: string; hash: string }[];
};

export class SentClips {
  // The clips of each video as they were last sent here, by their key.
  private sent = new Map<string, Map<string, { hash: string; clip: ClipEntry }>>();

  // The state with every clip, or null when it names a clip by its key
  // that was not sent here as it is, which happens when the window was
  // opened again while the Go side went on: then the Go side is asked
  // for every clip, see SendWhole. A clip that did not change is the
  // very same clip as before.
  whole(state: SentState): VideoState | null {
    if (!state.order) return state as VideoState;
    const had = this.sent.get(state.video);
    const came = new Map(state.clips.map((c) => [c.key, c]));
    const now = new Map<string, { hash: string; clip: ClipEntry }>();
    const clips: ClipEntry[] = [];
    for (const { key, hash } of state.order) {
      let clip: ClipEntry | undefined;
      const sent = came.get(key);
      if (sent) {
        const { hash: _, ...rest } = sent;
        clip = rest;
      } else if (had?.get(key)?.hash === hash) {
        clip = had.get(key)!.clip;
      }
      if (!clip) return null;
      now.set(key, { hash, clip });
      clips.push(clip);
    }
    this.sent.set(state.video, now);
    const { order: _, ...rest } = state;
    return { ...rest, clips };
  }
}
