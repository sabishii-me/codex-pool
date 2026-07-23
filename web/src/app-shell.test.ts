import { describe, expect, it } from "vitest";
import { initialRoute, routeForPath } from "./routes";

describe("Phase 9 shell contract", () => {
  it("has stable member and operator workspace entry routes", () => {
    expect(routeForPath("/").operatorOnly).not.toBe(true);
    expect(routeForPath("/operator").operatorOnly).toBe(true);
    expect(routeForPath("/operator/monitor").view).toBe("insights");
  });

  it("selects the compact operator monitor entry without changing member entry", () => {
    expect(initialRoute(true, true)).toMatchObject({ path: "/operator/monitor", operatorOnly: true });
    expect(initialRoute(false, true)).toMatchObject({ path: "/", view: "pulse" });
  });

  it("keeps route adapters explicit while feature migration is incomplete", () => {
    expect(routeForPath("/operator/connections")).toMatchObject({ view: "accounts", operatorOnly: true });
    expect(routeForPath("/operator/routes")).toMatchObject({ view: "models", operatorOnly: true });
  });
});
