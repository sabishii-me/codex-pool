# Isolated Staging and Development Gateways

Keep three independent gateways:

| Resource | Production | Staging baseline | Active development |
|---|---|---|---|
| Compose project | current/default | `codex-pool-staging` | `codex-pool-dev` |
| Compose file | `docker-compose.yml` | `docker-compose.staging.yml` | `docker-compose.dev.yml` |
| Image | `codex-pool:latest` | pinned `codex-pool:staging-a91560b` | mutable `codex-pool:dev` |
| Endpoint | `localhost:8989` | `127.0.0.1:18990` | `127.0.0.2:18991` |
| Provider state | `./pool` | `./staging/pool` | `./dev/pool` |
| Provider specs | operator-defined | `./staging/provider-specs` | `./dev/provider-specs` |
| Data/session state | `./data` | `./staging/data` | `./dev/data` |
| Variables | production names | `STAGING_*` | `DEV_*` |

Staging is the validated legacy-UI control client. It has no Compose `build` section, so normal source rebuilds cannot replace it. Active development is disposable and follows the current checkout.

The gateways deliberately use different loopback IP addresses. Browser cookies are scoped by hostname, not port; using `127.0.0.1` for both would make their session and elevation cookies collide.

Neither non-production gateway mounts production databases, users, MFA, sessions, analytics, or JWT secrets. Provider credential snapshots may be copied deliberately, but they consume the same upstream quotas. Refresh is disabled by default.

## Staging baseline

Staging currently preserves the validated commit/image `a91560b` and the existing isolated baseline state.

Start without rebuilding:

```bash
docker compose -p codex-pool-staging -f docker-compose.staging.yml up -d
```

Check:

```bash
docker compose -p codex-pool-staging -f docker-compose.staging.yml ps
curl http://127.0.0.1:18990/healthz
```

Open `http://127.0.0.1:18990`.

Run the compatibility baseline:

```bash
cd web
POOL_LOCAL_DEV_BASELINE=1 POOL_BASE_URL=http://127.0.0.1:18990 npm run test:e2e:baseline
```

To promote a newly validated baseline, use an immutable tag containing its commit, update `STAGING_IMAGE`, and migrate staging deliberately. Never point staging at `codex-pool:dev` or `codex-pool:latest`.

## Staging candidate promotion

The next feature-phase goal is an immutable build that can be deployed to staging without replacing the pinned legacy control prematurely. Build a candidate from a clean commit:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-staging-candidate.ps1
```

This runs frontend tests/type/build, restores generated source, runs Go tests, and creates `codex-pool:staging-<commit>` with OCI revision/version/date labels. Candidate images are never tagged `dev` or `latest`.

Exercise it beside pinned staging with independent candidate state and port `18992`:

```powershell
$env:STAGING_CANDIDATE_IMAGE="codex-pool:staging-<commit>"
docker compose -p codex-pool-staging-candidate -f docker-compose.staging-candidate.yml up -d
```

Only after candidate browser acceptance should `STAGING_IMAGE` in the pinned staging deployment be changed deliberately. The candidate Compose contract has no `build` section and never mounts `staging/*`, `dev/*`, or production state.

## Active development

Optionally copy `.env.dev.example` to ignored `.env.dev`, then start/rebuild:

```bash
docker compose --env-file .env.dev -p codex-pool-dev -f docker-compose.dev.yml up -d --build
```

Defaults also work without an env file because all interpolation names are `DEV_*` and cannot inherit production equivalents.

Check:

```bash
docker compose -p codex-pool-dev -f docker-compose.dev.yml ps
curl http://127.0.0.2:18991/healthz
```

Open `http://127.0.0.2:18991`.

`DEV_LOCAL_SESSION=true` creates an isolated `developer@localhost.invalid` member in `dev/data/pool_users.json`. It does not grant administrator elevation. The server accepts this mode only for loopback `PUBLIC_URL` and request hosts.

For real Google authentication, use a separate OAuth client and callback:

```text
http://127.0.0.2:18991/auth/callback/google
```

## State rules

- `staging/data` is the durable baseline history; do not mutate/delete it during ordinary development.
- `dev/data` is disposable and must start without copied staging/production databases or users.
- `staging/pool` and `dev/pool` may contain intentional credential snapshots, but neither may mount `./pool`.
- Keep `PROXY_DISABLE_REFRESH=true` for shared snapshots unless a test explicitly requires refresh.
- Keep attempts at one to avoid multiplying real quota use.
- Do not inspect bind-mounted SQLite concurrently from Windows as proof of durability; stop the corresponding container or use its APIs.

## Vite UI development

`web/vite.config.ts` proxies to active development at `127.0.0.2:18991`.

```bash
cd web
npm install
npm run dev -- --host 127.0.0.2
```

Open the printed `127.0.0.2` URL. Staging remains available independently on `127.0.0.1:18990` for comparison.

## Codex OAuth broker

The host broker remains at `127.0.0.1:1460` and leases callback ports `1455`/`1457` only during authorization. Both gateways can request leases sequentially. The `gateway_origin` sent by the UI determines where the callback is forwarded.

## Stop/remove

```bash
# Active development only
docker compose -p codex-pool-dev -f docker-compose.dev.yml down

# Staging only
docker compose -p codex-pool-staging -f docker-compose.staging.yml down
```

Bind-mounted state is not removed by `down --volumes`. Delete `dev/` only when a clean development reset is intended. Do not delete `staging/` without first preserving the baseline intentionally.

## Guardrails

- Always include the project and Compose file in commands.
- Never run production Compose commands to validate staging/development changes.
- Never tag an active development build as `codex-pool:latest`.
- Never use broad Docker prune commands while any gateway is in use.
- Render both contracts before starting:

```bash
docker compose -p codex-pool-staging -f docker-compose.staging.yml config
docker compose -p codex-pool-dev -f docker-compose.dev.yml config
```

Resolved mounts must end in their own `staging/*` or `dev/*` directories, never production `pool/` or `data/`.
