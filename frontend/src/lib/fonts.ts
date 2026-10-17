// The word being typed in the caption box is written in the same face as
// the render. The face is served by the program, a bundled one or the copy
// of one installed on the machine, so nothing is fetched from the web and
// the word is drawn from the very file libass draws the short from.
import { api, type CaptionFont } from "./api";

// The faces declared so far. The list is asked for again each time a video
// opens, so a font installed since is declared then.
const declared = new Set<string>();

export async function installFonts(): Promise<CaptionFont[]> {
  const fonts = (await api.fonts()) ?? [];
  const fresh = fonts.filter((font) => !declared.has(font.name));
  if (fresh.length === 0) return fonts;
  for (const font of fresh) declared.add(font.name);
  const rules = fresh.map((font) => {
    const name = font.name.replace(/["\\]/g, "");
    // With the face's own weight, so the browser fakes bold only where
    // libass does too. Left out, every face counts as 400 and Inter Black
    // is thickened on screen and not in the short.
    const weight = Number.isInteger(font.weight) ? font.weight : 400;
    return `@font-face { font-family: "${name}"; font-weight: ${weight}; font-display: swap;
      src: url("/font?name=${encodeURIComponent(font.name)}"); }`;
  });
  const style = document.createElement("style");
  style.textContent = rules.join("\n");
  document.head.appendChild(style);
  return fonts;
}
