// The loudness of a video as the clip timeline keeps it, and the level each
// column of the clip timeline shows from it.
//
// The window keeps the whole of a video's loudness, the way it keeps the
// clips, and draws every view from it, so a swipe or a pinch asks the Go
// side for nothing and the waveform is there in the same frame as the
// hand. Go makes it and sends it whole, when the video opens and again
// when its state says more is measured, see engine.Loudness and
// docs/decisions/2026-11-18-the-window-holds-the-loudness.md. This file
// decides nothing about the video: it reads the bytes and says how loud
// each column is.

// How long one reading is, in seconds, timed.FrameSeconds on the Go side.
export const FRAME = 0.01;

// A video's loudness: how many readings, one every FRAME seconds from the
// start of the video, and the levels they are kept in. Level 0 is every
// reading, a byte each, and each level after it is the loudest of every
// two of the one before, down to a level of one.
export interface Loudness {
  count: number;
  levels: Uint8Array[];
}

export const silent: Loudness = { count: 0, levels: [] };

// The loudness as the Go side sends it, the bytes as base64, which is how
// Wails hands a []byte over.
export function readLoudness(sent: string): Loudness {
  const text = atob(sent);
  const bytes = new Uint8Array(text.length);
  for (let i = 0; i < text.length; i++) bytes[i] = text.charCodeAt(i);
  return loudnessOf(bytes);
}

// The loudness in its bytes: the number of readings as four bytes, little
// end first, then every level one after another. Bytes that do not add up
// are no loudness rather than a wrong one.
export function loudnessOf(bytes: Uint8Array): Loudness {
  if (bytes.length < 4) return silent;
  const count = new DataView(bytes.buffer, bytes.byteOffset, 4).getUint32(0, true);
  const levels: Uint8Array[] = [];
  let at = 4;
  for (let k = count; k > 0; k = Math.ceil(k / 2)) {
    if (at + k > bytes.length) return silent;
    levels.push(bytes.subarray(at, at + k));
    at += k;
    if (k === 1) break;
  }
  return { count, levels };
}

// A reading in dB: -90 to 0 as 0 to 180, half a decibel a step.
const decibels = (step: number) => step / 2 - 90;

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
// the readings, each standing at the middle of the ten milliseconds it
// measured, or the track steps in blocks eight pixels wide. Where no
// reading falls, past the end of what is measured, a column is -Infinity
// and is not drawn at all, so it does not read as silence.
export function columnLevels(l: Loudness, from: number, to: number, columns: number): Float32Array {
  const out = new Float32Array(Math.max(columns, 0)).fill(-Infinity);
  const n = l.count;
  if (!n || !(to > from) || columns <= 0) return out;
  const scale = columns / (to - from);
  const readings = l.levels[0];
  if (FRAME * scale > 1.5) {
    for (let x = 0; x < columns; x++) {
      const at = (from + x / scale) / FRAME - 0.5;
      const i = Math.floor(at);
      if (i < -1 || i >= n) continue;
      const a = readings[Math.max(i, 0)];
      const b = readings[Math.min(i + 1, n - 1)];
      out[x] = decibels(a + (b - a) * (at - i));
    }
    return out;
  }
  for (let x = 0; x < columns; x++) {
    const first = Math.max(Math.floor((from + x / scale) / FRAME), 0);
    // A hair off the end before rounding up. A column that ends right on
    // a reading's edge works out to a boundary like 7.000000000000001,
    // and rounding that up takes in the reading after it, which fattens
    // the waveform and fills in the dips between words. The hair is far
    // smaller than any real boundary and far larger than the error.
    const last = Math.min(Math.ceil((from + (x + 1) / scale) / FRAME - 1e-9) - 1, n - 1);
    if (first > last) continue;
    out[x] = decibels(loudest(l.levels, first, last));
  }
  return out;
}
