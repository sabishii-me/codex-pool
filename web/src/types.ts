export type Provider = string;

export interface FriendSession {
  public_url: string;
  email: string;
  is_admin: boolean;
  mfa_enrolled: boolean;
  origin_id: string;
  download_token: string;
  auth_json: string;
  gemini_auth_json: string;
  gemini_api_key: string;
  claude_api_key: string;
  pi_models_json: string;
  cute_code_settings_json: string;
}

export interface MFAStatus {
  enrolled: boolean;
  elevated: boolean;
  recovery_codes_remaining: number;
}

export interface ProviderConnectionStats {
  id: string;
  display_name: string;
  external_subject?: string;
  identity_attributes?: Record<string, string>;
  /** @deprecated Use external_subject/identity_attributes. */
  upstream_account_id?: string;
  /** @deprecated Use identity_attributes.email. */
  account_email?: string;
  type: Provider;
  plan_type: string;
  status: "healthy" | "degraded" | "cooldown" | "dead";
  penalty: number;
  primary_window_used_pct: number;
  secondary_window_used_pct: number;
  primary_window_available: boolean;
  secondary_window_available: boolean;
  primary_reset_minutes: number;
  secondary_reset_minutes: number;
  primary_window_minutes: number;
  secondary_window_minutes: number;
  primary_pace_ratio: number;
  secondary_pace_ratio: number;
  account_added_at?: string;
  total_input_tokens: number;
  total_cached_tokens: number;
  total_output_tokens: number;
  total_reasoning_tokens: number;
  total_billable_tokens: number;
  cache_hit_rate_pct: number;
  score: number;
  score_tooltip?: string;
  is_primary: boolean;
  subscription_cost_monthly: number;
  subscription_spend: number;
  subscription_billing_cycles: number;
  subscription_label: string;
  api_cost_estimate: number;
  api_cost_last_30d: number;
  roi: number;
  reset_credits_available?: number;
  reset_credit_expirations?: string[];
  reset_credits_known?: boolean;
}

/** @deprecated Use ProviderConnectionStats. */
export type AccountStats = ProviderConnectionStats;

export interface PoolStats {
  total_accounts: number;
  active_accounts: number;
  total_pool_users: number;
  last_24h_tokens: number;
  accounts: ProviderConnectionStats[];
  aggregate: {
    total_input_tokens: number;
    total_cached_tokens: number;
    total_output_tokens: number;
    total_reasoning_tokens: number;
    total_billable_tokens: number;
    overall_cache_hit_rate_pct: number;
    total_api_cost: number;
    total_subscription_cost: number;
    total_subscription_monthly: number;
    overall_roi: number;
  };
  capacity_analysis?: {
    total_samples: number;
    model_formula: string;
    plans: Record<string, {
      sample_count: number;
      confidence: "low" | "medium" | "high";
      total_input_tokens: number;
      total_output_tokens: number;
      total_cached_tokens: number;
      total_reasoning_tokens: number;
      output_multiplier: number;
      estimated_5h_capacity: number;
      estimated_7d_capacity: number;
    }>;
  };
  cyber_policy?: {
    healthy: boolean;
    cyber_candidates_available: number;
    counters: Record<string, number>;
  };
  generated_at: string;
}

export interface ModelDescriptor {
  id: string;
  name?: string;
  protocol: string;
  contextWindow?: number;
  description?: string;
  provider: Provider;
  upstream_id?: string;
  max_output_tokens?: number;
  protocols?: string[];
  modalities?: string[];
  capabilities?: Record<string, boolean>;
  supported_mime_types?: string[];
  recommended?: boolean;
  quota_remaining_fraction?: number;
  aliases?: string[];
  supporting_accounts?: number;
  available_accounts?: number;
  available_now: boolean;
  next_reset_at?: string;
  stale?: boolean;
}

export interface ModelCatalog {
  models: ModelDescriptor[];
}

export interface SignalEconomicsPoint {
  date: string;
  daily_api_value: number;
  cumulative_api_value: number;
  cumulative_subscription_spend: number;
  provider_api_value: Record<string, number>;
}

export interface HourlyUsage {
  hour: string;
  account_type: Provider | "unknown";
  input_tokens: number;
  cached_tokens: number;
  output_tokens: number;
  reasoning_tokens: number;
  billable_tokens: number;
  request_count: number;
}

export interface OriginWeeklyUsage {
  week_start: string;
  origin_id: string;
  account_id: string;
  account_type: Provider | "unknown";
  input_tokens: number;
  cached_tokens: number;
  output_tokens: number;
  reasoning_tokens: number;
  billable_tokens: number;
  request_count: number;
}

export interface ModelDailyUsage {
  date: string;
  account_type: Provider | "unknown";
  model: string;
  input_tokens: number;
  cached_tokens: number;
  output_tokens: number;
  reasoning_tokens: number;
  request_count: number;
  cost_usd: number;
}

export interface QuotaCapacityPoint {
  week_start: string;
  account_type: Provider | "unknown";
  plan_type: string;
  window_minutes: number;
  estimated_window_tokens: number;
  estimated_weekly_tokens: number;
  low_estimate_tokens: number;
  high_estimate_tokens: number;
  observed_quota_pct: number;
  interval_count: number;
  request_count: number;
  confidence: "low" | "medium" | "high";
}

export interface ModelQuotaEfficiency {
  account_type: Provider | "unknown";
  model: string;
  tokens: number;
  request_count: number;
  api_value: number;
  observed_quota_pct: number;
  api_value_per_quota_pct: number;
  relative_subsidy: number;
  interval_count: number;
  confidence: "low" | "medium" | "high";
}

export interface ResetObservation {
  account_id: string;
  account_type: Provider | "unknown";
  observed_at: string;
  expected_at?: string;
  deviation_minutes?: number;
  from_used_pct: number;
  to_used_pct: number;
  timing: "early" | "late" | "on_time" | "observed";
}

export interface SignalAnalytics {
  generated_at: string;
  origin_data_since: string;
  economics: SignalEconomicsPoint[];
  hourly: HourlyUsage[];
  origin_weekly: OriginWeeklyUsage[];
  model_daily: ModelDailyUsage[];
  quota_capacity: QuotaCapacityPoint[];
  model_efficiency: ModelQuotaEfficiency[];
  reset_observations: ResetObservation[];
  quota_generated_at?: string;
}

export interface ProviderConnectionIdentity {
  display_name: string;
  external_subject?: string;
  attributes?: Record<string, string>;
}

export interface OperatorProviderConnectionV2 {
  id: string;
  public_id: string;
  provider_id: Provider;
  identity: ProviderConnectionIdentity;
  plan_type?: string;
  disabled: boolean;
  dead: boolean;
  needs_verification?: boolean;
  verification_url?: string;
  health_error?: string;
  cyber_access?: boolean;
  inflight: number;
  expires_at?: string;
  last_refresh?: string;
  penalty: number;
  score: number;
  score_tooltip?: string;
  is_primary: boolean;
  usage: Record<string, unknown>;
  totals: Record<string, number>;
}

export interface OperatorProviderConnection {
  id: string;
  public_id: string;
  type: Provider;
  display_name: string;
  external_subject?: string;
  identity_attributes?: Record<string, string>;
  plan_type: string;
  account_id?: string;
  id_token_chatgpt_account_id?: string;
  email?: string;
  disabled: boolean;
  dead: boolean;
  inflight: number;
  expires_at?: string;
  last_refresh?: string;
  penalty: number;
  score: number;
  score_tooltip?: string;
  is_primary: boolean;
  usage: Record<string, unknown>;
  totals: Record<string, number>;
}

/** @deprecated Use OperatorProviderConnection. */
export type AdminAccount = OperatorProviderConnection;
