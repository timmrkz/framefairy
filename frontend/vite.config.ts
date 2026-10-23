import { readdirSync, readFileSync } from "node:fs";
import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Every package that ends up in the bundle has its licence shown in the
// app, from notices/notices.json. A package the bundle takes in without a
// notice, a notice for one it no longer takes in, or a package whose
// licence file is no longer the text its notice shows, stops the build
// here, because the day it changes is the day it is easiest to say what it
// is. A new version alone stops nothing: the app shows the version the
// lock file installs, see notices/notices.go, so an update that keeps its
// licence needs no new notice. make notices writes the notices again. It
// builds the interface itself to see what the bundle holds, and says so in
// FRAMEFAIRY_WRITING_NOTICES, so the notices it is there to write do not
// stop it.
function notices(): Plugin {
  return {
    name: "framefairy-notices",
    apply: (_, env) => env.command === "build" && !process.env.FRAMEFAIRY_WRITING_NOTICES,
    generateBundle(_, bundle) {
      const root = new URL("../notices/", import.meta.url);
      const noticed = new Map<string, string[]>();
      for (const n of JSON.parse(readFileSync(new URL("notices.json", root), "utf8"))) {
        if (n.part === "The app's interface") noticed.set(n.name, n.texts);
      }
      const found = new Set<string>();
      for (const output of Object.values(bundle)) {
        if (output.type !== "chunk") continue;
        for (const id of output.moduleIds) {
          const hit = /node_modules\/((?:@[^/]+\/)?[^/]+)\//.exec(id.replaceAll("\\", "/"));
          if (hit) found.add(hit[1]);
        }
      }
      const wrong: string[] = [];
      for (const name of found) {
        const texts = noticed.get(name);
        if (texts === undefined) {
          wrong.push(`${name} has no notice`);
          continue;
        }
        // The licence files at the top of the package, by the names
        // notices/licence looks for. A package without one, like the
        // Wails runtime, has its text from elsewhere and nothing to
        // compare.
        const dir = new URL(`./node_modules/${name}/`, import.meta.url);
        const files = readdirSync(dir).filter((f) => /^(licen[cs]e|copying)/i.test(f));
        const shown = texts.map((t) => readFileSync(new URL(`texts/${t}`, root), "utf8").trim());
        if (files.length > 0 && !files.some((f) => shown.includes(readFileSync(new URL(f, dir), "utf8").trim()))) {
          wrong.push(`${name} carries another licence text than its notice shows`);
        }
      }
      for (const name of noticed.keys()) {
        if (!found.has(name)) wrong.push(`${name} has a notice and is not in the bundle`);
      }
      if (found.size === 0) wrong.push("no package was found in the bundle, so nothing was checked");
      if (wrong.length > 0) this.error(`${wrong.join(", ")}. Run make notices.`);
    },
  };
}

// The app calls Go methods by name, so there are no generated bindings to
// load here. The build goes next to the app's Go code, because Go can only
// embed files from its own folder. It goes into dist/app/, which is emptied
// on every build, while dist/.keep stays so Go can embed dist/ before the
// first build.
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  // Vite warns about a chunk over 500 kB because a web page downloads it.
  // The app reads its interface from its own binary, so the size of one
  // chunk costs nothing the warning is about, and the build fails on any
  // warning. The bundle passed 500 kB with the bounce after a word step.
  // The limit stays, at twice that, to catch a library pulled in by
  // mistake.
  build: {
    outDir: "../cmd/framefairy-app/dist/app",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1000,
  },
  plugins: [svelte(), notices()],
});
