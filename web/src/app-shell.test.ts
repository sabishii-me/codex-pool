import { describe, expect, it } from "vitest";
import { ROUTES, routeForPath } from "./routes";

describe("single inherited shell contract", () => {
  it("keeps shared resources common to members and Admins", () => {
    for (const path of ["/", "/models", "/usage", "/setup", "/profile"]) {
      expect(routeForPath(path).adminOnly).not.toBe(true);
    }
  });

  it("has exactly three Admin-only destinations", () => {
    expect(ROUTES.filter(route => route.adminOnly).map(route => route.path)).toEqual([
      "/admin/connections",
      "/admin/members",
      "/admin/system",
    ]);
  });

  it("does not register Monitor or duplicate Models and Usage routes", () => {
    const paths = ROUTES.map(route => route.path);
    expect(paths.some(path => path.includes("monitor"))).toBe(false);
    expect(paths).not.toContain("/admin/routes");
    expect(paths).not.toContain("/admin/usage");
  });
});
