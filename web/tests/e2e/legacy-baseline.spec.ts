import { test, expect, type Page } from "@playwright/test";

const enabled = process.env.POOL_LOCAL_DEV_BASELINE === "1";

function observeFailures(page: Page) {
  const pageErrors: string[] = [];
  const consoleErrors: string[] = [];
  const failedRequests: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  page.on("console", (message) => { if (message.type() === "error") consoleErrors.push(message.text()); });
  page.on("requestfailed", (request) => failedRequests.push(`${request.method()} ${request.url()} ${request.failure()?.errorText ?? "failed"}`));
  return { pageErrors, consoleErrors, failedRequests };
}

async function expectNoFailures(failures: ReturnType<typeof observeFailures>, allowedConsole: RegExp[] = []) {
  expect(failures.pageErrors, `page errors: ${failures.pageErrors.join("; ")}`).toEqual([]);
  const unexpectedConsole = failures.consoleErrors.filter((message) => !allowedConsole.some((pattern) => pattern.test(message)));
  expect(unexpectedConsole, `console errors: ${unexpectedConsole.join("; ")}`).toEqual([]);
  expect(failures.failedRequests, `request failures: ${failures.failedRequests.join("; ")}`).toEqual([]);
}

test.describe("legacy UI local-development baseline", () => {
  test.skip(!enabled, "set POOL_LOCAL_DEV_BASELINE=1 against the isolated LOCAL_DEV_SESSION gateway");

  test("live session and every existing primary view remain usable", async ({ page }) => {
    const failures = observeFailures(page);
    const apiResponses: Array<{ url: string; status: number }> = [];
    page.on("response", (response) => {
      if (response.url().includes("/api/") || response.url().includes("/config/")) apiResponses.push({ url: response.url(), status: response.status() });
    });

    await page.goto("/");
    await expect(page.locator(".nav-item.active")).toContainText("PULSE");
    await expect(page.locator('[aria-label="Pool extraction summary"]')).toBeVisible();

    const views = [
      ["INSIGHTS", "Resource intelligence"],
      ["USAGE", "Burn analysis"],
      ["ACCOUNTS", "Account contribution"],
      ["MODELS", "Supported models"],
      ["SETUP", "Setup frequencies"],
    ] as const;
    for (const [navigation, heading] of views) {
      await page.locator(".nav-item", { hasText: navigation }).click();
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
    }
    await page.locator(".nav-item.nav-user").click();
    await expect(page.getByRole("heading", { name: "Profile" })).toBeVisible();
    await expect(page.getByText("developer@localhost.invalid", { exact: true })).toBeVisible();

    for (const required of ["/api/pool/session", "/api/pool/stats", "/api/pool/signal", "/api/pool/catalog"]) {
      expect(apiResponses.some((response) => response.url.includes(required) && response.status === 200), `missing successful ${required}`).toBe(true);
    }
    expect(apiResponses.filter((response) => response.status >= 500), "server API failures").toEqual([]);
    await expectNoFailures(failures);
  });

  test("manual refresh preserves fresh stats when analytics temporarily fails", async ({ page }) => {
    const failures = observeFailures(page);
    let failAnalytics = false;
    let statsRequests = 0;
    await page.route("**/api/pool/stats*", async (route) => { statsRequests++; await route.continue(); });
    await page.route("**/api/pool/signal*", async (route) => {
      if (failAnalytics) {
        await route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "baseline injected analytics outage" }) });
      } else {
        await route.continue();
      }
    });

    await page.goto("/");
    await expect(page.locator('[aria-label="Pool extraction summary"]')).toBeVisible();
    const initialStatsRequests = statsRequests;
    failAnalytics = true;
    await page.getByRole("button", { name: "SYNC" }).click();
    await expect(page.getByRole("alert")).toContainText("analytics: baseline injected analytics outage");
    await expect(page.locator('[aria-label="Pool extraction summary"]')).toBeVisible();
    expect(statsRequests).toBeGreaterThan(initialStatsRequests);
    await expectNoFailures(failures, [/503 \(Service Unavailable\)/]);
  });

  test("canonical v2 provider connections render in the existing admin Accounts screen", async ({ page }) => {
    const failures = observeFailures(page);
    await page.route("**/api/pool/session", async (route) => {
      const response = await route.fetch();
      const json = await response.json();
      await route.fulfill({ response, json: { ...json, is_admin: true, mfa_enrolled: true } });
    });
    await page.route("**/api/admin/mfa/status", async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ enrolled: true, elevated: true, recovery_codes_remaining: 8 }) });
    });
    await page.route("**/api/v2/provider-connections", async (route) => {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify([{
        id: "canonical-connection", public_id: "public-connection", provider_id: "runtime-provider",
        identity: { display_name: "Canonical Runtime Connection", external_subject: "subject", attributes: { region: "test" } },
        plan_type: "test", disabled: false, dead: false, inflight: 0, penalty: 0, score: 1, is_primary: true,
        usage: {}, totals: {},
      }]) });
    });
    await page.route("**/api/pool/stats", async (route) => {
      const response = await route.fetch(); const json = await response.json();
      json.accounts = [...(json.accounts ?? []), {
        id: "public-connection", display_name: "Canonical Runtime Connection", type: "runtime-provider", plan_type: "test",
        status: "healthy", penalty: 0, primary_window_used_pct: 0, secondary_window_used_pct: 0,
        primary_window_available: false, secondary_window_available: false, primary_reset_minutes: 0, secondary_reset_minutes: 0,
        primary_window_minutes: 0, secondary_window_minutes: 0, primary_pace_ratio: 0, secondary_pace_ratio: 0,
        total_input_tokens: 0, total_cached_tokens: 0, total_output_tokens: 0, total_reasoning_tokens: 0, total_billable_tokens: 0,
        cache_hit_rate_pct: 0, score: 1, is_primary: true, subscription_cost_monthly: 0, subscription_spend: 0,
        subscription_billing_cycles: 0, subscription_label: "", api_cost_estimate: 0, api_cost_last_30d: 0, roi: 0,
      }];
      await route.fulfill({ response, json });
    });

    await page.goto("/");
    await page.locator(".nav-item", { hasText: "ACCOUNTS" }).click();
    await expect(page.getByText("Canonical Runtime Connection", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("Runtime Provider", { exact: true }).first()).toBeVisible();
    await expectNoFailures(failures);
  });

  test("repeated refresh cycles do not duplicate the application shell", async ({ page }) => {
    const failures = observeFailures(page);
    await page.goto("/");
    await expect(page.locator('[aria-label="Pool extraction summary"]')).toBeVisible();
    for (let index = 0; index < 5; index++) {
      await page.getByRole("button", { name: "SYNC" }).click();
      await expect(page.getByRole("button", { name: "SYNC" })).toBeEnabled();
    }
    await expect(page.locator(".signal-app")).toHaveCount(1);
    await expect(page.locator(".signal-nav")).toHaveCount(1);
    await expectNoFailures(failures);
  });
});
