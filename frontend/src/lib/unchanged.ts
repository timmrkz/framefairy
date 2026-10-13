// A list read again, with every item the read says is as it was kept as
// the very same item. What depends on an item, the captions of the clip on
// screen above all, is worked out again when the item is a new one, so an
// item that is new only because it was read again sets all of that going
// for nothing. Items are told apart by their key and compared by what they
// hold.
export function unchanged<T extends { key: string }>(was: T[], read: T[]): T[] {
  if (!was.length) return read;
  const before = new Map(was.map((c) => [c.key, c]));
  return read.map((c) => {
    const old = before.get(c.key);
    return old && JSON.stringify(old) === JSON.stringify(c) ? old : c;
  });
}
