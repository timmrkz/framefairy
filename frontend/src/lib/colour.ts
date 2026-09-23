// The one colour the app is picked out in. Everything the app draws in its
// own colour takes it from --accent, and the lighter one and the wash are
// mixed from it in app.css, so setting this one moves all three.
//
// What a person typed is untrusted like anything else, so a value that is
// not a colour is ignored rather than written into the page.
const looksLikeColour = /^#?[0-9a-fA-F]{6}$/;

export function wearColour(colour: string) {
  const text = (colour ?? "").trim();
  if (!looksLikeColour.test(text)) return;
  const hex = text.startsWith("#") ? text : `#${text}`;
  document.documentElement.style.setProperty("--accent", hex);
}

// A colour as the engine reports it for the caption preview, rgba(r, g, b,
// a), taken apart into what a colour field and an opacity field hold: the
// colour as #rrggbb and how opaque it is, from 0 to 1. Anything else reads
// as opaque white, which is what the captions start out as.
export function splitColour(css: string): { hex: string; alpha: number } {
  const m = /^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})\s*(?:,\s*([\d.]+)\s*)?\)$/.exec(
    (css ?? "").trim(),
  );
  if (!m) return { hex: "#ffffff", alpha: 1 };
  const part = (v: string) =>
    Math.min(255, Math.max(0, Number(v)))
      .toString(16)
      .padStart(2, "0");
  const alpha = m[4] === undefined ? 1 : Math.min(1, Math.max(0, Number(m[4])));
  return { hex: `#${part(m[1])}${part(m[2])}${part(m[3])}`, alpha: Number.isFinite(alpha) ? alpha : 1 };
}

// And put back together, for drawing a colour while it is being picked.
export function joinColour(hex: string, alpha: number): string {
  const m = /^#([0-9a-fA-F]{2})([0-9a-fA-F]{2})([0-9a-fA-F]{2})$/.exec(hex ?? "");
  if (!m) return "rgba(255, 255, 255, 1)";
  const a = Math.min(1, Math.max(0, Number.isFinite(alpha) ? alpha : 1));
  return `rgba(${parseInt(m[1], 16)}, ${parseInt(m[2], 16)}, ${parseInt(m[3], 16)}, ${a})`;
}
