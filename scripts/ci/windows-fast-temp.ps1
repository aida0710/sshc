# Windows の runner の一時フォルダ（TEMP）を、OS のディスク（C）から作業用のディスクへ移す。
#
# runner の TEMP はプロファイルの下、つまり C にある。C はディスクへ流す書き込みが遅く、
# sshc の非公開の書き込み（FILE_WRITE_THROUGH で作り、FlushFileBuffers で流す）1 回が、
# C では 16.75ms、作業用のディスク（RUNNER_TEMP がある D）では 1.33ms だった
# （2026-10-27 の計測）。storage・secret・application などのテストは、この書き込みを
# 何千回も行うので、Windows の go test の時間の大半がこの差だった。
#
# テストの中には、TEMP がプロファイルの下にある前提で、受け継いだ ACL を確かめるものがある。
# 利用者の TEMP はいつもプロファイルの下なので、その前提は正しい。そこで新しいフォルダには、
# プロファイルの TEMP の ACL を、継承を止めたうえで明示の ACE として写す。
$ErrorActionPreference = 'Stop'

$profileTemp = [System.IO.Path]::GetTempPath()
$fastTemp = Join-Path $env:RUNNER_TEMP 'profile-temp'
New-Item -ItemType Directory -Force -Path $fastTemp | Out-Null

$acl = Get-Acl -LiteralPath $profileTemp
$acl.SetAccessRuleProtection($true, $true)
Set-Acl -LiteralPath $fastTemp -AclObject $acl

"TMP=$fastTemp" | Out-File -FilePath $env:GITHUB_ENV -Append -Encoding utf8
"TEMP=$fastTemp" | Out-File -FilePath $env:GITHUB_ENV -Append -Encoding utf8
"moved TEMP from $profileTemp to $fastTemp"
