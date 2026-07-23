import { useState } from "react";
import { loadLiveCuteCodeSettings, loadLivePiModels } from "../api";
import type { FriendSession } from "../types";
import { PageFrame } from "../components/ui";

export function SetupPage({ session }: { session: FriendSession }) {
  const [client, setClient] = useState<"pi" | "cute">("pi");
  const [config, setConfig] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const loadConfig = async (next: "pi" | "cute" = client) => {
    setClient(next); setLoading(true); setError("");
    try { setConfig(next === "pi" ? await loadLivePiModels(session.download_token) : await loadLiveCuteCodeSettings(session.download_token)); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Configuration unavailable"); setConfig(""); }
    finally { setLoading(false); }
  };
  const copy = () => { if (config) void navigator.clipboard?.writeText(config); };
  return <PageFrame kicker="Workspace" title="Setup" description="Configure a supported client and verify the gateway connection."><section className="setup-new"><div className="setup-intro"><span className="step-number">1</span><div><h2>Choose your client</h2><p>Configuration is generated from your authenticated session and current model catalog.</p></div></div><div className="setup-tools-new"><button className={client === "pi" ? "active" : ""} onClick={() => void loadConfig("pi")}>Pi</button><button className={client === "cute" ? "active" : ""} onClick={() => void loadConfig("cute")}>Cute Code</button></div><div className="code-panel"><header><span>{loading ? "Loading configuration…" : config ? "Live configuration" : "Generate configuration"}</span><button onClick={copy} disabled={!config}>Copy</button></header>{error ? <div className="empty-state"><b>Configuration unavailable</b><p>{error}</p></div> : <code>{config || `Gateway: ${session.public_url || "authenticated gateway"}\nSession: authenticated workspace\nChoose a client above to download its live configuration.`}</code>}</div></section></PageFrame>;
}
