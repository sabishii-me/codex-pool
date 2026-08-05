import { useEffect, useMemo, useState } from "react";
import { loadModelRouting } from "../api";
import type { ModelDescriptor, ModelRoutingProjection } from "../types";
import type { ResourceState } from "../resource-state";
import { PageFrame, StatusBadge } from "../components/ui";

export function ModelsPage({ models, isElevated }: { models: ModelDescriptor[]; isElevated: boolean }) {
  const initial = new URLSearchParams(typeof window === "undefined" ? "" : window.location.search).get("model");
  const [query, setQuery] = useState("");
  const [workload, setWorkload] = useState<"all" | "text_generation" | "image_generation">("all");
  const [selected, setSelected] = useState<string | null>(initial);
  const [routing, setRouting] = useState<ResourceState<ModelRoutingProjection>>({ status: "idle" });
  const filtered = useMemo(() => models.filter(model => (workload === "all" || modelKind(model) === workload) && `${model.id} ${model.name ?? ""} ${model.provider}`.toLowerCase().includes(query.toLowerCase())), [models, query, workload]);
  const detail = selected ? models.find(model => model.id === selected) : null;
  useEffect(() => {
    const params = new URLSearchParams(window.location.search); if (selected) params.set("model", selected); else params.delete("model"); window.history.replaceState({}, "", `${window.location.pathname}${params.size ? `?${params}` : ""}`);
    if (!isElevated || !selected) { setRouting({ status: "idle" }); return; }
    let cancelled = false; setRouting({ status: "loading" });
    loadModelRouting(selected).then(data => { if (!cancelled) setRouting({ status: "ready", data }); }).catch(error => { if (!cancelled) setRouting({ status: "error", message: error instanceof Error ? error.message : "Routing detail could not be loaded" }); });
    return () => { cancelled = true; };
  }, [selected, isElevated]);

  return <PageFrame kicker="Workspace" title="Models" description="Available models and capabilities.">
    <div className="toolbar"><input className="search-input" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search models or providers" aria-label="Search models" /><label><span className="sr-only">Workload</span><select aria-label="Filter by workload" value={workload} onChange={event => setWorkload(event.target.value as typeof workload)}><option value="all">All workloads</option><option value="text_generation">Text generation</option><option value="image_generation">Image generation</option></select></label><span>{filtered.length} of {models.length} models</span></div>
    <section className="model-list">{filtered.length ? filtered.map(model => <button className={`model-row-new ${selected === model.id ? "selected" : ""}`} key={model.id} onClick={() => setSelected(selected === model.id ? null : model.id)} aria-expanded={selected === model.id}>
      <div className="model-identity"><b>{model.name || model.id}</b><code>{model.id}</code></div><span className="model-provider">{model.provider}</span><span className="model-capabilities">{modelKindLabel(model)} · {Object.entries(model.capabilities ?? {}).filter(([, enabled]) => enabled).slice(0, 2).map(([name]) => name.replaceAll("_", " ")).join(" · ") || model.protocol}</span><StatusBadge tone={model.available_now ? "success" : "warning"}>{model.available_now ? "Available" : "Unavailable"}</StatusBadge>
    </button>) : <div className="empty-state"><b>No matching models</b><p>Try a provider name or model ID.</p></div>}</section>
    {detail ? <section className="model-detail-panel"><header><div><span>Selected model</span><h2>{detail.name || detail.id}</h2><code>{detail.id}</code></div><StatusBadge tone={detail.available_now ? "success" : "warning"}>{detail.available_now ? "Available" : "Unavailable"}</StatusBadge></header><div className="model-detail-grid"><div><span>Provider</span><b>{detail.provider}</b></div><div><span>Workload</span><b>{modelKindLabel(detail)}</b></div><div><span>Protocol</span><b>{detail.protocol}</b></div>{detail.input_modalities?.length ? <div><span>Inputs</span><b>{detail.input_modalities.join(", ")}</b></div> : null}{detail.output_modalities?.length ? <div><span>Outputs</span><b>{detail.output_modalities.join(", ")}</b></div> : null}{detail.supported_mime_types?.length ? <div><span>Output formats</span><b>{detail.supported_mime_types.join(", ")}</b></div> : null}{detail.contextWindow ? <div><span>Context window</span><b>{detail.contextWindow.toLocaleString()}</b></div> : null}{detail.max_output_tokens ? <div><span>Maximum output</span><b>{detail.max_output_tokens.toLocaleString()}</b></div> : null}</div>{detail.capability_provenance ? <p className="routing-state">Capability: {detail.capability_provenance.replaceAll("_", " ")}{detail.capability_verified_at ? ` · verified ${detail.capability_verified_at}` : ""}</p> : null}{isElevated ? <RoutingDetail state={routing} /> : null}</section> : null}
  </PageFrame>;
}

function modelKind(model: ModelDescriptor) { return model.model_kind ?? "text_generation"; }
function modelKindLabel(model: ModelDescriptor) { return modelKind(model) === "image_generation" ? "Image generation" : "Text generation"; }

function RoutingDetail({ state }: { state: ResourceState<ModelRoutingProjection> }) {
  if (state.status === "loading") return <div className="routing-state"><b>Loading routing</b></div>;
  if (state.status === "error") return <div className="routing-state error" role="alert"><b>Routing detail could not be loaded</b><p>{state.message}</p></div>;
  if (state.status !== "ready") return null;
  const route = state.data;
  return <section className="routing-detail"><header><div><span>Runtime routing</span><h3>{route.provider_id}</h3><p>{route.requested_model === route.canonical_model ? route.canonical_model : `${route.requested_model} → ${route.canonical_model}`}</p></div><div><b>{route.eligible_connections.length}</b><small>eligible now</small></div></header><div className="routing-facts"><div><span>Selection</span><b>Per request</b></div><div><span>Evidence</span><b>{route.evidence.source.replaceAll("_", " ")}</b></div><div><span>Observed</span><b>{new Date(route.evidence.generated_at).toLocaleString()}</b></div></div>{route.eligible_connections.length ? <div className="routing-connections"><h4>Eligible connections</h4>{route.eligible_connections.map(connection => <article key={connection.public_id}><div><b>{connection.display_name}</b><small>{connection.public_id}{connection.plan_type ? ` · ${connection.plan_type}` : ""}</small></div>{connection.primary ? <StatusBadge tone="success">Primary</StatusBadge> : null}<span>{connection.inflight} in flight</span></article>)}</div> : <div className="routing-empty">No connection is eligible for this model right now.</div>}{route.excluded_connections.length ? <div className="routing-connections excluded"><h4>Excluded connections</h4>{route.excluded_connections.map(connection => <article key={connection.public_id}><div><b>{connection.display_name}</b><small>{connection.public_id}</small></div><StatusBadge tone="warning">{connection.reason}</StatusBadge></article>)}</div> : null}</section>;
}
