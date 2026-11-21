// The canvas the reflection of the video preview is drawn on, see
// Reflection.svelte. Where the webview has WebGPU, the graphics chip draws
// it: it is handed the 64 by 36 cells laid over the ground, see laid in
// reflection.ts, and works out every pixel of the band from them, the
// mix of the four cells round it with the noise that keeps a faint shade
// from falling into bands. The processor's work is then the blur on the
// cells and nothing per pixel, which is what it was before the band was
// drawn on every pixel. Where there is no WebGPU, the processor draws the
// same pixels, see painted in reflection.ts, the way the video preview
// falls back to a 2D canvas, see screen.ts.
//
// The walks ask for the 2D canvas with __flatScreen, as they do for the
// video preview.

import { COLS, ROWS, band, laid, painted, raster, spread, type Raster } from "./reflection";
import { BUFFER, gpuCanvas, onGPU, ownDevice, TEXTURE, type OnGPU } from "./gpu";

export interface Reflected {
  // Gives the canvas wide by high pixels, as many as the screen has under
  // the band, and draws from at once on them, so the screen never shows
  // the black a canvas is cleared to when it is given a new size.
  size(wide: number, high: number, from: Float32Array): void;
  // Draws spread's colours over the ground.
  draw(from: Float32Array): void;
}

// The canvas's drawing, the graphics chip's where there is one, see
// gpuCanvas in gpu.ts.
export async function reflectedOn(canvas: HTMLCanvasElement, ground: readonly [number, number, number]): Promise<Reflected> {
  const gpu = await GPUReflected.open(canvas, ground).catch((e) => {
    console.warn("reflection: no WebGPU canvas,", e);
    return null;
  });
  return gpu ?? new FlatReflected(canvas, ground);
}

class FlatReflected implements Reflected {
  private g: CanvasRenderingContext2D | null;
  private image: ImageData | null = null;
  private pixels: Uint32Array | null = null;
  private drawn: Raster | null = null;

  constructor(
    private canvas: HTMLCanvasElement,
    private ground: readonly [number, number, number],
  ) {
    this.g = canvas.getContext("2d", { alpha: false });
  }

  size(wide: number, high: number, from: Float32Array) {
    if (!this.drawn || this.drawn.wide !== wide || this.drawn.high !== high) {
      this.drawn = raster(wide, high);
      this.image = new ImageData(wide, high);
      this.pixels = new Uint32Array(this.image.data.buffer);
      this.canvas.width = wide;
      this.canvas.height = high;
    }
    this.draw(from);
  }

  draw(from: Float32Array) {
    if (!this.g || !this.image || !this.pixels || !this.drawn) return;
    painted(from, this.ground, this.drawn, this.pixels);
    this.g.putImageData(this.image, 0, 0);
  }
}

// The same as painted in reflection.ts, a pixel at a time: the four cells
// round the middle of the pixel, mixed by how near it is to each, before
// the first middle and after the last the cell at the edge, and the noise
// of interleaved gradient noise, of less than a step, before it is cut to
// a whole one.
const shader = `
@vertex fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
  let p = array(vec2f(-1, -1), vec2f(3, -1), vec2f(-1, 3));
  return vec4f(p[i], 0, 1);
}

@group(0) @binding(0) var cells: texture_2d<f32>;
// The canvas's pixels across and down.
@group(0) @binding(1) var<uniform> size: vec2f;

fn cell(c: f32, r: f32) -> vec3f {
  return textureLoad(cells, vec2u(u32(c), u32(r)), 0).rgb;
}

@fragment fn fs(@builtin(position) at: vec4f) -> @location(0) vec4f {
  let n = vec2f(${COLS}.0, ${ROWS}.0);
  // The middle of the pixel, in cells, from the middle of the first.
  let u = clamp(at.xy / size * n - 0.5, vec2f(0), n - 1.0);
  let c = min(floor(u), n - 2.0);
  let f = u - c;
  let top = mix(cell(c.x, c.y), cell(c.x + 1.0, c.y), f.x);
  let bottom = mix(cell(c.x, c.y + 1.0), cell(c.x + 1.0, c.y + 1.0), f.x);
  let colour = mix(top, bottom, f.y);
  let p = floor(at.xy) % 64.0;
  let ign = fract(52.9829189 * fract(0.06711056 * p.x + 0.00583715 * p.y));
  let noise = (floor(ign * 255.0) + 0.5) / 256.0;
  return vec4f(floor(colour + noise) / 255.0, 1);
}
`;

// Draws the reflection with one device, on a target of one format.
class Painter {
  private pipeline: GPURenderPipeline;
  private cells: GPUTexture;
  private sizes: GPUBuffer;
  private binding: GPUBindGroup;
  private opaque = new Float32Array(COLS * ROWS * 4);

  constructor(
    readonly device: GPUDevice,
    format: GPUTextureFormat,
    private ground: readonly [number, number, number],
  ) {
    const module = device.createShaderModule({ code: shader });
    this.pipeline = device.createRenderPipeline({
      layout: "auto",
      vertex: { module, entryPoint: "vs" },
      fragment: { module, entryPoint: "fs", targets: [{ format }] },
    });
    this.cells = device.createTexture({
      size: [COLS, ROWS],
      format: "rgba32float",
      usage: TEXTURE.copyDst | TEXTURE.binding,
    });
    this.sizes = device.createBuffer({ size: 8, usage: BUFFER.uniform | BUFFER.copyDst });
    this.binding = device.createBindGroup({
      layout: this.pipeline.getBindGroupLayout(0),
      entries: [
        { binding: 0, resource: this.cells.createView() },
        { binding: 1, resource: { buffer: this.sizes } },
      ],
    });
  }

  // Draws spread's colours on a view of wide by high pixels.
  paint(encoder: GPUCommandEncoder, view: GPUTextureView, wide: number, high: number, from: Float32Array) {
    laid(from, this.ground, this.opaque);
    const queue = this.device.queue;
    queue.writeTexture({ texture: this.cells }, this.opaque, { bytesPerRow: COLS * 16 }, [COLS, ROWS]);
    queue.writeBuffer(this.sizes, 0, new Float32Array([wide, high]));
    const pass = encoder.beginRenderPass({
      colorAttachments: [{ view, loadOp: "clear", storeOp: "store" }],
    });
    pass.setPipeline(this.pipeline);
    pass.setBindGroup(0, this.binding);
    pass.draw(3);
    pass.end();
  }

  destroy() {
    this.cells.destroy();
    this.sizes.destroy();
  }
}

class GPUReflected implements Reflected, OnGPU {
  private painter!: Painter;
  // What was drawn last, to draw again on a device that comes back.
  private last: Float32Array | null = null;
  private complained = false;

  private constructor(
    private canvas: HTMLCanvasElement,
    private ctx: GPUCanvasContext,
    private ground: readonly [number, number, number],
    device: GPUDevice,
  ) {
    onGPU(this, device);
  }

  static async open(canvas: HTMLCanvasElement, ground: readonly [number, number, number]): Promise<GPUReflected | null> {
    const gpu = await gpuCanvas(canvas);
    return gpu ? new GPUReflected(canvas, gpu.ctx, ground, gpu.device) : null;
  }

  // The device is the one every canvas shares, see gpu.ts, which sets the
  // reflection up again on a new one when it was lost.
  setUp(device: GPUDevice) {
    const format = navigator.gpu.getPreferredCanvasFormat();
    this.ctx.configure({ device, format, alphaMode: "opaque" });
    this.painter = new Painter(device, format, this.ground);
  }

  restored() {
    if (this.last) this.draw(this.last);
  }

  private complain(what: string) {
    if (this.complained) return;
    this.complained = true;
    console.error("reflection:", what);
  }

  size(wide: number, high: number, from: Float32Array) {
    if (this.canvas.width !== wide || this.canvas.height !== high) {
      this.canvas.width = wide;
      this.canvas.height = high;
    }
    this.draw(from);
  }

  draw(from: Float32Array) {
    this.last = from;
    const c = this.canvas;
    if (!c.width || !c.height) return;
    try {
      const device = this.painter.device;
      const encoder = device.createCommandEncoder();
      this.painter.paint(encoder, this.ctx.getCurrentTexture().createView(), c.width, c.height, from);
      device.queue.submit([encoder.finish()]);
    } catch (e) {
      this.complain(String(e));
    }
  }
}

// What the graphics chip draws of spread's colours on wide by high pixels,
// as bytes, red, green, blue and alpha, row after row, beside what the
// processor draws, see painted, for the walks to hold the one to the
// other, since a WebGPU canvas gives nothing back in headless Chromium,
// see checkScreen in screen.ts. Null without WebGPU.
// The walks hand it a band of wide by high pixels of colours that change
// across and down, see the walk "screen light", and hear how far the two
// are apart at worst, in steps of 256, and how many channels differ at
// all.
async function checkBand(wide: number, high: number): Promise<{ worst: number; off: number; of: number } | null> {
  const cells = new Float32Array(COLS * ROWS * 4);
  for (let r = 0; r < ROWS; r++)
    for (let c = 0; c < COLS; c++) cells.set([(200 * c) / COLS, (120 * r) / ROWS, 80, 255], (r * COLS + c) * 4);
  const from = new Float32Array(cells.length);
  spread(cells, band(wide, high), from);
  const got = await checkReflected(from, [22, 23, 26], wide, high);
  if (!got) return null;
  let worst = 0;
  let off = 0;
  for (let i = 0; i < got.gpu.length; i++) {
    const d = Math.abs(got.gpu[i] - got.cpu[i]);
    worst = Math.max(worst, d);
    if (d) off++;
  }
  return { worst, off, of: got.gpu.length };
}

if (typeof window !== "undefined") (window as { __checkReflected?: typeof checkBand }).__checkReflected = checkBand;

async function checkReflected(
  from: Float32Array,
  ground: readonly [number, number, number],
  wide: number,
  high: number,
): Promise<{ gpu: Uint8Array; cpu: Uint8Array } | null> {
  const device = await ownDevice();
  if (!device) return null;
  const painter = new Painter(device, "rgba8unorm", ground);
  const drawn = device.createTexture({
    size: [wide, high],
    format: "rgba8unorm",
    usage: TEXTURE.attachment | TEXTURE.copySrc,
  });
  // A row of a copy is a multiple of 256 bytes.
  const row = Math.ceil((wide * 4) / 256) * 256;
  const buffer = device.createBuffer({ size: row * high, usage: BUFFER.mapRead | BUFFER.copyDst });
  const encoder = device.createCommandEncoder();
  painter.paint(encoder, drawn.createView(), wide, high, from);
  encoder.copyTextureToBuffer({ texture: drawn }, { buffer, bytesPerRow: row }, [wide, high]);
  device.queue.submit([encoder.finish()]);
  await buffer.mapAsync(1);
  const rows = new Uint8Array(buffer.getMappedRange());
  const gpu = new Uint8Array(wide * high * 4);
  for (let y = 0; y < high; y++) gpu.set(rows.subarray(y * row, y * row + wide * 4), y * wide * 4);
  buffer.unmap();
  buffer.destroy();
  drawn.destroy();
  painter.destroy();
  device.destroy();
  const pixels = new Uint32Array(wide * high);
  painted(from, ground, raster(wide, high), pixels);
  return { gpu, cpu: new Uint8Array(pixels.buffer) };
}
