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
