// Dragging the playhead, on the clip timeline and on the range picker
// alike. The video preview follows the hand, which is the quickest way to
// find a moment. One seek a frame is enough, and it keeps a four hour
// episode moving: the pointer reports far more often than a frame is
// drawn, and every seek a frame cannot show is a seek the video preview
// has to throw away. Where the hand lets go is sought when it lets go,
// unless it was the last moment sought, so the playhead ends exactly where
// it was let go.
//
// One function for both, because the two are one playhead seen at two
// distances: a drag that felt different on each would be two playheads.

export function scrub(
  event: PointerEvent,
  // The moment under a point on the screen.
  timeAt: (clientX: number) => number,
  seek: (t: number) => void,
  // Told true when the drag starts and false when it ends, for what looks
  // different while the hand holds the playhead, and for a play that waits
  // under the hand. False comes after the last seek, so a play that goes
  // on as the hand lets go goes on from where it was let go.
  holding?: (held: boolean) => void,
): void {
  const target = event.currentTarget as HTMLElement;
  target.setPointerCapture(event.pointerId);
  let wanted = timeAt(event.clientX);
  let queued = 0;
  // The moment last sent, so letting go where the last seek went sends
  // nothing more. While the video plays, a click sent again as the hand
  // came up started the play over from the click, and the playhead that
  // had moved on from it went back.
  let sent = NaN;
  const send = (t: number) => {
    sent = t;
    seek(t);
  };
  const soon = (t: number) => {
    wanted = t;
    if (queued) return;
    queued = requestAnimationFrame(() => {
      queued = 0;
      send(wanted);
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
    if (wanted !== sent) send(wanted);
    holding?.(false);
  };
  target.addEventListener("pointermove", move);
  target.addEventListener("pointerup", up);
  target.addEventListener("pointercancel", up);
}
