import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const productFiles = [
  "src/App.tsx",
  "src/components/ui.tsx",
  "src/features/dashboard.tsx",
  "src/features/models.tsx",
  "src/features/usage.tsx",
  "src/features/profile.tsx",
  "src/features/admin.tsx",
];

describe("product truth boundary", () => {
  it("contains no synthetic chart series or visible implementation placeholders", () => {
    const source = productFiles.map(path => readFileSync(path, "utf8")).join("\n");
    expect(source).not.toContain("[12, 18, 14, 23");
    expect(source).not.toMatch(/dev review data|projection is limited|not yet exposed|shell is ready|no backend projection|routing projection unavailable|example only/i);
  });

  it("does not use browser-owned dialogs", () => {
    const source = productFiles.map(path => readFileSync(path, "utf8")).join("\n");
    expect(source).not.toMatch(/window\.(confirm|alert|prompt)\s*\(/);
  });
});
