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
  if (-not $Condition) { throw "package-smoke: $Because" }
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

# **入れる前の PATH を覚えておく。** アンインストールが元へ戻したかどうかは、
# ここと比べる以外に言いようがない。
$pathBefore = UserPathEntries
# 近い名前の項目を先に置く。**アンインストーラがこれを巻き添えにしないこと**
# が、前方一致で消していないことの証拠になる。
$neighbour = Join-Path $WorkRoot 'sshc-tools'
New-Item -ItemType Directory -Force -Path $neighbour | Out-Null
$decoy = "$cliDirectory-tools"
Set-ItemProperty -Path 'HKCU:\Environment' -Name Path `
  -Value (@($pathBefore + $decoy) -join ';')

try {
  Write-Output "installing $Installer"
  $install = Start-Process -FilePath $Installer -ArgumentList '/S' -Wait -PassThru
  Assert ($install.ExitCode -eq 0) "the installer exited with 0 (got $($install.ExitCode))"

  # ---- layout ----
  Assert (Test-Path -LiteralPath $shell) "the shell is at $shell"
  Assert (Test-Path -LiteralPath $cli) "the CLI is at $cli"

  # ---- user PATH ----
  $entries = UserPathEntries
  $mine = @($entries | Where-Object { $_ -eq $cliDirectory })
  Assert ($mine.Count -eq 1) "the CLI directory is on the user PATH exactly once"
  Assert ($entries -contains $decoy) "the neighbouring entry survived the install"

  # **入れ直しても増えない。** 同じ項目が二つ並ぶ PATH は、何度か入れ直した
  # 利用者の環境で起きる壊れ方である。
  $again = Start-Process -FilePath $Installer -ArgumentList '/S' -Wait -PassThru
  Assert ($again.ExitCode -eq 0) "the second install exited with 0"
  $mine = @(UserPathEntries | Where-Object { $_ -eq $cliDirectory })
  Assert ($mine.Count -eq 1) "reinstalling did not add the entry a second time"

  # ---- launcher registration ----
  $registered = (Get-ItemProperty -Path $launcherKey -Name Executable).Executable
  Assert ($registered -eq $shell) "the launcher key names the shell"

  # ---- the CLI answers ----
  $version = & $cli --help 2>&1
  Assert ($LASTEXITCODE -eq 0) "the installed CLI runs"
  Assert (($version -join "`n") -match 'headless') "its help names the headless owner"

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
    Assert (Test-Path -LiteralPath $handoff) "the engine published a handoff"

    # **中身は読まない。** ここにはワンタイムの資格情報が入っている。
    foreach ($private in @($stateDirectory, $handoff)) {
      & (Join-Path $scriptDirectory 'assert-private-acl.ps1') -Path $private
    }

    $status = & $cli vault status 2>&1
    Assert ($LASTEXITCODE -eq 0) "vault status answered"
    Assert (($status -join "`n") -match 'engine:\s*headless') "the owner is headless"

    # **端末が持っているあいだ、外殻は engine を横取りしない。**
    $bare = Start-Process -FilePath $cli -ArgumentList @() -Wait -PassThru `
      -RedirectStandardError (Join-Path $WorkRoot 'bare.err')
    Assert ($bare.ExitCode -ne 0) "bare sshc refused to displace the headless owner"
    $refusal = Get-Content -Raw (Join-Path $WorkRoot 'bare.err')
    Assert ($refusal -match 'headless') "the refusal names the headless owner"
  } finally {
    if (-not $engine.HasExited) { Stop-Process -Id $engine.Id -Force }
  }

  # ---- ConPTY descendants do not outlive the engine ----
  $survivors = @(Get-CimInstance Win32_Process -Filter "ParentProcessId=$($engine.Id)")
  Assert ($survivors.Count -eq 0) "no console outlived the engine"
} finally {
  Write-Output 'uninstalling'
  $uninstaller = Join-Path $installDirectory 'Uninstall sshc.exe'
  if (Test-Path -LiteralPath $uninstaller) {
    $remove = Start-Process -FilePath $uninstaller -ArgumentList '/S' -Wait -PassThru
    Assert ($remove.ExitCode -eq 0) "the uninstaller exited with 0"
  }
}

# ---- what is left ----
Assert (-not (Test-Path -LiteralPath $shell)) "the shell is gone"
Assert (-not (Test-Path -LiteralPath $cli)) "the CLI is gone"

$entries = UserPathEntries
Assert (-not ($entries -contains $cliDirectory)) "the CLI directory left the user PATH"
# **近い名前は残る。** 前方一致で消していたら、これも消えている。
Assert ($entries -contains $decoy) "the neighbouring entry survived the uninstall"
foreach ($entry in $pathBefore) {
  Assert ($entries -contains $entry) "the pre-existing entry $entry survived"
}

$stillRegistered = Get-ItemProperty -Path $launcherKey -Name Executable -ErrorAction SilentlyContinue
Assert ($null -eq $stillRegistered) "the launcher registration is gone"

# 置いた囮を片づける。**利用者の PATH を、来たときの姿へ戻す。**
Set-ItemProperty -Path 'HKCU:\Environment' -Name Path -Value ($pathBefore -join ';')

Write-Output "package-smoke: $Architecture passed"
