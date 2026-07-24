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
    if ((git status --porcelain).Length -ne 0) { throw "Working tree must be clean before building a staging candidate." }
    if (-not $SkipTests) {
        Push-Location web
        try { npm test; if ($LASTEXITCODE) { throw "frontend tests failed" }; npx tsc --noEmit --pretty false; if ($LASTEXITCODE) { throw "TypeScript failed" }; npm run build; if ($LASTEXITCODE) { throw "frontend build failed" } }
        finally { Pop-Location }
        git checkout -- web/src/generated/provider-connections-v2.ts
        go test ./...; if ($LASTEXITCODE) { throw "Go tests failed" }
    }
    $date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    docker build --build-arg BUILD_VERSION="staging-$($commit.Substring(0,7))" --build-arg BUILD_COMMIT=$commit --build-arg BUILD_DATE=$date -t $Tag .
    if ($LASTEXITCODE) { throw "Docker build failed" }
    $inspectJSON = docker image inspect $Tag | ConvertFrom-Json
    $inspect = $inspectJSON[0].Config.Labels.'org.opencontainers.image.revision'
    if ($inspect -ne $commit) { throw "Image revision label mismatch: $inspect" }
    Write-Host "Built immutable staging candidate: $Tag"
    Write-Host "Run beside pinned staging:"
    Write-Host "  `$env:STAGING_CANDIDATE_IMAGE='$Tag'; docker compose -p codex-pool-staging-candidate -f docker-compose.staging-candidate.yml up -d"
} finally { Pop-Location }
