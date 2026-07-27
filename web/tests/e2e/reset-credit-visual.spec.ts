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
  await expect(panel.getByText("Optional fallback:")).toHaveCount(0);
  const result = await panel.evaluate(element => {
    const box = element.getBoundingClientRect();
    const link = element.querySelector("a");
    const fallback = element.querySelector(".reset-credit-fallback");
    const style = getComputedStyle(element);
    const fallbackStyle = fallback ? getComputedStyle(fallback) : null;
    const linkStyle = link ? getComputedStyle(link) : null;
    return {
      contained: box.left >= 0 && box.right <= document.documentElement.clientWidth,
      noHorizontalOverflow: element.scrollWidth <= element.clientWidth,
      hasCardBorder: parseFloat(style.borderTopWidth) > 0 && parseFloat(style.borderTopLeftRadius) > 0,
      linkDesigned: Boolean(fallbackStyle && linkStyle && parseFloat(fallbackStyle.fontSize) <= 10 && parseFloat(linkStyle.borderTopWidth) === 0 && linkStyle.backgroundColor === "rgba(0, 0, 0, 0)"),
    };
  });
  expect(result).toEqual({ contained: true, noHorizontalOverflow: true, hasCardBorder: true, linkDesigned: true });
  await panel.screenshot({ path: testInfo.outputPath("reset-credit-panel.png") });
});
