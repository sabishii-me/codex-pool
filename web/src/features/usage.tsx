import type { PoolStats, SignalAnalytics } from "../types";
import { CardHeader, Metric, PageFrame, SplineChart, compact } from "../components/ui";

export function UsagePage({ stats, signal, operator }: { stats: PoolStats | null; signal: SignalAnalytics | null; operator: boolean }) {
  const economics = signal?.economics ?? [];
  const latest = economics.at(-1);
  const previous = economics.at(-2);
  const change = latest && previous ? ((latest.daily_api_value - previous.daily_api_value) / Math.max(previous.daily_api_value, 0.01)) * 100 : null;
  return <PageFrame kicker={operator ? "Operations" : "Member workspace"} title={operator ? "Usage & economics" : "My usage"} description={operator ? "Pool-wide usage and economics with evidence state." : "Your measured gateway activity."}>
    <section className="metric-grid"><Metric label="Processed tokens" value={compact(stats?.aggregate.total_billable_tokens ?? 0)} note="Measured" /><Metric label="Requests" value={signal ? signal.hourly.reduce((n, x) => n + x.request_count, 0).toLocaleString() : "—"} note="Available data" /><Metric label="API value" value={latest ? `$${latest.daily_api_value.toFixed(2)}` : "—"} note="Latest measured day" /><Metric label="Day-over-day" value={change === null ? "—" : `${change >= 0 ? "+" : ""}${change.toFixed(1)}%`} note="Daily API value" /></section>
    <section className="metric-grid"><Metric label="Input tokens" value={compact(stats?.aggregate.total_input_tokens ?? 0)} note="Measured" /><Metric label="Cached tokens" value={compact(stats?.aggregate.total_cached_tokens ?? 0)} note={`${stats?.aggregate.overall_cache_hit_rate_pct.toFixed(1) ?? "—"}% cache hit rate`} /><Metric label="Output tokens" value={compact(stats?.aggregate.total_output_tokens ?? 0)} note="Measured" /><Metric label="ROI" value={stats ? `${stats.aggregate.overall_roi.toFixed(1)}x` : "—"} note="Measured economics" /></section>
    <section className="bento-card chart-card"><CardHeader title={operator ? "Pool activity" : "Your activity"} subtitle="Measured request volume" /><SplineChart values={signal?.hourly.slice(-30).map(x => x.request_count) ?? []} /></section>
  </PageFrame>;
}
