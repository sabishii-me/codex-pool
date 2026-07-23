# Engineering Learnings

This document records implementation lessons that should survive individual refactors and prevent known failures from being reintroduced.

## Codex OAuth on a Docker-hosted gateway

### Protocol constraints

- OpenAI's Codex OAuth client allowlists only loopback callback ports `1455` and `1457`.
- An arbitrary gateway port such as `8989` or `18990` is rejected by the authorization server, even when the path is `/auth/callback`.
- Device authorization is a different product flow and can require a ChatGPT security setting. It is not a transparent replacement for regular Codex browser OAuth.

### Failed approaches

#### Redirect directly to the gateway port

This fails at authorization because the redirect URI is not in the Codex client allowlist.

#### Publish `1455` permanently from Docker

This makes OAuth work but reserves a scarce provider callback port for the gateway lifetime. Codex CLI, Pi, and other tools cannot acquire it on demand.

#### One-shot Compose relay

A manually started, single-callback container releases the port correctly but is unacceptable product UX:

- users must run infrastructure commands before every account;
- consecutive account additions fail after the first relay exits;
- a stale callback page is confusing;
- the dashboard cannot prove readiness before opening OpenAI;
- container restart can invalidate in-memory gateway OAuth state.

#### In-memory OAuth sessions

Pending state and PKCE material disappear on gateway restart. A valid browser callback then reports an expired state. OAuth sessions must be durable for their short lifetime.

### Working architecture

Use a persistent host-side broker with ephemeral callback leases:

1. Broker control API stays on `127.0.0.1:1460`.
2. Dashboard prepares a lease before opening OpenAI.
3. Broker binds `1455`, falling back to `1457`.
4. Gateway creates state and PKCE using the selected redirect port.
5. Broker forwards one callback to the gateway.
6. Broker immediately releases the callback port.
7. The same broker process serves unlimited sequential account additions.

The broker must reject concurrent ambiguous leases, enforce exact CORS/gateway origins, bind only to loopback, omit codes from logs, and release leases on success, cancellation, and timeout.

### Browser details

- Open a blank authorization tab synchronously inside the click handler; opening it after network calls may trigger popup blocking.
- Do not detach `window.opener` when callback completion relies on `postMessage`; status polling remains the reliable fallback.
- Chromium private-network requests from a dashboard to a loopback broker require correct CORS preflight handling, including `Access-Control-Allow-Private-Network` when requested.
- The browser must not open OpenAI until both broker lease and gateway OAuth session creation succeed.

### Durable session rules

Persist pending sessions under the gateway data directory with mode `0600` on Unix/container deployments. Store only:

- session ID;
- state;
- PKCE verifier/challenge;
- exact redirect URI;
- target origin;
- timestamps;
- terminal status and sanitized error/result.

Never persist the authorization code. Use a 15-minute expiry and an exactly-once transition from `pending` to `exchanging` to terminal state. Duplicate callbacks must not exchange twice.

### Connection identity

- Credential filenames should be deterministic full SHA-256 hashes of the upstream `chatgpt_account_id`.
- Reauthorizing one upstream account should update the same file and preserve durable metadata.
- Different upstream accounts must never be distinguished using `_2`, `_3`, or email-derived filenames.
- Email is useful temporary presentation metadata for Codex but is not a generic connection identity.
- The target domain model uses an editable `ProviderConnection.display_name` plus optional structured identity attributes.

### Test lessons

- Test at least three sequential leases in one broker process.
- Occupy `1455` and assert transparent `1457` fallback.
- Occupy both ports and assert an actionable error.
- Prove callback ports can be rebound after success and timeout.
- Test the broker from a real Chromium page, not only direct HTTP clients, to catch CORS/private-network behavior.
- Restart the gateway between session creation and callback and verify the pending session survives.
- Run frontend build and Go tests sequentially because Vite clears `web/dist`, which Go embeds at compile time; parallel execution creates transient missing-embed failures.
- Windows `os.Stat().Mode().Perm()` does not reliably represent Unix owner-only permission semantics. Assert `0600` on Unix/Linux, while production container execution remains the security boundary.

## Context-window metadata

- Client-facing `contextWindow` is an operational input budget, not necessarily the provider's largest total token envelope.
- Pi uses this value to trigger proactive compaction at `contextWindow - reserveTokens`.
- GPT-5.6 Sol/Terra/Luna must advertise the conservative Codex client input budget of `272000`, not `372000`; otherwise Pi waits too long and upstream can reject context before proactive compaction.
- Keep generated Pi configuration, Codex model injection, and executable catalog tests synchronized.
- Structured overflow errors still let Pi compact and retry, but correct metadata prevents overflow recovery from becoming the normal compaction path.

## Isolated development deployment

- Staging freezes the validated legacy UI at image `codex-pool:staging-a91560b`, endpoint `127.0.0.1:18990`, and separate `staging/pool`, `staging/data`, and `staging/provider-specs` mounts. Its Compose file has no build section.
- Active development uses image `codex-pool:dev`, endpoint `127.0.0.2:18991`, and separate `dev/pool`, `dev/data`, and `dev/provider-specs` mounts. A different loopback IP—not merely a different port—is required because browser cookies are host-scoped.
- Use `127.0.0.1` for development and `localhost` for production to prevent host-only browser cookies from colliding across ports.
- Development Compose interpolation variables use a `DEV_` prefix so the root production `.env` cannot be imported accidentally.
- Never mount or copy live OAuth credentials into both instances; refresh-token rotation can race and corrupt state.

## UI/data contract

- A public connection hash is not an understandable label.
- Upstream UUIDs are useful technical identifiers but poor primary presentation.
- Generic UI must render `display_name`; provider-specific email/workspace/tenant information belongs in optional identity metadata.
- Unknown values must not be displayed as zero.
- Provider-specific token accounting belongs in normalized backend view models, not React branches.
