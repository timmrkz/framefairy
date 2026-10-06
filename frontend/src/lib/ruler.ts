// The ruler of a track, the range picker's and the clip timeline's alike:
// a line every so often with its time written beside it.
//
// A time is written GAP to the right of its line, so two times stand clear
// of each other when the next line is at least GAP past the end of the
// time before it. The step is the smallest that leaves that room, worked
// out from how wide the widest time really is. It was a guess, 72 pixels a
// time on the range picker and 96 on the clip timeline, where a time like
// 5:00 is about 25: a range picker 370 pixels wide was given room for five
// times and drew one every five minutes on a six minute episode, where a
// time every minute fits with room to spare.

// The space between a line and its time, the margin of .time in the
// stylesheets of both tracks.
export const GAP = 4;

// The smallest step in steps, in seconds, whose times stand clear of each
// other on a track width pixels wide showing span seconds, when the widest
// time is label pixels wide. The largest step when none does.
export function rulerStep(span: number, width: number, label: number, steps: number[]): number {
  const need = label + 2 * GAP;
  return steps.find((s) => (s / span) * width >= need) ?? steps[steps.length - 1];
}

// Whether a time whose line is at x fits on a track width pixels wide,
// with GAP to spare at the end, rather than running into its edge.
export function fitsAt(x: number, label: number, width: number): boolean {
  return x + GAP + label + GAP <= width;
}

// How wide a time is in the ruler's type, 11 pixels of the app's font with
// figures of one width. Every figure is measured as a nought, which is as
// wide as any other where the figures are of one width, and the widest
// where they are not. Measured once for each shape of time.
const widths = new Map<string, number>();
let context: CanvasRenderingContext2D | null = null;
export function timeWidth(time: string): number {
  const shape = time.replace(/[0-9]/g, "0");
  const known = widths.get(shape);
  if (known !== undefined) return known;
  context ??= document.createElement("canvas").getContext("2d");
  if (!context) return shape.length * 7;
  context.font = `11px ${getComputedStyle(document.body).fontFamily}`;
  const width = Math.ceil(context.measureText(shape).width);
  widths.set(shape, width);
  return width;
}
