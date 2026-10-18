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

// A sentence's words, each without the marks around it, which are not
// bold, and the sentence in small letters, kept for the sentences of the
// video in front, which are searched again at every letter typed.
type Words = { words: { from: number; to: number; bare: string }[]; have: string };
const known = new Map<string, Words>();
function wordsOf(text: string): Words {
  let w = known.get(text);
  if (w) return w;
  if (known.size > 50000) known.clear();
  w = {
    words: [...text.matchAll(/[\p{L}\p{N}](?:\S*[\p{L}\p{N}])?/gu)].map((m) => ({
      from: m.index,
      to: m.index + m[0].length,
      bare: m[0],
    })),
    have: text.toLowerCase(),
  };
  known.set(text, w);
  return w;
}

// Whether a word answers a word typed, kept for what is typed now: a
// video says the same few thousand words over and over, and asking each
// of them once is what keeps a search of four hours quick enough to type
// into.
let answers: { typed: string; of: { a: string; seen: Map<string, boolean> }[] } = { typed: "", of: [] };

// Where a sentence of the transcript answers what was typed in the search
// in the clip column, as the words to set in bold, from and to in the
// text, or null where it does not answer. A sentence answers when it holds
// what was typed, and the words that hold it are bold. Otherwise every
// word typed has to find a word of the sentence the way a font finds its
// name, see matches, and every word found is bold. Nothing typed answers
// with nothing bold.
export function found(text: string, typed: string): [number, number][] | null {
  const want = typed.trim().toLowerCase().replace(/\s+/g, " ");
  if (!want) return [];
  const { words, have } = wordsOf(text);
  // The words that hold what was typed, walked once, since both the
  // places it is found and the words are in order.
  const held: [number, number][] = [];
  let w = 0;
  for (let at = have.indexOf(want); at >= 0; at = have.indexOf(want, at + want.length)) {
    const end = at + want.length;
    while (w < words.length && words[w].to <= at) w++;
    for (let k = w; k < words.length && words[k].from < end; k++) {
      if (held.at(-1)?.[0] !== words[k].from) held.push([words[k].from, words[k].to]);
    }
  }
  if (held.length) return held;
  if (answers.typed !== want) answers = { typed: want, of: want.split(" ").map((a) => ({ a, seen: new Map() })) };
  const bold = new Array<boolean>(words.length).fill(false);
  for (const { a, seen } of answers.of) {
    let any = false;
    for (let k = 0; k < words.length; k++) {
      const bare = words[k].bare;
      let yes = seen.get(bare);
      if (yes === undefined) seen.set(bare, (yes = matches(bare, a)));
      if (yes) any = bold[k] = true;
    }
    if (!any) return null;
  }
  return words.filter((_, k) => bold[k]).map((w) => [w.from, w.to]);
}
