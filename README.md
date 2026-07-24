<p align="center">
  <img src="logo.png" alt="codex-pool" width="400">
</p>

<h1 align="center">codex-pool</h1>

<p align="center">
  <strong>Pool your accounts. Share with friends. Never swap credentials again.</strong>
</p>

---

A reverse proxy that distributes coding-agent sessions across pooled provider accounts. Got three Codex accounts? Five Claude logins? The proxy spreads your usage across all of them automatically - no manual switching, no juggling auth files. Google subscription accounts use the Antigravity sign-in flow; Gemini remains the API-key provider.

The setup dashboard configures **Codex CLI**, **Claude Code**, **Gemini CLI**, **Grok Build**, **Pi**. Grok Build runs through the proxy without its own login and can select the other pool models; Pi merges pool providers into its existing `models.json`.

<p align="center">
  <img src="screenshots/analytics-dashboard.png" alt="Pool Analytics" width="700">
</p>

---

## Why

You hit rate limits. You have multiple accounts. Swapping credentials is annoying.

Or maybe you want to pool accounts with friends - everyone throws their accounts into the pot, everyone benefits from the combined capacity.

**codex-pool** handles it:
- Distributes sessions across all your accounts for each service
- Routes to whichever account has capacity
- Pins conversations to the same account (ensures standard cached token performance)
- Auto-refreshes tokens before they expire
- Proxies WebSocket upgrades (including Codex Responses WS and realtime `/ws` flows)
- Tracks usage so you can see who's burning through quota

---

## Screenshots

### Setup Dashboard

<p align="center">
  <img src="screenshots/local-mode.png" alt="Local Mode" width="700">
</p>

### Friends Mode
Share your pool with others using a friend code.

<p align="center">
  <img src="screenshots/friends-mode-login.png" alt="Friends Mode" width="500">
</p>

---

## Quick Start

### 1. Add your accounts

```bash
mkdir -p pool/codex pool/claude pool/gemini pool/antigravity

# Codex accounts
cp ~/.codex/auth.json pool/codex/work.json
cp ~/backup/.codex/auth.json pool/codex/personal.json

# Claude accounts
cp ~/.claude/credentials.json pool/claude/main.json

# Gemini accounts
cp ~/.gemini/oauth_creds.json pool/gemini/main.json
```

Structure:
```
pool/
├── codex/
│   ├── work.json
│   └── personal.json
├── claude/
│   └── main.json
└── gemini/
    └── main.json
```

### 2. Run it

```bash
go build && ./codex-pool
```

### Isolated staging and development

When the main gateway is in active use, keep a pinned staging baseline and a separate active-development project. See [Staging and Development Gateways](docs/development-instance.md).

```bash
# Stable legacy UI baseline (no build)
docker compose -p codex-pool-staging -f docker-compose.staging.yml up -d

# Active source checkout
docker compose -p codex-pool-dev -f docker-compose.dev.yml up -d --build
```

Staging is `http://127.0.0.1:18990`; active development is `http://127.0.0.2:18991`. Each has separate provider, database, user, session, and specification directories, and neither mounts production `pool/` or `data/`.

### 3. Point your CLI

**Codex** - `~/.codex/config.toml`:
```toml
model_provider = "codex-pool"
chatgpt_base_url = "http://127.0.0.1:8989/backend-api"

[model_providers.codex-pool]
name = "OpenAI via codex-pool proxy"
base_url = "http://127.0.0.1:8989/v1"
wire_api = "responses"
requires_openai_auth = true
```

**Claude Code**:
```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8989"
export ANTHROPIC_API_KEY="pool"
```

**Gemini CLI**:
```bash
export CODE_ASSIST_ENDPOINT="http://127.0.0.1:8989"
```

**Codex account**: OpenAI only allowlists Codex OAuth callbacks on loopback ports `1455` and `1457`. Install the per-user local broker once on Windows:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/install-codex-oauth-broker.ps1
```

The dashboard then prepares every Codex login automatically. The broker selects a free allowlisted port, forwards one callback, and releases it immediately; no Docker or relay command is required between accounts. Check or remove it with `-Status` or `-Uninstall`.

**Google Antigravity account**: open the dashboard, choose "Contribute an account", then press "Google Antigravity". The popup completes the callback automatically. Pasting the callback URL remains available when popups are blocked.

The sign-in flow uses Antigravity's shipped Google OAuth client and its fixed `http://localhost:51121/oauth-callback` redirect, matching CLIProxyAPI and VibeProxy. When the pool runs on the same machine as the browser, the popup completes on its own. For a remote pool, paste the failed localhost callback URL into the contribution dialog; the state and PKCE verifier are still checked before exchange.

`ANTIGRAVITY_OAUTH_CLIENT_ID`, `ANTIGRAVITY_OAUTH_CLIENT_SECRET`, and `ANTIGRAVITY_OAUTH_REDIRECT_URI` remain available for tests or a separately registered Google OAuth client. `ANTIGRAVITY_CLIENT_VERSION` overrides the Antigravity client version used in upstream requests. `UPSTREAM_ANTIGRAVITY_BASE`, `UPSTREAM_ANTIGRAVITY_DAILY_BASE`, and `UPSTREAM_ANTIGRAVITY_ONBOARD_BASE` override the production, generation, and onboarding Cloud Code Assist hosts.

---

## Friends Mode

Pool accounts with friends, gated by Google sign-in and an explicit allowlist -
no shared secret to rotate when someone should lose access.

1. In [Google Cloud Console](https://console.cloud.google.com/apis/credentials),
   create an OAuth Client ID of type "Web application" with redirect URI
   `<public_url>/auth/callback/google` (e.g. `http://localhost:8989/auth/callback/google`
   for local use) and scopes `openid email profile`.
2. Configure the pool:

```toml
# config.toml
oauth_google_client_id = "your-client-id.apps.googleusercontent.com"
oauth_google_client_secret = "your-client-secret"
allowed_emails = ["friend1@example.com", "your-domain.com"]
friend_name = "YourName"

[pool_users]
jwt_secret = "32-char-secret-for-jwt-tokens!!"
```

An empty `allowed_emails` denies everyone - it must be set explicitly. Entries
with no `@` are treated as a bare domain and match any address on that domain
(`your-domain.com` allows anyone `@your-domain.com`); entries with an `@` must
match the full address exactly.

Friends open the dashboard, click "Sign in with Google," and (if their email
is on the list) land on the setup instructions and their own credentials.
You see everyone's usage in analytics.

---

## Operator Access

Account enable/disable/resurrect/refresh, provider admin routes, and
`/metrics` require two things: your verified Google email must be in
`admin_emails` (exact addresses only - no bare-domain wildcard, unlike
`allowed_emails`), and a second factor (TOTP, RFC 6238 - Google
Authenticator, Authy, 1Password, any standard authenticator app works).

The first time an admin-listed email opens the Accounts tab, it prompts to
set up 2FA: scan/enter the shown key in an authenticator app, confirm with
the current code, then save the 10 one-time recovery codes shown - they
won't be shown again, and are the normal way back in if you lose the
authenticator (same as GitHub/Google's own 2FA). Losing both the
authenticator and every recovery code means deleting that email's entry
from `data/admin_mfa.json` on the host and re-enrolling - a true last
resort, not the everyday recovery path.

---

## Configuration

```toml
listen_addr = "127.0.0.1:8989"
pool_dir = "pool"

# Friends mode (Google OAuth gate)
oauth_google_client_id = "your-client-id.apps.googleusercontent.com"
oauth_google_client_secret = "your-client-secret"
allowed_emails = ["you@example.com"]
friend_name = "YourName"

# Operator access (exact addresses only, plus TOTP 2FA on first sign-in)
admin_emails = ["you@example.com"]

# Multi-user tracking
[pool_users]
jwt_secret = "32-char-secret-for-jwt-tokens!!"
```

Environment variable `PROXY_MAX_INMEM_BODY_BYTES` controls how large a request body can be before the proxy streams it directly (no retries). Default is 16777216 (16 MiB).

---

## Credential Formats

**Codex** - `pool/codex/*.json`
```json
{"tokens": {"access_token": "...", "refresh_token": "...", "account_id": "acct_..."}}
```

**Claude** - `pool/claude/*.json`
```json
{"claudeAiOauth": {"accessToken": "...", "refreshToken": "...", "expiresAt": 1234567890000}}
```

**Gemini** - `pool/gemini/*.json`
```json
{"access_token": "ya29...", "refresh_token": "1//...", "expiry_date": 1234567890000}
```

**Antigravity** - `pool/antigravity/*.json`
```json
{"type":"antigravity","access_token":"ya29...","refresh_token":"1//...","email":"person@example.com","project_id":"project-id","expiry_date":1234567890000}
```

Antigravity model names come from Google's live `fetchAvailableModels` response. Use `antigravity/<model-id>` to force this provider. `/api/pool/models`, `/v1/models`, `/v1beta/models`, Pi, and the Codex catalog consume the same registry. Temporary quota exhaustion changes `available_now` without removing a supported model from the catalog.

**Kimi, Kimi Platform, MiniMax, Z.ai, Xiaomi, DeepSeek, Qwen, OpenRouter, NVIDIA** - `pool/<provider>/*.json`
```json
{"api_key": "..."}
```
These all authenticate with a plain API key (added via the dashboard's "Contribute an account", or dropped straight into `pool/<provider>/`) against an Anthropic-compatible endpoint - except NVIDIA, which only speaks standard OpenAI Chat Completions.

**Kimi has two distinct products with separate keys.** `pool/kimi/` is the Kimi Code Console's **Coding Plan** - a subscription with an included quota and models such as `kimi-for-coding`. `pool/kimi-platform/` is the pay-as-you-go **Kimi Open Platform**, whose models include `kimi-k3`, `kimi-k2.7-code`, `kimi-k2.6`, and `kimi-k2.5`. Open Platform models route directly by their real IDs; an explicit prefix such as `kimi-platform/kimi-k3` is also accepted and stripped upstream. Open Platform keys from `platform.kimi.ai` and `platform.kimi.com` are not interchangeable, so `UPSTREAM_KIMI_PLATFORM_BASE` must target the platform where the key was created.

**OpenRouter and NVIDIA are aggregators**, not single-model-family providers - they route to hundreds of vendor-prefixed models rather than a fixed catalog. Force a request to one of them with an explicit prefix: `openrouter/<vendor>/<model>` (e.g. `openrouter/anthropic/claude-opus-4.5`) or `nvidia/<vendor>/<model>` (e.g. `nvidia/meta/llama-3.3-70b-instruct`) - the prefix is stripped before the request reaches the upstream. Same convention as `antigravity/<model-id>` above.

---

## Disclaimer

This pools credentials you own. Using multiple accounts or sharing access may violate terms of service. If something goes sideways, that's on you.

---

## License

MIT
