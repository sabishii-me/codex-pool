# Environment data migration

The reusable operational procedure for image promotion, migration review/apply, validation, and rollback is tracked at `.claude/skills/codex-pool-release/SKILL.md`. This document defines the underlying store-ownership contract.

Deployments promote an immutable image through the three environments:

```text
Test (18991) -> Staging (18990) -> Production (8989)
```

A deployment never copies a destination data directory. The destination keeps its own persistent state and the new binary performs additive schema migration in place.

## Store ownership

| Store | Rule |
|---|---|
| `analytics.db/usage_events` | Canonical immutable usage ledger. Merge real upstream usage from any environment only by `(connection_id, request_id)` with full-payload conflict detection and source provenance. Environment-local authentication does not make real provider consumption synthetic. |
| `request_costs`, `daily_costs` | Derived projections. Rebuild from `usage_events`; never merge independently. |
| `proxy.db` | Compatibility/cache state. Never file-merge between environments. |
| `pool_users.json` | Environment authorization state. Production-authoritative only during an explicit sanitized refresh. Test synthetic users never move forward. |
| `admin_mfa.json` | Security state. Production-authoritative only during an explicit sanitized refresh; never union secrets. |
| `pool/` | Provider credentials and connection state. Production-authoritative only during an explicit sanitized refresh; never merge stale refresh tokens. |
| OAuth sessions/cookies | Transient and environment-local. Never copy or merge. |
| Fingerprints/quota/cache files | Environment-local runtime state. Never copy as accounting truth. |

## Usage migration command

`usage-migrate` is read-only unless `--apply` is supplied. Apply requires an offline target and an explicit rollback directory.

Dry run:

```powershell
codex-pool usage-migrate `
  --target staging/data/analytics.db `
  --source path/to/source-analytics.db `
  --target-environment staging `
  --source-environment staging-recovered-YYYYMMDD `
  --report path/to/dry-run.json
```

Apply after reviewing a conflict-free report:

```powershell
# Stop only the destination environment first.
codex-pool usage-migrate `
  --target staging/data/analytics.db `
  --source path/to/source-analytics.db `
  --target-environment staging `
  --source-environment staging-recovered-YYYYMMDD `
  --backup-dir staging/backups/usage-migration-YYYYMMDD-HHMMSS `
  --report staging/backups/usage-migration-apply.json `
  --apply
```

The command:

1. hashes source and target;
2. compares every canonical event;
3. skips exact duplicates;
4. rejects any reused identity with different payload fields;
5. rejects source rows lacking canonical identity;
6. creates a consistent rollback database with SQLite `VACUUM INTO`;
7. labels existing and imported events with source provenance;
8. inserts missing events transactionally;
9. rebuilds `request_costs` and `daily_costs` from the canonical ledger;
10. records an idempotency manifest in `usage_migrations`.

Staging Compose additionally normalizes bind-mounted state ownership for the unprivileged `codex` runtime user before starting the gateway. This is required after host-side snapshots or migrations because SQLite must be able to create `analytics.db-wal` and `analytics.db-shm` as `codex`. A readable main database alone is not sufficient for WAL mode.

Running the same apply twice is a no-op and does not create a second rollback snapshot.

## Conflict handling

Conflicts are never automatically resolved. In particular, do not rewrite `user_id` to make two records compare equal. A conflict report must be reviewed against request provenance and authentication evidence. If attribution cannot be proven, keep the source quarantined.

## Optional Windows production merge and deployment automation

For installations that use this repository's Windows Docker Compose topology,
canonical Test/Staging ledgers, Pi acceptance client, and Task Scheduler, use
`scripts/deploy-production.ps1` for a reviewed Production merge, promotion, or
machine relocation. Other installation topologies should use their own deployment
workflow rather than assuming these environment names or Windows facilities.
Do not reproduce this workflow as ad-hoc shell commands once it has been selected.

The script is self-contained after launch. It does not depend on an LLM, terminal
session, or API request remaining connected. It writes atomic phase state and local
evidence, validates native programs by exit code rather than stderr text, and
rolls back the matching complete Production-owned state and image after any
post-stop failure.

### Safety and downtime model

1. Verify the immutable image, OCI revision, all source runtimes, Compose files,
   destination URL, and existing Pi provider configuration.
2. Take consistent SQLite online snapshots and simulate the complete ordered merge
   while Production remains available.
3. For relocation, transfer and verify the immutable image before downtime.
4. Stop Test and Staging only long enough to freeze the final canonical source
   ledgers, then restart and verify both exact accepted images immediately.
   Production remains available throughout this source freeze.
5. Stop Production only after final source inputs are ready and Test/Staging are
   healthy again.
6. Capture a complete stopped Production rollback archive, including data, pool,
   provider specifications, Compose, and environment configuration.
7. Apply Test then Staging canonical usage through `usage-migrate`, repeat both
   applies as idempotency checks, and run SQLite integrity/uniqueness checks.
8. Start the exact image at the destination and validate the actual configured URL,
   signed-out API boundary, image/revision, Pi inference, and a durable canonical
   accounting write.
9. Any failure after Production stops first checks whether source Production is
   already healthy and refuses an unnecessary recreate. Otherwise it restores the
   prior matching Production image/state. Test/Staging restart is idempotent and
   occurs before Production downtime, not after destination acceptance.

A global local mutex prevents concurrent deployments. No script path modifies Pi
configuration. The named Pi provider must already point exactly to the destination
endpoint, otherwise preflight fails before downtime.

### Configuration

Copy `scripts/deploy-production.config.example.json` to an ignored
`.local.production-deploy*.json` file and fill in the exact artifacts and endpoints.
Do not commit local hostnames, SSH identity paths, Pi profile paths, or deployment
configuration. Normal in-place operation uses:

```json
{
  "Mode": "InPlace",
  "AcceptedImage": "codex-pool:test-<commit>",
  "AcceptedRevision": "<OCI revision>",
  "TargetEndpoint": "http://<production-lan-ip>:8989",
  "TargetPublicURL": "http://<production-lan-ip>:8989",
  "TargetOAuthRedirectURI": "http://<production-lan-ip>:8989/auth/callback/google",
  "ConflictPolicy": "Fail",
  "PiProvider": "REPLACE_PI_PROVIDER",
  "PiModel": "gpt-5.6-sol"
}
```

A relocation additionally requires:

```json
{
  "Mode": "Relocate",
  "TargetHost": "ssh-user@new-production-host",
  "TargetSSHIdentityFile": "C:\\Users\\REPLACE_USER\\.ssh\\target-key",
  "TargetBootstrap": false,
  "TargetRoot": "/srv/codex-pool",
  "TargetEndpoint": "http://new-production-host:8989",
  "TargetPublicURL": "http://new-production-host:8989",
  "TargetOAuthRedirectURI": "http://new-production-host:8989/auth/callback/google",
  "PiEndpointSwitchMode": "UpdateOnCutover"
}
```

A relocation with `PiEndpointSwitchMode: UpdateOnCutover` first validates the new
host using an isolated temporary Pi profile through `PI_CODING_AGENT_DIR`. Only
after that passes does it atomically update the selected real Pi provider URL and
retest through the normal Pi profile. The original `models.json` is preserved in
evidence and restored automatically if the cutover rolls back. `ValidateOnly`
never edits Pi and requires the provider to already point exactly at the target.

Relocation transfers the complete **Production-owned** authority state only after
all source writers stop. This is not permission to copy Test/Staging credentials,
users, MFA, OAuth, or provider state. The target root must be empty of Production
state; the script refuses to overwrite an existing authority.

Conflict policy defaults to `Fail`. `TestWinsUserIdOnly` is available only for an
explicitly reviewed recurrence of duplicate canonical identities whose sole payload
difference is `user_id`; every quarantined identity is written to evidence. It is
never selected implicitly.

### Trigger and monitor

Run non-destructive preflight in the foreground:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File scripts/deploy-production.ps1 `
  -ConfigPath .local.production-deploy.json
```

Run the complete online snapshot/conflict/simulation path without stopping or
changing any gateway:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File scripts/deploy-production.ps1 `
  -ConfigPath .local.production-deploy.json `
  -PrepareOnly
```

Launch the complete autonomous operation and return immediately:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File scripts/start-production-deployment.ps1 `
  -ConfigPath .local.production-deploy.json `
  -Execute `
  -Confirmation DEPLOY-PRODUCTION-WITH-CANONICAL-USAGE
```

Read status without changing anything:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File scripts/deploy-production-status.ps1
```

Success requires `COMPLETED.json`; failure writes `FAILED.json`. The transcript,
process stdout/stderr, migration reports, snapshots, integrity reports, quarantine
evidence, Pi output, accounting evidence, and rollback manifest are under
`data/backups/production-deploy-<UTC>/`.

## Promotion procedure

1. Build and validate an immutable image in Test.
2. Back up Staging state.
3. Deploy the same image to Staging without replacing Staging data.
4. Run required schema/data migrations against Staging in dry-run mode.
5. Stop Staging, apply reviewed migrations, restart, and validate authentication, integrity, and accounting.
6. Repeat the same image and migration sequence for Production only after Staging approval.
7. Merge canonical real-account usage from Test or Staging only through reviewed migration reports. Preserve source provenance, quarantine rows without canonical identity, and never copy Test authorization or credential state forward.
