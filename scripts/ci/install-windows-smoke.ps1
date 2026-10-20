# Exercise the actual Windows release in an isolated profile; restore all user registry values.
param(
    [Parameter(Mandatory = $true)][string]$DistDir,
    [Parameter(Mandatory = $true)][string]$ExpectedVersion
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$dist = (Resolve-Path $DistDir).Path
$fixture = Join-Path ([System.IO.Path]::GetTempPath()) ('sshc-installer-' + [System.Guid]::NewGuid())
$release = Join-Path $fixture 'release'
$installed = Join-Path $fixture 'installed with spaces'
$smokeProfile = Join-Path $fixture 'profile'
$shadow = Join-Path $fixture 'shadow'
$tools = Join-Path $fixture 'tools'
$target = Join-Path $installed 'sshc.exe'
$receiptPath = Join-Path $installed '.sshc-install-receipt.json'
$uninstallKey = 'Software\Microsoft\Windows\CurrentVersion\Uninstall\sshc'
$environmentNames = @('PATH', 'USERPROFILE', 'HOME', 'SSHC_INSTALL_TESTING', 'SSHC_TEST_RELEASE_DIR',
    'SSHC_INSTALL_DIR', 'SSHC_ADD_TO_PATH', 'SSHC_VERSION', 'SSHC_INSTALL_CALLER', 'SSHC_SMOKE_GH_LOG', 'SSHC_SMOKE_GH_FAIL',
    'SSHC_SMOKE_OLDER_SSHC_STOP_SIGNAL')
$originalEnvironment = @{}
foreach ($name in $environmentNames) { $originalEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
$environmentKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
$originalPath = $environmentKey.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
$originalPathKind = if ($null -ne $originalPath) { $environmentKey.GetValueKind('Path') } else { $null }
$originalRegistration = @{}
$registration = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($uninstallKey)
$registrationExisted = $null -ne $registration
if ($registrationExisted) {
    foreach ($name in $registration.GetValueNames()) {
        $originalRegistration[$name] = @{
            Value = $registration.GetValue($name, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            Kind = $registration.GetValueKind($name)
        }
    }
    $registration.Dispose()
}

# The task name carries the user's SID. Never touch this account's existing task.
$scheduler = New-Object -ComObject 'Schedule.Service'
$scheduler.Connect()
if (@($scheduler.GetFolder('\').GetTasks(1) | Where-Object { $_.Name -eq ('sshc Startup Task-' + [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value) }).Count -ne 0) {
    $environmentKey.Dispose()
    throw 'run this smoke test in an account without an existing sshc task'
}

function Assert-UserPath([string]$Expected, $Kind) {
    $actual = $environmentKey.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    if ($actual -cne $Expected -or $environmentKey.GetValueKind('Path') -ne $Kind) {
        throw "user PATH changed unexpectedly: $actual"
    }
}

function Invoke-Installer {
    & (Join-Path $root 'install.ps1')
}

# Fixtures are built from Go source so they run as native executables, as the programs they stand in for do.
function Build-GoFixture([string]$Name, [string]$Source, [string]$Output) {
    $sourcePath = Join-Path $fixture "$Name.go"
    [System.IO.File]::WriteAllText($sourcePath, $Source)
    go build -o $Output $sourcePath
    if ($LASTEXITCODE -ne 0) { throw "could not build $Name fixture" }
}

function Assert-OlderSshcReplaced {
    $installedVersion = (& $target version).Trim()
    if ($installedVersion -cne "sshc $ExpectedVersion windows/$goarch") { throw "older sshc was not replaced: $installedVersion" }
    if ([System.IO.File]::ReadAllText($receiptPath) -cne $receiptBefore) { throw 'replacing an older sshc changed the receipt' }
}

# Waiting on a process handle verifies exit without a fixed delay.
$engineExitTimeoutMilliseconds = 45000
$engine = $null
$olderEngine = $null
try {
    New-Item -ItemType Directory -Path $release, $installed, $smokeProfile, $shadow, $tools | Out-Null
    $assets = @(Get-ChildItem -LiteralPath $dist -Filter 'sshc-windows-*.exe' -File)
    if ($assets.Count -ne 2) { throw "expected two Windows release assets, found $($assets.Count)" }
    $checksumLines = foreach ($asset in $assets) {
        Copy-Item -LiteralPath $asset.FullName -Destination $release
        $hash = (Get-FileHash -LiteralPath $asset.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($asset.Name)"
    }
    $checksumPath = Join-Path $release 'checksums.txt'
    [System.IO.File]::WriteAllLines($checksumPath, $checksumLines)
    $goarch = (go env GOARCH).Trim()
    Copy-Item -LiteralPath (Join-Path $release "sshc-windows-$goarch.exe") -Destination (Join-Path $shadow 'sshc.exe')
    # gh is stubbed because CI binaries have not yet received release attestations.
    # Verify both attestation calls and fail closed when verification fails.
    Build-GoFixture 'gh' @'
package main
import ("fmt"; "os"; "strings")
func main() {
    arguments := strings.Join(os.Args[1:], " ")
    if arguments == "attestation verify --help" || strings.HasPrefix(arguments, "auth status") { return }
    log, err := os.OpenFile(os.Getenv("SSHC_SMOKE_GH_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
    if err != nil { panic(err) }
    fmt.Fprintln(log, arguments)
    log.Close()
    if os.Getenv("SSHC_SMOKE_GH_FAIL") == "1" { os.Exit(1) }
}
'@ (Join-Path $tools 'gh.exe')
    # An sshc before v0.42.0 prints nothing on standard output when status cannot reach its
    # engine, and its service status fails on Windows (as v0.32.0 does). Those releases have
    # no stop either; this one has, so the installer can stop linger, its stand-in engine.
    $olderSshc = Join-Path $fixture 'older-sshc.exe'
    Build-GoFixture 'older-sshc' @'
package main
import ("fmt"; "os"; "strings"; "time")
// Longer than the installer's 45 s wait for exit, so only stop can end linger in time.
const lingerLimit = 3 * time.Minute
func main() {
    arguments := strings.Join(os.Args[1:], " ")
    stopSignal := os.Getenv("SSHC_SMOKE_OLDER_SSHC_STOP_SIGNAL")
    switch arguments {
    case "status", "status --json":
        fmt.Fprintln(os.Stderr, `sshc: Get "http://127.0.0.1:49152/cli/status": dial tcp 127.0.0.1:49152: connectex: No connection could be made because the target machine actively refused it.`)
        os.Exit(1)
    case "service status":
        fmt.Fprintln(os.Stderr, "sshc: service is unavailable: managed user services are supported only on Linux and macOS")
        os.Exit(1)
    case "stop":
        if stopSignal == "" { fmt.Fprintln(os.Stderr, "SSHC_SMOKE_OLDER_SSHC_STOP_SIGNAL is not set"); os.Exit(1) }
        if err := os.WriteFile(stopSignal, nil, 0600); err != nil { panic(err) }
    case "linger":
        for deadline := time.Now().Add(lingerLimit); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
            if _, err := os.Stat(stopSignal); err == nil { return }
        }
        os.Exit(1)
    default:
        fmt.Fprintf(os.Stderr, "older sshc fixture: unexpected arguments %q\n", arguments)
        os.Exit(2)
    }
}
'@ $olderSshc
    $env:PATH = "$tools;$shadow;$env:PATH"
    $env:USERPROFILE = $smokeProfile
    $env:HOME = $smokeProfile
    $env:SSHC_INSTALL_TESTING = '1'
    $env:SSHC_TEST_RELEASE_DIR = $release
    $env:SSHC_INSTALL_DIR = $installed
    $env:SSHC_ADD_TO_PATH = '1'
    $env:SSHC_VERSION = $ExpectedVersion
    $env:SSHC_INSTALL_CALLER = ''
    $env:SSHC_SMOKE_GH_LOG = Join-Path $fixture 'gh.log'
    $env:SSHC_SMOKE_GH_FAIL = ''
    $env:SSHC_SMOKE_OLDER_SSHC_STOP_SIGNAL = Join-Path $fixture 'older-sshc-stop-signal'
    $rawPath = '%USERPROFILE%\tools;C:\Windows'
    $environmentKey.SetValue('Path', "$rawPath;$($installed.ToUpperInvariant())\", [Microsoft.Win32.RegistryValueKind]::ExpandString)

    # Invoke-Expression is the irm | iex path: preferences and variables must stay local.
    $Repository = 'caller value'
    $ErrorActionPreference = 'Continue'
    $installerOutput = Invoke-Expression ([System.IO.File]::ReadAllText((Join-Path $root 'install.ps1'))) | Out-String
    if ($ErrorActionPreference -ne 'Continue' -or $Repository -ne 'caller value') { throw 'installer leaked caller variables/preferences' }
    $ErrorActionPreference = 'Stop'
    $strictModeLeaked = $false
    try { $null = $sshcDeliberatelyUndefinedSmokeVariable } catch { $strictModeLeaked = $true }
    if ($strictModeLeaked) { throw 'installer leaked StrictMode' }
    if ($installerOutput -notlike '*another sshc comes first on your PATH*') { throw 'shadowed sshc was not reported' }
    Assert-UserPath "$installed;$rawPath" ([Microsoft.Win32.RegistryValueKind]::ExpandString)
    $attestations = [System.IO.File]::ReadAllText($env:SSHC_SMOKE_GH_LOG)
    if ($attestations -notlike '*checksums.txt*' -or $attestations -notlike "*sshc-windows-$goarch.exe*" -or
        $attestations -notlike '*--deny-self-hosted-runners*') { throw 'release files were not attested' }

    $version = (& $target version).Trim()
    if ($version -cne "sshc $ExpectedVersion windows/$goarch") { throw "unexpected version: $version" }
    $receiptBefore = [System.IO.File]::ReadAllText($receiptPath)
    $receipt = $receiptBefore | ConvertFrom-Json
    $digest = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($receipt.schemaVersion -ne 1 -or $receipt.manager -cne 'install.ps1' -or
        $receipt.repository -cne 'aida0710/sshc' -or $receipt.version -cne $ExpectedVersion -or
        $receipt.sha256 -cne $digest) { throw 'invalid install receipt' }
    $registration = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($uninstallKey)
    try {
        foreach ($name in @('UninstallString', 'QuietUninstallString')) {
            if ($registration.GetValue($name) -notlike '*-NoProfile -ExecutionPolicy Bypass -File*uninstall.ps1"') { throw "invalid $name" }
        }
        if ($registration.GetValue('DisplayName') -cne 'sshc' -or $registration.GetValue('Publisher') -cne 'aida0710' -or
            $registration.GetValue('DisplayVersion') -cne $ExpectedVersion -or $registration.GetValue('InstallLocation') -cne $installed -or
            $registration.GetValue('DisplayIcon') -cne $target -or $registration.GetValue('NoModify') -ne 1 -or
            $registration.GetValue('NoRepair') -ne 1) { throw 'invalid Installed apps registration' }
    }
    finally { $registration.Dispose() }

    Invoke-Installer
    Assert-UserPath "$installed;$rawPath" ([Microsoft.Win32.RegistryValueKind]::ExpandString)
    if ([System.IO.File]::ReadAllText($receiptPath) -cne $receiptBefore) { throw 'reinstall changed the receipt' }
    # Preserve REG_SZ too; do not upgrade it to REG_EXPAND_SZ or expand its raw contents.
    $environmentKey.SetValue('Path', $rawPath, [Microsoft.Win32.RegistryValueKind]::String)
    Invoke-Installer
    Assert-UserPath "$installed;$rawPath" ([Microsoft.Win32.RegistryValueKind]::String)

    & $target open --no-browser
    $handoffPath = Join-Path $smokeProfile '.ssh\sshc\cli'
    $engine = Get-Process -Id ((Get-Content -LiteralPath $handoffPath -Raw | ConvertFrom-Json).pid)
    $null = $engine.Handle
    Invoke-Installer
    if (-not $engine.WaitForExit($engineExitTimeoutMilliseconds)) { throw 'installer did not stop the old engine' }
    $engine.Dispose()
    $engine = Get-Process -Id ((Get-Content -LiteralPath $handoffPath -Raw | ConvertFrom-Json).pid)
    $null = $engine.Handle
    & $target status --json

    # Keep a loaded image alive, as sshc update/the web engine do during replacement.
    $env:SSHC_INSTALL_CALLER = 'web-update'
    Invoke-Installer
    if ($engine.HasExited) { throw 'web installer stopped the calling engine' }
    if (@(Get-ChildItem -LiteralPath $installed -Filter '.sshc.backup.*.exe' -Force).Count -ne 1) { throw 'loaded image was not retained as a backup' }
    & $target stop
    if (-not $engine.WaitForExit($engineExitTimeoutMilliseconds)) { throw 'engine did not exit after stop' }
    $engine.Dispose()
    $engine = $null
    $env:SSHC_INSTALL_CALLER = ''
    Invoke-Installer
    if (@(Get-ChildItem -LiteralPath $installed -Filter '.sshc.backup.*.exe' -Force).Count -ne 0) { throw 'next install did not remove the backup' }

    $shadowExecutable = Join-Path $shadow 'sshc.exe'
    & $shadowExecutable open --no-browser
    $engine = Get-Process -Id ((Get-Content -LiteralPath $handoffPath -Raw | ConvertFrom-Json).pid)
    $null = $engine.Handle
    Invoke-Installer
    if ($engine.HasExited) { throw 'installer stopped another installation' }
    & $shadowExecutable stop
    if (-not $engine.WaitForExit($engineExitTimeoutMilliseconds)) { throw 'shadow engine did not exit' }
    $engine.Dispose()
    $engine = $null

    # An older sshc that cannot report its engine is replaced when nothing runs from it.
    Copy-Item -LiteralPath $olderSshc -Destination $target -Force
    Invoke-Installer
    Assert-OlderSshcReplaced

    # One running from the installed path is stopped first, and the new engine is started in its place.
    Copy-Item -LiteralPath $olderSshc -Destination $target -Force
    Remove-Item -LiteralPath $env:SSHC_SMOKE_OLDER_SSHC_STOP_SIGNAL -Force -ErrorAction SilentlyContinue
    $olderEngine = Start-Process -FilePath $target -ArgumentList 'linger' -NoNewWindow -PassThru
    $null = $olderEngine.Handle
    Invoke-Installer
    # The installer waits for the process to exit before it replaces the image.
    if (-not $olderEngine.HasExited -or $olderEngine.ExitCode -ne 0) { throw 'installer did not stop the older sshc' }
    $olderEngine.Dispose()
    $olderEngine = $null
    Assert-OlderSshcReplaced
    $engine = Get-Process -Id ((Get-Content -LiteralPath $handoffPath -Raw | ConvertFrom-Json).pid)
    $null = $engine.Handle
    & $target stop
    if (-not $engine.WaitForExit($engineExitTimeoutMilliseconds)) { throw 'engine started in place of the older sshc did not exit' }
    $engine.Dispose()
    $engine = $null

    foreach ($failure in @('checksum', 'attestation')) {
        if ($failure -eq 'checksum') {
            $bad = $checksumLines | ForEach-Object {
                if ($_ -like "*  sshc-windows-$goarch.exe") { ('0' * 64) + "  sshc-windows-$goarch.exe" } else { $_ }
            }
            [System.IO.File]::WriteAllLines($checksumPath, $bad)
        } else { $env:SSHC_SMOKE_GH_FAIL = '1' }
        $failed = $false
        try { Invoke-Installer } catch { $failed = $true }
        if (-not $failed) { throw "installer accepted failed $failure verification" }
        if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant() -cne $digest -or
            [System.IO.File]::ReadAllText($receiptPath) -cne $receiptBefore) { throw 'failed verification changed the installation' }
        [System.IO.File]::WriteAllLines($checksumPath, $checksumLines)
        $env:SSHC_SMOKE_GH_FAIL = ''
    }

    $config = Join-Path $smokeProfile '.ssh\config'
    $retained = Join-Path $smokeProfile '.ssh\sshc\retained.txt'
    [System.IO.File]::WriteAllText($config, 'Host retained')
    [System.IO.File]::WriteAllText($retained, 'keep this data')
    & $target open --no-browser
    $engine = Get-Process -Id ((Get-Content -LiteralPath $handoffPath -Raw | ConvertFrom-Json).pid)
    $null = $engine.Handle
    # A file that cannot be deleted must stop uninstall before PATH/registration removal.
    $lockedBackup = Join-Path $installed '.sshc.backup.locked.exe'
    $lockedFile = [System.IO.File]::Open($lockedBackup, [System.IO.FileMode]::Create, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
    try {
        $failed = $false
        try { & (Join-Path $installed 'uninstall.ps1') } catch { $failed = $true }
        if (-not $failed -or -not (Test-Path -LiteralPath $target) -or -not (Test-Path -LiteralPath $receiptPath)) { throw 'uninstall continued after a file deletion failed' }
        Assert-UserPath "$installed;$rawPath" ([Microsoft.Win32.RegistryValueKind]::String)
        $registration = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($uninstallKey)
        if ($null -eq $registration) { throw 'failed uninstall removed app registration' }
        $registration.Dispose()
    }
    finally { $lockedFile.Dispose() }
    $windowsPowerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    & $windowsPowerShell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $installed 'uninstall.ps1')
    if ($LASTEXITCODE -ne 0) { throw 'Windows PowerShell uninstall failed' }
    if (-not $engine.WaitForExit($engineExitTimeoutMilliseconds)) { throw 'uninstall did not stop the engine' }
    $engine.Dispose()
    $engine = $null
    if (@(Get-ChildItem -LiteralPath $installed -Force).Count -ne 0) { throw 'uninstall left installer files' }
    Assert-UserPath $rawPath ([Microsoft.Win32.RegistryValueKind]::String)
    $registration = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($uninstallKey)
    if ($null -ne $registration) { $registration.Dispose(); throw 'uninstall left the app registration' }
    if ([System.IO.File]::ReadAllText($config) -cne 'Host retained' -or
        [System.IO.File]::ReadAllText($retained) -cne 'keep this data') { throw 'uninstall removed user data' }
    Write-Output 'install-windows-smoke: ok'
}
finally {
    try {
        # linger would end by itself, but not before the fixture below is removed.
        if ($null -ne $olderEngine -and -not $olderEngine.HasExited) {
            $olderEngine.Kill()
            $null = $olderEngine.WaitForExit($engineExitTimeoutMilliseconds)
        }
        if ($null -ne $engine -and -not $engine.HasExited -and (Test-Path -LiteralPath $target)) {
            & $target stop
            if (-not $engine.WaitForExit($engineExitTimeoutMilliseconds)) { throw 'smoke engine did not exit during cleanup' }
        }
    }
    finally {
        if ($null -eq $originalPath) { $environmentKey.DeleteValue('Path', $false) }
        else { $environmentKey.SetValue('Path', $originalPath, $originalPathKind) }
        $environmentKey.Dispose()
        [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($uninstallKey, $false)
        if ($registrationExisted) {
            $registration = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($uninstallKey)
            try { foreach ($name in $originalRegistration.Keys) { $registration.SetValue($name, $originalRegistration[$name].Value, $originalRegistration[$name].Kind) } }
            finally { $registration.Dispose() }
        }
        foreach ($name in $environmentNames) { [Environment]::SetEnvironmentVariable($name, $originalEnvironment[$name], 'Process') }
        if ('SshcInstaller.EnvironmentNotification' -as [type]) {
            $notificationResult = [UIntPtr]::Zero
            [SshcInstaller.EnvironmentNotification]::SendMessageTimeout([IntPtr]0xffff, 0x001a, [UIntPtr]::Zero, 'Environment', 0x0002, 5000, [ref]$notificationResult) | Out-Null
        }
        Remove-Item -LiteralPath $fixture -Recurse -Force -ErrorAction SilentlyContinue
    }
}
