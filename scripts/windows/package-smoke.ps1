<#
.SYNOPSIS
Installs the per-user NSIS package, checks what it did, and uninstalls it.

.DESCRIPTION
Everything here is a claim the installer makes that nothing else can check.
makensis proves the script compiles; the Go suite proves the CLI reads what
the installer writes. Only running it proves the installer writes it.

The order is deliberate: install, then layout, then PATH, then the launcher
registration, then the application itself, then uninstall, then what is left
behind. A later step failing means the earlier one really did happen, which
is what makes the cleanup check meaningful.

**Nothing secret is printed.** The handoff holds a one-time credential; this
reports whether a file is there and who may read it, never its body.
#>
param(
  [Parameter(Mandatory = $true)][string]$Installer,
  [Parameter(Mandatory = $true)][string]$Architecture,
  [Parameter(Mandatory = $true)][string]$WorkRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$scriptDirectory = Split-Path -Parent $PSCommandPath
$installDirectory = Join-Path $env:LOCALAPPDATA 'Programs\sshc'
$cliDirectory = Join-Path $installDirectory 'resources\cli'
$cli = Join-Path $cliDirectory 'sshc.exe'
$shell = Join-Path $installDirectory 'sshc.exe'
$launcherKey = 'HKCU:\Software\sshc\Desktop'
$stateDirectory = Join-Path $env:USERPROFILE '.ssh\sshc'

function Assert([bool]$Condition, [string]$Because) {
  # **落ちたときの文言が、事実の記述に読めてはならない。** "the shell is at X"
  # は成功したときの言い方であり、それを失敗の理由として投げると、読んだ人は
  # 何が起きたのか分からない。期待していたことだと分かる形で言う。
  if (-not $Condition) { throw "package-smoke: expected $Because" }
  Write-Output "ok: $Because"
}

function UserPathEntries() {
  $value = (Get-ItemProperty -Path 'HKCU:\Environment' -Name Path -ErrorAction SilentlyContinue).Path
  if ($null -eq $value -or $value -eq '') { return @() }
  return @($value -split ';' | Where-Object { $_ -ne '' })
}

if (-not (Test-Path -LiteralPath $Installer)) {
  throw "package-smoke: no installer at $Installer"
}
if ($Architecture -notin @('x64', 'arm64')) {
  throw "package-smoke: unsupported architecture $Architecture"
}
New-Item -ItemType Directory -Force -Path $WorkRoot | Out-Null

# **入っていない機械で確かめる。** NSIS は前回の InstallLocation を覚えていて、
# 入れ直しはそこへ戻る——名前を変えても既存のインストールは動かない。既に
# 入っているところで走らせると、この検査が報告するのは「新しい利用者が受け取る
# もの」ではなく「更新した結果」になる。実機でまさにそれが起き、置き場が
# 変わったことに気づくのが一手遅れた。
if (Test-Path -LiteralPath $installDirectory) {
  throw "package-smoke: sshc is already installed at $installDirectory; uninstall it first, or this measures an upgrade"
}

# **入れる前の PATH を覚えておく。** アンインストールが元へ戻したかどうかは、
# ここと比べる以外に言いようがない。
$pathBefore = UserPathEntries
$restoredPath = $false
# 近い名前の項目を先に置く。**アンインストーラがこれを巻き添えにしないこと**
# が、前方一致で消していないことの証拠になる。
$neighbour = Join-Path $WorkRoot 'sshc-tools'
New-Item -ItemType Directory -Force -Path $neighbour | Out-Null
$decoy = "$cliDirectory-tools"
Set-ItemProperty -Path 'HKCU:\Environment' -Name Path `
  -Value (@($pathBefore + $decoy) -join ';')

# **置いたものは、どう終わっても片づける。** 途中で落ちた検査が利用者の PATH に
# 囮を残していくのは、確かめに来ただけのものが環境を壊すということである。
# 最初にこれを走らせた実機で、実際にそれをやってしまった。
try {
  Write-Output "installing $Installer"
  $install = Start-Process -FilePath $Installer -ArgumentList '/S' -Wait -PassThru
  Assert ($install.ExitCode -eq 0) "the installer to exit with 0 (got $($install.ExitCode))"

  # ---- layout ----
  Assert (Test-Path -LiteralPath $shell) "the shell at $shell"
  Assert (Test-Path -LiteralPath $cli) "the CLI at $cli"

  # ---- user PATH ----
  $entries = UserPathEntries
  $mine = @($entries | Where-Object { $_ -eq $cliDirectory })
  Assert ($mine.Count -eq 1) "the CLI directory on the user PATH exactly once"
  Assert ($entries -contains $decoy) "the neighbouring entry to survive the install"

  # **入れ直しても増えない。** 同じ項目が二つ並ぶ PATH は、何度か入れ直した
  # 利用者の環境で起きる壊れ方である。
  $again = Start-Process -FilePath $Installer -ArgumentList '/S' -Wait -PassThru
  Assert ($again.ExitCode -eq 0) "the second install to exit with 0"
  $mine = @(UserPathEntries | Where-Object { $_ -eq $cliDirectory })
  Assert ($mine.Count -eq 1) "reinstalling not to add the entry a second time"

  # ---- launcher registration ----
  $registered = (Get-ItemProperty -Path $launcherKey -Name Executable).Executable
  Assert ($registered -eq $shell) "the launcher key to name the shell"

  # ---- the CLI answers ----
  $version = & $cli --help 2>&1
  Assert ($LASTEXITCODE -eq 0) "the installed CLI to run"
  Assert (($version -join "`n") -match 'headless') "its help to name the headless owner"

  # ---- a headless engine comes up and its state is private ----
  $engine = Start-Process -FilePath $cli -ArgumentList 'headless' -PassThru `
    -RedirectStandardOutput (Join-Path $WorkRoot 'engine.out') `
    -RedirectStandardError (Join-Path $WorkRoot 'engine.err')
  try {
    $handoff = Join-Path $stateDirectory 'cli'
    $deadline = (Get-Date).AddSeconds(60)
    while (-not (Test-Path -LiteralPath $handoff) -and (Get-Date) -lt $deadline) {
      Start-Sleep -Milliseconds 200
    }
    Assert (Test-Path -LiteralPath $handoff) "the engine to publish a handoff"

    # **中身は読まない。** ここにはワンタイムの資格情報が入っている。
    foreach ($private in @($stateDirectory, $handoff)) {
      & (Join-Path $scriptDirectory 'assert-private-acl.ps1') -Path $private
    }

    $status = & $cli vault status 2>&1
    Assert ($LASTEXITCODE -eq 0) "vault status to answer"
    Assert (($status -join "`n") -match 'engine:\s*headless') "the owner to be headless"

    # **端末が持っているあいだ、外殻は engine を横取りしない。**
    # **引数無しは -ArgumentList を書かない。** 空の配列は拒まれる
    # （"The argument is null, empty, ..."）——渡すものが無いことを、
    # 空のものを渡すことで表せない。
    $bare = Start-Process -FilePath $cli -Wait -PassThru `
      -RedirectStandardError (Join-Path $WorkRoot 'bare.err')
    Assert ($bare.ExitCode -ne 0) "bare sshc to refuse to displace the headless owner"
    $refusal = Get-Content -Raw (Join-Path $WorkRoot 'bare.err')
    Assert ($refusal -match 'headless') "the refusal to name the headless owner"
  } finally {
    if (-not $engine.HasExited) { Stop-Process -Id $engine.Id -Force }
  }

  # ---- ConPTY descendants do not outlive the engine ----
  $survivors = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$($engine.Id)")
  Assert ($survivors.Count -eq 0) "no console to outlive the engine"
  # ---- uninstall ----
  # **アンインストールも検査の一部である。** 後片付けの finally に置くと、
  # その終了コードを誰も見ない。
  Write-Output 'uninstalling'
  $uninstaller = Join-Path $installDirectory 'Uninstall sshc.exe'
  Assert (Test-Path -LiteralPath $uninstaller) "an uninstaller at $uninstaller"
  $remove = Start-Process -FilePath $uninstaller -ArgumentList '/S' -Wait -PassThru
  Assert ($remove.ExitCode -eq 0) "the uninstaller to exit with 0 (got $($remove.ExitCode))"

  # ---- what is left ----
  Assert (-not (Test-Path -LiteralPath $shell)) "the shell to be gone"
  Assert (-not (Test-Path -LiteralPath $cli)) "the CLI to be gone"

  $entries = UserPathEntries
  Assert (-not ($entries -contains $cliDirectory)) "the CLI directory to leave the user PATH"
  # **近い名前は残る。** 前方一致で消していたら、これも消えている。
  Assert ($entries -contains $decoy) "the neighbouring entry to survive the uninstall"
  foreach ($entry in $pathBefore) {
    Assert ($entries -contains $entry) "the pre-existing entry $entry to survive"
  }

  $stillRegistered = Get-ItemProperty -Path $launcherKey -Name Executable -ErrorAction SilentlyContinue
  Assert ($null -eq $stillRegistered) "the launcher registration to be gone"
} finally {
  # **どう終わっても、来たときの PATH へ戻す。** 途中で落ちた検査が囮を残して
  # いくのは、確かめに来ただけのものが利用者の環境を壊すということである。
  Set-ItemProperty -Path 'HKCU:\Environment' -Name Path -Value ($pathBefore -join ';')
  $restoredPath = $true

  # 入ったままなら、消してから帰る。ここでの失敗は報告するだけで、検査の
  # 結果を上書きしない——本当の理由は既に投げられている。
  $leftover = Join-Path $installDirectory 'Uninstall sshc.exe'
  if (Test-Path -LiteralPath $leftover) {
    $sweep = Start-Process -FilePath $leftover -ArgumentList '/S' -Wait -PassThru
    if ($sweep.ExitCode -ne 0) {
      Write-Output "package-smoke: could not remove the leftover install ($($sweep.ExitCode))"
    }
  }
}

Assert $restoredPath "the user PATH to be restored"
Write-Output "package-smoke: $Architecture passed"
