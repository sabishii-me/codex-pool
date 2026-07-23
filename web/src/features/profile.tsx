import type { FriendSession } from "../types";
import { PageFrame } from "../components/ui";

export function ProfilePage({ session }: { session: FriendSession }) { return <PageFrame kicker="Workspace" title="Profile" description="Identity and security for this gateway member."><section className="profile-new"><div><span>Signed in as</span><b>{session.email}</b></div><div><span>Workspace</span><b>Member</b></div><div><span>Security</span><b>Managed by gateway authentication</b></div></section></PageFrame>; }
