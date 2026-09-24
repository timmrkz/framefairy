import { readFileSync } from "node:fs";
import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Every package that ends up in the bundle has its licence shown in the
// app, from notices/notices.json. A package the bundle takes in without a
// notice, or at another version than its notice, stops the build here,
// because the day it is added is the day it is easiest to say what it is.
// make notices writes the notices again.
function notices(): Plugin {
  return {
    name: "framefairy-notices",
    apply: "build",
    generateBundle(_, bundle) {
      const noticed = new Map<string, string>();
      const list = JSON.parse(readFileSync(new URL("../notices/notices.json", import.meta.url), "utf8"));
      for (const n of list) noticed.set(n.name, n.version);
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
        const pkg = JSON.parse(readFileSync(new URL(`./node_modules/${name}/package.json`, import.meta.url), "utf8"));
        const version = noticed.get(name);
        if (version === undefined) wrong.push(`${name} has no notice`);
        else if (version !== pkg.version) wrong.push(`${name} is ${pkg.version} and its notice is for ${version}`);
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
  build: {
    outDir: "../cmd/framefairy-app/dist/app",
    emptyOutDir: true,
  },
  plugins: [svelte(), notices()],
});
