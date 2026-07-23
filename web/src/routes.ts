export type View = "pulse" | "insights" | "usage" | "accounts" | "models" | "setup" | "profile";

export type AppRoute =
  | "/"
  | "/models"
  | "/setup"
  | "/usage"
  | "/profile"
  | "/operator"
  | "/operator/monitor"
  | "/operator/connections"
  | "/operator/routes"
  | "/operator/usage"
  | "/operator/members"
  | "/operator/system";

export type RouteTarget = {
  path: AppRoute;
  view: View;
  operatorOnly?: boolean;
};

export const ROUTES: readonly RouteTarget[] = [
  { path: "/", view: "pulse" },
  { path: "/models", view: "models" },
  { path: "/setup", view: "setup" },
  { path: "/usage", view: "usage" },
  { path: "/profile", view: "profile" },
  { path: "/operator", view: "pulse", operatorOnly: true },
  // These adapters keep the existing feature implementations reachable while
  // the replacement operator pages are migrated incrementally.
  { path: "/operator/monitor", view: "insights", operatorOnly: true },
  { path: "/operator/connections", view: "accounts", operatorOnly: true },
  { path: "/operator/routes", view: "models", operatorOnly: true },
  { path: "/operator/usage", view: "usage", operatorOnly: true },
  { path: "/operator/members", view: "accounts", operatorOnly: true },
  { path: "/operator/system", view: "insights", operatorOnly: true },
];

const byPath = new Map(ROUTES.map((route) => [route.path, route]));
const byView = new Map(ROUTES.filter((route) => !route.operatorOnly).map((route) => [route.view, route]));

export function routeForPath(pathname: string): RouteTarget {
  return byPath.get(pathname as AppRoute) ?? byPath.get("/")!;
}

export function routeForView(view: View, operator = false): RouteTarget {
  if (operator) {
    const operatorRoute = ROUTES.find((route) => route.operatorOnly && route.view === view);
    if (operatorRoute) return operatorRoute;
  }
  return byView.get(view) ?? byPath.get("/")!;
}

export function initialRoute(isOperator: boolean, compact: boolean): RouteTarget {
  if (compact && isOperator) return routeForPath("/operator/monitor");
  return routeForPath(isOperator ? "/operator" : "/");
}

export function routeIsAllowed(route: RouteTarget, isOperator: boolean): boolean {
  return !route.operatorOnly || isOperator;
}

export function isCompactViewport(): boolean {
  return window.matchMedia("(max-width: 760px)").matches;
}

export function currentRoute(): RouteTarget {
  return routeForPath(window.location.pathname);
}

export function navigateTo(route: RouteTarget, replace = false): void {
  const method = replace ? "replaceState" : "pushState";
  window.history[method]({}, "", route.path);
  window.dispatchEvent(new PopStateEvent("popstate"));
}
