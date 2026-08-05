import type { GatewayHealth, GatewayMember, OperatorProviderConnection, OperatorProviderConnectionV2, GatewaySession, MFAStatus, ModelCatalog, ModelRoutingProjection, PoolStats, PoolUserStats, SetupClientsProjection, SignalAnalytics, SystemProjection, UsageEconomicsProjection, UsageProjection } from "./types";

export class APIError extends Error {
  constructor(message: string, readonly status: number) { super(message); this.name = "APIError"; }
}

export function isAuthorizationError(error: unknown): boolean {
  return error instanceof APIError && (error.status === 401 || error.status === 403);
}

async function decode<T>(response: Response): Promise<T> {
  const text = await response.text();
  let data: T | { error?: string } | null = null;
  if (text) {
    try { data = JSON.parse(text) as T | { error?: string }; }
    catch { data = null; }
  }
  if (!response.ok) {
    const structured = data && typeof data === "object" && "error" in data ? data.error : null;
    const plain = data === null ? text.trim() : "";
    throw new APIError(structured || plain || `${response.status} ${response.statusText}`, response.status);
  }
  return data as T;
}

// loadSession hydrates the CLI-credential bundle for the signed-in Google
// account (the pool_session httpOnly cookie is sent automatically on this
// same-origin request). Returns null when there's no valid session instead
// of throwing, so callers can fall through to the sign-in screen.
export async function loadSession(): Promise<GatewaySession | null> {
  const response = await fetch("/api/pool/session", { cache: "no-store" });
  if (response.status === 401) return null;
  return decode<GatewaySession>(response);
}

export async function logout() {
  await fetch("/auth/logout", { method: "POST" });
}

export async function loadSystemProjection(): Promise<SystemProjection> {
  return decode(await fetch("/api/v2/system", { cache: "no-store" }));
}

export async function runSystemOperation(action: "reload-connections" | "clear-rate-limits") {
  return decode<Record<string, unknown>>(await fetch(`/api/v2/system/${action}`, { method: "POST" }));
}

export async function loadGatewayHealth(): Promise<GatewayHealth> {
  return decode(await fetch("/healthz", { cache: "no-store" }));
}
export async function loadGatewayMembers(): Promise<{ users: GatewayMember[]; count: number }> {
  return decode(await fetch("/admin/pool-users", { cache: "no-store" }));
}

export async function createGatewayMember(email: string, planType: string): Promise<{ user: GatewayMember; token: string }> {
  return decode(await fetch("/admin/pool-users", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email, plan_type: planType }) }));
}

export async function setGatewayMemberEnabled(id: string, enabled: boolean) {
  return decode<Record<string, unknown>>(await fetch(`/admin/pool-users/${encodeURIComponent(id)}/${enabled ? "enable" : "disable"}`, { method: "POST" }));
}

export async function loadPoolUsers(): Promise<{ users: PoolUserStats[]; total_users: number }> {
  return decode(await fetch("/api/pool/users", { cache: "no-store" }));
}

export async function loadPoolStats(): Promise<PoolStats> {
  return decode(await fetch("/api/pool/stats", { cache: "no-store" }));
}

export async function loadSignalAnalytics(): Promise<SignalAnalytics> {
  const signal = await decode<SignalAnalytics>(await fetch("/api/pool/signal?weeks=6", { cache: "no-store" }));
  return {
    ...signal,
    economics: signal.economics ?? [],
    hourly: signal.hourly ?? [],
    origin_weekly: signal.origin_weekly ?? [],
    model_daily: signal.model_daily ?? [],
    quota_capacity: signal.quota_capacity ?? [],
    model_efficiency: signal.model_efficiency ?? [],
    reset_observations: signal.reset_observations ?? [],
    quota_generated_at: signal.quota_generated_at,
  };
}

export async function loadModelRouting(modelID: string): Promise<ModelRoutingProjection> {
  return decode(await fetch(`/api/v2/models/${encodeURIComponent(modelID)}/routing`, { cache: "no-store" }));
}

export async function loadModelCatalog(): Promise<ModelCatalog> {
  const catalog = await decode<ModelCatalog>(await fetch("/api/pool/catalog", { cache: "no-store" }));
  return { models: catalog.models ?? [] };
}

export interface DashboardResources {
  stats?: PoolStats;
  signal?: SignalAnalytics;
  catalog?: ModelCatalog;
  errors: string[];
}

function rejectionMessage(label: string, result: PromiseRejectedResult) {
  const detail = result.reason instanceof Error ? result.reason.message : "unavailable";
  return `${label}: ${detail}`;
}

// Dashboard resources have different durability and refresh costs. A transient
// analytics or catalog failure must not discard fresh pool statistics (or vice
// versa), so callers can preserve each last-known-good resource independently.
export async function loadDashboardResources(): Promise<DashboardResources> {
  const [stats, signal, catalog] = await Promise.allSettled([
    loadPoolStats(),
    loadSignalAnalytics(),
    loadModelCatalog(),
  ]);
  return {
    stats: stats.status === "fulfilled" ? stats.value : undefined,
    signal: signal.status === "fulfilled" ? signal.value : undefined,
    catalog: catalog.status === "fulfilled" ? catalog.value : undefined,
    errors: [
      ...(stats.status === "rejected" ? [rejectionMessage("pool stats", stats)] : []),
      ...(signal.status === "rejected" ? [rejectionMessage("analytics", signal)] : []),
      ...(catalog.status === "rejected" ? [rejectionMessage("model catalog", catalog)] : []),
    ],
  };
}

export async function loadUsageProjection(scope: "me" | "pool" | "member", options?: { memberId?: string; hours?: number; days?: number }): Promise<UsageProjection> {
  const query = new URLSearchParams({ scope });
  if (options?.memberId) query.set("member_id", options.memberId);
  if (options?.hours) query.set("hours", String(options.hours));
  if (options?.days) query.set("days", String(options.days));
  return decode(await fetch(`/api/v2/usage?${query}`, { cache: "no-store" }));
}

export async function loadUsageEconomics(): Promise<UsageEconomicsProjection> {
  return decode(await fetch("/api/v2/usage/economics?scope=pool", { cache: "no-store" }));
}

export async function loadSetupClients(): Promise<SetupClientsProjection> {
  return decode(await fetch("/api/v2/setup/clients", { cache: "no-store" }));
}

export async function loadSetupConfig(configURL: string): Promise<string> {
  const response = await fetch(configURL, { cache: "no-store" });
  const config = await decode<unknown>(response);
  return JSON.stringify(config, null, 2);
}

// Admin routes require a signed-in, admin-listed, MFA-elevated session -
// all carried by httpOnly cookies, so no header/token is attached here.
// A 401/403 means "not currently elevated," which callers handle by
// falling back to the MFA prompt.

export async function loadOperatorProviderConnections(): Promise<OperatorProviderConnection[]> {
  return decode(await fetch("/admin/accounts", { cache: "no-store" }));
}

/** @deprecated Use loadOperatorProviderConnections. */
export async function loadAdminAccounts(): Promise<OperatorProviderConnection[]> {
  return loadOperatorProviderConnections();
}

export async function loadProviderConnectionsV2(): Promise<OperatorProviderConnectionV2[]> {
  return decode(await fetch("/api/v2/provider-connections", { cache: "no-store" }));
}

export async function renameProviderConnection(accountID: string, displayName: string): Promise<{ status: string; connection_id: string }> {
  return decode(await fetch(`/api/v2/provider-connections/${encodeURIComponent(accountID)}/identity`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ display_name: displayName }),
  }));
}

export async function mutateProviderConnection(accountID: string, action: "enable" | "disable" | "recover" | "remove" | "refresh" | "refresh-reset-credits" | "redeem-reset-credit") {
  return decode<Record<string, unknown>>(await fetch(`/api/v2/provider-connections/${encodeURIComponent(accountID)}/${action}`, {
    method: "POST",
  }));
}

/** @deprecated Use mutateProviderConnection. */
export async function mutateAccount(accountID: string, action: "enable" | "disable" | "resurrect" | "refresh") {
  return mutateProviderConnection(accountID, action === "resurrect" ? "recover" : action);
}

export interface MFAEnrollResult {
  secret: string;
  otpauth_url: string;
}

export interface MFAConfirmResult {
  success: boolean;
  recovery_codes: string[];
}

export interface MFARegenerateResult {
  secret: string;
  otpauth_url: string;
  recovery_codes: string[];
}

export async function checkMFAStatus(): Promise<MFAStatus> {
  return decode(await fetch("/api/admin/mfa/status", { cache: "no-store" }));
}

export async function enrollMFA(): Promise<MFAEnrollResult> {
  return decode(await fetch("/api/admin/mfa/enroll", { method: "POST" }));
}

export async function confirmMFA(code: string): Promise<MFAConfirmResult> {
  return decode(await fetch("/api/admin/mfa/confirm", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code }),
  }));
}

export async function verifyMFA(input: { code?: string; recoveryCode?: string }): Promise<{ success: boolean }> {
  return decode(await fetch("/api/admin/mfa/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code: input.code, recovery_code: input.recoveryCode }),
  }));
}

export async function regenerateMFA(): Promise<MFARegenerateResult> {
  return decode(await fetch("/api/admin/mfa/regenerate", { method: "POST" }));
}

export async function regenerateRecoveryCodes(): Promise<{ recovery_codes: string[] }> {
  return decode(await fetch("/api/admin/mfa/regenerate-codes", { method: "POST" }));
}

export interface AccountContributionResult {
  success?: boolean;
  account_id?: string;
  oauth_url?: string;
  verifier?: string;
  state?: string;
	  session_id?: string;
	  status?: "pending" | "exchanging" | "complete" | "error";
  relay_required?: boolean;
	  error?: string;
}

export async function contributeAPIKey(provider: "kimi" | "kimi-platform" | "minimax" | "zai" | "xiaomi" | "deepseek" | "qwen" | "openrouter" | "nvidia" | "google-ai-image" | "bfl", apiKey: string) {
  return decode<AccountContributionResult>(await fetch(`/api/pool/accounts/${provider}/add`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ api_key: apiKey }),
  }));
}

export async function contributeGrok(authJSON: string) {
  return decode<AccountContributionResult>(await fetch("/api/pool/accounts/grok/add", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ auth_json: authJSON }),
  }));
}

export interface CodexBrokerLease {
  lease_id: string;
  port: 1455 | 1457;
  expires_at: string;
}

export async function prepareCodexOAuthBroker(gatewayOrigin: string): Promise<CodexBrokerLease> {
  let response: Response;
  try {
    response = await fetch("http://127.0.0.1:1460/v1/leases", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ gateway_origin: gatewayOrigin }),
    });
  } catch {
    throw new Error("Codex OAuth broker is not running. Install or start the local broker before connecting Codex.");
  }
  return decode<CodexBrokerLease>(response);
}

export async function cancelCodexOAuthBrokerLease(): Promise<void> {
  await fetch("http://127.0.0.1:1460/v1/leases", { method: "DELETE" }).catch(() => undefined);
}

export async function startAccountOAuth(provider: "codex" | "claude", redirectPort?: number) {
  return decode<AccountContributionResult>(await fetch(`/api/pool/accounts/${provider}/add`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(provider === "codex" ? { redirect_port: redirectPort } : {}),
  }));
}

export async function codexOAuthStatus(sessionID: string) {
  return decode<AccountContributionResult>(await fetch("/api/pool/accounts/codex/status", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ session_id: sessionID }),
  }));
}

export async function exchangeAccountOAuth(provider: "codex" | "claude", code: string, verifier: string) {
  return decode<AccountContributionResult>(await fetch(`/api/pool/accounts/${provider}/exchange`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ code, verifier }),
  }));
}

export async function startAntigravityOAuth() {
  return decode<AccountContributionResult>(await fetch("/api/pool/accounts/antigravity/add", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
  }));
}

export async function antigravityOAuthStatus(sessionID: string) {
  return decode<AccountContributionResult>(await fetch("/api/pool/accounts/antigravity/status", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ session_id: sessionID }),
  }));
}

export async function exchangeAntigravityOAuth(sessionID: string, value: string, state: string) {
  const trimmed = value.trim();
  const isCallback = /^https?:\/\//i.test(trimmed);
  return decode<AccountContributionResult>(await fetch("/api/pool/accounts/antigravity/exchange", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
	    body: JSON.stringify({ session_id: sessionID, ...(isCallback ? { callback_url: trimmed } : { code: trimmed, state }) }),
  }));
}

export async function reloadAccounts() {
  const response = await fetch("/admin/reload", { method: "POST" });
  if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
}
