import type { ResourceState } from "../resource-state";
import type { GatewayHealth, OperatorProviderConnectionV2, PoolUserStats } from "../types";
import { PageFrame, StatusBadge } from "../components/ui";

export function AdminCapabilityCheckingPage({ resource }: { resource: "Connections" | "Members" | "System" }) {
  return <PageFrame kicker="Administration" title={resource} description="Verifying current MFA elevation before loading protected data.">
    <section className="admin-lock-screen"><span className="admin-lock-icon">◇</span><div><span>Admin identity</span><h2>Checking Admin capability</h2><p>The protected projection will load only after the backend confirms this session's MFA elevation.</p></div></section>
  </PageFrame>;
}

export function AdminLockedPage({ resource, onUnlock }: { resource: "Connections" | "Members" | "System"; onUnlock: () => void }) {
  return <PageFrame kicker="Administration" title={resource} description="Admin identity recognized; MFA elevation is required for protected data.">
    <section className="admin-lock-screen"><span className="admin-lock-icon">◆</span><div><span>Admin identity</span><h2>{resource} is locked</h2><p>Your member workspace remains available. Elevate this session in Profile to load protected {resource.toLowerCase()} data.</p><button className="primary-button" onClick={onUnlock}>Unlock in Profile</button></div></section>
  </PageFrame>;
}

export function ConnectionsPage({ state }: { state: ResourceState<OperatorProviderConnectionV2[]> }) {
  return <PageFrame kicker="Administration" title="Connections" description="Credentialed upstream capacity and lifecycle state."><ResourceMessage state={state} loading="Loading provider connections" empty="No provider connections are configured" />{state.status === "ready" ? <section className="admin-table">{state.data.map(connection => <article className="admin-row" key={connection.id}><div><b>{connection.identity.display_name || connection.provider_id}</b><small>{connection.provider_id} · {connection.plan_type || "plan unavailable"}</small></div><StatusBadge tone={connection.dead || connection.disabled || connection.health_error ? "warning" : "success"}>{connection.dead ? "Dead" : connection.disabled ? "Disabled" : connection.health_error ? "Degraded" : "Enabled"}</StatusBadge><span>{connection.health_error || `${connection.inflight} in flight`}</span></article>)}</section> : null}</PageFrame>;
}

export function MembersPage({ state }: { state: ResourceState<PoolUserStats[]> }) {
  return <PageFrame kicker="Administration" title="Members" description="Gateway user identities and concise measured activity."><ResourceMessage state={state} loading="Loading gateway members" empty="No member activity is available" />{state.status === "ready" ? <section className="admin-table">{state.data.map(user => <article className="admin-row" key={user.user_id}><div><b>{user.user_id}</b><small>{user.last_seen ? `Last activity ${new Date(user.last_seen).toLocaleString()}` : "No activity timestamp"}</small></div><span>{user.request_count.toLocaleString()} requests</span><span>{user.total_billable_tokens.toLocaleString()} billable tokens</span></article>)}</section> : null}<div className="resource-limitation"><b>Administration projection is limited</b><p>The current backend exposes usage identities and activity, but not member role, plan, enabled state, or security operations.</p></div></PageFrame>;
}

export function SystemPage({ state }: { state: ResourceState<GatewayHealth> }) {
  return <PageFrame kicker="Administration" title="System" description="Platform state that is not owned by Models, Usage, Connections, or Members."><ResourceMessage state={state} loading="Loading runtime health" empty="Runtime health is unavailable" />{state.status === "ready" ? <section className="system-grid"><div className="system-card"><span>Gateway runtime</span><b>{state.data.status}</b><small>Uptime {state.data.uptime}</small></div><Unavailable title="Persistence health" /><Unavailable title="Projection freshness" /><Unavailable title="Background jobs" /><Unavailable title="Configuration revision" /><Unavailable title="Recovery readiness" /></section> : null}</PageFrame>;
}

function ResourceMessage<T>({ state, loading, empty }: { state: ResourceState<T>; loading: string; empty: string }) {
  if (state.status === "idle") return <div className="resource-message"><b>Admin capability required</b><p>This protected resource is locked.</p></div>;
  if (state.status === "loading") return <div className="resource-message"><b>{loading}</b><p>Waiting for the backend projection.</p></div>;
  if (state.status === "error") return <div className="resource-message error"><b>Projection unavailable</b><p>{state.message}</p></div>;
  if (state.status === "empty") return <div className="resource-message"><b>{empty}</b><p>The authorized projection returned no records.</p></div>;
  return null;
}
function Unavailable({ title }: { title: string }) { return <div className="system-card unavailable"><span>{title}</span><b>Unavailable</b><small>No backend projection</small></div>; }
