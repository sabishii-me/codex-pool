import { useEffect, useMemo, useState } from "react";
import { loadSetupClients, loadSetupConfig } from "../api";
import type { FriendSession, SetupClient, SetupEnvironment } from "../types";
import { PageFrame } from "../components/ui";

function Step({ number, title, summary, active, complete, children }: { number: number; title: string; summary?: string; active: boolean; complete: boolean; children?: React.ReactNode }) {
  return <section className={`setup-step ${active ? "active" : ""} ${complete ? "complete" : ""}`}>
    <div className="setup-step-marker" aria-hidden="true">{complete ? "✓" : number}</div>
    <div className="setup-step-content">
      <header><div><span>Step {number}</span><h2>{title}</h2>{summary && !active ? <p>{summary}</p> : null}</div></header>
      {active ? <div className="setup-step-body">{children}</div> : null}
    </div>
  </section>;
}

function Choice({ selected, title, detail, onClick }: { selected: boolean; title: string; detail: string; onClick: () => void }) {
  return <button type="button" className={`setup-choice ${selected ? "selected" : ""}`} aria-pressed={selected} onClick={onClick}><b>{title}</b><span>{detail}</span></button>;
}

function CodeBlock({ label, value, onCopy }: { label: string; value: string; onCopy: () => void }) {
  return <div className="code-panel setup-code"><header><span>{label}</span><button type="button" onClick={onCopy}>Copy</button></header><code>{value}</code></div>;
}

export function SetupPage({ session: _session }: { session: FriendSession }) {
  const [clients, setClients] = useState<SetupClient[]>([]);
  const [clientID, setClientID] = useState("");
  const [environmentID, setEnvironmentID] = useState("");
  const [step, setStep] = useState<1 | 2 | 3 | 4>(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [config, setConfig] = useState("");
  const [configError, setConfigError] = useState("");

  const refresh = () => {
    setLoading(true); setError("");
    void loadSetupClients().then(result => setClients(result.clients)).catch(cause => setError(cause instanceof Error ? cause.message : "Setup clients unavailable")).finally(() => setLoading(false));
  };
  useEffect(refresh, []);

  const client = useMemo(() => clients.find(item => item.id === clientID), [clients, clientID]);
  const environment = useMemo(() => client?.environments.find(item => item.id === environmentID), [client, environmentID]);
  const chooseClient = (next: SetupClient) => { setClientID(next.id); setEnvironmentID(""); setConfig(""); setConfigError(""); setStep(2); };
  const chooseEnvironment = (next: SetupEnvironment) => { setEnvironmentID(next.id); setConfig(""); setConfigError(""); setStep(3); };
  const copy = (value: string) => void navigator.clipboard?.writeText(value);
  const preview = async () => {
    if (!environment?.config_url) return;
    setConfigError("");
    try { setConfig(await loadSetupConfig(environment.config_url)); }
    catch (cause) { setConfigError(cause instanceof Error ? cause.message : "Configuration unavailable"); }
  };

  return <PageFrame kicker="Workspace" title="Setup" description="Install and configure a supported coding client with your authenticated gateway access.">
    <div className="setup-stepper">
      <Step number={1} title="Choose your client" active={step === 1} complete={step > 1} summary={client?.display_name}>
        {loading ? <div className="empty-state"><b>Loading supported clients…</b></div> : error ? <div className="empty-state"><b>Setup clients unavailable</b><p>{error}</p><button className="secondary-button" onClick={refresh}>Retry</button></div> : <div className="setup-choice-grid">{clients.map(item => <Choice key={item.id} selected={item.id === clientID} title={item.display_name} detail={item.description} onClick={() => chooseClient(item)} />)}</div>}
      </Step>
      <Step number={2} title="Choose your environment" active={step === 2} complete={step > 2} summary={environment?.label}>
        <div className="setup-choice-grid">{client?.environments.map(item => <Choice key={item.id} selected={item.id === environmentID} title={item.label} detail={item.shell === "powershell" ? "PowerShell" : "Bash"} onClick={() => chooseEnvironment(item)} />)}</div>
        <button className="text-button" onClick={() => setStep(1)}>Change client</button>
      </Step>
      <Step number={3} title="Install and configure" active={step === 3} complete={step > 3} summary={client && environment ? `${client.display_name} · ${environment.label}` : undefined}>
        {environment ? <>
          {environment.configuration_note ? <p className="setup-note">{environment.configuration_note}</p> : null}
          {environment.config_file ? <p className="setup-file"><span>Configuration location</span><code>{environment.config_file}</code></p> : null}
          <CodeBlock label={`${environment.shell === "powershell" ? "PowerShell" : "Terminal"} install command`} value={environment.install_command} onCopy={() => copy(environment.install_command)} />
          <div className="setup-actions"><button className="secondary-button" onClick={() => setStep(2)}>Change environment</button>{environment.config_url ? <button className="secondary-button" onClick={() => void preview()}>Preview configuration</button> : null}<button className="primary-button" onClick={() => setStep(4)}>Continue to verification</button></div>
          {configError ? <div className="empty-state"><b>Configuration unavailable</b><p>{configError}</p></div> : config ? <CodeBlock label="Live configuration" value={config} onCopy={() => copy(config)} /> : null}
        </> : null}
      </Step>
      <Step number={4} title="Verify locally" active={step === 4} complete={false}>
        {client && environment ? <>
          <p className="setup-note">The gateway generated your setup instructions. Run this command on the configured computer to verify the local client installation.</p>
          <CodeBlock label="Local verification command" value={environment.verify_command} onCopy={() => copy(environment.verify_command)} />
          <div className="setup-launch"><span>Then launch {client.display_name} with</span><code>{environment.launch_command}</code><button className="text-button" onClick={() => copy(environment.launch_command)}>Copy</button></div>
          <div className="setup-actions"><button className="secondary-button" onClick={() => setStep(3)}>Back to installation</button><button className="primary-button" onClick={() => { setClientID(""); setEnvironmentID(""); setConfig(""); setStep(1); }}>Configure another client</button></div>
        </> : null}
      </Step>
    </div>
  </PageFrame>;
}
