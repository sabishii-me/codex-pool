import type { ResourceState } from "../resource-state";
import type { AppRoute, CapabilityStatus } from "../routes";
import type { GatewaySession, GatewayMember, ModelDescriptor, OperatorProviderConnectionV2, PoolStats, PoolUserStats, SignalAnalytics, SystemProjection } from "../types";
import { ConnectionsPage, MembersPage, SystemPage } from "./admin";
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
  session: GatewaySession;
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
  if (route === "/usage") return <UsagePage isElevated={isElevated} members={users} identities={members} />;
  if (route === "/setup") return <SetupPage session={session} />;
  if (route === "/profile") return <ProfilePage session={session} capability={capability} onCapabilityRefresh={onCapabilityRefresh} />;
  if (route === "/admin/connections") return <ConnectionsPage state={connections} onRefresh={onConnectionsRefresh} onAuthorizationLost={onAuthorizationLost} />;
  if (route === "/admin/members") return <MembersPage state={members} onRefresh={onMembersRefresh} />;
  if (route === "/admin/system") return <SystemPage state={health} />;
  return <div className="not-found-page"><span>404</span><h1>Page not found</h1><button className="primary-button" onClick={() => onNavigate("/")}>Return Home</button></div>;
}
