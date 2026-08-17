; sshc の per-user インストーラが足す二つのこと。
;
; **管理者権限を求めない。** HKEY_LOCAL_MACHINE にも、machine の PATH にも
; 触れない。ここが書くのはこの利用者の枝だけであり、書いたものは
; アンインストールで、書いたときと同じ厳密さで消える。

!include "LogicLib.nsh"
!include "WordFunc.nsh"
!include "WinMessages.nsh"

; CLI が入る場所。**Electron の隣ではなく、resources の下である。**
; %LOCALAPPDATA%\Programs\sshc\sshc.exe が外殻で、
; %LOCALAPPDATA%\Programs\sshc\resources\cli\sshc.exe が端末から呼ばれる方。
!define SSHC_CLI_SUBDIR "resources\cli"

; 起動登録。**internal/platform/windowsregistry と対である。**
; 綴りを二箇所に持つが、片方は NSIS でもう片方は Go なので共有できない。
; どちらかを変えるときは、必ず両方を変えること。
!define SSHC_LAUNCHER_KEY "Software\sshc\Desktop"
!define SSHC_LAUNCHER_VALUE "Executable"

; 環境変数が変わったことを、いま動いているシェルへ知らせる。これを送らないと、
; インストール後に開いていた端末では、再起動するまで sshc が見つからない。
!define SSHC_ENVIRONMENT_CHANGED \
  "SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 'STR:Environment' /TIMEOUT=5000"

; sshcPathHasEntry は、利用者の PATH に候補がちょうど一件あるかを $R0 に返す
; （"1" か "0"）。
;
; **前方一致では見ない。** `C:\a\sshc` を探しているときに `C:\a\sshc-old` が
; 当たると、追加されるべきものが追加されない。区切りで割って、項目ごとに
; 突き合わせる——StrCmp は大文字小文字を区別しないので、Windows のパスの
; 綴り違いはそのまま同じものとして扱われる。
!macro SSHC_PATH_HAS_ENTRY UN
Function ${UN}sshcPathHasEntry
  Exch $R1 ; 探す項目
  Push $R2 ; いまの PATH
  Push $R3 ; 項目の数
  Push $R4 ; 走査位置
  Push $R5 ; 取り出した項目

  ReadRegStr $R2 HKCU "Environment" "Path"
  StrCpy $R0 "0"
  ${If} $R2 != ""
    ${WordFind} "$R2" ";" "#" $R3
    StrCpy $R4 1
    ${Do}
      ${If} $R4 > $R3
        ${Break}
      ${EndIf}
      ${WordFind} "$R2" ";" "+$R4" $R5
      ${If} $R5 == $R1
        StrCpy $R0 "1"
        ${Break}
      ${EndIf}
      IntOp $R4 $R4 + 1
    ${Loop}
  ${EndIf}

  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
FunctionEnd
!macroend

!insertmacro SSHC_PATH_HAS_ENTRY ""
!insertmacro SSHC_PATH_HAS_ENTRY "un."

; sshcRemovePathEntry は、利用者の PATH から候補と厳密に一致する項目だけを
; 取り除いた文字列を $R0 に返す。
;
; **近い名前を巻き添えにしない。** 部分文字列で切り出すと、`C:\a\sshc` を
; 消すつもりで `C:\a\sshc-tools` の頭を削り、利用者の PATH を壊す。
!macro SSHC_REMOVE_PATH_ENTRY UN
Function ${UN}sshcRemovePathEntry
  Exch $R1 ; 取り除く項目
  Push $R2 ; いまの PATH
  Push $R3 ; 項目の数
  Push $R4 ; 走査位置
  Push $R5 ; 取り出した項目

  ReadRegStr $R2 HKCU "Environment" "Path"
  StrCpy $R0 ""
  ${If} $R2 != ""
    ${WordFind} "$R2" ";" "#" $R3
    StrCpy $R4 1
    ${Do}
      ${If} $R4 > $R3
        ${Break}
      ${EndIf}
      ${WordFind} "$R2" ";" "+$R4" $R5
      ${If} $R5 != $R1
      ${AndIf} $R5 != ""
        ${If} $R0 == ""
          StrCpy $R0 "$R5"
        ${Else}
          StrCpy $R0 "$R0;$R5"
        ${EndIf}
      ${EndIf}
      IntOp $R4 $R4 + 1
    ${Loop}
  ${EndIf}

  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
FunctionEnd
!macroend

; 取り除くのはアンインストーラだけである。インストール側にも同じ関数を置くと、
; 呼ばれないコードが installer に残る。
!insertmacro SSHC_REMOVE_PATH_ENTRY "un."

!macro customInstall
  ; **同じ項目を二度足さない。** 入れ直すたびに PATH が伸びる installer は、
  ; 何度か入れ直した利用者の環境を壊す。
  StrCpy $R9 "$INSTDIR\${SSHC_CLI_SUBDIR}"
  Push "$R9"
  Call sshcPathHasEntry
  ${If} $R0 == "0"
    ReadRegStr $R8 HKCU "Environment" "Path"
    ${If} $R8 == ""
      WriteRegExpandStr HKCU "Environment" "Path" "$R9"
    ${Else}
      WriteRegExpandStr HKCU "Environment" "Path" "$R8;$R9"
    ${EndIf}
    ${SSHC_ENVIRONMENT_CHANGED}
  ${EndIf}

  ; 端末から `sshc` と打った人のために、外殻の居場所を残す。
  ; 読むのは cmd/sshc/launch_windows.go である。
  WriteRegStr HKCU "${SSHC_LAUNCHER_KEY}" "${SSHC_LAUNCHER_VALUE}" "$INSTDIR\${PRODUCT_FILENAME}.exe"
!macroend

!macro customUnInstall
  ; **自分が足した項目だけを消す。** 利用者が自分で足した他の項目も、名前の
  ; 近い別のものも、そのまま残す。
  StrCpy $R9 "$INSTDIR\${SSHC_CLI_SUBDIR}"
  Push "$R9"
  Call un.sshcPathHasEntry
  ${If} $R0 == "1"
    Push "$R9"
    Call un.sshcRemovePathEntry
    ${If} $R0 == ""
      DeleteRegValue HKCU "Environment" "Path"
    ${Else}
      WriteRegExpandStr HKCU "Environment" "Path" "$R0"
    ${EndIf}
    ${SSHC_ENVIRONMENT_CHANGED}
  ${EndIf}

  ; **自分が書いた登録だけを消す。** 二つの版が入っている機械では、別の場所を
  ; 指しているものは、残っている方のインストールのものである。
  ReadRegStr $R8 HKCU "${SSHC_LAUNCHER_KEY}" "${SSHC_LAUNCHER_VALUE}"
  ${If} $R8 == "$INSTDIR\${PRODUCT_FILENAME}.exe"
    DeleteRegValue HKCU "${SSHC_LAUNCHER_KEY}" "${SSHC_LAUNCHER_VALUE}"
    DeleteRegKey /ifempty HKCU "${SSHC_LAUNCHER_KEY}"
  ${EndIf}
!macroend
