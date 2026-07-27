import { useEffect, useMemo, useState } from "react";
import { loadModelRouting } from "../api";
import type { ModelDescriptor, ModelRoutingProjection } from "../types";
import type { ResourceState } from "../resource-state";
import { PageFrame, StatusBadge } from "../components/ui";

export function ModelsPage({ models, isElevated }: { models: ModelDescriptor[]; isElevated: boolean }) {
  const initial = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search).get("model");
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string | null>(initial);
  const [routing, setRouting] = useState<ResourceState<ModelRoutingProjection>>({ status: "idle" });
  const filtered = useMemo(() => models.filter(model => `${model.id} ${model.name ?? ""} ${model.provider}`.toLowerCase().includes(query.toLowerCase())), [models, query]);
  const detail = selected ? models.find(model => model.id === selected) : null;
  useEffect(() => {
    const params = new URLSearchParams(window.location.search); if (selected) params.set("model", selected); else params.delete("model"); window.history.replaceState({}, "", `${window.location.pathname}${params.size ? `?${params}` : ""}`);
    if (!isElevated || !selected) { setRouting({ status: "idle" }); return; }
    let cancelled = false; setRouting({ status: "loading" });
    loadModelRouting(selected).then(data => { if (!cancelled) setRouting({ status: "ready", data }); }).catch(error => { if (!cancelled) setRouting({ status: "error", message: error instanceof Error ? error.message : "Routing detail could not be loaded" }); });
    return () => { cancelled = true; };
  }, [selected, isElevated]);

  return <PageFrame kicker="Workspace" title="Models" description="Available models and capabilities.">
    <div className="toolbar"><input className="search-input" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search models or providers" aria-label="Search models" /><span>{filtered.length} of {models.length} models</span></div>
    <section className="model-list">{filtered.length ? filtered.map(model => <button className={`model-row-new ${selected === model.id ? "selected" : ""}`} key={model.id} onClick={() => setSelected(selected === model.id ? null : model.id)} aria-expanded={selected === model.id}>
      <div className="model-identity"><b>{model.name || model.id}</b><code>{model.id}</code></div><span className="model-provider">{model.provider}</span><span className="model-capabilities">{Object.entries(model.capabilities ?? {}).filter(([, enabled]) => enabled).slice(0, 3).map(([name]) => name).join(" · ") || model.protocol}</span><StatusBadge tone={model.available_now ? "success" : "warning"}>{model.available_now ? "Available" : "Unavailable"}</StatusBadge>
    </button>) : <div className="empty-state"><b>No matching models</b><p>Try a provider name or model ID.</p></div>}</section>
    {detail ? <section className="model-detail-panel"><header><div><span>Selected model</span><h2>{detail.name || detail.id}</h2><code>{detail.id}</code></div><StatusBadge tone={detail.available_now ? "success" : "warning"}>{detail.available_now ? "Available" : "Unavailable"}</StatusBadge></header><div className="model-detail-grid"><div><span>Provider</span><b>{detail.provider}</b></div><div><span>Protocol</span><b>{detail.protocol}</b></div>{detail.contextWindow ? <div><span>Context window</span><b>{detail.contextWindow.toLocaleString()}</b></div> : null}{detail.max_output_tokens ? <div><span>Maximum output</span><b>{detail.max_output_tokens.toLocaleString()}</b></div> : null}</div>{isElevated ? <RoutingDetail state={routing} /> : null}</section> : null}
  </PageFrame>;
}

function RoutingDetail({ state }: { state: ResourceState<ModelRoutingProjection> }) {
  if (state.status === "loading") return <div className="routing-state"><b>Loading routing</b></div>;
  if (state.status === "error") return <div className="routing-state error" role="alert"><b>Routing detail could not be loaded</b><p>{state.message}</p></div>;
  if (state.status !== "ready") return null;
  const route = state.data;
  return <section className="routing-detail"><header><div><span>Runtime routing</span><h3>{route.provider_id}</h3><p>{route.requested_model === route.canonical_model ? route.canonical_model : `${route.requested_model} → ${route.canonical_model}`}</p></div><div><b>{route.eligible_connections.length}</b><small>eligible now</small></div></header><div className="routing-facts"><div><span>Selection</span><b>Per request</b></div><div><span>Evidence</span><b>{route.evidence.source.replaceAll("_", " ")}</b></div><div><span>Observed</span><b>{new Date(route.evidence.generated_at).toLocaleString()}</b></div></div>{route.eligible_connections.length ? <div className="routing-connections"><h4>Eligible connections</h4>{route.eligible_connections.map(connection => <article key={connection.public_id}><div><b>{connection.display_name}</b><small>{connection.public_id}{connection.plan_type ? ` · ${connection.plan_type}` : ""}</small></div>{connection.primary ? <StatusBadge tone="success">Primary</StatusBadge> : null}<span>{connection.inflight} in flight</span></article>)}</div> : <div className="routing-empty">No connection is eligible for this model right now.</div>}{route.excluded_connections.length ? <div className="routing-connections excluded"><h4>Excluded connections</h4>{route.excluded_connections.map(connection => <article key={connection.public_id}><div><b>{connection.display_name}</b><small>{connection.public_id}</small></div><StatusBadge tone="warning">{connection.reason}</StatusBadge></article>)}</div> : null}</section>;
}
