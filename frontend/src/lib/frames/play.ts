// A play of the video preview, worked out and read by the Go side, see
// engine.Play and docs/decisions/2026-11-19-go-plays-the-video-preview.md.
// The window says which pieces, whether they loop and where the playhead
// is, and pulls the play's pictures and its sound a batch at a time, each
// stamped with where on the play's clock it goes. Which frames, the cuts
// and the fades are the Go side's. What is left here is what a person
// sees and hears: the frames waiting to be drawn, and the sound waiting
// for the sound card.
//
// The clock is the program's: seconds from the start of the clip's first
// piece, on and on through a loop. A clip changed while it plays is taken
// into the play by the Go side, see take, on the same clock, so the
// picture and the sound go on with no stop.
import type { AppFrames } from "./app";
import type { Picture } from "./picture";

export type Piece = { start: number; end: number };

// A frame of a play: where on the clock it is drawn, the moment of the
// video the play stands at there, where its frame begins in the video,
// whether it is the first or last frame of its piece, which is drawn
// however short a time it is due for, how many changes of the clip the
// play had taken in, see take, and its picture.
export type Shot = { at: number; moment: number; begins: number; edge: boolean; gen: number; frame: Picture };

// Sound on its way to the sound card: where on the clock it starts, in
// samples, how many moments it holds, the changes taken in, and one array
// a channel.
export type Chunk = { out: number; length: number; gen: number; data: Float32Array<ArrayBuffer>[] };

// What a play is asked for: a clip by the name of its framing, see
// Framing.name, or the video by its pieces, whether it loops, the moment
// of the video to start from, the sound card's rate and channels, and
// whether it is a cue, see go.
export type PlayAsk = {
  framing: string | null;
  pieces: Piece[];
  loop: boolean;
  at: number;
  rate: number;
  channels: number;
  cue: boolean;
};

// How a play took a changed clip in, see take: how many changes it has
// taken, where on the clock this one took over, where the play now ends,
// Infinity while it loops, and where its first time through the new
// pieces ends. Video says the playhead was left outside the clip, and the
// play goes on as a play of the video.
export type Taken = { gen: number; at: number; end: number; lap: number; video: boolean };

// How many frames are pulled at a time, and how many moments of sound a
// chunk of the Go side's holds, engine.SoundChunk.
const BATCH = 4;
const SOUND_BATCH = 16;

export class GoPlay {
  // Where on the clock the play starts and ends, Infinity while it loops,
  // and where its first time through the clip ends. Where its frame
  // begins in the video, the frame it starts with, or -1 for none, and
  // the moment it starts from.
  readonly from: number;
  end: number;
  length: number;
  readonly first: number;
  readonly moment: number;
  // Where on the clock each change taken in took over, by how many
  // changes it made, see take. What came of fewer changes from there on
  // is the clip as it was, and goes.
  private cuts = new Map<number, number>();
  // The frames come and not drawn yet, in the order they are drawn.
  readonly shots: Shot[] = [];
  picturesEnded = false;
  soundEnded = false;
  // The Go side closed the play before its end, left unpulled for a
  // while, see previewIdle in frames.go, or to make room: nothing more
  // comes of it.
  gone = false;
  // Why the Go side stopped, in its words, or nothing.
  trouble = "";
  private pictures: string;
  private sound: string;
  private pullingPictures = false;
  private pullingSound: Promise<Chunk[]> | null = null;
  private closed = false;
  // Told when frames came, see more.
  onShots: (came: number) => void = () => {};

  private constructor(
    private app: AppFrames,
    readonly ask: PlayAsk,
    answer: { pictures: string; sound: string; from: number; end: number; length: number; first: number; moment: number },
    private size: { width: number; height: number },
  ) {
    this.pictures = answer.pictures;
    this.sound = answer.sound;
    this.from = answer.from;
    this.end = answer.end < 0 ? Infinity : answer.end;
    this.length = answer.length;
    this.first = answer.first;
    this.moment = answer.moment;
  }

  // Opens a play on the Go side, at the size the frames are drawn at, or
  // says why it could not.
  static async open(app: AppFrames, ask: PlayAsk, size: { width: number; height: number }): Promise<GoPlay | string> {
    const q = new URLSearchParams({
      path: app.path,
      w: String(size.width),
      h: String(size.height),
      grid: app.gridText,
      rate: String(ask.rate),
      ch: String(ask.channels),
      at: String(ask.at),
    });
    if (ask.framing) q.set("framing", ask.framing);
    else q.set("pieces", ask.pieces.map((p) => `${p.start}:${p.end}`).join(","));
    if (ask.loop) q.set("loop", "1");
    if (ask.cue) q.set("cue", "1");
    // Its frames come dressed as the frames are, see AppFrames.dressWith.
    app.dressing(q);
    try {
      const r = await fetch(`/frames/play?${q}`);
      if (!r.ok) return `the play could not start, it answered ${r.status}: ${(await r.text()).trim()}`;
      return new GoPlay(app, ask, await r.json(), size);
    } catch (e) {
      return `the play could not start: ${e instanceof Error ? e.message : String(e)}`;
    }
  }

  // Pulls the next frames while fewer than want are waiting, and tells
  // onShots how many came once they are here.
  more(want: number) {
    if (this.pullingPictures || this.picturesEnded || this.closed || this.shots.length >= want) return;
    this.pullingPictures = true;
    const { width, height } = this.size;
    void this.app.pull(`/frames/read?id=${this.pictures}&n=${BATCH}`, width, height, 40, BATCH).then((got) => {
      this.pullingPictures = false;
      if (this.closed) {
        for (const { frame } of got.frames) frame.close();
        return;
      }
      for (const { at, frame, stamp } of got.frames) {
        if (this.stale(stamp[3], at)) frame.close();
        else this.shots.push({ at, moment: stamp[0], begins: stamp[1], edge: stamp[2] === 1, gen: stamp[3], frame });
      }
      if (got.closed || got.status === 404) {
        this.gone = true;
        this.picturesEnded = true;
      } else if (got.end || got.status === 0) {
        this.picturesEnded = true;
        if (got.error) this.trouble = got.error;
      }
      this.onShots(got.frames.length);
    });
  }

  // The next sound, in the order it is heard, or nothing once it has all
  // come. One pull at a time.
  moreSound(): Promise<Chunk[]> {
    if (this.soundEnded || this.closed) return Promise.resolve([]);
    this.pullingSound ??= this.pullSound().finally(() => (this.pullingSound = null));
    return this.pullingSound;
  }

  get pullingSoundNow(): boolean {
    return this.pullingSound !== null;
  }

  private async pullSound(): Promise<Chunk[]> {
    const ch = this.ask.channels;
    let r: Response;
    try {
      r = await fetch(`/frames/read?id=${this.sound}&n=${SOUND_BATCH}`);
    } catch (e) {
      this.soundEnded = true;
      this.trouble = `the sound stopped: ${e instanceof Error ? e.message : String(e)}`;
      return [];
    }
    if (r.headers.get("X-Frames-Closed") || r.status === 404) {
      this.gone = true;
      this.soundEnded = true;
    } else if (r.headers.get("X-Frames-End")) {
      this.soundEnded = true;
      const why = r.headers.get("X-Frames-Error");
      if (why) this.trouble = why;
    }
    const body = new Uint8Array(await r.arrayBuffer());
    if (this.closed) return [];
    return this.fresh(soundOf(body, ch));
  }

  // Where on the clock what came of gen changes ends: where a later
  // change took over, or Infinity.
  private cutOf(gen: number): number {
    let cut = Infinity;
    for (const [g, at] of this.cuts) if (g > gen) cut = Math.min(cut, at);
    return cut;
  }

  // A frame of gen changes at a place on the clock is the clip as it was
  // before a later change took over there.
  private stale(gen: number, at: number): boolean {
    return at >= this.cutOf(gen) - 1e-9;
  }

  // The sound that is not the clip as it was, cut at the sample where a
  // later change took over.
  private fresh(chunks: Chunk[]): Chunk[] {
    const out: Chunk[] = [];
    for (const c of chunks) {
      const cut = Math.round(this.cutOf(c.gen) * this.ask.rate);
      if (c.out >= cut) continue;
      if (c.out + c.length > cut) c.length = cut - c.out;
      out.push(c);
    }
    return out;
  }

  // Hands the play the clip changed while it plays, by the name of its
  // new framing, and where on the clock its sound reached: the Go side
  // takes it in a little past there and says how, see servePlayTake, or
  // null where it cannot, the play paused or past its end, or where the
  // pieces are the same and only how the frames are dressed changed,
  // which the frames pulled from then on show. The frames come and
  // waiting of the clip as it was from there on go at once, and what is
  // pulled after.
  async take(framing: string, loop: boolean, heard: number): Promise<Taken | null> {
    if (this.closed) return null;
    const q = new URLSearchParams({ id: this.pictures, heard: String(heard), w: String(this.size.width), h: String(this.size.height) });
    if (loop) q.set("loop", "1");
    // The frames taken in are dressed as the frames are, by the framing
    // taken in.
    this.app.dressing(q);
    q.set("framing", framing);
    let got: Taken;
    try {
      const r = await fetch(`/frames/play/take?${q}`);
      if (r.status !== 200) return null;
      got = await r.json();
    } catch {
      return null;
    }
    if (this.closed) return null;
    if (got.end < 0) got.end = Infinity;
    this.cuts.set(got.gen, got.at);
    if (got.gen >= Math.max(0, ...this.cuts.keys())) {
      this.end = got.end;
      this.length = got.lap;
    }
    for (const s of this.shots.splice(0)) {
      if (this.stale(s.gen, s.at)) s.frame.close();
      else this.shots.push(s);
    }
    return got;
  }

  // Gives the Go side's decoder to a cued play: the space bar was pressed.
  go() {
    this.held = false;
    if (!this.closed) void fetch(`/frames/play/go?id=${this.pictures}`).catch(() => {});
  }

  // Paused: the Go side's decoder goes to the hand and the reading ahead,
  // and everything handed over stays, so go goes on from here at once.
  hold() {
    if (this.closed || this.held) return;
    this.held = true;
    void fetch(`/frames/play/hold?id=${this.pictures}`).catch(() => {});
  }
  held = false;

  // Switches the loop at a place on the clock, and says where the play
  // now ends.
  async loop(on: boolean, at: number): Promise<number> {
    const q = new URLSearchParams({ id: this.pictures, at: String(at) });
    if (on) q.set("loop", "1");
    try {
      const r = await fetch(`/frames/play/loop?${q}`);
      if (r.ok) {
        const { end } = (await r.json()) as { end: number };
        this.end = end < 0 ? Infinity : end;
      }
    } catch {}
    return this.end;
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    for (const s of this.shots) s.frame.close();
    this.shots.length = 0;
    for (const id of [this.pictures, this.sound]) void fetch(`/frames/close?id=${id}`).catch(() => {});
  }
}

// The sound in what the Go side sent for a pull of a play's sound: each
// chunk the sample of the clock it starts at, how many moments it holds
// and how many changes of the clip the play had taken in, 8 bytes each,
// then its float samples, the channels side by side, little endian, see
// servePlay. Chunks that follow one another of the same changes are one.
export function soundOf(body: Uint8Array, channels: number): Chunk[] {
  const v = new DataView(body.buffer, body.byteOffset, body.byteLength);
  // Where each chunk's samples are and where it goes, then one chunk for
  // each that follow one another.
  const parts: { at: number; n: number; gen: number; off: number }[] = [];
  for (let off = 0; off + 24 <= body.length; ) {
    const at = v.getFloat64(off, true);
    const n = v.getFloat64(off + 8, true);
    const gen = v.getFloat64(off + 16, true);
    off += 24;
    parts.push({ at, n: Math.min(n, Math.floor((body.length - off) / (4 * channels))), gen, off });
    off += n * channels * 4;
  }
  const out: Chunk[] = [];
  for (let i = 0; i < parts.length; ) {
    let j = i + 1;
    let length = parts[i].n;
    while (j < parts.length && parts[j].gen === parts[i].gen && parts[j].at === parts[i].at + length) length += parts[j++].n;
    const data = Array.from({ length: channels }, () => new Float32Array(length));
    let k = 0;
    for (; i < j; i++) {
      const p = parts[i];
      for (let m = 0; m < p.n; m++, k++) for (let c = 0; c < channels; c++) data[c][k] = v.getFloat32(p.off + (m * channels + c) * 4, true);
    }
    out.push({ out: parts[j - 1].at + parts[j - 1].n - length, length, gen: parts[j - 1].gen, data });
  }
  return out;
}
