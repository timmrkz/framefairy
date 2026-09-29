// Dragging the playhead, on the clip timeline and on the range picker
// alike. The video preview follows the hand, which is the quickest way to
// find a moment. One seek a frame is enough, and it keeps a four hour
// episode moving: the pointer reports far more often than a frame is
// drawn, and every seek a frame cannot show is a seek the video element
// has to throw away. The last place the hand was is sought again when it
// lets go, so the playhead ends exactly where it was let go.
//
// One function for both, because the two are one playhead seen at two
// distances: a drag that felt different on each would be two playheads.

export function scrub(
  event: PointerEvent,
  // The moment under a point on the screen.
  timeAt: (clientX: number) => number,
  seek: (t: number) => void,
  // Told true when the drag starts and false when it ends, for what looks
  // different while the hand holds the playhead.
  holding?: (held: boolean) => void,
): void {
  const target = event.currentTarget as HTMLElement;
  target.setPointerCapture(event.pointerId);
  let wanted = timeAt(event.clientX);
  let queued = 0;
  const soon = (t: number) => {
    wanted = t;
    if (queued) return;
    queued = requestAnimationFrame(() => {
      queued = 0;
      seek(wanted);
    });
  };
  holding?.(true);
  soon(wanted);
  const move = (e: PointerEvent) => soon(timeAt(e.clientX));
  const up = () => {
    target.removeEventListener("pointermove", move);
    target.removeEventListener("pointerup", up);
    target.removeEventListener("pointercancel", up);
    cancelAnimationFrame(queued);
    queued = 0;
    holding?.(false);
    seek(wanted);
  };
  target.addEventListener("pointermove", move);
  target.addEventListener("pointerup", up);
  target.addEventListener("pointercancel", up);
}
