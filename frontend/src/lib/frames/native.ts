// The picture of a file the webview cannot decode, decoded by the system's
// own decoder in the app's own process, VideoToolbox on the Mac, see
// engine/pictures.go. The page reads the file and feeds the samples as it
// would feed a VideoDecoder, the decoder stays open for as long as the
// episode does, and nothing is started or read again for a jump: the
// fastest way to a frame there is. Where there is no such decoder, the Go
// side's ffmpeg streams are used instead, see app.ts.
//
// The samples go to the Go side in batches, one request for everything fed
// in a moment, and the frames the queue will draw come back scaled to the
// canvas, in 8-bit NV12.

// Opens a decoder, or answers null where the system has none for this
// track. codec is the file's name for it, avc1 or hvc1 and the like.
export async function openNative(
  codec: string,
  config: Uint8Array | undefined,
  coded: { width: number; height: number },
  size: { width: number; height: number },
): Promise<string | null> {
  if (!config) return null;
  const q = new URLSearchParams({
    codec: codec.split(".")[0],
    cw: String(coded.width),
    ch: String(coded.height),
    w: String(size.width),
    h: String(size.height),
  });
  try {
    const res = await fetch(`/frames/native?${q}`, { method: "POST", body: new Uint8Array(config) });
    if (!res.ok) return null;
    const j = (await res.json()) as { id?: string };
    return j.id ?? null;
  } catch {
    return null;
  }
}

type Fed = { timestamp: number; key: boolean; keep: boolean; data: Uint8Array; rank: number };

// A picture decoder for the frame queue, with the calls it makes of a
// VideoDecoder. Like AppPictures, it puts out only the frames the queue
// will draw, in the order they are shown.
export class NativePictures {
  state: "configured" | "closed" = "configured";
  private waiting: Fed[] = [];
  private sending = false;
  // Asked for and not come yet.
  private asked = 0;
  private held = new Map<number, VideoFrame | null>();
  private next = -1;
  private generation = 0;
  private flushes: { done: () => void; fail: (e: Error) => void }[] = [];

  constructor(
    private id: string,
    private width: number,
    private height: number,
    // The picture's colours, in full range, which is what the system's
    // decoder is asked for, see engine.Pictures.
    private colour: VideoColorSpaceInit,
    private output: (f: VideoFrame) => void,
    private dequeue: () => void,
    private failed: (why: string) => void,
  ) {}

  get decodeQueueSize() {
    return this.asked;
  }

  decode(timestamp: number, key: boolean, data: Uint8Array, rank: number, wanted: boolean, first: number) {
    if (this.state !== "configured") return;
    if (wanted) {
      if (this.next < 0) this.next = first;
      this.asked++;
    }
    this.waiting.push({ timestamp, key, keep: wanted, data, rank });
    if (!this.sending) {
      this.sending = true;
      queueMicrotask(() => void this.send());
    }
  }

  // Everything fed since the last request, in one: for each sample its
  // timestamp, whether to keep its frame, its length and its bytes.
  private async send() {
    while (this.waiting.length && this.state === "configured") {
      const batch = this.waiting;
      this.waiting = [];
      const generation = this.generation;
      const length = batch.reduce((n, f) => n + 13 + f.data.length, 0);
      const body = new Uint8Array(length);
      const dv = new DataView(body.buffer);
      let at = 0;
      for (const f of batch) {
        dv.setFloat64(at, f.timestamp, true);
        dv.setUint8(at + 8, (f.key ? 1 : 0) | (f.keep ? 2 : 0));
        dv.setUint32(at + 9, f.data.length, true);
        body.set(f.data, at + 13);
        at += 13 + f.data.length;
      }
      let answer: ArrayBuffer | null = null;
      let why = "";
      try {
        const res = await fetch(`/frames/decode?id=${this.id}`, { method: "POST", body });
        if (res.ok) answer = await res.arrayBuffer();
        else why = (await res.text()).trim() || `it answered ${res.status}`;
      } catch (e) {
        why = String(e);
      }
      if (generation !== this.generation || this.state !== "configured") continue;
      if (!answer) {
        this.failed(why);
        return;
      }
      const ranks = new Map(batch.map((f) => [f.timestamp, f.rank]));
      const size = (this.width * this.height * 3) / 2;
      const kept = new Set<number>();
      for (let off = 0; off + 8 + size <= answer.byteLength; off += 8 + size) {
        const timestamp = new DataView(answer, off, 8).getFloat64(0, true);
        const rank = ranks.get(timestamp);
        if (rank === undefined) continue;
        kept.add(rank);
        this.held.set(
          rank,
          new VideoFrame(new Uint8Array(answer, off + 8, size), {
            format: "NV12",
            codedWidth: this.width,
            codedHeight: this.height,
            timestamp,
            colorSpace: this.colour,
          }),
        );
      }
      // A sample asked for that made no frame is put out as nothing, in its
      // place, so the ones after it are not held for it.
      for (const f of batch) {
        if (!f.keep) continue;
        this.asked--;
        if (!kept.has(f.rank)) this.held.set(f.rank, null);
      }
      this.putOut(false);
      this.dequeue();
      this.settle();
    }
    this.sending = false;
  }

  private putOut(all: boolean) {
    if (all) {
      for (const r of [...this.held.keys()].sort((a, b) => a - b)) {
        const f = this.held.get(r);
        this.held.delete(r);
        if (f) this.output(f);
      }
      return;
    }
    while (this.held.has(this.next)) {
      const f = this.held.get(this.next);
      this.held.delete(this.next);
      this.next++;
      if (f) this.output(f);
    }
  }

  flush(): Promise<void> {
    return new Promise((done, fail) => {
      this.flushes.push({ done, fail });
      this.settle();
    });
  }

  private settle() {
    if (this.asked > 0 || this.waiting.length || !this.flushes.length) return;
    this.putOut(true);
    const was = this.flushes;
    this.flushes = [];
    for (const f of was) f.done();
  }

  reset() {
    this.generation++;
    this.asked = 0;
    this.next = -1;
    this.waiting = [];
    for (const f of this.held.values()) f?.close();
    this.held.clear();
    const was = this.flushes;
    this.flushes = [];
    for (const f of was) f.fail(new DOMException("reset", "AbortError"));
  }

  close() {
    this.reset();
    this.state = "closed";
    void fetch(`/frames/close?id=${this.id}`).catch(() => {});
  }
}
