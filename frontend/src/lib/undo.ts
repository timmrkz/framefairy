import { changed, moved } from "./dictating";

// Undo and Redo of a field of text, kept by the page rather than the
// browser. The browser keeps the letters typed in a textarea as steps of
// its own, and text put in by the page, as a dictation does, is no step
// of its: once some came in, Undo took the wrong letters out and Redo put
// letters in twice, in Chromium and in WebKit alike. Here every change
// that is no step, like the words a dictation writes in, moves the steps
// it comes before, so each step still names its own letters.

// A change to the text: the letters from at to to are replaced by put.
export type Change = [number, number, string];

// A step: put stands at at where removed stood. Undone, removed stands
// there again, until it is done again.
type Step = { at: number; removed: string; put: string };

export class Undo {
  private undos: Step[] = [];
  private redos: Step[] = [];
  // How many steps the dictation in hand began after, which Undo goes
  // no further back than while it lasts. The steps before it name their
  // letters in the text as it was then.
  private floor = 0;

  // did keeps a change the person made, already in the text, which took
  // removed away. Typing on from the last letter typed is the same step,
  // and so is Backspace on from the last one, the way a Mac keeps them.
  // A new line is a step of its own, and so is a change made alone.
  did(at: number, to: number, removed: string, put: string, alone = false) {
    this.redos = [];
    const last = this.undos.length > this.floor && !alone ? this.undos.at(-1) : undefined;
    if (last && !removed && !last.removed && put && at === last.at + last.put.length && !/\n/.test(put + last.put.at(-1))) last.put += put;
    else if (last && removed && !put && !last.put && to === last.at) [last.at, last.removed] = [at, removed + last.removed];
    else if (last && removed && !put && !last.put && at === last.at) last.removed += removed;
    // A letter put together from more than one key, like ü, replaces the
    // mark it began with at the end of what was typed, and is the same step.
    else if (last && removed && put && at >= last.at && to === last.at + last.put.length) last.put = last.put.slice(0, at - last.at) + put;
    else this.undos.push({ at, removed, put });
  }

  // moved follows a change that is no step of its own: the letters from
  // at to to replaced by length letters. A step it comes in front of
  // moves along, and a change right at a step's end stays out of it.
  moved(at: number, to: number, length: number) {
    const move = (s: Step, here: string) => {
      const from = moved(s.at, at, to, length, here.length > 0);
      const end = moved(s.at + here.length, at, to, length, false);
      s.at = from;
      // A change inside the step's own letters leaves it naming others.
      if (end - from !== here.length) s.at = -1;
    };
    for (const s of this.undos.slice(this.floor)) move(s, s.put);
    for (const s of this.redos) move(s, s.removed);
  }

  // undo gives the change that takes the last step back in text, or
  // nothing when there is none, or when its letters are not there.
  undo(text: string): Change | null {
    if (this.undos.length <= this.floor) return null;
    const s = this.undos.pop()!;
    if (s.at < 0 || text.slice(s.at, s.at + s.put.length) !== s.put) return this.lost();
    this.redos.push(s);
    return [s.at, s.at + s.put.length, s.removed];
  }

  // redo gives the change that does the last step undone again.
  redo(text: string): Change | null {
    const s = this.redos.pop();
    if (!s) return null;
    if (s.at < 0 || text.slice(s.at, s.at + s.removed.length) !== s.removed) return this.lost();
    this.undos.push(s);
    return [s.at, s.at + s.removed.length, s.put];
  }

  // begin is the start of a dictation. Undo goes back no further than
  // here while it lasts, since the steps before it name letters of the
  // text before the words it wrote in.
  begin() {
    this.redos = [];
    this.floor = this.undos.length;
  }

  // end makes everything from the start of the dictation one step, the
  // text as it was then become the text as it is now, so Cmd+Z takes it
  // back at once and the steps before it name their letters again.
  end(before: string, now: string) {
    this.undos.length = this.floor;
    this.redos = [];
    this.floor = 0;
    const [at, to, put] = changed(before, now);
    if (at !== to || put) this.undos.push({ at, removed: before.slice(at, to), put });
  }

  // lost forgets every step, rather than take letters out that are not
  // the step's, which a change that was no step could leave behind.
  private lost(): null {
    this.undos = [];
    this.redos = [];
    this.floor = 0;
    return null;
  }
}
