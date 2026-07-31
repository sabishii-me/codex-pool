[CmdletBinding()]
param(
    [ValidateSet("InPlace", "Relocate")][string]$Mode = "InPlace",
    [switch]$Execute,
    [switch]$PrepareOnly,
    [string]$Confirmation,

    [string]$AcceptedImage,
    [string]$AcceptedRevision,
    [string]$SourceRoot = "",
    [string]$ConfigPath = "",

    [string]$TargetHost = "",
    [string]$TargetSSHIdentityFile = "",
    [switch]$TargetBootstrap,
    [string]$TargetRoot = "",
    [string]$TargetEndpoint = "http://localhost:8989",
    [string]$TargetPublicURL = "",
    [string]$TargetOAuthRedirectURI = "",
    [string]$TargetBindAddress = "0.0.0.0",
    [string]$TargetComposeProject = "codex-pool",

    [ValidateSet("Fail", "TestWinsUserIdOnly")]
    [string]$ConflictPolicy = "Fail",

    [string]$PiProvider,
    [string]$PiModel,
    [string]$PiExecutable = "pi.cmd",
    [string]$PiConfigPath = "$HOME\.pi\agent\models.json",
    [ValidateSet("ValidateOnly","UpdateOnCutover")][string]$PiEndpointSwitchMode = "ValidateOnly",
    [ValidateRange(10, 7200)][int]$CommandTimeoutSeconds = 1800,
    [ValidateRange(10, 600)][int]$PiTimeoutSeconds = 90,
    [ValidateRange(10, 600)][int]$HealthTimeoutSeconds = 120,

    [string]$TestEndpoint = "http://127.0.0.1:18991",
    [string]$StagingEndpoint = "http://127.0.0.1:18990",
    [switch]$SkipAdjacentImageGate
)

<#
.SYNOPSIS
Autonomous canonical-usage merge and immutable Production deployment.

.DESCRIPTION
This script is deliberately self-contained and non-interactive after -Execute.
It does not depend on an LLM, API session, or caller remaining connected.

InPlace:
  * prepares immutable online SQLite snapshots and simulates the full merge while
    all gateways remain online;
  * stops Test, Staging, and Production only for final snapshots/reconciliation;
  * backs up the stopped Production analytics database and previous image;
  * applies canonical Test then Staging usage through usage-migrate;
  * promotes the exact image, starts Production, and validates the actual web URL,
    signed-out boundary, exact image, Pi inference, and exactly-once accounting;
  * restarts Test and Staging;
  * restores the matching Production database/image and restarts all environments
    automatically on any post-stop failure.

Relocate:
  * additionally stages the immutable image on a POSIX SSH destination while the
    source remains online;
  * after all source writers stop, packages the complete Production-owned data,
    pool, provider-spec, Compose, and .env state and installs it on TargetRoot;
  * starts and validates the destination before declaring success;
  * on failure stops the destination and restarts the unchanged source Production.

Relocate assumes OpenSSH (ssh/scp) and Docker Compose on the destination. It never
prints .env, pool credentials, Pi keys, OAuth state, MFA state, or event payloads.

Success requires a real Pi request. The script never edits Pi configuration. The
selected Pi provider must already point to TargetEndpoint.
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$RequiredConfirmation = "DEPLOY-PRODUCTION-WITH-CANONICAL-USAGE"

# A checked/local JSON config makes a run reproducible and avoids a long,
# error-prone command line. Explicit command-line parameters win over JSON.
if ($ConfigPath) {
    $resolvedConfig = [IO.Path]::GetFullPath($ConfigPath)
    if (-not (Test-Path -LiteralPath $resolvedConfig)) { throw "Config file missing: $resolvedConfig" }
    $cfg = Get-Content -LiteralPath $resolvedConfig -Raw | ConvertFrom-Json
    $allowed = @("Mode","AcceptedImage","AcceptedRevision","SourceRoot","TargetHost","TargetSSHIdentityFile","TargetBootstrap","TargetRoot","TargetEndpoint","TargetPublicURL","TargetOAuthRedirectURI","TargetBindAddress","TargetComposeProject","ConflictPolicy","PiProvider","PiModel","PiExecutable","PiConfigPath","PiEndpointSwitchMode","CommandTimeoutSeconds","PiTimeoutSeconds","HealthTimeoutSeconds","TestEndpoint","StagingEndpoint","SkipAdjacentImageGate","PrepareOnly")
    foreach ($property in $cfg.psobject.Properties) {
        if ($allowed -notcontains $property.Name) { throw "Unknown deployment config property: $($property.Name)" }
        if (-not $PSBoundParameters.ContainsKey($property.Name)) { Set-Variable -Name $property.Name -Value $property.Value }
    }
}

if (-not $SourceRoot) { $SourceRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path }
if (-not $TargetPublicURL) { $TargetPublicURL = $TargetEndpoint }
if ($Mode -notin @("InPlace","Relocate")) { throw "Mode must be InPlace or Relocate" }
if ($ConflictPolicy -notin @("Fail","TestWinsUserIdOnly")) { throw "Unsupported ConflictPolicy" }
if ($CommandTimeoutSeconds -lt 10 -or $CommandTimeoutSeconds -gt 7200) { throw "CommandTimeoutSeconds outside 10..7200" }
if ($PiTimeoutSeconds -lt 10 -or $PiTimeoutSeconds -gt 600) { throw "PiTimeoutSeconds outside 10..600" }
if ($HealthTimeoutSeconds -lt 10 -or $HealthTimeoutSeconds -gt 600) { throw "HealthTimeoutSeconds outside 10..600" }
$targetUri=$null
if(-not[Uri]::TryCreate($TargetEndpoint,[UriKind]::Absolute,[ref]$targetUri) -or $targetUri.Scheme -notin @("http","https")){throw "TargetEndpoint must be an absolute HTTP(S) URL"}
if(-not $TargetOAuthRedirectURI){throw "TargetOAuthRedirectURI is required and must name the destination"}
$redirectUri=$null
if(-not[Uri]::TryCreate($TargetOAuthRedirectURI,[UriKind]::Absolute,[ref]$redirectUri)){throw "TargetOAuthRedirectURI must be absolute"}
if($Mode -eq "Relocate" -and $targetUri.Host -in @("localhost","127.0.0.1","::1")){throw "Relocation TargetEndpoint must be reachable from the source/Pi client, not localhost"}
$Root = [IO.Path]::GetFullPath($SourceRoot)
$TestCompose = Join-Path $Root "docker-compose.dev.yml"
$StagingCompose = Join-Path $Root "docker-compose.staging.yml"
$ProductionCompose = Join-Path $Root "docker-compose.yml"
$TestEnv = Join-Path $Root ".env.dev"
$StagingEnv = Join-Path $Root ".env.staging.local"
$TestContainer = "codex-pool-dev-codex-pool-dev-1"
$StagingContainer = "codex-pool-staging-codex-pool-staging-1"
$ProductionContainer = "$TargetComposeProject-codex-pool-1"
$LocalProductionContainer = "codex-pool-codex-pool-1"
$EvidenceDir = ""
$StatePath = ""
$LogPath = ""
$AcceptedImageID = ""
$PreviousProductionImageID = ""
$RollbackTag = ""
$Stopped = $false
$AdjacentStopped = $false
$AdjacentStoppedAt = $null
$TargetStarted = $false
$PiConfigChanged = $false
$PiConfigBackup = ""
$Completed = $false
$Stamp = ""
$Mutex = $null

function Write-Phase {
    param([string]$Phase, [string]$Detail = "")
    $record = [ordered]@{
        schema = 1
        phase = $Phase
        detail = $Detail
        mode = $Mode
        pid = $PID
        updated_utc = (Get-Date).ToUniversalTime().ToString("o")
        evidence_dir = $EvidenceDir
        completed = $Completed
    }
    if ($StatePath) {
        $temp = "$StatePath.tmp"
        $record | ConvertTo-Json | Set-Content -LiteralPath $temp -Encoding utf8
        Move-Item -LiteralPath $temp -Destination $StatePath -Force
    }
    Write-Host "[$($record.updated_utc)] PHASE=$Phase $Detail"
}

function ConvertTo-NativeArgument {
    param([string]$Value)
    if ($null -eq $Value -or $Value.Length -eq 0) { return '""' }
    if ($Value -notmatch '[\s"]') { return $Value }
    # CommandLineToArgvW-compatible quoting for Windows PowerShell 5.1.
    $builder = New-Object Text.StringBuilder
    [void]$builder.Append('"')
    $slashes = 0
    foreach ($ch in $Value.ToCharArray()) {
        if ($ch -eq [char]92) { $slashes++; continue }
        if ($ch -eq [char]34) {
            [void]$builder.Append(([string][char]92) * ($slashes * 2 + 1))
            [void]$builder.Append([char]34)
            $slashes = 0
            continue
        }
        if ($slashes) { [void]$builder.Append(([string][char]92) * $slashes); $slashes = 0 }
        [void]$builder.Append($ch)
    }
    if ($slashes) { [void]$builder.Append(([string][char]92) * ($slashes * 2)) }
    [void]$builder.Append('"')
    $builder.ToString()
}

function Invoke-Process {
    param(
        [Parameter(Mandatory=$true)][string]$File,
        [string[]]$Arguments = @(),
        [string]$WorkingDirectory = $Root,
        [switch]$AllowFailure,
        [switch]$CaptureOutput,
        [switch]$SensitiveArguments,
        [int]$TimeoutSeconds = 0,
        [hashtable]$Environment = @{}
    )
    if ($TimeoutSeconds -le 0) { $TimeoutSeconds = $CommandTimeoutSeconds }
    if (-not $SensitiveArguments) { Write-Host ("> " + $File + " " + ($Arguments -join " ")) }
    else { Write-Host ("> " + $File + " [arguments redacted]") }
    $id = [guid]::NewGuid().ToString("N")
    $stdout = if ($EvidenceDir) { Join-Path $EvidenceDir "process-$id.stdout.log" } else { Join-Path $env:TEMP "codex-$id.out" }
    $stderr = if ($EvidenceDir) { Join-Path $EvidenceDir "process-$id.stderr.log" } else { Join-Path $env:TEMP "codex-$id.err" }
    $psi = New-Object Diagnostics.ProcessStartInfo
    $psi.FileName = $File
    $psi.Arguments = (($Arguments | ForEach-Object { ConvertTo-NativeArgument ([string]$_) }) -join " ")
    $psi.WorkingDirectory = $WorkingDirectory
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    foreach($entry in $Environment.GetEnumerator()){$psi.EnvironmentVariables[[string]$entry.Key]=[string]$entry.Value}
    $process = New-Object Diagnostics.Process
    $process.StartInfo = $psi
    if (-not $process.Start()) { throw "Unable to start $File" }
    $stdoutTask = $process.StandardOutput.ReadToEndAsync()
    $stderrTask = $process.StandardError.ReadToEndAsync()
    if (-not $process.WaitForExit($TimeoutSeconds * 1000)) {
        try { $process.Kill() } catch {}
        try { $process.WaitForExit() } catch {}
        throw "$File exceeded timeout=$TimeoutSeconds seconds; logs: $stdout , $stderr"
    }
    $out = $stdoutTask.Result
    $err = $stderrTask.Result
    [IO.File]::WriteAllText($stdout, $out)
    [IO.File]::WriteAllText($stderr, $err)
    $exitCode = $process.ExitCode
    $process.Dispose()
    if (-not $CaptureOutput) {
        if ($out) { Write-Host $out.TrimEnd() }
        if ($err) { [Console]::Error.Write($err) }
    }
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "$File exited with code $exitCode; logs: $stdout , $stderr"
    }
    [pscustomobject]@{ ExitCode=$exitCode; StdOut=$out; StdErr=$err; StdOutPath=$stdout; StdErrPath=$stderr }
}

function Invoke-Docker { param([string[]]$Arguments, [switch]$CaptureOutput, [switch]$AllowFailure)
    Invoke-Process -File "docker" -Arguments $Arguments -CaptureOutput:$CaptureOutput -AllowFailure:$AllowFailure
}

function Get-ImageInfo {
    param([string]$Image)
    $result = Invoke-Docker -Arguments @("image", "inspect", $Image) -CaptureOutput
    $items = $result.StdOut | ConvertFrom-Json
    if (@($items).Count -ne 1) { throw "Expected one image for $Image" }
    [pscustomobject]@{ ID=$items[0].Id; Revision=$items[0].Config.Labels.'org.opencontainers.image.revision' }
}

function Get-ContainerInfo {
    param([string]$Name)
    $result = Invoke-Docker -Arguments @("inspect", $Name) -CaptureOutput
    $items = $result.StdOut | ConvertFrom-Json
    if (@($items).Count -ne 1) { throw "Expected one container for $Name" }
    $item = $items[0]
    [pscustomobject]@{
        Name=$Name; ImageID=$item.Image; ImageRef=$item.Config.Image
        Revision=$item.Config.Labels.'org.opencontainers.image.revision'
        Running=[bool]$item.State.Running
        Health=if ($item.State.Health) { $item.State.Health.Status } else { "none" }
        RestartCount=[int]$item.RestartCount
    }
}

function Assert-HTTP {
    param([string]$URL, [int[]]$Expected)
    $result = Invoke-Process -File "curl.exe" -Arguments @("-sS", "--connect-timeout", "3", "--max-time", "10", "-o", "NUL", "-w", "%{http_code}", $URL) -CaptureOutput -AllowFailure
    $text = $result.StdOut.Trim()
    $code = 0
    [void][int]::TryParse($text, [ref]$code)
    if ($result.ExitCode -ne 0 -or $Expected -notcontains $code) {
        throw "Expected HTTP $($Expected -join '/') from $URL; exit=$($result.ExitCode), status=$text"
    }
    Write-Host "HTTP $code $URL"
}

function Assert-WebAndOAuth {
    param([string]$Endpoint)
    $html=Invoke-Process -File "curl.exe" -Arguments @("-sS","--connect-timeout","3","--max-time","10","$Endpoint/") -CaptureOutput
    if($html.StdOut -notmatch 'Private model gateway|AI Pool'){throw "Destination root did not return the Production web shell"}
    $headers=Invoke-Process -File "curl.exe" -Arguments @("-sS","--connect-timeout","3","--max-time","10","-D","-","-o","NUL","--max-redirs","0","$Endpoint/auth/login/google") -CaptureOutput -AllowFailure
    if($headers.StdOut -notmatch '(?im)^HTTP/\S+\s+302\b'){throw "Google OAuth entry did not return HTTP 302"}
    $location=[regex]::Match($headers.StdOut,'(?im)^Location:\s*(.+?)\r?$').Groups[1].Value
    if(-not $location){throw "Google OAuth entry omitted Location"}
    $encoded=[Uri]::EscapeDataString($TargetOAuthRedirectURI)
    if($location -notlike "*$encoded*" -and $location -notlike "*$TargetOAuthRedirectURI*"){throw "OAuth redirect does not target configured destination callback"}
    Write-Host "Web shell and OAuth destination passed"
}

function Wait-LocalHealthy {
    param([string]$Container)
    $deadline = (Get-Date).AddSeconds($HealthTimeoutSeconds)
    do {
        try {
            $info = Get-ContainerInfo $Container
            if ($info.Running -and $info.Health -eq "healthy") { return }
        } catch {}
        Start-Sleep -Seconds 1
    } while ((Get-Date) -lt $deadline)
    throw "$Container did not become healthy within $HealthTimeoutSeconds seconds"
}

function Get-RelativePathCompat {
    param([string]$BasePath, [string]$ChildPath)
    $base = [IO.Path]::GetFullPath($BasePath).TrimEnd([char]92, [char]47) + [IO.Path]::DirectorySeparatorChar
    $child = [IO.Path]::GetFullPath($ChildPath)
    if (-not $child.StartsWith($base, [StringComparison]::OrdinalIgnoreCase)) { throw "Path outside repository: $child" }
    $child.Substring($base.Length)
}

function To-ContainerPath {
    param([string]$Path)
    "/work/" + (Get-RelativePathCompat $Root $Path).Replace([char]92, [char]47)
}

function Write-SQLiteHelper {
    $path = Join-Path $EvidenceDir "sqlite_snapshot.py"
    @'
import hashlib, json, os, sqlite3, sys
src, dst, report = sys.argv[1:4]
canonical_only = len(sys.argv) > 4 and sys.argv[4] == "canonical-only"
if os.path.exists(dst): raise SystemExit("snapshot destination exists")
# Python backup() is a consistent online snapshot and includes committed WAL data.
srcdb = sqlite3.connect("file:" + os.path.abspath(src).replace(chr(92), "/") + "?mode=ro", uri=True, timeout=30)
out = sqlite3.connect(dst, timeout=30)
try:
    srcdb.backup(out)
finally:
    out.close(); srcdb.close()
con = sqlite3.connect(dst, timeout=30)
try:
    quick = con.execute("pragma quick_check").fetchone()[0]
    if quick != "ok": raise RuntimeError("quick_check=" + str(quick))
    total = con.execute("select count(*) from usage_events").fetchone()[0]
    canonical = con.execute("select count(*) from usage_events where trim(coalesce(connection_id,''))<>'' and trim(coalesce(request_id,''))<>''").fetchone()[0]
    duplicates = con.execute("select count(*) from (select connection_id,request_id from usage_events where trim(coalesce(connection_id,''))<>'' and trim(coalesce(request_id,''))<>'' group by connection_id,request_id having count(*)>1)").fetchone()[0]
    if duplicates: raise RuntimeError("duplicate canonical identity groups=" + str(duplicates))
    if canonical_only:
        con.execute("delete from usage_events where trim(coalesce(connection_id,''))='' or trim(coalesce(request_id,''))=''")
        con.commit(); con.execute("vacuum")
finally:
    con.close()
h=hashlib.sha256()
with open(dst,"rb") as f:
    for b in iter(lambda:f.read(1024*1024),b""): h.update(b)
with open(report,"w",encoding="utf-8") as f:
    json.dump({"quick_check":"ok","source_events_total":total,"source_events_canonical":canonical,"source_events_noncanonical":total-canonical,"events_in_snapshot":canonical if canonical_only else total,"canonical_only":canonical_only,"duplicate_canonical_identity_groups":duplicates,"snapshot_sha256":h.hexdigest()},f,indent=2);f.write("\n")
'@ | Set-Content -LiteralPath $path -Encoding utf8
    $path
}

function New-Snapshot {
    param([string]$Source, [string]$Destination, [string]$Report, [string]$Helper, [switch]$CanonicalOnly)
    $args = @($Helper, $Source, $Destination, $Report)
    if ($CanonicalOnly) { $args += "canonical-only" }
    [void](Invoke-Process -File "python" -Arguments $args)
}

function Invoke-Migration {
    param([string]$Target, [string]$Source, [string]$SourceEnvironment, [string]$Report, [switch]$Apply, [string]$BackupDir="", [switch]$AllowConflictExit)
    $mount = "type=bind,source=$Root,target=/work"
    $args = @("run","--rm","--user","0:0","--mount",$mount,"--entrypoint","/app/codex-pool",$AcceptedImage,
        "usage-migrate","--target",(To-ContainerPath $Target),"--source",(To-ContainerPath $Source),
        "--target-environment","production","--source-environment",$SourceEnvironment,"--report",(To-ContainerPath $Report))
    if ($Apply) { $args += @("--backup-dir",(To-ContainerPath $BackupDir),"--apply") }
    $result = Invoke-Docker -Arguments $args -AllowFailure
    if ($result.ExitCode -ne 0 -and -not ($AllowConflictExit -and (Test-Path -LiteralPath $Report))) {
        throw "usage-migrate exited $($result.ExitCode)"
    }
}

function Assert-MigrationReport {
    param([string]$Path, [string]$Mode, [switch]$Repeat)
    $r = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if ($r.mode -ne $Mode) { throw "Wrong migration mode in $Path" }
    if ([int]$r.source_invalid_events -ne 0) { throw "Invalid source events in $Path" }
    if ([int]$r.conflicting_events -ne 0) { throw "Unresolved payload conflicts in $Path" }
    if ([int]$r.insertable_events + [int]$r.identical_events -ne [int]$r.source_events) { throw "Source count mismatch in $Path" }
    if ($Mode -eq "apply" -and -not $Repeat -and [int]$r.inserted_events -ne [int]$r.insertable_events) { throw "Insert count mismatch in $Path" }
    if ($Repeat -and [int]$r.inserted_events -ne 0) { throw "Repeat migration inserted rows in $Path" }
    $r
}

function Resolve-StagingConflicts {
    param([string]$Source, [string]$ConflictReport, [string]$Resolved, [string]$Quarantine)
    if ($ConflictPolicy -ne "TestWinsUserIdOnly") { throw "Migration conflicts found and ConflictPolicy=Fail" }
    $resolver = Join-Path $EvidenceDir "resolve_user_id_conflicts.py"
    @'
import json, shutil, sqlite3, sys
source, report_path, resolved, quarantine = sys.argv[1:5]
r=json.load(open(report_path,encoding="utf-8-sig")); conflicts=r.get("conflicts",[])
if not conflicts: raise SystemExit("resolver called without conflicts")
bad=[x for x in conflicts if x.get("fields") != ["user_id"]]
if bad: raise SystemExit("non-user_id conflicts="+str(len(bad)))
shutil.copy2(source,resolved); db=sqlite3.connect(resolved)
removed=[]
try:
    db.execute("begin immediate")
    for x in conflicts:
        cur=db.execute("delete from usage_events where connection_id=? and request_id=?",(x["connection_id"],x["request_id"]))
        if cur.rowcount != 1: raise RuntimeError("expected one source row")
        removed.append({"connection_id":x["connection_id"],"request_id":x["request_id"],"reason":"explicit_test_precedence_user_id_only"})
    db.commit()
    if db.execute("pragma quick_check").fetchone()[0] != "ok": raise RuntimeError("quick_check failed")
    db.execute("vacuum")
finally: db.close()
with open(quarantine,"w",encoding="utf-8") as f:
    json.dump({"policy":"TestWinsUserIdOnly","quarantined_events":len(removed),"events":removed},f,indent=2);f.write("\n")
'@ | Set-Content -LiteralPath $resolver -Encoding utf8
    [void](Invoke-Process -File "python" -Arguments @($resolver,$Source,$ConflictReport,$Resolved,$Quarantine))
}

function Prepare-Merge {
    param([string]$Name, [switch]$Final)
    $dir = Join-Path $EvidenceDir $Name
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $helper = Write-SQLiteHelper
    $test = Join-Path $dir "test-canonical.db"
    $staging = Join-Path $dir "staging-canonical.db"
    $production = Join-Path $dir "production.db"
    New-Snapshot (Join-Path $Root "dev\data\analytics.db") $test (Join-Path $dir "test-summary.json") $helper -CanonicalOnly
    New-Snapshot (Join-Path $Root "staging\data\analytics.db") $staging (Join-Path $dir "staging-summary.json") $helper -CanonicalOnly
    New-Snapshot (Join-Path $Root "data\analytics.db") $production (Join-Path $dir "production-summary.json") $helper
    $simulation = Join-Path $dir "simulation.db"
    Copy-Item -LiteralPath $production -Destination $simulation
    $backups = Join-Path $dir "simulation-backups"
    Invoke-Migration $simulation $test "test-$Stamp" (Join-Path $dir "test-dry-run.json")
    [void](Assert-MigrationReport (Join-Path $dir "test-dry-run.json") "dry-run")
    Invoke-Migration $simulation $test "test-$Stamp" (Join-Path $dir "test-apply.json") -Apply -BackupDir $backups
    [void](Assert-MigrationReport (Join-Path $dir "test-apply.json") "apply")

    $conflicts = Join-Path $dir "staging-conflict-check.json"
    Invoke-Migration $simulation $staging "staging-$Stamp" $conflicts -AllowConflictExit
    $cr = Get-Content -LiteralPath $conflicts -Raw | ConvertFrom-Json
    $resolved = $staging
    if ([int]$cr.conflicting_events -gt 0) {
        $resolved = Join-Path $dir "staging-canonical-resolved.db"
        Resolve-StagingConflicts $staging $conflicts $resolved (Join-Path $dir "staging-quarantine.json")
    } elseif ([int]$cr.source_invalid_events -ne 0) { throw "Invalid Staging source rows" }

    Invoke-Migration $simulation $resolved "staging-$Stamp" (Join-Path $dir "staging-dry-run.json")
    [void](Assert-MigrationReport (Join-Path $dir "staging-dry-run.json") "dry-run")
    Invoke-Migration $simulation $resolved "staging-$Stamp" (Join-Path $dir "staging-apply.json") -Apply -BackupDir $backups
    [void](Assert-MigrationReport (Join-Path $dir "staging-apply.json") "apply")
    New-Snapshot $simulation (Join-Path $dir "validated.db") (Join-Path $dir "final-integrity.json") $helper
    [pscustomobject]@{ Directory=$dir; Test=$test; Staging=$resolved; Production=$production; Candidate=$simulation }
}

function Stop-Adjacent {
    Write-Phase "freezing-sources" "Stopping Test and Staging while Production remains online"
    $script:AdjacentStopped=$true
    $script:AdjacentStoppedAt=Get-Date
    [void](Invoke-Docker -Arguments @("compose","--env-file",$TestEnv,"--project-name","codex-pool-dev","-f",$TestCompose,"stop","codex-pool-dev"))
    $env:STAGING_IMAGE=$AcceptedImage
    [void](Invoke-Docker -Arguments @("compose","--env-file",$StagingEnv,"--project-name","codex-pool-staging","-f",$StagingCompose,"stop","codex-pool-staging"))
    foreach ($n in @($TestContainer,$StagingContainer)) { if ((Get-ContainerInfo $n).Running) { throw "$n did not stop" } }
}

function Stop-Production {
    Write-Phase "stopping-production" "Final source inputs are ready; entering short Production downtime"
    $script:Stopped=$true
    [void](Invoke-Docker -Arguments @("compose","--project-name","codex-pool","-f",$ProductionCompose,"stop","codex-pool"))
    if ((Get-ContainerInfo $LocalProductionContainer).Running) { throw "$LocalProductionContainer did not stop" }
}

function Start-TestAndStaging {
    if(-not $AdjacentStopped){return}
    Write-Phase "restarting-sources" "Restarting Test and Staging immediately after frozen snapshots"
    $override = Join-Path $EvidenceDir "test-image.override.yml"
    "services:`n  codex-pool-dev:`n    image: $AcceptedImage`n" | Set-Content -LiteralPath $override -Encoding utf8
    $env:STAGING_IMAGE=$AcceptedImage
    [void](Invoke-Docker -Arguments @("compose","--env-file",$StagingEnv,"--project-name","codex-pool-staging","-f",$StagingCompose,"up","--detach","--no-build","--force-recreate"))
    Wait-LocalHealthy $StagingContainer
    [void](Invoke-Docker -Arguments @("compose","--env-file",$TestEnv,"--project-name","codex-pool-dev","-f",$TestCompose,"-f",$override,"up","--detach","--no-build","--force-recreate","codex-pool-dev"))
    Wait-LocalHealthy $TestContainer
    $testInfo=Get-ContainerInfo $TestContainer; $stagingInfo=Get-ContainerInfo $StagingContainer
    if($testInfo.ImageID -ne $AcceptedImageID -or $stagingInfo.ImageID -ne $AcceptedImageID){throw "Adjacent environment restarted with wrong image"}
    Assert-HTTP "$StagingEndpoint/healthz" @(200)
    Assert-HTTP "$TestEndpoint/healthz" @(200)
    $script:AdjacentStopped=$false
    if($AdjacentStoppedAt){
        $seconds=[Math]::Round(((Get-Date)-$AdjacentStoppedAt).TotalSeconds,3)
        [ordered]@{stopped_seconds=$seconds;restarted_utc=(Get-Date).ToUniversalTime().ToString("o");test_image_id=$testInfo.ImageID;staging_image_id=$stagingInfo.ImageID}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "adjacent-freeze-window.json") -Encoding utf8
        $script:AdjacentStoppedAt=$null
    }
}

function Save-LocalRollback {
    $dir = Join-Path $EvidenceDir "rollback"
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $archive = Join-Path $dir "production-state.tar.gz"
    $archiveTemp = "$archive.tmp"
    Package-ProductionState $archiveTemp
    Move-Item -LiteralPath $archiveTemp -Destination $archive -Force
    if(Test-Path -LiteralPath (Join-Path $Root ".env")){Copy-Item (Join-Path $Root ".env") (Join-Path $dir ".env") -Force}
    foreach ($n in @("analytics.db","analytics.db-wal","analytics.db-shm")) {
        $p=Join-Path $Root "data\$n"; if (Test-Path -LiteralPath $p) { Copy-Item -LiteralPath $p -Destination (Join-Path $dir $n) -Force }
    }
    $meta=[ordered]@{ previous_image_id=$PreviousProductionImageID; rollback_tag=$RollbackTag; files=@() }
    Get-ChildItem -LiteralPath $dir -File | ForEach-Object { $meta.files += [ordered]@{name=$_.Name;bytes=$_.Length;sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()} }
    $meta|ConvertTo-Json -Depth 5|Set-Content -LiteralPath (Join-Path $dir "manifest.json") -Encoding utf8
    $dir
}

function Test-LocalProductionHealthy {
    try{
        $info=Get-ContainerInfo $LocalProductionContainer
        if(-not $info.Running -or $info.Health -ne "healthy"){return $false}
        $probe=Invoke-Process -File "curl.exe" -Arguments @("-sS","--connect-timeout","2","--max-time","5","-o","NUL","-w","%{http_code}","http://127.0.0.1:8989/healthz") -CaptureOutput -AllowFailure
        return ($probe.ExitCode -eq 0 -and $probe.StdOut.Trim() -eq "200")
    }catch{return $false}
}

function Restore-LocalRollback {
    param([string]$RollbackDir)
    Write-Phase "rollback" "Restoring matching source Production image and complete owned state"
    if(Test-LocalProductionHealthy){
        Write-Phase "rollback-source-already-healthy" "Source Production is already healthy; refusing unnecessary restart"
        return
    }
    [void](Invoke-Docker -Arguments @("compose","--project-name","codex-pool","-f",$ProductionCompose,"stop","codex-pool") -AllowFailure)
    $failedArchive=Join-Path $EvidenceDir "failed-production-state.tar.gz"
    try { Package-ProductionState $failedArchive } catch { [Console]::Error.WriteLine("Unable to preserve failed state: "+$_.Exception.Message) }
    $archive=Join-Path $RollbackDir "production-state.tar.gz"
    if(-not(Test-Path -LiteralPath $archive)){throw "Complete rollback archive missing"}
    Get-ChildItem -LiteralPath (Join-Path $Root "data") -Force | Where-Object {$_.Name -ne "backups"} | Remove-Item -Recurse -Force
    foreach($name in @("pool","provider-specs")){$p=Join-Path $Root $name;if(Test-Path $p){Remove-Item $p -Recurse -Force}}
    $extract=Join-Path $EvidenceDir "extract_rollback.py"
@'
import os,sys,tarfile
archive,root=sys.argv[1:3]
with tarfile.open(archive,"r:gz") as t:
  base=os.path.abspath(root)
  for m in t.getmembers():
    dest=os.path.abspath(os.path.join(base,m.name))
    if dest!=base and not dest.startswith(base+os.sep): raise RuntimeError("unsafe archive path")
  t.extractall(root)
'@ | Set-Content -LiteralPath $extract -Encoding utf8
    [void](Invoke-Process -File "python" -Arguments @($extract,$archive,$Root))
    if(Test-Path -LiteralPath (Join-Path $RollbackDir ".env")){Copy-Item (Join-Path $RollbackDir ".env") (Join-Path $Root ".env") -Force}
    [void](Invoke-Docker -Arguments @("tag",$PreviousProductionImageID,"codex-pool:latest"))
    [void](Invoke-Docker -Arguments @("compose","--project-name","codex-pool","-f",$ProductionCompose,"up","--detach","--no-build","--force-recreate","codex-pool"))
    Wait-LocalHealthy $LocalProductionContainer
}

function Set-LocalTargetEnvironment {
    if($Mode -ne "InPlace"){return}
    $envPath=Join-Path $Root ".env"
    if(-not(Test-Path -LiteralPath $envPath)){throw "Production .env missing"}
    $editor=Join-Path $EvidenceDir "set_local_target_env.py"
@'
import os,sys
path,public_url,redirect=sys.argv[1:4]
values={"PUBLIC_URL":public_url}
if redirect: values["OAUTH_GOOGLE_REDIRECT_URI"]=redirect
lines=open(path,encoding="utf-8-sig").read().splitlines();out=[];seen=set()
for line in lines:
  stripped=line.strip()
  if not stripped or stripped.startswith("#") or "=" not in line: out.append(line);continue
  key=line.split("=",1)[0].strip()
  if key in values: out.append(key+"="+values[key]);seen.add(key)
  else: out.append(line)
for key,value in values.items():
  if key not in seen: out.append(key+"="+value)
with open(path,"w",encoding="utf-8",newline="\n") as f:f.write("\n".join(out)+"\n")
'@|Set-Content -LiteralPath $editor -Encoding utf8
    [void](Invoke-Process -File "python" -Arguments @($editor,$envPath,$TargetPublicURL,$TargetOAuthRedirectURI) -SensitiveArguments)
}

function Apply-FinalMerge {
    param($Merge)
    $target=Join-Path $Root "data\analytics.db"; $backups=Join-Path $EvidenceDir "apply-backups"
    Invoke-Migration $target $Merge.Test "test-$Stamp" (Join-Path $EvidenceDir "production-test-dry-run.json")
    [void](Assert-MigrationReport (Join-Path $EvidenceDir "production-test-dry-run.json") "dry-run")
    Invoke-Migration $target $Merge.Test "test-$Stamp" (Join-Path $EvidenceDir "production-test-apply.json") -Apply -BackupDir $backups
    [void](Assert-MigrationReport (Join-Path $EvidenceDir "production-test-apply.json") "apply")
    Invoke-Migration $target $Merge.Staging "staging-$Stamp" (Join-Path $EvidenceDir "production-staging-dry-run.json")
    [void](Assert-MigrationReport (Join-Path $EvidenceDir "production-staging-dry-run.json") "dry-run")
    Invoke-Migration $target $Merge.Staging "staging-$Stamp" (Join-Path $EvidenceDir "production-staging-apply.json") -Apply -BackupDir $backups
    [void](Assert-MigrationReport (Join-Path $EvidenceDir "production-staging-apply.json") "apply")
    Invoke-Migration $target $Merge.Test "test-$Stamp" (Join-Path $EvidenceDir "production-test-repeat.json") -Apply -BackupDir $backups
    [void](Assert-MigrationReport (Join-Path $EvidenceDir "production-test-repeat.json") "apply" -Repeat)
    Invoke-Migration $target $Merge.Staging "staging-$Stamp" (Join-Path $EvidenceDir "production-staging-repeat.json") -Apply -BackupDir $backups
    [void](Assert-MigrationReport (Join-Path $EvidenceDir "production-staging-repeat.json") "apply" -Repeat)
    $helper=Write-SQLiteHelper
    New-Snapshot $target (Join-Path $EvidenceDir "production-final.db") (Join-Path $EvidenceDir "production-final-integrity.json") $helper
}

function Get-PiProviderConfig {
    param([string]$ModelsPath=$PiConfigPath)
    if (-not (Test-Path -LiteralPath $ModelsPath)) { throw "Pi config missing: $ModelsPath" }
    $config=Get-Content -LiteralPath $ModelsPath -Raw|ConvertFrom-Json
    $property=$config.providers.psobject.Properties[$PiProvider]
    if (-not $property) { throw "Pi provider not found: $PiProvider" }
    $property.Value
}

function New-TemporaryPiProfile {
    $sourceDir=Split-Path $PiConfigPath -Parent
    $tempDir=Join-Path $EvidenceDir "pi-acceptance-profile"
    New-Item -ItemType Directory -Force -Path $tempDir|Out-Null
    foreach($name in @("models.json","auth.json","settings.json")){$p=Join-Path $sourceDir $name;if(Test-Path -LiteralPath $p){Copy-Item $p (Join-Path $tempDir $name) -Force}}
    $editor=Join-Path $EvidenceDir "set_pi_provider_url.py"
@'
import json,sys
path,provider,url=sys.argv[1:4]
with open(path,encoding="utf-8-sig") as f:x=json.load(f)
if provider not in x.get("providers",{}):raise SystemExit("Pi provider missing")
x["providers"][provider]["baseUrl"]=url
with open(path,"w",encoding="utf-8",newline="\n") as f:json.dump(x,f,indent=2);f.write("\n")
'@|Set-Content $editor -Encoding utf8
    [void](Invoke-Process -File "python" -Arguments @($editor,(Join-Path $tempDir "models.json"),$PiProvider,$TargetEndpoint) -SensitiveArguments)
    $tempDir
}

function Set-RealPiEndpoint {
    if($PiEndpointSwitchMode -ne "UpdateOnCutover"){return}
    $script:PiConfigBackup=Join-Path $EvidenceDir "pi-models.before-cutover.json"
    Copy-Item -LiteralPath $PiConfigPath -Destination $PiConfigBackup -Force
    $editor=Join-Path $EvidenceDir "set_pi_provider_url.py"
    [void](Invoke-Process -File "python" -Arguments @($editor,$PiConfigPath,$PiProvider,$TargetEndpoint) -SensitiveArguments)
    $script:PiConfigChanged=$true
}

function Restore-PiConfig {
    if($PiConfigChanged -and $PiConfigBackup -and (Test-Path -LiteralPath $PiConfigBackup)){
        Copy-Item -LiteralPath $PiConfigBackup -Destination $PiConfigPath -Force
        $script:PiConfigChanged=$false
    }
}

function Test-Pi {
    param([string]$Endpoint,[string]$AgentDir="")
    if (-not $PiProvider -or -not $PiModel) { throw "PiProvider and PiModel are required for executable acceptance" }
    $modelsPath=if($AgentDir){Join-Path $AgentDir "models.json"}else{$PiConfigPath}
    $provider=Get-PiProviderConfig $modelsPath
    if ($provider.baseUrl.TrimEnd('/') -ne $Endpoint.TrimEnd('/')) { throw "Pi provider $PiProvider does not point to TargetEndpoint" }
    $token="PI_PRODUCTION_OK_$($Stamp.Substring($Stamp.Length-7,7))"
    $args=@("--provider",$PiProvider,"--model",$PiModel,"--no-session","--no-tools","--no-extensions","--no-skills","--no-context-files","--print","Reply with exactly: $token")
    $environment=@{}
    if($AgentDir){$environment.PI_CODING_AGENT_DIR=$AgentDir}
    $result=Invoke-Process -File $PiExecutable -Arguments $args -CaptureOutput -SensitiveArguments -TimeoutSeconds $PiTimeoutSeconds -Environment $environment
    if ($result.StdOut.Trim() -ne $token) { throw "Pi acceptance returned unexpected output; see $($result.StdOutPath)" }
    Write-Host "Pi acceptance passed: provider=$PiProvider model=$PiModel"
}

function Get-LocalCanonicalCount {
    $helper = Join-Path $EvidenceDir "canonical_count.py"
    if (-not (Test-Path -LiteralPath $helper)) {
@'
import os,sqlite3,sys
p=os.path.abspath(sys.argv[1]).replace(chr(92),'/')
db=sqlite3.connect('file:'+p+'?mode=ro',uri=True,timeout=10)
try:
 print(db.execute("select count(*) from usage_events where trim(coalesce(connection_id,''))<>'' and trim(coalesce(request_id,''))<>''").fetchone()[0])
finally: db.close()
'@ | Set-Content -LiteralPath $helper -Encoding utf8
    }
    $result=Invoke-Process -File "python" -Arguments @($helper,(Join-Path $Root "data\analytics.db")) -CaptureOutput
    [int64]$result.StdOut.Trim()
}

function Test-LocalPiWithAccounting {
    param([string]$Endpoint)
    $before=Get-LocalCanonicalCount
    Test-Pi $Endpoint
    $deadline=(Get-Date).AddSeconds(15);$after=$before
    do { Start-Sleep -Milliseconds 500; $after=Get-LocalCanonicalCount; if($after -ge $before+1){break} } while((Get-Date)-lt $deadline)
    if($after -lt $before+1){throw "Pi request did not create a canonical accounting event; before=$before after=$after"}
    [ordered]@{before=$before;after=$after;observed_growth=$after-$before;provider=$PiProvider;model=$PiModel;note="Concurrent Production traffic may increase the global count by more than one; canonical uniqueness is separately integrity-checked."}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "pi-accounting.json") -Encoding utf8
    Write-Host "Pi accounting write passed: $before -> $after"
}

function Validate-LocalProduction {
    Write-Phase "validating" "Web, auth boundary, exact image, Pi, and accounting"
    Wait-LocalHealthy $LocalProductionContainer
    $info=Get-ContainerInfo $LocalProductionContainer
    if ($info.ImageID -ne $AcceptedImageID -or $info.Revision -ne $AcceptedRevision) { throw "Production image mismatch" }
    Assert-HTTP "$TargetEndpoint/" @(200)
    Assert-HTTP "$TargetEndpoint/healthz" @(200)
    Assert-HTTP "$TargetEndpoint/api/v2/usage?range=24h" @(401)
    Assert-WebAndOAuth $TargetEndpoint
    Test-LocalPiWithAccounting $TargetEndpoint
    $helper=Write-SQLiteHelper
    New-Snapshot (Join-Path $Root "data\analytics.db") (Join-Path $EvidenceDir "post-pi-integrity.db") (Join-Path $EvidenceDir "post-pi-integrity.json") $helper
}

function Get-SSHArguments {
    $a=@("-o","BatchMode=yes","-o","ConnectTimeout=10")
    if($TargetSSHIdentityFile){$a+=@("-i",$TargetSSHIdentityFile)}
    $a+=$TargetHost
    $a
}
function Invoke-SSH { param([Parameter(Mandatory=$true)][string]$RemoteCommand, [switch]$AllowFailure)
    if(-not $RemoteCommand){throw "Empty remote SSH command"}
    # Encode the complete POSIX program so Windows command-line parsing and
    # OpenSSH's remote command joining cannot reinterpret quotes/metacharacters.
    $encoded=[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($RemoteCommand))
    $wire="echo $encoded|base64 -d|sh"
    $a=@(Get-SSHArguments);$a+=$wire
    Invoke-Process -File "ssh.exe" -Arguments $a -AllowFailure:$AllowFailure -CaptureOutput -SensitiveArguments
}
function Invoke-SCP { param([string]$Local, [string]$Remote)
    $a=@("-o","BatchMode=yes","-o","ConnectTimeout=10")
    if($TargetSSHIdentityFile){$a+=@("-i",$TargetSSHIdentityFile)}
    $a+=@($Local,"$TargetHost`:$Remote")
    Invoke-Process -File "scp.exe" -Arguments $a -SensitiveArguments
}
function Quote-Sh([string]$s) {
    if ($s.Contains([string][char]39)) { throw "Remote shell value may not contain a single quote" }
    return ([string][char]39) + $s + ([string][char]39)
}

function Initialize-RemoteTarget {
    if($TargetSSHIdentityFile){
        $script:TargetSSHIdentityFile=[IO.Path]::GetFullPath($TargetSSHIdentityFile)
        if(-not(Test-Path -LiteralPath $TargetSSHIdentityFile)){throw "Target SSH identity missing"}
    }
    $basic=@'
test "$(uname -s)" = Linux && command -v docker >/dev/null && command -v python3 >/dev/null
'@
    [void](Invoke-SSH -RemoteCommand $basic)
    if($TargetBootstrap){
        Write-Phase "bootstrapping-target" "Installing bounded prerequisites and creating target root"
        $qRoot=Quote-Sh $TargetRoot
        $bootstrap=@'
set -eu
if ! docker compose version >/dev/null 2>&1; then
  sudo -n apt-get update >/dev/null
  if apt-cache show docker-compose-plugin >/dev/null 2>&1; then pkg=docker-compose-plugin; else pkg=docker-compose-v2; fi
  sudo -n apt-get install -y "$pkg" >/dev/null
fi
sudo -n mkdir -p {0}
sudo -n chown "$(id -u):$(id -g)" {0}
test -w {0}
docker compose version >/dev/null
'@ -f $qRoot
        [void](Invoke-SSH -RemoteCommand $bootstrap)
    }else{
        [void](Invoke-SSH "test -d $(Quote-Sh $TargetRoot) && test -w $(Quote-Sh $TargetRoot) && docker compose version >/dev/null")
    }
}

function Stage-RemoteImage {
    if (-not $TargetHost -or -not $TargetRoot) { throw "Relocate requires TargetHost and TargetRoot" }
    if (-not $TargetRoot.StartsWith('/')) { throw "Relocation TargetRoot must be an absolute POSIX path" }
    if ($TargetBindAddress -notmatch '^(0\.0\.0\.0|127\.0\.0\.1|(?:[0-9]{1,3}\.){3}[0-9]{1,3})$') { throw "TargetBindAddress must be an explicit IPv4 address" }
    Initialize-RemoteTarget
    Write-Phase "staging-remote-image" "Transferring immutable image before downtime"
    $tar=Join-Path $EvidenceDir "accepted-image.tar"
    [void](Invoke-Docker -Arguments @("save","-o",$tar,$AcceptedImage))
    $remoteStage="$TargetRoot/.codex-deploy-$Stamp"
    [void](Invoke-SSH "docker compose version >/dev/null && mkdir -p $(Quote-Sh $remoteStage)")
    $qData=Quote-Sh "$TargetRoot/data"; $qPool=Quote-Sh "$TargetRoot/pool"; $qEnv=Quote-Sh "$TargetRoot/.env"
    $targetCheck = Invoke-SSH "if [ -e $qData ] || [ -e $qPool ] || [ -e $qEnv ]; then echo occupied; else echo empty; fi"
    if ($targetCheck.StdOut.Trim() -ne "empty") { throw "Relocation target already contains Production authority state; refusing overwrite" }
    Invoke-SCP $tar "$remoteStage/accepted-image.tar"
    $remoteCounterLocal=Join-Path $EvidenceDir "remote_canonical_count.py"
@'
import sqlite3,sys
db=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True,timeout=10)
try:
 print(db.execute("select count(*) from usage_events where trim(coalesce(connection_id,''))<>'' and trim(coalesce(request_id,''))<>''").fetchone()[0])
finally: db.close()
'@ | Set-Content -LiteralPath $remoteCounterLocal -Encoding utf8
    Invoke-SCP $remoteCounterLocal "$remoteStage/remote_canonical_count.py"
    $envEditor=Join-Path $EvidenceDir "set_target_env.py"
@'
import os,sys
path,public_url,redirect=sys.argv[1:4]
values={"PUBLIC_URL":public_url}
if redirect: values["OAUTH_GOOGLE_REDIRECT_URI"]=redirect
lines=[]
if os.path.exists(path): lines=open(path,encoding="utf-8-sig").read().splitlines()
out=[]; seen=set()
for line in lines:
  stripped=line.strip()
  if not stripped or stripped.startswith("#") or "=" not in line:
    out.append(line); continue
  key=line.split("=",1)[0].strip()
  if key in values:
    out.append(key+"="+values[key]); seen.add(key)
  else: out.append(line)
for key,value in values.items():
  if key not in seen: out.append(key+"="+value)
with open(path,"w",encoding="utf-8",newline="\n") as f: f.write("\n".join(out)+"\n")
'@ | Set-Content -LiteralPath $envEditor -Encoding utf8
    Invoke-SCP $envEditor "$remoteStage/set_target_env.py"
    $composeEditor=Join-Path $EvidenceDir "set_target_compose.py"
@'
import re,sys
path,bind=sys.argv[1:3]
s=open(path,encoding="utf-8-sig").read()
# Production Compose contract has exactly one gateway port and these mounts.
s,n=re.subn(r'(?m)^\s*-\s*"(?:0\.0\.0\.0:)?8989:8989"\s*$', '      - "'+bind+':8989:8989"', s)
if n!=1: raise SystemExit("expected exactly one Production port mapping")
required=['./pool:/app/pool','./data:/app/data']
for item in required:
  if item not in s: raise SystemExit("required Production mount missing: "+item)
open(path,"w",encoding="utf-8",newline="\n").write(s)
'@ | Set-Content -LiteralPath $composeEditor -Encoding utf8
    Invoke-SCP $composeEditor "$remoteStage/set_target_compose.py"
    $remoteTar = "$remoteStage/accepted-image.tar"
    $quotedRemoteTar = Quote-Sh $remoteTar
    $quotedImage = Quote-Sh $AcceptedImage
    $verifyImage=@'
docker load -i {0} >/dev/null
test "$(docker image inspect {1} --format '{{{{.Id}}}}')" = {2}
test "$(docker image inspect {1} --format '{{{{index .Config.Labels "org.opencontainers.image.revision"}}}}')" = {3}
'@ -f $quotedRemoteTar,$quotedImage,(Quote-Sh $AcceptedImageID),(Quote-Sh $AcceptedRevision)
    [void](Invoke-SSH -RemoteCommand $verifyImage)
    Remove-Item -LiteralPath $tar -Force
}

function Package-ProductionState {
    param([string]$Output)
    $helper=Join-Path $EvidenceDir "package_state.py"
    @'
import os,sys,tarfile
root,out=sys.argv[1:3]
items=["data","pool","provider-specs","docker-compose.yml",".env"]
with tarfile.open(out,"w:gz") as t:
  for n in items:
    p=os.path.join(root,n)
    if not os.path.exists(p): continue
    if n=="data":
      def filt(info):
        rel=os.path.relpath(info.name,"data")
        if rel=="backups" or rel.startswith("backups/"): return None
        return info
      t.add(p,arcname=n,filter=filt)
    else: t.add(p,arcname=n)
'@|Set-Content -LiteralPath $helper -Encoding utf8
    [void](Invoke-Process -File "python" -Arguments @($helper,$Root,$Output))
}

function Install-RemoteState {
    $package=Join-Path $EvidenceDir "production-state.tar.gz"
    Package-ProductionState $package
    $remoteStage="$TargetRoot/.codex-deploy-$Stamp"
    Invoke-SCP $package "$remoteStage/production-state.tar.gz"
    $previous = "$remoteStage/previous"
    $previousData = "$previous/data"
    $previousPool = "$previous/pool"
    $previousSpecs = "$previous/provider-specs"
    $remotePackage = "$remoteStage/production-state.tar.gz"
    $qRoot = Quote-Sh $TargetRoot
    $qPrevious = Quote-Sh $previous
    $qPreviousData = Quote-Sh $previousData
    $qPreviousPool = Quote-Sh $previousPool
    $qPreviousSpecs = Quote-Sh $previousSpecs
    $qRemotePackage = Quote-Sh $remotePackage
    $qImage = Quote-Sh $AcceptedImage
    $qProject = Quote-Sh $TargetComposeProject
    $qEnvEditor=Quote-Sh "$remoteStage/set_target_env.py"
    $qPublicURL=Quote-Sh $TargetPublicURL
    $qRedirect=Quote-Sh $TargetOAuthRedirectURI
    $qBind=Quote-Sh $TargetBindAddress
    $composeEditor=Quote-Sh "$remoteStage/set_target_compose.py"
    $script:TargetStarted=$true
    $install=@'
set -eu
mkdir -p {0}
cd {0}
if [ -e data ] || [ -e pool ]; then
  mkdir -p {1}
  [ ! -e data ] || mv data {2}
  [ ! -e pool ] || mv pool {3}
  [ ! -e provider-specs ] || mv provider-specs {4}
fi
tar -xzf {5}
python3 {6} .env {7} {8}
python3 {9} docker-compose.yml {10}
docker tag {11} codex-pool:latest
uid=$(docker run --rm --entrypoint id {11} -u codex)
gid=$(docker run --rm --entrypoint id {11} -g codex)
sudo -n chown -R "$uid:$gid" data pool
test "$(stat -c %u data)" = "$uid"
test "$(stat -c %g data)" = "$gid"
docker compose -p {12} -f docker-compose.yml up -d --no-build --force-recreate codex-pool
'@ -f $qRoot,$qPrevious,$qPreviousData,$qPreviousPool,$qPreviousSpecs,$qRemotePackage,$qEnvEditor,$qPublicURL,$qRedirect,$composeEditor,$qBind,$qImage,$qProject
    [void](Invoke-SSH -RemoteCommand $install)
}

function Get-RemoteCanonicalCount {
    $helper="$TargetRoot/.codex-deploy-$Stamp/remote_canonical_count.py"
    $database="$TargetRoot/data/analytics.db"
    $result=Invoke-SSH "python3 $(Quote-Sh $helper) $(Quote-Sh $database)"
    [int64]$result.StdOut.Trim()
}

function Test-RemotePiWithAccounting {
    param([string]$Endpoint,[string]$AgentDir="")
    $before=Get-RemoteCanonicalCount
    Test-Pi $Endpoint $AgentDir
    $deadline=(Get-Date).AddSeconds(15);$after=$before
    do { Start-Sleep -Milliseconds 500; $after=Get-RemoteCanonicalCount; if($after -ge $before+1){break} } while((Get-Date)-lt $deadline)
    if($after -lt $before+1){throw "Remote Pi request did not create a canonical accounting event; before=$before after=$after"}
    [ordered]@{before=$before;after=$after;observed_growth=$after-$before;provider=$PiProvider;model=$PiModel}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "pi-accounting.json") -Encoding utf8
}

function Assert-RemoteIntegrity {
    $database="$TargetRoot/data/analytics.db"
    $code=@'
import sqlite3,sys
db=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True,timeout=30)
try:
 q=db.execute('pragma quick_check').fetchone()[0]
 d=db.execute("select count(*) from (select connection_id,request_id from usage_events where trim(coalesce(connection_id,''))<>'' and trim(coalesce(request_id,''))<>'' group by connection_id,request_id having count(*)>1)").fetchone()[0]
 print(str(q)+'|'+str(d))
finally:db.close()
'@
    $local=Join-Path $EvidenceDir "remote_integrity.py";$code|Set-Content $local -Encoding utf8
    $remote="$TargetRoot/.codex-deploy-$Stamp/remote_integrity.py";Invoke-SCP $local $remote
    $result=Invoke-SSH "python3 $(Quote-Sh $remote) $(Quote-Sh $database)"
    if($result.StdOut.Trim() -ne "ok|0"){throw "Remote SQLite integrity/uniqueness failed: $($result.StdOut.Trim())"}
    [ordered]@{quick_check="ok";duplicate_canonical_identity_groups=0}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "remote-post-pi-integrity.json") -Encoding utf8
}

function Capture-RemoteFailureEvidence {
    $dir=Join-Path $EvidenceDir "remote-failure"
    New-Item -ItemType Directory -Force -Path $dir|Out-Null
    foreach($item in @(
        @{name="inspect.txt";command="docker inspect $(Quote-Sh $ProductionContainer)"},
        @{name="logs.txt";command="docker logs --tail 200 $(Quote-Sh $ProductionContainer)"},
        @{name="compose-ps.txt";command="cd $(Quote-Sh $TargetRoot) && docker compose -p $(Quote-Sh $TargetComposeProject) -f docker-compose.yml ps -a"}
    )){
        try{
            $r=Invoke-SSH -RemoteCommand $item.command -AllowFailure
            # Application logs may contain request metadata. Preserve locally in
            # ignored evidence; never print their contents to the transcript.
            [IO.File]::WriteAllText((Join-Path $dir $item.name),$r.StdOut+"`n--- stderr ---`n"+$r.StdErr)
        }catch{}
    }
}

function Validate-RemoteProduction {
    Write-Phase "validating-remote" "Destination web, container, Pi"
    $deadline=(Get-Date).AddSeconds($HealthTimeoutSeconds);$healthy=$false;$last=""
    do {
        $probe=Invoke-Process -File "curl.exe" -Arguments @("-sS","--connect-timeout","3","--max-time","5","-o","NUL","-w","%{http_code}","$TargetEndpoint/healthz") -CaptureOutput -AllowFailure -TimeoutSeconds 10
        $last="exit=$($probe.ExitCode) http=$($probe.StdOut.Trim())"
        if($probe.ExitCode -eq 0 -and $probe.StdOut.Trim() -eq "200"){$healthy=$true;break}
        $remote=Invoke-SSH -RemoteCommand ("docker inspect " + (Quote-Sh $ProductionContainer) + " --format " + (Quote-Sh '{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}|{{.State.ExitCode}}|{{.State.Error}}')) -AllowFailure
        if($remote.ExitCode -ne 0 -or $remote.StdOut -match 'exited|dead|unhealthy|restarting'){
            Capture-RemoteFailureEvidence
            throw "Remote Production failed before health timeout; evidence=$EvidenceDir\remote-failure; state=$($remote.StdOut.Trim()); curl $last"
        }
        Start-Sleep 1
    }while((Get-Date)-lt $deadline)
    if(-not $healthy){Capture-RemoteFailureEvidence;throw "Remote Production health timed out after $HealthTimeoutSeconds seconds; evidence=$EvidenceDir\remote-failure; $last"}
    Assert-HTTP "$TargetEndpoint/" @(200)
    Assert-HTTP "$TargetEndpoint/api/v2/usage?range=24h" @(401)
    Assert-WebAndOAuth $TargetEndpoint
    $format='{{.State.Status}}|{{.State.Health.Status}}|{{index .Config.Labels "org.opencontainers.image.revision"}}'
    $inspect=Invoke-SSH ("docker inspect " + (Quote-Sh $ProductionContainer) + " --format " + (Quote-Sh $format))
    if ($inspect.StdOut.Trim() -notmatch "^running\|healthy\|$([regex]::Escape($AcceptedRevision))$") { throw "Remote Production runtime mismatch" }
    $tempPi=New-TemporaryPiProfile
    Test-RemotePiWithAccounting $TargetEndpoint $tempPi
    Assert-RemoteIntegrity
    Set-RealPiEndpoint
    Test-RemotePiWithAccounting $TargetEndpoint
}

function Stop-RemoteBestEffort {
    if ($TargetStarted) {
        try { [void](Invoke-SSH "cd $(Quote-Sh $TargetRoot) && docker compose -p $(Quote-Sh $TargetComposeProject) -f docker-compose.yml down" -AllowFailure) } catch {}
        try {
            $failed="$TargetRoot/.codex-deploy-$Stamp/failed-installed-state"
            $qFailed=Quote-Sh $failed
            $qRoot=Quote-Sh $TargetRoot
            [void](Invoke-SSH "mkdir -p $qFailed; cd $qRoot; [ ! -e data ] || mv data $qFailed/; [ ! -e pool ] || mv pool $qFailed/; [ ! -e provider-specs ] || mv provider-specs $qFailed/; [ ! -e .env ] || mv .env $qFailed/; [ ! -e docker-compose.yml ] || mv docker-compose.yml $qFailed/" -AllowFailure)
        } catch {}
    }
}

function Test-RemotePreflight {
    if($Mode -ne "Relocate"){return}
    if(-not $TargetHost -or -not $TargetRoot){throw "Relocate requires TargetHost and TargetRoot"}
    if(-not $TargetRoot.StartsWith('/')){throw "Relocation TargetRoot must be an absolute POSIX path"}
    if($TargetSSHIdentityFile){
        $script:TargetSSHIdentityFile=[IO.Path]::GetFullPath($TargetSSHIdentityFile)
        if(-not(Test-Path -LiteralPath $TargetSSHIdentityFile)){throw "Target SSH identity missing: $TargetSSHIdentityFile"}
    }
    $i=$TargetRoot.LastIndexOf('/');$parent=if($i -le 0){'/'}else{$TargetRoot.Substring(0,$i)}
    $qRoot=Quote-Sh $TargetRoot;$qParent=Quote-Sh $parent
    $remoteCheck=@'
set -eu
test "$(uname -s)" = Linux
test "$(uname -m)" = x86_64
command -v docker >/dev/null
command -v python3 >/dev/null
docker info >/dev/null
if ss -ltn 2>/dev/null | grep -q ':8989 '; then echo port-occupied; else echo port-free; fi
if [ -e {0} ]; then
  if find {0} -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then echo root-occupied; else echo root-empty; fi
else echo root-absent; fi
if docker compose version >/dev/null 2>&1; then echo compose-ready
elif apt-cache show docker-compose-plugin >/dev/null 2>&1 || apt-cache show docker-compose-v2 >/dev/null 2>&1; then echo compose-installable
else echo compose-missing; fi
if test -w {1}; then echo parent-writable
elif sudo -n true 2>/dev/null; then echo sudo-ready
else echo no-bootstrap-access; fi
'@ -f $qRoot,$qParent
    $check=Invoke-SSH -RemoteCommand $remoteCheck
    $lines=@($check.StdOut -split "`r?`n" | Where-Object {$_})
    if($lines.Count -eq 0){throw "Remote preflight returned no validation markers; stdout=$($check.StdOutPath) stderr=$($check.StdErrPath)"}
    if($lines -contains "port-occupied"){throw "Target port 8989 is occupied"}
    if($lines -contains "root-occupied"){throw "Target root is not empty"}
    if($lines -contains "compose-missing"){throw "Docker Compose is unavailable and not installable"}
    if($lines -contains "no-bootstrap-access"){throw "Target root cannot be created and noninteractive sudo is unavailable"}
    if(-not $TargetBootstrap -and (($lines -notcontains "compose-ready") -or ($lines -notcontains "root-empty"))){throw "Target requires bootstrap but TargetBootstrap is false"}
    Write-Host "Remote preflight passed: $($lines -join ', ')"
}

function Assert-EmbeddedCutoverSafety {
    $text=[IO.File]::ReadAllText($PSCommandPath)
    $mainStart=$text.LastIndexOf('$RollbackTag="codex-pool:production-rollback-')
    $mainEnd=$text.IndexOf('$Completed=$true; Write-Phase "complete"',$mainStart)
    if($mainStart -lt 0 -or $mainEnd -le $mainStart){throw "Embedded safety gate cannot identify cutover block"}
    $main=$text.Substring($mainStart,$mainEnd-$mainStart)
    $freeze=$main.IndexOf("`n    Stop-Adjacent")
    $final=$main.IndexOf('$final=Prepare-Merge "final"',$freeze)
    $restart=$main.IndexOf("`n    Start-TestAndStaging",$final)
    $prodStop=$main.IndexOf("`n    Stop-Production",$restart)
    $apply=$main.IndexOf("`n    Apply-FinalMerge",$prodStop)
    $remote=$main.IndexOf("`n        Install-RemoteState",$apply)
    if(-not($freeze -ge 0 -and $final -gt $freeze -and $restart -gt $final -and $prodStop -gt $restart -and $apply -gt $prodStop -and $remote -gt $apply)){
        throw "Embedded safety gate rejected cutover ordering"
    }
    if(($main.Split([string[]]@('Start-TestAndStaging'),[StringSplitOptions]::None).Count-1) -ne 1){throw "Embedded safety gate rejected adjacent restart count"}
    foreach($required in @('if(Test-LocalProductionHealthy)','rollback-source-already-healthy','Capture-RemoteFailureEvidence','adjacent-freeze-window.json')){
        if(-not $text.Contains($required)){throw "Embedded safety gate missing invariant: $required"}
    }
}

Push-Location $Root
try {
    $Mutex=New-Object Threading.Mutex($false,"Local\CodexPoolProductionDeployment")
    if (-not $Mutex.WaitOne(0)) { throw "Another Production deployment is already running" }
    foreach($tool in @("docker","python","curl.exe",$PiExecutable)){if(-not(Get-Command $tool -ErrorAction SilentlyContinue)){throw "Required tool missing: $tool"}}
    foreach($p in @($TestCompose,$StagingCompose,$ProductionCompose,$TestEnv,$StagingEnv)){if(-not(Test-Path -LiteralPath $p)){throw "Required path missing: $p"}}
    if (-not $AcceptedImage -or -not $AcceptedRevision) { throw "AcceptedImage and AcceptedRevision are required" }
    if (-not $PiProvider -or -not $PiModel) { throw "PiProvider and PiModel are required" }
    if ($Mode -eq "Relocate") { foreach($tool in @("ssh","scp")){if(-not(Get-Command $tool -ErrorAction SilentlyContinue)){throw "Relocate tool missing: $tool"}}; Test-RemotePreflight }

    $image=Get-ImageInfo $AcceptedImage; $AcceptedImageID=$image.ID
    if ($image.Revision -ne $AcceptedRevision) { throw "Accepted image revision mismatch: $($image.Revision)" }
    $test=Get-ContainerInfo $TestContainer; $staging=Get-ContainerInfo $StagingContainer; $prod=Get-ContainerInfo $LocalProductionContainer
    if (-not $SkipAdjacentImageGate -and ($test.ImageID -ne $AcceptedImageID -or $staging.ImageID -ne $AcceptedImageID)) { throw "Test and Staging must run the exact accepted image" }
    if ($test.Health -ne "healthy" -or $staging.Health -ne "healthy" -or $prod.Health -ne "healthy") { throw "All source environments must begin healthy" }
    $PreviousProductionImageID=$prod.ImageID
    Assert-HTTP "$TestEndpoint/healthz" @(200); Assert-HTTP "$StagingEndpoint/healthz" @(200)
    if ($Mode -eq "InPlace") { Assert-HTTP "$TargetEndpoint/healthz" @(200) }
    $piProviderConfig=Get-PiProviderConfig
    $piPointsToTarget=$piProviderConfig.baseUrl.TrimEnd('/') -eq $TargetEndpoint.TrimEnd('/')
    if($PiEndpointSwitchMode -eq "ValidateOnly" -and -not $piPointsToTarget){throw "Pi provider $PiProvider must point exactly to TargetEndpoint when PiEndpointSwitchMode=ValidateOnly"}
    if($PiEndpointSwitchMode -eq "UpdateOnCutover" -and $Mode -ne "Relocate"){throw "UpdateOnCutover is only valid for Relocate"}
    if($PiEndpointSwitchMode -eq "UpdateOnCutover" -and $piPointsToTarget){throw "Pi already points to TargetEndpoint; use ValidateOnly"}
    $models=Invoke-Process -File $PiExecutable -Arguments @("--list-models") -CaptureOutput -TimeoutSeconds $PiTimeoutSeconds
    $expectedModelLine="(?m)^"+[regex]::Escape($PiProvider)+"\s+"+[regex]::Escape($PiModel)+"\s"
    if($models.StdOut -notmatch $expectedModelLine){throw "Pi does not recognize $PiProvider/$PiModel"}

    if (-not $Execute -and -not $PrepareOnly) {
        Write-Host "PLAN ONLY: preflight passed; no state changed."
        Write-Host "Prepare only: .\scripts\deploy-production.ps1 -PrepareOnly -ConfigPath <config>"
        Write-Host "Execute: .\scripts\deploy-production.ps1 -Execute -Confirmation $RequiredConfirmation [same parameters]"
        exit 0
    }
    if ($Execute -and $PrepareOnly) { throw "Execute and PrepareOnly are mutually exclusive" }
    if ($Execute -and $Confirmation -ne $RequiredConfirmation) { throw "Execution requires -Confirmation $RequiredConfirmation" }
    if($Execute){Assert-EmbeddedCutoverSafety}

    $Stamp=(Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
    $EvidenceDir=Join-Path $Root "data\backups\production-deploy-$Stamp"
    New-Item -ItemType Directory -Force -Path $EvidenceDir|Out-Null
    $StatePath=Join-Path $EvidenceDir "state.json"; $LogPath=Join-Path $EvidenceDir "transcript.txt"
    Start-Transcript -LiteralPath $LogPath|Out-Null
    [ordered]@{mode=$Mode;accepted_image=$AcceptedImage;accepted_image_id=$AcceptedImageID;accepted_revision=$AcceptedRevision;target_host=$TargetHost;target_root=$TargetRoot;target_endpoint=$TargetEndpoint;pi_provider=$PiProvider;pi_model=$PiModel;conflict_policy=$ConflictPolicy}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "inputs.json") -Encoding utf8
    Write-Phase "preparing" "Online snapshots and full simulation; Production remains available"
    [void](Prepare-Merge "preparation")
    if ($PrepareOnly) {
        $Completed=$true
        Write-Phase "prepared" "Full online simulation passed; no gateway was stopped or changed"
        [ordered]@{prepared_utc=(Get-Date).ToUniversalTime().ToString("o");accepted_image_id=$AcceptedImageID;accepted_revision=$AcceptedRevision;runtime_changes=$false}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "PREPARED.json") -Encoding utf8
        exit 0
    }
    if ($Mode -eq "Relocate") { Stage-RemoteImage }

    $RollbackTag="codex-pool:production-rollback-$Stamp"
    [void](Invoke-Docker -Arguments @("tag",$PreviousProductionImageID,$RollbackTag))
    Stop-Adjacent
    Write-Phase "final-source-reconciliation" "Frozen Test/Staging sources; Production still online"
    $final=Prepare-Merge "final"
    # Test and Staging are no longer part of Production downtime or destination
    # validation. Their immutable snapshots above are the migration inputs.
    Start-TestAndStaging
    Stop-Production
    $rollback=Save-LocalRollback
    Write-Phase "applying" "Applying final reviewed canonical sources to stopped Production ledger"
    Apply-FinalMerge $final

    if ($Mode -eq "InPlace") {
        [void](Invoke-Docker -Arguments @("tag",$AcceptedImageID,"codex-pool:production-$($AcceptedRevision.Substring(0,[Math]::Min(7,$AcceptedRevision.Length)))-$Stamp"))
        [void](Invoke-Docker -Arguments @("tag",$AcceptedImageID,"codex-pool:latest"))
        Set-LocalTargetEnvironment
        [void](Invoke-Docker -Arguments @("compose","--project-name","codex-pool","-f",$ProductionCompose,"up","--detach","--no-build","--force-recreate","codex-pool"))
        Validate-LocalProduction
    } else {
        Write-Phase "relocating" "Transferring complete stopped Production authority state"
        Install-RemoteState
        Validate-RemoteProduction
        Write-Phase "retiring-source" "Removing stopped source container to prevent split-brain"
        [void](Invoke-Docker -Arguments @("compose","--project-name","codex-pool","-f",$ProductionCompose,"rm","--force","--stop","codex-pool"))
    }

    $Completed=$true; Write-Phase "complete" "Deployment and executable acceptance passed"
    [ordered]@{completed_utc=(Get-Date).ToUniversalTime().ToString("o");mode=$Mode;accepted_image_id=$AcceptedImageID;accepted_revision=$AcceptedRevision;previous_image_id=$PreviousProductionImageID;rollback_tag=$RollbackTag;target_endpoint=$TargetEndpoint;pi_acceptance="passed"}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "COMPLETED.json") -Encoding utf8
    exit 0
}
catch {
    $failure=$_
    [Console]::Error.WriteLine("DEPLOYMENT FAILED: "+$failure.Exception.Message)
    if ($EvidenceDir) { try { Write-Phase "failed" $failure.Exception.Message } catch {} }
    if ($Stopped) {
        try {
            Stop-RemoteBestEffort
            Restore-PiConfig
            $rollback=Join-Path $EvidenceDir "rollback"
            if(Test-LocalProductionHealthy){
                Write-Phase "rollback-source-already-healthy" "Source Production is healthy; no restart performed"
            } elseif(Test-Path -LiteralPath (Join-Path $rollback "production-state.tar.gz")) {
                Restore-LocalRollback $rollback
            } else {
                Write-Phase "rollback-fallback" "Rollback archive unavailable; restarting unchanged state with previous image"
                [void](Invoke-Docker -Arguments @("tag",$PreviousProductionImageID,"codex-pool:latest"))
                [void](Invoke-Docker -Arguments @("compose","--project-name","codex-pool","-f",$ProductionCompose,"up","--detach","--no-build","--force-recreate","codex-pool"))
                Wait-LocalHealthy $LocalProductionContainer
            }
        } catch { [Console]::Error.WriteLine("PRODUCTION ROLLBACK FAILED: "+$_.Exception.Message) }
    }
    if ($AdjacentStopped) {
        try { Start-TestAndStaging } catch { [Console]::Error.WriteLine("ADJACENT RESTART FAILED: "+$_.Exception.Message) }
    }
    if ($EvidenceDir) {
        try {
            [ordered]@{failed_utc=(Get-Date).ToUniversalTime().ToString("o");error=$failure.Exception.Message;rollback_attempted=$Stopped}|ConvertTo-Json|Set-Content (Join-Path $EvidenceDir "FAILED.json") -Encoding utf8
        } catch {}
    }
    exit 1
}
finally {
    try{Stop-Transcript|Out-Null}catch{}
    if($Mutex){try{$Mutex.ReleaseMutex()}catch{};$Mutex.Dispose()}
    Pop-Location
}
