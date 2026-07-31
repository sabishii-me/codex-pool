[CmdletBinding()]
param(
    [string]$EvidenceRoot = "",
    [switch]$AsJson
)
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$root=(Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if(-not $EvidenceRoot){$EvidenceRoot=Join-Path $root "data\backups"}
$latest=Get-ChildItem -LiteralPath $EvidenceRoot -Directory -Filter "production-deploy-*" -ErrorAction SilentlyContinue|Sort-Object Name -Descending|Select-Object -First 1
if(-not $latest){throw "No production-deploy evidence directory found"}
$statePath=Join-Path $latest.FullName "state.json"
$state=if(Test-Path $statePath){Get-Content $statePath -Raw|ConvertFrom-Json}else{[pscustomobject]@{phase="unknown";pid=$null;updated_utc=$null;detail="state.json missing"}}
$running=$false
if($state.pid){$running=[bool](Get-Process -Id ([int]$state.pid) -ErrorAction SilentlyContinue)}
$marker=if(Test-Path (Join-Path $latest.FullName "COMPLETED.json")){"COMPLETED"}elseif(Test-Path (Join-Path $latest.FullName "PREPARED.json")){"PREPARED"}elseif(Test-Path (Join-Path $latest.FullName "FAILED.json")){"FAILED"}else{"IN_PROGRESS"}
$result=[ordered]@{evidence_dir=$latest.FullName;marker=$marker;runner_running=$running;pid=$state.pid;phase=$state.phase;detail=$state.detail;updated_utc=$state.updated_utc;transcript=(Join-Path $latest.FullName "transcript.txt")}
$task=Get-ScheduledTask -TaskName "CodexPoolProductionDeploy-*" -ErrorAction SilentlyContinue|Sort-Object TaskName -Descending|Select-Object -First 1
if($task){
    $result.task_name=$task.TaskName;$result.task_state=[string]$task.State
    try{$taskInfo=Get-ScheduledTaskInfo -TaskName $task.TaskName -ErrorAction Stop;$result.task_last_result=$taskInfo.LastTaskResult}catch{$result.task_last_result="unavailable"}
}
if($AsJson){$result|ConvertTo-Json}else{$result.GetEnumerator()|ForEach-Object{"$($_.Key)=$($_.Value)"}}
