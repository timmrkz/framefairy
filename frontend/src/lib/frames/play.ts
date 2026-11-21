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
// piece, on and on through a loop. A play started to take over from the
// one before it, the clip changed while it plays, counts on the clock of
// the one before, moved by shift, so the picture and the sound go on with
// no stop, see FrameQueue.changed.
import type { AppFrames } from "./app";
import type { Picture } from "./picture";

export type Piece = { start: number; end: number };

// A frame of a play: where on the clock it is drawn, the moment of the
// video the play stands at there, where its frame begins in the video,
// whether it is the first or last frame of its piece, which is drawn
// however short a time it is due for, and its picture.
export type Shot = { at: number; moment: number; begins: number; edge: boolean; frame: Picture };

// Sound on its way to the sound card: where on the clock it starts, in
// samples, how many moments it holds, and one array a channel.
export type Chunk = { out: number; length: number; data: Float32Array<ArrayBuffer>[] };

// What a play is asked for: the pieces of a clip, cuts jumped, whether
// they loop, the moment of the video to start from, the sound card's
// rate and channels, and whether it is a cue, see go. One that takes over
// from a play under way, after, starts at the moment that play stands at
// at a place on the clock, and counts on its clock, see open.
export type PlayAsk = {
  pieces: Piece[];
  loop: boolean;
  at: number;
  rate: number;
  channels: number;
  cue: boolean;
  after?: { play: GoPlay; at: number };
};

// How many frames are pulled at a time, and how many moments of sound a
// chunk of the Go side's holds, engine.SoundChunk.
const BATCH = 4;
const SOUND_BATCH = 16;

export class GoPlay {
  // Where on the clock the play starts and ends, Infinity while it loops,
  // and how long one time through the clip lasts. Where its frame begins
  // in the video, the frame it starts with, or -1 for none, and the moment
  // it starts from.
  readonly from: number;
  end: number;
  readonly length: number;
  readonly first: number;
  readonly moment: number;
  // Where on the clock its program starts.
  readonly shift: number;
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
    // A play that takes over starts where it takes over on the clock of
    // the one before.
    this.shift = ask.after ? ask.after.at - answer.from : 0;
    this.from = answer.from + this.shift;
    this.end = answer.end < 0 ? Infinity : answer.end + this.shift;
    this.length = answer.length;
    this.first = answer.first;
    this.moment = answer.moment;
  }

  // Opens a play on the Go side, at the size the frames are drawn at, or
  // says why it could not, or null for one taking over where the new
  // pieces do not hold the moment the play under way stands at.
  static async open(app: AppFrames, ask: PlayAsk, size: { width: number; height: number }): Promise<GoPlay | string | null> {
    const q = new URLSearchParams({
      path: app.path,
      w: String(size.width),
      h: String(size.height),
      grid: app.gridText,
      rate: String(ask.rate),
      ch: String(ask.channels),
      pieces: ask.pieces.map((p) => `${p.start}:${p.end}`).join(","),
      at: String(ask.at),
    });
    if (ask.loop) q.set("loop", "1");
    if (ask.cue) q.set("cue", "1");
    if (ask.after) {
      q.set("after", ask.after.play.pictures);
      q.set("place", String(ask.after.at - ask.after.play.shift));
    }
    try {
      const r = await fetch(`/frames/play?${q}`);
      if (r.status === 409) return null;
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
    void this.app.puller.pull(`/frames/read?id=${this.pictures}&n=${BATCH}`, width, height, 32, BATCH).then((got) => {
      this.pullingPictures = false;
      if (this.closed) {
        for (const { frame } of got.frames) frame.close();
        return;
      }
      for (const { at, frame, stamp } of got.frames) {
        this.shots.push({ at: at + this.shift, moment: stamp[0], begins: stamp[1], edge: stamp[2] === 1, frame });
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
    const shift = Math.round(this.shift * this.ask.rate);
    return soundOf(body, ch, shift);
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
    const q = new URLSearchParams({ id: this.pictures, at: String(at - this.shift) });
    if (on) q.set("loop", "1");
    try {
      const r = await fetch(`/frames/play/loop?${q}`);
      if (r.ok) {
        const { end } = (await r.json()) as { end: number };
        this.end = end < 0 ? Infinity : end + this.shift;
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
// chunk the sample of the clock it starts at and how many moments it
// holds, 8 bytes each, then its float samples, the channels side by side,
// little endian, see servePlay. Chunks that follow one another are one.
// shift moves them on the clock, in samples.
export function soundOf(body: Uint8Array, channels: number, shift = 0): Chunk[] {
  const v = new DataView(body.buffer, body.byteOffset, body.byteLength);
  // Where each chunk's samples are and where it goes, then one chunk for
  // each that follow one another.
  const parts: { at: number; n: number; off: number }[] = [];
  for (let off = 0; off + 16 <= body.length; ) {
    const at = v.getFloat64(off, true) + shift;
    const n = v.getFloat64(off + 8, true);
    off += 16;
    parts.push({ at, n: Math.min(n, Math.floor((body.length - off) / (4 * channels))), off });
    off += n * channels * 4;
  }
  const out: Chunk[] = [];
  for (let i = 0; i < parts.length; ) {
    let j = i + 1;
    let length = parts[i].n;
    while (j < parts.length && parts[j].at === parts[i].at + length) length += parts[j++].n;
    const data = Array.from({ length: channels }, () => new Float32Array(length));
    let k = 0;
    for (; i < j; i++) {
      const p = parts[i];
      for (let m = 0; m < p.n; m++, k++) for (let c = 0; c < channels; c++) data[c][k] = v.getFloat32(p.off + (m * channels + c) * 4, true);
    }
    out.push({ out: parts[j - 1].at + parts[j - 1].n - length, length, data });
  }
  return out;
}
