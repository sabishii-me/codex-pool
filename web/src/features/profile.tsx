import { useState } from "react";
import { confirmMFA, enrollMFA } from "../api";
import type { CapabilityStatus } from "../routes";
import type { GatewaySession } from "../types";
import { PageFrame, StatusBadge } from "../components/ui";

export function ProfilePage({ session, capability, onCapabilityRefresh }: { session: GatewaySession; capability: CapabilityStatus; onCapabilityRefresh: () => Promise<void> }) {
  const [enrollment, setEnrollment] = useState<{ secret: string; otpauth_url: string } | null>(null);
  const [code, setCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const beginEnrollment = async () => { setBusy(true); setError(""); try { setEnrollment(await enrollMFA()); } catch (failure) { setError(failure instanceof Error ? failure.message : "Enrollment failed"); } finally { setBusy(false); } };
  const confirm = async () => { if (code.length !== 6) return; setBusy(true); setError(""); try { const result = await confirmMFA(code); setRecoveryCodes(result.recovery_codes); setEnrollment(null); setCode(""); await onCapabilityRefresh(); } catch (failure) { setError(failure instanceof Error ? failure.message : "Confirmation failed"); } finally { setBusy(false); } };
  return <PageFrame kicker="Workspace" title="Profile" description="Identity, role, and session security.">
    <section className="profile-new"><div><span>Signed in as</span><b>{session.email}</b></div><div><span>Role</span><b>{session.is_admin ? "Admin" : "Member"}</b></div><div><span>Session origin</span><b>{session.origin_id}</b></div></section>
    {session.is_admin ? <section className="security-panel">
      <h2>Admin security</h2>
      {capability.status === "idle" || capability.status === "checking" ? <div className="security-state"><StatusBadge tone="warning">Checking</StatusBadge><p>Verifying MFA enrollment and current elevation.</p></div> : null}
      {capability.status === "error" ? <div className="security-state"><StatusBadge tone="warning">Error</StatusBadge><p>{capability.message}</p></div> : null}
      {capability.status === "resolved" ? <>
        <div className="profile-new security-facts"><div><span>MFA</span><b>{capability.enrolled ? "Enrolled" : "Not enrolled"}</b></div><div><span>Admin session</span><b>{capability.elevated ? "Elevated" : "Standard"}</b></div>{capability.enrolled ? <div><span>Recovery codes</span><b>{capability.recoveryCodesRemaining} remaining</b></div> : null}</div>
        {!capability.enrolled && !enrollment && !recoveryCodes.length ? <button className="primary-button" disabled={busy} onClick={() => void beginEnrollment()}>{busy ? "Starting…" : "Set up authenticator"}</button> : null}
        {enrollment ? <section className="mfa-enrollment"><h3>Add AI Pool to your authenticator</h3><p>Open the setup link in an authenticator app, or enter the secret manually. Then confirm the current code.</p><a href={enrollment.otpauth_url}>Open authenticator setup</a><label><span>Manual secret</span><code>{enrollment.secret}</code></label><label><span>Six-digit code</span><input aria-label="MFA enrollment code" inputMode="numeric" maxLength={6} value={code} onChange={event => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))} /></label><button className="primary-button" disabled={busy || code.length !== 6} onClick={() => void confirm()}>{busy ? "Confirming…" : "Confirm authenticator"}</button></section> : null}
        {recoveryCodes.length ? <section className="recovery-codes"><h3>Save your recovery codes</h3><p>Store these codes securely. Each code can be used once.</p><div>{recoveryCodes.map(value => <code key={value}>{value}</code>)}</div><button className="secondary-button" onClick={() => void navigator.clipboard.writeText(recoveryCodes.join("\n"))}>Copy recovery codes</button></section> : null}
        {capability.enrolled && !capability.elevated ? <p className="security-note">Admin resources request your authenticator code when you open them.</p> : null}
        {error ? <p className="form-error" role="alert">{error}</p> : null}
      </> : null}
    </section> : null}
  </PageFrame>;
}
