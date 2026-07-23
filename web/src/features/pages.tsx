import type { FriendSession, ModelDescriptor, OperatorProviderConnectionV2, PoolStats, PoolUserStats, SignalAnalytics } from "../types";
import type { AppRoute } from "../routes";
import { DashboardPage } from "./dashboard";
import { ModelsPage } from "./models";
import { MonitorPage } from "./monitor";
import { OperatorPage } from "./operator";
import { ProfilePage } from "./profile";
import { SetupPage } from "./setup";
import { UsagePage } from "./usage";

export function Page({ route, stats, signal, models, connections, users, session, onNavigate }: { route: string; stats: PoolStats | null; signal: SignalAnalytics | null; models: ModelDescriptor[]; connections: OperatorProviderConnectionV2[]; users: PoolUserStats[]; session: FriendSession; onNavigate: (path: AppRoute) => void }) {
  if (route === "/models" || route === "/operator/routes") return <ModelsPage models={models} operator={route.startsWith("/operator")} />;
  if (route === "/setup") return <SetupPage session={session} />;
  if (route === "/usage" || route === "/operator/usage") return <UsagePage stats={stats} signal={signal} operator={route.startsWith("/operator")} />;
  if (route === "/profile") return <ProfilePage session={session} />;
  if (route === "/operator/connections") return <OperatorPage title="Provider connections" description="Credentialed upstream capacity and lifecycle state." rows={connections.map(a => ({ title: a.identity.display_name || a.provider_id, meta: `${a.provider_id} · ${a.plan_type || "plan unknown"}`, state: a.dead ? "dead" : a.disabled ? "disabled" : a.health_error ? "degraded" : "healthy", detail: a.health_error || `${a.inflight} in flight · score ${a.score.toFixed(1)}` }))} />;
  if (route === "/operator/members") return <OperatorPage title="Members" description="Gateway users and access state." rows={users.map(user => ({ title: user.user_id, meta: user.last_seen ? `Last seen ${new Date(user.last_seen).toLocaleDateString()}` : "No recent activity", state: "healthy", detail: `${user.request_count.toLocaleString()} requests · ${user.total_billable_tokens.toLocaleString()} billable tokens` }))} empty="No gateway member usage recorded yet." />;
  if (route === "/operator/system") return <OperatorPage title="System" description="Runtime, persistence, configuration, and recovery readiness." rows={[]} empty="System health projection is not available yet." />;
  if (route === "/operator/monitor") return <MonitorPage stats={stats} signal={signal} />;
  return <DashboardPage stats={stats} signal={signal} models={models} operator={route === "/operator"} onNavigate={onNavigate} />;
}
