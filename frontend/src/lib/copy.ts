// Copy, Cmd+C, of cards picked in a list, through the web view's own copy:
// Copy in the Edit menu is the system's, and a page says what it copies by
// answering the copy event. A page with nothing chosen on it has Copy
// greyed out, so the list answers beforecopy too, which is how WebKit asks
// whether the page has something to copy. Nothing here reads the
// clipboard.

// Whether Copy belongs to text chosen on the page or in a field, which the
// web view copies itself. A field that cannot say what is chosen in it, a
// number field, keeps it too.
export function textChosen(): boolean {
  const on = document.activeElement;
  if (on instanceof HTMLInputElement || on instanceof HTMLTextAreaElement) {
    let start: number | null = null;
    let end: number | null = null;
    try {
      start = on.selectionStart;
      end = on.selectionEnd;
    } catch {
      return true;
    }
    if (start === null || end === null || end > start) return true;
  }
  const chosen = window.getSelection();
  return !!chosen && !chosen.isCollapsed && chosen.toString() !== "";
}

// Answers Copy with the text what says, while the text is not empty and
// nothing is chosen on the page. Gives back the function that stops it.
export function copies(what: () => string): () => void {
  const ask = (e: Event) => {
    if (!textChosen() && what()) e.preventDefault();
  };
  const copy = (e: ClipboardEvent) => {
    if (textChosen() || !e.clipboardData) return;
    const text = what();
    if (!text) return;
    e.clipboardData.setData("text/plain", text);
    e.preventDefault();
  };
  document.addEventListener("beforecopy", ask);
  document.addEventListener("copy", copy);
  return () => {
    document.removeEventListener("beforecopy", ask);
    document.removeEventListener("copy", copy);
  };
}

// The cards picked: from the card the first Shift and click started at to
// the card clicked last. The first starts at the current card, or, where
// none is current, the way none is among the answers to a search, at the
// card clicked, which a second Shift and click then picks on from.
export type Picked = { from: string; to: string };
export function pick(was: Picked | null, current: string | null, clicked: string): Picked {
  return { from: was?.from ?? current ?? clicked, to: clicked };
}

// The keys from one card to another, both in, in the order of the list,
// the way Shift and a click pick files in Finder. A card from that is no
// longer in the list picks only the card clicked, and a card clicked that
// is no longer in it picks none.
export function between(keys: string[], from: string, to: string): string[] {
  const b = keys.indexOf(to);
  if (b < 0) return [];
  const a = keys.indexOf(from);
  if (a < 0) return [to];
  return keys.slice(Math.min(a, b), Math.max(a, b) + 1);
}

// The words of cards, as one running text, card by card as they stand.
export function joined(texts: string[]): string {
  return texts
    .map((t) => t.trim())
    .filter(Boolean)
    .join(" ");
}
