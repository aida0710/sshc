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

# 末尾の要素は、文字列として取り出す。
#
# **Split-Path は通さない。** あれはファイルシステムのプロバイダを通るので、
# `[` を含むパスに対する振る舞いがプラットフォームごとに違い、Linux の pwsh では
# 実在しない相対パスで例外になる。ここで確かめたいのは名前そのものであって、
# ディスク上の何かではない。
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
