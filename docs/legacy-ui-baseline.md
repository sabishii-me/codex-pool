# Legacy UI Baseline

The current Signal Room UI is the stable compatibility client for the refactored gateway. It is intentionally **not** the Phase 9 redesign. Keep it available during the long daily-use period so backend regressions can be distinguished from redesign regressions.

## Baseline guarantees

- Uses the current production UI routes and visual structure without redesign.
- Hydrates pool stats, analytics, and catalog independently and preserves last-known-good data when one endpoint fails.
- Uses canonical `/api/v2/provider-connections` identity data for elevated operator workflows.
- Treats unknown runtime provider IDs through the neutral provider presentation fallback.
- Exercises every existing primary view against the isolated development gateway.
- Detects JavaScript exceptions, unexpected console errors, failed requests, server 5xx responses, duplicate application mounts, and refresh regressions.
- Keeps production authentication tests separate from loopback `LOCAL_DEV_SESSION` tests.

## Quick daily check

The isolated development gateway must be healthy at `http://127.0.0.1:18990`.

```bash
cd web
POOL_LOCAL_DEV_BASELINE=1 \
POOL_BASE_URL=http://127.0.0.1:18990 \
npm run test:e2e:baseline
```

PowerShell:

```powershell
cd web
$env:POOL_LOCAL_DEV_BASELINE = "1"
$env:POOL_BASE_URL = "http://127.0.0.1:18990"
npm run test:e2e:baseline
```

## Soak run

Defaults to 12 cycles with five minutes between cycles (about one hour):

```bash
POOL_BASE_URL=http://127.0.0.1:18990 npm run test:e2e:baseline:soak
```

Useful overrides:

```text
POOL_BASELINE_CYCLES=288
POOL_BASELINE_DELAY_SECONDS=300
```

That configuration samples the UI every five minutes for approximately 24 hours. Run the soak against development only; its provider snapshots consume real shared upstream quotas.

## Daily-use feedback record

For every incident preserve:

- Date/time and browser version
- Current git commit and development image ID
- Active view and action
- Screenshot and Playwright trace, when available
- Failed URL/status and visible error text
- Whether a manual Sync recovered
- Gateway request ID for provider traffic
- Model/provider/connection if provider traffic was involved

Classify incidents as:

1. Gateway/API contract
2. Authentication/session
3. Provider routing or quota
4. Usage/analytics freshness
5. Current UI compatibility
6. Candidate redesign-only issue

Do not fix category 6 in the baseline UI unless it blocks daily operation. Feed those observations into Phase 9 instead.

## Promotion rule for the redesign

The future UI should run alongside this baseline and consume the new gateway APIs. Do not remove the baseline until the new UI has matched its route coverage, long-run reliability, setup workflows, provider contribution workflows, and operator recovery paths.
