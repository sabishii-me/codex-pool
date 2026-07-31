import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { OperatorProviderConnectionV2 } from "../types";
import { AccountContribution, ConnectionsPage, OAuthSessionDetails } from "./admin";

describe("provider account contribution UX", () => {
  it("exposes Add account even when no provider connections exist", () => {
    const html = renderToStaticMarkup(<ConnectionsPage state={{ status: "empty" }} onRefresh={async () => {}} onAuthorizationLost={() => {}} />);
    expect(html).toContain("Add account");
    expect(html).toContain("No provider connections are configured");
  });

  it("renders a copyable one-time authorization link for another browser", () => {
    const html = renderToStaticMarkup(<OAuthSessionDetails providerLabel="Claude" oauthURL="https://auth.example/authorize?state=one-time" phase="authorizing" copied={true} showCallbackInput={true} credential="" onOpen={() => {}} onCopy={() => {}} onCredentialChange={() => {}} />);
    expect(html).toContain("Copied ✓");
    expect(html).toContain("https://auth.example/authorize?state=one-time");
    expect(html).toContain("Paste callback URL");
    expect(html).toContain("final localhost URL");
  });

  it("presents reauthorization as an update to the existing stable connection", () => {
    const connection = { id: "stable", public_id: "public", provider_id: "codex", identity: { display_name: "person@example.com", attributes: { email: "person@example.com" } } } as unknown as OperatorProviderConnectionV2;
    const html = renderToStaticMarkup(<AccountContribution reauthorizing={connection} initialProvider="codex" onClose={() => {}} onAdded={async () => {}} />);
    expect(html).toContain("Reauthorize account");
    expect(html).toContain("same upstream account updates this connection rather than creating a duplicate");
    expect(html).toContain("person@example.com");
    expect(html).toContain("Generate authorization link");
    expect(html).not.toContain("<select");
  });

  it("keeps one recovery action for a dead OAuth connection", () => {
    const connection = { id: "stable", public_id: "public", provider_id: "codex", dead: true, disabled: false, inflight: 0, is_primary: false, identity: { display_name: "person@example.com", attributes: {} }, runtime: { status: "dead" }, totals: {}, reset_credits: { available_count: 0, expirations: [], known: false, management_available: false, inventory_refresh_available: false, redemption_available: false, dashboard_url: "#" } } as unknown as OperatorProviderConnectionV2;
    const html = renderToStaticMarkup(<AccountContribution reauthorizing={connection} initialProvider="codex" onClose={() => {}} onAdded={async () => {}} />);
    expect(html).toContain("Reauthorize account");
    expect(html).not.toContain("Activate reauthorized account");
  });

  it("includes every backend-supported contribution provider and credential modes", () => {
    const html = renderToStaticMarkup(<AccountContribution onClose={() => {}} onAdded={async () => {}} />);
    for (const provider of ["Codex", "Claude", "Google Antigravity", "Kimi Coding Plan", "Kimi Platform", "MiniMax", "Z.ai", "Xiaomi", "DeepSeek", "Qwen", "OpenRouter", "NVIDIA", "Grok"]) {
      expect(html).toContain(provider);
    }
    expect(html).toContain("Generate authorization link");
  });
});
