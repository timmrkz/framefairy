import { describe, expect, test } from "vitest";
import { revealed, type Scrolled } from "./reveal";

// The report's field: ten lines of 19 px with 8 px of padding over and
// under them, holding thirty lines.
const lines = 30;
const field = (top: number): Scrolled => ({ top, height: 10 * 19 + 16, end: lines * 19 + 16, pad: 8, foot: 8, line: 19 });
const end = lines * 19 + 16 - (10 * 19 + 16);

// The cursor on line k, counted from 0, in a box shorter than its line,
// shifted from its middle by off, the way an italic face sits in it.
const cursor = (k: number, off = 0): [number, number] => [8 + 19 * k + 2 + off, 8 + 19 * k + 17 + off];

describe("the cursor kept in view", () => {
  test("going down a line at a time, the field scrolls a line at a time and stops at its end", () => {
    let top = 0;
    const seen: number[] = [];
    for (let k = 0; k < lines; k++) {
      top = revealed(field(top), ...cursor(k));
      seen.push(top);
    }
    const want = Array.from({ length: lines }, (_, k) => Math.max(0, (k - 9) * 19));
    want[lines - 1] = end;
    expect(seen).toEqual(want);
    // Never past its end, however often the last line is asked for.
    for (let i = 0; i < 3; i++) top = revealed(field(top), ...cursor(lines - 1));
    expect(top).toBe(end);
  });

  test("going up a line at a time, the field scrolls a line at a time and stops at its top", () => {
    let top = end;
    const seen: number[] = [];
    for (let k = lines - 1; k >= 0; k--) {
      top = revealed(field(top), ...cursor(k));
      seen.push(top);
    }
    const want = Array.from({ length: lines }, (_, i) => Math.min(end, (lines - 1 - i) * 19));
    expect(seen).toEqual(want);
  });

  test("leaves the field where it is while the cursor's line is in view", () => {
    for (let k = 3; k < 13; k++) expect(revealed(field(57), ...cursor(k))).toBe(57);
  });

  test("lands on the same pixel wherever the cursor stands in its line", () => {
    for (const off of [-2, -1, 1, 2]) {
      let top = 0;
      for (let k = 0; k < lines; k++) top = revealed(field(top), ...cursor(k, off));
      expect([off, top]).toEqual([off, end]);
      expect(revealed(field(0), ...cursor(10, off))).toBe(19);
    }
  });
});
