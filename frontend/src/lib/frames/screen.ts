// The canvas of the video preview. Where the webview has WebGPU it is a
// WebGPU canvas in the extended mode, half floats in which 1.0 is the
// white of the screen and brighter values are brighter light, so an HDR
// episode can shine on an HDR screen, see docs/VIDEO-PREVIEW.md, HDR.
// Where it has none it is a 2D canvas, which ends at white. Either way a
// frame is drawn as large as the canvas allows, in its own shape, in the
// middle, on whole pixels, and black around it. Nothing here decides
// colour: the frames are ffmpeg's colours and are drawn as they come.

import type { Picture } from "./picture";

export interface Screen {
  readonly kind: "webgpu" | "2d";
  // Draws the frame, or black for none. The frame is only read, never
  // kept, so whoever owns it may close it straight after.
  draw(frame: Picture | null): void;
  // Called once a lost GPU is back, so the frame can be drawn again.
  onRestored?: () => void;
  close(): void;
}

// Where a frame of fw by fh goes on a canvas of cw by ch: as large as the
// side that runs out first allows, in the middle, on whole pixels.
export function fit(cw: number, ch: number, fw: number, fh: number): { x: number; y: number; w: number; h: number } | null {
  if (!cw || !ch || !fw || !fh) return null;
  const scale = Math.min(cw / fw, ch / fh);
  const w = Math.round(fw * scale);
  const h = Math.round(fh * scale);
  return { x: Math.round((cw - w) / 2), y: Math.round((ch - h) / 2), w, h };
}

// The screen for this canvas. A canvas keeps the first kind of context it
// is asked for, so a GPU device is in hand before the canvas is asked for
// WebGPU, and only without one is it asked for the 2D context.
export async function openScreen(canvas: HTMLCanvasElement): Promise<Screen> {
  const gpu = await GPUScreen.open(canvas).catch((e) => {
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
    this.ownG.putImageData(new ImageData(new Uint8ClampedArray(d.buffer as ArrayBuffer, d.byteOffset, d.byteLength), frame.width, frame.height), 0, 0);
    this.g.drawImage(this.own, at.x, at.y, at.w, at.h);
  }

  close() {}
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

@fragment fn fs(o: Out) -> @location(0) vec4f {
  return vec4f(textureSample(frame, pick, o.uv).rgb, 1);
}
`;

const target: GPUTextureFormat = "rgba16float";

// A frame of colours in 8 bits, see picture.ts, is written to a texture as
// it is and drawn on the canvas, where 1.0 is white. GPUTextureUsage's
// COPY_DST and TEXTURE_BINDING: TypeScript knows the types, not the values.
const frameUsage = 0x02 | 0x04;

class GPUScreen implements Screen {
  readonly kind = "webgpu";
  onRestored?: () => void;
  private pipeline!: GPURenderPipeline;
  private sampler!: GPUSampler;
  private texture: GPUTexture | null = null;
  private binding: GPUBindGroup | null = null;
  private closed = false;
  private complained = false;

  private constructor(
    private canvas: HTMLCanvasElement,
    private ctx: GPUCanvasContext,
    private device: GPUDevice,
  ) {
    this.setUp();
  }

  static async open(canvas: HTMLCanvasElement): Promise<GPUScreen | null> {
    const device = await GPUScreen.device();
    if (!device) return null;
    const ctx = canvas.getContext("webgpu") as GPUCanvasContext | null;
    if (!ctx) {
      device.destroy();
      return null;
    }
    return new GPUScreen(canvas, ctx, device);
  }

  private static async device(): Promise<GPUDevice | null> {
    if (typeof navigator === "undefined" || !navigator.gpu) return null;
    const adapter = await navigator.gpu.requestAdapter();
    return adapter ? adapter.requestDevice() : null;
  }

  private setUp() {
    const device = this.device;
    this.ctx.configure({
      device,
      format: target,
      colorSpace: "srgb",
      alphaMode: "opaque",
      toneMapping: { mode: "extended" },
    });
    const module = device.createShaderModule({ code: shader });
    this.pipeline = device.createRenderPipeline({
      layout: "auto",
      vertex: { module, entryPoint: "vs" },
      fragment: { module, entryPoint: "fs", targets: [{ format: target }] },
    });
    this.sampler = device.createSampler({ magFilter: "linear", minFilter: "linear" });
    this.texture = null;
    this.binding = null;
    device.onuncapturederror = (e) => this.complain(e.error.message);
    // A GPU can be lost, to sleep or to a driver that restarts. The
    // canvas cannot become a 2D canvas then, so a new device is asked for.
    void device.lost.then(async (info) => {
      if (this.closed || info.reason === "destroyed") return;
      console.warn("video preview: the GPU was lost,", info.message);
      const next = await GPUScreen.device().catch(() => null);
      if (!next || this.closed) return;
      this.device = next;
      this.setUp();
      this.onRestored?.();
    });
  }

  private complain(what: string) {
    if (this.complained) return;
    this.complained = true;
    console.error("video preview:", what);
  }

  // The texture the frame is copied to, kept while frames keep their size.
  private textureFor(w: number, h: number): GPUTexture {
    if (this.texture && this.texture.width === w && this.texture.height === h) return this.texture;
    this.texture?.destroy();
    this.texture = this.device.createTexture({ size: [w, h], format: "rgba8unorm", usage: frameUsage });
    this.binding = this.device.createBindGroup({
      layout: this.pipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: this.sampler },
        { binding: 1, resource: this.texture.createView() },
      ],
    });
    return this.texture;
  }

  draw(frame: Picture | null) {
    if (this.closed) return;
    const c = this.canvas;
    if (!c.width || !c.height) return;
    try {
      const d = frame?.data;
      const at = frame && d && fit(c.width, c.height, frame.width, frame.height);
      if (at) {
        const texture = this.textureFor(frame.width, frame.height);
        this.device.queue.writeTexture({ texture }, d as Uint8Array<ArrayBuffer>, { bytesPerRow: frame.width * 4 }, [frame.width, frame.height]);
      }
      const encoder = this.device.createCommandEncoder();
      const pass = encoder.beginRenderPass({
        colorAttachments: [{ view: this.ctx.getCurrentTexture().createView(), loadOp: "clear", storeOp: "store", clearValue: [0, 0, 0, 1] }],
      });
      if (at && this.binding) {
        pass.setViewport(at.x, at.y, at.w, at.h, 0, 1);
        pass.setPipeline(this.pipeline);
        pass.setBindGroup(0, this.binding);
        pass.draw(3);
      }
      pass.end();
      this.device.queue.submit([encoder.finish()]);
    } catch (e) {
      this.complain(String(e));
    }
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    this.texture?.destroy();
    this.ctx.unconfigure();
    this.device.destroy();
  }
}
