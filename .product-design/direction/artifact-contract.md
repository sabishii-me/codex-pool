# Codex Pool mounted artifact contract

Status: draft for Step 0

This contract extends the Product Design Harness artifact boundary for Codex Pool.

## Ownership boundary

The Step 0 artifact contains only a fictional simulated Codex Pool experience. Product Design Harness owns mounting, viewport/runtime selection, inspection, comments, ratings, screenshots, reports, and review persistence. None of those controls may appear inside the artifact.

## Runtime profiles

- `member` simulates an ordinary signed-in member and shows only member navigation and personal-scope data.
- `operator` simulates an authorized operator and adds a visibly separate Operations navigation group and pool-scope diagnostic access.

These query values select presentation fixtures only. They do not grant, infer, or broaden authority.

## Content safety

- Use only the fictional data in `fixtures.json` or clearly fictional derivatives.
- Do not include real names, emails, provider connection IDs, API keys, session tokens, endpoints, host paths, quotas, spend, or incidents.
- Provider and model product names may be used as categorical labels, but all operational values are fictional.
- Do not copy credential or setup secrets from any deployed environment.

## Technical boundary

The artifact must:

- load only relative HTML, CSS, and JavaScript beneath its registered directory;
- use semantic landmarks, headings, buttons, links, tables/lists, and status text;
- expose stable product-oriented attributes such as `data-product-page`, `data-product-action`, and `data-product-region` where useful for review feedback;
- respond to actual iframe dimensions without scaling;
- keep root horizontal overflow controlled and place unavoidable scrolling on named product regions;
- simulate navigation, time range, dismissible details, and resource-state changes only in memory;
- reset all simulated state on reload;
- provide visible keyboard focus and respect reduced motion.

The artifact must not use:

- `fetch`, XMLHttpRequest, WebSocket, EventSource, or external assets;
- localStorage, sessionStorage, IndexedDB, cookies, service workers, or other persistence;
- parent/top/opener frame access;
- real authentication, OAuth, provider, filesystem, shell, clipboard, microphone, or host operations;
- harness, inspector, feedback, rating, screenshot, candidate, or viewport controls.

A copy action may visually simulate success but must not invoke the real clipboard.

## Step 0 scope

Step 0 must make these decisions reviewable:

- conventional application shell;
- member/operator navigation separation;
- dashboard information hierarchy;
- dashboard versus Monitor boundary;
- one representative member route and one operator route;
- baseline visual tokens and component vocabulary;
- desktop, tablet, portrait mobile, and landscape mobile behavior;
- ready, degraded/stale, empty, and localized-error presentation.

Step 0 does not need production-complete setup, OAuth, MFA, connection mutation, routing configuration, economics, or member administration flows. Those remain explicit parity obligations in `docs/ui-redesign-plan.md`.

## Acceptance boundary

A valid artifact and passing automated checks mean only that Step 0 is reviewable. The harness must not mark the direction frozen or passed. Only explicit owner acceptance can authorize freezing the interaction architecture or creating divergent candidates.
