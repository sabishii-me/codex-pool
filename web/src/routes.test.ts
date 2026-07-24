import { describe, expect, it } from "vitest";
import { capabilityPending, initialCapability, isElevated, routeForPath } from "./routes";

describe("current product routes", () => {
  it("maps the five shared member resources", () => {
    expect(routeForPath("/").view).toBe("home");
    expect(routeForPath("/models").view).toBe("models");
    expect(routeForPath("/usage").view).toBe("usage");
    expect(routeForPath("/setup").view).toBe("setup");
    expect(routeForPath("/profile").view).toBe("profile");
  });

  it("contains only the three additional Admin resources", () => {
    expect(routeForPath("/admin/connections")).toMatchObject({ view: "admin-connections", adminOnly: true });
    expect(routeForPath("/admin/members")).toMatchObject({ view: "admin-members", adminOnly: true });
    expect(routeForPath("/admin/system")).toMatchObject({ view: "admin-system", adminOnly: true });
  });

  it.each(["/operator", "/operator/monitor", "/operator/routes", "/operator/usage", "/admin/routes", "/admin/usage", "/admin/monitor", "/unknown"])("treats %s as not found without a compatibility fallback", path => {
    expect(routeForPath(path)).toMatchObject({ path: "/not-found", view: "not-found", notFound: true });
  });
});

describe("capability progression", () => {
  it("resolves a member without Admin elevation", () => {
    const capability = initialCapability(false);
    expect(capabilityPending(capability)).toBe(false);
    expect(isElevated(capability)).toBe(false);
  });

  it("keeps Admin identity unresolved until backend MFA status arrives", () => {
    const capability = initialCapability(true);
    expect(capabilityPending(capability)).toBe(true);
    expect(isElevated(capability)).toBe(false);
  });

  it("requires resolved backend elevation", () => {
    expect(isElevated({ status: "resolved", enrolled: true, elevated: false, recoveryCodesRemaining: 8 })).toBe(false);
    expect(isElevated({ status: "resolved", enrolled: true, elevated: true, recoveryCodesRemaining: 8 })).toBe(true);
  });
});
