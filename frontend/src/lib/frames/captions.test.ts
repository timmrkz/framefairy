import { afterEach, describe, expect, it, vi } from "vitest";
import { CaptionLayers, laidPart, readLayer, readPop, readShowing, turned, unturned, type Layer, type Over } from "./captions";

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

describe("the layers of the captions", () => {
  afterEach(() => vi.unstubAllGlobals());

  function body(l: Layer): ArrayBuffer {
    const b = new ArrayBuffer(8 + l.pixels.length);
    const v = new DataView(b);
    [l.x, l.y, l.w, l.h].forEach((n, i) => v.setUint16(2 * i, n, true));
    new Uint8Array(b, 8).set(l.pixels);
    return b;
  }

  // The answer to the nth ask, the way the Go side gives it: a layer whose
  // first red is n, and the caption n with its first word lit.
  function answerOf(n: number): Response {
    return new Response(body(layer(0, 0, 1, 1, [n, 0, 0, 255])), { headers: { "X-Caption-Showing": `${n} 0` } });
  }

  it("keeps the last layer up while the one for a newer frame is on its way, and draws again when it comes", async () => {
    const asked: string[] = [];
    let answer: (() => void)[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      asked.push(url);
      const n = asked.length;
      return new Promise((done) => {
        answer.push(() => done(answerOf(n)));
      });
    });
    let came = 0;
    const layers = new CaptionLayers(() => came++);
    layers.set({ path: "/v.mp4", track: "abc", drafts: { text: "#ffffff" } });
    expect(layers.at(1.0, [10, 20])).toBeNull();
    expect(asked[0]).toContain("/frames/captions?");
    const q = new URLSearchParams(asked[0].split("?")[1]);
    expect(Object.fromEntries(q)).toEqual({ path: "/v.mp4", track: "abc", at: "1.000000", w: "10", h: "20", text: "#ffffff" });
    answer.shift()!();
    await vi.waitFor(() => expect(came).toBe(1));
    expect(layers.at(1.0, [10, 20])?.pixels[0]).toBe(1);
    // What the layer shows comes with it, for the caption box.
    expect(layers.at(1.0, [10, 20])?.showing).toEqual({ caption: 1, word: 0 });
    // The next frame: the first one's layer stays until its own comes.
    expect(layers.at(1.04, [10, 20])?.pixels[0]).toBe(1);
    answer.shift()!();
    await vi.waitFor(() => expect(came).toBe(2));
    expect(layers.at(1.04, [10, 20])?.pixels[0]).toBe(2);
    // A frame on its way asks ahead, and is there when it is drawn.
    layers.coming(1.08, [10, 20]);
    answer.shift()!();
    await vi.waitFor(() => expect(asked.length).toBe(3));
    await new Promise((r) => setTimeout(r));
    expect(layers.at(1.08, [10, 20])?.pixels[0]).toBe(3);
    expect(came).toBe(2);
    // No captions, nothing drawn and nothing kept up.
    layers.set(null);
    expect(layers.at(1.08, [10, 20])).toBeNull();
    answer = [];
  });

  // A fetch that answers each ask in turn: a layer whose first red is the
  // number of the ask, or a failure where fail says so.
  function answering(fail: (n: number) => boolean = () => false) {
    const asked: string[] = [];
    const answer: (() => void)[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      asked.push(url);
      const n = asked.length;
      return new Promise((done, failed) => {
        answer.push(() =>
          fail(n)
            ? failed(new Error("gone"))
            : done(answerOf(n)),
        );
      });
    });
    return { asked, answer };
  }

  it("keeps no layer that did not come, and asks for it once more", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout"] });
    try {
      const { asked, answer } = answering((n) => n === 1);
      let came = 0;
      const layers = new CaptionLayers(() => came++);
      layers.set({ path: "/v.mp4", track: "abc", drafts: {} });
      expect(layers.at(1.0, [10, 20])).toBeNull();
      answer.shift()!();
      // The failure leaves the frame waiting, and a second ask set for later.
      await vi.waitFor(() => expect(vi.getTimerCount()).toBe(1));
      expect(layers.busy).toBe(true);
      expect(asked.length).toBe(1);
      vi.advanceTimersByTime(300);
      expect(asked.length).toBe(2);
      answer.shift()!();
      await vi.waitFor(() => expect(came).toBe(1));
      expect(layers.at(1.0, [10, 20])?.pixels[0]).toBe(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it("gives up on a layer that fails twice, and asks again when the frame is drawn again", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout"] });
    try {
      const { asked, answer } = answering((n) => n <= 2);
      const layers = new CaptionLayers(() => {});
      layers.set({ path: "/v.mp4", track: "abc", drafts: {} });
      layers.at(1.0, [10, 20]);
      answer.shift()!();
      await vi.waitFor(() => expect(vi.getTimerCount()).toBe(1));
      vi.advanceTimersByTime(300);
      expect(asked.length).toBe(2);
      answer.shift()!();
      await vi.waitFor(() => expect(layers.busy).toBe(false));
      expect(vi.getTimerCount()).toBe(0);
      expect(asked.length).toBe(2);
      // Nothing was kept, so the next draw of the frame asks once more.
      expect(layers.at(1.0, [10, 20])).toBeNull();
      expect(asked.length).toBe(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it("never asks for a layer with an ask newer than the frame's, and drops another clip's captions", async () => {
    const { asked, answer } = answering();
    const layers = new CaptionLayers(() => {});
    layers.set({ path: "/v.mp4", track: "one", drafts: {} });
    // Three on their way fill the room, so the frame drawn waits for it.
    layers.coming(1.0, [10, 20]);
    layers.coming(1.04, [10, 20]);
    layers.coming(1.08, [10, 20]);
    layers.at(1.12, [10, 20]);
    expect(asked.length).toBe(3);
    // Another clip is chosen before any came: what waited is not asked for
    // with the new clip's track, nor kept up over it.
    layers.set({ path: "/v.mp4", track: "two", drafts: {} });
    answer.shift()!();
    await vi.waitFor(() => expect(layers.busy).toBe(true));
    answer.shift()!();
    answer.shift()!();
    await vi.waitFor(() => expect(layers.busy).toBe(false));
    expect(asked.length).toBe(3);
    expect(asked.every((a) => new URLSearchParams(a.split("?")[1]).get("track") === "one")).toBe(true);
    expect(layers.at(1.0, [10, 20])).toBeNull();
    expect(new URLSearchParams(asked[3].split("?")[1]).get("track")).toBe("two");
  });

  it("keeps the layer given last up while a draft changes, but not when the clip does", async () => {
    const { answer } = answering();
    let came = 0;
    const layers = new CaptionLayers(() => came++);
    layers.set({ path: "/v.mp4", track: "one", drafts: {} });
    layers.at(1.0, [10, 20]);
    answer.shift()!();
    await vi.waitFor(() => expect(came).toBe(1));
    expect(layers.at(1.0, [10, 20])?.pixels[0]).toBe(1);
    layers.set({ path: "/v.mp4", track: "one", drafts: { text: "#ff0000" } });
    expect(layers.at(1.0, [10, 20])?.pixels[0]).toBe(1);
    layers.set({ path: "/v.mp4", track: "two", drafts: {} });
    expect(layers.at(1.0, [10, 20])).toBeNull();
  });
});

describe("the pop over a paused frame", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  function body(red: number): ArrayBuffer {
    const b = new ArrayBuffer(8 + 4);
    const v = new DataView(b);
    [0, 0, 1, 1].forEach((n, i) => v.setUint16(2 * i, n, true));
    new Uint8Array(b, 8).set([red, 0, 0, 255]);
    return b;
  }

  // The Go side: a frame's own layer is red 100 plus the word it lights,
  // with a pop from a frame behind it to two ahead, and the layer so many
  // frames ahead is red 50 and that many.
  function goSide(word: () => number) {
    const asked: string[] = [];
    vi.stubGlobal("fetch", async (url: string) => {
      asked.push(url);
      const ahead = Number(new URLSearchParams(url.split("?")[1]).get("ahead") ?? 0);
      if (ahead) return new Response(body(50 + ahead), { headers: { "X-Caption-Showing": `0 ${word()}` } });
      return new Response(body(100 + word()), { headers: { "X-Caption-Showing": `0 ${word()}`, "X-Caption-Pop": "1 2" } });
    });
    return asked;
  }

  // Tim stepped onto a word and saw it stand still, where the browser's
  // captions before #201 popped it.
  // A step lands a frame into the word, so the pop starts a frame behind.
  it("plays the frames of the short round a frame that lights another word, from the word's first, then shows the frame's own", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout"] });
    // Lets every answer on its way come, without moving the clock.
    const flush = async () => {
      for (let i = 0; i < 10; i++) await new Promise((r) => (globalThis as unknown as { setImmediate: (f: () => void) => void }).setImmediate(() => r(undefined)));
    };
    let lit = 0;
    const asked = goSide(() => lit);
    let paused = true;
    let came = 0;
    const layers = new CaptionLayers(() => came++, { paused: () => paused, frameLength: () => 1 / 30 });
    layers.set({ path: "/v.mp4", track: "abc", drafts: {} });
    const red = (at: number) => layers.at(at, [10, 20])?.pixels[0];
    // The first frame: its word lights, and its pop plays.
    red(1.0);
    await flush();
    expect(came).toBe(1);
    expect(red(1.0)).toBe(100);
    await flush();
    expect(red(1.0)).toBe(49);
    expect(layers.popping).toBe(true);
    const ahead = asked.filter((a) => a.includes("ahead="));
    expect(ahead.map((a) => new URLSearchParams(a.split("?")[1]).get("ahead"))).toEqual(["-1", "1", "2"]);
    expect(new URLSearchParams(ahead[0].split("?")[1]).get("at")).toBe("1.000000");
    vi.advanceTimersByTime(34);
    expect(red(1.0)).toBe(100);
    vi.advanceTimersByTime(34);
    expect(red(1.0)).toBe(51);
    vi.advanceTimersByTime(34);
    expect(red(1.0)).toBe(52);
    expect(layers.popping).toBe(true);
    vi.advanceTimersByTime(34);
    expect(red(1.0)).toBe(100);
    expect(layers.popping).toBe(false);
    // Drawn again on the same word, nothing pops.
    expect(red(1.0)).toBe(100);
    // The next frame, the same word: nothing pops.
    red(1.04);
    await flush();
    expect(red(1.04)).toBe(100);
    expect(layers.stats.popped).toBe(1);
    // A step onto another word pops it, and a step away stops the pop.
    lit = 1;
    red(1.5);
    await flush();
    red(1.5);
    await flush();
    expect(red(1.5)).toBe(49);
    expect(layers.stats.popped).toBe(2);
    red(1.54);
    expect(layers.popping).toBe(false);
    // Playing, a new word pops in the frames themselves, not over them.
    paused = false;
    lit = 2;
    red(2.0);
    await flush();
    expect(red(2.0)).toBe(102);
    expect(layers.stats.popped).toBe(2);
  });
});

describe("what a layer shows", () => {
  it("is read off the header the Go side sends, and is nothing for a header it cannot read", () => {
    expect(readShowing("3 1")).toEqual({ caption: 3, word: 1 });
    expect(readShowing("0 -1")).toEqual({ caption: 0, word: -1 });
    expect(readShowing("-1 -1")).toEqual({ caption: -1, word: -1 });
    expect(readShowing(null)).toBeUndefined();
    expect(readShowing("")).toBeUndefined();
    expect(readShowing("1.5 0")).toBeUndefined();
  });

  it("reads the frames of a pop off its header, and nothing for one it cannot read", () => {
    expect(readPop("1 3")).toEqual({ behind: 1, ahead: 3 });
    expect(readPop("0 0")).toEqual({ behind: 0, ahead: 0 });
    expect(readPop("2")).toBeUndefined();
    expect(readPop("-1 2")).toBeUndefined();
    expect(readPop(null)).toBeUndefined();
    expect(readShowing("one two")).toBeUndefined();
  });
});
