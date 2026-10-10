// pull.ts in a Worker: a pull is asked for with its number, and the frames
// come back with it, handed over rather than copied.
import { pull } from "./pull";

self.onmessage = async (e: MessageEvent<{ n: number; url: string; width: number; height: number }>) => {
  const { n, url, width, height } = e.data;
  if (n < 0) {
    // The page asks once whether the Go side can be reached from here.
    const reached = await fetch(new URL("/frames/close?id=none", self.location.href).href).then(
      (r) => r.ok,
      () => false,
    );
    self.postMessage({ n, reached });
    return;
  }
  const got = await pull(new URL(url, self.location.href).href, width, height);
  self.postMessage({ n, got }, { transfer: got.frames.map((f) => f.data) });
};
