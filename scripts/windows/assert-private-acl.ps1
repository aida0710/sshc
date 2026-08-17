<#
.SYNOPSIS
Fails unless the given path can be read only by its owner, SYSTEM and Administrators.

.DESCRIPTION
Windows does not answer "who can read this" with mode bits. It answers with a
DACL, so that is what this reads. A path is accepted when every access rule
that grants read belongs to the owner, to SYSTEM, or to Administrators, and
when the DACL is not inherited from somewhere that could widen it later.

Nothing about the file's contents is printed. These paths hold a one-time
credential and a sealed vault; a failure says which trustee was allowed, and
no more.
#>
param(
  [Parameter(Mandatory = $true)][string]$Path
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath $Path)) {
  throw "assert-private-acl: $Path does not exist"
}

$acl = Get-Acl -LiteralPath $Path
$owner = $acl.Owner

# SID で照合する。**表示名で照合しない** — 表示名は言語ごとに変わり、
# 日本語の Windows では "Administrators" は "Administrators" のままだが、
# "SYSTEM" は "SYSTEM" でも、どちらも保証されているものではない。
$allowedSids = @(
  'S-1-5-18' # SYSTEM
  'S-1-5-32-544' # Administrators
)
$ownerSid = (New-Object System.Security.Principal.NTAccount($owner)).Translate(
  [System.Security.Principal.SecurityIdentifier]).Value

$offenders = @()
foreach ($rule in $acl.Access) {
  if ($rule.AccessControlType -ne [System.Security.AccessControl.AccessControlType]::Allow) {
    # 拒否はここでは無視する。読みを広げるのは許可だけである。
    continue
  }
  $readMask = [System.Security.AccessControl.FileSystemRights]::Read `
    -bor [System.Security.AccessControl.FileSystemRights]::ReadData `
    -bor [System.Security.AccessControl.FileSystemRights]::ReadAndExecute `
    -bor [System.Security.AccessControl.FileSystemRights]::FullControl
  if (($rule.FileSystemRights -band $readMask) -eq 0) {
    continue
  }
  $sid = $rule.IdentityReference.Translate(
    [System.Security.Principal.SecurityIdentifier]).Value
  if ($sid -eq $ownerSid -or $allowedSids -contains $sid) {
    continue
  }
  $offenders += $sid
}

if ($offenders.Count -gt 0) {
  throw "assert-private-acl: $Path grants read to $($offenders -join ', ')"
}

# **継承を許したままにしない。** 親のほうを緩めた誰かが、あとからここを
# 広げられる。書いた側は継承を切っているはずで、切れていないなら、その
# 経路を通っていない。
if (-not $acl.AreAccessRulesProtected) {
  throw "assert-private-acl: $Path still inherits its access rules"
}

Write-Output "private: $Path"
