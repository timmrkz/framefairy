// A double-click, told from two presses: the second comes within the
// Mac's time for a double-click of the first and within its distance,
// measured both ways, as the Mac measures it. Measured along the track
// only, a press on a caption block and then on a keyframe over it at the
// same x was a double-click, which removed the keyframe, #535. A press
// that makes a double-click is no first press of the next one, so three
// quick presses are one double-click and a press.
export type Press = { at: number; x: number; y: number };

// The Mac's time for a double-click, in ms, and the distance either way,
// in pixels.
export const DOUBLE_CLICK_MS = 500;
export const DOUBLE_CLICK_PX = 6;

const none: Press = { at: -Infinity, x: 0, y: 0 };

export class Presses {
  #last: Press = none;

  // Whether this press is the second of a double-click.
  twice(p: Press): boolean {
    const last = this.#last;
    const again = p.at - last.at < DOUBLE_CLICK_MS && Math.abs(p.x - last.x) < DOUBLE_CLICK_PX && Math.abs(p.y - last.y) < DOUBLE_CLICK_PX;
    this.#last = again ? none : p;
    return again;
  }
}
