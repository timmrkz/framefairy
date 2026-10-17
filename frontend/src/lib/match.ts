// Whether a name answers what was typed in the search field over a list.
//
// It answers when it holds what was typed, or when the letters typed come
// in it in order, in pieces. A piece that starts a word is free, so amtyp
// finds American Typewriter and brnar Bridge Narrow. A piece that starts
// in the middle of a word is allowed once for every two letters typed, so
// a letter left out, helvtica for Helvetica, or a few letters from inside
// the name, tms for Times, still find it. What it keeps out is a name
// where every letter typed was picked out of somewhere else in it, which
// is how a long name came up for almost anything. Spaces typed count for
// nothing, and neither does the case.
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
  // The fewest pieces from the middle of a word the rest of the letters
  // need, worked out once per place, so a name full of the same letter
  // costs no more than any other.
  const known = new Map<string, number>();
  const fewest = (letter: number, last: number): number => {
    if (letter === want.length) return 0;
    const key = `${letter} ${last}`;
    const seen = known.get(key);
    if (seen !== undefined) return seen;
    let best = Infinity;
    for (let at = last + 1; at < have.length && best > 0; at++) {
      if (have[at] !== want[letter]) continue;
      const piece = at === last + 1 || starts[at] ? 0 : 1;
      best = Math.min(best, piece + fewest(letter + 1, at));
    }
    known.set(key, best);
    return best;
  };
  return fewest(0, -1) <= Math.ceil(want.length / 2);
}
