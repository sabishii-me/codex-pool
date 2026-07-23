import type { ReactNode } from "react";
import type { View } from "./routes";

export type AppShellProps = {
  view: View;
  routePath: string;
  operator: boolean;
  header: ReactNode;
  navigation: ReactNode;
  children: ReactNode;
};

/**
 * Stable migration boundary for the Phase 9 shell.
 * Feature pages remain owned by the legacy implementations until each route
 * has a replacement and parity evidence. The shell owns only workspace
 * context, landmarks, and the layout surface around those pages.
 */
export function AppShell({ view, routePath, operator, header, navigation, children }: AppShellProps) {
  return (
    <div
      className="signal-app app-shell-boundary"
      data-app-shell="phase-9"
      data-workspace={operator ? "operator" : "member"}
      data-route={routePath}
      data-view={view}
    >
      <div className="signal-noise" aria-hidden="true" />
      {header}
      <div className="app-grid">
        {navigation}
        <main className="signal-main" id="main-content" tabIndex={-1}>
          {children}
        </main>
      </div>
    </div>
  );
}
