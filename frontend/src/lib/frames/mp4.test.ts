import { describe, expect, test } from "vitest";
import { findMoov, hevcCodec, MP4Error, parseMoov, rankAt } from "./mp4";

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

const avc1 = (w: number, h: number) =>
  box("avc1", zeros(6), u16(1), zeros(16), u16(w), u16(h), zeros(50), box("avcC", [1, 0x64, 0x00, 0x1f, 0xff, 0xe0, 0]));
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
    expect(rankAt(s, 0.039)).toBe(0);
    expect(rankAt(s, 0.04)).toBe(1);
    expect(rankAt(s, 59.53 - 57)).toBe(63);
    // A sum of seconds that comes out a hair before a frame is in it.
    expect(rankAt(s, 0.1 + 0.2 - 0.3 + 0.04 - 1e-12)).toBe(1);
    expect(rankAt(s, -5)).toBe(0);
    expect(rankAt(s, 1000)).toBe(99);
  });
});
