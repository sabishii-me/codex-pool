import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

// Test uses the non-authoritative Test environment: only the external link
// should remain, without deployment or authority narration.
test("reset-credit fallback stays concise and contained", async ({ page }, testInfo) => {
  await loginAs(page, "admin-elevated");
  await page.goto("/admin/connections");
  const codex = page.locator(".connection-row").filter({ hasText: "codex" }).first();
  await expect(codex).toBeVisible();
  await codex.click();

  const link = page.locator(".reset-credit-standalone-link");
  await expect(link).toHaveText(/Open ChatGPT/);
  await expect(page.locator(".reset-credit-card")).toHaveCount(0);
  await expect(page.getByText(/Optional fallback|provider-state authority|provider-owned|managed in Production/i)).toHaveCount(0);
  const contained = await link.evaluate(element => {
    const box = element.getBoundingClientRect();
    return box.left >= 0 && box.right <= document.documentElement.clientWidth;
  });
  expect(contained).toBe(true);
  await link.screenshot({ path: testInfo.outputPath("reset-credit-link.png") });
});
