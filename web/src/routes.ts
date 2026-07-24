export type View =
  | "home"
  | "models"
  | "usage"
  | "setup"
  | "profile"
  | "admin-connections"
  | "admin-members"
  | "admin-system"
  | "not-found";

export type AppRoute =
  | "/"
  | "/models"
  | "/usage"
  | "/setup"
  | "/profile"
  | "/admin/connections"
  | "/admin/members"
  | "/admin/system"
  | "/not-found";

export type RouteTarget = {
  path: AppRoute;
  view: View;
  adminOnly?: boolean;
  notFound?: boolean;
};

export const ROUTES: readonly RouteTarget[] = [
  { path: "/", view: "home" },
  { path: "/models", view: "models" },
  { path: "/usage", view: "usage" },
  { path: "/setup", view: "setup" },
  { path: "/profile", view: "profile" },
  { path: "/admin/connections", view: "admin-connections", adminOnly: true },
  { path: "/admin/members", view: "admin-members", adminOnly: true },
  { path: "/admin/system", view: "admin-system", adminOnly: true },
  { path: "/not-found", view: "not-found", notFound: true },
];

const byPath = new Map(ROUTES.map(route => [route.path, route]));
const NOT_FOUND: RouteTarget = { path: "/not-found", view: "not-found", notFound: true };

export function routeForPath(pathname: string): RouteTarget {
  return byPath.get(pathname as AppRoute) ?? NOT_FOUND;
}

export type CapabilityStatus =
  | { status: "idle" }
  | { status: "checking" }
  | { status: "resolved"; enrolled: boolean; elevated: boolean; recoveryCodesRemaining: number }
  | { status: "error"; message: string };

export function initialCapability(isAdmin: boolean): CapabilityStatus {
  return isAdmin
    ? { status: "idle" }
    : { status: "resolved", enrolled: false, elevated: false, recoveryCodesRemaining: 0 };
}

export function isElevated(capability: CapabilityStatus): boolean {
  return capability.status === "resolved" && capability.elevated;
}

export function capabilityPending(capability: CapabilityStatus): boolean {
  return capability.status === "idle" || capability.status === "checking";
}

export function currentRoute(): RouteTarget {
  return routeForPath(window.location.pathname);
}

export function navigateTo(route: RouteTarget, replace = false): void {
  window.history[replace ? "replaceState" : "pushState"]({}, "", route.path);
  window.dispatchEvent(new PopStateEvent("popstate"));
}
