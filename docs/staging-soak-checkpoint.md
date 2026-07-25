# Staging long-soak checkpoint

Updated: 2026-07-25

## Active release under soak

| Item | Value |
|---|---|
| Environment | Staging |
| Endpoint | `http://127.0.0.1:18990` |
| Compose project | `codex-pool-staging` |
| Image | `codex-pool:staging-0291cc0` |
| Image revision | `0291cc0bbbb2` |
| Authentication | Real Google OAuth; `LOCAL_DEV_SESSION=false` |
| OAuth callback | `http://127.0.0.1:18990/auth/callback/google` |
| Persistent state | `staging/pool`, `staging/data`, `staging/provider-specs` |
| Canonical events after recovery | 5,285 total; 5,284 with canonical connection/request identity |
| Recovered Staging events | 1,158 with provenance `staging-recovered-20260725` |
| Native/current events at migration checkpoint | 4,127 with provenance `staging` |
| Migration ID | `e04c833ea39a52ce0ceb6a5ad36e8d2a` |
| Migration version | `usage-event-merge-v1` |
| Migration commit | `f7cc2e0` |
| Analytics permission fix | `cc25f3f` |
| Rollback snapshot | `staging/backups/usage-migration-20260725-104427/` |
| Apply report | `staging/backups/usage-migration-apply-20260725-104427.json` |
| Idempotency report | `staging/backups/usage-migration-repeat-20260725-104427.json` |

The backup/report paths are local ignored operational state, not repository artifacts.

## Entry validation

Passed at soak start:

- Staging container healthy on `18990`.
- Signed-out session returns `401`.
- Google OAuth entry returns `302` to Google with the Staging callback.
- Three real GatewayUsers loaded; no synthetic GatewayUser exists in Staging.
- Eight provider connections loaded from isolated Staging state.
- `PRAGMA integrity_check` returns `ok`.
- Usage migration dry run reported zero source-invalid rows and zero payload conflicts.
- Apply inserted 1,158 archived Staging events.
- Repeating the same apply inserted zero events and created no second backup.
- `/api/pool/signal` returns `200` with economics, hourly usage, and model-demand projections.
- SQLite WAL/SHM files are created by the unprivileged `codex` user after a full Compose recreate.
- Direct image reading through Pi/Test and the Anthropic Messages gateway path is proven.
- Typed Anthropic context overflow and complete cache-aware usage are covered by contracts.
- Sol image-generation event streams are preserved.

## Soak acceptance matrix

Record evidence; do not mark a row complete from memory.

| Area | Required evidence | Status |
|---|---|---|
| Runtime availability | health checks across the entire soak; no unexplained restart | In progress |
| Signed-out/auth | login shell, Google redirect/callback, allowlist denial, sign-out | In progress |
| Member product | Home, Models, Usage, Setup, Profile direct routes desktop/mobile | In progress |
| Admin gate | locked Admin challenge, cancel behavior, successful elevation, expiry | In progress |
| Connections | inventory, detail, refresh, enable/disable/recover, localized errors | In progress |
| Members | real identities, create token, enable/disable, no synthetic member | In progress |
| System | runtime/build/persistence/registry truth and working operations | In progress |
| Usage/economics | personal/pool/member scopes, model/provider/connection attribution, curves | In progress |
| Exactly-once accounting | one canonical event per request, restart reconciliation, no duplicate IDs | In progress |
| Setup | Codex, Claude, Gemini, Grok, Pi generated from backend registry | In progress |
| Protocol errors | typed buffered/streaming overflow; no assistant-text errors | In progress |
| Images | image generation and Pi read-tool image result path | In progress |
| Restart recovery | graceful restart, auth remains valid as intended, analytics returns | In progress |
| Responsive UX | desktop/mobile direct route review; no release-blocking visible defect | In progress |
| Runtime cleanliness | no unexpected browser exceptions, server 5xx, SQLite errors, or projection alerts | In progress |

## Recurring checks

Health and restart state:

```powershell
curl.exe http://127.0.0.1:18990/healthz
docker inspect codex-pool-staging-codex-pool-staging-1 --format '{{.RestartCount}} {{.State.StartedAt}} {{.State.Health.Status}}'
```

Error scan:

```powershell
docker logs --since 24h codex-pool-staging-codex-pool-staging-1 2>&1 |
  Select-String -Pattern 'panic|fatal|unable to open database|failed to build|persistence failed|database is locked|corrupt'
```

Database checks must use an online snapshot or occur while Staging is stopped. Do not inspect a live bind-mounted SQLite file from the host as a durability test.

Browser acceptance must use Staging authentication/configuration. The Test Playwright fixture signs cookies using Test state and is not valid evidence when pointed unchanged at Staging.

## Incident record

For each incident preserve:

- UTC and local timestamp;
- current Staging image tag, digest, and OCI revision;
- browser/client and model;
- signed-out/member/locked Admin/elevated Admin state;
- route or gateway protocol;
- exact action and expected result;
- visible error and HTTP status;
- gateway request ID and relevant logs;
- screenshot/trace where applicable;
- canonical usage identity `(connection_id, request_id)` for accounting incidents;
- whether retry, refresh, or restart changed the result;
- classification and disposition.

Classify incidents as:

1. authentication/session;
2. authorization/MFA;
3. provider routing/quota;
4. gateway protocol/translation;
5. canonical usage/accounting;
6. analytics/economics projection;
7. persistence/migration;
8. frontend runtime/routing;
9. responsive/visual polish;
10. environment/deployment configuration.

## Promotion decision

Production promotion remains blocked while any of these are unresolved:

- data loss, attribution uncertainty, duplicate canonical usage, or failed reconciliation;
- typed provider failures converted to successful assistant content;
- incomplete Anthropic usage reported as complete;
- OAuth, member, Admin, or MFA access-control defect;
- missing/fabricated operational data;
- recurring SQLite/WAL, persistence, projection, or restart failure;
- release-blocking desktop/mobile runtime defect;
- image digest differs from the Staging-accepted artifact.

Non-blocking visual polish may be deferred only when it does not hide data, prevent an operation, misrepresent state, or break responsive use.

## Known remaining product work

- Complete Profile authenticator rotation and recovery-code regeneration controls.
- Run the full Staging-specific authenticated browser matrix and preserve evidence.
- Continue long-soak observation before Production promotion.
- Perform broad accessibility/performance/visual polish after functional blockers are closed.
