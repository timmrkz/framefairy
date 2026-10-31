// Reading a pull of frames from the Go side, see cmd/framefairy-app/
// frames.go: each frame is where it starts, 8 bytes, then the frame in
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
  // Each frame's pixels, see Picture. They are bytes rather than a
  // Picture so they can come from a Worker, see pictures.
  frames: { at: number; data: ArrayBuffer }[];
  // The buffers handed to the pull that it had no frame for, to be kept
  // for the next, see pool.ts.
  spare: ArrayBuffer[];
};

// The frames of a pull as Pictures of width by height.
export function pictures(got: Pulled, width: number, height: number): { at: number; frame: Picture }[] {
  return got.frames.map(({ at, data }) => ({ at, frame: new Picture(data, width, height, Math.round(at * 1e6), got.light) }));
}

// The frames are colours ready to draw, see framewire.Picture. Nothing
// here decides colour. Each frame is read into one of the spare buffers
// while there are any of its size, and into a new one after.
export async function pull(url: string, width: number, height: number, spare: ArrayBuffer[] = []): Promise<Pulled> {
  const res = await fetch(url).catch(() => null);
  if (!res) return { status: 0, end: false, error: "", closed: false, times: "", light: "", frames: [], spare };
  const out: Pulled = {
    status: res.status,
    end: !!res.headers.get("X-Frames-End"),
    error: res.headers.get("X-Frames-Error") ?? "",
    closed: !!res.headers.get("X-Frames-Closed"),
    times: res.headers.get("X-Frames-Times") ?? "",
    light: lightOf(res.headers.get("X-Frames-Light")),
    frames: [],
    spare: [],
  };
  const size = width * height * 4;
  const fits = spare.filter((b) => b.byteLength === size);
  out.spare = fits;
  if (!res.ok || !res.body) return out;
  const head = new Uint8Array(8);
  let headHas = 0;
  let body: ArrayBuffer | null = null;
  let bodyHas = 0;
  const reader = res.body.getReader();
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    let at = 0;
    while (at < value.length) {
      if (headHas < 8) {
        const n = Math.min(8 - headHas, value.length - at);
        head.set(value.subarray(at, at + n), headHas);
        headHas += n;
        at += n;
        if (headHas === 8) {
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
        const when = new DataView(head.buffer).getFloat64(0, true);
        out.frames.push({ at: when, data: body! });
        headHas = 0;
        body = null;
      }
    }
  }
  // A frame cut short leaves its buffer for the next pull.
  if (body && size) out.spare.push(body);
  return out;
}
