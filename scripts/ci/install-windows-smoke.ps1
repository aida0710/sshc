# Exercise install.ps1 with the actual binaries produced by the release job.
param(
    [Parameter(Mandatory = $true)][string]$DistDir,
    [Parameter(Mandatory = $true)][string]$ExpectedVersion
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

$root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$dist = (Resolve-Path $DistDir).Path
$fixture = Join-Path ([System.IO.Path]::GetTempPath()) ('sshc-installer-fixture-' + [System.Guid]::NewGuid())
$installed = Join-Path ([System.IO.Path]::GetTempPath()) ('sshc-installer-target-' + [System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $fixture, $installed | Out-Null

try {
    $assets = @(Get-ChildItem -LiteralPath $dist -Filter 'sshc-windows-*.exe' -File)
    if ($assets.Count -ne 2) { throw "expected two Windows release assets, found $($assets.Count)" }
    foreach ($asset in $assets) { Copy-Item -LiteralPath $asset.FullName -Destination $fixture }
    $checksumLines = foreach ($asset in $assets) {
        $hash = (Get-FileHash -LiteralPath $asset.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($asset.Name)"
    }
    [System.IO.File]::WriteAllLines((Join-Path $fixture 'checksums.txt'), $checksumLines)

    $env:SSHC_INSTALL_TESTING = '1'
    $env:SSHC_TEST_RELEASE_DIR = $fixture
    $env:SSHC_INSTALL_DIR = $installed
    $env:SSHC_ADD_TO_PATH = '0'
    $env:SSHC_VERSION = $ExpectedVersion

    & (Join-Path $root 'install.ps1')
    $target = Join-Path $installed 'sshc.exe'
    $version = (& $target version).Trim()
    $goarch = (go env GOARCH).Trim()
    $want = "sshc $ExpectedVersion windows/$goarch"
    if ($version -cne $want) { throw "installed version is $version, want $want" }

    $receiptPath = Join-Path $installed '.sshc-install-receipt.json'
    $receiptBefore = [System.IO.File]::ReadAllText($receiptPath)
    $receipt = $receiptBefore | ConvertFrom-Json
    $installedDigest = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($receipt.schemaVersion -ne 1 -or $receipt.manager -cne 'install.ps1' -or
        $receipt.repository -cne 'aida0710/sshc' -or $receipt.version -cne $ExpectedVersion -or
        $receipt.sha256 -cne $installedDigest) { throw 'installer receipt does not identify the installed executable' }

    # Reinstallation exercises replacement and receipt invalidation/publication.
    & (Join-Path $root 'install.ps1')
    if ([System.IO.File]::ReadAllText($receiptPath) -cne $receiptBefore) { throw 'reinstall changed the verified receipt' }

    # A bad checksum must not change the existing executable.
    $before = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash
    $checksumPath = Join-Path $fixture 'checksums.txt'
    $lines = [System.IO.File]::ReadAllLines($checksumPath)
    $nativeAsset = "sshc-windows-$goarch.exe"
    $bad = $lines | ForEach-Object {
        if ($_ -like "*  $nativeAsset") { ('0' * 64) + "  $nativeAsset" } else { $_ }
    }
    [System.IO.File]::WriteAllLines($checksumPath, $bad)
    $failed = $false
    try { & (Join-Path $root 'install.ps1') } catch { $failed = $true }
    if (-not $failed) { throw 'installer accepted a mismatched checksum' }
    $after = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash
    if ($after -cne $before) { throw 'failed installation changed the existing executable' }
    if ([System.IO.File]::ReadAllText($receiptPath) -cne $receiptBefore) { throw 'failed installation changed the receipt' }

    Get-ChildItem -LiteralPath $installed -Force | ForEach-Object {
        if ($_.Name -like '.sshc.install.*' -or $_.Name -like '.sshc.backup.*' -or $_.Name -like '.sshc.receipt.*') {
            throw "installer left staging file $($_.Name)"
        }
    }
    Write-Output 'install-windows-smoke: ok'
}
finally {
    Remove-Item -LiteralPath $fixture, $installed -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item Env:SSHC_INSTALL_TESTING, Env:SSHC_TEST_RELEASE_DIR, Env:SSHC_INSTALL_DIR, `
        Env:SSHC_ADD_TO_PATH, Env:SSHC_VERSION -ErrorAction SilentlyContinue
}
