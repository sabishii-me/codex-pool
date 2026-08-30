---
name: codex-pool-verify
description: Verify a codex-pool environment is healthy, smoke-test the codex model protocols, and diagnose request-format errors (e.g. prompt_cache_retention not supported). Use after any deploy, before declaring success, or when a user reports a model request failure.
---

# Codex Pool verify & diagnose

Verify the ONE target described by `.dev.vars`, smoke-test codex protocols, and diagnose upstream 4xx errors. Every `${VAR}` resolves from `.dev.vars`.

## Before acting

1. Read `<repo>/.dev.vars`. If a required var is missing, **ask** — never guess endpoints or tokens.
2. Never print `.dev.vars` values, tokens, or passwords.

## Health verification flow

1. **Healthz**:
   ```
   curl -s -o /dev/null -w '%{http_code}\n' ${HEALTHZ_URL}
   ```
   Expect `200`.

2. **Container + image state** (ssh if remote):
   ```
   ssh -i ${SSH_KEY} ${SSH_USER}@${HOST} "echo '${SUDO_PASSWORD}' | sudo -S docker ps --format '{{.Names}} {{.Image}} {{.Status}}' | grep codex"
   ```
   Confirm api is `Up (healthy)` with the expected `${API_IMAGE_NAME}:${API_IMAGE_TAG}` and web/ingress are `Up (healthy)`.

3. **web/ingress unchanged check** (during a deploy): record their IDs before and after; they must match.

## Codex model smoke tests

Use a pool token. Derive one from `${POOL_JWT_SECRET}` (e.g. via the repo's `generateClaudePoolToken` in tests) or ask the user for the token. Test all three client protocols against a codex model (e.g. `gpt-5.6-sol`):

1. `/v1/responses`:
   ```
   curl -s -X POST <base>/v1/responses -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
     -d '{"model":"gpt-5.6-sol","input":"hi","max_output_tokens":10}'
   ```
   Expect `200` with `output[].content[].text`.

2. `/v1/messages` (Anthropic):
   ```
   curl -s -X POST <base>/v1/messages -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -H "anthropic-version: 2023-06-01" \
     -d '{"model":"gpt-5.6-sol","max_tokens":10,"messages":[{"role":"user","content":"hi"}]}'
   ```

3. `/v1/chat/completions`:
   ```
   curl -s -X POST <base>/v1/chat/completions -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
     -d '{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"ping"}],"max_tokens":10}'
   ```
   Expect `"pong"`.

## Diagnosing `prompt_cache_retention is not supported on this model`

This 400 means an Anthropic cache parameter leaked through to the OpenAI/codex upstream.

1. **Root cause**: `sanitizeCodexResponsesParams` in `main.go` must strip `prompt_cache_retention` (and `cache_control`). If it is not in the strip list, requests carrying it are forwarded untouched and the upstream rejects them.
2. **Repro**: send the same request WITH `prompt_cache_retention`:
   ```
   curl -s -X POST <base>/v1/responses -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
     -d '{"model":"gpt-5.6-sol","input":"hi","max_output_tokens":10,"prompt_cache_retention":{"default":"24h"}}'
   ```
   - `200` → fixed.
   - `400 ... prompt_cache_retention is not supported` → the running image is an OLD version that does not strip it. **The fix exists only in images built after the strip commit**; check which image the target runs and confirm it includes the fix.
3. **Key trap**: the bug is per-image. Two environments can behave differently because one deployed the fixed image and the other did not. Verify the **running image**, not the source branch.

## Pitfalls

- **Usage events only record successful requests** — a 400 failure will NOT appear in usage analytics. "All success in usage" proves nothing about failures.
- **Do not blame the client** when a request-format error is upstream's 400: check whether the gateway stripped the offending field first.
- **Never print tokens or `.dev.vars` values**.
- **Different environments have different accounts/credentials** — a 401 on staging does not mean production is broken; check the running image and account expiry separately.
