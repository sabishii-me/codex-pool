# Current engineering handoff

Updated: 2026-07-28 after Staging promotion of `bd1f276`.

Start the next session by reading:

1. `.claude/skills/codex-pool-release/SKILL.md` for reusable deployment, migration, validation, and rollback procedure.
2. `docs/environment-data-migration.md` for store ownership and canonical usage rules.
3. `docs/staging-soak-checkpoint.md` for the acceptance matrix (its historical active-release table predates the current image; use the runtime state below).
4. `docs/production-deployment-9ffe963.md` for the prior Production recovery/cutover and rollback evidence.

## Repository state

- Repository: the checkout root containing this document
- Branch: `feat/ui-ux-redesign`
- Product release commit: `bd1f27622f8b`
- Product release subject: `fix: compose usage curves with fixed columns`
- Push only to `origin`, never `upstream`.
- The release-skill/handoff documentation commit follows `bd1f276`; it does not change the product image.

## Runtime state

| Environment | Endpoint | Running image | OCI revision | State |
|---|---|---|---|---|
| Test | `http://127.0.0.1:18991` | `codex-pool:test-bd1f276` | `bd1f27622f8b` | Healthy |
| Staging | `http://127.0.0.1:18990` | exact same `codex-pool:test-bd1f276` image | `bd1f27622f8b` | Healthy; newly promoted |
| Production | `http://localhost:8989` | `codex-pool:latest` | `9ffe963ddcd1` | Healthy; untouched |

Promoted artifact image ID/digest as reported by Docker:

```text
sha256:57688cb620214dfbca5ad57125d2de6cd9e93f91138f6ffb1342ee34aa5ba5ce
```

At Staging promotion:

- `/healthz` returned `200`;
- signed-out `/api/v2/usage?...` returned `401`;
- the served frontend bundle contained the composed usage-chart artifact;
- Staging retained `staging/pool`, `staging/data`, and `staging/provider-specs`;
- no Test state was copied;
- Production remained on the same healthy container/image.

## What changed in the accepted graph

`MultiSeriesChart` now uses the existing `d3-scale` and `d3-shape` packages rather than custom curve interpolation:

- D3 `curveMonotoneX` model/provider lines;
- stacked usage columns in the same plot;
- fixed `14px` column width;
- fixed pixel plot coordinates measured with `ResizeObserver`;
- no `preserveAspectRatio="none"` in this chart;
- shared D3 scales for columns, lines, axes, and bucket positions.

Validation before image creation passed:

- 8 frontend Vitest files;
- 50 frontend tests;
- TypeScript;
- frontend production build;
- all Go tests;
- `git diff --check`;
- image revision validation;
- Test health and served-artifact check.

The user accepted the graph and explicitly approved Staging deployment.

## 24h usage investigation

Do not "fix" the empty **My usage** range by pooling or merging user identities.

Sanitized real Test-state findings at investigation time:

- one gateway user owned 309 events and 16,956,294 billable tokens in the active 24h window;
- the user represented by the approximately 338M-token 7d screenshot had no events in that 24h window and last activity on July 24;
- the recent activity therefore belonged to another GatewayUser;
- event identity matched a real GatewayUser, so this was not a missing-user or timestamp-query defect.

**My usage** must remain scoped to the authenticated GatewayUser. Elevated **Pool usage** is the authorized place to view all users. If requests were intended to belong to the browser user, correct the client credential ownership rather than rewriting usage attribution.

## Immediate next work

1. Perform real authenticated Staging acceptance with Staging Google OAuth and MFA; Test cookies are not valid Staging evidence.
2. Review desktop/mobile Usage, including the composed graph at 24h/7d/30d and personal/pool scopes.
3. Validate provider connections and Test/Staging-owned operations, especially Codex reset-credit refresh/redemption states.
4. Inspect Staging logs, restart behavior, SQLite/WAL health, and exactly-once accounting during soak.
5. Update `docs/staging-soak-checkpoint.md` with the current image and captured evidence rather than marking rows complete from memory.
6. Do not promote to Production until Staging acceptance/soak is complete and the user explicitly approves Production maintenance.
7. Do not run canonical Test usage migration until testing is explicitly complete and the user approves it. Preserve `dev/data/analytics.db` and WAL state meanwhile.

## Still blocked or unresolved

- Real authenticated Staging acceptance and soak are incomplete.
- Production is far behind the current product image but remains healthy; this is not permission to deploy it.
- Canonical Test usage migration is intentionally pending explicit approval and a maintenance window.
- Reset-credit redemption still lacks a complete durable idempotency design across retry/restart.
- Test has five mounted Codex credentials; the expected sixth is absent and must not be copied or synthesized.
- Test runtime classified four Plus and one Pro while earlier credential-claim inspection classified all five as Plus.
- Historical billable/cost correction awaits verified provider semantics and an approved rebuild policy.
- Z.ai `glm-5.2` historical cost remains unknown.
- `go test -race` is unavailable on Windows without CGO.
- Strict ownership work remains for provider response/conversation/asynchronous-operation state, retries, reconnects, replay, and `cyber_swap_ws.go`.

## Safety reminders

- Exactly three environments; no candidate runtime or fourth port.
- Ordinary behavior must match across Test, Staging, and Production.
- Promote immutable artifacts; migrate destination state in place.
- Never copy whole environment directories or credential/authentication state.
- Merge canonical usage only by `(connection_id, request_id)` with provenance and conflict rejection.
- Never expose local environment values, credentials, cookies, MFA secrets, or provider files in logs or handoff text.
- Provider failures remain typed errors; unknown accounting/quota/pricing remains unknown.
- Production promotion and any `usage-migrate --apply` require explicit approval.
