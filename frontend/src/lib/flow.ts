// What the app does by itself, in one place and away from the screen, so
// it can be tested. Two promises live here: adding a video is enough for
// it to be transcribed, and the first clips are found as soon as the
// transcript covers the chosen stretch. Both have broken before, which is
// why they are written as plain rules with tests beside them.

export type TranscribeState = {
  // The episode file itself is gone.
  missing: boolean;
  // The episode has its work folder, <episode>.framefairy, beside it.
  work: boolean;
  // Anything of this episode is running or waiting in the queue.
  busy: boolean;
};

// An episode with no work folder has nothing at all: it was added a moment
// ago, or what it had was deleted by hand. Either way that is where a new
// episode starts, and a new episode transcribes itself.
export function shouldTranscribe(s: TranscribeState): boolean {
  return !s.missing && !s.work && !s.busy;
}

export type SearchState = {
  // How far the transcript reaches, in seconds, and the end of the stretch
  // chosen on the range picker.
  covered: number;
  to: number;
  // What the episode already has.
  plans: number;
  clips: number;
  // A search or a render is running, or one was asked for a moment ago.
  busy: boolean;
  // Anyone has ever searched this episode, in this run of the app or an
  // earlier one. An episode whose clips were all removed has still been
  // looked at, so it never gets a search of its own again.
  looked: boolean;
};

// The first search follows the transcript: as soon as it covers the chosen
// stretch, and only for an episode nobody has ever searched. Whether the
// transcript is finished is not part of it, so an episode that was already
// transcribed when it was added gets its clips too. An episode that was
// searched before and emptied since is not searched again, because a search
// is the machine's time and nobody asked for it.
export function shouldLook(s: SearchState): boolean {
  if (s.looked || s.busy) return false;
  if (s.plans > 0 || s.clips > 0) return false;
  if (s.to <= 0) return false;
  // Half a second of slack, because the transcript is counted in frames.
  return s.covered >= s.to - 0.5;
}

export type PictureState = {
  // The window has decoded a frame of the episode at all.
  ready: boolean;
  // The moment the picture is showing, or -1 while it shows nothing.
  shows: number;
  // Where the playhead stands.
  at: number;
};

// The picture has to agree with the playhead. While the machine is busy
// transcribing or searching, the window often cannot read the episode file,
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

export function pictureIsStale(s: PictureState): boolean {
  if (!s.ready || s.shows < 0) return true;
  return Math.abs(s.shows - s.at) > 0.5;
}

// A stretch of the episode, in seconds. The range picker works in these.
export type Stretch = { from: number; to: number };

// Where the window goes when it has to choose for itself: the first
// stretch nobody has looked at, and at most the first half hour of it.
// With the whole episode searched there is no free room left, so it rests
// on the last stretch that was searched, which is the one whose clips are
// on screen.
//
// A window is never left lying on material that has just been searched. It
// reads as an X-ray there, it hides the clip marks under it, and its trash
// can offers to throw away the clips that were only just found, which is
// the opposite of what the search was for.
export function nextWindow(
  free: Stretch[],
  searched: Stretch[],
  duration: number,
  firstLook: number,
): Stretch {
  const room = free.find((w) => w.to - w.from > 0.5);
  if (!room) {
    const last = searched[searched.length - 1];
    return { from: last?.from ?? 0, to: last?.to ?? duration };
  }
  const span = room.to - room.from;
  // A stretch only a little longer than the half hour is taken whole,
  // rather than leaving a scrap behind that is too short to search.
  return { from: room.from, to: room.from + (span > firstLook * 1.5 ? firstLook : span) };
}

// A run of asks where only the newest answer counts.
//
// The window asks the engine for the frame under the playhead every time
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

  // Whether this answer is still the newest one to arrive. Asking marks it
  // used either way, so an answer older than this one is never taken after
  // it, including the answer to an ask that failed.
  keep(ticket: number): boolean {
    if (ticket <= this.used) return false;
    this.used = ticket;
    return true;
  }
}

// How far an episode has been heard, which can only ever grow.
//
// Two numbers say it and they disagree on purpose. The saved transcript is
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
// So the furthest seen is kept, and the edge never falls below it. It
// starts over only when the mark is about a transcript that no longer
// exists: another episode, or one being read again from the beginning.
export class Heard {
  private at = 0;
  private of = "";

  // saved is how far the transcript on disk reaches. running is how far a
  // running transcription says it has got, or null when none is running.
  // restart says the mark is about to be meaningless: the transcript is out
  // of date and will be read again, or there is no work folder left at all.
  //
  // A transcription carried on after a pause reports from where the saved
  // transcript ends, which is behind the mark. That is not a step back:
  // those seconds were heard, they were only never written down, so the
  // edge stands still until the work passes it rather than rewinding.
  seen(episode: string, saved: number, running: number | null, restart: boolean): number {
    if (episode !== this.of || restart) {
      this.of = episode;
      this.at = 0;
    }
    const now = Math.max(saved, running ?? 0);
    if (now > this.at) this.at = now;
    return this.at;
  }
}

// A word as the engine hands it over: a moment and what was said.
export type Spoken = { start: number; end: number; text: string };

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

// The word of the episode a caption word came from.
//
// A caption word is not always one word of the episode. A correction that
// reads as two words is drawn as two, in the captions and in the render
// alike, but what the engine keeps is still the one word it corrected, at
// the one moment. Both halves have to find that one word, and they do,
// because both lie inside it.
//
// The match is made on the middle of the caption word rather than on
// either edge. An edge would do in every case there is, and that is the
// reason not to use one: an edge has no room on one side, so anything
// that moves a moment by a hair moves it out of the word. Walking two
// clocks adds and subtracts the same offsets in a different order, and
// that alone is enough to land a thousandth of a millisecond short. A cut
// that falls on a frame rather than on a word clips an edge outright. The
// middle is the one point in a caption word that is furthest from both
// edges of the word it came from.
//
// Nothing else is close enough to be meant. A middle that falls in no word
// at all and is not within a hair of one is no word, rather than the
// nearest one, because correcting a word nobody pointed at is worse than
// correcting none.
export function saidWord(pieces: Piece[], words: Spoken[], word: Piece): Spoken | null {
  // With no pieces there is no clip, and with no clip the two clocks are
  // not the same clock, they are one clock and one guess. A caption word
  // would then be read as a moment of the episode it has nothing to do
  // with and point at whatever word happened to be there.
  if (!pieces.length) return null;
  const when = inEpisode(pieces, (word.start + word.end) / 2);
  let near: Spoken | null = null;
  let off = Infinity;
  for (const w of words) {
    if (when >= w.start && when < w.end) return w;
    const away = when < w.start ? w.start - when : when - w.end;
    if (away < off) {
      off = away;
      near = w;
    }
  }
  // The same hair the engine allows itself when it decides which words
  // belong to a piece.
  return off <= 0.02 ? near : null;
}

// Whether the playhead is inside a clip, and so whether the crop frame is
// drawn solid or dashed.
//
// The frame a piece begins in belongs to it, and so does the one it ends
// in. Not the moment, the whole frame, and that is the whole of this.
//
// The playhead is put on a piece's first second and the picture answers
// with the frame it is showing, which is a frame at or before that second
// and never the second itself. The playhead is then a hair outside the
// clip it is standing at the very start of, and the crop frame says as
// much by going dashed the moment the clip is picked. One press of an
// arrow key put it right, which is the giveaway: what was wrong was a
// fraction of a frame and nothing else.
export function insideClip(pieces: Piece[], at: number, frame: number): boolean {
  return pieces.some((p) => at >= p.start - frame && at <= p.end + frame);
}
