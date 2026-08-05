import { expect, test } from "@playwright/test";
import { loginAs } from "./helpers/acceptance";

async function selectFirstModel(page: import("@playwright/test").Page) {
  const row = page.locator(".model-row-new").first(); await expect(row).toBeVisible(); await row.click();
}

test("Models exposes first-class workload filters and native image metadata", async ({ page }) => {
  await loginAs(page, "admin-elevated");
  await page.route("**/api/pool/catalog", route => route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ models: [
    { id: "text", provider: "codex", protocol: "openai", available_now: true },
    { id: "bfl/flux-2-pro", provider: "bfl", protocol: "openai-images", model_kind: "image_generation", input_modalities: ["text"], output_modalities: ["image"], supported_mime_types: ["image/png"], capabilities: { native_image_generation: true }, capability_provenance: "official_provider_openapi", capability_verified_at: "2026-08-02", available_now: true },
  ] }) }));
  await page.goto("/models");
  await page.getByLabel("Filter by workload").selectOption("image_generation");
  await expect(page.locator(".model-row-new")).toHaveCount(1);
  await page.getByRole("button", { name: /bfl\/flux-2-pro/ }).click();
  const detail = page.locator(".model-detail-panel");
  await expect(detail.getByText("Image generation", { exact: true })).toBeVisible();
  await expect(detail.getByText("image/png", { exact: true })).toBeVisible();
  await expect(detail.getByText(/official provider openapi/)).toBeVisible();
});

test("Member sees shared model inventory without routing context", async ({ page }) => {
  await loginAs(page, "member"); await page.goto("/models"); await selectFirstModel(page);
  await expect(page.getByText("Selected model")).toBeVisible();
  await expect(page.getByText("Routing projection unavailable")).toHaveCount(0);
});

test("Elevated Admin sees backend-owned runtime routing context", async ({ page }) => {
  await loginAs(page, "admin-elevated"); await page.goto("/models"); await selectFirstModel(page);
  await expect(page.getByText("Runtime routing")).toBeVisible();
  await expect(page.getByText("Per request")).toBeVisible();
  await expect(page.getByText("Eligible connections")).toBeVisible();
  await expect(page.getByText("Routing projection unavailable")).toHaveCount(0);
  await expect(page.getByText(/fallback order/i)).toHaveCount(0);
});

test("Member never requests protected model routing", async ({ page }) => {
  let routingRequests = 0; page.on("request", request => { if (request.url().includes("/api/v2/models/") && request.url().endsWith("/routing")) routingRequests++; });
  await loginAs(page, "member"); await page.goto("/models"); await selectFirstModel(page);
  expect(routingRequests).toBe(0);
});
