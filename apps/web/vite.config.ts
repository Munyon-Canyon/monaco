/// <reference types="vitest/config" />
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig, loadEnv, type Plugin } from "vite";
import { fundCSP } from "./csp";

// The API origin differs per deploy, and _headers and vercel.json are static, so the full policy
// is a meta tag written at build time. The header keeps frame-ancestors, which a meta tag cannot set.
// Dev skips it: Vite's dev server injects inline scripts the policy would block.
function fundPolicy(apiURL: string): Plugin {
  return {
    name: "fund-csp",
    apply: "build",
    transformIndexHtml: {
      order: "post",
      handler(html, ctx) {
        if (!ctx.path.startsWith("/fund/")) return html;
        return html.replace("<head>", `<head>\n<meta http-equiv="Content-Security-Policy" content="${fundCSP(apiURL)}">`);
      },
    },
  };
}

export default defineConfig(({ mode }) => ({
  plugins: [react(), fundPolicy(loadEnv(mode, import.meta.dirname).VITE_MONACO_API_URL || "http://localhost:8080")],
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
}));
