// The word being typed in the caption box is written in the same face as
// the render. The face is served by the program, a bundled one or the copy
// of one installed on the machine, so nothing is fetched from the web and
// the word is drawn from the very file libass draws the short from.
//
// A face of the machine's is asked of the system first, by its own names,
// which is the same file the copy is made from and costs nothing. The list
// of fonts draws every name in its own face, hundreds of them on a Mac,
// and a copy of each would be hundreds of files read and sent for one
// look at the list.
import { api, type CaptionFont } from "./api";

// The faces declared so far. The list is asked for again each time a video
// opens, so a font installed since is declared then.
const declared = new Set<string>();

const quoted = (name: string) => `"${name.replace(/["\\]/g, "")}"`;

export async function installFonts(): Promise<CaptionFont[]> {
  const fonts = (await api.fonts()) ?? [];
  const fresh = fonts.filter((font) => !declared.has(font.name));
  if (fresh.length === 0) return fonts;
  for (const font of fresh) declared.add(font.name);
  const rules = fresh.map((font) => {
    // With the face's own weight, so the browser fakes bold only where
    // libass does too. Left out, every face counts as 400 and Inter Black
    // is thickened on screen and not in the short.
    const weight = Number.isInteger(font.weight) ? font.weight : 400;
    const local = font.machine
      ? [font.postscript, font.name].filter(Boolean).map((n) => `local(${quoted(n!)}), `).join("")
      : "";
    return `@font-face { font-family: ${quoted(font.name)}; font-weight: ${weight}; font-display: swap;
      src: ${local}url("/font?name=${encodeURIComponent(font.name)}"); }`;
  });
  const style = document.createElement("style");
  style.textContent = rules.join("\n");
  document.head.appendChild(style);
  return fonts;
}

// How a row in a list of fonts draws its name: in the face itself, at its
// own weight, and in the app's own face where it has no letter for one.
export function faceLook(font: CaptionFont): { font: string; weight: number } {
  return { font: `${quoted(font.name)}, var(--font)`, weight: Number.isInteger(font.weight) ? font.weight : 400 };
}

// A family of faces, the way Pages offers a font: American Typewriter, and
// in it Regular, Bold, Condensed Light.
export interface Family {
  name: string;
  // Installed on the machine, rather than one of the app's own. A family
  // with a face of the app's counts as the app's.
  machine: boolean;
  // Narrowest first, then lightest, then upright before leaning, the
  // order Font Book lists them in.
  faces: CaptionFont[];
}

// The families of the faces, the app's own first and then the machine's,
// each by name, the way Font Book lists them.
export function families(fonts: CaptionFont[]): Family[] {
  const byName = new Map<string, Family>();
  for (const font of fonts) {
    const name = font.family || font.name;
    let family = byName.get(name);
    if (!family) {
      family = { name, machine: !!font.machine, faces: [] };
      byName.set(name, family);
    }
    family.machine &&= !!font.machine;
    family.faces.push(font);
  }
  const all = [...byName.values()];
  for (const family of all) {
    family.faces.sort(
      (a, b) =>
        (a.width || 5) - (b.width || 5) ||
        (a.weight || 400) - (b.weight || 400) ||
        Number(!!a.italic) - Number(!!b.italic),
    );
  }
  const ours = all.filter((f) => !f.machine).sort((a, b) => a.name.localeCompare(b.name));
  const theirs = all.filter((f) => f.machine).sort((a, b) => a.name.localeCompare(b.name));
  return [...ours, ...theirs];
}

// The face of a family nearest to like: upright or leaning the same, then
// as wide, then as heavy. So picking another family keeps the look, and a
// family picked from scratch comes in its regular face.
export function nearestFace(family: Family, like?: CaptionFont): CaptionFont {
  const want = like ?? ({ weight: 400, width: 5, italic: false } as CaptionFont);
  const cost = (f: CaptionFont) =>
    (!!f.italic !== !!want.italic ? 100000 : 0) +
    Math.abs((f.width || 5) - (want.width || 5)) * 1000 +
    Math.abs((f.weight || 400) - (want.weight || 400));
  let best = family.faces[0];
  for (const face of family.faces) {
    if (like && face.style === like.style) return face;
    if (cost(face) < cost(best)) best = face;
  }
  return best;
}

// What each face of a family is called in its list: its style, or its
// full name where two faces of one family call themselves the same.
export function styleNames(family: Family): Map<string, string> {
  const count = new Map<string, number>();
  for (const face of family.faces) count.set(face.style, (count.get(face.style) ?? 0) + 1);
  return new Map(
    family.faces.map((face) => [face.name, (count.get(face.style) ?? 0) > 1 || !face.style ? face.name : face.style]),
  );
}
