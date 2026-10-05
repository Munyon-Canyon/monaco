import { resolve } from "node:path";
import { defineConfig } from "vite";

export default defineConfig({
  build: {
    outDir: "dist",
    // Vite's default "assets" would mix hashed bundles into the hand-made files under /assets/*.
    assetsDir: "_app",
    // Keeps the waitlist page byte-identical to its source; the fund page has no hand-written CSS to shrink.
    cssMinify: false,
    rollupOptions: {
      input: {
        main: resolve(import.meta.dirname, "index.html"),
      },
    },
  },
});
