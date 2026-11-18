// The loudness of a video as the clip timeline keeps it, and the level each
// column of the clip timeline shows from it.
//
// The window keeps the whole of a video's loudness, the way it keeps the
// clips, and draws every view from it, so a swipe or a pinch asks the Go
// side for nothing and the waveform is there in the same frame as the
// hand. Go makes it and sends it whole, when the video opens and again
// when the video's state says it changed, see engine.Loudness and
// docs/decisions/2026-11-18-the-window-holds-the-loudness.md. This file
// decides nothing about the video: it reads the bytes, which say how to
// read themselves, and says how loud each column is.

// A video's loudness: how many readings, how long each is in seconds, one
// after another from the start of the video, how loud a byte is, and the
// levels the readings are kept in. Level 0 is every reading, a byte each,
// and each level after it is the loudest of every two of the one before,
// down to a level of one.
export interface Loudness {
  count: number;
  each: number;
  floor: number;
  steps: number;
  levels: Uint8Array[];
}

export const silent: Loudness = { count: 0, each: 0.01, floor: -90, steps: 2, levels: [] };

// How long the head of the bytes is, see engine.Loudness.
const HEAD = 8;

// The loudness in its bytes, as engine.Loudness makes them: a head that
// says how many readings, how long one is in microseconds, the floor in
// dB and how many steps make a decibel, then every level one after
// another. Bytes that do not add up to exactly that are no loudness
// rather than a wrong one, so the two sides cannot drift apart unseen: a
// waveform that is not drawn at all is what the walks look for.
export function loudnessOf(bytes: Uint8Array): Loudness {
  if (bytes.length < HEAD) return silent;
  const head = new DataView(bytes.buffer, bytes.byteOffset, HEAD);
  const count = head.getUint32(0, true);
  const each = head.getUint16(4, true) / 1e6;
  const floor = head.getInt8(6);
  const steps = head.getUint8(7);
  if (!each || !steps) return silent;
  const levels: Uint8Array[] = [];
  let at = HEAD;
  for (let k = count; k > 0; k = Math.ceil(k / 2)) {
    if (at + k > bytes.length) return silent;
    levels.push(bytes.subarray(at, at + k));
    at += k;
    if (k === 1) break;
  }
  if (at !== bytes.length) return silent;
  return { count, each, floor, steps, levels };
}

// The loudest reading from first to last, both included. It takes the
// levels the way a tree of maxima is read: a reading that has no partner
// in the span is taken on its own, and the rest are taken two at a time
// from the level above, so a span of a million readings is about forty.
function loudest(levels: Uint8Array[], first: number, last: number): number {
  let best = 0;
  for (let k = 0; first <= last; k++) {
    const level = levels[k];
    if (first & 1) best = Math.max(best, level[first++]);
    if (!(last & 1)) best = Math.max(best, level[last--]);
    first >>= 1;
    last = (last - 1) >> 1;
  }
  return best;
}

// The level in dB in each of columns columns of a view from from to to
// seconds, the way an editor draws a waveform: zoomed out a column is the
// loudest reading that falls in it, because a peak averaged away is a
// peak nobody can see. Zoomed in past the measurement, where one reading
// has more than a column and a half to itself, the outline runs between
// the readings, each standing at the middle of the time it measured, or
// the track steps in blocks eight pixels wide. Where no reading falls,
// past the end of what is measured, a column is -Infinity and is not
// drawn at all, so it does not read as silence.
export function columnLevels(l: Loudness, from: number, to: number, columns: number): Float32Array {
  const out = new Float32Array(Math.max(columns, 0)).fill(-Infinity);
  const n = l.count;
  if (!n || !(to > from) || columns <= 0) return out;
  const scale = columns / (to - from);
  const decibels = (step: number) => l.floor + step / l.steps;
  const readings = l.levels[0];
  if (l.each * scale > 1.5) {
    for (let x = 0; x < columns; x++) {
      const at = (from + x / scale) / l.each - 0.5;
      const i = Math.floor(at);
      if (i < -1 || i >= n) continue;
      const a = readings[Math.max(i, 0)];
      const b = readings[Math.min(i + 1, n - 1)];
      out[x] = decibels(a + (b - a) * (at - i));
    }
    return out;
  }
  for (let x = 0; x < columns; x++) {
    const first = Math.max(Math.floor((from + x / scale) / l.each), 0);
    // A hair off the end before rounding up. A column that ends right on
    // a reading's edge works out to a boundary like 7.000000000000001,
    // and rounding that up takes in the reading after it, which fattens
    // the waveform and fills in the dips between words. The hair is far
    // smaller than any real boundary and far larger than the error.
    const last = Math.min(Math.ceil((from + (x + 1) / scale) / l.each - 1e-9) - 1, n - 1);
    if (first > last) continue;
    out[x] = decibels(loudest(l.levels, first, last));
  }
  return out;
}

// Asks the Go side for a video's loudness, /frames/levels.
export async function askLoudness(path: string): Promise<Loudness> {
  const r = await fetch(`/frames/levels?${new URLSearchParams({ path })}`);
  if (!r.ok) throw new Error(`the loudness was answered with ${r.status}`);
  return loudnessOf(new Uint8Array(await r.arrayBuffer()));
}

// The loudness one clip timeline holds. It is read when the video's state
// says it changed, by the stamp Go sends with it, EpisodeStatus.loudness,
// and never otherwise: when to read is Go's to say. One reading is out at
// a time, and a stamp that changed while it was out is read once it is
// back. A clip timeline out of the page, a workspace put away, reads
// nothing until it is back, and then reads the newest stamp it was told.
export class HeldLoudness {
  private readFor = "";
  private wanted = { path: "", stamp: "", shown: false };
  private out = false;

  constructor(
    private got: (path: string, l: Loudness) => void,
    private ask: (path: string) => Promise<Loudness> = askLoudness,
  ) {}

  // What the clip timeline shows now: the video, the stamp of its
  // loudness, and whether it is in the page.
  want(path: string, stamp: string, shown: boolean) {
    this.wanted = { path, stamp, shown };
    void this.read();
  }

  private async read() {
    const { path, stamp, shown } = this.wanted;
    const key = `${path}\n${stamp}`;
    if (!path || !shown || this.out || key === this.readFor) return;
    this.out = true;
    this.readFor = key;
    try {
      const l = await this.ask(path);
      if (this.wanted.path === path) this.got(path, l);
    } catch {
      // Not there, or the video is gone: what it had stays, the state says
      // why elsewhere, and the next stamp asks again. Asking again at once
      // would ask without end.
    } finally {
      this.out = false;
    }
    void this.read();
  }
}
