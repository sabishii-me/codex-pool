import { useCallback, useEffect, useRef, useState } from "react";
import { checkMFAStatus, loadDashboardResources, loadGatewayHealth, loadPoolUsers, loadProviderConnectionsV2, loadSession, logout } from "./api";
import type { FriendSession, GatewayHealth, ModelDescriptor, OperatorProviderConnectionV2, PoolStats, PoolUserStats, SignalAnalytics } from "./types";
import type { ResourceState } from "./resource-state";
import { capabilityPending, currentRoute, initialCapability, isElevated, navigateTo, routeForPath, type AppRoute, type CapabilityStatus } from "./routes";
import { Page } from "./features/pages";

export function providerPresentation(provider: string) {
  if (provider === "nvidia") return { label: "NVIDIA", color: "#76b900", dither: "green", glyph: "◓" as const };
  const label = provider.split("-").filter(Boolean).map(part => part.charAt(0).toUpperCase() + part.slice(1)).join(" ") || "Unknown";
  return { label, color: "#8b8b8b", dither: "grey", glyph: "◇" as const };
}

export function App() {
  const [session, setSession] = useState<FriendSession | null>(null);
  const [booting, setBooting] = useState(true);
  const [route, setRoute] = useState(routeForPath(window.location.pathname));
  const [stats, setStats] = useState<PoolStats | null>(null);
  const [signal, setSignal] = useState<SignalAnalytics | null>(null);
  const [models, setModels] = useState<ModelDescriptor[]>([]);
  const [connections, setConnections] = useState<ResourceState<OperatorProviderConnectionV2[]>>({ status: "idle" });
  const [users, setUsers] = useState<ResourceState<PoolUserStats[]>>({ status: "idle" });
  const [health, setHealth] = useState<ResourceState<GatewayHealth>>({ status: "idle" });
  const [capability, setCapability] = useState<CapabilityStatus>({ status: "idle" });
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const capabilityGeneration = useRef(0);

  const elevated = isElevated(capability);
  const clearProtected = useCallback(() => { setConnections({ status: "idle" }); setUsers({ status: "idle" }); setHealth({ status: "idle" }); }, []);
  const refreshCapability = useCallback(async (initial = false) => {
    if (!session?.is_admin) return;
    const generation = ++capabilityGeneration.current;
    if (initial) setCapability({ status: "checking" });
    try {
      const status = await checkMFAStatus();
      if (generation !== capabilityGeneration.current) return;
      setCapability({ status: "resolved", enrolled: status.enrolled, elevated: status.elevated, recoveryCodesRemaining: status.recovery_codes_remaining });
      if (!status.elevated) clearProtected();
    } catch (error) {
      if (generation !== capabilityGeneration.current) return;
      clearProtected();
      setCapability({ status: "error", message: error instanceof Error ? error.message : "MFA status unavailable" });
    }
  }, [session?.is_admin, clearProtected]);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const dashboard = await loadDashboardResources();
      if (dashboard.stats) setStats(dashboard.stats);
      if (dashboard.signal) setSignal(dashboard.signal);
      if (dashboard.catalog) setModels(dashboard.catalog.models);
      setErrors(dashboard.errors);
      if (!elevated) { clearProtected(); return; }
      setConnections({ status: "loading" }); setUsers({ status: "loading" }); setHealth({ status: "loading" });
      const [connectionResult, userResult, healthResult] = await Promise.allSettled([loadProviderConnectionsV2(), loadPoolUsers(), loadGatewayHealth()]);
      setConnections(connectionResult.status === "fulfilled" ? (connectionResult.value.length ? { status: "ready", data: connectionResult.value } : { status: "empty" }) : { status: "error", message: connectionResult.reason instanceof Error ? connectionResult.reason.message : "Connections unavailable" });
      setUsers(userResult.status === "fulfilled" ? (userResult.value.users.length ? { status: "ready", data: userResult.value.users } : { status: "empty" }) : { status: "error", message: userResult.reason instanceof Error ? userResult.reason.message : "Members unavailable" });
      setHealth(healthResult.status === "fulfilled" ? { status: "ready", data: healthResult.value } : { status: "error", message: healthResult.reason instanceof Error ? healthResult.reason.message : "Runtime health unavailable" });
    } finally { setLoading(false); }
  }, [elevated, clearProtected]);

  useEffect(() => { loadSession().then(setSession).catch(() => setSession(null)).finally(() => setBooting(false)); }, []);
  useEffect(() => {
    if (!session) return;
    setCapability(initialCapability(session.is_admin));
    if (session.is_admin) void refreshCapability(true);
    return () => { capabilityGeneration.current++; };
  }, [session?.email, session?.is_admin, refreshCapability]);
  useEffect(() => { if (!session) return; void refresh(); const timer = window.setInterval(refresh, 30_000); return () => window.clearInterval(timer); }, [session, refresh]);
  useEffect(() => { const onPop = () => setRoute(routeForPath(window.location.pathname)); window.addEventListener("popstate", onPop); return () => window.removeEventListener("popstate", onPop); }, []);
  useEffect(() => {
    if (!session || capabilityPending(capability)) return;
    const incoming = currentRoute();
    if (incoming.adminOnly && !session.is_admin) { const home = routeForPath("/"); navigateTo(home, true); setRoute(home); }
  }, [session, capability, elevated]);

  const go = (path: AppRoute) => { const target = routeForPath(path); if (target.adminOnly && !session?.is_admin) return; navigateTo(target); setRoute(target); };
  const signOut = async () => { await logout(); setSession(null); const home = routeForPath("/"); navigateTo(home, true); setRoute(home); };

  if (booting) return <div className="new-boot"><span>AI POOL</span><small>Loading workspace</small></div>;
  if (!session) return <AccessGate />;

  return <div className="new-app" data-admin={session.is_admin ? "true" : "false"} data-elevated={elevated ? "true" : "false"} data-route={route.path}>
    <Topbar session={session} isAdmin={session.is_admin} elevated={elevated} loading={loading} onRefresh={refresh} />
    <div className="new-layout"><Sidebar route={route.path} isAdmin={session.is_admin} elevated={elevated} onNavigate={go} onSignOut={signOut} email={session.email} />
      <main className="new-main" id="main-content" tabIndex={-1}>
        {errors.length ? <div className="new-alert" role="alert"><b>Some projections are unavailable</b><span>{errors.join(" · ")}</span></div> : null}
        {session.is_admin && capability.status === "resolved" && !capability.elevated ? <div className="capability-notice"><b>Admin controls are locked</b><span>Member capabilities remain available. Open Profile to elevate with MFA.</span><button onClick={() => go("/profile")}>Open Profile</button></div> : null}
        <Page route={route.path} stats={stats} signal={signal} models={models} connections={connections} users={users} health={health} session={session} capability={capability} isElevated={elevated} onCapabilityRefresh={() => refreshCapability()} onNavigate={go} />
      </main>
    </div>
  </div>;
}

function AccessGate() { return <div className="new-access"><div className="access-card"><span className="logo-mark">AI</span><p className="kicker">Private model gateway</p><h1>Welcome to AI Pool</h1><p>Sign in to use your gateway membership and authorized Admin capabilities.</p><a className="primary-button" href="/auth/login/google">Continue with Google</a></div></div>; }
function Topbar({ session, isAdmin, elevated, loading, onRefresh }: { session: FriendSession; isAdmin: boolean; elevated: boolean; loading: boolean; onRefresh: () => void }) { return <header className="new-topbar"><div className="brand"><span className="logo-mark">AI</span><span><b>AI Pool</b><small>Model gateway</small></span></div><div className="topbar-context">{elevated ? "Admin capability active" : isAdmin ? "Admin identity · controls locked" : "Member workspace"}<strong>{elevated ? "Gateway administration" : isAdmin ? "Administration requires MFA" : "Gateway workspace"}</strong></div><div className="topbar-tools">{isAdmin ? <span className={`identity-badge ${elevated ? "elevated" : "locked"}`}>{elevated ? "Admin elevated" : "Admin locked"}</span> : <span className="identity-badge">Member</span>}<span className="updated">{session.email}</span><button className="icon-button" onClick={onRefresh} disabled={loading} aria-label="Refresh data">↻</button></div></header>; }
function Sidebar({ route, isAdmin, elevated, onNavigate, onSignOut, email }: { route: string; isAdmin: boolean; elevated: boolean; onNavigate: (path: AppRoute) => void; onSignOut: () => void; email: string }) {
  const member = [["/", "Home", "⌂"], ["/models", "Models", "◇"], ["/usage", "Usage", "▥"], ["/setup", "Setup", "↗"], ["/profile", "Profile", "●"]] as const;
  const admin = [["/admin/connections", "Connections", "⇄"], ["/admin/members", "Members", "◎"], ["/admin/system", "System", "⚙"]] as const;
  return <aside className="new-sidebar"><div className="sidebar-section"><span className="section-label">Workspace</span>{member.map(([path, label, icon]) => <NavButton key={path} path={path} current={route} label={label} icon={icon} onNavigate={onNavigate} />)}</div>{isAdmin ? <div className={`sidebar-section admin-nav ${elevated ? "elevated" : "locked"}`}><span className="section-label">Administration <em>{elevated ? "MFA active" : "MFA locked"}</em></span>{admin.map(([path, label, icon]) => <NavButton key={path} path={path} current={route} label={label} icon={icon} onNavigate={onNavigate} locked={!elevated} />)}</div> : null}<div className="sidebar-bottom"><span className="signed-in-identity"><span className="avatar">{email.slice(0, 2).toUpperCase()}</span><small>{email}</small></span><button className="signout-link" onClick={onSignOut}>Sign out</button></div></aside>;
}
function NavButton({ path, current, label, icon, onNavigate, locked = false }: { path: string; current: string; label: string; icon: string; onNavigate: (path: AppRoute) => void; locked?: boolean }) { return <button className={`new-nav-link ${current === path ? "active" : ""} ${locked ? "locked" : ""}`} onClick={() => onNavigate(path as AppRoute)} aria-current={current === path ? "page" : undefined}><span>{icon}</span>{label}{locked ? <small aria-label="MFA locked">🔒</small> : null}</button>; }
