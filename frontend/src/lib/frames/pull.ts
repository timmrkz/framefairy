// Reading a pull of frames from the Go side, see cmd/framefairy-app/
// frames.go: each frame is where it starts, 8 bytes, then the frame in
// 8-bit I420. Each frame is read straight into a buffer of its own as the
// answer arrives and handed to its VideoFrame without another copy, so a
// frame is copied once on its way in, not twice. It runs in a Worker, see
// pull.worker.ts, so it never holds up the page while a hand is on the
// clip timeline, and on the page itself where a Worker will not have it.

export type Pulled = {
  // The answer's status, 404 for a stream the Go side has closed.
  status: number;
  // The episode has ended, and why when it failed.
  end: boolean;
  error: string;
  frames: { at: number; frame: VideoFrame }[];
};

// Whether a VideoFrame can take a buffer over instead of copying it.
let transfers = true;

function frameFrom(buffer: ArrayBuffer, width: number, height: number, at: number): VideoFrame {
  const init = { format: "I420" as const, codedWidth: width, codedHeight: height, timestamp: Math.round(at * 1e6) };
  if (transfers) {
    try {
      return new VideoFrame(buffer, { ...init, transfer: [buffer] } as VideoFrameBufferInit);
    } catch {
      transfers = false;
    }
  }
  return new VideoFrame(buffer, init);
}

export async function pull(url: string, width: number, height: number): Promise<Pulled> {
  const res = await fetch(url).catch(() => null);
  if (!res) return { status: 0, end: false, error: "", frames: [] };
  const out: Pulled = {
    status: res.status,
    end: !!res.headers.get("X-Frames-End"),
    error: res.headers.get("X-Frames-Error") ?? "",
    frames: [],
  };
  if (!res.ok || !res.body) return out;
  const size = (width * height * 3) / 2;
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
        out.frames.push({ at: when, frame: frameFrom(body!, width, height, when) });
        headHas = 0;
        body = null;
      }
    }
  }
  return out;
}
