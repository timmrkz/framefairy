import { describe, expect, test } from "vitest";
import cases from "./suggest.cases.json";
import { suggestedCount, suggestedWindow } from "./suggest";

// The same table the engine is held to, in engine/suggest_test.go.
describe("the window and the target follow the episode", () => {
  test.each(cases.windows)("an episode of $duration s is cut into windows of $window s", (c) => {
    expect(suggestedWindow(c.duration)).toBeCloseTo(c.window, 0);
  });

  test.each(cases.counts)("$window s at $least to $most s looks for $count clips", (c) => {
    expect(suggestedCount(c.window, c.least, c.most)).toBe(c.count);
  });

  test("the windows divide the episode evenly and are never under ten minutes", () => {
    for (let d = 1; d < 6 * 3600; d += 37) {
      const w = suggestedWindow(d);
      const n = d / w;
      expect(Math.abs(n - Math.round(n))).toBeLessThan(1e-6);
      if (d > 600) expect(w).toBeGreaterThanOrEqual(600 - 1e-6);
    }
  });
});
