[CmdletBinding()]
param()
Set-StrictMode -Version Latest
$ErrorActionPreference='Stop'
$path=Join-Path $PSScriptRoot 'deploy-production.ps1'
$text=[IO.File]::ReadAllText($path)
$errors=$null;$tokens=$null
[Management.Automation.Language.Parser]::ParseFile($path,[ref]$tokens,[ref]$errors)|Out-Null
if($errors.Count){throw "PowerShell parser errors: $($errors -join '; ')"}
function Require([bool]$Condition,[string]$Message){if(-not $Condition){throw $Message}}
$mainStart=$text.LastIndexOf('$RollbackTag="codex-pool:production-rollback-')
$mainEnd=$text.IndexOf('$Completed=$true; Write-Phase "complete"',$mainStart)
Require ($mainStart -ge 0 -and $mainEnd -gt $mainStart) 'Unable to identify executable cutover block'
$main=$text.Substring($mainStart,$mainEnd-$mainStart)
$freeze=$main.IndexOf("`n    Stop-Adjacent")
$final=$main.IndexOf('$final=Prepare-Merge "final"',$freeze)
$restart=$main.IndexOf("`n    Start-TestAndStaging",$final)
$prodStop=$main.IndexOf("`n    Stop-Production",$restart)
$apply=$main.IndexOf("`n    Apply-FinalMerge",$prodStop)
$remote=$main.IndexOf("`n        Install-RemoteState",$apply)
Require ($freeze -ge 0 -and $final -gt $freeze -and $restart -gt $final -and $prodStop -gt $restart -and $apply -gt $prodStop -and $remote -gt $apply) 'Unsafe order: freeze -> snapshot -> adjacent restart -> Production stop -> apply -> relocate is required'
Require (($main.Split([string[]]@('Start-TestAndStaging'),[StringSplitOptions]::None).Count-1) -eq 1) 'Adjacent services must restart exactly once in normal cutover'
Require ($text.Contains('if(Test-LocalProductionHealthy)')) 'Rollback must guard against restarting an already healthy source Production'
Require ($text.Contains('rollback-source-already-healthy')) 'Healthy-source rollback evidence phase missing'
Require ($text.Contains('Remote Production failed before health timeout')) 'Remote validation must fail early on exited/unhealthy container'
Require ($text.Contains("'exited|dead|unhealthy|restarting'")) 'Remote restart-loop detection missing'
Require ($text.Contains('Capture-RemoteFailureEvidence')) 'Remote failure evidence capture missing'
Require ($text.Contains('adjacent-freeze-window.json')) 'Adjacent freeze duration evidence is required'
Require ($text.Contains('sudo -n chown -R "$uid:$gid" data pool')) 'Remote ownership normalization must be privileged and fail closed'
Require (-not $text.Contains('chown -R "$uid:$gid" data pool 2>/dev/null || true')) 'Suppressed remote ownership failure remains'
Require (-not $text.Contains('do { try { Assert-HTTP "$TargetEndpoint/healthz"')) 'Old swallowed remote-health loop remains'
'production deployment safety regression tests=passed'
