import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

const resources = [
  ["/admin/connections", "Connections", "Credentialed upstream capacity"],
  ["/admin/members", "Members", "People authorized"],
  ["/admin/system", "System", "Measured runtime"],
] as const;

for (const [route, heading, description] of resources) {
  test(`${heading} is a distinct elevated Admin resource`, async ({ page }) => {
    await loginAs(page, "admin-elevated"); await page.goto(route);
    await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByText(new RegExp(description))).toBeVisible();
  });
}

test("Connections inventory opens provider-neutral detail and uses canonical operations", async ({ page }) => {
  await loginAs(page, "admin-elevated");
  const connections = [{ id: "stable-1", public_id: "public-1", provider_id: "codex", identity: { display_name: "Primary Codex", external_subject: "subject-1", attributes: { email: "owner@example.com" } }, plan_type: "team", disabled: false, dead: false, inflight: 2, last_refresh: "2026-07-22T10:00:00Z", penalty: 0, score: 2, is_primary: true, usage: {}, totals: { total_input_tokens: 10, total_cached_tokens: 4, total_output_tokens: 3, total_billable_tokens: 13 } }];
  await page.route("**/api/v2/provider-connections", route => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(connections) }));
  await page.goto("/admin/connections"); await page.getByRole("button", { name: /Primary Codex/ }).click();
  const detail = page.getByLabel("Connection detail");
  await expect(detail.getByText("public-1")).toBeVisible(); await expect(detail.getByText("subject-1")).toBeVisible(); await expect(detail.getByText("owner@example.com")).toBeVisible();
  await detail.getByRole("button", { name: "Rename connection" }).click();
  await detail.getByLabel("Display name").fill("Renamed Codex");
  const rename = page.waitForRequest(request => request.url().endsWith("/api/v2/provider-connections/stable-1/identity") && request.method() === "PATCH");
  await detail.getByRole("button", { name: "Save name" }).click(); await rename;
  await detail.getByRole("button", { name: "Disable" }).click();
  const confirmation = page.getByRole("alertdialog", { name: "Disable connection?" });
  await expect(confirmation).toBeVisible();
  const disable = page.waitForRequest(request => request.url().endsWith("/api/v2/provider-connections/stable-1/disable") && request.method() === "POST");
  await confirmation.getByRole("button", { name: "Disable connection" }).click(); await disable;
});

test("Connections operation failure remains localized", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/admin/connections");
  const first = page.locator(".connection-row").first(); await first.click();
  await page.route("**/api/v2/provider-connections/*/refresh", route => route.fulfill({ status: 400, contentType: "application/json", body: JSON.stringify({ error: "injected refresh failure" }) }));
  await page.getByLabel("Connection detail").getByRole("button", { name: "Refresh credentials" }).click();
  await expect(page.getByRole("alert")).toContainText("injected refresh failure");
  await expect(page).toHaveURL(/\/admin\/connections$/);
});

test("Connections renders localized error and empty states", async ({ page }) => {
  await loginAs(page, "admin-elevated");
  await page.route("**/api/v2/provider-connections", route => route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "injected connection outage" }) }));
  await page.goto("/admin/connections"); await expect(page.getByText("Projection unavailable")).toBeVisible(); await expect(page.getByText("injected connection outage")).toBeVisible();
  await page.unroute("**/api/v2/provider-connections");
  await page.route("**/api/v2/provider-connections", route => route.fulfill({ status: 200, contentType: "application/json", body: "[]" }));
  await page.reload(); await expect(page.getByText("No provider connections are configured")).toBeVisible();
});

test("Members renders real identities, plans, state, and working administration", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/admin/members");
  await expect(page.locator(".member-row").first()).toContainText("@");
  await expect(page.getByText("Administration projection is limited")).toHaveCount(0);
  await expect(page.getByText(/Dev review data/)).toHaveCount(0);
  await page.getByRole("button", { name: "Add member" }).click();
  await expect(page.getByRole("heading", { name: "Grant gateway access" })).toBeVisible();
  await expect(page.getByLabel("Email")).toBeVisible();
});

test("destructive controls use accessible product dialogs, never browser dialogs", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/admin/members");
  await page.locator(".member-row").filter({ has: page.getByRole("button", { name: "Disable" }) }).first().getByRole("button", { name: "Disable" }).click();
  const dialog = page.getByRole("alertdialog", { name: "Disable member?" });
  await expect(dialog).toBeVisible(); await expect(dialog.getByRole("button", { name: "Cancel" })).toBeFocused();
  await dialog.getByRole("button", { name: "Cancel" }).click(); await expect(dialog).toHaveCount(0);
  await page.goto("/admin/system"); await page.getByRole("button", { name: "Clear rate limits" }).click();
  await expect(page.getByRole("alertdialog", { name: "Clear active rate limits?" })).toBeVisible();
});

test("No active product page advertises missing future implementation", async ({ page }) => {
  await loginAs(page, "admin-elevated");
  for (const route of ["/", "/models", "/usage", "/profile", "/admin/connections", "/admin/members", "/admin/system"]) {
    await page.goto(route);
    const text = await page.locator("main").innerText();
    expect(text).not.toMatch(/not yet exposed|projection is limited|projection pending|shell is ready|no backend projection|routing projection unavailable|dev review data/i);
  }
});

test("System renders measured runtime, persistence, registry, and working operations", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/admin/system");
  for (const label of ["Gateway runtime", "Connection capacity", "Provider registry", "Usage ledger", "Analytics projection", "Gateway users"]) await expect(page.getByText(label, { exact: true }).last()).toBeVisible();
  await expect(page.getByText("No backend projection")).toHaveCount(0);
  const reload = page.waitForRequest(request => request.url().endsWith("/api/v2/system/reload-connections") && request.method() === "POST");
  await page.getByRole("button", { name: "Reload connections" }).click(); await reload;
  await expect(page.getByRole("status")).toContainText("reloaded");
});
