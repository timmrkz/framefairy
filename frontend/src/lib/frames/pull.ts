// Reading a pull of frames from the Go side, see cmd/framefairy-app/
// frames.go: each frame is where it starts, 8 bytes, and for a play what
// more the Go side says of it, 8 bytes each, see servePlay, then the frame in
// colours, RGBA with 8 bits each and an opaque alpha, which ffmpeg made
// from the file's tags. Each frame is read straight into a buffer of its
// own as the answer arrives and kept as it is, so a frame is copied once
// on its way in, not twice. It runs in a Worker, see pull.worker.ts, so
// it never holds up the page while a hand is on the clip timeline, and on
// the page itself where a Worker will not have it.

import { lightOf, type Light } from "./light";
import { Picture } from "./picture";

export type Pulled = {
  // The answer's status, 404 for a stream the Go side has closed.
  status: number;
  // The episode has ended, and why when it failed.
  end: boolean;
  error: string;
  // The Go side closed the stream, to make room for newer ones or because
  // nobody pulled from it: no end of the episode and no failure.
  closed: boolean;
  // Where the stream's time went before its first frame, sent with its
  // first frame: "started,opened,first,file,on", milliseconds and then
  // 1 or 0, see engine.PreviewTimes.
  times: string;
  // How the frames code their brightness, see light.ts.
  light: Light;
  // A look's frame is the one asked for, not one on the way to it, see
  // engine.Seen.
  exact: boolean;
  // A look's frame was kept on the Go side, nothing decoded for it.
  kept: boolean;
  // The Go side holds frames around a look's place, see AppFrames.near.
  near: boolean;
  // Each frame's pixels, see Picture. They are bytes rather than a
  // Picture so they can come from a Worker, see pictures.
  frames: { at: number; data: ArrayBuffer; stamp: number[] }[];
  // The buffers handed to the pull that it had no frame for, to be kept
  // for the next, see pool.ts.
  spare: ArrayBuffer[];
};

// The frames of a pull as Pictures of width by height.
export function pictures(got: Pulled, width: number, height: number): { at: number; frame: Picture; stamp: number[] }[] {
  return got.frames.map(({ at, data, stamp }) => ({ at, stamp, frame: new Picture(data, width, height, Math.round(at * 1e6), got.light) }));
}

// The frames are colours ready to draw, see framewire.Picture. Nothing
// here decides colour. Each frame is read into one of the spare buffers
// while there are any of its size, and into a new one after. head is how
// many bytes come before each frame.
export async function pull(url: string, width: number, height: number, spare: ArrayBuffer[] = [], head = 8): Promise<Pulled> {
  const res = await fetch(url).catch(() => null);
  if (!res) return { status: 0, end: false, error: "", closed: false, times: "", light: "", exact: false, kept: false, near: false, frames: [], spare };
  const out: Pulled = {
    status: res.status,
    end: !!res.headers.get("X-Frames-End"),
    error: res.headers.get("X-Frames-Error") ?? "",
    closed: !!res.headers.get("X-Frames-Closed"),
    times: res.headers.get("X-Frames-Times") ?? "",
    light: lightOf(res.headers.get("X-Frames-Light")),
    exact: !!res.headers.get("X-Frames-Exact"),
    kept: !!res.headers.get("X-Frames-Kept"),
    near: !!res.headers.get("X-Frames-Near"),
    frames: [],
    spare: [],
  };
  const size = width * height * 4;
  const fits = spare.filter((b) => b.byteLength === size);
  out.spare = fits;
  if (!res.ok || !res.body) return out;
  const before = new Uint8Array(head);
  let headHas = 0;
  let body: ArrayBuffer | null = null;
  let bodyHas = 0;
  const reader = res.body.getReader();
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    let at = 0;
    while (at < value.length) {
      if (headHas < head) {
        const n = Math.min(head - headHas, value.length - at);
        before.set(value.subarray(at, at + n), headHas);
        headHas += n;
        at += n;
        if (headHas === head) {
          body = fits.pop() ?? new ArrayBuffer(size);
          bodyHas = 0;
        }
        continue;
      }
      const n = Math.min(size - bodyHas, value.length - at);
      new Uint8Array(body!, bodyHas, n).set(value.subarray(at, at + n));
      bodyHas += n;
      at += n;
      if (bodyHas === size) {
        const v = new DataView(before.buffer);
        const stamp: number[] = [];
        for (let i = 8; i + 8 <= head; i += 8) stamp.push(v.getFloat64(i, true));
        out.frames.push({ at: v.getFloat64(0, true), data: body!, stamp });
        headHas = 0;
        body = null;
      }
    }
  }
  // A frame cut short leaves its buffer for the next pull.
  if (body && size) out.spare.push(body);
  return out;
}
