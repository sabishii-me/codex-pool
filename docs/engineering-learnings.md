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

## Three-environment deployment

- Runtime environments are exactly Test (`127.0.0.1:18991`), Staging (`127.0.0.1:18990`), and Production (`localhost:8989`). Do not create a fourth candidate runtime; immutable images are artifacts.
- Test uses image `codex-pool:dev` and isolated `dev/pool`, `dev/data`, and `dev/provider-specs` mounts. Synthetic local sessions are Test-only.
- Staging receives an explicit immutable image promoted from Test and keeps isolated `staging/pool`, `staging/data`, and `staging/provider-specs` mounts. It uses real authentication and is not permanently pinned.
- Browser cookies are hostname-scoped, so Test and Staging sessions can collide on `127.0.0.1`; do not assume port isolation. Use separate browser contexts or clear cookies during cross-environment review.
- Compose interpolation variables use `DEV_` and `STAGING_` prefixes so root Production settings cannot be imported accidentally.
- Deployment changes code and runs destination migrations in place; it does not copy entire data directories.
- Never share writable OAuth/provider state between environments; refresh-token rotation can race and corrupt credentials.
- Host-side migration can change bind-mount ownership. Normalize ownership before starting the unprivileged runtime and verify SQLite can create WAL/SHM files.

## UI/data contract

- A public connection hash is not an understandable label.
- Upstream UUIDs are useful technical identifiers but poor primary presentation.
- Generic UI must render `display_name`; provider-specific email/workspace/tenant information belongs in optional identity metadata.
- Unknown values must not be displayed as zero.
- Provider-specific token accounting belongs in normalized backend view models, not React branches.

## Protocol semantics and routing affinity

- A complete prompt can be independently executable and still benefit from provider-side prompt caching. Stateless correctness, cache affinity, and provider-managed conversation state are separate concepts.
- Interpret routing data only through the declared wire protocol or an explicitly registered client/provider extension. Similar field names do not establish equivalent semantics.
- Preservation can be permissive, but interpretation must be strict: an unknown extension may be forwarded where the adapter contract permits it, but it must not influence connection selection.
- `prompt_cache_key`, client session IDs, provider conversation references, and `previous_response_id` must not be collapsed into one untyped conversation string.
- OpenAI cache keys are client/provider cache-routing hints where the selected contract supports them. Forward them unchanged; do not invent, rewrite, or derive them from prompt content.
- Anthropic `cache_control` identifies cacheable boundaries but does not by itself provide a unique affinity key. Preserve it and its cache-read/cache-creation usage without manufacturing a session.
- Provider response/conversation references may be account-scoped state. Bind them strictly to the connection that created them unless a protocol adapter proves complete safe replay.
- Client session and cache identities are soft affinity by default. Namespace their internal HMAC by GatewayUser, provider, canonical model, protocol, and affinity kind; never persist or log raw values.
- Do not use attribution fields such as `metadata.user_id` as conversation identity without an explicit client contract.
- Official public API documentation defines field meaning, but does not guarantee cache portability across separate subscription accounts or internal Codex transports. Treat those as observed performance behavior, not correctness contracts.
- `store=false` and prompt caching are separate concerns. Never infer supported response continuation merely because a field is present.

## Cost-aware pooled scheduling

- The objective is minimizing total economic waste: paid capacity expiring at reset, avoidable uncached repeated input, overflow spend, failures/retries, and latency—not making request counts look equal.
- Balance new sessions and unbound requests across all eligible Codex Plus, Pro, and Prolite connections. Preserve one request/turn on one connection.
- Do not encode plan multipliers. Learn per-connection effective tokens per observed quota percentage from canonical usage and valid quota observation intervals.
- Evaluate primary and secondary quota windows together. Remaining allowance is useful only at the rate the tighter window can safely admit.
- Provider percentages are stale and quantized. Reserve provisional local usage and in-flight work immediately, then reconcile it when fresh telemetry arrives.
- Unknown quota remains unknown. Give low-confidence connections bounded exploration; never treat missing data as zero use or unlimited capacity.
- Existing soft affinity needs hysteresis and minimum residence so small telemetry changes do not destroy cache locality through oscillation.
- Break affinity for health, cooldown, incompatibility, hard limits, reserve protection, exceptional explicit policy, or a sustained material economic advantage—not for cosmetic equality.
- Most correction should occur by assigning new sessions to underused, near-reset capacity. Moving an active cached session is a later evidence-driven optimization.
- Exactly-once usage remains the accounting authority. Provisional scheduling state is not a second ledger and must never alter canonical attribution.
