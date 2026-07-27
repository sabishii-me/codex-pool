import type { AppRoute } from "../routes";
import type { ModelDescriptor, OperatorProviderConnectionV2, PoolStats } from "../types";
import type { ResourceState } from "../resource-state";
import { CardHeader, PageFrame, StatusBadge } from "../components/ui";

export function DashboardPage({ stats, models, connections, isElevated, onNavigate }: {
  stats: PoolStats | null;
  models: ModelDescriptor[];
  connections: ResourceState<OperatorProviderConnectionV2[]>;
  isElevated: boolean;
  onNavigate: (path: AppRoute) => void;
}) {
  const availableModels = models.filter(model => model.available_now).length;
  const constrainedModels = models.filter(model => !model.available_now);

  return <PageFrame
    kicker="Workspace"
    title="Home"
    description="Model availability and recent activity."
    action={<button className="primary-button" onClick={() => onNavigate("/setup")}>Configure a client</button>}
  >
    <section className="home-orientation">
      <div className="home-welcome">
        <span className="home-eyebrow">{models.length && availableModels > 0 ? "Ready to use" : "Gateway catalog"}</span>
        <h2>{models.length ? availableModels > 0 ? `${availableModels} models are currently available` : "No models are currently available" : "Loading model availability"}</h2>
        <p>Choose a model or configure a client to get started.</p>
        <div className="home-actions">
          <button className="primary-button" onClick={() => onNavigate("/models")}>Browse models</button>
          <button className="secondary-button" onClick={() => onNavigate("/usage")}>Open usage</button>
        </div>
      </div>
      <div className="home-facts" aria-label="Workspace summary">
        <div><span>Models online</span><b>{models.length ? `${availableModels} / ${models.length}` : "Loading"}</b><small>Currently available</small></div>
        {stats ? <><div><span>Active connections</span><b>{stats.active_accounts} / {stats.total_accounts}</b><small>Serving gateway traffic</small></div><div><span>Last 24 hours</span><b>{stats.last_24h_tokens.toLocaleString()}</b><small>Billable tokens</small></div></> : null}
      </div>
    </section>

    {stats && stats.accounts.some(account => account.type === "codex" && account.reset_credits_known && Number(account.reset_credits_available ?? 0) > 0) ? <section className="bento-card admin-attention" role="status">
      <CardHeader title="Codex reset credit available" />
      <div className="attention-list"><div><StatusBadge tone="warning">Available</StatusBadge><span>{stats.accounts.filter(account => account.type === "codex").reduce((sum, account) => sum + Number(account.reset_credits_available ?? 0), 0)} reset credit(s) across Codex connections.</span>{isElevated ? <button onClick={() => onNavigate("/admin/connections")}>Review connections →</button> : <a href="https://chatgpt.com/" target="_blank" rel="noreferrer">Official Codex dashboard ↗</a>}</div></div>
    </section> : null}

    <section className="home-resource-grid">
      <button className="home-resource-card" onClick={() => onNavigate("/models")}>
        <span>Models</span><b>Choose a model</b><p>Browse available models and capabilities.</p><em>Open Models →</em>
      </button>
      <button className="home-resource-card" onClick={() => onNavigate("/setup")}>
        <span>Setup</span><b>Connect a client</b><p>Install and configure a supported client.</p><em>Open Setup →</em>
      </button>
      <button className="home-resource-card" onClick={() => onNavigate("/usage")}>
        <span>Usage</span><b>Inspect activity</b><p>Review tokens, requests, and cost.</p><em>Open Usage →</em>
      </button>
    </section>

    {isElevated && <section className="bento-card admin-attention">
      <CardHeader title="Admin attention" subtitle="Items that may need action" />
      <div className="attention-list">
        {connections.status === "error" ? <div><StatusBadge tone="warning">Unavailable</StatusBadge><span>{connections.message}</span><button onClick={() => onNavigate("/admin/connections")}>Connections →</button></div> : null}
        {connections.status === "empty" ? <div><StatusBadge tone="warning">Action</StatusBadge><span>No provider connections are configured.</span><button onClick={() => onNavigate("/admin/connections")}>Connections →</button></div> : null}
        {connections.status === "ready" && connections.data.filter(c => c.reset_credits.available_count > 0).map(connection => <div key={`reset-${connection.id}`}><StatusBadge tone="warning">Reset credit</StatusBadge><span>{connection.identity.display_name || connection.provider_id}: {connection.reset_credits.available_count} available</span><button onClick={() => onNavigate("/admin/connections")}>Connections →</button></div>)}
        {connections.status === "ready" && connections.data.filter(c => c.dead || c.disabled || c.health_error).map(connection => <div key={connection.id}><StatusBadge tone="warning">Connection</StatusBadge><span>{connection.identity.display_name || connection.provider_id}: {connection.dead ? "dead" : connection.disabled ? "disabled" : connection.health_error}</span><button onClick={() => onNavigate("/admin/connections")}>Connections →</button></div>)}
        {connections.status === "ready" && !connections.data.some(c => c.dead || c.disabled || c.health_error) && constrainedModels.length === 0 ? <div><StatusBadge tone="success">Clear</StatusBadge><span>No connection or model issues require attention.</span></div> : null}
        {constrainedModels.slice(0, 3).map(model => <div key={model.id}><StatusBadge tone="warning">Model</StatusBadge><span>{model.name || model.id} is unavailable now.</span><button onClick={() => onNavigate("/models")}>Models →</button></div>)}
      </div>
    </section>}
  </PageFrame>;
}
