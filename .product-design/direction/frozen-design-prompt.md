# Frozen design prompt

Status: **superseded for production information architecture**

> Historical design-reference record only. The separate member/operator workspace and route inventory below are not current implementation constraints. The approved source of truth is [`../../docs/frontend-product-architecture.md`](../../docs/frontend-product-architecture.md). The visual direction may remain reference input; route and workspace decisions are superseded.

Frozen on: 2026-07-23

Accepted artifact: `step-0/primary` — **Baseline dashboard + monitor**

Harness decision: `selected`

## Accepted direction

Implement Codex Pool as a conventional, maintainable dashboard and monitoring product. The member experience makes gateway availability, setup, model discovery, and personal usage immediately understandable. Operator diagnostics and administration are visibly separate and backend-authorized.

The accepted default theme is a Fitonist-inspired dark bento system on a full-viewport product canvas:

- near-black application canvas and charcoal cards;
- white metric hierarchy and cool-gray support text;
- warm coral-red primary accent (`#f87171`) with deep red chart gradient (`#b91c1c`);
- green health/trends (`#4ade80`), yellow comparison/warnings (`#fde047`), and cyan information (`#22d3ee`);
- pill controls, rounded bento cards, subtle inner highlights, restrained shadows/glow, and dotted chart fields;
- smooth dual-spline time-series charts with red primary area/line, yellow comparison line, minimal axes, active marker, nodes, and tooltip;
- textured capsule bars only for discrete comparisons.

The application has no decorative page background behind it and no inset/floating outer frame.

## Frozen interaction architecture

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

Dashboard summarizes. Monitor diagnoses. Administration configures.

### Responsive behavior

- Desktop uses a persistent, divider-collapsible sidebar and full dashboard composition.
- Tablet portrait retains a reduced dashboard hierarchy.
- Tablet landscape is a secondary-screen focus view with a collapsed rail, four role-scoped readouts, freshness, and one dominant live graph.
- Member mobile uses a personal focused dashboard.
- Operator mobile starts in focused Monitor.
- Portrait and landscape mobile share the same focused composition and fit above fixed navigation.

## Frozen product constraints

- Canonical language is `GatewayUser → Provider → ProviderConnection → ModelRoute`.
- The front page is a concise dashboard, not the complete operations console.
- Member and operator navigation are separate; presentation context never grants authority.
- Unknown runtime providers render safely with neutral presentation.
- Unknown, zero, stale, unavailable, measured, and inferred remain distinct.
- Backend projections own provider accounting, quota, health, evidence, and routing semantics.
- Themes cannot alter workflows, authorization, validation, semantics, or accessibility.
- Responsive, keyboard, partial-failure, and last-known-good behavior are product requirements.
- Nonfunctional controls do not appear.
- Desktop has one visible profile control; compact layouts have one visible profile control.
- The legacy staging client remains available until parity and soak criteria pass.

## Implementation authorization boundary

Owner acceptance authorizes production implementation planning and incremental work on `feat/ui-ux-redesign`.

It does not authorize:

- production deployment or restart;
- staging rebuild or replacement;
- removal of the legacy Signal Room;
- real credential/provider operations from design artifacts;
- bypassing backend access policy;
- silently changing the frozen information architecture or default theme.

Material changes to the frozen direction require a recorded design decision or a new review round. Production migration remains a separate explicit release decision.
