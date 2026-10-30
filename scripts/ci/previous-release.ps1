# 前のリリース（GitHub の最新の安定版）のうち、この Windows のランナーで動く sshc を落とし、
# 同じリリースの checksums.txt と照らしてから、そのパスを出す。前のリリースから上げる
# 試験（cmd/sshc/upgrade_test.go）が使う。Unix は previous-release.sh。
param(
    [Parameter(Mandatory = $true)][string]$Directory
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

$repository = if ($env:GITHUB_REPOSITORY) { $env:GITHUB_REPOSITORY } else { 'aida0710/sshc' }
$tag = (gh release view --repo $repository --json tagName --jq .tagName)
$goarch = (go env GOARCH)
$asset = "sshc-windows-$goarch.exe"

New-Item -ItemType Directory -Force -Path $Directory | Out-Null
gh release download $tag --repo $repository --dir $Directory --pattern $asset --pattern checksums.txt --clobber | Out-Host
$expected = $null
foreach ($line in Get-Content (Join-Path $Directory 'checksums.txt')) {
    $fields = $line -split '\s+'
    if ($fields.Count -ge 2 -and $fields[1] -ceq $asset) { $expected = $fields[0] }
}
if (-not $expected) { throw "checksums.txt of $tag has no line for $asset" }
$binary = Join-Path $Directory $asset
$actual = (Get-FileHash -Algorithm SHA256 -Path $binary).Hash.ToLowerInvariant()
if ($actual -cne $expected) { throw "$asset of $tag does not match checksums.txt" }
Write-Host "previous release: $tag ($asset)"
Write-Output $binary
