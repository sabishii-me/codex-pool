import { describe, expect, it } from "vitest";
import { initialRoute, routeForPath, routeForView, routeIsAllowed } from "./routes";

describe("production route adapters", () => {
  it("maps member URLs to existing view implementations", () => {
    expect(routeForPath("/").view).toBe("pulse");
    expect(routeForPath("/models").view).toBe("models");
    expect(routeForPath("/setup").view).toBe("setup");
    expect(routeForPath("/usage").view).toBe("usage");
    expect(routeForPath("/profile").view).toBe("profile");
  });

  it("keeps unknown paths on the member dashboard", () => {
    expect(routeForPath("/not-a-route").path).toBe("/");
    expect(routeForPath("/not-a-route").view).toBe("pulse");
  });

  it("marks operator routes and never treats them as member routes", () => {
    const monitor = routeForPath("/operator/monitor");
    expect(monitor.operatorOnly).toBe(true);
    expect(routeIsAllowed(monitor, false)).toBe(false);
    expect(routeIsAllowed(monitor, true)).toBe(true);
  });

  it("defaults compact operators to Monitor while members remain on Dashboard", () => {
    expect(initialRoute(true, true).path).toBe("/operator/monitor");
    expect(initialRoute(false, true).path).toBe("/");
    expect(initialRoute(true, false).path).toBe("/operator");
  });

  it("adapts existing operator views behind stable routes", () => {
    expect(routeForView("accounts", true).path).toBe("/operator/connections");
    expect(routeForView("insights", true).path).toBe("/operator/monitor");
    expect(routeForView("models", false).path).toBe("/models");
  });
});
