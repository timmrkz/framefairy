import { describe, expect, test } from "vitest";
import { landing, type Key } from "./caret";

const key = (name: string, mods: Partial<Key> = {}): Key => ({ key: name, shiftKey: false, altKey: false, metaKey: false, ctrlKey: false, ...mods });
const text = "one two\nthree four\nfive";

describe("where a key takes the cursor", () => {
  test("the arrows up and down go a line from the cursor, the side of a selection they go to", () => {
    expect(landing(text, 10, 10, false, key("ArrowDown"))).toEqual([10, 1]);
    expect(landing(text, 10, 10, false, key("ArrowUp"))).toEqual([10, -1]);
    expect(landing(text, 2, 12, false, key("ArrowUp"))).toEqual([2, -1]);
    expect(landing(text, 2, 12, false, key("ArrowDown"))).toEqual([12, 1]);
  });

  test("with Shift, from the end of the selection that moves", () => {
    expect(landing(text, 2, 12, true, key("ArrowDown", { shiftKey: true }))).toEqual([2, 1]);
    expect(landing(text, 2, 12, false, key("ArrowUp", { shiftKey: true }))).toEqual([12, -1]);
  });

  test("Cmd with an arrow up or down goes to the start or the end of the text", () => {
    expect(landing(text, 10, 10, false, key("ArrowUp", { metaKey: true }))).toEqual([0, 0]);
    expect(landing(text, 10, 10, false, key("ArrowDown", { metaKey: true }))).toEqual([text.length, 0]);
  });

  test("Option with an arrow up or down goes to the start or the end of the paragraph, then the next", () => {
    expect(landing(text, 12, 12, false, key("ArrowUp", { altKey: true }))).toEqual([8, 0]);
    expect(landing(text, 8, 8, false, key("ArrowUp", { altKey: true }))).toEqual([0, 0]);
    expect(landing(text, 10, 10, false, key("ArrowDown", { altKey: true }))).toEqual([18, 0]);
    expect(landing(text, 18, 18, false, key("ArrowDown", { altKey: true }))).toEqual([text.length, 0]);
  });

  test("the arrows left and right go a letter, a word with Option, and stay on the line with Cmd", () => {
    expect(landing(text, 8, 8, false, key("ArrowLeft"))).toEqual([7, 0]);
    expect(landing(text, 7, 7, false, key("ArrowRight"))).toEqual([8, 0]);
    expect(landing(text, 0, 0, false, key("ArrowLeft"))).toEqual([0, 0]);
    expect(landing(text, 7, 7, false, key("ArrowRight", { altKey: true }))).toEqual([13, 0]);
    expect(landing(text, 13, 13, false, key("ArrowLeft", { altKey: true }))).toEqual([8, 0]);
    expect(landing(text, 10, 10, false, key("ArrowLeft", { metaKey: true }))).toEqual([10, 0]);
    // A selection collapses to the side the key goes to.
    expect(landing(text, 2, 12, false, key("ArrowRight"))).toEqual([12, 0]);
  });

  test("Ctrl+N, Ctrl+P, Ctrl+F and Ctrl+B move like the arrows, as in every Mac text field", () => {
    expect(landing(text, 10, 10, false, key("n", { ctrlKey: true }))).toEqual([10, 1]);
    expect(landing(text, 10, 10, false, key("p", { ctrlKey: true }))).toEqual([10, -1]);
    expect(landing(text, 7, 7, false, key("f", { ctrlKey: true }))).toEqual([8, 0]);
    expect(landing(text, 7, 7, false, key("b", { ctrlKey: true }))).toEqual([6, 0]);
    expect(landing(text, 7, 7, false, key("a", { ctrlKey: true }))).toBeNull();
  });

  test("a key that moves no cursor is left to the browser", () => {
    expect(landing(text, 3, 3, false, key("a"))).toBeNull();
    expect(landing(text, 3, 3, false, key("PageDown"))).toBeNull();
    expect(landing(text, 3, 3, false, key("ArrowDown", { ctrlKey: true }))).toBeNull();
  });
});
