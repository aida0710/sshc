# WinSCP機能差分台帳

更新日: 2026-10-23

比較対象: sshc main `7ebdde6f3`（2026-10-23時点） / WinSCP 6.5.6

今回はsshcの実装と説明を照合した。WinSCP側の比較項目は前回の6.5.6を基準とし、新しいバージョンとの差は調べていない。

## 目的

WinSCPに存在する機能を漏れなく分類し、sshcで同じ利用目的を満たせるかを追跡する。WinSCPの画面をそのまま複製することは目的にせず、デスクトップではCommanderに近い高密度な操作、モバイルではExplorerに近い単一ペイン操作として成立させる。

状態は次の意味で使う。

- `対応`: sshcに同じ目的を満たす機能がある
- `部分`: 基本機能はあるが、WinSCPの操作またはオプションが不足している
- `未対応`: sshcに相当機能がない
- `判断`: Windows固有、別プロトコル、外部SDKなど、sshcへ入れるかを別途決める

## 結論

sshcのSFTPは、ローカル／リモートの2ペイン、ファイル操作、内容検索、メタデータとSHA256による比較、転送の速度上限、通信断からの自動復旧に対応している。ローカルを含む比較は読み取り専用で、自動復旧も転送経路によって範囲が異なる。下の表では、こうした制限を含めて対応状況を示す。

残っている主な機能は、ディレクトリツリー、ディレクトリの同期・ミラー・変更監視、転送ごとの設定とプリセット、外部エディタとの連携である。設定を暗号化して保存するSyncは、ここでいうディレクトリ同期とは別の機能である。

## 1. ファイルパネルとナビゲーション

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| Explorer型の単一remote panel | 対応 | SFTP画面が相当 | 維持 |
| Commander型のlocal／remote 2 panel | 対応 | 左右それぞれにローカルまたはリモートを表示する。ローカルはブラウザ側ではなくsshcエンジンのマシンのファイル。ファイル操作、直接転送、読み取り専用の比較に対応。スマートフォンは左右の全タブを一列にまとめ、選んだ一覧を表示する | ディレクトリ同期は別項目で管理 |
| remote／remote 2 panel | 対応 | desktopで2つのhost／directoryを並べる。左右が独立したtab列を持ち、表示中のtab間でfile／directoryをDrag & Dropしてcopy／moveできる | 維持 |
| `..`による親directory移動 | 対応 | リモート・ローカルとも一覧先頭の`..`行で移動する。ローカルもOSルートまで移動でき、ルートでは`..`を表示しない | 維持 |
| path breadcrumb／直接入力 | 対応 | リモート・ローカルとも階層移動、パスの直接入力、コピーに対応。`~`と`~/`も使え、Windowsのローカルは`~\`も受け付ける | 維持 |
| Back／Forward履歴 | 対応 | リモート・ローカルともhostを切り替えるまでpath履歴を保持する。ローカルは別tab表示中も履歴と一覧を保持する | 維持 |
| Home directoryへ移動 | 対応 | serverのworking directoryを再解決して移動 | 維持 |
| Root directoryへ移動 | 対応 | navigation buttonまたは`/`の直接入力 | 維持 |
| directory bookmark | 未対応 | 2026-09-17に場所menuごと削除。Back／Forward履歴とpath直接入力で代替する | 持たない |
| 最近開いたdirectory | 未対応 | bookmarkと同時に削除。tabごとのBack／Forward履歴だけを持つ | 持たない |
| directory tree | 未対応 | 一覧だけ | desktopの任意表示として検討 |
| remote検索 | 対応 | 名前の検索とUTF-8ファイルの内容検索。内容は大文字小文字を区別した固定文字列で検索し、パス・行番号・本文を表示する。リンクは辿らず、件数と読み取り量に上限がある | 更新日時やサイズでの条件は未対応 |
| synchronized browsing | 未対応 | local panelはあるが連動操作は未実装 | 連動操作の必要性を判断 |
| pathをclipboardへcopy | 対応 | 現在directoryと選択項目のfull pathをcopy可能 | 維持 |
| opposite panelのpathへ移動 | 未対応 | local panelはあるが連動操作は未実装 | 連動操作の必要性を判断 |
| directory stateのsession別記憶 | 部分 | URLへalias/pathを反映し、左右の全tabのalias/pathとsort、ペインの分割をこのブラウザに保存する | selectionとhistoryの再読み込み後復元は未対応 |
| 複数SFTP tab | 対応 | 左右それぞれ最大8tab。幅を超えたtab列は横scrollし、固定した追加操作と選択tabの自動追従を持つ。各tabが自分のhost、履歴、選択を持ち、開いていた場所を再読み込み後も復元する | 維持 |
| panel内の名前filter | 対応 | 現在directoryを名前の部分一致で絞り込み | mask式は後続 |
| remote配下の再帰file検索 | 対応 | symlinkを辿らないserver側上限付き検索を実装済み | 更新日時やサイズ条件は未対応 |
| directory cache | 未対応 | 現在pathを都度取得 | stale表示を避ける明示cacheとして設計 |
| refresh | 対応 | path横の移動操作で再取得可能 | icon／shortcutを明確化 |
| 名前sort | 対応 | 昇順／降順 | 維持 |
| 種類／拡張子sort | 部分 | entry typeでsort。拡張子sortなし | 拡張子sortを追加 |
| size sort | 対応 | あり | 維持 |
| modified time sort | 対応 | あり | 維持 |
| permissions sort | 未対応 | modeは表示するがsort不可 | 一覧操作の改善として検討 |
| owner／group sort | 未対応 | 数値のUID/GIDを表示するが、並べ替えはできない | 並べ替えの必要性を判断 |
| owner／group／link target列 | 部分 | サーバーから取得できるUID/GIDを広い一覧と詳細に表示し、リンクは名前とリンク先を表示する。狭いデスクトップの一覧では権限とUID/GIDを省略し、詳細から確認する | 列表示の任意設定は未対応 |
| 列幅／列表示のcustomize | 未対応 | 固定 | desktop向けに追加 |
| status barと選択数／合計size | 部分 | 選択した項目の件数・サイズを表示する。一覧の下にはサーバーが対応していれば空き容量と合計容量を表示する | ディレクトリ全体の容量は詳細から確認 |

## 2. 選択と入力操作

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| clickによる単一選択 | 対応 | あり | 維持 |
| checkboxによる複数選択 | 対応 | desktop／mobileとも対応 | 維持 |
| Select All | 対応 | header checkbox、`Ctrl/Cmd+A` | 維持 |
| keyboardで行移動／Enterで開く | 対応 | ↑↓／Home／Endで行移動、Spaceで選択、Enterでフォルダを開くかプレビュー | 維持 |
| Shiftによるrange選択 | 対応 | 表示中の並びを基準にrange選択 | 維持 |
| Ctrl/Cmdによる追加選択 | 対応 | clickで追加／解除 | 維持 |
| 選択反転 | 対応 | 表示中のentryだけ反転 | 維持 |
| 選択解除／選択復元 | 部分 | header checkboxで解除可能。復元なし | 明示解除と直前選択の復元を追加 |
| maskで選択／解除 | 未対応 | なし | filter/mask共通構文の後に追加 |
| 同じ拡張子を選択 | 未対応 | なし | 選択menuへ追加 |
| double clickでdirectoryを開く | 対応 | あり | 維持 |
| context menu | 対応 | 右click／長押しで、右上の三点menuと同一のaction一覧を開く | 維持 |
| file manager shortcut（F2/F4/F5/F7/F8等） | 部分 | F2でrename、Deleteで削除、Escで選択解除、Ctrl/Cmd+Aで全選択 | F4／F5／F7／F8は未割り当て |
| drag and drop upload | 対応 | file／folder、空directoryを扱う | 維持 |
| remote rowをfolderへdragして移動 | 未対応 | 同じ一覧内のフォルダへのdropは未対応。2ペイン間のmoveは可能 | 同じ一覧内での操作として検討 |
| queueへdropして転送 | 未対応 | ファイル一覧へのdropで転送する | キューへのdropの必要性を判断 |

## 3. remote file操作

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| folder作成 | 対応 | `+` menu | 維持 |
| 空file作成 | 対応 | 作成menuから既存pathを上書きせずに0 byte fileを作成 | 維持 |
| symbolic link作成／編集 | 対応 | リモートでリンクを作成し、詳細からリンク先を変更できる。既存項目を上書きせず、安全な置換に対応しないサーバーでは変更を拒否する | 維持 |
| internal text editor | 対応 | リモート・ローカルとも2 MiB以下のUTF-8テキストをMonacoで編集。検索・置換・行への移動、保存前の競合検査に対応する | 維持 |
| external editor／Edit With | 未対応 | browserから外部editorを起動しない | desktop native連携の判断が必要 |
| file preview | 部分 | 詳細modalで画像とtextを表示。型はengineが先頭bytesから決める | PDFは<iframe>とCSPのblob:許可が要るため対象外 |
| file upload | 対応 | picker／drop、競合確認、atomic publish、resume | 維持 |
| folder upload | 対応 | relative pathと空folderを維持 | 維持 |
| file download | 対応 | revision固定、Range resume | 維持 |
| folder download | 対応 | symlinkを追わないZIP | ZIP resumeは未対応 |
| upload/download後にsourceを削除（move transfer） | 未対応 | copy transferのみ | 完了確認後だけsource削除するjobとして追加 |
| symlinkの転送 | 部分 | CLIのput／getはリンク先として転送し、ディレクトリへのリンクは中身ごと送る。壊れたリンクは飛ばす。Webのhost間copy／engine diskとの転送はリンクを対象外にする | Webの転送でも最上位のリンクだけ辿るかを判断 |
| remote内copy／duplicate | 対応 | 2ペイン間のcopyに加え、現在directoryで名前を指定した複製に対応 | 維持 |
| remote内move to | 対応 | rename、2ペイン間のmove、絶対pathを指定した複数項目のmoveに対応 | directory pickerは必要性に応じて追加 |
| rename | 対応 | 単一選択 | 維持 |
| delete | 対応 | 複数選択、確認、symlink非追跡 | remote recycle binは未対応 |
| clipboard copy／paste | 未対応 | file objectのclipboard操作なし | local panel／OS bridgeと合わせて設計 |
| file名をcopy | 対応 | 単一／複数を改行区切りでcopy | 維持 |
| full pathをcopy | 対応 | 単一／複数を改行区切りでcopy | 維持 |
| file URL生成 | 未対応 | なし | `sftp://`とsshc内deep linkを分けて設計 |
| properties表示 | 対応 | パス、種類、サイズ、更新日時、権限、revision、取得できたUID/GIDとリンク先を表示。フォルダの容量・件数は上限付きで集計する | 維持 |
| chmod | 対応 | リモートの単一・再帰・複数選択に対応。複数選択はfile／directory別mode、全対象プランと確認modalで保護。symlinkは対象外。lsetstat拡張バージョン1が必要 | ローカルchmodは対象外 |
| chown／chgrp | 部分 | リモートの通常ファイルとフォルダへ数値UID/GIDを指定できる。`lsetstat@openssh.com`とサーバー側の権限が必要。リンク経由は拒否する | 名前による指定と再帰変更は未対応 |
| timestamp変更 | 未対応 | なし | SFTP Setstat対応後 |
| propertiesの複数／再帰適用 | 部分 | 権限はリモートの複数選択・再帰に対応。事前競合は全件未変更、実行中の失敗は部分適用を表示 | 権限以外の一括適用は未対応 |
| lock／unlock | 未対応 | protocol lock操作なし | server capability依存として判断 |
| directory size計算 | 対応 | 最大20,000項目・深さ32でsymlinkを辿らず集計し、打ち切り時は部分値と明示 | 維持 |
| custom file command | 部分 | SnippetsとTerminalはあるが選択pathを渡せない | file path変数を安全にquoteして接続 |
| Open Terminal | 対応 | SFTPの現在host/pathを引き継いでTerminalを開ける。TerminalのOSC 7 cwdもSFTPへ渡せる | 維持 |

## 4. 転送キュー

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| background queue | 対応 | 全転送をengine authoritative queueで管理 | 維持 |
| queueをfile list下部へ表示 | 対応 | 下部dock、空なら非表示 | 維持 |
| batch全体とfile別進捗 | 対応 | batchと各jobの進捗、速度、残り時間 | 維持 |
| waiting／active／paused／failed／completed表示 | 対応 | あり | 維持 |
| pause／resume／retry／cancel | 対応 | job単位 | 維持 |
| completed消去 | 対応 | 手動の一括消去に加え、engineが設定時間で完了・取消を自動消去する | 維持 |
| waiting順のmove up/down | 対応 | 待機中のjobだけをengine側の待機列で入れ替える。runningの位置は動かない | top/bottomはAPIにあるがUIは上下のみ |
| Execute now／同時数を一時超過 | 未対応 | 最大同時数を厳守 | 必要性を判断 |
| queue processing全体の開始／停止 | 対応 | queue headerの停止／再開。停止中は待機jobを新しく開始せず、実行中のものは走り切る | 維持 |
| Suspend All／Resume All／Cancel All | 対応 | 下部queueの操作menuから全jobへ適用 | 維持 |
| job別speed limit | 未対応 | 全転送で共有する速度上限だけを持つ | 転送ごとの設定として検討 |
| 全体speed limit | 対応 | SettingsのSFTPでKiB/sを設定する。0は無制限。並列接続、複数job、複数画面、CLIがsshcエンジン共通の上限を使い、実行中にも適用する | job別の上限は未対応 |
| 最大同時転送数の設定 | 対応 | SettingsのSFTPで1〜8件を設定する。metadata.jsonの`fileTransfers`に保存し、sshcエンジン再起動後も適用する | 維持 |
| 複数接続で複数fileを転送 | 対応 | 既定では2件を同時転送。同じ接続先のSFTP接続を再利用し、未使用の接続は60秒で閉じる | 維持 |
| 1 fileを複数connectionで分割 | 対応 | 512 GiBまでのupload／downloadに対応。初期設定は1接続で分割しない。2接続以上に設定すると、既定100 MiB以上のファイルを32 MiB単位で分割する。開始サイズ16〜1024 MiB、接続数1〜128、chunk 8〜4096 MiB。TOTP認証の接続は1接続に制限する | 実サーバーで継続検証 |
| queueの折りたたみ | 対応 | headerを残して展開／折りたたみ | 維持 |
| queue高さのresize | 対応 | 上端に24pxのtouch領域を持つgripを表示。mobileは画面高に応じた3段階へsnapし、desktopは96〜560pxを連続変更する。双方の高さを別々に保存し、keyboardの↑↓／Home／Endにも対応 | 維持 |
| queue file listの展開 | 部分 | キュー全体を折りたたみ、展開すると各ファイルの進捗と操作を表示する | batchごとの折りたたみは未対応 |
| prompt／errorの保留表示 | 部分 | overwriteは開始前確認、errorはjob表示 | queue内で再確認待ちを扱えるようにする |
| 完了時action（disconnect/sleep/shutdown） | 未対応 | なし | browser製品では通知／engine停止までを候補とする |
| 再読み込み後のqueue復元 | 部分 | engine authoritative queueを2秒ごとに再取得する。engineが処理するremote↔engine-local jobはブラウザを閉じても継続する。ブラウザから選択した従来のuploadは再選択が必要 | ブラウザ選択ファイルの再接続は別途検討 |
| process再起動後のqueue復元 | 対応 | `~/.ssh/sshc/transfers.json`へ0600・atomic保存し、待機・一時停止・再開可能jobを復元する。マシン固有のstateとして同期しない | 永続形式のmigrationが必要になった時点でschema versionを更新 |
| transfer中の自動再接続 | 部分 | SettingsのSFTPで有効にする。既定は無効、上限1〜10回。sshcエンジン側の単一ファイル転送とremote copyは内容を照合して再開する。ブラウザ転送も種類に応じて復旧する | CLI自身の通信断と、sshcエンジン側のフォルダ転送・移動・削除は自動再試行しない。詳細は[転送マネージャー](../pages/sftp/transfers.md#通信断後の自動復旧) |
| browser通知 | 対応 | tabがbackgroundの時、許可済みのWeb通知で完了／失敗を知らせる。Androidはnative通知を使う | 維持 |

## 5. 転送設定

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| transfer options dialog | 未対応 | 即時queue登録 | 普段は省略可能な確認sheetを追加 |
| destination directory／operation mask | 部分 | current path＋元file名のみ | 転送前にdestinationとrename maskを指定可能にする |
| default transfer settings | 部分 | SettingsのSFTPで同時転送数、完了項目の保持、分割転送、速度上限、自動復旧、除外ルールを変更し、sshcエンジンに保存する。CLIの`sshc sftp settings`でも共有設定を変更できる | 名前付きプリセットと転送ごとの設定画面は未対応 |
| per-transfer settings | 部分 | CLIのget／putでは同時数や分割転送の設定をその実行だけ変更できる | Webの転送ごとの設定画面は未対応 |
| named preset | 未対応 | なし | host／path rule付きpresetは後段 |
| background／foreground選択 | 判断 | sshcは常にbackground | UIをblockするforegroundは導入しない |
| binary mode | 対応 | byte streamとして転送 | 維持 |
| text／automatic transfer mode | 未対応 | newline／encoding変換なし | 誤変換防止のため明示設定だけ検討 |
| filename case変換 | 未対応 | 元名を保持 | operation maskと合わせて追加 |
| invalid filename置換 | 未対応 | 保存先browser/OSに委ねる | ローカル操作の設定として検討 |
| upload時permission設定 | 部分 | 新規fileはserver default（umask）、上書き時は既存modeを維持。2026-09-19までは新規fileを0600にしていた | 転送optionとしての指定は未対応 |
| timestamp保持 | 対応 | upload（Web、CLI）、CLIのget、host間copy、engine disk↔hostのput／getで元のmtimeを付ける。browserへのdownloadだけはbrowserの保存に委ねる | 維持 |
| directory timestamp保持 | 未対応 | なし | file timestamp後 |
| downloaded read-only保持 | 未対応 | browser保存に委ねる | ローカル転送の設定として検討 |
| permission error無視 | 未対応 | job failure | optionとして明示する場合のみ追加 |
| total size事前計算 | 部分 | local uploadはsize既知、remote folderはZIP作成時に確定 | directory scanの進捗を追加 |
| speed limit | 対応 | sshcエンジン共通の速度上限をWebとCLIの転送に適用する。ファイル内容の通信が対象で、メタデータ通信は含めない | job別の上限は未対応 |
| include/exclude file mask | 部分 | SettingsのSFTPに除外ルールを保存し、転送開始時のルールをjobに保持する。WebとCLIのフォルダ転送に適用する | 含める条件による絞り込みとプリセットは未対応 |
| newer/updated only | 未対応 | target存在時はoverwrite確認 | compare/sync基盤と共通化 |
| hidden file除外 | 部分 | 隠しファイルも通常は転送する。除外ルールで名前・パスを指定できる | 隠し属性による専用の除外設定は未対応 |
| empty directory除外 | 未対応 | 空directoryを維持 | optionとして追加 |
| overwrite／resume／append mode | 部分 | overwrite確認とpart resumeは対応。appendなし | overwrite policyとappendを追加 |
| resume threshold | 未対応 | uploadは常にpart、downloadは常にspool | 固定安全方式を維持するか判断 |
| transfer settingからautomation code生成 | 未対応 | なし | 既存のSFTP CLIを使った生成を検討 |

## 6. directory比較と同期

ここでいう同期は、sshcの設定をS3へ暗号化保存する既存の「Sync」とは別物である。

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| local／remote directory比較 | 対応 | ローカルとリモート、リモート同士を再帰比較する。ローカルを含む比較は読み取り専用。スマートフォンでも比較する2つのタブを選べる | 比較結果からローカルを含むコピー・同期は未対応 |
| 相違fileのhighlight／選択 | 部分 | 左のみ・右のみ・変更あり・種別違いを表示する。リモート同士なら項目を選んで左右へコピーできる。ローカルを含む比較は差分の確認だけを行う | ローカルを含む比較結果からのコピーは未対応 |
| local→remote同期 | 未対応 | なし | ディレクトリ同期の設計後 |
| remote→local同期 | 未対応 | なし | ディレクトリ同期の設計後 |
| 双方向同期 | 未対応 | なし | conflict model確立後 |
| mirror mode | 未対応 | なし | delete preview必須で追加 |
| timestampだけ同期 | 未対応 | なし | Setstat対応後 |
| existing files only | 未対応 | なし | sync option |
| selected files only | 未対応 | なし | sync option |
| changes checklist／preview | 部分 | remote／remote比較でcopy対象を選択でき、相手側だけの項目は自動削除しない | local／remote sync planへ共通化 |
| 比較criteria（time/size/checksum） | 対応 | メタデータと内容（SHA256）を選べる。内容比較は同じサイズ・更新日時でも差を検出する。通常ファイルを対象に両側合計256 MiBまで読み、上限や対象外の種類は未比較と表示する | 方式を切り替えたり画面を閉じたりすると読み取りを中止する |
| Keep remote directory up to date | 未対応 | local変更監視なし | desktop native/local bridge導入後 |
| synchronization in background queue | 部分 | remote／remote比較で選んだcopyは共通のengine queueへ登録される。mirror planは未対応 | local／remote sync planもjob batchへ変換 |
| remote recycle binへのbackup | 未対応 | deleteは即時 | host単位のtrash pathを設計 |
| remote-to-remote同期 | 部分 | 2つのdirectoryを再帰比較し、選択した差分を左右へcopyできる。自動削除、mirror、継続監視は行わない | deleteを伴う同期は明示previewと再確認を設計 |

## 7. connection、protocol、session

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| Site Manager／保存済みhost選択 | 対応 | modalでalias、host、userを検索。最近接続した順／group表示を切替 | 維持 |
| SFTP | 対応 | OpenSSH設定を正本に接続 | 維持 |
| SCP file transfer | 未対応 | terminal SSHのみ | 判断 |
| FTP／FTPS | 未対応 | なし | SSH clientという製品範囲外候補 |
| WebDAV | 未対応 | なし | 製品範囲外候補 |
| S3 file manager | 未対応 | Sync storage用途だけ | 汎用browserは製品範囲外候補 |
| password authentication | 対応 | vaultとprompt | 維持 |
| keyboard-interactive | 対応 | SSH接続で対応 | 維持 |
| public key | 対応 | key管理、agent、remote登録 | 維持 |
| Kerberos/GSSAPI | 未対応 | SSH transportにGSSAPIなし | enterprise需要で判断 |
| SSH agent | 対応 | platform agent連携 | 維持 |
| ProxyJump／ProxyCommand | 対応 | OpenSSH解決結果を利用 | 維持 |
| connection tunneling | 対応 | local／remote／dynamic forwarding | 維持 |
| initial local／remote directory | 部分 | remoteはserver home。host別override UIなし | OpenSSH metadataまたはsshc metadataで追加 |
| last remote directory記憶 | 部分 | URL中は維持、hostの永続状態ではない | host単位でoption化 |
| session disconnect／reconnect | 部分 | 接続先の選択時に接続し、保存したタブの復元時は接続ボタンを押すまで接続しない。同じ接続先のSFTP接続は操作間で再利用し、未使用なら60秒で閉じる | 常時接続の維持は行わない |
| server/protocol information | 部分 | Diagnostics／effective configに分散 | SFTP contextから開けるsummaryを追加 |
| session URL／code生成 | 部分 | sshc内URLはあるが共有UIなし | copy actionを追加 |
| change password | 未対応 | なし | remote command依存のため判断 |
| install public key | 対応 | Remote Key機能 | SFTP／connection contextから導線追加 |

## 8. automation、統合、運用

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| command-line interface | 対応 | 接続、run、sync、管理、SFTP転送CLIあり | 維持 |
| SFTP upload/download CLI | 対応 | `sshc sftp get/put`。recursive、上書き確認、skip、dry-run、JSON、file並列と大容量file分割を備える | 維持 |
| file operation scripting | 対応 | `--yes --json`で非対話実行でき、終了codeで失敗を返す | operation mask等は未対応 |
| script file／batch automation | 部分 | Snippetsと`sshc ssh --non-interactive`はremote command用 | file transfer planは未対応 |
| .NET assembly／COM API | 判断 | なし | cross-platform REST/CLIを正本とし導入しない候補 |
| operation code生成 | 未対応 | なし | 既存のSFTP CLIを使った生成を検討 |
| custom file commands | 部分 | Snippetsはあるがselected path連携なし | 安全なpath変数を追加 |
| Windows shell／drag-drop integration | 判断 | browser/PWA中心 | packaged desktop連携の範囲を決める |
| PuTTY/Pageant integration | 判断 | OpenSSH config/agentを正本にする | PuTTY固有統合は導入しない候補 |
| portable configuration storage | 部分 | standalone binaryだがworkspaceは固定位置 | `--home`等の明示rootを検討 |
| master password | 対応 | vaultを保護 | 維持 |
| stored site protection | 対応 | secretはvault、OpenSSH configは通常file | 境界を維持 |
| transparent remote file encryption | 未対応 | Sync snapshot暗号化だけ | SFTP転送機能としては別設計 |
| transfer/session logging | 部分 | app logとdiagnostic report | transfer履歴と監査logを追加 |
| XML logging | 判断 | なし | JSON structured logを採用する候補 |
| configurable log retention | 部分 | 転送キューをディスクに保存し、完了・取消項目を設定した時間で自動削除する | 独立した監査ログの保持設定は未対応 |
| update check | 対応 | self-updateとrelease検証 | 維持 |
| administrative restrictions/policy | 未対応 | なし | managed deployment需要で判断 |
| selectable configuration storage | 未対応 | workspace固定 | portable modeと共通 |
| application/file associations | 未対応 | なし | desktop packagingの範囲で判断 |
| localization | 部分 | 日本語／英語 | 対象言語は需要に応じて追加 |

## 残っている作業

実装済みの操作を今後の作業へ重複して載せない。対応範囲の拡大は、上の表の未対応・部分の行で管理する。

- 日常操作: ディレクトリツリー、列幅・表示列の変更、所有者・グループによる並べ替え
- 転送設定: 転送ごとの速度上限、名前付きプリセット、任意の転送先名を作る規則
- ディレクトリ同期: ローカルを含む比較結果からのコピー、双方向同期、ミラー、変更監視。削除を伴う操作には事前の差分確認が必要
- 製品範囲の判断: SCP、FTP/FTPS、WebDAV、汎用S3ブラウザ、GSSAPI/Kerberos、外部エディタ、OSとの統合

## 完了条件

- 各行は実装commitまたは明示的な非採用decisionへ結び付ける。
- `部分`は不足項目が0になった時点で`対応`へ変更する。
- 新しいWinSCP stable release時にFeature Index、Commander Main Menu、Transfer Settings、Queue、Synchronizationの公式ページを再確認する。
- desktopだけでなく360px相当のmobileで、主要操作が欠落しないことをE2Eで確認する。
- 破壊操作、上書き、mirror、source削除はpreview/evidenceを実行対象へ結び付ける。

## WinSCP公式資料

- Feature Index: https://winscp.net/eng/docs/feature_index
- Introduction / protocols: https://winscp.net/eng/docs/introduction
- User Interfaces: https://winscp.net/eng/docs/interfaces
- Commander Main Menu: https://winscp.net/eng/docs/ui_commander_menu
- Navigating: https://winscp.net/eng/docs/task_navigate
- Background Queue: https://winscp.net/eng/docs/ui_queue
- Transfer Settings: https://winscp.net/eng/docs/ui_transfer_custom
- Transfer Settings Presets: https://winscp.net/eng/docs/transfer_settings
- Synchronizing: https://winscp.net/eng/docs/task_synchronize
- Synchronize Dialog: https://winscp.net/eng/docs/ui_synchronize
- Directory Comparison: https://winscp.net/eng/docs/task_compare_directories
- Finding Files: https://winscp.net/eng/docs/task_find
- File Properties: https://winscp.net/eng/docs/task_properties
- File Masks: https://winscp.net/eng/docs/file_mask
- Resume / Endurance: https://winscp.net/eng/docs/ui_pref_resume

## sshc側の主な根拠

- SFTP UI: `web/src/sftp/SFTPPanel.tsx`
- Transfer Manager UI: `web/src/sftp/TransferManagerList.tsx`
- browser transfer scheduler: `web/src/sftp/transferManager.ts`
- SFTP service: `internal/sftp/service.go`
- 比較: `internal/sftp/directory_comparison.go`, `internal/sftp/comparison_content.go`, `internal/sftp/local_comparison.go`
- 接続の再利用: `internal/sftp/remotepool.go`
- 速度・復旧・除外: `internal/sftp/transfer_settings.go`, `internal/sftp/transfer_limiter.go`, `internal/sftp/transfer_exclusions.go`
- リンク・所有者の変更: `internal/sftp/service_symlink.go`, `internal/sftp/service_ownership.go`
- authoritative queue: `internal/sftp/jobs*.go`
- API contract: `api/openapi.yaml`
- user documentation: `pages/features/sftp.md`, `pages/sftp/transfers.md`
- app-state Sync（SFTP directory syncとは別）: `pages/features/sync.md`
