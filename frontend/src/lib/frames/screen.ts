// The canvas of the video preview. Where the webview has WebGPU it is a
// WebGPU canvas in the extended mode, half floats in which 1.0 is the
// white of the screen and brighter values are brighter light, so an HDR
// episode can shine on an HDR screen, see docs/VIDEO-PREVIEW.md, HDR.
// Where it has none it is a 2D canvas, which ends at white. Either way a
// frame is drawn as large as the canvas allows, in its own shape, in the
// middle, on whole pixels, and black around it. The frames are ffmpeg's
// colours, which light.ts gives the look of standard video or turns from
// HDR into light. The captions are laid over the frame before that, the
// way the render burns them into the short before anything shows it, see
// captions.ts.

import { laidPart, inkBox, quarters, type Layer, type Over } from "./captions";
import { lift, lightOfPixel, srgb, toSDR, wgsl, type Light } from "./light";
import { Picture } from "./picture";

export interface Screen {
  readonly kind: "webgpu" | "2d";
  // Draws the frame, or black for none, with what lies over it. The frame
  // is only read, never kept, so whoever owns it may close it straight
  // after. What is drawn stays until the next draw.
  draw(frame: Picture | null, over?: Over | null): void;
  // Called once a lost GPU is back, so the frame can be drawn again.
  onRestored?: () => void;
}

// Where a frame of fw by fh goes on a canvas of cw by ch: as large as the
// side that runs out first allows, in the middle, on whole pixels.
export function fit(
  cw: number,
  ch: number,
  fw: number,
  fh: number,
): { x: number; y: number; w: number; h: number } | null {
  if (!cw || !ch || !fw || !fh) return null;
  const scale = Math.min(cw / fw, ch / fh);
  const w = Math.round(fw * scale);
  const h = Math.round(fh * scale);
  return { x: Math.round((cw - w) / 2), y: Math.round((ch - h) / 2), w, h };
}

const screens = new WeakMap<HTMLCanvasElement, Promise<Screen>>();

// The screen of this canvas, made once and kept with the canvas, so the
// frame on it stays when the queue that drew it closes, until a queue
// after it draws, see sleeping in Player.svelte.
export function screenOf(canvas: HTMLCanvasElement): Promise<Screen> {
  let screen = screens.get(canvas);
  if (!screen) {
    screen = openScreen(canvas);
    screens.set(canvas, screen);
  }
  return screen;
}

// A canvas keeps the first kind of context it is asked for, so a GPU
// device is in hand before the canvas is asked for WebGPU, and only
// without one is it asked for the 2D context.
//
// The walks read the video preview back from its canvas, which headless
// Chromium does not give for a WebGPU canvas, so they ask for the 2D
// canvas with __flatScreen and check what the GPU draws with checkPixels.
async function openScreen(canvas: HTMLCanvasElement): Promise<Screen> {
  const flat = (globalThis as { __flatScreen?: boolean }).__flatScreen === true;
  const gpu = flat
    ? null
    : await GPUScreen.open(canvas).catch((e) => {
        console.warn("video preview: no WebGPU canvas,", e);
        return null;
      });
  return gpu ?? new FlatScreen(canvas);
}

class FlatScreen implements Screen {
  readonly kind = "2d";
  private g: CanvasRenderingContext2D;
  // The frame at its own size, which is then drawn to fit.
  private own = document.createElement("canvas");
  private ownG: CanvasRenderingContext2D;
  // The frame as 8-bit colours, HDR cut at white, as an SDR screen shows
  // it.
  private sdr: ImageData | null = null;
  // The pixels the frame at its own size was made from, which a frame
  // drawn again over the same pixels, as a bounce does, does not make
  // again.
  private made: Uint8Array | null = null;

  constructor(private canvas: HTMLCanvasElement) {
    const g = canvas.getContext("2d", { alpha: false });
    const ownG = this.own.getContext("2d");
    if (!g || !ownG) throw new Error("the canvas has no 2d context");
    this.g = g;
    this.ownG = ownG;
  }

  draw(frame: Picture | null, over: Over | null = null) {
    const c = this.canvas;
    this.g.fillStyle = "#000";
    this.g.fillRect(0, 0, c.width, c.height);
    const d = frame?.data;
    const at = frame && d && fit(c.width, c.height, frame.width, frame.height);
    if (!at) return;
    if (this.made !== d || this.own.width !== frame.width || this.own.height !== frame.height) {
      if (this.own.width !== frame.width || this.own.height !== frame.height) {
        this.own.width = frame.width;
        this.own.height = frame.height;
      }
      if (this.sdr?.width !== frame.width || this.sdr.height !== frame.height)
        this.sdr = new ImageData(frame.width, frame.height);
      toSDR(frame.light, d, this.sdr.data);
      this.ownG.putImageData(this.sdr, 0, 0);
      this.made = d;
    }
    this.g.drawImage(this.own, at.x, at.y, at.w, at.h);
    // The captions, at the canvas's own pixels: the part they fall on
    // drawn again with them laid over it, before its look.
    const part = over && laidPart(d, frame.width, frame.height, at.w, at.h, over);
    if (part) {
      const img = new ImageData(part.w, part.h);
      toSDR(frame.light, part.data, img.data);
      this.g.putImageData(img, at.x + part.x, at.y + part.y);
    }
  }
}

const shader = `
struct Out {
  @builtin(position) at: vec4f,
  @location(0) uv: vec2f,
}

// One triangle over the whole viewport, which is where the frame goes.
@vertex fn vs(@builtin(vertex_index) i: u32) -> Out {
  let p = array(vec2f(-1, -1), vec2f(3, -1), vec2f(-1, 3));
  var o: Out;
  o.at = vec4f(p[i], 0, 1);
  o.uv = p[i] * vec2f(0.5, -0.5) + 0.5;
  return o;
}

@group(0) @binding(0) var pick: sampler;
@group(0) @binding(1) var frame: texture_2d<f32>;
// 0 for standard video, 1 for PQ, 2 for HLG.
@group(0) @binding(2) var<uniform> kind: u32;

// The captions, see captions.ts: the box of the layer's ink in the picture
// as shown, in shares, the share of the texture the layer fills, how many
// quarter turns clockwise the picture as shown is of the frame, and
// whether there is a layer at all. Its pixels are each taken whole, the
// way laidPart takes them, since it is drawn at the size it is shown.
// After a word step the word is lifted out of it: the box of the word at
// rest, its size in pixels, the scale it is drawn at round the middle of
// its box, and whether there is one. Its texture has a clear pixel round
// it, so sampling between its pixels fades it out at its edge, the way
// laidPart samples it.
struct Over {
  box: vec4f,
  fill: vec2f,
  turn: u32,
  on: u32,
  liftBox: vec4f,
  liftSize: vec2f,
  scale: f32,
  lifted: u32,
}
@group(1) @binding(0) var words: texture_2d<f32>;
@group(1) @binding(1) var<uniform> laid: Over;
@group(1) @binding(2) var whole: sampler;
@group(1) @binding(3) var word: texture_2d<f32>;
@group(1) @binding(4) var soft: sampler;
${wgsl}
fn turned(uv: vec2f, q: u32) -> vec2f {
  if (q == 1u) {
    return vec2f(1.0 - uv.y, uv.x);
  }
  if (q == 2u) {
    return vec2f(1.0) - uv;
  }
  if (q == 3u) {
    return vec2f(uv.y, 1.0 - uv.x);
  }
  return uv;
}

// The frame with the captions laid over it as colour plus frame times one
// less alpha, the blend the render makes, and then its look.
@fragment fn fs(o: Out) -> @location(0) vec4f {
  var c = textureSample(frame, pick, o.uv).rgb;
  let p = (turned(o.uv, laid.turn) - laid.box.xy) / max(laid.box.zw - laid.box.xy, vec2f(1e-6));
  let l = textureSampleLevel(words, whole, p * laid.fill, 0.0);
  let inside = laid.on == 1u && all(p >= vec2f(0.0)) && all(p < vec2f(1.0));
  c = select(c, l.rgb + c * (1.0 - l.a), inside);
  let d = turned(o.uv, laid.turn);
  let mid = (laid.liftBox.xy + laid.liftBox.zw) * 0.5;
  let q = mid + (d - mid) / max(laid.scale, 1e-3);
  let t = 1.0 + (q - laid.liftBox.xy) / max(laid.liftBox.zw - laid.liftBox.xy, vec2f(1e-6)) * laid.liftSize;
  let w = textureSampleLevel(word, soft, t / vec2f(textureDimensions(word)), 0.0);
  let near = laid.lifted == 1u && all(t >= vec2f(0.5)) && all(t <= laid.liftSize + 1.5);
  c = select(c, w.rgb + c * (1.0 - w.a), near);
  return vec4f(shown(c, kind), 1);
}
`;

// Every frame is 10 bits a colour with red lowest, X2BGR10, which is
// rgb10a2unorm on the GPU, see framewire.Picture. The kind says what the
// shader makes of it.
const format: GPUTextureFormat = "rgb10a2unorm";
const kinds: Record<Light, number> = { "": 0, pq: 1, hlg: 2 };

const target: GPUTextureFormat = "rgba16float";

// GPUTextureUsage's COPY_SRC, COPY_DST, TEXTURE_BINDING and
// RENDER_ATTACHMENT, and GPUBufferUsage's MAP_READ, COPY_DST, COPY_SRC and
// UNIFORM: TypeScript knows the types, not the values.
const TEXTURE = {
  copySrc: 0x01,
  copyDst: 0x02,
  binding: 0x04,
  attachment: 0x10,
};
const BUFFER = { mapRead: 0x01, copySrc: 0x04, copyDst: 0x08, uniform: 0x40 };

async function gpuDevice(): Promise<GPUDevice | null> {
  if (typeof navigator === "undefined" || !navigator.gpu) return null;
  const adapter = await navigator.gpu.requestAdapter();
  return adapter ? adapter.requestDevice() : null;
}

// Draws frames with one device: each frame is written to a texture as it
// is and drawn where it is told, in light where 1.0 is white.
class Painter {
  private pipeline: GPURenderPipeline;
  private sampler: GPUSampler;
  private lightBuffer: GPUBuffer;
  private texture: GPUTexture | null = null;
  private light: Light = "";
  private binding: GPUBindGroup | null = null;
  // The layer of captions, in a texture kept while layers fit in it, and
  // the word lifted out of it after a word step, in another.
  private wordsSampler: GPUSampler;
  private wordSampler: GPUSampler;
  private overBuffer: GPUBuffer;
  private words: GPUTexture | null = null;
  private word: GPUTexture | null = null;
  private overBinding: GPUBindGroup | null = null;
  // The pixels each texture holds now. A frame drawn again over the same
  // frame and the same captions, as on every frame of the screen while a
  // word bounces, writes none of them again: the frame alone is megabytes,
  // and writing it sixty times a second is a bounce that stutters.
  private holds: { frame?: Uint8Array; words?: Uint8Array; word?: Uint8Array } = {};

  constructor(readonly device: GPUDevice) {
    const module = device.createShaderModule({ code: shader });
    this.pipeline = device.createRenderPipeline({
      layout: "auto",
      vertex: { module, entryPoint: "vs" },
      fragment: { module, entryPoint: "fs", targets: [{ format: target }] },
    });
    this.sampler = device.createSampler({
      magFilter: "linear",
      minFilter: "linear",
    });
    this.lightBuffer = device.createBuffer({
      size: 16,
      usage: BUFFER.uniform | BUFFER.copyDst,
    });
    this.wordsSampler = device.createSampler({
      magFilter: "nearest",
      minFilter: "nearest",
    });
    this.wordSampler = device.createSampler({
      magFilter: "linear",
      minFilter: "linear",
    });
    this.overBuffer = device.createBuffer({
      size: 64,
      usage: BUFFER.uniform | BUFFER.copyDst,
    });
    this.texturesFor(1, 1, 1, 1);
  }

  // The textures the layer of captions and the word lifted out of it are
  // written to, kept while they fit in them.
  private texturesFor(w: number, h: number, ww: number, wh: number) {
    const fits = (t: GPUTexture | null, w: number, h: number) => !!t && t.width >= w && t.height >= h;
    if (fits(this.words, w, h) && fits(this.word, ww, wh)) return;
    const grown = (t: GPUTexture | null, w: number, h: number) => {
      if (fits(t, w, h)) return t!;
      t?.destroy();
      return this.device.createTexture({
        size: [Math.max(w, t?.width ?? 1), Math.max(h, t?.height ?? 1)],
        format: "rgba8unorm",
        usage: TEXTURE.copyDst | TEXTURE.binding,
      });
    };
    const words = grown(this.words, w, h);
    const word = grown(this.word, ww, wh);
    if (words !== this.words) this.holds.words = undefined;
    if (word !== this.word) this.holds.word = undefined;
    this.words = words;
    this.word = word;
    this.overBinding = this.device.createBindGroup({
      layout: this.pipeline.getBindGroupLayout(1),
      entries: [
        { binding: 0, resource: words.createView() },
        { binding: 1, resource: { buffer: this.overBuffer } },
        { binding: 2, resource: this.wordsSampler },
        { binding: 3, resource: word.createView() },
        { binding: 4, resource: this.wordSampler },
      ],
    });
  }

  // What lies over the frame, written for the shader.
  private lay(over: Over | null) {
    const l = over?.layer;
    const on = !!l && l.w > 0 && l.h > 0;
    const lift = over?.layer.lift;
    const lifted = !!lift && lift.layer.w > 0 && lift.layer.h > 0;
    const head = new ArrayBuffer(64);
    const f = new Float32Array(head);
    const u = new Uint32Array(head);
    this.texturesFor(on ? l.w : 1, on ? l.h : 1, lifted ? lift.layer.w + 2 : 1, lifted ? lift.layer.h + 2 : 1);
    if (on) {
      const words = this.words!;
      if (this.holds.words !== l.pixels) {
        this.device.queue.writeTexture(
          { texture: words },
          l.pixels as Uint8Array<ArrayBuffer>,
          { bytesPerRow: l.w * 4 },
          [l.w, l.h],
        );
        this.holds.words = l.pixels;
      }
      f.set([...inkBox(over), l.w / words.width, l.h / words.height]);
      u.set([quarters(over.turn), 1], 6);
    }
    if (lifted) {
      if (this.holds.word !== lift.layer.pixels) {
        const [w, h] = [lift.layer.w + 2, lift.layer.h + 2];
        this.device.queue.writeTexture(
          { texture: this.word! },
          bordered(lift.layer) as Uint8Array<ArrayBuffer>,
          { bytesPerRow: w * 4 },
          [w, h],
        );
        this.holds.word = lift.layer.pixels;
      }
      f.set([...inkBox({ ...over!, layer: lift.layer }), lift.layer.w, lift.layer.h, lift.scale], 8);
      u[6] = quarters(over!.turn);
      u[15] = 1;
    }
    this.device.queue.writeBuffer(this.overBuffer, 0, head);
  }

  // The texture the frame is written to, kept while frames keep their size
  // and their light.
  private textureFor(w: number, h: number, light: Light): GPUTexture {
    if (
      this.texture &&
      this.texture.width === w &&
      this.texture.height === h &&
      this.light === light
    )
      return this.texture;
    this.texture?.destroy();
    this.holds.frame = undefined;
    const kind = kinds[light];
    this.texture = this.device.createTexture({
      size: [w, h],
      format,
      usage: TEXTURE.copyDst | TEXTURE.binding,
    });
    this.light = light;
    this.device.queue.writeBuffer(
      this.lightBuffer,
      0,
      new Uint32Array([kind, 0, 0, 0]),
    );
    this.binding = this.device.createBindGroup({
      layout: this.pipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: this.sampler },
        { binding: 1, resource: this.texture.createView() },
        { binding: 2, resource: { buffer: this.lightBuffer } },
      ],
    });
    return this.texture;
  }

  // The frame on a target of width by height, black around it, encoded
  // but not yet submitted.
  paint(
    encoder: GPUCommandEncoder,
    view: GPUTextureView,
    width: number,
    height: number,
    frame: Picture | null,
    over: Over | null = null,
  ) {
    const d = frame?.data;
    const at = frame && d && fit(width, height, frame.width, frame.height);
    if (at) {
      this.lay(over);
      const texture = this.textureFor(frame.width, frame.height, frame.light);
      if (this.holds.frame !== d) {
        this.device.queue.writeTexture(
          { texture },
          d as Uint8Array<ArrayBuffer>,
          { bytesPerRow: frame.width * 4 },
          [frame.width, frame.height],
        );
        this.holds.frame = d;
      }
    }
    const pass = encoder.beginRenderPass({
      colorAttachments: [
        { view, loadOp: "clear", storeOp: "store", clearValue: [0, 0, 0, 1] },
      ],
    });
    if (at && this.binding && this.overBinding) {
      pass.setViewport(at.x, at.y, at.w, at.h, 0, 1);
      pass.setPipeline(this.pipeline);
      pass.setBindGroup(0, this.binding);
      pass.setBindGroup(1, this.overBinding);
      pass.draw(3);
    }
    pass.end();
  }

  destroy() {
    this.texture?.destroy();
    this.words?.destroy();
    this.word?.destroy();
    this.lightBuffer.destroy();
    this.overBuffer.destroy();
  }
}

// The pixels of a word lifted out of a layer with a clear pixel all round,
// which the shader samples between, so it fades out at its edge rather than
// running on into whatever the texture held before.
function bordered(l: Layer): Uint8Array {
  const w = l.w + 2;
  const out = new Uint8Array(w * (l.h + 2) * 4);
  for (let y = 0; y < l.h; y++) out.set(l.pixels.subarray(y * l.w * 4, (y + 1) * l.w * 4), ((y + 1) * w + 1) * 4);
  return out;
}

class GPUScreen implements Screen {
  readonly kind = "webgpu";
  onRestored?: () => void;
  private painter!: Painter;
  private complained = false;

  private constructor(
    private canvas: HTMLCanvasElement,
    private ctx: GPUCanvasContext,
    device: GPUDevice,
  ) {
    this.setUp(device);
  }

  static async open(canvas: HTMLCanvasElement): Promise<GPUScreen | null> {
    const device = await gpuDevice();
    if (!device) return null;
    const ctx = canvas.getContext("webgpu") as GPUCanvasContext | null;
    if (!ctx) {
      device.destroy();
      return null;
    }
    return new GPUScreen(canvas, ctx, device);
  }

  private setUp(device: GPUDevice) {
    this.ctx.configure({
      device,
      format: target,
      colorSpace: "srgb",
      alphaMode: "opaque",
      toneMapping: { mode: "extended" },
    });
    this.painter = new Painter(device);
    device.onuncapturederror = (e) => this.complain(e.error.message);
    // A GPU can be lost, to sleep or to a driver that restarts. The
    // canvas cannot become a 2D canvas then, so a new device is asked for.
    void device.lost.then(async (info) => {
      if (info.reason === "destroyed") return;
      console.warn("video preview: the GPU was lost,", info.message);
      const next = await gpuDevice().catch(() => null);
      if (!next) return;
      this.setUp(next);
      this.onRestored?.();
    });
  }

  private complain(what: string) {
    if (this.complained) return;
    this.complained = true;
    console.error("video preview:", what);
  }

  draw(frame: Picture | null, over: Over | null = null) {
    const c = this.canvas;
    if (!c.width || !c.height) return;
    try {
      const device = this.painter.device;
      const encoder = device.createCommandEncoder();
      this.painter.paint(
        encoder,
        this.ctx.getCurrentTexture().createView(),
        c.width,
        c.height,
        frame,
        over,
      );
      device.queue.submit([encoder.finish()]);
    } catch (e) {
      this.complain(String(e));
    }
  }
}

// A half float to a number.
function half(h: number): number {
  const sign = h & 0x8000 ? -1 : 1;
  const exp = (h >> 10) & 0x1f;
  const frac = h & 0x3ff;
  if (exp === 0) return sign * Math.pow(2, -14) * (frac / 1024);
  if (exp === 31) return frac ? NaN : sign * Infinity;
  return sign * Math.pow(2, exp - 15) * (1 + frac / 1024);
}

// What the GPU gives the canvas for each frame, the middle pixel of a
// frame drawn at its own size, red, green and blue as the extended canvas
// takes them. For the walks, which cannot read a WebGPU canvas back in
// headless Chromium and check the drawing this way, see
// preview/walks/bridge.mjs. Null without WebGPU.
export async function checkScreen(
  frames: Picture[],
  overs: (Over | null)[] = [],
): Promise<number[][] | null> {
  const device = await gpuDevice();
  if (!device) return null;
  const painter = new Painter(device);
  const out: number[][] = [];
  for (const frame of frames) {
    const drawn = device.createTexture({
      size: [frame.width, frame.height],
      format: target,
      usage: TEXTURE.attachment | TEXTURE.copySrc,
    });
    // A row of a copy is a multiple of 256 bytes, 32 pixels of 8.
    const buffer = device.createBuffer({
      size: 256,
      usage: BUFFER.mapRead | BUFFER.copyDst,
    });
    const encoder = device.createCommandEncoder();
    painter.paint(
      encoder,
      drawn.createView(),
      frame.width,
      frame.height,
      frame,
      overs[frames.indexOf(frame)] ?? null,
    );
    encoder.copyTextureToBuffer(
      { texture: drawn, origin: [frame.width >> 1, frame.height >> 1] },
      { buffer, bytesPerRow: 256 },
      [1, 1],
    );
    device.queue.submit([encoder.finish()]);
    await buffer.mapAsync(1);
    const h = new Uint16Array(buffer.getMappedRange().slice(0, 8));
    out.push([half(h[0]), half(h[1]), half(h[2])]);
    buffer.unmap();
    buffer.destroy();
    drawn.destroy();
  }
  painter.destroy();
  device.destroy();
  return out;
}

// A pixel as the Go side sends it, 4 bytes, drawn by the GPU and worked
// out here on the processor by light.ts, both as the extended canvas
// takes it, for the walks to hold one to the other. over is a pixel of a
// layer of captions laid over it, red, green, blue and alpha with the
// alpha multiplied in, which the processor lays over it with laidPart, and
// lift a word lifted out of that layer, laid over it grown as at the peak
// of a bounce.
export async function checkPixels(
  pixels: { light: Light; bytes: number[]; over?: number[]; lift?: number[] }[],
): Promise<{ gpu: number[][]; cpu: number[][] } | null> {
  const frames = pixels.map(
    (p) =>
      new Picture(
        new Uint8Array([...p.bytes, ...p.bytes, ...p.bytes, ...p.bytes]),
        2,
        2,
        0,
        p.light,
      ),
  );
  // A layer of 2 by 2 over the whole of each frame, the turn of the
  // second quarter, so the turn is gone through too.
  const square = (p: number[]) => ({ x: 0, y: 0, w: 2, h: 2, pixels: new Uint8Array([...p, ...p, ...p, ...p]) });
  const overs = pixels.map(({ over, lift }) =>
    over || lift
      ? {
          layer: { ...square(over ?? [0, 0, 0, 0]), ...(lift ? { lift: { layer: square(lift), scale: 1.15 } } : {}) },
          size: [2, 2] as [number, number],
          left: 0,
          width: 1,
          turn: -180,
        }
      : null,
  );
  const gpu = await checkScreen(frames, overs);
  if (!gpu) return null;
  const cpu = pixels.map(({ light, bytes: sent }, i) => {
    const o = overs[i];
    const bytes = o ? [...laidPart(new Uint8Array(sent), 1, 1, 1, 1, o)!.data] : sent;
    const w =
      (bytes[0] | (bytes[1] << 8) | (bytes[2] << 16) | (bytes[3] << 24)) >>> 0;
    const c = [w & 1023, (w >>> 10) & 1023, (w >>> 20) & 1023].map(
      (v) => v / 1023,
    );
    if (!light) return c.map(lift);
    const l = lightOfPixel(light, c[0], c[1], c[2]);
    return l.map((v) => Math.sign(v) * srgb(Math.abs(v)));
  });
  return { gpu, cpu };
}

// A white word of 2 by 2 in the middle of a grey frame of 8 by 8, lifted
// out and grown by each scale, drawn whole by the GPU and by laidPart, as
// the extended canvas takes them, for the walks to hold one to the other
// pixel by pixel: the word grows round its middle in every direction
// alike on both. Checked at one middle pixel before, a word grown one way
// only passed, found by the second review. Null without WebGPU.
export async function checkLift(scales: number[]): Promise<{ gpu: number[][][]; cpu: number[][][] } | null> {
  const device = await gpuDevice();
  if (!device) return null;
  const grey = (512 | (512 << 10) | (512 << 20)) >>> 0;
  const bytes = new Uint8Array(new Uint32Array(64).fill(grey).buffer);
  const frame = new Picture(bytes, 8, 8, 0, "");
  const white = new Uint8Array(16).fill(255);
  const overs: Over[] = scales.map((scale) => ({
    layer: { x: 0, y: 0, w: 1, h: 1, pixels: new Uint8Array(4), lift: { layer: { x: 3, y: 3, w: 2, h: 2, pixels: white }, scale } },
    size: [8, 8],
    left: 0,
    width: 1,
    turn: 0,
  }));
  const painter = new Painter(device);
  const gpu: number[][][] = [];
  for (const over of overs) {
    const drawn = device.createTexture({ size: [8, 8], format: target, usage: TEXTURE.attachment | TEXTURE.copySrc });
    // A row of a copy is a multiple of 256 bytes.
    const buffer = device.createBuffer({ size: 256 * 8, usage: BUFFER.mapRead | BUFFER.copyDst });
    const encoder = device.createCommandEncoder();
    painter.paint(encoder, drawn.createView(), 8, 8, frame, over);
    encoder.copyTextureToBuffer({ texture: drawn }, { buffer, bytesPerRow: 256 }, [8, 8]);
    device.queue.submit([encoder.finish()]);
    await buffer.mapAsync(1);
    const h = new Uint16Array(buffer.getMappedRange().slice(0));
    gpu.push(Array.from({ length: 64 }, (_, i) => [0, 1, 2].map((c) => half(h[(i >> 3) * 128 + (i & 7) * 4 + c]))));
    buffer.unmap();
    buffer.destroy();
    drawn.destroy();
  }
  painter.destroy();
  device.destroy();
  const cpu = overs.map((over) => {
    const words = new Uint32Array(64).fill(grey);
    const part = laidPart(bytes, 8, 8, 8, 8, over);
    if (part) {
      const from = new Uint32Array(part.data.buffer);
      for (let y = 0; y < part.h; y++)
        for (let x = 0; x < part.w; x++) words[(part.y + y) * 8 + part.x + x] = from[y * part.w + x];
    }
    return [...words].map((w) => [w & 1023, (w >>> 10) & 1023, (w >>> 20) & 1023].map((v) => lift(v / 1023)));
  });
  return { gpu, cpu };
}

if (typeof window !== "undefined")
  (window as { __checkPixels?: typeof checkPixels }).__checkPixels =
    checkPixels;
if (typeof window !== "undefined") (window as { __checkLift?: typeof checkLift }).__checkLift = checkLift;
