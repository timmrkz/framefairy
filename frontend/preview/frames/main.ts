// The frame queue on its own, for a probe: the episode the harness makes
// for it, played onto one canvas, with what was drawn and what was heard
// written down for the probe to read. See "The frame queue" in the
// interface skill.
//
//   ?read   reads the frame number back from the picture of every frame
//           drawn, which costs a read of the canvas each time
//   ?hear   records every sample the sound card was handed, with the
//           sound card's own frame it was played at
import { movieOf } from "../../src/lib/frames/movie";
import { FrameQueue, type Shown } from "../../src/lib/frames/queue";

type Drawn = Shown & { pictured?: number };

declare global {
  interface Window {
    __queue: FrameQueue;
    __drawn: Drawn[];
    __heard: { frame: number; data: Float32Array }[];
    __ready: boolean | string;
  }
}

const params = new URLSearchParams(location.search);
// Headless Chromium gives back nothing of a WebGPU canvas, so a probe that
// reads the picture back has the queue draw on a 2D canvas, the way the
// walks do, see openScreen in frames/screen.ts.
if (params.has("read")) (globalThis as { __flatScreen?: boolean }).__flatScreen = true;
const canvas = document.querySelector("canvas")!;
// What the engine would say of the episode open.mjs makes for this page:
// ten minutes at 25 frames a second, 192 by 108, with its sound in Opus,
// which is decoded at 48 kHz, in one channel.
const queue = new FrameQueue(
  canvas,
  "/frames.mp4",
  movieOf({ duration: 600, width: 192, height: 108, grid: { origin: 0, start: 0, num: 25, den: 1, count: 15000 }, soundRate: 48000, soundChannels: 1 }),
);
// The canvas is as many device pixels as the stylesheet makes it, the way
// the video preview tells it.
queue.resize(canvas.clientWidth * devicePixelRatio, canvas.clientHeight * devicePixelRatio);
window.__queue = queue;
window.__drawn = [];
window.__heard = [];

// The episode's top half is its frame number in sixteen bars of twelve
// pixels, the highest bit first, so what is on the canvas can be read back
// rather than taken on trust.
const read = params.has("read");
// The 2D canvas is read through a copy of its row, which keeps the
// queue's own context out of it.
const strip = document.createElement("canvas");
const looks = strip.getContext("2d", { willReadFrequently: true })!;
function pictured(): number {
  if (strip.width !== canvas.width) strip.width = canvas.width;
  strip.height = 1;
  looks.drawImage(canvas, 0, 27, canvas.width, 1, 0, 0, canvas.width, 1);
  const row = looks.getImageData(0, 0, canvas.width, 1).data;
  let n = 0;
  for (let b = 0; b < 16; b++) {
    const x = Math.round(((b * 12 + 6) * canvas.width) / 192);
    n = n * 2 + (row[x * 4] > 125 ? 1 : 0);
  }
  return n;
}

// Every report, which while playing is every animation frame. While
// playing only the ones that drew a frame are read back.
queue.listen((s) => {
  const d: Drawn = { ...s };
  if (read && (s.drew || !s.playing)) d.pictured = pictured();
  window.__drawn.push(d);
});

const recorder = `
registerProcessor("recorder", class extends AudioWorkletProcessor {
  last = -Infinity;
  process(inputs) {
    const ch = inputs[0][0];
    // Chromium now and then hands two render quanta the same currentFrame
    // and the one after that two quanta on, so a quantum is never earlier
    // than the one before it and one on.
    const frame = Math.max(currentFrame, this.last + 128);
    this.last = frame;
    if (ch) this.port.postMessage({ frame, data: ch.slice() });
    return true;
  }
});`;

queue.ready.then(
  async () => {
    if (params.has("hear")) {
      const url = URL.createObjectURL(new Blob([recorder], { type: "text/javascript" }));
      await queue.audio.audioWorklet.addModule(url);
      const node = new AudioWorkletNode(queue.audio, "recorder", { numberOfOutputs: 1 });
      node.port.onmessage = (e) => window.__heard.push(e.data);
      const hush = queue.audio.createGain();
      hush.gain.value = 0;
      queue.out.connect(node).connect(hush).connect(queue.audio.destination);
    }
    window.__ready = true;
  },
  (e) => (window.__ready = String(e)),
);
