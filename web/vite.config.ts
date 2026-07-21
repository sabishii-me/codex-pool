import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";
import { configDefaults } from "vitest/config";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    manifest: true,
  },
  test: {
    // tests/e2e/** are Playwright specs (npm run test:e2e), not vitest -
    // both use the *.spec.ts naming convention, so without this exclude
    // vitest tries to run Playwright's test/expect through its own runner.
    exclude: [...configDefaults.exclude, "tests/e2e/**"],
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:18990",
      "/admin": "http://127.0.0.1:18990",
      "/auth": "http://127.0.0.1:18990",
      "/setup": "http://127.0.0.1:18990",
      "/config": "http://127.0.0.1:18990",
      "/hero.webp": "http://127.0.0.1:18990",
    },
  },
});
