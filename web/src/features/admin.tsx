import { useEffect, useState, type FormEvent } from "react";
import { contributeAPIKey, contributeGrok, createGatewayMember, exchangeAccountOAuth, exchangeAntigravityOAuth, isAuthorizationError, loadProviderConnectionsV2, mutateProviderConnection, renameProviderConnection, runSystemOperation, setGatewayMemberEnabled, startAccountOAuth, startAntigravityOAuth } from "../api";
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

export function ConnectionsPage({ state, onRefresh, isElevated, onAuthorizationLost, onRequireElevation }: { state: ResourceState<OperatorProviderConnectionV2[]>; onRefresh: () => Promise<void>; isElevated: boolean; onAuthorizationLost: () => void; onRequireElevation: () => void }) {
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [contributionTarget, setContributionTarget] = useState<OperatorProviderConnectionV2 | null | undefined>(undefined);
  const [operation, setOperation] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const selected = state.status === "ready" ? state.data.find(connection => connection.id === selectedID) ?? null : null;
  useEffect(() => { if (state.status === "ready" && selectedID && !state.data.some(connection => connection.id === selectedID)) setSelectedID(null); }, [state, selectedID]);
  const run = async (label: string, task: () => Promise<unknown>, requireElevation = false) => {
    if (operation) return;
    // Only destructive removal requires MFA elevation; reads and additions
    // (POST) are admin-signin only.
    if (requireElevation && !isElevated) { onRequireElevation(); return; }
    setOperation(label); setFeedback(null);
    try { await task(); await onRefresh(); setFeedback({ tone: "success", text: `${label} completed.` }); }
    catch (error) {
      if (isAuthorizationError(error)) { onAuthorizationLost(); return; }
      setFeedback({ tone: "error", text: error instanceof Error ? error.message : `${label} failed` });
    } finally { setOperation(null); }
  };
  return <PageFrame kicker="Administration" title="Connections" description="Accounts, limits, and health." action={<button className="primary-button" onClick={() => setContributionTarget(null)}>Add account</button>}>
    <ResourceMessage state={state} loading="Loading provider connections" empty="No provider connections are configured" />
    {feedback ? <div className={`operation-feedback ${feedback.tone}`} role={feedback.tone === "error" ? "alert" : "status"}>{feedback.text}</div> : null}
    {state.status === "ready" ? <section className="connections-workspace">
      <div className="admin-table connection-list" aria-label="Provider connections">{state.data.map(connection => { const runtime = connectionRuntimePresentation(connection); return <button className={`admin-row connection-row ${selectedID === connection.id ? "selected" : ""}`} key={connection.id} onClick={() => setSelectedID(connection.id)} aria-pressed={selectedID === connection.id}><div><b>{connection.identity.display_name || connection.provider_id}</b><small>{connection.provider_id} · {connection.plan_type || "plan unavailable"}{connection.is_primary ? " · Primary" : ""}</small></div><StatusBadge tone={runtime.tone}>{runtime.label}</StatusBadge><span>{runtime.summary || `${connection.inflight} in flight`}</span></button>; })}</div>
      {selected ? <ConnectionDetail connection={selected} operation={operation} onClose={() => setSelectedID(null)} onRun={run} onReauthorize={() => setContributionTarget(selected)} /> : <div className="connection-detail-empty"><span>Connection detail</span><b>Select a connection</b></div>}
    </section> : null}
    {contributionTarget !== undefined ? <AccountContribution initialProvider={contributionTarget?.provider_id as ContributableProvider | undefined} reauthorizing={contributionTarget ?? undefined} isElevated={isElevated} onRequireElevation={onRequireElevation} onClose={() => setContributionTarget(undefined)} onAdded={async () => { await onRefresh(); setContributionTarget(undefined); setFeedback({ tone: "success", text: contributionTarget ? "Account reauthorized." : "Account added." }); }} onAuthorizationLost={onAuthorizationLost} /> : null}
  </PageFrame>;
}

type ContributableProvider = "codex" | "antigravity" | "kimi" | "kimi-platform" | "minimax" | "zai" | "xiaomi" | "grok" | "deepseek" | "qwen" | "openrouter" | "nvidia" | "google-ai-image" | "bfl";
type ContributionMode = "oauth" | "key" | "json";
const CONTRIBUTION_PROVIDERS: Array<{ id: ContributableProvider; label: string; mode: ContributionMode; hint?: string }> = [
  { id: "codex", label: "Codex", mode: "oauth" }, { id: "antigravity", label: "Google Antigravity", mode: "oauth" },
  { id: "kimi", label: "Kimi Coding Plan", mode: "key", hint: "Use a Kimi Code Console coding-plan key." }, { id: "kimi-platform", label: "Kimi Platform", mode: "key" },
  { id: "minimax", label: "MiniMax", mode: "key" }, { id: "zai", label: "Z.ai", mode: "key", hint: "Use a GLM Coding Plan key." }, { id: "xiaomi", label: "Xiaomi", mode: "key", hint: "Use a MiMo Token Plan key." },
  { id: "deepseek", label: "DeepSeek", mode: "key" }, { id: "qwen", label: "Qwen", mode: "key" }, { id: "openrouter", label: "OpenRouter", mode: "key" }, { id: "nvidia", label: "NVIDIA", mode: "key" }, { id: "google-ai-image", label: "Google AI Studio · Image generation", mode: "key", hint: "Use a Google AI Studio API key. This is separate from Gemini LLM and Antigravity OAuth accounts." }, { id: "bfl", label: "Black Forest Labs", mode: "key" },
  { id: "grok", label: "Grok", mode: "json" },
];

function oauthCallbackCode(value: string, expectedState?: string) {
  const trimmed = value.trim();
  let callback: URL;
  try { callback = new URL(trimmed); }
  catch { throw new Error("Paste the complete final callback URL"); }
  const code = callback.searchParams.get("code")?.trim();
  const state = callback.searchParams.get("state")?.trim();
  if (!code) throw new Error("The callback URL does not contain an authorization code");
  if (expectedState && state !== expectedState) throw new Error("This callback URL belongs to a different authorization session");
  return code;
}

export function OAuthSessionDetails({ providerLabel, oauthURL, phase, copied, showCallbackInput, credential, onOpen, onCopy, onCredentialChange }: { providerLabel: string; oauthURL: string; phase: "idle" | "preparing" | "authorizing" | "exchanging"; copied: boolean; showCallbackInput: boolean; credential: string; onOpen: () => void; onCopy: () => void; onCredentialChange: (value: string) => void }) {
  return <><div className="account-oauth-actions"><a href={oauthURL} target="_blank" rel="noreferrer" onClick={onOpen}>Open authorization link ↗</a><button type="button" className="secondary-button" onClick={onCopy}>{copied ? "Copied ✓" : "Copy authorization link"}</button></div><div className="account-oauth-share"><span>Authorization link</span><input aria-label="Authorization link" readOnly value={oauthURL} onFocus={event => event.currentTarget.select()} /></div>{showCallbackInput ? <label className="account-callback-field"><span>Paste callback URL</span><p>After signing in elsewhere, copy the complete final localhost URL from that browser’s address bar and paste it here.</p><input autoFocus aria-label="Paste callback URL" placeholder="http://localhost:1455/auth/callback?code=…&amp;state=…" value={credential} onChange={event => onCredentialChange(event.target.value)} autoComplete="off" /></label> : <p>{phase === "exchanging" ? "Finishing authorization…" : `Open the link to continue with ${providerLabel}, or copy it to use another browser.`}</p>}</>;
}

export async function activateReauthorizedConnection(connection: OperatorProviderConnectionV2, exchangedAccountID: string | undefined) {
  if (!exchangedAccountID) throw new Error("Authorization succeeded but did not identify the updated account");
  if (exchangedAccountID !== connection.id) throw new Error("The authorized identity does not match this connection. Its lifecycle state was not changed.");
  // Production's recover operation clears Dead in memory; reapplying the
  // existing enabled state then persists that cleared lifecycle state without
  // changing whether the operator enabled or disabled the connection.
  await mutateProviderConnection(connection.id, "recover");
  await mutateProviderConnection(connection.id, connection.disabled ? "disable" : "enable");
  const connections = await loadProviderConnectionsV2();
  const restored = connections.find(candidate => candidate.id === connection.id);
  if (!restored || restored.dead || restored.runtime.status === "dead") throw new Error("Credentials were replaced, but production did not persist the restored account state");
  return restored;
}

export function AccountContribution({ onClose, onAdded, onAuthorizationLost, isElevated, onRequireElevation, initialProvider = "codex", reauthorizing }: { onClose: () => void; onAdded: () => Promise<void>; onAuthorizationLost?: () => void; isElevated?: boolean; onRequireElevation?: () => void; initialProvider?: ContributableProvider; reauthorizing?: OperatorProviderConnectionV2 }) {
  const [provider, setProvider] = useState<ContributableProvider>(initialProvider);
  const [credential, setCredential] = useState("");
  const [oauth, setOAuth] = useState<{ verifier?: string; sessionID?: string; state?: string; url: string; relayRequired?: boolean } | null>(null);
  const [phase, setPhase] = useState<"idle" | "preparing" | "authorizing" | "exchanging">("idle");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const [showCallbackInput, setShowCallbackInput] = useState(false);
  const [error, setError] = useState("");
  const [exchangedAccountID, setExchangedAccountID] = useState<string | undefined>();
  const selected = CONTRIBUTION_PROVIDERS.find(item => item.id === provider)!;

  const choose = (next: ContributableProvider) => { setProvider(next); setCredential(""); setOAuth(null); setPhase("idle"); setCopied(false); setShowCallbackInput(false); setError(""); setExchangedAccountID(undefined); };
  const copyAuthorizationLink = async () => {
    if (!oauth?.url) return;
    setShowCallbackInput(true);
    try {
      await navigator.clipboard.writeText(oauth.url);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2500);
    } catch {
      setError("Could not copy automatically. Select and copy the authorization link below.");
    }
  };
  const startOAuth = async () => {
    setBusy(true); setError(""); setPhase("preparing");
    try {
      const result = provider === "antigravity" ? await startAntigravityOAuth() : await startAccountOAuth(provider as "codex", 1455);
      if (!result.oauth_url || (provider === "antigravity" ? !result.session_id : !result.verifier)) throw new Error("Provider did not return an authorization session");
      setOAuth({ verifier: result.verifier, sessionID: result.session_id, state: result.state, url: result.oauth_url });
      setPhase("authorizing");
    } catch (failure) {
      setPhase("idle"); setError(failure instanceof Error ? failure.message : "Could not start authorization");
    } finally { setBusy(false); }
  };
  const submit = async (event: FormEvent) => {
    event.preventDefault(); if (busy) return;
    // Adding/reauthorizing an account is a POST and does not require MFA elevation.
    setBusy(true); setError("");
    try {
      if (selected.mode === "oauth") {
        if (reauthorizing && exchangedAccountID) {
          await activateReauthorizedConnection(reauthorizing, exchangedAccountID);
        } else {
          if (!oauth) { setBusy(false); await startOAuth(); return; }
          let result;
          if (provider === "antigravity") {
            if (!oauth.sessionID || !credential.trim()) throw new Error("Paste the authorization code or callback URL");
            result = await exchangeAntigravityOAuth(oauth.sessionID, credential, oauth.state || "");
          } else {
            const code = oauthCallbackCode(credential, oauth.state); if (!oauth.verifier) throw new Error("Authorization session expired");
            result = await exchangeAccountOAuth("codex", code, oauth.verifier);
          }
          if (reauthorizing) {
            setExchangedAccountID(result.account_id);
            await activateReauthorizedConnection(reauthorizing, result.account_id);
          }
        }
      } else if (selected.mode === "json") await contributeGrok(credential);
      else await contributeAPIKey(provider as Exclude<ContributableProvider, "codex" | "antigravity" | "grok">, credential);
      await onAdded();
    } catch (failure) {
      if (isAuthorizationError(failure) && onAuthorizationLost) { onAuthorizationLost(); return; }
      setError(failure instanceof Error ? failure.message : "Could not add account");
    } finally { setBusy(false); }
  };
  const manualOAuth = oauth && showCallbackInput;
  return <div className="account-modal-layer" role="presentation">
    <button className="account-modal-backdrop" aria-label="Close add account" onClick={onClose} />
    <form className="account-modal" role="dialog" aria-modal="true" aria-labelledby="add-account-title" onSubmit={submit}>
      <header><div><span>{reauthorizing ? "Existing provider connection" : "New provider connection"}</span><h2 id="add-account-title">{reauthorizing ? "Reauthorize account" : "Add account"}</h2><p>{reauthorizing ? `Sign in to ${reauthorizing.identity.display_name || reauthorizing.provider_id} again to replace its credentials. The same upstream account updates this connection rather than creating a duplicate.` : "Choose a provider and connect credentials to add gateway capacity."}</p></div><button type="button" className="icon-button" aria-label="Close" onClick={onClose}>×</button></header>
      <div className="account-modal-body">
        {reauthorizing ? <div className="reauthorize-identity"><span>Connection</span><b>{reauthorizing.identity.display_name || reauthorizing.provider_id}</b><small>{reauthorizing.identity.attributes?.email || reauthorizing.public_id}</small></div> : <label className="account-provider-field"><span>Provider</span><select value={provider} onChange={event => choose(event.target.value as ContributableProvider)}>{CONTRIBUTION_PROVIDERS.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}</select></label>}
        {selected.mode === "oauth" ? <section className="account-oauth">
          {!oauth ? <><p>Create a one-time link for this {selected.label} account.</p><button type="button" className="primary-button" disabled={busy} onClick={() => void startOAuth()}>{phase === "preparing" ? "Generating…" : "Generate authorization link"}</button></> : <OAuthSessionDetails providerLabel={selected.label} oauthURL={oauth.url} phase={phase} copied={copied} showCallbackInput={showCallbackInput} credential={credential} onOpen={() => setShowCallbackInput(true)} onCopy={() => void copyAuthorizationLink()} onCredentialChange={setCredential} />}
        </section> : selected.mode === "json" ? <label className="account-credential-field"><span>Grok auth JSON</span><textarea autoFocus value={credential} onChange={event => setCredential(event.target.value)} spellCheck={false} /></label> : <label className="account-credential-field"><span>{selected.label} API key</span><input autoFocus type="password" value={credential} onChange={event => setCredential(event.target.value)} autoComplete="off" />{selected.hint ? <small>{selected.hint}</small> : null}</label>}
        {error ? <div className="operation-feedback error" role="alert">{error}</div> : null}
      </div>
      <footer><button type="button" className="secondary-button" onClick={onClose}>Cancel</button>{selected.mode !== "oauth" || manualOAuth ? <button className="primary-button" disabled={busy || !credential.trim()}>{busy ? (reauthorizing ? "Reauthorizing…" : "Adding…") : (reauthorizing ? "Reauthorize account" : "Add account")}</button> : null}</footer>
    </form>
  </div>;
}

function ConnectionDetail({ connection, operation, onClose, onRun, onReauthorize }: { connection: OperatorProviderConnectionV2; operation: string | null; onClose: () => void; onRun: (label: string, task: () => Promise<unknown>, requireElevation?: boolean) => Promise<void>; onReauthorize: () => void }) {
  const [editing, setEditing] = useState(false);
  const [confirmDisable, setConfirmDisable] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const [name, setName] = useState(connection.identity.display_name);
  useEffect(() => { setName(connection.identity.display_name); }, [connection.id, connection.identity.display_name]);
  const busy = operation !== null;
  const runtime = connectionRuntimePresentation(connection);
  const total = (key: string) => Number(connection.totals[key] ?? 0).toLocaleString();
  return <aside className="connection-detail" aria-label="Connection detail">
    <header><div><span>Connection detail</span><h2>{connection.identity.display_name || connection.provider_id}</h2><small className="connection-public-id">Connection ID · <code>{connection.public_id}</code></small></div><button className="icon-button" aria-label="Close connection detail" onClick={onClose}>×</button></header>
    {editing ? <form className="connection-rename" onSubmit={event => { event.preventDefault(); void onRun("Rename", () => renameProviderConnection(connection.id, name)).then(() => setEditing(false)); }}><label><span>Display name</span><input autoFocus maxLength={120} value={name} onChange={event => setName(event.target.value)} /></label><div><button type="button" className="secondary-button" onClick={() => setEditing(false)}>Cancel</button><button className="primary-button" disabled={busy || !name.trim()}>Save name</button></div></form> : <button className="text-action" onClick={() => setEditing(true)}>Rename connection</button>}
    <div className="connection-facts"><Fact label="Provider" value={connection.provider_id} />{connection.plan_type ? <Fact label="Plan" value={connection.plan_type} /> : null}<Fact label="Status" value={runtime.label} /><Fact label="Enabled" value={connection.disabled ? "No" : "Yes"} /><Fact label="Active requests" value={String(connection.inflight)} /><Fact label="Primary connection" value={connection.is_primary ? "Yes" : "No"} />{connection.credit_balance !== undefined ? <Fact label="Provider credits" value={connection.credit_balance.toLocaleString()} /> : null}{connection.needs_verification ? <Fact label="Account verification" value="Required" /> : null}{connection.last_refresh && !connection.last_refresh.startsWith("0001-") ? <Fact label="Credentials refreshed" value={formatTimestamp(connection.last_refresh)} /> : null}{connection.expires_at && !connection.expires_at.startsWith("0001-") ? <Fact label="Access expires" value={formatTimestamp(connection.expires_at)} /> : null}</div>
    {connection.runtime.status_detail ? <div className={`connection-health-error ${connection.runtime.status === "healthy" ? "" : "active"}`}><b>Status details</b><p>{presentStatusDetail(connection)}</p></div> : null}
    {connection.health_error ? <div className="connection-health-error"><b>Connection error</b><p>{connection.health_error}</p></div> : null}
    {connection.identity.external_subject || Object.keys(connection.identity.attributes ?? {}).length ? <section className="connection-identity"><h3>Account details</h3>{connection.identity.external_subject ? <Fact label="Provider account ID" value={connection.identity.external_subject} /> : null}{Object.entries(connection.identity.attributes ?? {}).map(([key, value]) => <Fact key={key} label={accountAttributeLabel(key)} value={value} />)}</section> : null}
    <section><h3>Rate limits</h3><div className="connection-totals">{runtimeQuotaFacts(connection).map(fact => <Fact key={fact.label} label={fact.label} value={fact.value} />)}</div></section>
    {connection.provider_id === "codex" ? <ResetCreditsPanel connection={connection} busy={busy} operation={operation} confirmOpen={confirmReset} onConfirmOpenChange={setConfirmReset} onRun={onRun} /> : null}
    <section><h3>Usage totals</h3><div className="connection-totals"><Fact label="Input tokens" value={total("total_input_tokens")} /><Fact label="Cached tokens" value={total("total_cached_tokens")} /><Fact label="Output tokens" value={total("total_output_tokens")} /><Fact label="Billable tokens" value={total("total_billable_tokens")} /></div></section>
    <footer className="connection-actions">
      {isOAuthProvider(connection.provider_id) ? <><button className="primary-button" disabled={busy} onClick={onReauthorize}>Reauthorize account</button>{connection.disabled ? <button disabled={busy} onClick={() => void onRun("Enable", () => mutateProviderConnection(connection.id, "enable"))}>Enable</button> : <ConfirmDialog open={confirmDisable} onOpenChange={setConfirmDisable} title="Disable connection?" description={`${connection.identity.display_name || connection.provider_id} will immediately stop receiving gateway traffic. You can enable it again later.`} confirmLabel="Disable connection" busy={operation === "Disable"} onConfirm={() => void onRun("Disable", () => mutateProviderConnection(connection.id, "disable")).then(() => setConfirmDisable(false))}><button className="danger-action" disabled={busy}>Disable</button></ConfirmDialog>}</> : <><button disabled={busy} onClick={() => void onRun("Refresh", () => mutateProviderConnection(connection.id, "refresh"))}>{operation === "Refresh" ? "Refreshing…" : "Refresh credentials"}</button>{connection.dead ? <button disabled={busy} onClick={() => void onRun("Recover", () => mutateProviderConnection(connection.id, "recover"))}>Recover</button> : connection.disabled ? <button disabled={busy} onClick={() => void onRun("Enable", () => mutateProviderConnection(connection.id, "enable"))}>Enable</button> : <ConfirmDialog open={confirmDisable} onOpenChange={setConfirmDisable} title="Disable connection?" description={`${connection.identity.display_name || connection.provider_id} will immediately stop receiving gateway traffic. You can enable it again later.`} confirmLabel="Disable connection" busy={operation === "Disable"} onConfirm={() => void onRun("Disable", () => mutateProviderConnection(connection.id, "disable")).then(() => setConfirmDisable(false))}><button className="danger-action" disabled={busy}>Disable</button></ConfirmDialog>}</>}
      <ConfirmDialog open={confirmRemove} onOpenChange={setConfirmRemove} title="Remove connection permanently?" description={`${connection.identity.display_name || connection.provider_id} and its stored credentials will be permanently removed from this gateway. Historical usage records will remain.`} confirmLabel="Remove connection" busy={operation === "Remove"} onConfirm={() => void onRun("Remove", () => mutateProviderConnection(connection.id, "remove"), true).then(() => { setConfirmRemove(false); onClose(); })}><button className="danger-action" disabled={busy}>Remove connection</button></ConfirmDialog>
    </footer>
  </aside>;
}
function isOAuthProvider(provider: string): provider is "codex" | "antigravity" { return provider === "codex" || provider === "antigravity"; }
function ResetCreditsPanel({ connection, busy, operation, confirmOpen, onConfirmOpenChange, onRun }: {
  connection: OperatorProviderConnectionV2;
  busy: boolean;
  operation: string | null;
  confirmOpen: boolean;
  onConfirmOpenChange: (open: boolean) => void;
  onRun: (label: string, task: () => Promise<unknown>, requireElevation?: boolean) => Promise<void>;
}) {
  const credits = connection.reset_credits;
  const available = credits.available_count > 0;
  const managedHere = credits.management_available;
  const state = !managedHere
    ? !credits.known
      ? { tone: "unknown", eyebrow: "Status unavailable", title: "Reset credits", description: "Reset-credit status could not be checked." }
      : available
        ? { tone: "available", eyebrow: `${credits.available_count} available`, title: credits.available_count === 1 ? "A reset credit is available" : "Reset credits are available", description: "Redemption is not available from this gateway." }
        : { tone: "empty", eyebrow: "None available", title: "No reset credit is available", description: "No reset credit was reported for this account." }
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
      {available && !credits.redemption_available ? <span className="reset-credit-readonly">Redeem unavailable</span> : null}
      <a className="reset-credit-fallback" href={credits.dashboard_url} target="_blank" rel="noreferrer">Open Codex usage <span aria-hidden="true">↗</span></a>
    </footer>
  </section>;
}

function presentStatusDetail(connection: OperatorProviderConnectionV2) {
  let detail = connection.runtime.status_detail ?? "";
  const primary = limitName(connection.runtime.primary_window_minutes, "Short-term limit");
  const secondary = limitName(connection.runtime.secondary_window_minutes, "Long-term limit");
  detail = detail.replace(/primary quota/gi, primary).replace(/secondary quota/gi, secondary).replace(/quota/gi, "limit");
  return detail;
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
    summary: `${connection.runtime.status_detail ? presentStatusDetail(connection) : quotaSummary}${resetSummary}`,
  };
}
function runtimeQuotaFacts(connection: OperatorProviderConnectionV2): Array<{ label: string; value: string }> {
  const runtime = connection.runtime;
  const facts: Array<{ label: string; value: string }> = [];
  const addWindow = (used: number | undefined, minutes: number | undefined, resetAt: string | undefined, fallback: string) => {
    if (used === undefined && !resetAt) return;
    const name = limitName(minutes, fallback);
    if (used !== undefined) facts.push({ label: `${name} used`, value: `${used.toFixed(1)}%` });
    if (resetAt) facts.push({ label: `${name} resets`, value: formatTimestamp(resetAt) });
  };
  addWindow(runtime.primary_used_percent, runtime.primary_window_minutes, runtime.primary_reset_at, "Short-term limit");
  addWindow(runtime.secondary_used_percent, runtime.secondary_window_minutes, runtime.secondary_reset_at, "Long-term limit");
  if (runtime.rate_limit_until) facts.push({ label: "Available again", value: formatTimestamp(runtime.rate_limit_until) });
  if (!facts.length) facts.push({ label: "Limit information", value: "Unavailable" });
  return facts;
}
function limitName(minutes: number | undefined, fallback: string) {
  if (!minutes) return fallback;
  if (minutes === 300) return "5-hour limit";
  if (minutes === 10080) return "Weekly limit";
  if (minutes >= 40320 && minutes <= 44640) return "Monthly limit";
  return `${formatWindow(minutes)} limit`;
}
function accountAttributeLabel(key: string) {
  const labels: Record<string, string> = { email: "Email", region: "Region", account_id: "Account ID", organization_id: "Organization ID", workspace_id: "Workspace ID" };
  return labels[key] ?? key.replaceAll("_", " ");
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

export function MembersPage({ state, onRefresh, isElevated, onRequireElevation }: { state: ResourceState<GatewayMember[]>; onRefresh: () => Promise<void>; isElevated: boolean; onRequireElevation: () => void }) {
  const [creating, setCreating] = useState(false);
  const [email, setEmail] = useState("");
  const [plan, setPlan] = useState("pro");
  const [busy, setBusy] = useState<string | null>(null);
  const [issuedToken, setIssuedToken] = useState("");
  const [error, setError] = useState("");
  const [confirmMember, setConfirmMember] = useState<GatewayMember | null>(null);
  const create = async () => {
    if (!email.trim() || busy) return;
    if (!isElevated) { onRequireElevation(); return; }
    setBusy("create"); setError(""); setIssuedToken("");
    try { const result = await createGatewayMember(email.trim(), plan); setIssuedToken(result.token); setEmail(""); setCreating(false); await onRefresh(); }
    catch (failure) { setError(failure instanceof Error ? failure.message : "Member creation failed"); }
    finally { setBusy(null); }
  };
  const toggle = async (member: GatewayMember): Promise<boolean> => {
    if (busy) return false;
    if (!isElevated) { onRequireElevation(); return false; }
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

export function SystemPage({ state, isElevated, onRequireElevation }: { state: ResourceState<SystemProjection>; isElevated: boolean; onRequireElevation: () => void }) {
  const [operation, setOperation] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [confirmClear, setConfirmClear] = useState(false);
  const run = async (label: string, action: "reload-connections" | "clear-rate-limits"): Promise<boolean> => {
    if (operation) return false;
    if (!isElevated) { onRequireElevation(); return false; }
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
