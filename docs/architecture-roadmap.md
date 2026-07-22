# Architecture Roadmap

Status: proposed

Baseline commit: `267e80b` (`feat: expand provider gateway and usage accounting`)

Related implementation learnings: [`engineering-learnings.md`](engineering-learnings.md)

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

- Move Anthropic Messages, OpenAI Chat, OpenAI Responses, and Gemini behavior behind tested protocol contracts.
- Keep Codex and Antigravity custom implementations.

Exit criterion: provider implementations no longer duplicate protocol usage parsing or streaming logic.

### Phase 4 — Add declarative provider specifications

- Define the provider schema and loader.
- Convert DeepSeek and Z.ai first.
- Convert Qwen, MiniMax, Xiaomi, Kimi Platform, OpenRouter, and NVIDIA.
- Add atomic runtime hot reload.

Exit criterion: a compatible provider can be added at runtime without recompiling Go or editing React.

### Phase 5 — Centralize routing and selection

- Build one model route registry for normal, streamed, websocket, and large-body paths.
- Extract connection selection, retry policy, cooldowns, and lifecycle state.

Exit criterion: one route definition drives every request path.

### Phase 6 — Separate HTTP APIs

- Isolate gateway, authentication, provider administration, and read-only data handlers.
- Return `display_name`, public connection ID, and visibility-filtered structured identity metadata from connection view models.
- Do not expose provider-specific identity fields directly on generic stats DTOs.
- Generate frontend API types from a schema where practical.

Exit criterion: UI/data handlers do not depend on proxy internals or mutable connection structs.

### Phase 7 — Operational hardening

- Give background jobs a shared cancellation context.
- Add graceful shutdown and bounded worker ownership.
- Add persistence health metrics, dead-letter/retry visibility, and reconciliation checks.

Exit criterion: startup, reload, failure, and shutdown behavior are deterministic and observable.

### Phase 8 — Evaluate physical service separation

After the event and API contracts stabilize, consider:

```text
Gateway → durable usage events → usage/analytics service → data API → UI
```

Do not split deployment merely to compensate for unclear package boundaries.

### Phase 9 — UI/UX redesign

Review and redesign the UI separately after the domain language and data API are agreed. The UI should organize around users, providers, provider connections, models, usage, and system health—not the current overloaded account abstraction.

## First engineering milestone

> Every request through every provider produces exactly one canonical, durably persisted usage event, and every displayed total derives from those events.

This milestone is the prerequisite for reliable provider hot loading, analytics, economics, and UI redesign.
