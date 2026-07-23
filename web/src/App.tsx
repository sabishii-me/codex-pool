import { useCallback, useEffect, useMemo, useState } from "react";
import {
  loadDashboardResources,
  loadModelCatalog,
  loadSession,
  logout,
} from "./api";
import type { FriendSession, ModelDescriptor, PoolStats, Provider, SignalAnalytics } from "./types";

export function providerPresentation(provider: string) {
  if (provider === "nvidia") return { label: "NVIDIA", color: "#76b900", dither: "green", glyph: "◓" as const };
  const label = provider.split("-").filter(Boolean).map(part => part.charAt(0).toUpperCase() + part.slice(1)).join(" ") || "Unknown";
  return { label, color: "#8b8b8b", dither: "grey", glyph: "◇" as const };
}
import { currentRoute, initialRoute, isCompactViewport, navigateTo, routeForPath, type AppRoute, type View } from "./routes";

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

function Page({ route, stats, signal, models, session, onNavigate }: { route: string; stats: PoolStats | null; signal: SignalAnalytics | null; models: ModelDescriptor[]; session: FriendSession; onNavigate: (path: AppRoute) => void }) {
  if (route === "/models" || route === "/operator/routes") return <ModelsPage models={models} operator={route.startsWith("/operator")} />;
  if (route === "/setup") return <SetupPage session={session} />;
  if (route === "/usage" || route === "/operator/usage") return <UsagePage stats={stats} signal={signal} operator={route.startsWith("/operator")} />;
  if (route === "/profile") return <ProfilePage session={session} />;
  if (route === "/operator/connections") return <OperatorPage title="Provider connections" description="Credentialed upstream capacity and lifecycle state." rows={stats?.accounts.map(a => ({ title: a.display_name || a.type, meta: a.type, state: a.status, detail: `${a.secondary_window_available ? "Capacity available" : "Capacity unknown"}` })) ?? []} />;
  if (route === "/operator/members") return <OperatorPage title="Members" description="Gateway users and access state." rows={[]} empty="Member projection is not available yet." />;
  if (route === "/operator/system") return <OperatorPage title="System" description="Runtime, persistence, configuration, and recovery readiness." rows={[]} empty="System health projection is not available yet." />;
  if (route === "/operator/monitor") return <MonitorPage stats={stats} signal={signal} />;
  return <DashboardPage stats={stats} signal={signal} operator={route === "/operator"} onNavigate={onNavigate} />;
}

function DashboardPage({ stats, signal, operator, onNavigate }: { stats: PoolStats | null; signal: SignalAnalytics | null; operator: boolean; onNavigate: (path: AppRoute) => void }) {
  const accounts = stats?.accounts ?? [];
  const healthy = accounts.filter(a => a.status === "healthy").length;
  const latest = signal?.hourly.slice(-24) ?? [];
  return <PageFrame kicker={operator ? "Operations overview" : "Workspace overview"} title={operator ? "Gateway overview" : "Good afternoon"} description={operator ? "The current state of routing, capacity, and usage." : "Here is what matters across your gateway right now."} action={<button className="primary-button" onClick={() => onNavigate("/setup")}>Configure a client</button>}><section className="status-banner"><span className="status-symbol">✓</span><div><div className="status-title">Gateway operational <StatusBadge tone="success">Healthy</StatusBadge></div><p>Requests are routing normally across available providers.</p></div><div className="status-facts"><b>{stats ? `${stats.active_accounts} / ${stats.total_accounts}` : "—"}</b><small>Healthy connections</small></div></section><section className="metric-grid"><Metric label={operator ? "Pool requests" : "Your requests"} value={stats ? stats.last_24h_tokens.toLocaleString() : "—"} note="Last 24 hours" /><Metric label="Processed tokens" value={stats ? compact(stats.aggregate.total_billable_tokens) : "—"} note="Measured" /><Metric label="Models available" value="—" note="Catalog projection" /><Metric label="Provider health" value={`${healthy} / ${accounts.length || "—"}`} note="Current state" /></section><div className="bento-grid"><section className="bento-card chart-card"><CardHeader title="Request activity" subtitle="Rolling activity and comparison" action={<span className="chart-legend"><i className="red-key" />Current <i className="yellow-key" />Prior</span>} /><SplineChart values={latest.map(x => x.request_count)} /></section><section className="bento-card attention-card"><CardHeader title="Attention required" subtitle="Only actionable conditions" /><div className="empty-state"><span className="status-symbol small">✓</span><b>No action required</b><p>There are no active member-facing incidents.</p></div></section><section className="bento-card provider-card"><CardHeader title="Provider health" subtitle="Availability by provider" action={<button className="text-button" onClick={() => onNavigate("/operator/connections")}>View details →</button>} />{accounts.slice(0, 5).map(a => <div className="provider-summary" key={a.id}><span className="provider-dot" /><b>{a.display_name || a.type}</b><span>{a.status}</span><small>{a.secondary_window_available ? `${a.secondary_window_used_pct.toFixed(0)}% used` : "Capacity unknown"}</small></div>)}</section><section className="bento-card quick-card"><CardHeader title="Quick actions" subtitle="Common next steps" /><button className="action-row" onClick={() => onNavigate("/models")}>Browse models <span>→</span></button><button className="action-row" onClick={() => onNavigate("/usage")}>View my usage <span>→</span></button></section></div></PageFrame>;
}

function MonitorPage({ stats, signal }: { stats: PoolStats | null; signal: SignalAnalytics | null }) { return <PageFrame kicker="Operations" title="Monitor" description="Live routing, capacity, and persistence state."><section className="focus-monitor"><CardHeader title="Live pool health" subtitle="Rolling 30 minutes" action={<span className="live-label"><span className="live-dot" />Live</span>} /><section className="focus-metrics"><Metric label="Request rate" value={signal?.hourly.at(-1)?.request_count?.toString() ?? "—"} note="Measured" /><Metric label="P95 latency" value="—" note="Projection pending" /><Metric label="Error rate" value="—" note="Projection pending" /><Metric label="Headroom" value={stats ? `${Math.max(0, 100 - Math.round(stats.aggregate.overall_cache_hit_rate_pct))}%` : "—"} note="Estimated" /></section><SplineChart values={signal?.hourly.slice(-30).map(x => x.request_count) ?? []} /></section><section className="bento-card incident-card"><CardHeader title="Operational detail" subtitle="Available diagnostics" /><div className="empty-state"><b>Detailed monitor projection pending</b><p>The shell is ready; normalized latency, incidents, and persistence health will be connected through the v2 monitor contract.</p></div></section></PageFrame>; }

function UsagePage({ stats, signal, operator }: { stats: PoolStats | null; signal: SignalAnalytics | null; operator: boolean }) { return <PageFrame kicker={operator ? "Operations" : "Member workspace"} title={operator ? "Usage & economics" : "My usage"} description={operator ? "Pool-wide usage and economics with evidence state." : "Your measured gateway activity."}><section className="metric-grid"><Metric label="Processed tokens" value={compact(stats?.aggregate.total_billable_tokens ?? 0)} note="Measured" /><Metric label="Requests" value={signal ? signal.hourly.reduce((n, x) => n + x.request_count, 0).toLocaleString() : "—"} note="Available data" /><Metric label="Cost" value="—" note="Projection pending" /><Metric label="Trend" value="—" note="Comparison pending" /></section><section className="bento-card chart-card"><CardHeader title={operator ? "Pool activity" : "Your activity"} subtitle="Smooth time-series view" /><SplineChart values={signal?.hourly.slice(-30).map(x => x.request_count) ?? []} /></section></PageFrame>; }

function ModelsPage({ models, operator }: { models: ModelDescriptor[]; operator: boolean }) { const [query, setQuery] = useState(""); const filtered = models.filter(m => `${m.id} ${m.name ?? ""} ${m.provider}`.toLowerCase().includes(query.toLowerCase())); return <PageFrame kicker={operator ? "Operations" : "Workspace"} title="Models" description="Find an available model route and copy its public ID."><div className="toolbar"><input className="search-input" value={query} onChange={e => setQuery(e.target.value)} placeholder="Search models or providers" aria-label="Search models" /><span>{filtered.length} routes</span></div><section className="model-list">{filtered.map(model => <article className="model-row-new" key={model.id}><div><b>{model.name || model.id}</b><code>{model.id}</code></div><span>{model.provider}</span><StatusBadge tone={model.available_now ? "success" : "warning"}>{model.available_now ? "Available" : "Limited"}</StatusBadge><button className="secondary-button" onClick={() => navigator.clipboard?.writeText(model.id)}>Copy ID</button></article>)}</section></PageFrame>; }

function SetupPage({ session }: { session: FriendSession }) { return <PageFrame kicker="Workspace" title="Setup" description="Configure a supported client and verify the gateway connection."><section className="setup-new"><div className="setup-intro"><span className="step-number">1</span><div><h2>Choose your client</h2><p>Start with the setup path for your preferred tool. Generated configuration stays scoped to your signed-in session.</p></div></div><div className="setup-tools-new"><button className="active">Pi</button><button>Claude Code</button><button>Codex CLI</button><button>Gemini CLI</button></div><div className="code-panel"><header><span>Recommended configuration</span><button>Copy</button></header><code>Gateway: {session.public_url || "active development gateway"}\nSession: authenticated workspace\nModels: current catalog</code></div></section></PageFrame>; }

function ProfilePage({ session }: { session: FriendSession }) { return <PageFrame kicker="Workspace" title="Profile" description="Identity and security for this gateway member."><section className="profile-new"><div><span>Signed in as</span><b>{session.email}</b></div><div><span>Workspace</span><b>Member</b></div><div><span>Security</span><b>Managed by gateway authentication</b></div></section></PageFrame>; }

function OperatorPage({ title, description, rows, empty }: { title: string; description: string; rows: Array<{ title: string; meta: string; state: string; detail: string }>; empty?: string }) { return <PageFrame kicker="Operations" title={title} description={description}><section className="operator-list">{rows.length ? rows.map(row => <article className="model-row-new" key={row.title}><div><b>{row.title}</b><small>{row.meta}</small></div><StatusBadge tone={row.state === "healthy" ? "success" : "warning"}>{row.state}</StatusBadge><span>{row.detail}</span><button className="secondary-button">Inspect</button></article>) : <div className="empty-state"><b>{empty}</b><p>This route is part of the approved shell and awaits its normalized production projection.</p></div>}</section></PageFrame>; }

function PageFrame({ kicker, title, description, action, children }: { kicker: string; title: string; description: string; action?: React.ReactNode; children: React.ReactNode }) { return <div className="page-frame"><header className="page-heading"><div><span className="kicker">{kicker}</span><h1>{title}</h1><p>{description}</p></div>{action}</header>{children}</div>; }
function CardHeader({ title, subtitle, action }: { title: string; subtitle: string; action?: React.ReactNode }) { return <header className="card-header"><div><h2>{title}</h2><p>{subtitle}</p></div>{action}</header>; }
function Metric({ label, value, note }: { label: string; value: string; note: string }) { return <article className="metric"><span>{label}</span><strong>{value}</strong><small>{note}</small></article>; }
function StatusBadge({ tone, children }: { tone: "success" | "warning"; children: React.ReactNode }) { return <span className={`status-badge ${tone}`}>{children}</span>; }
function compact(value: number) { return new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 }).format(value || 0); }
function SplineChart({ values }: { values: number[] }) { const normalized = values.map(value => Number.isFinite(value) ? value : 0); const points = normalized.length > 1 ? normalized : [12, 18, 14, 23, 20, 29, 25, 35, 31, 40, 37, 46]; const max = Math.max(...points, 1); const path = points.map((value, i) => `${i ? "L" : "M"}${(i / (points.length - 1)) * 100},${94 - (value / max) * 78}`).join(" "); const area = `${path} L100,100 L0,100 Z`; return <div className="spline-chart" role="img" aria-label="Request activity trend chart"><svg viewBox="0 0 100 100" preserveAspectRatio="none"><defs><linearGradient id="red-area" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="#f87171" stopOpacity=".28" /><stop offset="1" stopColor="#f87171" stopOpacity="0" /></linearGradient></defs><path className="chart-area" d={area} /><path className="chart-line comparison" d={points.map((value, i) => `${i ? "L" : "M"}${(i / (points.length - 1)) * 100},${94 - (Math.max(1, value * .68) / max) * 78}`).join(" ")} /><path className="chart-line" d={path} /></svg></div>; }
