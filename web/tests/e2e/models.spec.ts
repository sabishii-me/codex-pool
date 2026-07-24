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

test("Elevated Admin selected model gets precise unavailable routing context", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/models"); await selectFirstModel(page);
  await expect(page.getByText("Routing projection unavailable")).toBeVisible();
  await expect(page.getByText(/Eligible connections, selected connection, fallback order/)).toBeVisible();
});
