import { describe, expect, it } from "vitest";

// Every icon in the app is a Lucide icon, named in Icon.svelte by what it
// is for. A name Icon.svelte does not know draws nothing at all, an empty
// button with no error anywhere, so the names the app asks for are checked
// against the names it has. Plan row 2.199.
const files = import.meta.glob(["../**/*.svelte", "../**/*.ts", "!../**/*.test.ts"], {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const icon = Object.entries(files).find(([file]) => file.endsWith("/Icon.svelte"))![1];

// The keys of one object literal in Icon.svelte, quoted or not.
function keys(object: string): string[] {
  const start = icon.indexOf(`const ${object}`);
  const body = icon.slice(icon.indexOf("{", start), icon.indexOf("};", start));
  return [...body.matchAll(/^\s*"?([a-z][a-z-]*)"?:/gm)].map((m) => m[1]);
}

// Every name the app asks for: each string in an <Icon> tag, and each
// icon: field of the lists that hand one to Pick and to the actions.
function asked(): Map<string, string> {
  const names = new Map<string, string>();
  for (const [file, text] of Object.entries(files)) {
    for (const tag of text.matchAll(/<Icon\b[^>]*?\/>/gs)) {
      for (const m of tag[0].matchAll(/"([a-z][a-z-]*)"/g)) names.set(m[1], file);
    }
    for (const m of text.matchAll(/\bicon:\s*"([a-z][a-z-]*)"/g)) names.set(m[1], file);
  }
  return names;
}

describe("icons", () => {
  const known = new Set([...keys("glyphs"), ...keys("brackets")]);

  it("finds the names on both sides", () => {
    expect(known.size).toBeGreaterThan(20);
    expect(asked().size).toBeGreaterThan(20);
  });

  it("has an icon for every name the app asks for", () => {
    const missing = [...asked()].filter(([name]) => !known.has(name));
    expect(missing).toEqual([]);
  });

  it("takes every icon from Lucide, one icon at a time", () => {
    // One file per icon keeps the build from reading the two thousand
    // others, which importing from the package itself would.
    const imports = [...icon.matchAll(/^\s*import .* from "([^"]+)";$/gm)].map((m) => m[1]);
    const lucide = imports.filter((from) => from.includes("lucide"));
    expect(lucide.length).toBeGreaterThan(20);
    for (const from of lucide) expect(from).toMatch(/^@lucide\/svelte\/icons\/[a-z0-9-]+$/);
    // And nothing drawn by hand but the one path the brackets share.
    expect(icon.match(/<path\b/g)).toHaveLength(1);
  });
});
