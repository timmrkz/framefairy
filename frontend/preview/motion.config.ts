// The motion bench: every way the app shows work in hand, on one page.
//
//   make motion
//
// It is a page of its own rather than a state of the app, because seeing
// all five at once in the app would mean starting five different jobs and
// catching them at the right moment. It is in preview/ and never in the
// build the app embeds: the product only serves making shorts.
import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { resolve } from "node:path";

const here = import.meta.dirname;
const frontend = resolve(here, "..");

export default defineConfig({
  root: resolve(here, "motion"),
  // The bench uses the app's own stylesheet and the app's own Busy, which
  // live above its root.
  server: { host: "127.0.0.1", port: 9246, strictPort: true, fs: { allow: [frontend] } },
  build: { outDir: resolve(here, "dist-motion"), emptyOutDir: true },
  plugins: [svelte({ configFile: resolve(frontend, "svelte.config.js") })],
});
