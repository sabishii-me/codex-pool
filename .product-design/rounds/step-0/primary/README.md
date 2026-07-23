# Codex Pool primary Step 0 interpretation

Status: reviewable draft — owner acceptance required

## Purpose

This single artifact tests the Phase 9 interaction architecture before visual alternatives are created:

- conventional application shell;
- separate member and operator navigation;
- concise front-page dashboard;
- detailed operator Monitor;
- representative Models and My usage experiences, including a privacy-conscious weekly leaderboard;
- accessible collapsible desktop navigation and compact mobile-landscape navigation;
- neutral professional baseline presentation;
- responsive desktop, tablet portrait, tablet-landscape secondary-screen, portrait-mobile, and landscape-mobile hierarchy;
- role-aware compact defaults: operator mobile opens Monitor, member landscape opens My usage, and tablet landscape opens a focused live graph;
- localized resource failure and in-memory recovery.

It does not claim full legacy parity or final visual approval. Setup, Provider connections, Model routes, and System provide representative Step 0 content and interactions so the information architecture can be reviewed; their production workflows remain part of later tracked phases.

## Runtime profiles

- `member` — member workspace, personal metric scope, no operator navigation.
- `operator` — adds a separate Operations group, degraded-capacity example, pool metric scope, and Monitor. Compact operator profiles start in Monitor; tablet landscape uses a pool-health focus view.

Runtime profiles are presentation-only fixtures and do not authorize anything.

## Interactions

- Navigate Dashboard, Models, My usage, Setup, Monitor, Provider connections, Model routes, and System.
- Review role-aware device defaults: desktop Dashboard, operator mobile Monitor, member mobile landscape My usage, and tablet-landscape live focus.
- Collapse and expand the desktop sidebar for the current in-memory preview session.
- Review a fictional weekly leaderboard with abbreviated member labels and aggregate processed-token totals.
- Change dashboard activity range.
- Search and filter model routes.
- Simulate copying a model ID without using the clipboard.
- Open mobile navigation.
- In Monitor, simulate a localized persistence-health error and retry it.

All state is in memory and resets on reload. The artifact performs no network or real product operations.

## Owner review questions

1. Does the dashboard communicate gateway state and next action within ten seconds?
2. Is the separation between member workspace and Operations clear?
3. Is Monitor the right home for current Signal Room diagnostic depth?
4. Is the baseline sufficiently conventional and professional?
5. Does mobile keep ordinary member work primary while preserving operator reachability?
6. What must change before this interaction architecture can be frozen?
