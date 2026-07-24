import type { ResourceState } from "../resource-state";
import type { AppRoute, CapabilityStatus } from "../routes";
import type { FriendSession, GatewayMember, ModelDescriptor, OperatorProviderConnectionV2, PoolStats, PoolUserStats, SignalAnalytics, SystemProjection } from "../types";
import { AdminCapabilityCheckingPage, AdminLockedPage, ConnectionsPage, MembersPage, SystemPage } from "./admin";
import { DashboardPage } from "./dashboard";
import { ModelsPage } from "./models";
import { ProfilePage } from "./profile";
import { SetupPage } from "./setup";
import { UsagePage } from "./usage";

export function Page({ route, stats, signal, models, connections, users, members, health, session, capability, isElevated, onConnectionsRefresh, onMembersRefresh, onCapabilityRefresh, onAuthorizationLost, onNavigate }: {
  route: string;
  stats: PoolStats | null;
  signal: SignalAnalytics | null;
  models: ModelDescriptor[];
  connections: ResourceState<OperatorProviderConnectionV2[]>;
  users: ResourceState<PoolUserStats[]>;
  members: ResourceState<GatewayMember[]>;
  health: ResourceState<SystemProjection>;
  session: FriendSession;
  capability: CapabilityStatus;
  isElevated: boolean;
  onConnectionsRefresh: () => Promise<void>;
  onMembersRefresh: () => Promise<void>;
  onCapabilityRefresh: () => Promise<void>;
  onAuthorizationLost: () => void;
  onNavigate: (path: AppRoute) => void;
}) {
  if (route === "/") return <DashboardPage stats={stats} models={models} connections={connections} isElevated={isElevated} onNavigate={onNavigate} />;
  if (route === "/models") return <ModelsPage models={models} isElevated={isElevated} />;
  if (route === "/usage") return <UsagePage isElevated={isElevated} members={users} />;
  if (route === "/setup") return <SetupPage session={session} />;
  if (route === "/profile") return <ProfilePage session={session} capability={capability} onCapabilityRefresh={onCapabilityRefresh} />;
  if (route === "/admin/connections") return capability.status === "idle" || capability.status === "checking" ? <AdminCapabilityCheckingPage resource="Connections" /> : isElevated ? <ConnectionsPage state={connections} onRefresh={onConnectionsRefresh} onAuthorizationLost={onAuthorizationLost} /> : <AdminLockedPage resource="Connections" onUnlock={() => onNavigate("/profile")} />;
  if (route === "/admin/members") return capability.status === "idle" || capability.status === "checking" ? <AdminCapabilityCheckingPage resource="Members" /> : isElevated ? <MembersPage state={members} onRefresh={onMembersRefresh} /> : <AdminLockedPage resource="Members" onUnlock={() => onNavigate("/profile")} />;
  if (route === "/admin/system") return capability.status === "idle" || capability.status === "checking" ? <AdminCapabilityCheckingPage resource="System" /> : isElevated ? <SystemPage state={health} /> : <AdminLockedPage resource="System" onUnlock={() => onNavigate("/profile")} />;
  return <div className="not-found-page"><span>404</span><h1>Page not found</h1><p>This route is not part of the current product architecture.</p><button className="primary-button" onClick={() => onNavigate("/")}>Return Home</button></div>;
}
