; sshc の per-user インストーラが足す二つのこと。
;
; **管理者権限を求めない。** HKEY_LOCAL_MACHINE にも、machine の PATH にも
; 触れない。ここが書くのはこの利用者の枝だけであり、書いたものは
; アンインストールで、書いたときと同じ厳密さで消える。
;
; **Function を定義しない。** electron-builder は installer と uninstaller を
; 別々にコンパイルし、makensis を警告=エラーで走らせる。`un.` を付けた関数を
; include の時点で定義すると、WriteUninstaller を持たない側のコンパイルで
; 「uninstaller のコードがあるのに uninstaller が作られない」(6020) になる。
; どちらの macro も一度しか挿入されないので、その場に展開して困ることは無い。

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

!macro customInstall
  ; **同じ項目を二度足さない。** 入れ直すたびに PATH が伸びる installer は、
  ; 何度か入れ直した利用者の環境を壊す。
  ;
  ; **前方一致では見ない。** `C:\a\sshc` を探しているときに `C:\a\sshc-old` が
  ; 当たると、追加されるべきものが追加されない。区切りで割って、項目ごとに
  ; 突き合わせる——StrCmp は大文字小文字を区別しないので、Windows のパスの
  ; 綴り違いはそのまま同じものとして扱われる。
  Push $R0 ; 足したい項目
  Push $R1 ; いまの PATH
  Push $R2 ; 項目の数
  Push $R3 ; 走査位置
  Push $R4 ; 取り出した項目
  Push $R5 ; 見つかったか

  StrCpy $R0 "$INSTDIR\${SSHC_CLI_SUBDIR}"
  ReadRegStr $R1 HKCU "Environment" "Path"
  StrCpy $R5 "0"
  ${If} $R1 != ""
    ${WordFind} "$R1" ";" "#" $R2
    StrCpy $R3 1
    ${Do}
      ${If} $R3 > $R2
        ${Break}
      ${EndIf}
      ${WordFind} "$R1" ";" "+$R3" $R4
      ${If} $R4 == $R0
        StrCpy $R5 "1"
        ${Break}
      ${EndIf}
      IntOp $R3 $R3 + 1
    ${Loop}
  ${EndIf}

  ${If} $R5 == "0"
    ${If} $R1 == ""
      WriteRegExpandStr HKCU "Environment" "Path" "$R0"
    ${Else}
      WriteRegExpandStr HKCU "Environment" "Path" "$R1;$R0"
    ${EndIf}
    ${SSHC_ENVIRONMENT_CHANGED}
  ${EndIf}

  ; 端末から `sshc` と打った人のために、外殻の居場所を残す。
  ; 読むのは cmd/sshc/launch_windows.go である。
  WriteRegStr HKCU "${SSHC_LAUNCHER_KEY}" "${SSHC_LAUNCHER_VALUE}" "$INSTDIR\${PRODUCT_FILENAME}.exe"

  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Pop $R0
!macroend

!macro customUnInstall
  ; **自分が足した項目だけを消す。** 利用者が自分で足した他の項目も、名前の
  ; 近い別のものも、そのまま残す。部分文字列で切り出すと、`C:\a\sshc` を消す
  ; つもりで `C:\a\sshc-tools` の頭を削り、利用者の PATH を壊す。
  Push $R0 ; 取り除く項目
  Push $R1 ; いまの PATH
  Push $R2 ; 項目の数
  Push $R3 ; 走査位置
  Push $R4 ; 取り出した項目
  Push $R5 ; 組み直した PATH
  Push $R6 ; 取り除いたか

  StrCpy $R0 "$INSTDIR\${SSHC_CLI_SUBDIR}"
  ReadRegStr $R1 HKCU "Environment" "Path"
  StrCpy $R5 ""
  StrCpy $R6 "0"
  ${If} $R1 != ""
    ${WordFind} "$R1" ";" "#" $R2
    StrCpy $R3 1
    ${Do}
      ${If} $R3 > $R2
        ${Break}
      ${EndIf}
      ${WordFind} "$R1" ";" "+$R3" $R4
      ${If} $R4 == $R0
        StrCpy $R6 "1"
      ${ElseIf} $R4 != ""
        ${If} $R5 == ""
          StrCpy $R5 "$R4"
        ${Else}
          StrCpy $R5 "$R5;$R4"
        ${EndIf}
      ${EndIf}
      IntOp $R3 $R3 + 1
    ${Loop}
  ${EndIf}

  ${If} $R6 == "1"
    ${If} $R5 == ""
      DeleteRegValue HKCU "Environment" "Path"
    ${Else}
      WriteRegExpandStr HKCU "Environment" "Path" "$R5"
    ${EndIf}
    ${SSHC_ENVIRONMENT_CHANGED}
  ${EndIf}

  ; **自分が書いた登録だけを消す。** 二つの版が入っている機械では、別の場所を
  ; 指しているものは、残っている方のインストールのものである。
  ReadRegStr $R4 HKCU "${SSHC_LAUNCHER_KEY}" "${SSHC_LAUNCHER_VALUE}"
  ${If} $R4 == "$INSTDIR\${PRODUCT_FILENAME}.exe"
    DeleteRegValue HKCU "${SSHC_LAUNCHER_KEY}" "${SSHC_LAUNCHER_VALUE}"
    DeleteRegKey /ifempty HKCU "${SSHC_LAUNCHER_KEY}"
  ${EndIf}

  Pop $R6
  Pop $R5
  Pop $R4
  Pop $R3
  Pop $R2
  Pop $R1
  Pop $R0
!macroend
