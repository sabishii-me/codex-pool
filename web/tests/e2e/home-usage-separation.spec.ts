import { expect, test } from "@playwright/test";
import { expectClean, loginAs, watchRuntime } from "./helpers/acceptance";

test("Home orients and never duplicates Usage analytics", async ({ page }) => {
  await loginAs(page, "admin-elevated"); const runtime = watchRuntime(page); await page.goto("/");
  await expect(page.getByText("Ready to use")).toBeVisible();
  await expect(page.getByText("Choose a model", { exact: true })).toBeVisible();
  await expect(page.getByText("Connect a client", { exact: true })).toBeVisible();
  await expect(page.getByText("Pool request history")).toHaveCount(0);
  await expect(page.getByText("Token composition")).toHaveCount(0);
  await expect(page.getByText("Economics", { exact: true })).toHaveCount(0);
  expectClean(runtime);
});

test("Usage owns pool history, composition, and economics", async ({ page }) => {
  await loginAs(page, "admin-elevated"); const runtime = watchRuntime(page); await page.goto("/usage");
  await expect(page.getByText("Ready to use")).toHaveCount(0);
  await expect(page.getByText("Choose a model", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Pool usage" }).click();
  await expect(page.getByText("Tokens by model over time")).toBeVisible();
  await expect(page.getByRole("img", { name: "Billable tokens by model over time" })).toBeVisible();
  await expect(page.locator(".multi-series-chart .chart-line").count()).resolves.toBeGreaterThan(1);
  await expect(page.getByText("Token composition")).toBeVisible();
  await expect(page.getByText("Economics", { exact: true })).toBeVisible();
  await expect(page.getByText("Usage by model")).toBeVisible();
  await expect(page.getByText("Usage by provider")).toBeVisible();
  await expect(page.getByText("Usage by connection")).toBeVisible();
  await expect(page.locator(".usage-dimension-list article").count()).resolves.toBeGreaterThan(1);
  expectClean(runtime);
});

test("Usage ranges request and render distinct measured time domains", async ({ page }) => {
  await loginAs(page, "member");
  const requested: string[] = [];
  await page.route("**/api/v2/usage?*", route => {
    const url = new URL(route.request().url());
    requested.push(`${url.searchParams.get("hours")}/${url.searchParams.get("days")}`);
    const body = {
      scope: "me",
      range_hours: Number(url.searchParams.get("hours")),
      range_days: Number(url.searchParams.get("days")),
      evidence: { kind: "measured", source: "canonical_usage_store", generated_at: "2026-07-27T23:30:00Z" },
      totals: { total_input_tokens: 100, total_cached_tokens: 0, total_output_tokens: 20, total_reasoning_tokens: 0, total_billable_tokens: 120, request_count: 1 },
      hourly: [], daily: [],
      by_model: [{ id: "gpt", provider_id: "codex", requests: 1, input_tokens: 100, cached_tokens: 0, cache_write_tokens: 0, output_tokens: 20, reasoning_tokens: 0, billable_tokens: 120, cost_usd: 0 }],
      by_provider: [{ id: "codex", requests: 1, input_tokens: 100, cached_tokens: 0, cache_write_tokens: 0, output_tokens: 20, reasoning_tokens: 0, billable_tokens: 120, cost_usd: 0 }],
      model_hourly: [{ hour: "2026-07-27T22:00:00Z", model_id: "gpt", provider_id: "codex", billable_tokens: 120, requests: 1 }],
      partial_failures: [],
    };
    return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.goto("/usage");
  await expect(page.getByText("7 days · measured daily billable tokens")).toBeVisible();
  await expect(page.locator(".chart-hover-point")).toHaveCount(7);

  await page.getByRole("button", { name: "24h", exact: true }).click();
  await expect(page.getByText("24 hours · measured hourly billable tokens")).toBeVisible();
  await expect(page.locator(".chart-hover-point")).toHaveCount(24);

  await page.getByRole("button", { name: "30d", exact: true }).click();
  await expect(page.getByText("30 days · measured daily billable tokens")).toBeVisible();
  await expect(page.locator(".chart-hover-point")).toHaveCount(30);
  expect(requested).toEqual(expect.arrayContaining(["168/7", "24/1", "720/30"]));
  await expect(page.getByText("Billable tokens", { exact: true }).last()).toBeVisible();
  await expect(page.getByText("Time", { exact: true })).toBeVisible();
});

test("Member Usage never exposes pool scope or economics", async ({ page }) => {
  await loginAs(page, "member"); await page.goto("/usage");
  await expect(page.getByRole("button", { name: "Pool usage" })).toHaveCount(0);
  await expect(page.getByText("Economics", { exact: true })).toHaveCount(0);
});
