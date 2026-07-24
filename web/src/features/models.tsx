import { useMemo, useState } from "react";
import type { ModelDescriptor } from "../types";
import { PageFrame, StatusBadge } from "../components/ui";

export function ModelsPage({ models, isElevated }: { models: ModelDescriptor[]; isElevated: boolean }) {
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<string | null>(null);
  const filtered = useMemo(() => models.filter(model => `${model.id} ${model.name ?? ""} ${model.provider}`.toLowerCase().includes(query.toLowerCase())), [models, query]);
  const detail = selected ? models.find(model => model.id === selected) : null;

  return <PageFrame kicker="Workspace" title="Models" description="One model inventory with member-facing details and authorized Admin context.">
    <div className="toolbar"><input className="search-input" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search models or providers" aria-label="Search models" /><span>{filtered.length} of {models.length} models</span></div>
    <section className="model-list">{filtered.length ? filtered.map(model => <button className={`model-row-new ${selected === model.id ? "selected" : ""}`} key={model.id} onClick={() => setSelected(selected === model.id ? null : model.id)} aria-expanded={selected === model.id}>
      <div className="model-identity"><b>{model.name || model.id}</b><code>{model.id}</code></div>
      <span className="model-provider">{model.provider}</span>
      <span className="model-capabilities">{Object.entries(model.capabilities ?? {}).filter(([, enabled]) => enabled).slice(0, 3).map(([name]) => name).join(" · ") || model.protocol}</span>
      <StatusBadge tone={model.available_now ? "success" : "warning"}>{model.available_now ? "Available" : "Limited"}</StatusBadge>
    </button>) : <div className="empty-state"><b>No matching models</b><p>Try a provider name or model ID.</p></div>}</section>
    {detail ? <section className="model-detail-panel">
      <header><div><span>Selected model</span><h2>{detail.name || detail.id}</h2><code>{detail.id}</code></div><StatusBadge tone={detail.available_now ? "success" : "warning"}>{detail.available_now ? "Available" : "Limited"}</StatusBadge></header>
      <div className="model-detail-grid"><div><span>Provider</span><b>{detail.provider}</b></div><div><span>Protocol</span><b>{detail.protocol}</b></div><div><span>Context window</span><b>{detail.contextWindow ? detail.contextWindow.toLocaleString() : "Unavailable"}</b></div><div><span>Maximum output</span><b>{detail.max_output_tokens ? detail.max_output_tokens.toLocaleString() : "Unavailable"}</b></div></div>
      {isElevated ? <div className="routing-unavailable"><b>Routing projection unavailable</b><p>Upstream mapping: {detail.upstream_id || "unavailable"}. Eligible connections, selected connection, fallback order, exclusions, evidence, and freshness are not exposed by a dedicated backend projection.</p></div> : null}
    </section> : null}
  </PageFrame>;
}
