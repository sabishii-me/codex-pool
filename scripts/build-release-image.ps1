param(
    [string]$Tag,
    [switch]$SkipTests
)
$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $root
try {
    $commit = (git rev-parse --short=12 HEAD).Trim()
    if (-not $Tag) { $Tag = "codex-pool:staging-$($commit.Substring(0,7))" }
    if ((git status --porcelain).Length -ne 0) { throw "Working tree must be clean before building a release image." }
    if ($Tag -in @("codex-pool:dev", "codex-pool:latest")) { throw "Release images must use an immutable tag." }
    if (-not $SkipTests) {
        Push-Location web
        try {
            npm test
            if ($LASTEXITCODE) { throw "frontend tests failed" }
            npx tsc --noEmit --pretty false
            if ($LASTEXITCODE) { throw "TypeScript failed" }
            npm run build
            if ($LASTEXITCODE) { throw "frontend build failed" }
        } finally { Pop-Location }
        git checkout -- web/src/generated/provider-connections-v2.ts
        go test ./...
        if ($LASTEXITCODE) { throw "Go tests failed" }
    }
    $date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    docker build --build-arg BUILD_VERSION="staging-$($commit.Substring(0,7))" --build-arg BUILD_COMMIT=$commit --build-arg BUILD_DATE=$date -t $Tag .
    if ($LASTEXITCODE) { throw "Docker build failed" }
    $inspectJSON = docker image inspect $Tag | ConvertFrom-Json
    $labels = $inspectJSON[0].Config.Labels
    if ($labels.'org.opencontainers.image.revision' -ne $commit) {
        throw "Image revision label mismatch: $($labels.'org.opencontainers.image.revision')"
    }
    Write-Host "Built immutable release image: $Tag"
    Write-Host "Validate in Test, then promote the exact tag to Staging:"
    Write-Host "  `$env:STAGING_IMAGE='$Tag'; docker compose --env-file .env.staging.local -p codex-pool-staging -f docker-compose.staging.yml up -d"
    Write-Host "This script creates an image artifact only; it does not create another runtime or port."
} finally { Pop-Location }
