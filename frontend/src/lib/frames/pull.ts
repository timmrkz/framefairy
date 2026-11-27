// Reading a pull of frames from the Go side, see cmd/framefairy-app/
// frames.go: each frame is where it starts, 8 bytes, and for a play what
// more the Go side says of it, 8 bytes each, see servePlay, then the frame in
// colours, RGBA with 8 bits each and an opaque alpha, which ffmpeg made
// from the file's tags, then what it is dressed with, its dress as JSON
// and the layer of captions over it, each after its length in 4 bytes,
// see writeDress. Each frame is read straight into a buffer of its
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
  // nobody pulled from it: no end of the play and no failure.
  closed: boolean;
  // How the frames code their brightness, see light.ts.
  light: Light;
  // A look's frame is the one asked for, not one on the way to it, see
  // engine.Seen.
  exact: boolean;
  // A look's frame was kept on the Go side, nothing decoded for it.
  kept: boolean;
  // The Go side holds frames around a look's place, see AppFrames.near.
  near: boolean;
  // The frames of the words an Ahead asked for that the app holds
  // already, which it keeps as used last, see AppFrames.aheadNow.
  keep: number[];
  // Each frame's pixels, see Picture, with its dress as the Go side wrote
  // it and the layer of captions over it, empty where there is none. They
  // are bytes rather than a Picture so they can come from a Worker, see
  // pictures.
  frames: { at: number; data: ArrayBuffer; stamp: number[]; dress: string; layer: ArrayBuffer | null }[];
  // The buffers handed to the pull that it had no frame for, to be kept
  // for the next, see pool.ts.
  spare: ArrayBuffer[];
};

// The frames of a pull as Pictures of width by height, each with its
// dress and its layer as they came.
export function pictures(
  got: Pulled,
  width: number,
  height: number,
): { at: number; frame: Picture; stamp: number[]; dress: string; layer: ArrayBuffer | null }[] {
  return got.frames.map(({ at, data, stamp, dress, layer }) => ({
    at,
    stamp,
    dress,
    layer,
    frame: new Picture(data, width, height, Math.round(at * 1e6), got.light),
  }));
}

// The frames are colours ready to draw, see framewire.Picture. Nothing
// here decides colour. Each frame is read into one of the spare buffers
// while there are any of its size, and into a new one after. head is how
// many bytes come before each frame.
export async function pull(url: string, width: number, height: number, spare: ArrayBuffer[] = [], head = 8): Promise<Pulled> {
  const res = await fetch(url).catch(() => null);
  if (!res) return { status: 0, end: false, error: "", closed: false, light: "", exact: false, kept: false, near: false, keep: [], frames: [], spare };
  const out: Pulled = {
    status: res.status,
    end: !!res.headers.get("X-Frames-End"),
    error: res.headers.get("X-Frames-Error") ?? "",
    closed: !!res.headers.get("X-Frames-Closed"),
    light: lightOf(res.headers.get("X-Frames-Light")),
    exact: !!res.headers.get("X-Frames-Exact"),
    kept: !!res.headers.get("X-Frames-Kept"),
    near: !!res.headers.get("X-Frames-Near"),
    keep: (res.headers.get("X-Frames-Keep") ?? "")
      .split(",")
      .filter((x) => x !== "")
      .map(Number)
      .filter((x) => Number.isInteger(x) && x >= 0),
    frames: [],
    spare: [],
  };
  const size = width * height * 4;
  const fits = spare.filter((b) => b.byteLength === size);
  out.spare = fits;
  if (!res.ok || !res.body) return out;
  // What is being read: the head, the pixels, the length of the dress,
  // the dress, the length of the layer, the layer.
  let phase = 0;
  let has = 0;
  const before = new Uint8Array(head);
  const length = new Uint8Array(4);
  let body: ArrayBuffer | null = null;
  let dress = new Uint8Array(0);
  let layer = new Uint8Array(0);
  const text = new TextDecoder();
  // The part being read, and how long it is.
  const part = (): Uint8Array =>
    phase === 0 ? before : phase === 1 ? new Uint8Array(body!) : phase === 2 || phase === 4 ? length : phase === 3 ? dress : layer;
  // Moves on once a part is read, past a dress or a layer of nothing.
  const next = () => {
    has = 0;
    for (;;) {
      if (phase === 0) body = fits.pop() ?? new ArrayBuffer(size);
      else if (phase === 2) dress = new Uint8Array(new DataView(length.buffer).getUint32(0, true));
      else if (phase === 4) layer = new Uint8Array(new DataView(length.buffer).getUint32(0, true));
      else if (phase === 5) {
        const v = new DataView(before.buffer);
        const stamp: number[] = [];
        for (let i = 8; i + 8 <= head; i += 8) stamp.push(v.getFloat64(i, true));
        out.frames.push({
          at: v.getFloat64(0, true),
          data: body!,
          stamp,
          dress: text.decode(dress),
          layer: layer.length ? (layer.buffer as ArrayBuffer) : null,
        });
        body = null;
        phase = 0;
        return;
      }
      phase++;
      if (part().length) return;
    }
  };
  const reader = res.body.getReader();
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    let at = 0;
    while (at < value.length) {
      const into = part();
      const n = Math.min(into.length - has, value.length - at);
      into.set(value.subarray(at, at + n), has);
      has += n;
      at += n;
      if (has === into.length) next();
    }
  }
  if (body && size) out.spare.push(body);
  return out;
}
