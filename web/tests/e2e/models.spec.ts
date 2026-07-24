import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

async function selectFirstModel(page: import("@playwright/test").Page) {
  const row = page.locator(".model-row-new").first(); await expect(row).toBeVisible(); await row.click();
}

test("Member sees shared model inventory without routing context", async ({ page }) => {
  await loginAs(page, "member"); await page.goto("/models"); await selectFirstModel(page);
  await expect(page.getByText("Selected model")).toBeVisible();
  await expect(page.getByText("Routing projection unavailable")).toHaveCount(0);
});

test("Elevated Admin sees backend-owned runtime routing context", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/models"); await selectFirstModel(page);
  await expect(page.getByText("Runtime routing")).toBeVisible();
  await expect(page.getByText("Per request")).toBeVisible();
  await expect(page.getByText("Eligible connections")).toBeVisible();
  await expect(page.getByText("Routing projection unavailable")).toHaveCount(0);
  await expect(page.getByText(/fallback order/i)).toHaveCount(0);
});

test("Member never requests protected model routing", async ({ page }) => {
  let routingRequests = 0; page.on("request", request => { if (request.url().includes("/api/v2/models/") && request.url().endsWith("/routing")) routingRequests++; });
  await loginAs(page, "member"); await page.goto("/models"); await selectFirstModel(page);
  expect(routingRequests).toBe(0);
});
