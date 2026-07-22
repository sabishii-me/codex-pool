# Codex OAuth Blocker Plan

Status: blocking implementation

## Problem statement

OpenAI's Codex OAuth client only accepts browser callbacks on:

```text
http://localhost:1455/auth/callback
http://localhost:1457/auth/callback
```

The gateway runs in Docker. Publishing either port on the gateway container reserves it for the container lifetime, preventing Pi, Codex CLI, and other coding tools from using the same allowlisted ports. A one-shot Compose relay avoids permanent reservation but requires manual lifecycle management, fails on consecutive account additions, and loses pending sessions when the gateway restarts.

Device authorization is not an acceptable substitute because it requires a separate ChatGPT security setting and is a different user flow.

Codex OAuth is a release blocker. General architecture and UI implementation must not resume until the acceptance criteria in this document pass.

## Decision

Use normal browser OAuth with an automatic host-side callback broker.

The broker is a lightweight process on the browser's machine. It keeps only a loopback control port open. It binds `1455` or `1457` only after the dashboard asks it to prepare a Codex login, forwards one callback to the gateway, and immediately releases the callback port.

The Docker Compose one-shot relay will be removed.

## Components

### 1. Local OAuth broker

Command:

```text
codex-pool oauth-broker
```

Responsibilities:

- Listen on a non-provider loopback control address, default `127.0.0.1:1460`.
- Accept prepare requests only from configured dashboard origins.
- Validate the gateway callback origin against an explicit allowlist.
- Try `127.0.0.1:1455`, then `127.0.0.1:1457`.
- Return the selected callback port before the authorization URL is opened.
- Forward only `code`, `state`, `error`, `error_description`, and `iss`.
- Never log authorization codes.
- Release the callback listener after one callback, cancellation, or timeout.
- Support any number of sequential OAuth sessions without restarting the broker.
- Expose health and active-lease state on the control API.
- Refuse a second concurrent lease rather than routing a callback ambiguously.

The control process may stay running. Ports `1455` and `1457` may not.

### 2. Windows installation

Provide an idempotent PowerShell installer that:

1. Builds or installs the broker executable under `%LOCALAPPDATA%\CodexPool`.
2. Creates a per-user startup task.
3. Starts the broker immediately.
4. Verifies its health endpoint.
5. Supports uninstall and status commands.

No administrator privileges should be required. Broker logs must not contain OAuth codes or tokens.

Other platforms can run the same broker command under their user service manager; Windows is the blocking supported path for this deployment.

### 3. Dashboard handshake

The Codex connection flow becomes:

1. User presses **Connect Codex**.
2. UI calls broker `POST /v1/leases` with the current gateway origin.
3. Broker binds an available allowlisted callback port and returns a lease and port.
4. UI asks the gateway to create OAuth state/PKCE for that exact port.
5. UI opens OpenAI authorization only after both preparation steps succeed.
6. UI polls the gateway session and shows explicit phases:
   - Preparing local callback
   - Waiting for OpenAI authorization
   - Exchanging credentials
   - Connection added
   - Failed with recovery action
7. OpenAI calls the broker.
8. Broker redirects the browser to the gateway callback and releases its port.
9. Gateway validates state and PKCE, exchanges the code, and saves the connection.

If the broker is unavailable, the UI must not open a callback URL that is guaranteed to fail. It shows installation/status guidance. Manual callback pasting remains an advanced fallback for remote-machine scenarios.

### 4. Durable gateway OAuth sessions

Pending Codex OAuth sessions must survive gateway and container restarts.

Persist under the gateway data directory with owner-only permissions:

```text
session_id
state
PKCE verifier
redirect URI
created/expires timestamps
status
resulting connection ID or sanitized error
```

Requirements:

- Atomic writes.
- Maximum 15-minute pending lifetime.
- Startup cleanup of expired sessions.
- State and session IDs generated cryptographically.
- Callback lookup by state.
- Exactly-once exchange transition: `pending → exchanging → complete|error`.
- Duplicate callbacks are idempotent.
- No authorization code is persisted.
- Status polling never returns tokens or PKCE material.

### 5. Connection identity

Credential filenames remain deterministic full hashes of `chatgpt_account_id`.

The temporary current UI may display Codex email to make blocker testing understandable. The development refactor replaces this with:

```text
ProviderConnection.display_name
ConnectionIdentity.external_subject
ConnectionIdentity.attributes[email|workspace|tenant|region]
```

Email is optional metadata, not a generic connection identity requirement.

## Security boundaries

- Broker control and callback listeners bind only to loopback.
- CORS allows exact configured dashboard origins, never `*`.
- Gateway callback targets are allowlisted; arbitrary forwarding is rejected.
- Only ports 1455 and 1457 are valid production Codex redirect ports.
- Gateway still validates OAuth state and PKCE after broker forwarding.
- Callback and status responses use `Cache-Control: no-store`.
- Codes, tokens, verifiers, and full callback URLs are excluded from logs.
- Credential files remain owner-only.

## Test plan

### Broker unit tests

- Selects 1455 when free.
- Falls back to 1457 when 1455 is occupied.
- Returns a clear error when both are occupied.
- Rejects invalid origins and gateway targets.
- Enforces exact CORS origins.
- Forwards only approved query fields.
- Releases the callback port after success.
- Releases the callback port after cancellation and timeout.
- Handles three sequential leases in one broker process.
- Rejects ambiguous concurrent leases.

### Gateway unit tests

- Accepts only redirect ports 1455/1457.
- Persists pending session before returning the OAuth URL.
- Reloads pending sessions after simulated restart.
- Rejects missing, mismatched, and expired state.
- Uses the session's exact redirect URI for token exchange.
- Performs only one token exchange for duplicate callbacks.
- Does not persist authorization codes.
- Upserts credentials by full account hash.
- Extracts optional email metadata without requiring it.

### Integration tests

Use fake authorize/token endpoints and configurable test callback ports:

- Broker preparation → gateway session → callback → exchange → credential save.
- Three sequential accounts without restarting broker or gateway.
- 1455 occupied causes transparent 1457 use.
- Gateway restart between authorization start and callback still completes.
- Broker unavailable prevents browser launch and gives an actionable UI state.
- Failed exchange releases the callback port and reports a sanitized error.

### Frontend tests

- Authorization is not opened before broker and gateway preparation succeed.
- Correct phase transitions are rendered.
- Repeated Codex additions request fresh leases automatically.
- Broker-unavailable and both-ports-busy states are distinct.
- Manual fallback is secondary and explicit.
- Connection list uses display label/email compatibility value rather than UUID-only identification.

### Production acceptance

Starting with zero Codex credentials:

1. Add three different Codex accounts consecutively from the dashboard.
2. Run no Docker or relay commands between accounts.
3. Confirm each row displays an understandable temporary email label.
4. Confirm three distinct account-hash files exist.
5. Confirm each connection passes usage refresh.
6. Confirm 1455 and 1457 are free after every completed callback.
7. Bind a test listener to 1455 after each login to prove release.
8. Restart the gateway after starting a fourth OAuth session, then complete it successfully.
9. Confirm Pi/Codex CLI can subsequently acquire 1455.
10. Run Go, frontend, integration, and Playwright suites.

## Delivery sequence

1. Freeze and remove the one-shot production relay.
2. Add durable OAuth session store and tests.
3. Implement broker core and exhaustive port/lifecycle tests.
4. Implement browser-to-broker handshake and UI states.
5. Add Windows installer, startup, status, and uninstall paths.
6. Remove Compose relay services and obsolete instructions.
7. Run fake-upstream integration and restart tests.
8. Install broker on this host and validate health.
9. Deploy gateway once.
10. Perform production acceptance with clean Codex state.
11. Only then resume the architecture implementation roadmap.

## Rollback

If acceptance fails:

- Disable Codex contribution in the UI with a clear maintenance message.
- Keep existing loaded Codex credentials usable for routing.
- Stop the broker; callback ports remain free.
- Do not restore the manual one-shot relay or device authorization flow.
