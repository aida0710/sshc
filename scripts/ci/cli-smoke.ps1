# リリース対象の Windows バイナリを実行して検証する。
#
# Makefile の recipe は POSIX シェルを前提とするため、Windows では使わない。
# native command の失敗を検出するよう PowerShell を設定する。
param(
    [Parameter(Mandatory = $true)][string]$DistDir,
    [Parameter(Mandatory = $true)][string]$ExpectedVersion
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

$goos = (go env GOOS)
$goarch = (go env GOARCH)
$binary = Join-Path $DistDir "sshc-$goos-$goarch.exe"

if (-not (Test-Path $binary)) {
    Get-ChildItem $DistDir | Out-Host
    throw "no runnable artifact for this machine at $binary"
}
Write-Host "cli-smoke: $binary"

# ① バージョン、OS、アーキテクチャを確認する。
$versionLine = (& $binary version)
Write-Host "  $versionLine"
$want = "sshc $ExpectedVersion $goos/$goarch"
if ($versionLine -ne $want) {
    throw "version line is $versionLine, want `"$want`""
}

# 実環境を変更せず、info と engine が同じ一時 HOME だけを見るようにする。
$smokeHome = Join-Path ([System.IO.Path]::GetTempPath()) ("sshc-smoke-" + [System.Guid]::NewGuid())
$sshDir = Join-Path $smokeHome ".ssh"
New-Item -ItemType Directory -Path $sshDir -Force | Out-Null
$originalUserProfile = $env:USERPROFILE
$originalHome = $env:HOME
$env:USERPROFILE = $smokeHome
$env:HOME = $smokeHome
$config = "Host smoke-info`n  HostName 192.0.2.10`n  User smoke-user`n  Port 22`n"
[System.IO.File]::WriteAllText((Join-Path $sshDir "config"), $config, [System.Text.UTF8Encoding]::new($false))

# The engine has already released its lock; allow time for the OS to finish process exit.
$engineExitTimeoutMilliseconds = 10000
$engine = $null

try {
    # ② info が engine なしで実効接続先を安定した JSON として返すことを確認する。
    $info = (& $binary info smoke-info --json | ConvertFrom-Json)
    if ($info.schemaVersion -ne 1 -or $info.success -ne $true -or `
        $info.result.schemaVersion -ne 1 -or $info.result.alias -ne "smoke-info" -or `
        $info.result.destination.hostName -ne "192.0.2.10" -or `
        $info.result.destination.user -ne "smoke-user" -or $info.result.destination.port -ne "22") {
        throw "info returned an unexpected target: $($info | ConvertTo-Json -Compress)"
    }
    Write-Host "  info resolved smoke-info without an engine"

    # ③ engine が停止中の場合に復旧手順が表示されることを確認する。
    $PSNativeCommandUseErrorActionPreference = $false
    $absent = (& $binary status 2>&1 | Out-String)
    $absentCode = $LASTEXITCODE
    $PSNativeCommandUseErrorActionPreference = $true
    if ($absentCode -eq 0) { throw "status succeeded with no engine" }
    if ($absent -notlike "*sshc engine*") {
        throw "the no-engine message does not say what to do: $absent"
    }
    Write-Host "  no engine: $($absent.Trim())"

    # ④ 起動、応答、正常終了を確認する。
    $entrance = (& $binary open --no-browser).Trim()
    $handoff = Join-Path $smokeHome ".ssh\sshc\cli"
    if (-not (Test-Path $handoff)) { throw "the engine never published a handoff" }
    $enginePID = (Get-Content $handoff -Raw | ConvertFrom-Json).pid
    $engine = Get-Process -Id $enginePID
    if ($engine.MainWindowHandle -ne [IntPtr]::Zero) { throw "the background engine has a visible window" }

    $status = (& $binary status | Out-String)
    Write-Host "  $(($status -split "`n" | ForEach-Object { $_.Trim() }) -join '; ')"
    if ($status -notlike "*running (pid*") {
        throw "status did not report a running engine: $status"
    }

    # ⑤ 埋め込み UI を HTTP 経由で取得できることを確認する。
    $page = (Invoke-WebRequest -Uri $entrance -TimeoutSec 10 -UseBasicParsing).Content
    if ($page -notlike '*<div id="root">*') {
        throw "the entrance did not return the app shell"
    }
    Write-Host "  the bundled UI answered"
    & $binary stop
    if (-not $engine.WaitForExit($engineExitTimeoutMilliseconds)) { throw "the engine did not exit after stop" }
    & $binary stop
}
finally {
    try {
        & $binary stop
        Remove-Item -Recurse -Force $smokeHome -ErrorAction SilentlyContinue
    }
    finally {
        $env:USERPROFILE = $originalUserProfile
        $env:HOME = $originalHome
    }
}

Write-Host "cli-smoke: ok"
