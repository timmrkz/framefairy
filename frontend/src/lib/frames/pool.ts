// The buffers frames are read into, kept once no frame holds them and
// handed to the next pull, so a play makes no new memory for its frames.
//
// A frame of the video preview is its canvas in pixels, 4 bytes each,
// about 7 MB on a Mac's screen, and a play reads 25 or more a second.
// Each one in a buffer of its own was 170 MB a second for the page to
// clean up after, and the page's cleaning stops it from drawing: in a
// play in the cloud it took about 1.3 s of every 12 s, and the refreshes
// it missed came with it, issue #251.
//
// A buffer is only ever given back by the last frame that held it, see
// Picture, so no frame's pixels change under it.
export class Pool {
  private free = new Map<number, ArrayBuffer[]>();
  private kept = 0;

  // limit is the most bytes kept, beyond which a buffer given back is let
  // go of.
  constructor(readonly limit: number) {}

  // A buffer of size bytes, or undefined when none is kept.
  take(size: number): ArrayBuffer | undefined {
    const b = this.free.get(size)?.pop();
    if (b) this.kept -= b.byteLength;
    return b;
  }

  // Up to n buffers of size bytes.
  takeUpTo(size: number, n: number): ArrayBuffer[] {
    const out: ArrayBuffer[] = [];
    while (out.length < n) {
      const b = this.take(size);
      if (!b) break;
      out.push(b);
    }
    return out;
  }

  // A buffer no frame holds any more. One that was handed to a Worker has
  // nothing in it and is passed over. Buffers of another size than the
  // one given last go first, since frames change size only when the app
  // does.
  give(b: ArrayBuffer) {
    if (b.byteLength === 0) return;
    for (const [size, list] of this.free) {
      if (size === b.byteLength) continue;
      while (list.length && this.kept + b.byteLength > this.limit) this.kept -= list.pop()!.byteLength;
    }
    if (this.kept + b.byteLength > this.limit) return;
    const list = this.free.get(b.byteLength) ?? [];
    list.push(b);
    this.free.set(b.byteLength, list);
    this.kept += b.byteLength;
  }

  // The bytes kept, for the tests.
  get bytes(): number {
    return this.kept;
  }
}

// The buffers of the video preview's frames: about a dozen frames on a
// Mac's screen, more than a play has on its way at once.
export const frames = new Pool(96 * 1024 * 1024);
