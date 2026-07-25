# Test, Staging, and Production

There are exactly three runtime environments:

| Environment | Purpose | Compose project | Compose file | Endpoint | Persistent state |
|---|---|---|---|---|---|
| Test | active development and automated acceptance | `codex-pool-dev` | `docker-compose.dev.yml` | `http://127.0.0.1:18991` | `dev/pool`, `dev/data`, `dev/provider-specs` |
| Staging | production-like long soak and final release validation | `codex-pool-staging` | `docker-compose.staging.yml` | `http://127.0.0.1:18990` | `staging/pool`, `staging/data`, `staging/provider-specs` |
| Production | live gateway | default/current | `docker-compose.yml` | `http://localhost:8989` | `pool`, `data` |

There is no candidate runtime and no fourth port. An immutable Docker image is a build artifact, not an environment.

## Promotion flow

```text
Test (18991) -> Staging (18990) -> Production (8989)
```

1. Develop and validate in Test.
2. Build one immutable, commit-tagged release image.
3. Promote that exact image to Staging without replacing Staging data.
4. Run Staging schema/data migrations in place and complete the long soak.
5. Promote the same accepted image to Production without replacing Production data.

Staging is not pinned or frozen. It changes when a Test build is promoted. It has no Compose `build` section, so promotion always names the exact immutable image explicitly.

## Build an immutable release image

From a clean commit:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-release-image.ps1
```

The script runs frontend tests/type/build and Go tests, then creates `codex-pool:staging-<commit>` with OCI revision/version/date labels. It creates no runtime or port.

## Test

Copy `.env.dev.example` to ignored `.env.dev` and configure the Test OAuth callback:

```text
http://127.0.0.1:18991/auth/callback/google
```

Start or rebuild Test:

```bash
docker compose --env-file .env.dev -p codex-pool-dev -f docker-compose.dev.yml up -d --build
curl http://127.0.0.1:18991/healthz
```

`DEV_LOCAL_SESSION=true` is allowed only in Test. It creates `developer@localhost.invalid` in Test state. That identity and its data never move to Staging or Production.

## Promote to Staging

Create ignored `.env.staging.local` from `.env.staging.example`. Set `STAGING_IMAGE` to the exact image accepted in Test and provide real Staging OAuth, allowlist, Admin, and JWT settings.

```bash
docker compose --env-file .env.staging.local -p codex-pool-staging -f docker-compose.staging.yml up -d
curl http://127.0.0.1:18990/healthz
```

Required Staging properties:

- `STAGING_LOCAL_SESSION=false`;
- real Google OAuth callback `http://127.0.0.1:18990/auth/callback/google`;
- real allowed/Admin policy;
- isolated Staging state;
- no synthetic GatewayUser;
- bind-mounted files writable by the unprivileged `codex` runtime user;
- destination data retained and migrated in place.

The one-shot `staging-data-permissions` service normalizes bind-mount ownership before the gateway starts so SQLite can create WAL/SHM files.

## Data rules

Deployment promotes code, not data directories. See [Environment data migration](environment-data-migration.md).

- Never replace destination `data/`, `pool/`, or provider specs during ordinary promotion.
- Never merge Test usage into Staging or Production.
- Never copy OAuth sessions or cookies.
- Users, MFA, and provider credentials require an explicit, sanitized refresh policy; they are not generic merge inputs.
- Merge only canonical usage events by `(connection_id, request_id)` with conflict rejection and provenance.
- Rebuild derived analytics from canonical events.
- Back up before every applied migration and prove idempotency.

Provider connections in all three environments can consume the same real upstream quota even though gateway state and accounting are isolated.

## Staging long soak

The active checkpoint and evidence procedure are in [Staging soak checkpoint](staging-soak-checkpoint.md).

Minimum recurring checks:

```bash
curl http://127.0.0.1:18990/healthz
docker logs --since 24h codex-pool-staging-codex-pool-staging-1
```

Exercise signed-out, member, locked Admin, elevated Admin, setup clients, provider routing, typed protocol errors, image generation/read, canonical usage persistence, economics, and restart recovery. Do not use Test fixture authentication against Staging.

## Promote to Production

Production promotion is authorized only after the Staging checkpoint has no unresolved release blocker. Use the exact image digest accepted on Staging. Preserve Production mounts and configuration, dry-run migrations first, take rollback backups, stop only Production for apply, restart, and validate health/auth/accounting.

Do not tag or rebuild a different image during promotion.

## Stop an environment

```bash
# Test only
docker compose --env-file .env.dev -p codex-pool-dev -f docker-compose.dev.yml down

# Staging only
docker compose --env-file .env.staging.local -p codex-pool-staging -f docker-compose.staging.yml down
```

Never use broad Docker prune commands while any environment is in use.
