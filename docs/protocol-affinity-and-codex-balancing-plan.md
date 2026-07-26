# Protocol-Aware Affinity and Reset-Aware Codex Scheduling Plan

Status: planned; required before replacing the current Codex tier scheduler

Related roadmap: [`architecture-roadmap.md`](architecture-roadmap.md)

## Purpose

Reduce the real cost of pooled coding traffic by minimizing both:

1. paid subscription capacity that expires unused at a provider reset; and
2. repeated input work caused by unnecessarily losing upstream prompt-cache locality.

The scheduler must not pursue equal request counts or rotate every turn. It should allocate new work according to reset-aware capacity and preserve a recognized client or provider affinity when doing so is safe and economically useful.

This plan also removes routing guesses. Request semantics come only from the selected wire protocol, an explicit provider capability, or a registered client extension.

## Non-goals

This work does not:

- create, rewrite, or derive an upstream prompt cache key;
- hash prompt content to manufacture a session;
- store prompts or provider cache contents;
- claim that caches are portable between subscription accounts;
- interpret arbitrary JSON metadata or similarly named headers as conversation state;
- make equal request count, equal token count, or equal percentage use the objective;
- split one request's tokens across multiple connections;
- replace exact-ID, capability, image, health, cooldown, retry, reserve, or cyber-policy constraints;
- silently remove a state reference to make a retry succeed.

## Authoritative protocol references

The implementation must be checked against the currently supported version of each protocol, with request/response fixtures pinned in tests.

### OpenAI

- Responses API create contract: <https://platform.openai.com/docs/api-reference/responses/create>
- Conversation state guide: <https://platform.openai.com/docs/guides/conversation-state>
- Prompt caching guide: <https://platform.openai.com/docs/guides/prompt-caching>
- Chat Completions create contract: <https://platform.openai.com/docs/api-reference/chat/create>

Relevant concepts must remain distinct:

- `prompt_cache_key` is a cache-routing hint where the selected contract supports it;
- `previous_response_id` refers to prior provider response state;
- a Responses conversation reference refers to provider-managed conversation state;
- a client extension such as `conversation_id` is not automatically an official provider conversation object.

### Anthropic

- Messages API: <https://docs.anthropic.com/en/api/messages>
- Prompt caching: <https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching>

Anthropic `cache_control` marks cacheable prompt boundaries. It is not, by itself, a unique session key for gateway routing. Cache reads and cache creation remain separate canonical usage dimensions.

### Provider and client extensions

Codex subscription transport, Claude Code headers, Pi behavior, and other coding-agent extensions are not assumed to have base-protocol semantics. Each extension used by routing must have:

- a named client/provider profile;
- an exact body location or canonical header name;
- a declared meaning and strictness;
- forwarding rules;
- fixtures captured from the supported client;
- a compatibility/version policy.

Observed provider behavior is evidence for optimization, not a correctness contract. In particular, cross-account cache sharing is not guaranteed unless the provider documents it for the exact product and credential type.

## Core rules

### Parse by declared protocol

Route resolution identifies the incoming protocol before affinity extraction:

```text
route + protocol engine + provider capabilities + registered client profile
    -> typed request routing context
```

The scheduler never scans arbitrary JSON independently.

### Preserve permissively; interpret strictly

A transparent adapter may preserve an allowed extension field unchanged. Preservation does not give that field routing meaning.

Only these sources may influence affinity:

1. a field defined by the selected protocol contract;
2. a field/header defined by an explicit provider capability; or
3. a field/header defined by a registered coding-client profile.

Unknown metadata is ignored for routing. Translation that cannot preserve a documented semantic safely returns a typed protocol/capability error.

### The client declares; the gateway binds; the provider caches

For a client-supplied cache or session identifier:

```text
client identifier --forward unchanged--> provider
       |
       +--HMAC namespaced locally--> preferred ProviderConnection
```

The gateway does not replace the upstream value with its HMAC. It stores no raw identifier in the affinity index.

### Keep affinity kinds typed

```go
type AffinityKind string

const (
    AffinityPromptCache       AffinityKind = "prompt_cache"
    AffinityClientSession     AffinityKind = "client_session"
    AffinityProviderResponse  AffinityKind = "provider_response"
    AffinityProviderConversation AffinityKind = "provider_conversation"
    AffinityProviderOperation AffinityKind = "provider_operation"
)

type AffinityStrictness string

const (
    AffinitySoft   AffinityStrictness = "soft"
    AffinityStrict AffinityStrictness = "strict"
)

type AffinitySignal struct {
    Kind       AffinityKind
    Strictness AffinityStrictness
    Source     string
    Value      string // request-scoped only; never log or persist raw
}

type RequestRoutingContext struct {
    Protocol         ProtocolID
    ProviderID       ProviderID
    CanonicalModel   string
    Operation        OperationType
    Affinities       []AffinitySignal
    ReplayCapability ReplayCapability
}
```

One request may contain several signals. A strict provider-state owner controls selection; a cache key remains a secondary soft hint. Signals are not collapsed into the current untyped `conversationID` string.

## Affinity semantics

| Source | Meaning | Default routing behavior |
|---|---|---|
| Gateway async operation ID | Existing upstream operation ownership | Strict exact connection |
| Provider conversation reference | Provider-managed conversation state | Strict exact connection |
| `previous_response_id` | Prior provider response state | Strict exact connection |
| Documented client conversation/session ID | Client grouping, not necessarily provider state | Soft preferred connection |
| `prompt_cache_key` where supported | Provider cache-routing hint | Soft preferred connection |
| Anthropic `cache_control` alone | Cache boundary declaration, no unique identity | Preserve only; no binding key |
| Unknown field or metadata | Unknown | No routing meaning |

Strict state must never be retried on another connection unless a protocol adapter can prove and explicitly perform a complete, safe replay. Expensive tool or media submissions also require their own exactly-once idempotency proof.

Soft affinity may be broken for:

- disabled, dead, incompatible, or verification-required connection state;
- cooldown or hard quota exclusion;
- reserve protection;
- an explicit exceptional policy such as cyber-only retry;
- sustained, materially worse normalized pressure when otherwise-expiring capacity justifies the expected cache loss.

Rebinding uses hysteresis and a minimum residence period so telemetry jitter does not make a session oscillate.

## Privacy and isolation

The internal key is derived as:

```text
HMAC(gateway affinity secret,
     gateway_user_id || provider_id || canonical_model || protocol ||
     affinity_kind || client_value)
```

Requirements:

- never log or persist the raw client value;
- never include raw prompt content;
- namespace by authenticated GatewayUser to prevent cross-user coupling;
- use canonical provider/model identities after route resolution;
- cap active bindings per user and globally;
- use sliding inactivity expiry plus an absolute maximum lifetime;
- remove or quarantine bindings when a connection is deleted;
- coordinate atomic get-or-assign in a multi-replica deployment;
- treat the HMAC secret as runtime security material with an explicit rotation policy.

An in-memory index is acceptable only for the initial single-process Test slice. Production durability/coordination must be decided before promotion; silent replica-local disagreement is not acceptable.

## Economic scheduling objective

The objective is to minimize:

```text
unused paid capacity at reset
+ avoidable uncached repeated input
+ external/overflow spend
+ provider failures and retry work
+ latency penalty
```

### Capacity estimate

For each connection and provider quota window, maintain:

- provider-reported used percentage and reset time;
- whether the observation is known and fresh;
- measured canonical tokens per observed percentage change;
- local provisional tokens not yet reflected by provider telemetry;
- in-flight estimated work;
- configured reserve;
- confidence/sample count for the capacity estimate.

Conceptually:

```text
remaining_effective_tokens(window) =
    (1 - reported_used_fraction - provisional_fraction) * learned_capacity
    - reserve_tokens

required_burn_rate(window) =
    max(0, remaining_effective_tokens) / max(time_to_reset, minimum_horizon)
```

The short and long Codex windows are evaluated together. A connection cannot consume weekly capacity faster than its short window safely allows. Unknown telemetry remains unknown and receives bounded exploration rather than being treated as zero, unlimited, or preferred.

No fixed Plus/Pro multiplier is encoded. Capacity is learned per connection from canonical usage and observed quota deltas, with robust handling for quantized percentages, resets, stale samples, outliers, and model mix.

### New work

New, unbound sessions and independent requests are the primary balancing mechanism. Eligible connections compete using reset urgency, normalized depletion pressure, provisional use, health, in-flight work, penalty, and reserve state. Deterministic rotation applies only among candidates inside a competitive score window.

A Pro connection receives more work only when measured remaining capacity and reset urgency support it. A Plus connection receives work when its paid allowance would otherwise expire unused.

### Existing soft affinity

A healthy binding is retained when its projected economic cost remains within a configured hysteresis band of the best alternative. Cache history can raise the cost of moving a binding:

```text
estimated_move_cost =
    expected_lost_cache_read_tokens * destination_token_value
```

The first release need not make an automated monetary comparison. It must first expose the required evidence and use a conservative bounded-affinity policy. Economic migration can be enabled only after Production observations are trustworthy.

### Provisional debit

Provider percentages can be stale or quantized. On terminal canonical usage for a request, debit the selected connection provisionally:

```text
provisional_fraction = canonical_effective_tokens / learned_tokens_per_fraction
```

Selection sees reported use plus unacknowledged provisional use. On a fresh quota observation:

- identify a reset before reconciliation;
- acknowledge only provisional work plausibly represented by the new observation;
- retain newer/unacknowledged work;
- update capacity estimates only from valid observation intervals;
- never convert a stale unchanged percentage into fresh free capacity.

The canonical request remains attributed wholly to one connection. Provisional scheduling state is not a second accounting ledger and never changes exactly-once usage.

## Current implementation findings

The current code is not yet this design:

- `main.go` broadly searches body fields, metadata, and several headers and collapses them into one `conversationID`;
- body-level `prompt_cache_key` is preserved by Responses translation but is not extracted by the body conversation helper;
- `metadata.user_id` can currently become a conversation ID even though attribution metadata does not prove session semantics;
- successful responses can create a pool pin, but the pin has no typed source, user/provider/model namespace, TTL, or privacy-safe key;
- Codex pins currently reject non-Pro/Prolite connections, so Plus cannot retain ordinary affinity;
- Codex `accountTier` places Pro/Prolite ahead of Plus, which can drain the highest plan while other paid capacity expires;
- the competitive selector rotates only after tier and eligibility policy have already narrowed the pool;
- provider telemetry has no local provisional debit in selection;
- WebSocket cyber swap currently strips `previous_response_id` to replay on another account. This behavior must be retained only under an explicit, tested replay contract; otherwise strict state must fail typed rather than being silently weakened;
- `store=false` is forced on Codex Responses translations, so state-continuation capabilities must be declared and tested rather than inferred;
- canonical usage already preserves cached-input dimensions and connection attribution, providing the measurement foundation.

These are migration findings, not permission to broaden heuristics.

## Implementation sequence

### Slice 0 — Freeze contracts and capture evidence

1. Add protocol fixtures from supported Pi, Codex CLI, and Claude Code request shapes without secrets or prompt content.
2. Record which route and protocol version produced each fixture.
3. Add tests proving recognized fields are forwarded unchanged through no-translation and translation paths.
4. Add negative tests proving unknown fields, `metadata.user_id`, and lookalike headers do not influence routing.
5. Document provider capabilities for prompt cache hints, cache markers, response state, conversation state, stateless replay, and cache usage reporting.

Exit: every routing-relevant affinity source has a protocol or registered-extension contract; broad heuristic behavior is characterized before removal.

### Slice 1 — Typed request routing context

1. Add protocol-owned affinity extractors for OpenAI Responses, OpenAI Chat, Anthropic Messages, and supported client profiles.
2. Resolve canonical model/provider before namespacing affinity.
3. Return typed soft and strict signals plus replay capability.
4. Pass `RequestRoutingContext` through HTTP, large-body, and WebSocket paths.
5. Remove scheduler dependence on generic JSON/header scanning.

Exit: selection consumes typed context only; unknown request content cannot create a pin.

### Slice 2 — Safe soft-affinity store

1. Add HMAC namespacing and no-raw-value logging tests.
2. Implement atomic get-or-assign, touch, break, rebind, expiry, and connection-removal behavior.
3. Bind prompt-cache and registered client-session signals softly.
4. Permit Plus, Pro, and Prolite bindings under the same eligibility rules.
5. Add bounded residence/hysteresis and explicit break reasons.
6. Decide and implement restart/multi-replica coordination before Production promotion.

Exit: repeated recognized sessions prefer one healthy connection without allowing cross-user coupling or permanent unsafe pins.

### Slice 3 — Strict provider-state ownership

1. Capture response/conversation identifiers emitted by the gateway-supported upstream contract.
2. Bind them to provider, connection, and protocol namespace.
3. Require exact owner selection for continuations.
4. Return typed `state_affinity_unavailable`, `state_reference_unknown`, or capability errors as appropriate.
5. Audit every retry, cyber swap, WebSocket reconnect, and translation path for silent state removal or cross-account replay.

Exit: provider state is never sent to or stripped for another account without an explicit replay proof.

### Slice 4 — All-eligible Codex capacity pool

1. Remove ordinary Codex Pro/Prolite Tier 1 versus Plus Tier 2 preference.
2. Preserve required-plan routes as explicit constraints, not ordinary preference.
3. Preserve exact-ID, image, model capability, health, cooldown, IP, retry exclusion, reserve, and cyber-policy constraints.
4. Introduce reset-aware normalized pressure and urgency for both quota windows.
5. Rotate deterministically among genuinely competitive candidates.
6. Give unknown-capacity connections bounded exploration and confidence tracking.

Exit: five Plus and one Pro can all receive ordinary eligible work according to reset-aware measured capacity; no tier must exhaust before another participates.

### Slice 5 — Provisional quota debits

1. Define the canonical effective-token input without changing persisted usage semantics.
2. Apply one request/turn debit to its selected connection after terminal accounting.
3. Include in-flight reservations to avoid concurrent burst concentration.
4. Reconcile against fresh provider observations and reset detection.
5. Expose estimate age, confidence, provisional amount, and reconciliation health.

Exit: unchanged or quantized provider percentages do not concentrate a burst on one connection.

### Slice 6 — Evidence-driven affinity economics

1. Measure affinity retain/miss/break/rebind counts by typed reason.
2. Measure cache-read, cache-creation, and uncached input by connection/model and affinity cohort.
3. Measure projected unused capacity at reset and actual reset waste.
4. Start with conservative policy; enable economic affinity breaks only after offline replay and Staging evidence.
5. Keep Admin UI/backend projections aggregate and privacy-safe.

Exit: policy changes demonstrate lower reset waste without materially increasing uncached repeated input or failures.

## Test matrix

### Protocol contracts

- OpenAI Responses cache key, previous response, and conversation fields;
- OpenAI Chat fields only where the pinned contract supports them;
- Anthropic `cache_control` preserved with no invented affinity key;
- registered Pi/Codex CLI/Claude Code extensions;
- unknown and lookalike fields ignored for routing;
- translated fields preserved only through documented mappings;
- streaming, non-streaming, large-body, and WebSocket parity.

### Affinity

- same user/provider/model/protocol/key retains a connection;
- same raw key from different users does not share a binding;
- same key across providers/models/kinds does not collide;
- concurrent first requests atomically select one winner;
- cooldown, disable, death, capability mismatch, hard quota, reserve, and deletion break safely;
- no oscillation inside hysteresis/minimum residence;
- expiry and bounded-memory cleanup;
- restart and multi-replica behavior;
- strict state never crosses a connection;
- retries preserve exactly-once usage.

### Scheduler

- mixed five-Plus/one-Pro capacity;
- unequal learned capacities without plan multipliers;
- primary and secondary resets at different times;
- near-reset underuse urgency;
- stale and quantized telemetry;
- provisional debit acknowledgement and reset reconciliation;
- unknown quota and low-confidence estimates;
- deterministic fairness among competitive candidates;
- concurrent in-flight reservations;
- cooldown, penalties, required plans, image/model capabilities, and explicit reserves;
- `cyber_access` isolation and cyber-only retry behavior;
- one request remains entirely attributed to one connection.

### Economic acceptance

Compare current policy and candidate policy using the same anonymized canonical workload:

- projected and observed unused quota at reset;
- cache-read ratio and uncached repeated input;
- total canonical billable/effective tokens;
- request success, typed failures, retries, and latency;
- per-connection normalized pressure spread;
- affinity break frequency and reason;
- overflow/external spend if applicable.

A candidate fails if it makes percentages look balanced while increasing total economic waste.

## Rollout and promotion

1. Land extraction and observability with selection behavior unchanged.
2. Run offline replay against the current six-connection Production shape.
3. Enable soft affinity in Test behind a backend feature flag.
4. Enable all-eligible reset-aware selection in Test; retain immediate rollback to the old selector.
5. Add provisional debits after deterministic concurrency tests pass.
6. Build one immutable image and promote Test to real-auth Staging.
7. Validate cache and reset metrics during Staging soak using isolated browser/auth state.
8. Do not count Staging as production-readiness evidence until the Production authority/outbox boundary makes provider lifecycle and canonical global usage authoritative.
9. Promote the exact accepted image Staging to Production through the documented maintenance path.
10. Keep old policy configuration and binding-state migration/clear behavior available for bounded rollback.

No fourth runtime is created. Frontend and Go builds remain sequential because Go embeds `web/dist`.

## Required observability

Expose backend-owned, privacy-safe fields and metrics:

- affinity kind and source (never raw value);
- retained, assigned, broken, rebound, expired;
- break reason and prior/new connection public identifiers;
- strict-state owner missing/unavailable;
- reported/provisional/effective quota pressure;
- quota observation age and capacity confidence;
- reset urgency and projected unused capacity;
- cache-read/cache-creation/uncached-input totals by safe aggregate cohort;
- outlier/reconciliation/reset detection counts;
- scheduler policy revision and decision reason.

Debug traces must not contain prompts, raw cache keys, access tokens, provider response contents, or sensitive upstream identifiers.

## Definition of done

This work is complete when:

- routing interprets only protocol-declared or registered-extension fields;
- recognized client cache instructions are forwarded unchanged and never synthesized;
- soft affinity is privacy-safe, bounded, atomic, and available to Plus/Pro/Prolite;
- strict provider state cannot silently cross accounts;
- ordinary Codex routing uses one all-eligible reset-aware capacity pool;
- local provisional use prevents stale-telemetry bursts;
- exactly-once accounting remains unchanged and reconciled;
- Production evidence shows less paid capacity expiring unused without an unacceptable cache-read regression;
- Test → Staging → Production promotion and rollback evidence are complete.
