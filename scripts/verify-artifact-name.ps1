param(
    [Parameter(Mandatory = $true)]
    [string] $Artifact,

    [Parameter(Mandatory = $true)]
    [ValidateSet("darwin", "linux", "windows")]
    [string] $OS,

    [Parameter(Mandatory = $true)]
    [ValidateSet("amd64", "arm64")]
    [string] $Architecture
)

$ErrorActionPreference = "Stop"

$suffix = if ($OS -eq "windows") { ".exe" } else { "" }
$expected = "sshc-$OS-$Architecture$suffix"
$name = Split-Path -Leaf $Artifact

if ($name -cne $expected) {
    Write-Error "artifact name mismatch: expected $expected, got $name"
    exit 1
}

Write-Output "verified artifact name: $name"
