// The walk's clock, which a play runs on in the page the walks run on, see
// install. It is a test's own clock, so it lives here with the walks and
// the app never builds it: the app plays on the sound card's clock, see
// FrameQueue.clock.
//
// It stands on each frame of a play until it is up, and holds the sound
// card meanwhile, so a walk sees every frame of a play on any machine and
// the sound stays with the picture. By the sound card's clock a busy
// machine showed 2 frames of 9, issue #534, and standing on frames alone
// left the sound running on ahead, so a loop switched off or a cut made
// while playing went by the sound's moment, not the picture's, issue #588.
import { FrameQueue, type Play, type PlayClock } from "../src/lib/frames/queue";

export function walkClock(play: Play): PlayClock {
  // A frame coming, decoded and waiting, goes up on this tick, so the
  // sound card runs on for it.
  return (heard, unshown, coming) => {
    void holdSound(play, "frame", heard > unshown && !coming);
    return Math.min(heard, unshown);
  };
}

// Who holds a play's sound card: the walk's clock for a frame not up yet,
// "frame", and the walk's hand while it drags, "hand", see holdSound in
// walks/bridge.mjs and issue #583. The sound card runs again only once
// nobody holds it, and only while a play goes on: a play the app pauses or
// ends holds it itself, and a play that starts starts it. Held again on
// every tick it is wanted, since the app may have started it meanwhile.
// Settles once the sound card has done what it was told.
const holders = new WeakMap<Play, Set<string>>();
export function holdSound(play: Play, who: string, held: boolean): Promise<void> {
  let by = holders.get(play);
  if (!by) holders.set(play, (by = new Set()));
  if (held) {
    by.add(who);
    return play.audio.state === "running" ? play.audio.suspend() : Promise.resolve();
  }
  if (!by.delete(who) || by.size) return Promise.resolve();
  return play.playing && play.audio.state === "suspended" ? play.audio.resume() : Promise.resolve();
}

// Hands every queue the page makes the walk's clock, unless the walk asked
// for the sound card's with window.__playClock, see open in
// walks/bridge.mjs. window.__holdSound(held) holds the sound card of the
// queue on the video preview by the walk's hand, or lets it go. Called by
// wails-bridge.ts before the app makes its first queue.
export function install() {
  const page = window as unknown as { __playClock?: string; __frameQueue?: Play; __holdSound?: (held: boolean) => Promise<void> };
  if (page.__playClock !== "sound") FrameQueue.clock = walkClock;
  page.__holdSound = (held) => {
    if (!page.__frameQueue) throw new Error("there is no video preview whose sound card the hand could hold");
    return holdSound(page.__frameQueue, "hand", held);
  };
}
