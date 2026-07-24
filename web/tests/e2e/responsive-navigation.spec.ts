import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

test("mobile Member can reach every shared destination", async ({ page }) => {
  await loginAs(page, "member"); await page.goto("/");
  for (const label of ["Home", "Models", "Usage", "Setup"]) await expect(page.locator(".new-nav-link", { hasText: label })).toBeVisible();
});

test("mobile elevated Admin can reach all Admin destinations", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/");
  for (const label of ["Connections", "Members", "System"]) {
    const button = page.locator(".new-nav-link", { hasText: label });
    await expect(button).toBeVisible(); await button.click(); await expect(page.getByRole("heading", { name: label })).toBeVisible();
  }
});
