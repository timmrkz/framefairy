// Until the picture on the video preview stands, see standing in
// lib/frames/queue.ts, for a walk or a probe to read it then: the frame
// queue of the app, window.__frameQueue, or of the frame queue's own page,
// window.__queue. A step that opens another video makes a new queue, which
// is followed. Woken by the queue's reports and by the page changing,
// never by looking again and again.
//
// Answers true once the queue stands, or when there is none. With held,
// the app's time is held, see holdClock in walks/bridge.mjs, and the
// queue cannot come to stand by itself, so it answers false at once
// instead of waiting for good. A walk that reads the picture once it
// stands reads it in a then of this, so nothing comes between. While it
// waits it says why the queue does not stand, see waits.ts.
import { waitsOn } from "./waits";

type Queue = { standing: boolean; notStanding?: string; listen(fn: () => void): () => void };

(window as any).__standing = (held = false) =>
  new Promise<boolean>((done) => {
    let q: Queue | null = null;
    let off = () => {};
    const o = new MutationObserver(() => again());
    const again = () => {
      const now: Queue | null = (window as any).__frameQueue ?? (window as any).__queue ?? null;
      if (now !== q) {
        off();
        q = now;
        off = q ? q.listen(again) : () => {};
      }
      if (q && !q.standing && !held) {
        waitsOn(`the picture to stand: ${q.notStanding ?? "the frame queue has not said why"}`);
        return;
      }
      off();
      o.disconnect();
      done(!q || q.standing);
    };
    o.observe(document, { subtree: true, childList: true, attributes: true });
    again();
  });
