import type { CropMark } from "../api";

// What the video preview draws over a frame, its dress: the crop, the turn
// and the keyframe on it, and the captions. None of it is worked out here:
// the Go side dresses each frame the way the render crops and turns it,
// see engine.FrameDress, and libass draws
// the captions on the Go side, in the episode's decoder, from the caption file the
// render burns into the short, at the moment libass reads on that frame of
// the short, see dressFrame in cmd/framefairy-app/dress.go and
// docs/CAPTIONS.md. Both come with the frame, in the same answer, so a
// frame is never drawn with the crop of one moment and the captions of
// another, and never waits for either. What comes back is a layer, which the screen lays over
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
// The Go side names a layer by tag, and a frame drawn the same as the
// layer the window has is sent with its tag alone, see dressedOf.
// Every layer carries the dress of its frame, dress. A frame of a clip
// with no captions to draw has a layer of nothing, which carries its
// dress alone.
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
  tag?: string;
  dress?: FrameDress;
};

// The dress of a frame, as the Go side sends it with the frame, see
// sentDress in cmd/framefairy-app/dress.go and engine.FrameDress: the
// piece that holds the frame, or the one nearest to it, whether one holds
// it, whether the frame is in the clip at all, how far it is turned, the
// picture turned and the crop's width and left edge on it, in pixels of
// the video, whether the crop was placed by hand, the keyframe on the
// frame and whether a drag may place the crop there. Key is the ask it
// was dressed for, see keyOf, and track the captions over it. The layer
// of captions over it is named by tag, drawn at size, shows the caption
// and the words lit in showing, its pill drawn at scale, and the word of
// a word step bounces by bounce. Turned is its dress once R turns the
// part that holds it, under the framing R turns the clip to, see
// turnedDress in cmd/framefairy-app/dress.go.
export type FrameDress = {
  piece: number;
  in: boolean;
  clip: boolean;
  turn: number;
  width: number;
  height: number;
  cropWidth: number;
  left: number;
  moved: boolean;
  mark: CropMark | null;
  movable: boolean;
  key?: string;
  track?: string;
  tag?: string;
  size?: [number, number];
  showing?: [number, number, number];
  scale?: number;
  bounce?: [number, number, number];
  turned?: FrameDress & { framing: string };
  // Whether Put the crop back would move the crop on this frame, for an
  // ask that names the framing to put back.
  back?: boolean;
};

// Whether two dresses draw the same crop, turn and keyframe, and offer the
// same in the crop frame's corner.
export function sameDress(a: FrameDress | null | undefined, b: FrameDress | null | undefined): boolean {
  if (a === b) return true;
  if (!a || !b) return false;
  return (
    a.piece === b.piece &&
    a.in === b.in &&
    a.clip === b.clip &&
    a.turn === b.turn &&
    a.width === b.width &&
    a.height === b.height &&
    a.cropWidth === b.cropWidth &&
    a.left === b.left &&
    a.moved === b.moved &&
    !!a.back === !!b.back &&
    a.movable === b.movable &&
    JSON.stringify(a.mark) === JSON.stringify(b.mark)
  );
}

// The layer over a frame from what the Go side sent with it: the dress
// as JSON and the layer's bytes, or none, where the frame is sent with the
// tag of a layer the window has, which had finds by its tag, or where
// nothing lies over it. Null for a frame sent bare, with no dress.
export function dressedOf(json: string, bytes: ArrayBuffer | null, had: (tag: string) => Layer | null): Layer | null {
  if (!json) return null;
  let d: FrameDress;
  try {
    d = JSON.parse(json) as FrameDress;
  } catch {
    return null;
  }
  if (typeof d !== "object" || d === null || !Number.isFinite(d.left) || !Number.isFinite(d.turn)) return null;
  const drawn = bytes ? readLayer(bytes) : d.tag ? had(d.tag) : null;
  const layer: Layer = drawn ? { ...drawn } : bare();
  layer.dress = d;
  layer.track = d.track;
  layer.tag = drawn ? d.tag : undefined;
  layer.size = d.size;
  layer.scale = d.scale && d.scale > 0 && d.scale < 2 ? d.scale : 1;
  const s = d.showing;
  layer.showing = s && s.every(Number.isInteger) ? { caption: s[0], word: s[1], last: s[2] } : undefined;
  return layer;
}

const bare = (): Layer => ({ x: 0, y: 0, w: 0, h: 0, pixels: new Uint8Array(0) });

// How the video preview bounces a word after a word step: so many
// milliseconds from rest to rest, rising for the share rise of them to
// peak, as the Go side says it in the header X-Caption-Bounce,
// "<ms> <rise> <peak>", see engine.BounceCurve. The pop the short burns in
// has the same shape, laid out on its frames.
export type Bounce = { length: number; rise: number; peak: number };

export function readBounce(sent: [number, number, number] | undefined): Bounce | undefined {
  if (!sent) return undefined;
  const [length, rise, peak] = sent;
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
// view, and the words of it lit, word to last, counted across its lines,
// each -1 for none, as the Go side says it with the frame's dress,
// see engine.CaptionShowing. It is one word but where words too short for
// a frame of their own are lit together, #280. The caption box over the video preview shows the
// caption and the word the layer on screen shows, so the frame round a word
// is never a frame ahead of the pill under it. It followed the playhead,
// which runs ahead of the frame on screen while the episode plays, and Tim
// saw the frame go to the next caption a few frames before the pill did.
export type Showing = { caption: number; word: number; last: number };

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
  const [a, b, c, d, e, f] = turning(turn);
  return [a * u + b * v + c, d * u + e * v + f];
}

// turned as the numbers of its sum, worked out once for a turn: the point
// as shown is a·u + b·v + c across and d·u + e·v + f down. laidPart turns
// every pixel it draws, and worked the quarter turns out again for each.
function turning(turn: number): [number, number, number, number, number, number] {
  switch (quarters(turn)) {
    case 1:
      return [0, -1, 1, 1, 0, 0];
    case 2:
      return [-1, 0, 1, 0, -1, 1];
    case 3:
      return [0, 1, 0, -1, 0, 1];
    default:
      return [1, 0, 0, 0, 1, 0];
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
  const [ta, tb, tc, td, te, tf] = turning(o.turn);
  for (let y = y0; y < y1; y++) {
    const v = (y + 0.5) / h;
    const sy = v * fh - 0.5;
    const iy = Math.floor(sy);
    const fy = sy - iy;
    for (let x = x0; x < x1; x++) {
      const u = (x + 0.5) / w;
      // The frame between its four nearest pixels.
      const sx = u * fw - 0.5;
      const ix = Math.floor(sx);
      const fx = sx - ix;
      const du = ta * u + tb * v + tc;
      const dv = td * u + te * v + tf;
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
// -1 for a word lit on no frame. For the first of words lit together,
// grow, the middle of their one pill, which it grows round in the pop,
// as shares of the short. See engine.WordFrame and /frames/captions/words.
export type WordFrame = {
  x: number;
  y: number;
  w: number;
  h: number;
  r: number;
  base: number;
  from: number;
  to: number;
  grow?: { x: number; y: number };
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
// changing on the way, as /frames/dress takes them.
// Typing is the word being typed in the caption box, "<caption> <word>",
// which libass leaves out, see engine.CaptionDrawing.Typing.
// What a frame is dressed with: the framing of the clip, see
// Framing.name, on the clip or, with on, on the video, which shows the
// episode as filmed, and the captions of the track, if there are any to
// draw. clip names the clip they belong to. The framing and the track
// change with every saved edit and every step of a drag. Every frame the
// Go side sends is dressed as the ask it is asked with says, see
// AppFrames.dressWith.
// size is the canvas's in device pixels, which the captions are drawn
// at, whatever size the frames are decoded at.
export type CaptionAsk = {
  path: string;
  clip: string;
  framing: string;
  on?: "video";
  track?: string;
  drafts: Record<string, string>;
  typing?: string;
  size?: [number, number];
  // The framing the clip had before the automatic crop was brought back,
  // so each frame's dress says whether Put the crop back would move its
  // crop, see FrameDress.back.
  back?: string;
};

// What an ask asks the Go side for, a frame and a size aside, with the key
// it is known by, which each frame's dress says back, see keyOf.
export function asked(ask: CaptionAsk): Record<string, string> {
  return {
    framing: ask.framing,
    ...(ask.on ? { on: ask.on } : {}),
    ...(ask.track ? { track: ask.track, ...(ask.typing ? { typing: ask.typing } : {}), ...ask.drafts } : {}),
    ...(ask.size ? { lw: String(ask.size[0]), lh: String(ask.size[1]) } : {}),
    ...(ask.back ? { back: ask.back } : {}),
    dressKey: keyOf(ask),
  };
}

// What tells one ask from another.
export function keyOf(ask: CaptionAsk | null): string {
  if (!ask) return "";
  return new URLSearchParams({
    path: ask.path,
    framing: ask.framing,
    ...(ask.on ? { on: ask.on } : {}),
    ...(ask.track ? { track: ask.track, ...(ask.typing ? { typing: ask.typing } : {}), ...ask.drafts } : {}),
    ...(ask.size ? { lw: String(ask.size[0]), lh: String(ask.size[1]) } : {}),
    ...(ask.back ? { back: ask.back } : {}),
  }).toString();
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

// The frames of the screen a bounce is drawn on, by the clock of each,
// the browser's own unless handed in. A walk hands in frames it gives
// itself one at a time, so a bounce is drawn on the same frames at the
// same moments on any machine, and a busy one cannot draw it on too few
// to see, issue #460.
export type Frames = { next(draw: (now: number) => void): number; cancel(id: number): void };
const screenFrames: Frames = {
  next: (draw) => requestAnimationFrame(draw),
  cancel: (id) => {
    if (typeof cancelAnimationFrame === "function") cancelAnimationFrame(id);
  },
};

// How the layers say in the app's log that one was given up on, see
// lib/said.ts, which the tests leave out.
export type Tell = (text: string, video: string) => void;

// The captions over the frame on screen, which came with the frame, see
// dressedOf, and the word a word step lands on, lifted out of them and
// bounced, see Lift. came is told when the parts of a lifted word come,
// and on every frame of the screen while it bounces.
export class CaptionLayers {
  private ask: CaptionAsk | null = null;
  // For the walks: how many words bounced, and how many times the screen
  // drew a bounce.
  readonly stats = { popped: 0, bounced: 0 };
  // The frames a bounce is drawn on, see Frames.
  frames: Frames = screenFrames;
  // The word lifted out of the frame paused on, and the frame on screen
  // when a word step asked for a bounce, which the next frame other than it
  // bounces on. Null when the last step was no word step.
  private lift: Lift | null = null;
  private shownAt = NaN;
  private bounceFrom: number | null = null;
  // The layer given last, which a word that bounces shows while its parts
  // are on their way, see raise.
  private last: Layer | null = null;
  // How many frames on screen wait for their dress for the clip as it is
  // now, see waitFor.
  private redressing = 0;

  // Lost is told every time a part of a lifted word is given up on, the
  // same failure again too, for the walks, see gaveUp.
  constructor(
    private came: () => void,
    private still?: Still,
    private tell?: Tell,
    private lost?: () => void,
  ) {}

  // What was said of parts given up on, so each failure is said once for
  // each video, not once a frame.
  private told = new Set<string>();

  // Says that what, a part of a lifted word, was given up on and why, so
  // a word that did not bounce leaves a trace.
  private gaveUp(ask: CaptionAsk, what: string, why: string) {
    this.lost?.();
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

  // Whether a dress is on its way or a word bounces, for the walks: until
  // neither, the screen may not show what it shows next.
  get busy(): boolean {
    return this.redressing > 0 || this.popping || (this.lift !== null && !this.lift.word);
  }

  // Whether the frame on screen waits for its dress for the clip as it is
  // now, for the walks.
  get late(): boolean {
    return this.redressing > 0;
  }

  // Settles once the frame on screen, paused, has its dress for the clip
  // as it is now, for the walks: the crop, the turn and the captions over
  // it are then those of the clip as it is now. Null when nothing is on
  // its way for it.
  dressed(): Promise<void> | null {
    if (!this.dressing) return null;
    return new Promise((done) => this.dressWaiters.push(done));
  }
  get dressing(): boolean {
    return this.redressing > 0 && this.still?.paused() !== false;
  }
  private dressWaiters: (() => void)[] = [];

  // The frame on screen waits for its dress for the clip as it is now
  // until dressed settles, see FrameQueue.redress.
  waitFor(dressed: Promise<unknown>) {
    this.redressing++;
    void dressed.finally(() => {
      this.redressing--;
      if (this.redressing) return;
      const was = this.dressWaiters;
      this.dressWaiters = [];
      for (const done of was) done();
      this.settleRest();
    });
  }

  // What the frames are dressed with now, which the parts of a lifted
  // word are asked with. Another video forgets the layer given last.
  set(ask: CaptionAsk | null) {
    if (!ask || ask.path !== this.ask?.path) this.last = null;
    this.ask = ask;
  }

  // The playhead was put somewhere, by a word step or by anything else.
  // The next frame other than the one on screen bounces its word after a
  // word step, and after anything else does not.
  sought(wordStep: boolean) {
    this.bounceFrom = wordStep ? this.shownAt : null;
  }

  // Whether the layer at gave last is the frame's own, rather than the
  // one a word lifted out of it shows while its parts are on their way,
  // for the walks, which read what libass drew over the frame.
  own = false;

  // What lies over the frame that starts at frame, at a size: the layer
  // that came with it, own, or the word lifted out of it, see Lift. Null
  // with nothing to lay over it.
  at(frame: number, size: [number, number], own: Layer | null): Layer | null {
    this.own = false;
    const lift = this.lift;
    // The word stays lifted only over the frame it was lifted from,
    // paused. New captions over that frame, a word corrected or the style
    // changed, lift it again at rest.
    if (lift && (!own || lift.frame !== frame || lift.size.join() !== size.join() || !this.still?.paused())) {
      this.drop();
      this.settleRest();
    }
    if (!own) return null;
    // The frame now on screen, so a second word step before its parts
    // came bounces the word it lands on, not the one it passed over.
    this.shownAt = frame;
    const key = `${own.dress?.key ?? ""}\n${own.tag ?? ""}\n${frame.toFixed(6)}\n${size.join("x")}`;
    const before = this.last;
    this.last = own;
    // The frame a word step came to.
    if (this.bounceFrom !== null && frame !== this.bounceFrom) {
      this.bounceFrom = null;
      if (own.showing && own.showing.word >= 0 && this.still?.paused()) this.raise(key, frame, size, own.scale === 1 ? own : before, !calm());
    } else if (this.lift && this.lift.key !== key) this.raise(key, frame, size, this.lifted(this.lift), false);
    if (!this.lift) {
      this.own = true;
      return own;
    }
    // A lift shows the layer it was lifted over until both its parts are
    // here.
    const shown = this.lifted(this.lift) ?? own;
    this.own = shown === own || (this.lift.key === key && !!this.lift.rest && !!this.lift.word);
    return shown;
  }

  // What the screen lays over the frame of a lift: the rest with the word
  // over it at its scale, or before while they are on their way.
  private lifted(lift: Lift): Layer | null {
    if (!lift.rest || !lift.word) return lift.before;
    return { ...lift.rest, scale: lift.scale, lift: { layer: lift.word, scale: lift.scale } };
  }

  // The word of the frame that starts at frame lifted out of its caption,
  // the two parts asked for at once, and bounced once both are there, or
  // held at rest.
  private raise(key: string, frame: number, size: [number, number], before: Layer | null, bounce: boolean) {
    const ask = this.ask;
    this.drop();
    if (!ask?.track) return;
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
    const showing = before?.showing;
    let why = "";
    const part = (part: "word" | "rest") =>
      fetch(`/frames/dress?${new URLSearchParams({ path: ask.path, ...asked(ask), at: frame.toFixed(6), w: String(size[0]), h: String(size[1]), part })}`, {
        signal: lift.off.signal,
      })
        .then(async (r) => {
          const layer = r.ok ? dressedOf(r.headers.get("X-Frame-Dress") ?? "", await r.arrayBuffer(), () => null) : null;
          if (layer && layer.w) {
            layer.showing ??= showing;
            if (part === "word") lift.bounce = readBounce(layer.dress?.bounce);
            return layer;
          }
          why = layer ? "an empty layer" : await failure(r);
          return null;
        })
        .catch((e) => {
          why = String(e);
          return null;
        });
    void Promise.all([part("rest"), part("word")]).then(([rest, word]) => {
      if (this.lift !== lift) return;
      if (!rest || !word) {
        // Without its parts the frame shows its own layer.
        this.gaveUp(ask, "the word lifted out", why);
        this.lift = null;
        this.drawAgain();
        return;
      }
      lift.rest = rest;
      lift.word = word;
      if (!lift.bouncing || !lift.bounce || typeof requestAnimationFrame !== "function") {
        lift.done = true;
        this.drawAgain();
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
        else lift.tick = this.frames.next(step);
        this.stats.bounced++;
        this.drawAgain();
      };
      lift.tick = this.frames.next(step);
      // At rest until the screen's first frame.
      this.came();
    });
  }

  // The frame on screen drawn again with what lies over it now.
  private drawAgain() {
    this.came();
    this.settleRest();
  }

  // The word put back, by a move to another frame, a play, or another ask.
  private drop() {
    if (!this.lift) return;
    this.frames.cancel(this.lift.tick);
    this.lift.off.abort();
    this.lift = null;
  }

  // Settles once no dress is on its way and no word bounces, for the
  // walks and the tests, which wait on it rather than on the clock.
  rested(): Promise<void> {
    return new Promise((done) => {
      this.restWaiters.push(done);
      this.settleRest();
    });
  }
  private restWaiters: (() => void)[] = [];

  private settleRest() {
    if (this.busy) return;
    const was = this.restWaiters;
    this.restWaiters = [];
    for (const done of was) done();
  }
}
