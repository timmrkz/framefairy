// The captions of the video preview. They are not drawn here: libass draws
// them on the Go side, in the episode's decoder, from the caption file the
// render burns into the short, at the moment libass reads on that frame of
// the short, see /frames/captions in cmd/framefairy-app/captions.go and
// docs/CAPTIONS.md. What comes back is a layer, which the screen lays over
// the frame before it gives the frame its look, as colour plus frame times
// one less alpha, the way the render blends it. So the video preview, the
// thumbnails and the short show one drawing, pixel for pixel, the bounce
// and the padding of the highlight included. Tim saw them differ while a
// browser drew the preview's.

// The captions at a moment: the box of the drawing that has any ink in it,
// and its pixels, red, green, blue and alpha, the alpha multiplied in, in
// full range, see framewire.Layer. A moment with no captions is a box of
// nothing.
// A layer from the Go side also says which captions it was drawn from,
// track, and what it shows of them, showing.
// And which frames of the short the video preview plays over the frame,
// paused, for its word to pop, pop, so many before its own and so many
// after, see Replay.
export type Layer = {
  x: number;
  y: number;
  w: number;
  h: number;
  pixels: Uint8Array;
  track?: string;
  showing?: Showing;
  pop?: Pop;
};

// Frames of the short before a frame and after it, as the Go side says it
// in the header X-Caption-Pop, "<behind> <ahead>", see engine.PopFrames.
export type Pop = { behind: number; ahead: number };

// What a layer shows: the caption, by its place among the captions of the
// view, and the word of it being spoken, counted across its lines, each
// -1 for none, as the Go side says it in the header X-Caption-Showing, see
// engine.CaptionShowing. The caption box over the video preview shows the
// caption and the word the layer on screen shows, so the frame round a word
// is never a frame ahead of the pill under it. It followed the playhead,
// which runs ahead of the frame on screen while the episode plays, and Tim
// saw the frame go to the next caption a few frames before the pill did.
export type Showing = { caption: number; word: number };

export function readShowing(header: string | null): Showing | undefined {
  const [caption, word] = (header ?? "").split(" ").map(Number);
  return Number.isInteger(caption) && Number.isInteger(word) ? { caption, word } : undefined;
}

// The frames of a pop, from the header X-Caption-Pop, nothing when it is
// missing or not two counts.
export function readPop(header: string | null): Pop | undefined {
  const m = /^(\d+) (\d+)$/.exec(header ?? "");
  return m ? { behind: Number(m[1]), ahead: Number(m[2]) } : undefined;
}

// The size of a layer's box in its body: four 16-bit numbers.
const HEAD = 8;

export function readLayer(body: ArrayBuffer): Layer | null {
  if (body.byteLength < HEAD) return null;
  const v = new DataView(body);
  const [x, y, w, h] = [0, 2, 4, 6].map((at) => v.getUint16(at, true));
  const pixels = new Uint8Array(body, HEAD);
  return pixels.length === w * h * 4 ? { x, y, w, h, pixels } : null;
}

// What lies over a frame: a layer drawn at size, the crop's own pixels on
// the screen, laid over the crop, which stands at left and is width wide in
// shares of the picture as it is shown, from top to bottom. The picture is
// the frame turned by turn degrees the way the video preview turns it, see
// turned.
export type Over = {
  layer: Layer;
  size: [number, number];
  left: number;
  width: number;
  turn: number;
};

// Where a point of the frame, u and v as shares of its width and height
// from its top left, is in the picture as shown: the frame is turned by
// the stylesheet through -turn degrees, see .turned canvas in
// Player.svelte, and a quarter turn clockwise takes the top left corner to
// the top right.
export function turned(u: number, v: number, turn: number): [number, number] {
  switch (quarters(turn)) {
    case 1:
      return [1 - v, u];
    case 2:
      return [1 - u, 1 - v];
    case 3:
      return [v, 1 - u];
    default:
      return [u, v];
  }
}

// Where a point of the picture as shown is in the frame, the other way
// round from turned.
export function unturned(u: number, v: number, turn: number): [number, number] {
  switch (quarters(turn)) {
    case 1:
      return [v, 1 - u];
    case 2:
      return [1 - u, 1 - v];
    case 3:
      return [1 - v, u];
    default:
      return [u, v];
  }
}

// How many quarter turns clockwise the picture as shown is of the frame.
export function quarters(turn: number): number {
  return ((Math.round(-turn / 90) % 4) + 4) % 4;
}

// The box of the layer's ink in the picture as shown, in shares.
export function inkBox(o: Over): [number, number, number, number] {
  const [W, H] = o.size;
  const l = o.layer;
  return [
    o.left + (o.width * l.x) / W,
    l.y / H,
    o.left + (o.width * (l.x + l.w)) / W,
    (l.y + l.h) / H,
  ];
}

// The part of a frame drawn at width by height that the layer's ink falls
// on, with the layer laid over it, as the screen without WebGPU draws it:
// at each pixel drawn, the frame sampled between its four nearest pixels
// and the pixel of the layer that falls there taken whole, the way the
// shader samples them, see screen.ts. The frame is 10 bits a colour with
// red lowest, see framewire.Picture, and so is the part. Null where the ink
// falls on nothing drawn.
export function laidPart(
  data: Uint8Array,
  fw: number,
  fh: number,
  w: number,
  h: number,
  o: Over,
): { x: number; y: number; w: number; h: number; data: Uint8Array } | null {
  const l = o.layer;
  if (!l.w || !l.h || !fw || !fh || !w || !h) return null;
  const frame = new Uint32Array(data.buffer, data.byteOffset, data.byteLength / 4);
  const [u0, v0, u1, v1] = inkBox(o);
  // The part drawn that the box falls on, turned back.
  const corners = [
    [u0, v0],
    [u1, v1],
  ].map(([u, v]) => unturned(u, v, o.turn));
  const xs = corners.map((c) => c[0] * w);
  const ys = corners.map((c) => c[1] * h);
  const x0 = Math.max(0, Math.floor(Math.min(...xs)));
  const x1 = Math.min(w, Math.ceil(Math.max(...xs)));
  const y0 = Math.max(0, Math.floor(Math.min(...ys)));
  const y1 = Math.min(h, Math.ceil(Math.max(...ys)));
  if (x1 <= x0 || y1 <= y0) return null;
  const pw = x1 - x0;
  const out = new Uint32Array(pw * (y1 - y0));
  const colour = (x: number, y: number, c: number) =>
    (frame[Math.min(fh - 1, Math.max(0, y)) * fw + Math.min(fw - 1, Math.max(0, x))] >>> (10 * c)) & 1023;
  for (let y = y0; y < y1; y++) {
    for (let x = x0; x < x1; x++) {
      const u = (x + 0.5) / w;
      const v = (y + 0.5) / h;
      // The frame between its four nearest pixels.
      const sx = u * fw - 0.5;
      const sy = v * fh - 0.5;
      const ix = Math.floor(sx);
      const iy = Math.floor(sy);
      const fx = sx - ix;
      const fy = sy - iy;
      const [du, dv] = turned(u, v, o.turn);
      const pu = (du - u0) / (u1 - u0);
      const pv = (dv - v0) / (v1 - v0);
      const inside = pu >= 0 && pu < 1 && pv >= 0 && pv < 1;
      const at = inside ? (Math.floor(pv * l.h) * l.w + Math.floor(pu * l.w)) * 4 : -1;
      const keep = inside ? (255 - l.pixels[at + 3]) / 255 : 1;
      let word = 0;
      for (let c = 0; c < 3; c++) {
        const f =
          (colour(ix, iy, c) * (1 - fx) + colour(ix + 1, iy, c) * fx) * (1 - fy) +
          (colour(ix, iy + 1, c) * (1 - fx) + colour(ix + 1, iy + 1, c) * fx) * fy;
        const laid = inside ? (l.pixels[at + c] * 1023) / 255 : 0;
        word |= Math.min(1023, Math.round(laid + f * keep)) << (10 * c);
      }
      out[(y - y0) * pw + x - x0] = word >>> 0;
    }
  }
  return { x: x0, y: y0, w: pw, h: y1 - y0, data: new Uint8Array(out.buffer) };
}

// Where libass put a word of the captions: the box its pill takes at
// rest, as shares of the short's width and height from its top left
// corner, and the radius of its corners as a share of the height. And
// when the picture lights it, from and to on the episode's clock, from the
// start of the first frame libass draws it lit on to the end of the last,
// -1 for a word lit on no frame. See engine.WordFrame and
// /frames/captions/words.
export type WordFrame = { x: number; y: number; w: number; h: number; r: number; from: number; to: number };

// The word frames of the captions of a track, each caption's words in
// their order, or null where the Go side cannot say, see WordFrame.
export async function wordFramesOf(path: string, track: string, signal: AbortSignal): Promise<WordFrame[][] | null> {
  const r = await fetch(`/frames/captions/words?${new URLSearchParams({ path, track })}`, { signal });
  return r.ok ? ((await r.json()) as WordFrame[][] | null) : null;
}

// What the video preview asks the captions of: the episode, the captions
// by their track, see engine.CaptionsView, and the drafts the hand is
// changing on the way, as /frames/captions takes them.
export type CaptionAsk = { path: string; track: string; drafts: Record<string, string> };

// How many layers are kept, and how many may be on their way at once. A
// layer is the crop's width by the captions' height, a few hundred
// kilobytes, and a frame on its way to the screen asks for its own a few
// frames ahead, see FrameQueue.over.
const KEPT = 48;
const GOING = 3;

// How long a layer that did not come waits before it is asked for again,
// once, in milliseconds. A layer that fails is not kept, so the next frame
// that needs it asks again as well.
const AGAIN = 250;

// A layer asked for: the ask it belongs to, the frame and the size.
type Wanted = { key: string; ask: CaptionAsk; frame: number; size: [number, number] };

// The pop played over a paused frame. The short pops the word being
// spoken in the frames after it lights, and a frame paused on shows one
// moment of that. So when the video preview comes to stand on a frame
// that lights another word than the frame before, by a step, Shift and an
// arrow or a click, it plays the layers of the frames of the short round
// it over the still picture, the ones the Go side says, see
// engine.PopFrames, one each frame of the short, and comes back to the
// frame's own layer, which looks as the last of them leaves it. They start
// at the word's first frame, behind the frame paused on: a step lands a
// frame into a word, already part of the way up, and a pop played from
// there left its start out and was half as large. The word
// pops the way it pops in the short and the frame paused on stays the
// short's. Before #201 the browser played a pop of its own on every new
// word, and Tim missed it once the video preview drew libass's captions.
type Replay = { key: string; layers: (Layer | null)[]; shown: number; timer?: ReturnType<typeof setTimeout> };

// Whether the person asked the system for less motion, which the pop over
// a paused frame then leaves out, as the browser's pop did.
function calm(): boolean {
  return typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
}

// What the layers need to know of the video preview to play a pop: whether
// it is paused, and how long a frame of the short lasts, in seconds.
export type Still = { paused(): boolean; frameLength(): number };

// The layers of the captions of the video preview, asked for each frame
// of the episode by the moment it starts, at a size. A frame on its way to
// the screen asks ahead, coming, so its layer is there when it is drawn.
// came is told when a layer that was not there for a frame drawn comes.
export class CaptionLayers {
  private ask: CaptionAsk | null = null;
  private askKey = "";
  private kept = new Map<string, Layer>();
  private going = new Set<string>();
  // The layers that did not come and were asked for again.
  private again = new Set<string>();
  // The frame drawn whose layer is not there yet, asked for once one on
  // its way has come. It carries its own ask, so a layer asked for after
  // the ask changed is never asked for with the new one.
  private waiting: Wanted | null = null;
  // The layer given last, which stays while the one for a newer frame or a
  // newer draft is on its way, so the captions never blink off in between.
  // Another clip's captions never stay.
  private last: Layer | null = null;
  // For the walks: how many layers were asked for and how many a frame
  // was drawn with that was not its own yet.
  readonly stats = { asked: 0, late: 0, popped: 0 };
  // The pop being played over the frame paused on, and what the layer
  // given last showed, by its track, which says when the word changed.
  private replay: Replay | null = null;
  private seen = "";

  constructor(
    private came: () => void,
    private still?: Still,
  ) {}

  // Whether a pop is being played over a paused frame, for the walks.
  get popping(): boolean {
    return this.replay !== null;
  }

  // Whether a layer is on its way, for the walks.
  get busy(): boolean {
    return this.going.size > 0 || this.waiting !== null;
  }

  // Whether the frame on screen was drawn with a layer not its own, the
  // last one given, while its own is on its way, for the walks.
  get late(): boolean {
    return this.waiting !== null;
  }

  set(ask: CaptionAsk | null) {
    const key = ask ? `${ask.path}\n${ask.track}\n${new URLSearchParams(ask.drafts)}` : "";
    if (!ask || ask.path !== this.ask?.path || ask.track !== this.ask?.track) this.last = null;
    if (key !== this.askKey) {
      this.waiting = null;
      this.again.clear();
    }
    this.ask = ask;
    this.askKey = key;
  }

  private wanted(frame: number, size: [number, number]): Wanted | null {
    const ask = this.ask;
    if (!ask) return null;
    return { key: `${this.askKey}\n${frame.toFixed(6)}\n${size[0]}x${size[1]}`, ask, frame, size };
  }

  // The layer of the frame that starts at frame, or while it is on its way
  // the one given last. Null with no captions to draw.
  at(frame: number, size: [number, number]): Layer | null {
    const want = this.wanted(frame, size);
    if (want?.key !== this.replay?.key) this.stop();
    if (!want) return null;
    const have = this.kept.get(want.key);
    if (have) {
      this.last = have;
      const seen = have.showing ? `${have.track}\n${have.showing.caption} ${have.showing.word}` : "";
      const changed = seen !== this.seen;
      this.seen = seen;
      const pop = have.pop;
      if (changed && have.showing && have.showing.word >= 0 && pop && pop.behind + pop.ahead > 0 && this.still?.paused() && !calm())
        this.play(want, have, pop);
      return this.replay?.layers[this.replay.shown] ?? have;
    }
    this.stats.late++;
    this.waiting = want;
    this.fetch(want);
    return this.last;
  }

  // The pop over the frame of want, whose own layer is have: its layers
  // asked for all at once, then shown one a frame, ending on the frame's
  // own.
  private play(want: Wanted, have: Layer, pop: Pop) {
    const { key, ask, frame, size } = want;
    const replay: Replay = { key, layers: [], shown: -1 };
    this.replay = replay;
    this.stats.popped++;
    const frames = Array.from({ length: pop.behind + pop.ahead + 1 }, (_, n) => n - pop.behind);
    const asked = frames.map((n) =>
      n === 0
        ? Promise.resolve(have)
        : fetch(
            `/frames/captions?${new URLSearchParams({
              path: ask.path,
              track: ask.track,
              at: frame.toFixed(6),
              w: String(size[0]),
              h: String(size[1]),
              ahead: String(n),
              ...ask.drafts,
            })}`,
          )
            .then(async (r) => (r.ok ? readLayer(await r.arrayBuffer()) : null))
            .catch(() => null),
    );
    void Promise.all(asked).then((layers) => {
      if (this.replay !== replay) return;
      replay.layers = layers.map((l) => l && { ...l, track: ask.track, showing: this.last?.showing });
      const step = Math.max(1, (this.still?.frameLength() ?? 1 / 30) * 1000);
      const next = () => {
        if (this.replay !== replay) return;
        replay.shown++;
        if (replay.shown >= replay.layers.length) this.replay = null;
        else replay.timer = setTimeout(next, step);
        this.came();
      };
      next();
    });
  }

  // The pop over a paused frame stopped, by a move to another frame, a
  // play, or another ask.
  private stop() {
    if (!this.replay) return;
    clearTimeout(this.replay.timer);
    this.replay = null;
  }

  // A frame that starts at frame is on its way to the screen.
  coming(frame: number, size: [number, number]) {
    const want = this.wanted(frame, size);
    if (want && !this.kept.has(want.key)) this.fetch(want);
  }

  private fetch(want: Wanted) {
    const { key, ask, frame, size } = want;
    if (this.going.has(key) || this.going.size >= GOING) return;
    this.going.add(key);
    this.stats.asked++;
    const q = new URLSearchParams({
      path: ask.path,
      track: ask.track,
      at: frame.toFixed(6),
      w: String(size[0]),
      h: String(size[1]),
      ...ask.drafts,
    });
    void fetch(`/frames/captions?${q}`)
      .then(async (r) => {
        const layer = r.ok ? readLayer(await r.arrayBuffer()) : null;
        if (layer) {
          layer.track = ask.track;
          layer.showing = readShowing(r.headers.get("X-Caption-Showing"));
          layer.pop = readPop(r.headers.get("X-Caption-Pop"));
        }
        return layer;
      })
      .catch(() => null)
      .then((layer) => {
        this.going.delete(key);
        const w = this.waiting;
        if (layer) {
          this.again.delete(key);
          this.kept.set(key, layer);
          while (this.kept.size > KEPT) this.kept.delete(this.kept.keys().next().value!);
        } else if (w?.key === key) {
          // Nothing is kept for a layer that did not come: the frame drawn
          // keeps the layer given last, and its own is asked for once more
          // in a moment.
          if (!this.again.has(key)) {
            this.again.add(key);
            setTimeout(() => {
              if (this.waiting?.key === key) this.fetch(this.waiting);
            }, AGAIN);
          } else this.waiting = null;
          return;
        }
        // The frame drawn without its own layer is drawn again once it
        // has come, and asked for once there is room.
        if (!w) return;
        if (this.kept.has(w.key)) {
          this.waiting = null;
          this.came();
        } else this.fetch(w);
      });
  }
}
