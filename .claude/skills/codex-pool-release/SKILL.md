---
name: codex-pool-release
description: Safely build, qualify, promote, migrate, validate, and roll back codex-pool across Test, Staging, and Production.
---

# Codex Pool release and data migration

Use this skill when asked to deploy, promote, migrate environment data, prepare a maintenance window, roll back, or hand off a release for `codex-pool`.

Read these repository contracts before acting:

- `docs/environment-data-migration.md`
- `docs/staging-soak-checkpoint.md`
- the applicable `docs/production-deployment-*.md`
- `scripts/build-release-image.ps1`
- `docker-compose.dev.yml`, `docker-compose.staging.yml`, and `docker-compose.yml`

## Non-negotiable invariants

1. There are exactly three runtimes:
   - Test: `http://127.0.0.1:18991`
   - Staging: `http://127.0.0.1:18990`
   - Production: `http://localhost:8989`
2. Promotion only moves `Test -> Staging -> Production`.
3. Build once. Promote the exact accepted immutable image ID/digest; never rebuild between environments.
4. Verify `org.opencontainers.image.revision` against the intended Git commit before every promotion.
5. Never create a candidate runtime, alternate port, or fourth environment.
6. Never copy an entire `data/`, `pool/`, provider-state, OAuth-session, cache, user, MFA, or credential directory between environments.
7. Destination state stays mounted in place. Ordinary additive schema migration is performed by the promoted binary at destination startup.
8. Canonical usage is the only cross-environment data eligible for a merge, and only through `usage-migrate` using `(connection_id, request_id)` identity, provenance, exact-duplicate skipping, and full-payload conflict rejection.
9. Never rewrite `user_id`, request identity, or payload fields to force a merge. Quarantine and review unresolved rows.
10. Derived `request_costs` and `daily_costs` are rebuilt from `usage_events`; never merge them independently.
11. Never expose or print values from local environment files, container environments, OAuth credentials, provider files, pool tokens, or MFA state.
12. Push only to `origin`, never `upstream`.
13. Production changes and any `usage-migrate --apply` require explicit user approval and a maintenance/rollback plan. A request to deploy Staging is not approval to migrate data or deploy Production.
14. Preserve Test `analytics.db` and WAL state until testing is explicitly complete and canonical usage migration is approved.

## Classify the requested operation

Before running commands, state which operation is being performed:

- **Build**: create one immutable image from a clean commit.
- **Test deploy**: recreate only Test with that image and existing Test mounts.
- **Staging promotion**: recreate only Staging using the exact accepted Test image.
- **Production promotion**: recreate only Production using the exact Staging-accepted image after explicit approval.
- **Ordinary schema migration**: destination binary updates its own destination state in place on startup.
- **Canonical usage migration**: reviewed merge of a consistent source snapshot into an offline destination ledger.
- **Recovery**: exceptional forensic repair; do not treat a prior one-off recovery decision as reusable merge policy.
- **Rollback**: restore both the prior image and the matching destination-state snapshot when state changed.

If the request is ambiguous, stop and ask whether it authorizes image promotion, usage migration, and/or Production maintenance.

## Phase 1: preflight

From the repository root:

```powershell
git status --short
git branch --show-current
git log -5 --oneline
git remote -v
```

Require a clean working tree before building. Confirm the intended commit and ensure any push target is `origin`.

Inspect all three runtimes without printing environment variables:

```powershell
docker compose --env-file .env.dev -p codex-pool-dev -f docker-compose.dev.yml ps
docker compose --env-file .env.staging.local -p codex-pool-staging -f docker-compose.staging.yml ps
docker ps --filter publish=8989
```

Record, for the source and destination:

- container name;
- image tag and image ID/digest;
- OCI revision;
- health and restart count;
- Compose project/file;
- public endpoint;
- destination mount paths (paths only, never contents or secrets).

Abort if the destination mounts point to another environment, the expected image is missing, the revision label is wrong, or an unrelated runtime would be changed.

## Phase 2: build one immutable artifact

Frontend and Go validation must remain sequential because Go embeds `web/dist`.

```powershell
$commit = (git rev-parse --short=7 HEAD).Trim()
powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File scripts/build-release-image.ps1 `
  -Tag "codex-pool:test-$commit"
```

Do not use `-SkipTests` for a release candidate. The build script requires a clean tree and validates the OCI revision label.

Inspect the result:

```powershell
docker image inspect "codex-pool:test-$commit" `
  --format 'id={{.Id}} revision={{index .Config.Labels "org.opencontainers.image.revision"}}'
```

Record the immutable image ID/digest. A mutable tag alone is not promotion evidence.

## Phase 3: deploy and qualify Test

Test must use `.env.dev` interpolation and `docker-compose.dev.yml`. A temporary image-only override may select the immutable artifact; it must not change ports, mounts, environment policy, or behavior.

```powershell
@"
services:
  codex-pool-dev:
    image: codex-pool:test-<commit>
"@ | Set-Content .tmp-test-image.override.yml

try {
  docker compose --env-file .env.dev -p codex-pool-dev `
    -f docker-compose.dev.yml -f .tmp-test-image.override.yml `
    up -d --no-build --force-recreate codex-pool-dev
} finally {
  Remove-Item .tmp-test-image.override.yml -ErrorAction SilentlyContinue
}
```

Wait for `healthy`; do not validate immediately after recreation. Then verify:

- `GET http://127.0.0.1:18991/healthz` returns `200`;
- protected APIs return `401` while signed out;
- the running image ID and OCI revision match the built artifact;
- authentication/OAuth entry remains truthful;
- real authenticated acceptance uses a real Test session, not synthetic cookies;
- affected member/admin routes work at desktop and mobile widths;
- affected provider operations use Test-owned credentials only;
- usage/accounting writes remain exactly once;
- logs contain no panic, fatal, SQLite corruption/locking, or projection failures.

Tests and source assertions do not replace browser-visible or real-state acceptance.

## Phase 4: promote the exact image to Staging

Do not rebuild. Do not copy Test state. Confirm Staging approval, then:

```powershell
$env:STAGING_IMAGE = 'codex-pool:test-<commit>'
docker compose --env-file .env.staging.local -p codex-pool-staging `
  -f docker-compose.staging.yml `
  up -d --no-build --force-recreate
```

The `staging-data-permissions` one-shot service normalizes ownership of Staging bind mounts for the unprivileged runtime. It does not import Test data.

Wait for health and verify:

```powershell
curl.exe -f http://127.0.0.1:18990/healthz
docker inspect codex-pool-staging-codex-pool-staging-1 `
  --format 'image={{.Config.Image}} health={{.State.Health.Status}} revision={{index .Config.Labels "org.opencontainers.image.revision"}}'
```

Also prove Production's container/image/health did not change. Complete Staging-specific real authentication, MFA, provider, accounting, responsive UI, restart, and soak acceptance. Test-signed cookies are not Staging evidence.

## Phase 5: snapshot rules before canonical usage migration

Image promotion does **not** imply usage migration. Perform this phase only when explicitly approved.

A migration source must be a consistent SQLite snapshot. Acceptable approaches are:

- SQLite online backup/VACUUM snapshot made through a supported tool while the owner is online; or
- stop the owning environment and capture `analytics.db`, `analytics.db-wal`, and `analytics.db-shm` together before creating the migration snapshot.

Never treat a host copy of only a live `analytics.db` file as consistent WAL evidence. Never mutate the source snapshot.

Create a timestamped ignored evidence directory and record sanitized metadata:

- UTC timestamp;
- source environment and snapshot method;
- source hash;
- source row count and canonical/noncanonical counts;
- destination image/revision before maintenance;
- destination health before maintenance;
- planned rollback paths.

Do not put databases, credentials, secrets, or raw private events in Git.

## Phase 6: canonical usage dry run

Use the built `codex-pool` binary containing `usage-migrate`, or run the exact accepted image with explicit file mounts. The following shows the binary form:

```powershell
codex-pool usage-migrate `
  --target <destination>/data/analytics.db `
  --source <consistent-source-snapshot>.db `
  --target-environment <destination-name> `
  --source-environment <stable-source-provenance> `
  --report <evidence>/usage-migration-dry-run.json
```

Review the report. Required gates:

- `source_invalid_events == 0` unless invalid rows were explicitly quarantined outside the migration source;
- `conflicting_events == 0`;
- insertable and identical counts reconcile with source canonical rows;
- source/target paths and environment labels are correct;
- source hash matches the reviewed snapshot;
- no identity or attribution was rewritten.

Any payload conflict, uncertain attribution, source mutation, or count mismatch stops the migration. Preserve the report and escalate for review.

## Phase 7: apply canonical usage migration

Applying requires the destination gateway to be offline. Announce maintenance and stop only the destination service. Capture rollback state before modifying it.

For Staging:

```powershell
$env:STAGING_IMAGE = '<exact-accepted-image>'
docker compose --env-file .env.staging.local -p codex-pool-staging `
  -f docker-compose.staging.yml stop codex-pool-staging
```

For Production, use its actual Compose project and require explicit Production approval. Capture the stopped destination `analytics.db`, WAL, and SHM plus image/container inspection and checksums in a timestamped rollback directory. Do not remove WAL/SHM before this capture.

Apply only the reviewed, conflict-free snapshot:

```powershell
codex-pool usage-migrate `
  --target <destination>/data/analytics.db `
  --source <reviewed-source-snapshot>.db `
  --target-environment <destination-name> `
  --source-environment <stable-source-provenance> `
  --backup-dir <destination>/backups/usage-migration-<UTC> `
  --report <destination>/backups/usage-migration-apply-<UTC>.json `
  --apply
```

`--apply` must:

- obtain an exclusive target lock and reject an online destination;
- create a consistent rollback database;
- skip exact duplicates;
- reject conflicts/noncanonical source rows;
- insert missing events transactionally;
- preserve provenance;
- rebuild derived projections;
- write an idempotency manifest.

Run the same apply a second time and require a no-op (`already_applied` or zero inserts) with no second effective migration.

Before restart, validate SQLite integrity and canonical uniqueness using a consistent offline connection/snapshot. Reconcile:

```text
target_after = target_before + inserted_events
unique canonical identities = canonical usage-event rows
```

Preserved historical noncanonical rows must be reported separately, never counted as canonical.

## Phase 8: restart and validate destination

Restart with the exact accepted image and existing destination mounts. Never install a prepared whole data directory.

Validate:

- healthy endpoint;
- running image ID/digest and OCI revision;
- signed-out protected API behavior;
- real destination authentication and MFA;
- provider inventory and connection ownership;
- provider-spec loading;
- frontend artifact/version;
- personal, pool, and member usage scopes;
- provider/model/connection totals from canonical events;
- exactly one canonical event for each validation request;
- SQLite WAL/SHM write access by the unprivileged runtime;
- restart count and error logs;
- no changes to the next environment in the promotion chain.

Do not claim authenticated acceptance if no real destination session was available. Mark it pending.

## Phase 9: Production promotion

Production promotion is a separate approved operation after Staging acceptance and soak. Before changing Production:

1. Confirm the Staging-running image ID/digest and revision.
2. Confirm the requested Production artifact is that exact image.
3. Record the current Production image as rollback artifact.
4. Capture destination state according to whether data will change.
5. If usage migration is required, stop Production and perform Phases 5–8.
6. If no usage migration is required, preserve all Production mounts and recreate only the Production service with the accepted image.
7. Never use an unreviewed `docker compose up --build` in Production.
8. Validate real authentication, MFA, provider operations, typed failures, exactly-once accounting, and database integrity.

Because `docker-compose.yml` currently names `codex-pool:latest`, first tag the exact accepted image ID with a timestamped immutable Production rollback/promotion tag, record both IDs, and ensure `latest` resolves to that same accepted ID before recreation. Never rebuild `latest` during promotion.

## Rollback

Rollback scope follows what changed:

- Image-only failure: restore the previous destination image while retaining destination state only if schema/state remains backward compatible and verified.
- Image plus data migration failure: stop destination, preserve failed-state evidence, restore the matching pre-migration database snapshot (including WAL handling while offline), restore the previous image, normalize ownership, restart, and validate.
- Never roll back by copying another environment's directories.

Keep rollback artifacts until the soak/maintenance window is formally closed.

## Required deployment report

End every operation with a sanitized report containing:

- operation performed and explicit scope not performed;
- Git commit;
- image tag, immutable ID/digest, and OCI revision;
- source and destination environment;
- health/HTTP checks;
- authentication acceptance status (real, pending, or not applicable);
- migration report path and counts, if applicable;
- rollback artifact path, if applicable;
- proof adjacent environments were untouched;
- unresolved gates before the next promotion.

Never include secrets, raw credentials, cookies, OAuth values, private event payloads, or environment-file contents.
