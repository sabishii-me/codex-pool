import type { CapabilityStatus } from "../routes";
import type { FriendSession } from "../types";
import { PageFrame, StatusBadge } from "../components/ui";

export function ProfilePage({ session, capability }: { session: FriendSession; capability: CapabilityStatus }) {
  return <PageFrame kicker="Workspace" title="Profile" description="Identity, role, and session security.">
    <section className="profile-new"><div><span>Signed in as</span><b>{session.email}</b></div><div><span>Role</span><b>{session.is_admin ? "Admin" : "Member"}</b></div><div><span>Session origin</span><b>{session.origin_id}</b></div></section>
    {session.is_admin ? <section className="security-panel">
      <h2>Admin security</h2>
      {capability.status === "idle" || capability.status === "checking" ? <div className="security-state"><StatusBadge tone="warning">Checking</StatusBadge><p>Verifying MFA enrollment and current elevation.</p></div> : null}
      {capability.status === "error" ? <div className="security-state"><StatusBadge tone="warning">Unavailable</StatusBadge><p>{capability.message}</p></div> : null}
      {capability.status === "resolved" ? <>
        <div className="profile-new security-facts"><div><span>MFA enrollment</span><b>{capability.enrolled ? "Enrolled" : "Not enrolled"}</b></div><div><span>Admin capability</span><b>{capability.elevated ? "Elevated" : "Locked"}</b></div><div><span>Recovery codes</span><b>{capability.enrolled ? `${capability.recoveryCodesRemaining} remaining` : "Unavailable"}</b></div></div>
        {capability.enrolled && !capability.elevated ? <div className="resource-message"><b>Admin verification on demand</b><p>Select an Administration resource to open the MFA challenge.</p></div> : null}
        {!capability.enrolled && !capability.elevated ? <div className="resource-message"><b>MFA enrollment required</b><p>The existing backend enrollment workflow is not yet exposed by this redesigned Profile surface.</p></div> : null}
      </> : null}
    </section> : null}
  </PageFrame>;
}
