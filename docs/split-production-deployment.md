# Split Production deployment

Production is split into independently replaceable services:

- `ingress`: stable owner of public port `8989` and same-origin routing.
- `web`: static SPA only; no credentials or mutable data.
- `api`: Go model/API gateway with the existing `pool/`, `data/`, and `provider-specs/` mounts.

Use `docker-compose.split.yml`. The current `docker-compose.yml` remains the combined-runtime definition until an explicitly approved one-time cutover.

## Independent releases

Frontend only:

```powershell
./scripts/deploy-production-web.ps1 -Image codex-pool-web:<immutable-tag>
```

Backend only:

```powershell
./scripts/deploy-production-api.ps1 -Image codex-pool-api:<immutable-tag>
```

Both scripts use `docker compose up --no-deps` and therefore do not recreate unrelated services. Frontend replacement cannot interrupt API/model traffic. Ingress uses Docker DNS re-resolution so replacing `web` or `api` does not require an ingress restart.

## One-time cutover

The first migration requires a short maintenance window because the existing combined container currently owns host port `8989`.

1. Build immutable `api`, `web`, and `ingress` images.
2. Preserve the current combined image as the rollback target.
3. Stop the combined service; do not alter or synchronize `pool/` or `data/`.
4. Start `api`, `web`, and `ingress` from `docker-compose.split.yml`.
5. Validate `/healthz`, `/ingress-healthz`, browser routes, assets, authentication, OAuth redirects, SSE, WebSockets, and a bounded model request.
6. On failure, stop the split stack and restart the preserved combined image against the unchanged mounts.

After cutover, normal UI changes replace only `web`.
