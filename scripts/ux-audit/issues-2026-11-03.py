"""UI/UX 点検（2026-11-03）の所見を GitHub のイシューにする。

使い方:
  python3 scripts/ux-audit/issues-2026-11-03.py images <issue-assets の作業ツリー>  画像を写す（名前を付け直す）
  python3 scripts/ux-audit/issues-2026-11-03.py bodies <出力先> [まとめの番号]        本文を書き出す（検査用）
  python3 scripts/ux-audit/issues-2026-11-03.py post <出力先>                         まとめ → 各項目 → まとめの一覧の順に登録する
画像は issue-assets ブランチの ux-audit-2026-11-03/ に置き、raw の URL で貼る。
"""
import json
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SHOTS = ROOT / "artifacts" / "ux-audit" / "2026-11-03"
IMAGE_DIR = "ux-audit-2026-11-03"
IMAGE_BASE = f"https://raw.githubusercontent.com/aida0710/sshc/issue-assets/{IMAGE_DIR}"
REPOSITORY = "aida0710/sshc"
SOURCE = "main の `8dc4a7e96`（v0.59.9）"
SUMMARY_TITLE = "UI/UX点検（2026-11-03）: 24件の改善案のまとめ"

# 貼る画像: 公開の名前 → 撮った画像。ユーザー名を含むパスが写っていないものだけを選んだ。
IMAGES = {
    "ux35-windows-powershell.png": "windows/05-local-powershell.png",
    "ux35-windows-ssh.png": "windows/07-ssh-session.png",
    "ux36-sftp-host-key.png": "flows/F6-00-host-key-not-asked.png",
    "ux37-config-no-diagnostic.png": "rough/desktop--config.png",
    "ux37-typo-host.png": "rough/phone--connections.png",
    "ux38-sync-first.png": "flows/F8A-03-round-2.png",
    "ux38-sync-second.png": "flows/F8B-04-final.png",
    "ux39-auth-message.png": "view/f1b.png",
    "ux39-raw-errors.png": "flows/F4-02-down-15s.png",
    "ux40-light-keybar-phone.png": "clean/phone--theme-light-settings-keybar.png",
    "ux40-light-keybar-desktop.png": "clean/desktop--theme-light-settings-keybar.png",
    "ux42-gave-up.png": "flows/F4-04-gone-180s.png",
    "ux42-sidebar-failures.png": "rough/phone--drawer-open.png",
    "ux44-home-empty.png": "flows/F1-02-home-empty.png",
    "ux44-connections-empty.png": "flows/F1-02b-connections-empty.png",
    "ux44-empty-states.png": "view/e1.png",
    "ux45-tabby-quick-connect.png": "references/tabby/05-quick-connect-typed.png",
    "ux45-sshc-create.png": "flows/F1-04-create-filled.png",
    "ux46-tabby-password.png": "references/tabby/07-auth.png",
    "ux46-sshc-password.png": "flows/F3-password-only-02-after-host-key.png",
    "ux47-split-disabled.png": "flows/F5-01-more.png",
    "ux48-tabby-reconnect.png": "references/tabby/09-server-stopped.png",
    "ux50-shortcuts-phone.png": "clean/phone--settings-shortcuts.png",
    "ux50-keybar-phone.png": "clean/phone--settings-keybar.png",
    "ux50-vault.png": "clean/desktop--settings-vault.png",
    "ux51-create-no-jump.png": "flows/F2-01-create-form.png",
    "ux52-sftp-list.png": "clean/phone--sftp-remote-photos.png",
    "ux52-sftp-preview.png": "clean/phone--sftp-preview-image.png",
    "ux52-terminal-preview.png": "clean/phone--terminal-preview-image.png",
    "ux53-quick-command.png": "flows/F7-04-quick-commands.png",
    "ux54-sync-twice.png": "view/f8.png",
    "ux54-connections.png": "clean/desktop--connections.png",
    "ux54-session-name.png": "clean/phone--terminal-ssh-session.png",
    "ux55-dates.png": "clean/phone--new-session-dialog.png",
    "ux56-many-groups.png": "rough/phone--home.png",
    "ux56-history.png": "clean/phone--history.png",
}


def image(name: str, caption: str) -> str:
    return f'<img src="{IMAGE_BASE}/{name}" alt="{caption}" width="720">\n\n{caption}'


# 各項目: 番号、種類（不具合・デザイン・機能・文言）、優先度、規模、ラベル、タイトル、本文。
ITEMS = [
    {
        "id": "UX-35", "kind": "不具合", "priority": "高", "size": "小", "label": "bug",
        "title": "Windows: ターミナルの既定の字体がBIZ UDGothic（古いWindowsではＭＳ ゴシック）に落ち、\\が¥で出て罫線がずれる",
        "body": f"""## 何が起きるか

Windowsのブラウザでターミナルを開くと、既定の「システムの等幅フォントを使用」がBIZ UDGothicで描かれます。次の3つが起きます。

- `\\`が`¥`の形で出ます（`C:¥Users¥sshcadmin`）。
- 字の幅が狭く行が詰まり、`drwx------`のような記号の並びがつながって見えます。
- 罫線（`┌─┬─┐ │あ│B│`）の位置がずれます。

利用者がふだん使っている環境（Windows・Chrome）です。

{image("ux35-windows-powershell.png", "ローカルのPowerShell。パスの区切りが¥になっている")}

{image("ux35-windows-ssh.png", "SSHのセッション。行が詰まり、罫線の位置がずれる（左の上の帯は開発用のビルドの案内で、この件とは関係ない）")}

## 環境

Windows 11 Enterprise Evaluation（日本語）、Chrome 156、sshc は{SOURCE}をWindows向けに組み立てたもの。r100の仮想マシンで確かめました。

## 原因

`web/src/terminal/fonts.ts:3-4`の`defaultStack`は`ui-monospace, SFMono-Regular, "SF Mono", Menlo, "Roboto Mono", "Droid Sans Mono", monospace`です。Windowsの字体（Cascadia Mono・Consolas）が入っていません。WindowsのChromeは`ui-monospace`に対応していないので、最後の`monospace`、つまり日本語のChromeの既定の等幅に落ちます。実際に描かれた字体がBIZ UDGothicであることは、CDPの`CSS.getPlatformFontsForNode`で確かめました。この仮想マシンでは`document.fonts.check`でCascadia Mono・Consolasも使えます。

## 直し方の案

- `defaultStack`の`monospace`の前に`"Cascadia Mono", Consolas`を足します（macOS・Linuxの並びは変えない）。
- 同梱のJetBrains Monoを既定にする案もあります。字の形が全部のOSでそろいますが、今の既定を変えるので判断が要ります。

## 確かめ方

WindowsのChromeでターミナルの行を`CSS.getPlatformFontsForNode`で調べ、Cascadia MonoかConsolasになっていること。`\\`が`\\`の形で出ること。
""",
    },
    {
        "id": "UX-36", "kind": "不具合", "priority": "高", "size": "中", "label": "bug",
        "title": "SFTP: ホスト鍵をまだ知らない接続先を開くと、鍵の確認が出ずに英語のエラーで止まる",
        "body": f"""## 何が起きるか

`known_hosts`にまだ無い接続先をSFTPの画面で開くと、ホスト鍵を確かめる画面が出ません。「接続できませんでした。」［再試行］と、英語の`sshc: ssh: handshake failed: this host is not in known_hosts`で止まります。［再試行］を押しても進みません。先にターミナルでつないで鍵を受け入れると、SFTPでも開けます。

{image("ux36-sftp-host-key.png", "SFTPで初めての接続先を開いたところ")}

## 手順

1. 初めての接続先（`known_hosts`に無い）を作る。
2. SFTP →［接続先を選択］→ その接続先を選ぶ。
3. 期待: ターミナルと同じホスト鍵の枠（指紋と［やめる］［接続する］）が出て、受け入れると一覧が開く。
4. 実際: 上の画面で止まる。

## ほかのアプリ

TabbyとTermiusは、SFTPでもターミナルと同じ確認を出します。

## 直し方の案

ターミナルのホスト鍵の枠（`POST /api/v1/terminal/sessions/{{id}}/host-key`、#610・#614）と同じ問いを、SFTPの接続にも出します。少なくとも、文を日本語にして「ターミナルで一度つないで鍵を受け入れてください」と案内します。

## 確かめ方

`known_hosts`が空のHOMEで、SFTPから初めての接続先を開き、鍵の確認のあと一覧が出ること。
""",
    },
    {
        "id": "UX-37", "kind": "不具合", "priority": "高", "size": "中", "label": "bug",
        "title": "SSH Config: OpenSSHが読めない行（キーワードの打ち間違い）を警告しない。OpenSSHはその1行で設定全体を読まなくなる",
        "body": f"""## 何が起きるか

`HostNmae 192.0.2.77`のように知らないキーワードの行があると、OpenSSHは次の文を出して止まり、ほかの接続先も含めて`ssh`が使えなくなります。

```
config: line 5: Bad configuration option: hostnmae
config: terminating, 1 bad configuration options
```

sshcは自前の読み込みで知らないキーワードを読み飛ばすので、sshcからは接続できます。SSH Configの見出しは「診断0件」、左下は「Includeの問題はありません」のままです。Connectionsでは、その接続先が`deploy@typo-host`のように接続名をホスト名として出します。

{image("ux37-config-no-diagnostic.png", "打ち間違いの行があるのに診断0件")}

{image("ux37-typo-host.png", "Connectionsでは接続名がそのままホスト名として出る")}

## 環境

r100のOpenSSH 10.2p1で、`ssh -G`が止まることを確かめました。sshcは{SOURCE}です。

## 原因

知らせの種類（`internal/application/notice.go`）に、OpenSSHが受け付けないキーワードの行を表すものがありません。`internal/effective`はOpenSSHのキーワードの一覧を持っていません。

## 直し方の案

OpenSSHのキーワードの一覧（`ssh_config(5)`の全キーワード）を持ち、知らない行を`unknown_directive`のような知らせにします。SSH Configの診断とHomeの「確認が必要」に、ファイルと行を出します。sshcはOpenSSHの設定ファイルをそのまま使う方針なので、OpenSSHが止まる設定を「問題なし」と出すのは重い食い違いです。

## 確かめ方

`resolve_differential_test.go`と同じ形で、知らないキーワードの行を足した設定に知らせが出ること。`ssh -G`が止まる例と一致すること。
""",
    },
    {
        "id": "UX-38", "kind": "不具合", "priority": "高", "size": "中", "label": "bug",
        "title": "同期: 1台目が送る前に2台目を設定すると、2台目はキーを聞かずに別の暗号化キーを作る",
        "body": f"""## 何が起きるか

1台目で［接続を確認］→［確認して保存］を押すと、共通の暗号化キーと「いまコピーして、他のマシンに入力してください。この画面を離れると二度と表示されません。」が出ます。ただし、この時点ではバケットに何も書きません。

1台目が［今すぐ同期］を押す前に2台目で同じバケットを設定すると、2台目はキーを聞きません。別のキーを作り、自分のスナップショットを送ります。1台目の「他のマシンに入力してください」に従う欄が、2台目に出ません。1台目があとで同期すると、キーが合いません。

{image("ux38-sync-first.png", "1台目: キーが出る。まだ何も送っていない")}

{image("ux38-sync-second.png", "2台目: キーを聞かれず、別のキーができて送ってしまう")}

## 手順

1. 空のバケットで、1台目のSyncを設定する（［確認して保存］まで）。
2. 1台目では同期しないまま、2台目で同じバケットを設定する。
3. 期待: 2台目が共通の暗号化キーを聞く（または、1台目の設定の時点でバケットにキーの確認用の印が置かれる）。
4. 実際: 2台目が別のキーを作る。

1台目が先に［今すぐ同期］していれば、2台目はキーの欄を出しました（同じ試験で確かめました）。

## 環境

S3互換のサーバー（versitygw v1.9.0、自己署名の証明書）と、sshcのエンジンを2つ（{SOURCE}）使いました。

## 直し方の案

- 1台目の［確認して保存］の時点で、バケットにキーの確認に使う印（暗号化した小さなオブジェクト）を置きます。2台目はそれを見てキーを聞きます。
- 2台目がバケットを空と判断したときも、「新しく始める」と「ほかのマシンのキーを入れる」を選ばせます。

## そのほか同じ画面で見つけたこと

- 「いまコピーして、他のマシンに入力してください。…」が2回続けて出ます（UX-54）。
- 「最後に確認したのは2026-11-03T09:58:38Zです。」が世界協定時のままです（UX-55）。
- 違うキーを入れたときの文に`Code: wrong_passphrase`がそのまま出ます（UX-56）。
""",
    },
    {
        "id": "UX-39", "kind": "不具合", "priority": "中", "size": "小", "label": "bug",
        "title": "接続のエラーが英語の生の文で出る（keyboard-interactiveをすぐ断るサーバーで「unexpected message type 51」など）",
        "body": f"""## 何が起きるか

サーバーがkeyboard-interactiveを問いを出さずにすぐ断ると、ターミナルに次の文が出ます。上の帯と下の枠は「接続できませんでした。理由はターミナルに表示されています。」です。PAMの無いOpenSSHで、`PasswordAuthentication no`・`KbdInteractiveAuthentication yes`の構成がこれに当たります。

```
sshc: ssh: handshake failed: ssh: unexpected message type 51 (expected 60)
```

OpenSSHは同じ相手に`Permission denied (publickey,keyboard-interactive).`と出すので、何が足りないかが分かります。

{image("ux39-auth-message.png", "上: ホスト鍵の確認。下: 受け入れたあと意味の取れない文で止まる")}

再接続の間にも、英語の生の文がターミナルに残ります（`wait: remote command exited without exit status or exit signal`、`sshc: dial tcp 127.0.0.1:29396: connect: connection refused`）。

{image("ux39-raw-errors.png", "再接続の間のターミナル")}

## 原因

golang.org/x/crypto v0.56.0の`ssh/client_auth.go:621-623`は、INFO_REQUESTの前にFAILUREが来ると`unexpectedMessageError`を返します。sshcの`internal/sshclient/auth.go`（keyboard-interactive）は、これを言い換えずに出しています。

## 直し方の案

- この形を認証の失敗として扱い、「認証に失敗しました。サーバーが受け付ける方式: publickey, keyboard-interactive」と出します（FAILUREのメッセージに方式の一覧がある）。
- よく出る接続の失敗（拒否された、応答が無い、名前を引けない、相手が切った）にも、`[sshc]`の行で日本語の説明を添えます。元の英語の文は接続のログ（詳しく）に残します。

## 確かめ方

PAMの無いsshd（`PasswordAuthentication no`）に鍵なしでつなぎ、日本語の認証の失敗の文と受け付ける方式が出ること。
""",
    },
    {
        "id": "UX-40", "kind": "不具合", "priority": "中", "size": "小", "label": "bug",
        "title": "ライトのテーマ: Settings > Key barの並びの見本でキーの形が見えない（白地に白）",
        "body": f"""## 何が起きるか

ライト・ホワイトブルー・ホワイトパープルのテーマでSettings > Key barを開くと、並びの見本のキーに地も縁も無く、文字だけが並びます。ダークのテーマでは、キーの形が見えます。

{image("ux40-light-keybar-phone.png", "スマホ・ライト")}

{image("ux40-light-keybar-desktop.png", "PC・ライト")}

## 原因

`web/src/keybar/KeyBarPreview.tsx:30`は見本の地を`bg-term-bg`、`:24`はキーの地を`bg-card`にしています。ライトのテーマではどちらも白です。

## 直し方の案

ターミナルの下に出る本物のキーバーと同じ色の組（キーの地と縁）を見本でも使います。

## 確かめ方

テーマ7つのそれぞれで見本を撮り、キーの形が見えること。
""",
    },
    {
        "id": "UX-41", "kind": "不具合", "priority": "中", "size": "小", "label": "bug",
        "title": "文言: もう無い画面の名前を指す案内が残っている（［sshc］タブ、［Basic］、［Master password］）",
        "body": """## 何が起きるか

画面の作り直しで名前が変わったのに、案内の文が古い名前を指しています。

| キー | 今の文 | 今の画面 |
|---|---|---|
| `vpn.connectionsHint`（`VPNPanel.tsx:284`） | ［Connections］で接続先を選び、［sshc］タブの「使用するVPNプロファイル」を変更します。 | タブは「基本」「踏み台・転送・その他」「表示とターミナル」「最終的な値」 |
| `connection.vpnChangeStopsAssignments`（`HostVPNProfileField.tsx:74`） | パスワードとワンタイムパスワードは［Basic］で基本設定を保存し直し… | タブは「基本」 |
| `settings.vaultAutoLockPasswordless` | ［Master password］でマスターパスワードを設定すると適用されます。 | Settings > Vaultの中の節 |

## 直し方の案

今の画面の名前に直します。あわせて、`ja.ts`と`en.ts`の［…］の中の名前を画面の文言のキーから引けるかを検査で確かめると、次の作り直しで同じことが起きません。VPNの欄が「表示とターミナル」のタブにあるのも、経路の設定なので「踏み台・転送・その他」の方が探しやすいです（置き場所を変えるかは判断が要ります）。
""",
    },
    {
        "id": "UX-42", "kind": "不具合", "priority": "低", "size": "小", "label": "bug",
        "title": "サイドバー: 再接続をあきらめたセッションが「開けませんでした」と出て、開けなかった理由も行に出ない",
        "body": f"""## 何が起きるか

- 一度つながったあと、再接続の回数の上限まで試してあきらめたセッションの行が「開けませんでした」になります。一度は開けていたので、文が合いません。
- 開けなかったセッション（拒否された・応答が無い・ホスト鍵が変わった・認証に失敗）は、どれも同じ「開けませんでした」で、違いが行から分かりません。

{image("ux42-gave-up.png", "再接続をあきらめたあと。行は「開けませんでした」")}

{image("ux42-sidebar-failures.png", "失敗の理由が違う5つが同じ文")}

## 直し方の案

- あきらめたセッションは「切断・再接続できませんでした」にします。
- 開けなかった理由を短い言葉（拒否・応答なし・鍵が変わった・認証）で2行目に出します（#525で長い文が切れた件の続き）。
""",
    },
    {
        "id": "UX-43", "kind": "不具合", "priority": "低", "size": "小", "label": "bug",
        "title": "Windows: ローカルのPowerShellのセッション名が「管理者: C:¥WINDOWS¥System…」になる",
        "body": f"""## 何が起きるか

Windowsでローカルのシェル（PowerShell）を開くと、PowerShellが出す窓の題名（`管理者: C:\\WINDOWS\\System…`で始まる、実行ファイルのパス）がセッション名になり、サイドバーと見出しに出ます。

{image("ux35-windows-powershell.png", "見出しとサイドバーの行")}

## 直し方の案

題名が実行ファイルのパスのときは、シェルの名前（PowerShell）か作業フォルダの名前を出します。Windows Terminalはプロファイルの名前を既定の題名にしています。
""",
    },
    {
        "id": "UX-44", "kind": "デザイン", "priority": "高", "size": "小", "label": "enhancement",
        "title": "空の状態: 最初の起動のHomeに接続先を足すボタンが無い。ほかのページも空の状態がそろっていない",
        "body": f"""## 何が起きるか

- 最初に起動したHome（接続先0件）には「接続先がまだありません。SSH ConfigにHostを追加してください。」とだけ出て、接続先を足すボタンがありません。見出しの［Connectionsを管理］→［接続先を追加］と進むしかありません。0件なのに、検索欄・［パネル｜リスト］・「0件 / 全0件」・「リモート同期は設定されていません」も出ます。
- Connectionsの0件の一覧は「この条件に一致する接続はありません。」で、絞り込みの結果の文です。
- Known Hostsの0件は、検索欄だけで文がありません。SSH Keys・Known Hostsの0件では、要約の札が全部0で並びます。
- Snippetsだけは絵と［スニペットを追加］のある空の状態です。

{image("ux44-home-empty.png", "最初の起動のHome")}

{image("ux44-connections-empty.png", "Connectionsの0件")}

{image("ux44-empty-states.png", "スマホの空の状態（左からSnippets・SSH Keys・Known Hosts・Account passwords・History・VPN）")}

## ほかのアプリ

Termiusは最初の画面に［New Host］があり、Tabbyは接続の選択を開いてアドレスを打てばつながります。

## 案

Snippetsの空の状態の形（絵・一言・主の操作）を共通の部品にし、Home・Connections・Known Hosts・SSH Keysで使います。Homeは［接続先を追加］と［~/.ssh/configを読み込む／同期で受け取る］を出します。0件のときは、検索欄・表示の切り替え・件数・0の札を出しません。見た目を変えるので、案の画像を見せてから作ります。
""",
    },
    {
        "id": "UX-45", "kind": "機能", "priority": "中", "size": "中", "label": "enhancement",
        "title": "最初の接続までの手数が多い: 保存せずにuser@hostでつなぐ方法が無く、接続名を先に入れないと作れない",
        "body": f"""## 何が起きるか

最初の起動から最初の接続まで、手数は13です（Vaultの作成3、画面の移動2、入力5、作成1、接続1、ホスト鍵1）。Tabbyは約5です。

- 接続先を保存しないとつなげません。新しいセッションの検索欄に`deploy@10.0.0.5`と打っても、つなぐ候補は出ません。
- 新しい接続先は接続名を入れないと作れません（「接続名を入力してください。」）。

{image("ux45-tabby-quick-connect.png", "Tabby: 接続の選択にuser@host:portを打つと、その場でつなぐ候補が出る")}

{image("ux45-sshc-create.png", "sshc: 新しい接続先の画面")}

## 案

- 新しいセッションの検索欄（とCtrl+K）で`user@host[:port]`を受け付け、「つなぐ」と「接続先として保存してつなぐ」を出します。OpenSSHの設定ファイルをそのまま使う方針は変わりません（保存するときはHostのブロックを書く）。
- 新しい接続先で接続名を空にしたら、ホスト名から補います（`10.0.0.5` → `10-0-0-5`、`web.example.com` → `web`）。
""",
    },
    {
        "id": "UX-46", "kind": "機能", "priority": "中", "size": "中", "label": "enhancement",
        "title": "パスワード: 新しい接続先で「毎回入力する」を選べず、接続のときに入れたパスワードを保存するかも聞かない",
        "body": f"""## 何が起きるか

- 新しい接続先の認証方法は「SSH秘密鍵」「この接続先専用の暗号化パスワード」「保存済みパスワード」「新しい保存済みパスワード」の4つで、保存しない選び方がありません。
- 保存していない接続先では、ターミナルに`Password for deploy@127.0.0.1:29396 (password-only):`が出て、打てば接続できます。ただ、成功したあとVaultに保存するかを聞きません。

{image("ux46-sshc-password.png", "sshc: ターミナルでパスワードを聞く")}

{image("ux46-tabby-password.png", "Tabby: 聞く画面に［Remember］")}

## 案

- 認証方法に「接続のときに入力する（保存しない）」を足します。
- ターミナルでパスワードを入れて成功したら、ターミナルの知らせの枠で「この接続先のパスワードとしてVaultに保存しますか」と1回だけ聞きます。
""",
    },
    {
        "id": "UX-47", "kind": "機能", "priority": "中", "size": "中", "label": "enhancement",
        "title": "分割表示: 新しいセッションを横に開けず、セッションが1つだと［右に分割して表示］が押せない理由も出ない",
        "body": f"""## 何が起きるか

ターミナルの「…」の［右に分割して表示］［下に分割して表示］は、別のセッションを今のセッションの横に並べる操作です。セッションが1つだけだと押せず（灰色）、理由も出ません（`web/src/terminal/splitView/useSplitView.ts:32`の`canSplit`）。新しいセッションを横に開く道がありません。

{image("ux47-split-disabled.png", "セッションが1つのときの「…」")}

## ほかのアプリ

- Windows Terminal: Alt+Shift+=で右、Alt+Shift+-で下に新しいペインを開きます（既定のプロファイル。今のペインの複製も選べる）。フォーカスのあるペインは、さし色の枠で示します。
- Warp: 使っていないペインを暗くできます。
- Tabby: 今のプロファイルを複製して分割します。

## 案

- 横に並べるセッションが無いときは、［右に分割して表示］から新しいセッションの選択（ローカルのシェル・今の接続先の複製・ほかの接続先）を開きます。
- 割り当てられるショートカットに「右に分割」「下に分割」を足します（既定は未設定）。
""",
    },
    {
        "id": "UX-48", "kind": "機能", "priority": "低", "size": "小", "label": "enhancement",
        "title": "再接続: 回数の上限であきらめたあと、キーを押してつなぎ直せない",
        "body": f"""## 何が起きるか

再接続をあきらめると、上の帯と下の枠に「SSHの接続が切れ、再接続できる回数の上限まで試してもつながりませんでした。」と［再接続］が出ます。つなぎ直すには、［再接続］を押すしかありません。下の枠はターミナルの最後の数行に重なります。

{image("ux42-gave-up.png", "sshc: あきらめたあと")}

{image("ux48-tabby-reconnect.png", "Tabby: 「Press any key to reconnect」")}

## 案

あきらめたあとのターミナルに`[sshc] Enterで再接続します`と出し、Enterでつなぎ直します。ほかのキーは捨てます（打った文字がつなぎ直したシェルへ入らないように）。
""",
    },
    {
        "id": "UX-49", "kind": "機能", "priority": "中", "size": "中", "label": "enhancement",
        "title": "コマンド検索（Ctrl+K）で実行できる操作が4つしかない。ショートカットの操作と設定の切り替えも名前で探せるようにする",
        "body": """## 何が起きるか

Ctrl+Kで実行できる操作は「新しい接続先」「SFTPを開く」「ローカルのシェル」「Vaultをロック」の4つほどです（`web/src/shell/usePaletteCommands.ts`）。接続先・セッション・ファイル・設定のページへの移動はできます。

## ほかのアプリ

- Windows Terminal: すべての操作（actions）が名前でパレットに出ます。キーに割り当てた操作も、割り当てていない操作も同じ一覧です。
- Warp: 操作だけでなく、設定の切り替え（Compact mode、Tab indicatorsなど）も名前で探して切り替えられます。

## 案

- ショートカットの一覧の操作（18種、#533）を全部パレットに出し、割り当てたキーを右に添えます。
- よく切り替える設定（テーマ、文字の大きさ、OSC 52、画面を常にオン、分割）も名前で切り替えられるようにします。
""",
    },
    {
        "id": "UX-50", "kind": "デザイン", "priority": "中", "size": "中", "label": "enhancement",
        "title": "Settings: ショートカット・キーバーのページで「標準」の選択欄が2つ縦に並び、プリセットの操作の形もそろっていない",
        "body": f"""## 何が起きるか

- Keyboard shortcutsとKey barのページでは、設定のプリセットの選択（標準／このマシンで使用中）のすぐ下に、ショートカット（キーバー）のプリセットの選択（標準）が並びます。どちらの「標準」が何か分かりません。見出しの説明「ショートカットのプリセットはすべてのマシンで共通です。どれを使うかは、プリセットごとに選びます。」でも、「プリセット」が2つの意味で使われています。
- プリセットの操作が2つのページで違います。ショートカットは「プリセット名」の欄と［複製して保存］［名前を変更］、キーバーは選択と「…」です。
- Vaultのページは、設定のプリセットの帯の下に、プリセットに入らないマスターパスワード（マシンごと）が並びます。節の見出しだけ「Master password」と英語で、形もほかの節と違います。

{image("ux50-shortcuts-phone.png", "Keyboard shortcuts（スマホ）")}

{image("ux50-keybar-phone.png", "Key bar（スマホ）")}

{image("ux50-vault.png", "Vault（PC）")}

## 案

- ショートカット・キーバーのプリセットは「キーの割り当て」「キーの並び」と呼び分け、選択欄の名前に入れます。操作の形はキーバーの「選択＋…」にそろえます。
- マスターパスワードはプリセットの帯の外（区切った別の節）に置きます。

見た目を変えるので、案の画像を見せてから作ります。
""",
    },
    {
        "id": "UX-51", "kind": "デザイン", "priority": "低", "size": "小", "label": "enhancement",
        "title": "新しい接続先: 踏み台を作成の画面で選べず、作ったあとで別のタブを開いて保存し直す",
        "body": f"""## 何が起きるか

新しい接続先の画面には踏み台の欄がありません。手順は、作成 →［踏み台・転送・その他］のタブ →「経由する接続先」の選択 →［保存］→［接続］です。PCでは、ポート転送の種類の選択が「Localトンネ」と切れます。

{image("ux51-create-no-jump.png", "新しい接続先の画面")}

## ほかのアプリ

Termiusは作成の画面の「Show more」でHost Chaining（踏み台）を選べます。グループに付けると、中の接続先が受け継ぎます。

## 案

新しい接続先の「接続先」の節に、「経由する接続先」の選択を足します（既定は「なし（直接つなぐ）」）。グループの既定値で踏み台を受け継げるかも、あわせて見ます。
""",
    },
    {
        "id": "UX-52", "kind": "デザイン", "priority": "低", "size": "中", "label": "enhancement",
        "title": "SFTP: スマホの行のチェックボックスが常に出る、2つのプレビューの部品が違う、未接続でも絞り込みと比較が出る",
        "body": f"""## 何が起きるか

- スマホの一覧では、すべての行に選択のチェックボックスが常に出ます。
- 同じ名前が2回出ます（タブの「deploy／web-01」とパスの帯の「deploy」）。「空き容量: …」も常に出ます（前回の点検でも挙げたもの）。
- SFTPのプレビューとターミナルのプレビューで、部品が違います。SFTPは見出しの名前が「IMG_20…」と切れ、［閉じる］が文字のボタンです。ターミナルは×のアイコンです。
- 未接続のタブ（PC）にも、［絞り込み］の欄と［比較］が出ます。

{image("ux52-sftp-list.png", "SFTPの一覧（スマホ）")}

{image("ux52-sftp-preview.png", "SFTPのプレビュー")}

{image("ux52-terminal-preview.png", "ターミナルのプレビュー（同じ画像）")}

## 案

- スマホは長押しで選ぶ形（選んでいる間だけチェックボックスを出す）にします。
- プレビューは1つの部品にそろえ、ファイル名を見出しに出します。
- 未接続のタブでは絞り込みと比較を出しません。
""",
    },
    {
        "id": "UX-53", "kind": "デザイン", "priority": "低", "size": "小", "label": "enhancement",
        "title": "スニペット: クイックコマンドの変数の欄の名前が「{{lines}}」のまま、実行まで6手かかる",
        "body": f"""## 何が起きるか

- クイックコマンドの変数の欄の名前が、スニペットの書き方の`{{{{lines}}}}`のままです。
- 実行までは、…→［クイックコマンド］→ スニペットを選ぶ → 変数 →［実行内容を確認］→［実行］の6手です。

{image("ux53-quick-command.png", "クイックコマンド")}

## ほかのアプリ

Termiusはターミナルの横の欄（Ctrl/Cmd+.）からスニペットを1回で「貼る」か「実行」できます。

## 案

- 欄の名前は変数の名前（`lines`）にし、スニペットの説明があれば添えます。
- 変数の無いスニペットは確認を挟まずに貼れるようにします（実行は今のとおり確認する）。
""",
    },
    {
        "id": "UX-54", "kind": "デザイン", "priority": "低", "size": "小", "label": "enhancement",
        "title": "同じことを2か所に出す: Syncの説明とキーの文、件数、Connectionsのグループの列、セッションの見出しの2行目",
        "body": f"""## 何が起きるか

利用者の方針（同じことを2か所に書かない）から外れる所です。

- Sync: 見出しの説明「~/.sshを暗号化してから、オブジェクトストレージのバケット経由でマシン間に同期します。」と、手順の下の「~/.sshを暗号化してから送ります。」。キーの「いまコピーして、他のマシンに入力してください。…」が2回続けて出ます。「最終同期：…」「このマシンはまだ同期していません。」も2か所に出ます。エンドポイントの例の文字と、下の説明のR2の例も同じです。
- HomeとConnections: 見出しの「接続先18件」と、その下の「18件 / 全18件」。
- Connections（PC）: グループの見出しの下の行に、同じグループ名の列（production…）が毎行あります。
- 名前を付けていないセッション: 見出しとサイドバーの行の1行目と2行目が「web-01／web-01」で同じです。

{image("ux54-sync-twice.png", "Sync（上: 1台目、下: 2台目）")}

{image("ux54-connections.png", "Connections（PC）")}

{image("ux54-session-name.png", "名前を付けていないセッション")}

## 案

- Syncの重なる文を1つにまとめます。
- 件数は「18件 / 全18件」の方だけにし、絞り込んでいないときは「18件」にします。
- グループの見出しで分けているときは、グループの列を出しません。
- 名前と接続名が同じときは、2行目にuser@hostを出します。
""",
    },
    {
        "id": "UX-55", "kind": "デザイン", "priority": "低", "size": "小", "label": "enhancement",
        "title": "日時の書き方がそろっていない（「今日」「2026/11/03 17:49」「最終接続：今日 17:49」、Syncは世界協定時の生の形）",
        "body": f"""## 何が起きるか

- Homeのカードは「今日」です。
- Connections（PC）の列は「最終接続：今日 17:49」です。
- 新しいセッションとSFTPの接続先の選択は「最終接続：2026/11/03 17:49」です。
- Syncは「最後に確認したのは2026-11-03T09:58:38Zです。」と世界協定時の生の形で、ほかは「2026/11/03 18:58」です。

{image("ux55-dates.png", "新しいセッションの一覧")}

## 案

日時を整える関数を1つにし、近いものは「今日 17:49」「昨日 9:02」、それより前は「11/1 9:02」、年が違えば年を付けます。Syncの日時もその関数を通します。
""",
    },
    {
        "id": "UX-56", "kind": "デザイン", "priority": "低", "size": "中", "label": "enhancement",
        "title": "表示の細部: 「…」の切り替えの「—」、グループの札が多いと1画面を埋める、内部の名前が出るなど",
        "body": f"""## 何が起きるか

1つずつは小さいので、まとめました。

- ターミナルの「…」の「すべてのターミナル」で、OSC 52・画面を常にオンのオフが「—」です。切り替えだと分かりません。
- スマホのHomeで、グループの札が多い（14個）と1画面目を埋め、接続先の一覧が見えません。
- Settings > Machines（スマホ）で、使うプリセットの選択とポートの欄に名前がありません（PCは表の見出しがある）。最後の起動の日時にも名前がありません。
- Settings > Open connectionsは「● 5件」だけで、何が開いているかが出ません。
- Historyで「その他の変更」が並び、何が変わったかが分かりません。
- 0の札（SSH Keysの「確認が必要 0」、Known Hostsの「スキャン候補 0」、Historyの「中断 0」）が出ます。SSH Keysの「確認済みSSHファイル」は意味が伝わりません。
- 内部の名前がそのまま出ます: ショートカットの「Ctrl+Shift+ArrowUp」「Meta+K」、Syncの「Code: wrong_passphrase」。
- Settings > Terminalの再生バッファが「262144 バイト」と生の数です。

{image("ux56-many-groups.png", "グループが14個のスマホのHome")}

{image("ux56-history.png", "History")}

## 案

- 「…」の切り替えはスイッチの形にします。
- グループの札は横にスクロールする1行にします。
- 欄には名前を付けます。0の札は出しません。
- キーの名前は↑・⌘のように記号で出します。内部の記号は詳しくの中へ移します。
""",
    },
    {
        "id": "UX-57", "kind": "文言", "priority": "低", "size": "中", "label": "enhancement",
        "title": "文言の用語の揺れ: 「ディレクトリ」と「フォルダ」、「リモートホスト」、「経路」、日本語の画面に残る英語",
        "body": """## 何が起きるか

`web/src/i18n/messages/ja.ts`（3040件）を数えた結果です。

| 揺れ | 件数 | 例 |
|---|---|---|
| 「ディレクトリ」と「フォルダ」 | 35と50 | SFTPの中でも「このディレクトリは空です」「ホームディレクトリ」と「フォルダ」が混ざる。`workspace.executionNotice`だけ「作業ディレクトリ」（ほかは「作業フォルダ」） |
| 「リモートホスト」 | 3 | Remote Keysの`rk.description`・`rk.registerFailed`・`rk.remoteRuns`（ほかは「接続先」） |
| 「経路」 | 21 | 「VPN経路」「経路の設定」（利用者の決めた言い方は「ルート」） |
| 「失敗しました」と「できませんでした」 | 28と113 | 多い方の「できませんでした」に寄せる |

日本語の画面に残る英語（ページ名は英語のままにする決まりなので除く）: サイドバーの検索のボタン「Search」、`home.quickConnect`「Quick Connect」、新しいセッションの「Local shell」、Vaultの設定の節「Master password」、ポート転送の「Localトンネル」。

## 案

「フォルダ」「接続先」「ルート」に寄せます。英語の5つは日本語にします（「検索」「ローカルのシェル」「マスターパスワード」「ローカル転送」など）。`retiredJapaneseTerms.test.ts`に、寄せた語の古い形を足します。
""",
    },
    {
        "id": "UX-58", "kind": "文言", "priority": "低", "size": "小", "label": "enhancement",
        "title": "常に出る長い説明: Settings > Terminalの上限の説明など、画面を見れば分かる文・数の範囲の羅列",
        "body": """## 何が起きるか

利用者の方針（常に出る長い説明文を置かない。知らないと困ることだけ短く）から外れる文です。

- `terminal.limitsHint`: 「空欄はそれぞれ50・262144・…。セッションは1〜200、再生バッファは16384〜4194304、…」。数の範囲は欄の検査で外れたときに出せば足ります。
- `terminal.notificationSoundHint`・`terminal.touchModeAutoHint`の「このマシンが使うプリセットに保存されます」「このマシンが使うプリセットの設定で」。ナビのプリセットの帯で分かります。
- Vaultの3ページの「値は編集するときだけ表示されます。」。
- SFTPの空の状態の「ローカルか保存した接続先を選んでください。接続先は選ぶと接続します。」（ボタンと一覧で分かる）。

## 案

上の文を消すか、1文に縮めます。範囲は欄の検査の文へ移します。
""",
    },
]


def summary_body(numbers: dict) -> str:
    def link(item_id: str) -> str:
        return f"#{numbers[item_id]}" if item_id in numbers else item_id

    rows = "\n".join(
        f"- [ ] {link(item['id'])} {item['id']} {item['title']}（{item['kind']}、優先度 {item['priority']}・規模 {item['size']}）"
        for item in ITEMS
    )
    return f"""sshcの完成度を上げるための点検です。前回の点検（#511）が止まった画面を中心に見たのに対し、今回は次の4つの観点で見ました。

- これまでの指摘から決まりを22個取り出し、全ページに当てはめる
- ほかのアプリ（Termius・Windows Terminal・Warp・Tabby）と作業の流れで比べる
- 動きと状態（読み込み中・空・失敗、接続から再接続・失敗まで）
- 画面の文言

対象は{SOURCE}です。

## 結論

- **すぐ直したい不具合が4つあります。** WindowsではターミナルがBIZ UDGothicで描かれ、`\\`が`¥`で出て罫線がずれます（{link("UX-35")}）。SFTPでは、まだホスト鍵を知らない接続先を開けません（{link("UX-36")}）。OpenSSHが設定全体を読めなくなる打ち間違いを、sshcは警告しません（{link("UX-37")}）。同期で、1台目が送る前に2台目を設定すると暗号化キーが分かれます（{link("UX-38")}）。
- **最初の体験が弱いです。** 最初の起動のHomeに接続先を足すボタンが無く（{link("UX-44")}）、最初の接続までの手数はTabbyの約5に対して13です（{link("UX-45")}）。
- **前回の34件で元に戻ったものはありません。** 今回の画像で、#512〜#525・#527・#530〜#533・#535〜#543を確かめました。#528・#534・#544・#545は確かめていません。#526・#529は今回の対象外です。

## 一覧

優先度は高・中・低、規模は小（1日以内）・中（数日）・大（1週間以上）の目安です。

{rows}

## 調べ方

- 架空のデータの3つの組（整ったデータ、接続先187件・失敗するセッション・長い名前の荒れたデータ、~/.sshが空の状態）を入れた一時のHOMEでsshcのエンジンを動かしました。Playwright（Chromium、`--disable-webgl`、日本語）で、スマホ縦・タブレット縦・2in1・PC（ほかにスマホ横・タブレット横・2in1のタブレットの姿勢・1366×768）を撮りました。テーマは7つのうちライト系3つとダーク系2つも撮りました。撮影の道具は`scripts/ux-audit/capture-screens.mjs`です。
- 作業の流れ（最初の接続・踏み台・認証・再接続・分割・SFTP・スニペット・2台の同期）を`scripts/ux-audit/flows.mjs`で実際に流し、手数と画面を記録しました。接続先は使い捨てのsshdのコンテナ、同期はS3互換のサーバー（versitygw）です。
- Windowsは、r100に作ったWindows 11の仮想マシンで、Chrome 156をCDPで操作して撮りました。
- 比べたアプリ: Tabby 1.0.238は、使い捨てのコンテナで動かして触りました。Termius（最初にアカウントの作成が要る）・Windows Terminal・Warpは、公式の文書で比べました。

## 確かめていないもの

- キーボードだけでの操作とフォーカスの順、画面の切り替えの速さ（動きと状態の観点のうち2つ）。
- スマホのCtrl+C・貼り付け・指のスクロール。2026-11-02のスマホの点検（Androidのエミュレータ）で見たので、今回は流していません。
- iPadのSafari、macOSの実機。

## 作業の流れで比べた結果

| 流れ | sshc | ほかのアプリの良い所 |
|---|---|---|
| 最初の接続 | Vault作成 → Home（追加のボタンが無い）→ Connections → 追加 → 5つの欄 → 作成 → 接続 → 鍵。手数13 | Tabbyはuser@hostを打てばつながる。Termiusは［New Host］が最初の画面にある |
| 踏み台 | 作ったあとで別のタブで選ぶ | Termiusは作成の画面で選べ、グループで受け継げる |
| パスワード | 保存しないとターミナルで聞かれる。成功しても保存を勧めない | Tabbyは聞く画面に［Remember］ |
| 再接続 | 帯・枠・サイドバー・記録がそろう。あきらめたら［再接続］を押す | Tabbyは「Press any key to reconnect」 |
| 分割 | 別のセッションを横に並べる。1つだと押せない | Windows Terminalは新しいペインを開く・複製する |
| SFTP | 初めての接続先は鍵の確認が無く失敗 | Tabby・Termiusはターミナルと同じ確認を出す |
| スニペット | …→クイックコマンド→選ぶ→変数→確認→実行 | Termiusは横の欄から1回で貼る・実行 |
| コマンド検索 | 実行できる操作は4つ | Windows Terminal・Warpはすべての操作と設定の切り替え |
"""


def write_bodies(out: Path, summary_number: str | None) -> list:
    out.mkdir(parents=True, exist_ok=True)
    issues = []
    for item in ITEMS:
        footer = f"\n\n---\nUI/UX点検（2026-11-03）の{item['id']}です。" + (f"まとめは #{summary_number}。" if summary_number else "")
        body = item["body"].strip() + footer + "\n"
        path = out / f"{item['id']}.md"
        path.write_text(body)
        issues.append({"id": item["id"], "title": f"{item['title']}（{item['id']}）", "label": item["label"], "body": str(path)})
    (out / "issues.json").write_text(json.dumps(issues, ensure_ascii=False, indent=2))
    created = json.loads((out / "created.json").read_text()) if (out / "created.json").exists() else {}
    (out / "summary.md").write_text(summary_body(created))
    return issues


def copy_images(assets: Path) -> None:
    target = assets / IMAGE_DIR
    target.mkdir(parents=True, exist_ok=True)
    for name, source in IMAGES.items():
        shutil.copyfile(SHOTS / source, target / name)
    print(f"copied {len(IMAGES)} images to {target}")


def gh(*args: str) -> str:
    return subprocess.run(["gh", *args], check=True, capture_output=True, text=True).stdout.strip()


def post(out: Path) -> None:
    out.mkdir(parents=True, exist_ok=True)
    number_file = out / "summary-number"
    if number_file.exists():
        summary = number_file.read_text().strip()
    else:
        (out / "placeholder.md").write_text("作成中です。各項目のイシューを登録したあと、一覧に置き換えます。\n")
        summary = gh("issue", "create", "--repo", REPOSITORY, "--title", SUMMARY_TITLE, "--label", "enhancement", "--body-file", str(out / "placeholder.md")).rsplit("/", 1)[1]
        number_file.write_text(summary)
    print("summary", summary)
    issues = write_bodies(out, summary)
    created_file = out / "created.json"
    created = json.loads(created_file.read_text()) if created_file.exists() else {}
    for issue in issues:
        if issue["id"] in created:
            continue
        url = gh("issue", "create", "--repo", REPOSITORY, "--title", issue["title"], "--label", issue["label"], "--body-file", issue["body"])
        created[issue["id"]] = url.rsplit("/", 1)[1]
        created_file.write_text(json.dumps(created, indent=2))
        print(issue["id"], url)
    # 番号がそろったので、本文のUX-NNを番号に置き換え、まとめを一覧にする。
    write_bodies(out, summary)
    for issue in issues:
        text = Path(issue["body"]).read_text()
        for item_id, number in created.items():
            text = text.replace(f"（{item_id}）", f"（#{number}）") if item_id != issue["id"] else text
        Path(issue["body"]).write_text(text)
        gh("issue", "edit", created[issue["id"]], "--repo", REPOSITORY, "--body-file", issue["body"])
    gh("issue", "edit", summary, "--repo", REPOSITORY, "--body-file", str(out / "summary.md"))
    print("bodies and summary updated")


if __name__ == "__main__":
    command, place = sys.argv[1], Path(sys.argv[2])
    if command == "images":
        copy_images(place)
    elif command == "bodies":
        write_bodies(place, sys.argv[3] if len(sys.argv) > 3 else None)
        print("written to", place)
    elif command == "post":
        post(place)
