import { useEffect, useState } from "react";
import type { PoolStats, SignalAnalytics } from "../types";
import { CardHeader, Metric, PageFrame, SplineChart, compact } from "../components/ui";

type UsageScope = "me" | "pool";

export function UsagePage({ stats, signal, originId, isElevated }: {
  stats: PoolStats | null;
  signal: SignalAnalytics | null;
  originId: string;
  isElevated: boolean;
}) {
  const [scope, setScope] = useState<UsageScope>("me");
  useEffect(() => { if (!isElevated) setScope("me"); }, [isElevated]);
  const effectiveScope: UsageScope = isElevated ? scope : "me";
  const isPool = effectiveScope === "pool";

  const personalRows = signal?.origin_weekly.filter(row => row.origin_id === originId) ?? [];
  const personal = personalRows.reduce((total, row) => ({
    tokens: total.tokens + row.billable_tokens,
    requests: total.requests + row.request_count,
    input: total.input + row.input_tokens,
    output: total.output + row.output_tokens,
  }), { tokens: 0, requests: 0, input: 0, output: 0 });
  const hasPersonal = personalRows.length > 0;
  const latestEconomics = signal?.economics.at(-1);
  const priorEconomics = signal?.economics.at(-2);
  const dayChange = latestEconomics && priorEconomics
    ? ((latestEconomics.daily_api_value - priorEconomics.daily_api_value) / Math.max(priorEconomics.daily_api_value, 0.01)) * 100
    : null;

  return <PageFrame kicker="Activity" title="Usage" description="Measured activity, historical context, and backend-authorized scope.">
    {isElevated && <div className="scope-bar" aria-label="Usage scope">
      <button className={scope === "me" ? "active" : ""} onClick={() => setScope("me")}>My usage</button>
      <button className={scope === "pool" ? "active" : ""} onClick={() => setScope("pool")}>Pool usage</button>
    </div>}

    {!isPool && !hasPersonal ? <section className="usage-unavailable">
      <span>Personal scope</span><h2>Personal usage is unavailable for this session</h2>
      <p>The copied production history has no canonical record for this authenticated session origin. No other origin is substituted.</p>
    </section> : null}

    {!isPool && hasPersonal ? <>
      <section className="metric-grid usage-metrics">
        <Metric label="Billable tokens" value={compact(personal.tokens)} note="Canonical session origin" />
        <Metric label="Requests" value={personal.requests.toLocaleString()} note="Canonical session origin" />
        <Metric label="Input tokens" value={compact(personal.input)} note="Measured aggregate" />
        <Metric label="Output tokens" value={compact(personal.output)} note="Measured aggregate" />
      </section>
      <section className="usage-unavailable compact-state"><span>History</span><h2>Personal time series is unavailable</h2><p>The backend exposes weekly categorical origin totals, not a canonical continuous personal request series.</p></section>
    </> : null}

    {isPool ? <>
      <section className="metric-grid usage-metrics">
        <Metric label="Billable tokens" value={stats ? compact(stats.aggregate.total_billable_tokens) : "—"} note="Pool projection" />
        <Metric label="Input tokens" value={stats ? compact(stats.aggregate.total_input_tokens) : "—"} note="Pool projection" />
        <Metric label="Output tokens" value={stats ? compact(stats.aggregate.total_output_tokens) : "—"} note="Pool projection" />
        <Metric label="Requests in series" value={(signal?.hourly.reduce((sum, row) => sum + row.request_count, 0) ?? 0).toLocaleString()} note="Loaded hourly range" />
      </section>
      <section className="usage-chart bento-card">
        <CardHeader title="Pool request history" subtitle={signal ? `Projection generated ${new Date(signal.generated_at).toLocaleString()}` : "Hourly projection unavailable"} />
        <SplineChart values={signal?.hourly.slice(-48).map(row => row.request_count) ?? []} />
      </section>
      <section className="usage-breakdown-grid">
        <div className="bento-card"><CardHeader title="Token composition" subtitle="Cumulative measured pool totals" /><div className="breakdown-list"><div><span>Input</span><b>{stats ? compact(stats.aggregate.total_input_tokens) : "—"}</b></div><div><span>Cached</span><b>{stats ? compact(stats.aggregate.total_cached_tokens) : "—"}</b></div><div><span>Output</span><b>{stats ? compact(stats.aggregate.total_output_tokens) : "—"}</b></div><div><span>Reasoning</span><b>{stats ? compact(stats.aggregate.total_reasoning_tokens) : "—"}</b></div></div></div>
        <div className="bento-card"><CardHeader title="Economics" subtitle="Pool scope only" /><div className="breakdown-list"><div><span>Latest daily API value</span><b>{latestEconomics ? `$${latestEconomics.daily_api_value.toFixed(2)}` : "—"}</b></div><div><span>Day over day</span><b>{dayChange === null ? "—" : `${dayChange >= 0 ? "+" : ""}${dayChange.toFixed(1)}%`}</b></div><div><span>Cumulative API value</span><b>{latestEconomics ? `$${latestEconomics.cumulative_api_value.toFixed(2)}` : "—"}</b></div><div><span>Subscription spend</span><b>{latestEconomics ? `$${latestEconomics.cumulative_subscription_spend.toFixed(2)}` : "—"}</b></div></div></div>
      </section>
    </> : null}
  </PageFrame>;
}
