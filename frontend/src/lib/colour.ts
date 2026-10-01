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

// The colours a caption is most often made in, offered first wherever a
// caption's colour is picked: the white and the black of nearly every
// caption, the yellow of subtitles, the app's own purple, which is the
// highlight a short starts out with, and a few strong ones that stand out
// on a picture. The Mac's own, as its dark mode draws them.
export const captionColours = [
  { hex: "#ffffff", name: "White" },
  { hex: "#000000", name: "Black" },
  { hex: "#ffd60a", name: "Yellow" },
  { hex: "#ff9f0a", name: "Orange" },
  { hex: "#ff453a", name: "Red" },
  { hex: "#ff375f", name: "Pink" },
  { hex: "#942192", name: "Purple, the app's own" },
  { hex: "#0a84ff", name: "Blue" },
  { hex: "#30d158", name: "Green" },
];

// A colour as #rrggbb, lower case, or nothing when it is not one. What is
// typed into a hex field is read through this, with or without the #, and
// three digits stand for six the way they do in a stylesheet.
export function readHex(text: string): string | null {
  let t = (text ?? "").trim().replace(/^#/, "");
  if (/^[0-9a-fA-F]{3}$/.test(t)) t = t.replace(/./g, (c) => c + c);
  if (!/^[0-9a-fA-F]{6}$/.test(t)) return null;
  return `#${t.toLowerCase()}`;
}

// A colour as hue, saturation and brightness, the three the picker's
// square and strip move: the hue from 0 to 360, the other two from 0 to 1.
export type HSV = { h: number; s: number; v: number };

export function hexToHSV(hex: string): HSV {
  const c = readHex(hex) ?? "#ffffff";
  const r = parseInt(c.slice(1, 3), 16) / 255;
  const g = parseInt(c.slice(3, 5), 16) / 255;
  const b = parseInt(c.slice(5, 7), 16) / 255;
  const max = Math.max(r, g, b);
  const d = max - Math.min(r, g, b);
  let h = 0;
  if (d > 0) {
    if (max === r) h = ((g - b) / d) % 6;
    else if (max === g) h = (b - r) / d + 2;
    else h = (r - g) / d + 4;
    h *= 60;
    if (h < 0) h += 360;
  }
  return { h, s: max === 0 ? 0 : d / max, v: max };
}

export function hsvToHex({ h, s, v }: HSV): string {
  const hue = (((h % 360) + 360) % 360) / 60;
  const sat = Math.min(1, Math.max(0, s));
  const val = Math.min(1, Math.max(0, v));
  const c = val * sat;
  const x = c * (1 - Math.abs((hue % 2) - 1));
  const m = val - c;
  const [r, g, b] =
    hue < 1 ? [c, x, 0] : hue < 2 ? [x, c, 0] : hue < 3 ? [0, c, x] : hue < 4 ? [0, x, c] : hue < 5 ? [x, 0, c] : [c, 0, x];
  const part = (n: number) =>
    Math.round((n + m) * 255)
      .toString(16)
      .padStart(2, "0");
  return `#${part(r)}${part(g)}${part(b)}`;
}

// A pixel read from a picture, as #rrggbb.
export function rgbToHex(r: number, g: number, b: number): string {
  const part = (n: number) => Math.min(255, Math.max(0, Math.round(n))).toString(16).padStart(2, "0");
  return `#${part(r)}${part(g)}${part(b)}`;
}
