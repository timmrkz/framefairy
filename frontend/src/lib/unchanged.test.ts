import { describe, expect, it } from "vitest";
import { unchanged } from "./unchanged";

describe("unchanged", () => {
  const a = { key: "a", start: 1, cuts: [{ at: 2 }] };
  const b = { key: "b", start: 5, cuts: [] };

  it("keeps an item read again as it was as the very same item", () => {
    const read = [structuredClone(a), structuredClone(b)];
    const now = unchanged([a, b], read);
    expect(now[0]).toBe(a);
    expect(now[1]).toBe(b);
  });

  it("takes an item that changed, and one that is new, as read", () => {
    const moved = { ...structuredClone(a), cuts: [{ at: 3 }] };
    const c = { key: "c", start: 9, cuts: [] };
    const now = unchanged([a, b], [moved, structuredClone(b), c]);
    expect(now[0]).toBe(moved);
    expect(now[1]).toBe(b);
    expect(now[2]).toBe(c);
  });

  it("follows the read for order and for what is gone", () => {
    const now = unchanged([a, b], [structuredClone(b)]);
    expect(now).toEqual([b]);
    expect(now[0]).toBe(b);
  });
});
