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
$home_ = Join-Path ([System.IO.Path]::GetTempPath()) ("sshc-smoke-" + [System.Guid]::NewGuid())
$sshDir = Join-Path $home_ ".ssh"
New-Item -ItemType Directory -Path $sshDir -Force | Out-Null
$env:USERPROFILE = $home_
$env:HOME = $home_
$config = "Host smoke-info`n  HostName 192.0.2.10`n  User smoke-user`n  Port 22`n"
[System.IO.File]::WriteAllText((Join-Path $sshDir "config"), $config, [System.Text.UTF8Encoding]::new($false))

$engine = $null

try {
    # ② info が engine なしで実効接続先を安定した JSON として返すことを確認する。
    $info = (& $binary info smoke-info --json | ConvertFrom-Json)
    if ($info.schemaVersion -ne 1 -or $info.alias -ne "smoke-info" -or `
        $info.destination.hostName -ne "192.0.2.10" -or `
        $info.destination.user -ne "smoke-user" -or $info.destination.port -ne "22") {
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
    $log = Join-Path $home_ "engine.log"
    $engine = Start-Process -FilePath $binary -ArgumentList "engine" -PassThru -NoNewWindow `
        -RedirectStandardOutput $log -RedirectStandardError "$log.err"
    $handoff = Join-Path $home_ ".ssh\sshc\cli"
    $ready = $false
    foreach ($attempt in 1..100) {
        if (Test-Path $handoff) { $ready = $true; break }
        Start-Sleep -Milliseconds 200
    }
    if (-not $ready) {
        Get-Content $log, "$log.err" -ErrorAction SilentlyContinue | Out-Host
        throw "the engine never published a handoff"
    }

    $status = (& $binary status | Out-String)
    Write-Host "  $(($status -split "`n" | ForEach-Object { $_.Trim() }) -join '; ')"
    if ($status -notlike "*running (pid*") {
        throw "status did not report a running engine: $status"
    }

    # ⑤ 埋め込み UI を HTTP 経由で取得できることを確認する。
    $entrance = (& $binary open).Trim()
    $page = (Invoke-WebRequest -Uri $entrance -TimeoutSec 10 -UseBasicParsing).Content
    if ($page -notlike '*<div id="root">*') {
        throw "the entrance did not return the app shell"
    }
    Write-Host "  the bundled UI answered"
}
finally {
    if ($null -ne $engine -and -not $engine.HasExited) { Stop-Process -Id $engine.Id -Force }
    Remove-Item -Recurse -Force $home_ -ErrorAction SilentlyContinue
}

Write-Host "cli-smoke: ok"
