import { useEffect, useState } from "react";
import { createGatewayMember, isAuthorizationError, mutateProviderConnection, renameProviderConnection, runSystemOperation, setGatewayMemberEnabled } from "../api";
import type { ResourceState } from "../resource-state";
import type { GatewayMember, OperatorProviderConnectionV2, SystemProjection } from "../types";
import { ConfirmDialog } from "../components/confirm-dialog";
import { PageFrame, StatusBadge } from "../components/ui";

export function AdminCapabilityCheckingPage({ resource }: { resource: "Connections" | "Members" | "System" }) {
  return <PageFrame kicker="Administration" title={resource} description="Checking access…">
    <section className="admin-lock-screen"><span className="admin-lock-icon">◇</span><div><span>Admin access</span><h2>Checking access</h2></div></section>
  </PageFrame>;
}

export function AdminLockedPage({ resource, onUnlock }: { resource: "Connections" | "Members" | "System"; onUnlock: () => void }) {
  return <PageFrame kicker="Administration" title={resource} description="MFA verification required.">
    <section className="admin-lock-screen"><span className="admin-lock-icon">◆</span><div><span>Admin access</span><h2>{resource} is locked</h2><button className="primary-button" onClick={onUnlock}>Verify in Profile</button></div></section>
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
  return <PageFrame kicker="Administration" title="Connections" description="Upstream accounts, capacity, and health.">
    <ResourceMessage state={state} loading="Loading provider connections" empty="No provider connections are configured" />
    {feedback ? <div className={`operation-feedback ${feedback.tone}`} role={feedback.tone === "error" ? "alert" : "status"}>{feedback.text}</div> : null}
    {state.status === "ready" ? <section className="connections-workspace">
      <div className="admin-table connection-list" aria-label="Provider connections">{state.data.map(connection => { const runtime = connectionRuntimePresentation(connection); return <button className={`admin-row connection-row ${selectedID === connection.id ? "selected" : ""}`} key={connection.id} onClick={() => setSelectedID(connection.id)} aria-pressed={selectedID === connection.id}><div><b>{connection.identity.display_name || connection.provider_id}</b><small>{connection.provider_id} · {connection.plan_type || "plan unavailable"}{connection.is_primary ? " · Primary" : ""}</small></div><StatusBadge tone={runtime.tone}>{runtime.label}</StatusBadge><span>{runtime.summary || `${connection.inflight} in flight`}</span></button>; })}</div>
      {selected ? <ConnectionDetail connection={selected} operation={operation} onClose={() => setSelectedID(null)} onRun={run} /> : <div className="connection-detail-empty"><span>Connection detail</span><b>Select a connection</b></div>}
    </section> : null}
  </PageFrame>;
}

function ConnectionDetail({ connection, operation, onClose, onRun }: { connection: OperatorProviderConnectionV2; operation: string | null; onClose: () => void; onRun: (label: string, task: () => Promise<unknown>) => Promise<void> }) {
  const [editing, setEditing] = useState(false);
  const [confirmDisable, setConfirmDisable] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const [name, setName] = useState(connection.identity.display_name);
  useEffect(() => { setName(connection.identity.display_name); }, [connection.id, connection.identity.display_name]);
  const busy = operation !== null;
  const runtime = connectionRuntimePresentation(connection);
  const total = (key: string) => Number(connection.totals[key] ?? 0).toLocaleString();
  return <aside className="connection-detail" aria-label="Connection detail">
    <header><div><span>Connection detail</span><h2>{connection.identity.display_name || connection.provider_id}</h2><code>{connection.public_id}</code></div><button className="icon-button" aria-label="Close connection detail" onClick={onClose}>×</button></header>
    {editing ? <form className="connection-rename" onSubmit={event => { event.preventDefault(); void onRun("Rename", () => renameProviderConnection(connection.id, name)).then(() => setEditing(false)); }}><label><span>Display name</span><input autoFocus maxLength={120} value={name} onChange={event => setName(event.target.value)} /></label><div><button type="button" className="secondary-button" onClick={() => setEditing(false)}>Cancel</button><button className="primary-button" disabled={busy || !name.trim()}>Save name</button></div></form> : <button className="text-action" onClick={() => setEditing(true)}>Rename connection</button>}
    <div className="connection-facts"><Fact label="Provider" value={connection.provider_id} />{connection.plan_type ? <Fact label="Plan" value={connection.plan_type} /> : null}<Fact label="Runtime state" value={runtime.label} /><Fact label="Lifecycle" value={connection.disabled ? "Disabled" : "Enabled"} /><Fact label="In flight" value={String(connection.inflight)} /><Fact label="Primary" value={connection.is_primary ? "Yes" : "No"} />{connection.needs_verification ? <Fact label="Provider verification" value="Required" /> : null}{connection.last_refresh && !connection.last_refresh.startsWith("0001-") ? <Fact label="Last refresh" value={formatTimestamp(connection.last_refresh)} /> : null}{connection.expires_at && !connection.expires_at.startsWith("0001-") ? <Fact label="Credential expiry" value={formatTimestamp(connection.expires_at)} /> : null}</div>
    {connection.runtime.status_detail ? <div className={`connection-health-error ${connection.runtime.status === "healthy" ? "" : "active"}`}><b>Runtime evidence</b><p>{connection.runtime.status_detail}</p></div> : null}
    {connection.health_error ? <div className="connection-health-error"><b>Current health error</b><p>{connection.health_error}</p></div> : null}
    {connection.identity.external_subject || Object.keys(connection.identity.attributes ?? {}).length ? <section className="connection-identity"><h3>Provider identity</h3>{connection.identity.external_subject ? <Fact label="External subject" value={connection.identity.external_subject} /> : null}{Object.entries(connection.identity.attributes ?? {}).map(([key, value]) => <Fact key={key} label={key.replaceAll("_", " ")} value={value} />)}</section> : null}
    <section><h3>Runtime availability</h3><div className="connection-totals">{runtimeQuotaFacts(connection).map(fact => <Fact key={fact.label} label={fact.label} value={fact.value} />)}</div></section>
    {connection.provider_id === "codex" ? <ResetCreditsPanel connection={connection} busy={busy} operation={operation} confirmOpen={confirmReset} onConfirmOpenChange={setConfirmReset} onRun={onRun} /> : null}
    <section><h3>Measured totals</h3><div className="connection-totals"><Fact label="Input tokens" value={total("total_input_tokens")} /><Fact label="Cached tokens" value={total("total_cached_tokens")} /><Fact label="Output tokens" value={total("total_output_tokens")} /><Fact label="Billable tokens" value={total("total_billable_tokens")} /></div></section>
    <footer className="connection-actions"><button disabled={busy} onClick={() => void onRun("Refresh", () => mutateProviderConnection(connection.id, "refresh"))}>{operation === "Refresh" ? "Refreshing…" : "Refresh credentials"}</button>{connection.dead ? <button disabled={busy} onClick={() => void onRun("Recover", () => mutateProviderConnection(connection.id, "recover"))}>Recover</button> : connection.disabled ? <button disabled={busy} onClick={() => void onRun("Enable", () => mutateProviderConnection(connection.id, "enable"))}>Enable</button> : <ConfirmDialog open={confirmDisable} onOpenChange={setConfirmDisable} title="Disable connection?" description={`${connection.identity.display_name || connection.provider_id} will immediately stop receiving gateway traffic. You can enable it again later.`} confirmLabel="Disable connection" busy={operation === "Disable"} onConfirm={() => void onRun("Disable", () => mutateProviderConnection(connection.id, "disable")).then(() => setConfirmDisable(false))}><button className="danger-action" disabled={busy}>Disable</button></ConfirmDialog>}</footer>
  </aside>;
}
function ResetCreditsPanel({ connection, busy, operation, confirmOpen, onConfirmOpenChange, onRun }: {
  connection: OperatorProviderConnectionV2;
  busy: boolean;
  operation: string | null;
  confirmOpen: boolean;
  onConfirmOpenChange: (open: boolean) => void;
  onRun: (label: string, task: () => Promise<unknown>) => Promise<void>;
}) {
  const credits = connection.reset_credits;
  const available = credits.available_count > 0;
  const managedHere = credits.management_available;
  const state = !managedHere
    ? { tone: "unknown", eyebrow: "Managed in Production", title: "Reset credits", description: "Check and redeem credits from the Production gateway." }
    : !credits.known
      ? { tone: "unknown", eyebrow: "Not checked", title: "Reset credits", description: "Check Codex for available credits." }
      : available
      ? { tone: "available", eyebrow: `${credits.available_count} available`, title: credits.available_count === 1 ? "A reset credit is ready" : "Reset credits are ready", description: "A credit can reset eligible Codex rate-limit windows for this connection." }
      : { tone: "empty", eyebrow: "None available", title: "No reset credit is available", description: "Codex has not reported an available reset credit for this connection." };
  return <section className={`reset-credit-card ${state.tone}`} aria-labelledby={`reset-credit-${connection.public_id}`}>
    <header className="reset-credit-heading">
      <div className="reset-credit-icon" aria-hidden="true">↻</div>
      <div><span>{state.eyebrow}</span><h3 id={`reset-credit-${connection.public_id}`}>{state.title}</h3><p>{state.description}</p></div>
    </header>
    {(credits.expirations[0] || credits.retrieved_at) ? <dl className="reset-credit-meta">
      {credits.expirations[0] ? <div><dt>Earliest expiry</dt><dd>{formatTimestamp(credits.expirations[0])}</dd></div> : null}
      {credits.retrieved_at ? <div><dt>Inventory checked</dt><dd>{formatTimestamp(credits.retrieved_at)}</dd></div> : null}
    </dl> : null}
    <footer className="reset-credit-actions">
      {credits.inventory_refresh_available ? <button className={available ? "secondary-button" : "primary-button"} disabled={busy} onClick={() => void onRun("Check reset credits", () => mutateProviderConnection(connection.id, "refresh-reset-credits"))}>{operation === "Check reset credits" ? "Checking…" : credits.known ? "Check again" : "Check for credits"}</button> : null}
      {available && credits.redemption_available ? <ConfirmDialog open={confirmOpen} onOpenChange={onConfirmOpenChange} title="Redeem Codex reset credit?" description="The earliest-expiring credit will be used and quota will be refreshed. This cannot be undone." confirmLabel="Redeem reset credit" busy={operation === "Redeem reset credit"} onConfirm={() => void onRun("Redeem reset credit", () => mutateProviderConnection(connection.id, "redeem-reset-credit")).then(() => onConfirmOpenChange(false))}><button className="primary-button" disabled={busy}>{operation === "Redeem reset credit" ? "Redeeming…" : "Redeem now"}</button></ConfirmDialog> : null}
      {available && !credits.redemption_available && managedHere ? <span className="reset-credit-readonly">Redemption unavailable</span> : null}
      <a className="reset-credit-fallback" href={credits.dashboard_url} target="_blank" rel="noreferrer">Open ChatGPT <span aria-hidden="true">↗</span></a>
    </footer>
  </section>;
}

function connectionRuntimePresentation(connection: OperatorProviderConnectionV2): { label: string; tone: "success" | "warning"; summary: string } {
  const labels: Record<string, string> = { healthy: "Healthy", degraded: "Degraded", cooldown: "Cooldown", disabled: "Disabled", dead: "Dead", verification_required: "Verify" };
  const status = connection.runtime.status;
  const quota = connection.runtime.secondary_used_percent ?? connection.runtime.primary_used_percent;
  const quotaSummary = quota === undefined ? "" : `${quota.toFixed(0)}% used`;
  const reset = status === "cooldown" ? connection.runtime.rate_limit_until ?? connection.runtime.secondary_reset_at ?? connection.runtime.primary_reset_at : undefined;
  const resetSummary = reset ? ` · resets ${formatRelativeTimestamp(reset)}` : "";
  return {
    label: labels[status] ?? status,
    tone: status === "healthy" ? "success" : "warning",
    summary: `${connection.runtime.status_detail || quotaSummary}${resetSummary}`,
  };
}
function runtimeQuotaFacts(connection: OperatorProviderConnectionV2): Array<{ label: string; value: string }> {
  const runtime = connection.runtime;
  const facts: Array<{ label: string; value: string }> = [{ label: "State", value: connectionRuntimePresentation(connection).label }];
  if (runtime.primary_used_percent !== undefined) facts.push({ label: runtime.primary_window_minutes ? `${formatWindow(runtime.primary_window_minutes)} used` : "Primary used", value: `${runtime.primary_used_percent.toFixed(1)}%` });
  if (runtime.primary_reset_at) facts.push({ label: "Primary reset", value: formatTimestamp(runtime.primary_reset_at) });
  if (runtime.secondary_used_percent !== undefined) facts.push({ label: runtime.secondary_window_minutes ? `${formatWindow(runtime.secondary_window_minutes)} used` : "Secondary used", value: `${runtime.secondary_used_percent.toFixed(1)}%` });
  if (runtime.secondary_reset_at) facts.push({ label: "Secondary reset", value: formatTimestamp(runtime.secondary_reset_at) });
  if (runtime.rate_limit_until) facts.push({ label: "Cooldown until", value: formatTimestamp(runtime.rate_limit_until) });
  if (runtime.usage_source) facts.push({ label: "Evidence source", value: runtime.usage_source });
  if (runtime.usage_retrieved_at) facts.push({ label: "Evidence retrieved", value: formatTimestamp(runtime.usage_retrieved_at) });
  return facts;
}
function formatWindow(minutes: number) { if (minutes % 10080 === 0) return `${minutes / 10080}w window`; if (minutes % 1440 === 0) return `${minutes / 1440}d window`; if (minutes % 60 === 0) return `${minutes / 60}h window`; return `${minutes}m window`; }
export function formatRelativeTimestamp(value: string) {
  const totalSeconds = Math.max(0, Math.ceil((new Date(value).getTime() - Date.now()) / 1000));
  if (totalSeconds === 0) return "now";
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const parts: string[] = [];
  if (days) parts.push(`${days}d`);
  if (days || hours) parts.push(`${hours}h`);
  if (days || hours || minutes) parts.push(`${minutes}m`);
  parts.push(`${seconds}s`);
  return `in ${parts.join(" ")}`;
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
  return <PageFrame kicker="Administration" title="Members" description="People with access to AI Pool." action={<button className="primary-button" onClick={() => setCreating(true)}>Add member</button>}>
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
  return <PageFrame kicker="Administration" title="System" description="Runtime and storage health."><ResourceMessage state={state} loading="Loading system" empty="System data is unavailable" />{state.status === "ready" ? <>
    <div className="evidence-strip"><span>{state.data.evidence.kind}</span><b>{state.data.evidence.source}</b><small>Generated {new Date(state.data.evidence.generated_at).toLocaleString()}</small></div>
    <section className="system-grid"><div className="system-card"><span>Gateway runtime</span><b>{state.data.runtime.status}</b><small>{state.data.runtime.version} · {state.data.runtime.commit.slice(0, 12)}</small><small>Started {new Date(state.data.runtime.started_at).toLocaleString()} · {formatUptime(state.data.runtime.uptime_seconds)}</small></div><div className="system-card"><span>Connection capacity</span><b>{state.data.capacity.connections_active} active</b><small>{state.data.capacity.connections_total} total · {state.data.capacity.connections_disabled} disabled · {state.data.capacity.connections_dead} dead</small></div><div className="system-card"><span>Provider registry</span><b>{state.data.capacity.providers_registered} providers</b><small>{state.data.capacity.declarative_providers} declarative specifications active</small></div>{state.data.persistence.map(item => <div className={`system-card ${item.healthy ? "" : "unavailable"}`} key={item.name}><span>{item.name}</span><b>{item.healthy ? "Healthy" : item.configured ? "Unavailable" : "Not configured"}</b><small>{item.detail}</small></div>)}</section>
    <section className="system-operations"><div><span>Maintenance</span><h2>System actions</h2></div><div><button disabled={operation !== null} onClick={() => void run("Reload", "reload-connections")}>{operation === "Reload" ? "Reloading…" : "Reload connections"}</button><button disabled={operation !== null} onClick={() => setConfirmClear(true)}>{operation === "Clear" ? "Clearing…" : "Clear rate limits"}</button></div>{message ? <p role="status">{message}</p> : null}</section><ConfirmDialog open={confirmClear} onOpenChange={setConfirmClear} title="Clear active rate limits?" description="All provider cooldowns currently tracked by this gateway will be cleared. New upstream rate limits can be applied again immediately." confirmLabel="Clear rate limits" busy={operation === "Clear"} onConfirm={() => void run("Clear", "clear-rate-limits").then(success => { if (success) setConfirmClear(false); })} />
  </> : null}</PageFrame>;
}
function formatUptime(seconds: number) { const days = Math.floor(seconds / 86400); const hours = Math.floor(seconds % 86400 / 3600); const minutes = Math.floor(seconds % 3600 / 60); return `Uptime ${days ? `${days}d ` : ""}${hours}h ${minutes}m`; }

function ResourceMessage<T>({ state, loading, empty }: { state: ResourceState<T>; loading: string; empty: string }) {
  if (state.status === "idle") return <div className="resource-message"><b>Admin access required</b></div>;
  if (state.status === "loading") return <div className="resource-message"><b>{loading}</b></div>;
  if (state.status === "error") return <div className="resource-message error"><b>Could not load data</b><p>{state.message}</p></div>;
  if (state.status === "empty") return <div className="resource-message"><b>{empty}</b></div>;
  return null;
}
