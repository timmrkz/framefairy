// Whether a name answers what was typed in the search field over a list.
//
// It answers when it holds what was typed, or when the letters typed come
// in order and each one either starts a word or follows the letter before
// it, so amtyp finds American Typewriter and brnar Bridge Narrow. A letter
// picked out of the middle of a word does not count: orb found Kohinoor
// Bangla and Chalkboard, from an o and an r somewhere inside them, which
// no one typing orb was looking for. Spaces typed count for nothing, and
// neither does the case.
export function matches(name: string, typed: string): boolean {
  const want = typed.toLowerCase().replace(/\s+/g, "");
  if (!want) return true;
  const have = name.toLowerCase();
  if (have.includes(typed.trim().toLowerCase().replace(/\s+/g, " "))) return true;
  const starts = [...name].map((c, i) => {
    if (i === 0) return true;
    const before = name[i - 1];
    if (!/[\p{L}\p{N}]/u.test(before)) return true;
    // A capital after a small letter starts a word, as in FreeSerif, and
    // so does a digit after a letter, as in Bodoni 72 written Bodoni72.
    if (/\p{Lu}/u.test(c) && /\p{Ll}/u.test(before)) return true;
    return /\p{N}/u.test(c) !== /\p{N}/u.test(before);
  });
  // Where each typed letter may stand, worked out once per place, so a
  // name full of the same letter costs no more than any other.
  const tried = new Set<string>();
  const from = (letter: number, last: number): boolean => {
    if (letter === want.length) return true;
    const key = `${letter} ${last}`;
    if (tried.has(key)) return false;
    tried.add(key);
    for (let at = last + 1; at < have.length; at++) {
      if (have[at] !== want[letter]) continue;
      if ((at === last + 1 || starts[at]) && from(letter + 1, at)) return true;
    }
    return false;
  };
  return from(0, -1);
}
