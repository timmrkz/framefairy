// The index of an episode file, read once: where every sample of the
// picture and the sound lies in the file, when it is shown, and how the
// decoders are to be set up for it. The video preview reads the samples
// straight from the file with this, see queue.ts, so the file is never
// handed to a <video> element.
//
// MP4 and MOV keep the index in one box, moov, which is either at the
// start of the file or at its end. Everything here works on that box
// alone, so a four hour episode is read with two or three small range
// requests before the first frame, whatever its size.
//
// This is our own parser rather than mp4box.js. It reads the dozen boxes
// playback needs and nothing else, and it keeps the sample tables as typed
// arrays: four hours at 25 frames a second with AAC beside it is over a
// million samples, which mp4box.js holds as one object each, and which
// here is about 20 MB of flat numbers. Fragmented MP4, where the index is
// spread through the file in moof boxes, is not read: a file like that
// needs the playback copy of the episode.
//
// The file is the person's own, but it is still input from outside, so
// every read is checked against the box it is in and a broken file ends
// in an error that says so, never in a read past the end.

export type VideoTrack = {
  kind: "video";
  // What VideoDecoder.configure takes, apart from the hardware hints.
  codec: string;
  description?: Uint8Array;
  width: number;
  height: number;
  samples: Samples;
  // How long a frame lasts, in seconds, the most common step between two
  // frames. A file that changes its frame rate on the way still has one.
  frame: number;
};

export type AudioTrack = {
  kind: "audio";
  codec: string;
  description?: Uint8Array;
  sampleRate: number;
  channels: number;
  samples: Samples;
};

// The samples of one track, in the order they are decoded. Times are in
// the track's own ticks, already moved by its edit list, so a time of zero
// is the start of the episode. time() gives seconds.
export type Samples = {
  count: number;
  timescale: number;
  offset: Float64Array;
  size: Uint32Array;
  // When the sample is shown, in ticks, and how long it lasts.
  pts: Float64Array;
  duration: Float64Array;
  // Whether decoding can start at this sample.
  key: Uint8Array;
  // The samples in the order they are shown: order[r] is the sample shown
  // r-th, and rank[i] is where sample i comes in that order. The same as
  // the decode order for sound, and for a picture without B-frames.
  order: Uint32Array;
  rank: Uint32Array;
};

export type Movie = {
  // Seconds, the longer of the two tracks.
  duration: number;
  video?: VideoTrack;
  audio?: AudioTrack;
};

export class MP4Error extends Error {}

function fail(what: string): never {
  throw new MP4Error(what);
}

// A window on the bytes of a box, with reads that cannot leave it.
class View {
  private dv: DataView;
  constructor(
    readonly bytes: Uint8Array,
    readonly from = 0,
    readonly to = bytes.length,
  ) {
    this.dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  }
  private at(at: number, n: number): number {
    if (at < this.from || at + n > this.to) fail("a box is shorter than what it says is in it");
    return at;
  }
  u8(at: number): number {
    return this.dv.getUint8(this.at(at, 1));
  }
  u16(at: number): number {
    return this.dv.getUint16(this.at(at, 2));
  }
  i16(at: number): number {
    return this.dv.getInt16(this.at(at, 2));
  }
  u32(at: number): number {
    return this.dv.getUint32(this.at(at, 4));
  }
  i32(at: number): number {
    return this.dv.getInt32(this.at(at, 4));
  }
  u64(at: number): number {
    const hi = this.u32(at);
    const lo = this.u32(at + 4);
    const v = hi * 2 ** 32 + lo;
    if (v > Number.MAX_SAFE_INTEGER) fail("a number in the file is too large");
    return v;
  }
  i64(at: number): number {
    const hi = this.i32(at);
    const lo = this.u32(at + 4);
    return hi * 2 ** 32 + lo;
  }
  type(at: number): string {
    this.at(at, 4);
    return String.fromCharCode(this.bytes[at], this.bytes[at + 1], this.bytes[at + 2], this.bytes[at + 3]);
  }
  slice(from: number, to: number): Uint8Array {
    this.at(from, to - from);
    return this.bytes.slice(from, to);
  }
  within(from: number, to: number): View {
    if (from < this.from || to > this.to || from > to) fail("a box runs past the box it is in");
    return new View(this.bytes, from, to);
  }
}

type Box = { type: string; start: number; body: number; end: number };

// The boxes one after another in [from, to). A box of size zero runs to
// the end, the way the last box of a file may.
function* boxesIn(v: View, from = v.from, to = v.to): Generator<Box> {
  let at = from;
  while (at + 8 <= to) {
    let size = v.u32(at);
    const type = v.type(at + 4);
    let body = at + 8;
    if (size === 1) {
      size = v.u64(at + 8);
      body = at + 16;
    } else if (size === 0) {
      size = to - at;
    }
    if (size < body - at || at + size > to) fail(`the ${type} box runs past its end`);
    yield { type, start: at, body, end: at + size };
    at += size;
  }
}

function child(v: View, parent: { body: number; end: number }, type: string): Box | undefined {
  for (const b of boxesIn(v, parent.body, parent.end)) if (b.type === type) return b;
  return undefined;
}

function path(v: View, parent: { body: number; end: number }, ...types: string[]): Box | undefined {
  let at: { body: number; end: number } | undefined = parent;
  let found: Box | undefined;
  for (const t of types) {
    found = at ? child(v, at, t) : undefined;
    if (!found) return undefined;
    at = found;
  }
  return found;
}

// Where moov is, from the top of the file. read(from, to) answers with the
// bytes [from, to), and size is the length of the file. The first read
// takes the start of the file, which holds moov whenever the file was
// written for streaming, and from there only box headers are read until
// moov is found.
export async function findMoov(
  read: (from: number, to: number) => Promise<Uint8Array>,
  size: number,
  first = 1 << 16,
): Promise<Uint8Array> {
  let head = await read(0, Math.min(first, size));
  let headFrom = 0;
  let at = 0;
  for (let guard = 0; at + 8 <= size && guard < 10000; guard++) {
    if (at + 16 > headFrom + head.length) {
      headFrom = at;
      head = await read(at, Math.min(at + 16, size));
    }
    const v = new View(head);
    const rel = at - headFrom;
    let boxSize = v.u32(rel);
    const type = v.type(rel + 4);
    if (boxSize === 1) boxSize = v.u64(rel + 8);
    else if (boxSize === 0) boxSize = size - at;
    if (boxSize < 8) fail("the file is not an MP4 or MOV");
    if (type === "moof") fail("the file is a fragmented MP4, which the video preview cannot read");
    if (type === "moov") {
      if (at + boxSize > size) fail("the moov box runs past the end of the file");
      if (rel + boxSize <= head.length) return head.subarray(rel, rel + boxSize);
      return read(at, at + boxSize);
    }
    at += boxSize;
  }
  fail("the file has no moov box, so it is not an MP4 or MOV it can read");
}

// The whole index, from the bytes of moov.
export function parseMoov(bytes: Uint8Array): Movie {
  const v = new View(bytes);
  const moov = [...boxesIn(v)].find((b) => b.type === "moov");
  if (!moov) fail("this is not a moov box");
  const mvhd = child(v, moov, "mvhd");
  if (!mvhd) fail("the file has no mvhd box");
  const movieScale = v.u8(mvhd.body) === 1 ? v.u32(mvhd.body + 20) : v.u32(mvhd.body + 12);
  if (!movieScale) fail("the movie has no time scale");
  const movie: Movie = { duration: 0 };
  for (const trak of boxesIn(v, moov.body, moov.end)) {
    if (trak.type !== "trak") continue;
    const track = parseTrak(v, trak, movieScale);
    if (!track) continue;
    const end = track.samples.count
      ? (track.samples.pts[track.samples.order[track.samples.count - 1]] +
          track.samples.duration[track.samples.order[track.samples.count - 1]]) /
        track.samples.timescale
      : 0;
    movie.duration = Math.max(movie.duration, end);
    if (track.kind === "video" && !movie.video) movie.video = track;
    if (track.kind === "audio" && !movie.audio) movie.audio = track;
  }
  if (!movie.video && !movie.audio) fail("the file has neither a picture nor sound it can read");
  return movie;
}

function parseTrak(v: View, trak: Box, movieScale: number): VideoTrack | AudioTrack | undefined {
  const tkhd = child(v, trak, "tkhd");
  // A track that is switched off, like a second camera angle kept beside
  // the first, is not played.
  if (tkhd && (v.u32(tkhd.body) & 1) === 0) return undefined;
  const mdia = child(v, trak, "mdia");
  if (!mdia) return undefined;
  const hdlr = child(v, mdia, "hdlr");
  const mdhd = child(v, mdia, "mdhd");
  const stbl = path(v, mdia, "minf", "stbl");
  if (!hdlr || !mdhd || !stbl) return undefined;
  const handler = v.type(hdlr.body + 8);
  if (handler !== "vide" && handler !== "soun") return undefined;
  const timescale = v.u8(mdhd.body) === 1 ? v.u32(mdhd.body + 20) : v.u32(mdhd.body + 12);
  if (!timescale) fail("a track has no time scale");
  const stsd = child(v, stbl, "stsd");
  if (!stsd) fail("a track has no sample description");
  // The first description. A file whose description changes on the way
  // is very rare and is played with the first.
  const entry = [...boxesIn(v, stsd.body + 8, stsd.end)][0];
  if (!entry) fail("a track has no sample description");
  const shift = editShift(v, trak, movieScale, timescale);
  const samples = parseSamples(v, stbl, shift, timescale, handler === "vide");
  if (handler === "vide") return videoEntry(v, entry, samples);
  return audioEntry(v, entry, samples);
}

// How far the edit list moves the track, in the track's ticks. The usual
// edit list says where the track starts: the priming of AAC and the
// pre-skip of Opus are cut off the front of the sound, and a picture with
// B-frames is moved back by the delay they add, so the first frame shows
// at zero. An empty edit first delays the track.
function editShift(v: View, trak: Box, movieScale: number, timescale: number): number {
  const elst = path(v, trak, "edts", "elst");
  if (!elst) return 0;
  const version = v.u8(elst.body);
  const count = v.u32(elst.body + 4);
  const step = version === 1 ? 20 : 12;
  let delay = 0;
  for (let i = 0; i < count; i++) {
    const at = elst.body + 8 + i * step;
    const duration = version === 1 ? v.u64(at) : v.u32(at);
    const mediaTime = version === 1 ? v.i64(at + 8) : v.i32(at + 4);
    if (mediaTime === -1) {
      delay += (duration / movieScale) * timescale;
      continue;
    }
    return Math.round(delay) - mediaTime;
  }
  return Math.round(delay);
}

function parseSamples(v: View, stbl: Box, shift: number, timescale: number, reorder: boolean): Samples {
  const stsz = child(v, stbl, "stsz");
  if (!stsz) fail(child(v, stbl, "stz2") ? "compact sample sizes are not read" : "a track has no sample sizes");
  const fixed = v.u32(stsz.body + 4);
  const count = v.u32(stsz.body + 8);
  if (count > 50_000_000) fail("a track has more samples than any episode");
  const size = new Uint32Array(count);
  for (let i = 0; i < count; i++) size[i] = fixed || v.u32(stsz.body + 12 + i * 4);

  // Where each chunk starts, and how many samples each chunk holds.
  const stco = child(v, stbl, "stco");
  const co64 = child(v, stbl, "co64");
  const chunks = stco ?? co64;
  if (!chunks) fail("a track has no chunk offsets");
  const chunkCount = v.u32(chunks.body + 4);
  const chunkAt = (c: number) => (co64 ? v.u64(chunks.body + 8 + c * 8) : v.u32(chunks.body + 8 + c * 4));
  const stsc = child(v, stbl, "stsc");
  if (!stsc) fail("a track has no sample to chunk table");
  const runs = v.u32(stsc.body + 4);
  const offset = new Float64Array(count);
  let sample = 0;
  for (let r = 0; r < runs && sample < count; r++) {
    const first = v.u32(stsc.body + 8 + r * 12) - 1;
    const perChunk = v.u32(stsc.body + 12 + r * 12);
    const last = r + 1 < runs ? v.u32(stsc.body + 8 + (r + 1) * 12) - 1 : chunkCount;
    if (first < 0 || last > chunkCount || last < first) fail("the sample to chunk table is broken");
    for (let c = first; c < last && sample < count; c++) {
      let at = chunkAt(c);
      for (let k = 0; k < perChunk && sample < count; k++) {
        offset[sample] = at;
        at += size[sample];
        sample++;
      }
    }
  }
  if (sample < count) fail("the chunks hold fewer samples than the track has");

  // When each sample is decoded, then when it is shown.
  const stts = child(v, stbl, "stts");
  if (!stts) fail("a track has no time to sample table");
  const duration = new Float64Array(count);
  const pts = new Float64Array(count);
  let i = 0;
  let dts = 0;
  const sttsCount = v.u32(stts.body + 4);
  for (let e = 0; e < sttsCount && i < count; e++) {
    const n = v.u32(stts.body + 8 + e * 8);
    const delta = v.u32(stts.body + 12 + e * 8);
    for (let k = 0; k < n && i < count; k++, i++) {
      pts[i] = dts;
      duration[i] = delta;
      dts += delta;
    }
  }
  if (i < count) fail("the time to sample table is shorter than the track");
  const ctts = child(v, stbl, "ctts");
  if (ctts) {
    const signed = v.u8(ctts.body) === 1;
    const n = v.u32(ctts.body + 4);
    let s = 0;
    for (let e = 0; e < n && s < count; e++) {
      const run = v.u32(ctts.body + 8 + e * 8);
      const off = signed ? v.i32(ctts.body + 12 + e * 8) : v.u32(ctts.body + 12 + e * 8);
      for (let k = 0; k < run && s < count; k++, s++) pts[s] += off;
    }
  }
  for (let s = 0; s < count; s++) pts[s] += shift;

  const key = new Uint8Array(count);
  const stss = child(v, stbl, "stss");
  if (stss) {
    const n = v.u32(stss.body + 4);
    for (let e = 0; e < n; e++) {
      const s = v.u32(stss.body + 8 + e * 4) - 1;
      if (s >= 0 && s < count) key[s] = 1;
    }
  } else {
    key.fill(1);
  }

  const order = new Uint32Array(count);
  for (let s = 0; s < count; s++) order[s] = s;
  if (reorder && ctts) order.sort((a, b) => pts[a] - pts[b] || a - b);
  const rank = new Uint32Array(count);
  for (let r = 0; r < count; r++) rank[order[r]] = r;
  return { count, timescale, offset, size, pts, duration, key, order, rank };
}

// The most common frame length, so a file whose rate wanders still has a
// rate to play at.
function commonFrame(s: Samples): number {
  const seen = new Map<number, number>();
  let best = 0;
  let bestN = 0;
  const step = Math.max(1, Math.floor(s.count / 2000));
  for (let i = 0; i < s.count; i += step) {
    const d = s.duration[i];
    const n = (seen.get(d) ?? 0) + 1;
    seen.set(d, n);
    if (n > bestN) [best, bestN] = [d, n];
  }
  return best > 0 ? best / s.timescale : 1 / 25;
}

function hex2(n: number): string {
  return n.toString(16).padStart(2, "0");
}

function videoEntry(v: View, entry: Box, samples: Samples): VideoTrack {
  const width = v.u16(entry.body + 24);
  const height = v.u16(entry.body + 26);
  const inner = { body: entry.body + 78, end: entry.end };
  const base = { kind: "video" as const, width, height, samples, frame: commonFrame(samples) };
  switch (entry.type) {
    case "avc1":
    case "avc3": {
      const avcC = child(v, inner, "avcC");
      if (!avcC) fail("an H.264 track has no avcC box");
      const d = v.slice(avcC.body, avcC.end);
      if (d.length < 4) fail("the avcC box is too short");
      return { ...base, codec: `${entry.type}.${hex2(d[1])}${hex2(d[2])}${hex2(d[3])}`, description: d };
    }
    case "hvc1":
    case "hev1": {
      const hvcC = child(v, inner, "hvcC");
      if (!hvcC) fail("an HEVC track has no hvcC box");
      const d = v.slice(hvcC.body, hvcC.end);
      return { ...base, codec: hevcCodec(entry.type, d), description: d };
    }
    case "vp09": {
      const vpcC = child(v, inner, "vpcC");
      if (!vpcC) fail("a VP9 track has no vpcC box");
      const profile = v.u8(vpcC.body + 4);
      const level = v.u8(vpcC.body + 5);
      const depth = v.u8(vpcC.body + 6) >> 4;
      const two = (n: number) => String(n).padStart(2, "0");
      return { ...base, codec: `vp09.${two(profile)}.${two(level)}.${two(depth)}` };
    }
    default:
      fail(`the picture is ${entry.type.trim()}, which the video preview cannot decode`);
  }
}

// The codec string of an HEVC track, from its hvcC, the way ISO/IEC
// 14496-15 annex E writes it: hvc1.1.6.L93.B0 for 8 bit main at level 3.1.
export function hevcCodec(type: string, d: Uint8Array): string {
  if (d.length < 13) fail("the hvcC box is too short");
  const space = ["", "A", "B", "C"][d[1] >> 6];
  const tier = (d[1] >> 5) & 1 ? "H" : "L";
  const profile = d[1] & 0x1f;
  let compat = ((d[2] << 24) | (d[3] << 16) | (d[4] << 8) | d[5]) >>> 0;
  // The compatibility flags are written in the reverse bit order.
  let reversed = 0;
  for (let i = 0; i < 32; i++) {
    reversed = ((reversed << 1) | (compat & 1)) >>> 0;
    compat >>>= 1;
  }
  const constraints = Array.from(d.subarray(6, 12));
  while (constraints.length && constraints[constraints.length - 1] === 0) constraints.pop();
  const parts = [type, `${space}${profile}`, reversed.toString(16).toUpperCase(), `${tier}${d[12]}`];
  for (const c of constraints) parts.push(c.toString(16).toUpperCase());
  return parts.join(".");
}

function audioEntry(v: View, entry: Box, samples: Samples): AudioTrack {
  // The sound description is the ISO one, or QuickTime's version 1 or 2,
  // which put more fields before the boxes inside.
  const version = v.u16(entry.body + 8);
  let channels = v.u16(entry.body + 16);
  let sampleRate = v.u32(entry.body + 24) / 65536;
  let inner = entry.body + 28;
  if (version === 1) inner += 16;
  if (version === 2) {
    v.u32(entry.body + 36);
    sampleRate = new DataView(v.bytes.buffer, v.bytes.byteOffset).getFloat64(entry.body + 32);
    channels = v.u32(entry.body + 40);
    inner = entry.body + 64;
  }
  const box = { body: inner, end: entry.end };
  if (inner > entry.end) fail("the sound description is too short");
  switch (entry.type) {
    case "mp4a": {
      // MOV keeps esds inside a wave box.
      const esds = child(v, box, "esds") ?? path(v, box, "wave", "esds");
      if (!esds) fail("an AAC track has no esds box");
      const config = audioSpecificConfig(v, esds);
      const objectType = config[0] >> 3 === 31 ? 32 + (((config[0] & 7) << 3) | (config[1] >> 5)) : config[0] >> 3;
      return {
        kind: "audio",
        codec: `mp4a.40.${objectType}`,
        description: config,
        sampleRate: sampleRate || samples.timescale,
        channels,
        samples,
      };
    }
    case "Opus": {
      const dOps = child(v, box, "dOps");
      if (!dOps) fail("an Opus track has no dOps box");
      return { kind: "audio", codec: "opus", description: opusHead(v, dOps), sampleRate: 48000, channels, samples };
    }
    default:
      fail(`the sound is ${entry.type.trim()}, which the video preview cannot decode`);
  }
}

// The AudioSpecificConfig in an esds box, which is what AudioDecoder takes
// as the description of AAC. The box is a nest of descriptors, each a tag
// and a length written seven bits at a time.
function audioSpecificConfig(v: View, esds: Box): Uint8Array {
  let at = esds.body + 4;
  const end = esds.end;
  const descriptor = (want: number): number => {
    const tag = v.u8(at++);
    let length = 0;
    for (let i = 0; i < 4; i++) {
      const b = v.u8(at++);
      length = (length << 7) | (b & 0x7f);
      if (!(b & 0x80)) break;
    }
    if (tag !== want) fail("the esds box is not the shape AAC has");
    if (at + length > end) fail("the esds box runs past its end");
    return length;
  };
  descriptor(0x03);
  at += 2;
  const flags = v.u8(at++);
  if (flags & 0x80) at += 2;
  if (flags & 0x40) at += 1 + v.u8(at);
  if (flags & 0x20) at += 2;
  descriptor(0x04);
  const objectType = v.u8(at);
  if (objectType !== 0x40) fail("the sound is MPEG audio that is not AAC");
  at += 13;
  const length = descriptor(0x05);
  if (length < 2) fail("the AAC configuration is too short");
  return v.slice(at, at + length);
}

// The Opus identification header AudioDecoder takes, made from dOps,
// which holds the same fields big-endian and without the magic.
function opusHead(v: View, dOps: Box): Uint8Array {
  const b = dOps.body;
  const family = v.u8(b + 10);
  const table = family ? 2 + v.u8(b + 1) : 0;
  const out = new Uint8Array(19 + table);
  const dv = new DataView(out.buffer);
  out.set([0x4f, 0x70, 0x75, 0x73, 0x48, 0x65, 0x61, 0x64]);
  out[8] = 1;
  out[9] = v.u8(b + 1);
  dv.setUint16(10, v.u16(b + 2), true);
  dv.setUint32(12, v.u32(b + 4), true);
  dv.setInt16(16, v.i16(b + 8), true);
  out[18] = family;
  if (table) out.set(v.slice(b + 11, b + 11 + table), 19);
  return out;
}

// Seconds from ticks.
export function seconds(s: Samples, ticks: number): number {
  return ticks / s.timescale;
}

// The rank of the frame that holds a moment: the last one shown at or
// before it. Before the first frame it is the first.
export function rankAt(s: Samples, at: number): number {
  const ticks = at * s.timescale;
  let lo = 0;
  let hi = s.count - 1;
  if (hi < 0) return 0;
  // A moment a hair before a frame's own start, from a sum of seconds, is
  // in that frame.
  const slack = 1e-6 * s.timescale;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (s.pts[s.order[mid]] <= ticks + slack) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}
