import { afterEach, describe, expect, it, vi } from "vitest";
import {
  bounceScale,
  CaptionLayers,
  inkBox,
  laidPart,
  readBounce,
  readLayer,
  readShowing,
  turned,
  unturned,
  type Layer,
  type Over,
} from "./captions";

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
    layers.set({ path: "/v.mp4", clip: "a", track: "abc", drafts: { text: "#ffffff" } });
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
      layers.set({ path: "/v.mp4", clip: "a", track: "abc", drafts: {} });
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
      const { asked, answer } = answering((n) => n <= 4);
      // What the layers say in the app's log, see said.ts. A failed
      // answer said nothing before, found by the second review.
      const told: string[] = [];
      const layers = new CaptionLayers(
        () => {},
        undefined,
        (text, video) => told.push(`${video}: ${text}`),
      );
      layers.set({ path: "/v.mp4", clip: "a", track: "abc", drafts: {} });
      layers.at(1.0, [10, 20]);
      answer.shift()!();
      await vi.waitFor(() => expect(vi.getTimerCount()).toBe(1));
      expect(told).toEqual([]);
      vi.advanceTimersByTime(300);
      expect(asked.length).toBe(2);
      answer.shift()!();
      await vi.waitFor(() => expect(layers.busy).toBe(false));
      expect(vi.getTimerCount()).toBe(0);
      expect(asked.length).toBe(2);
      expect(told).toEqual(["/v.mp4: captions: a layer did not come, Error: gone"]);
      // Nothing was kept, so the next draw of the frame asks once more.
      expect(layers.at(1.0, [10, 20])).toBeNull();
      expect(asked.length).toBe(3);
      // Given up on again for the same reason, it is not said again.
      answer.shift()!();
      await vi.waitFor(() => expect(layers.busy).toBe(false));
      expect(asked.length).toBe(3);
      expect(told.length).toBe(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("never asks for a layer with an ask newer than the frame's, and drops another clip's captions", async () => {
    const { asked, answer } = answering();
    const layers = new CaptionLayers(() => {});
    layers.set({ path: "/v.mp4", clip: "a", track: "one", drafts: {} });
    // Three on their way fill the room, so the frame drawn waits for it.
    layers.coming(1.0, [10, 20]);
    layers.coming(1.04, [10, 20]);
    layers.coming(1.08, [10, 20]);
    layers.at(1.12, [10, 20]);
    expect(asked.length).toBe(3);
    // Another clip is chosen before any came: what waited is not asked for
    // with the new clip's track, nor kept up over it.
    layers.set({ path: "/v.mp4", clip: "b", track: "two", drafts: {} });
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

  // Found by the second review: the track changes with every saved edit
  // and every step of a caption edge being dragged, and the captions
  // blinked off each time until the new track's layer came.
  it("keeps the layer given last up while a draft or the track changes, but not when the clip does", async () => {
    const { answer } = answering();
    let came = 0;
    const layers = new CaptionLayers(() => came++);
    layers.set({ path: "/v.mp4", clip: "a", track: "one", drafts: {} });
    layers.at(1.0, [10, 20]);
    answer.shift()!();
    await vi.waitFor(() => expect(came).toBe(1));
    expect(layers.at(1.0, [10, 20])?.pixels[0]).toBe(1);
    layers.set({ path: "/v.mp4", clip: "a", track: "one", drafts: { text: "#ff0000" } });
    expect(layers.at(1.0, [10, 20])?.pixels[0]).toBe(1);
    layers.set({ path: "/v.mp4", clip: "a", track: "two", drafts: {} });
    expect(layers.at(1.0, [10, 20])?.pixels[0]).toBe(1);
    layers.set({ path: "/v.mp4", clip: "b", track: "three", drafts: {} });
    expect(layers.at(1.0, [10, 20])).toBeNull();
    layers.set({ path: "/v.mp4", clip: "b", track: "three", drafts: {} });
    layers.set(null);
    expect(layers.at(1.0, [10, 20])).toBeNull();
  });
});

describe("a layer laid while the app is resized", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // Found by the second review: the layer given last, drawn at the old
  // size, was laid by the new one until its own came, so the captions
  // jumped toward the crop's top left on every step of a resize.
  it("is laid by the size it was drawn at, where the new size's own will stand", async () => {
    const answers: (() => void)[] = [];
    vi.stubGlobal(
      "fetch",
      (url: string) =>
        new Promise<Response>((resolve) => {
          const q = new URLSearchParams(url.split("?")[1]);
          const w = Number(q.get("w"));
          // The same ink at every size: a quarter in and down, a half
          // wide and high.
          const b = new ArrayBuffer(8 + (w / 2) * (w / 2) * 4);
          const v = new DataView(b);
          [w / 4, w / 4, w / 2, w / 2].forEach((n, i) => v.setUint16(2 * i, n, true));
          answers.push(() => resolve(new Response(b)));
        }),
    );
    let came = 0;
    const layers = new CaptionLayers(() => came++);
    layers.set({ path: "/v.mp4", clip: "a", track: "one", drafts: {} });
    layers.at(1.0, [20, 20]);
    answers.shift()!();
    await vi.waitFor(() => expect(came).toBe(1));
    const own = layers.at(1.0, [20, 20])!;
    expect(own.size).toEqual([20, 20]);
    // The app grows: the old layer stays up until the new one comes.
    const old = layers.at(1.0, [40, 40])!;
    expect(old).toBe(own);
    const at = (layer: Layer, size: [number, number]) => inkBox({ layer, size, left: 0.1, width: 0.8, turn: 0 });
    answers.shift()!();
    await vi.waitFor(() => expect(came).toBe(2));
    const grown = layers.at(1.0, [40, 40])!;
    expect(grown.size).toEqual([40, 40]);
    expect(at(old, [40, 40])).toEqual(at(grown, [40, 40]));
    [0.3, 0.25, 0.7, 0.75].forEach((v, i) => expect(at(old, [40, 40])[i]).toBeCloseTo(v, 9));
  });
});

describe("the bounce after a word step", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function body(red: number): ArrayBuffer {
    const b = new ArrayBuffer(8 + 4);
    const v = new DataView(b);
    [0, 0, 1, 1].forEach((n, i) => v.setUint16(2 * i, n, true));
    new Uint8Array(b, 8).set([red, 0, 0, 255]);
    return b;
  }

  // The Go side: a frame's own layer is red 100 plus the word it lights,
  // drawn at scale, and the rest of the caption apart is red 20 and the
  // word apart red 30, with the curve it bounces by.
  function goSide(word: () => number, scale: () => string) {
    const asked: string[] = [];
    vi.stubGlobal("fetch", async (url: string) => {
      asked.push(url);
      const q = new URLSearchParams(url.split("?")[1]);
      const showing = { "X-Caption-Showing": `0 ${word()}` };
      if (q.get("part") === "rest") return new Response(body(20), { headers: { ...showing, "X-Caption-Scale": "1.0000" } });
      if (q.get("part") === "word")
        return new Response(body(30), { headers: { ...showing, "X-Caption-Scale": "1.0000", "X-Caption-Bounce": "220 0.55 1.15" } });
      return new Response(body(100 + word()), { headers: { ...showing, "X-Caption-Scale": scale() } });
    });
    return asked;
  }

  // The screen's frames, one at a time, at the moment given.
  function screen() {
    let waiting: [number, FrameRequestCallback][] = [];
    let n = 0;
    vi.stubGlobal("requestAnimationFrame", (f: FrameRequestCallback) => {
      waiting.push([++n, f]);
      return n;
    });
    vi.stubGlobal("cancelAnimationFrame", (id: number) => {
      waiting = waiting.filter(([k]) => k !== id);
    });
    return {
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

  // Lets every answer on its way come.
  const flush = async () => {
    for (let i = 0; i < 10; i++) await new Promise((r) => setTimeout(r, 0));
  };

  // Tim saw the pop the short burns in played again over every frame he
  // stepped to, the arrow keys included, and wanted a step from word to
  // word to bounce its word and a step from frame to frame to show only
  // the short's frames. Then he found the bounce laggy, the short's frames
  // of the pop played at 25 a second, and wanted it the same for every
  // word, a word of one frame too.
  it("bounces the word a word step lands on at the screen's rate, and shows only the frame after any other step", async () => {
    let lit = 0;
    let scale = "1.0000";
    const asked = goSide(
      () => lit,
      () => scale,
    );
    const s = screen();
    let paused = true;
    let came = 0;
    const layers = new CaptionLayers(() => came++, { paused: () => paused });
    layers.set({ path: "/v.mp4", clip: "a", track: "abc", drafts: {} });
    const at = (t: number) => layers.at(t, [10, 20]);
    // The first frame shows its own layer, and nothing bounces.
    at(1.0);
    await flush();
    expect(came).toBe(1);
    expect(at(1.0)?.pixels[0]).toBe(100);
    expect(layers.popping).toBe(false);
    // An arrow alone onto another word shows its frame and nothing over it.
    layers.sought(false);
    lit = 1;
    at(1.04);
    await flush();
    expect(at(1.04)?.pixels[0]).toBe(101);
    expect(at(1.04)?.lift).toBeUndefined();
    expect(layers.stats.popped).toBe(0);
    // A word step lifts the word it lands on out of its caption. The
    // frame's own layer, at rest, stays while the parts are on their way.
    layers.sought(true);
    lit = 2;
    at(1.5);
    await flush();
    expect(at(1.5)?.pixels[0]).toBe(102);
    expect(layers.popping).toBe(true);
    expect(layers.busy).toBe(true);
    await flush();
    const parts = asked.filter((a) => a.includes("part="));
    expect(parts.map((a) => new URLSearchParams(a.split("?")[1]).get("part")).sort()).toEqual(["rest", "word"]);
    expect(new URLSearchParams(parts[0].split("?")[1]).get("at")).toBe("1.500000");
    expect(parts.some((a) => a.includes("ahead") || a.includes("bounce"))).toBe(false);
    // The rest over the frame, the word over that, at rest until the
    // screen's first frame.
    let l = at(1.5)!;
    expect(l.pixels[0]).toBe(20);
    expect(l.lift?.layer.pixels[0]).toBe(30);
    expect(l.lift?.scale).toBe(1);
    expect(l.showing).toEqual({ caption: 0, word: 2 });
    // Then one scale for each frame of the screen, by its clock: up to its
    // peak a little past the middle, and back to rest.
    const scales: number[] = [];
    for (let ms = 1000; s.waiting; ms += 1000 / 60) {
      s.frame(ms);
      scales.push(at(1.5)!.lift!.scale);
    }
    expect(scales.length).toBeGreaterThanOrEqual(13);
    expect(scales[0]).toBe(1);
    expect(Math.max(...scales)).toBeGreaterThan(1.149);
    expect(scales[scales.length - 1]).toBe(1);
    const top = scales.indexOf(Math.max(...scales));
    scales.slice(1, top + 1).forEach((v, k) => expect(v).toBeGreaterThan(scales[k]));
    scales.slice(top + 1).forEach((v, k) => expect(v).toBeLessThan(scales[top + k]));
    expect(at(1.5)!.scale).toBe(1);
    expect(layers.stats.bounced).toBe(scales.length);
    // At rest it stays over the frame, every word alike, though the short
    // may show the frame otherwise.
    expect(layers.popping).toBe(false);
    expect(layers.busy).toBe(false);
    expect(at(1.5)?.lift?.layer.pixels[0]).toBe(30);
    expect(layers.stats.popped).toBe(1);
    // A draft changed over that frame lifts the word again at rest, with
    // no bounce.
    layers.set({ path: "/v.mp4", clip: "a", track: "abc", drafts: { text: "#ffffff" } });
    expect(at(1.5)?.lift?.scale).toBe(1);
    await flush();
    at(1.5);
    await flush();
    expect(at(1.5)?.lift?.layer.pixels[0]).toBe(30);
    expect(s.waiting).toBe(0);
    expect(layers.stats.popped).toBe(1);
    // An arrow alone: nothing bounces, and the frame shows its own layer.
    layers.sought(false);
    lit = 3;
    at(1.54);
    await flush();
    expect(at(1.54)?.pixels[0]).toBe(103);
    expect(at(1.54)?.lift).toBeUndefined();
    // A word of one frame, which the short shows at its peak: the layer
    // before stays while the parts come, and the bounce starts at rest.
    layers.sought(true);
    lit = 4;
    scale = "1.1500";
    at(1.6);
    await flush();
    expect(at(1.6)?.pixels[0]).toBe(103);
    await flush();
    s.frame(5000);
    expect(at(1.6)?.lift?.scale).toBe(1);
    s.frame(5100);
    expect(at(1.6)?.lift?.scale).toBeGreaterThan(1.1);
    expect(layers.popping).toBe(true);
    // A step away puts the word back at once.
    layers.sought(false);
    scale = "1.0000";
    at(1.64);
    expect(layers.popping).toBe(false);
    expect(s.waiting).toBe(0);
    expect(layers.stats.popped).toBe(2);
    // Playing, nothing bounces over the frames.
    paused = false;
    layers.sought(true);
    lit = 5;
    at(2.0);
    await flush();
    expect(at(2.0)?.pixels[0]).toBe(105);
    expect(layers.stats.popped).toBe(2);
  });

  // Found by the second review: Shift and an arrow pressed twice quickly
  // bounced the word passed over when its layer came, and the word landed
  // on sat still.
  it("bounces the word a second quick word step lands on, not the one it passed over", async () => {
    const held: { at: string; part: string | null; go: () => void }[] = [];
    vi.stubGlobal(
      "fetch",
      (url: string) =>
        new Promise<Response>((resolve) => {
          const q = new URLSearchParams(url.split("?")[1]);
          const at = q.get("at")!;
          const part = q.get("part");
          const red = part === "rest" ? 20 : part === "word" ? 30 : 100;
          const headers: Record<string, string> = { "X-Caption-Showing": "0 1", "X-Caption-Scale": "1.0000" };
          if (part === "word") headers["X-Caption-Bounce"] = "220 0.55 1.15";
          held.push({ at, part, go: () => resolve(new Response(body(red), { headers })) });
        }),
    );
    screen();
    const layers = new CaptionLayers(() => {}, { paused: () => true });
    layers.set({ path: "/v.mp4", clip: "a", track: "abc", drafts: {} });
    const at = (t: number) => layers.at(t, [10, 20]);
    at(1.0);
    held.shift()!.go();
    await flush();
    expect(at(1.0)?.pixels[0]).toBe(100);
    // Two word steps, the second before the first frame's layer came.
    // The screen still shows the frame passed over while the one landed on
    // is decoded, and draws it again when its layer comes.
    layers.sought(true);
    at(1.04);
    layers.sought(true);
    held.splice(held.findIndex((h) => h.at === "1.040000"), 1)[0].go();
    await flush();
    expect(at(1.04)?.pixels[0]).toBe(100);
    expect(layers.stats.popped).toBe(0);
    at(1.08);
    held.splice(held.findIndex((h) => h.at === "1.080000" && !h.part), 1)[0].go();
    await flush();
    at(1.08);
    expect(layers.stats.popped).toBe(1);
    const parts = held.filter((h) => h.part);
    expect(parts.map((h) => h.at)).toEqual(["1.080000", "1.080000"]);
  });

  it("follows one curve, at rest at either end and at its peak a little past the middle", () => {
    const b = readBounce("220 0.55 1.15")!;
    expect(b).toEqual({ length: 220, rise: 0.55, peak: 1.15 });
    expect(bounceScale(-5, b)).toBe(1);
    expect(bounceScale(0, b)).toBe(1);
    expect(bounceScale(121, b)).toBeCloseTo(1.15, 9);
    expect(bounceScale(220, b)).toBe(1);
    expect(bounceScale(500, b)).toBe(1);
    expect(bounceScale(60, b)).toBeGreaterThan(1.05);
    for (const bad of [null, "", "220", "0 0.5 1.1", "220 1.5 1.1", "220 0.5 3"]) expect(readBounce(bad)).toBeUndefined();
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

describe("what a layer shows", () => {
  it("is read off the header the Go side sends, and is nothing for a header it cannot read", () => {
    expect(readShowing("3 1")).toEqual({ caption: 3, word: 1 });
    expect(readShowing("0 -1")).toEqual({ caption: 0, word: -1 });
    expect(readShowing("-1 -1")).toEqual({ caption: -1, word: -1 });
    expect(readShowing(null)).toBeUndefined();
    expect(readShowing("")).toBeUndefined();
    expect(readShowing("1.5 0")).toBeUndefined();
  });

  it("is nothing for words where numbers should be", () => {
    expect(readShowing("one two")).toBeUndefined();
  });
});
