import { describe, expect, test } from "vitest";
import { placeBubble, type Box } from "./bubble";

// A text that takes so many pixels of area: twice as wide, half as tall.
const text = (area: number) => (width: number) => Math.ceil(area / width);

const mark = (x: number, y: number): Box => ({ top: y, bottom: y + 20, left: x, right: x + 20 });

// The whole bubble inside the app, and not over the mark that opened it.
function readable(at: ReturnType<typeof placeBubble>, height: number, app: { width: number; height: number }, m: Box) {
  expect(at.scroll).toBe(false);
  expect(at.top).toBeGreaterThanOrEqual(0);
  expect(at.left).toBeGreaterThanOrEqual(0);
  expect(at.top + height).toBeLessThanOrEqual(app.height);
  expect(at.left + at.width).toBeLessThanOrEqual(app.width);
  const over = at.left < m.right && at.left + at.width > m.left && at.top < m.bottom && at.top + height > m.top;
  expect(over).toBe(false);
}

describe("where an info bubble goes", () => {
  const app = { width: 1500, height: 900 };

  test("a short text hangs under its mark", () => {
    const at = placeBubble(mark(400, 80), app, "left", text(260 * 100));
    expect(at.where).toBe("below");
    expect(at.width).toBe(260);
  });

  test("near the foot of the app it goes over the mark", () => {
    const at = placeBubble(mark(1400, 800), app, "right", text(260 * 300));
    expect(at.where).toBe("above");
    readable(at, 300, app, mark(1400, 800));
  });

  // The clip timeline's: 828 pixels tall at the narrow width, its mark at
  // 700, so it fits neither under nor over. It used to go under anyway and
  // run 635 pixels past the foot of the app.
  test("a text too tall for either side is made wider until it fits", () => {
    const tall = text(260 * 828);
    const m = mark(1460, 700);
    const at = placeBubble(m, app, "right", tall);
    expect(at.where).toBe("above");
    expect(at.width).toBeGreaterThan(260);
    readable(at, tall(at.width), app, m);
  });

  test("with no room over or under at any width it goes beside the mark", () => {
    const small = { width: 1000, height: 560 };
    const tall = text(260 * 828);
    const m = mark(960, 420);
    const at = placeBubble(m, small, "right", tall);
    expect(at.where).toBe("beside");
    readable(at, tall(at.width), small, m);
  });

  test("only an app too small for it at any width scrolls it", () => {
    const tiny = { width: 400, height: 300 };
    const at = placeBubble(mark(360, 140), tiny, "right", text(260 * 2000));
    expect(at.scroll).toBe(true);
    expect(at.left + at.width).toBeLessThanOrEqual(tiny.width);
  });

  // Every mark anywhere in apps of several sizes, with texts of every
  // length the app has: all of it is readable or, only where the app is
  // too small for the text at any width, it scrolls.
  test("every mark in every app shows all of its text", () => {
    for (const size of [{ width: 1000, height: 560 }, { width: 1200, height: 640 }, { width: 1500, height: 900 }, { width: 2400, height: 1300 }]) {
      for (const area of [260 * 100, 260 * 300, 260 * 600, 260 * 828]) {
        for (let x = 10; x < size.width - 30; x += 97) {
          for (let y = 10; y < size.height - 30; y += 53) {
            for (const side of ["left", "right"] as const) {
              const m = mark(x, y);
              const tall = text(area);
              const at = placeBubble(m, size, side, tall);
              if (at.scroll) {
                expect(area / 520).toBeGreaterThan(Math.max(m.top, size.height - m.bottom) - 14);
                continue;
              }
              readable(at, tall(at.width), size, m);
            }
          }
        }
      }
    }
  });
});
