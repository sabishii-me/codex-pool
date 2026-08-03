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
        docker build -f Dockerfile.api -t $Image .
        if ($LASTEXITCODE) { throw "API image build failed" }
    }
    $webBefore = docker inspect "$Project-web-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    $ingressBefore = docker inspect "$Project-ingress-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    if (-not $webBefore -or -not $ingressBefore) { throw "web and ingress services must be running before an independent API deployment" }
    $env:PRODUCTION_API_IMAGE = $Image
    # Never recreate ingress or web during a backend release.
    docker compose -p $Project -f $ComposeFile up -d --no-deps --no-build api
    if ($LASTEXITCODE) { throw "API service deployment failed" }
    $deadline = (Get-Date).AddSeconds(120)
    do {
        $health = docker inspect "$Project-api-1" --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' 2>$null
        if ($health -eq "healthy") { break }
        Start-Sleep 1
    } while ((Get-Date) -lt $deadline)
    if ($health -ne "healthy") { throw "API service did not become healthy" }
    $webAfter = docker inspect "$Project-web-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    $ingressAfter = docker inspect "$Project-ingress-1" --format '{{.Id}} {{.RestartCount}}' 2>$null
    if ($webAfter -ne $webBefore -or $ingressAfter -ne $ingressBefore) { throw "safety violation: API deployment changed web or ingress" }
    Write-Host "API deployed independently; ingress and frontend were not recreated."
} finally {
    Remove-Item Env:PRODUCTION_API_IMAGE -ErrorAction SilentlyContinue
    Pop-Location
}
