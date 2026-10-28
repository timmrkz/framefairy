// Builds the interface against the real Go side, through the bridge in
// cmd/framefairy-app/bridge_test.go. The same as vite.config.ts but for
// the runtime it is built with and where it goes. See docs/TESTING.md.
import { defineConfig, mergeConfig } from "vite";
import { resolve } from "node:path";
import preview from "./vite.config";

const here = import.meta.dirname;

export default mergeConfig(
  preview,
  defineConfig({
    resolve: { alias: { "@wailsio/runtime": resolve(here, "wails-bridge.ts") } },
    // When the walks count the lines they run, make walks COVERAGE=, each
    // script gets its source map beside it, which the walks read to name
    // the line of the component a count belongs to. Hidden, so the page
    // itself is built the same as on every other run.
    build: {
      outDir: resolve(here, "dist-bridge"),
      emptyOutDir: true,
      sourcemap: process.env.WALKCOVERAGE ? "hidden" : false,
    },
  }),
);
