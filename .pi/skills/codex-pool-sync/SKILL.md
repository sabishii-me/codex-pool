---
name: codex-pool-sync
description: Copy provider account/pool data between codex-pool environments (e.g. production accounts to staging for testing) and keep provider-specs in sync across copies. Use when a user wants one environment's accounts available in another.
---

# Codex Pool data sync

Copy provider account data (`pool/` + `data/`) from a source to a target codex-pool deployment, and sync `provider-specs` copies. Every `${VAR}` resolves from `.dev.vars`.

## Before acting

1. Read `<repo>/.dev.vars`. If source/target host/paths are missing, **ask** — never guess.
2. Never print credentials, tokens, or `.dev.vars` values.
3. **Stop the target container before copying its data dirs** (SQLite/WAL lock conflicts). Restart after.

## Account/pool data copy (source → target)

Source and target may be local or remote. This flow assumes source is remote and target is local staging (the common case), but it generalizes: swap the ssh/`docker` commands for either side.

1. **Stop the target** (avoid sqlite lock):
   ```
   docker compose --env-file .env.staging.local -p codex-pool-staging -f docker-compose.staging.yml stop
   ```

2. **Back up the target's current state** (cheap insurance, timestamped):
   ```
   mkdir -p staging/backups/pre-copy-$(date +%Y%m%d%H%M)
   cp -r staging/pool staging/data staging/backups/pre-copy-$(date +%Y%m%d%H%M)/
   ```

3. **Archive source on the remote** (needs sudo for 600-permission files):
   ```
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "cd ${DEPLOY_DIR} && echo '${SUDO_PASSWORD}' | sudo -S tar czf /home/${SSH_USER}/pool-data.tar.gz pool data && echo '${SUDO_PASSWORD}' | sudo -S chown ${SSH_USER}:${SSH_USER} /home/${SSH_USER}/pool-data.tar.gz"
   ```

4. **Fetch + extract locally**:
   ```
   scp -i ${SSH_KEY} ${SSH_USER}@${HOST}:/home/${SSH_USER}/pool-data.tar.gz /tmp/pool-data.tar.gz
   rm -rf staging/pool staging/data
   mkdir -p staging/pool staging/data
   tar xzf /tmp/pool-data.tar.gz -C staging/
   ```

5. **Restart target + verify**:
   ```
   docker compose --env-file .env.staging.local -p codex-pool-staging -f docker-compose.staging.yml up -d
   curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18990/healthz
   ```
   Confirm the log shows the expected account counts (e.g. `codex=6`) and usage fetch lines per account.

6. **Clean up remote archive**:
   ```
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "echo '${SUDO_PASSWORD}' | sudo -S rm -f /home/${SSH_USER}/pool-data.tar.gz"
   ```

## Provider-specs sync

`provider-specs/*.json` (source of truth) and `provider-specs.builtin/*.json` (embedded fallback) must stay identical:

```
for f in provider-specs/*.json; do cp "$f" "provider-specs.builtin/$(basename "$f")"; done
```

Also copy to any environment that mounts its own copy (e.g. `staging/provider-specs/`) if that environment is being used.

## Pitfalls

- **Stop the target before copying `data/`** — copying a live SQLite DB (proxy.db / analytics.db with WAL) can corrupt it.
- **`pool/` files are 600 owned by a different uid** — plain `scp` as an unprivileged user fails with Permission denied. Use `sudo tar` on the source.
- **Never copy credential/secret files for a user's own use** unless the user explicitly asks to move accounts between environments.
- **Restart, don't hot-copy**: after copying, always restart the target so it reloads accounts.
- **Clean up remote archives** — don't leave tar files on the source.
- **Never print account tokens or `data/*.json` contents** — including pool_users.json, MFA, OAuth sessions.
