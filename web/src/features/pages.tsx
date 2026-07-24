import type { ResourceState } from "../resource-state";
import type { AppRoute, CapabilityStatus } from "../routes";
import type { FriendSession, GatewayHealth, ModelDescriptor, OperatorProviderConnectionV2, PoolStats, PoolUserStats, SignalAnalytics } from "../types";
import { AdminCapabilityCheckingPage, AdminLockedPage, ConnectionsPage, MembersPage, SystemPage } from "./admin";
import { DashboardPage } from "./dashboard";
import { ModelsPage } from "./models";
import { ProfilePage } from "./profile";
import { SetupPage } from "./setup";
import { UsagePage } from "./usage";

export function Page({ route, stats, signal, models, connections, users, health, session, capability, isElevated, onCapabilityRefresh, onNavigate }: {
  route: string;
  stats: PoolStats | null;
  signal: SignalAnalytics | null;
  models: ModelDescriptor[];
  connections: ResourceState<OperatorProviderConnectionV2[]>;
  users: ResourceState<PoolUserStats[]>;
  health: ResourceState<GatewayHealth>;
  session: FriendSession;
  capability: CapabilityStatus;
  isElevated: boolean;
  onCapabilityRefresh: () => Promise<void>;
  onNavigate: (path: AppRoute) => void;
}) {
  if (route === "/") return <DashboardPage stats={stats} models={models} connections={connections} isElevated={isElevated} onNavigate={onNavigate} />;
  if (route === "/models") return <ModelsPage models={models} isElevated={isElevated} />;
  if (route === "/usage") return <UsagePage stats={stats} signal={signal} originId={session.origin_id} isElevated={isElevated} />;
  if (route === "/setup") return <SetupPage session={session} />;
  if (route === "/profile") return <ProfilePage session={session} capability={capability} onCapabilityRefresh={onCapabilityRefresh} />;
  if (route === "/admin/connections") return capability.status === "idle" || capability.status === "checking" ? <AdminCapabilityCheckingPage resource="Connections" /> : isElevated ? <ConnectionsPage state={connections} /> : <AdminLockedPage resource="Connections" onUnlock={() => onNavigate("/profile")} />;
  if (route === "/admin/members") return capability.status === "idle" || capability.status === "checking" ? <AdminCapabilityCheckingPage resource="Members" /> : isElevated ? <MembersPage state={users} /> : <AdminLockedPage resource="Members" onUnlock={() => onNavigate("/profile")} />;
  if (route === "/admin/system") return capability.status === "idle" || capability.status === "checking" ? <AdminCapabilityCheckingPage resource="System" /> : isElevated ? <SystemPage state={health} /> : <AdminLockedPage resource="System" onUnlock={() => onNavigate("/profile")} />;
  return <div className="not-found-page"><span>404</span><h1>Page not found</h1><p>This route is not part of the current product architecture.</p><button className="primary-button" onClick={() => onNavigate("/")}>Return Home</button></div>;
}
