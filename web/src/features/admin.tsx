import { useEffect, useState } from "react";
import { isAuthorizationError, mutateProviderConnection, renameProviderConnection, runSystemOperation } from "../api";
import type { ResourceState } from "../resource-state";
import type { OperatorProviderConnectionV2, PoolUserStats, SystemProjection } from "../types";
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

export function ConnectionsPage({ state, onRefresh, onAuthorizationLost }: { state: ResourceState<OperatorProviderConnectionV2[]>; onRefresh: () => Promise<void>; onAuthorizationLost: () => void }) {
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [operation, setOperation] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const selected = state.status === "ready" ? state.data.find(connection => connection.id === selectedID) ?? null : null;
  useEffect(() => { if (state.status === "ready" && selectedID && !state.data.some(connection => connection.id === selectedID)) setSelectedID(null); }, [state, selectedID]);
  const run = async (label: string, task: () => Promise<unknown>) => {
    if (operation) return;
    setOperation(label); setFeedback(null);
    try { await task(); await onRefresh(); setFeedback({ tone: "success", text: `${label} completed.` }); }
    catch (error) {
      if (isAuthorizationError(error)) { onAuthorizationLost(); return; }
      setFeedback({ tone: "error", text: error instanceof Error ? error.message : `${label} failed` });
    } finally { setOperation(null); }
  };
  return <PageFrame kicker="Administration" title="Connections" description="Credentialed upstream capacity and lifecycle state.">
    <ResourceMessage state={state} loading="Loading provider connections" empty="No provider connections are configured" />
    {feedback ? <div className={`operation-feedback ${feedback.tone}`} role={feedback.tone === "error" ? "alert" : "status"}>{feedback.text}</div> : null}
    {state.status === "ready" ? <section className="connections-workspace">
      <div className="admin-table connection-list" aria-label="Provider connections">{state.data.map(connection => <button className={`admin-row connection-row ${selectedID === connection.id ? "selected" : ""}`} key={connection.id} onClick={() => setSelectedID(connection.id)} aria-pressed={selectedID === connection.id}><div><b>{connection.identity.display_name || connection.provider_id}</b><small>{connection.provider_id} · {connection.plan_type || "plan unavailable"}{connection.is_primary ? " · Primary" : ""}</small></div><StatusBadge tone={connection.dead || connection.disabled || connection.health_error ? "warning" : "success"}>{connection.dead ? "Dead" : connection.disabled ? "Disabled" : connection.health_error ? "Degraded" : "Enabled"}</StatusBadge><span>{connection.health_error || `${connection.inflight} in flight`}</span></button>)}</div>
      {selected ? <ConnectionDetail connection={selected} operation={operation} onClose={() => setSelectedID(null)} onRun={run} /> : <div className="connection-detail-empty"><span>Connection detail</span><b>Select a connection</b><p>Inspect identity, credential lifecycle, measured usage, and available operations.</p></div>}
    </section> : null}
  </PageFrame>;
}

function ConnectionDetail({ connection, operation, onClose, onRun }: { connection: OperatorProviderConnectionV2; operation: string | null; onClose: () => void; onRun: (label: string, task: () => Promise<unknown>) => Promise<void> }) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(connection.identity.display_name);
  useEffect(() => { setName(connection.identity.display_name); }, [connection.id, connection.identity.display_name]);
  const busy = operation !== null;
  const lifecycle = connection.dead ? "Dead" : connection.disabled ? "Disabled" : connection.health_error ? "Degraded" : "Enabled";
  const total = (key: string) => Number(connection.totals[key] ?? 0).toLocaleString();
  return <aside className="connection-detail" aria-label="Connection detail">
    <header><div><span>Connection detail</span><h2>{connection.identity.display_name || connection.provider_id}</h2><code>{connection.public_id}</code></div><button className="icon-button" aria-label="Close connection detail" onClick={onClose}>×</button></header>
    {editing ? <form className="connection-rename" onSubmit={event => { event.preventDefault(); void onRun("Rename", () => renameProviderConnection(connection.id, name)).then(() => setEditing(false)); }}><label><span>Display name</span><input autoFocus maxLength={120} value={name} onChange={event => setName(event.target.value)} /></label><div><button type="button" className="secondary-button" onClick={() => setEditing(false)}>Cancel</button><button className="primary-button" disabled={busy || !name.trim()}>Save name</button></div></form> : <button className="text-action" onClick={() => setEditing(true)}>Rename connection</button>}
    <div className="connection-facts"><Fact label="Provider" value={connection.provider_id} /><Fact label="Plan" value={connection.plan_type || "Unavailable"} /><Fact label="Lifecycle" value={lifecycle} /><Fact label="In flight" value={String(connection.inflight)} /><Fact label="Primary" value={connection.is_primary ? "Yes" : "No"} /><Fact label="MFA verification" value={connection.needs_verification ? "Required by provider" : "Not reported"} /><Fact label="Last refresh" value={formatTimestamp(connection.last_refresh)} /><Fact label="Credential expiry" value={formatTimestamp(connection.expires_at)} /></div>
    {connection.health_error ? <div className="connection-health-error"><b>Current health error</b><p>{connection.health_error}</p></div> : null}
    <section className="connection-identity"><h3>Provider identity</h3><Fact label="External subject" value={connection.identity.external_subject || "Unavailable"} />{Object.entries(connection.identity.attributes ?? {}).map(([key, value]) => <Fact key={key} label={key.replaceAll("_", " ")} value={value} />)}</section>
    <section><h3>Measured totals</h3><div className="connection-totals"><Fact label="Input tokens" value={total("total_input_tokens")} /><Fact label="Cached tokens" value={total("total_cached_tokens")} /><Fact label="Output tokens" value={total("total_output_tokens")} /><Fact label="Billable tokens" value={total("total_billable_tokens")} /></div></section>
    <div className="resource-limitation"><b>Quota evidence</b><p>{Object.keys(connection.usage).length ? "Provider-reported usage is available in the connection projection; normalized quota windows are not yet contracted." : "No provider quota projection is available."}</p></div>
    <footer className="connection-actions"><button disabled={busy} onClick={() => void onRun("Refresh", () => mutateProviderConnection(connection.id, "refresh"))}>{operation === "Refresh" ? "Refreshing…" : "Refresh credentials"}</button>{connection.dead ? <button disabled={busy} onClick={() => void onRun("Recover", () => mutateProviderConnection(connection.id, "recover"))}>Recover</button> : connection.disabled ? <button disabled={busy} onClick={() => void onRun("Enable", () => mutateProviderConnection(connection.id, "enable"))}>Enable</button> : <button className="danger-action" disabled={busy} onClick={() => { if (window.confirm(`Disable ${connection.identity.display_name || connection.provider_id}? It will stop receiving traffic.`)) void onRun("Disable", () => mutateProviderConnection(connection.id, "disable")); }}>Disable</button>}</footer>
  </aside>;
}
function Fact({ label, value }: { label: string; value: string }) { return <div className="connection-fact"><span>{label}</span><b>{value}</b></div>; }
function formatTimestamp(value?: string) { if (!value || value.startsWith("0001-")) return "Unavailable"; const date = new Date(value); return Number.isNaN(date.valueOf()) ? "Unavailable" : date.toLocaleString(); }

export function MembersPage({ state }: { state: ResourceState<PoolUserStats[]> }) {
  return <PageFrame kicker="Administration" title="Members" description="Gateway user identities and concise measured activity."><ResourceMessage state={state} loading="Loading gateway members" empty="No member activity is available" />{state.status === "ready" ? <section className="admin-table">{state.data.map(user => <article className="admin-row" key={user.user_id}><div><b>{user.user_id}</b><small>{user.last_seen ? `Last activity ${new Date(user.last_seen).toLocaleString()}` : "No activity timestamp"}</small></div><span>{user.request_count.toLocaleString()} requests</span><span>{user.total_billable_tokens.toLocaleString()} billable tokens</span></article>)}</section> : null}<div className="resource-limitation"><b>Administration projection is limited</b><p>The current backend exposes usage identities and activity, but not member role, plan, enabled state, or security operations.</p></div></PageFrame>;
}

export function SystemPage({ state }: { state: ResourceState<SystemProjection> }) {
  const [operation, setOperation] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const run = async (label: string, action: "reload-connections" | "clear-rate-limits") => {
    if (operation) return;
    setOperation(label); setMessage("");
    try { const result = await runSystemOperation(action); setMessage(action === "clear-rate-limits" ? `${Number(result.cleared ?? 0)} active rate limits cleared.` : "Provider connections reloaded."); }
    catch (error) { setMessage(error instanceof Error ? error.message : `${label} failed`); }
    finally { setOperation(null); }
  };
  return <PageFrame kicker="Administration" title="System" description="Measured runtime, persistence, and registry state owned by the gateway."><ResourceMessage state={state} loading="Loading system projection" empty="System projection is unavailable" />{state.status === "ready" ? <>
    <div className="evidence-strip"><span>{state.data.evidence.kind}</span><b>{state.data.evidence.source}</b><small>Generated {new Date(state.data.evidence.generated_at).toLocaleString()}</small></div>
    <section className="system-grid"><div className="system-card"><span>Gateway runtime</span><b>{state.data.runtime.status}</b><small>Started {new Date(state.data.runtime.started_at).toLocaleString()} · {formatUptime(state.data.runtime.uptime_seconds)}</small></div><div className="system-card"><span>Connection capacity</span><b>{state.data.capacity.connections_active} active</b><small>{state.data.capacity.connections_total} total · {state.data.capacity.connections_disabled} disabled · {state.data.capacity.connections_dead} dead</small></div><div className="system-card"><span>Provider registry</span><b>{state.data.capacity.providers_registered} providers</b><small>{state.data.capacity.declarative_providers} declarative specifications active</small></div>{state.data.persistence.map(item => <div className={`system-card ${item.healthy ? "" : "unavailable"}`} key={item.name}><span>{item.name}</span><b>{item.healthy ? "Healthy" : item.configured ? "Unavailable" : "Not configured"}</b><small>{item.detail}</small></div>)}</section>
    <section className="system-operations"><div><span>Operational controls</span><h2>Gateway maintenance</h2><p>These actions use current backend operations and do not imply persistence or recovery guarantees beyond their response.</p></div><div><button disabled={operation !== null} onClick={() => void run("Reload", "reload-connections")}>{operation === "Reload" ? "Reloading…" : "Reload connections"}</button><button disabled={operation !== null} onClick={() => { if (window.confirm("Clear all active provider rate-limit cooldowns?")) void run("Clear", "clear-rate-limits"); }}>{operation === "Clear" ? "Clearing…" : "Clear rate limits"}</button></div>{message ? <p role="status">{message}</p> : null}</section>
  </> : null}</PageFrame>;
}
function formatUptime(seconds: number) { const days = Math.floor(seconds / 86400); const hours = Math.floor(seconds % 86400 / 3600); const minutes = Math.floor(seconds % 3600 / 60); return `Uptime ${days ? `${days}d ` : ""}${hours}h ${minutes}m`; }

function ResourceMessage<T>({ state, loading, empty }: { state: ResourceState<T>; loading: string; empty: string }) {
  if (state.status === "idle") return <div className="resource-message"><b>Admin capability required</b><p>This protected resource is locked.</p></div>;
  if (state.status === "loading") return <div className="resource-message"><b>{loading}</b><p>Waiting for the backend projection.</p></div>;
  if (state.status === "error") return <div className="resource-message error"><b>Projection unavailable</b><p>{state.message}</p></div>;
  if (state.status === "empty") return <div className="resource-message"><b>{empty}</b><p>The authorized projection returned no records.</p></div>;
  return null;
}
