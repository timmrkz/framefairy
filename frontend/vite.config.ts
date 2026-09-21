import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

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
  plugins: [svelte()],
});
