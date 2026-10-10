// Reading a pull of frames from the Go side, see cmd/framefairy-app/
// frames.go: each frame is where it starts, 8 bytes, then the frame in
// colours, RGBA with 8 bits each and an opaque alpha, which ffmpeg made
// from the file's tags. Each frame is read straight into a buffer of its
// own as the answer arrives and kept as it is, so a frame is copied once
// on its way in, not twice. It runs in a Worker, see pull.worker.ts, so
// it never holds up the page while a hand is on the clip timeline, and on
// the page itself where a Worker will not have it.

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
  // first frame: "started,opened,first" in milliseconds, see
  // engine.PreviewTimes.
  times: string;
  // Each frame's pixels, see Picture. They are bytes rather than a
  // Picture so they can come from a Worker, see pictures.
  frames: { at: number; data: ArrayBuffer }[];
};

// The frames of a pull as Pictures of width by height.
export function pictures(got: Pulled, width: number, height: number): { at: number; frame: Picture }[] {
  return got.frames.map(({ at, data }) => ({ at, frame: new Picture(data, width, height, Math.round(at * 1e6)) }));
}

// The frames are colours ready to draw, see engine.PreviewFrames. Nothing
// here decides colour.
export async function pull(url: string, width: number, height: number): Promise<Pulled> {
  const res = await fetch(url).catch(() => null);
  if (!res) return { status: 0, end: false, error: "", closed: false, times: "", frames: [] };
  const out: Pulled = {
    status: res.status,
    end: !!res.headers.get("X-Frames-End"),
    error: res.headers.get("X-Frames-Error") ?? "",
    closed: !!res.headers.get("X-Frames-Closed"),
    times: res.headers.get("X-Frames-Times") ?? "",
    frames: [],
  };
  if (!res.ok || !res.body) return out;
  const size = width * height * 4;
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
          body = new ArrayBuffer(size);
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
  return out;
}
