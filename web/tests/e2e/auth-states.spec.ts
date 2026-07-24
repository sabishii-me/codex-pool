import { expect, test } from "@playwright/test";
import { expectClean, loginAs, watchRuntime } from "./helpers/acceptance";

const memberLinks = ["Home", "Models", "Usage", "Setup", "Profile"];
const adminLinks = ["Connections", "Members", "System"];

for (const state of ["member", "admin-locked", "admin-elevated"] as const) {
  test(`${state} renders the authorized capability state`, async ({ page }) => {
    await loginAs(page, state); const runtime = watchRuntime(page); await page.goto("/");
    for (const label of memberLinks) await expect(page.getByRole("button", { name: new RegExp(label, "i") }).first()).toBeVisible();
    for (const label of adminLinks) {
      const locator = page.getByRole("button", { name: new RegExp(label, "i") });
      if (state === "member") await expect(locator).toHaveCount(0); else await expect(locator).toBeVisible();
    }
    if (state === "admin-locked") await expect(page.getByRole("dialog")).toHaveCount(0);
    if (state === "admin-elevated") await expect(page.getByText("Admin attention")).toBeVisible();
    if (state === "member") await expect(page.getByText("Admin attention")).toHaveCount(0);
    expectClean(runtime);
  });
}

test("locked Admin opens one full-screen MFA dialog and stays on the current page", async ({ page }) => {
  await loginAs(page, "admin-locked"); await page.goto("/");
  await page.getByRole("button", { name: /Connections/i }).click();
  const dialog = page.getByRole("dialog", { name: "Unlock connections" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel("MFA verification code")).toBeFocused();
  await expect(page.getByRole("heading", { name: "Home", exact: true })).toBeVisible();
  await expect(page).toHaveURL(/\/$/);
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
});

test("locked Admin deep link opens the same MFA dialog without rendering a warning page", async ({ page }) => {
  await loginAs(page, "admin-locked"); const response = await page.goto("/admin/connections"); expect(response?.status()).toBe(200);
  await expect(page.getByRole("dialog", { name: "Unlock connections" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Home", exact: true })).toBeVisible();
  await expect(page.getByText("Connections is locked")).toHaveCount(0);
});

test("member cannot admit a protected route", async ({ page }) => {
  await loginAs(page, "member"); await page.goto("/admin/system"); await expect(page.getByRole("heading", { name: "Home" })).toBeVisible();
});
