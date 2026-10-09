// pull.ts in a Worker: a pull is asked for with its number, and the frames
// come back with it, handed over rather than copied.
import { pull } from "./pull";

self.onmessage = async (e: MessageEvent<{ n: number; url: string; width: number; height: number }>) => {
  const { n, url, width, height } = e.data;
  if (n < 0) {
    // The page asks once whether a frame can come back from here, and
    // whether the Go side can be reached from here at all.
    const reached = await fetch(new URL("/frames/close?id=none", self.location.href).href).then(
      (r) => r.ok,
      () => false,
    );
    if (!reached) {
      self.postMessage({ n, frame: null });
      return;
    }
    const frame = new VideoFrame(new Uint8Array(8 * 8 * 4), { format: "RGBX", codedWidth: 8, codedHeight: 8, timestamp: 0 });
    self.postMessage({ n, frame }, { transfer: [frame] });
    return;
  }
  const got = await pull(new URL(url, self.location.href).href, width, height);
  self.postMessage({ n, got }, { transfer: got.frames.map((f) => f.frame) });
};
