import { describe, expect, test } from "vitest";
import { Undo, type Change } from "./undo";

// A text and its undo, the way the report's field keeps them: what the
// person types is a step, and what comes in by itself only moves them.
class Text {
  text = "";
  u = new Undo();
  type(at: number, put: string, to = at) {
    const removed = this.text.slice(at, to);
    this.text = this.text.slice(0, at) + put + this.text.slice(to);
    this.u.did(at, to, removed, put);
  }
  comes(at: number, put: string, to = at) {
    this.text = this.text.slice(0, at) + put + this.text.slice(to);
    this.u.moved(at, to, put.length);
  }
  apply(c: Change | null) {
    if (c) this.text = this.text.slice(0, c[0]) + c[2] + this.text.slice(c[1]);
    return this.text;
  }
  undo() {
    return this.apply(this.u.undo(this.text));
  }
  redo() {
    return this.apply(this.u.redo(this.text));
  }
}

describe("undo in the report's field", () => {
  test("takes back a run of typing as one step, and redo brings it back", () => {
    const t = new Text();
    for (const [i, c] of [..."abc"].entries()) t.type(i, c);
    t.type(3, "\n");
    t.type(4, "d");
    expect(t.undo()).toBe("abc\n");
    expect(t.undo()).toBe("abc");
    expect(t.undo()).toBe("");
    expect(t.redo()).toBe("abc");
    expect(t.redo()).toBe("abc\n");
    expect(t.redo()).toBe("abc\nd");
  });

  test("Backspace on from the last one is one step", () => {
    const t = new Text();
    t.type(0, "hello");
    t.type(4, "", 5);
    t.type(3, "", 4);
    expect(t.text).toBe("hel");
    expect(t.undo()).toBe("hello");
  });

  test("a letter put together from more than one key is one step with the mark it began with", () => {
    const t = new Text();
    t.type(0, "Gr");
    t.type(2, "¨");
    t.type(2, "ü", 3);
    t.type(3, "n");
    expect(t.text).toBe("Grün");
    expect(t.undo()).toBe("");
  });

  test("words that come in by themselves are no step, and the steps keep their own letters", () => {
    // In Chromium and WebKit the browser's own undo took out the wrong
    // letters here, and redo put "abc" in twice.
    const t = new Text();
    t.type(0, "abc");
    t.comes(3, " said");
    t.type(8, "X");
    expect(t.undo()).toBe("abc said");
    expect(t.undo()).toBe(" said");
    expect(t.redo()).toBe("abc said");
    expect(t.redo()).toBe("abc saidX");
  });

  test("a step moves along with words that come in front of it", () => {
    const t = new Text();
    t.type(0, "one two");
    t.u.begin();
    t.type(3, "X");
    t.comes(0, ">> ");
    expect(t.undo()).toBe(">> one two");
    expect(t.redo()).toBe(">> oneX two");
  });

  test("while dictating, undo goes back no further than the dictation, and after it takes it all back as one step", () => {
    const t = new Text();
    t.type(0, "It froze.");
    const before = t.text;
    t.u.begin();
    t.comes(9, " one");
    t.type(13, " X");
    t.comes(15, " two");
    expect(t.undo()).toBe("It froze. one two");
    expect(t.undo()).toBe("It froze. one two");
    t.u.end(before, t.text);
    expect(t.undo()).toBe("It froze.");
    expect(t.undo()).toBe("");
    expect(t.redo()).toBe("It froze.");
    expect(t.redo()).toBe("It froze. one two");
  });

  test("a step whose letters were changed by something else is forgotten, not undone over other letters", () => {
    const t = new Text();
    t.u.begin();
    t.type(0, "abc");
    t.comes(1, "Z", 2);
    expect(t.undo()).toBe("aZc");
    expect(t.redo()).toBe("aZc");
  });
});
