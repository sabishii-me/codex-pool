import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

const resources = [
  ["/admin/connections", "Connections", "Credentialed upstream capacity"],
  ["/admin/members", "Members", "Gateway user identities"],
  ["/admin/system", "System", "Platform state"],
] as const;

for (const [route, heading, description] of resources) {
  test(`${heading} is a distinct elevated Admin resource`, async ({ page }) => {
    await loginAs(page, "admin-elevated"); await page.goto(route);
    await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await expect(page.getByText(new RegExp(description))).toBeVisible();
  });
}

test("Connections renders localized error and empty states", async ({ page }) => {
  await loginAs(page, "admin-elevated");
  await page.route("**/api/v2/provider-connections", route => route.fulfill({ status: 503, contentType: "application/json", body: JSON.stringify({ error: "injected connection outage" }) }));
  await page.goto("/admin/connections"); await expect(page.getByText("Projection unavailable")).toBeVisible(); await expect(page.getByText("injected connection outage")).toBeVisible();
  await page.unroute("**/api/v2/provider-connections");
  await page.route("**/api/v2/provider-connections", route => route.fulfill({ status: 200, contentType: "application/json", body: "[]" }));
  await page.reload(); await expect(page.getByText("No provider connections are configured")).toBeVisible();
});

test("System does not fabricate missing projections", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/admin/system");
  for (const label of ["Persistence health", "Projection freshness", "Background jobs", "Configuration revision", "Recovery readiness"]) await expect(page.getByText(label)).toBeVisible();
  await expect(page.getByText("No backend projection").first()).toBeVisible();
});
