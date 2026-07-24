# Frontend Acceptance and E2E Recovery Plan

Status: active functional acceptance gate; the original recovery blockers are complete.

Current checkpoint: truthful scoped Usage, model/provider/connection attribution, measured per-model curves, Connections/Members/System baselines, Profile MFA enrollment, one MFA gate, and accessible dialogs are implemented. The next active feature is backend-owned selected-model routing. Broad hardening and polish follow feature completeness.

Source of product truth: `docs/frontend-product-architecture.md`

## Why the current review failed

1. The Go router serves the React shell only for `/`. Direct browser requests to `/models`, `/usage`, `/profile`, `/setup`, and `/admin/*` fall through to the model proxy and return a pool-token 401. Browser navigation and API/proxy routing are mixed.
2. Unit/build success did not verify the running browser experience.
3. The authenticated-state test suite is stale and still asserts the removed Pulse/Accounts UI.
4. No deterministic browser matrix proves signed-out, Member, locked Admin, and elevated Admin behavior.
5. No automated assertion proves Home and Usage have different jobs.
6. Admin identity without elevation intentionally inherits Member capabilities, but the UI does not make the capability progression and Admin-specific destination set sufficiently visible and testable.

## Non-negotiable acceptance model

### Signed out

- Every canonical frontend URL loads the React shell, not a proxy/API error.
- The shell shows the Google sign-in gate.
- Google login start redirects to Google with callback `http://127.0.0.1:18991/auth/callback/google`.
- API endpoints continue to return JSON/HTTP authorization errors; frontend routes never do.

### Member

Navigation contains exactly:

- Home
- Models
- Usage
- Setup
- Profile

Member cannot see or open Admin resources.

### Admin identity, not elevated

- Inherits all Member navigation and pages.
- Clearly identifies the user as Admin with controls locked.
- Provides one working path to Profile/MFA elevation.
- Does not fetch or display protected Admin data.
- Direct `/admin/*` admission resolves safely to a locked experience or canonical Member Home; it never leaks data and never returns a proxy/API error.

### Elevated Admin

In the same product shell, navigation visibly gains exactly:

- Connections
- Members
- System

Shared resources visibly gain Admin context:

- Home: operational attention and links to owning Admin resources.
- Models: selected-model Admin routing context from the dedicated backend projection.
- Usage: pool scope and economics.
- Profile: elevated state and recovery status.

Elevated Admin must not look identical to Member: additional navigation and authorized contextual sections are mandatory and tested.

## Page job assertions

### Home

Must contain:

- orientation/start actions;
- available-model summary;
- setup/model/usage entry actions;
- Admin attention only when elevated.

Must not contain:

- usage metric dashboard;
- historical usage chart;
- token composition;
- economics.

### Usage

Owns:

- personal unavailable state or canonical personal totals;
- historical chart only where the backend provides a suitable series;
- elevated pool scope;
- token composition;
- economics;
- evidence/freshness copy.

Must not repeat Home orientation cards.

## Server routing correction

Introduce an explicit browser-navigation boundary before proxy fallback.

- `GET`/`HEAD` requests with browser HTML intent (`Accept: text/html`) for canonical frontend paths serve embedded `index.html`.
- Canonical paths:
  - `/`
  - `/models`
  - `/usage`
  - `/setup`
  - `/profile`
  - `/admin/connections`
  - `/admin/members`
  - `/admin/system`
- Discarded frontend paths may also receive the shell solely to render the React not-found page; they are not redirects or aliases.
- `/api/*`, `/auth/*`, `/config/*`, setup-download child paths, `/v1/*`, `/backend-api/*`, WebSocket/proxy routes, assets, and system routes retain their existing handlers.
- Non-browser requests to unknown proxy paths must never receive HTML.

Add Go contract tests for:

- direct canonical HTML navigation returns the embedded shell;
- frontend navigation does not require a pool bearer token;
- `/api/pool/session` remains JSON 401 while signed out;
- `/v1/*` and proxy paths never receive the SPA shell;
- `/setup` frontend route and `/setup/<client>/...` download routes remain distinct;
- discarded routes render frontend not-found without compatibility mapping.

## Deterministic Playwright framework

Replace stale E2E assertions with a state matrix using signed test cookies. Google itself is not automated; OAuth start/callback configuration is tested separately.

### Test identities

Use dedicated E2E fixture users in isolated dev data, never production:

- enabled Member user;
- enabled Admin user with copied/test MFA enrollment;
- disabled user.

Use the isolated dev JWT secret to mint:

- `pool_session` for Member;
- `pool_session` for Admin identity;
- `admin_elevated` for elevated Admin.

The helper must mirror backend JWT claims exactly. No frontend-only role overrides or query-string hacks.

### Projects/viewports

Run every applicable state at:

- Desktop Chromium: 1440×1000
- Mobile Chromium: 390×844

### Required Playwright suites

1. `frontend-routing.spec.ts`
   - Direct-load every canonical path.
   - Assert HTML shell, expected page heading, no plaintext 401.
   - Reload every path.
   - Back/forward navigation.
   - Unknown/discarded paths show React 404.
   - API endpoints still return API responses.

2. `auth-states.spec.ts`
   - Signed out: login gate on every canonical path.
   - Member: exact Member navigation; no Admin links/data.
   - Locked Admin: inherited Member navigation, locked notice, Profile elevation path, no protected data.
   - Elevated Admin: inherited navigation plus exactly Connections/Members/System.
   - Disabled/invalid session: signed-out behavior.

3. `home-usage-separation.spec.ts`
   - Home has orientation actions and no Usage graph/economics/token-dashboard labels.
   - Usage has activity semantics and no Home orientation cards.
   - Elevated Usage can switch to pool scope and see chart/economics.
   - Losing elevation removes pool scope and economics.

4. `admin-resources.spec.ts`
   - Connections, Members, System direct-load for elevated Admin.
   - Each has distinct heading and content ownership.
   - Loading/error/empty/ready states are tested via network fixtures.
   - Member and locked Admin cannot retrieve protected payloads.

5. `models.spec.ts`
   - Shared inventory for Member and Admin.
   - Member selected detail contains only public metadata and never requests routing detail.
   - Elevated Admin selected detail requests `/api/v2/models/:id/routing`.
   - Known model renders backend-owned provider, canonical model, selection mode, eligible connections, exclusions, and evidence.
   - Alias and declarative-provider resolution are covered.
   - Unknown model and authorization-expiry failures remain localized.
   - No selected connection or fallback order is displayed unless the backend explicitly owns that fact.

6. `responsive-navigation.spec.ts`
   - Member navigation remains usable on mobile.
   - Elevated Admin can reach all three Admin destinations on mobile.
   - No Admin destination is hidden without an alternative menu.

7. `runtime-integrity.spec.ts`
   - Capture `pageerror`, console error, failed requests, unexpected 401/403/429, and React blank-root failures.
   - Fail on any unexpected issue.
   - Assert no request loop or brute-force ban caused by MFA polling.

8. `oauth-entry.spec.ts`
   - Login action uses `/auth/login/google`.
   - Redirect contains the expected dev callback and configured client ID.
   - OAuth failure query states render actionable login feedback.

## Failure feedback artifacts

Playwright configuration must produce:

- HTML report;
- JUnit XML for CI/agent parsing;
- trace on first retry;
- screenshot on failure;
- video on failure;
- retained network/console summary attachment;
- deterministic test title including identity state and viewport.

A failed run must print:

- route;
- identity state;
- expected capability;
- response status/content type;
- console/page errors;
- failed API requests;
- artifact path.

## Automated gate order

Run sequentially:

```bash
cd web
npm test
npx tsc --noEmit --pretty false
npm run build
cd ..
git checkout -- web/src/generated/provider-connections-v2.ts
go test ./...
docker compose --env-file .env.dev -p codex-pool-dev -f docker-compose.dev.yml up -d --build
cd web
POOL_BASE_URL=http://127.0.0.1:18991 npm run test:e2e
cd ..
git diff --check
```

Then run an automated HTTP smoke matrix against every canonical frontend route and API boundary.

## Completion rule

Do not request human review unless:

- all unit, type, build, Go, and Playwright tests pass;
- no tests are skipped;
- no unexpected browser console/network errors exist;
- desktop and mobile screenshots are generated for all four identity states;
- Home/Usage separation assertions pass;
- direct route reloads pass;
- Admin navigation/context assertions pass;
- production and pinned staging remain untouched.

The final human review receives only:

- one dev URL;
- one concise pass matrix;
- artifact/report path;
- known backend projection limitations.
