# Isolated Development Instance

Use a second Docker Compose project for development. Do not rebuild, restart, or mount the state of the gateway currently serving clients.

## Isolation model

| Resource | Production | Development |
|---|---|---|
| Compose project | default/current | `codex-pool-dev` |
| Image | `codex-pool:latest` | `codex-pool:dev` |
| Host endpoint | `localhost:8989` | `127.0.0.1:18990` |
| Provider state | `./pool` | `./dev/pool` |
| Database/state | `./data` | `./dev/data` |
| Environment | `.env`/host | `.env.dev` |

The development endpoint intentionally uses `127.0.0.1` rather than `localhost`. Browser cookies do not include ports, so two dashboards on `localhost` would overwrite each other's `pool_session`, OAuth, and administrator-elevation cookies.

The host port binds only to loopback. It is not reachable from another machine unless the Compose file is deliberately changed.

## First-time setup

From the repository root:

```bash
cp .env.dev.example .env.dev
```

On PowerShell:

```powershell
Copy-Item .env.dev.example .env.dev
```

Edit `.env.dev`:

1. Generate a development-only `DEV_POOL_JWT_SECRET`.
2. Add your address to `DEV_ALLOWED_EMAILS` and, if needed, `DEV_ADMIN_EMAILS`.
3. Use a separate Google OAuth web client with this exact callback:

```text
http://127.0.0.1:18990/auth/callback/google
```

Do not reuse production session/JWT secrets. OAuth client separation is recommended so callback configuration and credential rotation cannot disrupt production.

## Start and stop

Start or rebuild development:

```bash
docker compose \
  --env-file .env.dev \
  -p codex-pool-dev \
  -f docker-compose.dev.yml \
  up -d --build
```

Check status and logs:

```bash
docker compose -p codex-pool-dev -f docker-compose.dev.yml ps
docker compose -p codex-pool-dev -f docker-compose.dev.yml logs -f codex-pool-dev
curl http://127.0.0.1:18990/healthz
```

Open:

```text
http://127.0.0.1:18990
```

Stop development without deleting its state:

```bash
docker compose -p codex-pool-dev -f docker-compose.dev.yml down
```

Delete only development containers and state:

```bash
docker compose -p codex-pool-dev -f docker-compose.dev.yml down --volumes
rm -rf dev/pool dev/data
```

The bind-mounted state directories are not removed by `down --volumes`; remove them explicitly only when a clean development state is intended.

## Provider credentials

Development starts with an empty provider pool. Prefer fixtures and protocol test servers while implementing UI, routing, and accounting changes.

If a real upstream smoke test is necessary:

1. Add a credential specifically owned by the development instance through its dashboard.
2. Prefer a low-risk/test key with spending limits.
3. Keep `DEV_PROXY_MAX_ATTEMPTS=1`.
4. Enable refresh only if the test requires it:

```text
DEV_PROXY_DISABLE_REFRESH=false
```

Do not mount `./pool` into development. Do not casually copy OAuth account files from production: both instances could refresh the same credential and race while persisting rotated tokens.

A production database should not be copied while live. Create purpose-built fixtures or use a database-native consistent snapshot that has been stripped of secrets and personal data.

## Vite frontend development

`web/vite.config.ts` already proxies application routes to `127.0.0.1:18990`.

Run the isolated backend first, then:

```bash
cd web
npm install
npm run dev -- --host 127.0.0.1
```

Open the `127.0.0.1` Vite URL it prints. Keep the hostname consistent to avoid sending development cookies to the production `localhost` origin.

OAuth callbacks currently return to the backend endpoint configured in `.env.dev`. After login, return to the Vite URL for hot-reload UI work.

## Operational guardrails

- Always include `-p codex-pool-dev -f docker-compose.dev.yml` in development Compose commands.
- Never run `docker compose down` against the production file while doing development work.
- Never tag a development build as `codex-pool:latest`.
- Do not use broad Docker prune commands while the production container is running.
- Inspect mounts before starting:

```bash
docker compose --env-file .env.dev -p codex-pool-dev -f docker-compose.dev.yml config
```

The resolved mounts must end in `./dev/pool` and `./dev/data`, never `./pool` or `./data`. Development interpolation variables are deliberately `DEV_`-prefixed, so omitting `--env-file` cannot silently import equivalent production values from the root `.env`.
