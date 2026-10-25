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
    build: { outDir: resolve(here, "dist-bridge"), emptyOutDir: true },
  }),
);
