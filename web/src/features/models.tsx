import { useMemo, useState } from "react";
import type { ModelDescriptor } from "../types";
import { PageFrame, StatusBadge } from "../components/ui";

export function ModelsPage({ models, operator }: { models: ModelDescriptor[]; operator: boolean }) {
  const [query, setQuery] = useState("");
  const filtered = useMemo(() => models.filter(m => `${m.id} ${m.name ?? ""} ${m.provider}`.toLowerCase().includes(query.toLowerCase())), [models, query]);
  return <PageFrame kicker={operator ? "Operations" : "Workspace"} title="Models" description="Browse the live model routes available to your workspace."><div className="toolbar"><input className="search-input" value={query} onChange={e => setQuery(e.target.value)} placeholder="Search models or providers" aria-label="Search models" /><span>{filtered.length} of {models.length} routes</span></div><section className="model-list">{filtered.length ? filtered.map(model => <article className="model-row-new" key={model.id}><div className="model-identity"><b>{model.name || model.id}</b><code>{model.id}</code></div><span className="model-provider">{model.provider}</span><span className="model-accounts">{model.available_accounts ?? 0} / {model.supporting_accounts ?? 0} accounts</span><StatusBadge tone={model.available_now ? "success" : "warning"}>{model.available_now ? "Available" : "Limited"}</StatusBadge></article>) : <div className="empty-state"><b>No matching models</b><p>Try a provider name, model ID, or clear the search.</p></div>}</section></PageFrame>;
}
