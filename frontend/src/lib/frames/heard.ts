// What the sound card plays the video preview with that is not the Go
// side's: the fade a pause and a play go out and in on, and where the
// sound being heard is now. Which samples a play plays, and the fades at
// the edges of its pieces, are the Go side's, see engine.Stretch.
import type { Source } from "./even";

// The fade at each edge of a piece, the same as the render's, Fade in
// engine/render.go. A pause and a play fade the sound card's output over
// the same, so neither clicks.
export const FADE = 0.015;

// Where the sound being heard is now, on the sound card's clock. The
// output timestamp says which moment of the clock the speaker plays at
// which moment of the page's clock, so the latency of the output is in it.
// Where a browser does not give one, the clock less the latency it says.
//
// Never later than the clock itself: nothing can be heard that was not
// played yet. A stamp from before the sound card was held goes on counting
// from where it was taken, and after a pause of a second it said the sound
// was a second further on than it was, and play went on from there.
//
// And never far from the clock less its latency. WebKitGTK without a sound
// device gives stamps on another page clock, a quarter of a minute off,
// and the picture stood still on its first frame waiting for them, then
// jumped. A stamp taken more than a second ago, or in the future, or more
// than a quarter of a second from what the clock says, is not taken.
//
// It says where the moment came from too, for the log: the output
// timestamp, the timestamp held back to the clock, or the clock less its
// latency, see even.ts.
export function heard(
  stamp: { contextTime: number; performanceTime: number } | null,
  currentTime: number,
  latency: number,
  now: number,
): { at: number; source: Source } {
  const plain = Math.max(0, currentTime - latency);
  const age = stamp ? now - stamp.performanceTime : -1;
  if (stamp && stamp.performanceTime > 0 && stamp.contextTime > 0 && age >= -20 && age <= 1000) {
    const said = stamp.contextTime + (now - stamp.performanceTime) / 1000;
    const at = Math.min(said, currentTime);
    if (Math.abs(at - plain) <= 0.25) return { at, source: said > currentTime ? "held" : "stamp" };
  }
  return { at: plain, source: "plain" };
}
