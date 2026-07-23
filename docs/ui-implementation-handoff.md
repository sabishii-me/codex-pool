# Phase 9 Production UI Implementation Handoff

Status: ready for implementation

Frozen design checkpoint: `f0d30d5`

Accepted artifact source checkpoint: `70cfcd4`

Current branch: `feat/ui-ux-redesign`

## Purpose

Translate the selected Product Design Harness Step 0 artifact into the production React application incrementally without destabilizing the existing Signal Room, staging baseline, gateway contracts, authentication flows, or provider operations.

The artifact is an interaction and visual contract, not production code to copy wholesale. It uses fictional in-memory data and intentionally omits real asynchronous, authorization, persistence, and compatibility behavior. Production implementation must use typed read models and existing backend policy.

## Non-negotiable implementation boundaries

1. Keep the legacy UI available until replacement parity and soak criteria pass.
2. Do not rebuild or restart pinned staging or production during ordinary implementation.
3. Develop against `http://127.0.0.2:18991` and compare with staging at `http://127.0.0.1:18990`.
4. Preserve existing authentication, MFA, setup downloads, contribution/OAuth, operator recovery, unknown-provider, and partial-failure behavior.
5. Presentation state never grants operator authority; backend policy remains authoritative.
6. Do not implement provider accounting, quota, health, evidence, or routing semantics in React.
7. Do not ship fictional Step 0 data or silently map “unknown” to zero.
8. Frontend build and Go validation run sequentially because Go embeds `web/dist`.
9. Material deviation from `.product-design/direction/frozen-design-prompt.md` requires an explicit design decision or new harness round.

## Frozen production routes

Use stable browser routes and real history navigation.

### Member

| Route | Page | Existing source readiness |
|---|---|---|
| `/` | Dashboard | Partial: stats, analytics, and catalog exist; member-scoped usage projection is missing |
| `/models` | Models | Ready through `/api/pool/catalog` |
| `/setup` | Setup | Mostly ready through session and `/config/*` downloads; credential scoping remains a future backend improvement |
| `/usage` | My usage | Partial: analytics exist, but a canonical member-scoped summary/timeseries is missing |
| `/profile` | Profile/security | Partial: session and MFA exist; issued-credential/session management projections are incomplete |

### Operator

| Route | Page | Existing source readiness |
|---|---|---|
| `/operator` | Operator overview | Partial through stats/signal; explicit incident projection is missing |
| `/operator/monitor` | Monitor | Partial through stats/signal; latency, persistence health, and event feed projections are missing |
| `/operator/connections` | Provider connections | Ready for initial migration through `/api/v2/provider-connections` plus existing admin operations |
| `/operator/routes` | Model routes | Partial through catalog; routing policy and eligible-connection explanation projection is missing |
| `/operator/usage` | Usage and economics | Existing `/api/pool/signal` supports initial migration |
| `/operator/members` | Members | Existing `/api/pool/users` and admin user operations support initial migration |
| `/operator/system` | System | Partial: existing admin operations exist; normalized system-health projection is missing |

Unauthorized operator URLs must render an access result or redirect based on authenticated policy; they must never be unlocked by route state, media query, local preference, or presentation runtime.

## Frontend target structure

```text
web/src/
  app/
    App.tsx
    router.tsx
    route-config.ts
    providers.tsx
    session/
    shell/
      AppShell.tsx
      Sidebar.tsx
      TopBar.tsx
      MobileNavigation.tsx

  components/
    actions/
    charts/
      SplineChart.tsx
      TexturedBarChart.tsx
      ChartFrame.tsx
    data-display/
      DataTable.tsx
      DisclosureList.tsx
      MetricCard.tsx
      StatusBadge.tsx
    feedback/
      ResourceState.tsx
      EmptyState.tsx
      Alert.tsx
    forms/
    overlays/
    surfaces/

  features/
    dashboard/
    models/
    setup/
    usage/
    profile/
    operator/
      overview/
      monitor/
      connections/
      routes/
      economics/
      members/
      system/

  data/
    client.ts
    resource.ts
    session.ts
    dashboard.ts
    models.ts
    connections.ts

  generated/
  presentation/
    provider-presentation.ts
    format.ts
  styles/
    reset.css
    tokens.css
    foundations.css
    themes/
      default.css
      signal-room.css
```

Do not perform a one-commit rewrite of the current `App.tsx`. Introduce the new shell and routes behind a deliberate development boundary, then migrate workflows page by page.

## Default theme contract

The accepted default theme uses semantic tokens. Components must never expose color-named variants.

```text
canvas             #0e0e10
surface            #18181b
subtle surface      #202024
border              #27272a
strong border       #3f3f46
primary text        #ffffff
secondary text      #d4d4d8
muted text          #a1a1aa
primary accent      #f87171
accent hover        #fca5a5
accent depth        #b91c1c
success             #4ade80
warning/comparison  #fde047
information         #22d3ee
danger              #ff5c57
```

Required visual behavior:

- full-viewport dark product canvas; no decorative page behind the app;
- rounded dark bento cards, subtle borders, inner highlights, and restrained shadow/glow;
- pill active navigation and segmented controls;
- white metrics with muted labels;
- warm-red primary series, yellow comparison, green health, cyan information;
- visible focus and AA contrast;
- no status communicated by color alone.

## Chart implementation contract

### Continuous time series

Use one shared responsive chart component with:

- smooth cubic spline primary and comparison paths;
- warm-red primary stroke and area gradient;
- thin yellow comparison stroke;
- dot-matrix chart field;
- minimal axes with formatted units/time;
- keyboard/pointer active index;
- dashed active marker;
- primary and comparison nodes;
- dark pill tooltip;
- text/table summary for assistive technology;
- explicit missing/unknown intervals;
- reduced-motion behavior;
- token-driven colors read at runtime.

The Step 0 local SVG renderer is geometry reference only. Production may use the existing D3 dependencies, but chart semantics, accessibility, and responsive behavior belong in shared components.

### Discrete comparisons

Use textured capsule bars for categories/days:

- hatched muted inactive values;
- solid red/yellow segmentation for the selected or peak category;
- exact value labels;
- no spline interpolation across categorical values.

Bubble clusters are permitted only for proportional composition data. Halftone maps are permitted only for actual geographic data. Neither is required for the first implementation milestone.

## Responsive production contract

| Profile | Initial role behavior | Composition |
|---|---|---|
| Desktop 1440×900 | Member Dashboard / operator Overview | Full shell and bento dashboard; collapsible divider rail |
| Tablet portrait 820×1024 | Role home | Reduced dashboard hierarchy |
| Tablet landscape 1180×820 | Role home | Secondary-screen focus: collapsed rail, four readouts, freshness, one dominant graph |
| Mobile portrait 390×844 | Member focused Dashboard / operator focused Monitor | Shared focus composition above fixed navigation |
| Mobile landscape 844×390 | Member focused Dashboard / operator focused Monitor | Shared single-screen focus composition above fixed navigation |

Desktop sidebar collapse preference may be persisted only as a presentation preference; it cannot affect authorization or route access.

## Existing APIs usable in the first slices

| Need | Existing API/function |
|---|---|
| Session and role hint | `GET /api/pool/session` / `loadSession()` |
| Independent dashboard resources | `loadDashboardResources()` using stats, signal, catalog |
| Pool stats | `GET /api/pool/stats` |
| Analytics/economics | `GET /api/pool/signal?weeks=6` |
| Model catalog | `GET /api/pool/catalog` |
| Canonical connection identity | `GET /api/v2/provider-connections` |
| Rename connection | `PATCH /api/v2/provider-connections/:id/identity` |
| Existing operator lifecycle | `/admin/accounts/:id/*` compatibility routes |
| Setup material | Session fields and `/config/*/:download_token` routes |
| MFA | `/api/admin/mfa/*` |
| Contribution/OAuth | Existing `/api/pool/accounts/*` compatibility routes and Codex broker |
| Members | `/api/pool/users` and existing admin member routes |

Existing endpoints are compatibility inputs, not permission to carry legacy `Account` language into new components.

## Required backend projections

Do not block the shell/Models/Connections migration on every projection, but do not fake these fields in React.

### Priority A — dashboard and personal usage

Define versioned view models for:

```text
GET /api/v2/dashboard/summary
GET /api/v2/usage/me/summary
GET /api/v2/usage/me/timeseries?range=24h|7d|30d
```

Required semantics:

- explicit personal or pool scope;
- requests, processed tokens, normalized token composition;
- available model/provider counts;
- freshness and source timestamps;
- measured/estimated/inferred/unavailable evidence state;
- trend comparison period;
- unknown distinct from zero.

### Priority B — monitor and system health

```text
GET /api/v2/monitor/summary
GET /api/v2/monitor/timeseries?range=30m|24h
GET /api/v2/monitor/incidents
GET /api/v2/system/health
```

Needed fields include request rate, latency, error rate, capacity/headroom, canonical persistence health, projection lag, active incidents, routing compensation, recovery estimate, and recent operational events.

### Priority C — route explanation

```text
GET /api/v2/model-routes
GET /api/v2/model-routes/:id
```

Return public route, upstream target, eligibility summary, normalized policy label, fallback behavior, current availability, and an operator-safe explanation. Do not expose mutable routing internals directly.

### Priority D — leaderboard

A leaderboard requires an explicit product/privacy decision and backend projection before production implementation.

```text
GET /api/v2/usage/leaderboard?range=week
```

Minimum contract:

- backend-authorized visibility;
- abbreviated or anonymized display identity;
- opt-out/privacy policy;
- aggregate normalized metric;
- deterministic rank and movement period;
- no full email or upstream identity leakage;
- no reconstruction from other users' raw usage in React.

Until this contract exists, My usage ships without the leaderboard rather than with fabricated data.

## Resource-state model

Every query owns:

```text
status: idle | loading | ready | stale | error
lastSuccessfulData
lastSuccessfulAt
attemptedAt
errorMessage
retry()
```

Rules:

- one failed resource must not blank unrelated panels;
- safe last-known-good values remain visible with stale labeling;
- plain-text backend errors remain actionable;
- page headers may summarize freshness/incident count but cannot hide healthy resources;
- mutations invalidate only relevant queries;
- no duplicate startup refresh.

A small project-owned resource layer is sufficient initially. Do not add a query dependency solely to avoid defining the resource contract.

## Implementation slices

### Slice 0 — preserve accepted evidence

- [x] Select the Step 0 artifact in harness review state.
- [x] Freeze the direction prompt and config.
- [x] Preserve ten responsive/runtime captures and browser validation.
- [x] Keep a clean commit boundary before production code.

### Slice 1 — application foundation

Status: **in progress — route adapter and legacy shell boundary complete; replacement shell primitives remain**

Implemented in the first bounded production change:

- stable member and operator route adapters in `web/src/routes.ts`;
- browser history and `popstate` synchronization;
- compact operator default to `/operator/monitor`;
- operator-route client navigation guard as a UX safeguard (backend remains authoritative);
- member/operator navigation separation using existing view implementations;
- stable URL adapters for existing Models, Setup, Usage, Profile, Monitor, Connections, and operator routes;
- route contract tests in `web/src/routes.test.ts`;
- legacy feature internals remain unchanged behind the adapter boundary.

Validation evidence for this increment:

- Vitest: 24/24;
- TypeScript: pass;
- Vite production build: pass (existing >500 kB chunk warning remains).

Remaining Slice 1 work:

- extract accepted semantic tokens into production CSS;
- implement the replacement AppShell/sidebar/mobile navigation primitives;
- implement real responsive tablet-landscape focus boundary;
- add shell-level browser tests for authorization, collapse, and compact defaults;
- preserve the legacy shell as an explicit migration fallback while the replacement shell is introduced.

Exit: shell renders real session/role state and representative resource fixtures without changing legacy workflows.

### Slice 2 — shared charts and Dashboard

- Implement `SplineChart` and textured bar primitive.
- Build Dashboard against existing resources where semantics are truthful.
- Mark unavailable personal-scope fields explicitly until v2 projections land.
- Preserve independent refresh and last-known-good behavior.

Exit: dashboard answers status, availability, activity, attention, and next action with real data or explicit unavailable states.

### Slice 3 — Models and Setup

- Migrate catalog search/filter/copy and unknown-provider presentation.
- Migrate all client setup/config downloads.
- Preserve copy feedback and setup error text.
- Preserve sensitive-data constraints and current download-token authorization.

Exit: all live setup/config paths and model discovery pass parity tests.

### Slice 4 — My usage and Profile

- Add personal usage once v2 projections exist.
- Add leaderboard only after Priority D privacy contract exists.
- Migrate profile, session/security, MFA, and sign-out behavior.

Exit: member path is complete without operator infrastructure concepts.

### Slice 5 — operator workspace

- Migrate Overview and Monitor.
- Migrate canonical Provider connections plus lifecycle compatibility operations.
- Migrate Model routes when explanation projection exists.
- Migrate usage/economics, members, and system recovery.
- Preserve contribution and all OAuth flows.

Exit: operator diagnosis, contribution, mutation, and recovery match legacy coverage.

### Slice 6 — parity and promotion evidence

- Run desktop/mobile/tablet task tests.
- Run keyboard, screen-reader, and automated accessibility checks.
- Run unknown-provider and partial-failure contract tests.
- Run baseline staging comparison and extended development soak.
- Record explicit release decision; do not infer promotion from branch completion.

## Testing requirements

For each migrated route:

- unit tests for formatting and view-model mapping;
- resource-state tests for loading/ready/stale/error/retry;
- browser tests for member/operator authorization and stable URLs;
- mobile portrait, mobile landscape, tablet portrait, tablet landscape, and desktop coverage;
- keyboard navigation and focus restoration;
- no unexpected horizontal overflow;
- no uncaught page errors, console errors, or failed same-origin requests;
- visual snapshots for accepted default theme;
- unknown runtime provider fixture;
- long labels, missing metadata, and unavailable evidence fixture.

Keep the legacy baseline suite running against pinned staging throughout migration.

## First production coding task

Implement **Slice 1 only** as the next bounded change:

1. establish the route map and shell architecture;
2. extract accepted semantic tokens into production CSS;
3. implement accessible desktop sidebar divider collapse;
4. implement mobile drawer/bottom navigation and role-aware defaults;
5. implement tablet-landscape focus layout boundary;
6. preserve legacy UI behind an explicit development migration boundary;
7. use real session authorization state;
8. add shell-level tests before migrating dashboard business content.

Do not copy fictional artifact metrics into production and do not begin by replacing all of `App.tsx`.
