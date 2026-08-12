package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// The API gateway is API-only. The SPA is served by the dedicated `web`
// service in front of the gateway; this binary never embeds or serves HTML,
// scripts, or other frontend assets.

// gatewaySessionResponse is the CLI-credential bundle plus identity/admin
// status returned by GET /api/pool/session.
type gatewaySessionResponse struct {
	PublicURL      string `json:"public_url"`
	Email          string `json:"email"`
	IsAdmin        bool   `json:"is_admin"`
	MFAEnrolled    bool   `json:"mfa_enrolled"`
	OriginID       string `json:"origin_id"`
	DownloadToken  string `json:"download_token"`
	AuthJSON       string `json:"auth_json"`
	GeminiAuthJSON string `json:"gemini_auth_json"`
	GeminiAPIKey   string `json:"gemini_api_key"`
	PiModelsJSON   string `json:"pi_models_json"`
}

// writeGatewaySessionJSON builds the CLI-credential bundle for an already
// resolved, already-authorized pool user and writes it as JSON. This is what
// GET /api/pool/session returns once the Google OAuth gate has established a
// session (see oauth_login.go) - the identity check happens before this is
// called, not inside it.
func (h *proxyHandler) writeGatewaySessionJSON(w http.ResponseWriter, r *http.Request, user *GatewayUser) {
	secret := getPoolJWTSecret()
	authData, err := generateCodexAuth(secret, user)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "Failed to generate credentials.")
		return
	}
	authJSONBytes, _ := json.MarshalIndent(authData, "", "  ")

	// Generate Gemini Auth JSON
	geminiAuthData, err := generateGeminiAuth(secret, user)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "Failed to generate gemini credentials.")
		return
	}
	geminiJSONBytes, _ := json.MarshalIndent(geminiAuthData, "", "  ")

	codexAccessToken := ""
	if authData.Tokens != nil {
		codexAccessToken = authData.Tokens.AccessToken
	}
	// Anthropic-compatible providers (Kimi, MiniMax, Z.ai, etc.) share a pool
	// credential format with the removed Claude account integration.
	anthropicPoolKey := generateClaudePoolToken(secret, user.ID)
	piModelsJSON, err := generatePiModelsJSON(h.getEffectiveModelAPIURL(r), codexAccessToken, anthropicPoolKey, h.pool, h.pricing)
	if err != nil {
		respondJSONError(w, http.StatusInternalServerError, "Failed to generate pi models config.")
		return
	}

	// Generate Gemini API key for API key mode (bypasses OAuth)
	geminiAPIKey := generateGeminiAPIKey(secret, user)

	publicURL := h.getEffectivePublicURL(r)

	isAdmin := adminEmailAllowed(h.cfg.adminEmails, user.Email)
	mfaEnrolled := false
	if isAdmin && h.adminTOTP != nil {
		if entry := h.adminTOTP.Get(user.Email); entry != nil && entry.Confirmed {
			mfaEnrolled = true
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gatewaySessionResponse{
		PublicURL:      publicURL,
		Email:          user.Email,
		IsAdmin:        isAdmin,
		MFAEnrolled:    mfaEnrolled,
		OriginID:       hashRequestOrigin(r, poolHashSalt(getPoolJWTSecret())),
		DownloadToken:  user.Token,
		AuthJSON:       string(authJSONBytes),
		GeminiAuthJSON: string(geminiJSONBytes),
		GeminiAPIKey:   geminiAPIKey,               // API key for Gemini CLI API key mode
		PiModelsJSON:   string(piModelsJSON),
	})
}

func (h *proxyHandler) getEffectivePublicURL(r *http.Request) string {
	if u := getPublicURL(); u != "" {
		return u
	}
	// Infer from request
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8989"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

// getEffectiveModelAPIURL returns the base URL that generated client
// configurations should use for model traffic. When MODEL_API_BASE_URL is
// configured (frontend/backend split), it points at the private model port;
// otherwise it falls back to the public URL for single-endpoint deployments.
func (h *proxyHandler) getEffectiveModelAPIURL(r *http.Request) string {
	if u := getModelAPIBaseURL(); u != "" {
		return strings.TrimRight(u, "/")
	}
	return h.getEffectivePublicURL(r)
}

func wantsPowerShell(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("shell"))) {
	case "powershell", "pwsh", "ps", "ps1":
		return true
	default:
		return false
	}
}

func (h *proxyHandler) serveCodexSetupScript(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/setup/codex/")
	if token == "" || strings.Contains(token, "/") {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}
	publicURL := h.getEffectivePublicURL(r)
	modelBase := h.getEffectiveModelAPIURL(r)

	if wantsPowerShell(r) {
		script := fmt.Sprintf(`#requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Token = '%s'
$BaseUrl = '%s'
$ModelBase = '%s'

$authDir = Join-Path $HOME '.codex'
$configFile = Join-Path $authDir 'config.toml'
$authFile = Join-Path $authDir 'auth.json'
$modelCatalog = Join-Path $authDir 'model_catalog.json'
$mcpScript = Join-Path $authDir 'model_sync.ps1'
$nl = [Environment]::NewLine

# PS 5.1 compat wrapper: ConvertFrom-Json -Depth was added in PS 6
function ConvertFrom-JsonCompat {
  param([Parameter(ValueFromPipeline)]$InputObject)
  process {
    if ($PSVersionTable.PSVersion.Major -ge 6) {
      $InputObject | ConvertFrom-Json -Depth 20
    } else {
      $InputObject | ConvertFrom-Json
    }
  }
}

# PS 5.1 writes UTF-8 with BOM which breaks JSON/TOML parsers. Write without BOM.
function Set-Utf8NoBom {
  param([string]$Path, [string]$Value)
  $utf8 = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $Value, $utf8)
}

# Safe property check that works with Set-StrictMode -Version Latest
function Has-Property {
  param($Obj, [string]$Name)
  if ($null -eq $Obj) { return $false }
  if ($Obj -is [System.Collections.IDictionary]) { return $Obj.ContainsKey($Name) }
  return [bool]($Obj.PSObject.Properties.Name -contains $Name)
}

Write-Host 'Initializing Codex Pool setup...'
New-Item -ItemType Directory -Path $authDir -Force | Out-Null

Write-Host '1. Fetching credentials...'
$authUrl = "$BaseUrl/config/codex/$Token"
if ($PSVersionTable.PSEdition -eq 'Desktop') {
  $authContent = (Invoke-WebRequest -UseBasicParsing -Uri $authUrl).Content
} else {
  $authContent = (Invoke-WebRequest -Uri $authUrl).Content
}
Set-Utf8NoBom -Path $authFile -Value $authContent

Write-Host '2. Fetching model catalog...'
try {
  $raw = Get-Content -Path $authFile -Raw
  $auth = $raw | ConvertFrom-JsonCompat
  $accessToken = $null
  if ($auth -and (Has-Property $auth 'tokens') -and (Has-Property $auth.tokens 'access_token')) {
    $accessToken = [string]$auth.tokens.access_token
  } elseif ($auth -and (Has-Property $auth 'access_token')) {
    $accessToken = [string]$auth.access_token
  }
  if (-not [string]::IsNullOrWhiteSpace($accessToken)) {
    $modelsUrl = $ModelBase.TrimEnd('/') + '/backend-api/codex/models?client_version=0.125.0'
    $headers = @{ Authorization = "Bearer $accessToken" }
    $tmp = [System.IO.Path]::GetTempFileName()
    try {
      if ($PSVersionTable.PSEdition -eq 'Desktop') {
        Invoke-WebRequest -UseBasicParsing -Uri $modelsUrl -Headers $headers -OutFile $tmp -TimeoutSec 10 | Out-Null
      } else {
        Invoke-WebRequest -Uri $modelsUrl -Headers $headers -OutFile $tmp -TimeoutSec 10 | Out-Null
      }
      Move-Item -Force -Path $tmp -Destination $modelCatalog
      Write-Host "Model catalog saved to $modelCatalog"
    } catch {
      if (Test-Path $tmp) { Remove-Item -Force $tmp -ErrorAction SilentlyContinue }
      Write-Host "Warning: Could not fetch model catalog (non-fatal): $_"
    }
  }
} catch {
  Write-Host "Warning: Could not parse auth for model catalog fetch (non-fatal): $_"
}

Write-Host '3. Installing model sync MCP sidecar...'
$mcpContent = @'
param(
  [string]$BaseUrl = ""
)

$ErrorActionPreference = 'Stop'

$authDir = Join-Path $HOME '.codex'
$authFile = Join-Path $authDir 'auth.json'
$modelCatalog = Join-Path $authDir 'model_catalog.json'

# PS 5.1 compat wrapper: ConvertFrom-Json -Depth was added in PS 6
function ConvertFrom-JsonCompat {
  param([Parameter(ValueFromPipeline)]$InputObject)
  process {
    if ($PSVersionTable.PSVersion.Major -ge 6) {
      $InputObject | ConvertFrom-Json -Depth 20
    } else {
      $InputObject | ConvertFrom-Json
    }
  }
}

function Refresh-ModelCatalog {
  param([string]$Url)
  if ([string]::IsNullOrWhiteSpace($Url)) { return }
  if (-not (Test-Path $authFile)) { return }

  try {
    $raw = Get-Content -Path $authFile -Raw
    $auth = $raw | ConvertFrom-JsonCompat
  } catch {
    return
  }

  $token = $null
  if ($auth -and $auth.tokens -and $auth.tokens.access_token) {
    $token = [string]$auth.tokens.access_token
  } elseif ($auth -and $auth.access_token) {
    $token = [string]$auth.access_token
  }
  if ([string]::IsNullOrWhiteSpace($token)) { return }

  $modelsUrl = $Url.TrimEnd('/') + '/backend-api/codex/models?client_version=0.125.0'
  $headers = @{ Authorization = "Bearer $token" }

  try {
    $tmp = [System.IO.Path]::GetTempFileName()
    if ($PSVersionTable.PSEdition -eq 'Desktop') {
      Invoke-WebRequest -UseBasicParsing -Uri $modelsUrl -Headers $headers -OutFile $tmp -TimeoutSec 5 | Out-Null
    } else {
      Invoke-WebRequest -Uri $modelsUrl -Headers $headers -OutFile $tmp -TimeoutSec 5 | Out-Null
    }
    Move-Item -Force -Path $tmp -Destination $modelCatalog
  } catch {
    if ($tmp -and (Test-Path $tmp)) { Remove-Item -Force -Path $tmp -ErrorAction SilentlyContinue }
  }
}

function Write-McpResponse {
  param(
    [string]$Payload,
    [string]$Transport = 'framed'
  )

  if ($Transport -eq 'jsonl') {
    [Console]::Out.WriteLine($Payload)
    [Console]::Out.Flush()
    return
  }

  $bytes = [System.Text.Encoding]::UTF8.GetBytes($Payload)
  $nl = [Environment]::NewLine
  [Console]::Out.Write("Content-Length: " + $bytes.Length + $nl + $nl + $Payload)
  [Console]::Out.Flush()
}

Refresh-ModelCatalog -Url $BaseUrl

while ($true) {
  $transport = 'framed'
  $contentLength = 0
  $body = ''

  $firstLine = [Console]::In.ReadLine()
  if ($null -eq $firstLine) { exit 0 }

  if ($firstLine -match '^[ \t]*\{') {
    $transport = 'jsonl'
    $body = $firstLine
  } else {
    $line = $firstLine
    while ($true) {
      if ($line -eq '') { break }
      if ($line -match '^(?i)content-length:') {
        $lengthText = ($line -split ':', 2)[1].Trim()
        [int]::TryParse($lengthText, [ref]$contentLength) | Out-Null
      }
      $line = [Console]::In.ReadLine()
      if ($null -eq $line) { exit 0 }
    }
  }

  if ($transport -eq 'framed' -and $contentLength -le 0) { continue }
  if ($transport -eq 'framed') {
    $buffer = New-Object char[] $contentLength
    $readTotal = 0
    while ($readTotal -lt $contentLength) {
      $readNow = [Console]::In.Read($buffer, $readTotal, $contentLength - $readTotal)
      if ($readNow -le 0) { exit 0 }
      $readTotal += $readNow
    }
    $body = -join $buffer
  }

  try {
    $request = $body | ConvertFrom-JsonCompat
  } catch {
    continue
  }

  if (-not $request.PSObject.Properties.Name.Contains('id')) {
    continue
  }

  $method = ''
  if ($request.PSObject.Properties.Name.Contains('method')) {
    $method = [string]$request.method
  }

  $result = $null
  switch ($method) {
    'initialize' {
      $result = @{
        protocolVersion = '2024-11-05'
        capabilities = @{
          tools = @{ listChanged = $false }
          resources = @{ listChanged = $false }
          prompts = @{ listChanged = $false }
        }
        serverInfo = @{
          name = 'model_sync'
          version = '1.0.0'
        }
      }
    }
    'tools/list' { $result = @{ tools = @() } }
    'resources/list' { $result = @{ resources = @() } }
    'prompts/list' { $result = @{ prompts = @() } }
    'ping' { $result = @{} }
    default {
      $errorResponse = @{
        jsonrpc = '2.0'
        id = $request.id
        error = @{
          code = -32601
          message = 'Method not found'
        }
      } | ConvertTo-Json -Compress -Depth 20
      Write-McpResponse -Payload $errorResponse -Transport $transport
      continue
    }
  }

  $response = @{
    jsonrpc = '2.0'
    id = $request.id
    result = $result
  } | ConvertTo-Json -Compress -Depth 20
  Write-McpResponse -Payload $response -Transport $transport
}
'@
Set-Content -Path $mcpScript -Value $mcpContent -Encoding UTF8

# Find PowerShell executable path robustly
$mcpCommand = $null
try { $mcpCommand = (Get-Process -Id $PID).Path } catch {}
if ([string]::IsNullOrWhiteSpace($mcpCommand)) {
  try { $mcpCommand = (Get-Command pwsh -ErrorAction SilentlyContinue).Source } catch {}
}
if ([string]::IsNullOrWhiteSpace($mcpCommand)) {
  try { $mcpCommand = (Get-Command powershell -ErrorAction SilentlyContinue).Source } catch {}
}
if ([string]::IsNullOrWhiteSpace($mcpCommand)) {
  $mcpCommand = 'powershell'
}

$modelCatalogToml = $modelCatalog -replace '\\', '\\\\'
$mcpScriptToml = $mcpScript -replace '\\', '\\\\'
$mcpCommandToml = $mcpCommand -replace '\\', '\\\\'

Write-Host '4. Updating configuration...'
if (-not (Test-Path $configFile)) {
  New-Item -ItemType File -Path $configFile -Force | Out-Null
}

$existing = ''
try { $existing = Get-Content -Path $configFile -Raw } catch {}
if ($null -eq $existing) { $existing = '' }

if ($existing -notmatch 'codex-pool') {
  $new = @"
# Codex Pool Proxy Config
model_provider = "codex-pool"
chatgpt_base_url = "$ModelBase/backend-api"
model_catalog_json = "$modelCatalogToml"

$existing

[model_providers.codex-pool]
name = "OpenAI via codex-pool proxy"
base_url = "$BaseUrl"
wire_api = "responses"
requires_openai_auth = true
supports_websockets = true

[model_providers.codex-pool.features]
responses_websockets_v2 = true

[mcp_servers.model_sync]
command = "$mcpCommandToml"
args = ["-NoLogo", "-NoProfile", "-File", "$mcpScriptToml", "$ModelBase"]
"@

  Set-Utf8NoBom -Path $configFile -Value $new
  Write-Host "Configuration updated in $configFile"
} else {
  $updated = $false

  if ($existing -notmatch '(?m)^[ \t]*model_catalog_json[ \t]*=') {
    $existing = 'model_catalog_json = "' + $modelCatalogToml + '"' + $nl + $existing
    $updated = $true
  }

  if ($existing -match '(?m)^\[mcp_servers\.codex_pool_model_sync\]') {
    $existing = $existing -replace '(?m)^\[mcp_servers\.codex_pool_model_sync\]', '[mcp_servers.model_sync]'
    $updated = $true
  }

  if ($existing -notmatch '(?m)^\[mcp_servers\.model_sync\]') {
    $existing = $existing.TrimEnd() +
      $nl + $nl +
      '[mcp_servers.model_sync]' + $nl +
      'command = "' + $mcpCommandToml + '"' + $nl +
      'args = ["-NoLogo", "-NoProfile", "-File", "' + $mcpScriptToml + '", "' + $ModelBase + '"]' + $nl
    $updated = $true
  }

  if ($updated) {
    Set-Utf8NoBom -Path $configFile -Value $existing
    Write-Host "Configuration updated in $configFile"
  } else {
    Write-Host "Configuration already present in $configFile. Skipping."
  }
}

Write-Host 'Setup complete! You are ready to use the pool.'
`, token, publicURL, modelBase)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(script))
		return
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
TOKEN="%s"
BASE_URL="%s"
MODEL_BASE="%s"
AUTH_DIR="$HOME/.codex"
CONFIG_FILE="$AUTH_DIR/config.toml"
AUTH_FILE="$AUTH_DIR/auth.json"
MODEL_CATALOG="$AUTH_DIR/model_catalog.json"
MCP_SCRIPT="$AUTH_DIR/model_sync.sh"

echo "Initializing Codex Pool setup..."
mkdir -p "$AUTH_DIR"

echo "1. Fetching credentials..."
curl -sL "$BASE_URL/config/codex/$TOKEN" -o "$AUTH_FILE"
chmod 600 "$AUTH_FILE"

echo "2. Fetching model catalog..."
ACCESS_TOKEN=$(sed -n 's/.*"access_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$AUTH_FILE" | head -n 1)
if [ -n "${ACCESS_TOKEN:-}" ]; then
    curl --connect-timeout 5 --max-time 10 -fsSL \
        -H "Authorization: Bearer $ACCESS_TOKEN" \
        "$MODEL_BASE/backend-api/codex/models?client_version=0.125.0" \
        -o "$MODEL_CATALOG" 2>/dev/null && chmod 600 "$MODEL_CATALOG" 2>/dev/null || true
fi

echo "3. Installing model sync MCP sidecar..."
cat <<'EOF' > "$MCP_SCRIPT"
#!/bin/bash
set -euo pipefail

BASE_URL="${1:-}"
AUTH_DIR="${HOME}/.codex"
AUTH_FILE="$AUTH_DIR/auth.json"
MODEL_CATALOG="$AUTH_DIR/model_catalog.json"
CLIENT_VERSION="0.125.0"

refresh_model_catalog() {
    if [ -z "$BASE_URL" ] || [ ! -f "$AUTH_FILE" ]; then
        return 0
    fi

    local token
    token=$(sed -n 's/.*"access_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$AUTH_FILE" | head -n 1)
    if [ -z "${token:-}" ]; then
        return 0
    fi

    local tmp_file
    tmp_file=$(mktemp "${MODEL_CATALOG}.tmp.XXXXXX")
    if curl --connect-timeout 2 --max-time 5 -fsSL -H "Authorization: Bearer $token" \
        "${MODEL_BASE%%/}/backend-api/codex/models?client_version=${CLIENT_VERSION}" \
        -o "$tmp_file"; then
        mv "$tmp_file" "$MODEL_CATALOG"
        chmod 600 "$MODEL_CATALOG" 2>/dev/null || true
    else
        rm -f "$tmp_file"
    fi
}

read_request() {
    local line content_length
    content_length=0

    if ! IFS= read -r line; then
        return 1
    fi
    line="${line%%$'\r'}"

    if [[ "$line" == \{* ]]; then
        MCP_TRANSPORT_MODE="jsonl"
        REQUEST_BODY="$line"
        return 0
    fi

    while true; do
        if [ -z "$line" ]; then
            break
        fi
        case "$line" in
            [Cc]ontent-[Ll]ength:*|[Cc]ONTENT-[Ll]ENGTH:*|CONTENT-LENGTH:*|content-length:*)
                content_length=$(printf '%%s' "${line#*:}" | tr -d '[:space:]')
                ;;
        esac
        if ! IFS= read -r line; then
            return 1
        fi
        line="${line%%$'\r'}"
    done

    if [ -z "$content_length" ] || ! [[ "$content_length" =~ ^[0-9]+$ ]] || [ "$content_length" -le 0 ]; then
        return 1
    fi

    MCP_TRANSPORT_MODE="framed"
    REQUEST_BODY=$(dd bs=1 count="$content_length" 2>/dev/null)
    return 0
}

write_response() {
    local payload="$1"
    if [ "${MCP_TRANSPORT_MODE:-framed}" = "jsonl" ]; then
        printf '%%s\n' "$payload"
        return
    fi

    local length
    length=$(printf '%%s' "$payload" | LC_ALL=C wc -c | tr -d '[:space:]')
    printf 'Content-Length: %%s\r\n\r\n%%s' "$length" "$payload"
}

handle_request() {
    local request="$1"
    local method id payload

    method=$(printf '%%s' "$request" | sed -n 's/.*"method"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
    id=$(printf '%%s' "$request" | sed -n 's/.*"id"[[:space:]]*:[[:space:]]*\([^,}]*\).*/\1/p' | head -n 1)
    if [ -z "${id:-}" ]; then
        return 0
    fi

    case "$method" in
        initialize)
            payload='{"jsonrpc":"2.0","id":'"$id"',"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{"listChanged":false},"resources":{"listChanged":false},"prompts":{"listChanged":false}},"serverInfo":{"name":"model_sync","version":"1.0.0"}}}'
            ;;
        tools/list)
            payload='{"jsonrpc":"2.0","id":'"$id"',"result":{"tools":[]}}'
            ;;
        resources/list)
            payload='{"jsonrpc":"2.0","id":'"$id"',"result":{"resources":[]}}'
            ;;
        prompts/list)
            payload='{"jsonrpc":"2.0","id":'"$id"',"result":{"prompts":[]}}'
            ;;
        ping)
            payload='{"jsonrpc":"2.0","id":'"$id"',"result":{}}'
            ;;
        *)
            payload='{"jsonrpc":"2.0","id":'"$id"',"error":{"code":-32601,"message":"Method not found"}}'
            ;;
    esac

    write_response "$payload"
}

refresh_model_catalog >/dev/null 2>&1 &

while true; do
    REQUEST_BODY=""
    if ! read_request; then
        exit 0
    fi
    handle_request "$REQUEST_BODY"
done
EOF
chmod 700 "$MCP_SCRIPT"

echo "4. Updating configuration..."
if [ ! -f "$CONFIG_FILE" ]; then
    touch "$CONFIG_FILE"
fi

# Check if config already exists to avoid duplication
if ! grep -q "codex-pool" "$CONFIG_FILE"; then
    # Create temp file with pool config at TOP, then append existing config
    TEMP_FILE=$(mktemp)
    cat <<EOF > "$TEMP_FILE"
# Codex Pool Proxy Config
model_provider = "codex-pool"
chatgpt_base_url = "$MODEL_BASE/backend-api"
model_catalog_json = "$MODEL_CATALOG"

EOF
    # Append existing config
    cat "$CONFIG_FILE" >> "$TEMP_FILE"

    # Add model_providers section at the end (sections go after top-level keys)
    cat <<EOF >> "$TEMP_FILE"

[model_providers.codex-pool]
name = "OpenAI via codex-pool proxy"
base_url = "$BASE_URL"
wire_api = "responses"
requires_openai_auth = true
supports_websockets = true

[model_providers.codex-pool.features]
responses_websockets_v2 = true

[mcp_servers.model_sync]
command = "bash"
args = ["$MCP_SCRIPT", "$MODEL_BASE"]
EOF

    mv "$TEMP_FILE" "$CONFIG_FILE"
    chmod 600 "$CONFIG_FILE"
    echo "Configuration updated in $CONFIG_FILE"
else
    UPDATED=0

    if ! grep -Eq '^[[:space:]]*model_catalog_json[[:space:]]*=' "$CONFIG_FILE"; then
        TEMP_FILE=$(mktemp)
        cat <<EOF > "$TEMP_FILE"
model_catalog_json = "$MODEL_CATALOG"
EOF
        cat "$CONFIG_FILE" >> "$TEMP_FILE"
        mv "$TEMP_FILE" "$CONFIG_FILE"
        UPDATED=1
    fi

    if grep -q '^\[mcp_servers\.codex_pool_model_sync\]' "$CONFIG_FILE"; then
        TEMP_FILE=$(mktemp)
        sed 's/^\[mcp_servers\.codex_pool_model_sync\]/[mcp_servers.model_sync]/' "$CONFIG_FILE" > "$TEMP_FILE"
        mv "$TEMP_FILE" "$CONFIG_FILE"
        UPDATED=1
    fi

    if ! grep -q '^\[mcp_servers\.model_sync\]' "$CONFIG_FILE"; then
        cat <<EOF >> "$CONFIG_FILE"

[mcp_servers.model_sync]
command = "bash"
args = ["$MCP_SCRIPT", "$MODEL_BASE"]
EOF
        UPDATED=1
    fi

    chmod 600 "$CONFIG_FILE"
    if [ "$UPDATED" -eq 1 ]; then
        echo "Configuration updated in $CONFIG_FILE"
    else
        echo "Configuration already present in $CONFIG_FILE. Skipping."
    fi
fi

echo "Setup complete! You are ready to use the pool."
`, token, publicURL, modelBase)

	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Write([]byte(script))
}

func (h *proxyHandler) serveGeminiSetupScript(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/setup/gemini/")
	if token == "" || strings.Contains(token, "/") {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}

	// Validate token and get user to generate credentials
	if h.poolUsers == nil {
		http.Error(w, "pool users not configured", http.StatusServiceUnavailable)
		return
	}
	user := h.poolUsers.GetByToken(token)
	if user == nil {
		http.Error(w, "invalid token", http.StatusNotFound)
		return
	}
	if user.Disabled {
		http.Error(w, "user disabled", http.StatusForbidden)
		return
	}

	// Generate pool OAuth credentials for this user
	secret := getPoolJWTSecret()
	if secret == "" {
		http.Error(w, "JWT secret not configured", http.StatusServiceUnavailable)
		return
	}
	geminiAuth, err := generateGeminiAuth(secret, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Gemini CLI config points entirely at the model API, which is internal in
	// the split deployment, so use the model base URL rather than the public one.
	publicURL := h.getEffectiveModelAPIURL(r)

	// Script sets env vars to bypass Google OAuth validation and route through proxy
	// Uses GOOGLE_GENAI_USE_GCA + GOOGLE_CLOUD_ACCESS_TOKEN to skip getTokenInfo() check
	if wantsPowerShell(r) {
		script := fmt.Sprintf(`#requires -Version 5.1
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$BaseUrl = '%s'
$PoolToken = '%s'

# PS 5.1 writes UTF-8 with BOM which breaks parsers. Write without BOM.
function Set-Utf8NoBom {
  param([string]$Path, [string]$Value)
  $utf8 = New-Object System.Text.UTF8Encoding($false)
  [System.IO.File]::WriteAllText($Path, $Value, $utf8)
}

Write-Host 'Configuring Gemini CLI for pool access...'
Write-Host ''

# Set env vars for the current session
$env:CODE_ASSIST_ENDPOINT = $BaseUrl
$env:GOOGLE_GENAI_USE_GCA = '1'
$env:GOOGLE_CLOUD_ACCESS_TOKEN = $PoolToken

# Persist env vars for future PowerShell sessions
$profilePath = $PROFILE.CurrentUserAllHosts
New-Item -ItemType Directory -Force -Path (Split-Path $profilePath) | Out-Null
if (-not (Test-Path $profilePath)) { New-Item -ItemType File -Force -Path $profilePath | Out-Null }

$start = '# >>> Gemini Pool Configuration >>>'
$end = '# <<< Gemini Pool Configuration <<<'
$nl = [Environment]::NewLine
$blockLines = @(
  $start,
  ('$env:CODE_ASSIST_ENDPOINT = "' + $BaseUrl + '"'),
  ('$env:GOOGLE_GENAI_USE_GCA = "1"'),
  ('$env:GOOGLE_CLOUD_ACCESS_TOKEN = "' + $PoolToken + '"'),
  $end
)
$block = $blockLines -join $nl

$existing = ''
try { $existing = Get-Content -Path $profilePath -Raw } catch {}
if ($null -eq $existing) { $existing = '' }

$pattern = [regex]::Escape($start) + '.*?' + [regex]::Escape($end)
if ([regex]::IsMatch($existing, $pattern, [Text.RegularExpressions.RegexOptions]::Singleline)) {
  $updated = [regex]::Replace($existing, $pattern, $block, [Text.RegularExpressions.RegexOptions]::Singleline)
} else {
  $sep = ''; if ($existing -and -not ($existing.EndsWith($nl))) { $sep = $nl }
  $updated = $existing + $sep + $nl + $block + $nl
}

Set-Utf8NoBom -Path $profilePath -Value $updated
Write-Host ("Added Gemini pool config to " + $profilePath)

Write-Host ''
Write-Host 'Setup complete!'
Write-Host ''
Write-Host ("Gemini CLI will use the pool proxy at: " + $BaseUrl)
Write-Host 'No Google login required - validation is bypassed.'
Write-Host ''
Write-Host 'Start a new terminal, or run: . $PROFILE'
`, publicURL, geminiAuth.AccessToken)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(script))
		return
	}

	script := fmt.Sprintf(`#!/bin/bash
set -e
BASE_URL="%s"
POOL_TOKEN="%s"

echo "Configuring Gemini CLI for pool access..."
echo ""

# Add env vars to shell profile
add_to_profile() {
    for profile in "$HOME/.zshrc" "$HOME/.bashrc"; do
        if [ -f "$profile" ]; then
            # Remove old Gemini-related env vars
            grep -v "GEMINI_API_KEY=" "$profile" 2>/dev/null | \
            grep -v "GOOGLE_GEMINI_BASE_URL=" 2>/dev/null | \
            grep -v "CODE_ASSIST_ENDPOINT=" 2>/dev/null | \
            grep -v "GOOGLE_GENAI_USE_GCA=" 2>/dev/null | \
            grep -v "GOOGLE_CLOUD_ACCESS_TOKEN=" 2>/dev/null > "$profile.tmp" || true
            mv "$profile.tmp" "$profile"

            # Add pool configuration
            cat >> "$profile" << 'ENVEOF'

# Gemini Pool Configuration
export CODE_ASSIST_ENDPOINT="%s"
export GOOGLE_GENAI_USE_GCA=1
export GOOGLE_CLOUD_ACCESS_TOKEN="%s"
ENVEOF
            echo "✓ Added Gemini pool config to $(basename $profile)"
            return
        fi
    done

    # Fallback: create .zshrc
    cat >> "$HOME/.zshrc" << 'ENVEOF'

# Gemini Pool Configuration
export CODE_ASSIST_ENDPOINT="%s"
export GOOGLE_GENAI_USE_GCA=1
export GOOGLE_CLOUD_ACCESS_TOKEN="%s"
ENVEOF
    echo "✓ Created ~/.zshrc with Gemini pool config"
}

add_to_profile

echo ""
echo "Setup complete!"
echo ""
echo "Gemini CLI will use the pool proxy at: $BASE_URL"
echo "No Google login required - validation is bypassed."
echo ""
echo "Run 'source ~/.zshrc' or start a new terminal, then run 'gemini'."
`, publicURL, geminiAuth.AccessToken,
		publicURL, geminiAuth.AccessToken,
		publicURL, geminiAuth.AccessToken)

	w.Header().Set("Content-Type", "text/x-shellscript")
	w.Write([]byte(script))
}

func hashAccountID(id string) string {
	h := sha256.Sum256([]byte(id + "pool-salt-2024"))
	return hex.EncodeToString(h[:])[:12]
}

// formatPlanWithTier appends the rate limit tier suffix (e.g. "max 20x") if available.
func formatPlanWithTier(planType, tier string) string {
	switch tier {
	case "default_claude_max_20x":
		return planType + " 20x"
	case "default_claude_max_5x":
		return planType + " 5x"
	default:
		return planType
	}
}

// PoolStats represents anonymized pool statistics
type PoolStats struct {
	TotalAccounts    int                       `json:"total_accounts"`
	ActiveAccounts   int                       `json:"active_accounts"`
	TotalPoolUsers   int                       `json:"total_pool_users"`
	Accounts         []ProviderConnectionStats `json:"accounts"`
	AggregateUsage   AggregateStats            `json:"aggregate"`
	CapacityAnalysis *CapacityAnalysis         `json:"capacity_analysis,omitempty"`
	Last24hTokens    int64                     `json:"last_24h_tokens"`
	DailyCosts       []DailyCostEntry          `json:"daily_costs,omitempty"`
	CyberPolicy      CyberPolicyStats          `json:"cyber_policy"`
	GeneratedAt      time.Time                 `json:"generated_at"`
}

// CyberPolicyStats summarizes how often the cyber_policy safety net
// has fired since process start. Operators read this to alert when
// suppressions happen without successful swaps (i.e. the cyber pool
// is depleted) or when synthetic refusals are being emitted.
type CyberPolicyStats struct {
	// Healthy is true when there's been at least one cyber_policy
	// detection AND every detection was paired with either a
	// successful swap or a buffered/4xx retry. False when suppressions
	// fell back to synthetic refusal (no cyber candidate available).
	Healthy bool `json:"healthy"`
	// CyberCandidatesAvailable is the count of live, non-disabled
	// cyber_access accounts in the pool right now. Zero means the
	// next cyber_policy hit will fall back to a synthetic refusal.
	CyberCandidatesAvailable int `json:"cyber_candidates_available"`
	// Counters by action: "suppressed_ws", "suppressed_sse",
	// "suppressed_buffered", "swap_succeeded", "swap_no_candidate",
	// "retry_buffered", "retry_4xx".
	Counters map[string]int64 `json:"counters"`
	// PerAccount["shiv_1"]["suppressed_ws"] = 5 — useful when you
	// want to see which non-cyber account is hitting the policy
	// classifier most often.
	PerAccount map[string]map[string]int64 `json:"per_account,omitempty"`
}

type ProviderConnectionStats struct {
	ID                        string            `json:"id"` // hashed connection ID
	DisplayName               string            `json:"display_name"`
	ExternalSubject           string            `json:"external_subject,omitempty"`
	IdentityAttributes        map[string]string `json:"identity_attributes,omitempty"`
	UpstreamAccountID         string            `json:"upstream_account_id,omitempty"` // Deprecated compatibility field.
	AccountEmail              string            `json:"account_email,omitempty"`       // Deprecated compatibility field.
	Type                      string            `json:"type"`
	PlanType                  string            `json:"plan_type"`
	Status                    string            `json:"status"` // healthy, degraded, dead
	Penalty                   float64           `json:"penalty"`
	PrimaryWindowUsed         float64           `json:"primary_window_used_pct"`
	SecondaryWindowUsed       float64           `json:"secondary_window_used_pct"`
	PrimaryWindowAvailable    bool              `json:"primary_window_available"`
	SecondaryWindowAvailable  bool              `json:"secondary_window_available"`
	PrimaryResetMinutes       int               `json:"primary_reset_minutes"`
	SecondaryResetMinutes     int               `json:"secondary_reset_minutes"`
	PrimaryWindowMinutes      int               `json:"primary_window_minutes"`
	SecondaryWindowMinutes    int               `json:"secondary_window_minutes"`
	PrimaryPaceRatio          float64           `json:"primary_pace_ratio"`
	SecondaryPaceRatio        float64           `json:"secondary_pace_ratio"`
	AccountAddedAt            string            `json:"account_added_at,omitempty"`
	TotalInputTokens          int64             `json:"total_input_tokens"`
	TotalCachedTokens         int64             `json:"total_cached_tokens"`
	TotalOutputTokens         int64             `json:"total_output_tokens"`
	TotalReasoningTokens      int64             `json:"total_reasoning_tokens"`
	TotalBillableTokens       int64             `json:"total_billable_tokens"`
	CacheHitRate              float64           `json:"cache_hit_rate_pct"`
	CreditsBalance            float64           `json:"credits_balance,omitempty"`
	HasCredits                bool              `json:"has_credits"`
	Score                     float64           `json:"score"`
	ScoreTooltip              string            `json:"score_tooltip,omitempty"`
	IsPrimary                 bool              `json:"is_primary"` // highest score for this provider type
	SubscriptionCostMonthly   float64           `json:"subscription_cost_monthly"`
	SubscriptionSpend         float64           `json:"subscription_spend"`
	SubscriptionBillingCycles int               `json:"subscription_billing_cycles"`
	CostTrackingStartedAt     string            `json:"cost_tracking_started_at,omitempty"`
	SubscriptionLabel         string            `json:"subscription_label"`
	APICostEstimate           float64           `json:"api_cost_estimate"` // all-time
	APICostLast30d            float64           `json:"api_cost_last_30d"` // last 30 days
	ROI                       float64           `json:"roi"`               // all-time API value / subscription spend
	ResetCreditsAvailable     int               `json:"reset_credits_available"`
	ResetCreditExpirations    []string          `json:"reset_credit_expirations,omitempty"`
	ResetCreditsKnown         bool              `json:"reset_credits_known"`
}

// AccountStats is retained for API/test source compatibility.
// Deprecated: use ProviderConnectionStats.
type AccountStats = ProviderConnectionStats

type AggregateStats struct {
	TotalInputTokens         int64                          `json:"total_input_tokens"`
	TotalCachedTokens        int64                          `json:"total_cached_tokens"`
	TotalOutputTokens        int64                          `json:"total_output_tokens"`
	TotalReasoningTokens     int64                          `json:"total_reasoning_tokens"`
	TotalBillableTokens      int64                          `json:"total_billable_tokens"`
	AvgPrimaryUsed           float64                        `json:"avg_primary_window_used_pct"`
	AvgSecondaryUsed         float64                        `json:"avg_secondary_window_used_pct"`
	OverallCacheHitRate      float64                        `json:"overall_cache_hit_rate_pct"`
	TotalAPICost             float64                        `json:"total_api_cost"`
	TotalSubscriptionCost    float64                        `json:"total_subscription_cost"`
	TotalSubscriptionMonthly float64                        `json:"total_subscription_monthly"`
	OverallROI               float64                        `json:"overall_roi"`
	CostByProvider           map[string]ProviderCostSummary `json:"cost_by_provider,omitempty"`
}

// CapacityAnalysis contains token capacity estimation data for the stats API.
type CapacityAnalysis struct {
	TotalSamples int64                       `json:"total_samples"`
	Plans        map[string]PlanCapacityInfo `json:"plans"`
	ModelFormula string                      `json:"model_formula"`
}

type PlanCapacityInfo struct {
	SampleCount                int64   `json:"sample_count"`
	Confidence                 string  `json:"confidence"`
	TotalInputTokens           int64   `json:"total_input_tokens"`
	TotalOutputTokens          int64   `json:"total_output_tokens"`
	TotalCachedTokens          int64   `json:"total_cached_tokens"`
	TotalReasoningTokens       int64   `json:"total_reasoning_tokens"`
	OutputMultiplier           float64 `json:"output_multiplier"`
	EstimatedPrimaryCapacity   int64   `json:"estimated_5h_capacity"`
	EstimatedSecondaryCapacity int64   `json:"estimated_7d_capacity"`
}

// quotaPaceRatio compares current quota consumption with an even burn across
// the elapsed portion of a reset window. Values over one will exhaust the
// window early if the current pace holds.
func quotaPaceRatio(usedPercent float64, resetMinutes, windowMinutes int) float64 {
	if usedPercent <= 0 || resetMinutes <= 0 || windowMinutes <= 0 || resetMinutes >= windowMinutes {
		return 0
	}
	elapsed := windowMinutes - resetMinutes
	// Upstream quota usage is quantized to percentage points. Before one
	// percentage point of an even-burn budget has elapsed, extrapolating the
	// first non-zero sample produces extreme and misleading pace forecasts.
	if float64(elapsed) < float64(windowMinutes)/100 {
		return 0
	}
	return usedPercent / (100 * float64(elapsed) / float64(windowMinutes))
}

func (h *proxyHandler) handlePoolStats(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	snapshots := h.connectionViewService().PoolStatsConnections(now)
	stats := PoolStats{
		TotalAccounts: len(snapshots),
		Accounts:      make([]ProviderConnectionStats, 0, len(snapshots)),
		GeneratedAt:   now,
	}

	if h.poolUsers != nil {
		stats.TotalPoolUsers = len(h.poolUsers.List())
	}

	var totalInput, totalCached, totalOutput, totalReasoning, totalBillable int64
	var primarySum, secondarySum float64
	var primaryCount, secondaryCount int
	for _, snapshot := range snapshots {
		view := snapshot.View
		stats.Accounts = append(stats.Accounts, view)
		if view.Status == "healthy" || view.Status == "degraded" {
			stats.ActiveAccounts++
		}
		totalInput += view.TotalInputTokens
		totalCached += view.TotalCachedTokens
		totalOutput += view.TotalOutputTokens
		totalReasoning += view.TotalReasoningTokens
		totalBillable += view.TotalBillableTokens
		if view.PrimaryWindowAvailable {
			primarySum += snapshot.PrimaryUsage
			primaryCount++
		}
		if view.SecondaryWindowAvailable {
			secondarySum += snapshot.SecondaryUsage
			secondaryCount++
		}
	}

	overallCacheRate := float64(0)
	if totalInput > 0 {
		overallCacheRate = float64(totalCached) / float64(totalInput) * 100
	}

	avgPrimary := float64(0)
	avgSecondary := float64(0)
	if primaryCount > 0 {
		avgPrimary = (primarySum / float64(primaryCount)) * 100
	}
	if secondaryCount > 0 {
		avgSecondary = (secondarySum / float64(secondaryCount)) * 100
	}

	stats.AggregateUsage = AggregateStats{
		TotalInputTokens:     totalInput,
		TotalCachedTokens:    totalCached,
		TotalOutputTokens:    totalOutput,
		TotalReasoningTokens: totalReasoning,
		TotalBillableTokens:  totalBillable,
		AvgPrimaryUsed:       avgPrimary,
		AvgSecondaryUsed:     avgSecondary,
		OverallCacheHitRate:  overallCacheRate,
	}

	// Load capacity analysis from store
	if h.store != nil {
		caps, err := h.store.loadAllPlanCapacity()
		if err == nil && len(caps) > 0 {
			analysis := &CapacityAnalysis{
				Plans:        make(map[string]PlanCapacityInfo),
				ModelFormula: "effective = input + (cached × 0.1) + (output × mult) + (reasoning × mult)",
			}
			for planType, cap := range caps {
				analysis.TotalSamples += cap.SampleCount
				confidence := "low"
				if cap.SampleCount >= 20 {
					confidence = "high"
				} else if cap.SampleCount >= 5 {
					confidence = "medium"
				}
				mult := cap.OutputMultiplier
				if mult == 0 {
					mult = 4.0
				}
				var estPrimary, estSecondary int64
				if cap.EffectivePerPrimaryPct > 0 {
					estPrimary = int64(cap.EffectivePerPrimaryPct)
				}
				if cap.EffectivePerSecondaryPct > 0 {
					estSecondary = int64(cap.EffectivePerSecondaryPct)
				}
				analysis.Plans[planType] = PlanCapacityInfo{
					SampleCount:                cap.SampleCount,
					Confidence:                 confidence,
					TotalInputTokens:           cap.TotalInputTokens,
					TotalOutputTokens:          cap.TotalOutputTokens,
					TotalCachedTokens:          cap.TotalCachedTokens,
					TotalReasoningTokens:       cap.TotalReasoningTokens,
					OutputMultiplier:           mult,
					EstimatedPrimaryCapacity:   estPrimary,
					EstimatedSecondaryCapacity: estSecondary,
				}
			}
			stats.CapacityAnalysis = analysis
		}
	}

	// Include last 24h tokens aggregate from hourly buckets
	if h.store != nil {
		if hourly, err := h.store.getGlobalHourlyUsage(24); err == nil {
			for _, hu := range hourly {
				throughput := hu.InputTokens + hu.OutputTokens
				stats.Last24hTokens += throughput
			}
		}
	}

	// Populate cost data from analytics store
	if h.analyticsStore != nil {
		// Per-account costs and their measurement periods.
		allTimeCostStats, _ := h.analyticsStore.getAllTimeAccountCostStats()
		last30dCosts, _ := h.analyticsStore.getCostByAccount(30)

		// Build account ID -> real ID mapping for cost lookup
		// (stats use hashed IDs, costs use real IDs)
		accountIDMap := make(map[string]string) // hashed -> real
		accountAddedAt := make(map[string]time.Time)
		for _, snapshot := range snapshots {
			accountIDMap[snapshot.View.ID] = snapshot.ConnectionID
			accountAddedAt[snapshot.ConnectionID] = snapshot.AddedAt
		}

		// Update per-account cost fields
		for i := range stats.Accounts {
			as := &stats.Accounts[i]
			realID := accountIDMap[as.ID]

			subCost, subLabel := getSubscriptionCost(AccountType(as.Type), accountPlanForSubscription(as.PlanType))
			costStats := allTimeCostStats[realID]
			as.SubscriptionCostMonthly = subCost
			as.SubscriptionLabel = subLabel
			as.APICostEstimate = costStats.CostUSD
			as.APICostLast30d = last30dCosts[realID]
			spendStart := accountAddedAt[realID]
			if spendStart.IsZero() {
				// Legacy in-memory fixtures can lack an admission timestamp. Keep
				// historical cost data as their compatibility fallback only.
				spendStart = costStats.FirstSeen
			}
			if spendStart.IsZero() {
				// A paid account with no recorded API use still consumed its current
				// subscription billing cycle.
				spendStart = stats.GeneratedAt
			}
			as.CostTrackingStartedAt = spendStart.UTC().Format(time.RFC3339)
			as.SubscriptionSpend, as.SubscriptionBillingCycles = estimateSubscriptionSpend(subCost, spendStart, stats.GeneratedAt)
			if as.SubscriptionSpend > 0 {
				as.ROI = as.APICostEstimate / as.SubscriptionSpend
			}
		}

		// Provider-level aggregation
		providerCosts := make(map[string]ProviderCostSummary)
		for i := range stats.Accounts {
			as := &stats.Accounts[i]
			pcs := providerCosts[as.Type]
			pcs.APICost += as.APICostEstimate
			pcs.SubscriptionCost += as.SubscriptionSpend
			pcs.MonthlySubscriptionCost += as.SubscriptionCostMonthly
			pcs.AccountCount++
			providerCosts[as.Type] = pcs
		}
		for k, pcs := range providerCosts {
			if pcs.SubscriptionCost > 0 {
				pcs.ROI = pcs.APICost / pcs.SubscriptionCost
			}
			providerCosts[k] = pcs
		}
		stats.AggregateUsage.CostByProvider = providerCosts

		// Pool-level ROI covers the same current-account set on both sides.
		// Historical API value from removed accounts cannot be compared with the
		// subscription spend of only the accounts still in the pool.
		var totalAPICost, totalSubCost, totalSubMonthly float64
		for _, pcs := range providerCosts {
			totalAPICost += pcs.APICost
			totalSubCost += pcs.SubscriptionCost
			totalSubMonthly += pcs.MonthlySubscriptionCost
		}
		stats.AggregateUsage.TotalAPICost = totalAPICost
		stats.AggregateUsage.TotalSubscriptionCost = totalSubCost
		stats.AggregateUsage.TotalSubscriptionMonthly = totalSubMonthly
		if totalSubCost > 0 {
			stats.AggregateUsage.OverallROI = totalAPICost / totalSubCost
		}

		// Daily cost trend for chart
		if dailyCosts, err := h.analyticsStore.getDailyCosts(30); err == nil {
			stats.DailyCosts = dailyCosts
		}
	}

	stats.CyberPolicy = h.computeCyberPolicyStatsFromSnapshots(snapshots)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// computeCyberPolicyStats summarises the metrics counters for the
// pool-stats UI/API. Healthy means: either no suppressions fired yet,
// or every suppression event was paired with a successful swap or a
// buffered/4xx retry — i.e. no synthetic-refusal fallbacks AND there's
// still a cyber candidate available for the next hit.
func (h *proxyHandler) computeCyberPolicyStats(accounts []*ProviderConnection) CyberPolicyStats {
	pool := newProviderPool(accounts)
	snapshots := NewConnectionViewService(pool).PoolStatsConnections(time.Now())
	return h.computeCyberPolicyStatsFromSnapshots(snapshots)
}

func (h *proxyHandler) computeCyberPolicyStatsFromSnapshots(snapshots []poolStatsConnectionSnapshot) CyberPolicyStats {
	out := CyberPolicyStats{
		Counters:   map[string]int64{},
		PerAccount: map[string]map[string]int64{},
	}

	for _, snapshot := range snapshots {
		if snapshot.CyberEligible {
			out.CyberCandidatesAvailable++
		}
	}

	if h.metrics == nil {
		out.Healthy = out.CyberCandidatesAvailable > 0
		return out
	}

	snap := h.metrics.cyberPolicySnapshot()
	for k, v := range snap {
		out.Counters[k.action] += v
		if k.account != "" {
			perAcc, ok := out.PerAccount[k.account]
			if !ok {
				perAcc = map[string]int64{}
				out.PerAccount[k.account] = perAcc
			}
			perAcc[k.action] += v
		}
	}

	suppressions := out.Counters["suppressed_ws"] + out.Counters["suppressed_sse"] + out.Counters["suppressed_buffered"]
	resolutions := out.Counters["swap_succeeded"] + out.Counters["retry_buffered"] + out.Counters["retry_4xx"]
	noCandidate := out.Counters["swap_no_candidate"]

	switch {
	case suppressions == 0:
		out.Healthy = out.CyberCandidatesAvailable > 0
	case noCandidate > 0 || resolutions < suppressions:
		out.Healthy = false
	default:
		out.Healthy = out.CyberCandidatesAvailable > 0
	}
	return out
}

// accountPlanForSubscription normalizes plan type strings for subscription cost lookup.
func accountPlanForSubscription(planType string) string {
	// The plan type from the pool may include tier info like "pro 5x", "pro 20x"
	// Normalize for subscription lookup
	pt := strings.ToLower(strings.TrimSpace(planType))
	switch {
	case strings.Contains(pt, "20x"):
		return "max_20x"
	case strings.Contains(pt, "5x"):
		return "max_5x"
	case strings.Contains(pt, "team"):
		return "team"
	case strings.Contains(pt, "prolite"):
		return "prolite"
	case strings.Contains(pt, "pro"):
		return "pro"
	case strings.Contains(pt, "plus"):
		return "plus"
	default:
		return pt
	}
}

// PoolUserStats represents a user's usage for the leaderboard.
type PoolUserStats struct {
	UserID              string    `json:"user_id"`
	TotalBillableTokens int64     `json:"total_billable_tokens"`
	TotalInputTokens    int64     `json:"total_input_tokens"`
	TotalOutputTokens   int64     `json:"total_output_tokens"`
	RequestCount        int64     `json:"request_count"`
	FirstSeen           time.Time `json:"first_seen"`
	LastSeen            time.Time `json:"last_seen"`
}

// handlePoolUsers returns the public leaderboard of all users' usage.
func (h *proxyHandler) handlePoolUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.getAllUserUsage()
	if err != nil {
		http.Error(w, "failed to fetch user usage", http.StatusInternalServerError)
		return
	}

	// Convert to API format
	stats := make([]PoolUserStats, len(users))
	for i, u := range users {
		stats[i] = PoolUserStats{
			UserID:              u.UserID,
			TotalBillableTokens: u.TotalBillableTokens,
			TotalInputTokens:    u.TotalInputTokens,
			TotalOutputTokens:   u.TotalOutputTokens,
			RequestCount:        u.RequestCount,
			FirstSeen:           u.FirstSeen,
			LastSeen:            u.LastSeen,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"users":       stats,
		"total_users": len(stats),
	})
}
