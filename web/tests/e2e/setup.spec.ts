import { expect, test } from "@playwright/test";
import { expectClean, loginAs, watchRuntime } from "./helpers/acceptance";

for (const client of ["Codex CLI", "Claude Code", "Gemini CLI", "Grok Build", "Pi"]) {
  test(`Setup exposes backend-owned ${client} workflow`, async ({ page }) => {
    await loginAs(page, "member");
    const runtime = watchRuntime(page);
    await page.goto("/setup");
    await expect(page.getByRole("heading", { name: "Choose your client" })).toBeVisible();
    await page.getByRole("button", { name: new RegExp(`^${client}`) }).click();
    await expect(page.getByRole("heading", { name: "Choose your environment" })).toBeVisible();
    await page.getByRole("button", { name: /^macOS or Linux/ }).click();
    await expect(page.getByRole("heading", { name: "Install and configure" })).toBeVisible();
    await expect(page.getByText(/curl -fsSL .*\/setup\//)).toBeVisible();
    await page.getByRole("button", { name: "Continue to verification" }).click();
    await expect(page.getByRole("heading", { name: "Verify locally" })).toBeVisible();
    await expect(page.getByText(/--version/)).toBeVisible();
    expectClean(runtime);
  });
}

test("Setup selects a backend-owned PowerShell workflow and has no removed client", async ({ page }) => {
  await loginAs(page, "member");
  await page.goto("/setup");
  await expect(page.getByText(/Cute Code/i)).toHaveCount(0);
  await page.getByRole("button", { name: /^Claude Code/ }).click();
  await page.getByRole("button", { name: /^Windows/ }).click();
  await expect(page.getByText(/Invoke-Expression \(Invoke-RestMethod .*shell=powershell/)).toBeVisible();
  await expect(page.getByText(/%USERPROFILE%\\\.claude\\settings\.json/)).toBeVisible();
});

test("removed Cute Code routes are not compatibility aliases", async ({ request }) => {
  for (const path of ["/cute-code", "/setup/cute-code/token", "/config/cute-code/token"]) {
    const response = await request.get(path);
    expect(response.status()).toBe(404);
  }
});
