# Production deployment checkpoint — 9ffe963

Prepared and deployed 2026-07-26. Production was changed only during the explicitly approved maintenance window documented below.

## Release artifact

- Accepted revision: `9ffe963ddcd1c66ef5c8866a6fec8cd6c17f07f4`
- Tested Staging image: `codex-pool:staging-9ffe963`
- Production deployment tag: `codex-pool:production-9ffe963`
- Production `codex-pool:latest` now resolves to the same immutable image ID after the approved cutover.

## Data audit

Consistent Test and Staging snapshots were created with SQLite `VACUUM INTO` while each owning gateway remained online. Production could not create an online snapshot. A stable detached copy of the Production main database reports B-tree corruption in `usage_events` and derived projections. SQLite `.recover` produced an integrity-valid offline recovery candidate without a `lost_and_found` table.

Snapshot directory:

```text
data/backups/prod-prep-20260726T015829Z/
```

The directory is deployment evidence and is intentionally excluded from Git.

Recovered Production:

- total rows: 8,706;
- canonical rows: 8,705;
- preserved legacy noncanonical rows: 1;
- integrity: `quick_check=ok` after forensic recovery;
- latest completion: `2026-07-25T14:12:00.007658431Z`.

Source snapshots:

- Test: 7,261 total rows; 7,259 canonical; 2 quarantined noncanonical rows;
- Staging: 5,911 total rows; 5,910 canonical; 1 quarantined noncanonical row.

## Merge resolution

Canonical real-provider usage is eligible regardless of source environment. Test authentication state is not promoted, but Test requests that consumed real provider accounts belong in global burn history.

Offline simulation:

1. Recover Production to an integrity-valid offline database.
2. Merge canonical Test usage:
   - 3,565 inserted;
   - 3,694 exact duplicates;
   - zero payload conflicts against recovered Production.
3. Compare canonical Staging usage after Test:
   - 1,028 unique insertable events;
   - 4,126 exact duplicates;
   - 756 repeated canonical identities differing only by `user_id`.
4. The 756 Test variants map to the real user `spj@c0dt.app`; the Staging variants map to Test-only `developer@localhost.invalid`. Provider, connection, request, origin, timestamps, model, token dimensions, and cost are identical.
5. Retain the real-user Test variant exactly once and quarantine the Staging attribution variant.
6. Merge the remaining 1,028 Staging events.
7. Rebuild `request_costs` and `daily_costs` from canonical `usage_events`.
8. Repeat both migrations to prove idempotency.

Final offline candidate:

- total rows: 13,299;
- canonical rows: 13,298;
- unique canonical identities: 13,298;
- preserved Production legacy row: 1;
- Production provenance: 8,706;
- Test real-account provenance: 3,565;
- Staging real-account provenance: 1,028;
- integrity: `quick_check=ok`;
- SHA-256: `b469f153a154e2854395010243b5fdc6049c86348cae7ed28ca3b8c821a5ea66`.

Evidence includes:

```text
production-combined-summary.json
test-invalid-quarantine.json
staging-invalid-quarantine.json
staging-attribution-conflict-quarantine.json
test-merge-simulation.json
staging-resolved-merge-simulation.json
test-merge-idempotency.json
staging-merge-idempotency.json
```

## Executed maintenance window

Production was stopped at checkpoint `20260726T023330Z`. The original `analytics.db`, WAL, SHM, container inspection, image inspection, Compose file, health response, and checksums were captured under:

```text
data/backups/production-cutover-20260726T023330Z/
```

The stopped-state recovery matched the prepared ledger exactly. Fresh Test and Staging snapshots were merged with the reviewed attribution rule, projections were rebuilt, and both migrations were repeated as idempotent no-ops. The final installed ledger has:

- 13,299 total rows;
- 13,298 canonical and unique identities;
- one preserved Production legacy row;
- 8,706 Production-origin rows;
- 3,565 Test real-account rows;
- 1,028 Staging real-account rows;
- SHA-256 `970fdac2eb709a5f5a492c255dd16afa4c2e47ad42783a429028464a5cb54aa0` before gateway startup.

The exact accepted image was promoted to `codex-pool:latest` and Production restarted successfully. Post-cutover validation confirmed:

- image revision `9ffe963ddcd1`;
- healthy gateway on port `8989`;
- frontend and health endpoint return HTTP 200;
- protected APIs return HTTP 401 when signed out;
- online SQLite snapshot succeeds;
- `quick_check=ok`;
- 13,299 persisted rows and 13,298 unique canonical identities before live validation traffic;
- a real authenticated Pi Spark tool round trip wrote exact bytes `SPARK_PRODUCTION_OK\n`;
- the tool-call and follow-up turns produced exactly two additional canonical events, bringing the live ledger to 13,301 rows;
- no analytics corruption, disk I/O, economics-build, or database-open failures in startup or post-request logs.

The prior image remains tagged as `codex-pool:production-rollback-20260726T023330Z`. Database rollback files and migration evidence remain in the cutover directory.

## Required maintenance procedure

This is the required procedure for repeating or rolling forward this class of deployment. Never copy a prepared candidate blindly over a live database; stop or fully quiesce every writer and reconcile any traffic accepted after the preparation snapshots.

1. Announce maintenance and stop only Production.
2. Copy the stopped Production database, WAL, and SHM files into a timestamped immutable rollback directory.
3. Run integrity checks against the stopped database.
4. Recover the stopped Production ledger with SQLite `.recover` if it remains corrupt.
5. Compare the final recovered ledger with the prepared recovery base so requests completed after the checkpoint are retained.
6. Re-run canonical Test and Staging migrations from fresh snapshots or reconcile the prepared candidate with the final recovered Production delta.
7. Require zero unresolved payload conflicts. Quarantine only explicitly reviewed noncanonical or attribution-variant rows.
8. Rebuild derived projections from canonical usage.
9. Run `quick_check`, canonical uniqueness checks, row-count reconciliation, and migration idempotency checks.
10. Atomically install the recovered/merged `analytics.db`; remove stale WAL/SHM files only while Production remains stopped and after rollback capture.
11. Normalize ownership for the unprivileged `codex` user.
12. Point the Production Compose deployment at the exact accepted image artifact and recreate Production without replacing `data/`, `pool/`, users, MFA, OAuth state, or credentials.
13. Validate health, real authentication, MFA, provider inventory, `/api/pool/signal`, model routing, typed errors, Spark tool calls, canonical accounting, and database write access.
14. Roll back both image and database if any validation fails.

## Remaining architecture gate

The shared-account authority boundary described in the Staging soak checkpoint is still unresolved. Staging currently has environment-local provider lifecycle/accounting state. Historical usage is now consolidated in Production, but ongoing Staging activity still requires an authenticated durable outbox and Production authority ingestion to remain synchronized exactly once. The roadmap now also includes scoped maintenance mode so future work can drain selected writers and services while unaffected health, authentication, status, and read-only controls remain available.
