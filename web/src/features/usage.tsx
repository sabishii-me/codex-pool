import { useEffect, useMemo, useState } from "react";
import { loadPoolUsers, loadUsageProjection } from "../api";
import type { GatewayMember, PoolUserStats, UsageProjection } from "../types";
import type { ResourceState } from "../resource-state";
import { CardHeader, Metric, MultiSeriesChart, PageFrame, PieChart, compact, type ChartSeries } from "../components/ui";

type UsageScope = "me" | "pool" | "member";
type Range = "24h" | "7d" | "30d";
const ranges: Record<Range, { hours: number; days: number; label: string; bucket: "hour" | "day" }> = { "24h": { hours: 24, days: 1, label: "24 hours", bucket: "hour" }, "7d": { hours: 168, days: 7, label: "7 days", bucket: "day" }, "30d": { hours: 720, days: 30, label: "30 days", bucket: "day" } };

export function UsagePage({ isElevated, isAdmin, members, identities = { status: "idle" } }: { isElevated: boolean; isAdmin: boolean; members: ResourceState<PoolUserStats[]>; identities?: ResourceState<GatewayMember[]> }) {
  // Reading usage never requires MFA: any admin can view pool/member scopes
  // without elevating. isElevated is kept for backward compatibility but no
  // longer gates read-only views.
  const canViewAll = isAdmin || isElevated;
  const initial = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search);
  const requestedScope = initial.get("scope") as UsageScope | null;
  const [scope, setScope] = useState<UsageScope>(canViewAll && requestedScope && ["pool", "member"].includes(requestedScope) ? requestedScope : "me");
  const [memberID, setMemberID] = useState(initial.get("member") ?? "");
  const [range, setRange] = useState<Range>("7d");
  const [state, setState] = useState<ResourceState<UsageProjection>>({ status: "loading" });
  const effectiveScope: UsageScope = canViewAll ? scope : "me";

  // Member pie chart data follows the selected range (24h/7d/30d) so the
  // slices reflect the current window, not the all-time leaderboard totals.
  const [pieUsers, setPieUsers] = useState<ResourceState<PoolUserStats[]>>({ status: "idle" });
  useEffect(() => {
    if (effectiveScope !== "member") return;
    let cancelled = false;
    setPieUsers({ status: "loading" });
    loadPoolUsers(ranges[range].hours).then(({ users }) => {
      if (!cancelled) setPieUsers({ status: "ready", data: users });
    }).catch(error => {
      if (!cancelled) setPieUsers({ status: "error", message: error instanceof Error ? error.message : "Members unavailable" });
    });
    return () => { cancelled = true; };
  }, [effectiveScope, range]);

  useEffect(() => { if (!canViewAll && scope !== "me") setScope("me"); }, [canViewAll, scope]);
  useEffect(() => {
    const query = new URLSearchParams(); if (effectiveScope !== "me") query.set("scope", effectiveScope); if (effectiveScope === "member" && memberID) query.set("member", memberID);
    if (typeof window !== "undefined") window.history.replaceState({}, "", `${window.location.pathname}${query.size ? `?${query}` : ""}`);
    if (effectiveScope === "member" && !memberID) { setState({ status: "idle" }); return; }
    let cancelled = false; setState({ status: "loading" });
    loadUsageProjection(effectiveScope, { memberId: effectiveScope === "member" ? memberID : undefined, ...ranges[range] }).then(data => {
      if (cancelled) return;
      if (data.range_hours !== ranges[range].hours || data.range_days !== ranges[range].days) { setState({ status: "error", message: "The requested usage range was not applied" }); return; }
      setState({ status: "ready", data });
    }).catch(error => { if (!cancelled) setState({ status: "error", message: error instanceof Error ? error.message : "Usage unavailable" }); });
    return () => { cancelled = true; };
  }, [effectiveScope, memberID, range]);

  const projection = state.status === "ready" ? state.data : null;
  const chart = useMemo(() => buildModelChart(projection, range), [projection, range]);
  const chartTitle = chart.granularity === "model" ? "Tokens by model over time" : chart.granularity === "provider" ? "Tokens by provider over time" : "Tokens over time";
  const economics = projection?.economics?.at(-1);
  const maxModelTokens = Math.max(...(projection?.by_model ?? []).map(row => row.billable_tokens), 1);

  const memberLabel = (id: string) => identities.status === "ready" ? identities.data.find(member => member.id === id)?.email ?? id : id;

  return <PageFrame kicker="Activity" title="Usage" description="Tokens, requests, and cost.">
    <div className="usage-controls"><div className="scope-bar" aria-label="Usage scope"><button className={effectiveScope === "me" ? "active" : ""} onClick={() => setScope("me")}>My usage</button>{canViewAll ? <><button className={effectiveScope === "pool" ? "active" : ""} onClick={() => setScope("pool")}>Pool usage</button><button className={effectiveScope === "member" ? "active" : ""} onClick={() => setScope("member")}>Member usage</button></> : null}</div><div className="range-bar">{(Object.keys(ranges) as Range[]).map(value => <button key={value} className={range === value ? "active" : ""} onClick={() => setRange(value)}>{value}</button>)}</div></div>
    {effectiveScope === "member" ? <section className="member-pie-bento bento-card"><CardHeader title="Member usage" subtitle={`Billable tokens by member · ${ranges[range].label} — click a slice to inspect`} /><PieChart slices={(pieUsers.status === "ready" ? pieUsers.data : []).map(member => ({ label: memberLabel(member.user_id), value: member.total_billable_tokens }))} selectedLabel={memberID ? memberLabel(memberID) : undefined} onSelect={label => { const found = pieUsers.status === "ready" ? pieUsers.data.find(member => memberLabel(member.user_id) === label) : undefined; if (found) setMemberID(found.user_id); }} /></section> : null}
    {state.status === "loading" ? <section className="usage-unavailable"><span>{effectiveScope} scope</span><h2>Loading usage</h2></section> : null}
    {state.status === "idle" ? <section className="usage-unavailable"><span>Member usage</span><h2>Select a member</h2></section> : null}
    {state.status === "error" ? <section className="usage-unavailable"><span>Unavailable</span><h2>Usage could not be loaded</h2><p>{state.message}</p></section> : null}
    {projection ? <>
      <div className="evidence-strip usage-updated"><span>Updated</span><b>{new Date(projection.evidence.generated_at).toLocaleString()}</b><small>{ranges[range].label}</small></div>
      <section className="metric-grid usage-metrics"><Metric label="Billable tokens" value={compact(projection.totals.total_billable_tokens)} note={`${projection.scope} measured`} /><Metric label="Requests" value={projection.totals.request_count.toLocaleString()} note={`${projection.scope} measured`} /><Metric label="Input tokens" value={compact(projection.totals.total_input_tokens)} note="Measured" /><Metric label="Output tokens" value={compact(projection.totals.total_output_tokens)} note="Measured" /></section>
      <section className="usage-chart bento-card"><CardHeader title={chartTitle} subtitle={`${ranges[range].label} · measured ${ranges[range].bucket === "hour" ? "hourly" : "daily"} billable tokens`} /><MultiSeriesChart series={chart.series} xLabels={chart.labels} ariaLabel={`${chartTitle}, ${ranges[range].label}`} /></section>
      <section className="usage-breakdown-grid"><div className="bento-card"><CardHeader title="Token composition" subtitle="Measured selected-scope totals" /><div className="breakdown-list"><div><span>Input</span><b>{compact(projection.totals.total_input_tokens)}</b></div><div><span>Cached</span><b>{compact(projection.totals.total_cached_tokens)}</b></div><div><span>Output</span><b>{compact(projection.totals.total_output_tokens)}</b></div><div><span>Reasoning</span><b>{compact(projection.totals.total_reasoning_tokens)}</b></div></div></div>{projection.scope === "pool" && economics ? <div className="bento-card"><CardHeader title="Economics" subtitle="Estimated pool context" /><div className="breakdown-list"><div><span>Latest daily API value</span><b>${economics.daily_api_value.toFixed(2)}</b></div><div><span>Cumulative API value</span><b>${economics.cumulative_api_value.toFixed(2)}</b></div><div><span>Subscription spend</span><b>${economics.cumulative_subscription_spend.toFixed(2)}</b></div></div></div> : null}</section>
      {projection.by_model.length ? <section className="usage-detail-card"><CardHeader title="Usage by model" subtitle={`${projection.by_model.length} models in the selected range`} /><div className="usage-dimension-list">{projection.by_model.map(row => <article key={`${row.provider_id}:${row.id}`}><div><b>{row.id}</b><small>{row.provider_id} · {row.requests.toLocaleString()} requests</small></div><div className="usage-dimension-bar"><i style={{ width: `${row.billable_tokens / maxModelTokens * 100}%` }} /></div><strong>{compact(row.billable_tokens)}</strong><small>{compact(row.input_tokens)} in · {compact(row.output_tokens)} out · {compact(row.cached_tokens)} cached</small></article>)}</div></section> : null}
      <section className="usage-breakdown-grid usage-dimensions">{projection.by_provider.length ? <div className="bento-card"><CardHeader title="Usage by provider" subtitle="Measured provider attribution" /><div className="dimension-table">{projection.by_provider.map(row => <div key={row.id}><span><b>{row.id}</b><small>{row.requests.toLocaleString()} requests</small></span><strong>{compact(row.billable_tokens)}</strong></div>)}</div></div> : null}{projection.scope === "pool" && projection.by_connection?.length ? <div className="bento-card"><CardHeader title="Usage by connection" subtitle="Measured serving connections" /><div className="dimension-table">{projection.by_connection.map(row => <div key={`${row.provider_id}:${row.id}`}><span><b>{row.id.slice(0, 12)}</b><small>{row.provider_id} · {row.requests.toLocaleString()} requests</small></span><strong>{compact(row.billable_tokens)}</strong></div>)}</div></div> : null}</section>
      {projection.partial_failures?.length ? <div className="resource-message error"><b>Some data could not be loaded</b><p>{projection.partial_failures.join(" · ")}</p></div> : null}
    </> : null}
  </PageFrame>;
}

export function buildModelChart(projection: UsageProjection | null, range: Range): { labels: string[]; series: ChartSeries[]; granularity: "model" | "provider" | "none" } {
  if (!projection) return { labels: [], series: [], granularity: "none" };
  const config = ranges[range];
  const end = new Date(projection.evidence.generated_at);
  if (!Number.isFinite(end.getTime())) return { labels: [], series: [], granularity: "none" };
  end.setUTCMinutes(0, 0, 0);
  const start = config.bucket === "hour"
    ? new Date(end.getTime() - (config.hours - 1) * 3_600_000)
    : new Date(Date.UTC(end.getUTCFullYear(), end.getUTCMonth(), end.getUTCDate() - (config.days - 1)));
  const bucketKeys: string[] = [];
  if (config.bucket === "hour") {
    for (let offset = 0; offset < config.hours; offset++) bucketKeys.push(new Date(start.getTime() + offset * 3_600_000).toISOString().slice(0, 13));
  } else {
    for (let offset = 0; offset < config.days; offset++) bucketKeys.push(new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth(), start.getUTCDate() + offset)).toISOString().slice(0, 10));
  }
  const labels = bucketKeys.map(key => config.bucket === "hour"
    ? new Date(`${key}:00:00Z`).toLocaleString([], { month: "short", day: "numeric", hour: "numeric" })
    : new Date(`${key}T00:00:00Z`).toLocaleDateString([], { month: "short", day: "numeric" }));
  const modelRows = projection.model_hourly ?? [];
  const sourceRows = modelRows.length > 0
    ? modelRows.map(row => ({ hour: row.hour, id: `${row.provider_id}:${row.model_id}`, label: row.model_id, billableTokens: row.billable_tokens }))
    : (projection.hourly ?? []).map(row => ({ hour: row.hour, id: `provider:${row.account_type || "unknown"}`, label: row.account_type || "unknown", billableTokens: row.billable_tokens }));
  const granularity = modelRows.length > 0 ? "model" : sourceRows.length > 0 ? "provider" : "none";
  const rowsBySeries = new Map<string, { label: string; values: Map<string, number> }>();
  for (const row of sourceRows) {
    const item = rowsBySeries.get(row.id) ?? { label: row.label, values: new Map<string, number>() };
    const parsed = new Date(row.hour.endsWith("Z") ? row.hour : `${row.hour}:00:00Z`);
    const rowTime = parsed.getTime();
    if (!Number.isFinite(rowTime)) continue;
    const bucket = config.bucket === "hour"
      ? parsed.toISOString().slice(0, 13)
      : parsed.toISOString().slice(0, 10);
    if (!bucketKeys.includes(bucket)) continue;
    item.values.set(bucket, (item.values.get(bucket) ?? 0) + row.billableTokens);
    rowsBySeries.set(row.id, item);
  }
  return { labels, series: [...rowsBySeries.entries()].map(([id, item]) => ({ id, label: item.label, values: bucketKeys.map(key => item.values.get(key) ?? 0) })), granularity };
}
