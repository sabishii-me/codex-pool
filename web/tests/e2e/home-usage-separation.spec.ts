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
  await expect(page.getByText("Pool request history")).toBeVisible();
  await expect(page.getByText("Token composition")).toBeVisible();
  await expect(page.getByText("Economics", { exact: true })).toBeVisible();
  expectClean(runtime);
});

test("Member Usage never exposes pool scope or economics", async ({ page }) => {
  await loginAs(page, "member"); await page.goto("/usage");
  await expect(page.getByRole("button", { name: "Pool usage" })).toHaveCount(0);
  await expect(page.getByText("Economics", { exact: true })).toHaveCount(0);
});
