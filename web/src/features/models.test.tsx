import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ModelDescriptor } from "../types";
import { ModelsPage } from "./models";

const models: ModelDescriptor[] = [
  { id: "text", protocol: "openai", provider: "codex", available_now: true },
  { id: "bfl/flux-2-pro", protocol: "openai-images", provider: "bfl", model_kind: "image_generation", input_modalities: ["text"], output_modalities: ["image"], supported_mime_types: ["image/png"], capabilities: { native_image_generation: true }, capability_provenance: "official_provider_contract", capability_verified_at: "2026-08-01", available_now: true },
];

describe("first-class model workloads", () => {
  it("renders workload filtering and separates image output from text models", () => {
    const html = renderToStaticMarkup(<ModelsPage models={models} isElevated={false} />);
    expect(html).toContain("Filter by workload");
    expect(html).toContain("Image generation");
    expect(html).toContain("Text generation");
    expect(html).toContain("native image generation");
  });
});
