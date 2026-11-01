# Install the sshc Windows CLI without administrator privileges.
#
#   powershell -NoProfile -ExecutionPolicy Bypass -Command `
#     "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
#
# Environment variables:
#   SSHC_VERSION      release to install (default: latest)
#   SSHC_INSTALL_DIR  destination (default: %LOCALAPPDATA%\Programs\sshc)
#   SSHC_ADD_TO_PATH  set to 0 to leave the user PATH unchanged (default: 1)

& {
[CmdletBinding()]
param([switch]$Uninstall)

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
# Native failures are checked explicitly, including expected status/gh failures.
$PSNativeCommandUseErrorActionPreference = $false
$installerBody = $MyInvocation.MyCommand.ScriptBlock.ToString()
$uninstallKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\sshc'
$uninstallerName = 'uninstall.ps1'
$receiptFileName = '.sshc-install-receipt.json'

$Repository = 'aida0710/sshc'

function Write-Note([string]$Message) {
    Write-Output "  $Message"
}

function Stop-Install([string]$Message) {
    throw "sshc: $Message"
}

function Get-ReleaseArchitecture {
    $reported = @($env:PROCESSOR_ARCHITEW6432, $env:PROCESSOR_ARCHITECTURE) |
        Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    if ($reported -contains 'ARM64') { return 'arm64' }
    if ($reported -contains 'AMD64') { return 'amd64' }
    Stop-Install "$($reported -join '/') is not an architecture sshc publishes a Windows binary for"
}

function Get-ReleaseTag {
    if ([string]::IsNullOrWhiteSpace($env:SSHC_VERSION)) { return 'latest' }
    $tag = $env:SSHC_VERSION.Trim()
    if (-not $tag.StartsWith('v', [System.StringComparison]::Ordinal)) { $tag = "v$tag" }
    if ($tag -cnotmatch '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$') {
        Stop-Install "SSHC_VERSION is not a semantic version: $tag"
    }
    return $tag
}

function Copy-ReleaseFile([string]$Name, [string]$Destination, [string]$BaseUrl) {
    if ($env:SSHC_INSTALL_TESTING -eq '1' -and -not [string]::IsNullOrWhiteSpace($env:SSHC_TEST_RELEASE_DIR)) {
        Copy-Item -LiteralPath (Join-Path $env:SSHC_TEST_RELEASE_DIR $Name) -Destination $Destination
        return
    }
    $uri = "$BaseUrl/$Name"
    $previousSecurityProtocol = [Net.ServicePointManager]::SecurityProtocol
    try {
        # Windows PowerShell 5.1 may otherwise negotiate a protocol GitHub rejects.
        [Net.ServicePointManager]::SecurityProtocol = $previousSecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
        Invoke-WebRequest -Uri $uri -OutFile $Destination -UseBasicParsing -TimeoutSec 180 `
            -Headers @{ 'User-Agent' = 'sshc-install.ps1' }
    }
    catch {
        Stop-Install "could not download $uri ($($_.Exception.Message))"
    }
    finally { [Net.ServicePointManager]::SecurityProtocol = $previousSecurityProtocol }
}

function Read-PublishedChecksum([string]$ChecksumPath, [string]$AssetName) {
    $foundHashes = @()
    foreach ($line in [System.IO.File]::ReadAllLines($ChecksumPath)) {
        if ($line -match '^([0-9A-Fa-f]{64})[ \t]+\*?(.+)$' -and $Matches[2] -ceq $AssetName) {
            $foundHashes += $Matches[1].ToLowerInvariant()
        }
    }
    if ($foundHashes.Count -ne 1) {
        Stop-Install "checksums.txt must list $AssetName exactly once"
    }
    return $foundHashes[0]
}

function Test-ReparsePoint([string]$Path, [string]$Description) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    if ($null -eq $item) { return }
    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        Stop-Install "$Description is a symbolic link or junction: $Path. Remove it first, or set SSHC_INSTALL_DIR."
    }
}

function Remove-Backups([string]$Directory, [switch]$Required) {
    foreach ($backup in @(Get-ChildItem -LiteralPath $Directory -Filter '.sshc.backup.*.exe' -File -Force)) {
        try { Remove-Item -LiteralPath $backup.FullName -Force }
        catch {
            if ($Required) { throw }
            Write-Note "backup is still in use: $($backup.FullName); it will be removed on the next install"
        }
    }
}

function Publish-Executable([string]$Source, [string]$Target) {
    $directory = Split-Path -Parent $Target
    $staged = Join-Path $directory ('.sshc.install.' + [System.Guid]::NewGuid().ToString('N') + '.exe')
    $backup = Join-Path $directory ('.sshc.backup.' + [System.Guid]::NewGuid().ToString('N') + '.exe')
    try {
        [System.IO.File]::Copy($Source, $staged, $false)
        # Windows permits renaming a loaded image, but cannot overwrite/delete it.
        # Keep sshc update (or the web engine) alive on the renamed image until it exits.
        if ([System.IO.File]::Exists($Target)) { [System.IO.File]::Move($Target, $backup) }
        try { [System.IO.File]::Move($staged, $Target) }
        catch {
            if ([System.IO.File]::Exists($backup)) { [System.IO.File]::Move($backup, $Target) }
            Stop-Install "could not publish $Target; the previous executable was restored. ($($_.Exception.Message))"
        }
    }
    finally {
        if ([System.IO.File]::Exists($staged)) { Remove-Item -LiteralPath $staged -Force }
        # Never delete the backup from a failure path: it may be the only surviving executable.
    }
    Remove-Backups $directory
}

# The receipt binds service registration to the bytes installed at this stable path.
# Like install.sh, prereleases do not receive a managed-installation receipt.
function Publish-InstallReceipt([string]$Target, [string]$Version) {
    if ($Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') { return }
    $directory = Split-Path -Parent $Target
    $receipt = Join-Path $directory $receiptFileName
    $staged = Join-Path $directory ('.sshc.receipt.' + [System.Guid]::NewGuid().ToString('N'))
    try {
        $metadata = [ordered]@{
            schemaVersion = 1
            manager = 'install.ps1'
            repository = $Repository
            version = $Version
            sha256 = (Get-FileHash -LiteralPath $Target -Algorithm SHA256).Hash.ToLowerInvariant()
        }
        $utf8 = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($staged, ($metadata | ConvertTo-Json -Compress), $utf8)
        [System.IO.File]::Move($staged, $receipt)
    }
    finally {
        Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
    }
}

function Test-SamePath([string]$Left, [string]$Right) {
    return [Environment]::ExpandEnvironmentVariables($Left.Trim()).TrimEnd('\') -ieq
        [Environment]::ExpandEnvironmentVariables($Right.Trim()).TrimEnd('\')
}

function Send-EnvironmentChange {
    if (-not ('SshcInstaller.EnvironmentNotification' -as [type])) {
        Add-Type -Namespace SshcInstaller -Name EnvironmentNotification -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Unicode, SetLastError = true)]
public static extern System.IntPtr SendMessageTimeout(System.IntPtr window, uint message, System.UIntPtr wParam, string lParam, uint flags, uint timeout, out System.UIntPtr result);
'@
    }
    # Broadcast to top-level windows; bound each hung recipient instead of blocking installation.
    $broadcastWindow = [IntPtr]0xffff
    $settingChange = 0x001a
    $abortIfHung = 0x0002
    $notificationTimeoutMilliseconds = 5000
    $notificationResult = [UIntPtr]::Zero
    $sent = [SshcInstaller.EnvironmentNotification]::SendMessageTimeout($broadcastWindow, $settingChange,
        [UIntPtr]::Zero, 'Environment', $abortIfHung, $notificationTimeoutMilliseconds, [ref]$notificationResult)
    if ($sent -eq [IntPtr]::Zero) {
        Write-Note 'PATH was saved, but some windows did not acknowledge it; sign out and back in if a new terminal still uses the old PATH'
    }
}

function Update-UserPath([string]$Directory, [switch]$Remove) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
    try {
        $raw = $key.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        if ($null -ne $raw) { $kind = $key.GetValueKind('Path') }
        if ($kind -ne [Microsoft.Win32.RegistryValueKind]::String -and
            $kind -ne [Microsoft.Win32.RegistryValueKind]::ExpandString) { Stop-Install 'user PATH is not a string' }
        $entries = @()
        if ($null -ne $raw -and $raw -ne '') { $entries = @($raw -split ';') }
        $remaining = @($entries | Where-Object { -not (Test-SamePath $_ $Directory) })
        $next = if ($Remove) { $remaining -join ';' } else { (@($Directory) + $remaining) -join ';' }
        # Reinstalling finds the directory already first; leave the value and say nothing.
        if ($next -ceq [string]$raw) { return }
        if ($null -ne $raw -or -not $Remove) { $key.SetValue('Path', $next, $kind) }
    }
    finally { $key.Dispose() }
    Send-EnvironmentChange
    # Do not prepend to the current process: it would hide a machine PATH entry
    # that still takes priority when Windows constructs a fresh user's environment.
    Write-Note 'user PATH updated; open a new terminal to use it'
}

function Show-ShadowedExecutable([string]$Target) {
    $commands = @(Get-Command sshc -All -CommandType Application -ErrorAction SilentlyContinue)
    if ($commands.Count -gt 0 -and -not (Test-SamePath $commands[0].Path $Target)) {
        Write-Output 'sshc: another sshc comes first on your PATH'
        Write-Note "typing sshc runs $($commands[0].Path), not $Target; check your PATH in a new terminal"
    }
    # A machine entry precedes the user PATH even after a fresh login.
    $machinePath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
    foreach ($entry in @($machinePath -split ';')) {
        if ([string]::IsNullOrWhiteSpace($entry)) { continue }
        $candidate = Join-Path ([Environment]::ExpandEnvironmentVariables($entry.Trim())) 'sshc.exe'
        if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) { continue }
        if (-not (Test-SamePath $candidate $Target)) {
            Write-Output 'sshc: another sshc comes first on your PATH'
            Write-Note "the machine PATH selects $candidate before the user PATH entry for $Target"
        }
        break
    }
}

function Confirm-ReleaseAttestation([string[]]$Paths, [string]$Tag) {
    # PATH can hold more than one gh.exe; run the one PowerShell would run, not all of them.
    $github = Get-Command gh -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $github) {
        Write-Note 'GitHub CLI (gh) is not installed, so the release attestation was not checked; only the checksum was'
        return
    }
    & $github.Path attestation verify --help *> $null
    if ($LASTEXITCODE -ne 0) {
        Write-Note 'GitHub CLI (gh) cannot verify attestations (gh 2.49.0 or later is needed), so the release attestation was not checked; only the checksum was'
        return
    }
    & $github.Path auth status --hostname github.com *> $null
    if ($LASTEXITCODE -ne 0) {
        Write-Note 'GitHub CLI (gh) is not signed in to github.com, so the release attestation was not checked; only the checksum was'
        return
    }
    $signedTag = if ($Tag -eq 'latest') { 'v[^/]+' } else { [regex]::Escape($Tag) }
    $signer = '^https://github\.com/' + [regex]::Escape($Repository) + '/\.github/workflows/release\.yml@refs/(tags/' + $signedTag + '|heads/main)$'
    foreach ($path in $Paths) {
        & $github.Path attestation verify $path --repo $Repository --cert-identity-regex $signer --deny-self-hosted-runners
        if ($LASTEXITCODE -ne 0) {
            Stop-Install "gh attestation verify did not confirm that the Release workflow of $Repository signed the download, so nothing was installed: $path"
        }
    }
    Write-Note "attestation matches the Release workflow of $Repository"
}

# Read-StatusFailureKind returns failure.kind from the output of `sshc status --json`, or ''
# when the output carries none. sshc v0.32.0 and earlier print nothing on standard output
# when status fails, and StrictMode stops at a property that is not there.
function Read-StatusFailureKind([string]$StatusText) {
    if ([string]::IsNullOrWhiteSpace($StatusText)) { return '' }
    try { $status = $StatusText | ConvertFrom-Json } catch { return '' }
    if ($null -eq $status) { return '' }
    $failure = $status.PSObject.Properties['failure']
    if ($null -eq $failure -or $null -eq $failure.Value) { return '' }
    $kind = $failure.Value.PSObject.Properties['kind']
    if ($null -eq $kind -or $null -eq $kind.Value) { return '' }
    return [string]$kind.Value
}

# Find-TargetProcess returns a running process whose image is $Target, or $null.
function Find-TargetProcess([string]$Target) {
    $found = $null
    $imageName = [System.IO.Path]::GetFileNameWithoutExtension($Target)
    foreach ($process in @(Get-Process -Name $imageName -ErrorAction SilentlyContinue)) {
        $path = $null
        # Path cannot be read for elevated processes or those of other users; neither is this installation's engine.
        try { $path = $process.Path } catch { }
        if ($null -eq $found -and -not [string]::IsNullOrEmpty($path) -and (Test-SamePath $path $Target)) { $found = $process }
        else { $process.Dispose() }
    }
    if ($null -ne $found) { $null = $found.Handle }
    return $found
}

function Get-InstalledEngine([string]$Target) {
    if (-not (Test-Path -LiteralPath $Target -PathType Leaf)) { return $null }
    $statusText = (& $Target status --json | Out-String)
    $statusExit = $LASTEXITCODE
    if ($statusExit -ne 0) {
        $failureKind = Read-StatusFailureKind $statusText
        if ($failureKind -eq 'engine_not_running') { return $null }
        if ($failureKind -ne '') { Stop-Install "could not inspect the existing engine: $statusText" }
        # An older sshc that cannot report its engine leaves only its processes as evidence.
        # One running from $Target is asked to stop like any other engine of this installation.
        $process = Find-TargetProcess $Target
        # Write-Host, unlike Write-Note, keeps these lines out of the returned value.
        if ($null -eq $process) {
            Write-Host '  the installed sshc did not report its engine, and no process runs from it'
            return $null
        }
        Write-Host "  the installed sshc did not report its engine; stopping pid $($process.Id), which runs from it"
        return $process
    }
    # status proves the handoff first. Its PID is then checked against the actual
    # executable, so installing a second copy does not stop another installation.
    $display = (& $Target status | Out-String)
    if ($LASTEXITCODE -ne 0 -or $display -notmatch '(?m)^engine\s+running \(pid ([0-9]+)\)') {
        Stop-Install 'could not identify the existing engine process'
    }
    $engine = Get-Process -Id ([int]$Matches[1]) -ErrorAction Stop
    if (-not (Test-SamePath $engine.Path $Target)) {
        Write-Warning "the running engine uses $($engine.Path); it was left running"
        $engine.Dispose()
        return $null
    }
    $null = $engine.Handle
    return $engine
}

# Invoke-EngineStop runs `sshc stop` and shows its standard error only when it fails.
# On success stop reminds that the logon task starts the engine again, which does not
# apply here: the installer starts the engine again itself, and uninstalling removes
# the task first. Start-Process keeps standard error away from PowerShell 5.1, which
# would otherwise turn each line into a terminating error under ErrorActionPreference Stop.
function Invoke-EngineStop([string]$Target) {
    $errorFile = [System.IO.Path]::GetTempFileName()
    try {
        $process = Start-Process -FilePath $Target -ArgumentList 'stop' -NoNewWindow -Wait -PassThru -RedirectStandardError $errorFile
        if ($process.ExitCode -ne 0) {
            Get-Content -LiteralPath $errorFile | ForEach-Object { Write-Host $_ }
        }
        return $process.ExitCode
    }
    finally { Remove-Item -LiteralPath $errorFile -Force -ErrorAction SilentlyContinue }
}

function Stop-InstalledEngine([string]$Target, $Engine) {
    if ($null -eq $Engine -or $Engine.HasExited) { return }
    if ((Invoke-EngineStop $Target) -ne 0) {
        Stop-Install "could not stop the existing engine (pid $($Engine.Id)); no files were replaced"
    }
    # stop waits for the engine lock; wait for the OS process too before deleting its image.
    # Include VPN shutdown and process cleanup in the deadline; no fixed sleep guesses at exit.
    $engineExitTimeoutMilliseconds = 45000
    if (-not $Engine.WaitForExit($engineExitTimeoutMilliseconds)) { Stop-Install 'the engine did not exit after stop' }
}

function Get-SshcTask {
    $scheduler = New-Object -ComObject 'Schedule.Service'
    $scheduler.Connect()
    # Task Scheduler names are per machine; per-user software appends the user's SID (as OneDrive does).
    $taskName = 'sshc Startup Task-' + [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    try { return $scheduler.GetFolder('\').GetTask($taskName) }
    catch {
        $exception = $_.Exception
        while ($null -ne $exception.InnerException) { $exception = $exception.InnerException }
        # HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND) means the task is absent.
        $taskNotFound = -2147024894
        if ($exception.HResult -eq $taskNotFound) { return $null }
        throw
    }
}

function Get-EnabledService([string]$Target) {
    # Without this account's task there is no service to resume. sshc before v0.48.0 cannot
    # create the task, and its service status fails on Windows, so it is not asked.
    if ($null -eq (Get-SshcTask)) { return $null }
    $serviceStatus = (& $Target service status | Out-String)
    if ($LASTEXITCODE -ne 0) { Stop-Install "could not inspect the sshc service: $serviceStatus" }
    if ($serviceStatus -match 'service is not installed') { return $null }
    $task = Get-SshcTask
    if ($null -eq $task) { Stop-Install 'the sshc task changed during inspection' }
    if (-not (Test-SamePath $task.Definition.Actions.Item(1).Path $Target)) {
        Stop-Install 'the sshc task belongs to a different installation; it was left unchanged'
    }
    if (-not $task.Enabled) { return $null }
    return [pscustomobject]@{ Task = $task; Definition = $task.Xml }
}

# Test-LastRunFailed says whether the sshc task's last run ended in a failure. Task Scheduler gives up
# after its retries, and the engine stays stopped until something starts the task again: installing a
# release that fixes the failure starts it, as launchd and systemd keep a crashed engine running.
# sshc stop ends the engine with 0, so an engine the user stopped is left stopped.
function Test-LastRunFailed($Service) {
    $result = $Service.Task.LastTaskResult
    # SCHED_S_TASK_HAS_NOT_RUN and SCHED_S_TASK_TERMINATED are not failures of the engine.
    $taskHasNotRun = 0x41303
    $taskTerminated = 0x41306
    return $result -ne 0 -and $result -ne $taskHasNotRun -and $result -ne $taskTerminated
}

function Start-InstalledEngine([string]$Target, $Service) {
    if ($null -eq $Service) {
        & $Target open --no-browser
        if ($LASTEXITCODE -ne 0) { Stop-Install 'installation succeeded, but the engine did not restart; run sshc to retry' }
        Write-Output 'sshc: engine restarted'
        return
    }
    # Task Scheduler updates its state after process exit. Poll the actual state
    # and then the proved status response, rather than assuming a sleep is enough.
    $serviceReadyTimeoutSeconds = 20
    $servicePollMilliseconds = 100
    $deadline = [DateTime]::UtcNow.AddSeconds($serviceReadyTimeoutSeconds)
    # Task Scheduler's TASK_STATE_RUNNING and TASK_STATE_QUEUED values are locale independent.
    $taskRunningState = 4
    $taskQueuedState = 2
    while ($Service.Task.State -eq $taskRunningState -or $Service.Task.State -eq $taskQueuedState) {
        if ([DateTime]::UtcNow -ge $deadline) { Stop-Install 'the sshc task did not finish stopping' }
        Start-Sleep -Milliseconds $servicePollMilliseconds
    }
    if ($Service.Task.Xml -cne $Service.Definition) { Stop-Install 'the sshc task changed during installation; it was not started' }
    $Service.Task.Run($null) | Out-Null
    while ([DateTime]::UtcNow -lt $deadline) {
        $status = (& $Target status --json | Out-String) | ConvertFrom-Json
        if ($LASTEXITCODE -eq 0 -and $status.success) {
            $engine = Get-InstalledEngine $Target
            if ($null -eq $engine) { Stop-Install 'the running engine belongs to another installation' }
            $engine.Dispose()
            Write-Output 'sshc: engine restarted through the sshc task'
            return
        }
        Start-Sleep -Milliseconds $servicePollMilliseconds
    }
    Stop-Install 'installation succeeded, but the sshc task did not become ready; run sshc service status'
}

function Register-Uninstaller([string]$Directory, [string]$Version) {
    $uninstaller = Join-Path $Directory $uninstallerName
    Test-ReparsePoint $uninstaller 'the uninstaller'
    # Store this same implementation locally: uninstall works offline, including irm | iex installs.
    [System.IO.File]::WriteAllText($uninstaller, "& {`n$installerBody`n} -Uninstall`n", (New-Object System.Text.UTF8Encoding($true)))
    $powershell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $command = '"' + $powershell + '" -NoProfile -ExecutionPolicy Bypass -File "' + $uninstaller + '"'
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($uninstallKey)
    try {
        $values = @{
            DisplayName = 'sshc'; DisplayVersion = $Version; Publisher = 'aida0710'
            InstallLocation = $Directory; DisplayIcon = (Join-Path $Directory 'sshc.exe')
            UninstallString = $command; QuietUninstallString = $command
        }
        foreach ($name in $values.Keys) { $key.SetValue($name, $values[$name], [Microsoft.Win32.RegistryValueKind]::String) }
        foreach ($name in @('NoModify', 'NoRepair')) { $key.SetValue($name, 1, [Microsoft.Win32.RegistryValueKind]::DWord) }
    }
    finally { $key.Dispose() }
}

function Remove-AppRegistration([string]$Directory) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($uninstallKey)
    if ($null -eq $key) { return }
    try { $location = [string]$key.GetValue('InstallLocation', '') }
    finally { $key.Dispose() }
    if (-not (Test-SamePath $location $Directory)) {
        Write-Note "Installed apps refers to $location; that registration was left unchanged"
        return
    }
    [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($uninstallKey, $false)
}

function Show-UninstallRemainder([string]$Directory) {
    Write-Note "remaining files in ${Directory}:"
    Get-ChildItem -LiteralPath $Directory -Force | ForEach-Object { Write-Note $_.FullName }
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment')
    try { $path = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) }
    finally { $key.Dispose() }
    $pathRemains = @($path -split ';' | Where-Object { Test-SamePath $_ $Directory }).Count -gt 0
    Write-Note "user PATH entry remains: $pathRemains"
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($uninstallKey)
    if ($null -eq $key) { Write-Note 'Installed apps registration remains: False' }
    else {
        try { Write-Note "Installed apps registration remains: $($key.GetValue('InstallLocation', 'unknown location'))" }
        finally { $key.Dispose() }
    }
    $task = Get-SshcTask
    Write-Note "sshc task remains: $($null -ne $task)"
}

function Remove-Installation([string]$Directory) {
    $target = Join-Path $Directory 'sshc.exe'
    $engine = $null
    $step = 'stop the engine and disable the service'
    try {
        if (Test-Path -LiteralPath $target -PathType Leaf) {
            Test-ReparsePoint $target 'the installed executable'
            $engine = Get-InstalledEngine $target
            $task = Get-SshcTask
            if ($null -ne $task -and -not (Test-SamePath $task.Definition.Actions.Item(1).Path $target)) {
                Stop-Install 'the sshc task belongs to a different installation; it was left unchanged'
            }
            # Disabling the task first also stops an engine it runs, so no stop reminder
            # about the task is shown for a task that is about to be removed.
            if ($null -ne $task) {
                & $target service disable --yes
                if ($LASTEXITCODE -ne 0) { Stop-Install 'could not disable the sshc service' }
            }
            Stop-InstalledEngine $target $engine
        }
        else {
            $task = Get-SshcTask
            if ($null -ne $task -and (Test-SamePath $task.Definition.Actions.Item(1).Path $target)) {
                Stop-Install 'sshc.exe is missing but its task remains; install sshc again before uninstalling'
            }
        }
        $step = 'remove the executable, receipt and backups'
        Remove-Backups $Directory -Required
        foreach ($name in @('sshc.exe', $receiptFileName)) {
            $path = Join-Path $Directory $name
            if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
        }
        $step = 'remove the user PATH entry'
        Update-UserPath $Directory -Remove
        $step = 'remove the Installed apps registration'
        Remove-AppRegistration $Directory
        $step = 'remove the uninstaller'
        $uninstaller = Join-Path $Directory $uninstallerName
        if (Test-Path -LiteralPath $uninstaller) { Remove-Item -LiteralPath $uninstaller -Force }
        Write-Output 'sshc: uninstalled'
    }
    catch {
        Write-Output "sshc: uninstall stopped while trying to $step"
        try { Show-UninstallRemainder $Directory }
        catch { Write-Note "could not inspect all remaining items: $($_.Exception.Message)" }
        Write-Note 'later steps were not performed; resolve the error and run uninstall.ps1 again'
        throw
    }
    finally {
        if ($null -ne $engine) { $engine.Dispose() }
        $profileDirectory = $env:USERPROFILE
        Write-Note "data was kept in $profileDirectory\.ssh\sshc; SSH settings and keys were kept in $profileDirectory\.ssh"
    }
}

$installDirectory = if ($Uninstall) {
    $PSScriptRoot
} elseif (-not [string]::IsNullOrWhiteSpace($env:SSHC_INSTALL_DIR)) {
    [System.IO.Path]::GetFullPath($env:SSHC_INSTALL_DIR)
} else {
    $local = [Environment]::GetFolderPath([System.Environment+SpecialFolder]::LocalApplicationData)
    if ([string]::IsNullOrWhiteSpace($local)) { Stop-Install 'LOCALAPPDATA is unavailable' }
    Join-Path $local 'Programs\sshc'
}
if ($Uninstall) {
    Test-ReparsePoint $installDirectory 'the install directory'
    Remove-Installation $installDirectory
    return
}

$architecture = Get-ReleaseArchitecture
$asset = "sshc-windows-$architecture.exe"
$tag = Get-ReleaseTag
$baseUrl = if ($tag -eq 'latest') {
    "https://github.com/$Repository/releases/latest/download"
} else {
    "https://github.com/$Repository/releases/download/$tag"
}

$target = Join-Path $installDirectory 'sshc.exe'

Write-Output "sshc: installing $asset ($tag)"
Write-Note "into $target"

Test-ReparsePoint $installDirectory 'the install directory'
New-Item -ItemType Directory -Path $installDirectory -Force | Out-Null
Test-ReparsePoint $installDirectory 'the install directory'
Test-ReparsePoint $target 'the existing sshc executable'
if (Test-Path -LiteralPath $target -PathType Container) {
    Stop-Install "$target is a directory"
}

$work = Join-Path ([System.IO.Path]::GetTempPath()) ('sshc-install-' + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$engine = $null
try {
    $download = Join-Path $work $asset
    $checksumFile = Join-Path $work 'checksums.txt'
    Copy-ReleaseFile $asset $download $baseUrl
    Copy-ReleaseFile 'checksums.txt' $checksumFile $baseUrl

    $expected = Read-PublishedChecksum $checksumFile $asset
    $actual = (Get-FileHash -LiteralPath $download -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -cne $expected) {
        Stop-Install "the download does not match its published checksum`n  expected $expected`n  got      $actual"
    }
    Write-Note 'checksum matches'
    Confirm-ReleaseAttestation @($download, $checksumFile) $tag

    $versionLine = (& $download version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $versionLine -notmatch '^sshc (v[^ ]+) windows/(amd64|arm64)$') {
        Stop-Install "the downloaded executable does not identify itself as an sshc Windows release: $versionLine"
    }
    $incomingTag = $Matches[1]
    $incomingArchitecture = $Matches[2]
    if ($incomingArchitecture -cne $architecture) {
        Stop-Install "the downloaded executable reports windows/$incomingArchitecture, expected windows/$architecture"
    }
    if ($tag -ne 'latest' -and $incomingTag -cne $tag) {
        Stop-Install "the downloaded executable reports $incomingTag, expected $tag"
    }

    Remove-Backups $installDirectory
    $service = $null
    $failedService = $null
    # The web engine must save its update result before the detached helper stops it.
    if ($env:SSHC_INSTALL_CALLER -ne 'web-update') {
        $engine = Get-InstalledEngine $target
        if ($null -ne $engine) {
            $service = Get-EnabledService $target
            Stop-InstalledEngine $target $engine
        }
        elseif (Test-Path -LiteralPath $target -PathType Leaf) {
            # Installing may be what repairs a broken executable, so not being able to ask it about the
            # task leaves the task alone instead of stopping the installation.
            try {
                $stoppedService = Get-EnabledService $target
                if ($null -ne $stoppedService -and (Test-LastRunFailed $stoppedService)) { $failedService = $stoppedService }
            }
            catch { Write-Note "the sshc task was not inspected ($($_.Exception.Message)); start it with sshc service install if needed" }
        }
    }

    # Invalidate the previous receipt before replacing the executable. An interrupted
    # installation remains unmanaged instead of retaining a receipt for different bytes.
    $receipt = Join-Path $installDirectory $receiptFileName
    Test-ReparsePoint $receipt 'the install receipt'
    if (Test-Path -LiteralPath $receipt) { Remove-Item -LiteralPath $receipt -Force }
    Publish-Executable $download $target
    $installedLine = (& $target version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $installedLine -cne $versionLine) {
        Stop-Install "the installed executable did not report the verified version"
    }

    Publish-InstallReceipt $target $incomingTag
    Register-Uninstaller $installDirectory $incomingTag

    if ($env:SSHC_ADD_TO_PATH -ne '0') {
        Update-UserPath $installDirectory
    }
    elseif ($env:SSHC_INSTALL_TESTING -ne '1') {
        Write-Note "PATH was not changed; add $installDirectory before running sshc"
    }

    Show-ShadowedExecutable $target
    Write-Output "sshc: installed $installedLine"
    if ($null -ne $engine) { Start-InstalledEngine $target $service }
    elseif ($null -ne $failedService) {
        Write-Note "the sshc task's last run failed (result $($failedService.Task.LastTaskResult)); starting it again"
        Start-InstalledEngine $target $failedService
    }
}
catch {
    Write-Note "installation stopped; inspect $installDirectory for the executable, receipt, uninstaller and backups"
    Write-Note 'if the engine was stopped, run sshc after resolving the error to start it again'
    throw
}
finally {
    if ($null -ne $engine) { $engine.Dispose() }
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
}
