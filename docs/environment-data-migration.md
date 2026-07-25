# Environment data migration

Deployments promote an immutable image through the three environments:

```text
Test (18991) -> Staging (18990) -> Production (8989)
```

A deployment never copies a destination data directory. The destination keeps its own persistent state and the new binary performs additive schema migration in place.

## Store ownership

| Store | Rule |
|---|---|
| `analytics.db/usage_events` | Canonical immutable usage ledger. Merge only by `(connection_id, request_id)` with full-payload conflict detection. |
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

Running the same apply twice is a no-op and does not create a second rollback snapshot.

## Conflict handling

Conflicts are never automatically resolved. In particular, do not rewrite `user_id` to make two records compare equal. A conflict report must be reviewed against request provenance and authentication evidence. If attribution cannot be proven, keep the source quarantined.

## Promotion procedure

1. Build and validate an immutable image in Test.
2. Back up Staging state.
3. Deploy the same image to Staging without replacing Staging data.
4. Run required schema/data migrations against Staging in dry-run mode.
5. Stop Staging, apply reviewed migrations, restart, and validate authentication, integrity, and accounting.
6. Repeat the same image and migration sequence for Production only after Staging approval.
7. Never merge Test usage into Staging or Production.
