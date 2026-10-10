// A frame of the video preview: its pixels as ffmpeg made them on the Go
// side, red, green, blue and an opaque alpha with 8 bits each, kept as
// the bytes they came as, see pull.ts. The bytes are never changed once
// they are here, so a frame shared by two holders is the same bytes, not
// a copy. It is the app's own frame rather than WebCodecs' VideoFrame,
// because the WebGPU canvas takes bytes on every system, see screen.ts,
// and VideoFrame holds nothing brighter than white.

export class Picture {
  private bytes: Uint8Array | null;

  constructor(
    data: ArrayBuffer | Uint8Array,
    readonly width: number,
    readonly height: number,
    // When the frame starts, in microseconds, as a VideoFrame's timestamp.
    readonly timestamp: number,
  ) {
    this.bytes = data instanceof Uint8Array ? data : new Uint8Array(data);
    if (this.bytes.length !== width * height * 4) throw new Error(`a frame of ${width} by ${height} needs ${width * height * 4} bytes, not ${this.bytes.length}`);
  }

  // The pixels, 4 bytes each, row after row, or null once closed.
  get data(): Uint8Array | null {
    return this.bytes;
  }

  get closed(): boolean {
    return this.bytes === null;
  }

  // The same frame for another holder, under another time if need be.
  clone(timestamp = this.timestamp): Picture {
    if (!this.bytes) throw new Error("the frame is closed");
    return new Picture(this.bytes, this.width, this.height, timestamp);
  }

  // Lets go of the pixels. The other holders of the same frame keep them.
  close() {
    this.bytes = null;
  }

  // The pixels of a piece of the frame, as a 2D canvas takes them. What
  // lies outside the frame is left clear.
  imageData(x: number, y: number, w: number, h: number): ImageData {
    const out = new ImageData(w, h);
    const src = this.bytes;
    if (!src) return out;
    for (let row = 0; row < h; row++) {
      const sy = y + row;
      if (sy < 0 || sy >= this.height) continue;
      const from = Math.max(0, x);
      const to = Math.min(this.width, x + w);
      if (to <= from) continue;
      out.data.set(src.subarray((sy * this.width + from) * 4, (sy * this.width + to) * 4), (row * w + from - x) * 4);
    }
    return out;
  }
}
