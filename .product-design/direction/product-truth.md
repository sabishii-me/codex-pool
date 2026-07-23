# Codex Pool product truth

Status: draft for Step 0 owner review

## Product

Codex Pool is a private multi-provider AI model gateway. Members configure supported AI clients once and use stable public model route IDs while the gateway selects eligible upstream provider connections, handles protocol translation, and records usage exactly once. Operators maintain upstream capacity, routing, access, and system health.

The product is not an AI chat client, a provider billing portal, or a decorative network-operations visualization. It is a gateway administration and usage product whose default experience must make routine access understandable without exposing unnecessary infrastructure detail.

## Canonical object hierarchy

```text
GatewayUser
  sends requests through ModelRoutes

Provider
  owns one or more ProviderConnections

ProviderConnection
  contributes credentialed upstream capacity

ModelRoute
  maps a public model ID to provider behavior

UsageEvent
  records one canonical terminal request outcome
```

Use **member** or **user** for a gateway identity, **provider** for an upstream service, **provider connection** for a credentialed capacity source, and **model route** for the public-to-upstream mapping. Do not use “account” as the generic connection label.

## Primary member jobs

1. Confirm that the gateway is usable now.
2. Configure a supported client through a short copy-and-verify path.
3. Find an available model and copy its route ID.
4. Understand personal recent usage.
5. Identify any action personally required.

Members should not need to understand provider credentials, pool economics, quota inference, or routing policy to complete these jobs.

## Primary operator jobs

1. Detect availability, capacity, persistence, or routing incidents.
2. Identify the provider connection or route causing an incident.
3. Add, validate, label, enable, disable, test, rename, or remove a provider connection.
4. Understand quota windows, cooldowns, recovery timing, and routing compensation.
5. Inspect pool-wide usage and economics with evidence quality clearly labeled.
6. Administer members, authentication recovery, and system state safely.

## Information architecture

### Member workspace

- Dashboard
- Models
- Setup
- My usage
- Profile

### Operator workspace

- Overview
- Monitor
- Provider connections
- Model routes
- Usage and economics
- Members
- System

Operator visibility is supplied by authenticated backend policy in the product. The Step 0 `runtime=operator` query simulates presentation only and grants no authority.

## Dashboard boundary

The front page is a concise overview. It answers:

1. Is the gateway operational?
2. Can this member use it now?
3. What recent personal activity matters?
4. Which models/providers are available?
5. Does anything require action?

It contains a status summary, a restrained metric set with explicit scope, one primary activity trend, provider-level health, actionable incidents, and common next actions. It does not contain raw connection management, detailed routing diagnostics, complete setup flows, quota methodology, and economic analysis simultaneously.

## Monitor boundary

Monitor is a denser operator diagnostic workspace for request activity, latency, errors, provider/connection availability, quotas and cooldowns, route eligibility, usage persistence, and operational events. Monitoring observes and explains. Configuration remains in explicit administration destinations.

## Interaction architecture

- Major destinations have stable URLs in the production implementation.
- One application shell provides navigation, environment identity, freshness, and member controls.
- Member and operator destinations are visually grouped rather than mixed as peers.
- One primary action is emphasized per page or dialog.
- Lists support search/filtering and responsive detail disclosure.
- Resource failures are local to their panel or query and retain last-known-good data where safe.
- Unknown, zero, stale, unavailable, measured, and inferred are distinct states.
- Destructive actions explain consequences and require confirmation.

## Persistence and authority boundaries

The Step 0 artifact is fictional and in-memory. Reloading resets it. It performs no network, credential, provider, filesystem, authentication, or persistence operation.

In the real product:

- SQLite canonical usage events are authoritative for accounting.
- Backend access policy determines member/operator authority.
- Backend protocol and read-model layers own provider accounting, health, quota, evidence, and routing semantics.
- React formats and presents normalized projections; it does not infer provider policy.
- Theme selection changes presentation only.

## Responsive priorities

- At 1440px, show a persistent sidebar and a readable two-column dashboard hierarchy.
- At 820px portrait, retain clear workspace grouping while reducing simultaneous secondary content.
- At 1180×820 landscape tablet, assume secondary-screen monitoring: show one dominant live graph, freshness, and a small current-state strip; member scope is personal usage and operator scope is pool health.
- At 390px portrait, use the same focused composition as landscape tablet—freshness, four readouts, and one dominant graph. Operator scope starts in Monitor; member scope starts with personal gateway usage.
- At 844×390 landscape mobile, preserve that same role-scoped focus hierarchy in a single screen above fixed navigation; broad diagnostics and administration remain behind navigation.
- Wide comparison tables must have disclosure/list alternatives on narrow screens.

## Visual direction

Step 0 uses a dark, data-first default inspired by the supplied dashboard reference. The reference is design input only and is not loaded by the artifact.

Default palette:

| Token role | Name | Value |
|---|---|---|
| Canvas/deepest background | Black Hole | `#020203` |
| Primary surface/sidebar layer | Gluon Grey | `#1a191c` |
| Elevated surface/strong border | Avocado Peel | `#39373d` |
| Muted structural information | Granite Canyon | `#6c6e79` |
| Secondary emphasis/comparison/warning | Morning Tea | `#c7bc92` |
| Primary text/action/data series | Pale Phthalo Blue | `#cad2fd` |

Visual rules:

- preserve large near-black negative space instead of filling every region with cards;
- use charcoal elevation and low-contrast borders for structure;
- reserve pale blue for primary data, text, active navigation, and primary actions;
- reserve tea for warnings, comparison series, and secondary emphasis;
- keep charts sparse and high contrast, especially in mobile/tablet focus modes;
- do not use decorative remote imagery or reproduce the reference asset in the product;
- retain text labels and shapes for semantic state so palette does not become the only status signal.

The accepted interaction architecture may later support additional themes, including a more explicitly heraldic Signal Room variant. Every theme must retain the same routes, components, semantics, contrast obligations, and tests.

## Forbidden product claims

The artifact must not imply that it:

- is connected to live production, staging, or development data;
- can authorize an operator or validate credentials;
- can perform real provider or model operations;
- knows real user identity, quotas, spend, or incidents;
- persists preferences, comments, or workflow changes;
- represents final approved visual design;
- freezes the Phase 9 direction before explicit owner acceptance.
