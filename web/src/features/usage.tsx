import type { PoolStats, SignalAnalytics } from "../types";
import { CardHeader, Metric, PageFrame, SplineChart, compact } from "../components/ui";

export function UsagePage({ stats, signal, operator, originId }: { stats: PoolStats | null; signal: SignalAnalytics | null; operator: boolean; originId: string }) {
  const economics = signal?.economics ?? [];
  const latest = economics.at(-1);
  const previous = economics.at(-2);
  const change = latest && previous ? ((latest.daily_api_value - previous.daily_api_value) / Math.max(previous.daily_api_value, 0.01)) * 100 : null;
  const memberRows = signal?.origin_weekly.filter(row => row.origin_id === originId) ?? [];
  const member = memberRows.reduce((total, row) => ({ tokens: total.tokens + row.billable_tokens, requests: total.requests + row.request_count, input: total.input + row.input_tokens, output: total.output + row.output_tokens }), { tokens: 0, requests: 0, input: 0, output: 0 });
  const tokens = operator ? stats?.aggregate.total_billable_tokens ?? 0 : member.tokens;
  const requests = operator ? signal?.hourly.reduce((n, x) => n + x.request_count, 0) ?? 0 : member.requests;
  return <PageFrame kicker={operator ? "Operations" : "Member workspace"} title={operator ? "Usage & economics" : "My usage"} description={operator ? "Pool-wide usage and economics with evidence state." : "Your measured gateway activity."}>
    <section className="metric-grid"><Metric label="Processed tokens" value={tokens ? compact(tokens) : "—"} note={operator ? "Pool measured" : "Origin measured"} /><Metric label="Requests" value={requests ? requests.toLocaleString() : "—"} note="Available data" /><Metric label="API value" value={operator && latest ? `$${latest.daily_api_value.toFixed(2)}` : "—"} note={operator ? "Latest measured day" : "Not calculated per member"} /><Metric label="Day-over-day" value={operator && change !== null ? `${change >= 0 ? "+" : ""}${change.toFixed(1)}%` : "—"} note="Daily API value" /></section>
    <section className="metric-grid"><Metric label="Input tokens" value={compact(operator ? stats?.aggregate.total_input_tokens ?? 0 : member.input)} note="Measured" /><Metric label="Cached tokens" value={operator ? compact(stats?.aggregate.total_cached_tokens ?? 0) : "—"} note={operator ? `${stats?.aggregate.overall_cache_hit_rate_pct.toFixed(1) ?? "—"}% cache hit rate` : "Pool-only metric"} /><Metric label="Output tokens" value={compact(operator ? stats?.aggregate.total_output_tokens ?? 0 : member.output)} note="Measured" /><Metric label="ROI" value={operator && stats ? `${stats.aggregate.overall_roi.toFixed(1)}x` : "—"} note={operator ? "Measured economics" : "Pool-only metric"} /></section>
    <section className="bento-card chart-card"><CardHeader title={operator ? "Pool activity" : "Your activity"} subtitle={operator ? "Measured request volume" : "Measured origin request volume"} /><SplineChart values={operator ? signal?.hourly.slice(-30).map(x => x.request_count) ?? [] : memberRows.map(x => x.request_count)} /></section>
  </PageFrame>;
}
