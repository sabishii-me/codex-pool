import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

// Test uses the non-authoritative Test environment. The feature remains a
// complete status section, while actions reflect the capabilities returned by the API.
test("reset-credit status stays understandable and contained", async ({ page }, testInfo) => {
  await loginAs(page, "admin-elevated");
  await page.goto("/admin/connections");
  const codex = page.locator(".connection-row").filter({ hasText: "codex" }).first();
  await expect(codex).toBeVisible();
  await codex.click();

  const panel = page.locator(".reset-credit-card");
  await expect(panel).toBeVisible();
  await expect(panel.getByRole("heading", { name: "Reset credits" })).toBeVisible();
  await expect(panel).toContainText(/Reset-credit status could not be checked|No reset credit was reported|Redemption is not available/);
  await expect(panel.getByRole("link", { name: /Open Codex usage/ })).toHaveAttribute("href", "https://chatgpt.com/codex/settings/usage");
  await expect(panel.getByRole("button", { name: /Check|Redeem/ })).toHaveCount(0);
  await expect(panel.getByText(/Optional fallback|provider-state authority|provider-owned|managed in Production/i)).toHaveCount(0);
  const result = await panel.evaluate(element => {
    const box = element.getBoundingClientRect();
    const style = getComputedStyle(element);
    return {
      contained: box.left >= 0 && box.right <= document.documentElement.clientWidth,
      noHorizontalOverflow: element.scrollWidth <= element.clientWidth,
      hasCardBorder: parseFloat(style.borderTopWidth) > 0 && parseFloat(style.borderTopLeftRadius) > 0,
    };
  });
  expect(result).toEqual({ contained: true, noHorizontalOverflow: true, hasCardBorder: true });
  await panel.screenshot({ path: testInfo.outputPath("reset-credit-panel.png") });
});
