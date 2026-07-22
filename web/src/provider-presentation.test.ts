import { describe, expect, it } from "vitest";
import { providerPresentation } from "./App";

describe("provider presentation", () => {
  it("preserves known provider branding", () => {
    expect(providerPresentation("nvidia")).toMatchObject({ label: "NVIDIA", color: "#76b900" });
  });

  it("renders runtime provider IDs with a neutral fallback", () => {
    expect(providerPresentation("runtime-provider")).toEqual({
      label: "Runtime Provider",
      color: "#8b8b8b",
      dither: "grey",
      glyph: "◇",
    });
  });
});
