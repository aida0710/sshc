param(
    [Parameter(Mandatory = $true)]
    [AllowEmptyString()]
    [string] $Artifact,

    [Parameter(Mandatory = $true)]
    [AllowEmptyString()]
    [string] $OS,

    [Parameter(Mandatory = $true)]
    [AllowEmptyString()]
    [string] $Architecture
)

$ErrorActionPreference = "Stop"

if ($OS -notin @("darwin", "linux", "windows")) {
    [Console]::Error.WriteLine("artifact OS rejected")
    exit 2
}

if ($Architecture -notin @("amd64", "arm64")) {
    [Console]::Error.WriteLine("artifact architecture rejected")
    exit 2
}

$suffix = if ($OS -eq "windows") { ".exe" } else { "" }
$expected = "sshc-$OS-$Architecture$suffix"
if ([string]::IsNullOrWhiteSpace($Artifact)) {
    [Console]::Error.WriteLine("artifact path rejected")
    exit 2
}

try {
    $name = Split-Path -LiteralPath $Artifact -Leaf
} catch {
    [Console]::Error.WriteLine("artifact path rejected")
    exit 2
}

if ($name -cne $expected) {
    [Console]::Error.WriteLine("artifact name rejected")
    exit 1
}

Write-Output "verified artifact name: $name"
