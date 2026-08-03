[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$Image,
    [switch]$Build,
    [string]$ComposeFile = "docker-compose.split.yml",
    [string]$Project = "codex-pool"
)
$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $root
try {
    if ($Build) {
        docker build -f Dockerfile.web -t $Image .
        if ($LASTEXITCODE) { throw "web image build failed" }
    }
    $apiBefore = docker inspect "$Project-api-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    $ingressBefore = docker inspect "$Project-ingress-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    if (-not $apiBefore -or -not $ingressBefore) { throw "API and ingress services must be running before an independent web deployment" }
    $env:PRODUCTION_WEB_IMAGE = $Image
    # --no-deps is the safety property: never recreate ingress or API for UI work.
    docker compose -p $Project -f $ComposeFile up -d --no-deps --no-build web
    if ($LASTEXITCODE) { throw "web service deployment failed" }
    $deadline = (Get-Date).AddSeconds(90)
    do {
        $health = docker inspect "$Project-web-1" --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' 2>$null
        if ($health -eq "healthy") { break }
        Start-Sleep 1
    } while ((Get-Date) -lt $deadline)
    if ($health -ne "healthy") { throw "web service did not become healthy" }
    $apiAfter = docker inspect "$Project-api-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    $ingressAfter = docker inspect "$Project-ingress-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    if ($apiAfter -ne $apiBefore -or $ingressAfter -ne $ingressBefore) { throw "safety violation: web deployment changed API or ingress" }
    Write-Host "Frontend deployed independently. API and ingress unchanged."
} finally {
    Remove-Item Env:PRODUCTION_WEB_IMAGE -ErrorAction SilentlyContinue
    Pop-Location
}
