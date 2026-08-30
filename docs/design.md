# 設計方針

この文書では設計判断と、その判断が適用される境界を説明します。使用方法は
[README](../README.md)、インストール方法は [release-install.md](release-install.md) を参照してください。

各節に、機能の対象範囲と対象外の範囲を記載します。

## セキュリティ境界

- HTTP サーバーは IPv4 の `127.0.0.1` だけに bind します。LAN、Tailnet、コンテナ外部など、ネットワークへ公開して安全な設計ではありません。
- このアプリケーションは利用者の `~/.ssh` を読み書きし、鍵を生成し、埋め込みターミナルからリモートホストへ接続します。それぞれの境界は以下の各節で説明します。
- cookie 単体ではリクエスト元を検証できません。cookie はポートを区別せず、`SameSite` の site 判定にもポートは含まれないため、同じブラウザで `http://127.0.0.1:<別ポート>` を開くと session cookie は別ポートにも送信されます。そのため、読み取りを含むすべての API リクエストに `X-SSHC-CSRF` を要求します。このトークンは port を含む origin ごとの `sessionStorage` に保持され、別ポートには送信されません。例外は 2 つです。`POST /api/v1/session/bootstrap` は最初のトークンを発行するため、代わりに `Origin` の完全一致を要求します。`GET /api/v1/health` も例外です。`POST /api/v1/session/renew` は `sessionStorage` に残った現在のトークンを検証してから新しいトークンを発行します。
- bootstrap、session、CSRF の値をログへ出してはいけません。bootstrap は URL fragment に置き、ブラウザが直ちに履歴から除去します。
- 同一マシン上の悪意あるプロセス、侵害されたブラウザ、ブラウザ拡張から秘密を完全には保護できません。将来の秘密鍵 reveal/copy 機能でも、ブラウザ拡張やローカルのクリップボード監視・履歴ツールに対して秘密は脆弱です。
- UI は埋め込みファイルシステムからのみ配信し、URL を OS ファイルパスへ変換しません。存在しない API は SPA へフォールバックしません。

## SSH config エンジンの境界

- `~/.ssh/config` と `Include` 先を構成情報の保存元として読み書きします。無変更の parse/render は byte-for-byte で一致し、コメント、空行、引用、`key=value`、未知のディレクティブを保持します。
- 解釈できない行は `LineUnstructured` として原文のまま保持し、UI からは Raw 編集だけを許可します。推測による整形や削除は行いません。
- 書き込みは解決済みの `~/.ssh` 配下だけに限定します。`..`、シンボリックリンク、外部パスで書き込み範囲は広がりません。読み取りは `O_NOFOLLOW` を使います。
- `Include` が `~/.ssh` の外を指す場合は、グラフ表示と読み取りのみ許可します。
- `%h` など接続先が決まるまで確定しないトークンは展開せず、`include_unsupported_expansion` として報告します。
- 変更は `~/.ssh/sshc/journal/` に予定を記録し、全ファイルを一時ファイルへ書き出して fsync した後に atomic rename します。中断した場合は `~/.ssh/sshc/backups/<id>/` の世代バックアップから復旧するか、staged 内容で完了させるかを選べます。
- 完了した変更は `~/.ssh/sshc/history/` に記録します。バックアップは自動削除しません。
- 複数ファイルの OS レベル完全 atomic commit は存在しないため、部分適用は隠さず pending として提示します。
- ディレクトリ構成要素の入れ替えに対する time-of-check/time-of-use 競合は best-effort でしか防げません。`O_NOFOLLOW` と構成要素ごとの検査を行いますが、同一ユーザー権限で動作する悪意あるプロセスからは完全には保護できません。
- このエンジンは `/api/v1/config/*` として同一オリジンの HTTP API に公開済みです。境界は次節を参照してください。

## Serial／Telnet CLI の境界

- SerialとTelnetはOpenSSH configへ保存せず、一回限りのCLI接続として扱います。独自の保存接続先model、vault、engine、Web API、同期snapshotへは加えません。
- 対話接続はローカル端末とbyte streamを直接結び、`Ctrl+]`を必ずローカル切断として消費します。SerialはOSのdeviceを排他的に開き、context取消と終了の両方で閉じます。TelnetはIACをapplication dataから除き、BINARY、ECHO、Suppress Go Ahead、Terminal Type、NAWSだけを交渉し、未知optionは拒否します。BINARY成立前はNVTのCR LF／CR NUL規則を適用し、成立後だけbyte列をそのまま渡します。
- SerialとTelnetの`--non-interactive`にはremote processの終了statusがありません。終了条件はRE2による`expect`または明示した`readFor`であり、timeout、step数、pattern、送信、transcript、subnegotiationを固定上限で制限します。`readFor`の0 byte成功を許容しつつ、応答必須の呼出元は`--require-output`で`no_output`失敗にできます。送信と受信がblockしてもcontextまたはtimeoutで呼び出し元へ制御を返し、streamを閉じます。
- AIなどの呼び出し元はversion付きJSON script／結果を利用できます。invalid UTF-8はbase64で返し、環境変数から送ったsecretは完全一致byte列をtranscriptからmaskします。引数やscriptのliteralへ秘密を書いた場合は保護できません。scriptの`onFailure`は明示された送信だけをmain failure後に独立した最大5秒のtimeoutで試み、成功時には送信しません。装置非依存のCtrl+Cやpager解除は定義できないため自動推測せず、結果には送信内容を含めずattempt成否だけを返します。
- Telnetは平文でserver認証もないため、SSHと同じ安全性を示しません。接続時の警告はこの性質を変えず、信頼できないnetworkで資格情報を送る用途は対象外です。
- desktopのSerial backendはLinux、macOS、Windowsを対象とします。採用driverがflow control設定を公開しないため現時点ではnoneだけを実装し、RTS/CTSとXON/XOFFは黙って無視せず拒否します。Android USB serialはUSB Host permissionとnative driverが必要なため対象外です。
- 自動検査ではLinux PTYを本番Serial driverで開く仮想routerと、loopback TCP上でIAC交渉する仮想Telnet serverを使います。parserからtransportまでの高速testに加え、build済み`sshc` processのargv、標準出力、終了codeをintegration testで通します。PTYは電気的baud、USB descriptor、DTR／RTS／Break、物理抜線を再現しないため、これらは実機acceptanceから外しません。

## Connections UI とグループの境界

- 起動直後の Home はホスト中心のランチャーです。具体的な alias を検索し、グループ、タグ、最近の接続順を見ながらコンソールを開けます。Home を表示しただけでは DNS、TCP、SSH のどれも開始せず、「接続」を選んだときだけ埋め込みターミナルが開きます。
- Command Paletteはdesktop toolbarまたは`Ctrl/Cmd+K`から開き、現在読み込まれているhostとSSH設定file、保存済みsnippet、設定sectionを一時的なqueryで横断検索します。queryはURL、localStorage、snapshotへ保存しません。hostだけは選択時に接続を開始し、file、snippet、設定は該当画面へ移動します。mobileの入口は常設footerではなくdrawer内に置きます。
- Home は設定上の警告、中断した変更、同期状態を要約しますが、それらの編集機能は持ちません。詳細操作は Connections、Diagnostics、History、Sync の各専用画面で行います。

- フォーム編集、任意キー・値編集、ブロック Raw 編集、ファイル全体 Raw 編集は、すべて `~/.ssh/config` と `Include` 先から構築した同じ lossless 構文木を更新します。変更していない行は 1 バイトも書き換えません。
- 保存は必ず「読み込んだ内容」を base として送り、その SHA-256 を precondition にします。外部変更があった場合は書き込まず、三者差分を表示します。
- 保存前に再パースと Include グラフの再解決を行い、新しい構文エラーや Include エラーを生じる変更は拒否します。既存の問題だけで保存を拒否することはありません。
- UI 専用情報は `~/.ssh/sshc/metadata.json` に保存します。対象はスキーマバージョン、グループの表示情報（色、メモ、表示順、共通設定）、タグ、色、表示順です。鍵本文とパスフレーズは保存しません。Host とグループの対応は metadata ではなくディレクトリ構造で管理します。
- Host のコメントは metadata ではなく `Host` 行の直上のコメント行として設定ファイルに書きます。sshc を使わずにファイルを読む人にも残るためです。付随するコメントは `Host` 行の直上に連続するコメント行だけで、空行・ディレクティブ・ファイル先頭で切れます。空行で切るのは、ファイル冒頭の見出しコメントが最初の Host ブロックに取り込まれないようにするためです。
- 旧来の metadata の `note` は廃止しました。コメントが未設定のブロックでは編集欄の初期値として表示され、保存すると設定ファイルへ書き出されて metadata からは消えます。
- Host の識別は「正規化した相対パス + 具体的な主 alias」です。改名は config と metadata を同一トランザクションで更新し、対応先が消えた metadata は推測で付け替えず orphan として再関連付けを求めます。グループはディレクトリなので、グループを変えるとパスが変わり、識別も変わります。移動は config と metadata を同一トランザクションで更新するため、識別が変わっても orphan にはなりません。
- Host ブロックの別ファイルへの移動、グループディレクトリ間のファイル移動、グループの改名に対応しています。journal 対応の `storage.Move` と `storage.Removal` は鍵の trash 処理でも使用します。Config Explorer ではディレクトリを作成し、空のディレクトリだけを削除できます。宣言済みグループの削除は `group_is_declared` で拒否し、Groups 画面へ案内します。グループの改名と削除は領域、共通設定、metadata、鍵を 1 トランザクションで更新するため、操作箇所を Groups 画面に集約しています。ディレクトリ自体の改名には対応していません。各ファイルを参照する Include 行を複数ファイルにわたって 1 トランザクションで更新する必要があるためです。ディレクトリの作成と削除も `storage.Request` の `Directories` と `RemoveDirectories` として同じトランザクションに含めます。空ディレクトリの判定にはリクエスト適用後の状態を使うため、同じトランザクションで空にしたディレクトリを削除できます。グループ改名はファイル単位の move として処理し、空になったディレクトリを深い順に削除します。操作前から残っていた未宣言のサブディレクトリなどは削除せず、`group_directory_leftover` として報告します。
- グループは `~/.ssh/connections/<group>/` ディレクトリです。`~/.ssh/config` には生成マーカーで囲まれた領域を作り、宣言されたグループごとに `Include connections/<group>/*.conf` を 1 行ずつ、深い子グループから先に並べ、最後に `Include groups.sshc.conf` を置きます。単一のワイルドカードにしないのは、`*` が `/` を跨がないため入れ子グループに届かず、読み込み順が glob の辞書順という偶然に決まってしまうためです。マーカー間の行は次回保存時に置き換えられ、その外側は 1 バイトも変わりません。
- **この領域は最初の `Host`／`Match` 行より上に置きます。** `Include` も他のディレクティブと同じくそれが書かれたブロックに属し、OpenSSH は取り込んだファイルの値をそのブロックが一致したときにしか適用しません（パース自体は常に行うため `-v` には `Reading configuration data` が出ます）。無条件に読まれるのは最初のブロックヘッダより上の行だけなので、宣言はそこにしか置けません。既存の領域が `Host` ブロックの内側にある場合は、その場で置き換えず上へ移動します。結果として、グループのファイルはエントリファイル自身の Host ブロックより先に読まれます。同じ alias を両方に書いた場合はグループ側が勝ちますが、それは位置から推測せず、解決済みグラフが `duplicate_alias` として双方の行を示します。
- グループ名は `connections/` からの相対ディレクトリパスで、パスが親子関係を表します（`work/eu` は `work` の子）。`connections/` 配下のディレクトリでも、`Include` 行に宣言がなければグループとして扱いません。この状態は `group_not_declared` として報告します。宣言済みディレクトリが存在しない `group_directory_missing` とともに `/api/v1/config/overview` に含め、Groups 画面に表示します。これらは Connections 画面から修正できないため、同画面には表示しません。宣言済みで空のグループは `group_empty` として報告しますが、警告表示は行いません。作成直後や最後の接続を移動した後には正常に発生し、OpenSSH も一致しない `Include` をエラーにしないためです。Groups 画面では「メンバー: なし」と表示します。グループの共通設定は `groups.sshc.conf` に通常の `Host` ブロックとして生成し、子グループを親より先に配置します。
- 鍵は `~/.ssh/keys/<group>/` に置けます。鍵の改名・移動、グループの改名では、Include グラフが到達する範囲の `IdentityFile` と `CertificateFile` の行を同一トランザクションで書き換えます。解決できないパス、`~/.ssh` 外の設定ファイルからの参照、Include が設定として読んでしまう移動先、宣言されていないグループは、いずれも半端に適用せず操作そのものを拒否して理由を返します。
- ワイルドカード、否定パターン、`Match`、alias 重複により単純な継承として表現できない場合も、実効値は計算します。複雑な解決であることを示す印と出所を表示します。
- Effective タブと Diagnostics タブは、この解決器が決めた値とその出所を出します。`Match exec` と `CanonicalizeHostname` を含む設定については値を出さず、理由を出します。
- 実効値はこのアプリケーションの解決器が計算し、`ssh -G` には委ねません。これにより、設定の表示時に外部コマンドを実行せず、`ssh` がない環境でも一貫した値を表示できます。一方、OpenSSH 自身の解決結果を UI から直接確認する機能と、`Match exec`、`CanonicalizeHostname`、`Match final` を含む設定の解決には対応しません。`~/.ssh/config` はそのまま保持するため、これらの設定は端末から `ssh` で利用できます。
- 既定値を持つのは `HostName`（alias 自身）、`User`（ローカルのアカウント名）、`Port`（`22`）の 3 つだけです。OpenSSH の全既定値は、バージョンとビルドによって変わるため複製しません。`IdentityFile` は macOS と Linux で既定値の順序が異なることを差分試験で確認したため、既定値を設定しません。設定にないキーワードは返しません。
- 各セキュリティ機構の目的と、不要になる条件を次に示します。

  | 守り | 何から守るか | いつ要らなくなるか |
  | --- | --- | --- |
  | ループバックの TCP listener | 他の機械から届くこと | 画面と CLI が別経路になったとき |
  | bootstrap トークン | 最初のセッションを、残る場所に置かずに画面へ渡す | 残ります |
  | セッション cookie | どのウィンドウからの要求かを言えないこと | 残ります |
  | CSRF ヘッダー | **cookie がポートに紐づかないこと。** `127.0.0.1` はそれ自体が site なので、同じアドレスの別のポートで動くサーバーがこの cookie を受け取ります。token は port-origin ごとの `sessionStorage` にあってそこへは渡らないので、漏れた cookie 単体を無価値にします | 画面が自分のものだけになり、そこから他の `127.0.0.1` へ移動できないと言い切れるとき |
  | handoff の秘密 | `/cli/connect` に誰でも来られること | CLI が別経路になったとき |
  | ストリームのチケット | WebSocket のアップグレードにヘッダを付けられないこと | 残ります |
  | 一回限りの action token | 確認から実行までの間に対象が変わること | 残ります |

  条件が変わった場合は、対応するセキュリティ機構の必要性を再評価します。

- API は同一オリジンのみです。CORS は有効化せず、bootstrap と health を除く**全ての登録済み `/api/` 要求が**（読み取りと session renew も含めて）`X-SSHC-CSRF` header を要求し、`/api/` 応答は `Cache-Control: no-store` を返します。エラー応答は安定コードと位置情報のみを含み、設定本文は返しません（利用者が解決すべき競合差分を除く）。

## 見た目の境界

- 外観はライトとダークの 2 つで、既定は OS に従います。ヘッダーの「外観」で上書きでき、選択は `localStorage` の `sshc.theme` に記録します。三択（システム／ライト／ダーク）なのは、二択だと OS 設定から出られても戻れないためです。ブラウザの永続ストレージへ書くのはこれと `sshc.language` の 2 つだけで、`e2e/bootstrap.spec.ts` がその 2 つ以外が現れたら落ちる allowlist で守っています。
- 色は 20 個のトークンとして `web/src/index.css` に 1 テーマにつき 1 組ずつ置きます。コンポーネントは `bg-card` のようにトークン名で参照し、`dark:` variant は 1 つも書きません。値の名前は用途であって色ではありません（`card` には別テーマで別の値を与えられますが、`zinc-800` には与えられません）。
- 色は状態の表示に使用します。accent はセクションの主要操作、琥珀は注意、赤は失敗と破壊的操作、緑はローカルセッションが有効であることを示します。選択中の行やタブには色を付けません。
- カード外周は子要素に隠れるinset shadowではなく、背景との差を抑えた`--ui-line`の実borderで描きます。外周と区切りは面の境界が分かる最小限の強さにし、操作対象の`--ui-control-line`より明確に弱くします。大きなsurface、dialog、選択グループは12px、inputとbuttonは8px、小さな補助要素は6pxを基準にします。表の内部や連続したrowには角を付けず、接続状態badgeなど意味のあるpillだけ`rounded-full`を使用します。分割paneの左右headerは同じ最小高さとpaddingに揃えます。
- ConnectionsのBasic、Settings analysis、Advanced settingsは、tablistと選択中tabpanelを同じ外枠へ入れます。tabpanelを内側へpaddingし、タブから離れた別ブロックに見せません。各tabの`aria-controls`とtabpanelの`aria-labelledby`を対にします。
- accent は画面単位ではなくセクション単位で 1 つ使用できます。Sync のバケット設定とスナップショット送信、Keys の鍵作成・agent 登録・移動・パスフレーズ変更は、それぞれ異なる目的のセクションです。2 つの accent が同じ結果を生む場合は、操作の分割またはラベルを見直します。実例として、「保存済みパスワードを使う」と「新しいパスワードを保存する」の両方が「パスワード」と表示されていた問題は、操作名を明確にして解決しました。
- 意図的な例外が 2 つあります。コントロールのフォーカスリングと、チェック済みチェックボックスの印です。どちらも一過性で、どちらも広く定着した慣習で、どちらも「画面が何のためにあるか」ではなく「利用者が今どこにいるか」を指します。
- パレットの規則は `web/src/ui/palette.test.ts` で検査します。Tailwind のパレット名（`text-red-400` など）、任意値（`text-[#ff0000]`）、inline style の hex 値を走査し、違反箇所をファイル名と行番号付きで報告します。例外は `palette-exempt` を記載した行だけで、現在は native の色入力が独自の既定値を必要とする 2 行に使用しています。
- ローカルシェルの開始ディレクトリは、設定画面の「ターミナル」で変更できます。既定値は home です。エンジンの作業ディレクトリは、エンジンの起動方法によって変わり利用者が指定した値ではないため継承しません。`~/work` のようなパスはそのまま保存し、home の絶対パスに変換しません。存在しないディレクトリは保存時に拒否します。保存後にディレクトリが削除された場合は、シェルを起動できるよう home を使用します。
- **端末で範囲を選ぶと、選択を終えた時点でシステムのクリップボードへコピーします。** 右クリックは貼り付けです。どちらも既定は on で、設定画面 →「ターミナル」から個別に止められ、開いている端末にもすぐ反映されます。右クリック貼り付けと Cmd+V / Ctrl+Shift+V は xterm の paste 経路を通すため、接続先が bracketed paste mode を有効にしていればその印を付けます。**ただし、貼り付ける文字列が安全になるわけではなく、対応しないプログラムでは改行を含む内容がそのまま実行されることがあります。** Cmd+C（macOS 以外は Ctrl+Shift+C）による明示コピーも残します。**素の Ctrl+C はそのまま SIGINT として向こうへ渡します。** xterm の選択はブラウザの選択ではないので、これらは画面側で処理します。
- 開いているコンソールは左側のナビゲーションに表示します。`Home`、`Connections`、`SFTP`、`Terminal` を上部に固定し、その下の領域を「設定」と「ターミナル」で切り替えます。端末は接続一覧の隣ではなく、専用画面に表示します。「設定」側には従来のセクション一覧を表示します。既定はターミナル側ですが、セッションが 0 件の場合は設定側から開始します。セッションはプロセス終了時に失われるため、常にターミナル側から開始すると起動直後に空の一覧を表示することになるためです。切り替え状態は保存せず、起動時にこの規則で決定します。モバイルdrawerは40pxの操作行を維持しつつ、外周、group間、見出しの余白をdesktopより小さくして、800px高で保守項目まで到達しやすくします。
- コンソール一覧は状態別にグループ化しません。各項目は 2 行で、1 行目に名前、2 行目に「状態 · 接続先」を表示します。種類（SSH / シェル）は、接続先が `localhost` かどうかで判別できるため表示しません。終了した項目も元の位置に残し、接続失敗の詳細を見つけやすくします。
- 各項目の `···` メニューから、名前変更、同じ接続先への新規接続、上下移動を実行できます。ドラッグによる並べ替えに加え、キーボード操作を可能にするためメニューにも上下移動を用意します。名前と並び順はセッション固有であり、`metadata.json` には保存しません。
- 右側のインスペクタは Connections と Groups の 2 画面で使用します。Connections では `~/.ssh/config` に保存するグループ、コメント、alias を主画面に置き、`metadata.json` にだけ保存する色、タグ、表示順と、警告・継承元をインスペクタに置きます。Groups でも同様に、色、表示順、Connections での表示設定をインスペクタに置き、改名、削除、子グループ追加、共通設定を主画面に置きます。行を選択すると対象をインスペクタに表示します。他の 8 画面ではインスペクタを使用しないため、開閉ボタンも表示しません。
- インスペクタは既定で閉じており、開閉状態はセクションをまたいで保持します。中身に注意がある時だけ開閉ボタンに琥珀の印が付き、その印は読み上げにも「確認が必要な項目があります」として届きます。印が無ければ開ける価値が無いことを意味します。
- インスペクタにキーボードショートカットはありません。macOS の慣習は ⌥⌘I ですが、⌥⌘I は Chromium が開発者ツールとして先に取ります。
- 主ナビゲーションのグループ見出しは見出し要素ではなく `aria-label` 付きのリストです。Playwright はアクセシブル名を既定で部分一致させるため、`鍵とホスト` という見出しがあると `鍵` を指す既存の検索が 2 件に一致して落ちます。見出しの名前空間はパネルのものです。

## 鍵管理の境界

- `~/.ssh` 配下のファイルは内容と権限で分類します。ファイル名だけで秘密鍵と断定しません。`keys/<group>/` 配下も同様に走査します。走査の深さ上限は `~/.ssh` から 8 段（`keys.maxScanDepth`）なので、グループ名は 6 階層までに制限しています。`~/.ssh/sshc/`（backups、trash、journal、history）は走査対象、agent 登録対象、config 候補のいずれからも除外します。
- 通常のソフトウェア鍵（Ed25519、RSA、ECDSA）は Go プロセス内で生成・暗号化します。パスフレーズは argv にも環境変数にも載せません。
- FIDO の `ed25519-sk` と `ecdsa-sk` はハードウェアの操作が必要なため生成しません。実行すべき `ssh-keygen` コマンドだけを表示します。
- 生成可能なアルゴリズム一覧は、インストール済み OpenSSH の対応状況ではなく、このアプリケーションの実装に基づいて返します。通常の鍵は x/crypto で生成するためです。`TestEveryInProcessVariantCanActuallyBeGenerated` は一覧のすべてを生成できることを検査します。FIDO 鍵は `ssh-keygen` が必要なため、コマンドが見つかる場合だけ表示します。
- パスフレーズ変更は既存の秘密鍵を置き換えます。以前は平文の鍵を `~/.ssh/sshc/backups/` に残さないためバックアップを作成せず、`Rollback` も拒否していました。現在は世代バックアップをマスターパスワードで暗号化するため、パスフレーズ変更もバックアップ対象です。入力を誤ると鍵を復号できなくなるため、特に復元機能が重要な操作です。
- 削除は `~/.ssh/sshc/trash/<entry>/` への `rename` です。バイト列を複製せず、元の権限をそのまま保ちます。復元先が埋まっている、または同一 fingerprint の鍵が既に存在する場合は推測せず blocker を提示して拒否します。完全削除はバックアップを取らないため取り消せません。
- 秘密鍵の表示と完全削除は、session cookie と `X-SSHC-CSRF` に加えて一度限りの確認 token（`X-SSHC-Action`）を要求します。token は「確認ダイアログが表示していた内容」の digest に関連付けられます。digest はサーバ側で発行時と使用時の両方で計算するため、確認から実行までの間に鍵が差し替わった場合、その token は無効になります。
- 保存済み資格情報は対応する認証要求にだけ使用します。秘密鍵のパスフレーズは選択された鍵に使用します。アカウントパスワードは password 認証と、keyboard-interactive のうちプロンプトが 1 件で入力を表示しない場合に使用します。一般的な Linux サーバーは keyboard-interactive でパスワードを要求することがあるため、この条件を含めます。複数プロンプトの 2FA や入力を表示するプロンプトには使用しません。保存値が拒否された場合は利用者に入力を求めます。試行上限は方式ごとに 3 回で、同じ値は再送しません。
- 保存値で処理できない認証要求は接続元の端末に表示します。対象は未知のホスト鍵、未保存の鍵パスフレーズ、パスワード、2FA です。4 種類とも端末への表示と入力の読み取りという共通経路を使用します。入力値は端末に再表示しません。端末のない認証テストと公開鍵のリモート登録では、対話が必要な認証方式を提示しません。
- マスターパスワードの検証失敗後は、試行ごとに応答を遅延させます（上限 4 秒）。vault ファイルに対するオフライン攻撃への耐性は Argon2id が提供します。この遅延は、実行中アプリケーションに対する高速なオンライン試行を制限するものです。
- 鍵導出の同時実行数は 2 つまでです。1 回あたり数十 MiB と複数 thread を使用し、解錠・push・pull のいずれでも実行されるためです。上限がない場合、複数タブから数 GiB のメモリを確保できます。
- リモートスナップショットには、ローカルデータとは別のパラメータ上限を適用します。外部から受け取った値によって、パスフレーズ検証前に 1 GiB のメモリと 16 thread を使用しないようにするためです。
- 秘密鍵の表示応答は `Cache-Control: no-store` で返し、鍵本文はログ行にも history にもエラー応答にも出しません。history には「表示した」という事実と対象パスのみを記録します。
- 画面は表示した鍵本文をコンポーネント状態にのみ保持し、ダイアログを閉じた時点で破棄します。`localStorage`、`sessionStorage`、グローバルオブジェクトのいずれにも書きません。再表示には新しい確認 token が必要です。
- ssh-agent への登録には `SSH_AUTH_SOCK` の agent プロトコルを直接使用し、`ssh-add` は起動しません。鍵はプロセス内で復号し、復号済み鍵を agent に渡します。これにより、パスフレーズを子プロセスの標準入力に渡す必要がありません。
- パスフレーズを保存するのは、利用者がマスターパスワードを設定した場合だけです。ssh-agent への委譲は従来どおり利用者の明示的な操作で、既定では何も保存しません。保存する場合の境界は次節にまとめます。

## アプリケーションの施錠と秘密の保管庫の境界

- 初回起動ではマスターパスワードの設定が必須で、解錠するまで UI を利用できません。`/api/v1` は `vault_locked` で拒否します。例外は解錠前に必要な vault 自身の route、`GET /api/v1/health`、`POST /api/v1/session/bootstrap`、`POST /api/v1/session/renew` です。この制御は middleware に集約し、`Unlocked` が未設定の server は常に施錠状態とします。
- 保管庫は実行環境に関係なく、最後の使用から 12 時間で自動施錠します。以前はデスクトッププロセスの終了時に engine も終了する設計だったため、デスクトップではタイムアウトを無効にしていました。現在はブラウザを閉じても engine が動作し続けるため、同じタイムアウトを適用します。12 時間は同日中の再入力を避け、翌日には再認証を求める設定です。状態取得は使用として扱いません。タブを開いたままにするだけでタイムアウトが延長されるのを防ぐためです。
- この認証は `~/.ssh` 自体を暗号化しません。config と鍵は OpenSSH が読み取れる形式で保存されます。保護対象は UI、vault の内容、世代バックアップであり、ディスクへアクセスできる攻撃者は対象外です。

- 保管庫は `~/.ssh/sshc/secrets` の 1 ファイルです。AES-256-GCM で暗号化し、鍵はマスターパスワードから Argon2id で導出します。マスターパスワードは保存も送信もしません。復旧経路もないため、忘れた場合は内容を復元できません。
- 名前空間は 2 つで、混ざりません。**アカウントのパスワード**（リモートホストへログインするための秘密）と**鍵のパスフレーズ**（ローカルの秘密鍵を開くための秘密）です。ファイル形式、API、画面のいずれでも片方の名前がもう片方の選択肢に現れることはありません。取り違えると、鍵のパスフレーズを次の接続でリモートホストへログインパスワードとして送ることになるためです。
- 秘密には名前を付け、複数のホストや鍵から参照します。値は 1 か所にあるので、更新は 1 回の編集で済みます。まだ参照されている秘密の削除は `credential_in_use` として拒否します。
- 通常の一覧 API は名前と参照元だけを返します。名前付き資格情報の編集では、編集を明示した 1 件に限り、現在値のダイジェストへ結び付いた一度限りの action token を消費する専用 API が値を返します。応答は `Cache-Control: no-store` とし、一覧・更新応答・ログ・browser storage へ値を複製しません。名前と値、全参照の追従は 1 回の vault transaction で保存します。
- 起動時にはマスターパスワードを要求せず、必要な画面で要求します。解錠後は、明示的に施錠した場合、engine が終了した場合、または 12 時間使用されなかった場合に施錠します。画面表示時の状態取得は使用として扱いません。
- `~/.ssh/sshc/backups/` の世代バックアップは、設定ファイル、秘密鍵、vault のいずれもマスターパスワードで暗号化します。`storage.Manager` には暗号化関数だけを渡し、storage 層は秘密の形式を扱いません。読み取りは `ReadBackup` に集約し、rollback と履歴からの復元もこの経路を使用します。
- vault のschema更新は `internal/secret/migration.go` のregistryで `N → N+1` を一段ずつ行います。`SchemaVersion` を上げるcommitには、直前versionをkeyとする純粋なJSON変換と、その旧版fixtureを必ず含めます。全段階の変換と現行schemaによる厳密な再読込が成功するまでdiskも実行中vaultも変更しません。成功時は元の暗号文を暗号化済み世代backupへ残し、journal付きtransactionのcommit point後にだけ新しいvaultを公開します。失敗時は `vault_migration_failed` と失敗した版の組を返し、内部原因や秘密は応答へ含めません。新しいschemaから古いschemaへのdowngradeは行いません。
- そのため **rollback と履歴からの復元にはマスターパスワードが要ります**。アプリケーション自体がその後ろにあるので、実際には常に開いています。
- スナップショットは、既定ではバケット直下の `workspace.tar.gz.enc`、`path` 設定時はその配下にある 1 つの固定キーへ条件付きで書き込みます。暗号化前の形式は tar.gz です。初回は `If-None-Match: *`、以降は `If-Match: <前回の ETag>` を使用し、他のマシンによる push の上書きを防ぎます。条件付き書き込みの対象を一意にするため、ライブオブジェクトには日付付きキーを使用しません。
- push ごとに `snapshots/YYYY-MM-DD-HHMMSS-<origin>-<snapshot>-<sequence>.tar.gz.enc` へ日付付き候補を保存します。同じ秒の複数送信やプロセス再起動でも衝突しないよう、送信元、暗号文のdigest、プロセス内sequenceを含めます。候補を先に書き、その後ライブオブジェクトへ条件付きで書き込むため、ライブオブジェクトが部分更新されることはありません。ライブの条件付き書き込みが競合で拒否された場合は、その試行が作った候補だけを削除します。通信断などでライブ書き込みの成否を確定できない場合は証拠を失わないため候補を残し、次の周期のHEAD確認で再送信を止めます。成功したpushの履歴数にアプリケーション側の上限はなく、古いスナップショットには後から更新・削除した鍵も残ります。必要なら`snapshots/`にバケットのlifecycle ruleを設定してください。
- S3 の資格情報はマスターパスワードで暗号化し、`~/.ssh/sshc/sync-settings` に保存します。同期は `~/.ssh` 配下の通常ファイルを相対パスのまま再帰的に収集するため、ルート直下に手動配置した秘密鍵も対象です。一方、バケットへのアクセス情報、端末固有の状態、journal・backup・history・trash、一時ファイルは明示的に除外し、symlink・socket・FIFO・device は追跡も転送もしません。`TestASnapshotTravelsBetweenTwoMachines` がルート鍵を含む往復を、`TestASnapshotCarriesTheVaultAndNotTheKeyToItsOwnBucket` が除外を検査します。
- スナップショットはマスターパスワードではなく同期専用鍵で暗号化します。既定では 120 ビット、Crockford base32 で 24 文字の鍵を生成し、利用者が指定することもできます。鍵は `sync-settings` に保存し、平文では作成時の応答に一度だけ含めます。端末ごとに異なるマスターパスワードを使用できます。過去の暗号化方式を変換する専用操作は提供しません。同期は空の保存先または現行schemaだけの保存先から始め、force pushは現行schemaの別世代を、利用者が確認したETagに対して条件付きで置き換える場合だけに使います。
- 保管庫は同期元で復号し、スナップショット内の `sshc/secrets.json` として格納し、同期先でその端末のマスターパスワードを使って再暗号化します。保管庫ファイル自体を転送すると、同期先でも同期元のマスターパスワードが必要になるためです。アーカイブ全体は同期専用鍵で暗号化します。空の保管庫は同期せず、2 台目の最初の pull で不要な競合が生じないようにします。マニフェスト v5 は内容から検証できるrevision ID、親revision ID、commit messageを持ち、日付付き暗号化スナップショットをGit風の履歴として辿れます。読み取りはv5だけを受け付け、過去schemaの導出・変換・移行は行いません。
- 履歴画面は現在のlive objectと新しい順の履歴50件を端末内で復号し、HEAD、ancestor、branchに分類します。暗号文の取得は合計128 MiBで打ち切り、APIにはファイル内容を返さず、選択版とHEADの追加・変更・削除パスだけを返します。過去版の復元はまず通常のpull previewを表示し、適用後もlive objectのETagを保持します。次のpushは選択したrevisionを親とする新しいHEADを条件付きで作るため、リモートを無条件に巻き戻しません。
- 自動同期は保管庫が解錠されている間だけ 1 分ごとに実行します。自動同期による読み取りは使用として扱わず、12 時間の自動施錠を延長しません。施錠後は利用者が再度解錠するまで停止します。各周期はまずライブオブジェクトをHEADで確認し、リモートにもローカルにも変更がなければアップロードもダウンロードもしません。リモート変更、競合、削除を伴う適用では自動送信を停止し、利用者の判断を待ちます。送信専用設定でもリモート世代を確認し、動いていれば履歴候補を作る前に停止します。通常の手動pushもローカル差分がなければ拒否します。同期先はendpoint、bucket、region、object keyの組で識別し、同名objectを使う別bucketへ旧ETagを流用しません。自動同期の有効・無効は保管庫に保存するため、次回起動時にも同じ設定を使用します。
- 受信専用の端末でライブスナップショットが既知のrevision系譜から外れた場合、自動受信は安全のため停止します。利用者は現在のライブスナップショットの作成時刻、送信元、変更パスをプレビューした後に限り、そのHEADを明示的に採用できます。適用時には再取得したETagとrevisionがプレビュー時の値に一致することを検証し、途中で世代が変わっていれば何も書きません。この系譜回避は受信専用に限定し、双方向・送信専用・自動同期には適用しません。
- 競合時は「このマシンを残す」または「他のマシンを取る」を選択できます。前者は現在のファイルを維持し、次回 push でリモートを更新します。後者はローカルを置き換えますが、バックアップを残すため History から復元できます。どちらも適用前に同じプレビューを表示します。
- pull で削除するファイルもバックアップします。削除を含む pull は、バックアップ先を示す確認文への同意がなければ適用しません。

## SSH 実行の境界

- 埋め込みターミナルの SSH 接続はプロセス内で実装します。外部の `ssh` と PTY は使用せず、SSH channel を端末入出力として使用します。接続値はこのアプリケーションの解決器から取得し、設定ファイルを一時保存して `ssh -F` で再解決することはありません。`creack/pty` はローカルシェルだけで使用します。
- 埋め込みターミナル、`sshc ssh <接続先>`、認証テスト、公開鍵のリモート登録、ホスト鍵の取得は、外部の `ssh` を起動せずプロセス内で実行します。例外として、利用者が設定した `ProxyCommand` は実行します。プロセス内実装への移行に伴い、`SSH_ASKPASS` ヘルパー、`/askpass` endpoint、ワンタイムトークン、プロンプト文字列照合、関連する 5 個の環境変数除去処理を削除しました。
- `ProxyCommand` は接続経路の一部として実行します。この方針は、当初の「外部コマンドを実行しない」という判断を変更したものです。

  当初は `ProxyCommand` を拒否し、必要な場合は端末から `ssh` を利用する方針でした。

  しかし、`cloudflared access ssh`、`aws ssm start-session`、組織固有の bastion helper など、`ProxyCommand` を必要とする設定は一般的です。接続一覧には表示される一方、実際には接続できない状態になるため、方針を変更しました。

  現在は次の条件で実行します。

  - 接続時に、実行する ProxyCommand を端末へ 1 行表示します
  - 利用者の `~/.ssh/config` に書かれたコマンドだけを実行します。`%h`、`%p`、`%r`、`%n` を展開し、POSIX では `/bin/sh -c "exec ..."`、Windows では `cmd.exe /c` を使用します
  - `$SHELL` は使用しません。engine の起動元である tmux や systemd の環境変数に接続動作が依存しないよう、POSIX では常に `/bin/sh` を使用します
  - `ProxyJump` と `ProxyCommand` の同時指定は `inconsistent options: ProxyCommand+ProxyJump` で拒否します
  - jump host 経由で到達する先では `ProxyCommand` を使用できません。コマンドはローカルマシンで実行され、jump host 内では実行されないためです
  - 接続失敗時は、コマンドの標準エラー出力を理由に含めます
  - 接続終了時にパイプを閉じ、2 秒以内に終了しない場合はプロセスを強制終了します

  `internal/acceptance` の `TestOnlyTheNamedSubsystemsStartAProgram` は外部プログラムを起動する場所を allowlist で検査します。`internal/sshclient/proxycommand.go` は 4 番目の許可箇所です。

- 接続時にこのアプリケーションが実行する外部コマンドは、利用者が指定した `ProxyCommand` だけです。`ssh-keygen` はハードウェア鍵用のコマンド例として表示しますが、アプリケーションからは実行しません。`Toolchain.KeyGen()` は、その選択肢を表示できるか確認するためだけに使用します。agent への登録は `x/crypto/ssh/agent` で直接行います。
- 未使用になった外部コマンド実行インターフェース `RunOutput` は削除しました。現在、Go の製品コードから外部プログラムを起動する場所は 4 箇所です。`cmd/sshc/browser.go` はアクセス URL をブラウザへ渡し、`internal/terminal/pty_unix.go` はローカルシェル用の PTY を起動し、`internal/nativebuild/nativebuild.go` は配布物に含まれないビルドコマンドを実行し、`internal/sshclient/proxycommand.go` は利用者が設定した `ProxyCommand` を実行します。いずれも `os/exec` を直接使用します。`TestOnlyTheNamedSubsystemsStartAProgram` は `.go` ファイルを走査し、この allowlist 以外に外部プロセス起動が追加された場合に失敗します。
- 未対応の設定は無視せず、理由を警告として表示します。対象は `RemoteForward`（リモート側の listen と `AllowTcpForwarding` に依存）、`ForwardX11`（接続先となる X server がない）、`ControlMaster` / `ControlPath`（プロセス内の `ssh.Client` を再利用するため不要）、`SendEnv`（engine の環境変数は利用者の shell 環境と一致しない）、`CertificateFile`、`LocalCommand`（接続後のローカルコマンド実行は未対応）です。
- ポート転送（`LocalForward` / `DynamicForward`）と agent 転送（`ForwardAgent`）に対応します。有効な転送はコンソール一覧に表示し、設定ファイルから暗黙に開かれたローカルポートを確認できるようにします。
- 転送用 listener はループバックにだけ bind します。`LocalForward 0.0.0.0:8080` や `GatewayPorts yes` が設定されていても、他のマシンには公開しません。ただしループバックは同一OSユーザーへの隔離ではなく、共有ホストでは別のローカルユーザーから利用される可能性があります。ループバック以外の指定は警告します。ポートを確保できない転送があっても SSH 接続は継続し、失敗理由を一覧に表示します。
- 転送の存続期間はセッションと同じです。コンソールを閉じると listener も閉じます。`DynamicForward` は SOCKS5 の CONNECT だけに対応します。agent 転送では鍵データではなく、agent を通じて署名する機能をリモートへ提供します。
- 接続中に対話が必要になるのは、未知のホスト鍵、鍵のパスフレーズ、パスワード、keyboard-interactive の 4 種類です。保管庫に対応する値がない場合、または保存値が拒否された場合に端末へプロンプトを表示します。入力値は端末に再表示せず、接続中に入力した鍵パスフレーズも保存しません。保存は Secrets 画面から明示的に行います。
- ホスト鍵が `known_hosts` と一致しない場合は、確認を求めず接続を拒否します。未知のホストは `StrictHostKeyChecking` に従い、承認した鍵は Known Hosts 画面と同じトランザクションで保存します。読み取りには同じ parser を使用します。
- `ProxyJump` はプロセス内で処理し、`ProxyCommand` は上記の条件で外部プログラムとして実行します。両方を同時に指定した設定と、jump host 経由の接続先で `ProxyCommand` を指定した設定は拒否します。
- 接続失敗の詳細は対象コンソールに表示します。設定を解決できず接続を開始できない場合はセッションを作成せず、エラーコード付きの理由を返します。到達不能や認証失敗など接続開始後のエラーは、作成済みセッション内に表示します。
- 外部プロセスは argv を直接組み立てて実行します。シェル、`sh -c`、文字列連結した AppleScript は使いません。alias、hostname、user は OpenSSH が受理する値であっても信頼しません。
- `Match exec`、`ProxyCommand`、`KnownHostsCommand`、`LocalCommand`、`RemoteCommand` は実行可能ディレクティブとして構文木から検出し、コマンド文字列を表示します。設定の読み取りだけでは外部プロセスを起動しません。実行前の確認が必要なのは接続操作です。検出したディレクティブはすべて表示します。
- OpenSSH は展開したトークンをシェル向けにエスケープしません。hostname や user の値がそのまま危険ディレクティブのシェルへ届きます。UI と API はこの警告を必ず添えます。
- 接続テスト、`known_hosts` 変更、ホスト鍵の取得、公開鍵のリモート登録は、CSRF header に加えて、対象と操作種別と表示済みコマンドへ紐付いた一回限りの action token（`X-SSHC-Action`）を必要とします。digest はサーバ側で発行時と使用時の両方で計算するため、確認から実行までの間に設定を編集すると token は無効になります。
- 値の出所（provenance）は実在するファイルと行を参照します。ワイルドカード、否定パターン、`Match` ブロック、alias 重複により単純な継承として説明できない場合は、その状態を示しつつ計算した実効値を返します。出所を推測で補完しません。
- 到達性チェックは宛先を直接 dial します。`ProxyJump` と `ProxyCommand` は使いません。結果には必ずその旨を表示します。踏み台越しにしか到達できないホストがここで失敗するのは想定どおりです。
- 認証テストはタイムアウトとキャンセルを持ち、出力を上限つきで取得し、forwarding と `LocalCommand` をコマンドライン優先設定で無効化します。無効化できない実行可能ディレクティブが残る場合は、その内容を確認するまで開始しません。
## 画面の境界

- `sshc engine` が UI を HTTP で配信し、`sshc` がアクセス URL を 1 件発行してブラウザへ渡します。UI の配信元は engine だけで、別のコピーは持ちません。
- `sshc engine` は foreground で実行し、自動では detach しません。継続実行には tmux、screen、systemd など OS 上の process supervisor を使用します。起動した端末を閉じると engine も終了します。
- 引数なしの `sshc` は engine を起動せず、実行中の engine からアクセス URL を取得して表示します。engine が動作していない場合は `sshc engine` の実行方法を表示し、exit code 1 で終了します。`~/.ssh/sshc/engine.lock` を engine の起動から終了まで保持し、2 個目の engine の起動を防ぎます。状態確認後に起動する方式では同時実行時に競合するため、排他 lock を使用します。
- アクセス URL は要求ごとに新しく発行します。engine 起動時には表示しません。起動時に発行すると、engine の実行中ずっと有効なワンタイム資格情報が端末の scrollback やログに残るためです。
- `sshc` は URL を標準出力に表示した後、GUI が利用できる場合は既定のブラウザで開きます。GUI がない場合も URL の表示には成功します。`sshc open` はブラウザを起動せず URL だけを表示し、スクリプトや文書化した手順から利用できます。
- 設定画面の「開いている接続」から、すべてのコンソール、ポート転送、リモートへ転送した agent を終了できます。engine 自体は停止しません。再接続できる操作のため、確認ダイアログは表示しません。
- engine は実行中の端末から Ctrl-C または SIGTERM で停止します。開いているコンソール、転送、vault を終了した後に lock を解放します。
- **cookie はポートに紐づきません。** 同じ `127.0.0.1` の別ポートに居るサーバーがこの session の cookie を受け取りうるので、読み取りにも CSRF トークンを要求します。トークンは port-origin ごとの `sessionStorage` にあり、そこへは渡りません。
- デスクトップパッケージは廃止し、`sshc-<OS>-<アーキ>` 形式の CLI バイナリだけを配布します。署名、公証、インストーラは使用しません。`curl` で取得し、`chmod +x` を設定して実行できます。Homebrew formula はソースからビルドします。
- GUI アプリケーションとして配布するのは Android 版だけです。Android ではストア配布と WebView へのアクセス URL の受け渡しが必要です。iOS 版はラッパー、CI、検査がない状態で `ios-bind` target だけが残っていたため廃止しました。engine 側の `mobile` は gomobile のビルド対象として iOS を扱えますが、iOS 版の配布は保証しません。
- Android版のengine ownerは同一packageの単一app processです。desktop／CLI用のOS file lockは使わず、Go側のprocess内mutexでstart／stopを直列化します。Service再生成などで`Start`が重複した場合は二重起動エラーにせず、以前のengineを停止して新しいengineへ置き換えます。desktop／CLIは別processを起動できるため、引き続き`engine.lock`を保持します。

## 更新の境界

- 更新確認は、このアプリケーションが SSH 接続先以外の外部ホストへ通信する唯一の機能です。`https://api.github.com/repos/aida0710/sshc/releases/latest`へengine起動直後と画面上の確認時にGETします。起動時はHTTP受付開始を先に通知し、3秒以内のbest-effort確認だけを行います。失敗やoffline状態でengineを停止せず、新しい安定版がある場合だけ`sshc update`を案内します。server sideからリクエストするため、ページの`connect-src`は`'self'`のままです。
- `sshc update`は任意の実行ファイルを直接置換しません。Homebrew版は`brew --prefix --installed aida0710/tap/sshc`の管理対象と実行中ファイルを`SameFile`で照合してから、同じ`brew`のformula更新へ委ねます。`install.sh`版は隣接receiptに記録したrepository、安定版、SHA-256が実行中ファイルと一致するときだけ、確認した最新tagに固定したinstallerへ委ねます。Windows、手動配置、source build、変更済みファイル、判定不能な導入は拒否します。
- 以前削除した自己更新機能は、アプリケーション自身がネットワークからbinaryを取得して直接置換する方式でした。署名鍵をrelease workflowと同じ主体が扱う構成ではrepository侵害への防御が増えず、独自updaterの失敗境界だけが増えるため復活させません。現在の更新入口は既存のHomebrewまたはtag固定`install.sh`を管理元として維持し、後者はReleaseの`checksums.txt`、digest付きreceipt、同一directory内renameを必須にします。
- リリースでは 6 つの CLI バイナリを生成します（`sshc-darwin-arm64`、`sshc-darwin-amd64`、`sshc-linux-amd64`、`sshc-linux-arm64`、`sshc-windows-amd64.exe`、`sshc-windows-arm64.exe`）。各 OS の runner がその OS 向けの 2 アーキテクチャをビルドします。darwin では `CGO_ENABLED=1` を使用します。設定エンジンは `%u` と `%i` の展開に `os/user.Current()` を使用し、cgo を無効にした Go は `/etc/passwd` を参照するため、macOS の通常ユーザーではこれらの token を解決できない場合があります。Linux と Windows の CLI は `CGO_ENABLED=0` でビルドします。Android は署名済み APK を別ジョブで生成し、6 つの CLI とともにリリースへ添付します。
- リリースには `checksums.txt` を添付します。これはダウンロード中の破損検出に使用します。さらに、公開するCLIとAPKのdigestへGitHub artifact attestationを発行し、`gh attestation verify <file> --repo aida0710/sshc`でtagのRelease workflow由来であることを検証できます。`install.sh`経由の更新も公開済みtagの`checksums.txt`と照合します。Homebrew版はtag sourceのSHA-256をformulaが検査します。
- ソースから使っている場合の更新は `make update`（`git pull --ff-only` + `make install`）です。

- `make install` は `~/.local/bin/sshc` へ atomic にインストールし、sudo は不要です。`make uninstall` はこのバイナリだけを削除します。
- `sshc` と実行中 engine のバージョンが異なる場合は接続前に拒否し、それぞれの実行ファイルパスを表示します。どちらが古いかは判定しません。
- engine の継続実行は OS の process supervisor に委ねます。`sshc engine` は foreground で動作するため、`tmux`、`systemd` user unit、`launchd` などを使用できます。具体例は [docs/headless-examples.md](headless-examples.md) を参照してください。このアプリケーション自身は unit や scheduled task を作成しません。

  以前は、アプリケーションが `~/Library/LaunchAgents/com.github.aida0710.sshc.plist` と `~/.config/systemd/user/sshc.service` を作成し、`sshc service refresh` / `disable` で管理する方式と、デスクトップアプリケーションを OS のログイン項目に登録して engine を子プロセスとして起動する方式がありました。どちらも削除しました。

  現在、常駐対象は `sshc engine` の foreground process です。OS のログイン項目ではなく process supervisor に登録します。引数なしの `sshc` は engine を起動しないため、常駐設定には使用しません。
- `sshc ssh --list` は `~/.ssh/config` と到達可能な `Include` を読み、具体的な接続先 alias を辞書順で 1 行ずつ表示します。`Host *`、ワイルドカード、否定パターンは接続先名ではないため表示せず、重複 alias は 1 回だけ表示します。設定の読み取り時に `ssh` や `Match exec` は実行しません。
- 引数なしの `sshc ssh` は現在のターミナルに検索 TUI を表示します。alias、設定から計算した `HostName`・`User`・`Port`、metadata のタグで絞り込み、alias順に表示します。上下キーで選択し、Enter で同じ端末から接続します。Web UI は起動せず、`sshc ssh <接続先>` と同じ保存済み鍵パスフレーズの経路を使用します。設定ファイルが存在するが読み取れない場合は、接続先 0 件として扱わず読み取りエラーを表示します。
- TUI の入力は端末からの read 単位で解釈します。`Esc` は単独キーであると同時に矢印キー列の先頭でもあるため、read の末尾にある場合だけ単独キーとして扱います。未対応の escape sequence は終端まで読み捨て、`Delete` や `Ctrl-矢印` の後続バイトが検索文字列に入らないようにします。行は端末幅で切り、表示できない件数を `N more` として表示します。
- サブコマンドの一覧は `sshc -h`、`sshc ssh --help`、`sshc serial --help`、`sshc telnet --help` に出ます。SSH alias は必ず `sshc ssh` の後で解釈するため、`serial`、`telnet`、`status` などのトップレベルコマンド名と同じaliasにも接続できます。transportを省略した `sshc <alias>` は受け付けません。
- `sshc ssh <接続先>` は外部の `ssh` を起動せず、現在のターミナルからプロセス内 SSH client で接続します。engine は `~/.ssh/sshc/cli`（0600）に URL と起動ごとの秘密を保存し、CLI はこの情報を使って保存済み資格情報を取得します。応答には単回トークンではなく、その接続に必要な資格情報を直接含めます。engine は ProxyJump の接続チェーンを解決し、各 alias が使用する鍵パスフレーズとアカウントパスワードだけを返します。`Match exec` や `CanonicalizeHostname` により外部実行または DNS なしでは鍵を決定できない場合は返しません。接続チェーンに含まれない資格情報も返しません。2FA など保存値で処理できないプロンプトは端末に表示します。アカウントパスワードを追加しても、このファイルを読み取れる主体は既に vault 暗号文、秘密鍵、任意 alias の保存済みパスフレーズへアクセスできるため、信頼境界は変わりません。強制終了後にファイルが残っても参照先ポートには接続できず、秘密は次回起動時に再生成されます。
- engine が動作していない場合は `sshc engine` の起動方法を表示して終了します。CLI 自身は engine を起動しません。保存済み資格情報を使わずに接続する場合は、`ssh <接続先>` を利用できます。
- vault が施錠中の場合、対話式の `sshc ssh <接続先>` は解錠を待ちます。UI または別端末の `sshc vault unlock` で同じ engine を解錠すると、待機中の接続が続行します。この接続コマンド自体はマスターパスワードを要求しません。`--non-interactive` は待機せずエラーを返します。以前使用していた `/cli/unlock` route は削除済みで、`TestLegacyCLIUnlockRouteIsNotRegistered` が 404 を検査します。desktop は UI を 1 回 foreground にして無期限に待機し、headless は待機しません。
- UI の「接続」は外部ターミナルを起動せず、engine 内の SSH client を使用します。入出力のバイト列を WebSocket で UI へ送り、xterm.js で描画します。ローカルシェルは PTY を確保し、同じ WebSocket 経路を使用します。
- 埋め込みターミナルへの移行により、macOS では Terminal.app、iTerm2、kitty、Ghostty、WezTerm など利用者のターミナル設定をそのまま利用できなくなりました。外部ターミナルを選択する設定と `internal/platform/macos/terminal.go` の AppleScript・bundle ID 一覧は削除しました。一方、外部ターミナル起動に対応していなかった Linux でも UI 内から接続でき、接続直後のエラーを終了済みコンソールで確認できます。
- 埋め込みターミナルは、解錠済みページを侵害した攻撃者に任意コマンド実行を許すリスクがあります。以前はターミナル起動に単回 action token を要求していましたが、埋め込みターミナルでは要求しません。vault の解錠だけが条件です。したがって、解錠中のページを制御できる主体は確認ダイアログなしで複数の shell を起動できます。ターミナルを開くたびに確認を求める操作性とのトレードオフとして、既存の action token を削除しました。
- ネットワーク切断時は設定した回数だけ再接続します。

  ネットワーク切り替え、端末の sleep、接続先の再起動による切断は自動再接続の対象です。シェルが `exit` で終了した場合は再接続しません。`sshclient` はネットワーク切断を `TransportLost` として記録し、終了コードと区別します。

  再試行間隔の基準は 1、2、5、10、15 秒です。同時に切れた接続が一斉に再試行しないよう、session IDから計算した安定した±20%のjitterを加えます。既定の5回では、再試行終了まで最大40秒かかります。この間、切断されたコンソールは一覧に残ります。利用者が明示的に閉じた場合は再試行しません。設定画面の「ターミナル」で0〜5回を選択でき、0は再接続なしを意味します。

  - 再試行回数は試行ごとに設定から読み、実行中に 0 へ変更した場合は次の試行を行いません
  - 0 は明示的な設定値であり、未設定（既定値を使用）と区別するため pointer として保存します
  - 0 の場合は「再接続を諦めました」というメッセージを表示しません
  - 10秒以上安定していた接続は過去の再試行回数を持ち越さず、短時間に切断を繰り返す接続だけを上限で止めます
  - host key変更、失効、未知鍵、利用可能なidentityや認証方式の欠如、鍵パスフレーズの問題は、利用者の確認が必要なのでreopenを1回で止め、固定problem codeを返します
  - UI の「5 回・最大 40 秒」は最大jitterを含む再試行間隔から計算します。`internal/acceptance` は両言語の表示値を `terminal.ReconnectWindow` と照合します

  session APIはSSH processの状態を`connecting`、`connected`、`reconnecting`、`exited`で返します。再接続中は試行回数、上限、次回時刻と固定problem codeを返し、raw transport errorは返しません。WebSocketの接続状態はbrowser attachmentの状態であり、SSH processとは別に表示します。Web UIはsessionが存在するときだけ2秒間隔で一覧を更新し、世代番号が古い応答を捨てます。

  自動再接続が終了したSSH sessionまたはremote shellが終了したSSH sessionは、終了表示内の「再接続」から同じsession ID、pane、scrollbackのまま新しいshellを開けます。`POST /api/v1/terminal/sessions/{id}/reconnect`は終了済みSSH sessionだけを受け付け、live session、local shell、利用者が一覧から閉じたsessionを拒否します。新しい接続も通常のhost key確認、認証、同時session上限を通り、security checkを迂回しません。開始済みの再接続とcloseまたはengine停止が競合した場合は新processを破棄し、sessionを復活させません。成功後は新しい単回ticketで同じWebSocket表示へattachし直します。

- PTY と terminal session は engine が保持します。ブラウザのタブを閉じたり再読み込みしたりしてもセッションは継続し、再接続時に scrollback を再生します。利用者が確認dialogで「閉じる」を選んだ場合はSSH／localを区別せず即座に強制停止し、一覧からも一度で削除します。利用者操作ではなく子プロセス側から終了したセッションだけは、接続失敗の詳細を確認できるよう新しいものから 20 件まで一覧に残します。
- scrollback はメモリにだけ保持し、ディスクには保存しません。世代バックアップ、journal、history、remote snapshot の対象外です。`TestTheScrollbackNeverReachesTheStateDirectory` は `~/.ssh` 全体を走査して検査します。1 セッションの既定上限は 256 KiB（設定範囲 16 KiB〜4 MiB）、同時セッション数の既定上限は 50（設定範囲 1〜200）です。設定画面の「ターミナル」から変更し、`metadata.json` の `embeddedTerminal` に保存します。未設定は既定値を使用する状態であり、既定値と同じ値を明示的には保存しません。上限到達時は `terminal_session_limit` で新規作成を拒否し、既存セッションを自動終了しません。
- WebSocket endpoint は `/api/` の外にある `/terminal/stream` です。ブラウザは WebSocket handshake にカスタム header を付けられないため、`X-SSHC-CSRF` を必須とする `/api/` 配下には配置できません。代わりに、CSRF 検証済みの `POST /api/v1/terminal/sessions` が単回 ticket を発行します。ticket は 1 個の session ID に紐付き、10 秒で失効し、1 回だけ使用できます。無効、期限切れ、使用済みの場合は upgrade せず 403 を返します。upgrade 時には `Origin` の完全一致も確認します。
- CSP では xterm.js に必要な `style-src` だけを緩和します。xterm.js は文字寸法の計測後に `<style>` 要素を追加し、DOM renderer は各 cell に `style` 属性を設定します。nonce を渡す API がないため、`style-src 'self' 'unsafe-inline'` を使用します。`script-src 'self'` と `require-trusted-types-for 'script'` は変更しません。xterm.js 5.5.0 と 6.0.0 の配布物では、`innerHTML`、`insertAdjacentHTML`、`document.write`、`new Function`、`eval`、Worker、blob URL の使用が 0 件であることを確認しました。この変更により HTML injection が可能な場合は CSS injection も可能になりますが、React は文字列を escape し、`dangerouslySetInnerHTML` も使用していません。`connect-src 'self'` は同一 origin の `ws://` を許可するため変更しません。E2E テストは CSP violation が 0 件であることを確認します。
- プロセス内 SSH client の依存関係は `internal/app` で一度だけ構築します。埋め込みターミナル、`sshc ssh <接続先>`、認証テスト、公開鍵のリモート登録は、同じ鍵、`known_hosts`、設定解決器を使用します。
- ローカルシェルは SSH 接続ではないため、`localhost` を Home の接続一覧には表示しません。左ナビのターミナルにある「ローカルシェル」から起動します。シェルは、絶対パスで存在し実行可能な `$SHELL`、`/bin/zsh`、`/bin/bash`、`/bin/sh` の順に選択します。`PATH` は検索しません。argv[0] の先頭にハイフンを付け、login shell として起動します。
- 埋め込みターミナルは安全な文字集合の alias に限ります。それ以外は `unsafe_alias` として拒否します。`sshc ssh <alias>` で繋ごうとした場合も、拒否される理由を先に一言添えます。
- 外部ターミナルへ貼り付けるコマンドを生成する機能はありません。以前の `POST /api/v1/terminal/command` は削除し、CLI 接続には `sshc ssh <alias>` を使用します。
- ホスト鍵の取得結果は本人性を証明しません。常に「未検証」と表示し、別経路で取得した fingerprint の一致か、明示的な承認がある場合だけ追加します。
- `known_hosts` の変更は `storage.Manager` を通し、journal と世代バックアップを残します。削除は表示していた行の digest を伴い、ファイルが変化していれば衝突として拒否します。解析は無損失なので、指定された行以外は 1 バイトも変わりません。
- 公開鍵のリモート登録は POSIX shell を持つ環境に限定し、固定のリモート処理へ公開鍵を標準入力で渡します。ユーザー入力をリモートシェル文字列へ補間しません。対応外環境では手順の表示だけを行います。
- 応答に載せる `ssh` の出力は上限つきで、ホームディレクトリのパスを `~` に置換してから返します。利用者のアカウント名を応答本文へ持ち出さないためです。
- 自動テストは実リモートホストと利用者の `~/.ssh` を使用しません。例外は、一時ディレクトリの fixture に対して `ssh -G -F` を実行する差分テスト（`ssh` がない場合は skip）と、実 PTY で `/bin/echo` を起動するテスト 1 件です。前者はこの解決器と OpenSSH の結果を比較する検査専用経路で、製品コードからは使用しません。後者もリモートホストと利用者設定にはアクセスしません。

## SFTP、Workspace、Snippets の境界

- SFTP はターミナル channel と同じ接続設定、vault、`known_hosts`、`ProxyJump` chain を使いますが、現在の対話ターミナルの transport 自体は共有しません。各 API 操作に専用の非対話 SSH transport と SFTP subsystem を開き、処理後に全 hop を閉じます。未知のホスト鍵は常に拒否するため、最初の確認はターミナル接続で行う必要があります。
- リモート editor は UTF-8 の通常ファイルだけを扱い、上限は 2 MiB です。バイナリまたは大きなファイルは download を使用します。save は読み込み時の revision と現在の stat を比較して外部変更を検出し、同じ directory の一時ファイルを書いて rename します。既存 mode は維持します。delete は表示した stat に紐づく単回 action token が必要で、directory の再帰削除と symlink の追跡はしません。
- upload／downloadはOpenAPIで定義した共通Transfer Job APIと、Web全体で生存するTransfer Managerへ集約します。jobはdirection、file／folder、batch、attempt、status、bytes、速度、残り時間を持ち、状態遷移と同時2件の上限をbackendでも検査します。folder uploadはfile子jobへ展開し、同じbatchの成功済みfileを保ったまま失敗fileだけretryできます。画面移動中もqueueとApp内通知は生存します。browser reload後のuploadは同じname・size・lastModifiedのfileを再選択した場合だけ再開し、downloadの受信chunkはreloadをまたいで保持しません。
- uploadはfile/folder pickerとDrag & Dropを同じ経路へ集約し、relative pathを検査して親directoryを浅い順に作成します。各fileは1 MiB以下のchunkとして送ります。remote側では対象と同じdirectoryの予約part fileへ期待offsetが一致する場合だけ追記し、完了時にtarget revisionを再検証してatomic renameします。pauseはpartを維持し、cancelはpartを削除します。既存ファイルは409を受けた時点で個別に上書き確認し、暗黙には置換しません。file downloadは受信済みbytesを保持してHTTP Rangeで自動再試行します。directory downloadは共通queueへ入るもののresume対象外のsymlink非追跡ZIP streamで、retry時は先頭からやり直します。symlinkはextract先を脱出できないようlink targetを内容とする通常ファイルへ変換します。chmodは現在のmetadata revisionと単回action tokenを必要とし、symlinkには適用しません。
- SFTP は左ナビの Start 内で Terminal の直前に置きます。画面を開いただけでは接続せず、利用者が host を選んだ後に初めて一覧を取得します。SFTP、鍵、`known_hosts` の表は操作列を除く各データ列をクライアント側で安定ソートし、現在の方向を `aria-sort` でも公開します。
- Monaco Editor は SFTP 画面を開いたときだけ読み込みます。editor worker は build に同梱して同一 origin から読み込み、blob URL や CDN は使用しません。従来の `script-src 'self'` と Trusted Types の方針は維持します。
- 同じ engine 内の Monaco Editor 保存と upload 公開は、SSH alias と正規化済みtarget pathの組ごとに直列化します。これにより両操作が同じrevisionを同時に検証して互いを上書きすることはありません。ただし一般的な SFTP v3 には「revision が一致するときだけ rename」を行うatomic CASがないため、別のSSH clientやremote processが検証とrenameの間に書き換える競合の検出はbest-effortです。
- Workspace はpane種別、SSH aliasまたはローカルシェル、分割木、比率、focusだけを `~/.ssh/sshc/workspaces.json` に保存します。kindを持たないschema version 1のpaneはSSHとして読み、次の保存でversion 2へ更新します。ローカルシェルは`kind: shell`と固定target `localhost`で表し、再オープン時はSSH aliasと同様に新しいsessionを開始します。split separatorはpointerまたはkeyboardで10〜90%へ変更し、その比率を保存します。Focus Modeは保存木を変更せず単一paneだけを一時表示し、Escで元layoutへ戻します。terminal session ID、scrollback、remote process は保存しません。Homeは名前、pane数、更新時刻の一覧だけを読み、明示的な「ワークスペースを開く」でrestore要求を1回だけTerminalへ渡します。一部paneの接続失敗は他paneを閉じません。このファイルは端末固有で、世代 backup と remote snapshot の対象外です。pane移動はterminal本体ではなく専用handleから開始し、drop先paneとruntime node全体を交換します。target、session ID、接続状態はpaneに追従し、split方向、比率、focus pane IDは変えません。handleを2つ順に選んでも同じ交換を行えます。Command Centerはlayout全体から接続中のterminal session IDをkindに関係なく集め、SSHとローカルシェルが混在してもsessionごとにcommandとcarriage returnを現在のPTY入力へ送ります。Focus Modeで一時的に非表示のpaneも対象です。未接続paneを接続先名から開き直すfallbackは持ちません。管理操作はdesktop向けで、単一terminal時とモバイルでは表示しません。単一terminal時もterminal名と検索toolbarは表示し、検索ボタンまたは`Ctrl/Cmd+F`で検索欄を開けます。保存済みWorkspaceとdesktopで作成したlayout自体は変更しません。
- Terminal内検索はxtermのメモリ上のscrollbackを走査し、大文字小文字、正規表現、全一致highlightを切り替えて前後の一致を選択表示します。検索barはTerminalへ重ね、開閉時にPTYの行数や描画位置を変えません。sshc独自のcommand履歴・入力候補・remote path補完は持たず、shellや接続先アプリケーションの補完をそのまま利用します。
- Snippet library と startup binding は `~/.ssh/sshc/snippets.json` に保存しますが、ディスク上の文書全体をローカルの master key で暗号化します。旧版の平文文書は、unlock 後の初回読込または master password 変更時に検証してから原子的に暗号化します。remote sync では端末固有の暗号文を運ばず、検証済みの論理文書を snapshot 全体の暗号化内へ載せ、受信端末の master key で再暗号化します。
- command には `{{name}}` 形式の変数を使用できます。secret 型は入力欄と通常previewを伏せるための明示的な型であり、本文に秘密が含まれるかを推測して拒否する検出は行いません。通常previewは`[secret]`を返し、server-side dispatchは実値を使用します。Quick Commandsの挿入と実行は実値をbrowserへ返さず、表示済みpreviewのevidenceと同じcommand・process generationであることを再確認してからserver側で送ります。途中でSnippetまたはpaneが変わった場合は更新後のpreviewを表示し、もう一度明示操作を求めます。挿入はEnterを付けず、改行や制御文字を含むcommandは「挿入」の意味を破らないよう拒否します。copyだけは利用者が押した時点で実値を取得します。実行後はremote shell history、terminal output、clipboard等に残り得ます。
- Snippets画面の複数ホストjobは、解決済みの接続先と展開後commandをpreviewに表示し、そのevidenceに紐づく単回action tokenを消費して専用の非対話SSH executionを開始します。1 jobは最大64 targets、既定の並列数は4（上限8）で、server shutdownとcancelに追従します。この一般jobはlive terminalのcwdやshell状態を継承しません。
- Workspace Command Centerは別契約です。preview evidenceへ展開後commandのdigest、各terminal session ID、kind、接続先表示、title、process generationを含め、単回の`terminal.command.broadcast` tokenで確認します。送信時はkindではなくcapabilityを検査し、同じconnected sessionとgenerationの完全入力対応PTYだけに`command + "\r"`を全量入力します。再接続後の新しいshell、connecting／reconnecting／exited sessionは拒否します。通常打鍵と送信frameはsession単位で直列化します。PTY出力とexit codeをcommand単位には分離せず、APIは`delivered`／`failed`だけを返し、出力は各terminalのstreamとscrollbackへ流します。secret変数はpreviewでは伏せ、確認済みの送信時だけサーバー内で展開します。TTY echoやshell履歴へ残る可能性は通常のcommand入力と同じです。
- Startup snippet は alias ごとの opt-in です。必要な変数は暗号化された binding に保存します。初回接続と自動再接続の双方で、認証および remote shell の準備完了を `Ready` channel で確認してから command と carriage return を送ります。認証 prompt へ command を誤送信しません。
- remote sync受信と対応外VaultのresetでSnippetを別のmaster keyへ移すときは、旧鍵で封印された中間コピーを世代backupへ残しません。この変更を含むjournalが中断した場合はrollbackではなく、同じjournalをcompleteして新しい世代へ収束させます。

## 強化とリリースの境界

- リクエスト本文には二段の上限があります。middleware の `MaxRequestBodyCeiling`（2 MiB）が全 `/api/` 要求の天井で、各ハンドラーはさらに小さい上限を持ちます。宣言された `Content-Length` が天井を超える要求はハンドラーへ届く前に 413 で拒否し、長さを宣言しない chunked 要求は読み取り自体を天井で打ち切ります。本文を読まないルート（`/api/v1/diagnostics/config` や `/api/v1/keys/{keyId}/trash`）にも同じ天井が掛かるのは前者のためです。
- 外部コマンドの出力は `platform.MaxCapturedOutput`（64 KiB）で打ち切られます。認証テストの stderr は `MaxReportedOutput`（8 KiB）までに制限して表示します。
- `make fuzz` は `FUZZ_TARGETS` に列挙した全 target を順に実行します。`go test -fuzz` は一度に 1 target しか動かせないため、1 行で書くと最初の target しか回りません。target を追加して一覧に加え忘れると `TestMakefileFuzzTargetsCoverEveryFuzzFunction` が失敗します。
- fuzz の対象は、設定パーサーのラウンドトリップ、Include パターン展開、`known_hosts` リーダー、実効値の解決、`ssh -G` 出力パーサー、HTTP リクエストデコーダー、リモートスナップショットのリーダーの 7 つです。いずれも実 fixture を seed にしています。
- アクセス URL を表示するのは `sshc` と `sshc open` だけで、要求ごとに 1 つ発行します。`sshc engine` はアクセス URL を表示しません。旧版の `--own-engine` と `-open=false` はどちらも未定義であり、alias や互換用 option もありません。
- 配布物は UI を埋め込んだ単一バイナリです。`otool -L` はシステムライブラリのみを表示し、同梱ランタイムはありません。`make e2e` は毎回ビルドし直した実バイナリを Playwright で駆動するため、埋め込み済み UI が古いままだと E2E が失敗します。
- `make verify-generated` は `api/openapi.yaml` から Go と TypeScript の型を再生成し、コミット済みの生成物と一致しなければ失敗します。生成物を手で編集してはいけません。
- 自動テストは実際の `~/.ssh`、ssh-agent、リモートホストへ一切触れません。実バイナリを起動する試験でも `HOME` は一時ディレクトリです。実際に外部へ影響する操作（実接続、実 `authorized_keys` 変更、実ホストへのホスト鍵取得）は `docs/manual-acceptance.md` の手動試験に分離しています。`internal/sshclient` は 127.0.0.1 に立てたプロセス内の SSH サーバーと本物の握手を行いますが、そこもリモートではありません。
- `make integration`は固定digestのOpenSSHコンテナに対してSFTP subsystemを実際に開き、64 MiB超のfixtureを含むupload／chunk pause・remote offset resume／atomic complete／cancel cleanup／Range相当のoffset downloadとSHA-256一致、file・directory download／chmod／text save／競合拒否／rename／list／deleteを往復します。環境変数がない通常の`go test ./...`ではskipし、利用者のホストへ接続しません。
- 設計 §12 の完成条件は `go test ./internal/acceptance -run TestDesignCompletionConditions -v` で一覧できます。各条件について、それを証明するテスト名とコマンド名、そして自動化が届かない範囲を出力します。13 行のうち 7 行が自動テストのみで成立し、5 行は自動化が越えてはならない境界で止まり、1 行は OpenSSH が入っている場合にのみ証明されます。
- `internal/acceptance` はテストファイルのみで構成され、配布バイナリには含まれません。`TestNoTestOnlyPackageReachesTheShippedBinary` がそれを検査します。
- ログへの秘密混入検査は `app.Build` へ渡した logger だけを見ています。グローバルの `slog` 既定 logger へ書くと検査を素通りするため、新しいログは必ず注入された logger を使ってください。

## ライセンス

Apache License 2.0 です（[LICENSE](../LICENSE)）。
