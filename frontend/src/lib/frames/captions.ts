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
// track, and what it shows of them, showing, and how large the pill of
// its word is drawn, scale, 1 at rest, which the frame round the word
// grows by.
// It also says the size it was drawn at, size, which its pixels are laid
// by: while the app is resized, the layer given last stands over a frame
// of the new size until that size's own layer comes, and divided by the
// new size it was laid tens of pixels toward the crop's top left.
// After a word step a layer carries the word apart, lift, which the screen
// lays over the rest scaled by scale round the middle of its box, see
// Lift.
export type Layer = {
  x: number;
  y: number;
  w: number;
  h: number;
  pixels: Uint8Array;
  track?: string;
  showing?: Showing;
  scale?: number;
  size?: [number, number];
  lift?: { layer: Layer; scale: number };
};

// How the video preview bounces a word after a word step: so many
// milliseconds from rest to rest, rising for the share rise of them to
// peak, as the Go side says it in the header X-Caption-Bounce,
// "<ms> <rise> <peak>", see engine.BounceCurve. The pop the short burns in
// has the same shape, laid out on its frames.
export type Bounce = { length: number; rise: number; peak: number };

export function readBounce(header: string | null): Bounce | undefined {
  const [length, rise, peak] = (header ?? "").split(" ").map(Number);
  return length > 0 && rise > 0 && rise < 1 && peak >= 1 && peak < 2 ? { length, rise, peak } : undefined;
}

// The scale of the word t milliseconds into its bounce: at rest before
// and after, eased up like a sine to peak and back down like a cosine,
// the curve of engine.popScales, here at any moment rather than on frames.
export function bounceScale(t: number, b: Bounce): number {
  if (!(t > 0) || t >= b.length) return 1;
  const top = b.rise * b.length;
  return t < top
    ? 1 + (b.peak - 1) * Math.sin(((Math.PI / 2) * t) / top)
    : 1 + (b.peak - 1) * Math.cos(((Math.PI / 2) * (t - top)) / (b.length - top));
}

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

// How large the pill is drawn, from the header X-Caption-Scale, at rest
// when it is missing or makes no sense.
export function readScale(header: string | null): number {
  const s = Number(header ?? "1");
  return Number.isFinite(s) && s > 0 && s < 2 ? s : 1;
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

// The box of the layer's ink in the picture as shown, in shares, by the
// size the layer was drawn at.
export function inkBox(o: Over): [number, number, number, number] {
  const [W, H] = o.layer.size ?? o.size;
  const l = o.layer;
  return [
    o.left + (o.width * l.x) / W,
    l.y / H,
    o.left + (o.width * (l.x + l.w)) / W,
    (l.y + l.h) / H,
  ];
}

// The box in the picture as shown, in shares, that the word lifted out of
// a layer covers at its scale: its own box grown round its middle, with a
// pixel of the word more on each side, which sampling between its pixels
// reaches. Null for no word lifted.
export function liftBox(o: Over): [number, number, number, number] | null {
  const lift = o.layer.lift;
  if (!lift || !lift.layer.w || !lift.layer.h) return null;
  const [u0, v0, u1, v1] = inkBox({ ...o, layer: lift.layer });
  const [mu, mv] = [(u0 + u1) / 2, (v0 + v1) / 2];
  const [pu, pv] = [(u1 - u0) / lift.layer.w, (v1 - v0) / lift.layer.h];
  const s = lift.scale;
  return [mu - (mu - u0 + pu) * s, mv - (mv - v0 + pv) * s, mu + (u1 - mu + pu) * s, mv + (v1 - mv + pv) * s];
}

// The part of a frame drawn at width by height that the layer's ink falls
// on, with the layer laid over it, as the screen without WebGPU draws it:
// at each pixel drawn, the frame sampled between its four nearest pixels
// and the pixel of the layer that falls there taken whole, the way the
// shader samples them, see screen.ts. A word lifted out of the layer is
// laid over that, scaled round the middle of its box and sampled between
// its four nearest pixels, nothing beyond its edge, as the shader samples
// it. The frame is 10 bits a colour with red lowest, see
// framewire.Picture, and so is the part. Null where the ink falls on
// nothing drawn.
export function laidPart(
  data: Uint8Array,
  fw: number,
  fh: number,
  w: number,
  h: number,
  o: Over,
): { x: number; y: number; w: number; h: number; data: Uint8Array } | null {
  const l = o.layer;
  const lifted = liftBox(o);
  if (!fw || !fh || !w || !h || (!(l.w && l.h) && !lifted)) return null;
  const frame = new Uint32Array(data.buffer, data.byteOffset, data.byteLength / 4);
  const [u0, v0, u1, v1] = inkBox(o);
  // The part drawn that the boxes fall on, turned back.
  const corners = [
    ...(l.w && l.h
      ? [
          [u0, v0],
          [u1, v1],
        ]
      : []),
    ...(lifted
      ? [
          [lifted[0], lifted[1]],
          [lifted[2], lifted[3]],
        ]
      : []),
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
  // The word lifted out: its box at rest, its middle and its scale.
  const lift = l.lift && lifted ? l.lift : null;
  const [lu0, lv0, lu1, lv1] = lift ? inkBox({ ...o, layer: lift.layer }) : [0, 0, 1, 1];
  const [mu, mv] = [(lu0 + lu1) / 2, (lv0 + lv1) / 2];
  // A pixel of the word, nothing beyond its edge.
  const texel = (x: number, y: number, c: number) =>
    lift && x >= 0 && y >= 0 && x < lift.layer.w && y < lift.layer.h ? lift.layer.pixels[(y * lift.layer.w + x) * 4 + c] : 0;
  const laid = [0, 0, 0, 0];
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
      const inside = l.w > 0 && l.h > 0 && pu >= 0 && pu < 1 && pv >= 0 && pv < 1;
      const at = inside ? (Math.floor(pv * l.h) * l.w + Math.floor(pu * l.w)) * 4 : -1;
      const keep = inside ? (255 - l.pixels[at + 3]) / 255 : 1;
      // The word lifted out, between its four nearest pixels.
      if (lift) {
        const qx = ((mu + (du - mu) / lift.scale - lu0) / (lu1 - lu0)) * lift.layer.w - 0.5;
        const qy = ((mv + (dv - mv) / lift.scale - lv0) / (lv1 - lv0)) * lift.layer.h - 0.5;
        const jx = Math.floor(qx);
        const jy = Math.floor(qy);
        const gx = qx - jx;
        const gy = qy - jy;
        for (let c = 0; c < 4; c++)
          laid[c] =
            (texel(jx, jy, c) * (1 - gx) + texel(jx + 1, jy, c) * gx) * (1 - gy) +
            (texel(jx, jy + 1, c) * (1 - gx) + texel(jx + 1, jy + 1, c) * gx) * gy;
      }
      let word = 0;
      for (let c = 0; c < 3; c++) {
        const f =
          (colour(ix, iy, c) * (1 - fx) + colour(ix + 1, iy, c) * fx) * (1 - fy) +
          (colour(ix, iy + 1, c) * (1 - fx) + colour(ix + 1, iy + 1, c) * fx) * fy;
        let under = (inside ? (l.pixels[at + c] * 1023) / 255 : 0) + f * keep;
        if (lift) under = (laid[c] * 1023) / 255 + (under * (255 - laid[3])) / 255;
        word |= Math.min(1023, Math.round(under)) << (10 * c);
      }
      out[(y - y0) * pw + x - x0] = word >>> 0;
    }
  }
  return { x: x0, y: y0, w: pw, h: y1 - y0, data: new Uint8Array(out.buffer) };
}

// Where libass put a word of the captions: the box its pill takes at
// rest, as shares of the short's width and height from its top left
// corner, the radius of its corners as a share of the height, and the
// baseline of its line as a share of the height from the top. And
// when the picture lights it, from and to on the episode's clock, from the
// start of the first frame libass draws it lit on to the end of the last,
// -1 for a word lit on no frame. See engine.WordFrame and
// /frames/captions/words.
export type WordFrame = {
  x: number;
  y: number;
  w: number;
  h: number;
  r: number;
  base: number;
  from: number;
  to: number;
};

// The word frames of the captions of a track, each caption's words in
// their order, or null where the Go side cannot say, see WordFrame.
export async function wordFramesOf(
  path: string,
  track: string,
  signal: AbortSignal,
  tell?: Tell,
): Promise<WordFrame[][] | null> {
  const r = await fetch(`/frames/captions/words?${new URLSearchParams({ path, track })}`, { signal });
  if (r.ok) return (await r.json()) as WordFrame[][] | null;
  tell?.(`captions: where the words stand did not come, ${await failure(r)}`, path);
  return null;
}

// Why an answer brought no layer: its status and what the Go side said,
// or that it could not be read.
async function failure(r: Response): Promise<string> {
  if (r.ok) return "an answer that could not be read";
  const text = await r.text().catch(() => "");
  return `${r.status}${text.trim() ? ` ${text.trim().slice(0, 120)}` : ""}`;
}

// What the video preview asks the captions of: the episode, the captions
// by their track, see engine.CaptionsView, and the drafts the hand is
// changing on the way, as /frames/captions takes them.
// Typing is the word being typed in the caption box, "<caption> <word>",
// which libass leaves out, see engine.CaptionDrawing.Typing.
// The captions a frame is drawn with. clip names the clip they belong
// to: the track changes with every saved edit and every step of a drag,
// and the layer given last stays up over those, never over another clip.
export type CaptionAsk = { path: string; clip: string; track: string; drafts: Record<string, string>; typing?: string };

// How many layers are kept, and how many may be on their way at once. A
// layer is the crop's width by the captions' height, a few hundred
// kilobytes, and a frame on its way to the screen asks for its own a few
// frames ahead, see FrameQueue.over.
const KEPT = 48;
const GOING = 3;
// How many frames that asked ahead may wait for room among the GOING, the
// frames a decoder holds and those of the next piece's decoder with room
// to spare, see DEPTH in queue.ts.
const AHEAD = 12;

// How long a layer that did not come waits before it is asked for again,
// once, in milliseconds. A layer that fails is not kept, so the next frame
// that needs it asks again as well.
const AGAIN = 250;

// A layer asked for: the ask it belongs to, the frame and the size.
type Wanted = { key: string; ask: CaptionAsk; frame: number; size: [number, number] };

// What tells one ask from another, and one layer of an ask from another.
function keyOf(ask: CaptionAsk | null): string {
  return ask ? `${ask.path}\n${ask.track}\n${ask.typing ?? ""}\n${new URLSearchParams(ask.drafts)}` : "";
}

function layerKey(askKey: string, frame: number, size: [number, number]): string {
  return `${askKey}\n${frame.toFixed(6)}\n${size[0]}x${size[1]}`;
}

// The bounce after a word step. Shift and an arrow step from word to word,
// and the word stepped to bounces, from rest through its peak and back to
// rest, every word alike, a word the short shows at its peak on its one
// frame too: Tim wanted the bounce the same for every word, without
// exception, even where that is not the frame the short shows. Every
// other way of coming to stand on a frame, an arrow alone, a click, a
// drag, shows the frame as the short shows it and nothing over it. Tim
// saw the pop the short burns in played again over every frame he
// stepped to, two animations mixed into one, and wanted a step from frame
// to frame to show only the short's frames and a step from word to word to
// bounce its word.
//
// The two are two paths that share nothing but the shape of the curve.
// The pop the short burns in is libass's, on the short's frames, see
// engine.popScales, and the video preview shows it as it shows any frame.
// The bounce after a word step is the app's own: libass draws the word
// with its pill apart from the rest of the caption, both once and at
// rest, see engine.CaptionDrawing.Lifted, and the screen lays the rest
// over the frame and the word over that, scaled round its middle, at the
// screen's own rate, see screen.ts. That is how the Mac animates a layer,
// drawn once and moved by the GPU. The bounce was the short's own frames
// of the pop before, drawn on the Go side one by one and shown one each
// frame of the short, 25 a second on Tim's episode and stepped by a
// timer, and Tim found it laggy.
//
// A lift is the word lifted out of the frame at key, rest and word once
// they have come. Bouncing is whether it bounces once they are there,
// start when it began, by the clock of the screen's frames, and scale
// where it is now. Done once it is back at rest, where it then stays,
// over the frame, until the playhead moves or the video preview plays.
// Before is the layer given before the step, shown while the parts are on
// their way: the frame's own layer shows a word of one frame at its peak,
// and the bounce starts at rest.
type Lift = {
  key: string;
  frame: number;
  size: [number, number];
  rest: Layer | null;
  word: Layer | null;
  bounce?: Bounce;
  before: Layer | null;
  bouncing: boolean;
  start: number;
  scale: number;
  done: boolean;
  tick: number;
  // Calls off the two parts once the word is put back.
  off: AbortController;
};

// Whether the person asked the system for less motion, which the bounce
// over a paused frame then leaves out, as the browser's pop did.
function calm(): boolean {
  return typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches;
}

// What the layers need to know of the video preview to bounce a word:
// whether it is paused.
export type Still = { paused(): boolean };

// How the layers say in the app's log that one was given up on, see
// lib/said.ts, which the tests leave out.
export type Tell = (text: string, video: string) => void;

// The layers of the captions of the video preview, asked for each frame
// of the episode by the moment it starts, at a size. A frame on its way to
// the screen asks ahead, coming, so its layer is there when it is drawn.
// came is told when a layer that was not there for a frame drawn comes,
// and on every frame of the screen while a word bounces.
export class CaptionLayers {
  private ask: CaptionAsk | null = null;
  private askKey = "";
  private kept = new Map<string, Layer>();
  // The layers on their way, each with what calls it off once it is no
  // longer wanted: a draft or a track that changed, a seek. Every ask
  // that is not called off holds one of the GOING slots, and every draft
  // is a caption file on the Go side, found by the second review.
  private going = new Map<string, AbortController>();
  // The layers that did not come and were asked for again.
  private again = new Set<string>();
  // The frame drawn whose layer is not there yet, asked for once one on
  // its way has come. It carries its own ask, so a layer asked for after
  // the ask changed is never asked for with the new one.
  private waiting: Wanted | null = null;
  // The frames on their way to the screen of a playing clip that asked
  // ahead while GOING layers were on their way, asked for in turn as room
  // comes, after the frame drawn waiting for its own. They were dropped
  // before, so on a busy machine a frame was drawn with the layer of the
  // frame before and only then asked for its own, issue #288.
  private ahead: Wanted[] = [];
  // The layer given last, which stays while the one for a newer frame, a
  // newer draft or a newer track of the same clip is on its way, so the
  // captions never blink off in between. Another clip's captions never stay.
  private last: Layer | null = null;
  // For the walks: how many layers were asked for, how many a frame was
  // drawn with that was not its own yet, how many words bounced, and how
  // many times the screen drew a bounce.
  readonly stats = { asked: 0, late: 0, popped: 0, bounced: 0 };
  // The word lifted out of the frame paused on, and the frame on screen
  // when a word step asked for a bounce, which the next frame other than it
  // bounces on. Null when the last step was no word step.
  private lift: Lift | null = null;
  private shownAt = NaN;
  private bounceFrom: number | null = null;
  // The frames a gesture put the playhead on that wait to be put up until
  // their own layer is here, by the key of the layer, each with what tells
  // it the layer came, or will not, see ready.
  private promised = new Map<string, { want: Wanted; done: (() => void)[] }>();
  // The layers of the first frames of the clips on the cards, each with
  // captions of its own, still to be asked for, one at a time while
  // nothing else is on its way, and the keys of all of them, which are
  // kept whatever else comes, see next.
  private soon: Wanted[] = [];
  private cards = new Set<string>();

  constructor(
    private came: () => void,
    private still?: Still,
    private tell?: Tell,
  ) {}

  // What was said of layers given up on, so each failure is said once for
  // each video, not once a frame.
  private told = new Set<string>();

  // Says that the layer of what, or a part of it, was given up on and why:
  // the interface turned every failed answer into no layer and said
  // nothing, so a frame without its captions left no trace, found by the
  // second review.
  private gaveUp(ask: CaptionAsk, what: string, why: string) {
    const key = `${ask.path}\n${what}\n${why}`;
    if (!this.tell || this.told.has(key)) return;
    if (this.told.size > 64) this.told.clear();
    this.told.add(key);
    this.tell(`captions: ${what} did not come, ${why}`, ask.path);
  }

  // Whether a word is bouncing, or about to once its parts are there, for
  // the walks.
  get popping(): boolean {
    return this.lift !== null && this.lift.bouncing && !this.lift.done;
  }

  // Whether a layer is on its way or a word bounces, for the walks: until
  // neither, the screen may not show what it shows next.
  get busy(): boolean {
    return (
      this.going.size > 0 ||
      this.promised.size > 0 ||
      this.ahead.length > 0 ||
      this.waiting !== null ||
      this.popping ||
      (this.lift !== null && !this.lift.word)
    );
  }

  // Whether the frame on screen was drawn with a layer not its own, the
  // last one given, while its own is on its way, for the walks.
  get late(): boolean {
    return this.waiting !== null;
  }

  set(ask: CaptionAsk | null) {
    const key = keyOf(ask);
    if (!ask || ask.path !== this.ask?.path || ask.clip !== this.ask?.clip) this.last = null;
    if (key !== this.askKey) {
      this.waiting = null;
      this.ahead = [];
      this.again.clear();
      // A layer of the new captions on its way, read for a card before it
      // was clicked, is the one the click wants, and goes on.
      this.callOff((k) => !k.startsWith(`${key}\n`));
      // A frame that waited for a layer of other captions waits no longer:
      // it is put up with what there is, and asks again as it is drawn.
      for (const k of [...this.promised.keys()]) this.keep(k);
    }
    this.ask = ask;
    this.askKey = key;
    this.settleRest();
  }

  // The frame on screen drawn again with what lies over it now.
  private drawAgain() {
    this.came();
    this.settleRest();
  }

  // What lies over the frame that starts at frame, asked for now and before
  // anything else on its way, for a frame a gesture put the playhead on:
  // what answers is done once the layer is here, or once it did not come,
  // so the frame and its captions are put up together. Null when the layer
  // is here already or there are no captions to draw. Tim saw a clip card
  // picked put its picture up first and its captions a few frames later,
  // issue #383.
  ready(frame: number, size: [number, number]): Promise<void> | null {
    const want = this.wanted(frame, size);
    if (!want || this.kept.has(want.key)) return null;
    let p = this.promised.get(want.key);
    if (!p) {
      p = { want, done: [] };
      this.promised.set(want.key, p);
    }
    const waits = p;
    const came = new Promise<void>((done) => waits.done.push(done));
    if (!this.going.has(want.key)) {
      // The frame a gesture is waiting for goes before frames read ahead.
      if (this.going.size >= GOING) {
        const spare = [...this.going.keys()].find((k) => !this.promised.has(k) && k !== this.waiting?.key);
        if (spare) this.callOff((k) => k === spare);
      }
      this.fetch(want);
    }
    return came;
  }

  // The frames waiting on the layer of key are told it came, or will not.
  private keep(key: string) {
    const p = this.promised.get(key);
    if (!p) return;
    this.promised.delete(key);
    for (const done of p.done) done();
  }

  private wanted(frame: number, size: [number, number]): Wanted | null {
    const ask = this.ask;
    if (!ask) return null;
    return { key: layerKey(this.askKey, frame, size), ask, frame, size };
  }

  // The layers of the first frames of the clips on the cards, each of its
  // own captions, the likeliest to be clicked first, see Player.ahead.
  // They are read while nothing else is on its way, and kept for as long
  // as their cards are there, so the frame of a card clicked finds its
  // captions here. Each call replaces the ones before.
  next(wants: { ask: CaptionAsk; frame: number; size: [number, number] }[]) {
    const all = wants.map(({ ask, frame, size }) => ({ key: layerKey(keyOf(ask), frame, size), ask, frame, size }));
    this.cards = new Set(all.map((w) => w.key));
    this.soon = all.filter((w) => !this.kept.has(w.key));
    this.more();
  }

  // The playhead was put somewhere, by a word step or by anything else.
  // The next frame other than the one on screen bounces its word after a
  // word step, and after anything else does not.
  sought(wordStep: boolean) {
    this.bounceFrom = wordStep ? this.shownAt : null;
    // Playing, the frames that were coming are ahead of where the playhead
    // was, not where it went. The frame on screen waiting for its own
    // stays asked for, until another is drawn in its place. Paused, what
    // is on its way is the frame next to it, which a step may still want.
    if (this.still?.paused() !== false) return;
    const waiting = this.waiting?.key;
    this.ahead = [];
    this.callOff((key) => key !== waiting);
    this.more();
  }

  // Calls off the layers on their way that which picks, so their slots go
  // to the ones wanted now.
  private callOff(which: (key: string) => boolean) {
    for (const [key, off] of this.going) {
      if (!which(key)) continue;
      off.abort();
      this.going.delete(key);
      this.keep(key);
    }
    this.settleRest();
  }

  // The layer of the frame that starts at frame, or while it is on its way
  // the one given last. Null with no captions to draw.
  at(frame: number, size: [number, number]): Layer | null {
    const want = this.wanted(frame, size);
    const lift = this.lift;
    // The word stays lifted only over the frame it was lifted from,
    // paused. New captions over that frame, a word corrected or the style
    // changed, lift it again at rest.
    if (lift && (!want || lift.frame !== frame || lift.size.join() !== size.join() || !this.still?.paused())) {
      this.drop();
      this.more();
    }
    if (!want) return null;
    // The frame now on screen, whether its own layer is there yet or not,
    // so a second word step before it came bounces the word it lands on,
    // not the one it passed over.
    this.shownAt = frame;
    // Playing, a frame that asked ahead and was drawn before its turn is
    // the frame drawn now or one passed.
    const playing = this.still?.paused() === false;
    this.ahead = this.ahead.filter((a) => a.key !== want.key && (!playing || a.frame > frame));
    const have = this.kept.get(want.key);
    if (!have) {
      this.stats.late++;
      this.waiting = want;
      this.fetch(want);
      return this.lift ? this.lifted(this.lift) : this.last;
    }
    const before = this.last;
    this.last = have;
    // The frame a word step came to.
    if (this.bounceFrom !== null && frame !== this.bounceFrom) {
      this.bounceFrom = null;
      if (have.showing && have.showing.word >= 0 && this.still?.paused())
        this.raise(want, have.scale === 1 ? have : before, !calm());
    } else if (this.lift && this.lift.key !== want.key) this.raise(want, this.lifted(this.lift), false);
    return this.lift ? (this.lifted(this.lift) ?? have) : have;
  }

  // What the screen lays over the frame of a lift: the rest with the word
  // over it at its scale, or before while they are on their way.
  private lifted(lift: Lift): Layer | null {
    if (!lift.rest || !lift.word) return lift.before;
    return { ...lift.rest, scale: lift.scale, lift: { layer: lift.word, scale: lift.scale } };
  }

  // The word of the frame of want lifted out of its caption, the two parts
  // asked for at once, and bounced once both are there, or held at rest.
  private raise(want: Wanted, before: Layer | null, bounce: boolean) {
    const { key, ask, frame, size } = want;
    const showing = this.kept.get(key)?.showing;
    this.drop();
    const lift: Lift = {
      key,
      frame,
      size,
      rest: null,
      word: null,
      before,
      bouncing: bounce,
      start: NaN,
      scale: 1,
      done: !bounce,
      tick: 0,
      off: new AbortController(),
    };
    this.lift = lift;
    if (bounce) this.stats.popped++;
    const part = (part: "word" | "rest") =>
      fetch(
        `/frames/captions?${new URLSearchParams({
          path: ask.path,
          track: ask.track,
          at: frame.toFixed(6),
          w: String(size[0]),
          h: String(size[1]),
          part,
          ...(ask.typing ? { typing: ask.typing } : {}),
          ...ask.drafts,
        })}`,
        { signal: lift.off.signal },
      )
        .then(async (r) => {
          const layer = r.ok ? readLayer(await r.arrayBuffer()) : null;
          if (layer) {
            layer.track = ask.track;
            layer.size = size;
            layer.showing = showing;
            if (part === "word") lift.bounce = readBounce(r.headers.get("X-Caption-Bounce"));
          } else why = await failure(r);
          return layer;
        })
        .catch((e) => {
          why = String(e);
          return null;
        });
    let why = "";
    void Promise.all([part("rest"), part("word")]).then(([rest, word]) => {
      if (this.lift !== lift) return;
      if (!rest || !word) {
        // Without its parts the frame shows its own layer.
        this.gaveUp(ask, "the word lifted out", why);
        this.lift = null;
        this.drawAgain();
        this.more();
        return;
      }
      lift.rest = rest;
      lift.word = word;
      if (!lift.bouncing || !lift.bounce || typeof requestAnimationFrame !== "function") {
        lift.done = true;
        this.drawAgain();
        this.more();
        return;
      }
      // The bounce, one scale for each frame of the screen, by the clock
      // of that frame, drawn in the same frame it is worked out for.
      const step = (now: number) => {
        if (this.lift !== lift) return;
        if (Number.isNaN(lift.start)) lift.start = now;
        lift.scale = bounceScale(now - lift.start, lift.bounce!);
        lift.done = now - lift.start >= lift.bounce!.length;
        if (lift.done) lift.scale = 1;
        else lift.tick = requestAnimationFrame(step);
        this.stats.bounced++;
        this.drawAgain();
        if (lift.done) this.more();
      };
      lift.tick = requestAnimationFrame(step);
    });
  }

  // The word put back, by a move to another frame, a play, or another ask.
  private drop() {
    if (!this.lift) return;
    if (typeof cancelAnimationFrame === "function") cancelAnimationFrame(this.lift.tick);
    this.lift.off.abort();
    this.lift = null;
  }

  // A frame that starts at frame is on its way to the screen.
  coming(frame: number, size: [number, number]) {
    const want = this.wanted(frame, size);
    if (!want || this.kept.has(want.key) || this.going.has(want.key)) return;
    if (this.going.size < GOING) {
      this.fetch(want);
      return;
    }
    // Paused, the frames read ahead are the ones a step may come to, and
    // they are asked for by the step, as before, while a word bounces.
    if (this.still?.paused() !== false || this.ahead.some((a) => a.key === want.key)) return;
    this.ahead.push(want);
    if (this.ahead.length > AHEAD) this.ahead.shift();
  }

  // Asks for what waits for room: the frame drawn without its own layer
  // first, unless its layer did not come and is asked for again in a
  // moment, then the frames that asked ahead, in the order they asked.
  private more(drawn = true) {
    this.moreNow(drawn);
    this.settleRest();
  }

  // Settles once no layer is on its way and every card's layer is here or
  // was given up on, for the walks and the tests, which wait on it rather
  // than on the clock.
  rested(): Promise<void> {
    return new Promise((done) => {
      this.restWaiters.push(done);
      this.settleRest();
    });
  }
  private restWaiters: (() => void)[] = [];

  private settleRest() {
    if (this.busy || this.soon.some((a) => !this.kept.has(a.key))) return;
    const was = this.restWaiters;
    this.restWaiters = [];
    for (const done of was) done();
  }

  private moreNow(drawn: boolean) {
    for (const p of this.promised.values()) if (!this.again.has(p.want.key)) this.fetch(p.want);
    const w = this.waiting;
    if (drawn && w && !this.kept.has(w.key)) this.fetch(w);
    while (this.going.size < GOING && this.ahead.length > 0) {
      const a = this.ahead.shift()!;
      if (!this.kept.has(a.key)) this.fetch(a);
    }
    // The layers of the clips on the cards, one at a time and only while
    // the video preview rests with nothing else to wait for: each is
    // another caption file for libass, and the frame on screen comes first,
    // and so does a word lifted out of it and its bounce. Read beside the
    // bounce, they took the screen's frames from it on a busy machine,
    // and the walk saw the word pop over three of them and not grow.
    const lifting = this.lift !== null && !this.lift.done;
    if (this.going.size || this.waiting || this.promised.size || lifting || this.still?.paused() === false) return;
    for (let a = this.soon.shift(); a; a = this.soon.shift()) {
      if (this.kept.has(a.key)) continue;
      this.fetch(a);
      return;
    }
  }

  private fetch(want: Wanted) {
    const { key, ask, frame, size } = want;
    if (this.going.has(key) || this.going.size >= GOING) return;
    const off = new AbortController();
    this.going.set(key, off);
    this.stats.asked++;
    const q = new URLSearchParams({
      path: ask.path,
      track: ask.track,
      at: frame.toFixed(6),
      w: String(size[0]),
      h: String(size[1]),
      ...(ask.typing ? { typing: ask.typing } : {}),
      ...ask.drafts,
    });
    let why = "";
    void fetch(`/frames/captions?${q}`, { signal: off.signal })
      .then(async (r) => {
        const layer = r.ok ? readLayer(await r.arrayBuffer()) : null;
        if (layer) {
          layer.track = ask.track;
          layer.size = size;
          layer.showing = readShowing(r.headers.get("X-Caption-Showing"));
          layer.scale = readScale(r.headers.get("X-Caption-Scale"));
        } else why = await failure(r);
        return layer;
      })
      .catch((e) => {
        why = String(e);
        return null;
      })
      .then((layer) => {
        // Called off: its slot is free already, and nothing waits for it.
        if (off.signal.aborted) return;
        this.going.delete(key);
        const w = this.waiting;
        if (layer) {
          this.again.delete(key);
          this.kept.set(key, layer);
          // The cards' layers are kept apart from the count, see next.
          let over = this.kept.size - KEPT;
          for (const k of this.kept.keys()) if (this.cards.has(k)) over--;
          for (const k of [...this.kept.keys()]) {
            if (over <= 0) break;
            if (this.cards.has(k)) continue;
            this.kept.delete(k);
            over--;
          }
          this.keep(key);
        } else if (w?.key === key || this.promised.has(key)) {
          // Nothing is kept for a layer that did not come: the frame drawn
          // keeps the layer given last, and its own is asked for once more
          // in a moment. A frame waiting to be put up waits for that once
          // more too, and is put up without it after.
          if (!this.again.has(key)) {
            this.again.add(key);
            setTimeout(() => {
              if (this.waiting?.key === key) this.fetch(this.waiting);
              else {
                const p = this.promised.get(key);
                if (p) this.fetch(p.want);
              }
            }, AGAIN);
          } else {
            if (w?.key === key) this.waiting = null;
            this.keep(key);
            this.gaveUp(ask, "a layer", why);
          }
          this.more(false);
          return;
        }
        // The frame drawn without its own layer is drawn again once it
        // has come, and asked for once there is room, before the frames
        // that asked ahead.
        if (w && this.kept.has(w.key)) {
          this.waiting = null;
          this.drawAgain();
        }
        this.more();
      });
  }
}
