# WinSCP機能差分台帳

更新日: 2026-09-01  
比較対象: sshc `main`（2026-09-01時点） / WinSCP 6.5.6

## 目的

WinSCPに存在する機能を漏れなく分類し、sshcで同じ利用目的を満たせるかを追跡する。WinSCPの画面をそのまま複製することは目的にせず、デスクトップではCommanderに近い高密度な操作、モバイルではExplorerに近い単一ペイン操作として成立させる。

状態は次の意味で使う。

- `対応`: sshcに同じ目的を満たす機能がある
- `部分`: 基本機能はあるが、WinSCPの操作またはオプションが不足している
- `未対応`: sshcに相当機能がない
- `判断`: Windows固有、別プロトコル、外部SDKなど、sshcへ入れるかを別途決める

## 結論

sshcのSFTPは、安全なアップロード／ダウンロード、フォルダー転送、複数選択、リモート編集、競合検出、バックグラウンドキューという中核を既に持つ。一方、日常のファイルマネージャーとして使う際の不足は大きく、特に次がWinSCPとの差になっている。

1. ディレクトリブックマーク／ツリー、複数SFTPタブ、リモート検索
2. 空ファイル／リンク作成、複製、別ディレクトリへの移動、プロパティ表示と一括変更
3. キューの並べ替え、帯域制限、処理全体の停止、同時数設定、高さ変更
4. 転送前オプション、timestamp／permission保持、mask、プリセット
5. ローカルとリモートの2ペイン、ディレクトリ比較、ローカル・リモート同期、変更監視
6. SFTPファイル操作のCLI／automation

## 1. ファイルパネルとナビゲーション

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| Explorer型の単一remote panel | 対応 | SFTP画面が相当 | 維持 |
| Commander型のlocal／remote 2 panel | 未対応 | remoteだけを表示 | desktop向けlocal panelを設計。mobileは単一panelを維持 |
| `..`による親directory移動 | 対応 | 一覧先頭に表示 | 維持 |
| path直接入力 | 対応 | 絶対pathを入力して移動 | 維持 |
| Back／Forward履歴 | 対応 | hostを切り替えるまでpath履歴を保持 | 維持 |
| Home directoryへ移動 | 対応 | serverのworking directoryを再解決して移動 | 維持 |
| Root directoryへ移動 | 対応 | navigation buttonまたは`/`の直接入力 | 維持 |
| directory bookmark | 未対応 | favoriteを保持しない | host単位と共通bookmarkを設計 |
| directory tree | 未対応 | 一覧だけ | desktopの任意表示として検討 |
| synchronized browsing | 未対応 | local panelがない | 2 panel導入後 |
| pathをclipboardへcopy | 未対応 | path欄から手動選択のみ | P0 |
| opposite panelのpathへ移動 | 未対応 | local panelがない | 2 panel導入後 |
| directory stateのsession別記憶 | 部分 | URLへalias/pathを反映 | sort、selection、historyも保存する |
| 複数SFTP tab | 未対応 | SFTP画面は1接続だけ | host/path状態を持つtabを検討 |
| panel内の名前filter | 対応 | 現在directoryを名前の部分一致で絞り込み | mask式は後続 |
| remote配下の再帰file検索 | 未対応 | APIなし | P1、server側上限付き検索 |
| directory cache | 未対応 | 現在pathを都度取得 | stale表示を避ける明示cacheとして設計 |
| refresh | 対応 | path横の移動操作で再取得可能 | icon／shortcutを明確化 |
| 名前sort | 対応 | 昇順／降順 | 維持 |
| 種類／拡張子sort | 部分 | entry typeでsort。拡張子sortなし | 拡張子sortを追加 |
| size sort | 対応 | あり | 維持 |
| modified time sort | 対応 | あり | 維持 |
| permissions sort | 未対応 | modeは表示するがsort不可 | P1 |
| owner／group sort | 未対応 | owner／groupを取得しない | SFTP属性対応後 |
| owner／group／link target列 | 未対応 | name、modified、size、typeだけ | 列表示設定と合わせて追加 |
| 列幅／列表示のcustomize | 未対応 | 固定 | desktop向けに追加 |
| status barと選択数／合計size | 部分 | 選択数だけtoolbarへ表示 | directory／selectionの件数とsizeを下部へ表示 |

## 2. 選択と入力操作

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| clickによる単一選択 | 対応 | あり | 維持 |
| checkboxによる複数選択 | 対応 | desktop／mobileとも対応 | 維持 |
| Select All | 対応 | header checkbox、`Ctrl/Cmd+A` | 維持 |
| keyboardで行移動／Enterで開く | 部分 | focus済み行のEnterのみ | Arrow、Home、End、Spaceを追加 |
| Shiftによるrange選択 | 対応 | 表示中の並びを基準にrange選択 | 維持 |
| Ctrl/Cmdによる追加選択 | 対応 | clickで追加／解除 | 維持 |
| 選択反転 | 対応 | 表示中のentryだけ反転 | 維持 |
| 選択解除／選択復元 | 部分 | header checkboxで解除可能。復元なし | 明示解除と直前選択の復元を追加 |
| maskで選択／解除 | 未対応 | なし | filter/mask共通構文の後に追加 |
| 同じ拡張子を選択 | 未対応 | なし | 選択menuへ追加 |
| double clickでdirectoryを開く | 対応 | あり | 維持 |
| context menu | 未対応 | 右上の操作menuのみ | desktopの右click／長押しを同じaction modelへ接続 |
| file manager shortcut（F2/F4/F5/F7/F8等） | 未対応 | 一部の一般shortcutだけ | 入力欄と衝突しない範囲で追加 |
| drag and drop upload | 対応 | file／folder、空directoryを扱う | 維持 |
| remote rowをfolderへdragして移動 | 未対応 | なし | remote move実装後 |
| queueへdropして転送 | 未対応 | なし | local panel導入後 |

## 3. remote file操作

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| folder作成 | 対応 | `+` menu | 維持 |
| 空file作成 | 未対応 | uploadまたは既存file編集だけ | P1 |
| symbolic link作成／編集 | 未対応 | linkは表示するが操作不可 | SFTP symlink/readlink APIを追加 |
| internal text editor | 対応 | UTF-8、2 MiB以下をMonaco modalで編集 | 維持 |
| external editor／Edit With | 未対応 | browserから外部editorを起動しない | desktop native連携の判断が必要 |
| file preview | 未対応 | textはeditor、それ以外はdownload | image／PDFを明示操作のmodalで表示 |
| file upload | 対応 | picker／drop、競合確認、atomic publish、resume | 維持 |
| folder upload | 対応 | relative pathと空folderを維持 | 維持 |
| file download | 対応 | revision固定、Range resume | 維持 |
| folder download | 対応 | symlinkを追わないZIP | ZIP resumeは未対応 |
| upload/download後にsourceを削除（move transfer） | 未対応 | copy transferのみ | 完了確認後だけsource削除するjobとして追加 |
| remote内copy／duplicate | 未対応 | なし | server側copy。拡張未対応時はstream copy |
| remote内move to | 部分 | 同じdirectory内のrenameだけ | destination picker付きrenameへ拡張 |
| rename | 対応 | 単一選択 | 維持 |
| delete | 対応 | 複数選択、確認、symlink非追跡 | remote recycle binは未対応 |
| clipboard copy／paste | 未対応 | file objectのclipboard操作なし | local panel／OS bridgeと合わせて設計 |
| file名をcopy | 対応 | 単一／複数を改行区切りでcopy | 維持 |
| full pathをcopy | 対応 | 単一／複数を改行区切りでcopy | 維持 |
| file URL生成 | 未対応 | なし | `sftp://`とsshc内deep linkを分けて設計 |
| properties表示 | 部分 | mode、size、mtime、typeを一覧表示 | modalでpath、revision、link target等を表示 |
| chmod | 対応 | 単一file／directory | 複数選択／再帰へ拡張 |
| chown／chgrp | 未対応 | owner/group属性なし | capability確認付きで追加 |
| timestamp変更 | 未対応 | なし | SFTP Setstat対応後 |
| propertiesの複数／再帰適用 | 未対応 | chmodも単一だけ | P1 |
| lock／unlock | 未対応 | protocol lock操作なし | server capability依存として判断 |
| directory size計算 | 未対応 | folderはsize不明 | entry／depth／byte上限付きで追加 |
| custom file command | 部分 | SnippetsとTerminalはあるが選択pathを渡せない | file path変数を安全にquoteして接続 |
| Open Terminal | 部分 | Terminal機能はあるがSFTPのhost/pathから直接開かない | 現在host/pathを引き継ぐ操作を追加 |

## 4. 転送キュー

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| background queue | 対応 | 全転送をengine authoritative queueで管理 | 維持 |
| queueをfile list下部へ表示 | 対応 | 下部dock、空なら非表示 | 維持 |
| batch全体とfile別進捗 | 対応 | batchと各jobの進捗、速度、残り時間 | 維持 |
| waiting／active／paused／failed／completed表示 | 対応 | あり | 維持 |
| pause／resume／retry／cancel | 対応 | job単位 | 維持 |
| completed消去 | 対応 | 完了・取消をまとめて消去 | 表示保持期間は未設定 |
| waiting順のmove up/down | 未対応 | FIFO固定 | P0 |
| Execute now／同時数を一時超過 | 未対応 | 最大同時数を厳守 | 必要性を判断 |
| queue processing全体の開始／停止 | 未対応 | 常時自動処理 | P0 |
| Suspend All／Resume All／Cancel All | 対応 | 下部queueの操作menuから全jobへ適用 | 維持 |
| job別speed limit | 未対応 | なし | token bucketをengine側へ追加 |
| 全体speed limit | 未対応 | なし | job別上限と合わせて追加 |
| 最大同時転送数の設定 | 部分 | engineは可変だがUI設定なし、既定2 | Preferencesへ追加 |
| 複数接続で複数fileを転送 | 対応 | jobごとに独立SFTP transport、既定2並列 | 接続再利用は限定的 |
| 1 fileを複数connectionで分割 | 未対応 | 1 job 1 stream | 大容量downloadだけ将来検討 |
| queueの折りたたみ | 対応 | headerを残して展開／折りたたみ | 維持 |
| queue高さのresize | 未対応 | 最大高さ固定 | desktopへdrag handleを追加 |
| queue file listの展開 | 部分 | batch配下へfile jobを常時表示 | 折りたたみ可能にする |
| prompt／errorの保留表示 | 部分 | overwriteは開始前確認、errorはjob表示 | queue内で再確認待ちを扱えるようにする |
| 完了時action（disconnect/sleep/shutdown） | 未対応 | なし | browser製品では通知／engine停止までを候補とする |
| 再読み込み後のqueue復元 | 部分 | engine存続中は復元。uploadは元file再選択が必要 | desktop local bridge導入時に自動復旧を検討 |
| process再起動後のqueue復元 | 未対応 | in-memory ledger | 秘密値を含めないdurable ledgerを設計 |
| transfer中の自動再接続 | 部分 | manual retryとoffset resume | bounded automatic retry/backoffを追加 |

## 5. 転送設定

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| transfer options dialog | 未対応 | 即時queue登録 | 普段は省略可能な確認sheetを追加 |
| destination directory／operation mask | 部分 | current path＋元file名のみ | 転送前にdestinationとrename maskを指定可能にする |
| default transfer settings | 未対応 | 固定 | Preferencesへ追加 |
| per-transfer settings | 未対応 | 固定 | transfer sheetへ追加 |
| named preset | 未対応 | なし | host／path rule付きpresetは後段 |
| background／foreground選択 | 判断 | sshcは常にbackground | UIをblockするforegroundは導入しない |
| binary mode | 対応 | byte streamとして転送 | 維持 |
| text／automatic transfer mode | 未対応 | newline／encoding変換なし | 誤変換防止のため明示設定だけ検討 |
| filename case変換 | 未対応 | 元名を保持 | operation maskと合わせて追加 |
| invalid filename置換 | 未対応 | 保存先browser/OSに委ねる | local panel導入時 |
| upload時permission設定 | 未対応 | server default、上書き時は既存modeを維持 | transfer optionへ追加 |
| timestamp保持 | 未対応 | upload/downloadともmtimeを復元しない | P1 |
| directory timestamp保持 | 未対応 | なし | file timestamp後 |
| downloaded read-only保持 | 未対応 | browser保存に委ねる | native local panel導入時 |
| permission error無視 | 未対応 | job failure | optionとして明示する場合のみ追加 |
| total size事前計算 | 部分 | local uploadはsize既知、remote folderはZIP作成時に確定 | directory scanの進捗を追加 |
| speed limit | 未対応 | なし | queueと共通実装 |
| include/exclude file mask | 未対応 | 全選択対象を転送 | P1 |
| newer/updated only | 未対応 | target存在時はoverwrite確認 | compare/sync基盤と共通化 |
| hidden file除外 | 未対応 | hiddenも転送 | mask presetとして追加 |
| empty directory除外 | 未対応 | 空directoryを維持 | optionとして追加 |
| overwrite／resume／append mode | 部分 | overwrite確認とpart resumeは対応。appendなし | overwrite policyとappendを追加 |
| resume threshold | 未対応 | uploadは常にpart、downloadは常にspool | 固定安全方式を維持するか判断 |
| transfer settingからautomation code生成 | 未対応 | なし | SFTP CLI実装後 |

## 6. directory比較と同期

ここでいう同期は、sshcの設定をS3へ暗号化保存する既存の「Sync」とは別物である。

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| local／remote directory比較 | 未対応 | local panelなし | 2 panelと同じscannerを使う |
| 相違fileのhighlight／選択 | 未対応 | なし | compare previewへ追加 |
| local→remote同期 | 未対応 | なし | P2 |
| remote→local同期 | 未対応 | なし | P2 |
| 双方向同期 | 未対応 | なし | conflict model確立後 |
| mirror mode | 未対応 | なし | delete preview必須で追加 |
| timestampだけ同期 | 未対応 | なし | Setstat対応後 |
| existing files only | 未対応 | なし | sync option |
| selected files only | 未対応 | なし | sync option |
| changes checklist／preview | 未対応 | SFTPにはなし | 既存Syncのpreview UI primitiveを再利用 |
| 比較criteria（time/size/checksum） | 未対応 | なし | size/timeから開始しchecksumは明示実行 |
| Keep remote directory up to date | 未対応 | local変更監視なし | desktop native/local bridge導入後 |
| synchronization in background queue | 未対応 | SFTP queueはtransferだけ | sync planをjob batchへ変換 |
| remote recycle binへのbackup | 未対応 | deleteは即時 | host単位のtrash pathを設計 |
| remote-to-remote同期 | 未対応 | なし | 低優先 |

## 7. connection、protocol、session

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
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
| session disconnect／reconnect | 未対応 | SFTP操作ごとにtransportを開く | persistent browse sessionを採用するか判断 |
| server/protocol information | 部分 | Diagnostics／effective configに分散 | SFTP contextから開けるsummaryを追加 |
| session URL／code生成 | 部分 | sshc内URLはあるが共有UIなし | copy actionを追加 |
| change password | 未対応 | なし | remote command依存のため判断 |
| install public key | 対応 | Remote Key機能 | SFTP／connection contextから導線追加 |

## 8. automation、統合、運用

| WinSCP機能 | 状態 | sshcの現状 | 実装方針 |
|---|---|---|---|
| command-line interface | 対応 | 接続、run、sync、管理CLIあり | SFTP操作は未対応 |
| SFTP upload/download CLI | 未対応 | なし | P2、JSON結果とsafe bulkを含める |
| file operation scripting | 未対応 | なし | SFTP CLIを非対話primitiveにする |
| script file／batch automation | 部分 | Snippetsと`sshc run`はremote command用 | file transfer planは未対応 |
| .NET assembly／COM API | 判断 | なし | cross-platform REST/CLIを正本とし導入しない候補 |
| operation code生成 | 未対応 | なし | SFTP CLI実装後 |
| custom file commands | 部分 | Snippetsはあるがselected path連携なし | 安全なpath変数を追加 |
| Windows shell／drag-drop integration | 判断 | browser/PWA中心 | packaged desktop連携の範囲を決める |
| PuTTY/Pageant integration | 判断 | OpenSSH config/agentを正本にする | PuTTY固有統合は導入しない候補 |
| portable configuration storage | 部分 | standalone binaryだがworkspaceは固定位置 | `--home`等の明示rootを検討 |
| master password | 対応 | vaultを保護 | 維持 |
| stored site protection | 対応 | secretはvault、OpenSSH configは通常file | 境界を維持 |
| transparent remote file encryption | 未対応 | Sync snapshot暗号化だけ | SFTP転送機能としては別設計 |
| transfer/session logging | 部分 | app logとdiagnostic report | transfer履歴と監査logを追加 |
| XML logging | 判断 | なし | JSON structured logを採用する候補 |
| configurable log retention | 未対応 | transfer履歴はengine終了まで | durable historyと合わせて追加 |
| update check | 対応 | self-updateとrelease検証 | 維持 |
| administrative restrictions/policy | 未対応 | なし | managed deployment需要で判断 |
| selectable configuration storage | 未対応 | workspace固定 | portable modeと共通 |
| application/file associations | 未対応 | なし | desktop packagingの範囲で判断 |
| localization | 部分 | 日本語／英語 | 対象言語は需要に応じて追加 |

## 実装順

### P0 — WinSCPらしい日常操作

- Back／Forward／Home／Root、path copy
- panel filter、range／追加選択、基本keyboard操作
- file名／full path copy
- queue折りたたみ／高さ変更、waiting順変更、全件pause/resume/cancel
- queue最大同時数のUI設定

### P1 — remote file managerとしての完成度

- 空fileとsymbolic linkの作成
- properties modal、複数／再帰chmod、owner/group/link target
- remote copy／duplicate／move-to、directory size
- remote search、bookmark、context menu、preview
- timestamp／permission／mask／speed limitを含むtransfer option

### P2 — Commander相当の転送workflow

- desktop local panelと安全なlocal filesystem bridge
- directory compare、local↔remote sync、mirror、preview
- Keep remote directory up to date
- SFTP CLI／JSON automation
- durable queueとautomatic reconnect

### P3 — 製品範囲を決めてから扱うもの

- SCP、FTP/FTPS、WebDAV、汎用S3 browser
- GSSAPI/Kerberos
- external editor、OS shell integration、file association
- transparent file encryption
- managed policy、portable configuration root

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
- authoritative queue: `internal/sftp/jobs.go`
- API contract: `api/openapi.yaml`
- user documentation: `pages/features/sftp.md`, `pages/sftp/transfers.md`
- app-state Sync（SFTP directory syncとは別）: `pages/features/sync.md`
