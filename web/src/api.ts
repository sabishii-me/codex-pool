import type { AdminAccount, FriendSession, MFAStatus, ModelCatalog, PoolStats, SignalAnalytics } from "./types";

async function decode<T>(response: Response): Promise<T> {
  const data = (await response.json().catch(() => null)) as T | { error?: string } | null;
  if (!response.ok) {
    const message = data && typeof data === "object" && "error" in data ? data.error : null;
    throw new Error(message || `${response.status} ${response.statusText}`);
  }
  return data as T;
}

// loadSession hydrates the CLI-credential bundle for the signed-in Google
// account (the pool_session httpOnly cookie is sent automatically on this
// same-origin request). Returns null when there's no valid session instead
// of throwing, so callers can fall through to the sign-in screen.
export async function loadSession(): Promise<FriendSession | null> {
  const response = await fetch("/api/pool/session", { cache: "no-store" });
  if (response.status === 401) return null;
  return decode<FriendSession>(response);
}

export async function logout() {
  await fetch("/auth/logout", { method: "POST" });
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

export async function loadModelCatalog(): Promise<ModelCatalog> {
  const catalog = await decode<ModelCatalog>(await fetch("/api/pool/catalog", { cache: "no-store" }));
  return { models: catalog.models ?? [] };
}

export async function loadLivePiModels(downloadToken: string): Promise<string> {
  const config = await decode<unknown>(await fetch(`/config/pi/${encodeURIComponent(downloadToken)}`, { cache: "no-store" }));
  return JSON.stringify(config, null, 2);
}

export async function loadLiveCuteCodeSettings(downloadToken: string): Promise<string> {
  const config = await decode<unknown>(await fetch(`/config/cute-code/${encodeURIComponent(downloadToken)}`, { cache: "no-store" }));
  return JSON.stringify(config, null, 2);
}

// Admin routes require a signed-in, admin-listed, MFA-elevated session -
// all carried by httpOnly cookies, so no header/token is attached here.
// A 401/403 means "not currently elevated," which callers handle by
// falling back to the MFA prompt.

export async function loadAdminAccounts(): Promise<AdminAccount[]> {
  return decode(await fetch("/admin/accounts", { cache: "no-store" }));
}

export async function mutateAccount(accountID: string, action: "enable" | "disable" | "resurrect" | "refresh") {
  return decode<Record<string, unknown>>(await fetch(`/admin/accounts/${encodeURIComponent(accountID)}/${action}`, {
    method: "POST",
  }));
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

export async function contributeAPIKey(provider: "kimi" | "kimi-platform" | "minimax" | "zai" | "xiaomi" | "deepseek" | "qwen" | "openrouter" | "nvidia", apiKey: string) {
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

export async function startAccountOAuth(provider: "codex" | "claude") {
  return decode<AccountContributionResult>(await fetch(`/api/pool/accounts/${provider}/add`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: "{}",
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
