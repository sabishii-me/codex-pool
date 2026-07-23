# Codex Pool primary Step 0 interpretation

Status: reviewable draft — owner acceptance required

## Purpose

This single artifact tests the Phase 9 interaction architecture before visual alternatives are created:

- conventional application shell;
- separate member and operator navigation;
- concise front-page dashboard;
- detailed operator Monitor;
- representative Models experience;
- neutral professional baseline presentation;
- responsive desktop, tablet, portrait-mobile, and landscape-mobile hierarchy;
- localized resource failure and in-memory recovery.

It does not claim full legacy parity or final visual approval. Setup, My usage, Provider connections, Model routes, and System are navigation placeholders so their placement can be reviewed without pretending their workflows are complete.

## Runtime profiles

- `member` — member workspace, personal metric scope, no operator navigation.
- `operator` — adds a separate Operations group, degraded-capacity example, pool metric scope, and Monitor.

Runtime profiles are presentation-only fixtures and do not authorize anything.

## Interactions

- Navigate Dashboard, Models, Monitor, and placeholder routes.
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
