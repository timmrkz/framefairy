// A test on Vitest's fake clock passes.
import { expect, test, vi } from "vitest";

test("a fake clock", () => {
  vi.useFakeTimers();
  let fired: boolean = false;
  queueMicrotask(() => (fired = true));
  vi.advanceTimersByTime(100);
  expect(fired).toBe(true);
  vi.useRealTimers();
});
