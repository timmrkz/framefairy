// Back and Forward for the playhead, from the Go menu: the places it rested
// before, the way a browser keeps the pages it showed. Undo is for edits,
// and putting the playhead somewhere is not one, so this is a history of
// its own.
//
// A place is where the playhead stood still for a moment, paused, not every
// moment it passed through. A drag, playing or the keys walking the words
// go through hundreds of moments, and Back has to step over all of them to
// where the hand last stopped. Leaving a place, by a click, a clip chosen
// or playing on from it, puts it on the way back. So playing from a place
// and stopping somewhere else is two places, and Back after it is where
// playing started.
//
// Each place keeps the clip that was chosen there, so going back to it
// chooses that clip again. The clip timeline shows the clip chosen, and a
// place without its clip would be the right moment on the wrong clip.

export type Place = {
  // The playhead, in seconds of the episode.
  at: number;
  // The clip chosen there, or "" for none.
  clip: string;
};

// How long the playhead has to stand still before where it stands is a
// place. Long enough that a click on the way to somewhere else is not one.
export const settle = 1000;

// How far apart two places have to be to be two. Nearer than this, Back
// would go somewhere that looks like where it already is.
export const apart = 1;

// How long a jump of Back or Forward may take to land before what the
// playhead does is read again. A seek is not answered at once, and the
// moments it passes on its way are not the hand's.
const landing = 1500;

// How many places are kept each way.
const depth = 100;

function near(a: number, b: number): boolean {
  return Math.abs(a - b) < apart;
}

export class Places {
  private backs: Place[] = [];
  private forwards: Place[] = [];
  // Where the playhead rests now, once it has stood still long enough.
  private resting: Place | null = null;
  // Where it stands, and since when, while it may be about to rest.
  private still = { at: Number.NaN, since: 0 };
  // A jump of Back or Forward on its way, and when it should have landed.
  private going: { at: number; until: number } | null = null;

  // Told where the playhead is every so often, with the clip chosen,
  // whether the video plays, and the time now in milliseconds.
  see(at: number, clip: string, playing: boolean, now: number) {
    if (this.going) {
      if (!near(at, this.going.at) && now < this.going.until) return;
      this.going = null;
    }
    if (this.resting && !near(at, this.resting.at)) {
      this.keep(this.backs, this.resting);
      this.forwards = [];
      this.resting = null;
    }
    // Still is still within a frame or so: a paused video answers with the
    // moment of the frame it shows, which a seek made again can move by a
    // hair.
    if (playing || !(Math.abs(at - this.still.at) < 0.05)) {
      this.still = { at, since: now };
      return;
    }
    // A small step from a place, like a word walked to, is still that
    // place, and it is where the playhead rests now that it is kept.
    if (now - this.still.since >= settle && (this.resting?.at !== at || this.resting.clip !== clip)) {
      this.resting = { at, clip };
    }
  }

  // The place before, for Back, or undefined when there is none. here is
  // where the playhead is now, which Forward then comes back to.
  back(here: Place, now: number): Place | undefined {
    return this.go(this.backs, this.forwards, here, now);
  }

  // The place Back left, for Forward, or undefined when there is none.
  forward(here: Place, now: number): Place | undefined {
    return this.go(this.forwards, this.backs, here, now);
  }

  private go(from: Place[], to: Place[], here: Place, now: number): Place | undefined {
    // A place where the playhead already is would be a key that does
    // nothing.
    while (from.length && near(from[from.length - 1].at, here.at)) from.pop();
    const place = from.pop();
    if (!place) return undefined;
    this.keep(to, here);
    this.resting = place;
    this.still = { at: place.at, since: now };
    this.going = { at: place.at, until: now + landing };
    return place;
  }

  private keep(list: Place[], place: Place) {
    const last = list[list.length - 1];
    if (last && near(last.at, place.at)) list.pop();
    list.push(place);
    if (list.length > depth) list.splice(0, list.length - depth);
  }
}
