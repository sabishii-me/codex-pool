import { expect, test } from "@playwright/test";
import { e2eConfig } from "./helpers/session";

test("Google login entry uses the registered dev callback", async ({ request }) => {
  const response = await request.get("/auth/login/google", { maxRedirects: 0 });
  expect(response.status()).toBe(302);
  const location = response.headers().location; expect(location).toBeTruthy();
  const url = new URL(location!);
  expect(url.hostname).toBe("accounts.google.com");
  expect(url.searchParams.get("redirect_uri")).toBe(`${e2eConfig().baseURL}/auth/callback/google`);
  expect(url.searchParams.get("client_id")).toBeTruthy();
});

test("signed-out login control targets the backend OAuth entry", async ({ page }) => {
  await page.goto("/"); await expect(page.getByRole("link", { name: "Continue with Google" })).toHaveAttribute("href", "/auth/login/google");
});
