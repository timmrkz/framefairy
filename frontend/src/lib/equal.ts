// Whether two values read from JSON hold the same: the same numbers,
// strings, true, false and null, in arrays of the same length and objects
// with the same keys. It compares what Go sent twice without making a
// string of either, which made two strings of every clip on every state
// of a video only to throw them away, issue #805.
export function equal(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (typeof a !== "object" || typeof b !== "object" || a === null || b === null) return false;
  if (Array.isArray(a)) {
    if (!Array.isArray(b) || a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) if (!equal(a[i], b[i])) return false;
    return true;
  }
  if (Array.isArray(b)) return false;
  const x = a as Record<string, unknown>;
  const y = b as Record<string, unknown>;
  // A key that holds undefined is no key, the way JSON writes it.
  let n = 0;
  for (const k in x) {
    if (x[k] === undefined) continue;
    n++;
    if (!equal(x[k], y[k])) return false;
  }
  for (const k in y) if (y[k] !== undefined) n--;
  return n === 0;
}
