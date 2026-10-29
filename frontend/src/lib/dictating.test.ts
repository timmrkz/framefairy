import { describe, expect, test } from "vitest";
import { Dictating, edit, type Span } from "./dictating";

// A field the way the report has it: a textarea's text and selection,
// and a dictation in it. type and press do what the browser does with a
// key, and hear what the window does with what the Go side says.
class Field {
  d: Dictating;
  sel: Span;
  told: unknown[] = [];
  listening = true;
  constructor(text: string, at = text.length) {
    this.d = new Dictating(text, at);
    this.sel = [at, at];
  }
  get text() {
    return this.d.text;
  }
  // Typing, as the window puts it in: the space after a word that was
  // said, then the browser's own edit, which the dictation follows.
  type(s: string) {
    const put = this.d.typing(this.sel, s);
    this.change(this.sel[0], this.sel[1], put);
  }
  backspace() {
    const [a, b] = this.sel;
    this.change(a === b ? a - 1 : a, b, "");
  }
  change(at: number, to: number, put: string) {
    this.told.push(...this.d.edited(at, to, put, this.listening));
    this.sel = [at + put.length, at + put.length];
  }
  hear(said: string[], hearing: string, last = false) {
    this.sel = this.d.hear(said, hearing, this.sel, last);
  }
  // The cursor in the text, at the first letter of what.
  at(what: string, after = false) {
    const i = this.text.indexOf(what);
    this.sel = [i + (after ? what.length : 0), i + (after ? what.length : 0)];
  }
  get cursor() {
    return this.sel[0];
  }
}

describe("words said while dictating", () => {
  test("go in where the cursor was, a space from the words around them, and the cursor stays after them", () => {
    const f = new Field("Hello world", 5);
    f.hear([], "big");
    expect(f.text).toBe("Hello big world");
    expect(f.cursor).toBe("Hello big".length);
    f.hear([], "big red");
    f.hear(["big red"], "");
    f.hear(["big red"], "car");
    expect(f.text).toBe("Hello big red car world");
    expect(f.cursor).toBe("Hello big red car".length);
  });

  test("start a line of their own after a new line, with no space in front", () => {
    const f = new Field("Before\n");
    f.hear(["first"], "second");
    expect(f.text).toBe("Before\nfirst second");
  });

  test("leave a cursor somewhere else by the letters it was by", () => {
    const f = new Field("ab cd", 2);
    f.sel = [1, 1];
    f.hear([], "one");
    f.hear(["one"], "two");
    expect(f.text).toBe("ab one two cd");
    expect(f.cursor).toBe(1);
    // A selection after them moves with the letters it has.
    f.at("cd");
    f.sel = [f.cursor, f.cursor + 2];
    f.hear(["one", "two"], "three");
    expect(f.text.slice(...f.sel)).toBe("cd");
  });

  test("the same news twice, as in a silence, changes nothing at all", () => {
    const f = new Field("Text ");
    f.hear(["one two"], "three");
    const [text, sel] = [f.text, [...f.sel]];
    for (let i = 0; i < 20; i++) f.hear(["one two"], "three");
    expect(f.text).toBe(text);
    expect(f.sel).toEqual(sel);
    expect(f.told).toEqual([]);
  });
});

describe("typing while dictating", () => {
  test("a new line right after the words keeps what was said before it above it, issue #573", () => {
    const f = new Field("");
    f.hear([], "one two");
    f.type("\n");
    expect(f.told).toEqual([{ cut: 0 }]);
    // The speech model writes down the rest of what came before the new
    // line, and what is said after it goes after it.
    f.hear([], "four");
    expect(f.text).toBe("one two\nfour");
    f.hear(["one two three"], "four five");
    expect(f.text).toBe("one two three\nfour five");
    expect(f.cursor).toBe(f.text.length);
  });

  test("letters typed at the end stay after what was said before them and before what is said after, issue #584", () => {
    const f = new Field("");
    f.hear([], "hello");
    f.type("X");
    f.type("Y");
    expect(f.text).toBe("hello XY");
    f.hear(["hello there"], "again");
    expect(f.text).toBe("hello there XY again");
    // A word typed after words said gets a space, one typed after typing none.
    f.type("Z");
    expect(f.text).toBe("hello there XY again Z");
    expect(f.told).toEqual([{ cut: 0 }, { cut: 1 }]);
  });

  test("a full stop typed right after a word that was said has no space in front", () => {
    const f = new Field("");
    f.hear(["done"], "");
    f.type(".");
    expect(f.text).toBe("done.");
  });

  test("Backspace in the words being heard keeps them as they stand, and the Go side is told", () => {
    const f = new Field("");
    f.hear([], "hello world");
    f.backspace();
    expect(f.told).toEqual([{ settle: 0, words: "hello world" }]);
    expect(f.text).toBe("hello worl");
    // What the Go side said before it was told shows nothing twice.
    f.hear(["hello world"], "");
    expect(f.text).toBe("hello worl");
    f.hear(["hello world"], "next");
    expect(f.text).toBe("hello worl next");
  });

  test("a cursor put in the words being heard settles them, so they stay under it", () => {
    const f = new Field("");
    f.hear([], "hello world");
    f.sel = [3, 3];
    expect(f.d.inside(f.sel)).toBe(true);
    expect(f.d.settle()).toEqual({ settle: 0, words: "hello world" });
    f.hear(["hello world"], "more");
    expect(f.text).toBe("hello world more");
    expect(f.cursor).toBe(3);
    // Right before or right after them is not in them.
    expect(f.d.inside([f.d.heard[0], f.d.heard[0]])).toBe(false);
    expect(f.d.inside([f.d.heard[1], f.d.heard[1]])).toBe(false);
  });

  test("typing in a place whose words are still to come makes them the person's", () => {
    const f = new Field("");
    f.hear([], "one two");
    f.type("!");
    f.at("two", true);
    f.backspace();
    expect(f.text).toBe("one tw!");
    f.hear(["one two three"], "");
    expect(f.text).toBe("one tw!");
  });

  test("typing before the dictation moves everything in it along", () => {
    const f = new Field("ab", 2);
    f.hear(["one"], "two");
    f.sel = [0, 0];
    f.type("Q");
    f.hear(["one", "two"], "three");
    expect(f.text).toBe("Qab one two three");
  });
});

describe("the last words after the dictation ends", () => {
  test("go before what is typed right after them while they are written down", () => {
    const f = new Field("");
    f.hear([], "hello");
    f.listening = false;
    f.type("X");
    f.hear(["hello world", "and more"], "", true);
    expect(f.text).toBe("hello world and more X");
    expect(f.cursor).toBe(f.text.length);
  });

  test("are left out once the person typed in them", () => {
    const f = new Field("");
    f.hear([], "hello");
    f.listening = false;
    f.backspace();
    f.hear(["hello world"], "", true);
    expect(f.text).toBe("hell");
  });

  test("come in at the end with nothing typed", () => {
    const f = new Field("Before ");
    f.hear(["one"], "two");
    f.listening = false;
    f.hear(["one", "two three"], "", true);
    expect(f.text).toBe("Before one two three");
    expect(f.d.end).toBe(f.text.length);
  });
});

describe("the edit the browser made", () => {
  test("is found at the cursor in a run of one letter", () => {
    expect(edit("xay", "xaay", 3)).toEqual([2, 2, "a"]);
    expect(edit("xaay", "xay", 2)).toEqual([2, 3, ""]);
  });
  test("is a selection replaced by what was pasted", () => {
    expect(edit("one two three", "one 2 three", 5)).toEqual([4, 7, "2"]);
  });
  test("is found by comparing the texts when the cursor says nothing", () => {
    expect(edit("abc", "aXc", 0)).toEqual([1, 2, "X"]);
  });
});
