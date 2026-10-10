// The canvas of the video preview. Where the webview has WebGPU it is a
// WebGPU canvas in the extended mode, half floats in which 1.0 is the
// white of the screen and brighter values are brighter light, so an HDR
// episode can shine on an HDR screen, see docs/VIDEO-PREVIEW.md, HDR.
// Where it has none it is a 2D canvas, which ends at white. Either way a
// frame is drawn as large as the canvas allows, in its own shape, in the
// middle, on whole pixels, and black around it. The frames are ffmpeg's
// colours, which light.ts gives the look of standard video or turns from
// HDR into light.

import { lift, lightOfPixel, srgb, toSDR, wgsl, type Light } from "./light";
import { Picture } from "./picture";

export interface Screen {
  readonly kind: "webgpu" | "2d";
  // Draws the frame, or black for none. The frame is only read, never
  // kept, so whoever owns it may close it straight after. What is drawn
  // stays until the next draw.
  draw(frame: Picture | null): void;
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

  constructor(private canvas: HTMLCanvasElement) {
    const g = canvas.getContext("2d", { alpha: false });
    const ownG = this.own.getContext("2d");
    if (!g || !ownG) throw new Error("the canvas has no 2d context");
    this.g = g;
    this.ownG = ownG;
  }

  draw(frame: Picture | null) {
    const c = this.canvas;
    this.g.fillStyle = "#000";
    this.g.fillRect(0, 0, c.width, c.height);
    const d = frame?.data;
    const at = frame && d && fit(c.width, c.height, frame.width, frame.height);
    if (!at) return;
    if (this.own.width !== frame.width || this.own.height !== frame.height) {
      this.own.width = frame.width;
      this.own.height = frame.height;
    }
    if (this.sdr?.width !== frame.width || this.sdr.height !== frame.height)
      this.sdr = new ImageData(frame.width, frame.height);
    toSDR(frame.light, d, this.sdr.data);
    this.ownG.putImageData(this.sdr, 0, 0);
    this.g.drawImage(this.own, at.x, at.y, at.w, at.h);
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
${wgsl}
@fragment fn fs(o: Out) -> @location(0) vec4f {
  return vec4f(shown(textureSample(frame, pick, o.uv).rgb, kind), 1);
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
  ) {
    const d = frame?.data;
    const at = frame && d && fit(width, height, frame.width, frame.height);
    if (at) {
      const texture = this.textureFor(frame.width, frame.height, frame.light);
      this.device.queue.writeTexture(
        { texture },
        d as Uint8Array<ArrayBuffer>,
        { bytesPerRow: frame.width * 4 },
        [frame.width, frame.height],
      );
    }
    const pass = encoder.beginRenderPass({
      colorAttachments: [
        { view, loadOp: "clear", storeOp: "store", clearValue: [0, 0, 0, 1] },
      ],
    });
    if (at && this.binding) {
      pass.setViewport(at.x, at.y, at.w, at.h, 0, 1);
      pass.setPipeline(this.pipeline);
      pass.setBindGroup(0, this.binding);
      pass.draw(3);
    }
    pass.end();
  }

  destroy() {
    this.texture?.destroy();
    this.lightBuffer.destroy();
  }
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

  draw(frame: Picture | null) {
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
// takes it, for the walks to hold one to the other.
export async function checkPixels(
  pixels: { light: Light; bytes: number[] }[],
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
  const gpu = await checkScreen(frames);
  if (!gpu) return null;
  const cpu = pixels.map(({ light, bytes }) => {
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

if (typeof window !== "undefined")
  (window as { __checkPixels?: typeof checkPixels }).__checkPixels =
    checkPixels;
