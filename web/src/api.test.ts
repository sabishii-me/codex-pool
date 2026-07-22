import { afterEach, describe, expect, it, vi } from "vitest";
import { prepareCodexOAuthBroker, startAccountOAuth } from "./api";

afterEach(() => vi.unstubAllGlobals());

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
