/// <reference types="vitest/config" />
import { mkdirSync, writeFileSync } from "node:fs";
import { defineConfig, type Plugin } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// Every package that ends up in the bundle has its licence shown in the
// app. The build knows which ones those are, so it says so here: it writes
// the folder of each into dist/packages.json, and make hands that to
// notices/gen -built, which reads each package's version and licence from
// its folder and stops the build on a licence nobody allowed. The folder
// is the one the code came from, so a copy a package carries in its own
// node_modules counts as itself, not as the package that carries it.
function notices(): Plugin {
  return {
    name: "framefairy-notices",
    apply: "build",
    generateBundle(_, bundle) {
      const folders = new Set<string>();
      for (const output of Object.values(bundle)) {
        if (output.type !== "chunk") continue;
        for (const id of output.moduleIds) {
          const file = id.replaceAll("\\", "/");
          const at = file.indexOf("/node_modules/");
          if (at < 0) continue;
          // node_modules/a/node_modules/@b/c/dist/c.js comes from
          // node_modules/a/node_modules/@b/c.
          const hit = /^(?:node_modules\/(?:@[^/]+\/)?[^/]+\/)+/.exec(file.slice(at + 1));
          if (hit) folders.add(hit[0].slice(0, -1));
        }
      }
      if (folders.size === 0) this.error("no package was found in the bundle, so no licence can be shown");
      const dist = new URL("../cmd/framefairy-app/dist/", import.meta.url);
      mkdirSync(dist, { recursive: true });
      writeFileSync(new URL("packages.json", dist), JSON.stringify([...folders].sort(), null, 2) + "\n");
    },
  };
}

// With WERROR=1, which make passes on and CI sets for the Mac build, a
// warning fails the build: every warning is printed as before, and the
// build then ends with a status that is not zero. What is counted is what
// is printed. Vite's logger and Rolldown print with console.warn, Node
// prints its own, like a deprecation, through the process's warning
// event, and the Svelte plugin hands each to onwarn below.
const strict = process.env.WERROR === "1";
function warned(): void {
  if (strict) process.exitCode = 1;
}
if (strict) {
  const warn = console.warn.bind(console);
  console.warn = (...args: unknown[]) => {
    warned();
    warn(...args);
  };
  process.on("warning", warned);
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
  // warning, see WERROR above. The bundle passed 500 kB with the bounce
  // after a word step. The limit stays, at twice that, to catch a library
  // pulled in by mistake.
  build: {
    outDir: "../cmd/framefairy-app/dist/app",
    emptyOutDir: true,
    chunkSizeWarningLimit: 1000,
    // Rolldown warns when its plugins took most of a build's time, which
    // depends on how fast the machine is and says nothing about the code.
    // It came once on CI's Mac, on 8 November, and WERROR=1 failed on it.
    rolldownOptions: {
      checks: { bundlerTimings: false },
    },
  },
  plugins: [
    svelte({
      // The plugin prints no warning about a package's own component, so
      // that one is not counted either.
      onwarn(warning, handler) {
        if (!warning.filename?.includes("node_modules")) warned();
        handler(warning);
      },
    }),
    notices(),
  ],
  // Which lines the interface's own tests run, counted only when asked,
  // make interface COVERAGE=, for the nightly snapshot in Codecov, see
  // Coverage in docs/TESTING.md. The components are counted by the walks,
  // which are the tests that reach them. The paths are written from the
  // top of the repository, the way Codecov names every other file.
  test: {
    coverage: {
      provider: "v8",
      include: ["src/**/*.ts"],
      exclude: ["src/**/*.test.ts"],
      reporter: [["lcovonly", { projectRoot: ".." }]],
    },
  },
});
