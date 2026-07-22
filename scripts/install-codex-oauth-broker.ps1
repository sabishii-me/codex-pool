param(
    [switch]$Uninstall,
    [switch]$Status
)

$ErrorActionPreference = "Stop"
$TaskName = "CodexPoolOAuthBroker"
$InstallDir = Join-Path $env:LOCALAPPDATA "CodexPool"
$Executable = Join-Path $InstallDir "codex-pool-oauth-broker.exe"
$HealthURL = "http://127.0.0.1:1460/v1/status"

function Get-BrokerProcess {
    Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
        Where-Object { $_.ExecutablePath -eq $Executable -and $_.CommandLine -match "oauth-broker" }
}

function Stop-Broker {
    Get-BrokerProcess | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

if ($Status) {
    try {
        $health = Invoke-RestMethod -Uri $HealthURL -TimeoutSec 2
        Write-Host "Codex OAuth broker: $($health.status)"
        Write-Host "Callback ports: $($health.callback_ports -join ', ')"
        exit 0
    } catch {
        Write-Host "Codex OAuth broker is not running"
        exit 1
    }
}

if ($Uninstall) {
    Stop-Broker
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
    if (Test-Path $InstallDir) { Remove-Item $InstallDir -Recurse -Force }
    Write-Host "Codex OAuth broker uninstalled"
    exit 0
}

New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
Write-Host "Building Codex OAuth broker..."
& go build -o $Executable .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Stop-Broker
$arguments = 'oauth-broker --listen 127.0.0.1:1460 --callback-ports 1455,1457'
$action = New-ScheduledTaskAction -Execute $Executable -Argument $arguments
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Days 3650) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Description "Automatic loopback broker for Codex OAuth callbacks" -User $env:USERNAME -Force | Out-Null
Start-Process -FilePath $Executable -ArgumentList $arguments -WindowStyle Hidden

$deadline = (Get-Date).AddSeconds(10)
do {
    Start-Sleep -Milliseconds 250
    try {
        $health = Invoke-RestMethod -Uri $HealthURL -TimeoutSec 1
        if ($health.status -eq "ready") {
            Write-Host "Codex OAuth broker installed and ready"
            Write-Host "Callback ports are acquired only during an active login"
            exit 0
        }
    } catch { }
} while ((Get-Date) -lt $deadline)
throw "broker did not become healthy at $HealthURL"
