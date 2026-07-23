import type { FriendSession, ModelDescriptor, PoolStats, SignalAnalytics } from "../types";
import type { AppRoute } from "../routes";
import { DashboardPage } from "./dashboard";
import { ModelsPage } from "./models";
import { MonitorPage } from "./monitor";
import { OperatorPage } from "./operator";
import { ProfilePage } from "./profile";
import { SetupPage } from "./setup";
import { UsagePage } from "./usage";

export function Page({ route, stats, signal, models, session, onNavigate }: { route: string; stats: PoolStats | null; signal: SignalAnalytics | null; models: ModelDescriptor[]; session: FriendSession; onNavigate: (path: AppRoute) => void }) {
  if (route === "/models" || route === "/operator/routes") return <ModelsPage models={models} operator={route.startsWith("/operator")} />;
  if (route === "/setup") return <SetupPage session={session} />;
  if (route === "/usage" || route === "/operator/usage") return <UsagePage stats={stats} signal={signal} operator={route.startsWith("/operator")} />;
  if (route === "/profile") return <ProfilePage session={session} />;
  if (route === "/operator/connections") return <OperatorPage title="Provider connections" description="Credentialed upstream capacity and lifecycle state." rows={stats?.accounts.map(a => ({ title: a.display_name || a.type, meta: a.type, state: a.status, detail: `${a.secondary_window_available ? "Capacity available" : "Capacity unknown"}` })) ?? []} />;
  if (route === "/operator/members") return <OperatorPage title="Members" description="Gateway users and access state." rows={[]} empty="Member projection is not available yet." />;
  if (route === "/operator/system") return <OperatorPage title="System" description="Runtime, persistence, configuration, and recovery readiness." rows={[]} empty="System health projection is not available yet." />;
  if (route === "/operator/monitor") return <MonitorPage stats={stats} signal={signal} />;
  return <DashboardPage stats={stats} signal={signal} operator={route === "/operator"} onNavigate={onNavigate} />;
}
