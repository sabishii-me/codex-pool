---
name: codex-pool-deploy
description: Build, transfer, and deploy the codex-pool API image to a single target environment (split deployment) and roll back when needed. Use for any deploy/build/transfer/rollback request. Loads all target data from .dev.vars — never hardcode hosts, paths, keys, or tokens.
---

# Codex Pool deploy

Deploy the codex-pool API container to the ONE environment described by `.dev.vars`, then verify and (if needed) roll back.

## Before acting

1. Read `<repo>/.dev.vars` (gitignored). Every `${VAR}` below resolves from that file.
2. If any required var is missing/empty, **stop and ask** — never guess hosts, paths, or credentials.
3. Determine which container(s) the user wants touched. Default is **api only**; web/ingress stay untouched unless explicitly requested.

## Non-negotiable rules

1. **Deploy only after the change is confirmed**: tests pass and the user asked for deployment. Do not deploy speculative/untested code.
2. **api only by default**: use `--no-deps --no-build` and the `api` service so web/ingress are never recreated.
3. **Record before/after**: capture web/ingress container IDs before deploying; after deploy confirm they are unchanged.
4. **Never expose secrets**: do not print `.dev.vars` values, tokens, or passwords in output/logs/handoff.
5. **Clean up transfers**: remove the transferred `.tar` file from the target after `docker load`.
6. **Rollback = re-deploy previous image tag** with the same `--no-deps --no-build api` command. Rollback is a first-class operation, not an afterthought.

## Deploy flow (one environment)

Assume the repo is checked out locally and `docker` works locally and on the target.

1. **Confirm the local image exists**:
   ```
   docker images ${API_IMAGE_NAME}:${API_IMAGE_TAG}
   ```
   If missing, build it first (see build flow below) or ask which tag to use.

2. **Save + transfer + load**:
   ```
   docker save -o /tmp/${API_IMAGE_NAME}-${API_IMAGE_TAG}.tar ${API_IMAGE_NAME}:${API_IMAGE_TAG}
   scp -i ${SSH_KEY} /tmp/${API_IMAGE_NAME}-${API_IMAGE_TAG}.tar ${SSH_USER}@${HOST}:${DEPLOY_DIR}/
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "cd ${DEPLOY_DIR} && echo '${SUDO_PASSWORD}' | sudo -S docker load -i ${DEPLOY_DIR}/${API_IMAGE_NAME}-${API_IMAGE_TAG}.tar"
   ```

3. **Record pre-deploy container IDs** (to prove web/ingress untouched):
   ```
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "echo '${SUDO_PASSWORD}' | sudo -S docker inspect codex-pool-web-1 --format 'web={{.Id}}'; echo '${SUDO_PASSWORD}' | sudo -S docker inspect codex-pool-ingress-1 --format 'ingress={{.Id}}'"
   ```

4. **Deploy api only**:
   ```
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "cd ${DEPLOY_DIR} && echo '${SUDO_PASSWORD}' | sudo -S env PRODUCTION_API_IMAGE=${API_IMAGE_NAME}:${API_IMAGE_TAG} docker compose -p codex-pool -f docker-compose.split.yml up -d --no-deps --no-build api"
   ```

5. **Wait for health + verify**:
   ```
   sleep 25
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "echo '${SUDO_PASSWORD}' | sudo -S docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep codex"
   curl -s -o /dev/null -w '%{http_code}\n' ${HEALTHZ_URL}
   ```
   Confirm:
   - api is `Up (healthy)` with image `${API_IMAGE_NAME}:${API_IMAGE_TAG}`
   - web/ingress IDs match the pre-deploy snapshot (unchanged)

6. **Smoke test** the affected capability (see `codex-pool-verify` skill) before declaring success.

7. **Clean up**:
   ```
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "echo '${SUDO_PASSWORD}' | sudo -S rm -f ${DEPLOY_DIR}/${API_IMAGE_NAME}-${API_IMAGE_TAG}.tar"
   ```

## Build flow (if the tag does not exist yet)

Prefer the project's canonical build script (requires a clean tree):
```
powershell -ExecutionPolicy Bypass -File scripts/build-release-image.ps1
```
It produces `codex-pool:staging-<commit>` and prints the exact promoted tag. For the split `api` image, build with `Dockerfile.api`:
```
docker build -f Dockerfile.api -t ${API_IMAGE_NAME}:staging-<commit> --build-arg BUILD_VERSION=staging-<commit> --build-arg BUILD_COMMIT=<commit> --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" .
```

## Rollback flow

Roll back = deploy the previous known-good `${API_IMAGE_NAME}:<previous-tag>` using the **same** steps 2–7. You must know the previous tag (check what the target ran before, e.g. `challenge-fix` or the last accepted staging tag). If unsure, ask.

## Pitfalls

- **Do not deploy to a target the user has not named.** `.dev.vars` points at ONE target; changing it is the only way to switch. Confirm with the user which target before transferring anything.
- **Never use `up -d` without `--no-deps --no-build`** on the split compose — it can rebuild or recreate web/ingress.
- **Never `docker compose build` on the target**; images are built locally and loaded.
- **`SUDO_PASSWORD` may be empty** on some targets — check before embedding in the command.
- **Container names differ per target** (`codex-pool-api-1`, etc.) — verify with `docker ps` before relying on the name.
- **Do not leave `.tar` files on the target** — always clean up.
- **Prompt cache / request-format regressions**: after any API deploy, run the verify skill's smoke tests (especially `prompt_cache_retention` handling) before calling it done.
