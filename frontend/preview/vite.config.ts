// Builds the interface against a fake Go side, so it can be opened in a
// plain browser. The app itself cannot run in a cloud session: there is no
// screen to show it on, so this is the only way to see an interface
// change before Tim does.
//
// It lives beside frontend/ rather than in it so it finds the project's
// own node_modules, and inside preview/ rather than at the top of
// frontend/ so the real build never sees it: make names frontend/
// vite.config.ts on its own, and tsconfig only takes in src/.
import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { resolve } from "node:path";

const here = import.meta.dirname;
const frontend = resolve(here, "..");

export default defineConfig({
  root: frontend,
  resolve: {
    // Everything the interface asks the Go side for comes from here.
    alias: { "@wailsio/runtime": resolve(here, "wails-stub.ts") },
  },
  build: { outDir: resolve(here, "dist"), emptyOutDir: true },
  plugins: [svelte({ configFile: resolve(frontend, "svelte.config.js") })],
});
