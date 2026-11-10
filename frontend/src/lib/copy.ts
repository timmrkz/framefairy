// Copy in the Edit menu, Cmd+C. The menu has the key, the way it has Undo,
// so the Go side tells the window and the window says what to copy: the
// text chosen in a field or on the page when there is any, and otherwise
// what the part of the app in front offers, the cards picked in the search
// of what was said. Nothing here reads the clipboard: the text goes to the
// Go side, which puts it there.

// What the part of the app in front would copy, or null. The newest one
// offered is asked first, and one that is not in front says null.
const offers: (() => string | null)[] = [];

// Offers what to copy while it stands, and takes the offer back with the
// function it returns.
export function offerCopy(what: () => string | null): () => void {
  offers.unshift(what);
  return () => {
    const at = offers.indexOf(what);
    if (at >= 0) offers.splice(at, 1);
  };
}

// What Copy copies now, or "" for nothing.
export function toCopy(): string {
  const chosen = chosenText();
  if (chosen) return chosen;
  for (const offer of offers) {
    const text = offer();
    if (text) return text;
  }
  return "";
}

// The text chosen in the field the keyboard is in, or else on the page. A
// field with nothing chosen in it lets the copy go to the page, so a card
// clicked while the keyboard stays in the search field still copies.
function chosenText(): string {
  const on = document.activeElement;
  if (on instanceof HTMLInputElement || on instanceof HTMLTextAreaElement) {
    const [a, b] = [on.selectionStart ?? 0, on.selectionEnd ?? 0];
    // A password field gives nothing away, the way the system's own Copy
    // does not.
    if (b > a && on.type !== "password") return on.value.slice(a, b);
  }
  return window.getSelection()?.toString() ?? "";
}

// The keys from one card to another, both in, in the order of the list,
// the way Shift and a click pick files in Finder. A card from that is no
// longer in the list picks only the card clicked.
export function between(keys: string[], from: string | null, to: string): string[] {
  const b = keys.indexOf(to);
  if (b < 0) return [];
  const a = from === null ? -1 : keys.indexOf(from);
  if (a < 0) return [to];
  return keys.slice(Math.min(a, b), Math.max(a, b) + 1);
}

// The words of cards, as one running text. Two clips over the same words
// each have a card of them, and those words are copied once.
export function joined(cards: { start: number; text: string }[]): string {
  const out: string[] = [];
  let last: { start: number; text: string } | null = null;
  for (const card of cards) {
    const text = card.text.trim();
    if (!text || (last && last.start === card.start && last.text === text)) continue;
    out.push(text);
    last = { start: card.start, text };
  }
  return out.join(" ");
}
