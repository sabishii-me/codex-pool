# UI/UX Review and Redesign Direction

Status: proposed

Related roadmap: [`architecture-roadmap.md`](architecture-roadmap.md)

## Scope

This review covers information architecture, terminology, workflows, data presentation, frontend maintainability, resilience, accessibility, and the future backend boundary. It does not prescribe a final visual comp. The black/gold private-console identity can remain, but it should support clearer product concepts and workflows.

## Executive summary

The current UI is visually distinctive and functionally broad, but it asks every signed-in person to understand the gateway as an operator does. User setup, provider administration, connection health, pool economics, capacity forecasting, and personal identity share one navigation and one data model.

The main redesign goal is:

> Show each person the smallest truthful interface for their role and task, using the future `GatewayUser → Provider → ProviderConnection → ModelRoute` domain model.

The UI should consume normalized backend projections. It should not calculate provider-specific token semantics, subscription economics, connection state, or routing recommendations in React.

## Current strengths

- Strong and recognizable visual identity.
- Useful setup commands with copy affordances.
- Real attention to keyboard labels, reduced motion, empty states, and operator elevation.
- Broad operational visibility in one place.
- Model search and provider filtering are useful foundations.
- Credential validation happens before a connection joins rotation.
- Authenticated and signed-out smoke coverage exists.

## Primary UX problems

### 1. User and operator experiences are mixed

Every signed-in user sees navigation for pool-wide Pulse, Insights, Usage, Accounts, Models, and Setup. Most people primarily need:

1. Is the gateway usable?
2. Which models can I use now?
3. How do I configure my tool?
4. What did I personally consume?

Operators additionally need provider connections, routing, quota, failures, users, and economics. Those are different jobs and should not compete in one navigation hierarchy.

### 2. Information architecture overlaps

- **Pulse** includes economics, burn, providers, intervention, token composition, and origin drain.
- **Insights** contains overview, capacity, flow, and demand.
- **Usage** repeats burn, token composition, provider economics, and origin drain.
- **Accounts** mixes public connection health, economics, credential contribution, and destructive operator controls.

Users must remember where a metric appears rather than follow a task-oriented structure.

### 3. Terminology is overloaded or theatrical where precision is needed

Examples include:

- Account
- Contribute an account
- Burn
- Extraction
- Provider capital
- Intervention queue
- Cooked
- Leaning hard
- Signal interrupted
- Account equivalents

The voice is memorable, but live infrastructure and billing information need plain-language labels first. Brand language should be secondary copy, not the only explanation.

### 4. The upstream capacity object is mislabeled

The current **Accounts** screen represents upstream credentials and subscriptions. After gateway login, “account” naturally means the signed-in user's identity.

Recommended language:

| Current | Recommended |
|---|---|
| Account | Provider connection |
| Accounts | Connections |
| Contribute account | Connect provider |
| Account pool | Provider pool |
| Account status | Connection status |
| Pool users | Users or members |

### 5. Provider addition is hardcoded into the frontend

Provider names, colors, contribution modes, grouping, hints, and API unions are compiled into React. A runtime provider cannot appear without frontend changes.

The future data API should return provider display metadata and a connection form schema. The UI renders the form instead of containing a provider switch.

### 6. One failed request degrades the whole dashboard

The root refresh uses `Promise.all` for stats, signal analytics, and model catalog. A transient failure in subscription economics produces a global error even when models and connection health are still available.

Each resource and panel needs independent:

- loading state;
- last successful value;
- freshness timestamp;
- retry action;
- unavailable explanation.

Stale valid data is generally more useful than replacing an entire screen with a global failure state.

### 7. React contains accounting policy

Functions such as `tokenThroughput()` and `accountThroughput()` decide how cache tokens count based on provider identity. This has already produced misleading values.

The backend should return explicit fields such as:

```text
processed_tokens
uncached_input_tokens
cache_read_tokens
cache_write_tokens
output_tokens
estimated_cost
```

The frontend should not branch on provider to calculate BURN.

### 8. Estimated and measured values look equally authoritative

API-equivalent value, subscription spend, quota capacity, reset behavior, and routing recommendations have different evidence quality. Some are measured, some inferred, and some unknown.

Every derived metric should include:

- status: measured, estimated, inferred, unavailable;
- confidence;
- observation period;
- last updated time;
- concise methodology access.

A displayed `$0` must not mean “unknown.”

### 9. Setup is useful but handles too much sensitive data at once

The session payload hydrates multiple credentials and full configuration blobs into frontend memory even when the user never opens Setup.

The redesign should request setup material just in time per tool, provide scoped/rotatable credentials, and avoid returning unrelated secrets with session identity.

### 10. Navigation is not durable

The selected view is component state rather than a URL. Refresh, deep links, browser navigation, and sharing return to the default view. Insight subviews also lack routes.

Use real routes and preserve filters in the URL where useful.

### 11. Dense tables do not adapt well

The connection table is intentionally wide and becomes an 860px horizontal surface on mobile. Small uppercase labels and low-contrast tertiary text increase scanning cost.

Mobile should use prioritized connection summaries with expandable details, not a desktop table inside horizontal scrolling.

### 12. Frontend maintainability mirrors backend coupling

- `App.tsx` is approximately 112 KB and contains most views, workflows, calculations, and dialogs.
- `styles.css` is approximately 68 KB.
- API DTOs are handwritten and mirror backend structs.
- Provider metadata is duplicated.
- Dashboard calculations and view rendering are colocated.

This makes conceptual changes expensive and encourages more conditional logic in the root component.

## Proposed information architecture

### Standard user

```text
Overview
Models
My usage
Setup
Profile
```

#### Overview

Answer only:

- Gateway status
- Models available now
- Personal usage summary
- Active incidents or limitations
- Recommended next action

For a first-time user, the primary action should be **Set up a client**, not inspect pool economics.

#### Models

- Searchable model catalog
- Availability
- Capabilities
- Context/output limits
- Copy routing ID
- “Use in…” shortcuts into Setup

#### My usage

- Personal requests and normalized token composition
- Model mix
- Trends
- Optional cost/value if meaningful to the user

Pool-wide origin analysis should not be mixed into personal usage.

#### Setup

- Choose tool
- Choose operating system
- Copy/run one recommended command
- Verify connection
- Reveal advanced/manual configuration on demand
- Rotate/revoke the issued gateway credential

#### Profile

- Identity
- Sessions
- Credentials issued to this user
- Security and sign out

### Operator workspace

Expose an explicit **Admin** section only to administrators:

```text
Admin
  Overview
  Providers
  Connections
  Routing
  Usage and economics
  Users
  System
```

#### Admin overview

- Availability incidents
- Connections requiring action
- Capacity risk
- Persistence/data health
- Recent upstream failures

#### Providers

One row per provider definition:

- Protocol
- Models
- Regions
- Aggregate availability
- Number of connections

#### Connections

One row per `ProviderConnection`:

- Provider and region
- Credential label, never raw secret
- Health
- Quota
- Last successful request
- Recent error
- Subscription
- Enable/disable/test/remove actions

#### Routing

- Public model route
- Upstream provider/model
- Eligible connections
- Routing policy
- Fallbacks

#### Usage and economics

Keep advanced capacity, flow, demand, quota inference, and economics here. Organize around explicit questions rather than instrument codes.

## Recommended connection workflow

Replace “Contribute an account” with **Connect a provider**.

1. Select provider.
2. Select region/product where applicable.
3. Follow provider-specific credential instructions supplied by the backend schema.
4. Enter or authorize credential.
5. Validate credential and endpoint.
6. Show discovered identity/models/limits before activation.
7. Assign a human-readable connection label.
8. Activate connection.

The backend should provide a schema similar to:

```json
{
  "provider_id": "kimi-platform",
  "display_name": "Kimi Open Platform",
  "fields": [
    { "id": "region", "type": "select", "options": ["global", "china"] },
    { "id": "api_key", "type": "secret" }
  ],
  "help": "Use the platform where this key was created."
}
```

No provider-specific form switch should be required in React.

## Content and language direction

Preserve the private-console voice in:

- login;
- section codes;
- subtle labels;
- empty-state personality;
- decorative details.

Use plain language for:

- state;
- errors;
- quota;
- billing;
- destructive actions;
- authentication;
- setup instructions.

Examples:

| Current | Clear primary label | Optional flavor |
|---|---|---|
| SIGNAL INTERRUPTED | Analytics unavailable | Signal interrupted |
| COOKED | Disabled or authentication failed | — |
| BURN | Processed tokens | Burn rate |
| EXTRACTION | Value-to-spend ratio | Extraction |
| INTERVENTION QUEUE | Connections requiring attention | Intervention queue |

## Data presentation rules

1. Unknown is distinct from zero.
2. Stale is distinct from unavailable.
3. Provider-reported quota is distinct from inferred capacity.
4. Personal usage is distinct from pool-wide usage.
5. Processed tokens are distinct from priced/billable usage.
6. Connection health is distinct from model availability.
7. Every derived metric exposes its observation window and confidence.
8. Destructive controls include a clear consequence and confirmation.

## Error and loading model

Use resource-level states rather than one application error string:

```text
session
provider summary
connections
model catalog
usage summary
analytics
```

Each query retains its last successful data. The page header can show overall freshness and incident count without hiding healthy panels.

## Frontend structure

Suggested layout:

```text
src/
  app/
    router.tsx
    shell.tsx
    providers.tsx

  features/
    overview/
    models/
    usage/
    setup/
    profile/
    admin/providers/
    admin/connections/
    admin/routing/
    admin/analytics/
    admin/users/
    admin/system/

  components/
    status/
    data-table/
    charts/
    forms/
    feedback/

  data/
    client.ts
    queries.ts
    generated-types.ts

  domain/
    presentation.ts
    formatting.ts
```

Use a router and a query/cache layer. Provider presentation metadata should come from the backend with a restrained frontend fallback for unknown providers.

## Responsive direction

- Preserve the desktop console shell.
- Replace wide mobile tables with summary cards or disclosure rows.
- Keep primary actions visible without horizontal scrolling.
- Reduce simultaneous metrics on narrow screens.
- Ensure popovers and reset details work with touch, focus, and escape.
- Keep operational text above approximately 12px equivalent and improve contrast for live data.

## Accessibility priorities

- Use semantic tables where data is tabular; avoid representing every row as one large button.
- Provide row action menus and separate detail links.
- Ensure dialogs trap focus, close on Escape, and restore focus.
- Avoid hover-only reset-credit information.
- Expose chart summaries in text or tables.
- Test keyboard-only setup, provider connection, MFA, and destructive controls.
- Add automated accessibility checks in CI.

## Redesign roadmap

### UX Phase 0 — Research and task definition

- Identify standard-user and operator jobs.
- Inventory current screens and metrics.
- Decide which metrics are trustworthy after usage normalization.
- Define the new domain vocabulary with backend owners.

Deliverable: task map and approved terminology.

### UX Phase 1 — Information architecture and low-fidelity flows

- Define standard and admin navigation.
- Wireframe Overview, Models, Setup, Provider, and Connections.
- Design loading, empty, stale, unknown, and partial failure states.

Deliverable: clickable low-fidelity flow without visual polish.

### UX Phase 2 — Data contracts

- Define `/api/v2` view models.
- Remove provider accounting calculations from React.
- Add backend-supplied provider presentation and connection-form schemas.

Deliverable: versioned UI data contract with fixtures.

### UX Phase 3 — Frontend foundation

- Add routing and query caching.
- Split `App.tsx` by feature.
- Introduce shared status, table, dialog, and form components.
- Generate or validate API types.

Deliverable: new shell running against fixtures and compatibility APIs.

### UX Phase 4 — Core user experience

Implement:

1. Overview
2. Models
3. Setup
4. My usage
5. Profile

Deliverable: complete standard-user path.

### UX Phase 5 — Operator workspace

Implement:

1. Providers
2. Connections
3. Routing
4. Usage/economics
5. Users
6. System health

Deliverable: operator workflows without exposing raw internal structs.

### UX Phase 6 — Validation

- Desktop and mobile task tests
- Keyboard and screen-reader checks
- Visual regression tests
- Contract tests with unknown/dynamic providers
- Partial backend failure tests
- First-time setup usability test

Deliverable: measured completion and error rates for primary workflows.

## Immediate low-risk improvements

These can be made before the full redesign if desired:

1. Rename user-facing Accounts to Connections and contribution to Connect provider.
2. Change global refresh from `Promise.all` failure behavior to independent resource states.
3. Add URLs for current views.
4. Remove provider-specific token calculations from React once normalized backend fields exist.
5. Distinguish unknown from `$0` and measured from estimated.
6. Move pool-wide analytics behind the administrator workspace.
7. Split `App.tsx` into feature modules without changing visuals.

## Redesign success criteria

A standard user should be able to:

- identify whether the gateway is usable in under ten seconds;
- find an available model and copy its routing ID;
- configure a supported client without understanding provider connections;
- view personal usage without seeing operator-only infrastructure concepts.

An operator should be able to:

- identify the connection causing an incident;
- connect a provider with the correct product and region;
- understand whether data is measured, inferred, stale, or unavailable;
- inspect routing and usage without consulting source code;
- perform connection actions with clear consequences.
