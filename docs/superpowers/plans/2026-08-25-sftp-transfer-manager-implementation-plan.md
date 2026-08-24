# sshc v0.9.0 SFTP Transfer Manager Implementation Plan

## Goal

SFTPのfile・folder、upload・downloadを一つのTransfer Managerで扱い、同時転送数、file単位の進捗・速度・残り時間、pause／resume／retry／cancel、通知を共通化する。

Notion plan: https://app.notion.com/p/3c6613eb931781b28e5eedb88467b9b2

## Current gap

- v0.8.1はupload queueとdownload queueが別実装で、操作・status・保持規則が揃っていない。
- folder uploadはfileを個別投入するがbatchとしての完了・失敗・再実行がない。
- downloadはpause／resume／retry UIを持たず、速度・ETAも表示しない。
- 完了／失敗はSFTP画面を見ている場合しか分からない。
- backend APIはresumable upload primitiveだけで、queue jobと状態遷移を表さない。

## Design decisions

1. OpenAPIへ`SFTPTransferJob`と操作APIを追加し、Go側を状態遷移の正本にする。
2. concurrency上限はupload／download共通で2件。Web schedulerとbackend leaseの両方で検査する。
3. folder uploadは一つのbatch IDとfile子jobへ展開し、成功fileを保ったまま失敗fileだけretryする。
4. folder downloadは既存ZIP streamを共通queueへ載せる。ZIPはRange resumeせず、retry時に先頭から再実行する。
5. uploadは既存のremote part file、offset検証、target revision、atomic renameを再利用する。
6. route移動ではsingleton managerがFileとdownload chunkをmemoryに保持する。browser reload後のuploadは同じfile再選択を要求する。
7. notificationはOS permission不要のApp内`aria-live` regionとし、完了・失敗を全routeで表示する。
8. 速度は直近sampleのEWMA、ETAは残bytes／速度で計算し、sample不足時は表示しない。

## Phase 1 — API and core

- [x] job／batch型、status、direction、kind、actionを追加
- [x] create／list／action endpointを追加
- [x] invalid transition、offset rollback、size超過、active上限を拒否
- [x] retry attempt、terminal retention上限、concurrent accessのunit/race test
- [x] OpenAPIからGo／TypeScript型を再生成

## Phase 2 — Web Transfer Manager

- [x] upload／download queueを単一managerへ統合
- [x] folder batchとfile子job、failed-only retryを実装
- [x] global concurrency、pause／resume／retry／cancelを実装
- [x] download chunk保持とRange resumeを実装
- [x] bytes、速度、ETA、attempt、problemを表示
- [x] App共通notification centerを追加
- [x] localStorage restoreと同一file reattachを維持
- [x] unit/component/i18n testを追加

## Phase 3 — Verification and release

- [x] OpenSSH fixed-digest containerで64 MiB以上のfixtureをupload/download
- [x] part offset resume、atomic complete、cancel cleanup、digest一致を確認
- [x] full Go test／race、Android build、deadcodeを成功させる
- [x] Web test／typecheck／lint／production buildを成功させる
- [x] Playwright E2Eでroute移動、folder batch、failed-only retry、通知を確認
- [x] desktop／360px screenshotを目視確認
- [ ] 日本語commit、main push、annotated tag v0.9.0、Release、asset checksum、Homebrewを確認

## Acceptance criteria

- [x] file upload/downloadとfolder upload/downloadが同じqueueに表示される。
- [x] active network transferが設定上限を超えない。
- [x] fileごとに進捗、速度、残り時間を表示する。
- [x] pause／resume／retry／cancelがuploadとfile downloadで動作する。
- [x] uploadはremote part sizeから再開し、atomic renameでだけ公開する。
- [x] folder batchで失敗fileだけ再実行できる。
- [x] SFTP以外の画面でも転送を継続し、完了・失敗通知を表示する。
- [x] OpenSSH containerと64 MiB以上のfixtureでintegration testが成功する。

## Out of scope

- browser終了後もlocal file accessを自動で再取得すること
- File System Access APIによるdownloadのdisk直接書き込み
- directory ZIPのRange resume
- OS native notification permission
