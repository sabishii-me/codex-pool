# Frozen design prompt

Status: draft — not frozen

Do not launch divergent designers or create additional candidates until the owner has reviewed and explicitly accepted the runnable Step 0 interaction architecture.

## Draft direction under review

Design Codex Pool as a conventional, maintainable dashboard and monitoring product. The default member experience should make gateway availability, setup, model discovery, and personal usage immediately understandable. Operator diagnostics and administration must be clearly separated. Establish a neutral professional baseline first; preserve the existing Signal Room identity later as a theme over the same routes, components, semantics, and tests.

## Fixed product constraints

- Canonical language is `GatewayUser → Provider → ProviderConnection → ModelRoute`.
- The front page is a concise dashboard, not the full operations console.
- Dashboard summarizes; Monitor diagnoses; administration configures.
- Member and operator jobs require separate navigation hierarchy.
- Unknown runtime providers must render safely.
- Unknown, zero, stale, unavailable, measured, and inferred are distinct.
- React does not own provider accounting, quota, health, or routing semantics.
- Themes cannot alter workflows, authority, validation, semantics, or accessibility.
- Responsive, keyboard, partial-failure, and last-known-good behavior are product requirements.
- All design artifacts use fictional in-memory data and perform no real operations.

## Open Step 0 questions

- Is the member/operator workspace separation clear without feeling like two unrelated products?
- Does the dashboard answer the five primary overview questions within ten seconds?
- Is Monitor discoverable without allowing operator density to dominate the front page?
- Are navigation and page patterns conventional enough to maintain while retaining product character?
- Does the mobile hierarchy preserve primary member jobs and keep operator tools reachable?
- Is the neutral baseline strong enough to support a later Signal Room theme without component forks?

Owner acceptance will freeze interaction architecture only. It will not authorize production migration, remove the legacy staging client, or approve every visual detail.
