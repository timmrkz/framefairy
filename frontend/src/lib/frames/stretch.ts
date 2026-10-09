// The sound of a play at another speed, kept at its own pitch, the way
// YouTube plays it: a voice at twice the speed is quicker, not higher.
// Played faster as it is, the sound card raises the pitch with the speed.
//
// It is WSOLA, waveform similarity overlap-add. The sound is cut into
// windows 30 ms long that overlap by half, taken from the input a speed's
// worth of half windows apart and laid down in the output half a window
// apart. Each window is moved a little, a quarter of a window either way,
// to where it best carries on the window before, so the waves line up and
// nothing beats or clicks where two windows meet.
//
// It works on a stream: the chunks of a play go in one after another, in
// order, and the output comes out as soon as no later window can add to
// it. Output sample j stands for input sample start + j × speed, so the
// play's clock and the sound agree, within a window.

export type Stretched = { at: number; length: number; data: Float32Array<ArrayBuffer>[] };

export class Stretch {
  // The next input sample it expects, so a chunk that does not carry on
  // from the one before is known.
  next: number;
  private readonly n: number;
  private readonly hop: number;
  private readonly reach: number;
  private readonly window: Float32Array;
  // The input kept, per channel, from input sample base on.
  private input: Float32Array[];
  private base: number;
  private kept = 0;
  // The output not yet given out, per channel, from output sample done on.
  private output: Float32Array[];
  private done = 0;
  // The next window, and where in the input the last one was taken from.
  private k = 0;
  private last = 0;

  constructor(
    private readonly channels: number,
    rate: number,
    readonly speed: number,
    readonly start: number,
  ) {
    this.next = start;
    this.base = start;
    this.hop = Math.max(16, Math.round(rate * 0.015));
    this.n = this.hop * 2;
    this.reach = Math.floor(this.hop / 2);
    this.window = new Float32Array(this.n);
    for (let i = 0; i < this.n; i++) this.window[i] = 0.5 - 0.5 * Math.cos((2 * Math.PI * i) / this.n);
    this.input = Array.from({ length: channels }, () => new Float32Array(this.n * 8));
    this.output = Array.from({ length: channels }, () => new Float32Array(this.n * 2));
  }

  // A chunk in, and whatever output is finished, from output sample at.
  push(data: Float32Array[], length: number): Stretched | null {
    this.take(data, length);
    this.next += length;
    while (this.windowReady()) this.lay();
    const ready = this.k * this.hop - this.done;
    if (ready <= 0) return null;
    const out: Stretched = { at: this.done, length: ready, data: [] };
    for (let c = 0; c < this.channels; c++) {
      out.data.push(this.output[c].slice(0, ready));
      this.output[c].copyWithin(0, ready);
      this.output[c].fill(0, this.output[c].length - ready);
    }
    this.done += ready;
    this.trim();
    return out;
  }

  // Where window k is taken from before it is moved.
  private nominal(k: number): number {
    return this.start + Math.round(k * this.hop * this.speed);
  }

  // Whether everything window k may be taken from is in.
  private windowReady(): boolean {
    const end = this.base + this.kept;
    if (this.k === 0) return end >= this.start + this.n;
    const reach = this.nominal(this.k) + this.reach + this.n;
    return end >= Math.max(reach, this.last + this.hop + this.n);
  }

  private take(data: Float32Array[], length: number) {
    const need = this.kept + length;
    if (need > this.input[0].length) {
      const size = Math.max(need, this.input[0].length * 2);
      this.input = this.input.map((old) => {
        const grown = new Float32Array(size);
        grown.set(old.subarray(0, this.kept));
        return grown;
      });
    }
    for (let c = 0; c < this.channels; c++) this.input[c].set(data[c].subarray(0, length), this.kept);
    this.kept += length;
  }

  private lay() {
    let from = this.nominal(this.k);
    if (this.k > 0) from = this.best(from);
    const outAt = this.k * this.hop - this.done;
    if (outAt + this.n > this.output[0].length) {
      this.output = this.output.map((old) => {
        const grown = new Float32Array((outAt + this.n) * 2);
        grown.set(old);
        return grown;
      });
    }
    const i0 = from - this.base;
    for (let c = 0; c < this.channels; c++) {
      const src = this.input[c];
      const dst = this.output[c];
      for (let i = 0; i < this.n; i++) dst[outAt + i] += src[i0 + i] * this.window[i];
    }
    this.last = from;
    this.k++;
  }

  // The place near from whose sound best carries on the window before it:
  // the input that came right after that window's first half, matched
  // against the places a little either way, on the first channel, every
  // fourth sample, which is plenty to tell waves apart.
  private best(from: number): number {
    const x = this.input[0];
    const t = this.last + this.hop - this.base;
    const lo = Math.max(from - this.reach, this.base, this.start);
    const hi = from + this.reach;
    let best = from;
    let most = -Infinity;
    for (let p = lo; p <= hi; p += 2) {
      const a = p - this.base;
      let sum = 0;
      for (let i = 0; i < this.n; i += 4) sum += x[a + i] * x[t + i];
      if (sum > most) {
        most = sum;
        best = p;
      }
    }
    return best;
  }

  // Drops the input no window can be taken from any more.
  private trim() {
    const keepFrom = Math.min(this.nominal(this.k) - this.reach, this.last + this.hop);
    const drop = keepFrom - this.base;
    if (drop < this.n * 4) return;
    for (const ch of this.input) ch.copyWithin(0, drop, this.kept);
    this.kept -= drop;
    this.base += drop;
  }
}
