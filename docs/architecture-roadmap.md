# Architecture Roadmap

Status: active

Historical modularization baseline: `267e80b` (`feat: expand provider gateway and usage accounting`)

Current deployed Production code revision: `9ffe963ddcd1`

Current roadmap documentation checkpoint: `9ed853ec19a0`

Related implementation learnings: [`engineering-learnings.md`](engineering-learnings.md)

Active scheduling design: [`protocol-affinity-and-codex-balancing-plan.md`](protocol-affinity-and-codex-balancing-plan.md)

## Purpose

Make the gateway maintainable while preserving its current protocol coverage and deployment simplicity. The first goal is a modular monolith with explicit contracts. Separate deployed services should come only after those contracts are stable.

## Domain language

Use distinct names for user identity and upstream capacity:

- **GatewayUser**: a person authenticated into the gateway.
- **Provider**: an upstream service definition, such as DeepSeek or Z.ai.
- **ProviderConnection**: one configured upstream credential, endpoint, region, subscription, and health state.
- **ProviderPool**: the set of connections available for one provider.
- **ModelRoute**: a public model mapped to a provider and upstream model.
- **UsageEvent**: one canonical, terminal accounting event for one request.

The current `Account` abstraction should migrate to `ProviderConnection`. Compatibility fields and existing credential files can retain their old names during migration.

### Provider connection identity

A connection must not assume every provider exposes an email address. Use a provider-neutral identity contract:

```text
ProviderConnection
  id                  stable internal connection ID
  display_name        required, user-editable UI label
  provider_id         owning provider definition
  external_subject    optional upstream account/subject ID
  identity_attributes optional typed metadata (email, tenant, workspace, region)
```

Rules:

- Generic connection UI uses `display_name`, never email as its identity field.
- Provider plugins may suggest an initial name from email, workspace, subscription, or a shortened subject ID.
- Email is optional metadata and is not required by routing, usage, persistence, or generic DTOs.
- Renaming does not change the stable connection ID or usage attribution.
- Sensitive identity attributes have explicit visibility rules.
- API v2 returns structured identity metadata instead of provider-specific fields on generic connection DTOs.

The current Codex `account_email` field is a temporary compatibility aid and must leave the generic stats contract when this model lands.

## Current structural problems

1. All Go code is in `package main`; major files have become large state machines.
2. `proxyHandler` is a system-wide service locator.
3. The `Provider` interface combines credentials, refresh, transport, routing, protocol detection, usage parsing, and quota parsing.
4. Provider knowledge is duplicated across registries, switches, catalogs, routes, watchers, administration, Pi output, and the React provider table.
5. Normal and large-body request paths use different routing registries.
6. BoltDB/in-memory totals and SQLite analytics are competing usage sources of truth.
7. Persistence errors are often ignored, allowing those stores to diverge.
8. Usage fields have inconsistent semantics across OpenAI- and Anthropic-compatible providers.
9. React contains provider-specific accounting and subscription rules.
10. Background jobs do not share one lifecycle and cancellation owner.
11. Provider tests are numerous but do not form a complete contract matrix.

## Target modular structure

```text
cmd/gateway/
    main.go

internal/domain/
    user.go
    provider.go
    connection.go
    model.go
    usage.go

internal/config/
    loader.go
    schema.go
    watcher.go

internal/provider/
    registry.go
    spec.go
    connection_store.go
    plugins/

internal/protocol/
    anthropic/
    openaichat/
    responses/
    gemini/

internal/routing/
    model_router.go
    connection_selector.go
    retry_policy.go

internal/proxy/
    service.go
    stream.go
    websocket.go

internal/usage/
    collector.go
    recorder.go
    store.go
    aggregates.go

internal/analytics/
    service.go
    economics.go
    quota.go

internal/auth/
    users.go
    sessions.go
    mfa.go

internal/httpapi/
    gateway.go
    admin.go
    data.go

web/
```

## Provider design

### Protocol engines

Keep a small set of code-backed engines:

- Anthropic Messages
- OpenAI Chat Completions
- OpenAI Responses
- Gemini
- Codex custom transport/websocket behavior
- Antigravity custom behavior

A protocol engine owns request preparation, streaming, response translation, and usage decoding for its wire protocol.

### Declarative provider specifications

Most providers should be runtime-loaded specifications over a protocol engine:

```yaml
id: deepseek
protocol: anthropic-messages
base_url: https://api.deepseek.com/anthropic

auth:
  type: bearer
  credential_field: api_key

models:
  - id: deepseek-v4-flash
    aliases: [deepseek-flash]
    context_window: 128000
    max_output_tokens: 32768

validation:
  method: POST
  path: /v1/messages
  model: deepseek-v4-flash

usage:
  profile: anthropic-split
```

Provider specifications may define endpoints, region variants, headers, authentication style, model aliases, capabilities, validation, usage profile, quota headers, and pricing.

Specifications are schema-validated and loaded into an immutable registry snapshot. Hot reload atomically swaps snapshots only after validation succeeds. Invalid changes leave the previous snapshot active.

Custom plugins remain available for unusual OAuth, signing, discovery, or streaming behavior. The provider schema must not become a programming language.

## Canonical usage contract

```go
type UsageEvent struct {
    RequestID     string
    UserID        string
    ProviderID    string
    ConnectionID  string
    ModelID       string
    UncachedInput int64
    CacheRead     int64
    CacheWrite    int64
    Output        int64
    Reasoning     int64
    StartedAt     time.Time
    CompletedAt   time.Time
}
```

The contract must explicitly define whether reasoning is a subset of output and must not overload one input field with different provider semantics.

One usage recorder owns:

- split-event aggregation;
- request attribution;
- exactly-once deduplication;
- validation and normalization;
- cost calculation;
- transactional persistence;
- retries and observable failures.

## Data ownership

SQLite becomes the authoritative event store:

```text
usage_events
provider_connections
quota_observations
usage_daily
connection_totals
```

`usage_events` is immutable and unique by request ID. Daily and connection totals are rebuildable projections. In-memory values are caches. BoltDB may remain temporarily for compatibility but must stop acting as an independent accounting authority.

## API boundary

Expose a versioned data API independent from the gateway transport API:

```text
GET /api/v2/providers
GET /api/v2/provider-connections
GET /api/v2/models
GET /api/v2/usage/summary
GET /api/v2/usage/timeseries
GET /api/v2/economics
```

The backend returns normalized values such as processed tokens, cache reads/writes, estimated cost, subscription spend, quota utilization, and availability. The UI formats and visualizes these values but does not implement provider accounting semantics.

## Roadmap

### Phase 0 — Characterize behavior

Executable baseline: [`provider-contract-matrix.md`](provider-contract-matrix.md)

- Build a table-driven provider contract harness.
- Cover streaming and non-streaming responses.
- Cover cache read, cache write, reasoning, translation, and large request routing.
- Assert exactly one persisted usage event and unchanged response bytes.
- Fix platform-dependent tests and establish CI.
- Capture existing public API behavior as compatibility tests.

Exit criterion: every provider has an explicit support matrix and executable fixtures.

### Phase 1 — Establish domain language

Status: complete. The provider-neutral `ConnectionIdentity` seam is implemented with durable `display_name`, optional `external_subject`/attributes, migration-safe legacy fallback, normalized stats/operator DTOs, an elevated-operator rename endpoint, and a compatibility-free `/api/v2/provider-connections` contract. `GatewayUser`, `ProviderID`, `ProviderConnection`, `ProviderPool`, and `ModelRoute` are the canonical declared types used by production routing, provider adapters, lifecycle management, usage recording, gateway-user persistence, the model catalog, and TypeScript view-model consumers. Live usage recording writes `ConnectionID`/`ProviderID`; legacy `AccountID`/`AccountType` fields are normalized only for old Bolt events, projections, and source compatibility. Generic connection UI consumes v2 identity and does not read legacy email or upstream-account fields. Existing `Account`, `PoolUser`, `PoolUserStore`, `poolState`, and their constructors remain deprecated source aliases/wrappers; `AccountType`, compatibility JSON fields, credential-file fields, and `/admin/accounts` mutation URLs are explicit compatibility boundaries rather than production domain vocabulary.

- Introduce `GatewayUser`, `Provider`, `ProviderConnection`, `ProviderPool`, and `ModelRoute`.
- Add `ConnectionIdentity` with a required `display_name` and optional provider metadata.
- Backfill deterministic display names for existing connections and support operator rename.
- Keep email, tenant, workspace, and upstream subject identifiers optional and typed.
- Migrate `Account` and related method names internally.
- Preserve old JSON and API fields through compatibility adapters.
- Remove generic frontend dependence on `AccountStats.account_email` after API v2 is available.

Exit criterion: new code no longer uses `Account` for upstream credentials, and generic connection UI does not assume email exists.

### Phase 2 — Extract usage as an internal service

- Introduce canonical `UsageEvent`.
- Centralize streaming and non-streaming collection.
- Add request IDs and deduplication.
- Persist transactionally to SQLite.
- Stop discarding persistence errors.
- Rebuild totals from persisted events.
- Return normalized totals to the existing UI.

Exit criterion: every successful provider request produces exactly one durable event and all totals reconcile.

### Phase 3 — Extract protocol engines

Status: complete. Anthropic Messages, OpenAI Chat, OpenAI Responses, and Gemini usage engines are tested protocol capabilities. Claude, DeepSeek, Z.ai, MiniMax, Qwen, OpenRouter, Kimi, Kimi Platform, and Xiaomi delegate Anthropic streaming/non-streaming usage normalization to one implementation. NVIDIA and both Kimi adapters delegate OpenAI Chat normalization with an explicit cache-billing policy, preserving Kimi's legacy accounting rather than silently changing totals. Codex, Grok, and sampled response accounting delegate Responses normalization with explicit options for Chat aliases, cache writes, upstream billable totals, and billable-only events; Codex retains its custom `token_count` fallback. Gemini and Antigravity share native `usageMetadata` normalization with explicit envelope and reasoning-only policies, while Antigravity transport and translation remain custom. A shared stream observer owns SSE JSON envelope decoding, split-event accumulation, model fallback, and incomplete-stream flushing across normal and streamed-body proxy paths. Protocol stream detectors centralize content-type and path recognition for every provider. Translation writers remain centralized gateway conversion components rather than provider-local implementations. Phase 0 provider contracts remain the non-regression boundary.

- Move Anthropic Messages, OpenAI Chat, OpenAI Responses, and Gemini behavior behind tested protocol contracts.
- Keep Codex and Antigravity custom implementations.

Exit criterion: provider implementations no longer duplicate protocol usage parsing or streaming logic.

### Phase 4 — Add declarative provider specifications

Status: complete. A strict JSON `ProviderSpec` schema, immutable declarative providers, atomic registry snapshots, deterministic directory loading, and watched all-or-nothing hot reload are implemented. Runtime specifications can add Anthropic Messages or OpenAI Chat providers, credential directories, endpoints, auth strategies, ordered usage profiles, constrained quota profiles, fixed or prefix-wildcard model routing, canonical rewriting, target-format translation, and catalog entries without recompiling Go or editing React. Invalid reloads retain both the previous provider snapshot and connection pool. DeepSeek, Z.ai, Qwen, MiniMax, Xiaomi, Kimi Platform, OpenRouter, and NVIDIA are thin compatibility aliases over the generic implementation and route solely through declarative matching. The frontend accepts open provider IDs and renders unknown runtime providers with a safe neutral presentation. Examples and operator documentation are in `provider-specs.example/` and `docs/provider-specifications.md`.

- Define the provider schema and loader.
- Convert DeepSeek and Z.ai first.
- Convert Qwen, MiniMax, Xiaomi, Kimi Platform, OpenRouter, and NVIDIA.
- Add atomic runtime hot reload.

Exit criterion: a compatible provider can be added at runtime without recompiling Go or editing React.

### Phase 5 — Centralize routing and selection

Status: complete. `ModelRouteRegistry` returns one immutable `ResolvedModelRoute`—provider, canonical model, and body-safety policy—for buffered HTTP, large streamed bodies, Antigravity custom execution, and WebSocket `response.create` frames. Large requests reject routes requiring whole-body sanitization or translation, and established WebSockets reject cross-provider model switches instead of silently using the wrong credential. `ConnectionSelector` is the only production boundary for pinned, exact-ID, cyber-access, image-capability/fanout, general, and Antigravity model-capability selection; the proven scoring and lifecycle implementation remains behind it. `RetryPolicy` owns attempt budgeting, cooldown wait caps, cancellation-aware rotation, and buffered cyber retry decisions. Source-level architectural tests prevent request paths from bypassing these boundaries.

- Build one model route registry for normal, streamed, websocket, and large-body paths.
- Extract connection selection, retry policy, cooldowns, and lifecycle state.

Exit criterion: one route definition drives every request path.

### Protocol-aware affinity and reset-aware Codex scheduling

Status: **P0 critical; next routing implementation milestone**. Detailed implementation plan: [`protocol-affinity-and-codex-balancing-plan.md`](protocol-affinity-and-codex-balancing-plan.md).

Current Production evidence shows the structural failure directly: five healthy Plus connections displayed `0% used` while the one healthy Pro connection displayed `32% used`. The percentages must be compared only with their corresponding window/reset/retrieval metadata, and provider percentages are quantized, but this pattern is still consistent with—and expected from—the current Pro/Prolite Tier 1 gate. It places already-paid Plus allowance at material risk of expiring unused while concentrating failures and cache namespaces on Pro. This is a cost and capacity incident, not cosmetic fairness work.

The economic objective is not equal request counts. It is to consume already-paid capacity before provider reset while avoiding unnecessary loss of prompt-cache locality and preserving provider-owned state correctly.

Required architecture:

- protocol engines extract typed routing context from the declared OpenAI Responses, OpenAI Chat, Anthropic Messages, Gemini, or explicit client/provider-extension contract;
- routing must stop assigning semantics through broad JSON/header heuristics such as arbitrary `metadata.user_id` or lookalike session fields;
- preserve recognized client cache declarations unchanged, never synthesize an upstream cache key, and never derive one from prompt content;
- keep cache hints, client session identity, provider response state, provider conversations, and asynchronous operation ownership as different affinity kinds;
- use privacy-safe, GatewayUser/provider/model/protocol-namespaced soft bindings for cache/session locality;
- require exact connection ownership for provider-managed state unless an adapter proves a complete and safe replay;
- replace ordinary Codex Pro/Prolite Tier 1 and Plus Tier 2 behavior with one all-eligible capacity pool while preserving required-plan and exceptional cyber policy;
- score both primary and secondary windows using reset timing, normalized depletion, measured per-connection capacity, health, cooldown, reserves, in-flight work, and penalties;
- learn capacity from canonical usage versus valid observed quota deltas rather than hard-coded plan multipliers;
- apply local provisional debits and in-flight reservations so stale or quantized percentages cannot concentrate bursts;
- balance primarily by assigning new sessions; retain existing soft affinity with hysteresis and break it only for safety or a measured, material economic advantage;
- measure reset waste, cache reads/creation, uncached input, affinity breaks, failures, and latency before enabling economic migration of active sessions.

Implementation order is contract fixtures and observability, typed request context, safe soft affinity, strict provider-state ownership, all-eligible Codex selection, provisional quota debits, then evidence-driven tuning. HTTP, large-body, streaming, and WebSocket request paths must share the same typed rules.

Exit criterion: Production evidence shows less paid capacity expiring unused without unacceptable cache regression; no unknown field affects routing; provider state never silently crosses connections; exactly-once accounting remains reconciled.

### Shared account-state authority

Status: planned and required before Staging can qualify as a Production-like long soak.

Production remains the sole credential, refresh, lifecycle, canonical global usage, burn, and economics authority. Staging must submit canonical usage through a durable exactly-once outbox, read authoritative provider state, and forward mutations rather than writing shared credentials or databases. Authority authentication, replay protection, provenance, conflict visibility, lag, reconciliation, and fail-closed behavior are required. No writable SQLite, Bolt, OAuth, or provider credential directory may be mounted into more than one gateway.

Exit criterion: real Staging provider activity appears in authoritative accounting exactly once, lifecycle changes have one writer, authority lag/conflicts are observable, and restart/retry/reconciliation tests pass.

### Gateway media operation boundary

Status: planned.

Add capability- and operation-aware model routing plus a canonical submit/observe/cancel provider-operation contract. Asynchronous media operations require sticky provider/connection ownership, upstream-submission idempotency, typed `submission_unknown` reconciliation, tracing, and one terminal usage/cost event. The gateway may return transient provider output references but does not become an artifact store, gallery, transcoder, CDN, or product prompt-history service. A separate generation service owns product jobs, uploads, ingestion, durable artifacts, previews, retention, and downloads.

Exit criterion: one fake asynchronous provider proves restart durability, sticky polling/cancellation, ambiguous-submission handling, exactly-once terminal accounting, and transient outputs without gateway artifact persistence.

### Profile security completion

Status: planned.

Complete authenticated MFA authenticator rotation and recovery-code regeneration through `handleMFARegenerate` and `handleMFARegenerateCodes`, retaining the one full-screen elevation gate and accessible product dialogs.

Exit criterion: elevated Admin browser/API acceptance proves rotation, code regeneration, cancellation, typed failures, and recovery behavior without native browser dialogs.

### Phase 6 — Separate HTTP APIs

Status: complete. `ConnectionViewService` is the sole read-model boundary that snapshots mutable `ProviderConnection` state for canonical operator DTOs, pool statistics, and explicit compatibility projections. Pool-stat aggregation, economics, and cyber-policy summaries consume detached snapshots rather than locking pool connections in HTTP handlers. `DataAPI.TryServe` owns read-only pool analytics, model catalog, canonical provider-connection collection, compatibility connection collection, and per-user usage routes. `AccessPolicy` owns signed-in session, administrator allowlist, MFA elevation, ban, and attempt-tracking decisions. `ProviderAdminAPI`, `ProviderContributionAPI`, `ProviderOperationsAPI`, `AuthenticationAPI`, and `SystemAdminAPI` own their route groups while preserving existing authorization order, methods, status codes, errors, callbacks, and JSON compatibility. `proxyHandler.ServeHTTP` is now an orchestrator for these boundaries plus static/setup compatibility and gateway/protocol dispatch. Source-level guards prevent route ownership, mutable stats reads, and access decisions from drifting back. The broad `Provider` contract is an explicit compatibility composition of focused capabilities, and isolated consumers use narrower identity/usage contracts. The canonical provider-connections v2 DTO has a checked-in JSON schema, a deterministic TypeScript generator, backend schema-conformance coverage, and generated frontend types. UI redesign remains deliberately deferred to Phase 9.

Current route ownership:

| Boundary | Routes |
|---|---|
| Read-only data API | `/api/pool/{stats,whoami,users,origins,daily-breakdown,hourly,signal,catalog}`, `/api/pool/users/:id/{daily,hourly}`, `GET /api/v2/provider-connections`, `GET /admin/accounts` compatibility |
| Authentication/session | `AuthenticationAPI`: `/auth/*`, `/api/pool/session`, `/api/admin/mfa/*`; `AccessPolicy`: signed-in/elevated route decisions |
| Provider administration | `ProviderAdminAPI`: `/api/v2/provider-connections/:id/identity`, `/admin/accounts/:id/{identity,enable,disable,resurrect,refresh}`; `ProviderContributionAPI`: signed-in member additions under `/api/pool/accounts/*`; `ProviderOperationsAPI`: elevated `/admin/{codex,claude,antigravity,kimi,...}` operations with explicit public OAuth callback exceptions |
| System administration | `SystemAdminAPI`: metrics, reload, origins, capacity, rate-limit reset, anonymous purge, and `/admin/pool-users*` |
| Gateway/protocol | `/v1/*`, WebSocket upgrades, Codex compatibility/no-op paths, and upstream fallback |

- Isolate gateway, authentication, provider administration, and read-only data handlers.
- Return `display_name`, public connection ID, and visibility-filtered structured identity metadata from connection view models.
- Do not expose provider-specific identity fields directly on generic stats DTOs.
- Narrow the compatibility `Provider` composition into focused credential loading, authentication, refresh, routing target, stream detection, usage parsing, quota-header parsing, and target-format capabilities; isolated protocol consumers already depend on the narrow usage/identity capabilities.
- Generate canonical provider-connection frontend API types from `schemas/provider-connections-v2.schema.json`; keep legacy stats/admin DTOs as explicit compatibility types.

Exit criterion: UI/data handlers do not depend on proxy internals or mutable connection structs.

### Upstream synchronization backlog

Reviewed `darvell/codex-pool` through upstream commit `2aa8320` on 2026-07-22:

- Integrated `43c94c6` as `ecee91e`: reset-credit redemption refreshes both credit inventory and current Codex quota.
- Adapted `b1100e2` as `a6111c7`: backend pace, aggregate capacity, and per-connection weekly forecasts wait for one percentage point of elapsed budget before extrapolating quantized usage.
- No port needed for `5fec71b`: current declarative Kimi Platform catalog already includes Kimi K3, its 1M alias, richer metadata, docs, and tests.
- Do not port `79f5f3b`: its additive cyber bonus is superseded upstream by weighted fairness in `2aa8320`.
- Adapted `2aa8320` behind `ConnectionSelector`: ordinary Codex routing uses deterministic 2x cyber / 1x non-cyber weighting only among quota-competitive connections, while materially drained connections remain excluded and explicit cyber-policy retries remain cyber-only.
- Adapted `1b7bc67` with bounded-memory protocol rules: exact hosted MCP tools, transcript items, choices, SSE events, JSON outputs, and WebSocket frames are removed while local function/namespace tools, web search, and tool search remain intact. HTTP transformations and each SSE/WebSocket unit are capped by configured memory limits; oversized native Codex Responses requests are rejected before upstream transmission, oversized responses fail closed, and the upstream 512 MiB event/frame allowances were explicitly removed.
- Adapted the WebSocket portion of `a1049d8` in Phase 7: peer close status is preserved, terminations are classified/measured, heartbeat failures are reported, and registered sessions drain during graceful shutdown without bypassing centralized model-route or bounded hosted-MCP enforcement.

### Phase 7 — Operational hardening

Status: complete for the modular-monolith hardening scope. WebSocket lifecycle hardening from upstream `a1049d8` has been adapted across both general and Codex cyber-swap relays without bypassing model-route or hosted-MCP boundaries. Relay termination preserves valid peer close codes, classifies client/upstream/relay outcomes, records labeled metrics, reports heartbeat failures in both directions, normalizes upstream EOF only after a terminal Responses event, tracks active turns, and drains idle/complete sessions with close code 1012 during graceful process shutdown. Active turns receive the configurable `SHUTDOWN_GRACE_SECONDS` period before forced closure. A shared process cancellation context and `backgroundJobs` owner now govern usage polling, quota intelligence, pricing refresh, analytics rollups, Antigravity version/model refresh, Codex fingerprint updates, Claude UUID probing, request-pacer cleanup, and file watching; shutdown joins these jobs before deferred persistence teardown.

- Remaining persistence health, dead-letter/retry, and reconciliation observability can proceed as ongoing operations work rather than blocking the UI phase.

Exit criterion: startup, reload, failure, and shutdown behavior are deterministic and observable.

### Operational maintenance mode

Status: planned.

Add a first-class maintenance coordinator so routine operational work does not require shutting down every gateway capability at once. Maintenance is a backend-owned runtime state, not only a frontend page.

Required behavior:

- expose authenticated Admin controls and auditable status for entering, observing, and leaving maintenance;
- support scoped maintenance domains such as inference admission, WebSockets, provider mutations/OAuth refresh, analytics writes, and projection rebuilds;
- reject newly admitted work in affected domains with a typed `maintenance_unavailable` response and retry guidance;
- allow unaffected health, status, authentication, and read-only maintenance UI routes to remain available;
- drain in-flight HTTP streams, asynchronous provider operations, and WebSocket turns with bounded deadlines before reporting a scope as quiescent;
- pause and join background jobs that can mutate credentials, provider lifecycle, or the selected data store;
- expose drain counts, blockers, elapsed time, operator identity, reason, and safe-to-maintain evidence;
- persist or externally coordinate the maintenance lease so restarts cannot accidentally re-enable writes;
- fail closed if multiple replicas disagree about maintenance ownership;
- provide separate readiness and liveness semantics so traffic leaves maintained scopes without hiding process health;
- require every process that can access a SQLite database to release it before file replacement, recovery, or schema operations that require exclusive ownership;
- provide a tested abort path that resumes paused jobs and admission without losing canonical usage.

The first vertical slice should implement `normal → draining → quiescent → resuming` for inference admission and analytics writers in the current modular monolith. Later service separation may quiesce only the analytics authority while authenticated read-only product routes remain online. This feature must not imply that an open SQLite file can be replaced safely while another process still owns it.

Exit criterion: an Admin can place selected runtime domains into maintenance, observe deterministic drain completion, perform an offline-required storage operation after exclusive ownership is proven, and restore service without restarting unrelated deployed services.

### Phase 8 — Evaluate physical service separation

After the event and API contracts stabilize, consider:

```text
Gateway → durable usage events → usage/analytics service → data API → UI
```

Do not split deployment merely to compensate for unclear package boundaries.

### Pre-Phase 9 — Historical compatibility checkpoint

Status: superseded by the current unified React product and three-environment promotion workflow. The former pinned legacy-control strategy was useful during the initial refactor, but it is no longer an active deployment model. Staging now receives explicitly promoted immutable Test images and retains production-like authentication and isolated durable state. Current soak evidence and incident classification live in [`staging-soak-checkpoint.md`](staging-soak-checkpoint.md).

Historical compatibility findings remain useful: independently refreshed dashboard resources, actionable backend errors, canonical provider identities, unknown-provider handling, and repeatable runtime/error checks remain release requirements.

### Phase 9 — Functional product implementation

Status: **functional product deployed; authority synchronization and Staging qualification remain active**.

The historical Product Design Harness artifact and separate member/operator workspace plan are superseded by [`frontend-product-architecture.md`](frontend-product-architecture.md). The current implementation is one inherited Member → Admin → MFA-elevated product with canonical routes `/`, `/models`, `/usage`, `/setup`, `/profile`, `/admin/connections`, `/admin/members`, and `/admin/system`. Browser/API route separation, real OAuth dev isolation, one MFA gate, scoped Usage, detailed model/provider/connection attribution, measured per-model curves, Connections operations, Members lifecycle operations, System projections, MFA enrollment, no-fake-data enforcement, and accessible product dialogs are implemented.

Current functional checkpoint:

- **Complete and deployed:** shell/capability model, direct routing, Home/Usage ownership, scoped and detailed Usage, Connections, Members baseline, measured System, Profile MFA enrollment, no-placeholder/no-synthetic-data gate, backend-owned selected-model routing context, backend-driven Setup, typed Anthropic overflow/complete usage, image generation/read translation, immutable release images, explicit Test → Staging → Production promotion, real-auth Staging, conflict-safe historical usage migration/recovery, and the Production accounting recovery/cutover documented in [`production-deployment-9ffe963.md`](production-deployment-9ffe963.md).
- **In progress:** Production authority synchronization for ongoing Staging usage/provider lifecycle, Staging-specific authenticated browser acceptance, a new valid long-soak period after authority proof, protocol-aware affinity/reset-aware Codex scheduling, scoped operational maintenance, and functional Profile security rotation/recovery operations.
- **Deferred to Phase 10:** broad accessibility, responsive/visual polish, performance optimization, and release tuning that does not block operation or truthfulness.

The previous Staging elapsed time is not Production-readiness evidence because shared real accounts still lack one ongoing accounting/lifecycle authority boundary. See [`staging-soak-checkpoint.md`](staging-soak-checkpoint.md).

Phase 9 must prioritize real workflows and backend-owned contracts. It must not display future-work cards, fabricated trends, example inventory, inferred routing order, or nonfunctional controls.

Exit criterion: every active route performs its unique product job with real authorized data and working operations; Profile security workflows are functionally complete; the Staging-specific browser matrix and long soak pass against the exact immutable image intended for Production; canonical usage and projections reconcile after restart; no release-blocking incident remains.

### Phase 10 — Product hardening and polish

Status: planned, not active.

After Phase 9 feature completeness, perform validation and policy hardening, accessibility refinement, responsive/visual polish, performance work, reliability soak, and release preparation. Do not interrupt Phase 9 feature delivery for non-blocking polish.


## First engineering milestone

> Every request through every provider produces exactly one canonical, durably persisted usage event, and every displayed total derives from those events.

This milestone is the prerequisite for reliable provider hot loading, analytics, economics, and UI redesign.
