# UI/UX Review and Redesign Direction

Status: **historical discovery input; information architecture superseded**

> The role/job findings remain useful, but the proposed separate Operator workspace is discarded. Use [`frontend-product-architecture.md`](frontend-product-architecture.md) for current routes, capability inheritance, and resource ownership.

Implementation plan: [`ui-redesign-plan.md`](ui-redesign-plan.md)

GitHub tracker: [sabishii-me/codex-pool#1](https://github.com/sabishii-me/codex-pool/issues/1)

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
- Required user-editable display name
- Credential label, never raw secret
- Optional provider identity details such as email, workspace, tenant, or upstream subject ID
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

## Visual system and theming

The current stylesheet has many feature-specific treatments that independently define borders, spacing, typography, colors, meters, headers, and interaction states. This creates the “random” feeling: individual sections are polished, but they do not always look like parts of the same product.

The redesign should establish a small visual system before redesigning individual screens.

### Theme contract

Components must consume semantic tokens rather than literal colors or page-specific values. Themes should be selectable through a root attribute such as `data-theme` and should not require component changes.

```css
:root,
[data-theme="signal-dark"] {
  color-scheme: dark;

  --color-bg-canvas: #090a08;
  --color-bg-surface: #11120f;
  --color-bg-elevated: #181a15;
  --color-text-primary: #f3f0df;
  --color-text-secondary: #aaa790;
  --color-text-muted: #777564;
  --color-border-subtle: #292a22;
  --color-border-strong: #494738;
  --color-accent: #e0bd58;
  --color-on-accent: #111008;
  --color-success: #72b879;
  --color-warning: #daa64b;
  --color-danger: #d86c62;
  --color-info: #6fa7c8;

  --space-1: 0.25rem;
  --space-2: 0.5rem;
  --space-3: 0.75rem;
  --space-4: 1rem;
  --space-6: 1.5rem;
  --space-8: 2rem;

  --radius-control: 0.25rem;
  --radius-panel: 0.375rem;
  --border-width: 1px;
  --font-body: system-ui, sans-serif;
  --font-data: ui-monospace, monospace;
  --duration-fast: 120ms;
  --duration-normal: 200ms;
}
```

The exact values belong in visual design work; the important part is the stable semantic contract.

Token groups should cover:

- canvas, surface, elevated surface, and overlay;
- primary, secondary, muted, inverse, and disabled text;
- subtle, default, strong, focus, and interactive borders;
- accent, success, warning, danger, and information states;
- chart categorical and sequential palettes;
- spacing, typography, radius, border, elevation, and motion;
- control height and content width breakpoints.

Do not encode a theme name into component APIs. A button uses `variant="primary"`, not `variant="gold"`.

### Initial theme set

Support these through the same token contract:

1. **Signal dark** — evolution of the current black/gold identity and the default.
2. **Signal light** — readable light surfaces for bright environments and printing.
3. **High contrast** — stronger borders and text contrast with reduced decorative noise.
4. **System** — selects light or dark from `prefers-color-scheme`, with a persisted user override.

Theme preference should be available in Profile and applied before React renders to avoid a flash of the wrong theme.

### Provider identity is not a theme

Provider colors are categorical data, not structural UI colors. A provider may supply a validated display color or palette index through the API, but that value should only populate a scoped variable such as `--provider-color` for a marker or chart series.

Provider color must not determine text contrast, status, button style, or panel background. Provider identity must also be available as text or icon so color is never the sole distinction.

### Consistent component vocabulary

Reduce one-off patterns to a documented set of reusable components:

| Component | Responsibility |
|---|---|
| `PageHeader` | Title, description, freshness, and page-level actions |
| `Section` | Heading, supporting copy, actions, and content spacing |
| `Panel` | Standard surface, border, padding, and optional header/footer |
| `MetricCard` | Label, normalized value, trend, evidence state, and timeframe |
| `StatusBadge` | Semantic state with icon and text |
| `Button` / `IconButton` | Primary, secondary, quiet, and danger actions |
| `Tabs` | Route or local mode selection with one visual treatment |
| `DataTable` | Sorting, selection, actions, empty state, and responsive fallback |
| `DisclosureList` | Mobile/detail alternative to dense tables |
| `Field` | Label, help, validation, and control association |
| `Dialog` / `Drawer` | Focus-managed overlays with consistent actions |
| `EmptyState` | Explanation and next action |
| `ResourceState` | Loading, stale, partial failure, and retry behavior |
| `ChartFrame` | Title, legend, summary, evidence, and theme-aware chart palette |

Each component needs defined sizes, variants, allowed composition, keyboard behavior, loading/disabled state, and examples. Feature code may arrange these primitives, but should not recreate their visual rules.

### Layout rules

- Use one spacing scale; do not introduce arbitrary gaps for each dashboard.
- Define standard page widths and a responsive grid instead of bespoke grids per panel group.
- Use one panel header pattern and one section-heading hierarchy.
- Keep data alignment predictable: labels left, comparable numeric values right, status consistently located.
- Reserve monospaced type for IDs, models, commands, timestamps, and numeric data—not all explanatory copy.
- Use uppercase sparingly for short metadata labels; body and action text should use normal casing.
- Define one density setting for normal users and an optional compact density for operator tables.
- Decorative section codes such as `C.10` can remain secondary but must not replace descriptive headings.

### Interaction rules

- One primary action per page or dialog.
- Quiet actions remain visually quiet until hover/focus.
- Danger styling is reserved for destructive or materially risky actions.
- All interactive elements share focus-ring, hover, pressed, disabled, and busy treatments.
- Motion conveys state or hierarchy; it is not independently invented by each feature.
- Reduced-motion mode removes nonessential transitions and chart animation.
- Toasts report completed background actions; inline feedback explains field and resource errors.

### Chart consistency

Charts currently have distinctive visual treatments but need a shared theme adapter. Standardize:

- axes, grid, tooltip, legend, and focus behavior;
- series assignment from semantic chart palettes;
- number and time formatting;
- measured versus inferred line/dash styles;
- unknown and missing intervals;
- accessible tabular or textual summaries;
- compact and full-height chart sizes.

Charts must consume CSS token values at runtime so switching themes updates charts without remounting feature code.

### CSS organization

Replace the single large global stylesheet incrementally:

```text
src/styles/
  reset.css
  tokens.css
  themes/
    signal-dark.css
    signal-light.css
    high-contrast.css
  foundations/
    typography.css
    layout.css
    motion.css

src/components/
  button/Button.tsx
  button/Button.module.css
  panel/Panel.tsx
  panel/Panel.module.css
  ...
```

Use global CSS only for reset, tokens, themes, and basic document foundations. Component styles should be locally scoped. Feature styles should describe layout, not redefine controls, colors, typography, or status semantics.

Inline styles should be limited to genuinely dynamic values such as chart coordinates, progress percentages, and a validated provider marker variable. Move static colors and layout values out of JSX.

### Component catalog and governance

Maintain a development-only component catalog using Storybook, Ladle, or a small in-app catalog route. It should show:

- every component and variant;
- all themes;
- normal, hover, focus, disabled, loading, empty, error, and stale states;
- realistic short and long content;
- narrow and wide viewports;
- unknown providers and missing data;
- accessibility checks.

New one-off controls or panel treatments should require an explicit reason. Prefer extending a shared primitive only when the new behavior is useful in more than one feature.

### Visual-system migration

1. Inventory literal colors, spacing values, type sizes, repeated controls, panels, and badges.
2. Define semantic tokens by mapping current values before changing the look.
3. Build foundational components and catalog fixtures.
4. Convert the application shell, navigation, buttons, fields, and dialogs.
5. Convert status and resource-state components.
6. Convert panels, metrics, tables, and charts feature by feature.
7. Delete superseded selectors after each feature migration.
8. Add visual regression snapshots for every theme and key viewport.
9. Add a lint rule or CI check that prevents new unapproved color literals and global feature selectors.

A temporary compatibility layer may map old variables/classes to new tokens, but it should have an owner and removal milestone.

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

Status: complete as a historical design review. The accepted direction has since been implemented as one capability-aware product. Current release and soak evidence is tracked in [`frontend-product-architecture.md`](frontend-product-architecture.md) and [`staging-soak-checkpoint.md`](staging-soak-checkpoint.md); Staging is an explicitly promoted environment, not a pinned UI control.

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
- Add a provider-neutral connection identity contract centered on `display_name`.
- Treat email, workspace, tenant, region, and upstream subject ID as optional structured metadata with visibility rules.
- Remove the temporary generic `AccountStats.account_email` dependency.
- Remove provider accounting calculations from React.
- Add backend-supplied provider presentation and connection-form schemas.

Deliverable: versioned UI data contract with fixtures.

### UX Phase 3 — Frontend and visual-system foundation

- Add routing and query caching.
- Define semantic design tokens and the initial dark, light, and high-contrast themes.
- Build the component catalog and visual regression harness.
- Split `App.tsx` by feature.
- Introduce shared page, panel, status, button, table, dialog, chart, and form components.
- Migrate global styles into foundations, locally scoped component styles, and feature layouts.
- Generate or validate API types.

Deliverable: a theme-switchable new shell and component catalog running against fixtures and compatibility APIs.

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
8. Inventory repeated visual patterns and map literal colors/spacing to semantic tokens.
9. Standardize buttons, fields, panels, status badges, and focus states before adding new dashboard treatments.

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

The visual system should also meet these criteria:

- every core screen works in dark, light, and high-contrast themes without feature-specific overrides;
- the same state has the same label, color role, icon treatment, and placement throughout the product;
- core controls and data states are represented in the component catalog;
- new providers render without adding provider-specific CSS;
- no new unapproved color literals or globally scoped feature-control styles enter the codebase;
- desktop and mobile visual regressions are checked for every supported theme.
