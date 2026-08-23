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

# 存在しないパスや `[` を含む名前も検証できるよう、ファイルシステムへアクセスせず
# 文字列から末尾の要素を取り出す。
$separator = $Artifact.LastIndexOfAny([char[]]@([char]'/', [char]'\'))
$name = if ($separator -ge 0) { $Artifact.Substring($separator + 1) } else { $Artifact }
if ([string]::IsNullOrEmpty($name)) {
    [Console]::Error.WriteLine("artifact path rejected")
    exit 2
}

if ($name -cne $expected) {
    [Console]::Error.WriteLine("artifact name rejected")
    exit 1
}

Write-Output "verified artifact name: $name"
