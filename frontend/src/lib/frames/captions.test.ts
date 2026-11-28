import { afterEach, describe, expect, it, vi } from "vitest";
import { bounceScale, CaptionLayers, dressedOf, inkBox, laidPart, readBounce, readLayer, sameDress, turned, unturned, type Layer, type Over } from "./captions";

// A pixel of 10 bits a colour, red lowest, as the Go side sends every frame.
const word = (r: number, g: number, b: number) => (r | (g << 10) | (b << 20)) >>> 0;
const colours = (w: number) => [w & 1023, (w >>> 10) & 1023, (w >>> 20) & 1023];

function frame(w: number, h: number, grey: number): Uint8Array {
  return new Uint8Array(new Uint32Array(w * h).fill(word(grey, grey, grey)).buffer);
}

function pixelsOf(data: Uint8Array): number[][] {
  return [...new Uint32Array(data.buffer, data.byteOffset, data.byteLength / 4)].map(colours);
}

// The frame with the layer laid over it, drawn at its own size.
function laidOver(data: Uint8Array, fw: number, fh: number, o: Over): Uint8Array {
  const out = data.slice();
  const part = laidPart(data, fw, fh, fw, fh, o);
  if (!part) return out;
  const to = new Uint32Array(out.buffer);
  const from = new Uint32Array(part.data.buffer);
  for (let y = 0; y < part.h; y++)
    for (let x = 0; x < part.w; x++) to[(part.y + y) * fw + part.x + x] = from[y * part.w + x];
  return out;
}

function layer(x: number, y: number, w: number, h: number, pixel: number[]): Layer {
  return { x, y, w, h, pixels: new Uint8Array(Array.from({ length: w * h }, () => pixel).flat()) };
}

describe("a layer of captions", () => {
  it("is read the way framewire.Layer writes it", () => {
    const body = new ArrayBuffer(8 + 2 * 1 * 4);
    const v = new DataView(body);
    [3, 4, 2, 1].forEach((n, i) => v.setUint16(2 * i, n, true));
    new Uint8Array(body, 8).set([1, 2, 3, 4, 5, 6, 7, 8]);
    const l = readLayer(body);
    expect(l && { x: l.x, y: l.y, w: l.w, h: l.h, pixels: [...l.pixels] }).toEqual({
      x: 3,
      y: 4,
      w: 2,
      h: 1,
      pixels: [1, 2, 3, 4, 5, 6, 7, 8],
    });
    expect(readLayer(body.slice(0, 12))).toBeNull();
    expect(readLayer(new ArrayBuffer(4))).toBeNull();
  });

  it("turns with the picture and back", () => {
    for (const turn of [0, 90, 180, 270, -90, -180]) {
      for (const [u, v] of [
        [0.1, 0.2],
        [0.9, 0.3],
        [0.5, 0.75],
      ]) {
        const [du, dv] = turned(u, v, turn);
        const [bu, bv] = unturned(du, dv, turn);
        expect(bu).toBeCloseTo(u, 9);
        expect(bv).toBeCloseTo(v, 9);
      }
    }
    // The stylesheet turns the frame through -turn degrees, clockwise for
    // a turn of -90: the frame's top left is then the picture's top right.
    expect(turned(0, 0, -90)).toEqual([1, 0]);
    expect(turned(0, 0, 90)).toEqual([0, 1]);
  });

  it("is laid over the frame as the render blends it, colour plus frame times one less alpha", () => {
    // A frame of 4 by 2, the crop its middle half, and a layer over all of
    // the crop: red at half, which lets half of the grey through.
    const data = frame(4, 2, 512);
    const o: Over = { layer: layer(0, 0, 2, 2, [128, 0, 0, 128]), size: [2, 2], left: 0.25, width: 0.5, turn: 0 };
    const got = pixelsOf(laidOver(data, 4, 2, o));
    const half = Math.round(512 * (127 / 255));
    const red = [Math.min(1023, Math.round((128 * 1023) / 255) + half), half, half];
    expect(got).toEqual([[512, 512, 512], red, red, [512, 512, 512], [512, 512, 512], red, red, [512, 512, 512]]);
    // The frame itself is left as it was, for whoever holds it.
    expect(pixelsOf(data).every((p) => p[0] === 512)).toBe(true);
  });

  it("lays only its ink, where the ink is in the crop", () => {
    // A crop as large as the frame, drawn at 4 by 4, and ink only in its
    // bottom right pixel.
    const data = frame(4, 4, 100);
    const o: Over = { layer: layer(3, 3, 1, 1, [255, 255, 255, 255]), size: [4, 4], left: 0, width: 1, turn: 0 };
    const got = pixelsOf(laidOver(data, 4, 4, o));
    got.forEach((p, i) => expect(p[0]).toBe(i === 15 ? 1023 : 100));
  });

  it("is drawn at the size it is shown, not the frame's", () => {
    // A frame of 2 by 1, black and white, drawn at 8 by 4 with a layer as
    // large as all of it with ink in one pixel: the part is that pixel
    // alone, and the frame under the ink is sampled between its pixels.
    const data = new Uint8Array(new Uint32Array([word(0, 0, 0), word(1000, 1000, 1000)]).buffer);
    const o: Over = { layer: layer(3, 1, 1, 1, [0, 0, 0, 0]), size: [8, 4], left: 0, width: 1, turn: 0 };
    const part = laidPart(data, 2, 1, 8, 4, o)!;
    expect([part.x, part.y, part.w, part.h]).toEqual([3, 1, 1, 1]);
    // The middle of the fourth of 8 pixels is 3.5 / 8 of the way, which is
    // 0.375 of the way from the black pixel's middle to the white one's.
    expect(pixelsOf(part.data)[0][0]).toBe(375);
  });

  it("stands on the picture as shown when the picture is turned", () => {
    // A frame of 2 by 4 shown a quarter turn clockwise, 4 by 2, and ink in
    // the top left pixel of the picture as shown, which is the bottom left
    // of the frame.
    const data = frame(2, 4, 0);
    const o: Over = { layer: layer(0, 0, 1, 1, [255, 255, 255, 255]), size: [4, 2], left: 0, width: 1, turn: -90 };
    const got = pixelsOf(laidOver(data, 2, 4, o));
    got.forEach((p, i) => expect(p[0]).toBe(i === 6 ? 1023 : 0));
  });
});

// The bytes laidPart draws, folded into one number, for a frame and a
// layer with a lifted word, at every turn and at three sizes drawn. Taken
// from laidPart as it was before it worked out the turn once per draw,
// which must not change a byte.
describe("the captions laid over a frame", () => {
  const GOLDEN = [
    323565770, 3856472616, 1175872813, 3206193675, 2511689848, 519284877, 1068100129, 4257049211, 4203286498,
    3705036025, 2969228961, 2078544639, 3705036025, 2969228961, 2078544639, 3206193675, 2511689848, 519284877,
  ];
  function sample(turn: number, w: number, h: number): number {
    const fw = 12;
    const fh = 9;
    const frameWords = new Uint32Array(fw * fh);
    for (let i = 0; i < frameWords.length; i++) frameWords[i] = word((i * 37) % 1024, (i * 91) % 1024, (i * 13) % 1024);
    const ink = (i: number) => [(i * 29) % 256, (i * 53) % 256, (i * 71) % 256, (i * 97) % 256];
    const pixels = (n: number) => new Uint8Array(Array.from({ length: n }, (_, i) => ink(i)).flat());
    const lift = { layer: { x: 2, y: 1, w: 2, h: 2, pixels: pixels(4) }, scale: 1.3 };
    const o: Over = {
      layer: { x: 1, y: 1, w: 5, h: 3, pixels: pixels(15), lift } as Layer,
      size: [8, 6],
      left: 0.1,
      width: 0.8,
      turn,
    };
    const part = laidPart(new Uint8Array(frameWords.buffer), fw, fh, w, h, o);
    if (!part) return -1;
    let sum = (part.x * 31 + part.y * 17 + part.w * 7 + part.h) >>> 0;
    for (const v of new Uint32Array(part.data.buffer)) sum = (Math.imul(sum, 16777619) ^ v) >>> 0;
    return sum;
  }

  it("draws the same bytes at every turn and size", () => {
    const got = [0, -90, 90, 180, -180, 270].flatMap((turn) =>
      [
        [12, 9],
        [24, 18],
        [17, 13],
      ].map(([w, h]) => sample(turn, w, h)),
    );
    expect(got).toEqual(GOLDEN);
  });
});

// The pixels of a layer as the Go side sends them, see framewire.Layer.
function body(l: Layer): ArrayBuffer {
  const b = new ArrayBuffer(8 + l.pixels.length);
  const v = new DataView(b);
  [l.x, l.y, l.w, l.h].forEach((n, i) => v.setUint16(2 * i, n, true));
  new Uint8Array(b, 8).set(l.pixels);
  return b;
}

// A dress the Go side could send with a frame, see sentDress.
const dress = {
  piece: 0,
  in: true,
  clip: true,
  turn: 0,
  width: 480,
  height: 270,
  cropWidth: 152,
  left: 100,
  moved: false,
  mark: null,
  movable: true,
  key: "k",
  track: "abc",
  size: [10, 20],
};

describe("the dress that comes with a frame", () => {
  it("lays the layer sent with it over it, with what it shows and how large its pill is", () => {
    const sent = { ...dress, tag: "t1", showing: [3, 1, 2], scale: 1.1 };
    const l = dressedOf(JSON.stringify(sent), body(layer(2, 3, 1, 1, [9, 0, 0, 255])), () => null)!;
    expect([l.x, l.y, l.w, l.h, l.pixels[0]]).toEqual([2, 3, 1, 1, 9]);
    expect(l.dress?.left).toBe(100);
    expect(l.track).toBe("abc");
    expect(l.tag).toBe("t1");
    expect(l.size).toEqual([10, 20]);
    expect(l.scale).toBe(1.1);
    expect(l.showing).toEqual({ caption: 3, word: 1, last: 2 });
  });

  it("finds the layer of a frame sent with a tag alone, and gives it this frame's dress", () => {
    const had = dressedOf(JSON.stringify({ ...dress, tag: "t1" }), body(layer(0, 0, 1, 1, [7, 0, 0, 255])), () => null)!;
    const l = dressedOf(JSON.stringify({ ...dress, tag: "t1", left: 120 }), null, (tag) => (tag === "t1" ? had : null))!;
    expect(l.pixels[0]).toBe(7);
    expect(l.dress?.left).toBe(120);
    expect(had.dress?.left).toBe(100);
    // A tag the window no longer has: the dress, and nothing over it.
    const lost = dressedOf(JSON.stringify({ ...dress, tag: "t9" }), null, () => null)!;
    expect([lost.w, lost.tag, lost.dress?.left]).toEqual([0, undefined, 100]);
  });

  it("dresses a frame of a clip with no captions, and is nothing for a frame sent bare", () => {
    const l = dressedOf(JSON.stringify({ ...dress, track: "" }), null, () => null)!;
    expect([l.w, l.h, l.scale, l.showing]).toEqual([0, 0, 1, undefined]);
    expect(l.dress?.cropWidth).toBe(152);
    for (const bad of ["", "{", "null", "3", JSON.stringify({ ...dress, left: "x" })]) expect(dressedOf(bad, null, () => null)).toBeNull();
  });

  // After the automatic crop was brought back, a frame dressed again
  // differs from the one before only in whether Put the crop back would
  // move its crop. Taken for the same dress, the frame kept the one
  // without it, and the arrow never came.
  it("is another dress where only Put the crop back differs", () => {
    const d = dressedOf(JSON.stringify(dress), null, () => null)!.dress!;
    expect(sameDress(d, { ...d })).toBe(true);
    expect(sameDress(d, { ...d, back: true })).toBe(false);
    expect(sameDress({ ...d, back: false }, d)).toBe(true);
  });

  it("shows nothing it cannot read", () => {
    const of = (more: object) => dressedOf(JSON.stringify({ ...dress, ...more }), null, () => null)!;
    expect(of({ showing: [1.5, 0, 0] }).showing).toBeUndefined();
    expect(of({ showing: ["one", "two", 3] }).showing).toBeUndefined();
    expect(of({ showing: [0, -1, -1] }).showing).toEqual({ caption: 0, word: -1, last: -1 });
    for (const scale of [0, 2, -1, "big"]) expect(of({ scale }).scale).toBe(1);
  });
});

describe("the bounce after a word step", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // The layer that comes with a frame where the word lit is lit, red 100
  // plus the word, its pill drawn at scale, dressed for key.
  function own(lit: number, scale = 1, key = "k"): Layer {
    const sent = { ...dress, key, tag: `${key}${lit}${scale}`, showing: [0, lit, lit], scale };
    return dressedOf(JSON.stringify(sent), body(layer(0, 0, 1, 1, [100 + lit, 0, 0, 255])), () => null)!;
  }

  // The Go side: the rest of the caption apart is red 20 and the word
  // apart red 30, with the curve it bounces by.
  function goSide() {
    const asked: string[] = [];
    vi.stubGlobal("fetch", async (url: string) => {
      asked.push(url);
      const part = new URLSearchParams(url.split("?")[1]).get("part");
      const sent = { ...dress, showing: [0, 2, 2], scale: 1, ...(part === "word" ? { bounce: [220, 0.55, 1.15] } : {}) };
      return new Response(body(layer(0, 0, 1, 1, [part === "rest" ? 20 : 30, 0, 0, 255])), { headers: { "X-Frame-Dress": JSON.stringify(sent) } });
    });
    return asked;
  }

  // The screen's frames, one at a time, at the moment given. Handed in,
  // they are the layers' frames, the way a walk gives them, and the
  // browser's are never asked for.
  function screen(handed = false) {
    let waiting: [number, FrameRequestCallback][] = [];
    let n = 0;
    const next = (f: FrameRequestCallback) => {
      waiting.push([++n, f]);
      return n;
    };
    const cancel = (id: number) => {
      waiting = waiting.filter(([k]) => k !== id);
    };
    const never = () => {
      throw new Error("the browser's frames were asked for, where frames were handed in");
    };
    vi.stubGlobal("requestAnimationFrame", handed ? never : next);
    vi.stubGlobal("cancelAnimationFrame", handed ? never : cancel);
    return {
      next,
      cancel,
      frame(ms: number) {
        const now = waiting;
        waiting = [];
        now.forEach(([, f]) => f(ms));
      },
      get waiting() {
        return waiting.length;
      },
    };
  }

  // Tim saw the pop the short burns in played again over every frame he
  // stepped to, the arrow keys included, and wanted a step from word to
  // word to bounce its word and a step from frame to frame to show only
  // the short's frames. Then he found the bounce laggy, the short's frames
  // of the pop played at 25 a second, and wanted it the same for every
  // word, a word of one frame too.
  // A walk hands in frames of its own, so a busy machine cannot draw the
  // bounce on too few to see, issue #460: the bounce is the same on them.
  it.each([
    ["the browser's frames", false],
    ["frames handed in", true],
  ])("bounces the word a word step lands on at the screen's rate, and shows only the frame after any other step, on %s", async (_, handed) => {
    const asked = goSide();
    const s = screen(handed);
    let paused = true;
    // Told when the parts of a lifted word came, and on every frame of
    // the screen it bounces on.
    let told: (() => void)[] = [];
    const came = () => new Promise<void>((done) => told.push(done));
    const layers = new CaptionLayers(
      () => {
        const was = told;
        told = [];
        was.forEach((done) => done());
      },
      { paused: () => paused },
    );
    if (handed) layers.frames = s;
    layers.set({ path: "/v.mp4", clip: "a", framing: "fa", track: "abc", drafts: {} });
    const at = (t: number, l: Layer) => layers.at(t, [10, 20], l);
    // The first frame shows its own layer, and nothing bounces.
    expect(at(1.0, own(0))?.pixels[0]).toBe(100);
    expect(layers.popping).toBe(false);
    // An arrow alone onto another word shows its frame and nothing over it.
    layers.sought(false);
    expect(at(1.04, own(1))?.pixels[0]).toBe(101);
    expect(at(1.04, own(1))?.lift).toBeUndefined();
    expect(layers.stats.popped).toBe(0);
    // A word step lifts the word it lands on out of its caption. The
    // frame's own layer, at rest, stays while the parts are on their way.
    layers.sought(true);
    let parts = came();
    expect(at(1.5, own(2))?.pixels[0]).toBe(102);
    expect(layers.popping).toBe(true);
    expect(layers.busy).toBe(true);
    await parts;
    const sent = asked.map((a) => new URLSearchParams(a.split("?")[1]));
    expect(sent.map((q) => q.get("part")).sort()).toEqual(["rest", "word"]);
    expect(sent[0].get("at")).toBe("1.500000");
    expect(sent[0].get("track")).toBe("abc");
    // The rest over the frame, the word over that, at rest until the
    // screen's first frame.
    let l = at(1.5, own(2))!;
    expect(l.pixels[0]).toBe(20);
    expect(l.lift?.layer.pixels[0]).toBe(30);
    expect(l.lift?.scale).toBe(1);
    expect(l.showing).toEqual({ caption: 0, word: 2, last: 2 });
    // Then one scale for each frame of the screen, by its clock: up to its
    // peak a little past the middle, and back to rest.
    const scales: number[] = [];
    for (let ms = 1000; s.waiting; ms += 1000 / 60) {
      s.frame(ms);
      scales.push(at(1.5, own(2))!.lift!.scale);
    }
    expect(scales.length).toBeGreaterThanOrEqual(13);
    expect(scales[0]).toBe(1);
    expect(Math.max(...scales)).toBeGreaterThan(1.149);
    expect(scales[scales.length - 1]).toBe(1);
    const top = scales.indexOf(Math.max(...scales));
    scales.slice(1, top + 1).forEach((v, k) => expect(v).toBeGreaterThan(scales[k]));
    scales.slice(top + 1).forEach((v, k) => expect(v).toBeLessThan(scales[top + k]));
    expect(at(1.5, own(2))!.scale).toBe(1);
    expect(layers.stats.bounced).toBe(scales.length);
    // At rest it stays over the frame, every word alike, though the short
    // may show the frame otherwise.
    expect(layers.popping).toBe(false);
    expect(layers.busy).toBe(false);
    expect(at(1.5, own(2))?.lift?.layer.pixels[0]).toBe(30);
    expect(layers.stats.popped).toBe(1);
    // The frame dressed again for a draft changed over it lifts the word
    // again at rest, with no bounce.
    layers.set({ path: "/v.mp4", clip: "a", framing: "fa", track: "abc", drafts: { text: "#ffffff" } });
    parts = came();
    expect(at(1.5, own(2, 1, "k2"))?.lift?.scale).toBe(1);
    await parts;
    expect(at(1.5, own(2, 1, "k2"))?.lift?.layer.pixels[0]).toBe(30);
    expect(s.waiting).toBe(0);
    expect(layers.stats.popped).toBe(1);
    // An arrow alone: nothing bounces, and the frame shows its own layer.
    layers.sought(false);
    expect(at(1.54, own(3))?.pixels[0]).toBe(103);
    expect(at(1.54, own(3))?.lift).toBeUndefined();
    // A word of one frame, which the short shows at its peak: the layer
    // before stays while the parts come, and the bounce starts at rest.
    layers.sought(true);
    parts = came();
    expect(at(1.6, own(4, 1.15))?.pixels[0]).toBe(103);
    await parts;
    s.frame(5000);
    expect(at(1.6, own(4, 1.15))?.lift?.scale).toBe(1);
    s.frame(5100);
    expect(at(1.6, own(4, 1.15))?.lift?.scale).toBeGreaterThan(1.1);
    expect(layers.popping).toBe(true);
    // A step away puts the word back at once.
    layers.sought(false);
    at(1.64, own(5));
    expect(layers.popping).toBe(false);
    expect(s.waiting).toBe(0);
    expect(layers.stats.popped).toBe(2);
    // Playing, nothing bounces over the frames.
    paused = false;
    layers.sought(true);
    expect(at(2.0, own(5))?.pixels[0]).toBe(105);
    expect(layers.stats.popped).toBe(2);
  });

  it("follows one curve, at rest at either end and at its peak a little past the middle", () => {
    const b = readBounce([220, 0.55, 1.15])!;
    expect(b).toEqual({ length: 220, rise: 0.55, peak: 1.15 });
    expect(bounceScale(-5, b)).toBe(1);
    expect(bounceScale(0, b)).toBe(1);
    expect(bounceScale(121, b)).toBeCloseTo(1.15, 9);
    expect(bounceScale(220, b)).toBe(1);
    expect(bounceScale(500, b)).toBe(1);
    expect(bounceScale(60, b)).toBeGreaterThan(1.05);
    for (const bad of [undefined, [0, 0.5, 1.1], [220, 1.5, 1.1], [220, 0.5, 3]] as const)
      expect(readBounce(bad as [number, number, number] | undefined)).toBeUndefined();
  });

  it("lays the word over the rest, grown round its middle", () => {
    // A frame of 8 by 8, grey, shown at its size, the rest one dark pixel
    // in the corner and the word a white square of 2 by 2 in the middle.
    const data = frame(8, 8, 512);
    const white = Array.from({ length: 4 }, () => [255, 255, 255, 255]).flat();
    const word: Layer = { x: 3, y: 3, w: 2, h: 2, pixels: new Uint8Array(white) };
    const rest = layer(0, 0, 1, 1, [0, 0, 0, 255]);
    const at = (scale: number) => {
      const o: Over = { layer: { ...rest, lift: { layer: word, scale } }, size: [8, 8], left: 0, width: 1, turn: 0 };
      return pixelsOf(laidOver(data, 8, 8, o)).map((p) => p[0]);
    };
    // At rest: the word on its own four pixels, the rest in its corner.
    const still = at(1);
    still.forEach((v, i) => {
      const [x, y] = [i % 8, Math.floor(i / 8)];
      expect(v).toBe(x === 0 && y === 0 ? 0 : x >= 3 && x < 5 && y >= 3 && y < 5 ? 1023 : 512);
    });
    // Twice as large, it reaches a pixel further each way, sampled between
    // its pixels, and is as bright in its middle.
    const grown = at(2);
    expect(grown[3 * 8 + 3]).toBe(1023);
    expect(grown[2 * 8 + 2]).toBeGreaterThan(600);
    expect(grown[2 * 8 + 4]).toBeGreaterThan(still[2 * 8 + 4]);
    expect(grown[0]).toBe(0);
    expect(grown[7 * 8 + 7]).toBe(512);
    // Grown round its middle, in every direction alike: the grown word is
    // the same mirrored left to right and top to bottom, so it reaches as
    // far up as down and as far left as right. A bounce that grew one way
    // only passed this test before, found by the second review. The rest's
    // own pixel in the corner is left out.
    for (const scale of [1.15, 2]) {
      const g = at(scale);
      for (let y = 0; y < 8; y++)
        for (let x = 0; x < 8; x++)
          for (const [mx, my] of [
            [7 - x, y],
            [x, 7 - y],
          ]) {
            if ((x === 0 && y === 0) || (mx === 0 && my === 0)) continue;
            expect(Math.abs(g[y * 8 + x] - g[my * 8 + mx]), `${scale} at ${x},${y} against ${mx},${my}`).toBeLessThanOrEqual(1);
          }
      // And as far across as down.
      for (let i = 1; i < 7; i++) expect(Math.abs(g[i * 8 + 2] - g[2 * 8 + i])).toBeLessThanOrEqual(1);
    }
  });
});
