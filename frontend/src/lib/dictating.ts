// What a dictation does to the text of a field, as plain text and
// numbers, with no page in it. The field is a textarea, which draws the
// text and the cursor, moves the cursor and scrolls, the way every text
// field does. This only says what the text becomes as the speech model
// writes down what is said and the person types in it, and where the
// cursor goes. The report's field was a page of its own making, rebuilt
// ten times a second under the cursor, and each way WebKit read it
// differently was a bug Tim found: a cursor that bounced, jumped or was
// drawn on every line (29 October). Every place is counted in letters of
// the text, the way a textarea counts its selection.

// A letter a space goes in front of, after a word: not a full stop, a
// comma or a closing bracket.
const word = /^[^\s.,;:!?)\]}]/;

// What the Go side is told: the words being heard are the next piece, as
// they stand, or the next piece ends where they end, and what is said
// after it is the piece after. See listen.go.
export type Ask = { settle: number; words: string } | { cut: number };

// A stretch of the text, from its first letter to the one after its last,
// and a selection, from where it starts to where it ends.
export type Span = [number, number];

// A place kept for a piece still to come, with the words heard there
// standing in until it does. rest is a place for every piece still to
// come, kept when something is typed right after the words while the
// last of them are being written down.
type Place = { at: Span; piece: number; rest: boolean };

export class Dictating {
  text: string;
  // The words being heard, which the speech model may still change. A
  // piece it settles goes in at their start, and they follow it.
  heard: Span;
  places: Place[] = [];
  // How many pieces of the recording have a place in the text, and from
  // which piece on the last words are the person's, who typed in them
  // while they were being written down.
  placed = 0;
  theirs = Infinity;
  // Whether right before the end of what was said are words that were
  // said rather than typed: a word typed there gets a space in front.
  spoke = false;
  // Every change made to the text, from where to where it took letters
  // away and how many it put in, for the field's undo, see lib/undo.ts.
  moves: [number, number, number][] = [];

  constructor(text: string, at: number) {
    this.text = text;
    this.heard = [at, at];
  }

  // end is where what was said ends, where the next words go.
  get end(): number {
    return this.heard[1];
  }

  // hear takes in what the speech model says: every piece settled so far
  // and what is still being heard. It is given the selection and gives
  // it back where it is now. A cursor at the end stays at the end, so it
  // follows the words, and anywhere else it stays by the letters it was
  // by. last says the speech model has written down everything, and the
  // place kept for the rest takes what is left.
  hear(said: string[], hearing: string, sel: Span, last = false): Span {
    const atEnd = sel.map((p) => p === this.end);
    let [a, b] = sel;
    const put = (at: number, to: number, s: string) => {
      [a, b] = [moved(a, at, to, s.length, false), moved(b, at, to, s.length, false)];
      this.replace(at, to, s);
    };
    for (const p of [...this.places]) {
      if (p.rest ? !last : p.piece >= said.length) continue;
      this.places.splice(this.places.indexOf(p), 1);
      const words = p.rest ? said.slice(p.piece, this.theirs).join(" ") : said[p.piece];
      put(p.at[0], p.at[1], this.spaced(p.at[0], p.at[1], words));
      if (p.rest) this.placed = Math.max(this.placed, said.length);
    }
    put(this.heard[0], this.heard[1], "");
    for (; this.placed < Math.min(said.length, this.theirs); this.placed++) {
      const at = this.heard[0];
      put(at, at, this.spaced(at, at, said[this.placed]));
      this.spoke ||= !!said[this.placed].trim();
    }
    const at = this.heard[0];
    const now = this.theirs === Infinity ? this.spaced(at, at, hearing) : "";
    put(at, at, now);
    this.heard = [at, at + now.length];
    this.spoke ||= !!now;
    return [atEnd[0] ? this.end : a, atEnd[1] ? this.end : b];
  }

  // edited follows an edit the person made, which put put where the
  // letters from at to to were, and says what the Go side is to be told.
  // Typed in the words being heard, or in a place whose piece is still to
  // come, the words there are the person's as they stand. Typed right
  // after the words being heard, those words are the next piece, in a
  // place of their own, and what is typed stays after them: what is said
  // next goes after it, and what was said before it stays before it,
  // issues #573 and #584. Typed at the end of what was said, it is part of
  // it, and what is said next goes after it.
  edited(at: number, to: number, put: string, listening: boolean): Ask[] {
    const asks: Ask[] = [];
    const [h, e] = this.heard;
    const into = (s: Span) => s[0] < s[1] && ((at < s[1] && to > s[0]) || (at === to && at > s[0] && at < s[1]));
    if (into(this.heard)) {
      if (listening) asks.push(this.settle()!);
      else {
        this.theirs = this.placed;
        this.heard = [e, e];
      }
    } else if (h < e && at === e && to === e && put) {
      if (listening) asks.push({ cut: this.placed });
      this.places.push({ at: [h, e], piece: this.placed, rest: !listening });
      if (listening) this.placed++;
      this.heard = [e, e];
    }
    for (const p of [...this.places]) {
      if (!into(p.at)) continue;
      this.places.splice(this.places.indexOf(p), 1);
      if (p.rest) this.theirs = p.piece;
    }
    if (put && at === this.end) this.spoke = false;
    this.replace(at, to, put);
    return asks;
  }

  // typing is what is typed at the selection puts in: with a space in
  // front right after a word that was said.
  typing(sel: Span, put: string): string {
    if (!this.spoke || sel[0] !== this.end || sel[1] !== this.end) return put;
    return /\S$/.test(this.text.slice(0, this.end)) && word.test(put) ? " " + put : put;
  }

  // inside says whether the selection is in the words being heard, where
  // the next change to them would move the letters under it.
  inside(sel: Span): boolean {
    const [h, e] = this.heard;
    if (h === e) return false;
    if (sel[0] === sel[1]) return sel[0] > h && sel[0] < e;
    return sel[0] < e && sel[1] > h;
  }

  // settle makes the words being heard the next piece where they stand,
  // and says what to tell the Go side.
  settle(): Ask | null {
    const [h, e] = this.heard;
    if (h === e) return null;
    const ask = { settle: this.placed, words: this.text.slice(h, e).trim() };
    this.placed++;
    this.heard = [e, e];
    return ask;
  }

  // spaced is words to go where the letters from at to to are, with a
  // space from a word before them and from a word after them.
  private spaced(at: number, to: number, words: string): string {
    words = words.trim();
    if (!words) return "";
    const lead = /\S$/.test(this.text.slice(0, at)) && word.test(words) ? " " : "";
    const trail = /\S$/.test(words) && word.test(this.text.slice(to)) ? " " : "";
    return lead + words + trail;
  }

  // replace puts s where the letters from at to to are, and moves every
  // place counted after them. What goes in right where the words being
  // heard start or end goes in front of them, and what goes in right
  // after a place stays out of it.
  private replace(at: number, to: number, s: string) {
    this.text = this.text.slice(0, at) + s + this.text.slice(to);
    if (at !== to || s) this.moves.push([at, to, s.length]);
    const move = (p: number, right: boolean) => moved(p, at, to, s.length, right);
    this.heard = [move(this.heard[0], true), move(this.heard[1], true)];
    for (const p of this.places) p.at = [move(p.at[0], true), move(p.at[1], false)];
  }
}

// moved is where place p is once the letters from at to to are replaced
// by length letters. A place in them goes to their start, or with right
// to their end, and so does a place right at an insertion.
export function moved(p: number, at: number, to: number, length: number, right: boolean): number {
  if (p > to || (p === to && (at < to || right))) return p + length - (to - at);
  if (p > at) return right ? at + length : at;
  return p;
}

// edit is the edit that made now out of old, with the cursor at caret
// after it: where it starts, where what it took away ended, and what it
// put in. The text after the cursor is what it was, after typing and
// deleting, so the edit ends at the cursor, even in a run of one letter,
// where comparing the texts alone could not say which one was typed.
export function edit(old: string, now: string, caret: number): [number, number, string] {
  const tail = now.length - caret;
  if (tail >= 0 && tail <= old.length && old.endsWith(now.slice(caret))) {
    let a = 0;
    const most = Math.min(caret, old.length - tail);
    while (a < most && old[a] === now[a]) a++;
    return [a, old.length - tail, now.slice(a, caret)];
  }
  return changed(old, now);
}

// changed is the least edit that makes now out of old: the letters both
// start with and end with stay, and what is between them is replaced.
export function changed(old: string, now: string): [number, number, string] {
  let a = 0;
  while (a < old.length && a < now.length && old[a] === now[a]) a++;
  let s = 0;
  while (s < old.length - a && s < now.length - a && old[old.length - 1 - s] === now[now.length - 1 - s]) s++;
  return [a, old.length - s, now.slice(a, now.length - s)];
}
