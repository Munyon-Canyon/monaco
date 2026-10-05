/// <reference types="vitest/config" />
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  // node --test runs test/*.test.js; Vitest takes only the fund page's TypeScript tests.
  test: { include: ["src/**/*.test.ts"] },
  build: {
    outDir: "dist",
    // Vite's default "assets" would mix hashed bundles into the hand-made files under /assets/*.
    assetsDir: "_app",
    // Keeps the waitlist page byte-identical to its source; the fund page has no hand-written CSS to shrink.
    cssMinify: false,
    rollupOptions: {
      input: {
        main: resolve(import.meta.dirname, "index.html"),
        fund: resolve(import.meta.dirname, "fund/index.html"),
      },
    },
  },
});
