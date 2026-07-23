# Phase 9 UI Redesign Plan

Status: active — Product Design Harness Step 0 is reviewable and awaiting owner acceptance; production implementation has not started

Branch: `feat/ui-ux-redesign`

GitHub tracker: [sabishii-me/codex-pool#1](https://github.com/sabishii-me/codex-pool/issues/1)

Related documents:

- [UI/UX review and redesign direction](ui-ux-review.md)
- [Legacy UI compatibility baseline](legacy-ui-baseline.md)
- [Architecture roadmap](architecture-roadmap.md)
- [Development instance](development-instance.md)

## Decision

Phase 9 will first establish a conventional, maintainable dashboard and monitoring product. The existing Signal Room visual language remains valuable, but it becomes a presentation theme over a stable UX foundation rather than the structure of the application.

> The baseline defines how the product works. A theme defines how the product feels.

The redesign is not a cosmetic reskin. It separates information architecture, workflows, data semantics, components, and visual themes so each can evolve without forcing changes in the others.

## Product principles

1. **Conventional before custom.** Use familiar application-shell, navigation, page, table, form, dialog, and monitoring patterns before adding decorative treatments.
2. **Overview before detail.** The front page is a concise dashboard, not a complete operations console.
3. **Dashboard and monitor are different.** The dashboard summarizes state and next actions; the monitor supports diagnosis.
4. **Members and operators have different jobs.** Operator infrastructure must not dominate the ordinary member experience.
5. **Plain language carries meaning.** Brand language may add character but must not replace precise labels for health, quota, billing, authentication, and destructive actions.
6. **Backend projections own domain semantics.** React presents normalized health, usage, cost, quota, routing, and evidence states; it does not recreate provider policy.
7. **Themes cannot change behavior.** A theme may change tokens and nonessential decoration, but not navigation, permissions, validation, semantics, or workflow structure.
8. **Accessibility and resilience are foundation requirements.** They are not polish deferred until after visual design.
9. **The legacy client remains the regression control.** Replacement is allowed only after workflow parity and soak reliability are demonstrated.

## Information architecture

### Member workspace

- **Dashboard** — gateway status, personal activity, available models, incidents, and next actions.
- **Models** — searchable model catalog, availability, capabilities, limits, and copyable route IDs.
- **Setup** — guided client configuration and verification.
- **My usage** — personal requests, normalized token usage, trends, and meaningful cost/value data.
- **Profile** — identity, security, sessions, and issued gateway credentials.

### Operator workspace

Expose this workspace only when the backend authorizes the user:

- **Overview** — incidents, capacity risks, and actions requiring attention.
- **Monitor** — request activity, errors, latency, provider health, persistence health, quota windows, and routing behavior.
- **Provider connections** — credentialed upstream capacity and lifecycle operations.
- **Model routes** — public routes, upstream targets, eligible connections, and routing policy.
- **Usage and economics** — pool-wide accounting, capacity, evidence, and economics.
- **Members** — gateway users and access administration.
- **System** — runtime, persistence, configuration, and recovery state.

“Insights” and “Accounts” will not remain ambiguous top-level concepts. Their capabilities will move into the task-oriented destinations above. User-facing terminology follows `GatewayUser → Provider → ProviderConnection → ModelRoute`.

## Dashboard boundary

The dashboard must answer, in order:

1. Is the gateway operational?
2. Can the signed-in member use it now?
3. What recent activity or personal usage matters?
4. Which models and providers are available?
5. Does anything require action?

The initial dashboard composition is:

1. **Status summary** — current state, last refresh, available-model count, and concise degradation explanation.
2. **Primary metrics** — a restrained set such as requests, processed tokens, available models, and healthy connections. Scope must be explicit as personal or pool-wide.
3. **Activity trend** — one primary chart with clear units, timeframe, and a text summary.
4. **Provider health summary** — provider-level state and capacity, not a credential-management table.
5. **Attention required** — actionable incidents only; otherwise a concise no-action state.
6. **Quick actions** — configure a client, browse models, view usage, or enter operator monitoring when authorized.

The dashboard must not contain complete setup flows, raw connection management, detailed route diagnostics, quota methodology, and economics simultaneously.

## Monitor boundary

The monitor is a denser diagnostic workspace. It may expose:

- request rate, tokens, latency, and failures;
- provider and connection availability;
- quota windows, cooldowns, and recovery estimates;
- model-route eligibility and fallback behavior;
- canonical usage persistence and projection health;
- recent operational events and their impact.

Monitoring observes and explains. Configuration remains in Provider connections, Model routes, Members, and System.

## Baseline visual system

The first supported presentation is a neutral, professional baseline optimized for readability. It establishes:

- semantic color, typography, spacing, radius, elevation, motion, and density tokens;
- one application shell and responsive grid;
- consistent page headers and content hierarchy;
- standard controls and interaction states;
- accessible success, warning, danger, information, stale, unknown, and unavailable states;
- reusable chart framing and data formatting;
- locally scoped component styles rather than a growing global feature stylesheet.

Required primitives include:

- `AppShell`, `Sidebar`, `TopBar`, and `PageHeader`;
- `Button`, `IconButton`, `Field`, `Select`, and `Tabs`;
- `Panel`, `MetricCard`, `StatusBadge`, and `Alert`;
- `DataTable` with a narrow-screen disclosure alternative;
- `Dialog`, `Drawer`, `Menu`, and `Tooltip`;
- `LoadingState`, `EmptyState`, `ResourceError`, and `StaleState`;
- `ChartFrame`, `CodeBlock`, and copy feedback.

Each primitive requires documented variants, keyboard behavior, focus behavior, loading/disabled state, long-content behavior, and responsive examples.

## Theme boundary

Components consume semantic tokens such as `--color-bg-surface`, `--color-text-primary`, and `--color-status-danger`. Component APIs describe intent such as `variant="primary"`, never a theme color such as `variant="gold"`.

A theme may control:

- semantic token values;
- typography families within supported roles;
- radii, shadows, and supported density ranges;
- chart palettes;
- logo and brand assets;
- optional nonessential texture and effects.

A theme may not control:

- routes or navigation meaning;
- authorization or visibility decisions supplied by policy;
- health and evidence semantics;
- form validation or destructive-action behavior;
- provider accounting or routing logic;
- accessibility requirements.

Do not build a runtime plugin loader initially. First prove the boundary with two internal themes using the same components and tests:

1. **Baseline** — neutral, readable, and professional; the reference implementation.
2. **Signal Room** — black/gold, heraldic, and atmospheric; an optional branded presentation.

Runtime theme packaging can be evaluated only after both themes pass the same workflow, responsive, accessibility, and visual-regression suites.

## Delivery plan and progress

| Phase | Status | Deliverable | Exit criterion |
|---|---|---|---|
| 0. Discovery and direction | **Complete** | Current-state review, member/operator jobs, dashboard/monitor split, baseline-first and theme-boundary decision | Direction is documented and accepted |
| Harness Step 0. Runnable interpretation | **Awaiting owner review** | One mounted baseline dashboard + monitor artifact across member/operator and responsive profiles | Owner explicitly accepts the interaction architecture; only then may the prompt be frozen |
| 1. Information architecture | Not started | Route map, navigation model, role behavior, dashboard/monitor wireframes, standard page patterns | Primary member and operator flows can be reviewed without visual styling |
| 2. UI data contracts | Not started | Inventory of required v2 projections and fixtures; removal plan for React-owned accounting/provider policy | Every first-wave screen can render from normalized, versioned contracts including unknown/stale states |
| 3. Foundation | Not started | Router, application shell, resource/query layer, semantic tokens, baseline theme, primitives, component catalog, test harness | A responsive and keyboard-usable shell renders representative fixtures without legacy page CSS |
| 4. Dashboard and member workflows | Not started | Dashboard, Models, Setup, My usage, and Profile | A member can assess status, configure a client, find a model, and inspect personal usage without operator concepts |
| 5. Operator workspace | Not started | Operator overview, Monitor, Provider connections, Model routes, Usage/economics, Members, and System | Existing administration, contribution, diagnosis, and recovery workflows have replacement coverage |
| 6. Signal Room theme | Not started | Signal Room token/asset package over the shared baseline components | No feature component or workflow forks by theme; both themes pass the same tests |
| 7. Parity, soak, and promotion | Not started | Accessibility, responsive, visual, contract, partial-failure, and live soak evidence | Replacement meets the legacy baseline and promotion is an explicit release decision |

Update this table when a phase starts or its exit criterion is met. Detailed implementation work may use child issues linked from the umbrella GitHub tracker.

## Product Design Harness Step 0

The project-owned review kit lives under [`.product-design/`](../.product-design/). It contains one candidate only:

- Round: `step-0`
- Artifact: `primary` (`Baseline dashboard + monitor`)
- Direction status: `draft`
- Owner decision: pending
- Runtime profiles: `member`, `operator` (presentation fixtures, never authorization)
- Viewports: 1440×900 desktop, 820×1024 tablet portrait, 390×844 mobile portrait, and 844×390 mobile landscape

The artifact makes Dashboard, Models, and operator Monitor interactive. Setup, My usage, Provider connections, Model routes, and System are explicit information-architecture placeholders rather than falsely complete workflows. It uses fictional in-memory data, performs no network or real product operation, and resets on reload.

Review locally:

```bash
cd web
npm run design:serve
```

Then open the printed loopback URL and record page- or element-level feedback through the harness. Validation commands are:

```bash
npm run design:doctor
npm run design:validate
npm run design:validate:browser
npm run design:capture
```

Passing validation makes the artifact reviewable; it does not approve or freeze it. Do not create alternative candidates until explicit owner acceptance is recorded and `.product-design/direction/frozen-design-prompt.md` is deliberately frozen.

## Phase 1 decisions required

- Stable URL map and browser back/forward behavior.
- Member/operator navigation grouping and small-screen navigation.
- Whether operator context is a workspace switch or a separate route group.
- Dashboard metric scope and the source/evidence label for each metric.
- Alert severity and “attention required” inclusion rules.
- Standard page templates for overview, list, detail, setup, and monitor pages.
- Mobile alternatives for provider-connection and model tables.

## Quality gates

Every migrated route must cover:

- loading, empty, ready, stale, partial-failure, and fatal-failure states;
- unknown runtime providers and missing optional metadata;
- keyboard navigation and visible focus;
- meaningful headings and landmarks;
- focus-managed dialogs and drawers;
- status meaning that does not depend on color;
- narrow mobile, tablet, and desktop layouts;
- no unexpected page-level horizontal overflow;
- preservation of last-known-good data where appropriate;
- actionable plain-text backend errors;
- no provider-specific accounting or routing decisions in React.

Frontend build and Go validation remain sequential because Go embeds `web/dist` while Vite replaces it.

## Legacy parity checklist

Before promotion, the replacement must preserve or improve:

- [ ] Authentication, sign-out, local development session, and access-denied behavior
- [ ] Operator elevation and MFA enrollment, verification, recovery, and disable flows
- [ ] Dashboard independent-resource refresh and last-known-good preservation
- [ ] Model search, provider filtering, availability, metadata, and route-ID copying
- [ ] Pi, Cute Code, Codex, Gemini, Claude, and Grok setup/config downloads
- [ ] Provider contribution, credential validation, and Codex browser OAuth
- [ ] Canonical v2 provider identities and unknown runtime provider rendering
- [ ] Provider-connection inspect, rename, test, enable/disable, and remove operations
- [ ] Usage, capacity, routing, health, and economics operator visibility
- [ ] Member and system administration recovery paths
- [ ] Desktop and mobile route coverage
- [ ] Partial backend failure, repeated refresh, and shell uniqueness behavior
- [ ] No unexpected console errors, uncaught page errors, or failed same-origin requests
- [ ] Staging comparison and extended soak reliability

## Environment and release discipline

- Pinned legacy control: `http://127.0.0.1:18990` using `codex-pool:staging-a91560b` and `staging/*` state.
- Active redesign: `http://127.0.0.2:18991` using `codex-pool:dev` and `dev/*` state.
- Production: `http://localhost:8989`; unchanged during ordinary Phase 9 work.
- Do not rebuild staging or production to validate feature-branch work.
- Compare replacement behavior with staging throughout development.
- Promotion requires an explicit release decision after parity and soak evidence; branch completion alone is insufficient.

## Progress-recording convention

The umbrella GitHub issue is the human-visible tracker. This document records durable architectural decisions, phase status, and exit criteria.

When work advances:

1. Open or link a child issue for a bounded deliverable when useful.
2. Reference the umbrella issue in pull requests and commits.
3. Update the phase table only when status or exit evidence changes.
4. Record compatibility incidents using [the legacy baseline taxonomy](legacy-ui-baseline.md).
5. Do not mark a workflow complete based only on a visual implementation; include contract, browser, accessibility, and resilience evidence.
