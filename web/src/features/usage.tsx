import { useEffect, useMemo, useState } from "react";
import { loadUsageProjection } from "../api";
import type { GatewayMember, PoolUserStats, UsageProjection } from "../types";
import type { ResourceState } from "../resource-state";
import { CardHeader, Metric, PageFrame, SplineChart, compact } from "../components/ui";

type UsageScope = "me" | "pool" | "member";
type Range = "24h" | "7d" | "30d";
const ranges: Record<Range, { hours: number; days: number; label: string }> = { "24h": { hours: 24, days: 1, label: "24 hours" }, "7d": { hours: 168, days: 7, label: "7 days" }, "30d": { hours: 720, days: 30, label: "30 days" } };

export function UsagePage({ isElevated, members, identities = { status: "idle" } }: { isElevated: boolean; members: ResourceState<PoolUserStats[]>; identities?: ResourceState<GatewayMember[]> }) {
  const initial = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search);
  const requestedScope = initial.get("scope") as UsageScope | null;
  const [scope, setScope] = useState<UsageScope>(isElevated && requestedScope && ["pool", "member"].includes(requestedScope) ? requestedScope : "me");
  const [memberID, setMemberID] = useState(initial.get("member") ?? "");
  const [range, setRange] = useState<Range>("7d");
  const [state, setState] = useState<ResourceState<UsageProjection>>({ status: "loading" });
  const effectiveScope: UsageScope = isElevated ? scope : "me";

  useEffect(() => { if (!isElevated && scope !== "me") setScope("me"); }, [isElevated, scope]);
  useEffect(() => {
    const query = new URLSearchParams(); if (effectiveScope !== "me") query.set("scope", effectiveScope); if (effectiveScope === "member" && memberID) query.set("member", memberID);
    if (typeof window !== "undefined") window.history.replaceState({}, "", `${window.location.pathname}${query.size ? `?${query}` : ""}`);
    if (effectiveScope === "member" && !memberID) { setState({ status: "idle" }); return; }
    let cancelled = false; setState({ status: "loading" });
    loadUsageProjection(effectiveScope, { memberId: effectiveScope === "member" ? memberID : undefined, ...ranges[range] }).then(data => { if (!cancelled) setState({ status: "ready", data }); }).catch(error => { if (!cancelled) setState({ status: "error", message: error instanceof Error ? error.message : "Usage unavailable" }); });
    return () => { cancelled = true; };
  }, [effectiveScope, memberID, range]);

  const projection = state.status === "ready" ? state.data : null;
  const history = useMemo(() => projection?.hourly?.map(row => row.request_count) ?? [], [projection]);
  const economics = projection?.economics?.at(-1);
  const maxModelTokens = Math.max(...(projection?.by_model ?? []).map(row => row.billable_tokens), 1);

  const memberLabel = (id: string) => identities.status === "ready" ? identities.data.find(member => member.id === id)?.email ?? id : id;

  return <PageFrame kicker="Activity" title="Usage" description="Measured usage with backend-authorized subject, range, evidence, and economics scope.">
    <div className="usage-controls"><div className="scope-bar" aria-label="Usage scope"><button className={effectiveScope === "me" ? "active" : ""} onClick={() => setScope("me")}>My usage</button>{isElevated ? <><button className={effectiveScope === "pool" ? "active" : ""} onClick={() => setScope("pool")}>Pool usage</button><button className={effectiveScope === "member" ? "active" : ""} onClick={() => setScope("member")}>Member usage</button></> : null}</div><div className="range-bar">{(Object.keys(ranges) as Range[]).map(value => <button key={value} className={range === value ? "active" : ""} onClick={() => setRange(value)}>{value}</button>)}</div></div>
    {effectiveScope === "member" ? <label className="member-scope-select"><span>Member</span><select value={memberID} onChange={event => setMemberID(event.target.value)}><option value="">Select a member</option>{members.status === "ready" ? members.data.map(member => <option key={member.user_id} value={member.user_id}>{memberLabel(member.user_id)}</option>) : null}</select></label> : null}
    {state.status === "loading" ? <section className="usage-unavailable"><span>{effectiveScope} scope</span><h2>Loading measured usage</h2><p>Querying the canonical usage projection.</p></section> : null}
    {state.status === "idle" ? <section className="usage-unavailable"><span>Member scope</span><h2>Select a member</h2><p>Choose an authorized member identity to load its measured history.</p></section> : null}
    {state.status === "error" ? <section className="usage-unavailable"><span>Projection unavailable</span><h2>Usage could not be loaded</h2><p>{state.message}</p></section> : null}
    {projection ? <>
      <div className="evidence-strip"><span>{projection.evidence.kind}</span><b>{projection.evidence.source.replaceAll("_", " ")}</b><small>Generated {new Date(projection.evidence.generated_at).toLocaleString()} · Range {ranges[range].label}</small></div>
      <section className="metric-grid usage-metrics"><Metric label="Billable tokens" value={compact(projection.totals.total_billable_tokens)} note={`${projection.scope} measured`} /><Metric label="Requests" value={projection.totals.request_count.toLocaleString()} note={`${projection.scope} measured`} /><Metric label="Input tokens" value={compact(projection.totals.total_input_tokens)} note="Measured" /><Metric label="Output tokens" value={compact(projection.totals.total_output_tokens)} note="Measured" /></section>
      <section className="usage-chart bento-card"><CardHeader title={`${projection.scope === "pool" ? "Pool" : projection.scope === "member" ? "Member" : "Personal"} request history`} subtitle={`${ranges[range].label} · hourly requests`} /><SplineChart values={history} /></section>
      <section className="usage-breakdown-grid"><div className="bento-card"><CardHeader title="Token composition" subtitle="Measured selected-scope totals" /><div className="breakdown-list"><div><span>Input</span><b>{compact(projection.totals.total_input_tokens)}</b></div><div><span>Cached</span><b>{compact(projection.totals.total_cached_tokens)}</b></div><div><span>Output</span><b>{compact(projection.totals.total_output_tokens)}</b></div><div><span>Reasoning</span><b>{compact(projection.totals.total_reasoning_tokens)}</b></div></div></div>{projection.scope === "pool" && economics ? <div className="bento-card"><CardHeader title="Economics" subtitle="Estimated pool context" /><div className="breakdown-list"><div><span>Latest daily API value</span><b>${economics.daily_api_value.toFixed(2)}</b></div><div><span>Cumulative API value</span><b>${economics.cumulative_api_value.toFixed(2)}</b></div><div><span>Subscription spend</span><b>${economics.cumulative_subscription_spend.toFixed(2)}</b></div></div></div> : null}</section>
      {projection.by_model.length ? <section className="usage-detail-card"><CardHeader title="Usage by model" subtitle={`${projection.by_model.length} models in the selected range`} /><div className="usage-dimension-list">{projection.by_model.map(row => <article key={`${row.provider_id}:${row.id}`}><div><b>{row.id}</b><small>{row.provider_id} · {row.requests.toLocaleString()} requests</small></div><div className="usage-dimension-bar"><i style={{ width: `${row.billable_tokens / maxModelTokens * 100}%` }} /></div><strong>{compact(row.billable_tokens)}</strong><small>{compact(row.input_tokens)} in · {compact(row.output_tokens)} out · {compact(row.cached_tokens)} cached</small></article>)}</div></section> : null}
      <section className="usage-breakdown-grid usage-dimensions">{projection.by_provider.length ? <div className="bento-card"><CardHeader title="Usage by provider" subtitle="Measured provider attribution" /><div className="dimension-table">{projection.by_provider.map(row => <div key={row.id}><span><b>{row.id}</b><small>{row.requests.toLocaleString()} requests</small></span><strong>{compact(row.billable_tokens)}</strong></div>)}</div></div> : null}{projection.scope === "pool" && projection.by_connection?.length ? <div className="bento-card"><CardHeader title="Usage by connection" subtitle="Measured serving connections" /><div className="dimension-table">{projection.by_connection.map(row => <div key={`${row.provider_id}:${row.id}`}><span><b>{row.id.slice(0, 12)}</b><small>{row.provider_id} · {row.requests.toLocaleString()} requests</small></span><strong>{compact(row.billable_tokens)}</strong></div>)}</div></div> : null}</section>
      {projection.partial_failures?.length ? <div className="resource-message error"><b>Partial projection</b><p>{projection.partial_failures.join(" · ")}</p></div> : null}
    </> : null}
  </PageFrame>;
}
