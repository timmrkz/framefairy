// The graphics chip, for every canvas the app draws on with it: the video
// preview, see screen.ts, and the reflection under it, see reflected.ts.
// One device for all of them, kept while it lives and asked for again
// once it is lost, to sleep or to a driver that restarts, so ten
// workspaces kept open hold one device and not twenty.
//
// A canvas on the device is held weakly here, so a workspace that goes
// takes its canvas with it, and a new device sets up only the canvases
// that are still there.

// GPUTextureUsage's COPY_SRC, COPY_DST, TEXTURE_BINDING and
// RENDER_ATTACHMENT, and GPUBufferUsage's MAP_READ, COPY_DST, COPY_SRC and
// UNIFORM: TypeScript knows the types, not the values.
export const TEXTURE = {
  copySrc: 0x01,
  copyDst: 0x02,
  binding: 0x04,
  attachment: 0x10,
};
export const BUFFER = { mapRead: 0x01, copySrc: 0x04, copyDst: 0x08, uniform: 0x40 };

// A device of its own, or null where the webview has none, for a check
// that destroys it when it is done, see checkScreen.
export async function ownDevice(): Promise<GPUDevice | null> {
  if (typeof navigator === "undefined" || !navigator.gpu) return null;
  const adapter = await navigator.gpu.requestAdapter();
  return adapter ? adapter.requestDevice() : null;
}

// What draws on a canvas with the shared device. setUp is handed each
// device the canvas draws with, the first and every one after a loss,
// and restored is told after a loss, so whatever was drawn is drawn
// again.
export interface OnGPU {
  setUp(device: GPUDevice): void;
  restored(): void;
}

const drawing = new Set<WeakRef<OnGPU>>();
let shared: Promise<GPUDevice | null> | null = null;

// The device every canvas draws with, or null where there is none.
function sharedDevice(): Promise<GPUDevice | null> {
  if (shared) return shared;
  const asked = ownDevice().catch(() => null);
  shared = asked;
  void asked.then((device) => {
    if (!device) {
      if (shared === asked) shared = null;
      return;
    }
    // An error is said once a device, the first, which the rest follow.
    let complained = false;
    device.addEventListener("uncapturederror", (e) => {
      if (complained) return;
      complained = true;
      console.error("graphics chip:", (e as GPUUncapturedErrorEvent).error.message);
    });
    void device.lost.then(async (info) => {
      if (shared === asked) shared = null;
      if (info.reason === "destroyed") return;
      console.warn("graphics chip: the GPU was lost,", info.message);
      const next = await sharedDevice();
      if (!next) return;
      for (const ref of drawing) {
        const user = ref.deref();
        if (!user) {
          drawing.delete(ref);
          continue;
        }
        user.setUp(next);
        user.restored();
      }
    });
  });
  return asked;
}

// The canvas's WebGPU context and the shared device, or null where there
// is none, or where the walks asked for the 2D canvas with __flatScreen:
// headless Chromium gives nothing back of a WebGPU canvas. A canvas keeps
// the first kind of context it is asked for, so the device is in hand
// before the canvas is asked for WebGPU, and only without one is it asked
// for the 2D context.
export async function gpuCanvas(canvas: HTMLCanvasElement): Promise<{ ctx: GPUCanvasContext; device: GPUDevice } | null> {
  if ((globalThis as { __flatScreen?: boolean }).__flatScreen === true) return null;
  const device = await sharedDevice();
  if (!device) return null;
  const ctx = canvas.getContext("webgpu") as GPUCanvasContext | null;
  return ctx ? { ctx, device } : null;
}

// Sets user up on device now and on every device after it, for as long
// as user is held by anything else.
export function onGPU(user: OnGPU, device: GPUDevice) {
  user.setUp(device);
  drawing.add(new WeakRef(user));
}
