# Install the sshc Windows CLI without administrator privileges.
#
#   powershell -NoProfile -ExecutionPolicy Bypass -Command `
#     "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"
#
# Environment variables:
#   SSHC_VERSION      release to install (default: latest)
#   SSHC_INSTALL_DIR  destination (default: %LOCALAPPDATA%\Programs\sshc)
#   SSHC_ADD_TO_PATH  set to 0 to leave the user PATH unchanged (default: 1)

[CmdletBinding()]
param()

Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

# Windows PowerShell 5.1 may otherwise negotiate a protocol GitHub rejects.
[Net.ServicePointManager]::SecurityProtocol =
    [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

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
    try {
        Invoke-WebRequest -Uri $uri -OutFile $Destination -UseBasicParsing -TimeoutSec 180 `
            -Headers @{ 'User-Agent' = 'sshc-install.ps1' }
    }
    catch {
        Stop-Install "could not download $uri ($($_.Exception.Message))"
    }
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

function Publish-Executable([string]$Source, [string]$Target) {
    $directory = Split-Path -Parent $Target
    $staged = Join-Path $directory ('.sshc.install.' + [System.Guid]::NewGuid().ToString('N') + '.exe')
    $backup = Join-Path $directory ('.sshc.backup.' + [System.Guid]::NewGuid().ToString('N') + '.exe')
    try {
        [System.IO.File]::Copy($Source, $staged, $false)
        if ([System.IO.File]::Exists($Target)) {
            try {
                [System.IO.File]::Replace($staged, $Target, $backup, $true)
            }
            catch {
                Stop-Install "could not replace $Target. Stop the running sshc engine and try again. The previous executable was left unchanged. ($($_.Exception.Message))"
            }
            Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
        }
        else {
            [System.IO.File]::Move($staged, $Target)
        }
    }
    finally {
        Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $backup -Force -ErrorAction SilentlyContinue
    }
}

function Add-UserPath([string]$Directory) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = if ([string]::IsNullOrWhiteSpace($userPath)) { @() } else { @($userPath -split ';') }
    $wanted = [Environment]::ExpandEnvironmentVariables($Directory).TrimEnd('\')
    $present = $false
    foreach ($entry in $entries) {
        $expanded = [Environment]::ExpandEnvironmentVariables($entry.Trim()).TrimEnd('\')
        if ($expanded -ieq $wanted) { $present = $true; break }
    }
    if (-not $present) {
        $next = @($entries | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }) + $Directory
        [Environment]::SetEnvironmentVariable('Path', ($next -join ';'), 'User')
        Write-Note "added $Directory to your user PATH; open a new terminal to use it"
    }
    if (-not (($env:Path -split ';') | Where-Object {
        [Environment]::ExpandEnvironmentVariables($_.Trim()).TrimEnd('\') -ieq $wanted
    })) {
        $env:Path = "$Directory;$env:Path"
    }
}

$architecture = Get-ReleaseArchitecture
$asset = "sshc-windows-$architecture.exe"
$tag = Get-ReleaseTag
$baseUrl = if ($tag -eq 'latest') {
    "https://github.com/$Repository/releases/latest/download"
} else {
    "https://github.com/$Repository/releases/download/$tag"
}

$installDirectory = if (-not [string]::IsNullOrWhiteSpace($env:SSHC_INSTALL_DIR)) {
    [System.IO.Path]::GetFullPath($env:SSHC_INSTALL_DIR)
} else {
    $local = [Environment]::GetFolderPath([System.Environment+SpecialFolder]::LocalApplicationData)
    if ([string]::IsNullOrWhiteSpace($local)) { Stop-Install 'LOCALAPPDATA is unavailable' }
    Join-Path $local 'Programs\sshc'
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

    Publish-Executable $download $target
    $installedLine = (& $target version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $installedLine -cne $versionLine) {
        Stop-Install "the installed executable did not report the verified version"
    }

    if ($env:SSHC_ADD_TO_PATH -ne '0') {
        Add-UserPath $installDirectory
    }
    elseif ($env:SSHC_INSTALL_TESTING -ne '1') {
        Write-Note "PATH was not changed; add $installDirectory before running sshc"
    }

    Write-Output "sshc: installed $installedLine"
}
finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
