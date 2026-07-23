import { useCallback, useEffect, useState } from "react";
import {
  loadDashboardResources,
  loadSession,
  logout,
} from "./api";
import type { FriendSession, ModelDescriptor, PoolStats, SignalAnalytics } from "./types";
import { currentRoute, initialRoute, isCompactViewport, navigateTo, routeForPath, type AppRoute } from "./routes";

export function providerPresentation(provider: string) {
  if (provider === "nvidia") return { label: "NVIDIA", color: "#76b900", dither: "green", glyph: "◓" as const };
  const label = provider.split("-").filter(Boolean).map(part => part.charAt(0).toUpperCase() + part.slice(1)).join(" ") || "Unknown";
  return { label, color: "#8b8b8b", dither: "grey", glyph: "◇" as const };
}
import { Page } from "./features/pages";

export function App() {
  const [session, setSession] = useState<FriendSession | null>(null);
  const [booting, setBooting] = useState(true);
  const [route, setRoute] = useState(routeForPath(window.location.pathname));
  const [stats, setStats] = useState<PoolStats | null>(null);
  const [signal, setSignal] = useState<SignalAnalytics | null>(null);
  const [models, setModels] = useState<ModelDescriptor[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const operator = Boolean(session?.is_admin);
  const refresh = useCallback(async () => {
    setLoading(true);
    const resources = await loadDashboardResources();
    if (resources.stats) setStats(resources.stats);
    if (resources.signal) setSignal(resources.signal);
    if (resources.catalog) setModels(resources.catalog.models);
    setError(resources.errors.join(" · "));
    setLoading(false);
  }, []);

  useEffect(() => {
    loadSession().then(setSession).catch(() => setSession(null)).finally(() => setBooting(false));
  }, []);

  useEffect(() => {
    if (!session) return;
    refresh();
    const timer = window.setInterval(refresh, 30_000);
    return () => window.clearInterval(timer);
  }, [session, refresh]);

  useEffect(() => {
    const onPop = () => setRoute(routeForPath(window.location.pathname));
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  useEffect(() => {
    if (!session) return;
    const incoming = currentRoute();
    if (incoming.operatorOnly && !operator) {
      const fallback = initialRoute(false, isCompactViewport());
      navigateTo(fallback, true);
      setRoute(fallback);
      return;
    }
    if (incoming.path === "/" && operator && isCompactViewport()) {
      const compactHome = initialRoute(true, true);
      navigateTo(compactHome, true);
      setRoute(compactHome);
    }
  }, [session, operator]);

  const go = (path: AppRoute) => {
    const target = routeForPath(path);
    if (target.operatorOnly && !operator) return;
    navigateTo(target);
    setRoute(target);
  };

  if (booting) return <div className="new-boot"><span>AI POOL</span><small>Loading workspace</small></div>;
  if (!session) return <AccessGate />;

  const signOut = async () => {
    await logout();
    setSession(null);
    navigateTo(routeForPath("/"), true);
  };

  return (
    <div className="new-app" data-workspace={operator ? "operator" : "member"} data-route={route.path}>
      <Topbar session={session} operator={operator} loading={loading} onRefresh={refresh} />
      <div className="new-layout">
        <Sidebar route={route.path} operator={operator} onNavigate={go} onSignOut={signOut} email={session.email} />
        <main className="new-main" id="main-content" tabIndex={-1}>
          {error && <div className="new-alert" role="alert"><b>Data refresh incomplete</b><span>{error}</span></div>}
          <Page route={route.path} stats={stats} signal={signal} models={models} session={session} onNavigate={go} />
        </main>
      </div>
    </div>
  );
}

function AccessGate() {
  return <div className="new-access"><div className="access-card"><span className="logo-mark">AI</span><p className="kicker">Private model gateway</p><h1>Welcome to AI Pool</h1><p>One gateway for the models your workspace can use.</p><a className="primary-button" href="/auth/login/google">Continue with Google</a></div></div>;
}

function Topbar({ session, operator, loading, onRefresh }: { session: FriendSession; operator: boolean; loading: boolean; onRefresh: () => void }) {
  return <header className="new-topbar"><div className="brand"><span className="logo-mark">AI</span><span><b>AI Pool</b><small>Model gateway</small></span></div><div className="topbar-context"><span className="live-dot" />{operator ? "Operations" : "Workspace"}<strong>{operator ? "Live gateway monitor" : "Gateway overview"}</strong></div><div className="topbar-tools"><span className="live-label"><span className="live-dot" />Live</span><span className="updated">{session.email}</span><button className="icon-button" onClick={onRefresh} disabled={loading} aria-label="Refresh data">↻</button></div></header>;
}

function Sidebar({ route, operator, onNavigate, onSignOut, email }: { route: string; operator: boolean; onNavigate: (path: AppRoute) => void; onSignOut: () => void; email: string }) {
  const member = [["/", "Overview", "⌂"], ["/models", "Models", "◇"], ["/setup", "Setup", "↗"], ["/usage", "My usage", "▥"]] as const;
  const ops = [["/operator/monitor", "Monitor", "⌁"], ["/operator/connections", "Connections", "⇄"], ["/operator/routes", "Model routes", "⑂"], ["/operator/usage", "Usage & economics", "◫"], ["/operator/members", "Members", "◎"], ["/operator/system", "System", "⚙"]] as const;
  return <aside className="new-sidebar"><div className="sidebar-section"><span className="section-label">Workspace</span>{member.map(([path, label, icon]) => <NavButton key={path} path={path} current={route} label={label} icon={icon} onNavigate={onNavigate} />)}</div>{operator && <div className="sidebar-section operator-nav"><span className="section-label">Operations</span>{ops.map(([path, label, icon]) => <NavButton key={path} path={path} current={route} label={label} icon={icon} onNavigate={onNavigate} />)}</div>}<div className="sidebar-bottom"><button className="profile-link" onClick={() => onNavigate("/profile")}><span className="avatar">{email.slice(0, 2).toUpperCase()}</span><span><b>Profile</b><small>{email}</small></span></button><button className="signout-link" onClick={onSignOut}>Sign out</button></div></aside>;
}

function NavButton({ path, current, label, icon, onNavigate }: { path: string; current: string; label: string; icon: string; onNavigate: (path: AppRoute) => void }) {
  return <button className={`new-nav-link ${current === path ? "active" : ""}`} onClick={() => onNavigate(path as AppRoute)} aria-current={current === path ? "page" : undefined}><span>{icon}</span>{label}</button>;
}
