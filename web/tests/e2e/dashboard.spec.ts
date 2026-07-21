import { test, expect } from "@playwright/test";
import { readTestSessionConfig, sessionCookieValue } from "./helpers/session";

// This suite exists because a class of bug lives entirely outside what
// `tsc` or a mocked unit test can see: a real backend value (e.g. a new
// provider account type) reaching a frontend lookup table that was never
// updated for it, throwing at render time with no error boundary and
// silently blanking the whole page. Only actually loading the page in a
// browser and checking for a JS exception catches that.

test("signed-out root page renders the access gate with no console errors", async ({ page }) => {
  const pageErrors: Error[] = [];
  page.on("pageerror", (err) => pageErrors.push(err));

  await page.goto("/");
  await expect(page.getByText("SIGN IN WITH GOOGLE")).toBeVisible();

  expect(pageErrors, `unexpected page errors: ${pageErrors.map((e) => e.message).join("; ")}`).toHaveLength(0);
});

// Every id the frontend's `PROVIDERS` record (web/src/App.tsx) must have an
// entry for. Kept as a literal, not an import from ./types, so this list only
// grows when someone deliberately updates it alongside a new provider - the
// same manual step that must also touch PROVIDERS itself.
//
// When adding a new provider:
//   1. Add the type to web/src/types.ts (Provider union)
//   2. Add display metadata to the PROVIDERS record in web/src/App.tsx
//   3. Add the id string to ALL_PROVIDER_IDS below
//   4. Run `npx playwright test` to confirm every view renders cleanly
const ALL_PROVIDER_IDS = [
  "codex", "claude", "gemini", "antigravity", "kimi", "kimi-platform",
  "minimax", "zai", "xiaomi", "grok", "deepseek", "qwen", "openrouter", "nvidia",
];

function syntheticAccountStats(type: string, index: number) {
  return {
    id: `e2e-synthetic-${type}`,
    type,
    plan_type: type,
    // secondary_window_used_pct >= 80 puts this row in the "intervention
    // queue", which indexes PROVIDERS[account.type] directly (unlike
    // ProviderLanes, which only ever iterates PROVIDERS' own known keys and
    // so can never surface a missing entry) - this is the actual crash path.
    status: "healthy",
    penalty: 0,
    primary_window_used_pct: 90,
    secondary_window_used_pct: 90,
    primary_window_available: true,
    secondary_window_available: true,
    primary_reset_minutes: 60,
    secondary_reset_minutes: 60,
    primary_window_minutes: 300,
    secondary_window_minutes: 10080,
    primary_pace_ratio: 1,
    secondary_pace_ratio: 1,
    total_input_tokens: 1000 + index,
    total_cached_tokens: 0,
    total_output_tokens: 500,
    total_reasoning_tokens: 0,
    total_billable_tokens: 1500,
    cache_hit_rate_pct: 0,
    score: 80,
    is_primary: index === 0,
    subscription_cost_monthly: 20,
    subscription_spend: 20,
    subscription_billing_cycles: 1,
    subscription_label: "synthetic",
    api_cost_estimate: 1,
    api_cost_last_30d: 1,
    roi: 1,
  };
}

test.describe("authenticated dashboard", () => {
  const sessionConfig = readTestSessionConfig();

  test.skip(
    !sessionConfig,
    "requires POOL_JWT_SECRET, POOL_TEST_USER_ID, POOL_TEST_USER_EMAIL for an already-provisioned pool user",
  );

  // Guards against the exact class of bug that caused the production
  // black-page incident: this test only proves something if an account of
  // the new provider type actually exists in the pool at test time, which
  // isn't true for every type on every run. Injecting one synthetic account
  // per provider id into a real /api/pool/stats response forces every entry
  // in the frontend's PROVIDERS lookup table to be exercised deterministically,
  // regardless of what's actually pooled.
  test("every provider type renders on Pulse without a JS exception, including types with no real pooled account", async ({ page, baseURL }) => {
    const pageErrors: Error[] = [];
    page.on("pageerror", (err) => pageErrors.push(err));

    await page.route("**/api/pool/stats*", async (route) => {
      const response = await route.fetch();
      const json = await response.json();
      json.accounts = [
        ...(json.accounts ?? []),
        ...ALL_PROVIDER_IDS.map((type, index) => syntheticAccountStats(type, index)),
      ];
      await route.fulfill({ response, json });
    });

    const host = new URL(baseURL ?? "http://127.0.0.1:8989").hostname;
    await page.context().addCookies([
      {
        name: "pool_session",
        value: sessionCookieValue(sessionConfig!),
        domain: host,
        path: "/",
        httpOnly: true,
        secure: false,
      },
    ]);

    await page.goto("/");
    await expect(page.locator(".nav-item.active")).toContainText("PULSE");
    const root = page.locator("#root");
    await expect(root).not.toBeEmpty();

    expect(pageErrors, `unexpected page errors: ${pageErrors.map((e) => e.message).join("; ")}`).toHaveLength(0);
  });

  test("every primary view renders without a JS exception", async ({ page, baseURL }) => {
    const pageErrors: Error[] = [];
    page.on("pageerror", (err) => pageErrors.push(err));

    // Inject every known provider type so the Accounts table (the
    // production crash path) exercises the full PROVIDERS lookup table.
    await page.route("**/api/pool/stats*", async (route) => {
      const response = await route.fetch();
      const json = await response.json();
      json.accounts = [
        ...(json.accounts ?? []),
        ...ALL_PROVIDER_IDS.map((type, index) => syntheticAccountStats(type, index)),
      ];
      await route.fulfill({ response, json });
    });

    const host = new URL(baseURL ?? "http://127.0.0.1:8989").hostname;
    await page.context().addCookies([
      {
        name: "pool_session",
        value: sessionCookieValue(sessionConfig!),
        domain: host,
        path: "/",
        httpOnly: true,
        secure: false,
      },
    ]);

    await page.goto("/");

    // Pulse (default view) - iterates every pooled account's provider type.
    // Nav button text nodes combine an icon glyph with the label, so match
    // on the active tab's content rather than an exact standalone string.
    await expect(page.locator(".nav-item.active")).toContainText("PULSE");
    const root = page.locator("#root");
    await expect(root).not.toBeEmpty();

    // Accounts - the view that crashed in production: renders one row per
    // pooled account, looking up per-provider display metadata by type.
    await page.locator(".nav-item", { hasText: "ACCOUNTS" }).click();
    await expect(page.getByRole("heading", { name: "Account contribution" })).toBeVisible();

    // Profile - exercises the MFA status section.
    await page.locator(".nav-item.nav-user").click();
    await expect(page.getByRole("heading", { name: "Profile" })).toBeVisible();

    expect(pageErrors, `unexpected page errors: ${pageErrors.map((e) => e.message).join("; ")}`).toHaveLength(0);
  });
});
