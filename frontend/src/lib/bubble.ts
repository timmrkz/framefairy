// Where an info bubble goes, so the whole of what it says can be read.
//
// It hangs under the mark that opens it, or over it, or beside it, the
// first of those that holds all of it inside the app. A long text that
// fits nowhere at the narrow width is tried wider, which makes it
// shorter. Only an app too small for it at any width gets a bubble as
// tall as the app that scrolls. Tim found the clip timeline's bubble cut
// off by the foot of the app: it was too tall for the room under its
// mark and for the room over it, and it went under it anyway.

export type Box = { top: number; bottom: number; left: number; right: number };

export type Place = {
  top: number;
  left: number;
  width: number;
  where: "below" | "above" | "beside";
  scroll: boolean;
};

// The widths a bubble may take, narrowest first: a short line reads best,
// a wider one only when the text would not fit otherwise.
export const widths = [260, 340, 420, 520];

// Between the mark and the bubble, and between the bubble and the edge of
// the app.
const gap = 6;
const edge = 8;

// mark is the mark's box in the app, app its width and height, side the
// side the bubble grows towards, and tall how tall the text comes out at a
// width, which only the page can say.
export function placeBubble(
  mark: Box,
  app: { width: number; height: number },
  side: "left" | "right",
  tall: (width: number) => number,
): Place {
  const below = app.height - edge - (mark.bottom + gap);
  const above = mark.top - gap - edge;
  // Beside the mark, on the side the bubble grows towards, so it does not
  // cover the mark.
  const aside = side === "right" ? mark.left - gap - edge : app.width - edge - (mark.right + gap);
  const along = (width: number) => {
    const left = side === "right" ? mark.right + 8 - width : mark.left - 8;
    return Math.max(edge, Math.min(left, app.width - edge - width));
  };
  const fits = widths.filter((w) => w <= app.width - 2 * edge);
  if (!fits.length) fits.push(Math.max(120, app.width - 2 * edge));
  // Under or over the mark first, at any width, because that is where the
  // eye goes from the mark. Beside it only when neither has room.
  for (const width of fits) {
    const height = tall(width);
    if (height <= below) return { top: mark.bottom + gap, left: along(width), width, where: "below", scroll: false };
    if (height <= above) return { top: mark.top - gap - height, left: along(width), width, where: "above", scroll: false };
  }
  for (const width of fits) {
    const height = tall(width);
    if (width <= aside && height <= app.height - 2 * edge) {
      const top = Math.max(edge, Math.min(mark.top - 8, app.height - edge - height));
      const left = side === "right" ? mark.left - gap - width : mark.right + gap;
      return { top, left, width, where: "beside", scroll: false };
    }
  }
  const width = fits[fits.length - 1];
  return { top: edge, left: along(width), width, where: "below", scroll: true };
}
