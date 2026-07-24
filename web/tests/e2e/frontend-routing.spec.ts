import { expect, test } from "@playwright/test";
import { canonicalRoutes, expectClean, loginAs, watchRuntime } from "./helpers/acceptance";

const headings: Record<string, string> = { "/": "Home", "/models": "Models", "/usage": "Usage", "/setup": "Setup", "/profile": "Profile", "/admin/connections": "Connections", "/admin/members": "Members", "/admin/system": "System" };

test.describe("frontend navigation boundary", () => {
  for (const route of canonicalRoutes) {
    test(`signed-out direct load ${route} serves the login shell`, async ({ page }) => {
      const runtime = watchRuntime(page);
      const response = await page.goto(route);
      expect(response?.status()).toBe(200);
      expect(response?.headers()["content-type"]).toContain("text/html");
      await expect(page.getByRole("heading", { name: "Welcome to AI Pool" })).toBeVisible();
      await expect(page.getByRole("link", { name: "Continue with Google" })).toBeVisible();
      expectClean(runtime, { allowSignedOutSession401: true });
    });
  }

  for (const route of canonicalRoutes.slice(0, 5)) {
    test(`member direct load and reload ${route}`, async ({ page }) => {
      await loginAs(page, "member"); const runtime = watchRuntime(page);
      let response = await page.goto(route); expect(response?.status()).toBe(200); await expect(page.getByRole("heading", { name: headings[route], exact: true })).toBeVisible();
      response = await page.reload(); expect(response?.status()).toBe(200); await expect(page.getByRole("heading", { name: headings[route], exact: true })).toBeVisible(); expectClean(runtime);
    });
  }

  for (const route of canonicalRoutes.slice(5)) {
    test(`elevated Admin direct load and reload ${route}`, async ({ page }) => {
      await loginAs(page, "admin-elevated"); const runtime = watchRuntime(page);
      let response = await page.goto(route); expect(response?.status()).toBe(200); await expect(page.getByRole("heading", { name: headings[route], exact: true })).toBeVisible();
      response = await page.reload(); expect(response?.status()).toBe(200); await expect(page.getByRole("heading", { name: headings[route], exact: true })).toBeVisible(); expectClean(runtime);
    });
  }

  test("discarded routes render React not-found without redirect aliases", async ({ page }) => {
    await loginAs(page, "member");
    for (const route of ["/operator", "/operator/monitor", "/admin/routes", "/admin/usage", "/admin/monitor"]) {
      const response = await page.goto(route); expect(response?.status()).toBe(200); expect(page.url()).toContain(route); await expect(page.getByRole("heading", { name: "Page not found" })).toBeVisible();
    }
  });

  test("API boundary remains API while frontend route is HTML", async ({ request }) => {
    const frontend = await request.get("/usage", { headers: { Accept: "text/html" } }); expect(frontend.status()).toBe(200); expect(frontend.headers()["content-type"]).toContain("text/html");
    const api = await request.get("/api/pool/session"); expect(api.status()).toBe(401); expect(api.headers()["content-type"]).toContain("application/json");
    const proxy = await request.get("/v1/models", { headers: { Accept: "application/json" } }); expect(proxy.headers()["content-type"] ?? "").not.toContain("text/html");
  });
});
