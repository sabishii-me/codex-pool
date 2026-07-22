import { afterEach, describe, expect, it, vi } from "vitest";
import { loadDashboardResources, loadProviderConnectionsV2, prepareCodexOAuthBroker, renameProviderConnection, startAccountOAuth } from "./api";

afterEach(() => vi.unstubAllGlobals());

describe("API compatibility errors", () => {
  it("preserves plain-text backend errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("provider connection not found\n", { status: 404, statusText: "Not Found" })));
    await expect(loadProviderConnectionsV2()).rejects.toThrow("provider connection not found");
  });
});

describe("dashboard compatibility hydration", () => {
  it("keeps successful resources when a secondary endpoint fails", async () => {
    const stats = { total_accounts: 1 };
    const catalog = { models: [] };
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.includes("/stats")) return new Response(JSON.stringify(stats), { status: 200 });
      if (url.includes("/signal")) return new Response(JSON.stringify({ error: "rollup unavailable" }), { status: 503 });
      if (url.includes("/catalog")) return new Response(JSON.stringify(catalog), { status: 200 });
      throw new Error(`unexpected URL ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const resources = await loadDashboardResources();
    expect(resources.stats).toEqual(stats);
    expect(resources.catalog).toEqual(catalog);
    expect(resources.signal).toBeUndefined();
    expect(resources.errors).toEqual(["analytics: rollup unavailable"]);
  });
});

describe("Codex OAuth broker handshake", () => {
  it("prepares the local broker before creating a gateway session", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ lease_id: "lease", port: 1457, expires_at: "2026-01-01T00:00:00Z" }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ oauth_url: "https://auth.openai.test", verifier: "verifier", session_id: "session" }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const lease = await prepareCodexOAuthBroker("http://localhost:8989");
    await startAccountOAuth("codex", lease.port);

    expect(fetchMock).toHaveBeenNthCalledWith(1, "http://127.0.0.1:1460/v1/leases", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ gateway_origin: "http://localhost:8989" }),
    }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/pool/accounts/codex/add", expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ redirect_port: 1457 }),
    }));
  });

  it("reports an actionable error when the broker is unavailable", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("connection refused")));
    await expect(prepareCodexOAuthBroker("http://localhost:8989")).rejects.toThrow("Codex OAuth broker is not running");
  });
});

describe("provider connection API v2", () => {
  it("loads canonical provider identities without legacy account fields", async () => {
    const payload = [{
      id: "connection-1",
      public_id: "public-1",
      provider_id: "codex",
      identity: { display_name: "Production Codex", attributes: { region: "us-east" } },
      disabled: false,
      dead: false,
      inflight: 0,
      penalty: 0,
      score: 1,
      is_primary: true,
      usage: {},
      totals: {},
    }];
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(loadProviderConnectionsV2()).resolves.toEqual(payload);
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/provider-connections", { cache: "no-store" });
  });

  it("renames through the canonical identity route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: "renamed", connection_id: "connection-1" }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await renameProviderConnection("connection-1", "Primary Codex");
    expect(fetchMock).toHaveBeenCalledWith("/api/v2/provider-connections/connection-1/identity", expect.objectContaining({
      method: "PATCH",
      body: JSON.stringify({ display_name: "Primary Codex" }),
    }));
  });
});
