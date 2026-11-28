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
//
// A press on a clip's mark takes hold of the playhead too, the user found
// a mark that did not, issue #1007. There a press let go where it was
// pressed chooses the clip, so the playhead waits for the hand to move
// more than the 2 px an edge waits for on the clip timeline, and a click
// sends no seek and holds nothing.

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
  // Called for a press let go before it moved, when the press is about
  // something under it. Without it, the press is the playhead's at once.
  tap?: () => void,
): void {
  const target = event.currentTarget as HTMLElement;
  target.setPointerCapture(event.pointerId);
  const startX = event.clientX;
  let held = !tap;
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
  if (held) {
    holding?.(true);
    soon(wanted);
  }
  const move = (e: PointerEvent) => {
    if (!held) {
      if (Math.abs(e.clientX - startX) <= 2) return;
      held = true;
      holding?.(true);
    }
    soon(timeAt(e.clientX));
  };
  const up = (e: PointerEvent) => {
    target.removeEventListener("pointermove", move);
    target.removeEventListener("pointerup", up);
    target.removeEventListener("pointercancel", up);
    cancelAnimationFrame(queued);
    queued = 0;
    if (!held) {
      if (e.type !== "pointercancel") tap?.();
      return;
    }
    if (wanted !== sent) send(wanted);
    holding?.(false);
  };
  target.addEventListener("pointermove", move);
  target.addEventListener("pointerup", up);
  target.addEventListener("pointercancel", up);
}
