[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$ConfigPath,
    [switch]$Execute,
    [string]$Confirmation = ""
)
Set-StrictMode -Version Latest
$ErrorActionPreference="Stop"
$root=(Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$runner=Join-Path $PSScriptRoot "deploy-production.ps1"
$config=[IO.Path]::GetFullPath($ConfigPath)
if(-not(Test-Path -LiteralPath $config)){throw "Config missing: $config"}
$stamp=(Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$launchDir=Join-Path $root "data\backups\deployment-launches"
New-Item -ItemType Directory -Force -Path $launchDir|Out-Null
$stdout=Join-Path $launchDir "$stamp.output.log";$stderr=Join-Path $launchDir "$stamp.launch-error.log"
$startedMarker=Join-Path $launchDir "$stamp.started.json";$finishedMarker=Join-Path $launchDir "$stamp.finished.json"
$taskName="CodexPoolProductionDeploy-$stamp"
$wrapper=Join-Path $launchDir "$stamp.runner.ps1"
$frozenRunner=Join-Path $launchDir "$stamp.deploy-production.ps1"
$frozenConfig=Join-Path $launchDir "$stamp.config.json"
Copy-Item -LiteralPath $runner -Destination $frozenRunner -Force
Copy-Item -LiteralPath $config -Destination $frozenConfig -Force

# Remove only completed scheduler registrations from earlier launches. Their
# wrapper, logs, markers, and frozen inputs remain in deployment-launches.
Get-ScheduledTask -TaskName "CodexPoolProductionDeploy-*" -ErrorAction SilentlyContinue |
    Where-Object { $_.State -eq "Ready" } |
    Unregister-ScheduledTask -Confirm:$false -ErrorAction SilentlyContinue

function Quote-PS([string]$v){"'"+$v.Replace("'","''")+"'"}
$invoke=@("&",(Quote-PS $frozenRunner),"-SourceRoot",(Quote-PS $root),"-ConfigPath",(Quote-PS $frozenConfig))
if($Execute){$invoke+="-Execute";$invoke+=@("-Confirmation",(Quote-PS $Confirmation))}
$wrapperContent=@"
`$ErrorActionPreference = 'Continue'
`$PSDefaultParameterValues['Out-File:Encoding'] = 'utf8'
@{ started_utc = (Get-Date).ToUniversalTime().ToString('o'); pid = `$PID; runner_sha256 = (Get-FileHash $(Quote-PS $frozenRunner) -Algorithm SHA256).Hash.ToLowerInvariant(); config_sha256 = (Get-FileHash $(Quote-PS $frozenConfig) -Algorithm SHA256).Hash.ToLowerInvariant() } | ConvertTo-Json | Set-Content $(Quote-PS $startedMarker) -Encoding utf8
$($invoke -join ' ') *>&1 | Out-File $(Quote-PS $stdout) -Encoding utf8
`$code = if (`$LASTEXITCODE -is [int]) { `$LASTEXITCODE } else { if (`$?) { 0 } else { 1 } }
@{ finished_utc = (Get-Date).ToUniversalTime().ToString('o'); exit_code = `$code } | ConvertTo-Json | Set-Content $(Quote-PS $finishedMarker) -Encoding utf8
exit `$code
"@
[IO.File]::WriteAllText($wrapper,$wrapperContent,(New-Object Text.UTF8Encoding($false)))

# Task Scheduler is the independence boundary: the cutover continues if this
# terminal, API connection, or LLM session disappears. There is deliberately no
# Start-Process fallback.
function Quote-CmdLine([string]$v){'"'+$v.Replace('"','\"')+'"'}
$action=New-ScheduledTaskAction -Execute "powershell.exe" -Argument ("-NoProfile -ExecutionPolicy Bypass -File " + (Quote-CmdLine $wrapper)) -WorkingDirectory $root
$principal=New-ScheduledTaskPrincipal -UserId ([Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
$settings=New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName $taskName -Action $action -Principal $principal -Settings $settings -Description "Autonomous codex-pool Production deployment $stamp" -Force|Out-Null
try{Start-ScheduledTask -TaskName $taskName}catch{Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue;throw}
$deadline=(Get-Date).AddSeconds(10)
do{if(Test-Path -LiteralPath $startedMarker){break};Start-Sleep -Milliseconds 200}while((Get-Date)-lt $deadline)
if(-not(Test-Path -LiteralPath $startedMarker)){
    $info=Get-ScheduledTaskInfo -TaskName $taskName -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
    throw "Scheduled deployment did not create its start marker; task result=$($info.LastTaskResult)"
}
$task=Get-ScheduledTask -TaskName $taskName
[ordered]@{task=$taskName;state=[string]$task.State;started_utc=(Get-Date).ToUniversalTime().ToString("o");execute=[bool]$Execute;source_config=$config;frozen_config=$frozenConfig;frozen_runner=$frozenRunner;output=$stdout;launch_error=$stderr;started_marker=$startedMarker;finished_marker=$finishedMarker;status_command="powershell -File scripts/deploy-production-status.ps1"}|ConvertTo-Json
