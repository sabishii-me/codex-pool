import { spawnSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const playwrightCLI = resolve(here, "../node_modules/@playwright/test/cli.js");

const cycles = Number.parseInt(process.env.POOL_BASELINE_CYCLES ?? "12", 10);
const delaySeconds = Number.parseInt(process.env.POOL_BASELINE_DELAY_SECONDS ?? "300", 10);
const baseURL = process.env.POOL_BASE_URL ?? "http://127.0.0.1:18990";
if (!Number.isFinite(cycles) || cycles < 1 || !Number.isFinite(delaySeconds) || delaySeconds < 0) {
  console.error("POOL_BASELINE_CYCLES must be >= 1 and POOL_BASELINE_DELAY_SECONDS must be >= 0");
  process.exit(2);
}
for (let cycle = 1; cycle <= cycles; cycle++) {
  const started = new Date().toISOString();
  console.log(`[baseline-soak] cycle ${cycle}/${cycles} started ${started}`);
  const result = spawnSync(process.execPath, [playwrightCLI, "test", "tests/e2e/legacy-baseline.spec.ts"], {
    stdio: "inherit",
    env: { ...process.env, POOL_BASE_URL: baseURL, POOL_LOCAL_DEV_BASELINE: "1" },
  });
  if (result.error) {
    console.error(`[baseline-soak] failed to launch Playwright: ${result.error.message}`);
    process.exit(1);
  }
  if (result.status !== 0) process.exit(result.status ?? 1);
  if (cycle < cycles && delaySeconds > 0) {
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, delaySeconds * 1000);
  }
}
console.log(`[baseline-soak] completed ${cycles} cycles`);
