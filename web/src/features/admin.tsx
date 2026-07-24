import { useEffect, useState } from "react";
import { createGatewayMember, isAuthorizationError, mutateProviderConnection, renameProviderConnection, runSystemOperation, setGatewayMemberEnabled } from "../api";
import type { ResourceState } from "../resource-state";
import type { GatewayMember, OperatorProviderConnectionV2, SystemProjection } from "../types";
import { ConfirmDialog } from "../components/confirm-dialog";
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
  const [confirmDisable, setConfirmDisable] = useState(false);
  const [name, setName] = useState(connection.identity.display_name);
  useEffect(() => { setName(connection.identity.display_name); }, [connection.id, connection.identity.display_name]);
  const busy = operation !== null;
  const lifecycle = connection.dead ? "Dead" : connection.disabled ? "Disabled" : connection.health_error ? "Degraded" : "Enabled";
  const total = (key: string) => Number(connection.totals[key] ?? 0).toLocaleString();
  const usageFacts = providerUsageFacts(connection.usage);
  return <aside className="connection-detail" aria-label="Connection detail">
    <header><div><span>Connection detail</span><h2>{connection.identity.display_name || connection.provider_id}</h2><code>{connection.public_id}</code></div><button className="icon-button" aria-label="Close connection detail" onClick={onClose}>×</button></header>
    {editing ? <form className="connection-rename" onSubmit={event => { event.preventDefault(); void onRun("Rename", () => renameProviderConnection(connection.id, name)).then(() => setEditing(false)); }}><label><span>Display name</span><input autoFocus maxLength={120} value={name} onChange={event => setName(event.target.value)} /></label><div><button type="button" className="secondary-button" onClick={() => setEditing(false)}>Cancel</button><button className="primary-button" disabled={busy || !name.trim()}>Save name</button></div></form> : <button className="text-action" onClick={() => setEditing(true)}>Rename connection</button>}
    <div className="connection-facts"><Fact label="Provider" value={connection.provider_id} />{connection.plan_type ? <Fact label="Plan" value={connection.plan_type} /> : null}<Fact label="Lifecycle" value={lifecycle} /><Fact label="In flight" value={String(connection.inflight)} /><Fact label="Primary" value={connection.is_primary ? "Yes" : "No"} />{connection.needs_verification ? <Fact label="Provider verification" value="Required" /> : null}{connection.last_refresh && !connection.last_refresh.startsWith("0001-") ? <Fact label="Last refresh" value={formatTimestamp(connection.last_refresh)} /> : null}{connection.expires_at && !connection.expires_at.startsWith("0001-") ? <Fact label="Credential expiry" value={formatTimestamp(connection.expires_at)} /> : null}</div>
    {connection.health_error ? <div className="connection-health-error"><b>Current health error</b><p>{connection.health_error}</p></div> : null}
    {connection.identity.external_subject || Object.keys(connection.identity.attributes ?? {}).length ? <section className="connection-identity"><h3>Provider identity</h3>{connection.identity.external_subject ? <Fact label="External subject" value={connection.identity.external_subject} /> : null}{Object.entries(connection.identity.attributes ?? {}).map(([key, value]) => <Fact key={key} label={key.replaceAll("_", " ")} value={value} />)}</section> : null}
    <section><h3>Measured totals</h3><div className="connection-totals"><Fact label="Input tokens" value={total("total_input_tokens")} /><Fact label="Cached tokens" value={total("total_cached_tokens")} /><Fact label="Output tokens" value={total("total_output_tokens")} /><Fact label="Billable tokens" value={total("total_billable_tokens")} /></div></section>
    {usageFacts.length ? <section><h3>Provider usage</h3><div className="connection-totals">{usageFacts.map(fact => <Fact key={fact.label} label={fact.label} value={fact.value} />)}</div></section> : null}
    <footer className="connection-actions"><button disabled={busy} onClick={() => void onRun("Refresh", () => mutateProviderConnection(connection.id, "refresh"))}>{operation === "Refresh" ? "Refreshing…" : "Refresh credentials"}</button>{connection.dead ? <button disabled={busy} onClick={() => void onRun("Recover", () => mutateProviderConnection(connection.id, "recover"))}>Recover</button> : connection.disabled ? <button disabled={busy} onClick={() => void onRun("Enable", () => mutateProviderConnection(connection.id, "enable"))}>Enable</button> : <ConfirmDialog open={confirmDisable} onOpenChange={setConfirmDisable} title="Disable connection?" description={`${connection.identity.display_name || connection.provider_id} will immediately stop receiving gateway traffic. You can enable it again later.`} confirmLabel="Disable connection" busy={operation === "Disable"} onConfirm={() => void onRun("Disable", () => mutateProviderConnection(connection.id, "disable")).then(() => setConfirmDisable(false))}><button className="danger-action" disabled={busy}>Disable</button></ConfirmDialog>}</footer>
  </aside>;
}
function providerUsageFacts(usage: Record<string, unknown>): Array<{ label: string; value: string }> {
  const number = (...keys: string[]) => { for (const key of keys) if (typeof usage[key] === "number") return usage[key] as number; return null; };
  const text = (...keys: string[]) => { for (const key of keys) if (typeof usage[key] === "string" && usage[key]) return usage[key] as string; return ""; };
  const facts: Array<{ label: string; value: string }> = [];
  const primaryWindow = number("primary_window_minutes", "PrimaryWindowMinutes");
  const secondaryWindow = number("secondary_window_minutes", "SecondaryWindowMinutes");
  const primaryUsed = number("primary_used_percent", "PrimaryUsedPercent", "primary_used", "PrimaryUsed");
  const secondaryUsed = number("secondary_used_percent", "SecondaryUsedPercent", "secondary_used", "SecondaryUsed");
  if (primaryWindow && primaryUsed !== null) facts.push({ label: `${primaryWindow}m window used`, value: `${primaryUsed.toFixed(1)}%` });
  if (secondaryWindow && secondaryUsed !== null) facts.push({ label: `${secondaryWindow}m window used`, value: `${secondaryUsed.toFixed(1)}%` });
  const credits = number("credits_balance", "CreditsBalance");
  const hasCredits = usage.has_credits === true || usage.HasCredits === true;
  if (hasCredits && credits !== null) facts.push({ label: "Credits", value: credits.toLocaleString() });
  const source = text("source", "Source");
  if (source) facts.push({ label: "Source", value: source });
  const retrieved = text("retrieved_at", "RetrievedAt");
  if (retrieved && !retrieved.startsWith("0001-")) facts.push({ label: "Retrieved", value: formatTimestamp(retrieved) });
  return facts;
}
function Fact({ label, value }: { label: string; value: string }) { return <div className="connection-fact"><span>{label}</span><b>{value}</b></div>; }
function formatTimestamp(value: string) { const date = new Date(value); return date.toLocaleString(); }

export function MembersPage({ state, onRefresh }: { state: ResourceState<GatewayMember[]>; onRefresh: () => Promise<void> }) {
  const [creating, setCreating] = useState(false);
  const [email, setEmail] = useState("");
  const [plan, setPlan] = useState("pro");
  const [busy, setBusy] = useState<string | null>(null);
  const [issuedToken, setIssuedToken] = useState("");
  const [error, setError] = useState("");
  const [confirmMember, setConfirmMember] = useState<GatewayMember | null>(null);
  const create = async () => {
    if (!email.trim() || busy) return;
    setBusy("create"); setError(""); setIssuedToken("");
    try { const result = await createGatewayMember(email.trim(), plan); setIssuedToken(result.token); setEmail(""); setCreating(false); await onRefresh(); }
    catch (failure) { setError(failure instanceof Error ? failure.message : "Member creation failed"); }
    finally { setBusy(null); }
  };
  const toggle = async (member: GatewayMember): Promise<boolean> => {
    if (busy) return false;
    setBusy(member.id); setError("");
    try { await setGatewayMemberEnabled(member.id, member.disabled); await onRefresh(); return true; }
    catch (failure) { setError(failure instanceof Error ? failure.message : "Member update failed"); return false; }
    finally { setBusy(null); }
  };
  return <PageFrame kicker="Administration" title="Members" description="People authorized to sign in and use this gateway." action={<button className="primary-button" onClick={() => setCreating(true)}>Add member</button>}>
    <ResourceMessage state={state} loading="Loading gateway members" empty="No gateway members are configured" />
    {creating ? <section className="member-create"><div><span>New gateway member</span><h2>Grant gateway access</h2></div><label><span>Email</span><input autoFocus type="email" value={email} onChange={event => setEmail(event.target.value)} /></label><label><span>Plan</span><select value={plan} onChange={event => setPlan(event.target.value)}><option value="pro">Pro</option><option value="team">Team</option><option value="plus">Plus</option></select></label><div><button className="secondary-button" onClick={() => setCreating(false)}>Cancel</button><button className="primary-button" disabled={!email.trim() || busy !== null} onClick={() => void create()}>{busy === "create" ? "Creating…" : "Create member"}</button></div></section> : null}
    {issuedToken ? <section className="issued-token" role="status"><div><b>Member created</b><p>Copy this setup token now. It is only returned by the creation operation.</p></div><code>{issuedToken}</code><button onClick={() => void navigator.clipboard.writeText(issuedToken)}>Copy token</button></section> : null}
    {error ? <div className="operation-feedback error" role="alert">{error}</div> : null}
    {state.status === "ready" ? <section className="admin-table member-list" aria-label="Gateway members">{state.data.map(member => <article className="member-row" key={member.id}><div className="member-avatar">{member.email.slice(0, 2).toUpperCase()}</div><div><b>{member.email}</b><small>Joined {new Date(member.created_at).toLocaleDateString()} · ID {member.id.slice(0, 8)}</small></div><span>{member.plan_type || "Member"}</span><StatusBadge tone={member.disabled ? "warning" : "success"}>{member.disabled ? "Disabled" : "Active"}</StatusBadge>{member.disabled ? <button disabled={busy !== null} onClick={() => void toggle(member)}>{busy === member.id ? "Saving…" : "Enable"}</button> : <button className="danger-action" disabled={busy !== null} onClick={() => setConfirmMember(member)}>Disable</button>}</article>)}</section> : null}
    <ConfirmDialog open={confirmMember !== null} onOpenChange={open => { if (!open) setConfirmMember(null); }} title="Disable member?" description={`${confirmMember?.email ?? "This member"} will lose gateway access and existing generated credentials will stop working.`} confirmLabel="Disable member" busy={confirmMember ? busy === confirmMember.id : false} onConfirm={() => { if (confirmMember) void toggle(confirmMember).then(success => { if (success) setConfirmMember(null); }); }} />
  </PageFrame>;
}

export function SystemPage({ state }: { state: ResourceState<SystemProjection> }) {
  const [operation, setOperation] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [confirmClear, setConfirmClear] = useState(false);
  const run = async (label: string, action: "reload-connections" | "clear-rate-limits"): Promise<boolean> => {
    if (operation) return false;
    setOperation(label); setMessage("");
    try { const result = await runSystemOperation(action); setMessage(action === "clear-rate-limits" ? `${Number(result.cleared ?? 0)} active rate limits cleared.` : "Provider connections reloaded."); return true; }
    catch (error) { setMessage(error instanceof Error ? error.message : `${label} failed`); return false; }
    finally { setOperation(null); }
  };
  return <PageFrame kicker="Administration" title="System" description="Measured runtime, persistence, and registry state owned by the gateway."><ResourceMessage state={state} loading="Loading system projection" empty="System projection is unavailable" />{state.status === "ready" ? <>
    <div className="evidence-strip"><span>{state.data.evidence.kind}</span><b>{state.data.evidence.source}</b><small>Generated {new Date(state.data.evidence.generated_at).toLocaleString()}</small></div>
    <section className="system-grid"><div className="system-card"><span>Gateway runtime</span><b>{state.data.runtime.status}</b><small>Started {new Date(state.data.runtime.started_at).toLocaleString()} · {formatUptime(state.data.runtime.uptime_seconds)}</small></div><div className="system-card"><span>Connection capacity</span><b>{state.data.capacity.connections_active} active</b><small>{state.data.capacity.connections_total} total · {state.data.capacity.connections_disabled} disabled · {state.data.capacity.connections_dead} dead</small></div><div className="system-card"><span>Provider registry</span><b>{state.data.capacity.providers_registered} providers</b><small>{state.data.capacity.declarative_providers} declarative specifications active</small></div>{state.data.persistence.map(item => <div className={`system-card ${item.healthy ? "" : "unavailable"}`} key={item.name}><span>{item.name}</span><b>{item.healthy ? "Healthy" : item.configured ? "Unavailable" : "Not configured"}</b><small>{item.detail}</small></div>)}</section>
    <section className="system-operations"><div><span>Operational controls</span><h2>Gateway maintenance</h2><p>These actions use current backend operations and do not imply persistence or recovery guarantees beyond their response.</p></div><div><button disabled={operation !== null} onClick={() => void run("Reload", "reload-connections")}>{operation === "Reload" ? "Reloading…" : "Reload connections"}</button><button disabled={operation !== null} onClick={() => setConfirmClear(true)}>{operation === "Clear" ? "Clearing…" : "Clear rate limits"}</button></div>{message ? <p role="status">{message}</p> : null}</section><ConfirmDialog open={confirmClear} onOpenChange={setConfirmClear} title="Clear active rate limits?" description="All provider cooldowns currently tracked by this gateway will be cleared. New upstream rate limits can be applied again immediately." confirmLabel="Clear rate limits" busy={operation === "Clear"} onConfirm={() => void run("Clear", "clear-rate-limits").then(success => { if (success) setConfirmClear(false); })} />
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
