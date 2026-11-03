"""スマホ版（Android のアプリ）の点検（2026-11-03）の所見を GitHub のイシューにする。

使い方は issues-2026-11-03.py と同じ（images・bodies・post）。画像は issue-assets ブランチの
ux-audit-2026-11-03-mobile/ に置き、raw の URL で貼る。撮った画像は作業ツリー mobile-audit-1103 の
artifacts/mobile-audit/2026-11-03/ にある。
"""
import json
import shutil
import subprocess
import sys
from pathlib import Path

SHOTS = Path.home() / "projects/sshc-worktrees/mobile-audit-1103/artifacts/mobile-audit/2026-11-03"
IMAGE_DIR = "ux-audit-2026-11-03-mobile"
IMAGE_BASE = f"https://raw.githubusercontent.com/aida0710/sshc/issue-assets/{IMAGE_DIR}"
REPOSITORY = "aida0710/sshc"
SOURCE = "main の `8dc4a7e96`（v0.59.9）から組み立てた開発版の APK"
ENVIRONMENT = "Android 16 のエミュレータ（1080×2400・420dpi、ジェスチャーナビ、Gboard の日本語・12キー）"
SUMMARY_TITLE = "スマホ版（Androidのアプリ）の点検（2026-11-03）: 14件のまとめ"

IMAGES = {
    "ux59-kana-typed.png": "view/f1-c.png",
    "ux59-keyboards.png": "view/m08.png",
    "ux60-shift-tab.png": "shots/c24-main-prompt-no-keyboard.png",
    "ux60-permission.png": "view/c17.png",
    "ux61-waiting-sidebar.png": "shots/c65-drawer-waiting.png",
    "ux61-no-notification.png": "shots/c70-notification-shade.png",
    "ux62-gave-up.png": "shots/air-3-back-20s.png",
    "ux62-after-force-stop.png": "shots/restore-after-force-stop.png",
    "ux63-autofocus.png": "shots/m06-03-opened.png",
    "ux64-failed-send.png": "view/c3.png",
    "ux65-host-key-timeout.png": "view/c1.png",
    "ux66-small-keyboard.png": "shots/small-keyboard-open.png",
    "ux68-ssh-agent.png": "view/a2.png",
    "ux68-machines-about.png": "view/a5.png",
    "ux69-vault-check.png": "shots/c71-after-return.png",
    "ux70-selection.png": "shots/c32-long-press.png",
    "ux71-small-keybar.png": "shots/small-terminal.png",
    "ux72-folder-menu.png": "shots/sftp-03-folder-menu.png",
}


def image(name: str, caption: str) -> str:
    return f'<img src="{IMAGE_BASE}/{name}" alt="{caption}" width="360">\n\n{caption}'


ITEMS = [
    {
        "id": "UX-59", "kind": "不具合", "priority": "高", "size": "小", "label": "bug",
        "title": "スマホ: 日本語のGboardでは、接続名・ホスト名・ユーザーの欄がかなで開き、全角やかなが入って接続先を作れない",
        "body": f"""## 何が起きるか

新しい接続先の接続名・ホスト名・ユーザーの欄は`inputmode="url"`です（`web/src/ui/softKeyboard.ts`の`asciiTextEntry`、2026-11-02の#594〜#604で付けた）。ところが日本語のGboardは直前の配列を覚えていて、かな（12キー）のまま開くことがあります。最初の起動ではかなで開き、打った文字が「my-せrゔぇr」「１０．０．２．２」「でｐおｙ」のようになりました。ホスト名は「DNS名、IPv4アドレス…を入力してください。」で断られ、接続先を作れません。

{image("ux59-kana-typed.png", "左から2枚目: 接続名の欄でかなのキーボードが開いている")}

試しの欄を差し込み、直前にかなを使ったあとで指定ごとに開いてみました。

| 欄の指定 | キーボードに渡る入力の種類 | 開いた配列 |
|---|---|---|
| `inputmode="url"`（今） | 0x4011 | かなのまま |
| `type="url"` | 0x11 | かなのまま |
| `inputmode="email"` | 0x40d1 | 英字 |
| `type="email"` | 0xd1 | 英字 |

{image("ux59-keyboards.png", "右から: type=url はかな、ほかは英字（左端は直前にかなを使った状態）")}

## 環境

{ENVIRONMENT}、{SOURCE}。

## 直し方の案

- 接続名・ホスト名・ユーザーなど英字だけの欄は`inputmode="email"`にする（`asciiTextEntry`を変えれば全部の欄に効く）。メールの配列には`@`と`.`があり、ホスト名や`user@host`も打ちやすい。
- アプリだけの直し方として、WebViewの`onCreateInputConnection`で`IME_FLAG_FORCE_ASCII`を付ける方法もある。ブラウザで開いたときには効かない。

## 確かめ方

日本語のGboardでかなを使ったあと、新しい接続先の各欄を押して英字の配列が開くこと（`dumpsys input_method`の`inputType`）。
""",
    },
    {
        "id": "UX-60", "kind": "機能", "priority": "高", "size": "中", "label": "enhancement",
        "title": "スマホ: Claude Codeのモードの切り替え（Shift+Tab）を打てない。Claude Codeの操作に要るキーをキーバーにそろえる",
        "body": f"""## 何が起きるか

利用者はスマホのsshcを主にClaude Codeの操作に使います。Claude Code 2.1.304を動かして、スマホから操作できるかを確かめました。

| 操作 | Claude Codeのキー | スマホのsshcで |
|---|---|---|
| 止める | Esc | キーバーのEsc。1手 |
| 巻き戻し | Escを2回 | 素早く2回押せば出る |
| 許可の確認・メニューを選ぶ | ↑↓とEnter | ↑↓はキーバー、Enterは入力欄を空のまま送る。Enterのキーは既定のキーバーに無い |
| モードの切り替え | Shift+Tab | **打てない**。キーバーにShiftが無く、自作のキーは制御文字（ESC [ Z）を送れない（`KeyBarTextForbiddenPattern`） |
| コマンドの候補 | `/` | キーバーの「…」→「/」で2手 |
| ファイルの候補 | `@` | 既定のキーバーに無い（自作のキーで足せる）。日本語のGboardのかなでは全角の「＠」になる |
| 末尾へ移る | Ctrl+End | Ctrl→「…」→End の3手 |
| 終わる | Ctrl+Cを2回 | キーバーの^Cを2回 |

画面の下には「auto mode on (shift+tab to cycle)」と出ますが、スマホからは切り替えられません。

{image("ux60-shift-tab.png", "モードの行（⏵⏵は豆腐で出ている。UX-67）")}

{image("ux60-permission.png", "許可の確認は↓↑と空の送信で答えられた")}

## ほかのアプリ

Termiusのスマホ版は、長押しして動かすと矢印キー、ダブルタップでTab、キーボードの上のキーは並べ替え・自作ができます。横の欄からスニペットとコマンドの履歴を1回で実行できます（docs.termius.com/terminal/mobile-terminal）。

## 案

- 置けるキー（`internal/validate/keybar.go`の`KeyBarKeys`）に`Shift+Tab`（CSI Z）を足す。
- 既定のキーバーとは別に「Claude Code」のキーバーのプリセットを用意する（Esc・Shift+Tab・Enter・↑↓・^C・/・@・Tab）。設定のプリセットで選べる。
- 入力欄を空で送るとEnterになることを、送信のボタンの名前か形で分かるようにする（今は矢印のアイコンだけ）。
""",
    },
    {
        "id": "UX-61", "kind": "機能", "priority": "高", "size": "中", "label": "enhancement",
        "title": "スマホ: Claude Codeの許可待ちをAndroidの通知にしない。通知の許可を求める機会も無い",
        "body": f"""## 何が起きるか

- 依頼を送ってアプリを裏に回し、Claude Codeが許可を待つ状態にしました。サイドバーには「許可待ち · approve Bash: date > no…」が出ます（OSC 7501、エンジンが状態を持っている）。しかし、Androidの通知は出ませんでした。
- 通知の許可はSettings > Notificationsの［通知を有効にする］を押すまで求められません。最初の起動でも、Claude Codeが入力を待ったときにも求めません。
- Notificationsの説明は「このタブがバックグラウンドにあるとき」で、ブラウザの言い方のままです。

{image("ux61-waiting-sidebar.png", "サイドバーには許可待ちが出る")}

{image("ux61-no-notification.png", "裏に回して25秒待っても、sshcの通知は許可を求めたときのものだけ")}

## 原因

ターミナルの通知（`android/.../TerminalNotifications.java`）は、画面（WebView）がOSC 9・99・777を受けたときに出します。プログラムの状態（OSC 7501の`blocked`）からは出しません。アプリが裏にいる間は、画面のJavaScriptが止まることもあります。

## 案

- エンジン（`internal/terminal/program_status_records.go`）が`blocked`（許可待ち・入力待ち）に変わったとき、Androidの前面のサービス（`EngineService`）から通知を出す。画面が裏にいても届く。通知を押すとそのセッションを開く。
- 最初にClaude Codeの状態を受け取ったとき、通知の許可を求める知らせを1回出す。
""",
    },
    {
        "id": "UX-62", "kind": "機能", "priority": "高", "size": "大", "label": "enhancement",
        "title": "スマホ: 回線が約40秒切れると再接続をあきらめ、戻ってもつなぎ直さない。SSHが切れるのでClaude Codeも終わる",
        "body": f"""## 何が起きるか

- 機内モードで約40秒回線を切ると、裏にあったセッションは再接続の5回（1・3・6・11・16秒あと、合計約37秒）を使い切ってあきらめます。回線が戻っても、自動ではつなぎ直しません。スマホでは、電車・エレベーター・Wi-Fiとモバイル回線の切り替えでよく起きます。
- SSHが切れるので、tmuxの中でなければ、サーバーのClaude Codeも終わります。
- アプリを強制終了したとき（システムに閉じられたときも同じ）は、エンジンごと止まります。起動し直すと「前回のセッションを開き直しますか」が出ますが、開き直したのは新しいシェルで、Claude Codeは終わっています。`claude --resume`の案内もありません。

{image("ux62-gave-up.png", "回線が戻って20秒たっても「再接続できる回数の上限に達しました」のまま")}

{image("ux62-after-force-stop.png", "強制終了のあと開き直したセッション（Claude Codeは終わっている）")}

再接続の失敗の文はターミナルに英語の生の文で出ます（`sshc: dial tcp 10.0.2.2:29299: connect: network is unreachable`、#633と同じ）。

## ほかのアプリ

Termiusは、Moshで回線が変わってもセッションを保ちます（docs.termius.com）。

## 案

1. 回線が無い間（`network is unreachable`、Androidの`ConnectivityManager`が回線なしと言う間）は、再接続の回数に数えずに待つ。回線が戻ったらすぐ試す。
2. 接続先ごとに「tmuxで開く」（`tmux new -A -s sshc-<名前>`）を選べるようにする。再接続や強制終了のあとも、同じClaude Codeの画面に戻れる。サーバーにtmuxが無いときは知らせる。
3. 開き直しで新しいシェルになったとき、前のセッションでClaude Codeが動いていれば（OSC 7501を受けていた）、`claude --continue`を案内する。

1は不具合として先に直せます。2と3は設計が要るので、案を見せてから作ります。
""",
    },
    {
        "id": "UX-63", "kind": "不具合", "priority": "中", "size": "小", "label": "bug",
        "title": "スマホ: 新しい接続先の画面を開くと、接続名の欄にフォーカスが入ってキーボードが開く",
        "body": f"""## 何が起きるか

Connectionsの［接続先を追加］を押すと、接続名の欄に自動でフォーカスが入り、キーボードが開きます。画面の下半分が隠れ、入力の欄がどれだけあるかが見えません。利用者の方針（スマホでキーボードを勝手に開かない、2026-10-29）に反します。

{image("ux63-autofocus.png", "開いた直後")}

## 直し方の案

スマホの組み方（`mobileViewportQuery`）のときは、新しい接続先の画面で自動のフォーカスをしない。PCは今のままでよい。

## 確かめ方

スマホの幅で［接続先を追加］を押したあと、`document.activeElement`が入力欄でないこと。
""",
    },
    {
        "id": "UX-64", "kind": "不具合", "priority": "中", "size": "小", "label": "bug",
        "title": "スマホ: 接続できなかった・切れたセッションで入力欄から送ると、文が消えて何も起きない",
        "body": f"""## 何が起きるか

接続できなかったセッションで、入力欄に「echo lost-or-not」と入れて送信を押すと、入力欄が空になり、ターミナルにも何も出ません。Claude Codeへの長い依頼を書いて送ったときに、回線が切れていると依頼を失います。

{image("ux64-failed-send.png", "左: 送ったあと入力欄が空。何も起きていない")}

## 原因

`web/src/terminal/TerminalView.tsx`の`submitComposer`は、セッションの状態を見ずに送り、入力欄を空にします。

## 直し方の案

セッションが送れない状態（接続中・再接続中・失敗・終了）のときは、送信のボタンを押せなくする。押したら文を残したまま「つながってから送ります」か「再接続してから送ってください」と出す。再接続のあとに自動で送るのは、別のシェルに入るので避ける。
""",
    },
    {
        "id": "UX-65", "kind": "不具合", "priority": "中", "size": "小", "label": "bug",
        "title": "スマホ: ホスト鍵の確認が警告アイコンの中だけで、待つ間にサーバーに切られ「handshake failed: EOF」で失敗する",
        "body": f"""## 何が起きるか

スマホでは、ホスト鍵の確認が上のバーの警告アイコンの中にあり、最初は開きません（利用者の方針、2026-11-02）。気づかずに2分ほど置くと、サーバー（sshdの`LoginGraceTime`、既定120秒）が接続を切ります。そのあと［接続する］を押すと、`sshc: ssh: handshake failed: EOF`で失敗します。

{image("ux65-host-key-timeout.png", "受け入れたあとで失敗し、再接続を求められる")}

## 直し方の案

- 受け入れたときに接続が切れていたら、鍵を`known_hosts`に書いてからつなぎ直す（もう一度鍵を聞かない）。
- 確かめを待っている間は、警告アイコンを目立たせる（さし色、点滅しない程度の印）。アイコンだけの方針は保つ。
""",
    },
    {
        "id": "UX-66", "kind": "デザイン", "priority": "中", "size": "中", "label": "enhancement",
        "title": "スマホ: 小さい画面でキーボードを開くと、ターミナルが約5行しか残らない",
        "body": f"""## 何が起きるか

360×640の画面で入力欄を押してキーボードを開くと、ターミナルの高さは108px（約5行）です。見出し（54px）、キーバー、入力欄で約200pxを使っています。Claude Codeの許可の確認は「1. Yes」だけが見え、2〜4の選択肢が隠れます。標準の大きさ（412×915）でも、キーボードを開くと半分ほどになります。横向きでは、キーボードが無くても約7行です。

{image("ux66-small-keyboard.png", "360×640でキーボードを開いたところ")}

## 案

- キーボードが開いている間は、見出しを畳む（名前だけの細い行にするか、隠す）。
- キーバーと入力欄を1段にまとめる形も考えられる（キーバーを入力欄の上に横スクロールで重ねる）。

見た目を変えるので、案の画像を見せてから作ります。
""",
    },
    {
        "id": "UX-67", "kind": "不具合", "priority": "低", "size": "小", "label": "bug",
        "title": "スマホ: Claude Codeのモードの印「⏵⏵」が豆腐（□□）で出る",
        "body": f"""## 何が起きるか

Claude Codeの下の行「⏵⏵ auto mode on」の記号が□□で出ます。エミュレータ（Android 16、google_apisの像）の字体にU+23F5が無いためと見られます。実機（Pixelなど）で出るかは確かめていません。

{image("ux60-shift-tab.png", "下から2行目")}

## 直し方の案

ターミナルの字体の並びの最後に、U+23F5を含む記号の字体を足す（同梱するなら要る範囲だけに絞る）。#629で直した既定の並び（`web/src/terminal/fonts.ts`）と同じ場所。
""",
    },
    {
        "id": "UX-68", "kind": "文言", "priority": "中", "size": "小", "label": "enhancement",
        "title": "スマホ: PCを前提にした案内がアプリに出る（ssh-agent、sshc ssh host、ブラウザ、ポート、実行ファイルなど）",
        "body": f"""## 何が起きるか

Androidのアプリでは意味の無い案内です。

| 場所 | 文 |
|---|---|
| Vaultの作成・ロック解除 | このブラウザからサインアウト |
| SSH Keys | sshcからssh-agentに接続できないため、鍵を登録できません。sshcの環境変数SSH_AUTH_SOCKでssh-agentのソケットを指定してください。（Androidにssh-agentは無い） |
| Terminalの空の状態 | `$ sshc ssh host`（AndroidにCLIは無い）。［新しいセッション］もスマホの画面に無い（#638で扱う） |
| Settings > General | 表示言語「ブラウザに合わせる」 |
| Settings > Machines | 名前「android」、ポートの欄、「次のsshcエンジンの起動から効きます…初期値54447」 |
| Settings > Notifications | このタブがバックグラウンドにあるとき |
| About | 実行ファイル /system/bin/app_process64、導入方法 管理元不明 |
| Known Hosts | /data/data/com.github.aida0710.sshc.dev/files/.ssh… |
| History | sshc/machines/14c85e…json、変更前のファイルは~/.ssh/sshc/backupsに残します |

{image("ux68-ssh-agent.png", "右端: SSH Keysのssh-agentの警告")}

{image("ux68-machines-about.png", "Machines・About")}

## 案

Androidのアプリ（`web/src/android/`で判定できる）では、ssh-agentの節・ポートの欄・CLIの例・サインアウトを出さない。言葉は「この端末」「アプリ」に置き換える（Androidの文言では「デバイス」、利用者の方針 L15）。パスは`~/.ssh`からの相対で出す。
""",
    },
    {
        "id": "UX-69", "kind": "デザイン", "priority": "低", "size": "小", "label": "enhancement",
        "title": "スマホ: 裏から戻るたびに「Vaultのロック状態を確認しています…」が数秒、全画面を覆う",
        "body": f"""## 何が起きるか

アプリを25秒ほど裏に回して戻すと、全画面に「保護された内容を再表示する前に、Vaultのロック状態を確認しています…」が3秒以上出ます（エミュレータの数字）。数秒で戻したときは出ません。Claude Codeの様子をこまめに見に戻る使い方では、毎回待たされます。

{image("ux69-vault-check.png", "戻った直後")}

## 案

確かめている間も前の画面を出したまま（内容はぼかすか、操作だけ止める）にし、確かめが終わるまでの間は小さな印にする。Vaultがパスワードなしのときは確かめを省く。待つ長さの内訳（エンジンの応答か、WebViewの再開か）は、直す前に測る。
""",
    },
    {
        "id": "UX-70", "kind": "不具合", "priority": "低", "size": "小", "label": "bug",
        "title": "スマホ: ターミナルの文字を選んでいる間にAndroidの戻るを押すと、選択を外さずにターミナルを離れる",
        "body": f"""## 何が起きるか

ターミナルの語を長押しすると選択とAndroidの帯（翻訳・コピー・共有・すべて選択）が出ます。この状態で戻るを押すと、選択を外すのではなくHomeへ移ります。

{image("ux70-selection.png", "長押しで選んだところ")}

## 直し方の案

選択があるときの戻るは、選択を外すだけにする（ダイアログや引き出しを先に閉じるのと同じ扱い。利用者の方針 L142）。
""",
    },
    {
        "id": "UX-71", "kind": "デザイン", "priority": "低", "size": "小", "label": "enhancement",
        "title": "スマホ: 押す場所が小さい（パネル・リストの切り替え、鍵の名前、「詳しく」、小さい画面のキーバー）",
        "body": f"""## 何が起きるか

44px未満の押す場所です（見出しの30pxのボタンは利用者の方針で決めたので除く）。

| 場所 | 大きさ |
|---|---|
| Homeの［パネル｜リスト］ | 57×26 |
| SSH Keysの鍵の名前（リンク） | 115×20 |
| Settings > Vault・Historyの「詳しく」 | 388×18 |
| Settings > Key barの並びの見本のキー | 31×40 |
| 360幅の画面のキーバーのキー | 30×44 |

{image("ux71-small-keybar.png", "360幅のターミナル。キーバーのキーが幅30px")}

## 案

押す場所を44pxにする（見た目の大きさは変えず、余白で広げる）。キーバーは幅が足りないとき、キーを減らすか横スクロールにする。
""",
    },
    {
        "id": "UX-72", "kind": "デザイン", "priority": "低", "size": "小", "label": "enhancement",
        "title": "スマホ: SFTPの「フォルダ操作」が19項目で、並べ替えも同じメニューに入る",
        "body": f"""## 何が起きるか

SFTPのパスの帯の「…」（フォルダ操作）は、全画面のメニューに19項目が並びます。作成・アップロード、移動（戻る・進む・ホーム・ルート）、選択、並べ替え（6項目）が混ざっています。「ディレクトリを更新」「ホームディレクトリ」など、ディレクトリの語も残ります（#651）。

{image("ux72-folder-menu.png", "フォルダ操作")}

## 案

並べ替えは一覧の見出し（または別の小さなメニュー）へ移し、フォルダ操作は作成・アップロード・ターミナルを開く・パスをコピーに絞る。
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
    return f"""スマホ版（Androidのアプリ）の点検です。利用者はスマホのsshcを主にClaude Codeの操作に使うので、Claude Codeの操作のしやすさを中心に見ました。#628（PCを含む全体の点検）の続きです。

対象は{SOURCE}、環境は{ENVIRONMENT}です。

## 結論

- **Claude Codeの操作で困るものが4つあります。** モードの切り替え（Shift+Tab）をスマホから打てません（{link("UX-60")}）。許可待ちになってもAndroidの通知が出ません（{link("UX-61")}）。回線が約40秒切れると再接続をあきらめ、SSHが切れてClaude Codeも終わります（{link("UX-62")}）。接続できなかったセッションに入力欄から送ると、依頼の文が消えます（{link("UX-64")}）。
- **最初の接続で止まる不具合があります。** 日本語のGboardでは、接続名・ホスト名の欄がかなで開き、接続先を作れません（{link("UX-59")}）。
- **うまくいったこと:** 日本語の依頼は入力欄からそのまま届きます。許可の確認は↓↑と空の送信で答えられます。Escで止めることと、Escを2回押す巻き戻しもできます。長押しで文字を選べ、回転しても入力の途中の文とセッションは保たれます。

## 一覧

{rows}

#638（空の状態）に、スマホのTerminalの空の状態が［新しいセッション］を案内するのにボタンが無いことを足しました。

## 調べ方

- エミュレータに開発版のAPKを入れ、指の操作（adb）とキーボード（Gboard）で動かし、WebViewをCDPで調べて撮りました。接続先は使い捨てのsshdのコンテナです。
- Claude Code 2.1.304をコンテナに入れ、作りもののMessages API（道具の呼び出しと長い答えを返すだけで、モデルには送らない）で動かしました。
- 画面の大きさは標準（412×915）、小さいスマホ（360×640）、横向きで、文字の大きさはAndroidの1.3倍でも撮りました。
- 比べた相手: Termiusのスマホ版の公式の文書（docs.termius.com/terminal/mobile-terminal）。

## 確かめていないもの

- 実機（Pixelなど）。記号の豆腐（{link("UX-67")}）は実機で出るか分かりません。
- 画面の分割（アプリを2つ並べる）、折りたたみを開いた形、文字の大きさの最大。
- iPhone・iPad、AndroidのChromeで開く形（利用者が対象をAndroidのアプリに絞った）。
- 実際のモデルとのやり取り（作りもののAPIで代えた）。
"""


def write_bodies(out: Path, summary_number: str | None) -> list:
    out.mkdir(parents=True, exist_ok=True)
    issues = []
    for item in ITEMS:
        footer = f"\n\n---\nスマホ版の点検（2026-11-03）の{item['id']}です。" + (f"まとめは #{summary_number}。" if summary_number else "")
        path = out / f"{item['id']}.md"
        path.write_text(item["body"].strip() + footer + "\n")
        issues.append({"id": item["id"], "title": f"{item['title']}（{item['id']}）", "label": item["label"], "body": str(path)})
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
    write_bodies(out, summary)
    for issue in issues:
        text = Path(issue["body"]).read_text()
        for item_id, number in created.items():
            if item_id != issue["id"]:
                text = text.replace(f"（{item_id}）", f"（#{number}）")
        Path(issue["body"]).write_text(text)
        gh("issue", "edit", created[issue["id"]], "--repo", REPOSITORY, "--body-file", issue["body"])
    gh("issue", "edit", summary, "--repo", REPOSITORY, "--body-file", str(out / "summary.md"))
    print("bodies and summary updated")


if __name__ == "__main__":
    command, place = sys.argv[1], Path(sys.argv[2])
    {"images": lambda: copy_images(place), "bodies": lambda: write_bodies(place, sys.argv[3] if len(sys.argv) > 3 else None), "post": lambda: post(place)}[command]()
