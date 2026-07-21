import { defineConfig } from "@playwright/test";

// Points at a pool instance that's already running (e.g. `docker compose up`
// or `go run .` locally) - this harness does not start the server itself,
// since the pool needs real provider config/secrets to be meaningful.
export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 30_000,
  fullyParallel: true,
  reporter: [["list"]],
  use: {
    baseURL: process.env.POOL_BASE_URL ?? "http://127.0.0.1:8989",
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
});
