// Where a field of text is scrolled to keep its cursor in view, the way
// a text field keeps its own: the least scroll that shows the whole line
// the cursor is on, inside the field's padding, and on the last line all
// the way to the end, the padding under it included. A cursor whose line
// is in view leaves the field where it is, so moving it a line at a time
// scrolls the field a line at a time. Tim saw the report's field go a few
// pixels past its last line and back, and jump by several lines, while he
// moved up and down with the arrow keys (29 October).

// A field and where it is scrolled to, in pixels: top is its scrollTop,
// height its clientHeight, end its scrollHeight, pad and foot its padding
// over and under the text, and line the height of a line.
export type Scrolled = { top: number; height: number; end: number; pad: number; foot: number; line: number };

// revealed is where the field is scrolled to with the cursor's box from
// over to under, measured from the top of what the field holds. The
// cursor is shorter than its line and stands anywhere in it, a pixel or
// two either way in a slanted face, so its line is the one its middle is
// in, and the field lands on whole lines.
export function revealed(f: Scrolled, over: number, under: number): number {
  const at = f.pad + Math.floor(((over + under) / 2 - f.pad) / f.line) * f.line;
  // On the last line, nothing is left under it to show but the end.
  const bottom = at + f.line + f.foot > f.end - f.line / 2 ? f.end : at + f.line + f.foot;
  const head = Math.max(at - f.pad, 0);
  if (bottom > f.top + f.height) return Math.round(Math.min(bottom, f.end) - f.height);
  if (head < f.top) return Math.round(head);
  return f.top;
}
