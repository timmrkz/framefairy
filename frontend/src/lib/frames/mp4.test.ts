import { describe, expect, test } from "vitest";
import { aacRate, findMoov, hevcCodec, MP4Error, parseMoov, pcmPlanes, rankAt, type Pcm } from "./mp4";

// The boxes are written out here by hand, the way a muxer writes them, so
// the tests need no file and no ffmpeg. Each test builds only what it is
// about. The parser was also checked once against files ffmpeg made, H.264
// with B-frames and AAC in MP4, HEVC and AAC in MOV, VP9 and Opus in MP4,
// and every time and offset agreed with ffprobe.

const u8 = (n: number) => [n & 255];
const u16 = (n: number) => [(n >> 8) & 255, n & 255];
const u32 = (n: number) => [(n >>> 24) & 255, (n >> 16) & 255, (n >> 8) & 255, n & 255];
const u64 = (n: number) => [...u32(Math.floor(n / 2 ** 32)), ...u32(n >>> 0)];
const str = (s: string) => [...s].map((c) => c.charCodeAt(0));
const box = (type: string, ...body: number[][]): number[] => {
  const flat = body.flat();
  return [...u32(flat.length + 8), ...str(type), ...flat];
};
const full = (type: string, version: number, ...body: number[][]) => box(type, [version, 0, 0, 0], ...body);
const zeros = (n: number) => new Array(n).fill(0);

const mvhd = (scale: number) => full("mvhd", 0, u32(0), u32(0), u32(scale), u32(0), zeros(80));
const tkhd = (enabled = true) => box("tkhd", [0, 0, 0, enabled ? 1 : 0], zeros(80));
const mdhd = (scale: number) => full("mdhd", 0, u32(0), u32(0), u32(scale), u32(0), u32(0));
const hdlr = (kind: string) => full("hdlr", 0, u32(0), str(kind), zeros(12), [0]);
const stts = (runs: [number, number][]) => full("stts", 0, u32(runs.length), ...runs.map(([n, d]) => [...u32(n), ...u32(d)]));
const ctts = (runs: [number, number][]) => full("ctts", 0, u32(runs.length), ...runs.map(([n, o]) => [...u32(n), ...u32(o)]));
const stss = (keys: number[]) => full("stss", 0, u32(keys.length), ...keys.map(u32));
const stsz = (sizes: number[]) => full("stsz", 0, u32(0), u32(sizes.length), ...sizes.map(u32));
const stsc = (runs: [number, number][]) => full("stsc", 0, u32(runs.length), ...runs.map(([first, n]) => [...u32(first), ...u32(n), ...u32(1)]));
const stco = (offsets: number[]) => full("stco", 0, u32(offsets.length), ...offsets.map(u32));
const co64 = (offsets: number[]) => full("co64", 0, u32(offsets.length), ...offsets.map(u64));
const elst = (edits: [number, number][]) => box("edts", full("elst", 0, u32(edits.length), ...edits.map(([d, t]) => [...u32(d), ...u32(t >>> 0), ...u32(0x10000)])));

const avc1 = (w: number, h: number, ...more: number[][]) =>
  box("avc1", zeros(6), u16(1), zeros(16), u16(w), u16(h), zeros(50), box("avcC", [1, 0x64, 0x00, 0x1f, 0xff, 0xe0, 0]), ...more);
const mp4aMov = () =>
  box(
    "mp4a",
    zeros(6),
    u16(1),
    u16(1), // QuickTime sound description version 1
    zeros(6),
    u16(2),
    u16(16),
    zeros(4),
    u32(44100 * 65536),
    zeros(16),
    box("wave", box("frma", str("mp4a")), full("esds", 0, esdsBody([0x12, 0x10]))),
  );
// An ES descriptor holding a decoder config holding the AAC config.
function esdsBody(config: number[]): number[] {
  const dsi = [0x05, config.length, ...config];
  const dcd = [0x04, 13 + dsi.length, 0x40, 0x15, ...zeros(11), ...dsi];
  return [0x03, 3 + dcd.length, 0, 1, 0, ...dcd];
}

const trak = (head: number[], kind: string, scale: number, entry: number[], tables: number[][]) =>
  box(
    "trak",
    head,
    box(
      "mdia",
      mdhd(scale),
      hdlr(kind),
      box("minf", box("stbl", full("stsd", 0, u32(1), entry), ...tables)),
    ),
  );

const movie = (...traks: number[][]) => new Uint8Array(box("moov", mvhd(1000), ...traks));

// A little sound from zero on, beside a picture that starts late, which
// is what makes it late: the episode starts where its earliest track does.
const soundFromZero = (edits: [number, number][] = []) =>
  trak([...tkhd(), ...(edits.length ? elst(edits) : [])], "soun", 44100, mp4aMov(), [
    stts([[1, 1024]]),
    stsz([1]),
    stsc([[1, 1]]),
    stco([0]),
  ]);

describe("parseMoov", () => {
  test("reads where every frame is and when it is shown, B-frames and edit list included", () => {
    // Four frames at 25 a second in a timescale of 12800, decoded I P B B
    // and shown I B B P, moved back by the two frames of delay the
    // B-frames add, the way ffmpeg writes H.264.
    const m = parseMoov(
      movie(
        trak(
          [...tkhd(), ...elst([[160, 1024]])],
          "vide",
          12800,
          avc1(1920, 1080),
          [
            stts([[4, 512]]),
            ctts([
              [1, 1024],
              [1, 2048],
              [2, 512],
            ]),
            stss([1]),
            stsz([1000, 200, 50, 60]),
            stsc([[1, 2]]),
            stco([5000, 9000]),
          ],
        ),
      ),
    );
    const v = m.video!;
    expect(v.codec).toBe("avc1.64001f");
    expect([v.width, v.height]).toEqual([1920, 1080]);
    expect(v.frame).toBe(0.04);
    expect(Array.from(v.samples.offset)).toEqual([5000, 6000, 9000, 9050]);
    // Decode order I P B B: the P is shown fourth, the Bs second and third.
    const shown = Array.from(v.samples.order, (i) => v.samples.pts[i] / 12800);
    expect(shown).toEqual([0, 0.04, 0.08, 0.12]);
    expect(Array.from(v.samples.order)).toEqual([0, 2, 3, 1]);
    expect(Array.from(v.samples.rank)).toEqual([0, 3, 1, 2]);
    expect(Array.from(v.samples.key)).toEqual([1, 0, 0, 0]);
  });

  test("reads chunks of several samples, 64-bit offsets and an empty edit that delays the track", () => {
    const m = parseMoov(
      movie(
        trak(
          [...tkhd(), ...elst([
            [500, -1],
            [1000, 0],
          ])],
          "vide",
          1000,
          avc1(640, 360),
          [stts([[5, 40]]), stsz([10, 20, 30, 40, 50]), stsc([[1, 3], [2, 2]]), co64([2 ** 33, 2 ** 33 + 1000])],
        ),
        soundFromZero(),
      ),
    );
    const s = m.video!.samples;
    expect(Array.from(s.offset)).toEqual([2 ** 33, 2 ** 33 + 10, 2 ** 33 + 30, 2 ** 33 + 1000, 2 ** 33 + 1040]);
    // Half a second of nothing, then the track.
    expect(s.pts[0] / s.timescale).toBe(0.5);
    // No stss: every frame is a key frame.
    expect(Array.from(s.key)).toEqual([1, 1, 1, 1, 1]);
    expect(m.duration).toBeCloseTo(0.7);
  });

  test("counts from where the earliest track starts, the way ffmpeg and the engine do", () => {
    const picture = (edits: [number, number][]) =>
      trak([...tkhd(), ...elst(edits)], "vide", 1000, avc1(10, 10), [stts([[2, 40]]), stsz([1, 1]), stsc([[1, 2]]), stco([0])]);
    // Both tracks held back, the picture by half a second and the sound,
    // its priming not cut, by 0.476: the way ffmpeg writes a recording
    // cut out of a longer one, which it says starts at 0.476. The engine
    // cuts it counting from there, so the frame queue does too.
    const late = parseMoov(
      movie(
        picture([
          [500, -1],
          [1000, 0],
        ]),
        soundFromZero([
          [476, -1],
          [1000, 0],
        ]),
      ),
    );
    expect(late.video!.samples.pts[0] / 1000).toBeCloseTo(0.024, 4);
    expect(late.audio!.samples.pts[0]).toBe(0);
    expect(late.duration).toBeCloseTo(0.104, 4);
    // The priming cut off the front of AAC is before the start, not where
    // the episode starts, so a plain file still starts at zero.
    const plain = parseMoov(movie(picture([[1000, 0]]), soundFromZero([[1000, 1024]])));
    expect(plain.video!.samples.pts[0]).toBe(0);
    expect(plain.audio!.samples.pts[0]).toBe(-1024);
    // A picture alone that starts late starts the episode.
    expect(parseMoov(movie(picture([[250, -1], [1000, 0]]))).video!.samples.pts[0]).toBe(0);
  });

  test("reads the picture's colours from its colr box, and guesses them by its size without one", () => {
    const colourOf = (w: number, h: number, ...colr: number[][]) =>
      parseMoov(movie(trak(tkhd(), "vide", 1000, avc1(w, h, ...colr), [stts([[1, 40]]), stsz([1]), stsc([[1, 1]]), stco([0])]))).video!.colour;
    // Tim's start.mp4 says BT.709 in video range, as most files do. A
    // frame made from what the Go side decoded without it was drawn as if
    // in the whole range, its black a grey of 17.
    expect(colourOf(1920, 1080, box("colr", str("nclx"), u16(1), u16(1), u16(1), [0]))).toEqual({
      primaries: "bt709",
      transfer: "bt709",
      matrix: "bt709",
      fullRange: false,
    });
    expect(colourOf(1920, 1080, box("colr", str("nclx"), u16(1), u16(13), u16(1), [0x80])).fullRange).toBe(true);
    // QuickTime's nclc has no range and is video range.
    expect(colourOf(720, 576, box("colr", str("nclc"), u16(5), u16(6), u16(6)))).toEqual({
      primaries: "bt470bg",
      transfer: "smpte170m",
      matrix: "smpte170m",
      fullRange: false,
    });
    // Unspecified, BT.2020 and no box at all keep the guess by size.
    expect(colourOf(1920, 1080, box("colr", str("nclx"), u16(2), u16(2), u16(2), [0])).matrix).toBe("bt709");
    expect(colourOf(3840, 2160, box("colr", str("nclx"), u16(9), u16(14), u16(9), [0])).matrix).toBe("bt709");
    expect(colourOf(640, 480).matrix).toBe("smpte170m");
    expect(colourOf(1280, 720).primaries).toBe("bt709");
  });

  test("reads AAC in a QuickTime sound description, with esds inside wave", () => {
    const m = parseMoov(
      movie(
        trak(
          [...tkhd(), ...elst([[1000, 1024]])],
          "soun",
          44100,
          mp4aMov(),
          [stts([[3, 1024]]), stsz([300, 310, 320]), stsc([[1, 3]]), stco([100])],
        ),
      ),
    );
    const a = m.audio!;
    expect(a.codec).toBe("mp4a.40.2");
    expect(Array.from(a.description!)).toEqual([0x12, 0x10]);
    expect([a.sampleRate, a.channels]).toEqual([44100, 2]);
    // The priming of AAC is before zero.
    expect(Array.from(a.samples.pts)).toEqual([-1024, 0, 1024]);
  });

  test("reads plain sound by the chunk, however many moments the file counts", () => {
    // QuickTime's 16 bit little-endian sound, two channels at 48000, the
    // way ffmpeg writes pcm_s16le into a MOV: a sample for every moment,
    // here 48000 and 24000 of them in two chunks.
    const sowt = box("sowt", zeros(6), u16(1), u16(0), zeros(6), u16(2), u16(16), zeros(4), u32(48000 * 65536));
    const m = parseMoov(
      movie(trak(tkhd(), "soun", 48000, sowt, [stts([[72000, 1]]), full("stsz", 0, u32(4), u32(72000)), stsc([[1, 48000], [2, 24000]]), stco([1000, 500000])])),
    );
    const a = m.audio!;
    expect(a.codec).toBe("pcm");
    expect(a.pcm).toEqual({ bytes: 2, float: false, little: true, signed: true });
    expect([a.sampleRate, a.channels]).toEqual([48000, 2]);
    expect(a.samples.count).toBe(2);
    expect(Array.from(a.samples.offset)).toEqual([1000, 500000]);
    expect(Array.from(a.samples.size)).toEqual([192000, 96000]);
    expect(Array.from(a.samples.pts)).toEqual([0, 48000]);
    expect(Array.from(a.samples.duration)).toEqual([48000, 24000]);
    expect(m.duration).toBe(1.5);
  });

  test("reads which way round plain sound is from its four letters, enda and lpcm's flags", () => {
    const desc = (type: string, bits: number, ...inner: number[][]) =>
      box(type, zeros(6), u16(1), u16(0), zeros(6), u16(1), u16(bits), zeros(4), u32(44100 * 65536), ...inner);
    const lpcm = (flags: number, bits: number) =>
      box("lpcm", zeros(6), u16(1), u16(2), zeros(6), u16(3), u16(16), [0xff, 0xfe], u16(0), u32(65536), u32(72), [0x40, 0xe7, 0x70, 0, 0, 0, 0, 0], u32(1), u32(0x7f000000), u32(bits), u32(flags), u32(bits / 8), u32(1));
    const pcmOf = (entry: number[]) =>
      parseMoov(movie(trak(tkhd(), "soun", 44100, entry, [stts([[10, 1]]), full("stsz", 0, u32(2), u32(10)), stsc([[1, 10]]), stco([0])]))).audio!;
    expect(pcmOf(desc("twos", 16)).pcm).toEqual({ bytes: 2, float: false, little: false, signed: true });
    expect(pcmOf(desc("in24", 24)).pcm).toEqual({ bytes: 3, float: false, little: false, signed: true });
    expect(pcmOf(desc("in24", 24, box("wave", box("enda", u16(1))))).pcm).toEqual({ bytes: 3, float: false, little: true, signed: true });
    expect(pcmOf(desc("fl32", 32)).pcm).toEqual({ bytes: 4, float: true, little: false, signed: true });
    expect(pcmOf(desc("fl64", 64, box("enda", u16(1)))).pcm).toEqual({ bytes: 8, float: true, little: true, signed: true });
    expect(pcmOf(desc("ipcm", 16, full("pcmC", 0, [1, 24]))).pcm).toEqual({ bytes: 3, float: false, little: true, signed: true });
    // lpcm: float 1, big-endian 2, signed 4.
    const l = pcmOf(lpcm(1 | 8, 32));
    expect(l.pcm).toEqual({ bytes: 4, float: true, little: true, signed: true });
    expect(l.sampleRate).toBe(48000);
    expect(pcmOf(lpcm(2 | 4 | 8, 24)).pcm).toEqual({ bytes: 3, float: false, little: false, signed: true });
  });

  test("names sound it cannot play", () => {
    const alaw = box("alaw", zeros(6), u16(1), u16(0), zeros(6), u16(1), u16(8), zeros(4), u32(8000 * 65536));
    expect(() =>
      parseMoov(movie(trak(tkhd(), "soun", 8000, alaw, [stts([[1, 160]]), stsz([160]), stsc([[1, 1]]), stco([0])]))),
    ).toThrow(/alaw/);
  });

  test("leaves out a track that is switched off", () => {
    const m = parseMoov(
      movie(
        trak(tkhd(false), "vide", 1000, avc1(10, 10), [stts([[1, 40]]), stsz([1]), stsc([[1, 1]]), stco([0])]),
        trak(tkhd(), "vide", 1000, avc1(20, 20), [stts([[1, 40]]), stsz([1]), stsc([[1, 1]]), stco([0])]),
      ),
    );
    expect(m.video!.width).toBe(20);
  });

  test("says what is wrong with a broken file rather than reading past it", () => {
    const good = movie(trak(tkhd(), "vide", 1000, avc1(10, 10), [stts([[1, 40]]), stsz([1]), stsc([[1, 1]]), stco([0])]));
    for (let cut = 8; cut < good.length; cut += 7) {
      const broken = good.slice(0, cut);
      // The outer size still says the whole box.
      expect(() => parseMoov(broken)).toThrow(MP4Error);
    }
    const lying = good.slice();
    // stsz says a thousand samples and holds one.
    const at = [...lying.keys()].find((i) => String.fromCharCode(...lying.slice(i, i + 4)) === "stsz")!;
    lying.set(u32(1000), at + 12);
    expect(() => parseMoov(lying)).toThrow(MP4Error);
  });

  test("says a fragmented file is one, whose samples are not in moov", () => {
    const m = new Uint8Array(box("moov", mvhd(1000), box("mvex", full("trex", 0, zeros(20)))));
    expect(() => parseMoov(m)).toThrow(/fragmented/);
  });

  test("names a codec it cannot decode", () => {
    const prores = box("apch", zeros(6), u16(1), zeros(70));
    expect(() =>
      parseMoov(movie(trak(tkhd(), "vide", 1000, prores, [stts([[1, 40]]), stsz([1]), stsc([[1, 1]]), stco([0])]))),
    ).toThrow(/apch/);
  });
});

describe("findMoov", () => {
  const moov = movie(trak(tkhd(), "vide", 1000, avc1(10, 10), [stts([[1, 40]]), stsz([1]), stsc([[1, 1]]), stco([0])]));
  const file = (...parts: number[][]) => new Uint8Array(parts.flat());
  const reader = (bytes: Uint8Array) => {
    const asked: [number, number][] = [];
    return {
      asked,
      read: async (from: number, to: number) => {
        asked.push([from, to]);
        return bytes.slice(from, to);
      },
    };
  };

  test("finds moov at the start in the first read", async () => {
    const f = file(box("ftyp", str("isom")), Array.from(moov), box("mdat", zeros(100000)));
    const r = reader(f);
    expect(Array.from(await findMoov(r.read, f.length))).toEqual(Array.from(moov));
    expect(r.asked.length).toBe(1);
  });

  test("finds moov at the end, reading only the headers on the way", async () => {
    const f = file(box("ftyp", str("isom")), box("mdat", zeros(300000)), Array.from(moov));
    const r = reader(f);
    expect(Array.from(await findMoov(r.read, f.length, 4096))).toEqual(Array.from(moov));
    // The first read, the header after mdat, and moov itself.
    expect(r.asked.length).toBe(3);
    expect(r.asked.every(([a, b]) => b - a <= 4096 || a === f.length - moov.length)).toBe(true);
  });

  test("refuses a fragmented MP4 and a file that is not one", async () => {
    const frag = file(box("ftyp", str("isom")), box("moof", zeros(10)), box("mdat", zeros(10)));
    await expect(findMoov(reader(frag).read, frag.length)).rejects.toThrow(/fragmented/);
    const junk = new Uint8Array(100).fill(0x41);
    await expect(findMoov(reader(junk).read, junk.length)).rejects.toThrow(MP4Error);
  });
});

describe("hevcCodec", () => {
  test("writes the codec string the way the standard does", () => {
    // Main profile, compatible with main and main 10, level 3.1, the
    // progressive and frame-only constraints set.
    const d = new Uint8Array([1, 0x01, 0x60, 0, 0, 0, 0x90, 0, 0, 0, 0, 0, 93]);
    expect(hevcCodec("hvc1", d)).toBe("hvc1.1.6.L93.90");
    // Main 10 in the high tier at level 5.1.
    const ten = new Uint8Array([1, 0x22, 0x20, 0, 0, 0, 0xb0, 0, 0, 0, 0, 0, 153]);
    expect(hevcCodec("hev1", ten)).toBe("hev1.2.4.H153.B0");
  });
});

describe("rankAt", () => {
  const m = parseMoov(
    movie(trak(tkhd(), "vide", 25, avc1(10, 10), [stts([[100, 1]]), stsz(new Array(100).fill(1)), stsc([[1, 100]]), stco([0])])),
  );
  const s = m.video!.samples;
  test("is the frame that holds the moment", () => {
    expect(rankAt(s, 0)).toBe(0);
    expect(rankAt(s, 0.038)).toBe(0);
    expect(rankAt(s, 0.04)).toBe(1);
    // A frame's start kept to the millisecond, a hair before it, is in it.
    expect(rankAt(s, 0.0396)).toBe(1);
    expect(rankAt(s, 59.53 - 57)).toBe(63);
    // A sum of seconds that comes out a hair before a frame is in it.
    expect(rankAt(s, 0.1 + 0.2 - 0.3 + 0.04 - 1e-12)).toBe(1);
    expect(rankAt(s, -5)).toBe(0);
    expect(rankAt(s, 1000)).toBe(99);
  });

  // The frames of a phone come a little early or late, in ticks of 1/600
  // s, those of a screen recorder are left out where nothing moved, and a
  // picture can start after the sound by an empty edit. The frame that
  // holds a moment is found by the frames' own times all the same, never
  // by frame k at k over a rate. Plan row 2.130, and the render's own
  // rule, cutOf in engine/render.go.
  test("is the frame that holds the moment by its own time, at uneven times and after a late start", () => {
    const uneven = parseMoov(
      movie(
        trak(
          [...tkhd(), ...elst([
            [250, -1],
            [1000, 0],
          ])],
          "vide",
          600,
          avc1(10, 10),
          // 20, 25, 15 and 20 ticks, then one frame held for 200.
          [stts([[1, 20], [1, 25], [1, 15], [1, 20], [1, 200], [1, 20]]), stsz(new Array(6).fill(1)), stsc([[1, 6]]), stco([0])],
        ),
        soundFromZero(),
      ),
    ).video!.samples;
    const begins = Array.from(uneven.pts, (t) => t / 600);
    // A quarter of a second of nothing, then frames at 0, 20, 45, 60, 80
    // and 280 ticks after it.
    expect(begins.map((t) => t.toFixed(4))).toEqual(["0.2500", "0.2833", "0.3250", "0.3500", "0.3833", "0.7167"]);
    // Before the picture starts, its first frame.
    expect(rankAt(uneven, 0)).toBe(0);
    expect(rankAt(uneven, 0.282)).toBe(0);
    expect(rankAt(uneven, 0.2834)).toBe(1);
    // The second frame begins at 0.28333, which kept to the millisecond is
    // 0.283, and that moment is the second frame's too.
    expect(rankAt(uneven, 0.283)).toBe(1);
    // 0.33 is past where the third frame begins, though frame 2 at the
    // rate of 30 would begin at 0.3167 and frame 3 at 0.35.
    expect(rankAt(uneven, 0.33)).toBe(2);
    // The held frame holds every moment until the next begins.
    expect(rankAt(uneven, 0.7)).toBe(4);
    expect(rankAt(uneven, 0.7167)).toBe(5);
  });
});

describe("pcmPlanes", () => {
  const planes = (bytes: number[], pcm: Pcm, channels: number) => {
    const out = Array.from({ length: channels }, () => new Float32Array(8));
    const n = pcmPlanes(new Uint8Array(bytes), pcm, channels, out);
    return out.map((c) => Array.from(c.subarray(0, n)));
  };
  test("splits the channels and scales every layout to the same loudness", () => {
    const half = 0.5;
    expect(planes([0x00, 0x40, 0x00, 0xc0], { bytes: 2, float: false, little: true, signed: true }, 2)).toEqual([[half], [-half]]);
    expect(planes([0x40, 0x00, 0xc0, 0x00], { bytes: 2, float: false, little: false, signed: true }, 1)).toEqual([[half, -half]]);
    expect(planes([0x00, 0x00, 0x40, 0x00, 0x00, 0xc0], { bytes: 3, float: false, little: true, signed: true }, 1)).toEqual([[half, -half]]);
    expect(planes([0x40, 0x00, 0x00, 0xc0, 0x00, 0x00], { bytes: 3, float: false, little: false, signed: true }, 1)).toEqual([[half, -half]]);
    expect(planes([0x40, 0, 0, 0], { bytes: 4, float: false, little: false, signed: true }, 1)).toEqual([[half]]);
    expect(planes([0x3f, 0, 0, 0], { bytes: 4, float: true, little: false, signed: true }, 1)).toEqual([[half]]);
    expect(planes([0, 0, 0, 0, 0, 0, 0xe0, 0x3f], { bytes: 8, float: true, little: true, signed: true }, 1)).toEqual([[half]]);
    expect(planes([0xc0, 0x40], { bytes: 1, float: false, little: false, signed: false }, 1)).toEqual([[half, -half]]);
  });
  test("leaves a moment cut short at the end of the bytes out", () => {
    expect(planes([0, 0x40, 0, 0x40, 0], { bytes: 2, float: false, little: true, signed: true }, 2)).toEqual([[0.5], [0.5]]);
  });
});

describe("aacRate", () => {
  test("plain AAC comes out at its own rate", () => {
    // AAC LC, 44100, two channels.
    expect(aacRate(new Uint8Array([0x12, 0x10]))).toEqual({ core: 44100, out: 44100 });
  });
  test("HE-AAC said by its object type comes out at twice the core", () => {
    // SBR, 24000 core, two channels, 48000 out, then AAC LC.
    expect(aacRate(new Uint8Array([0x2b, 0x11, 0x88, 0x00]))).toEqual({ core: 24000, out: 48000 });
    // PS, the same, as an iPhone writes HE-AAC v2.
    expect(aacRate(new Uint8Array([0xeb, 0x09, 0x88, 0x00]))).toEqual({ core: 24000, out: 48000 });
  });
  test("HE-AAC said after the core's config, which a plain decoder passes over", () => {
    // AAC LC at 24000 in stereo, then 0x2b7, SBR, present, 48000.
    expect(aacRate(new Uint8Array([0x13, 0x10, 0x56, 0xe5, 0x98]))).toEqual({ core: 24000, out: 48000 });
  });
});
