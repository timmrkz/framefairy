import { describe, expect, test } from "vitest";
import { heard } from "./heard";

describe("heard", () => {
  test("follows the output timestamp, and the clock less its latency without one", () => {
    expect(heard({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1016).at).toBeCloseTo(10.016);
    expect(heard(null, 10.05, 0.03, 1016).at).toBeCloseTo(10.02);
    expect(heard({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1016).source).toBe("stamp");
    expect(heard(null, 10.05, 0.03, 1016).source).toBe("plain");
    expect(heard({ contextTime: 0, performanceTime: 0 }, 0.5, 0.1, 5).at).toBeCloseTo(0.4);
  });
  test("is never ahead of the clock, so a stamp from before a pause cannot carry play on", () => {
    // Held at 10.05 for 700 ms: the stamp would say 10.716.
    expect(heard({ contextTime: 10, performanceTime: 1000 }, 10.05, 0.03, 1716)).toEqual({ at: 10.05, source: "held" });
  });
  test("leaves a stamp on another clock for the clock less its latency", () => {
    expect(heard({ contextTime: 9.17, performanceTime: 21540 }, 9.186, 0.003, 8000).at).toBeCloseTo(9.183);
  });
});
