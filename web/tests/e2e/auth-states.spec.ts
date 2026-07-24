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
    if (state === "admin-locked") await expect(page.getByText("Admin controls are locked")).toBeVisible();
    if (state === "admin-elevated") await expect(page.getByText("Admin attention")).toBeVisible();
    if (state === "member") await expect(page.getByText("Admin attention")).toHaveCount(0);
    expectClean(runtime);
  });
}

test("locked Admin receives a distinct locked Administration resource", async ({ page }) => {
  await loginAs(page, "admin-locked"); const response = await page.goto("/admin/connections"); expect(response?.status()).toBe(200); await expect(page.getByRole("heading", { name: "Connections", exact: true })).toBeVisible(); await expect(page.getByRole("heading", { name: "Connections is locked" })).toBeVisible(); await expect(page.getByRole("button", { name: "Unlock in Profile" })).toBeVisible();
});

test("member cannot admit a protected route", async ({ page }) => {
  await loginAs(page, "member"); await page.goto("/admin/system"); await expect(page.getByRole("heading", { name: "Home" })).toBeVisible();
});
