import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

for (const state of ["member", "admin-locked", "admin-elevated"] as const) {
  test(`${state} visual evidence`, async ({ page }, testInfo) => {
    await loginAs(page, state);
    await page.goto("/");
    await page.screenshot({ path: `test-results/review/${testInfo.project.name}-${state}-home.png`, fullPage: true });
    if (state === "admin-elevated") {
      await page.goto("/usage"); await page.getByRole("button", { name: "Pool usage" }).click();
      await page.screenshot({ path: `test-results/review/${testInfo.project.name}-${state}-usage-pool.png`, fullPage: true });
      await page.goto("/admin/connections");
      await page.screenshot({ path: `test-results/review/${testInfo.project.name}-${state}-connections.png`, fullPage: true });
    }
    expect(testInfo.errors).toEqual([]);
  });
}

test("signed-out visual evidence", async ({ page }, testInfo) => {
  await page.goto("/");
  await page.screenshot({ path: `test-results/review/${testInfo.project.name}-signed-out.png`, fullPage: true });
});
