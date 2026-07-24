import { useCallback, useEffect, useRef, useState } from "react";
import { checkMFAStatus, loadDashboardResources, loadPoolUsers, loadProviderConnectionsV2, loadSession, loadSystemProjection, logout, verifyMFA } from "./api";
import type { FriendSession, GatewayHealth, ModelDescriptor, OperatorProviderConnectionV2, PoolStats, PoolUserStats, SignalAnalytics, SystemProjection } from "./types";
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
  const [health, setHealth] = useState<ResourceState<SystemProjection>>({ status: "idle" });
  const [capability, setCapability] = useState<CapabilityStatus>({ status: "idle" });
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const [mfaChallengeOpen, setMFAChallengeOpen] = useState(false);
  const [pendingAdminRoute, setPendingAdminRoute] = useState<AppRoute | null>(null);
  const capabilityGeneration = useRef(0);

  const elevated = isElevated(capability);
  const clearProtected = useCallback(() => { setConnections({ status: "idle" }); setUsers({ status: "idle" }); setHealth({ status: "idle" }); }, []);
  const refreshCapability = useCallback(async (initial = false): Promise<boolean> => {
    if (!session?.is_admin) return false;
    const generation = ++capabilityGeneration.current;
    if (initial) setCapability({ status: "checking" });
    try {
      const status = await checkMFAStatus();
      if (generation !== capabilityGeneration.current) return false;
      setCapability({ status: "resolved", enrolled: status.enrolled, elevated: status.elevated, recoveryCodesRemaining: status.recovery_codes_remaining });
      if (!status.elevated) clearProtected();
      return status.elevated;
    } catch (error) {
      if (generation !== capabilityGeneration.current) return false;
      clearProtected();
      setCapability({ status: "error", message: error instanceof Error ? error.message : "MFA status unavailable" });
      return false;
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
      const [connectionResult, userResult, healthResult] = await Promise.allSettled([loadProviderConnectionsV2(), loadPoolUsers(), loadSystemProjection()]);
      setConnections(connectionResult.status === "fulfilled" ? (connectionResult.value.length ? { status: "ready", data: connectionResult.value } : { status: "empty" }) : { status: "error", message: connectionResult.reason instanceof Error ? connectionResult.reason.message : "Connections unavailable" });
      setUsers(userResult.status === "fulfilled" ? (userResult.value.users.length ? { status: "ready", data: userResult.value.users } : { status: "empty" }) : { status: "error", message: userResult.reason instanceof Error ? userResult.reason.message : "Members unavailable" });
      setHealth(healthResult.status === "fulfilled" ? { status: "ready", data: healthResult.value } : { status: "error", message: healthResult.reason instanceof Error ? healthResult.reason.message : "Runtime health unavailable" });
    } finally { setLoading(false); }
  }, [elevated, clearProtected]);

  const refreshConnections = useCallback(async () => {
    setConnections({ status: "loading" });
    try {
      const data = await loadProviderConnectionsV2();
      setConnections(data.length ? { status: "ready", data } : { status: "empty" });
    } catch (error) {
      setConnections({ status: "error", message: error instanceof Error ? error.message : "Connections unavailable" });
      throw error;
    }
  }, []);
  const authorizationLost = useCallback(() => {
    clearProtected();
    setPendingAdminRoute(route.path);
    setMFAChallengeOpen(true);
    void refreshCapability(true);
  }, [clearProtected, refreshCapability, route.path]);

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
    if (incoming.adminOnly && !session.is_admin) { const home = routeForPath("/"); navigateTo(home, true); setRoute(home); return; }
    if (incoming.adminOnly && session.is_admin && !elevated) {
      setPendingAdminRoute(incoming.path);
      setMFAChallengeOpen(true);
    }
  }, [session, capability, elevated]);

  const go = (path: AppRoute) => {
    const target = routeForPath(path);
    if (target.adminOnly && !session?.is_admin) return;
    if (target.adminOnly && !elevated) {
      setPendingAdminRoute(path);
      setMFAChallengeOpen(true);
      return;
    }
    navigateTo(target); setRoute(target);
  };
  const closeMFAChallenge = () => {
    setMFAChallengeOpen(false);
    setPendingAdminRoute(null);
    if (route.adminOnly) { const home = routeForPath("/"); navigateTo(home, true); setRoute(home); }
  };
  const completeMFAChallenge = async () => {
    const nowElevated = await refreshCapability();
    if (!nowElevated) return false;
    const target = routeForPath(pendingAdminRoute ?? "/admin/connections");
    setMFAChallengeOpen(false); setPendingAdminRoute(null); navigateTo(target, route.adminOnly); setRoute(target);
    return true;
  };
  const signOut = async () => { await logout(); setSession(null); const home = routeForPath("/"); navigateTo(home, true); setRoute(home); };

  if (booting) return <div className="new-boot"><span>AI POOL</span><small>Loading workspace</small></div>;
  if (!session) return <AccessGate />;

  const renderedRoute = route.adminOnly && !elevated ? "/" : route.path;
  return <div className="new-app" data-admin={session.is_admin ? "true" : "false"} data-elevated={elevated ? "true" : "false"} data-route={route.path}>
    <Topbar session={session} isAdmin={session.is_admin} elevated={elevated} loading={loading} onRefresh={refresh} />
    <div className="new-layout"><Sidebar route={renderedRoute} isAdmin={session.is_admin} onNavigate={go} onSignOut={signOut} email={session.email} />
      <main className="new-main" id="main-content" tabIndex={-1}>
        {errors.length ? <div className="new-alert" role="alert"><b>Some projections are unavailable</b><span>{errors.join(" · ")}</span></div> : null}
        <Page route={renderedRoute} stats={stats} signal={signal} models={models} connections={connections} users={users} health={health} session={session} capability={capability} isElevated={elevated} onConnectionsRefresh={refreshConnections} onAuthorizationLost={authorizationLost} onNavigate={go} />
      </main>
    </div>
    {mfaChallengeOpen ? <MFAChallenge capability={capability} destination={pendingAdminRoute} onCancel={closeMFAChallenge} onVerified={completeMFAChallenge} /> : null}
  </div>;
}

function AccessGate() { return <div className="new-access"><div className="access-card"><span className="logo-mark">AI</span><p className="kicker">Private model gateway</p><h1>Welcome to AI Pool</h1><p>Sign in to use your gateway membership and authorized Admin capabilities.</p><a className="primary-button" href="/auth/login/google">Continue with Google</a></div></div>; }
function Topbar({ session, isAdmin, elevated, loading, onRefresh }: { session: FriendSession; isAdmin: boolean; elevated: boolean; loading: boolean; onRefresh: () => void }) { return <header className="new-topbar"><div className="brand"><span className="logo-mark">AI</span><span><b>AI Pool</b><small>Model gateway</small></span></div><div className="topbar-context">{elevated ? "Administration" : "Workspace"}<strong>{elevated ? "Gateway administration" : "Gateway workspace"}</strong></div><div className="topbar-tools">{isAdmin ? <span className="identity-badge">Admin</span> : null}<span className="updated">{session.email}</span><button className="icon-button" onClick={onRefresh} disabled={loading} aria-label="Refresh data">↻</button></div></header>; }
function Sidebar({ route, isAdmin, onNavigate, onSignOut, email }: { route: string; isAdmin: boolean; onNavigate: (path: AppRoute) => void; onSignOut: () => void; email: string }) {
  const member = [["/", "Home", "⌂"], ["/models", "Models", "◇"], ["/usage", "Usage", "▥"], ["/setup", "Setup", "↗"], ["/profile", "Profile", "●"]] as const;
  const admin = [["/admin/connections", "Connections", "⇄"], ["/admin/members", "Members", "◎"], ["/admin/system", "System", "⚙"]] as const;
  return <aside className="new-sidebar"><div className="sidebar-section"><span className="section-label">Workspace</span>{member.map(([path, label, icon]) => <NavButton key={path} path={path} current={route} label={label} icon={icon} onNavigate={onNavigate} />)}</div>{isAdmin ? <div className="sidebar-section admin-nav"><span className="section-label">Administration</span>{admin.map(([path, label, icon]) => <NavButton key={path} path={path} current={route} label={label} icon={icon} onNavigate={onNavigate} />)}</div> : null}<div className="sidebar-bottom"><span className="signed-in-identity"><span className="avatar">{email.slice(0, 2).toUpperCase()}</span><small>{email}</small></span><button className="signout-link" onClick={onSignOut}>Sign out</button></div></aside>;
}
function NavButton({ path, current, label, icon, onNavigate }: { path: string; current: string; label: string; icon: string; onNavigate: (path: AppRoute) => void }) { return <button className={`new-nav-link ${current === path ? "active" : ""}`} onClick={() => onNavigate(path as AppRoute)} aria-current={current === path ? "page" : undefined}><span>{icon}</span>{label}</button>; }

function MFAChallenge({ capability, destination, onCancel, onVerified }: { capability: CapabilityStatus; destination: AppRoute | null; onCancel: () => void; onVerified: () => Promise<boolean> }) {
  const [code, setCode] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const title = destination?.split("/").at(-1) ?? "Admin controls";
  const submit = async () => {
    if (code.length !== 6 || submitting) return;
    setSubmitting(true); setError("");
    try { await verifyMFA({ code }); if (!await onVerified()) setError("Admin elevation could not be confirmed."); }
    catch (failure) { setError(failure instanceof Error ? failure.message : "Verification failed"); }
    finally { setSubmitting(false); }
  };
  return <div className="mfa-modal-layer" role="presentation"><button className="mfa-modal-backdrop" aria-label="Close Admin verification" onClick={onCancel} /><section className="mfa-modal" role="dialog" aria-modal="true" aria-labelledby="mfa-title"><button className="mfa-modal-close" aria-label="Close Admin verification" onClick={onCancel}>×</button><span className="mfa-modal-mark">AI</span><p className="kicker">Admin verification</p><h1 id="mfa-title">Unlock {title}</h1><p>Enter the six-digit code from your authenticator app. You will continue to the requested Admin resource after verification.</p>{capability.status === "resolved" && !capability.enrolled ? <div className="resource-message"><b>MFA enrollment required</b><p>Enroll an authenticator before Admin controls can be unlocked.</p></div> : <><label className="mfa-code-field"><span>Authentication code</span><input autoFocus aria-label="MFA verification code" inputMode="numeric" autoComplete="one-time-code" maxLength={6} placeholder="000000" value={code} onChange={event => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))} onKeyDown={event => { if (event.key === "Enter") void submit(); }} /></label>{error ? <p className="form-error" role="alert">{error}</p> : null}<div className="mfa-modal-actions"><button className="secondary-button" onClick={onCancel}>Cancel</button><button className="primary-button" disabled={code.length !== 6 || submitting} onClick={submit}>{submitting ? "Verifying…" : "Verify and continue"}</button></div></>}</section></div>;
}
