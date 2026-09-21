// The captions in the picture are written in the same faces as the render.
// They are built into the program and served from it, so nothing has to be
// installed on the machine and nothing is fetched from the web.
import { api, type CaptionFont } from "./api";

let done = false;

export async function installFonts(): Promise<CaptionFont[]> {
  const fonts = (await api.fonts()) ?? [];
  if (done) return fonts;
  done = true;
  const rules = fonts.map((font) => {
    const name = font.name.replace(/["\\]/g, "");
    return `@font-face { font-family: "${name}"; font-display: swap;
      src: url("/font?name=${encodeURIComponent(font.name)}") format("truetype"); }`;
  });
  const style = document.createElement("style");
  style.textContent = rules.join("\n");
  document.head.appendChild(style);
  return fonts;
}
