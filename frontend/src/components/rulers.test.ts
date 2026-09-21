import { describe, expect, it } from "vitest";

// The components are read as text rather than rendered. This is a rule
// about how they are written, and Vite hands over the source itself, so
// the test needs no browser and no reading of files.
const files = import.meta.glob("./*.svelte", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

// The times on a track are the one thing nothing may cover.
//
// Both tracks draw a ruler: a line down the track and the time at the top
// of it. The line has to go under what is drawn on the track, because a
// minute that falls on the edge of the window on the range picker would
// otherwise paint over half of its border. The time has to go over
// everything, because it says where you are.
//
// Those two cannot be one element. An element with a z-index makes a
// stacking context, so a time written inside its line is held at the
// line's level however high its own z-index is. That is exactly how the
// times came to disappear under a window drawn over a stretch that had
// already been searched: the line was put under the window and took its
// time down with it.
//
// So this is a rule about shape, not only about numbers, and it is checked
// as both.

function source(name: string): string {
  const file = files[`./${name}.svelte`];
  expect(file, `${name}.svelte is there to read`).toBeTypeOf("string");
  return file;
}

const styleOf = (file: string) => {
  const at = file.lastIndexOf("<style>");
  expect(at, "the component has a style block").toBeGreaterThan(-1);
  return file.slice(at);
};

const markupOf = (file: string) => file.slice(0, file.lastIndexOf("<style>"));

// The z-index a rule sets for exactly this selector, ignoring rules that
// only narrow it, like .track.locked .window.
function layerOf(css: string, selector: string): number {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const rule = new RegExp(`(^|,)\\s*${escaped}\\s*(,[^{]*)?\\{([^}]*)\\}`, "m");
  const found = rule.exec(css);
  expect(found, `${selector} has a rule of its own`).not.toBeNull();
  const z = /z-index:\s*(-?\d+)/.exec(found![3]);
  expect(z, `${selector} says which layer it is in`).not.toBeNull();
  return Number(z![1]);
}

describe("the ruler on the range picker", () => {
  const file = source("RangeWindow");
  const css = styleOf(file);

  it("puts the line under the window and the time over it", () => {
    const line = layerOf(css, ".tick");
    const window = layerOf(css, ".window");
    const time = layerOf(css, ".time");
    expect(line, "the ruler line goes under the window, or it paints over its border")
      .toBeLessThan(window);
    expect(time, "the time goes over the window, or a searched stretch swallows it")
      .toBeGreaterThan(window);
  });

  it("keeps the time out of the line, so no stacking context can trap it", () => {
    const marks = [...markupOf(file).matchAll(/<div class="tick"[^>]*>([\s\S]*?)<\/div>/g)];
    expect(marks.length, "the ruler line is in the markup").toBeGreaterThan(0);
    for (const mark of marks) {
      expect(mark[1].trim(), "the ruler line holds nothing, the time is its own element").toBe("");
    }
  });

  it("draws every time it has as its own element", () => {
    const markup = markupOf(file);
    expect(markup).toMatch(/<span class="num time"/);
    expect(markup, "nothing writes a time inside the line any more")
      .not.toMatch(/class="tick"[^>]*>\s*\{?#?if/);
  });
});

describe("the ruler on the clip timeline", () => {
  const file = source("ClipTimeline");
  const css = styleOf(file);

  it("puts the time over everything drawn on the track", () => {
    const time = layerOf(css, ".time");
    for (const over of [".tick", ".piece", ".asleep"]) {
      const rule = new RegExp(`(^|,)\\s*\\${over}\\s*(,[^{]*)?\\{([^}]*)\\}`, "m");
      const found = rule.exec(css);
      if (!found) continue;
      const z = /z-index:\s*(-?\d+)/.exec(found[3]);
      const level = z ? Number(z[1]) : 0;
      expect(time, `the time goes over ${over}`).toBeGreaterThan(level);
    }
  });

  it("keeps the time out of the line, the same as the range picker", () => {
    const marks = [...markupOf(file).matchAll(/<div class="tick"[^>]*>([\s\S]*?)<\/div>/g)];
    expect(marks.length, "the ruler line is in the markup").toBeGreaterThan(0);
    for (const mark of marks) {
      expect(mark[1].trim(), "the ruler line holds nothing, the time is its own element").toBe("");
    }
  });
});

describe("the two rulers", () => {
  it("are the same thing, so they are written the same way", () => {
    for (const name of ["RangeWindow", "ClipTimeline"]) {
      const css = styleOf(source(name));
      expect(layerOf(css, ".time"), `${name} puts its times in the same layer`).toBe(4);
    }
  });
});
