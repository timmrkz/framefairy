import type { CaptionCue, Splice } from "./api";

// The captions of a step of a drag: the ones the Go side laid out again
// near what the hand changed, put together with the ones held from before
// the way splice says, see Splice in engine/captionsnear.go. The Go side
// decides which captions are which, so this only follows what it says.
// Without a splice the captions sent are all of them.
export function spliced(held: CaptionCue[], sent: CaptionCue[], splice?: Splice): CaptionCue[] {
  if (!splice) return sent;
  const { head, tail, shift } = splice;
  const later = (c: CaptionCue): CaptionCue =>
    shift === 0
      ? c
      : {
          ...c,
          start: c.start + shift,
          end: c.end + shift,
          lines: c.lines.map((l) => ({ ...l, words: l.words.map((w) => ({ ...w, start: w.start + shift, end: w.end + shift })) })),
        };
  return [...held.slice(0, head), ...sent, ...held.slice(tail).map(later)];
}
