# Production Frontend Product Architecture

Status: **approved source of truth**

Approved: 2026-07-24

Supersedes the route, workspace, and page inventory in:

- `.product-design/direction/frozen-design-prompt.md`
- `.product-design/direction/product-truth.md`
- `docs/ui-redesign-plan.md`
- `docs/ui-implementation-handoff.md`
- `docs/ui-ux-review.md`

Those documents remain historical design/discovery records. Their separate member/operator workspace architecture and `/operator/*` routes are not implementation constraints.

## Decision

Codex Pool has one authenticated product experience. An administrator is a member with additional capabilities, not a separate user type or workspace.

Canonical capability progression:

```text
Signed out
  -> Member
       -> Admin identity
            -> MFA-elevated admin capabilities
```

Canonical language is **Admin**, not Operator. The frontend and product copy must not introduce an Operator role distinct from Admin.

This application is still in development. Do not implement compatibility routes, redirects, aliases, fallback pages, or adapters for the discarded information architecture. Delete wrong development routes and components instead of wiring them forward.

## Navigation

### Member

```text
Home       /
Models     /models
Usage      /usage
Setup      /setup
Profile    /profile
```

Setup is frozen for a separate replan. Do not alter its content, status model, workflow, or route during the information-architecture correction.

### Admin identity before elevation

The admin inherits the complete member experience. Administrative mutation controls remain locked until backend-authorized MFA elevation. The product may show one clear **Unlock admin controls** action; it must not show a duplicate workspace.

### Elevated admin

The same navigation gains only resources that do not exist for members:

```text
Connections   /admin/connections
Members       /admin/members
System        /admin/system
```

Admin context augments shared resources:

- Home gains operational attention and incident summaries.
- Models gains routing context for a selected model.
- Usage gains pool/member scopes and economics.
- Profile gains admin security, MFA, elevation, and recovery controls.

## Explicitly rejected routes

Do not create or retain:

```text
/operator
/operator/monitor
/operator/connections
/operator/routes
/operator/usage
/operator/members
/operator/system
/admin/routes
/admin/usage
/admin/monitor
```

There are no compatibility redirects for these unfinished development routes.

## Resource ownership

### Home `/`

One concise orientation and attention surface.

Common member content:

- gateway usability;
- available-model summary;
- canonical personal-usage summary when available;
- personal action required.

Admin augmentation:

- degraded connections;
- constrained models/routes;
- runtime or projection failures;
- links into the owning resource.

Home summarizes and links. It does not duplicate complete Models, Usage, Connections, or System content.

### Models `/models`

One model inventory for every member.

Common content:

- public model identity and route ID;
- provider;
- capabilities and limits;
- member-facing availability and explanation.

Admin-only selected-model context:

- upstream provider/model mapping;
- eligible provider connections;
- selected connection and fallback order;
- health, quota, cooldown, or policy exclusions;
- unavailable reasons and projection freshness.

Routing is context on a selected model, not another route inventory. Durable state may use query parameters such as:

```text
/models?model=gpt-5.6-sol
/models?model=gpt-5.6-sol&view=routing
```

### Usage `/usage`

One usage explorer with backend-authorized scope.

Member default:

```text
scope=me
```

Admin gains:

```text
scope=me
scope=pool
scope=member&member=<id>
```

Pool scope may expose an Economics section. Economics is context over pool usage, not another top-level page.

Usage owns historical activity, token composition, model/provider/member breakdowns, date range, comparison, economics, evidence, and freshness.

Never select an arbitrary origin and call it personal usage. If canonical personal usage is absent, render an explicit unavailable state.

### Connections `/admin/connections`

Admin-only resource for credentialed upstream capacity:

- provider connection identity;
- provider and plan;
- enabled, disabled, dead, health, refresh, and inflight state;
- quota windows and recovery timing;
- recent errors;
- validate, rename, test, enable, disable, and remove operations.

Connections does not recreate Models or Usage.

### Members `/admin/members`

Admin-only gateway-user administration:

- identity and access state;
- plan and role;
- last activity and concise usage summary;
- enable, disable, recovery, and security actions.

Detailed member usage deep-links into `/usage?scope=member&member=<id>` rather than creating a second usage dashboard.

### System `/admin/system`

Admin-only platform state not owned by Models, Usage, Connections, or Members:

- gateway runtime and build;
- persistence health;
- projection freshness;
- background jobs;
- provider-spec/config revision;
- recovery readiness;
- administrative security state;
- recent system-level failures.

`/healthz` alone is insufficient to claim a complete System page. Missing projections remain unavailable rather than fabricated.

### Profile `/profile`

One identity/security surface. Admin capability adds MFA enrollment, elevation, and recovery controls to the same page.

### Setup `/setup`

No changes. Replan separately.

## Monitor decision

There is no top-level Monitor destination in the current plan. The previous Monitor concept overlaps with Home, Usage, Connections, Models routing context, and System.

Current ownership:

| Previously proposed monitor content | Owner |
|---|---|
| Active incidents | Home admin attention |
| Historical request activity | Usage |
| Provider pressure and cooldown | Connections |
| Route eligibility and fallback | Selected Model -> Routing |
| Runtime/persistence/projection health | System |
| Latency/error live telemetry | System live context when a real contract exists |

A distinct Monitor page may be proposed later only if real-time telemetry and incident workflows form a unique job and contract. It is not reserved now.

## Route admission test

A new top-level destination requires all three:

1. A unique resource not owned elsewhere.
2. A unique primary user job.
3. A distinct backend projection or workflow—not the same payload under a different heading.

If any condition fails, use scope, detail, disclosure, tab, filter, or query state on the existing resource.

## Data contracts required

Frontend work must not reconstruct missing semantics.

```text
GET /api/v2/home
GET /api/v2/models
GET /api/v2/models/:id/routing              admin
GET /api/v2/usage?scope=me
GET /api/v2/usage?scope=pool                admin
GET /api/v2/usage?scope=member&member_id=   admin
GET /api/v2/usage/economics?scope=pool      admin
GET /api/v2/provider-connections            admin
GET /api/v2/members                         admin
GET /api/v2/system                          admin
```

Every projection must preserve:

- authenticated scope and subject;
- measured, estimated, inferred, stale, or unavailable evidence;
- source and freshness timestamps;
- zero distinct from missing;
- localized partial failures;
- backend authorization.

## Current checkpoints

Updated: 2026-07-25

| Checkpoint | Status | Evidence |
|---|---|---|
| One inherited capability-aware product shell | Complete | Member, locked Admin, and elevated Admin browser matrix |
| Canonical frontend/API routing boundary | Complete | Direct-load/reload and API-boundary Go/Playwright contracts |
| No fake data, placeholder cards, or browser-owned dialogs | Complete | `product-truth.test.ts`, browser text gate, Radix alert dialogs |
| Scoped Usage with real detail | Complete | personal/pool/member scopes; model/provider/connection totals; measured model curves |
| Connections administration | Complete baseline | detail, rename, refresh, enable, disable, recover |
| Members administration | Complete baseline | real identities, create, one-time token, enable, disable |
| System administration | Complete baseline | measured runtime, persistence, registry, maintenance controls |
| Profile security | Partial | enrollment/confirmation/recovery display complete; rotation operations remain |
| Selected-model routing | Complete baseline | backend-owned runtime provider, eligibility, exclusions, and evidence on `/models` |
| Setup | Frozen | separate replan |

The current phase is feature/function delivery. Validation hardening and broad visual polish are deferred to the following phase unless required to make a feature safe or operable.

### Selected-model routing checkpoint

Completed baseline: `GET /api/v2/models/:id/routing` is an elevated-Admin read projection returning backend-owned requested/canonical model identity, provider, request-time selection mode, eligible connections, excluded connections with exact reasons, and evidence time. It does not claim a deterministic selected connection or fallback order because selection happens per request.

The frontend keeps routing detail inside `/models`; no `/admin/routes` route exists. Members never request protected routing data.

## Phase ordering

### Current functional phase

1. ~~Remove discarded routes and duplicate workspace.~~
2. ~~Establish inherited capability shell and one MFA gate.~~
3. ~~Implement truthful scoped/detailed Usage.~~
4. ~~Implement Connections, Members, and System functional baselines.~~
5. ~~Implement backend-owned selected-model routing context.~~
6. **Complete Profile security rotation/recovery operations.**
7. Replan Setup separately.

### Following hardening/polish phase

- member validation and policy hardening;
- authorization-expiry/retry refinements;
- full accessibility audit;
- visual and responsive polish;
- performance and request-concurrency tuning;
- soak/release preparation.

## Historical implementation order after plan approval

1. Remove the discarded `/operator/*` route and navigation implementation with no compatibility layer.
2. Restore one inherited capability-aware shell.
3. Remove arbitrary production-origin fallback from member Usage and Home.
4. Implement one Models resource with admin routing context.
5. Implement one Usage resource with member/admin scopes and economics context.
6. Implement `/admin/connections`, `/admin/members`, and `/admin/system` as unique resources.
7. Rebuild Home as a concise shared summary with admin attention augmentation.
8. Replan Setup separately; do not alter it during steps 1-7.

## Non-goals

- preserving wrong development routes;
- matching the Step 0 page inventory;
- separate member and admin workspaces;
- an Operator role distinct from Admin;
- duplicate model, usage, economics, dashboard, or monitoring pages;
- arbitrary snapshot origins presented as personal data;
- fictional runtime, routing, health, latency, incident, or economics semantics.
