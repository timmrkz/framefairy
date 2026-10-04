// What the workspace decides by itself, in one place and away from the
// screen, so it can be tested. When a search starts and stops is not among
// it: that is the Go side's, see docs/JOBS.md, and its tests are the path
// tests in cmd/framefairy-app.

export type PictureState = {
  // The video preview has decoded a frame of the episode at all.
  ready: boolean;
  // The moment the picture is showing, or -1 while it shows nothing.
  shows: number;
  // Where the playhead stands.
  at: number;
  // How long one frame of the episode lasts. A frame on screen stands for
  // every moment from its own to the next frame's, so a playhead that has
  // moved on inside it is still in the picture. Left out, it is taken as
  // nothing.
  frame?: number;
};

// The picture has to agree with the playhead. While the machine is busy
// transcribing or searching, the app often cannot read the episode file,
// so a seek is dropped and the picture stays on a frame that has nothing to
// do with where the playhead is. Whenever that happens the workspace asks
// the engine for the frame under the playhead instead.
// A piece of a clip, in the episode's own seconds.
export type Piece = { start: number; end: number };

// The piece a time falls in, or the one it runs into next. Past the last
// piece it is the last one, so a time beyond the clip still names a piece
// rather than nothing.
export function pieceAt(pieces: Piece[], at: number): number {
  const index = pieces.findIndex((p) => at < p.end);
  return index < 0 ? Math.max(pieces.length - 1, 0) : index;
}

// The piece the player is playing, kept inside the pieces that exist.
//
// The pieces change under the player whenever a cut is taken out or put
// back, and the player holds the one it is on as a number. A clip in three
// pieces playing its third becomes a clip in two pieces the moment a cut
// goes back, and the number then points past the end. Reading it gave
// undefined, and asking undefined where it ended threw inside the frame
// loop, which ended the loop: the playhead stopped moving and the picture
// stood still on whatever frame it had. That is a video preview that has
// got lost, and it took nothing more than putting a cut back while the
// clip was playing past it.
export function playingPiece(pieces: Piece[], was: number, at: number): number {
  if (!pieces.length) return 0;
  if (was >= 0 && was < pieces.length) return was;
  return pieceAt(pieces, at);
}

//
// What the picture shows is the moment of the frame on screen, where the
// browser can say so, see watchFrames in Player.svelte. That frame stands
// until the next one, so a playhead ahead of it by up to a frame is in it,
// and only the half second of room on top of that is what a seek is
// allowed to land off by. A playing episode of one frame a second read as
// stale for half of every second, and the frame the engine read was
// drawn over it.
export function pictureIsStale(s: PictureState): boolean {
  if (!s.ready || s.shows < 0) return true;
  if (s.at >= s.shows) return s.at - s.shows > 0.5 + (s.frame ?? 0);
  return s.shows - s.at > 0.5;
}

// Where the playhead goes while the video plays, from where it stands and
// what the video's clock says now. On with the clock, never back by a
// little: a clock that steps back while playing is the clock being put
// right, not the picture going back, see Player.svelte. A step back of half
// a second or more is the video really being somewhere else, the same half
// second pictureIsStale takes for the same place, and is followed, so a
// playhead can never be held away from a picture that moved.
export function onward(at: number, clock: number): number {
  if (clock >= at || at - clock >= 0.5) return clock;
  return at;
}

// What the video is doing on a frame of a play, as far as the playhead is
// concerned.
export type PlayingClock = {
  // The video's clock.
  clock: number;
  // The video has read nothing of the file yet, HAVE_NOTHING.
  empty: boolean;
  // A seek is on its way, so the clock is where it was sent.
  seeking: boolean;
  // A jump the playing clip made by itself has just landed.
  landed: boolean;
};

// Where the playhead goes on a frame of a play, from where it stands. A
// video that has read nothing of the file has a clock that says nothing:
// it answers zero, playing or not. Followed, the first press of the space
// bar after a search took the playhead to the start of the episode, the
// still of the clip's first frame gave way to a black picture and then to
// the start of the episode, and only once the file came did the video go
// to the clip and play it. So the playhead stands, with the still that
// shows it, until the video has something. Otherwise it goes with the
// clock, and only forward, see onward.
export function playingAt(at: number, v: PlayingClock): number {
  if (v.empty) return at;
  if (v.landed || v.seeking) return v.clock;
  return onward(at, v.clock);
}

// Where the frame a moment falls in starts: the frame a video element
// shows when it is sent there, the last one that starts at or before it.
// The still read from the file while the video preview catches up has to
// be that same frame, or the picture changes the moment the video lands.
// It was the nearest whole second, which from half past on is the next
// second's frame. The engine works the frame out the same way, Still in
// engine/frames.go. An episode whose rate is not known counts in seconds.
export function frameStart(t: number, fps: number): number {
  const rate = fps > 0 ? fps : 1;
  return Math.floor(Math.max(t, 0) * rate + 1e-6) / rate;
}

// Whether the still read for one frame belongs over the picture at a
// moment. Paused, only the still of the very frame the playhead is in: a
// still of a frame near it was a second picture for one spot, and a third
// once the video landed. Playing, the still of where the play began also
// stands for the half second after it, the room a seek is given, see
// pictureIsStale. Safari plays on before it shows the frame it plays from,
// see watchFrames in Player.svelte, and with the still of that one frame
// alone, the frame before it showed through again the moment the playhead
// left it, one frame into the play.
export function stillFits(stillAt: number, at: number, fps: number, playing: boolean): boolean {
  const here = frameStart(at, fps);
  if (Math.abs(stillAt - here) < 1e-6) return true;
  return playing && stillAt < here && here - stillAt <= 0.5;
}

// Parts of the episode, from and to in seconds, in order and apart. The
// transcript is heard in parts, wherever a search or a clip made by hand
// needs it first, see docs/ENGINE.md, and the Go side says which.
export type Parts = [number, number][];

// The parts in order, with any that touch or overlap made one.
export function joinParts(parts: Parts): Parts {
  const out: Parts = [];
  for (const [a, b] of [...parts].filter(([a, b]) => b > a).sort((x, y) => x[0] - y[0])) {
    const last = out[out.length - 1];
    if (last && a <= last[1] + 0.005) last[1] = Math.max(last[1], b);
    else out.push([a, b]);
  }
  return out;
}

// How many seconds of from..to the parts hold.
export function heardIn(parts: Parts, from: number, to: number): number {
  let sum = 0;
  for (const [a, b] of parts) sum += Math.max(Math.min(b, to) - Math.max(a, from), 0);
  return sum;
}

// Whether the parts hold all of from..to, give or take the half second
// the rest of the workspace allows an edge.
export function covers(parts: Parts, from: number, to: number): boolean {
  return heardIn(parts, from, to) >= to - from - 0.5;
}

// What of from..to the parts do not hold, in order.
export function gapsIn(parts: Parts, from: number, to: number): Parts {
  const out: Parts = [];
  let at = from;
  for (const [a, b] of parts) {
    if (b <= at || a >= to) continue;
    if (a > at) out.push([at, a]);
    at = Math.max(at, b);
  }
  if (at < to) out.push([at, to]);
  return out;
}

// How much of a window has been transcribed, by what has been heard: the
// row of a search waiting for the transcript is a view of the window on the
// range picker. Empty while nothing of the window is heard, full when all
// of it is. It was measured from the start of the episode, so a window two
// hours in began nearly full.
export function waitShare(from: number, heard: Parts, to: number): number {
  if (to - from < 0.5) return covers(heard, from, to) ? 1 : 0;
  return Math.min(Math.max(heardIn(heard, from, to) / (to - from), 0), 1);
}

// A part of the episode, in seconds. The range picker works in these.
export type Span = { from: number; to: number };

// The episode in parts, each with how many searches have read it, from
// its start to its end, the parts nobody has searched too, see
// SearchPasses in engine/windows.go.
export type Passes = { from: number; to: number; times: number }[];

// Where the window goes when it has to choose for itself: where the
// fewest searches have been, earliest first, and at most one window of
// it, as long as the episode's windows are, see suggestedWindow in
// suggest.ts. The first round walks the episode from its start, and when
// every part has been searched once the second starts over at the start,
// and so on, so New always does the same thing and never runs out. A part
// shorter than least, too short to hold a clip, is passed over.
//
// A window is never left lying on material that has just been searched
// while there is a part searched fewer times: its search made that part
// one more, so it is not the fewest any more.
export function nextWindow(passes: Passes, duration: number, size: number, least = 0): Span {
  const usable = passes.filter((p) => p.to - p.from >= Math.max(least, 0.5));
  if (!usable.length) return { from: 0, to: duration };
  const fewest = Math.min(...usable.map((p) => p.times));
  const room = usable.find((p) => p.times === fewest)!;
  const span = room.to - room.from;
  // A part only a little longer than a window is taken whole, rather than
  // leaving a scrap behind that is too short to search.
  return { from: room.from, to: room.from + (span > size * 1.5 ? size : span) };
}

// The round step a window's edges land on, on a range picker width pixels
// wide: the smallest round step still about eight pixels wide, so a window
// is something that can be said out loud. Five minutes for four hours on
// a narrow range picker, five seconds for six minutes. A drag lands on it,
// and so does every window the app places, so the two never disagree by
// the few seconds that show as a pixel.
export function gridStep(duration: number, width: number): number {
  const steps = [1, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800];
  const least = (duration / Math.max(width, 1)) * 8;
  return steps.find((step) => step >= least) ?? 1800;
}

// A window on the step: its start, and its length, which is never less
// than one step. The ends of the episode win over the step, and an end
// that would leave a scrap too short for a clip, least, or shorter than a
// step, takes it in rather than leaving it behind. With no step, the
// range picker not measured yet, the window is as it was.
export function onGrid(w: Span, length: number, duration: number, step: number, least = 0): Span {
  if (step <= 0) return { from: w.from, to: Math.min(w.from + length, duration) };
  const from = Math.min(Math.max(Math.round(w.from / step) * step, 0), duration);
  const size = Math.max(Math.round(length / step) * step, step);
  let to = Math.min(from + size, duration);
  if (duration - to < Math.max(least, step)) to = duration;
  return { from, to };
}

// Where the window goes after a search: on from where the search ended,
// as long as the person left the window, so a window made a minute long
// stays a minute long and the episode is walked in the steps they chose.
// The last one is cut at the end of the episode, and is only cut: the
// window after it is as long as the person made it again, so length is
// what they set and not the window just searched. At the end of the
// episode it starts over at the start, and a scrap at the end shorter than
// least, too short for a clip, is passed over the same way.
export function followingWindow(ended: number, length: number, duration: number, least = 0): Span {
  let from = ended;
  if (duration - from < Math.max(least, 0.5)) from = 0;
  return { from, to: Math.min(from + Math.max(length, 0), duration) };
}

// How many searches have read any of from..to, the most of them.
export function timesIn(passes: Passes, from: number, to: number): number {
  let most = 0;
  for (const p of passes) if (p.to > from + 0.5 && p.from < to - 0.5) most = Math.max(most, p.times);
  return most;
}

// A run of asks where only the newest answer counts.
//
// The app asks the engine for the frame under the playhead every time
// the playhead moves, and walking the clip list with the arrow keys moves
// it as fast as a key repeats. Several asks are then in the air at once,
// and nothing says they come back in the order they went out: a frame that
// took longer to read lands after a newer one and paints over it, and the
// picture is then of somewhere the playhead left. It stays wrong, because
// nothing asks again.
//
// Every ask takes a ticket. An answer is used only if no newer answer has
// been used already.
export class Newest {
  private sent = 0;
  private used = 0;

  // The ticket for an ask about to be sent.
  send(): number {
    return ++this.sent;
  }

  // The ticket the next ask will get, so something can wait for the
  // first answer asked from now on.
  next(): number {
    return this.sent + 1;
  }

  // Whether this answer is still the newest one to arrive. Asking marks it
  // used either way, so an answer older than this one is never taken after
  // it, including the answer to an ask that failed.
  keep(ticket: number): boolean {
    if (ticket <= this.used) return false;
    this.used = ticket;
    return true;
  }
}

// How something waits, which is the browser's timers in the app and a
// clock of its own in a test.
export type Timers = {
  later: (run: () => void, ms: number) => unknown;
  off: (waiting: unknown) => void;
};

const browserTimers: Timers = {
  later: (run, ms) => setTimeout(run, ms),
  off: (waiting) => clearTimeout(waiting as ReturnType<typeof setTimeout>),
};

// An ask that waits a moment before it is sent, so a need that passes by
// itself costs nothing.
//
// The workspace asks the engine for the frame under the playhead whenever
// the video preview cannot show it, see pictureIsStale. Most of the time
// the video lands a few milliseconds later and the frame was never needed,
// so the ask waits first. A newer need takes the place of the one waiting,
// which is what keeps a hand moving the playhead from sending an ask for
// every place it passes. null says nothing is needed any more.
export class Grace<T> {
  private waiting: unknown = undefined;

  constructor(
    private send: (what: T) => void,
    private wait: number,
    private timers: Timers = browserTimers,
  ) {}

  need(what: T | null): void {
    if (what === null) return;
    this.timers.off(this.waiting);
    this.waiting = this.timers.later(() => {
      this.waiting = undefined;
      this.send(what);
    }, this.wait);
  }
}

// What of an episode has been heard, which can only ever grow.
//
// Two things say it and they disagree on purpose. The saved transcript is
// rewritten whole, so it lands seconds apart and jumps minutes of audio at
// a time. A running transcription says where it got to as each chunk
// finishes, which is far more often. The range picker draws the second one,
// because that is the one that moves with the work.
//
// The moment the transcription stops, the second one is gone and what is
// left is the number from disk, which is behind it by however much had not
// been saved. The edge then walks backwards, and it cannot: it says how
// much of the episode has been read, and reading does not unhappen. Pausing
// showed this plainly, because pausing is the one moment the live number
// disappears while the saved one is at its most stale.
//
// The same holds for answers that cross. Every refresh of the episode is a
// call of its own, so an older one can land after a newer one and carry a
// smaller number with it.
//
// So everything seen is kept, and what is heard never shrinks. It starts
// over only when the mark is about a transcript that no longer exists:
// another episode, or one being read again from the beginning.
// A job as the Go side reports it, by its id and the number of its last
// change. The number grows with every change to any job, and is set where
// the change is made, so of two snapshots of one job the later one has the
// larger number. News is sent after the change is made, so two changes at
// nearly the same moment can arrive the other way round, and a list that
// kept whatever it heard last showed a finished job as waiting, for good.
export type Stamped = { id: string; seq?: number };

// The list with got in it, or null when the list already has a later
// snapshot of that job. A snapshot without a number is taken as it comes.
export function mergeJob<T extends Stamped>(list: T[], got: T): T[] | null {
  const i = list.findIndex((j) => j.id === got.id);
  if (i < 0) return [...list, got];
  if ((got.seq ?? 0) < (list[i].seq ?? 0)) return null;
  const out = list.slice();
  out[i] = got;
  return out;
}

export class Heard {
  private parts: Parts = [];
  private of = "";

  // saved are the parts the transcript on disk holds. running is the part a
  // running transcription says it has heard, from where it began to where
  // it has got, or null when none is running. restart says the mark is
  // about to be meaningless: the transcript is out of date and will be read
  // again, or there is no work folder left at all.
  //
  // A transcription carried on after a pause reports from where the saved
  // transcript ends, which is behind the mark. That is not a step back:
  // those seconds were heard, they were only never written down, so the
  // edge stands still until the work passes it rather than rewinding.
  seen(episode: string, saved: Parts, running: [number, number] | null, restart: boolean): Parts {
    if (episode !== this.of || restart) {
      this.of = episode;
      this.parts = [];
    }
    const now = [...this.parts, ...saved.map(([a, b]) => [a, b] as [number, number])];
    if (running) now.push([running[0], running[1]]);
    const joined = joinParts(now);
    // The same parts as before are the same answer, so what reads them
    // does not see a change that is not one.
    if (JSON.stringify(joined) !== JSON.stringify(this.parts)) this.parts = joined;
    return this.parts;
  }
}

// Where a moment of a clip falls in the episode.
//
// A clip runs on its own clock, with the cuts taken out, and that is the
// clock the captions are written on. This is the way back from it. It is
// what a word in the caption box needs in order to say which word of the
// episode it is, because a correction belongs to the episode and not to
// the clip: the same word corrected once is corrected in every clip that
// holds it.
export function inEpisode(pieces: Piece[], at: number): number {
  let sum = 0;
  for (const p of pieces) {
    const span = p.end - p.start;
    if (at < sum + span) return p.start + Math.max(at - sum, 0);
    sum += span;
  }
  const last = pieces[pieces.length - 1];
  return last ? last.end : at;
}

// Where a moment of the episode falls in a clip, the other way from
// inEpisode. A moment the clip cuts out is the moment it comes back, and
// one before or after it is its start or its end.
export function inClip(pieces: Piece[], at: number): number {
  let sum = 0;
  for (const p of pieces) {
    if (at < p.start) return sum;
    if (at <= p.end) return sum + at - p.start;
    sum += p.end - p.start;
  }
  return sum;
}


// Whether the playhead is inside a clip, and so whether the crop frame is
// drawn at all.
//
// The frame a piece begins in belongs to it, and so does the one it ends
// in. Not the moment, the whole frame, and that is the whole of this.
//
// The playhead is put on a piece's first second and the picture answers
// with the frame it is showing, which is a frame at or before that second
// and never the second itself. The playhead is then a hair outside the
// clip it is standing at the very start of, and the crop frame said as
// much by going dashed the moment the clip was picked. One press of an
// arrow key put it right, which is the giveaway: what was wrong was a
// fraction of a frame and nothing else.
export function insideClip(pieces: Piece[], at: number, frame: number): boolean {
  return pieces.some((p) => at >= p.start - frame && at <= p.end + frame);
}

export type ChaseState = {
  // Where the picture was sent, or -1 when it is not on its way anywhere.
  wanted: number;
  // Where it says it is, and whether it is playing.
  at: number;
  playing: boolean;
  // How many times the seek has been made again already.
  tries: number;
};

// Whether a seek that has not landed should be made again.
//
// A seek is chased because the webview drops one silently while the
// machine is busy, and a dropped seek leaves the picture on a frame that
// has nothing to do with the playhead. It is asked again, and then the
// file is read once more, and then it gives up.
//
// **It gives up at once when the picture is playing.** A playing clock is
// meant to run away from where it was sent: a second and a bit later it is
// a second and a bit further on, which reads exactly like a seek that
// never landed. The chase would then pull the picture back to where
// playing began, and on the try after that read the file again, which
// empties the element and stops it dead. Every play arms a chase, because
// playing seeks first, and the only thing that saved it was the seek
// answering in time. A seek that changes nothing answers with nothing:
// the playhead was already on that frame, no seeked ever comes, and the
// chase is left running against the playing picture.
export function shouldChase(s: ChaseState): boolean {
  if (s.wanted < 0 || s.playing) return false;
  if (s.tries > 2) return false;
  return Math.abs(s.at - s.wanted) >= 0.5;
}

// A caption edge being dragged, on the clip's clock.
export interface CaptionDraft {
  index: number;
  edge: "start" | "end";
  at: number;
}

// The captions with one edge where it is being dragged, the way the engine
// will put them once it is let go: a caption that appears earlier or later
// takes the one before it along where the two met, and never lies over it.
// Anything that shows the captions while an edge is dragged shows these, so
// the timeline and the video preview move together.
export function draftCaptions<T extends { start: number; end: number }>(
  captions: T[],
  draft: CaptionDraft | null,
): T[] {
  if (!draft || !captions[draft.index]) return captions;
  const out = captions.map((c) => ({ ...c }));
  const i = draft.index;
  if (draft.edge === "end") {
    out[i].end = draft.at;
    return out;
  }
  const was = out[i].start;
  out[i].start = draft.at;
  const before = out[i - 1];
  if (before && (Math.abs(before.end - was) < 0.001 || before.end > draft.at)) {
    before.end = draft.at;
  }
  return out;
}

// Which word of a caption is lit at a moment on the clip's clock, counted
// across its lines, or -1 before its first word. A word is lit from its
// start until the next word starts, and the last until the caption goes,
// the way the render burns the highlight in (engine/highlight.go). It is
// not lit only until its own end: the video preview did that, and a word
// went dark while it was still being said and between every two words,
// where the short and the clip timeline went on showing it.
export function litWord(lines: { words: { start: number }[] }[], at: number): number {
  let lit = -1;
  let n = 0;
  for (const line of lines) {
    for (const word of line.words) {
      if (at >= word.start) lit = n;
      n++;
    }
  }
  return lit;
}
