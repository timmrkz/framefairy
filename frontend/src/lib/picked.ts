// What a key does to the caption block picked on the clip timeline. A
// block clicked is picked, the way a file is in the Finder, and delete
// removes it: the key marked delete on the Mac, which is Backspace to the
// browser, and the forward delete key alike. Any other key lets the pick
// go, and does what it does. A block clicked only to put the playhead
// there, and then Shift and an arrow to walk to a word, is work on that
// word now: delete was meant for the word, and it removed the block still
// picked as well.
//
// A caption is known by the moment its first word starts, see
// CaptionCue.first.

export type PickKey = Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey" | "altKey" | "shiftKey" | "repeat">;

// The block picked after the key, and the block the key removes, if any.
// canRemove says whether a caption can be removed now: not while the clip
// is locked, a field or a box has the keyboard, or the clip timeline is
// asleep. It is asked only for a delete with a block picked.
export function keyOnPick(
  picked: number | null,
  key: PickKey,
  canRemove: () => boolean,
): { picked: number | null; remove: number | null } {
  if (key.key !== "Backspace" && key.key !== "Delete") return { picked: null, remove: null };
  // Held down, or with a modifier, delete is some other shortcut, and a
  // second press while the first removal is on its way removes no second
  // block.
  if (key.metaKey || key.ctrlKey || key.altKey || key.shiftKey || key.repeat) return { picked, remove: null };
  if (picked === null || !canRemove()) return { picked, remove: null };
  return { picked: null, remove: picked };
}
