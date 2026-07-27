import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

// Runs once in each configured Playwright project (desktop and mobile).
test("reset-credit state is a contained responsive product card", async ({ page }, testInfo) => {
  await loginAs(page, "admin-elevated");
  await page.goto("/admin/connections");
  const codex = page.locator(".connection-row").filter({ hasText: "codex" }).first();
  await expect(codex).toBeVisible();
  await codex.click();

  const panel = page.locator(".reset-credit-card");
  await expect(panel).toBeVisible();
  await expect(panel.getByRole("heading")).toBeVisible();
  await expect(panel.getByRole("link", { name: /Open ChatGPT/ })).toBeVisible();
  await expect(panel.getByText("Optional fallback:")).toBeVisible();
  const result = await panel.evaluate(element => {
    const box = element.getBoundingClientRect();
    const link = element.querySelector("a");
    const style = getComputedStyle(element);
    const linkStyle = link ? getComputedStyle(link) : null;
    return {
      contained: box.left >= 0 && box.right <= document.documentElement.clientWidth,
      noHorizontalOverflow: element.scrollWidth <= element.clientWidth,
      hasCardBorder: parseFloat(style.borderTopWidth) > 0 && parseFloat(style.borderTopLeftRadius) > 0,
      linkDesigned: Boolean(linkStyle && linkStyle.textDecorationLine !== "none" && parseFloat(linkStyle.fontSize) <= 10),
    };
  });
  expect(result).toEqual({ contained: true, noHorizontalOverflow: true, hasCardBorder: true, linkDesigned: true });
  await panel.screenshot({ path: testInfo.outputPath("reset-credit-panel.png") });
});
