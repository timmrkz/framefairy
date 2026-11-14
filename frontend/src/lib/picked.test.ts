import { describe, expect, test } from "vitest";

import { keyOnPick, type PickKey } from "./picked";

const press = (key: string, mods: Partial<PickKey> = {}): PickKey => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  repeat: false,
  ...mods,
});
const can = () => true;

// The keys pressed in turn on the clip timeline, from a block picked,
// and the blocks they removed.
function removedBy(picked: number | null, keys: PickKey[], canRemove = can): number[] {
  const removed: number[] = [];
  for (const key of keys) {
    const next = keyOnPick(picked, key, canRemove);
    picked = next.picked;
    if (next.remove !== null) removed.push(next.remove);
  }
  return removed;
}

describe("a caption block picked on the clip timeline", () => {
  test("is removed by either delete key, once", () => {
    expect(removedBy(13.9, [press("Backspace")])).toEqual([13.9]);
    expect(removedBy(13.9, [press("Delete")])).toEqual([13.9]);
    expect(removedBy(13.9, [press("Backspace"), press("Backspace")])).toEqual([13.9]);
  });

  test("is let go of by any other key, and delete after it removes no block", () => {
    for (const key of [press("Escape"), press("ArrowRight"), press("ArrowLeft", { shiftKey: true }), press("Shift"), press(" "), press("a")]) {
      expect(keyOnPick(13.9, key, can)).toEqual({ picked: null, remove: null });
      expect(removedBy(13.9, [key, press("Backspace")])).toEqual([]);
    }
  });

  test("stays picked through a delete held down or with a modifier, which removes nothing", () => {
    for (const mods of [{ metaKey: true }, { ctrlKey: true }, { altKey: true }, { shiftKey: true }, { repeat: true }]) {
      expect(keyOnPick(13.9, press("Backspace", mods), can)).toEqual({ picked: 13.9, remove: null });
    }
    expect(removedBy(13.9, [press("Backspace", { metaKey: true }), press("Backspace")])).toEqual([13.9]);
  });

  test("stays picked while it cannot be removed, and is not asked about for other keys", () => {
    let asked = 0;
    const cannot = () => {
      asked++;
      return false;
    };
    expect(keyOnPick(13.9, press("Delete"), cannot)).toEqual({ picked: 13.9, remove: null });
    expect(keyOnPick(13.9, press("Escape"), cannot)).toEqual({ picked: null, remove: null });
    expect(keyOnPick(null, press("Delete"), cannot)).toEqual({ picked: null, remove: null });
    expect(asked).toBe(1);
  });
});
