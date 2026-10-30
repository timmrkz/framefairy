// Where the cursor of a textarea is and where a key takes it, for
// lib/reveal.ts. A textarea does not say where its cursor is drawn, so a
// copy of its text is laid out the way the field lays it out, with a
// letter of no width where the cursor is, and that letter is measured.

// The styles that decide where a line of the field breaks and how tall
// it is.
const laid = [
  "fontFamily",
  "fontSize",
  "fontWeight",
  "fontStyle",
  "fontVariant",
  "fontFeatureSettings",
  "fontKerning",
  "letterSpacing",
  "wordSpacing",
  "lineHeight",
  "textIndent",
  "textTransform",
  "whiteSpace",
  "overflowWrap",
  "wordBreak",
  "tabSize",
  "paddingTop",
  "paddingLeft",
  "paddingRight",
] as const;

// caretBox is the box of the cursor at letter at of field f, from its
// top to its bottom, measured from the top of what the field holds, its
// padding included, the way scrollTop counts.
export function caretBox(f: HTMLTextAreaElement, at: number): [number, number] {
  const cs = getComputedStyle(f);
  const copy = document.createElement("div");
  for (const p of laid) copy.style[p] = cs[p];
  Object.assign(copy.style, { position: "absolute", visibility: "hidden", top: "0", left: "0", boxSizing: "border-box", border: "0", width: `${f.clientWidth}px` });
  const cursor = document.createElement("span");
  cursor.textContent = "​";
  copy.append(f.value.slice(0, at), cursor, f.value.slice(at));
  document.body.append(copy);
  const box: [number, number] = [cursor.offsetTop, cursor.offsetTop + cursor.offsetHeight];
  copy.remove();
  return box;
}

// A key that moves the cursor, as keydown has it.
export type Key = { key: string; shiftKey: boolean; altKey: boolean; metaKey: boolean; ctrlKey: boolean };

// landing is where key k takes the cursor in text, with the selection
// from start to end and back when it runs backwards: a letter, and a
// line up or down from it, or null for a key that moves no cursor. The
// keys are the Mac's: Cmd with an arrow goes to the end of the line or
// the text, Option to the word or the paragraph. A key the page does not
// know, it leaves to the browser.
export function landing(text: string, start: number, end: number, back: boolean, k: Key): [number, number] | null {
  if (k.ctrlKey) return null;
  const up = k.key === "ArrowUp" || k.key === "ArrowLeft";
  // Shift moves the end of the selection that moves, and without Shift a
  // selection collapses to the side the key goes to.
  const from = k.shiftKey ? (back ? start : end) : up ? start : end;
  const ranged = !k.shiftKey && start !== end;
  switch (k.key) {
    case "ArrowUp":
    case "ArrowDown":
      if (k.metaKey) return [up ? 0 : text.length, 0];
      if (k.altKey) return [up ? text.lastIndexOf("\n", from - 2) + 1 : paragraphEnd(text, from), 0];
      return [from, up ? -1 : 1];
    case "ArrowLeft":
    case "ArrowRight":
      if (ranged || k.metaKey) return [from, 0];
      if (k.altKey) return [up ? Math.max(text.slice(0, from).search(/\S+\s*$/), 0) : from + (text.slice(from).match(/^\s*\S+/)?.[0].length ?? 0), 0];
      return [Math.min(Math.max(from + (up ? -1 : 1), 0), text.length), 0];
  }
  return null;
}

// paragraphEnd is where the paragraph at letter at ends, or the next one
// when at is at its end already.
function paragraphEnd(text: string, at: number): number {
  const i = text.indexOf("\n", text[at] === "\n" ? at + 1 : at);
  return i < 0 ? text.length : i;
}
