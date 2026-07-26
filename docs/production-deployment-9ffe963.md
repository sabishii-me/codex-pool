# Production deployment checkpoint — 9ffe963

Prepared 2026-07-26. This checkpoint is a plan and immutable artifact set, not approval to change Production.

## Release artifact

- Accepted revision: `9ffe963ddcd1c66ef5c8866a6fec8cd6c17f07f4`
- Tested Staging image: `codex-pool:staging-9ffe963`
- Production preparation tag: `codex-pool:production-9ffe963`
- Production remains on `codex-pool:latest` until explicit cutover approval.

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

## Required maintenance window

Do not copy the prepared candidate blindly over the live database. Production continued accepting traffic after the snapshots, so cutover requires a final delta reconciliation.

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

## Remaining release gate

The shared-account authority boundary described in the Staging soak checkpoint is still unresolved. Staging currently has environment-local provider lifecycle/accounting state. This checkpoint makes historical accounting recoverable and prepares the exact image, but it does not by itself prove ongoing Staging-to-Production authoritative synchronization. Production promotion requires an explicit decision on that known gate.
